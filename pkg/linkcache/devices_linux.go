package linkcache

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/deviceutils"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/k8sclient"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/metrics"
)

const byIdDir = "/dev/disk/by-id"

func NewDeviceCacheForNode(ctx context.Context, period time.Duration, nodeName string, driverName string, deviceUtils deviceutils.DeviceUtils, metricsManager *metrics.MetricsManager) (*DeviceCache, error) {
	node, err := k8sclient.GetNodeWithRetry(ctx, nodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	return newDeviceCacheForNode(period, node, driverName, deviceUtils, metricsManager), nil
}

func NewTestDeviceCache(period time.Duration, node *v1.Node) *DeviceCache {
	return newDeviceCacheForNode(period, node, "pd.csi.storage.gke.io", deviceutils.NewDeviceUtils(), nil)
}

func NewTestNodeWithVolumes(volumes []string) *v1.Node {
	volumesInUse := make([]v1.UniqueVolumeName, len(volumes))
	for i, volume := range volumes {
		volumesInUse[i] = v1.UniqueVolumeName("kubernetes.io/csi/pd.csi.storage.gke.io^" + volume)
	}

	return &v1.Node{
		Status: v1.NodeStatus{
			VolumesInUse: volumesInUse,
		},
	}
}

func newDeviceCacheForNode(period time.Duration, node *v1.Node, driverName string, deviceUtils deviceutils.DeviceUtils, metricsManager *metrics.MetricsManager) *DeviceCache {
	deviceCache := &DeviceCache{
		symlinks:       make(map[string]deviceMapping),
		period:         period,
		deviceUtils:    deviceUtils,
		dir:            byIdDir,
		metricsManager: metricsManager,
	}

	// Look at the status.volumesInUse field.  For each, take the last section
	// of the string (after the last "/") and call AddVolume for that.
	// The expected format of the volume name is "kubernetes.io/csi/pd.csi.storage.gke.io^<volume-id>"
	for _, volume := range node.Status.VolumesInUse {
		volumeName := string(volume)
		tokens := strings.Split(volumeName, "^")
		if len(tokens) != 2 {
			klog.V(5).Infof("Skipping volume %q because splitting volumeName on `^` returns %d tokens, expected 2", volumeName, len(tokens))
			continue
		}

		// The first token is of the form "kubernetes.io/csi/<driver-name>" or just "<driver-name>".
		// We should check if it contains the driver name we are interested in.
		if !strings.Contains(tokens[0], driverName) {
			continue
		}
		klog.Infof("Adding volume %s to cache", string(volume))
		deviceCache.AddVolume(tokens[1])
	}

	return deviceCache
}

// Run since it needs an infinite loop to keep itself up to date
func (d *DeviceCache) Run(ctx context.Context) {
	klog.Infof("Starting device cache watcher for directory %s with period %s", d.dir, d.period)

	// Start the loop that runs every minute
	ticker := time.NewTicker(d.period)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			klog.Infof("Context done, stopping watcher")
			return
		case <-ticker.C:
			d.listAndUpdate()

			klog.Infof("Cache contents: %+v", d.symlinks)
		}
	}
}

// Add a volume.  This will yield a call to the filesystem to find a
// /dev/disk/by-id symlink and an evaluation of that symlink.
func (d *DeviceCache) AddVolume(volumeID string) error {
	klog.Infof("Adding volume %s to cache", volumeID)

	_, volumeKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return fmt.Errorf("error converting volume ID to key: %w", err)
	}
	deviceName, err := common.GetDeviceName(volumeKey)
	if err != nil {
		return fmt.Errorf("error getting device name: %w", err)
	}

	symlinks := d.deviceUtils.GetDiskByIdPaths(deviceName, "")
	if len(symlinks) == 0 {
		return fmt.Errorf("no symlink paths found for volume %s", volumeID)
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	// We may have multiple symlinks for a given device, we should add all of them.
	for _, symlink := range symlinks {
		realPath, err := filepath.EvalSymlinks(symlink)
		if err != nil {
			// This is not an error, as the symlink may not have been created yet.
			// Leave real_path empty; the periodic check will update it.
			klog.V(5).Infof("Could not evaluate symlink %s, will retry: %v", symlink, err)
			realPath = ""
		} else {
			klog.Infof("Found real path %s for volume %s", realPath, volumeID)
		}
		// The key is the symlink path. The value contains the evaluated
		// real path and the original volumeID for better logging.
		d.symlinks[symlink] = deviceMapping{
			volumeID: volumeID,
			realPath: realPath,
		}
		klog.V(4).Infof("Added volume %s to cache with symlink %s", volumeID, symlink)
	}

	return nil
}

// Remove the volume from the cache.
func (d *DeviceCache) RemoveVolume(volumeID string) {
	klog.Infof("Removing volume %s from cache", volumeID)
	d.mutex.Lock()
	defer d.mutex.Unlock()
	for symlink, device := range d.symlinks {
		if device.volumeID == volumeID {
			delete(d.symlinks, symlink)
		}
	}
}

func (d *DeviceCache) listAndUpdate() {
	for symlink, device := range d.symlinks {
		// Evaluate the symlink
		realPath, err := filepath.EvalSymlinks(symlink)
		if err != nil {
			klog.Warningf("Error evaluating symlink for volume %s: %v", device.volumeID, err)
			continue
		}

		// Check if the realPath has changed
		if realPath != device.realPath {
			klog.Warningf("Change in device path for volume %s (symlink: %s), previous path: %s, new path: %s", device.volumeID, symlink, device.realPath, realPath)
			if d.metricsManager != nil {
				d.metricsManager.RecordUnexpectedDevicePathChangesMetric()
			}

			// Update the cache with the new realPath
			device.realPath = realPath
			d.symlinks[symlink] = device
		}
	}
}
