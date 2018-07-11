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

	"golang.org/x/net/context"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider"
	utils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/utils"
)

const (
	project      = "test-project"
	zone         = "test-zone"
	node         = "test-node"
	driver       = "test-driver"
	testVolumeId = zone + "/" + "test-vol"
)

func TestCreateVolumeArguments(t *testing.T) {
	// Define "normal" parameters
	stdVolCap := []*csi.VolumeCapability{
		{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}
	stdCapRange := &csi.CapacityRange{
		RequiredBytes: utils.GbToBytes(20),
	}
	stdParams := map[string]string{
		"zone": zone,
		"type": "test-type",
	}

	// Define test cases
	testCases := []struct {
		name       string
		req        *csi.CreateVolumeRequest
		expVol     *csi.Volume
		expErrCode codes.Code
	}{
		{
			name: "success normal",
			req: &csi.CreateVolumeRequest{
				Name:               "test-vol",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCap,
				Parameters:         stdParams,
			},
			expVol: &csi.Volume{
				CapacityBytes: utils.GbToBytes(20),
				Id:            testVolumeId,
				Attributes:    nil,
			},
		},
		{
			name: "fail no name",
			req: &csi.CreateVolumeRequest{
				Name:               "",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCap,
				Parameters:         stdParams,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "success no capacity range",
			req: &csi.CreateVolumeRequest{
				Name:               "test-vol",
				VolumeCapabilities: stdVolCap,
				Parameters:         stdParams,
			},
			expVol: &csi.Volume{
				CapacityBytes: MinimumVolumeSizeInBytes,
				Id:            testVolumeId,
				Attributes:    nil,
			},
		},
		{
			name: "fail no capabilities",
			req: &csi.CreateVolumeRequest{
				Name:          "test-vol",
				CapacityRange: stdCapRange,
				Parameters:    stdParams,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "success no params",
			req: &csi.CreateVolumeRequest{
				Name:               "test-vol",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCap,
			},
			expVol: &csi.Volume{
				CapacityBytes: utils.GbToBytes(20),
				Id:            testVolumeId,
				Attributes:    nil,
			},
		},
		{
			name: "success with random secrets",
			req: &csi.CreateVolumeRequest{
				Name:                    "test-vol",
				CapacityRange:           stdCapRange,
				VolumeCapabilities:      stdVolCap,
				Parameters:              stdParams,
				ControllerCreateSecrets: map[string]string{"key1": "this is a random", "crypto": "secret"},
			},
			expVol: &csi.Volume{
				CapacityBytes: utils.GbToBytes(20),
				Id:            testVolumeId,
				Attributes:    nil,
			},
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		gceDriver := GetGCEDriver()
		fakeCloudProvider, err := gce.FakeCreateCloudProvider(project, zone)
		if err != nil {
			t.Fatalf("Failed to create fake cloud provider: %v", err)
		}
		err = gceDriver.SetupGCEDriver(fakeCloudProvider, nil, driver, node, "vendor-version")
		if err != nil {
			t.Fatalf("Failed to setup GCE Driver: %v", err)
		}

		// Start Test
		resp, err := gceDriver.cs.CreateVolume(context.TODO(), tc.req)
		//check response
		if err != nil {
			serverError, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Could not get error status code from err: %v", serverError)
			}
			if serverError.Code() != tc.expErrCode {
				t.Fatalf("Expected error code: %v, got: %v", tc.expErrCode, serverError.Code())
			}
			continue
		}
		if tc.expErrCode != codes.OK {
			t.Fatalf("Expected error: %v, got no error", tc.expErrCode)
		}

		// Make sure responses match
		vol := resp.GetVolume()
		if vol == nil {
			// If one is nil but not both
			t.Fatalf("Expected volume %v, got nil volume", tc.expVol)
		}

		if vol.GetCapacityBytes() != tc.expVol.GetCapacityBytes() {
			t.Fatalf("Expected volume capacity bytes: %v, got: %v", vol.GetCapacityBytes(), tc.expVol.GetCapacityBytes())
		}

		if vol.GetId() != tc.expVol.GetId() {
			t.Fatalf("Expected volume id: %v, got: %v", vol.GetId(), tc.expVol.GetId())
		}

		for akey, aval := range tc.expVol.GetAttributes() {
			if gotVal, ok := vol.GetAttributes()[akey]; !ok || gotVal != aval {
				t.Fatalf("Expected volume attribute for key %v: %v, got: %v", akey, aval, gotVal)
			}
		}
		if tc.expVol.GetAttributes() == nil && vol.GetAttributes() != nil {
			t.Fatalf("Expected volume attributes to be nil, got: %#v", vol.GetAttributes())
		}

	}
}

func TestDeleteVolume(t *testing.T) {
	testCases := []struct {
		name   string
		req    *csi.DeleteVolumeRequest
		expErr bool
	}{
		{
			name: "valid",
			req: &csi.DeleteVolumeRequest{
				VolumeId: testVolumeId,
			},
		},
		{
			name: "invalid id",
			req: &csi.DeleteVolumeRequest{
				VolumeId: testVolumeId + "/foo",
			},
			expErr: false,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		gceDriver := initGCEDriver(t)

		_, err := gceDriver.cs.DeleteVolume(context.TODO(), tc.req)
		if err == nil && tc.expErr {
			t.Fatalf("Expected error but got none")
		}
		if err != nil && !tc.expErr {
			t.Fatalf("Did not expect error but got: %v", err)
		}

		if err != nil {
			continue
		}

	}
}

func TestGetRequestCapacity(t *testing.T) {
	testCases := []struct {
		name     string
		capRange *csi.CapacityRange
		expCap   int64
		expErr   bool
	}{
		{
			name:     "nil cap range",
			capRange: nil,
			expCap:   MinimumVolumeSizeInBytes,
		},
		{
			name: "success: required below min",
			capRange: &csi.CapacityRange{
				RequiredBytes: MinimumVolumeSizeInBytes - 1,
			},
			expCap: MinimumVolumeSizeInBytes,
		},
		{
			name: "success: required equals min",
			capRange: &csi.CapacityRange{
				RequiredBytes: MinimumVolumeSizeInBytes,
			},
			expCap: MinimumVolumeSizeInBytes,
		},
		{
			name: "success: required above min",
			capRange: &csi.CapacityRange{
				RequiredBytes: MinimumVolumeSizeInBytes + 1,
			},
			expCap: MinimumVolumeSizeInBytes + 1,
		},
		{
			name: "fail: limit below min",
			capRange: &csi.CapacityRange{
				LimitBytes: MinimumVolumeSizeInBytes - 1,
			},
			expErr: true,
		},
		{
			name: "success: limit equal min",
			capRange: &csi.CapacityRange{
				LimitBytes: MinimumVolumeSizeInBytes,
			},
			expCap: MinimumVolumeSizeInBytes,
		},
		{
			name: "success: limit above min",
			capRange: &csi.CapacityRange{
				LimitBytes: MinimumVolumeSizeInBytes + 1,
			},
			expCap: MinimumVolumeSizeInBytes,
		},
		{
			name: "success: fully specified both above min",
			capRange: &csi.CapacityRange{
				RequiredBytes: utils.GbToBytes(20),
				LimitBytes:    utils.GbToBytes(50),
			},
			expCap: utils.GbToBytes(20),
		},
		{
			name: "success: fully specified required below min",
			capRange: &csi.CapacityRange{
				RequiredBytes: MinimumVolumeSizeInBytes - 1,
				LimitBytes:    utils.GbToBytes(50),
			},
			expCap: MinimumVolumeSizeInBytes,
		},
		{
			name: "success: fully specified both below min",
			capRange: &csi.CapacityRange{
				RequiredBytes: MinimumVolumeSizeInBytes - 2,
				LimitBytes:    MinimumVolumeSizeInBytes - 1,
			},
			expErr: true,
		},
		{
			name: "fail: limit less than required",
			capRange: &csi.CapacityRange{
				RequiredBytes: utils.GbToBytes(50),
				LimitBytes:    utils.GbToBytes(20),
			},
			expErr: true,
		},
		{
			name: "fail: limit less than required below min",
			capRange: &csi.CapacityRange{
				RequiredBytes: MinimumVolumeSizeInBytes - 2,
				LimitBytes:    MinimumVolumeSizeInBytes - 3,
			},
			expErr: true,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)

		gotCap, err := getRequestCapacity(tc.capRange)
		if err == nil && tc.expErr {
			t.Fatalf("Expected error but got none")
		}
		if err != nil && !tc.expErr {
			t.Fatalf("Did not expect error but got: %v", err)
		}

		if err != nil {
			continue
		}

		if gotCap != tc.expCap {
			t.Fatalf("Got capacity: %v, expected: %v", gotCap, tc.expCap)
		}
	}
}

func TestDiskIsAttached(t *testing.T) {
	testCases := []struct {
		name        string
		disk        *compute.Disk
		instance    *compute.Instance
		expAttached bool
	}{
		{
			name: "normal-attached",
			disk: &compute.Disk{
				Name: "test-disk",
			},
			instance: &compute.Instance{
				Disks: []*compute.AttachedDisk{
					{
						DeviceName: "test-disk",
					},
				},
			},
			expAttached: true,
		},
		{
			name: "normal-not-attached",
			disk: &compute.Disk{
				Name: "test-disk",
			},
			instance: &compute.Instance{
				Disks: []*compute.AttachedDisk{
					{
						DeviceName: "not-the-test-disk",
					},
				},
			},
			expAttached: false,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		if attached := diskIsAttached(tc.disk, tc.instance); attached != tc.expAttached {
			t.Errorf("Expected disk attached to be %v, but got %v", tc.expAttached, attached)
		}
	}
}

func TestDiskIsAttachedAndCompatible(t *testing.T) {
	testCases := []struct {
		name        string
		disk        *compute.Disk
		instance    *compute.Instance
		mode        string
		expAttached bool
		expErr      bool
	}{
		{
			name: "normal-attached",
			disk: &compute.Disk{
				Name: "test-disk",
			},
			instance: &compute.Instance{
				Disks: []*compute.AttachedDisk{
					{
						DeviceName: "test-disk",
						Mode:       "test-mode",
					},
				},
			},
			mode:        "test-mode",
			expAttached: true,
		},
		{
			name: "normal-not-attached",
			disk: &compute.Disk{
				Name: "test-disk",
			},
			instance: &compute.Instance{
				Disks: []*compute.AttachedDisk{
					{
						DeviceName: "not-the-test-disk",
						Mode:       "test-mode",
					},
				},
			},
			mode:        "test-mode",
			expAttached: false,
		},
		{
			name: "incompatible mode",
			disk: &compute.Disk{
				Name: "test-disk",
			},
			instance: &compute.Instance{
				Disks: []*compute.AttachedDisk{
					{
						DeviceName: "test-disk",
						Mode:       "test-mode",
					},
				},
			},
			mode:        "random-mode",
			expAttached: true,
			expErr:      true,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		attached, err := diskIsAttachedAndCompatible(tc.disk, tc.instance, nil, tc.mode)
		if err != nil && !tc.expErr {
			t.Errorf("Did not expect error but got: %v", err)
		}
		if err == nil && tc.expErr {
			t.Errorf("Expected error but got none")
		}
		if attached != tc.expAttached {
			t.Errorf("Expected disk attached to be %v, but got %v", tc.expAttached, attached)
		}
	}
}
