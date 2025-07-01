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
	"encoding/json"
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

	"cloud.google.com/go/compute/metadata"
	"k8s.io/klog/v2"
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
	// First, get Google Cloud metadata to find the nvmeNamespaceIdentifier for this device
	googleDisks, err := AttachedDisks()
	if err != nil {
		klog.V(4).Infof("Failed to get Google Cloud metadata, falling back to legacy method: %v", err)
		return mounter.getDiskNumberLegacy(deviceName)
	}

	// Find the nvmeNamespaceIdentifier for the given deviceName
	var targetNamespaceId uint64
	var diskInterface string
	found := false
	for _, disk := range googleDisks {
		if disk.DeviceName == deviceName {
			targetNamespaceId = disk.NvmeNamespaceIdentifier
			diskInterface = disk.Interface
			found = true
			klog.V(4).Infof("Found target namespace ID %d for device %s with interface %s", targetNamespaceId, deviceName, diskInterface)
			break
		}
	}

	if !found {
		klog.V(4).Infof("Device %s not found in Google Cloud metadata, falling back to legacy method", deviceName)
		return mounter.getDiskNumberLegacy(deviceName)
	}

	// Check if device is NVME - if not, use legacy method
	if diskInterface != "NVME" {
		klog.V(4).Infof("Device %s is %s interface (not NVME), falling back to legacy method", deviceName, diskInterface)
		return mounter.getDiskNumberLegacy(deviceName)
	}

	// Get Windows disk information
	listRequest := &diskapi.ListDiskIDsRequest{}
	diskIDsResponse, err := mounter.DiskClient.ListDiskIDs(context.Background(), listRequest)
	if err != nil {
		return "", err
	}
	diskIDsMap := diskIDsResponse.GetDiskIDs()

	// Iterate through Windows disks and convert EUI to decimal for matching
	for diskNum, diskInfo := range diskIDsMap {
		klog.V(4).Infof("found disk number %d, disk info %v", diskNum, diskInfo)

		// Check if this disk has an EUI identifier
		euiValue := mounter.extractEUIFromDiskInfo(diskInfo)
		if euiValue == "" {
			continue
		}

		// Convert EUI hex to decimal
		decimalValue, err := mounter.convertEUIToDecimal(euiValue)
		if err != nil {
			klog.V(4).Infof("Failed to convert EUI %s to decimal: %v", euiValue, err)
			continue
		}

		klog.V(4).Infof("Disk %d: EUI %s converts to decimal %d", diskNum, euiValue, decimalValue)

		// Check if this matches our target namespace identifier
		if decimalValue == targetNamespaceId {
			klog.V(4).Infof("Found matching disk: Windows disk %d matches Google namespace ID %d", diskNum, targetNamespaceId)
			return strconv.FormatUint(uint64(diskNum), 10), nil
		}
	}

	// Final fallback: if NVME matching failed, try legacy method
	klog.V(4).Infof("Could not find NVME match for device %s with namespace ID %d, falling back to legacy method", deviceName, targetNamespaceId)
	return mounter.getDiskNumberLegacy(deviceName)
}

// Helper function to extract EUI from disk info (v1 API format)
func (mounter *CSIProxyMounterV1) extractEUIFromDiskInfo(diskInfo *diskapi.DiskIDs) string {
	klog.V(4).Infof("extractEUIFromDiskInfo called for disk with Page83=%s, SerialNumber=%s", diskInfo.Page83, diskInfo.SerialNumber)

	// Check if Page83 contains an EUI format
	if diskInfo.Page83 != "" && strings.HasPrefix(diskInfo.Page83, "eui.") {
		klog.V(4).Infof("Found EUI in Page83: %s", diskInfo.Page83)
		return diskInfo.Page83
	}

	// For NVMe disks, check SerialNumber field and convert to EUI format
	if diskInfo.SerialNumber != "" {
		klog.V(4).Infof("Attempting to convert serial number %s to EUI", diskInfo.SerialNumber)
		// Convert serial number format like "10CC_9636_B6E3_CE3B_0000_0000_0000_0000." to EUI format
		eui := mounter.convertSerialToEUI(diskInfo.SerialNumber)
		if eui != "" {
			klog.V(4).Infof("Successfully converted serial number %s to EUI %s", diskInfo.SerialNumber, eui)
			return eui
		} else {
			klog.V(4).Infof("Failed to convert serial number %s to EUI", diskInfo.SerialNumber)
		}
	} else {
		klog.V(4).Infof("No serial number found for disk")
	}

	klog.V(4).Infof("No EUI found for disk")
	return ""
}

// Helper function to convert serial number to EUI format
func (mounter *CSIProxyMounterV1) convertSerialToEUI(serialNumber string) string {
	klog.V(4).Infof("convertSerialToEUI: input=%s", serialNumber)

	// Remove trailing period and underscores from serial number
	// Format: "10CC_9636_B6E3_CE3B_0000_0000_0000_0000." -> "10CC9636B6E3CE3B0000000000000000"
	cleaned := strings.TrimSuffix(serialNumber, ".")
	cleaned = strings.ReplaceAll(cleaned, "_", "")
	klog.V(4).Infof("convertSerialToEUI: cleaned=%s, length=%d", cleaned, len(cleaned))

	// Validate that it's a valid hex string of expected length
	if len(cleaned) != 32 {
		klog.V(4).Infof("convertSerialToEUI: invalid length %d, expected 32", len(cleaned))
		return ""
	}

	// Check if it's valid hex (just test parsing the first 16 chars to avoid overflow)
	if _, err := strconv.ParseUint(cleaned[:16], 16, 64); err != nil {
		klog.V(4).Infof("convertSerialToEUI: invalid hex in first 16 chars: %v", err)
		return ""
	}

	// Return in EUI format
	result := "eui." + cleaned
	klog.V(4).Infof("convertSerialToEUI: result=%s", result)
	return result
}

// Helper function to convert EUI hex to decimal
func (mounter *CSIProxyMounterV1) convertEUIToDecimal(euiValue string) (uint64, error) {
	// Extract hex part from EUI (first 16 characters after "eui.")
	if !strings.HasPrefix(euiValue, "eui.") {
		return 0, fmt.Errorf("invalid EUI format: %s", euiValue)
	}

	hexPart := strings.TrimPrefix(euiValue, "eui.")
	if len(hexPart) < 16 {
		return 0, fmt.Errorf("EUI hex part too short: %s", hexPart)
	}

	// Take first 16 hex characters
	hexPart = hexPart[:16]

	// Convert to decimal
	decimalValue, err := strconv.ParseUint(hexPart, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse hex %s: %v", hexPart, err)
	}

	return decimalValue, nil
}

// Helper function to get Google Cloud metadata
func (mounter *CSIProxyMounterV1) getGoogleCloudDisks() ([]GoogleCloudDisk, error) {
	disksResp, err := metadata.GetWithContext(context.Background(), "instance/disks/?recursive=true")
	if err != nil {
		return nil, fmt.Errorf("failed to get disks using metadata package: %v", err)
	}

	var disks []GoogleCloudDisk
	if err := json.Unmarshal([]byte(disksResp), &disks); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %v", err)
	}

	klog.V(4).Infof("Retrieved %d disks from Google Cloud metadata", len(disks))
	return disks, nil
}

// Legacy method for backward compatibility
func (mounter *CSIProxyMounterV1) getDiskNumberLegacy(deviceName string) (string, error) {
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

// MountSensitiveWithoutSystemdWithMountFlags is the same as MountSensitiveWithoutSystemd with additional mount flags.
// It's unimplemented in PD CSI Driver
func (mounter *CSIProxyMounterV1) MountSensitiveWithoutSystemdWithMountFlags(source string, target string, fstype string, options []string, sensitiveOptions []string, mountFlags []string) error {
	return errors.New("MountSensitiveWithoutSystemd is not implemented")
}

// CanSafelySkipMountPointCheck always returns false on Windows.
func (mounter *CSIProxyMounterV1) CanSafelySkipMountPointCheck() bool {
	return false
}

// IsMountPoint returns true if a directory is a mountpoint.
func (mounter *CSIProxyMounterV1) IsMountPoint(file string) (bool, error) {
	isNotMnt, err := mounter.IsLikelyNotMountPoint(file)
	if err != nil {
		return false, err
	}
	return !isNotMnt, nil
}
