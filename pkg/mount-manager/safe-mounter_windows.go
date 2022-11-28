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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	disk "github.com/kubernetes-csi/csi-proxy/v2/pkg/disk"
	diskapi "github.com/kubernetes-csi/csi-proxy/v2/pkg/disk/hostapi"

	fs "github.com/kubernetes-csi/csi-proxy/v2/pkg/filesystem"
	fsapi "github.com/kubernetes-csi/csi-proxy/v2/pkg/filesystem/hostapi"

	volume "github.com/kubernetes-csi/csi-proxy/v2/pkg/volume"
	volumeapi "github.com/kubernetes-csi/csi-proxy/v2/pkg/volume/hostapi"

	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
)

func NewSafeMounter() (*mount.SafeFormatAndMount, error) {
	realExec := exec.New()
	csiProxyMounter, err := NewCSIProxyMounter()
	if err != nil {
		return nil, err
	}

	return &mount.SafeFormatAndMount{
		Interface: csiProxyMounter,
		Exec:      realExec,
	}, nil
}

// CSIProxyMounter is the mounter interface exposed as a utility to
// internal methods
type CSIProxyMounter interface {
	mount.Interface

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

// csiProxyMounter is the mounter implementation using csi proxy
type CSIProxyMounterImpl struct {
	Fs     fs.Interface
	Disk   disk.Interface
	Volume volume.Interface
}

// check that CSIProxyMounterImpl implements CSIProxyMounter
var _ CSIProxyMounter = &CSIProxyMounterImpl{}

func NewCSIProxyMounter() (CSIProxyMounter, error) {
	fsClient, err := fs.New(fsapi.New())
	if err != nil {
		return nil, err
	}
	diskClient, err := disk.New(diskapi.New())
	if err != nil {
		return nil, err
	}
	volumeClient, err := volume.New(volumeapi.New())
	if err != nil {
		return nil, err
	}
	return &CSIProxyMounterImpl{
		Fs:     fsClient,
		Disk:   diskClient,
		Volume: volumeClient,
	}, nil
}

// Mount just creates a soft link at target pointing to source.
func (mounter *CSIProxyMounterImpl) Mount(source string, target string, fstype string, options []string) error {
	return mounter.MountSensitive(source, target, fstype, options, nil /* sensitiveOptions */)
}

// MountSensitive is the same as Mount() but this method allows
// sensitiveOptions to be passed in a separate parameter from the normal
// mount options and ensures the sensitiveOptions are never logged.
// Since Mount here is just create a synlink, so options and sensitiveOptions
// are not used here
func (mounter *CSIProxyMounterImpl) MountSensitive(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	// Mount is called after the format is done.
	// TODO: Confirm that fstype is empty.
	// Call the LinkPath CSI proxy from the source path to the target path
	parentDir := filepath.Dir(target)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return err
	}
	createSymlinkRequest := &fs.CreateSymlinkRequest{
		SourcePath: mount.NormalizeWindowsPath(source),
		TargetPath: mount.NormalizeWindowsPath(target),
	}
	_, err := mounter.Fs.CreateSymlink(context.Background(), createSymlinkRequest)
	if err != nil {
		return err
	}
	return nil
}

// Delete the given directory with Pod context. CSI proxy does a check for path prefix
// based on context
func (mounter *CSIProxyMounterImpl) RemovePodDir(target string) error {
	rmdirRequest := &fs.RmdirRequest{
		Path:  mount.NormalizeWindowsPath(target),
		Force: true,
	}
	_, err := mounter.Fs.Rmdir(context.Background(), rmdirRequest)
	if err != nil {
		return err
	}
	return nil
}

// UnmountDevice uses target path to find the volume id first, and then
// call DismountVolume through csi-proxy. If succeeded, it will delete the given path
// at last step. CSI proxy does a check for path prefix
// based on context
func (mounter *CSIProxyMounterImpl) UnmountDevice(target string) error {
	target = mount.NormalizeWindowsPath(target)
	if exists, err := mounter.ExistsPath(target); !exists {
		return err
	}
	idRequest := &volume.GetVolumeIDFromTargetPathRequest{
		TargetPath: target,
	}
	idResponse, err := mounter.Volume.GetVolumeIDFromTargetPath(context.Background(), idRequest)
	if err != nil {
		return err
	}
	volumeID := idResponse.VolumeID

	unmountRequest := &volume.UnmountVolumeRequest{
		TargetPath: target,
		VolumeID:   volumeID,
	}
	_, err = mounter.Volume.UnmountVolume(context.Background(), unmountRequest)
	if err != nil {
		return err
	}
	rmdirRequest := &fs.RmdirRequest{
		Path:  target,
		Force: true,
	}
	_, err = mounter.Fs.Rmdir(context.Background(), rmdirRequest)
	if err != nil {
		return err
	}

	// Set disk to offline mode to have a clean state
	getDiskNumberRequest := &volume.GetDiskNumberFromVolumeIDRequest{
		VolumeID: volumeID,
	}
	getDiskNumberResponse, err := mounter.Volume.GetDiskNumberFromVolumeID(context.Background(), getDiskNumberRequest)
	if err != nil {
		return err
	}
	diskNumber := getDiskNumberResponse.DiskNumber
	klog.V(4).Infof("get disk number %d from volume %s", diskNumber, volumeID)
	setDiskStateRequest := &disk.SetDiskStateRequest{
		DiskNumber: diskNumber,
		IsOnline:   false,
	}
	if _, err = mounter.Disk.SetDiskState(context.Background(), setDiskStateRequest); err != nil {
		return err
	}

	return nil
}

func (mounter *CSIProxyMounterImpl) Unmount(target string) error {
	return mounter.RemovePodDir(target)
}

func (mounter *CSIProxyMounterImpl) GetDiskNumber(deviceName string, partition string, volumeKey string) (string, error) {
	listRequest := &disk.ListDiskIDsRequest{}
	diskIDsResponse, err := mounter.Disk.ListDiskIDs(context.Background(), listRequest)
	if err != nil {
		return "", err
	}
	diskIDsMap := diskIDsResponse.DiskIDs
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
func (mounter *CSIProxyMounterImpl) FormatAndMount(source string, target string, fstype string, options []string) error {
	diskNumberUint64, err := strconv.ParseUint(source, 10, 64)
	if err != nil {
		return err
	}
	diskNumber := uint32(diskNumberUint64)

	// Call PartitionDisk CSI proxy call to partition the disk and return the volume id
	partionDiskRequest := &disk.PartitionDiskRequest{
		DiskNumber: diskNumber,
	}

	_, err = mounter.Disk.PartitionDisk(context.Background(), partionDiskRequest)
	if err != nil {
		return err
	}

	// make sure disk is online. if disk is already online, this call should also succeed.
	setDiskStateRequest := &disk.SetDiskStateRequest{
		DiskNumber: diskNumber,
		IsOnline:   true,
	}
	_, err = mounter.Disk.SetDiskState(context.Background(), setDiskStateRequest)
	if err != nil {
		return err
	}

	volumeIDsRequest := &volume.ListVolumesOnDiskRequest{
		DiskNumber: diskNumber,
	}
	volumeIDResponse, err := mounter.Volume.ListVolumesOnDisk(context.Background(), volumeIDsRequest)
	if err != nil {
		return err
	}
	// TODO: consider partitions and choose the right partition.
	if len(volumeIDResponse.VolumeIDs) == 0 {
		return fmt.Errorf("ListVolumesOnDisk does not return any volumes")
	}
	volumeID := volumeIDResponse.VolumeIDs[0]
	isVolumeFormattedRequest := &volume.IsVolumeFormattedRequest{
		VolumeID: volumeID,
	}
	isVolumeFormattedResponse, err := mounter.Volume.IsVolumeFormatted(context.Background(), isVolumeFormattedRequest)
	if err != nil {
		return err
	}
	if !isVolumeFormattedResponse.Formatted {
		formatVolumeRequest := &volume.FormatVolumeRequest{
			VolumeID: volumeID,
			// TODO (jingxu97): Accept the filesystem and other options
		}
		_, err = mounter.Volume.FormatVolume(context.Background(), formatVolumeRequest)
		if err != nil {
			return err
		}
	}
	// Mount the volume by calling the CSI proxy call.
	mountVolumeRequest := &volume.MountVolumeRequest{
		VolumeID:   volumeID,
		TargetPath: target,
	}
	_, err = mounter.Volume.MountVolume(context.Background(), mountVolumeRequest)
	if err != nil {
		return err
	}
	return nil
}

func (mounter *CSIProxyMounterImpl) GetMountRefs(pathname string) ([]string, error) {
	return []string{}, fmt.Errorf("GetMountRefs not implemented for ProxyMounter")
}

func (mounter *CSIProxyMounterImpl) IsLikelyNotMountPoint(file string) (bool, error) {
	isSymlinkRequest := &fs.IsSymlinkRequest{
		Path: file,
	}

	isSymlinkResponse, err := mounter.Fs.IsSymlink(context.Background(), isSymlinkRequest)
	if err != nil {
		return true, err
	}

	return !isSymlinkResponse.IsSymlink, nil
}

func (mounter *CSIProxyMounterImpl) List() ([]mount.MountPoint, error) {
	return []mount.MountPoint{}, nil
}

func (mounter *CSIProxyMounterImpl) IsMountPointMatch(mp mount.MountPoint, dir string) bool {
	return mp.Path == dir
}

// ExistsPath - Checks if a path exists. Unlike util ExistsPath, this call does not perform follow link.
func (mounter *CSIProxyMounterImpl) ExistsPath(path string) (bool, error) {
	isExistsResponse, err := mounter.Fs.PathExists(context.Background(),
		&fs.PathExistsRequest{
			Path: mount.NormalizeWindowsPath(path),
		})
	if err != nil {
		return false, err
	}
	return isExistsResponse.Exists, err
}

func (mounter *CSIProxyMounterImpl) GetDiskTotalBytes(devicePath string) (int64, error) {
	diskNumberUint64, err := strconv.ParseUint(devicePath, 10, 64)
	if err != nil {
		return 0, err
	}
	diskNumber := uint32(diskNumberUint64)

	DiskStatsResponse, err := mounter.Disk.GetDiskStats(context.Background(),
		&disk.GetDiskStatsRequest{
			DiskNumber: diskNumber,
		})
	return DiskStatsResponse.TotalBytes, err
}

// MountSensitiveWithoutSystemd is the same as MountSensitive() but this method disable using systemd mount.
// It's unimplemented in PD CSI Driver
func (mounter *CSIProxyMounterImpl) MountSensitiveWithoutSystemd(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	return errors.New("MountSensitiveWithoutSystemd is not implemented")
}

// MountSensitiveWithoutSystemdWithMountFlags is the same as MountSensitiveWithoutSystemd with additional mount flags.
// It's unimplemented in PD CSI Driver
func (mounter *CSIProxyMounterImpl) MountSensitiveWithoutSystemdWithMountFlags(source string, target string, fstype string, options []string, sensitiveOptions []string, mountFlags []string) error {
	return errors.New("MountSensitiveWithoutSystemd is not implemented")
}
