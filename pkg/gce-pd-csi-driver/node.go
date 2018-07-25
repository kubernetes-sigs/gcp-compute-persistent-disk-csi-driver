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

package gceGCEDriver

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/utils/exec"
	utils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/utils"
)

const (
	diskByHostIdPath     = "/host/dev/disk/by-id/"
	diskByIdPath         = "/dev/disk/by-id/"
	diskGooglePrefix     = "google-"
	diskScsiGooglePrefix = "scsi-0Google_PersistentDisk_"
	diskPartitionSuffix  = "-part"
	diskSDPath           = "/host/dev/sd"
	diskSDPattern        = "/host/dev/sd*"
	// How many times to retry for a consistent read of /proc/mounts.
	maxListTries = 3
	// Number of fields per line in /proc/mounts as per the fstab man page.
	expectedNumFieldsPerLine = 6
	// Location of the mount file to use
	procMountsPath = "/proc/mounts"
	// Location of the mountinfo file
	procMountInfoPath = "/proc/self/mountinfo"
	// 'fsck' found errors and corrected them
	fsckErrorsCorrected = 1
	// 'fsck' found errors but exited without correcting them
	fsckErrorsUncorrected = 4
	defaultMountCommand   = "mount"
)

type GCENodeServer struct {
	Driver  *GCEDriver
	Mounter mount.SafeFormatAndMount
	// TODO: Only lock mutually exclusive calls and make locking more fine grained
	mux sync.Mutex
}

var _ csi.NodeServer = &GCENodeServer{}

func (ns *GCENodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	ns.mux.Lock()
	defer ns.mux.Unlock()
	glog.Infof("NodePublishVolume called with req: %#v", req)

	// Validate Arguments
	targetPath := req.GetTargetPath()
	stagingTargetPath := req.GetStagingTargetPath()
	readOnly := req.GetReadonly()
	volumeID := req.GetVolumeId()
	volumeCapability := req.GetVolumeCapability()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume ID must be provided")
	}
	if len(stagingTargetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Staging Target Path must be provided")
	}
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Target Path must be provided")
	}
	if volumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume Capability must be provided")
	}

	notMnt, err := ns.Mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil && !os.IsNotExist(err) {
		glog.Errorf("cannot validate mount point: %s %v", targetPath, err)
		return nil, err
	}
	if !notMnt {
		// TODO(dyzz): check if mount is compatible. Return OK if it is, or appropriate error.
		return nil, nil
	}

	if err := ns.Mounter.MakeDir(targetPath); err != nil {
		glog.Errorf("mkdir failed on disk %s (%v)", targetPath, err)
		return nil, err
	}

	// Perform a bind mount to the full path to allow duplicate mounts of the same PD.
	options := []string{"bind"}
	if readOnly {
		options = append(options, "ro")
	}

	err = ns.Mounter.Mount(stagingTargetPath, targetPath, "ext4", options)
	if err != nil {
		notMnt, mntErr := ns.Mounter.IsLikelyNotMountPoint(targetPath)
		if mntErr != nil {
			glog.Errorf("IsLikelyNotMountPoint check failed: %v", mntErr)
			return nil, status.Error(codes.Internal, fmt.Sprintf("TODO: %v", err))
		}
		if !notMnt {
			if mntErr = ns.Mounter.Unmount(targetPath); mntErr != nil {
				glog.Errorf("Failed to unmount: %v", mntErr)
				return nil, status.Error(codes.Internal, fmt.Sprintf("TODO: %v", err))
			}
			notMnt, mntErr := ns.Mounter.IsLikelyNotMountPoint(targetPath)
			if mntErr != nil {
				glog.Errorf("IsLikelyNotMountPoint check failed: %v", mntErr)
				return nil, status.Error(codes.Internal, fmt.Sprintf("TODO: %v", err))
			}
			if !notMnt {
				// This is very odd, we don't expect it.  We'll try again next sync loop.
				glog.Errorf("%s is still mounted, despite call to unmount().  Will try again next sync loop.", targetPath)
				return nil, status.Error(codes.Internal, fmt.Sprintf("TODO: %v", err))
			}
		}
		os.Remove(targetPath)
		glog.Errorf("Mount of disk %s failed: %v", targetPath, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("TODO: %v", err))
	}

	glog.V(4).Infof("Successfully mounted %s", targetPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *GCENodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	ns.mux.Lock()
	defer ns.mux.Unlock()
	glog.Infof("NodeUnpublishVolume called with args: %v", req)
	// Validate Arguments
	targetPath := req.GetTargetPath()
	volID := req.GetVolumeId()
	if len(volID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Volume ID must be provided")
	}
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Target Path must be provided")
	}

	// TODO: Check volume still exists

	err := ns.Mounter.Unmount(targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("Unmount failed: %v\nUnmounting arguments: %s\n", err, targetPath))
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *GCENodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	ns.mux.Lock()
	defer ns.mux.Unlock()
	glog.Infof("NodeStageVolume called with req: %#v", req)

	// Validate Arguments
	volumeID := req.GetVolumeId()
	stagingTargetPath := req.GetStagingTargetPath()
	volumeCapability := req.GetVolumeCapability()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Volume ID must be provided")
	}
	if len(stagingTargetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Staging Target Path must be provided")
	}
	if volumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Volume Capability must be provided")
	}

	_, volumeName, err := utils.SplitZoneNameId(volumeID)
	if err != nil {
		return nil, err
	}

	// TODO: Check volume still exists?

	// Part 1: Get device path of attached device
	// TODO: Get real partitions
	partition := ""

	devicePaths := getDiskByIdPaths(volumeName, partition)
	devicePath, err := verifyDevicePath(devicePaths)

	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("Error verifying GCE PD (%q) is attached: %v", volumeName, err))
	}
	if devicePath == "" {
		return nil, status.Error(codes.Internal, fmt.Sprintf("Unable to find device path out of attempted paths: %v", devicePaths))
	}

	glog.Infof("Successfully found attached GCE PD %q at device path %s.", volumeName, devicePath)

	// Part 2: Check if mount already exists at targetpath
	notMnt, err := ns.Mounter.IsLikelyNotMountPoint(stagingTargetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := ns.Mounter.MakeDir(stagingTargetPath); err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to create directory (%q): %v", stagingTargetPath, err))
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, fmt.Sprintf("Unknown error when checking mount point (%q): %v", stagingTargetPath, err))
		}
	}

	if !notMnt {
		// TODO: Validate disk mode

		// TODO: Check who is mounted here. No error if its us
		return nil, fmt.Errorf("already a mount point")

	}

	// Part 3: Mount device to stagingTargetPath
	// Default fstype is ext4
	fstype := "ext4"
	options := []string{}
	if mnt := volumeCapability.GetMount(); mnt != nil {
		if mnt.FsType != "" {
			fstype = mnt.FsType
		}
		for _, flag := range mnt.MountFlags {
			// TODO: Not sure if MountFlags == options
			options = append(options, flag)
		}
	} else if blk := volumeCapability.GetBlock(); blk != nil {
		// TODO: Block volume support
		return nil, status.Error(codes.Unimplemented, fmt.Sprintf("Block volume support is not yet implemented"))
	}

	err = ns.Mounter.FormatAndMount(devicePath, stagingTargetPath, fstype, options)
	if err != nil {
		return nil, status.Error(codes.Internal,
			fmt.Sprintf("Failed to format and mount device from (%q) to (%q) with fstype (%q) and options (%q): %v",
				devicePath, stagingTargetPath, fstype, options, err))
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *GCENodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	ns.mux.Lock()
	defer ns.mux.Unlock()
	glog.Infof("NodeUnstageVolume called with req: %#v", req)
	// Validate arguments
	volumeID := req.GetVolumeId()
	stagingTargetPath := req.GetStagingTargetPath()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeUnstageVolume Volume ID must be provided")
	}
	if len(stagingTargetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeUnstageVolume Staging Target Path must be provided")
	}

	err := ns.Mounter.Unmount(stagingTargetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("NodeUnstageVolume failed to unmount at path %s: %v", stagingTargetPath, err))
	}
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *GCENodeServer) NodeGetId(ctx context.Context, req *csi.NodeGetIdRequest) (*csi.NodeGetIdResponse, error) {
	glog.Infof("NodeGetId called with req: %#v", req)

	return &csi.NodeGetIdResponse{
		NodeId: ns.Driver.nodeID,
	}, nil
}

func (ns *GCENodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	glog.Infof("NodeGetCapabilities called with req: %#v", req)

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: ns.Driver.nscap,
	}, nil
}

func (ns *GCENodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	glog.Infof("NodeGetInfo called with req: %#v", req)

	resp := &csi.NodeGetInfoResponse{
		NodeId: ns.Driver.nodeID,
		// TODO: Set MaxVolumesPerNode based on Node Type
		// Default of 0 means that CO Decides how many nodes can be published
		MaxVolumesPerNode:  0,
		AccessibleTopology: nil,
	}
	return resp, nil
}

// Returns list of all /dev/disk/by-id/* paths for given PD.
func getDiskByIdPaths(pdName string, partition string) []string {
	devicePaths := []string{
		path.Join(diskByIdPath, diskGooglePrefix+pdName),
		path.Join(diskByIdPath, diskScsiGooglePrefix+pdName),
		path.Join(diskByHostIdPath, diskScsiGooglePrefix+pdName),
		path.Join(diskByHostIdPath, diskScsiGooglePrefix+pdName),
	}

	if partition != "" {
		for i, path := range devicePaths {
			devicePaths[i] = path + diskPartitionSuffix + partition
		}
	}

	return devicePaths
}

// Returns the first path that exists, or empty string if none exist.
func verifyDevicePath(devicePaths []string) (string, error) {
	sdBefore, err := filepath.Glob(diskSDPattern)
	if err != nil {
		// Seeing this error means that the diskSDPattern is malformed.
		glog.Errorf("Error filepath.Glob(\"%s\"): %v\r\n", diskSDPattern, err)
	}
	sdBeforeSet := sets.NewString(sdBefore...)
	// TODO: Remove this udevadm stuff. Not applicable because can't access /dev/sd* from container
	if err := udevadmChangeToNewDrives(sdBeforeSet); err != nil {
		// udevadm errors should not block disk detachment, log and continue
		glog.Errorf("udevadmChangeToNewDrives failed with: %v", err)
	}

	for _, path := range devicePaths {
		if pathExists, err := pathExists(path); err != nil {
			return "", fmt.Errorf("Error checking if path exists: %v", err)
		} else if pathExists {
			return path, nil
		}
	}

	return "", nil
}

// Triggers the application of udev rules by calling "udevadm trigger
// --action=change" for newly created "/dev/sd*" drives (exist only in
// after set). This is workaround for Issue #7972. Once the underlying
// issue has been resolved, this may be removed.
func udevadmChangeToNewDrives(sdBeforeSet sets.String) error {
	sdAfter, err := filepath.Glob(diskSDPattern)
	if err != nil {
		return fmt.Errorf("Error filepath.Glob(\"%s\"): %v\r\n", diskSDPattern, err)
	}

	for _, sd := range sdAfter {
		if !sdBeforeSet.Has(sd) {
			return udevadmChangeToDrive(sd)
		}
	}

	return nil
}

// Calls "udevadm trigger --action=change" on the specified drive.
// drivePath must be the block device path to trigger on, in the format "/dev/sd*", or a symlink to it.
// This is workaround for Issue #7972. Once the underlying issue has been resolved, this may be removed.
func udevadmChangeToDrive(drivePath string) error {
	glog.V(5).Infof("udevadmChangeToDrive: drive=%q", drivePath)

	// Evaluate symlink, if any
	drive, err := filepath.EvalSymlinks(drivePath)
	if err != nil {
		return fmt.Errorf("udevadmChangeToDrive: filepath.EvalSymlinks(%q) failed with %v.", drivePath, err)
	}
	glog.V(5).Infof("udevadmChangeToDrive: symlink path is %q", drive)

	// Check to make sure input is "/dev/sd*"
	if !strings.Contains(drive, diskSDPath) {
		return fmt.Errorf("udevadmChangeToDrive: expected input in the form \"%s\" but drive is %q.", diskSDPattern, drive)
	}

	// Call "udevadm trigger --action=change --property-match=DEVNAME=/dev/sd..."
	_, err = exec.New().Command(
		"udevadm",
		"trigger",
		"--action=change",
		fmt.Sprintf("--property-match=DEVNAME=%s", drive)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("udevadmChangeToDrive: udevadm trigger failed for drive %q with %v.", drive, err)
	}
	return nil
}

// PathExists returns true if the specified path exists.
func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}
