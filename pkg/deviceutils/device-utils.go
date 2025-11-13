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
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
	pathutils "k8s.io/utils/path"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/resizefs"
)

const (
	diskByIdPath         = "/dev/disk/by-id/"
	diskGooglePrefix     = "google-"
	diskScsiGooglePrefix = "scsi-0Google_PersistentDisk_"
	diskPartitionSuffix  = "-part"
	diskSDPath           = "/dev/sd"
	diskSDGlob           = "/dev/sd*"
	diskNvmePath         = "/dev/nvme"
	diskNvmePattern      = "^/dev/nvme[0-9]+n[0-9]+$"
	diskNvmeGlob         = "/dev/nvme*"
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
	// google_nvme_id output should be in the form of:
	// ID_SERIAL_SHORT=<disk name>
	// Note: The google_nvme_id tool prints out multiple lines, hence we don't
	// use '^' and '$' to wrap nvmePattern as is done in scsiPattern.
	nvmePattern = `ID_SERIAL_SHORT=([\S]+)\s*`
	scsiIdPath  = "/lib/udev_containerized/scsi_id"
	nvmeIdPath  = "/lib/udev_containerized/google_nvme_id"
)

var (
	// regex to parse scsi_id output and extract the serial
	scsiRegex = regexp.MustCompile(scsiPattern)
	// regex to parse google_nvme_id output and extract the serial
	nvmeRegex = regexp.MustCompile(nvmePattern)
	// regex to filter for disk drive paths from filepath.Glob output of diskNvmeGlob
	diskNvmeRegex = regexp.MustCompile(diskNvmePattern)
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

	// Resize returns whether or not a device needs resizing.
	Resize(resizer resizefs.Resizefs, devicePath string, deviceMountPath string) (bool, error)

	// IsDeviceFilesystemInUse returns if a device path with the specified fstype
	// TODO: Mounter is passed in in order to call GetDiskFormat()
	// This is currently only implemented in mounter_linux, not mounter_windows.
	// Refactor this interface and function call up the stack to the caller once it is
	// implemented in mounter_windows.
	IsDeviceFilesystemInUse(mounter *mount.SafeFormatAndMount, devicePath, devFsPath string) (bool, error)
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
			return "", fmt.Errorf("error checking if path exists: %w", err)
		} else if pathExists {
			return devicePath, nil
		}
	}
	return "", nil
}

// getScsiSerial assumes that scsiIdPath exists and will error if it
// doesnt. It is the callers responsibility to verify the existence of this
// tool. Calls scsi_id on the given devicePath to get the serial number reported
// by that device.
func getScsiSerial(devicePath string) (string, error) {
	out, err := exec.Command(
		scsiIdPath,
		"--page=0x83",
		"--whitelisted",
		fmt.Sprintf("--device=%v", devicePath)).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("scsi_id failed for device %q with output %s: %w", devicePath, string(out), err)
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

// getNvmeSerial calls google_nvme_id on the given devicePath to get the serial
// number reported by that device.
// NOTE: getNvmeSerial assumes that nvmeIdPath exists and will error if it
// doesn't. It is the caller's responsibility to verify the existence of this
// tool.
func getNvmeSerial(devicePath string) (string, error) {
	out, err := exec.Command(
		nvmeIdPath,
		fmt.Sprintf("-d%s", devicePath)).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("google_nvme_id failed for device %q with output %s: %w", devicePath, out, err)
	}

	return parseNvmeSerial(string(out))
}

// Parse the output returned by google_nvme_id and extract the serial number
func parseNvmeSerial(output string) (string, error) {
	substrings := nvmeRegex.FindStringSubmatch(output)
	if substrings == nil {
		return "", fmt.Errorf("google_nvme_id output cannot be parsed: %q", output)
	}

	return substrings[1], nil
}

func ensureUdevToolExists(toolPath string) error {
	exists, err := pathutils.Exists(pathutils.CheckFollowSymlink, toolPath)
	if err != nil {
		return fmt.Errorf("failed to check existence of %q: %w", toolPath, err)
	}
	if !exists {
		// The driver should be containerized with the tool so maybe something is
		// wrong with the build process
		return fmt.Errorf("could not find tool at %q, unable to verify device paths", toolPath)
	}
	return nil
}

func ensureUdevToolsExist() error {
	if err := ensureUdevToolExists(scsiIdPath); err != nil {
		return err
	}
	if err := ensureUdevToolExists(nvmeIdPath); err != nil {
		return err
	}
	return nil
}

// VerifyDevicePath returns the first devicePath that maps to a real disk in the
// candidate devicePaths or an empty string if none is found.
// If the device is not found, it will attempt to fix any issues
// caused by missing paths or mismatched devices by running a udevadm --trigger.
func (m *deviceUtils) VerifyDevicePath(devicePaths []string, deviceName string) (string, error) {
	var devicePath string
	var err error
	const (
		pollInterval = 500 * time.Millisecond
		pollTimeout  = 3 * time.Second
	)

	// Ensure tools in /lib/udev_containerized directory exist
	err = ensureUdevToolsExist()
	if err != nil {
		return "", err
	}
	err = wait.Poll(pollInterval, pollTimeout, func() (bool, error) {
		var innerErr error
		devicePath, innerErr = existingDevicePath(devicePaths)
		if innerErr != nil {
			e := fmt.Errorf("for disk %s failed to check for existing device path: %w", deviceName, innerErr)
			klog.Errorf("Error: %s", e.Error())
			return false, e
		}

		if len(devicePath) == 0 {
			// Couldn't find a /dev/disk/by-id path for this deviceName, so we need to
			// find a /dev/* with a serial that matches deviceName. Then we attempt
			// to repair the symlink.
			klog.Warningf("For disk %s couldn't find a device path, calling udevadmTriggerForDiskIfExists", deviceName)
			innerErr := udevadmTriggerForDiskIfExists(deviceName)
			if innerErr != nil {
				e := fmt.Errorf("for disk %s failed to trigger udevadm fix of non existent device path: %w", deviceName, innerErr)
				klog.Errorf("Error: %s", e.Error())
				return false, e
			}
			// Go to next retry loop to get the deviceName again after
			// potentially fixing it with the udev command
			return false, nil
		}

		// If there exists a devicePath we make sure disk at /dev/* matches the
		// expected disk at devicePath by matching device Serial to the disk name
		devFsPath, innerErr := filepath.EvalSymlinks(devicePath)
		if innerErr != nil {
			e := fmt.Errorf("filepath.EvalSymlinks(%q) failed: %w", devicePath, innerErr)
			klog.Errorf("Error: %s", e.Error())
			return false, e
		}
		klog.V(4).Infof("For disk %s the /dev/* path is %s for disk/by-id path %s", deviceName, devFsPath, devicePath)

		devFsSerial, innerErr := getDevFsSerial(devFsPath)
		if innerErr != nil {
			e := fmt.Errorf("couldn't get serial number for disk %s at device path %s: %w", deviceName, devFsPath, innerErr)
			klog.Errorf("Error: %s", e.Error())
			return false, e
		}
		klog.V(4).Infof("For disk %s, device path %s, found serial number %s", deviceName, devFsPath, devFsSerial)
		// SUCCESS! devicePath points to a /dev/* path that has a serial
		// equivalent to our disk name
		if len(devFsSerial) != 0 && devFsSerial == deviceName {
			return true, nil
		}

		// A /dev/* path exists, but is either not a recognized /dev prefix type
		// (/dev/nvme* or /dev/sd*) or devicePath is not mapped to the correct disk.
		// Attempt a repair
		klog.Warningf("For disk %s and device path %s with mismatched serial number %q calling udevadmTriggerForDiskIfExists", deviceName, devFsPath, devFsSerial)
		innerErr = udevadmTriggerForDiskIfExists(deviceName)
		if innerErr != nil {
			e := fmt.Errorf("failed to trigger udevadm fix of misconfigured disk for %q: %w", deviceName, innerErr)
			klog.Errorf("Error: %s", e.Error())
			return false, e
		}
		// Go to next retry loop to get the deviceName again after
		// potentially fixing it with the udev command
		return false, nil
	})

	if err != nil {
		klog.Warningf("For device %s udevadmin failed: %v. Trying to manually link", deviceName, err)
		if err := manuallySetDevicePath(deviceName); err != nil {
			return "", fmt.Errorf("failed to manually set link for disk %s: %w", deviceName, err)
		}
	}

	return devicePath, nil
}

func (m *deviceUtils) Resize(resizer resizefs.Resizefs, devicePath string, deviceMountPath string) (bool, error) {
	return resizer.Resize(devicePath, deviceMountPath)
}

// getDevFsSerial returns the serial number of the /dev/* path at devFsPath.
// If devFsPath does not start with a known prefix, returns the empty string.
func getDevFsSerial(devFsPath string) (string, error) {
	switch {
	case strings.HasPrefix(devFsPath, diskSDPath):
		return getScsiSerial(devFsPath)
	case strings.HasPrefix(devFsPath, diskNvmePath):
		return getNvmeSerial(devFsPath)
	default:
		return "", nil
	}
}

func filterAvailableNvmeDevFsPaths(devNvmePaths []string) []string {
	// Devices under /dev/nvme need to be filtered for disk drive paths only.
	diskNvmePaths := []string{}
	for _, devNvmePath := range devNvmePaths {
		if diskNvmeRegex.MatchString(devNvmePath) {
			diskNvmePaths = append(diskNvmePaths, devNvmePath)
		}
	}
	return diskNvmePaths
}

func findAvailableDevFsPaths() ([]string, error) {
	diskSDPaths, err := filepath.Glob(diskSDGlob)
	if err != nil {
		return nil, fmt.Errorf("failed to filepath.Glob(\"%s\"): %w", diskSDGlob, err)
	}
	devNvmePaths, err := filepath.Glob(diskNvmeGlob)
	if err != nil {
		return nil, fmt.Errorf("failed to filepath.Glob(\"%s\"): %w", diskNvmeGlob, err)
	}
	// Devices under /dev/nvme need to be filtered for disk drive paths only.
	diskNvmePaths := filterAvailableNvmeDevFsPaths(devNvmePaths)
	return append(diskSDPaths, diskNvmePaths...), nil
}

func findDevice(deviceName string) (string, string, error) {
	devFsPathToSerial := map[string]string{}
	devFsPaths, err := findAvailableDevFsPaths()
	if err != nil {
		return "", "", err
	}
	for _, devFsPath := range devFsPaths {
		devFsSerial, err := getDevFsSerial(devFsPath)
		if err != nil || len(devFsSerial) == 0 {
			// If we get an error, ignore. Either this isn't a block device, or it
			// isn't something we can get a serial number from
			klog.Errorf("failed to get serial num for disk %s at device path %s: %v", deviceName, devFsPath, err.Error())
			continue
		}
		klog.V(4).Infof("device path %s, serial number %v", devFsPath, devFsSerial)
		devFsPathToSerial[devFsPath] = devFsSerial
		if devFsSerial == deviceName {
			return devFsPath, devFsSerial, nil
		}
	}
	return "", "", fmt.Errorf("udevadm --trigger requested to fix disk %s but no such disk was found in device path %v", deviceName, devFsPathToSerial)
}

func manuallySetDevicePath(deviceName string) error {
	devFsPath, devFsSerial, err := findDevice(deviceName)
	if err != nil {
		return err
	}
	return os.Symlink(devFsPath, path.Join(diskByIdPath, diskGooglePrefix+devFsSerial))
}

func udevadmTriggerForDiskIfExists(deviceName string) error {
	devFsPath, devFsSerial, err := findDevice(deviceName)
	if err != nil {
		return err
	}
	// Found the disk that we're looking for so run a trigger on it
	// to resolve its /dev/by-id/ path
	klog.Warningf("udevadm --trigger running to fix disk at path %s which has serial number %s", devFsPath, devFsSerial)
	err = udevadmChangeToDrive(devFsPath)
	if err != nil {
		return fmt.Errorf("udevadm --trigger failed to fix device path %s which has serial number %s: %w", devFsPath, devFsSerial, err)
	}
	return nil
}

// Calls "udevadm trigger --action=change" on the specified drive. drivePath
// must be the block device path to trigger on, in the format "/dev/*", or a
// symlink to it. This is workaround for Issue #7972
// (https://github.com/kubernetes/kubernetes/issues/7972). Once the underlying
// issue has been resolved, this may be removed.
// udevadm takes a little bit to work its magic in the background so any callers
// should not expect the trigger to complete instantly and may need to poll for
// the change
func udevadmChangeToDrive(devFsPath string) error {
	// Call "udevadm trigger --action=change --property-match=DEVNAME=/dev/..."
	cmd := exec.Command(
		"udevadm",
		"trigger",
		"--action=change",
		fmt.Sprintf("--property-match=DEVNAME=%s", devFsPath))
	klog.V(4).Infof("Running command: %s", cmd.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("udevadmChangeToDrive: udevadm trigger failed for drive %q with output %s: %v", devFsPath, string(out), err)
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
