/*
Copyright 2018 The Kubernetes Authors.

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

package deviceutils

import (
	"k8s.io/mount-utils"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/resizefs"
)

type fakeDeviceUtils struct {
	skipResize bool
}

var _ DeviceUtils = &fakeDeviceUtils{}

func NewFakeDeviceUtils(skipResize bool) *fakeDeviceUtils {
	return &fakeDeviceUtils{
		skipResize: skipResize,
	}
}

// Returns list of all /dev/disk/by-id/* paths for given PD.
func (m *fakeDeviceUtils) GetDiskByIdPaths(pdName string, partition string) []string {
	return []string{"/dev/disk/fake-path"}
}

// Returns the first path that exists, or empty string if none exist.
func (m *fakeDeviceUtils) VerifyDevicePath(devicePaths []string, diskName string) (string, error) {
	// Return any random device path to use as mount source
	return "/dev/disk/fake-path", nil
}

func (_ *fakeDeviceUtils) DisableDevice(devicePath string) error {
	// No-op for testing.
	return nil
}

func (du *fakeDeviceUtils) Resize(resizer resizefs.Resizefs, devicePath string, deviceMountPath string) (bool, error) {
	if du.skipResize {
		return false, nil
	}
	return resizer.Resize(devicePath, deviceMountPath)
}

func (_ *fakeDeviceUtils) IsDeviceFilesystemInUse(mounter *mount.SafeFormatAndMount, devicePath, devFsPath string) (bool, error) {
	// We don't support checking if a device filesystem is captured elsewhere by the system
	// Return false, to skip this check.
	return false, nil
}
