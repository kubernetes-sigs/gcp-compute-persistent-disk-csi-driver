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

package mountmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cloud.google.com/go/compute/metadata"
	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

// CSIProxyMounter is the mounter interface exposed as a utility to
// internal methods
type CSIProxyMounter interface {
	mount.Interface

	// GetAPIVersions returns the versions of the client APIs this mounter is using.
	GetAPIVersions() string

	// Delete the given directory with Pod context. CSI proxy does a check for path prefix
	// based on context
	RemovePodDir(target string) error

	// UnmountDevice uses target path to find the volume id first, and then
	// call DismountVolume through csi-proxy. If succeeded, it will delete the given path
	// at last step. CSI proxy does a check for path prefix
	// based on context
	UnmountDevice(target string) error

	// GetDiskNumber finds the disk number of the given device name
	GetDiskNumber(deviceName string, partition string, volumeKey string) (string, error)

	// FormatAndMount accepts the source disk number, target path to mount, the fstype to format with and options to be used.
	// After formatting, it will mount the disk to target path on the host
	FormatAndMount(source string, target string, fstype string, options []string) error

	// IsMountPointMatch checks if the mount point matches the directory `dir`
	IsMountPointMatch(mp mount.MountPoint, dir string) bool

	// ExistsPath checks if a path exists.
	// Unlike util ExistsPath, this call does not perform follow link.
	ExistsPath(path string) (bool, error)

	// GetDiskTotalBytes gets the total size of a disk
	GetDiskTotalBytes(devicePath string) (int64, error)
}

func NewSafeMounter(int, time.Duration) (*mount.SafeFormatAndMount, error) {
	csiProxyMounterV1, err := NewCSIProxyMounterV1()
	if err == nil {
		klog.V(4).Infof("using CSIProxyMounterV1, %s", csiProxyMounterV1.GetAPIVersions())
		return &mount.SafeFormatAndMount{
			Interface: csiProxyMounterV1,
			Exec:      utilexec.New(),
		}, nil
	}
	klog.V(4).Infof("failed to connect to csi-proxy v1 with error=%v, will try with v1Beta", err.Error())

	csiProxyMounterV1Beta, err := NewCSIProxyMounterV1Beta()
	if err == nil {
		klog.V(4).Infof("using CSIProxyMounterV1Beta, %s", csiProxyMounterV1Beta.GetAPIVersions())
		return &mount.SafeFormatAndMount{
			Interface: csiProxyMounterV1Beta,
			Exec:      utilexec.New(),
		}, nil
	}
	klog.V(4).Infof("failed to connect to csi-proxy v1beta with error=%v", err.Error())
	return nil, err
}

type googleCloudDisk struct {
	DeviceName              string `json:"deviceName"`
	Index                   int    `json:"index"`
	Interface               string `json:"interface"`
	Mode                    string `json:"mode"`
	NvmeNamespaceIdentifier uint64 `json:"nvmeNamespaceIdentifier"`
	Type                    string `json:"type"`
}

// attachedDisks returns the list of disks attached to the instance from which
// the metadata server is called.
func attachedDisks() ([]googleCloudDisk, error) {
	disksResp, err := metadata.GetWithContext(context.Background(), "instance/disks/?recursive=true")
	if err != nil {
		return nil, fmt.Errorf("failed to get disks using metadata package: %v", err)
	}

	var disks []googleCloudDisk
	if err := json.Unmarshal([]byte(disksResp), &disks); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %v", err)
	}

	klog.V(4).Infof("Retrieved %d disks from Google Cloud metadata", len(disks))
	return disks, nil
}
