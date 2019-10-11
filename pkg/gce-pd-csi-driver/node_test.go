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
	"errors"
	"fmt"
	"strconv"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
	utilexec "k8s.io/utils/exec"
	metadataservice "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/metadata"
	mountmanager "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"
)

const defaultVolumeID = "project/test001/zones/c1/disks/testDisk"
const defaultTargetPath = "/mnt/test"
const defaultStagingPath = "/staging"

func getTestGCEDriver(t *testing.T) *GCEDriver {
	return getCustomTestGCEDriver(t, mountmanager.NewFakeSafeMounter(), mountmanager.NewFakeDeviceUtils(), metadataservice.NewFakeService())
}

func getTestGCEDriverWithCustomMounter(t *testing.T, mounter *mount.SafeFormatAndMount) *GCEDriver {
	return getCustomTestGCEDriver(t, mounter, mountmanager.NewFakeDeviceUtils(), metadataservice.NewFakeService())
}

func getCustomTestGCEDriver(t *testing.T, mounter *mount.SafeFormatAndMount, deviceUtils mountmanager.DeviceUtils, metaService metadataservice.MetadataService) *GCEDriver {
	gceDriver := GetGCEDriver()
	err := gceDriver.SetupGCEDriver(nil, mounter, deviceUtils, metaService, nil, driver, "test-vendor")
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}
	return gceDriver
}

func getTestBlockingGCEDriver(t *testing.T, readyToExecute chan chan struct{}) *GCEDriver {
	gceDriver := GetGCEDriver()
	err := gceDriver.SetupGCEDriver(nil, mountmanager.NewFakeSafeBlockingMounter(readyToExecute), mountmanager.NewFakeDeviceUtils(), metadataservice.NewFakeService(), nil, driver, "test-vendor")
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}
	return gceDriver
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
	testCases := []struct {
		name       string
		req        *csi.NodePublishVolumeRequest
		expErrCode codes.Code
	}{
		{
			name: "Valid request",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          defaultVolumeID,
				TargetPath:        defaultTargetPath,
				StagingTargetPath: defaultStagingPath,
				Readonly:          false,
				VolumeCapability:  stdVolCap,
			},
		},
		{
			name: "Invalid request (invalid access mode)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          defaultVolumeID,
				TargetPath:        defaultTargetPath,
				StagingTargetPath: defaultStagingPath,
				Readonly:          false,
				VolumeCapability:  createVolumeCapability(csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER),
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No VolumeId)",
			req: &csi.NodePublishVolumeRequest{
				TargetPath:        defaultTargetPath,
				StagingTargetPath: defaultStagingPath,
				Readonly:          false,
				VolumeCapability:  stdVolCap,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No TargetPath)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          defaultVolumeID,
				StagingTargetPath: defaultStagingPath,
				Readonly:          false,
				VolumeCapability:  stdVolCap,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No StagingTargetPath)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         defaultVolumeID,
				TargetPath:       defaultTargetPath,
				Readonly:         false,
				VolumeCapability: stdVolCap,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (Nil VolumeCapability)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          defaultVolumeID,
				TargetPath:        defaultTargetPath,
				StagingTargetPath: defaultStagingPath,
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
	testCases := []struct {
		name       string
		req        *csi.NodeUnpublishVolumeRequest
		expErrCode codes.Code
	}{
		{
			name: "Valid request",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   defaultVolumeID,
				TargetPath: defaultTargetPath,
			},
		},
		{
			name: "Invalid request (No VolumeId)",
			req: &csi.NodeUnpublishVolumeRequest{
				TargetPath: defaultTargetPath,
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
	gceDriver := getTestGCEDriver(t)
	ns := gceDriver.ns
	volumeID := "project/test001/zones/c1/disks/testDisk"
	blockCap := &csi.VolumeCapability_Block{
		Block: &csi.VolumeCapability_BlockVolume{},
	}
	cap := &csi.VolumeCapability{
		AccessType: blockCap,
	}

	testCases := []struct {
		name       string
		req        *csi.NodeStageVolumeRequest
		expErrCode codes.Code
	}{
		{
			name: "Valid request",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: defaultStagingPath,
				VolumeCapability:  stdVolCap,
			},
		},
		{
			name: "Invalid request (Bad Access Mode)",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: defaultStagingPath,
				VolumeCapability:  createVolumeCapability(csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER),
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No VolumeId)",
			req: &csi.NodeStageVolumeRequest{
				StagingTargetPath: defaultStagingPath,
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
				StagingTargetPath: defaultStagingPath,
				VolumeCapability:  nil,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No Mount in capability)",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          volumeID,
				StagingTargetPath: defaultStagingPath,
				VolumeCapability:  cap,
			},
			expErrCode: codes.InvalidArgument,
		},
	}
	for _, tc := range testCases {
		t.Logf("Test case: %s", tc.name)
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
			},
			fsOrBlock:    "xfs",
			expRespBytes: resizedBytes,
		},
	}
	for _, tc := range testCases {
		t.Logf("Test case: %s", tc.name)

		execCallback := func(cmd string, args ...string) ([]byte, error) {
			switch cmd {
			case "blkid":
				if tc.fsOrBlock == "block" {
					// blkid returns exit code 2 when run on unformatted device
					return nil, utilexec.CodeExitError{
						Err:  errors.New("this is an exit error"),
						Code: 2,
					}
				}
				return []byte(fmt.Sprintf("DEVNAME=/dev/sdb\nTYPE=%s", tc.fsOrBlock)), nil
			case "resize2fs":
				if tc.fsOrBlock == "ext4" {
					return nil, nil
				}
				t.Fatalf("resize fs called on device with %s", tc.fsOrBlock)
			case "xfs_growfs":
				if tc.fsOrBlock != "xfs" {
					t.Fatalf("xfs_growfs called on device with %s", tc.fsOrBlock)
				}
				for _, arg := range args {
					if arg == tc.req.VolumePath {
						return nil, nil
					}
				}
				t.Errorf("xfs_growfs args did not contain volume path %s", tc.req.VolumePath)
			case "blockdev":
				return []byte(strconv.Itoa(int(resizedBytes))), nil
			}

			return nil, fmt.Errorf("fake exec got unknown call to %v %v", cmd, args)
		}
		mounter := mountmanager.NewFakeSafeMounterWithCustomExec(mount.NewFakeExec(execCallback))
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

func TestNodeUnstageVolume(t *testing.T) {
	gceDriver := getTestGCEDriver(t)
	ns := gceDriver.ns
	testCases := []struct {
		name       string
		req        *csi.NodeUnstageVolumeRequest
		expErrCode codes.Code
	}{
		{
			name: "Valid request",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          defaultVolumeID,
				StagingTargetPath: defaultStagingPath,
			},
		},
		{
			name: "Invalid request (No VolumeId)",
			req: &csi.NodeUnstageVolumeRequest{
				StagingTargetPath: defaultStagingPath,
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

func TestConcurrentNodeOperations(t *testing.T) {
	readyToExecute := make(chan chan struct{}, 1)
	gceDriver := getTestBlockingGCEDriver(t, readyToExecute)
	ns := gceDriver.ns

	vol1PublishTargetAReq := &csi.NodePublishVolumeRequest{
		VolumeId:          defaultVolumeID + "1",
		TargetPath:        defaultTargetPath + "a",
		StagingTargetPath: defaultStagingPath + "1",
		Readonly:          false,
		VolumeCapability:  stdVolCap,
	}
	vol1PublishTargetBReq := &csi.NodePublishVolumeRequest{
		VolumeId:          defaultVolumeID + "1",
		TargetPath:        defaultTargetPath + "b",
		StagingTargetPath: defaultStagingPath + "1",
		Readonly:          false,
		VolumeCapability:  stdVolCap,
	}
	vol2PublishTargetCReq := &csi.NodePublishVolumeRequest{
		VolumeId:          defaultVolumeID + "2",
		TargetPath:        defaultTargetPath + "c",
		StagingTargetPath: defaultStagingPath + "2",
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
