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
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metadataservice "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/metadata"
	mountmanager "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"
)

const defaultVolumeID = "project/test001/zones/c1/disks/testDisk"
const defaultTargetPath = "/mnt/test"
const defaultStagingPath = "/staging"

func getTestGCEDriver(t *testing.T) *GCEDriver {
	gceDriver := GetGCEDriver()
	err := gceDriver.SetupGCEDriver(nil, mountmanager.NewFakeSafeMounter(), mountmanager.NewFakeDeviceUtils(), metadataservice.NewFakeService(), driver, "test-vendor")
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
			expVolumeLimit: volumeLimit128,
		},
		{
			name:           "Predifined micro machine",
			machineType:    "f1-micro",
			expVolumeLimit: volumeLimit16,
		},
		{
			name:           "Predifined small machine",
			machineType:    "g1-small",
			expVolumeLimit: volumeLimit16,
		},
		{
			name:           "Custom machine with 1GiB Mem",
			machineType:    "custom-1-1024",
			expVolumeLimit: volumeLimit128,
		},
		{
			name:           "Custom machine with 4GiB Mem",
			machineType:    "custom-2-4096",
			expVolumeLimit: volumeLimit128,
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
				VolumeCapability:  &csi.VolumeCapability{},
			},
		},
		{
			name: "Invalid request (No VolumeId)",
			req: &csi.NodePublishVolumeRequest{
				TargetPath:        defaultTargetPath,
				StagingTargetPath: defaultStagingPath,
				Readonly:          false,
				VolumeCapability:  &csi.VolumeCapability{},
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No TargetPath)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          defaultVolumeID,
				StagingTargetPath: defaultStagingPath,
				Readonly:          false,
				VolumeCapability:  &csi.VolumeCapability{},
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No StagingTargetPath)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         defaultVolumeID,
				TargetPath:       defaultTargetPath,
				Readonly:         false,
				VolumeCapability: &csi.VolumeCapability{},
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
				VolumeCapability:  &csi.VolumeCapability{},
			},
		},
		{
			name: "Invalid request (No VolumeId)",
			req: &csi.NodeStageVolumeRequest{
				StagingTargetPath: defaultStagingPath,
				VolumeCapability:  &csi.VolumeCapability{},
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Invalid request (No StagingTargetPath)",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:         volumeID,
				VolumeCapability: &csi.VolumeCapability{},
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
			expErrCode: codes.Unimplemented,
		},
		// Capability Mount. codes.Unimplemented
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
