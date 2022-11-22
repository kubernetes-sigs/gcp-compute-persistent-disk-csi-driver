//go:build linux

/*
Copyright 2017 The Kubernetes Authors.

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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
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

// Resize perform resize of file system
func (resizefs *resizeFs) Resize(devicePath, deviceMountPath string) (needResize bool, err error) {
	resizer := mount.NewResizeFs(resizefs.mounter.Exec)

	klog.V(4).Infof("Checking if filesystem needs to be resized. Device: %s Mountpoint: %s", devicePath, deviceMountPath)
	if needResize, err = resizer.NeedResize(devicePath, deviceMountPath); err != nil {
		err = status.Errorf(codes.Internal, "Could not determine if filesystem %q needs to be resized: %v", deviceMountPath, err)
		return
	}

	if needResize {
		klog.V(4).Infof("Resizing filesystem. Device: %s Mountpoint: %s", devicePath, deviceMountPath)
		if _, err = resizer.Resize(devicePath, deviceMountPath); err != nil {
			err = status.Errorf(codes.Internal, "Failed to resize filesystem %q: %v", deviceMountPath, err)
			return
		}
	}

	return
}
