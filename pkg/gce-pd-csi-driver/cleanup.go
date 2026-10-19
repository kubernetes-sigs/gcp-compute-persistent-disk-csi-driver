package gceGCEDriver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	computev1 "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	corev1 "k8s.io/api/core/v1"
	kubeApiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
)

const (
	selfLinkPrefix = "https://www.googleapis.com/compute/v1/"

	pvcNamespaceKey = "kubernetes.io/created-for/pvc/namespace"
	pvcNameKey      = "kubernetes.io/created-for/pvc/name"
	pvNameKey       = "kubernetes.io/created-for/pv/name"
	createdByKey    = "storage.gke.io/created-by"
)

// VerifyClusterDisks checks validateds GCE disks associated with the cluster for potential leaked resources,
// and deletes them if enableDiskCleanup is true.
func (gceCS *GCEControllerServer) VerifyClusterDisks(ctx context.Context, enableDiskCleanup bool) error {
	klog.V(4).Infof("Starting disk leak cleanup cycle")

	cfg, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes config: %w", err)
	}
	kc, err := client.New(cfg, client.Options{})
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	disks, err := gceCS.listDisks(ctx)
	if err != nil {
		return fmt.Errorf("failed to list disks: %w", err)
	}

	// Avoid blocking controller start up by handling the disks asynchronously. Since
	// handle disk performs a no-op if a PVC exists, race conditions with the driver is a
	// a non-issue.
	go func() {
		bgCtx := context.WithoutCancel(ctx)
		for _, disk := range disks {
			if err := handleDisk(bgCtx, disk, kc, gceCS, enableDiskCleanup); err != nil {
				klog.Errorf("failed to handle potential leaked disk %v", disk.SelfLink)
			}
		}
	}()
	return nil
}

func (gceCS *GCEControllerServer) listDisks(ctx context.Context) ([]*computev1.Disk, error) {
	if gceCS.clusterOwnershipID == "" {
		return nil, fmt.Errorf("missing cluster ownership identifier")
	}
	clusterFilter := labelFilter(constants.ClusterIDLabel, gceCS.clusterOwnershipID)
	statusFilter := labelFilter(constants.VolumePublishStatus, constants.ProvisioningStatus)
	filter := fmt.Sprintf("%s AND %s", clusterFilter, statusFilter)
	disks, _, err := gceCS.CloudProvider.ListDisksWithFilter(ctx, []googleapi.Field{}, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list disks: %w", err)
	}
	klog.V(4).Infof("Listed %d disks with filter: %s", len(disks), filter)
	return disks, nil
}

func handleDisk(ctx context.Context, disk *computev1.Disk, kc client.Client, gceCS *GCEControllerServer, enableDiskCleanup bool) error {
	volumeID, tags, err := getDiskMeta(disk)
	if err != nil {
		return fmt.Errorf("failed to parse disk meta: %v", err)
	}

	project, volKey, err := common.VolumeIDToKey(strings.TrimPrefix(disk.SelfLink, selfLinkPrefix))
	if err != nil {
		return fmt.Errorf("failed to parse volume ID: %w", err)
	}

	hasPV, hasPVC, err := validateAssociatedObjects(ctx, kc, tags)
	if err != nil {
		return fmt.Errorf("failed to validate associated objects: %v", err)
	}

	// Refetch the disk to check current state since time may have passed since the initial list.
	curDisk, err := gceCS.CloudProvider.GetDisk(ctx, project, volKey)
	if err != nil {
		return fmt.Errorf("failed to get current disk state for %s: %v", volKey, err)
	}
	lbls := curDisk.GetLabels()
	if lbls == nil {
		lbls = make(map[string]string)
	}
	// If current disk now has ProvisionedStatus, skip further processing.
	if lbls[constants.VolumePublishStatus] == constants.ProvisionedStatus {
		klog.V(4).Infof("Disk %s now has ProvisionedStatus, skipping processing", volumeID)
		return nil
	}

	// Apply "provisioned" status to disks with existing PV.
	if hasPV {
		lbls[constants.VolumePublishStatus] = constants.ProvisionedStatus
		err = gceCS.CloudProvider.SetDiskLabels(ctx, project, volKey, curDisk, lbls)
		if err != nil {
			return fmt.Errorf("failed to update status label for %s: %v", volKey, err)
		}
	}

	// Skip cleanup for disks with existing PVC, as they may be in the process of being provisioned.
	if hasPVC {
		return nil
	}

	// Delete disks with no associated PV/PVC
	klog.Warningf("Disk %s is leaked", volumeID)
	if enableDiskCleanup {
		err = gceCS.CloudProvider.DeleteDisk(ctx, project, volKey)
		if err != nil {
			return fmt.Errorf("failed to delete leaked disk %v: %v", volKey, err)
		}
	}
	return nil
}

func labelFilter(key, value string) string {
	return fmt.Sprintf("labels.%s=%s", key, value)
}

func getDiskMeta(disk *computev1.Disk) (string, map[string]string, error) {
	volumeID := strings.TrimPrefix(disk.SelfLink, selfLinkPrefix)

	var tags map[string]string
	if err := json.Unmarshal([]byte(disk.Description), &tags); err != nil {
		return "", nil, fmt.Errorf("failed to unmarshal disk description for disk: %w", err)
	}

	requiredKeys := []string{pvcNamespaceKey, pvcNameKey, pvNameKey, createdByKey}
	for _, key := range requiredKeys {
		if tags[key] == "" {
			return "", nil, fmt.Errorf("disk description is missing value %s", key)
		}
	}
	if tags[createdByKey] != constants.DriverName {
		return "", nil, fmt.Errorf("disk is not managed by CSI driver")
	}

	return volumeID, tags, nil
}

func validateAssociatedObjects(ctx context.Context, c client.Client, tags map[string]string) (bool, bool, error) {
	pvName := tags[pvNameKey]
	pvcName := tags[pvcNameKey]
	pvcNamespace := tags[pvcNamespaceKey]

	var pvExists, pvcExists bool

	err := c.Get(ctx, client.ObjectKey{Name: pvName}, &corev1.PersistentVolume{})
	if err == nil {
		pvExists = true
	} else if !kubeApiErrors.IsNotFound(err) {
		return false, false, fmt.Errorf("failed to get PV %s: %v", pvName, err)
	}

	err = c.Get(ctx, client.ObjectKey{Namespace: pvcNamespace, Name: pvcName}, &corev1.PersistentVolumeClaim{})
	if err == nil {
		pvcExists = true
	} else if !kubeApiErrors.IsNotFound(err) {
		return false, false, fmt.Errorf("failed to get PVC %s/%s: %v", pvcNamespace, pvcName, err)
	}

	return pvExists, pvcExists, nil
}
