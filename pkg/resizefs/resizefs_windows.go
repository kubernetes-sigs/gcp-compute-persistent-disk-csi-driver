//go:build windows

/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resizefs

import (
	"context"
	"fmt"
	"strings"

	volumeapiv1 "github.com/kubernetes-csi/csi-proxy/client/api/volume/v1"
	volumeapiv1beta1 "github.com/kubernetes-csi/csi-proxy/client/api/volume/v1beta1"

	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
	mounter "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"
)

var _ Resizefs = &resizeFs{}

// ResizeFs Provides support for resizing file systems
type resizeFs struct {
	mounter *mount.SafeFormatAndMount
}

// NewResizeFs returns new instance of resizer
func NewResizeFs(mounter *mount.SafeFormatAndMount) *resizeFs {
	return &resizeFs{mounter: mounter}
}

// resize perform resize of file system
func (resizefs *resizeFs) Resize(devicePath string, deviceMountPath string) (bool, error) {
	switch resizefs.mounter.Interface.(type) {
	case *mounter.CSIProxyMounterV1:
		return resizefs.resizeV1(devicePath, deviceMountPath)
	case *mounter.CSIProxyMounterV1Beta:
		return resizefs.resizeV1Beta(devicePath, deviceMountPath)
	}
	return false, fmt.Errorf("resize.mounter.Interface is not valid")
}

func (resizefs *resizeFs) resizeV1(devicePath string, deviceMountPath string) (bool, error) {
	klog.V(3).Infof("resizeFS.Resize - Expanding mounted volume %s", deviceMountPath)

	proxy := resizefs.mounter.Interface.(*mounter.CSIProxyMounterV1)

	idRequest := &volumeapiv1.GetVolumeIDFromTargetPathRequest{
		TargetPath: deviceMountPath,
	}
	idResponse, err := proxy.VolumeClient.GetVolumeIDFromTargetPath(context.Background(), idRequest)
	if err != nil {
		return false, err
	}
	volumeId := idResponse.GetVolumeId()

	request := &volumeapiv1.ResizeVolumeRequest{
		VolumeId: volumeId,
	}
	_, err = proxy.VolumeClient.ResizeVolume(context.Background(), request)
	if err != nil {
		// Check if this is the Windows error indicating partition is already at correct size
		if strings.Contains(err.Error(), "The size of the extent is less than the minimum of 1MB") {
			klog.V(3).Infof("Partition is already at target size (extent difference < 1MB), treating as success: %v", err)
			return false, nil // Return false to indicate no resize was needed, but no error
		}
		return false, err
	}
	return true, nil
}

// resize perform resize of file system
func (resizefs *resizeFs) resizeV1Beta(devicePath string, deviceMountPath string) (bool, error) {
	klog.V(3).Infof("resizeFS.Resize - Expanding mounted volume %s", deviceMountPath)

	proxy := resizefs.mounter.Interface.(*mounter.CSIProxyMounterV1Beta)

	idRequest := &volumeapiv1beta1.VolumeIDFromMountRequest{
		Mount: deviceMountPath,
	}
	idResponse, err := proxy.VolumeClient.GetVolumeIDFromMount(context.Background(), idRequest)
	if err != nil {
		return false, err
	}
	volumeId := idResponse.GetVolumeId()

	request := &volumeapiv1beta1.ResizeVolumeRequest{
		VolumeId: volumeId,
	}
	_, err = proxy.VolumeClient.ResizeVolume(context.Background(), request)
	if err != nil {
		// Check if this is the Windows error indicating partition is already at correct size
		if strings.Contains(err.Error(), "The size of the extent is less than the minimum of 1MB") {
			klog.V(3).Infof("Partition is already at target size (extent difference < 1MB), treating as success: %v", err)
			return false, nil // Return false to indicate no resize was needed, but no error
		}
		return false, err
	}
	return true, nil
}
