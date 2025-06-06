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
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/k8sclient"
)

const byIdDir = "/dev/disk/by-id"

func NewDeviceCacheForNode(ctx context.Context, period time.Duration, nodeName string) (*DeviceCache, error) {
	node, err := k8sclient.GetNodeWithRetry(ctx, nodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	return newDeviceCacheForNode(period, node), nil
}

func TestDeviceCache(period time.Duration, node *v1.Node) *DeviceCache {
	return newDeviceCacheForNode(period, node)
}

func TestNodeWithVolumes(volumes []string) *v1.Node {
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

func newDeviceCacheForNode(period time.Duration, node *v1.Node) *DeviceCache {
	deviceCache := &DeviceCache{
		volumes: make(map[string]deviceMapping),
		period:  period,
		dir:     byIdDir,
	}

	// Look at the status.volumesInUse field.  For each, take the last section
	// of the string (after the last "/") and call AddVolume for that
	for _, volume := range node.Status.VolumesInUse {
		klog.Infof("Adding volume %s to cache", string(volume))
		vID, err := pvNameFromVolumeID(string(volume))
		if err != nil {
			klog.Warningf("failure to retrieve name, skipping volume %q: %v", string(volume), err)
			continue
		}
		deviceCache.AddVolume(vID)
	}

	return deviceCache
}

func pvNameFromVolumeID(volumeID string) (string, error) {
	tokens := strings.Split(volumeID, "^")
	if len(tokens) != 2 {
		return "", fmt.Errorf("invalid volume ID, split on `^` returns %d tokens, expected 2", len(tokens))
	}
	return tokens[1], nil
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

			klog.Infof("Cache contents: %+v", d.volumes)
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

	// Look at the dir for a symlink that matches the pvName
	symlink := filepath.Join(d.dir, "google-"+deviceName)
	klog.Infof("Looking for symlink %s", symlink)

	realPath, err := filepath.EvalSymlinks(symlink)
	if err != nil {
		klog.Warningf("Error evaluating symlink for volume %s: %v", volumeID, err)
		return nil
	}

	klog.Infof("Found real path %s for volume %s", realPath, volumeID)

	d.volumes[volumeID] = deviceMapping{
		symlink:  symlink,
		realPath: realPath,
	}

	return nil
}

// Remove the volume from the cache.
func (d *DeviceCache) RemoveVolume(volumeID string) {
	klog.Infof("Removing volume %s from cache", volumeID)
	delete(d.volumes, volumeID)
}

func (d *DeviceCache) listAndUpdate() {
	for volumeID, device := range d.volumes {
		// Evaluate the symlink
		realPath, err := filepath.EvalSymlinks(device.symlink)
		if err != nil {
			klog.Warningf("Error evaluating symlink for volume %s: %v", volumeID, err)
			continue
		}

		// Check if the realPath has changed
		if realPath != device.realPath {
			klog.Warningf("Change in device path for volume %s (symlink: %s), previous path: %s, new path: %s", volumeID, device.symlink, device.realPath, realPath)

			// Update the cache with the new realPath
			device.realPath = realPath
			d.volumes[volumeID] = device
		}
	}
}
