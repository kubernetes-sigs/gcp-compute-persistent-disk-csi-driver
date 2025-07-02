//go:build windows

package linkcache

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"
)

func NewDeviceCacheForNode(ctx context.Context, period time.Duration, nodeName string) (*DeviceCache, error) {
	klog.Warningf("NewDeviceCacheForNode is not implemented for Windows")
	return nil, nil
}

func (d *DeviceCache) Run(ctx context.Context) {
	// Not implemented for Windows
}

func (d *DeviceCache) AddVolume(volumeID string) error {
	return fmt.Errorf("AddVolume is not implemented for Windows")
}

func (d *DeviceCache) RemoveVolume(volumeID string) {
	// Not implemented for Windows
}
