package k8sclient

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// EventComponent identifies the GCE PD CSI driver as the source of the events
// it emits.
const EventComponent = "pdcsi"

var (
	backoff = wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   2.0,
		Steps:    5,
	}

	// For testing purposes, this function can be overridden to return a fake client.
	GetClient = func() (kubernetes.Interface, error) {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		return kubernetes.NewForConfig(cfg)
	}
)

func GetNodeWithRetry(ctx context.Context, nodeName string) (*v1.Node, error) {
	if nodeName == "" {
		return nil, fmt.Errorf("node name is empty")
	}
	kubeClient, err := GetClient()
	if err != nil {
		return nil, err
	}
	return getNodeWithRetry(ctx, kubeClient, nodeName)
}

func GetStorageClassWithRetry(ctx context.Context, scName string) (*storagev1.StorageClass, error) {
	kubeClient, err := GetClient()
	if err != nil {
		return nil, err
	}
	return getStorageClassWithRetry(ctx, kubeClient, scName)
}

func GetPersistentVolumeWithRetry(ctx context.Context, pvName string) (*v1.PersistentVolume, error) {
	kubeClient, err := GetClient()
	if err != nil {
		return nil, err
	}
	return getPersistentVolumeWithRetry(ctx, kubeClient, pvName)
}

func ListPodsInNamespace(ctx context.Context, namespace string) (*v1.PodList, error) {
	kubeClient, err := GetClient()
	if err != nil {
		return nil, err
	}
	return listPodsInNamespace(ctx, kubeClient, namespace)
}

func getNodeWithRetry(ctx context.Context, kubeClient kubernetes.Interface, nodeName string) (*v1.Node, error) {
	var nodeObj *v1.Node
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(_ context.Context) (bool, error) {
		node, err := kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("Error getting node %s: %v, retrying...\n", nodeName, err)
			return false, nil
		}
		nodeObj = node
		klog.V(4).Infof("Successfully retrieved node info %s\n", nodeName)
		return true, nil
	})

	if err != nil {
		klog.Errorf("Failed to get node %s after retries: %v\n", nodeName, err)
	}
	return nodeObj, err
}

func getStorageClassWithRetry(ctx context.Context, kubeClient kubernetes.Interface, scName string) (*storagev1.StorageClass, error) {
	var scObj *storagev1.StorageClass
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(_ context.Context) (bool, error) {
		sc, err := kubeClient.StorageV1().StorageClasses().Get(ctx, scName, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("Error getting StorageClass %s: %v, retrying...\n", scName, err)
			return false, nil
		}
		scObj = sc
		klog.V(4).Infof("Successfully retrieved StorageClass info %s\n", scName)
		return true, nil
	})

	if err != nil {
		klog.Errorf("Failed to get StorageClass %s after retries: %v\n", scName, err)
	}
	return scObj, err
}

func getPersistentVolumeWithRetry(ctx context.Context, kubeClient kubernetes.Interface, pvName string) (*v1.PersistentVolume, error) {
	var pvObj *v1.PersistentVolume
	var lastErr error
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(_ context.Context) (bool, error) {
		pv, err := kubeClient.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
		if err != nil {
			lastErr = err
			// A PersistentVolume that does not exist will not start existing
			// during the backoff, and callers need that answer promptly to tell
			// it apart from an API server they could not reach.
			if !isRetriableAPIError(err) {
				return false, err
			}
			klog.Warningf("Error getting PersistentVolume %s: %v, retrying...\n", pvName, err)
			return false, nil
		}
		pvObj = pv
		klog.V(4).Infof("Successfully retrieved PersistentVolume info %s\n", pvName)
		return true, nil
	})

	if err != nil {
		// On timeout the backoff reports only that it gave up, so surface the
		// error that actually caused the retries.
		if lastErr != nil {
			err = lastErr
		}
		klog.Errorf("Failed to get PersistentVolume %s: %v\n", pvName, err)
		return nil, err
	}
	return pvObj, nil
}

func listPodsInNamespace(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (*v1.PodList, error) {
	var podList *v1.PodList
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(_ context.Context) (bool, error) {
		pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.Warningf("Error listing pods in namespace %s: %v, retrying...\n", namespace, err)
			return false, nil
		}
		podList = pods
		klog.V(4).Infof("Successfully listed pods in namespace %s\n", namespace)
		return true, nil
	})

	if err != nil {
		klog.Errorf("Failed to list pods in namespace %s after retries: %v\n", namespace, err)
	}
	return podList, err
}

// GetVolumeAttributesClassForPV returns the parameters of the
// VolumeAttributesClass currently named by the claim a PersistentVolume is bound
// to, and whether the claim names one at all.
//
// This reads the class the user asks for now rather than one recorded earlier,
// so that work the driver picks up later acts on the current intent. A user who
// clears the class on their claim cancels that work, which is the behaviour a
// recorded copy of the parameters could not offer.
//
// A volume with no PersistentVolume, no claim, or no class reports no class
// rather than failing, since none of those are errors: they all mean there is
// nothing the user is currently asking for.
func GetVolumeAttributesClassForPV(ctx context.Context, pvName string) (map[string]string, bool, error) {
	kubeClient, err := GetClient()
	if err != nil {
		return nil, false, fmt.Errorf("failed to get kubernetes client: %w", err)
	}

	pv, err := GetPersistentVolumeWithRetry(ctx, pvName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if pv.Spec.ClaimRef == nil || pv.Spec.ClaimRef.Name == "" {
		return nil, false, nil
	}

	pvc, err := kubeClient.CoreV1().PersistentVolumeClaims(pv.Spec.ClaimRef.Namespace).Get(ctx, pv.Spec.ClaimRef.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get PersistentVolumeClaim %s/%s: %w", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name, err)
	}
	if pvc.Spec.VolumeAttributesClassName == nil || *pvc.Spec.VolumeAttributesClassName == "" {
		return nil, false, nil
	}

	vacName := *pvc.Spec.VolumeAttributesClassName
	vac, err := kubeClient.StorageV1().VolumeAttributesClasses().Get(ctx, vacName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get VolumeAttributesClass %s: %w", vacName, err)
	}
	return vac.Parameters, true, nil
}

// EmitPVEvent records an event against a PersistentVolume, so that the progress
// of work the driver does outside of a single call is visible to a user running
// kubectl describe or kubectl get events.
//
// The event is created directly rather than through an event broadcaster. The
// driver emits these one at a time on volume lifecycle events, so it has no use
// for the batching and deduplication a broadcaster adds, and creating the event
// inline avoids a background sender whose buffer can silently drop events when
// the driver shuts down.
//
// Events are a best effort notification, never the record of what happened:
// that is what the annotations are for. A volume with no PersistentVolume, or an
// event that could not be created, is logged and otherwise ignored.
func EmitPVEvent(ctx context.Context, pvName string, eventType, reason, action, message string) {
	kubeClient, err := GetClient()
	if err != nil {
		klog.Warningf("Could not emit %s event for PersistentVolume %s: %v\n", reason, pvName, err)
		return
	}

	pv, err := GetPersistentVolumeWithRetry(ctx, pvName)
	if err != nil {
		klog.Warningf("Could not emit %s event for PersistentVolume %s: %v\n", reason, pvName, err)
		return
	}

	now := metav1.Now()
	event := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			// Event names have to be unique within a namespace, and this is the
			// naming the API server's own recorders use.
			Name:      fmt.Sprintf("%s.%x", pvName, now.UnixNano()),
			Namespace: metav1.NamespaceDefault,
		},
		InvolvedObject: v1.ObjectReference{
			APIVersion: "v1",
			Kind:       "PersistentVolume",
			Name:       pv.Name,
			UID:        pv.UID,
		},
		Reason:              reason,
		Message:             message,
		Action:              action,
		Type:                eventType,
		Source:              v1.EventSource{Component: EventComponent},
		ReportingController: EventComponent,
		FirstTimestamp:      now,
		LastTimestamp:       now,
		Count:               1,
	}

	// A PersistentVolume is not namespaced, so its events are recorded in the
	// default namespace, the same as the events other cluster scoped objects get.
	if _, err := kubeClient.CoreV1().Events(metav1.NamespaceDefault).Create(ctx, event, metav1.CreateOptions{}); err != nil {
		klog.Warningf("Failed to emit %s event for PersistentVolume %s: %v\n", reason, pvName, err)
		return
	}
	klog.V(4).Infof("Emitted %s event for PersistentVolume %s\n", reason, pvName)
}

// GetPVAnnotation returns the value of a single annotation on a
// PersistentVolume, and whether the annotation is set at all.
//
// A PersistentVolume that does not exist reports the annotation as unset rather
// than failing, because a disk is not required to have a PersistentVolume named
// after it. Any other failure is returned, so callers that need to know the
// state for a safety decision can tell "there is no such annotation" apart from
// "the annotation could not be read".
func GetPVAnnotation(ctx context.Context, pvName string, annotationKey string) (string, bool, error) {
	pv, err := GetPersistentVolumeWithRetry(ctx, pvName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	value, exists := pv.Annotations[annotationKey]
	return value, exists, nil
}

// SetPVAnnotation adds or updates an annotation on a PersistentVolume.
func SetPVAnnotation(ctx context.Context, pvName string, annotationKey string, annotationValue string) error {
	return patchPVAnnotation(ctx, pvName, annotationKey, &annotationValue)
}

// RemovePVAnnotation removes an annotation from a PersistentVolume. Removing an
// annotation that is not present succeeds without doing anything.
func RemovePVAnnotation(ctx context.Context, pvName string, annotationKey string) error {
	return patchPVAnnotation(ctx, pvName, annotationKey, nil)
}

// UpdatePVAnnotation adds, updates, or removes an annotation on a PersistentVolume.
// Pass "" or "null" as the annotationValue to remove the annotation.
//
// Prefer SetPVAnnotation and RemovePVAnnotation, which say which of the two is
// intended instead of overloading the value.
func UpdatePVAnnotation(ctx context.Context, pvName string, annotationKey string, annotationValue string) error {
	if annotationValue == "" || annotationValue == "null" {
		return RemovePVAnnotation(ctx, pvName, annotationKey)
	}
	return SetPVAnnotation(ctx, pvName, annotationKey, annotationValue)
}

// patchPVAnnotation sets annotationKey to annotationValue on a PersistentVolume,
// or removes it when annotationValue is nil.
//
// Both cases use a merge patch (RFC 7386), where a null value deletes the key.
// A JSON patch "remove" would be rejected with a 422 when the annotation is not
// present, so removal would burn the whole backoff on what should be a no-op.
func patchPVAnnotation(ctx context.Context, pvName string, annotationKey string, annotationValue *string) error {
	if pvName == "" {
		return fmt.Errorf("PersistentVolume name is empty")
	}
	if annotationKey == "" {
		return fmt.Errorf("annotation key is empty")
	}

	kubeClient, err := GetClient()
	if err != nil {
		return fmt.Errorf("failed to get kubernetes client: %w", err)
	}

	// A nil value marshals to null, which merge patch semantics treat as a
	// deletion of the key.
	var value interface{}
	if annotationValue != nil {
		value = *annotationValue
	}
	patchPayload, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				annotationKey: value,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to build annotation patch for PersistentVolume %s: %w", pvName, err)
	}

	var lastErr error
	err = wait.ExponentialBackoffWithContext(ctx, backoff, func(_ context.Context) (bool, error) {
		_, patchErr := kubeClient.CoreV1().PersistentVolumes().Patch(ctx, pvName, types.MergePatchType, patchPayload, metav1.PatchOptions{})
		if patchErr != nil {
			lastErr = patchErr
			// Retrying a request the API server has already rejected on its
			// merits only delays the caller, which for the conversion hooks
			// runs inside a CSI call.
			if !isRetriableAPIError(patchErr) {
				return false, patchErr
			}
			klog.Warningf("Error patching annotation %s on PersistentVolume %s: %v, retrying...\n", annotationKey, pvName, patchErr)
			return false, nil
		}
		klog.V(4).Infof("Successfully patched annotation %s on PersistentVolume %s\n", annotationKey, pvName)
		return true, nil
	})

	if err != nil {
		// On timeout the backoff reports only that it gave up, so surface the
		// error that actually caused the retries.
		if lastErr != nil {
			err = lastErr
		}
		klog.Errorf("Failed to patch annotation %s on PersistentVolume %s: %v\n", annotationKey, pvName, err)
		return fmt.Errorf("failed to patch annotation %s on PersistentVolume %s: %w", annotationKey, pvName, err)
	}
	return nil
}

// isRetriableAPIError reports whether a failed request could succeed if tried
// again. Errors where the API server understood the request and refused it are
// not retried.
func isRetriableAPIError(err error) bool {
	switch {
	case apierrors.IsNotFound(err),
		apierrors.IsForbidden(err),
		apierrors.IsUnauthorized(err),
		apierrors.IsInvalid(err),
		apierrors.IsBadRequest(err),
		apierrors.IsMethodNotSupported(err),
		apierrors.IsRequestEntityTooLargeError(err):
		return false
	default:
		return true
	}
}
