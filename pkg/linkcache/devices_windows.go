//go:build windows

package linkcache

import (
	"context"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/deviceutils"
)

func NewDeviceCacheForNode(ctx context.Context, period time.Duration, nodeName string, driverName string, deviceUtils deviceutils.DeviceUtils) (*DeviceCache, error) {
	klog.Infof("NewDeviceCacheForNode is not implemented for Windows")
	return nil, nil
}

func (d *DeviceCache) Run(ctx context.Context) {
	// Not implemented for Windows
}

func (d *DeviceCache) AddVolume(volumeID string) error {
	klog.Infof("AddVolume is not implemented for Windows")
	return nil
}

func (d *DeviceCache) RemoveVolume(volumeID string) {
	// Not implemented for Windows
}
