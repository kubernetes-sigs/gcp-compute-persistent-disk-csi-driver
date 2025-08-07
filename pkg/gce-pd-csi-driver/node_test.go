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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k8s.io/utils/exec"
	testingexec "k8s.io/utils/exec/testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/deviceutils"
	metadataservice "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/metadata"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/linkcache"
	mountmanager "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"
)

const (
	defaultVolumeID    = "project/test001/zones/c1/disks/testDisk"
	defaultTargetPath  = "/mnt/test"
	defaultStagingPath = "/staging"
)

func getTestGCEDriver(t *testing.T) *GCEDriver {
	return getCustomTestGCEDriver(t, mountmanager.NewFakeSafeMounter(), deviceutils.NewFakeDeviceUtils(false), metadataservice.NewFakeService(), &NodeServerArgs{
		DeviceCache: linkcache.NewTestDeviceCache(1*time.Minute, linkcache.NewTestNodeWithVolumes([]string{defaultVolumeID})),
	})
}

func getTestGCEDriverWithCustomMounter(t *testing.T, mounter *mount.SafeFormatAndMount, args *NodeServerArgs) *GCEDriver {
	return getCustomTestGCEDriver(t, mounter, deviceutils.NewFakeDeviceUtils(false), metadataservice.NewFakeService(), args)
}

func getCustomTestGCEDriver(t *testing.T, mounter *mount.SafeFormatAndMount, deviceUtils deviceutils.DeviceUtils, metaService metadataservice.MetadataService, args *NodeServerArgs) *GCEDriver {
	gceDriver := GetGCEDriver()
	nodeServer := NewNodeServer(gceDriver, mounter, deviceUtils, metaService, mountmanager.NewFakeStatter(mounter), args)
	err := gceDriver.SetupGCEDriver(driver, "test-vendor", nil, nil, nil, nil, nodeServer)
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}
	return gceDriver
}

func getTestBlockingMountGCEDriver(t *testing.T, readyToExecute chan chan struct{}) *GCEDriver {
	gceDriver := GetGCEDriver()
	mounter := mountmanager.NewFakeSafeBlockingMounter(readyToExecute)
	args := &NodeServerArgs{
		EnableDeviceInUseCheck:   true,
		DeviceInUseTimeout:       0,
		EnableDataCache:          true,
		DataCacheEnabledNodePool: false,
	}
	nodeServer := NewNodeServer(gceDriver, mounter, deviceutils.NewFakeDeviceUtils(false), metadataservice.NewFakeService(), mountmanager.NewFakeStatter(mounter), args)
	err := gceDriver.SetupGCEDriver(driver, "test-vendor", nil, nil, nil, nil, nodeServer)
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}
	return gceDriver
}

func getTestBlockingFormatAndMountGCEDriver(t *testing.T, readyToExecute chan chan struct{}) *GCEDriver {
	gceDriver := GetGCEDriver()
	enableDataCache := true
	mounter := mountmanager.NewFakeSafeBlockingMounter(readyToExecute)
	args := &NodeServerArgs{
		EnableDeviceInUseCheck:   true,
		DeviceInUseTimeout:       0,
		EnableDataCache:          enableDataCache,
		DataCacheEnabledNodePool: false,
	}
	nodeServer := NewNodeServer(gceDriver, mounter, deviceutils.NewFakeDeviceUtils(false), metadataservice.NewFakeService(), mountmanager.NewFakeStatter(mounter), args).WithSerializedFormatAndMount(5*time.Second, 1)

	err := gceDriver.SetupGCEDriver(driver, "test-vendor", nil, nil, nil, nil, nodeServer)
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}
	return gceDriver
}

func makeFakeCmd(fakeCmd *testingexec.FakeCmd, cmd string, args ...string) testingexec.FakeCommandAction {
	c := cmd
	a := args
	return func(cmd string, args ...string) exec.Cmd {
		command := testingexec.InitFakeCmd(fakeCmd, c, a...)
		return command
	}
}

func TestNodeGetVolumeStats(t *testing.T) {
	gceDriver := getTestGCEDriver(t)
	ns := gceDriver.ns

	tempDir, err := os.MkdirTemp("", "ngvs")
	if err != nil {
		t.Fatalf("Failed to set up temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	targetPath := filepath.Join(tempDir, defaultTargetPath)
	stagingPath := filepath.Join(tempDir, defaultStagingPath)

	req := &csi.NodePublishVolumeRequest{
		VolumeId:          defaultVolumeID,
		TargetPath:        targetPath,
		StagingTargetPath: stagingPath,
		Readonly:          false,
		VolumeCapability:  stdVolCap,
	}

	_, err = ns.NodePublishVolume(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to set up test by publishing default vol: %v", err)
	}

	testCases := []struct {
		name           string
		volumeID       string
		volumePath     string
		expectedResp   *csi.NodeGetVolumeStatsResponse
		deviceCapacity int
		expectErr      bool
	}{
		{
			name:           "normal",
			volumeID:       defaultVolumeID,
			volumePath:     targetPath,
			deviceCapacity: 300 * 1024 * 1024 * 1024, // 300 GB
			expectedResp: &csi.NodeGetVolumeStatsResponse{
				Usage: []*csi.VolumeUsage{
					{
						Unit:  csi.VolumeUsage_BYTES,
						Total: 300 * 1024 * 1024 * 1024, // 300 GB,
					},
				},
			},
		},
		{
			name:       "no vol id",
			volumePath: targetPath,
			expectErr:  true,
		},
		{
			name:      "no vol path",
			volumeID:  defaultVolumeID,
			expectErr: true,
		},
		{
			name:       "bad vol path",
			volumeID:   defaultVolumeID,
			volumePath: "/mnt/fake",
			expectErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actionList := []testingexec.FakeCommandAction{
				makeFakeCmd(
					&testingexec.FakeCmd{
						CombinedOutputScript: []testingexec.FakeAction{
							func() ([]byte, []byte, error) {
								return []byte(fmt.Sprintf("%d", tc.deviceCapacity)), nil, nil
							},
						},
					},
					"blockdev",
					strings.Split("--getsize64 /dev/disk/fake-path", " ")...,
				),
			}

			mounter := mountmanager.NewFakeSafeMounterWithCustomExec(&testingexec.FakeExec{CommandScript: actionList})
			gceDriver := getTestGCEDriverWithCustomMounter(t, mounter, &NodeServerArgs{
				DeviceCache: linkcache.NewTestDeviceCache(1*time.Minute, linkcache.NewTestNodeWithVolumes([]string{tc.volumeID})),
			})
			ns := gceDriver.ns

			req := &csi.NodeGetVolumeStatsRequest{
				VolumeId:   tc.volumeID,
				VolumePath: tc.volumePath,
			}
			resp, err := ns.NodeGetVolumeStats(context.Background(), req)
			if err != nil && !tc.expectErr {
				t.Fatalf("Got unexpected err: %v", err)
			}
			if err == nil && tc.expectErr {
				t.Fatal("Did not get error but expected one")
			}
			nodeComparer := cmp.Comparer(func(a, b *csi.NodeGetVolumeStatsResponse) bool {
				if a == nil {
					return b == nil
				}
				if b == nil {
					return false
				}
				volUsageComparer := cmp.Comparer(func(x, y *csi.VolumeUsage) bool {
					if x == nil {
						return y == nil
					}
					if y == nil {
						return false
					}
					return x.Unit == y.Unit && x.Total == y.Total
				})
				return cmp.Diff(a.Usage, b.Usage, volUsageComparer) == ""
			})
			if diff := cmp.Diff(tc.expectedResp, resp, nodeComparer); diff != "" {
				t.Errorf("NodeGetVolumeStats(%s): -want, +got \n%s", req, diff)
			}
		})
	}
}

func TestNodeGetVolumeLimits(t *testing.T) {
	gceDriver := getTestGCEDriver(t)
	ns := gceDriver.ns
	req := &csi.NodeGetInfoRequest{}

	testCases := []struct {
		name           string
		machineType    string
		expVolumeLimit int64
		expectError    bool
	}{
		{
			name:           "Predifined standard machine",
			machineType:    "n1-standard-1",
			expVolumeLimit: volumeLimitBig,
		},
		{
			name:           "Predifined micro machine",
			machineType:    "f1-micro",
			expVolumeLimit: volumeLimitSmall,
		},
		{
			name:           "Predifined small machine",
			machineType:    "g1-small",
			expVolumeLimit: volumeLimitSmall,
		},
		{
			name:           "Custom machine with 1GiB Mem",
			machineType:    "custom-1-1024",
			expVolumeLimit: volumeLimitBig,
		},
		{
			name:           "Custom machine with 4GiB Mem",
			machineType:    "custom-2-4096",
			expVolumeLimit: volumeLimitBig,
		},
		{
			name:           "Predifined e2 machine",
			machineType:    "e2-micro",
			expVolumeLimit: volumeLimitSmall,
		},
		{
			name:           "c4-standard-192",
			machineType:    "c4-standard-192",
			expVolumeLimit: 127,
		},
		{
			name:           "c4-standard-48",
			machineType:    "c4-standard-48",
			expVolumeLimit: 63,
		},
		{
			name:           "c4-standard-2",
			machineType:    "c4-standard-2",
			expVolumeLimit: 7,
		},
		{
			name:           "c4a-standard-4",
			machineType:    "c4a-standard-4",
			expVolumeLimit: 15,
		},
		{
			name:           "n4-standard-16",
			machineType:    "n4-standard-16",
			expVolumeLimit: 31,
		},
		{
			name:           "n4-micro", // This type does not exist, but testing edge cases
			machineType:    "n4-micro",
			expVolumeLimit: volumeLimitBig,
			expectError:    true,
		},
		{
			name:           "n4-highcpu-4",
			machineType:    "n4-highcpu-4",
			expVolumeLimit: 15,
		},
		{
			name:           "n4-custom-8-12345-ext",
			machineType:    "n4-custom-8-12345-ext",
			expVolumeLimit: 15,
		},
		{
			name:           "n4-custom-16-12345",
			machineType:    "n4-custom-16-12345",
			expVolumeLimit: 31,
		},
		{
			name:           "invalid gen4 machine type",
			machineType:    "n4-highcpu-4xyz",
			expVolumeLimit: volumeLimitBig,
			expectError:    true,
		},
		{
			name:           "x4-megamem-960-metal",
			machineType:    "x4-megamem-960-metal",
			expVolumeLimit: x4HyperdiskLimit,
		},
		{
			name:           "a4-highgpu-8g",
			machineType:    "a4-highgpu-8g",
			expVolumeLimit: a4HyperdiskLimit,
		},
		{
			name:           "c3-standard-4",
			machineType:    "c3-standard-4",
			expVolumeLimit: volumeLimitBig,
		},
		{
			name:           "c3d-highmem-8-lssd",
			machineType:    "c3d-highmem-8-lssd",
			expVolumeLimit: volumeLimitBig,
		},
		{
			name:           "c4a-standard-32-lssd",
			machineType:    "c4a-standard-32-lssd",
			expVolumeLimit: 31,
		},
		{
			name:           "c4d-standard-32",
			machineType:    "c4d-standard-32",
			expVolumeLimit: 31,
		},
		{
			name:           "c3-highcpu-192-metal",
			machineType:    "c3-highcpu-192-metal",
			expVolumeLimit: c3MetalHyperdiskLimit,
		},
		{
			name:           "c3-standard-192-metal",
			machineType:    "c3-standard-192-metal",
			expVolumeLimit: c3MetalHyperdiskLimit,
		},
		{
			name:           "c3-highmem-192-metal",
			machineType:    "c3-highmem-192-metal",
			expVolumeLimit: c3MetalHyperdiskLimit,
		},
		{
			name:           "a4x-highgpu-1g",
			machineType:    "a4x-highgpu-1g",
			expVolumeLimit: 63,
		},
		{
			name:           "a4x-highgpu-2g",
			machineType:    "a4x-highgpu-2g",
			expVolumeLimit: 127,
		},
		{
			name:           "a4x-highgpu-2g-nolssd",
			machineType:    "a4x-highgpu-2g-nolssd",
			expVolumeLimit: 127,
		},
		{
			name:           "a4x-highgpu-4g",
			machineType:    "a4x-highgpu-4g",
			expVolumeLimit: 127,
		},
		{
			name:           "a4x-highgpu-8g",
			machineType:    "a4x-highgpu-8g",
			expVolumeLimit: 127,
		},
		{
			name:           "a4x-highgpu-metal",
			machineType:    "a4x-highgpu-metal",
			expVolumeLimit: a4xMetalHyperdiskLimit,
		},
		{
			name:           "a4x-max-metal",
			machineType:    "a4x-max-metal",
			expVolumeLimit: a4xMetalHyperdiskLimit,
		},
		{
			name:           "a4x-max-1g",
			machineType:    "a4x-max-1g",
			expVolumeLimit: 63,
		},
		{
			name:           "a4x-max-highgpu-2g",
			machineType:    "a4x-max-2g",
			expVolumeLimit: 127,
		},
		{
			name:           "a4x-max-4g",
			machineType:    "a4x-max-4g",
			expVolumeLimit: 127,
		},
		{
			name:           "a4x-max-8g", // -8g does not exist, testing edge case
			machineType:    "a4x-max-8g",
			expVolumeLimit: 127,
		},
		{
			name:           "a4x-medgpu-nolssd", // does not exist, testing edge case
			machineType:    "a4x-medgpu-nolssd",
			expVolumeLimit: volumeLimitBig,
		},
	}

	for _, tc := range testCases {
		t.Logf("Test case: %s", tc.name)

		metadataservice.SetMachineType(tc.machineType)
		res, err := ns.NodeGetInfo(context.Background(), req)
		if err != nil && !tc.expectError {
			t.Fatalf("Failed to get node info: %v", err)
		}

		volumeLimit := res.GetMaxVolumesPerNode()
		if volumeLimit != tc.expVolumeLimit {
			t.Fatalf("Expected volume limit: %v, got %v, for machine-type: %v",
				tc.expVolumeLimit, volumeLimit, tc.machineType)
		}

		t.Logf("Get node info: %v", res)
	}
}

func TestNodePublishVolume(t *testing.T) {
	gceDriver := getTestGCEDriver(t)
	ns := gceDriver.ns

	tempDir, err := os.MkdirTemp("", "npv")
	if err != nil {
		t.Fatalf("Failed to set up temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	targetPath := filepath.Join(tempDir, defaultTargetPath)
	stagingPath := filepath.Join(tempDir, defaultStagingPath)

	testCases := []struct {
		name       string
		req        *csi.NodePublishVolumeRequest
		expErrCode codes.Code
	}{
		{
			name: "Valid request",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          defaultVolumeID,
				TargetPath:        targetPath,
				StagingTargetPath: stagingPath,
				Readonly:          false,
				VolumeCapability:  stdVolCap,
			},
		},
		{
			name: "Invalid request (invalid access mode)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          defaultVolumeID,
				TargetPath:        targetPath,
				StagingTargetPath: stagingPath,
				Readonly:          false,
				VolumeCapability:  createVolumeCapability(csi.VolumeCapability_AccessMode_UNKNOWN),
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No VolumeId)",
			req: &csi.NodePublishVolumeRequest{
				TargetPath:        targetPath,
				StagingTargetPath: stagingPath,
				Readonly:          false,
				VolumeCapability:  stdVolCap,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No TargetPath)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          defaultVolumeID,
				StagingTargetPath: stagingPath,
				Readonly:          false,
				VolumeCapability:  stdVolCap,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No StagingTargetPath)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         defaultVolumeID,
				TargetPath:       targetPath,
				Readonly:         false,
				VolumeCapability: stdVolCap,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (Nil VolumeCapability)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          defaultVolumeID,
				TargetPath:        targetPath,
				StagingTargetPath: stagingPath,
				Readonly:          false,
				VolumeCapability:  nil,
			},
			expErrCode: codes.InvalidArgument,
		},
	}
	for _, tc := range testCases {
		t.Logf("Test case: %s", tc.name)
		_, err := ns.NodePublishVolume(context.Background(), tc.req)
		if err != nil {
			serverError, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Could not get error status code from err: %v", err)
			}
			if serverError.Code() != tc.expErrCode {
				t.Fatalf("Expected error code: %v, got: %v. err : %v", tc.expErrCode, serverError.Code(), err)
			}
			continue
		}
		if tc.expErrCode != codes.OK {
			t.Fatalf("Expected error: %v, got no error", tc.expErrCode)
		}
	}
}

func TestNodeUnpublishVolume(t *testing.T) {
	gceDriver := getTestGCEDriver(t)
	ns := gceDriver.ns

	tempDir, err := os.MkdirTemp("", "nupv")
	if err != nil {
		t.Fatalf("Failed to set up temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	targetPath := filepath.Join(tempDir, defaultTargetPath)

	testCases := []struct {
		name       string
		req        *csi.NodeUnpublishVolumeRequest
		expErrCode codes.Code
	}{
		{
			name: "Valid request",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   defaultVolumeID,
				TargetPath: targetPath,
			},
		},
		{
			name: "Invalid request (No VolumeId)",
			req: &csi.NodeUnpublishVolumeRequest{
				TargetPath: targetPath,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No TargetPath)",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId: defaultVolumeID,
			},
			expErrCode: codes.InvalidArgument,
		},
	}
	for _, tc := range testCases {
		t.Logf("Test case: %s", tc.name)
		_, err := ns.NodeUnpublishVolume(context.Background(), tc.req)
		if err != nil {
			serverError, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Could not get error status code from err: %v", err)
			}
			if serverError.Code() != tc.expErrCode {
				t.Fatalf("Expected error code: %v, got: %v. err : %v", tc.expErrCode, serverError.Code(), err)
			}
			continue
		}
		if tc.expErrCode != codes.OK {
			t.Fatalf("Expected error: %v, got no error", tc.expErrCode)
		}
	}
}

type fakeCmd struct {
	cmd    string
	args   string
	stdout string
	err    error
}

func TestNodeStageVolume(t *testing.T) {
	volumeID := "project/test001/zones/c1/disks/testDisk"
	blockCap := &csi.VolumeCapability_Block{
		Block: &csi.VolumeCapability_BlockVolume{},
	}
	cap := &csi.VolumeCapability{
		AccessType: blockCap,
	}

	tempDir, err := os.MkdirTemp("", "nsv")
	if err != nil {
		t.Fatalf("Failed to set up temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	stagingPath := filepath.Join(tempDir, defaultStagingPath)

	btrfsUUID := "00000000-0000-0000-0000-000000000001"
	btrfsPrefix := fmt.Sprintf("%s/sys/fs/btrfs/%s/allocation", tempDir, btrfsUUID)

	for _, suffix := range []string{"data", "metadata"} {
		dir := btrfsPrefix + "/" + suffix
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to set up fake sysfs dir %q: %v", dir, err)
		}
		fname := dir + "/bg_reclaim_threshold"
		if err := os.WriteFile(fname, []byte("0\n"), 0644); err != nil {
			t.Fatalf("write %q: %v", fname, err)
		}
	}

	testCases := []struct {
		name                 string
		req                  *csi.NodeStageVolumeRequest
		deviceSize           int
		blockExtSize         int
		readonlyBit          string
		expResize            bool
		expReadAheadUpdate   bool
		expReadAheadKB       string
		expReadOnlyRemount   bool
		expCommandList       []fakeCmd
		readAheadSectors     string
		btrfsReclaimData     string
		btrfsReclaimMetadata string
		sectorSizeInBytes    int
		expErrCode           codes.Code
	}{
		{
			name: "Valid request, resize even though block and filesystem sizes match",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability:  stdVolCap,
			},
			deviceSize:   1,
			blockExtSize: 1,
			readonlyBit:  "0",
			expResize:    true,
			expCommandList: []fakeCmd{
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "fsck",
					args:   "-a /dev/disk/fake-path",
					stdout: "",
				},
				{
					cmd:    "blockdev",
					args:   "--getro /dev/disk/fake-path",
					stdout: "%v",
				},
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "resize2fs",
					args:   "/dev/disk/fake-path",
					stdout: "",
				},
			},
		},
		{
			name: "Valid request, no resize bc readonly",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability:  stdVolCap,
			},
			deviceSize:   1,
			blockExtSize: 1,
			readonlyBit:  "1",
			expResize:    false,
			expCommandList: []fakeCmd{
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "fsck",
					args:   "-a /dev/disk/fake-path",
					stdout: "",
				},
				{
					cmd:    "blockdev",
					args:   "--getro /dev/disk/fake-path",
					stdout: "%v",
				},
			},
		},
		{
			name: "Valid request, resize bc size",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability:  stdVolCap,
			},
			deviceSize:   5,
			blockExtSize: 1,
			readonlyBit:  "0",
			expResize:    true,
			expCommandList: []fakeCmd{
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "fsck",
					args:   "-a /dev/disk/fake-path",
					stdout: "",
				},
				{
					cmd:    "blockdev",
					args:   "--getro /dev/disk/fake-path",
					stdout: "%v",
				},
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "resize2fs",
					args:   "/dev/disk/fake-path",
					stdout: "",
				},
			},
		},
		{
			name: "Valid request, no resize bc readonly capability",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability:  createVolumeCapability(csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY),
			},
			deviceSize:   5,
			blockExtSize: 1,
			readonlyBit:  "0",
			expResize:    false,
			expCommandList: []fakeCmd{
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
			},
		},
		{
			name: "btrfs-allocation-data-bg_reclaim_threshold is ignored on non-btrfs",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType:     "ext4",
							MountFlags: []string{"btrfs-allocation-data-bg_reclaim_threshold=90"},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			deviceSize:       1,
			blockExtSize:     1,
			readonlyBit:      "0",
			btrfsReclaimData: "0",
			expResize:        true,
			expCommandList: []fakeCmd{
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "fsck",
					args:   "-a /dev/disk/fake-path",
					stdout: "",
				},
				{
					cmd:    "blockdev",
					args:   "--getro /dev/disk/fake-path",
					stdout: "%v",
				},
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "resize2fs",
					args:   "/dev/disk/fake-path",
					stdout: "",
				},
			},
		},
		{
			name: "Valid request, set btrfs-allocation-data-bg_reclaim_threshold=90",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType:     "btrfs",
							MountFlags: []string{"btrfs-allocation-data-bg_reclaim_threshold=90"},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			deviceSize:       1,
			blockExtSize:     1,
			readonlyBit:      "0",
			btrfsReclaimData: "90",
			expCommandList: []fakeCmd{
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "fsck",
					args:   "-a /dev/disk/fake-path",
					stdout: "",
				},
				{
					cmd:    "blockdev",
					args:   "--getro /dev/disk/fake-path",
					stdout: "%v",
				},
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "blockdev",
					args:   "--getsize64 /dev/disk/fake-path",
					stdout: "%d",
				},
				{
					cmd:    "btrfs",
					args:   "inspect-internal dump-super -f /dev/disk/fake-path",
					stdout: "sectorsize %d\ntotal_bytes %d",
				},
				{
					cmd:    "blkid",
					args:   fmt.Sprintf("--match-tag UUID --output value %v", stagingPath),
					stdout: btrfsUUID + "\n",
				},
			},
		},
		{
			name: "Valid request, set btrfs-allocation-{,meta}data-bg_reclaim_threshold",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "btrfs",
							MountFlags: []string{
								"btrfs-allocation-data-bg_reclaim_threshold=90",
								"btrfs-allocation-metadata-bg_reclaim_threshold=91",
							},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			deviceSize:           1,
			blockExtSize:         1,
			readonlyBit:          "0",
			btrfsReclaimData:     "90",
			btrfsReclaimMetadata: "91",
			expCommandList: []fakeCmd{
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "fsck",
					args:   "-a /dev/disk/fake-path",
					stdout: "",
				},
				{
					cmd:    "blockdev",
					args:   "--getro /dev/disk/fake-path",
					stdout: "%v",
				},
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "blockdev",
					args:   "--getsize64 /dev/disk/fake-path",
					stdout: "%d",
				},
				{
					cmd:    "btrfs",
					args:   "inspect-internal dump-super -f /dev/disk/fake-path",
					stdout: "sectorsize %d\ntotal_bytes %d",
				},
				{
					cmd:    "blkid",
					args:   fmt.Sprintf("--match-tag UUID --output value %v", stagingPath),
					stdout: btrfsUUID + "\n",
				},
			},
		},
		{
			name: "Valid request, update readahead",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{"read_ahead_kb=4096"},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			deviceSize:         5,
			blockExtSize:       1,
			readonlyBit:        "0",
			expResize:          true,
			expReadAheadUpdate: true,
			readAheadSectors:   "8192",
			sectorSizeInBytes:  512,
			expCommandList: []fakeCmd{
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "fsck",
					args:   "-a /dev/disk/fake-path",
					stdout: "",
				},
				{
					cmd:    "blockdev",
					args:   "--getro /dev/disk/fake-path",
					stdout: "%v",
				},
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "resize2fs",
					args:   "/dev/disk/fake-path",
					stdout: "",
				},
				{
					cmd:    "blockdev",
					args:   "--getss /dev/disk/fake-path",
					stdout: "%d",
				},
				{
					cmd:  "blockdev",
					args: "--setra %v /dev/disk/fake-path",
				},
			},
		},
		{
			name: "Valid request, update readahead (different sectorsize)",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{"read_ahead_kb=4096"},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			deviceSize:         5,
			blockExtSize:       1,
			readonlyBit:        "0",
			expResize:          true,
			expReadAheadUpdate: true,
			readAheadSectors:   "4194304",
			sectorSizeInBytes:  1,
			expCommandList: []fakeCmd{
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "fsck",
					args:   "-a /dev/disk/fake-path",
					stdout: "",
				},
				{
					cmd:    "blockdev",
					args:   "--getro /dev/disk/fake-path",
					stdout: "%v",
				},
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "blkid",
					args:   "-p -s TYPE -s PTTYPE -o export /dev/disk/fake-path",
					stdout: "DEVNAME=/dev/sdb\nTYPE=%v",
				},
				{
					cmd:    "resize2fs",
					args:   "/dev/disk/fake-path",
					stdout: "",
				},
				{
					cmd:    "blockdev",
					args:   "--getss /dev/disk/fake-path",
					stdout: "%d",
				},
				{
					cmd:  "blockdev",
					args: "--setra %v /dev/disk/fake-path",
				},
			},
		},
		{
			name: "Invalid request (Bad Access Mode)",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability:  createVolumeCapability(csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER),
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No VolumeId)",
			req: &csi.NodeStageVolumeRequest{
				StagingTargetPath: stagingPath,
				VolumeCapability:  stdVolCap,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No StagingTargetPath)",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:         volumeID,
				VolumeCapability: stdVolCap,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (Nil VolumeCapability)",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability:  nil,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No Mount in capability)",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability:  cap,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (Invalid read_ahead_kb)",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{"read_ahead_kb=not_a_number"},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (negative read_ahead_kb)",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{"read_ahead_kb=-4096"},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			expErrCode: codes.InvalidArgument,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resizeCalled := false
			readAheadUpdateCalled := false
			blkidCalled := false
			fsType := tc.req.GetVolumeCapability().GetMount().GetFsType()
			if fsType == "" {
				fsType = "ext4"
			}
			actionList := []testingexec.FakeCommandAction{}
			for _, cmd := range tc.expCommandList {
				t.Logf("cmd: %+v", cmd)
				switch cmd.cmd {
				case "resize2fs":
					resizeCalled = true
				case "blockdev":
					if strings.Contains(cmd.args, "--getro") {
						cmd.stdout = fmt.Sprintf(cmd.stdout, tc.readonlyBit)
					} else if strings.Contains(cmd.args, "--getsize64") {
						cmd.stdout = fmt.Sprintf(cmd.stdout, tc.deviceSize)
					} else if strings.Contains(cmd.args, "--getss") {
						cmd.stdout = fmt.Sprintf(cmd.stdout, tc.sectorSizeInBytes)
					} else if strings.Contains(cmd.args, "--setra") {
						readAheadUpdateCalled = true
						cmd.args = fmt.Sprintf(cmd.args, tc.readAheadSectors)
					}
				case "blkid":
					if strings.Contains(cmd.args, "TYPE") {
						cmd.stdout = fmt.Sprintf(cmd.stdout, fsType)
					}
				case "btrfs":
					cmd.stdout = fmt.Sprintf(cmd.stdout, tc.blockExtSize, tc.deviceSize)
				}
				action := []testingexec.FakeAction{
					func() ([]byte, []byte, error) {
						return []byte(cmd.stdout), nil, cmd.err
					},
				}
				actionList = append(actionList, makeFakeCmd(
					&testingexec.FakeCmd{
						CombinedOutputScript: action,
						OutputScript:         action,
					},
					cmd.cmd,
					strings.Split(cmd.args, " ")...,
				))
			}
			mounter := mountmanager.NewFakeSafeMounterWithCustomExec(&testingexec.FakeExec{CommandScript: actionList, ExactOrder: true})
			gceDriver := getTestGCEDriverWithCustomMounter(t, mounter, &NodeServerArgs{
				DeviceCache: linkcache.NewTestDeviceCache(1*time.Minute, linkcache.NewTestNodeWithVolumes([]string{volumeID})),
			})
			ns := gceDriver.ns
			ns.SysfsPath = tempDir + "/sys"
			_, err := ns.NodeStageVolume(context.Background(), tc.req)
			if err != nil {
				serverError, ok := status.FromError(err)
				if !ok {
					t.Fatalf("Could not get error status code from err: %v", err)
				}
				if serverError.Code() != tc.expErrCode {
					t.Fatalf("Expected error code: %v, got: %v. err : %v", tc.expErrCode, serverError.Code(), err)
				}
				return
			}
			if tc.expErrCode != codes.OK {
				t.Fatalf("Expected error: %v, got no error", tc.expErrCode)
			}
			if tc.expResize == true && resizeCalled == false {
				t.Fatalf("Test did not call resize, but it was expected.")
			}
			if tc.expResize == false && resizeCalled == true {
				t.Fatalf("Test called resize, but it was not expected.")
			}
			if tc.expReadAheadUpdate == true && readAheadUpdateCalled == false {
				t.Fatalf("Test did not update read ahead, but it was expected.")
			}
			if tc.expReadAheadUpdate == false && readAheadUpdateCalled == true {
				t.Fatalf("Test updated read ahead, but it was not expected.")
			}
			if tc.btrfsReclaimData == "" && tc.btrfsReclaimMetadata == "" && blkidCalled {
				t.Fatalf("blkid was called, but was not expected.")
			}

			if tc.btrfsReclaimData != "" {
				fname := btrfsPrefix + "/data/bg_reclaim_threshold"
				got, err := os.ReadFile(fname)
				if err != nil {
					t.Fatalf("read %q: %v", fname, err)
				}
				if s := strings.TrimSpace(string(got)); s != tc.btrfsReclaimData {
					t.Fatalf("%q: expected %q, got %q", fname, tc.btrfsReclaimData, s)
				}
			}
			if tc.btrfsReclaimMetadata != "" {
				fname := btrfsPrefix + "/metadata/bg_reclaim_threshold"
				got, err := os.ReadFile(fname)
				if err != nil {
					t.Fatalf("read %q: %v", fname, err)
				}
				if s := strings.TrimSpace(string(got)); s != tc.btrfsReclaimMetadata {
					t.Fatalf("%q: expected %q, got %q", fname, tc.btrfsReclaimMetadata, s)
				}
			}
		})
	}
}

// TODO: This test is too brittle due to the fakeexec package not being
// expressive enough for our purposes. The main issue being that the actions
// executed by fakeexec are executed in order of definition instead of by
// "command name" or some other way. This forces the test to "code to the
// implementation" in that we have to take each test case and order the CMD
// actions in the exact order that we expect to see them appear and hardcode the
// expected results. This is an exercise in re-implementing the current state of
// the implementation of the function under test but with hardcoded return
// values and brings no real value besides incurring a brittle test. This
// functionality is covered by e2e tests instead. Beware those who would attempt
// to un-comment
/*
func TestNodeExpandVolume(t *testing.T) {
	// TODO: Add tests/functionality for non-existant volume
	var resizedBytes int64 = 2000000000
	volumeID := "project/test001/zones/c1/disks/testDisk"
	testCases := []struct {
		name         string
		req          *csi.NodeExpandVolumeRequest
		fsOrBlock    string
		expRespBytes int64
		expErrCode   codes.Code
	}{
		{
			name: "ext4 fs expand",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   volumeID,
				VolumePath: "some-path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: resizedBytes,
				},
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
				},
			},
			fsOrBlock:    "ext4",
			expRespBytes: resizedBytes,
		},
		{
			name: "block device expand",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   volumeID,
				VolumePath: "some-path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: resizedBytes,
				},
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
				},
			},
			fsOrBlock:    "block",
			expRespBytes: resizedBytes,
		},
		{
			name: "xfs fs expand",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   volumeID,
				VolumePath: "some-path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: resizedBytes,
				},
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
				},
			},
			fsOrBlock:    "xfs",
			expRespBytes: resizedBytes,
		},
	}
	for _, tc := range testCases {
		t.Logf("Test case: %s", tc.name)
		actionList := []testingexec.FakeCommandAction{
			makeFakeCmd(
				&testingexec.FakeCmd{
					CombinedOutputScript: []testingexec.FakeAction{
						func() ([]byte, []byte, error) {
							if tc.fsOrBlock == "block" {
								// blkid returns exit code 2 when run on unformatted device
								return nil, nil, exec.CodeExitError{
									Err:  errors.New("this is an exit error"),
									Code: 2,
								}
							}
							return []byte(fmt.Sprintf("DEVNAME=/dev/sdb\nTYPE=%s", tc.fsOrBlock)), nil, nil
						},
					},
				},
				"blkid",
			),
			makeFakeCmd(
				&testingexec.FakeCmd{
					CombinedOutputScript: []testingexec.FakeAction{
						func() ([]byte, []byte, error) {
							return []byte(strconv.Itoa(int(resizedBytes))), nil, nil
						},
					},
				},
				"blockdev",
			),
			makeFakeCmd(
				&testingexec.FakeCmd{
					CombinedOutputScript: []testingexec.FakeAction{
						func() ([]byte, []byte, error) {
							if tc.fsOrBlock == "ext4" {
								return nil, nil, nil
							}
							return nil, nil, fmt.Errorf("resize fs called on device with %s", tc.fsOrBlock)
						},
					},
				},
				"resize2fs",
			),

			makeFakeCmd(
				&testingexec.FakeCmd{
					CombinedOutputScript: []testingexec.FakeAction{
						func() ([]byte, []byte, error) {
							if tc.fsOrBlock != "xfs" {
								t.Fatalf("xfs_growfs called on device with %s", tc.fsOrBlock)
							}
							for _, arg := range args {
								if arg == tc.req.VolumePath {
									return nil, nil, nil
								}
							}
							return nil, nil, fmt.Errorf("xfs_growfs args did not contain volume path %s", tc.req.VolumePath)

							return nil,nil,nil
						},
					},
				},
				"xfs_growfs",
			),

		}
		mounter := mountmanager.NewFakeSafeMounterWithCustomExec(&testingexec.FakeExec{CommandScript: actionList}) // TODO(dyzz) add the command list to here.
		gceDriver := getTestGCEDriverWithCustomMounter(t, mounter)

		resp, err := gceDriver.ns.NodeExpandVolume(context.Background(), tc.req)
		if err != nil {
			serverError, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Could not get error status code from err: %v", err)
			}
			if serverError.Code() != tc.expErrCode {
				t.Fatalf("Expected error code: %v, got: %v. err : %v", tc.expErrCode, serverError.Code(), err)
			}
			continue
		}
		if tc.expErrCode != codes.OK {
			t.Fatalf("Expected error: %v, got no error", tc.expErrCode)
		}

		if resp.CapacityBytes != tc.expRespBytes {
			t.Fatalf("Expected bytes: %v, got: %v", tc.expRespBytes, resp.CapacityBytes)
		}
	}
}

func makeFakeCmd(fakeCmd *testingexec.FakeCmd, cmd string, args ...string) testingexec.FakeCommandAction {
	c := cmd
	a := args
	return func(cmd string, args ...string) exec.Cmd {
		command := testingexec.InitFakeCmd(fakeCmd, c, a...)
		return command
	}
}
*/

func TestNodeUnstageVolume(t *testing.T) {
	gceDriver := getTestGCEDriver(t)
	ns := gceDriver.ns
	tempDir, err := os.MkdirTemp("", "nusv")
	if err != nil {
		t.Fatalf("Failed to set up temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	stagingPath := filepath.Join(tempDir, defaultStagingPath)

	testCases := []struct {
		name       string
		req        *csi.NodeUnstageVolumeRequest
		expErrCode codes.Code
	}{
		{
			name: "Valid request",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          defaultVolumeID,
				StagingTargetPath: stagingPath,
			},
		},
		{
			name: "Invalid request (No VolumeId)",
			req: &csi.NodeUnstageVolumeRequest{
				StagingTargetPath: stagingPath,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No StagingTargetPath)",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId: defaultVolumeID,
			},
			expErrCode: codes.InvalidArgument,
		},
	}
	for _, tc := range testCases {
		t.Logf("Test case: %s", tc.name)
		_, err := ns.NodeUnstageVolume(context.Background(), tc.req)
		if err != nil {
			serverError, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Could not get error status code from err: %v", err)
			}
			if serverError.Code() != tc.expErrCode {
				t.Fatalf("Expected error code: %v, got: %v. err : %v", tc.expErrCode, serverError.Code(), err)
			}
			continue
		}
		if tc.expErrCode != codes.OK {
			t.Fatalf("Expected error: %v, got no error", tc.expErrCode)
		}
	}
}

func TestNodeGetCapabilities(t *testing.T) {
	gceDriver := getTestGCEDriver(t)
	ns := gceDriver.ns
	req := &csi.NodeGetCapabilitiesRequest{}

	_, err := ns.NodeGetCapabilities(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpedted error: %v", err)
	}
}

func runBlockingFormatAndMount(t *testing.T, gceDriver *GCEDriver, readyToExecute chan chan struct{}) {
	ns := gceDriver.ns
	tempDir, err := os.MkdirTemp("", "cno")
	if err != nil {
		t.Fatalf("Failed to set up temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	targetPath := filepath.Join(tempDir, defaultTargetPath)
	stagingPath := filepath.Join(tempDir, defaultStagingPath)

	vol1PublishTargetAReq := &csi.NodePublishVolumeRequest{
		VolumeId:          defaultVolumeID + "1",
		TargetPath:        targetPath + "a",
		StagingTargetPath: stagingPath + "1",
		Readonly:          false,
		VolumeCapability:  stdVolCap,
	}
	vol1PublishTargetBReq := &csi.NodePublishVolumeRequest{
		VolumeId:          defaultVolumeID + "1",
		TargetPath:        targetPath + "b",
		StagingTargetPath: stagingPath + "1",
		Readonly:          false,
		VolumeCapability:  stdVolCap,
	}
	vol2PublishTargetCReq := &csi.NodePublishVolumeRequest{
		VolumeId:          defaultVolumeID + "2",
		TargetPath:        targetPath + "c",
		StagingTargetPath: stagingPath + "2",
		Readonly:          false,
		VolumeCapability:  stdVolCap,
	}

	runRequest := func(req *csi.NodePublishVolumeRequest) chan error {
		response := make(chan error)
		go func() {
			_, err := ns.NodePublishVolume(context.Background(), req)
			response <- err
		}()
		return response
	}

	// Start first valid request vol1PublishTargetA and block until it reaches the Mount
	vol1PublishTargetAResp := runRequest(vol1PublishTargetAReq)
	execVol1PublishTargetA := <-readyToExecute

	// Start vol1PublishTargetB and allow it to execute to completion. Then check for Aborted error.
	// If a non Abort error is received or if the operation was started, then there is a problem
	// with volume locking.
	vol1PublishTargetBResp := runRequest(vol1PublishTargetBReq)
	select {
	case err := <-vol1PublishTargetBResp:
		if err != nil {
			serverError, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Could not get error status code from err: %v", err)
			}
			if serverError.Code() != codes.Aborted {
				t.Errorf("Expected error code: %v, got: %v. err : %v", codes.Aborted, serverError.Code(), err)
			}
		} else {
			t.Errorf("Expected error: %v, got no error", codes.Aborted)
		}
	case <-readyToExecute:
		t.Errorf("The operation for vol1PublishTargetB should have been aborted, but was started")
	}

	// Start vol2PublishTargetC and allow it to execute to completion. Then check for success.
	vol2PublishTargetCResp := runRequest(vol2PublishTargetCReq)
	execVol2PublishTargetC := <-readyToExecute
	execVol2PublishTargetC <- struct{}{}
	if err := <-vol2PublishTargetCResp; err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// To clean up, allow the vol1PublishTargetA to complete
	execVol1PublishTargetA <- struct{}{}
	if err := <-vol1PublishTargetAResp; err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestBlockingMount(t *testing.T) {
	readyToExecute := make(chan chan struct{}, 1)
	gceDriver := getTestBlockingMountGCEDriver(t, readyToExecute)
	runBlockingFormatAndMount(t, gceDriver, readyToExecute)
}

func TestBlockingFormatAndMount(t *testing.T) {
	readyToExecute := make(chan chan struct{}, 1)
	gceDriver := getTestBlockingFormatAndMountGCEDriver(t, readyToExecute)
	runBlockingFormatAndMount(t, gceDriver, readyToExecute)
}
