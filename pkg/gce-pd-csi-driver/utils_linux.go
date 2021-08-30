//go:build !windows

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

package gceGCEDriver

import (
	"fmt"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/utils/mount"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

func getDevicePath(ns *GCENodeServer, volumeID, partition string) (string, error) {
	volumeKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return "", err
	}
	deviceName, err := common.GetDeviceName(volumeKey)
	if err != nil {
		return "", fmt.Errorf("error getting device name: %v", err)
	}
	devicePaths := ns.DeviceUtils.GetDiskByIdPaths(deviceName, partition)
	devicePath, err := ns.DeviceUtils.VerifyDevicePath(devicePaths, deviceName)
	if err != nil {
		return "", status.Error(codes.Internal, fmt.Sprintf("error verifying GCE PD (%q) is attached: %v", deviceName, err))
	}
	if devicePath == "" {
		return "", status.Error(codes.Internal, fmt.Sprintf("Unable to find device path out of attempted paths: %v", devicePaths))
	}
	return devicePath, nil
}

func formatAndMount(source, target, fstype string, options []string, m *mount.SafeFormatAndMount) error {
	return m.FormatAndMount(source, target, fstype, options)
}

func preparePublishPath(path string, m *mount.SafeFormatAndMount) error {
	return os.MkdirAll(path, 0750)
}

func prepareStagePath(path string, m *mount.SafeFormatAndMount) error {
	return os.MkdirAll(path, 0750)
}

func cleanupPublishPath(path string, m *mount.SafeFormatAndMount) error {
	return mount.CleanupMountPoint(path, m, false /* bind mount */)
}

func cleanupStagePath(path string, m *mount.SafeFormatAndMount) error {
	return cleanupPublishPath(path, m)
}
