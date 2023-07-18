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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/utils/exec"
	testingexec "k8s.io/utils/exec/testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/deviceutils"
	metadataservice "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/metadata"
	mountmanager "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"
)

const defaultVolumeID = "project/test001/zones/c1/disks/testDisk"
const defaultTargetPath = "/mnt/test"
const defaultStagingPath = "/staging"

func getTestGCEDriver(t *testing.T) *GCEDriver {
	return getCustomTestGCEDriver(t, mountmanager.NewFakeSafeMounter(), deviceutils.NewFakeDeviceUtils(), metadataservice.NewFakeService())
}

func getTestGCEDriverWithCustomMounter(t *testing.T, mounter *mount.SafeFormatAndMount) *GCEDriver {
	return getCustomTestGCEDriver(t, mounter, deviceutils.NewFakeDeviceUtils(), metadataservice.NewFakeService())
}

func getCustomTestGCEDriver(t *testing.T, mounter *mount.SafeFormatAndMount, deviceUtils deviceutils.DeviceUtils, metaService metadataservice.MetadataService) *GCEDriver {
	gceDriver := GetGCEDriver()
	nodeServer := NewNodeServer(gceDriver, mounter, deviceUtils, metaService, mountmanager.NewFakeStatter(mounter))
	err := gceDriver.SetupGCEDriver(driver, "test-vendor", nil, nil, nil, nodeServer)
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}
	return gceDriver
}

func getTestBlockingMountGCEDriver(t *testing.T, readyToExecute chan chan struct{}) *GCEDriver {
	gceDriver := GetGCEDriver()
	mounter := mountmanager.NewFakeSafeBlockingMounter(readyToExecute)
	nodeServer := NewNodeServer(gceDriver, mounter, deviceutils.NewFakeDeviceUtils(), metadataservice.NewFakeService(), mountmanager.NewFakeStatter(mounter))
	err := gceDriver.SetupGCEDriver(driver, "test-vendor", nil, nil, nil, nodeServer)
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}
	return gceDriver
}

func getTestBlockingFormatAndMountGCEDriver(t *testing.T, readyToExecute chan chan struct{}) *GCEDriver {
	gceDriver := GetGCEDriver()
	mounter := mountmanager.NewFakeSafeBlockingMounter(readyToExecute)
	nodeServer := NewNodeServer(gceDriver, mounter, deviceutils.NewFakeDeviceUtils(), metadataservice.NewFakeService(), mountmanager.NewFakeStatter(mounter)).WithSerializedFormatAndMount(5*time.Second, 1)

	err := gceDriver.SetupGCEDriver(driver, "test-vendor", nil, nil, nil, nodeServer)
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

	tempDir, err := ioutil.TempDir("", "ngvs")
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
		name       string
		volumeID   string
		volumePath string
		expectErr  bool
	}{
		{
			name:       "normal",
			volumeID:   defaultVolumeID,
			volumePath: targetPath,
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

			req := &csi.NodeGetVolumeStatsRequest{
				VolumeId:   tc.volumeID,
				VolumePath: tc.volumePath,
			}
			_, err := ns.NodeGetVolumeStats(context.Background(), req)
			if err != nil && !tc.expectErr {
				t.Fatalf("Got unexpected err: %v", err)
			}
			if err == nil && tc.expectErr {
				t.Fatal("Did not get error but expected one")
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
	}

	for _, tc := range testCases {
		t.Logf("Test case: %s", tc.name)
		metadataservice.SetMachineType(tc.machineType)
		res, err := ns.NodeGetInfo(context.Background(), req)
		if err != nil {
			t.Fatalf("Failed to get node info: %v", err)
		} else {
			volumeLimit := res.GetMaxVolumesPerNode()
			if volumeLimit != tc.expVolumeLimit {
				t.Fatalf("Expected volume limit: %v, got %v, for machine-type: %v",
					tc.expVolumeLimit, volumeLimit, tc.machineType)
			}
			t.Logf("Get node info: %v", res)
		}
	}
}

func TestNodePublishVolume(t *testing.T) {
	gceDriver := getTestGCEDriver(t)
	ns := gceDriver.ns

	tempDir, err := ioutil.TempDir("", "npv")
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

	tempDir, err := ioutil.TempDir("", "nupv")
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

func TestNodeStageVolume(t *testing.T) {
	volumeID := "project/test001/zones/c1/disks/testDisk"
	blockCap := &csi.VolumeCapability_Block{
		Block: &csi.VolumeCapability_BlockVolume{},
	}
	cap := &csi.VolumeCapability{
		AccessType: blockCap,
	}

	tempDir, err := ioutil.TempDir("", "nsv")
	if err != nil {
		t.Fatalf("Failed to set up temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	stagingPath := filepath.Join(tempDir, defaultStagingPath)

	testCases := []struct {
		name       string
		req        *csi.NodeStageVolumeRequest
		expErrCode codes.Code
	}{
		{
			name: "Valid request",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability:  stdVolCap,
			},
		},
		{
			name: "Invalid request (Bad Access Mode)",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: stagingPath,
				VolumeCapability:  createVolumeCapability(csi.VolumeCapability_AccessMode_UNKNOWN),
			},
			expErrCode: codes.InvalidArgument,
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
	}
	for _, tc := range testCases {
		t.Logf("Test case: %s", tc.name)
		actionList := []testingexec.FakeCommandAction{
			makeFakeCmd(
				&testingexec.FakeCmd{
					CombinedOutputScript: []testingexec.FakeAction{
						func() ([]byte, []byte, error) {
							return []byte(fmt.Sprintf("DEVNAME=/dev/sdb\nTYPE=ext4")), nil, nil
						},
					},
				},
				"blkid",
			),
			makeFakeCmd(
				&testingexec.FakeCmd{
					CombinedOutputScript: []testingexec.FakeAction{
						func() ([]byte, []byte, error) {
							return []byte("1"), nil, nil
						},
					},
				},
				"blockdev",
			),
			makeFakeCmd(
				&testingexec.FakeCmd{
					CombinedOutputScript: []testingexec.FakeAction{
						func() ([]byte, []byte, error) {
							return []byte("1"), nil, nil
						},
					},
				},
				"blockdev",
			),
			makeFakeCmd(
				&testingexec.FakeCmd{
					CombinedOutputScript: []testingexec.FakeAction{
						func() ([]byte, []byte, error) {
							return []byte(fmt.Sprintf("DEVNAME=/dev/sdb\nTYPE=ext4")), nil, nil
						},
					},
				},
				"blkid",
			),
			makeFakeCmd(
				&testingexec.FakeCmd{
					CombinedOutputScript: []testingexec.FakeAction{
						func() ([]byte, []byte, error) {
							return []byte(fmt.Sprintf("block size: 1\nblock count: 1")), nil, nil
						},
					},
				},
				"dumpe2fs",
			),
		}
		mounter := mountmanager.NewFakeSafeMounterWithCustomExec(&testingexec.FakeExec{CommandScript: actionList})
		gceDriver := getTestGCEDriverWithCustomMounter(t, mounter)
		ns := gceDriver.ns
		_, err := ns.NodeStageVolume(context.Background(), tc.req)
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
	tempDir, err := ioutil.TempDir("", "nusv")
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
	tempDir, err := ioutil.TempDir("", "cno")
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
