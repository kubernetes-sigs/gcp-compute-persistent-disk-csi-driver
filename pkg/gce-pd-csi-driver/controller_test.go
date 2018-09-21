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
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"testing"
	"time"

	"golang.org/x/net/context"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/sets"

	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	metadataservice "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/metadata"
)

const (
	project = metadataservice.FakeProject
	zone    = metadataservice.FakeZone
	node    = "test-node"
	driver  = "test-driver"
	name    = "test-name"
)

var (
	// Define "normal" parameters
	stdVolCap = []*csi.VolumeCapability{
		{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}
	stdCapRange = &csi.CapacityRange{
		RequiredBytes: common.GbToBytes(20),
	}
	stdParams = map[string]string{
		common.ParameterKeyType: "test-type",
	}
	stdTopology = []*csi.Topology{
		{
			Segments: map[string]string{common.TopologyKeyZone: metadataservice.FakeZone},
		},
	}
	testVolumeId         = fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, name)
	region, _            = common.GetRegionFromZones([]string{zone})
	testRegionalId       = fmt.Sprintf("projects/%s/regions/%s/disks/%s", project, region, name)
	testSnapshotId       = fmt.Sprintf("projects/%s/global/snapshots/%s", project, name)
	totalSnapshotsNumber = 5
)

func TestCreateSnapshotArguments(t *testing.T) {
	timestamp, _ := time.Parse(time.RFC3339, gce.Timestamp)
	// Define test cases
	testCases := []struct {
		name        string
		req         *csi.CreateSnapshotRequest
		expSnapshot *csi.Snapshot
		expErrCode  codes.Code
	}{
		{
			name: "success default snapshot of zonal disk",
			req: &csi.CreateSnapshotRequest{
				Name:           name,
				SourceVolumeId: testVolumeId,
			},
			expSnapshot: &csi.Snapshot{
				Id:             testSnapshotId,
				SourceVolumeId: testVolumeId,
				CreatedAt:      timestamp.UnixNano(),
				SizeBytes:      common.GbToBytes(gce.DiskSizeGb),
				Status: &csi.SnapshotStatus{
					Type: csi.SnapshotStatus_UPLOADING,
				},
			},
		},
		{
			name: "fail no name",
			req: &csi.CreateSnapshotRequest{
				Name:           "",
				SourceVolumeId: testVolumeId,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "fail no source volume name",
			req: &csi.CreateSnapshotRequest{
				Name:           name,
				SourceVolumeId: "",
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "fail wrong source volume",
			req: &csi.CreateSnapshotRequest{
				Name:           name,
				SourceVolumeId: "/test/wrongname",
			},
			expErrCode: codes.NotFound,
		},
	}

	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		gceDriver := GetGCEDriver()
		fakeCloudProvider, err := gce.FakeCreateCloudProvider(project, zone)
		if err != nil {
			t.Fatalf("Failed to create fake cloud provider: %v", err)
		}
		err = gceDriver.SetupGCEDriver(fakeCloudProvider, nil, nil, metadataservice.NewFakeService(), driver, "vendor-version")
		if err != nil {
			t.Fatalf("Failed to setup GCE Driver: %v", err)
		}

		// Start Test
		resp, err := gceDriver.cs.CreateSnapshot(context.Background(), tc.req)
		//check response
		if err != nil {
			serverError, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Could not get error status code from err: %v", serverError)
			}
			if serverError.Code() != tc.expErrCode {
				t.Fatalf("Expected error code: %v, got: %v. err : %v", tc.expErrCode, serverError.Code(), err)
			}
			continue
		}
		if tc.expErrCode != codes.OK {
			t.Fatalf("Expected error: %v, got no error", tc.expErrCode)
		}

		// Make sure responses match
		snapshot := resp.GetSnapshot()
		if snapshot == nil {
			// If one is nil but not both
			t.Fatalf("Expected snapshot %v, got nil snapshot", tc.expSnapshot)
		}

		if !reflect.DeepEqual(snapshot, tc.expSnapshot) {
			errStr := fmt.Sprintf("Expected snapshot: %#v\n to equal snapshot: %#v\n", snapshot, tc.expSnapshot)
			t.Errorf(errStr)
		}
	}
}
func TestDeleteSnapshot(t *testing.T) {
	testCases := []struct {
		name       string
		req        *csi.DeleteSnapshotRequest
		expErrCode codes.Code
	}{
		{
			name: "valid",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: testSnapshotId,
			},
		},
		{
			name: "invalid id",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: testSnapshotId + "/foo",
			},
		},
		{
			name: "empty id",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: "",
			},
			expErrCode: codes.InvalidArgument,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		gceDriver := initGCEDriver(t)

		_, err := gceDriver.cs.DeleteSnapshot(context.Background(), tc.req)
		//check response
		if err != nil {
			serverError, ok := status.FromError(err)
			t.Logf("get server error %v", serverError)
			if !ok {
				t.Fatalf("Could not get error status code from err: %v", serverError)
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

func TestListSnapshotsArguments(t *testing.T) {
	// Define test cases
	testCases := []struct {
		name         string
		req          *csi.ListSnapshotsRequest
		expSnapshots int
		expErrCode   codes.Code
	}{
		{
			name: "valid",
			req: &csi.ListSnapshotsRequest{
				SnapshotId: testSnapshotId,
			},
			expSnapshots: 1,
		},
		{
			name: "invalid id",
			req: &csi.ListSnapshotsRequest{
				SnapshotId: testSnapshotId + "/foo",
			},
			expSnapshots: 0,
		},
		{
			name: "no id",
			req: &csi.ListSnapshotsRequest{
				SnapshotId: "",
			},
			expSnapshots: totalSnapshotsNumber + 1,
		},
	}

	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		gceDriver := GetGCEDriver()
		fakeCloudProvider, err := gce.FakeCreateCloudProvider(project, zone)
		if err != nil {
			t.Fatalf("Failed to create fake cloud provider: %v", err)
		}
		err = gceDriver.SetupGCEDriver(fakeCloudProvider, nil, nil, metadataservice.NewFakeService(), driver, "vendor-version")
		if err != nil {
			t.Fatalf("Failed to setup GCE Driver: %v", err)
		}

		createReq := &csi.CreateSnapshotRequest{
			Name:           name,
			SourceVolumeId: testVolumeId,
		}
		gceDriver.cs.CreateSnapshot(context.Background(), createReq)

		for i := 0; i < totalSnapshotsNumber; i++ {
			volumeId := fmt.Sprintf("%s%d", testVolumeId, i)
			nameId := fmt.Sprintf("%s%d", name, i)
			createReq := &csi.CreateSnapshotRequest{
				Name:           nameId,
				SourceVolumeId: volumeId,
			}
			_, err := gceDriver.cs.CreateSnapshot(context.Background(), createReq)
			if err != nil {
				t.Errorf("error %v", err)
			}
		}

		// Start Test
		resp, err := gceDriver.cs.ListSnapshots(context.Background(), tc.req)
		//check response
		if err != nil {
			serverError, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Could not get error status code from err: %v", serverError)
			}
			if serverError.Code() != tc.expErrCode {
				t.Fatalf("Expected error code: %v, got: %v. err : %v", tc.expErrCode, serverError.Code(), err)
			}
			continue
		}
		if tc.expErrCode != codes.OK {
			t.Fatalf("Expected error: %v, got no error", tc.expErrCode)
		}

		// Make sure responses match
		snapshots := resp.GetEntries()
		//expectsnapshots := expSnapshot.GetEntries()
		if (snapshots == nil || len(snapshots) == 0) && tc.expSnapshots == 0 {
			continue
		}

		if snapshots == nil || len(snapshots) == 0 {
			// If one is nil or empty but not both
			t.Fatalf("Expected snapshots number %v, got no snapshot", tc.expSnapshots)
		}
		if len(snapshots) != tc.expSnapshots {
			errStr := fmt.Sprintf("Expected snapshot: %#v\n to equal snapshot: %#v\n", snapshots[0].Snapshot, tc.expSnapshots)
			t.Errorf(errStr)
		}
	}
}

func TestCreateVolumeArguments(t *testing.T) {

	// Define test cases
	testCases := []struct {
		name       string
		req        *csi.CreateVolumeRequest
		expVol     *csi.Volume
		expErrCode codes.Code
	}{
		{
			name: "success default",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCap,
				Parameters:         stdParams,
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				Id:                 testVolumeId,
				Attributes:         nil,
				AccessibleTopology: stdTopology,
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
				Name:               "test-name",
				VolumeCapabilities: stdVolCap,
				Parameters:         stdParams,
			},
			expVol: &csi.Volume{
				CapacityBytes:      MinimumVolumeSizeInBytes,
				Id:                 testVolumeId,
				Attributes:         nil,
				AccessibleTopology: stdTopology,
			},
		},
		{
			name: "fail no capabilities",
			req: &csi.CreateVolumeRequest{
				Name:          "test-name",
				CapacityRange: stdCapRange,
				Parameters:    stdParams,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "success no params",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCap,
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				Id:                 testVolumeId,
				Attributes:         nil,
				AccessibleTopology: stdTopology,
			},
		},
		{
			name: "success with random secrets",
			req: &csi.CreateVolumeRequest{
				Name:                    "test-name",
				CapacityRange:           stdCapRange,
				VolumeCapabilities:      stdVolCap,
				Parameters:              stdParams,
				ControllerCreateSecrets: map[string]string{"key1": "this is a random", "crypto": "secret"},
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				Id:                 testVolumeId,
				Attributes:         nil,
				AccessibleTopology: stdTopology,
			},
		},
		{
			name: "success with topology",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCap,
				Parameters:         map[string]string{"type": "test-type"},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "topology-zone"},
						},
					},
				},
			},
			expVol: &csi.Volume{
				CapacityBytes: common.GbToBytes(20),
				Id:            fmt.Sprintf("projects/%s/zones/topology-zone/disks/%s", metadataservice.FakeProject, name),
				Attributes:    nil,
				AccessibleTopology: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone"},
					},
				},
			},
		},
		{
			name: "success with picking first preferred topology",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCap,
				Parameters:         map[string]string{"type": "test-type"},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "topology-zone3"},
						},
						{
							Segments: map[string]string{common.TopologyKeyZone: "topology-zone1"},
						},
						{
							Segments: map[string]string{common.TopologyKeyZone: "topology-zone2"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "topology-zone2"},
						},
						{
							Segments: map[string]string{common.TopologyKeyZone: "topology-zone3"},
						},
						{
							Segments: map[string]string{common.TopologyKeyZone: "topology-zone1"},
						},
					},
				},
			},
			expVol: &csi.Volume{
				CapacityBytes: common.GbToBytes(20),
				Id:            fmt.Sprintf("projects/%s/zones/topology-zone2/disks/%s", metadataservice.FakeProject, name),
				Attributes:    nil,
				AccessibleTopology: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone2"},
					},
				},
			},
		},
		{
			name: "fail with extra topology",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCap,
				Parameters:         stdParams,
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{"ooblezoners": "topology-zone", common.TopologyKeyZone: "top-zone"},
						},
					},
				},
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "fail with missing topology zone",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCap,
				Parameters:         stdParams,
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{},
						},
					},
				},
			},
			expErrCode: codes.InvalidArgument,
		},
		// RePD Tests
		{
			name: "success with topology with repd",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCap,
				Parameters:         map[string]string{common.ParameterKeyReplicationType: replicationTypeRegionalPD},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: region + "-c"},
						},
						{
							Segments: map[string]string{common.TopologyKeyZone: region + "-b"},
						},
					},
				},
			},
			expVol: &csi.Volume{
				CapacityBytes: common.GbToBytes(20),
				Id:            testRegionalId,
				Attributes:    nil,
				AccessibleTopology: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: region + "-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: region + "-b"},
					},
				},
			},
		},
		{
			name: "fail not enough topology with repd",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCap,
				Parameters: map[string]string{
					common.ParameterKeyReplicationType: replicationTypeRegionalPD,
				},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: region + "-c"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: region + "-c"},
						},
					},
				},
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "success with no toplogy specified with repd",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCap,
				Parameters: map[string]string{
					common.ParameterKeyReplicationType: replicationTypeRegionalPD,
				},
			},
			expVol: &csi.Volume{
				CapacityBytes: common.GbToBytes(20),
				Id:            testRegionalId,
				Attributes:    nil,
				AccessibleTopology: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: metadataservice.FakeZone},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "country-region-fakesecondzone"},
					},
				},
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
		err = gceDriver.SetupGCEDriver(fakeCloudProvider, nil, nil, metadataservice.NewFakeService(), driver, "vendor-version")
		if err != nil {
			t.Fatalf("Failed to setup GCE Driver: %v", err)
		}

		// Start Test
		resp, err := gceDriver.cs.CreateVolume(context.Background(), tc.req)
		//check response
		if err != nil {
			serverError, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Could not get error status code from err: %v", serverError)
			}
			if serverError.Code() != tc.expErrCode {
				t.Fatalf("Expected error code: %v, got: %v. err : %v", tc.expErrCode, serverError.Code(), err)
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

		if !reflect.DeepEqual(vol, tc.expVol) {
			errStr := fmt.Sprintf("Expected volume: %#v\nTopology %#v\n\n to equal volume: %#v\nTopology %#v\n\n",
				vol, vol.GetAccessibleTopology()[0], tc.expVol, tc.expVol.GetAccessibleTopology()[0])
			if len(vol.GetAccessibleTopology()) != len(tc.expVol.GetAccessibleTopology()) {
				t.Errorf("Accessible topologies are not the same length, got %v, expected %v", len(vol.GetAccessibleTopology()), len(tc.expVol.GetAccessibleTopology()))
			}
			for i := 0; i < len(vol.GetAccessibleTopology()); i++ {
				errStr = errStr + fmt.Sprintf("Got topology %#v\nExpected toplogy %#v\n\n", vol.GetAccessibleTopology()[i], tc.expVol.GetAccessibleTopology()[i])
			}
			t.Errorf(errStr)
		}
	}
}

func TestCreateVolumeRandomRequisiteTopology(t *testing.T) {
	req := &csi.CreateVolumeRequest{
		Name:               "test-name",
		CapacityRange:      stdCapRange,
		VolumeCapabilities: stdVolCap,
		Parameters:         map[string]string{"type": "test-type"},
		AccessibilityRequirements: &csi.TopologyRequirement{
			Requisite: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "topology-zone3"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "topology-zone1"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "topology-zone2"},
				},
			},
		},
	}

	gceDriver := GetGCEDriver()
	fakeCloudProvider, err := gce.FakeCreateCloudProvider(project, zone)
	if err != nil {
		t.Fatalf("Failed to create fake cloud provider: %v", err)
	}
	err = gceDriver.SetupGCEDriver(fakeCloudProvider, nil, nil, metadataservice.NewFakeService(), driver, "vendor-version")
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}

	tZones := map[string]bool{}
	// Start Test
	for i := 0; i < 25; i++ {
		resp, err := gceDriver.cs.CreateVolume(context.Background(), req)
		if err != nil {
			t.Fatalf("CreateVolume did not expect error, but got %v", err)
		}
		tZone, ok := resp.GetVolume().GetAccessibleTopology()[0].GetSegments()[common.TopologyKeyZone]
		if !ok {
			t.Fatalf("Could not find topology zone in response")
		}
		tZones[tZone] = true
	}
	// We expect that we should have picked all 3 topology zones here
	if len(tZones) != 3 {
		t.Fatalf("Expected all 3 topology zones to be rotated through, got only: %v", tZones)
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

		_, err := gceDriver.cs.DeleteVolume(context.Background(), tc.req)
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
				RequiredBytes: common.GbToBytes(20),
				LimitBytes:    common.GbToBytes(50),
			},
			expCap: common.GbToBytes(20),
		},
		{
			name: "success: fully specified required below min",
			capRange: &csi.CapacityRange{
				RequiredBytes: MinimumVolumeSizeInBytes - 1,
				LimitBytes:    common.GbToBytes(50),
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
				RequiredBytes: common.GbToBytes(50),
				LimitBytes:    common.GbToBytes(20),
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
		deviceName  string
		instance    *compute.Instance
		expAttached bool
	}{
		{
			name:       "normal-attached",
			deviceName: "test-disk",
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
			name:       "normal-not-attached",
			deviceName: "test-disk",
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
		if attached := diskIsAttached(tc.deviceName, tc.instance); attached != tc.expAttached {
			t.Errorf("Expected disk attached to be %v, but got %v", tc.expAttached, attached)
		}
	}
}

func TestDiskIsAttachedAndCompatible(t *testing.T) {
	testCases := []struct {
		name        string
		deviceName  string
		instance    *compute.Instance
		mode        string
		expAttached bool
		expErr      bool
	}{
		{
			name:       "normal-attached",
			deviceName: "test-disk",
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
			name:       "normal-not-attached",
			deviceName: "test-disk",
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
			name:       "incompatible mode",
			deviceName: "test-disk",
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
		attached, err := diskIsAttachedAndCompatible(tc.deviceName, tc.instance, nil, tc.mode)
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

func TestGetZonesFromTopology(t *testing.T) {
	testCases := []struct {
		name     string
		topology []*csi.Topology
		expZones sets.String
		expErr   bool
	}{
		{
			name: "succes: normal",
			topology: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "test-zone"},
				},
			},
			expZones: sets.NewString([]string{"test-zone"}...),
		},
		{
			name: "succes: multiple topologies",
			topology: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "test-zone"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "test-zone2"},
				},
			},
			expZones: sets.NewString([]string{"test-zone", "test-zone2"}...),
		},
		{
			name: "fail: wrong key",
			topology: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "test-zone"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "test-zone2"},
				},
				{
					Segments: map[string]string{"fake-key": "fake-value"},
				},
			},
			expErr: true,
		},
		{
			name: "success: duplicate",
			topology: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "test-zone"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "test-zone"},
				},
			},
			expZones: sets.NewString([]string{"test-zone"}...),
		},
		{
			name:     "success: empty",
			topology: []*csi.Topology{},
			expZones: sets.NewString(),
		},
		{
			name: "fail: wrong key inside",
			topology: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "test-zone", "fake-key": "fake-value"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "test-zone2"},
				},
			},
			expErr: true,
		},
		{
			name:     "success: no topology",
			expZones: sets.NewString(),
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		gotZones, err := getZonesFromTopology(tc.topology)
		if err != nil && !tc.expErr {
			t.Errorf("Did not expect error but got: %v", err)
		}
		if err == nil && tc.expErr {
			t.Errorf("Expected error but got none")
		}

		gotZonesSet := sets.NewString(gotZones...)
		if !gotZonesSet.Equal(tc.expZones) {
			t.Errorf("Expected zones: %v, instead got: %v", tc.expZones, gotZonesSet)
		}
	}
}

func TestPickZonesFromTopology(t *testing.T) {
	testCases := []struct {
		name     string
		top      *csi.TopologyRequirement
		numZones int
		expZones []string
		expErr   bool
	}{
		{
			name: "success: preferred",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone3"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone1"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone2"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone2"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone3"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone1"},
					},
				},
			},
			numZones: 2,
			expZones: []string{"topology-zone2", "topology-zone3"},
		},
		{
			name: "success: preferred and requisite",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone3"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone1"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone2"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone5"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone6"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone2"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone3"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone1"},
					},
				},
			},
			numZones: 5,
			expZones: []string{"topology-zone2", "topology-zone3", "topology-zone1", "topology-zone5", "topology-zone6"},
		},
		{
			name: "fail: not enough topologies",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone3"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone1"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone2"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone2"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone3"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone1"},
					},
				},
			},
			numZones: 4,
			expErr:   true,
		},
		{
			name: "success: only requisite",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone3"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone1"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "topology-zone2"},
					},
				},
			},
			numZones: 3,
			expZones: []string{"topology-zone2", "topology-zone3", "topology-zone1"},
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		gotZones, err := pickZonesFromTopology(tc.top, tc.numZones)
		if err != nil && !tc.expErr {
			t.Errorf("Did not expect error but got: %v", err)
		}
		if err == nil && tc.expErr {
			t.Errorf("Expected error but got none")
		}
		if !sets.NewString(gotZones...).Equal(sets.NewString(tc.expZones...)) {
			t.Errorf("Expected zones: %v, but got: %v", tc.expZones, gotZones)
		}
	}
}

func TestPickRandAndConsecutive(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	testCases := []struct {
		name   string
		slice  []string
		n      int
		expErr bool
	}{
		{
			name:  "success: normal",
			slice: []string{"test", "second", "third"},
			n:     2,
		},
		{
			name:  "success: full",
			slice: []string{"test", "second", "third"},
			n:     3,
		},
		{
			name:  "success: large",
			slice: []string{"test", "second", "third", "fourth", "fifth", "sixth"},
			n:     2,
		},
		{
			name:   "fail: n too large",
			slice:  []string{},
			n:      2,
			expErr: true,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		tot := sets.String{}
		sort.Strings(tc.slice)
		for i := 0; i < 25; i++ {
			theslice, err := pickRandAndConsecutive(tc.slice, tc.n)
			if err != nil && !tc.expErr {
				t.Errorf("Did not expect error but got: %v", err)
			}
			if err == nil && tc.expErr {
				t.Errorf("Expected error but got none")
			}
			if err != nil {
				break
			}
			if len(theslice) != tc.n {
				t.Errorf("expected the resulting slice to be length %v, but got %v instead", tc.n, theslice)
			}
			// Find where it is in the slice
			var idx = -1
			for j, elem := range tc.slice {
				if elem == theslice[0] {
					idx = j
					break
				}
			}
			if idx == -1 {
				t.Errorf("could not find %v in the original slice %v", theslice[0], tc.slice)
			}
			for j := 0; j < tc.n; j++ {
				if theslice[j] != tc.slice[(idx+j)%len(tc.slice)] {
					t.Errorf("did not pick sorted consecutive values from the slice")
				}
			}

			tot.Insert(theslice...)
		}
		if !tot.Equal(sets.NewString(tc.slice...)) {
			t.Errorf("randomly picking n from slice did not get all %v, instead got only %v", tc.slice, tot)
		}

	}
}
