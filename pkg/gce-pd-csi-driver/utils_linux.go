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
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

func getDevicePath(ns *GCENodeServer, volumeID, partition string) (string, error) {
	_, volumeKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return "", err
	}
	deviceName, err := common.GetDeviceName(volumeKey)
	if err != nil {
		return "", fmt.Errorf("error getting device name: %w", err)
	}
	devicePaths := ns.DeviceUtils.GetDiskByIdPaths(deviceName, partition)
	devicePath, err := ns.DeviceUtils.VerifyDevicePath(devicePaths, deviceName)
	if err != nil {
		return "", status.Error(codes.Internal, fmt.Sprintf("error verifying GCE PD (%q) is attached: %v", deviceName, err.Error()))
	}
	if devicePath == "" {
		return "", status.Error(codes.Internal, fmt.Sprintf("Unable to find device path out of attempted paths: %v", devicePaths))
	}
	return devicePath, nil
}

func (ns *GCENodeServer) formatAndMount(source, target, fstype string, options []string, m *mount.SafeFormatAndMount) error {
	if ns.formatAndMountSemaphore != nil {
		done := make(chan any)
		defer close(done)

		// Aquire the semaphore. This will block if another formatAndMount has put an item
		// into the semaphore channel.
		ns.formatAndMountSemaphore <- struct{}{}

		go func() {
			defer func() { <-ns.formatAndMountSemaphore }()

			// Add a timeout where so the semaphore will be released even if
			// formatAndMount is still working. This allows the node to make progress on
			// volumes if some error causes one formatAndMount to get stuck. The
			// motivation for this serialization is to reduce memory usage; if stuck
			// processes cause OOMs then the containers will be killed and restarted,
			// including the stuck threads and with any luck making progress.
			timeout := time.NewTimer(ns.formatAndMountTimeout)
			defer timeout.Stop()

			select {
			case <-done:
			case <-timeout.C:
			}
		}()
	}

	err := m.FormatAndMount(source, target, fstype, options)
	if ns.metricsManager != nil {
		ns.metricsManager.RecordMountErrorMetric(fstype, err)
	}
	return err
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

func getBlockSizeBytes(devicePath string, m *mount.SafeFormatAndMount) (int64, error) {
	output, err := m.Exec.Command("blockdev", "--getsize64", devicePath).CombinedOutput()
	if err != nil {
		return -1, fmt.Errorf("error when getting size of block volume at path %s: output: %s, err: %w", devicePath, string(output), err)
	}
	strOut := strings.TrimSpace(string(output))
	gotSizeBytes, err := strconv.ParseInt(strOut, 10, 64)
	if err != nil {
		return -1, fmt.Errorf("failed to parse %s into an int size", strOut)
	}
	return gotSizeBytes, nil
}

func setReadAheadKB(devicePath string, readAheadKB int64, m *mount.SafeFormatAndMount) error {
	output, err := m.Exec.Command("blockdev", "--getss", devicePath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error when reading sector size at path %s: output: %s, err: %w", devicePath, string(output), err)
	}
	strOut := strings.TrimSpace(string(output))
	sectorSizeBytes, err := strconv.ParseInt(strOut, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse %q into an int size", strOut)
	}
	readAheadInSectors := readAheadKB * 1024 / sectorSizeBytes
	readAheadInSectorsStr := strconv.FormatInt(readAheadInSectors, 10)
	// Empirical testing indicates that the actual read_ahead_kb size that is set is rounded to the
	// nearest 4KB.
	output, err = m.Exec.Command("blockdev", "--setra", readAheadInSectorsStr, devicePath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error when setting readahead at path %s: output: %s, err: %w", devicePath, string(output), err)
	}
	return nil
}
