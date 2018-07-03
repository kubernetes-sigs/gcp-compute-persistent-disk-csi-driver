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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider"
	utils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/utils"
)

const (
	project = "test-project"
	zone    = "test-zone"
	node    = "test-node"
	driver  = "test-driver"
)

// Create Volume Tests
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
				Id:            zone + "/" + "test-vol",
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
				Id:            zone + "/" + "test-vol",
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
			// TODO(dyzz) maybe if no params, we should have defaults.
			name: "success no params",
			req: &csi.CreateVolumeRequest{
				Name:               "test-vol",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCap,
			},
			expVol: &csi.Volume{
				CapacityBytes: utils.GbToBytes(20),
				Id:            zone + "/" + "test-vol",
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
				Id:            zone + "/" + "test-vol",
				Attributes:    nil,
			},
		},
		{
			name: "success capacity range: full specified",
			req: &csi.CreateVolumeRequest{
				Name: "test-vol",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: utils.GbToBytes(20),
					LimitBytes:    utils.GbToBytes(50),
				},
				VolumeCapabilities: stdVolCap,
				Parameters:         stdParams,
			},
			expVol: &csi.Volume{
				CapacityBytes: utils.GbToBytes(20),
				Id:            zone + "/" + "test-vol",
				Attributes:    nil,
			},
		},
		{
			name: "fail capacity range: limit less than required",
			req: &csi.CreateVolumeRequest{
				Name: "test-vol",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: utils.GbToBytes(50),
					LimitBytes:    utils.GbToBytes(20),
				},
				VolumeCapabilities: stdVolCap,
				Parameters:         stdParams,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "success capacity range: only limit specified",
			req: &csi.CreateVolumeRequest{
				Name: "test-vol",
				CapacityRange: &csi.CapacityRange{
					LimitBytes: utils.GbToBytes(50),
				},
				VolumeCapabilities: stdVolCap,
				Parameters:         stdParams,
			},
			expVol: &csi.Volume{
				CapacityBytes: MinimumVolumeSizeInBytes,
				Id:            zone + "/" + "test-vol",
				Attributes:    nil,
			},
		},
		{
			name: "fail capacity range: only limit specified too low",
			req: &csi.CreateVolumeRequest{
				Name: "test-vol",
				CapacityRange: &csi.CapacityRange{
					LimitBytes: utils.GbToBytes(2),
				},
				VolumeCapabilities: stdVolCap,
				Parameters:         stdParams,
			},
			expErrCode: codes.InvalidArgument,
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
		err = gceDriver.SetupGCEDriver(fakeCloudProvider, nil, driver, node)
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

// Test volume already exists

// Test volume with op pending

// Test DeleteVolume

// Test ControllerPublishVolume

// Test ControllerUnpublishVolume

// Test ValidateVolumeCapabilities

// Test ListVolumes

// Test GetCapacity

// Test ControllerGetCapabilities
