// +build windows

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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	diskapi "github.com/kubernetes-csi/csi-proxy/client/api/disk/v1beta1"
	diskclient "github.com/kubernetes-csi/csi-proxy/client/groups/disk/v1beta1"

	fsapi "github.com/kubernetes-csi/csi-proxy/client/api/filesystem/v1beta1"
	fsclient "github.com/kubernetes-csi/csi-proxy/client/groups/filesystem/v1beta1"

	volumeapi "github.com/kubernetes-csi/csi-proxy/client/api/volume/v1beta1"
	volumeclient "github.com/kubernetes-csi/csi-proxy/client/groups/volume/v1beta1"

	"k8s.io/klog"
	utilexec "k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

var _ mount.Interface = &CSIProxyMounter{}

type CSIProxyMounter struct {
	FsClient     *fsclient.Client
	DiskClient   *diskclient.Client
	VolumeClient *volumeclient.Client
}

func NewCSIProxyMounter() (*CSIProxyMounter, error) {
	fsClient, err := fsclient.NewClient()
	if err != nil {
		return nil, err
	}
	diskClient, err := diskclient.NewClient()
	if err != nil {
		return nil, err
	}
	volumeClient, err := volumeclient.NewClient()
	if err != nil {
		return nil, err
	}
	return &CSIProxyMounter{
		FsClient:     fsClient,
		DiskClient:   diskClient,
		VolumeClient: volumeClient,
	}, nil
}

func NewSafeMounter() (*mount.SafeFormatAndMount, error) {
	csiProxyMounter, err := NewCSIProxyMounter()
	if err != nil {
		return nil, err
	}
	return &mount.SafeFormatAndMount{
		Interface: csiProxyMounter,
		Exec:      utilexec.New(),
	}, nil
}

// Mount just creates a soft link at target pointing to source.
func (mounter *CSIProxyMounter) Mount(source string, target string, fstype string, options []string) error {
	return mounter.MountSensitive(source, target, fstype, options, nil /* sensitiveOptions */)
}

// MountSensitive is the same as Mount() but this method allows
// sensitiveOptions to be passed in a separate parameter from the normal
// mount options and ensures the sensitiveOptions are never logged.
// Since Mount here is just create a synlink, so options and sensitiveOptions
// are not used here
func (mounter *CSIProxyMounter) MountSensitive(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	// Mount is called after the format is done.
	// TODO: Confirm that fstype is empty.
	// Call the LinkPath CSI proxy from the source path to the target path
	parentDir := filepath.Dir(target)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return err
	}
	linkRequest := &fsapi.LinkPathRequest{
		SourcePath: mount.NormalizeWindowsPath(source),
		TargetPath: mount.NormalizeWindowsPath(target),
	}
	response, err := mounter.FsClient.LinkPath(context.Background(), linkRequest)
	if err != nil {
		return err
	}
	if response.Error != "" {
		return errors.New(response.Error)
	}
	return nil
}

// Delete the given directory with Pod context. CSI proxy does a check for path prefix
// based on context
func (mounter *CSIProxyMounter) RemovePodDir(target string) error {
	rmdirRequest := &fsapi.RmdirRequest{
		Path:    mount.NormalizeWindowsPath(target),
		Context: fsapi.PathContext_POD,
		Force:   true,
	}
	_, err := mounter.FsClient.Rmdir(context.Background(), rmdirRequest)
	if err != nil {
		return err
	}
	return nil
}

// UnmountDevice uses target path to find the volume id first, and then
// call DismountVolume through csi-proxy. If succeeded, it will delete the given path
// at last step. CSI proxy does a check for path prefix
// based on context
func (mounter *CSIProxyMounter) UnmountDevice(target string) error {
	target = mount.NormalizeWindowsPath(target)
	if exists, err := mounter.ExistsPath(target); !exists {
		return err
	}
	idRequest := &volumeapi.VolumeIDFromMountRequest{
		Mount: target,
	}
	idResponse, err := mounter.VolumeClient.GetVolumeIDFromMount(context.Background(), idRequest)
	if err != nil {
		return err
	}
	volumeId := idResponse.GetVolumeId()

	dismountRequest := &volumeapi.DismountVolumeRequest{
		Path:     target,
		VolumeId: volumeId,
	}
	_, err = mounter.VolumeClient.DismountVolume(context.Background(), dismountRequest)
	if err != nil {
		return err
	}
	rmdirRequest := &fsapi.RmdirRequest{
		Path:    target,
		Context: fsapi.PathContext_PLUGIN,
		Force:   true,
	}
	_, err = mounter.FsClient.Rmdir(context.Background(), rmdirRequest)
	if err != nil {
		return err
	}
	return nil
}

func (mounter *CSIProxyMounter) Unmount(target string) error {
	return mounter.RemovePodDir(target)
}

func (mounter *CSIProxyMounter) GetDevicePath(deviceName string, partition string, volumeKey string) (string, error) {
	id := "page83"
	listRequest := &diskapi.ListDiskIDsRequest{}
	diskIDsResponse, err := mounter.DiskClient.ListDiskIDs(context.Background(), listRequest)
	if err != nil {
		return "", err
	}
	diskIDsMap := diskIDsResponse.GetDiskIDs()
	for diskNum, diskInfo := range diskIDsMap {
		klog.V(4).Infof("found disk number %s, disk info %v", diskNum, diskInfo)
		idValue, found := diskInfo.Identifiers[id]
		// The page83 id for gce pd has format of "Google pvc-xxxxxxx(the device name passed in here)"
		if !found || idValue == "" {
			continue
		}
		names := strings.Fields(idValue)
		klog.V(4).Infof("get page83 id %s", idValue)
		if names[len(names)-1] == deviceName {
			return diskNum, nil
		}
	}
	return "", fmt.Errorf("could not find disk number for device %s", deviceName)

}

// FormatAndMount accepts the source disk number, target path to mount, the fstype to format with and options to be used.
// After formatting, it will mount the disk to target path on the host
func (mounter *CSIProxyMounter) FormatAndMount(source string, target string, fstype string, options []string) error {
	// Call PartitionDisk CSI proxy call to partition the disk and return the volume id
	partionDiskRequest := &diskapi.PartitionDiskRequest{
		DiskID: source,
	}

	_, err := mounter.DiskClient.PartitionDisk(context.Background(), partionDiskRequest)
	if err != nil {
		return err
	}
	volumeIDsRequest := &volumeapi.ListVolumesOnDiskRequest{
		DiskId: source,
	}
	volumeIdResponse, err := mounter.VolumeClient.ListVolumesOnDisk(context.Background(), volumeIDsRequest)
	if err != nil {
		return err
	}
	// TODO: consider partitions and choose the right partition.
	volumeID := volumeIdResponse.VolumeIds[0]
	isVolumeFormattedRequest := &volumeapi.IsVolumeFormattedRequest{
		VolumeId: volumeID,
	}
	isVolumeFormattedResponse, err := mounter.VolumeClient.IsVolumeFormatted(context.Background(), isVolumeFormattedRequest)
	if err != nil {
		return err
	}
	if !isVolumeFormattedResponse.Formatted {
		formatVolumeRequest := &volumeapi.FormatVolumeRequest{
			VolumeId: volumeID,
			// TODO (jingxu97): Accept the filesystem and other options
		}
		_, err = mounter.VolumeClient.FormatVolume(context.Background(), formatVolumeRequest)
		if err != nil {
			return err
		}
	}
	// Mount the volume by calling the CSI proxy call.
	mountVolumeRequest := &volumeapi.MountVolumeRequest{
		VolumeId: volumeID,
		Path:     target,
	}
	_, err = mounter.VolumeClient.MountVolume(context.Background(), mountVolumeRequest)
	if err != nil {
		return err
	}
	return nil
}

func (mounter *CSIProxyMounter) GetMountRefs(pathname string) ([]string, error) {
	return []string{}, fmt.Errorf("GetMountRefs not implemented for ProxyMounter")
}

func (mounter *CSIProxyMounter) IsLikelyNotMountPoint(file string) (bool, error) {
	isMountRequest := &fsapi.IsMountPointRequest{
		Path: file,
	}

	isMountResponse, err := mounter.FsClient.IsMountPoint(context.Background(), isMountRequest)
	if err != nil {
		return false, err
	}

	return !isMountResponse.IsMountPoint, nil
}

func (mounter *CSIProxyMounter) List() ([]mount.MountPoint, error) {
	return []mount.MountPoint{}, nil
}

func (mounter *CSIProxyMounter) IsMountPointMatch(mp mount.MountPoint, dir string) bool {
	return mp.Path == dir
}

// ExistsPath - Checks if a path exists. Unlike util ExistsPath, this call does not perform follow link.
func (mounter *CSIProxyMounter) ExistsPath(path string) (bool, error) {
	isExistsResponse, err := mounter.FsClient.PathExists(context.Background(),
		&fsapi.PathExistsRequest{
			Path: mount.NormalizeWindowsPath(path),
		})
	if err != nil {
		return false, err
	}
	return isExistsResponse.Exists, err
}

func (mounter *CSIProxyMounter) GetBlockSizeBytes(diskId string) (int64, error) {
	DiskStatsResponse, err := mounter.DiskClient.DiskStats(context.Background(),
		&diskapi.DiskStatsRequest{
			DiskID: diskId,
		})
	return DiskStatsResponse.DiskSize, err
}
