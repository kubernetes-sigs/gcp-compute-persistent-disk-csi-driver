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
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	pathutils "k8s.io/utils/path"
)

const (
	diskByIdPath         = "/dev/disk/by-id/"
	diskGooglePrefix     = "google-"
	diskScsiGooglePrefix = "scsi-0Google_PersistentDisk_"
	diskPartitionSuffix  = "-part"
	diskSDPath           = "/dev/sd"
	diskSDPattern        = "/dev/sd*"
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
	// scsi_id output should be in the form of:
	// 0Google PersistentDisk <disk name>
	scsiPattern = `^0Google\s+PersistentDisk\s+([\S]+)\s*$`
)

var (
	// regex to parse scsi_id output and extract the serial
	scsiRegex = regexp.MustCompile(scsiPattern)
)

// DeviceUtils are a collection of methods that act on the devices attached
// to a GCE Instance
type DeviceUtils interface {
	// GetDiskByIdPaths returns a list of all possible paths for a
	// given Persistent Disk
	GetDiskByIdPaths(deviceName string, partition string) []string

	// VerifyDevicePath returns the first of the list of device paths that
	// exists on the machine, or an empty string if none exists
	VerifyDevicePath(devicePaths []string, deviceName string) (string, error)
}

type deviceUtils struct {
}

var _ DeviceUtils = &deviceUtils{}

func NewDeviceUtils() *deviceUtils {
	return &deviceUtils{}
}

// Returns list of all /dev/disk/by-id/* paths for given PD.
func (m *deviceUtils) GetDiskByIdPaths(deviceName string, partition string) []string {
	devicePaths := []string{
		path.Join(diskByIdPath, diskGooglePrefix+deviceName),
		path.Join(diskByIdPath, diskScsiGooglePrefix+deviceName),
	}

	if partition != "" {
		for i, path := range devicePaths {
			devicePaths[i] = path + diskPartitionSuffix + partition
		}
	}

	return devicePaths
}

func existingDevicePath(devicePaths []string) (string, error) {
	for _, devicePath := range devicePaths {
		if pathExists, err := pathExists(devicePath); err != nil {
			return "", fmt.Errorf("error checking if path exists: %v", err)
		} else if pathExists {
			return devicePath, nil
		}
	}
	return "", nil
}

// getScsiSerial assumes that /lib/udev/scsi_id exists and will error if it
// doesnt. It is the callers responsibility to verify the existence of this
// tool. Calls scsi_id on the given devicePath to get the serial number reported
// by that device.
func getScsiSerial(devicePath string) (string, error) {
	out, err := exec.Command(
		"/lib/udev_containerized/scsi_id",
		"--page=0x83",
		"--whitelisted",
		fmt.Sprintf("--device=%v", devicePath)).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("scsi_id failed for device %q with output %s: %v", devicePath, string(out), err)
	}

	return parseScsiSerial(string(out))
}

// Parse the output returned by scsi_id and extract the serial number
func parseScsiSerial(output string) (string, error) {
	substrings := scsiRegex.FindStringSubmatch(output)
	if substrings == nil {
		return "", fmt.Errorf("scsi_id output cannot be parsed: %q", output)
	}

	return substrings[1], nil
}

// VerifyDevicePath returns the first devicePath that maps to a real disk in the
// candidate devicePaths or an empty string if none is found. If
// /lib/udev_containerized/scsi_id exists it will attempt to fix any issues
// caused by missing paths or mismatched devices by running a udevadm --trigger.
func (m *deviceUtils) VerifyDevicePath(devicePaths []string, deviceName string) (string, error) {
	var devicePath string
	var err error
	const (
		pollInterval = 500 * time.Millisecond
		pollTimeout  = 3 * time.Second
	)

	scsiIDPath := "/lib/udev_containerized/scsi_id"
	exists, err := pathutils.Exists(pathutils.CheckFollowSymlink, scsiIDPath)
	if err != nil {
		return "", fmt.Errorf("failed to check scsi_id existence: %v", err)
	}
	if !exists {
		// No SCSI ID tool, the driver should be containerized with the tool so
		// maybe something is wrong with the build process
		return "", fmt.Errorf("could not find scsi_id tool at %s, unable to verify device paths", scsiIDPath)
	}

	err = wait.Poll(pollInterval, pollTimeout, func() (bool, error) {
		var innerErr error

		devicePath, innerErr = existingDevicePath(devicePaths)
		if innerErr != nil {
			return false, fmt.Errorf("failed to check for existing device path: %v", innerErr)
		}

		if len(devicePath) == 0 {
			// Couldn't find the path so we need to find a /dev/sdx with the SCSI
			// serial that matches deviceName. Then we run udevadm trigger on that
			// device to get the device to show up in /dev/by-id/
			innerErr := udevadmTriggerForDiskIfExists(deviceName)
			if innerErr != nil {
				return false, fmt.Errorf("failed to trigger udevadm fix: %v", innerErr)
			}
			// Go to next retry loop to get the deviceName again after
			// potentially fixing it with the udev command
			return false, nil
		}

		// If there exists a devicePath we make sure disk at /dev/sdx matches the
		// expected disk at devicePath by matching SCSI Serial to the disk name
		devSDX, innerErr := filepath.EvalSymlinks(devicePath)
		if innerErr != nil {
			return false, fmt.Errorf("filepath.EvalSymlinks(%q) failed with %v", devicePath, innerErr)
		}
		// Check to make sure device path maps to the correct disk
		if strings.Contains(devSDX, diskSDPath) {
			scsiSerial, innerErr := getScsiSerial(devSDX)
			if innerErr != nil {
				return false, fmt.Errorf("couldn't get SCSI serial number for disk %s: %v", deviceName, innerErr)
			}
			// SUCCESS! devicePath points to a /dev/sdx that has a SCSI serial
			// equivilant to our disk name
			if scsiSerial == deviceName {
				return true, nil
			}
		}
		// The devicePath is not mapped to the correct disk
		innerErr = udevadmTriggerForDiskIfExists(deviceName)
		if innerErr != nil {
			return false, fmt.Errorf("failed to trigger udevadm fix: %v", innerErr)
		}
		// Go to next retry loop to get the deviceName again after
		// potentially fixing it with the udev command
		return false, nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to find and re-link disk %s with udevadm after retrying for %v: %v", deviceName, pollTimeout, err)
	}

	return devicePath, nil
}

func udevadmTriggerForDiskIfExists(deviceName string) error {
	devToSCSI := map[string]string{}
	sds, err := filepath.Glob(diskSDPattern)
	if err != nil {
		return fmt.Errorf("failed to filepath.Glob(\"%s\"): %v", diskSDPattern, err)
	}
	for _, devSDX := range sds {
		scsiSerial, err := getScsiSerial(devSDX)
		if err != nil {
			return fmt.Errorf("failed to get SCSI Serial num: %v", err)
		}
		devToSCSI[devSDX] = scsiSerial
		if scsiSerial == deviceName {
			// Found the disk that we're looking for so run a trigger on it
			// to resolve its /dev/by-id/ path
			klog.Warningf("udevadm --trigger running to fix disk at path %s which has SCSI ID %s", devSDX, scsiSerial)
			err := udevadmChangeToDrive(devSDX)
			if err != nil {
				return fmt.Errorf("failed to fix disk which has SCSI ID %s: %v", scsiSerial, err)
			}
			return nil
		}
	}
	klog.Warningf("udevadm --trigger requested to fix disk %s but no such disk was found in %v", deviceName, devToSCSI)
	return fmt.Errorf("udevadm --trigger requested to fix disk %s but no such disk was found", deviceName)
}

// Calls "udevadm trigger --action=change" on the specified drive. drivePath
// must be the block device path to trigger on, in the format "/dev/sd*", or a
// symlink to it. This is workaround for Issue #7972. Once the underlying issue
// has been resolved, this may be removed.
// udevadm takes a little bit to work its magic in the background so any callers
// should not expect the trigger to complete instantly and may need to poll for
// the change
func udevadmChangeToDrive(devSDX string) error {
	// Call "udevadm trigger --action=change --property-match=DEVNAME=/dev/sd..."
	out, err := exec.Command(
		"udevadm",
		"trigger",
		"--action=change",
		fmt.Sprintf("--property-match=DEVNAME=%s", devSDX)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("udevadmChangeToDrive: udevadm trigger failed for drive %q with output %s: %v.", devSDX, string(out), err)
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
