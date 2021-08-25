//go:build windows

/*
Copyright 2021 The Kubernetes Authors.

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
	"strconv"
	"strings"

	diskapi "github.com/kubernetes-csi/csi-proxy/client/api/disk/v1"
	diskclient "github.com/kubernetes-csi/csi-proxy/client/groups/disk/v1"

	fsapi "github.com/kubernetes-csi/csi-proxy/client/api/filesystem/v1"
	fsclient "github.com/kubernetes-csi/csi-proxy/client/groups/filesystem/v1"

	volumeapi "github.com/kubernetes-csi/csi-proxy/client/api/volume/v1"
	volumeclient "github.com/kubernetes-csi/csi-proxy/client/groups/volume/v1"

	"k8s.io/klog"
	mount "k8s.io/mount-utils"
)

// CSIProxyMounterV1 is the mounter implementation that uses the v1 API
type CSIProxyMounterV1 struct {
	FsClient     *fsclient.Client
	DiskClient   *diskclient.Client
	VolumeClient *volumeclient.Client
}

// check that CSIProxyMounterV1 implements CSIProxyMounter
var _ CSIProxyMounter = &CSIProxyMounterV1{}

func NewCSIProxyMounterV1() (*CSIProxyMounterV1, error) {
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
	return &CSIProxyMounterV1{
		FsClient:     fsClient,
		DiskClient:   diskClient,
		VolumeClient: volumeClient,
	}, nil
}

// GetAPIVersions returns the versions of the client APIs this mounter is using.
func (mounter *CSIProxyMounterV1) GetAPIVersions() string {
	return fmt.Sprintf(
		"API Versions Disk: %s, Filesystem: %s, Volume: %s",
		diskclient.Version,
		fsclient.Version,
		volumeclient.Version,
	)
}

// Mount just creates a soft link at target pointing to source.
func (mounter *CSIProxyMounterV1) Mount(source string, target string, fstype string, options []string) error {
	return mounter.MountSensitive(source, target, fstype, options, nil /* sensitiveOptions */)
}

// MountSensitive is the same as Mount() but this method allows
// sensitiveOptions to be passed in a separate parameter from the normal
// mount options and ensures the sensitiveOptions are never logged.
// Since Mount here is just create a synlink, so options and sensitiveOptions
// are not used here
func (mounter *CSIProxyMounterV1) MountSensitive(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	// Mount is called after the format is done.
	// TODO: Confirm that fstype is empty.
	// Call the LinkPath CSI proxy from the source path to the target path
	parentDir := filepath.Dir(target)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return err
	}
	createSymlinkRequest := &fsapi.CreateSymlinkRequest{
		SourcePath: mount.NormalizeWindowsPath(source),
		TargetPath: mount.NormalizeWindowsPath(target),
	}
	_, err := mounter.FsClient.CreateSymlink(context.Background(), createSymlinkRequest)
	if err != nil {
		return err
	}
	return nil
}

// Delete the given directory with Pod context. CSI proxy does a check for path prefix
// based on context
func (mounter *CSIProxyMounterV1) RemovePodDir(target string) error {
	rmdirRequest := &fsapi.RmdirRequest{
		Path:  mount.NormalizeWindowsPath(target),
		Force: true,
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
func (mounter *CSIProxyMounterV1) UnmountDevice(target string) error {
	target = mount.NormalizeWindowsPath(target)
	if exists, err := mounter.ExistsPath(target); !exists {
		return err
	}
	idRequest := &volumeapi.GetVolumeIDFromTargetPathRequest{
		TargetPath: target,
	}
	idResponse, err := mounter.VolumeClient.GetVolumeIDFromTargetPath(context.Background(), idRequest)
	if err != nil {
		return err
	}
	volumeId := idResponse.GetVolumeId()

	unmountRequest := &volumeapi.UnmountVolumeRequest{
		TargetPath: target,
		VolumeId:   volumeId,
	}
	_, err = mounter.VolumeClient.UnmountVolume(context.Background(), unmountRequest)
	if err != nil {
		return err
	}
	rmdirRequest := &fsapi.RmdirRequest{
		Path:  target,
		Force: true,
	}
	_, err = mounter.FsClient.Rmdir(context.Background(), rmdirRequest)
	if err != nil {
		return err
	}

	// Set disk to offline mode to have a clean state
	getDiskNumberRequest := &volumeapi.GetDiskNumberFromVolumeIDRequest{
		VolumeId: volumeId,
	}
	getDiskNumberResponse, err := mounter.VolumeClient.GetDiskNumberFromVolumeID(context.Background(), getDiskNumberRequest)
	if err != nil {
		return err
	}
	diskNumber := getDiskNumberResponse.GetDiskNumber()
	klog.V(4).Infof("get disk number %d from volume %s", diskNumber, volumeId)
	setDiskStateRequest := &diskapi.SetDiskStateRequest{
		DiskNumber: diskNumber,
		IsOnline:   false,
	}
	if _, err = mounter.DiskClient.SetDiskState(context.Background(), setDiskStateRequest); err != nil {
		return err
	}

	return nil
}

func (mounter *CSIProxyMounterV1) Unmount(target string) error {
	return mounter.RemovePodDir(target)
}

func (mounter *CSIProxyMounterV1) GetDiskNumber(deviceName string, partition string, volumeKey string) (string, error) {
	listRequest := &diskapi.ListDiskIDsRequest{}
	diskIDsResponse, err := mounter.DiskClient.ListDiskIDs(context.Background(), listRequest)
	if err != nil {
		return "", err
	}
	diskIDsMap := diskIDsResponse.GetDiskIDs()
	for diskNum, diskInfo := range diskIDsMap {
		klog.V(4).Infof("found disk number %d, disk info %v", diskNum, diskInfo)
		idValue := diskInfo.Page83
		// The page83 id for gce pd has format of "Google pvc-xxxxxxx(the device name passed in here)"
		if idValue == "" {
			continue
		}
		klog.V(4).Infof("get page83 id %s", idValue)
		names := strings.Fields(idValue)
		if names[len(names)-1] == deviceName {
			return strconv.FormatUint(uint64(diskNum), 10), nil
		}
	}
	return "", fmt.Errorf("could not find disk number for device %s", deviceName)

}

// FormatAndMount accepts the source disk number, target path to mount, the fstype to format with and options to be used.
// After formatting, it will mount the disk to target path on the host
func (mounter *CSIProxyMounterV1) FormatAndMount(source string, target string, fstype string, options []string) error {
	diskNumberUint64, err := strconv.ParseUint(source, 10, 64)
	if err != nil {
		return err
	}
	diskNumber := uint32(diskNumberUint64)

	// Call PartitionDisk CSI proxy call to partition the disk and return the volume id
	partionDiskRequest := &diskapi.PartitionDiskRequest{
		DiskNumber: diskNumber,
	}

	_, err = mounter.DiskClient.PartitionDisk(context.Background(), partionDiskRequest)
	if err != nil {
		return err
	}

	// make sure disk is online. if disk is already online, this call should also succeed.
	setDiskStateRequest := &diskapi.SetDiskStateRequest{
		DiskNumber: diskNumber,
		IsOnline:   true,
	}
	_, err = mounter.DiskClient.SetDiskState(context.Background(), setDiskStateRequest)
	if err != nil {
		return err
	}

	volumeIDsRequest := &volumeapi.ListVolumesOnDiskRequest{
		DiskNumber: diskNumber,
	}
	volumeIdResponse, err := mounter.VolumeClient.ListVolumesOnDisk(context.Background(), volumeIDsRequest)
	if err != nil {
		return err
	}
	// TODO: consider partitions and choose the right partition.
	if len(volumeIdResponse.VolumeIds) == 0 {
		return fmt.Errorf("ListVolumesOnDisk does not return any volumes")
	}
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
		VolumeId:   volumeID,
		TargetPath: target,
	}
	_, err = mounter.VolumeClient.MountVolume(context.Background(), mountVolumeRequest)
	if err != nil {
		return err
	}
	return nil
}

func (mounter *CSIProxyMounterV1) GetMountRefs(pathname string) ([]string, error) {
	return []string{}, fmt.Errorf("GetMountRefs not implemented for ProxyMounter")
}

func (mounter *CSIProxyMounterV1) IsLikelyNotMountPoint(file string) (bool, error) {
	isSymlinkRequest := &fsapi.IsSymlinkRequest{
		Path: file,
	}

	isSymlinkResponse, err := mounter.FsClient.IsSymlink(context.Background(), isSymlinkRequest)
	if err != nil {
		return true, err
	}

	return !isSymlinkResponse.IsSymlink, nil
}

func (mounter *CSIProxyMounterV1) List() ([]mount.MountPoint, error) {
	return []mount.MountPoint{}, nil
}

func (mounter *CSIProxyMounterV1) IsMountPointMatch(mp mount.MountPoint, dir string) bool {
	return mp.Path == dir
}

// ExistsPath - Checks if a path exists. Unlike util ExistsPath, this call does not perform follow link.
func (mounter *CSIProxyMounterV1) ExistsPath(path string) (bool, error) {
	isExistsResponse, err := mounter.FsClient.PathExists(context.Background(),
		&fsapi.PathExistsRequest{
			Path: mount.NormalizeWindowsPath(path),
		})
	if err != nil {
		return false, err
	}
	return isExistsResponse.Exists, err
}

func (mounter *CSIProxyMounterV1) GetDiskTotalBytes(devicePath string) (int64, error) {
	diskNumberUint64, err := strconv.ParseUint(devicePath, 10, 64)
	if err != nil {
		return 0, err
	}
	diskNumber := uint32(diskNumberUint64)

	DiskStatsResponse, err := mounter.DiskClient.GetDiskStats(context.Background(),
		&diskapi.GetDiskStatsRequest{
			DiskNumber: diskNumber,
		})
	return DiskStatsResponse.TotalBytes, err
}

// MountSensitiveWithoutSystemd is the same as MountSensitive() but this method disable using systemd mount.
// It's unimplemented in PD CSI Driver
func (mounter *CSIProxyMounterV1) MountSensitiveWithoutSystemd(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	return errors.New("MountSensitiveWithoutSystemd is not implemented")
}
