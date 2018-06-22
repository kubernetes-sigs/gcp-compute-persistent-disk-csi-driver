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

package mountmanager

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/exec"
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

// TODO(dyzz): Should be able to clean up this interface some more. Not everything here needs to be here
type Mounter interface {
	DoMount(source string, target string, fstype string, options []string) error
	FormatAndMount(source string, target string, fstype string, options []string) error
	IsLikelyNotMountPoint(file string) (bool, error)
	UnmountVolume(targetPath string) (string, error)
	GetDiskByIdPaths(pdName string, partition string) []string
	VerifyDevicePath(devicePaths []string) (string, error)
	MkdirAll(path string, perm os.FileMode) error
	Remove(name string) error
}

type GCEMounter struct {
}

func CreateMounter() (*GCEMounter, error) {
	return &GCEMounter{}, nil
}

// IsLikelyNotMountPoint determines if a directory is not a mountpoint.
// It is fast but not necessarily ALWAYS correct. If the path is in fact
// a bind mount from one part of a mount to another it will not be detected.
// mkdir /tmp/a /tmp/b; mount --bin /tmp/a /tmp/b; IsLikelyNotMountPoint("/tmp/b")
// will return true. When in fact /tmp/b is a mount point. If this situation
// if of interest to you, don't use this function...
func (m *GCEMounter) IsLikelyNotMountPoint(file string) (bool, error) {
	stat, err := os.Stat(file)
	if err != nil {
		return true, err
	}
	rootStat, err := os.Lstat(file + "/..")
	if err != nil {
		return true, err
	}
	// If the directory has a different device as parent, then it is a mountpoint.
	if stat.Sys().(*syscall.Stat_t).Dev != rootStat.Sys().(*syscall.Stat_t).Dev {
		return false, nil
	}

	return true, nil
}

func (m *GCEMounter) UnmountVolume(targetPath string) (string, error) {
	output, err := execRun("umount", targetPath)
	return string(output), err
}

func (m *GCEMounter) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// m.DoMount runs the mount command. mounterPath is the path to mounter binary if containerized mounter is used.
func (m *GCEMounter) DoMount(source string, target string, fstype string, options []string) error {
	mountCmd := defaultMountCommand
	mountArgs := makeMountArgs(source, target, fstype, options)

	glog.V(4).Infof("Mounting cmd (%s) with arguments (%s)", mountCmd, mountArgs)
	output, err := execRun(mountCmd, mountArgs...)
	if err != nil {
		args := strings.Join(mountArgs, " ")
		glog.Errorf("Mount failed: %v\nMounting command: %s\nMounting arguments: %s\nOutput: %s\n", err, mountCmd, args, string(output))
		return fmt.Errorf("mount failed: %v\nMounting command: %s\nMounting arguments: %s\nOutput: %s",
			err, mountCmd, args, string(output))
	}
	return err
}

func (m *GCEMounter) Remove(name string) error {
	return os.Remove(name)
}

// formatAndMount uses unix utils to format and mount the given disk
func (m *GCEMounter) FormatAndMount(source string, target string, fstype string, options []string) error {
	options = append(options, "defaults")

	// Run fsck on the disk to fix repairable issues
	glog.V(4).Infof("Checking for issues with fsck on disk: %s", source)
	args := []string{"-a", source}
	out, err := execRun("fsck", args...)
	if err != nil {
		ee, isExitError := err.(exec.ExitError)
		switch {
		case err == exec.ErrExecutableNotFound:
			glog.Warningf("'fsck' not found on system; continuing mount without running 'fsck'.")
		case isExitError && ee.ExitStatus() == fsckErrorsCorrected:
			glog.Infof("Device %s has errors which were corrected by fsck.", source)
		case isExitError && ee.ExitStatus() == fsckErrorsUncorrected:
			return fmt.Errorf("'fsck' found errors on device %s but could not correct them: %s.", source, string(out))
		case isExitError && ee.ExitStatus() > fsckErrorsUncorrected:
			glog.Infof("`fsck` error %s", string(out))
		}
	}

	// Try to mount the disk
	glog.V(4).Infof("Attempting to mount disk: %s %s %s", fstype, source, target)
	mountErr := m.DoMount(source, target, fstype, options)
	if mountErr != nil {
		// Mount failed. This indicates either that the disk is unformatted or
		// it contains an unexpected filesystem.
		existingFormat, err := getDiskFormat(source)
		if err != nil {
			return err
		}
		if existingFormat == "" {
			// Disk is unformatted so format it.
			args = []string{source}
			// Use 'ext4' as the default
			if len(fstype) == 0 {
				fstype = "ext4"
			}

			if fstype == "ext4" || fstype == "ext3" {
				args = []string{"-F", source}
			}
			glog.Infof("Disk %q appears to be unformatted, attempting to format as type: %q with options: %v", source, fstype, args)
			_, err := execRun("mkfs."+fstype, args...)
			if err == nil {
				// the disk has been formatted successfully try to mount it again.
				glog.Infof("Disk successfully formatted (mkfs): %s - %s %s", fstype, source, target)
				return m.DoMount(source, target, fstype, options)
			}
			glog.Errorf("format of disk %q failed: type:(%q) target:(%q) options:(%q)error:(%v)", source, fstype, target, options, err)
			return err
		} else {
			// Disk is already formatted and failed to mount
			if len(fstype) == 0 || fstype == existingFormat {
				// This is mount error
				return mountErr
			} else {
				// Block device is formatted with unexpected filesystem, let the user know
				return fmt.Errorf("failed to mount the volume as %q, it already contains %s. Mount error: %v", fstype, existingFormat, mountErr)
			}
		}
	}
	return mountErr
}

// Returns list of all /dev/disk/by-id/* paths for given PD.
func (m *GCEMounter) GetDiskByIdPaths(pdName string, partition string) []string {
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
func (m *GCEMounter) VerifyDevicePath(devicePaths []string) (string, error) {
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
