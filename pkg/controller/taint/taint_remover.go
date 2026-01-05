package taint

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
)

const (
	// DriverName must match the name returned by the CSI GetPluginInfo call
	DriverName = "pd.csi.storage.gke.io"

	controllerName = "node-taint-controller"

	// controllerTurnupGracePeriod defines how long the controller should wait before taking aggressive action to
	// remove the taint from an orphaned node. This may happen if the pdcsi controller never spins up healthy or
	// is deleted during turnup. 12 minutes is quite conservative to account for possibly slow node image pulls
	// (e.g. windows). PDCSI driver usually turnsup in <30 seconds.
	controllerTurnupGracePeriod = 12 * time.Minute

	// taintRemovalRetry is how long we should wait before retrying, in the event the CSINode is not yet available
	taintRemovalRetry = 3 * time.Second
)

// Add creates a new Taint Controller and adds it to the Manager.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNodeTaint{client: mgr.GetClient()}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to CSINode objects
	err = c.Watch(source.Kind(mgr.GetCache(), client.Object(&storagev1.CSINode{}), &handler.EnqueueRequestForObject{}))
	if err != nil {
		return err
	}

	// Watch for changes to Node objects to stay level-triggered on the taints themselves
	err = c.Watch(source.Kind(mgr.GetCache(), client.Object(&corev1.Node{}), &handler.EnqueueRequestForObject{}))
	if err != nil {
		return err
	}

	return nil
}

// ReconcileNodeTaint reconciles a Node object against its corresponding CSINode
type ReconcileNodeTaint struct {
	client client.Client
}

func (r *ReconcileNodeTaint) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := klog.FromContext(ctx).WithValues("node", request.Name)

	node := &corev1.Node{}
	err := r.client.Get(ctx, request.NamespacedName, node)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		logger.Error(err, "Failed to fetch Node")
		return reconcile.Result{}, err
	}

	// If the Node isn't tainted, we have nothing to do
	if !hasStartupTaint(node) {
		return reconcile.Result{}, nil
	}

	// The Node IS tainted. Fetch the CSINode to see if the driver is ready.
	csiNode := &storagev1.CSINode{}
	err = r.client.Get(ctx, request.NamespacedName, csiNode)

	shouldRemoveTaint := false

	if err != nil {
		if errors.IsNotFound(err) {
			// Check how old the Node is. If its too old, it's likely orphaned and we should remove the taint to allow scheduling.
			nodeAge := time.Since(node.CreationTimestamp.Time)

			if nodeAge < controllerTurnupGracePeriod {
				logger.V(4).Info("CSINode not found, but Node is new. Waiting for driver bootstrap.", "age", nodeAge.Round(time.Second))
				return reconcile.Result{RequeueAfter: taintRemovalRetry}, nil
			}

			logger.Info("CSINode missing for old Node. Taking safe approach to remove orphaned taint.", "age", nodeAge.Round(time.Second))
			shouldRemoveTaint = true
		} else {
			logger.Error(err, "Failed to fetch CSINode")
			return reconcile.Result{}, err
		}
	} else {
		// CSINode exists. Make sure our specific driver is registered.
		if isDriverRegistered(csiNode, DriverName) {
			logger.Info("Driver is registered. Removing startup taint.", "driver", DriverName)
			shouldRemoveTaint = true
		} else {
			logger.V(4).Info("Driver not yet registered on CSINode. Leaving taint intact.", "driver", DriverName)
			return reconcile.Result{RequeueAfter: taintRemovalRetry}, nil
		}
	}

	// Remove the taint if the conditions were met
	if shouldRemoveTaint {
		originalNode := node.DeepCopy()

		newTaints := make([]corev1.Taint, 0)
		for _, t := range node.Spec.Taints {
			if t.Key == constants.StartupTaintKey {
				continue
			}
			newTaints = append(newTaints, t)
		}
		node.Spec.Taints = newTaints

		// Issue Patch
		err = r.client.Patch(ctx, node, client.MergeFrom(originalNode))
		if err != nil {
			logger.Error(err, "Failed to remove taint from node")
			return reconcile.Result{}, fmt.Errorf("failed to remove taint from node %s (may indicate permission issues or node deletion in progress): %w", node.Name, err)
		}
		logger.Info("Successfully removed startup taint")
	}

	return reconcile.Result{}, nil
}

func isDriverRegistered(csiNode *storagev1.CSINode, driverName string) bool {
	for _, driver := range csiNode.Spec.Drivers {
		if driver.Name == driverName {
			return true
		}
	}
	return false
}

func hasStartupTaint(node *corev1.Node) bool {
	for _, t := range node.Spec.Taints {
		if t.Key == constants.StartupTaintKey && t.Effect == constants.StartupTaintEffect && t.Value == constants.StartupTaintValue {
			return true
		}
	}
	return false
}
