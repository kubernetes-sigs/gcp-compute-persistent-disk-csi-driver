package taint

import (
	"context"
	"fmt"

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
)

const (
	// DriverName must match the name returned by the CSI GetPluginInfo call
	DriverName = "pd.csi.storage.gke.io"

	// StartupTaintKey is the key that blocks scheduling until the driver is ready
	StartupTaintKey = "pd.csi.storage.gke.io/startup"

	controllerName = "node-taint-controller"
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
	return nil
}

// ReconcileNodeTaint reconciles a CSINode object
type ReconcileNodeTaint struct {
	client client.Client
}

// Reconcile reads the state of the cluster for a CSINode object and removes the taint if the driver is present
func (r *ReconcileNodeTaint) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// Fetch the CSINode instance
	csiNode := &storagev1.CSINode{}
	err := r.client.Get(ctx, request.NamespacedName, csiNode)
	if err != nil {
		if errors.IsNotFound(err) {
			// CSINode deleted, nothing to do
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Check if OUR driver is registered in this CSINode
	// If the driver is not yet in the list, it means the node-driver-registrar hasn't finished.
	// We return nil and wait for the next update to the CSINode object.
	if !isDriverRegistered(csiNode, DriverName) {
		klog.V(4).Infof("Driver %s not yet registered on CSINode %s", DriverName, request.Name)
		return reconcile.Result{}, nil
	}

	// Fetch the corresponding Node
	// CSINode Name is guaranteed to match Node Name
	node := &corev1.Node{}
	err = r.client.Get(ctx, request.NamespacedName, node)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Warningf("Node %s not found for existing CSINode", request.Name)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Check for Taint and Remove it
	if hasStartupTaint(node) {
		klog.Infof("Driver registered. Removing startup taint from Node %s", node.Name)

		// We use a MergeFrom patch here to avoid conflicts with other updates (status updates, heartbeat, etc)
		// This calculates the diff between originalNode and the modified state
		originalNode := node.DeepCopy()

		newTaints := make([]corev1.Taint, 0)
		for _, t := range node.Spec.Taints {
			if t.Key == StartupTaintKey {
				continue // Filter out our taint
			}
			newTaints = append(newTaints, t)
		}
		node.Spec.Taints = newTaints

		// Issue Patch
		err = r.client.Patch(ctx, node, client.MergeFrom(originalNode))
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to remove taint from node %s: %v", node.Name, err)
		}
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
		if t.Key == StartupTaintKey {
			return true
		}
	}
	return false
}
