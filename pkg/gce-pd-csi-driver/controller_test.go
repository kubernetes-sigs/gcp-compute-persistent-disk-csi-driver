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

	"context"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"github.com/golang/protobuf/ptypes"

	compute "google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/retry"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/apimachinery/pkg/util/clock"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	gcecloudprovider "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
)

const (
	project    = "test-project"
	zone       = "country-region-zone"
	secondZone = "country-region-fakesecondzone"
	node       = "test-node"
	driver     = "test-driver"
	name       = "test-name"
)

var (
	// Define "normal" parameters
	stdCapRange = &csi.CapacityRange{
		RequiredBytes: common.GbToBytes(20),
	}
	stdParams = map[string]string{
		common.ParameterKeyType: "test-type",
	}
	stdTopology = []*csi.Topology{
		{
			Segments: map[string]string{common.TopologyKeyZone: zone},
		},
	}
	testVolumeID   = fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, name)
	region, _      = common.GetRegionFromZones([]string{zone})
	testRegionalID = fmt.Sprintf("projects/%s/regions/%s/disks/%s", project, region, name)
	testSnapshotID = fmt.Sprintf("projects/%s/global/snapshots/%s", project, name)
	testNodeID     = fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone, node)
)

func TestCreateSnapshotArguments(t *testing.T) {
	thetime, _ := time.Parse(time.RFC3339, gce.Timestamp)
	tp, err := ptypes.TimestampProto(thetime)
	if err != nil {
		t.Fatalf("Unable to conver time to timestamp: %v", err)
	}
	// Define test cases
	testCases := []struct {
		name        string
		req         *csi.CreateSnapshotRequest
		seedDisks   []*gce.CloudDisk
		expSnapshot *csi.Snapshot
		expErrCode  codes.Code
	}{
		{
			name: "success default snapshot of zonal disk",
			req: &csi.CreateSnapshotRequest{
				Name:           name,
				SourceVolumeId: testVolumeID,
				Parameters:     map[string]string{common.ParameterKeyStorageLocations: " US-WEST2"},
			},
			seedDisks: []*gce.CloudDisk{
				createZonalCloudDisk(name),
			},
			expSnapshot: &csi.Snapshot{
				SnapshotId:     testSnapshotID,
				SourceVolumeId: testVolumeID,
				CreationTime:   tp,
				SizeBytes:      common.GbToBytes(gce.DiskSizeGb),
				ReadyToUse:     false,
			},
		},
		{
			name: "fail no name",
			req: &csi.CreateSnapshotRequest{
				Name:           "",
				SourceVolumeId: testVolumeID,
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
			name: "fail not found source volume",
			req: &csi.CreateSnapshotRequest{
				Name:           name,
				SourceVolumeId: common.CreateZonalVolumeID(project, zone, "non-exist-vol-name"),
			},
			expErrCode: codes.NotFound,
		},
		{
			name: "fail invalid source volume",
			req: &csi.CreateSnapshotRequest{
				Name:           name,
				SourceVolumeId: "/test/wrongname",
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "invalid snapshot parameter key",
			req: &csi.CreateSnapshotRequest{
				Name:           name,
				SourceVolumeId: testVolumeID,
				Parameters:     map[string]string{"bad-key": ""},
			},
			seedDisks: []*gce.CloudDisk{
				createZonalCloudDisk(name),
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "invalid snapshot locations",
			req: &csi.CreateSnapshotRequest{
				Name:           name,
				SourceVolumeId: testVolumeID,
				Parameters:     map[string]string{common.ParameterKeyStorageLocations: "bad-region"},
			},
			seedDisks: []*gce.CloudDisk{
				createZonalCloudDisk(name),
			},
			expErrCode: codes.InvalidArgument,
		},
	}

	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		gceDriver := initGCEDriver(t, tc.seedDisks)

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
				SnapshotId: testSnapshotID,
			},
		},
		{
			name: "invalid id",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: testSnapshotID + "/foo",
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
		gceDriver := initGCEDriver(t, nil)

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
		numSnapshots int
		expErrCode   codes.Code
	}{
		{
			name: "valid",
			req: &csi.ListSnapshotsRequest{
				SnapshotId: testSnapshotID + "0",
			},
			numSnapshots: 1,
		},
		{
			name: "invalid id",
			req: &csi.ListSnapshotsRequest{
				SnapshotId: testSnapshotID + "/foo",
			},
			numSnapshots: 0,
		},
		{
			name: "no id",
			req: &csi.ListSnapshotsRequest{
				SnapshotId: "",
			},
			numSnapshots: 5,
		},
	}

	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)

		disks := []*gce.CloudDisk{}
		for i := 0; i < tc.numSnapshots; i++ {
			sname := fmt.Sprintf("%s%d", name, i)
			disks = append(disks, createZonalCloudDisk(sname))
		}

		// Setup new driver each time so no interference
		gceDriver := initGCEDriver(t, disks)

		for i := 0; i < tc.numSnapshots; i++ {
			volumeID := fmt.Sprintf("%s%d", testVolumeID, i)
			nameID := fmt.Sprintf("%s%d", name, i)
			createReq := &csi.CreateSnapshotRequest{
				Name:           nameID,
				SourceVolumeId: volumeID,
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
		if (snapshots == nil || len(snapshots) == 0) && tc.numSnapshots == 0 {
			continue
		}

		if snapshots == nil || len(snapshots) == 0 {
			// If one is nil or empty but not both
			t.Fatalf("Expected snapshots number %v, got no snapshot", tc.numSnapshots)
		}
		if len(snapshots) != tc.numSnapshots {
			errStr := fmt.Sprintf("Expected snapshot: %#v\n to equal snapshot: %#v\n", snapshots[0].Snapshot, tc.numSnapshots)
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
				VolumeCapabilities: stdVolCaps,
				Parameters:         stdParams,
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				VolumeId:           testVolumeID,
				VolumeContext:      nil,
				AccessibleTopology: stdTopology,
			},
		},
		{
			name: "fail with MULTI_NODE_READER_ONLY",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: createVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY),
				Parameters:         stdParams,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "fail with mount/MULTI_NODE_MULTI_WRITER capabilities",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: createVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER),
				Parameters:         stdParams,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "success with block/MULTI_NODE_MULTI_WRITER capabilities",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: createBlockVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER),
				Parameters:         stdParams,
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				VolumeId:           testVolumeID,
				VolumeContext:      nil,
				AccessibleTopology: stdTopology,
			},
		},
		{
			name: "fail no name",
			req: &csi.CreateVolumeRequest{
				Name:               "",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         stdParams,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "success no capacity range",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				VolumeCapabilities: stdVolCaps,
				Parameters:         stdParams,
			},
			expVol: &csi.Volume{
				CapacityBytes:      MinimumVolumeSizeInBytes,
				VolumeId:           testVolumeID,
				VolumeContext:      nil,
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
				VolumeCapabilities: stdVolCaps,
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				VolumeId:           testVolumeID,
				VolumeContext:      nil,
				AccessibleTopology: stdTopology,
			},
		},
		{
			name: "success with random secrets",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         stdParams,
				Secrets:            map[string]string{"key1": "this is a random", "crypto": "secret"},
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				VolumeId:           testVolumeID,
				VolumeContext:      nil,
				AccessibleTopology: stdTopology,
			},
		},
		{
			name: "success with topology",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
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
				VolumeId:      fmt.Sprintf("projects/%s/zones/topology-zone/disks/%s", project, name),
				VolumeContext: nil,
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
				VolumeCapabilities: stdVolCaps,
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
				VolumeId:      fmt.Sprintf("projects/%s/zones/topology-zone2/disks/%s", project, name),
				VolumeContext: nil,
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
				VolumeCapabilities: stdVolCaps,
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
				VolumeCapabilities: stdVolCaps,
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
				VolumeCapabilities: stdVolCaps,
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
				VolumeId:      testRegionalID,
				VolumeContext: nil,
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
				VolumeCapabilities: stdVolCaps,
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
				VolumeCapabilities: stdVolCaps,
				Parameters: map[string]string{
					common.ParameterKeyReplicationType: replicationTypeRegionalPD,
				},
			},
			expVol: &csi.Volume{
				CapacityBytes: common.GbToBytes(20),
				VolumeId:      testRegionalID,
				VolumeContext: nil,
				AccessibleTopology: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: zone},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: secondZone},
					},
				},
			},
		},
		{
			name: "success with block volume capability",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: createBlockVolumeCapabilities(csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER),
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				VolumeId:           testVolumeID,
				VolumeContext:      nil,
				AccessibleTopology: stdTopology,
			},
		},
		{
			name: "fail with both mount and block volume capability",
			req: &csi.CreateVolumeRequest{
				Name:          name,
				CapacityRange: stdCapRange,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					{
						AccessType: &csi.VolumeCapability_Block{
							Block: &csi.VolumeCapability_BlockVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
				},
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "success with disk encryption kms key",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters: map[string]string{
					common.ParameterKeyDiskEncryptionKmsKey: "projects/KMS_PROJECT_ID/locations/REGION/keyRings/KEY_RING/cryptoKeys/KEY",
				},
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				VolumeId:           testVolumeID,
				VolumeContext:      nil,
				AccessibleTopology: stdTopology,
			},
		},
		{
			name: "success with labels parameter",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         map[string]string{"labels": "key1=value1,key2=value2"},
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				VolumeId:           testVolumeID,
				VolumeContext:      nil,
				AccessibleTopology: stdTopology,
			},
		},
		{
			name: "fail with malformed labels parameter",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         map[string]string{"labels": "key1=value1,#=$;;"},
			},
			expErrCode: codes.InvalidArgument,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		gceDriver := initGCEDriver(t, nil)

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

func TestListVolumeArgs(t *testing.T) {
	diskCount := 600
	testCases := []struct {
		name            string
		maxEntries      int32
		expectedEntries int
		expectedErr     bool
	}{
		{
			name:            "normal",
			expectedEntries: diskCount,
		},
		{
			name:            "fine amount of entries",
			maxEntries:      420,
			expectedEntries: 420,
		},
		{
			name:        "negative entries",
			maxEntries:  -1,
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup new driver each time so no interference
			var d []*gce.CloudDisk
			for i := 0; i < diskCount; i++ {
				// Create 600 dummy disks
				d = append(d, gce.CloudDiskFromV1(&compute.Disk{Name: fmt.Sprintf("%v", i)}))
			}
			gceDriver := initGCEDriver(t, d)
			lvr := &csi.ListVolumesRequest{
				MaxEntries: tc.maxEntries,
			}
			resp, err := gceDriver.cs.ListVolumes(context.TODO(), lvr)
			if tc.expectedErr && err == nil {
				t.Fatalf("Got no error when expecting an error")
			}
			if err != nil {
				if !tc.expectedErr {
					t.Fatalf("Got error %v, expecting none", err)
				}
				return
			}

			if len(resp.Entries) != tc.expectedEntries {
				t.Fatalf("Got %v entries, expected %v", len(resp.Entries), tc.expectedEntries)
			}
		})
	}
}

func TestCreateVolumeWithVolumeSourceFromSnapshot(t *testing.T) {
	// Define test cases
	testCases := []struct {
		name            string
		project         string
		volKey          *meta.Key
		snapshotOnCloud bool
		expErrCode      codes.Code
	}{
		{
			name:            "success with data source of snapshot type",
			project:         "test-project",
			volKey:          meta.ZonalKey("my-disk", zone),
			snapshotOnCloud: true,
		},
		{
			name:            "fail with data source of snapshot type that doesn't exist",
			project:         "test-project",
			volKey:          meta.ZonalKey("my-disk", zone),
			snapshotOnCloud: false,
			expErrCode:      codes.NotFound,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		gceDriver := initGCEDriver(t, nil)

		// Start Test
		req := &csi.CreateVolumeRequest{
			Name:               "test-name",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCaps,
			VolumeContentSource: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{
					Snapshot: &csi.VolumeContentSource_SnapshotSource{
						SnapshotId: testSnapshotID,
					},
				},
			},
		}

		if tc.snapshotOnCloud {
			snapshotParams, err := common.ExtractAndDefaultSnapshotParameters(req.GetParameters())
			if err != nil {
				t.Errorf("Got error extracting snapshot parameters: %v", err)
			}
			gceDriver.cs.CloudProvider.CreateSnapshot(context.Background(), tc.project, tc.volKey, name, snapshotParams)
		}
		resp, err := gceDriver.cs.CreateVolume(context.Background(), req)
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

		// Make sure response has snapshot
		vol := resp.GetVolume()
		if vol.ContentSource == nil || vol.ContentSource.Type == nil || vol.ContentSource.GetSnapshot() == nil || vol.ContentSource.GetSnapshot().SnapshotId == "" {
			t.Fatalf("Expected volume content source to have snapshot ID, got none")
		}

	}
}

func TestCreateVolumeWithVolumeSourceFromVolume(t *testing.T) {
	testSourceVolumeName := "test-volume-source-name"
	testZonalVolumeSourceID := fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, testSourceVolumeName)
	testRegionalVolumeSourceID := fmt.Sprintf("projects/%s/regions/%s/disks/%s", project, region, testSourceVolumeName)
	testVolumeSourceIDDifferentZone := fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, "different-zone", testSourceVolumeName)
	topology := &csi.TopologyRequirement{
		Requisite: []*csi.Topology{
			{
				Segments: map[string]string{common.TopologyKeyZone: region + "-b"},
			},
			{
				Segments: map[string]string{common.TopologyKeyZone: region + "-c"},
			},
		},
	}
	regionalParams := map[string]string{
		common.ParameterKeyType: "test-type", common.ParameterKeyReplicationType: "regional-pd",
	}
	// Define test cases
	testCases := []struct {
		name                string
		volumeOnCloud       bool
		expErrCode          codes.Code
		sourceVolumeID      string
		reqParameters       map[string]string
		sourceReqParameters map[string]string
		topology            *csi.TopologyRequirement
	}{
		{
			name:                "success with data source of zonal volume type",
			volumeOnCloud:       true,
			sourceVolumeID:      testZonalVolumeSourceID,
			reqParameters:       stdParams,
			sourceReqParameters: stdParams,
		},
		{
			name:                "success with data source of regional volume type",
			volumeOnCloud:       true,
			sourceVolumeID:      testRegionalVolumeSourceID,
			reqParameters:       regionalParams,
			sourceReqParameters: regionalParams,
			topology:            topology,
		},
		{
			name:                "fail with with data source of replication-type different from CreateVolumeRequest",
			volumeOnCloud:       true,
			expErrCode:          codes.InvalidArgument,
			sourceVolumeID:      testZonalVolumeSourceID,
			reqParameters:       stdParams,
			sourceReqParameters: regionalParams,
			topology:            topology,
		},
		{
			name:                "fail with data source of zonal volume type that doesn't exist",
			volumeOnCloud:       false,
			expErrCode:          codes.NotFound,
			sourceVolumeID:      testZonalVolumeSourceID,
			reqParameters:       stdParams,
			sourceReqParameters: stdParams,
		},
		{
			name:                "fail with data source of zonal volume type with invalid volume id format",
			volumeOnCloud:       false,
			expErrCode:          codes.InvalidArgument,
			sourceVolumeID:      testZonalVolumeSourceID + "invalid/format",
			reqParameters:       stdParams,
			sourceReqParameters: stdParams,
		},
		{
			name:           "fail with data source of zonal volume type with invalid disk parameters",
			volumeOnCloud:  true,
			expErrCode:     codes.InvalidArgument,
			sourceVolumeID: testVolumeSourceIDDifferentZone,
			reqParameters:  stdParams,
			sourceReqParameters: map[string]string{
				common.ParameterKeyType: "different-type",
			},
		},
		{
			name:                "fail with data source of zonal volume type with invalid replication type",
			volumeOnCloud:       true,
			expErrCode:          codes.InvalidArgument,
			sourceVolumeID:      testZonalVolumeSourceID,
			reqParameters:       regionalParams,
			sourceReqParameters: stdParams,
		},
	}

	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		gceDriver := initGCEDriver(t, nil)

		req := &csi.CreateVolumeRequest{
			Name:               name,
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCaps,
			Parameters:         tc.reqParameters,
			VolumeContentSource: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: tc.sourceVolumeID,
					},
				},
			},
		}

		sourceVolumeRequest := &csi.CreateVolumeRequest{
			Name:               testSourceVolumeName,
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCaps,
			Parameters:         tc.sourceReqParameters,
		}

		if tc.topology != nil {
			// req.AccessibilityRequirements = tc.topology
			sourceVolumeRequest.AccessibilityRequirements = tc.topology
		}

		if tc.volumeOnCloud {
			// Create the source volume.
			sourceVolume, _ := gceDriver.cs.CreateVolume(context.Background(), sourceVolumeRequest)
			req.VolumeContentSource = &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: sourceVolume.GetVolume().VolumeId,
					},
				},
			}
		}

		resp, err := gceDriver.cs.CreateVolume(context.Background(), req)
		t.Logf("response: %v err: %v", resp, err)
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

		// Make sure the response has the source volume.
		sourceVolume := resp.GetVolume()
		t.Logf("response has source volume: %v ", sourceVolume)
		if sourceVolume.ContentSource == nil || sourceVolume.ContentSource.Type == nil ||
			sourceVolume.ContentSource.GetVolume() == nil || sourceVolume.ContentSource.GetVolume().VolumeId == "" {
			t.Fatalf("Expected volume content source to have volume ID, got none")
		}
	}
}

func TestCreateVolumeRandomRequisiteTopology(t *testing.T) {
	req := &csi.CreateVolumeRequest{
		Name:               "test-name",
		CapacityRange:      stdCapRange,
		VolumeCapabilities: stdVolCaps,
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

	gceDriver := initGCEDriver(t, nil)

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

func createZonalCloudDisk(name string) *gce.CloudDisk {
	return gce.CloudDiskFromV1(&compute.Disk{
		Name: name,
	})
}

func TestDeleteVolume(t *testing.T) {
	testCases := []struct {
		name      string
		seedDisks []*gce.CloudDisk
		req       *csi.DeleteVolumeRequest
		expErr    bool
	}{
		{
			name: "valid",
			seedDisks: []*gce.CloudDisk{
				createZonalCloudDisk(name),
			},
			req: &csi.DeleteVolumeRequest{
				VolumeId: testVolumeID,
			},
		},
		{
			name: "invalid id",
			req: &csi.DeleteVolumeRequest{
				VolumeId: testVolumeID + "/foo",
			},
			expErr: false,
		},
		{
			name: "repairable ID",
			seedDisks: []*gce.CloudDisk{
				createZonalCloudDisk(name),
			},
			req: &csi.DeleteVolumeRequest{
				VolumeId: common.GenerateUnderspecifiedVolumeID(name, true /* isZonal */),
			},
			expErr: false,
		},
		{
			name: "non-repairable ID (invalid)",
			seedDisks: []*gce.CloudDisk{
				createZonalCloudDisk("nottherightname"),
			},
			req: &csi.DeleteVolumeRequest{
				VolumeId: common.GenerateUnderspecifiedVolumeID(name, true /* isZonal */),
			},
			expErr: false,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		gceDriver := initGCEDriver(t, tc.seedDisks)

		_, err := gceDriver.cs.DeleteVolume(context.Background(), tc.req)
		if err == nil && tc.expErr {
			t.Errorf("Expected error but got none")
		}
		if err != nil && !tc.expErr {
			t.Errorf("Did not expect error but got: %v", err)
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

func TestVolumeOperationConcurrency(t *testing.T) {
	readyToExecute := make(chan chan gcecloudprovider.Signal, 1)
	gceDriver := initBlockingGCEDriver(t, []*gce.CloudDisk{
		createZonalCloudDisk(name + "1"),
		createZonalCloudDisk(name + "2"),
	}, readyToExecute)
	cs := gceDriver.cs

	vol1CreateSnapshotAReq := &csi.CreateSnapshotRequest{
		Name:           name + "1A",
		SourceVolumeId: testVolumeID + "1",
	}
	vol1CreateSnapshotBReq := &csi.CreateSnapshotRequest{
		Name:           name + "1B",
		SourceVolumeId: testVolumeID + "1",
	}
	vol2CreateSnapshotReq := &csi.CreateSnapshotRequest{
		Name:           name + "2",
		SourceVolumeId: testVolumeID + "2",
	}

	runRequest := func(req *csi.CreateSnapshotRequest) <-chan error {
		response := make(chan error)
		go func() {
			_, err := cs.CreateSnapshot(context.Background(), req)
			response <- err
		}()
		return response
	}

	// Start first valid request vol1CreateSnapshotA and block until it reaches the CreateSnapshot
	vol1CreateSnapshotAResp := runRequest(vol1CreateSnapshotAReq)
	execVol1CreateSnapshotA := <-readyToExecute

	// Start vol1CreateSnapshotB and allow it to execute to completion. Then check for Aborted error.
	// If a non Abort error is received or if the operation was started, then there is a problem
	// with volume locking
	vol1CreateSnapshotBResp := runRequest(vol1CreateSnapshotBReq)
	select {
	case err := <-vol1CreateSnapshotBResp:
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
		t.Errorf("The operation for vol1CreateSnapshotB should have been aborted, but was started")
	}

	// Start vol2CreateSnapshot and allow it to execute to completion. Then check for success.
	vol2CreateSnapshotResp := runRequest(vol2CreateSnapshotReq)
	execVol2CreateSnapshot := <-readyToExecute
	execVol2CreateSnapshot <- gcecloudprovider.Signal{}
	if err := <-vol2CreateSnapshotResp; err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// To clean up, allow the vol1CreateSnapshotA to complete
	execVol1CreateSnapshotA <- gcecloudprovider.Signal{}
	if err := <-vol1CreateSnapshotAResp; err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCreateVolumeDiskReady(t *testing.T) {
	// Define test cases
	testCases := []struct {
		name       string
		diskStatus string
		req        *csi.CreateVolumeRequest
		expVol     *csi.Volume
		expErrCode codes.Code
	}{
		{
			name:       "disk status RESTORING",
			diskStatus: "RESTORING",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         stdParams,
			},
			expErrCode: codes.Internal,
		},
		{
			name:       "disk status CREATING",
			diskStatus: "CREATING",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         stdParams,
			},
			expErrCode: codes.Internal,
		},
		{
			name:       "disk status DELETING",
			diskStatus: "DELETING",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         stdParams,
			},
			expErrCode: codes.Internal,
		},
		{
			name:       "disk status FAILED",
			diskStatus: "FAILED",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         stdParams,
			},
			expErrCode: codes.Internal,
		},
		{
			name:       "success default",
			diskStatus: "READY",
			req: &csi.CreateVolumeRequest{
				Name:               "test-name",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         stdParams,
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				VolumeId:           testVolumeID,
				VolumeContext:      nil,
				AccessibleTopology: stdTopology,
			},
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fcp, err := gce.CreateFakeCloudProvider(project, zone, nil)
			if err != nil {
				t.Fatalf("Failed to create fake cloud provider: %v", err)
			}

			// Setup hook to create new disks with given status.
			fcp.UpdateDiskStatus(tc.diskStatus)
			// Setup new driver each time so no interference
			gceDriver := initGCEDriverWithCloudProvider(t, fcp)
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
				return
			}
			if tc.expErrCode != codes.OK {
				t.Fatalf("Expected error: %v, got no error", tc.expErrCode)
			}

			vol := resp.GetVolume()
			if !reflect.DeepEqual(vol, tc.expVol) {
				t.Fatalf("Mismatch in expected vol %v, current volume: %v\n", tc.expVol, vol)
			}
		})
	}
}

func TestControllerPublishInterop(t *testing.T) {
	readyToExecute := make(chan chan gcecloudprovider.Signal, 1)
	disk1 := name + "1"
	disk2 := name + "2"
	cloudDisks := []*gce.CloudDisk{
		createZonalCloudDisk(disk1),
		createZonalCloudDisk(disk2),
	}
	fcp, err := gce.CreateFakeCloudProvider(project, zone, cloudDisks)
	if err != nil {
		t.Fatalf("Failed to create fake cloud provider: %v", err)
	}
	fcpBlocking := &gce.FakeBlockingCloudProvider{
		FakeCloudProvider: fcp,
		ReadyToExecute:    readyToExecute,
	}
	instance := &compute.Instance{
		Name:  node,
		Disks: []*compute.AttachedDisk{},
	}
	fcp.InsertInstance(instance, zone, node)
	gceDriver := initGCEDriverWithCloudProvider(t, fcpBlocking)
	cs := gceDriver.cs
	cs.opsManager.ready = true
	runRequest := func(req *csi.ControllerPublishVolumeRequest) <-chan error {
		response := make(chan error)
		go func() {
			_, err := cs.ControllerPublishVolume(context.Background(), req)
			response <- err
		}()
		return response
	}

	vol1node1PublishReq := &csi.ControllerPublishVolumeRequest{
		VolumeId: testVolumeID + "1",
		NodeId:   testNodeID,
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}

	// Run controller publish request. This is expected to do following:
	// 1. check cache, find no current running ops for disk+instance key
	// 2. Start AttachDisk, update cache with op details
	// 3. release cache lock
	// 4. block on the mock Poll.
	vol1node1ControllerPublishResp := runRequest(vol1node1PublishReq)
	executeVol1ControllerPublish := <-readyToExecute

	//  Verify cache content
	diskInstanceKey := common.CreateDiskInstanceKey(project, zone, disk1, node)
	op := cs.opsManager.opsCache.DiskInstanceOps.GetOp(diskInstanceKey)
	if op == nil {
		t.Errorf("expected attach op in cache")
	}
	if op.Type != "attachDisk" {
		t.Errorf("Unexpected op %s, %s found", op.Name, op.Type)
	}

	// Start controller publish request for vol2 on node1. This is expected to do the following:
	// 1. Check cache and find no entry for disk+instance
	// 2. Start AttachDisk, update cache with op details
	// 3. release cache lock
	// 4. block on the mock Poll.
	vol2node1PublishReq := &csi.ControllerPublishVolumeRequest{
		VolumeId: testVolumeID + "2",
		NodeId:   testNodeID,
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}

	vol2node1ControllerPublishResp := runRequest(vol2node1PublishReq)
	executeVol2ControllerPublish := <-readyToExecute
	disk1InstanceKey := common.CreateDiskInstanceKey(project, zone, disk1, node)
	disk2InstanceKey := common.CreateDiskInstanceKey(project, zone, disk2, node)
	op1 := cs.opsManager.opsCache.DiskInstanceOps.GetOp(disk1InstanceKey)
	if op1 == nil {
		t.Errorf("expected attach op in cache for disk %q", disk1)
	}
	if op1.Type != "attachDisk" {
		t.Errorf("Unexpected op %s, %s found", op.Name, op.Type)
	}
	op2 := cs.opsManager.opsCache.DiskInstanceOps.GetOp(disk2InstanceKey)
	if op2 == nil {
		t.Errorf("expected attach op in cache")
	}
	if op2.Type != "attachDisk" {
		t.Errorf("Unexpected op %s, %s found", op.Name, op.Type)
	}

	// unblock execute of controller publish of first volume
	s := gcecloudprovider.Signal{}
	executeVol1ControllerPublish <- s
	if err := <-vol1node1ControllerPublishResp; err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// verify cache
	op1 = cs.opsManager.opsCache.DiskInstanceOps.GetOp(disk1InstanceKey)
	if op1 != nil {
		t.Errorf("unexpected attach op in cache")
	}

	// unblock execute of controller publish of second volume. Mock error in poll. The CSI op will return failure and mark the node with failures.
	s1 := gcecloudprovider.Signal{ReportError: true}
	executeVol2ControllerPublish <- s1
	if err := <-vol2node1ControllerPublishResp; err == nil {
		t.Errorf("expected error")
	}

	// verify cache still contains op for vol2.
	op2 = cs.opsManager.opsCache.DiskInstanceOps.GetOp(disk2InstanceKey)
	if op2 == nil {
		t.Errorf("Expected attach op in cache for disk %q", disk2)
	}
	if op2.Type != "attachDisk" {
		t.Errorf("Unexpected op %s, %s found", op.Name, op.Type)
	}
}

type DiskInstanceOpCacheEntry struct {
	Key common.DiskInstanceKey
	Op  common.OpInfo
}
type InstanceOpCacheEntry struct {
	Key common.InstanceKey
	Op  common.OpInfo
}

func TestControllerPublishUnpublishDiskInstanceOpCache(t *testing.T) {
	disk1 := name + "1"
	volId1 := testVolumeID + "1"
	containsDiskInstanceEntry := func(cache *common.OpsCache, e DiskInstanceOpCacheEntry) bool {
		op := cache.DiskInstanceOps.GetOp(e.Key)
		if op == nil {
			return false
		}

		if op.Name != e.Op.Name || op.Type != e.Op.Type {
			return false
		}
		return true
	}

	tests := []struct {
		name                        string
		initialCache                []DiskInstanceOpCacheEntry // pre-populate cache before calling CSI op.
		expectAttachDetachOpInCache bool                       // While poll filestore op is in progress, we expect the op to be present in cache.
		pubReq                      *csi.ControllerPublishVolumeRequest
		unpubReq                    *csi.ControllerUnpublishVolumeRequest
		checkOpStatusError          bool // Whether to return an error during check of op status
		checkOpStatusRunning        bool // Whether to return a running status during check of op status
		signalCheckOpDone           bool // if the code path is expected to reach the check op status, the fake blocking cloud provider will block for a signal to proceed forward.
		signalPollOp                bool // if the code path is expected to reach the stage where the filestore op is polled, the fake blocking cloud provider will block for a signal to proceed forward.
		pollOpDoneError             bool // whether to return an error to mock poll error.
		expectCSIOpError            bool // whether the high level csi op is expected to fail
	}{
		{
			name: "empty cache, publish op completes successfully",
			pubReq: &csi.ControllerPublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			signalPollOp: true,
		},
		{
			name: "non-empty initial cache, check op status returns error for the attach op, csi publish op returns error",
			initialCache: []DiskInstanceOpCacheEntry{
				{
					Key: common.CreateDiskInstanceKey(project, zone, disk1, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "attachDisk",
					},
				},
			},
			pubReq: &csi.ControllerPublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			signalCheckOpDone:  true,
			checkOpStatusError: true,
			expectCSIOpError:   true,
		},
		{
			name: "non-empty initial cache, attach op in progress, csi publish op returns error",
			initialCache: []DiskInstanceOpCacheEntry{
				{
					Key: common.CreateDiskInstanceKey(project, zone, disk1, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "attachDisk",
					},
				},
			},
			pubReq: &csi.ControllerPublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			signalCheckOpDone:    true,
			checkOpStatusRunning: true,
			expectCSIOpError:     true,
		},
		{
			name: "non-empty initial cache, detach op check returns error, csi publish op returns error",
			initialCache: []DiskInstanceOpCacheEntry{
				{
					Key: common.CreateDiskInstanceKey(project, zone, disk1, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "detachDisk",
					},
				},
			},
			pubReq: &csi.ControllerPublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			signalCheckOpDone:  true,
			checkOpStatusError: true,
			expectCSIOpError:   true,
		},
		{
			name: "non-empty initial cache, detach op in progress for same disk+instance, csi publish op returns error",
			initialCache: []DiskInstanceOpCacheEntry{
				{
					Key: common.CreateDiskInstanceKey(project, zone, disk1, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "detachDisk",
					},
				},
			},
			pubReq: &csi.ControllerPublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			signalCheckOpDone:    true,
			checkOpStatusRunning: true,
			expectCSIOpError:     true,
		},
		{
			name: "non-empty initial cache, detach op complete for same disk+instance, csi publish op returns success",
			initialCache: []DiskInstanceOpCacheEntry{
				{
					Key: common.CreateDiskInstanceKey(project, zone, disk1, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "detachDisk",
					},
				},
			},
			pubReq: &csi.ControllerPublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			signalCheckOpDone:           true,
			signalPollOp:                true,
			expectAttachDetachOpInCache: true,
		},
		{
			name: "non-empty initial cache, attach op complete for same disk+instance, csi publish op returns success",
			initialCache: []DiskInstanceOpCacheEntry{
				{
					Key: common.CreateDiskInstanceKey(project, zone, disk1, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "attachDisk",
					},
				},
			},
			pubReq: &csi.ControllerPublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			signalCheckOpDone:           true,
			signalPollOp:                true,
			expectAttachDetachOpInCache: true,
		},
		// Unpubish ops
		{
			name: "empty cache, unpublish op completes successfully",
			unpubReq: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
			},
			signalPollOp: true,
		},
		{
			name: "non-empty initial cache, detach op check returns error, csi publish op returns error",
			initialCache: []DiskInstanceOpCacheEntry{
				{
					Key: common.CreateDiskInstanceKey(project, zone, disk1, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "detachDisk",
					},
				},
			},
			unpubReq: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
			},
			signalCheckOpDone:  true,
			checkOpStatusError: true,
			expectCSIOpError:   true,
		},
		{
			name: "non-empty initial cache, detach op in progress for same disk+instance, csi publish op returns error",
			initialCache: []DiskInstanceOpCacheEntry{
				{
					Key: common.CreateDiskInstanceKey(project, zone, disk1, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "detachDisk",
					},
				},
			},
			unpubReq: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
			},
			signalCheckOpDone:    true,
			checkOpStatusRunning: true,
			expectCSIOpError:     true,
		},
		{
			name: "non-empty initial cache, attach op check returns error, csi publish op returns error",
			initialCache: []DiskInstanceOpCacheEntry{
				{
					Key: common.CreateDiskInstanceKey(project, zone, disk1, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "attachDisk",
					},
				},
			},
			unpubReq: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
			},
			signalCheckOpDone:  true,
			checkOpStatusError: true,
			expectCSIOpError:   true,
		},
		{
			name: "non-empty initial cache, attach op in progress for same disk+instance, csi publish op returns error",
			initialCache: []DiskInstanceOpCacheEntry{
				{
					Key: common.CreateDiskInstanceKey(project, zone, disk1, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "attachDisk",
					},
				},
			},
			unpubReq: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
			},
			signalCheckOpDone:    true,
			checkOpStatusRunning: true,
			expectCSIOpError:     true,
		},
		{
			name: "non-empty initial cache, detach op complete for same disk+instance, csi unpublish op returns success",
			initialCache: []DiskInstanceOpCacheEntry{
				{
					Key: common.CreateDiskInstanceKey(project, zone, disk1, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "detachDisk",
					},
				},
			},
			unpubReq: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
			},
			signalCheckOpDone:           true,
			signalPollOp:                true,
			expectAttachDetachOpInCache: true,
		},
		{
			name: "non-empty initial cache, attach op complete for same disk+instance, csi unpublish op returns success",
			initialCache: []DiskInstanceOpCacheEntry{
				{
					Key: common.CreateDiskInstanceKey(project, zone, disk1, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "attachDisk",
					},
				},
			},
			unpubReq: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
			},
			signalCheckOpDone:           true,
			signalPollOp:                true,
			expectAttachDetachOpInCache: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			readyToExecute := make(chan chan gcecloudprovider.Signal, 1)

			cloudDisks := []*gce.CloudDisk{
				createZonalCloudDisk(disk1),
			}
			fcp, err := gce.CreateFakeCloudProvider(project, zone, cloudDisks)
			if err != nil {
				t.Fatalf("Failed to create fake cloud provider: %v", err)
			}
			fcpBlocking := &gce.FakeBlockingCloudProvider{
				FakeCloudProvider: fcp,
				ReadyToExecute:    readyToExecute,
			}
			instance := &compute.Instance{
				Name:  node,
				Disks: []*compute.AttachedDisk{},
			}
			if tc.unpubReq != nil {
				instance.Disks = append(instance.Disks, &compute.AttachedDisk{DeviceName: disk1})
			}
			fcp.InsertInstance(instance, zone, node)
			gceDriver := initGCEDriverWithCloudProvider(t, fcpBlocking)
			cs := gceDriver.cs
			for _, e := range tc.initialCache {
				cs.opsManager.opsCache.DiskInstanceOps.AddOp(e.Key, e.Op)
			}
			cs.opsManager.ready = true

			runPublishRequest := func(req *csi.ControllerPublishVolumeRequest) <-chan error {
				response := make(chan error)
				go func() {
					_, err := cs.ControllerPublishVolume(context.Background(), req)
					response <- err
				}()
				return response
			}
			runUnpublishRequest := func(req *csi.ControllerUnpublishVolumeRequest) <-chan error {
				response := make(chan error)
				go func() {
					_, err := cs.ControllerUnpublishVolume(context.Background(), req)
					response <- err
				}()
				return response
			}

			var resp <-chan error
			if tc.pubReq != nil {
				resp = runPublishRequest(tc.pubReq)
			} else if tc.unpubReq != nil {
				resp = runUnpublishRequest(tc.unpubReq)
			} else {
				t.Errorf("invalid test case")
			}

			// If a key corresponding to the disk+instance found in the cache, controller will check the op status.
			if tc.signalCheckOpDone {
				s := gcecloudprovider.Signal{}
				if tc.checkOpStatusError {
					s.ReportError = true
				} else if tc.checkOpStatusRunning {
					s.ReportRunning = true
				}
				execute := <-readyToExecute
				execute <- s
			}

			if tc.expectAttachDetachOpInCache {
				// Find the running attach/detach op. This may need a few retries, because, at this time the controller publish/unpublish op will the cache and add a new op to cache.
				backoff := wait.Backoff{
					Duration: 10 * time.Millisecond,
					Steps:    100,
				}
				if err := retry.OnError(backoff, func(err error) bool { return true }, func() error {
					ops, innererr := cs.CloudProvider.ListZonalOps(context.Background(), map[string]bool{
						"attachDisk": true, "detachDisk": true})
					if innererr != nil {
						return innererr
					}
					if len(ops) > 0 {
						return nil
					}
					return fmt.Errorf("failed to find ops for fake cloud provider")
				}); err != nil {
					t.Errorf("timed out waiting for attach op to be updated")
					return
				}
				ops, err := cs.CloudProvider.ListZonalOps(context.Background(), map[string]bool{"attachDisk": true})
				if err != nil {
					t.Errorf("Unexpected error in finding ops")
				}
				if len(ops) != 1 {
					t.Errorf("Unexpected number of attach ops in cache")
					return
				}
				// Now the controller has updated the cache and should block at poll
				// verify cache content
				opinfo := common.OpInfo{
					Name: ops[0].Name,
					Type: ops[0].OperationType,
				}
				if !containsDiskInstanceEntry(cs.opsManager.opsCache, DiskInstanceOpCacheEntry{Key: common.CreateDiskInstanceKey(project, zone, disk1, node), Op: opinfo}) {
					t.Errorf("Unexpected cache entry detected")
				}
			}

			// Unblock the poll operation
			if tc.signalPollOp {
				s := gcecloudprovider.Signal{}
				if tc.pollOpDoneError {
					s.ReportError = true
				}
				execute := <-readyToExecute
				execute <- s
			}

			err = <-resp
			if tc.expectCSIOpError && err == nil {
				t.Errorf("Expected error found none")
			}
			if !tc.expectCSIOpError && err != nil {
				t.Errorf("Unexpected error found")
			}
		})
	}
}

func TestControllerPublishUnpublishInstanceOpCache(t *testing.T) {
	type InstanceCacheEntry struct {
		Key common.InstanceKey
		Op  common.OpInfo
	}
	disk1 := name + "1"
	volId1 := testVolumeID + "1"
	containsDiskInstanceEntry := func(cache *common.OpsCache, e DiskInstanceOpCacheEntry) bool {
		op := cache.DiskInstanceOps.GetOp(e.Key)
		if op == nil {
			return false
		}

		if op.Name != e.Op.Name || op.Type != e.Op.Type {
			return false
		}
		return true
	}

	tests := []struct {
		name                        string
		initialCache                []InstanceOpCacheEntry // pre-populate cache before calling CSI op.
		expectAttachDetachOpInCache bool                   // While poll filestore op is in progress, we expect the op to be present in cache.
		pubReq                      *csi.ControllerPublishVolumeRequest
		unpubReq                    *csi.ControllerUnpublishVolumeRequest
		checkOpStatusError          []bool // sequence of error to return during status check for each op.
		checkOpStatusRunning        []bool // sequence of done status to return during status check for each op.
		signalCheckOpDoneCount      int    // Expected number of times, the tester would block on check status op.
		signalPollOp                bool   // if the code path is expected to reach the stage where the filestore op is polled, the fake blocking cloud provider will block for a signal to proceed forward.
		expectCSIOpError            bool   // whether the high level csi op is expected to fail
	}{
		{
			name: "empty cache, publish op completes successfully",
			pubReq: &csi.ControllerPublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			expectAttachDetachOpInCache: true,
			signalPollOp:                true,
		},
		{
			name: "non-empty initial cache, check op status on same instance returns error, csi publish op returns error",
			initialCache: []InstanceOpCacheEntry{
				{
					Key: common.CreateInstanceKey(project, zone, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "attachDisk",
					},
				},
			},
			pubReq: &csi.ControllerPublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			signalCheckOpDoneCount: 1,
			checkOpStatusError:     []bool{true},
			expectCSIOpError:       true,
		},
		{
			name: "non-empty initial cache, check op status running on same instance, csi publish op returns error",
			initialCache: []InstanceOpCacheEntry{
				{
					Key: common.CreateInstanceKey(project, zone, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "attachDisk",
					},
				},
			},
			pubReq: &csi.ControllerPublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			signalCheckOpDoneCount: 1,
			checkOpStatusRunning:   []bool{true},
			expectCSIOpError:       true,
		},
		{
			name: "non-empty initial cache, subset of ops for same instance returns error, csi publish op returns error",
			initialCache: []InstanceOpCacheEntry{
				{
					Key: common.CreateInstanceKey(project, zone, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "attachDisk",
					},
				},
				{
					Key: common.CreateInstanceKey(project, zone, node),
					Op: common.OpInfo{
						Name: "op-2",
						Type: "detachDisk",
					},
				},
			},
			pubReq: &csi.ControllerPublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			checkOpStatusError: []bool{false, true},
			expectCSIOpError:   true,
		},
		{
			name: "non-empty initial cache, subset of ops for same instance still in progress, csi publish op returns error",
			initialCache: []InstanceOpCacheEntry{
				{
					Key: common.CreateInstanceKey(project, zone, node),
					Op: common.OpInfo{
						Name: "op-1",
						Type: "attachDisk",
					},
				},
				{
					Key: common.CreateInstanceKey(project, zone, node),
					Op: common.OpInfo{
						Name: "op-2",
						Type: "detachDisk",
					},
				},
			},
			pubReq: &csi.ControllerPublishVolumeRequest{
				VolumeId: volId1,
				NodeId:   testNodeID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			checkOpStatusRunning: []bool{false, true},
			expectCSIOpError:     true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			readyToExecute := make(chan chan gcecloudprovider.Signal, 1)

			cloudDisks := []*gce.CloudDisk{
				createZonalCloudDisk(disk1),
			}
			fcp, err := gce.CreateFakeCloudProvider(project, zone, cloudDisks)
			if err != nil {
				t.Fatalf("Failed to create fake cloud provider: %v", err)
			}
			fcpBlocking := &gce.FakeBlockingCloudProvider{
				FakeCloudProvider: fcp,
				ReadyToExecute:    readyToExecute,
			}
			instance := &compute.Instance{
				Name:  node,
				Disks: []*compute.AttachedDisk{},
			}
			if tc.unpubReq != nil {
				instance.Disks = append(instance.Disks, &compute.AttachedDisk{DeviceName: disk1})
			}
			fcp.InsertInstance(instance, zone, node)
			gceDriver := initGCEDriverWithCloudProvider(t, fcpBlocking)
			cs := gceDriver.cs
			for _, e := range tc.initialCache {
				cs.opsManager.opsCache.InstanceOps.AddOp(e.Key, e.Op)
			}
			cs.opsManager.ready = true

			runPublishRequest := func(req *csi.ControllerPublishVolumeRequest) <-chan error {
				response := make(chan error)
				go func() {
					_, err := cs.ControllerPublishVolume(context.Background(), req)
					response <- err
				}()
				return response
			}
			runUnpublishRequest := func(req *csi.ControllerUnpublishVolumeRequest) <-chan error {
				response := make(chan error)
				go func() {
					_, err := cs.ControllerUnpublishVolume(context.Background(), req)
					response <- err
				}()
				return response
			}

			var resp <-chan error
			if tc.pubReq != nil {
				resp = runPublishRequest(tc.pubReq)
			} else if tc.unpubReq != nil {
				resp = runUnpublishRequest(tc.unpubReq)
			} else {
				t.Errorf("invalid test case")
			}

			// If a key corresponding to the disk+instance found in the cache, controller will check the op status.
			for _, v := range tc.checkOpStatusError {
				s := gcecloudprovider.Signal{}
				if v {
					s.ReportError = true
				}
				execute := <-readyToExecute
				execute <- s
			}

			// If a key corresponding to the disk+instance found in the cache, controller will check the op status.
			for _, v := range tc.checkOpStatusRunning {
				s := gcecloudprovider.Signal{}
				if v {
					s.ReportRunning = true
				}
				execute := <-readyToExecute
				execute <- s
			}

			if tc.expectAttachDetachOpInCache {
				// Find the running attach/detach op. This may need a few retries, because, at this time the controller publish/unpublish op will the cache and add a new op to cache.
				backoff := wait.Backoff{
					Duration: 10 * time.Millisecond,
					Steps:    100,
				}
				if err := retry.OnError(backoff, func(err error) bool { return true }, func() error {
					ops, innererr := cs.CloudProvider.ListZonalOps(context.Background(), map[string]bool{
						"attachDisk": true, "detachDisk": true})
					if innererr != nil {
						return innererr
					}
					if len(ops) > 0 {
						return nil
					}
					return fmt.Errorf("failed to find ops for fake cloud provider")
				}); err != nil {
					t.Errorf("timed out waiting for attach op to be updated")
					return
				}
				ops, err := cs.CloudProvider.ListZonalOps(context.Background(), map[string]bool{"attachDisk": true})
				if err != nil {
					t.Errorf("Unexpected error in finding ops")
				}
				if len(ops) != 1 {
					t.Errorf("Unexpected number of attach ops in cache")
					return
				}
				// Now the controller has updated the cache and should block at poll
				// verify cache content
				opinfo := common.OpInfo{
					Name: ops[0].Name,
					Type: ops[0].OperationType,
				}
				if !containsDiskInstanceEntry(cs.opsManager.opsCache, DiskInstanceOpCacheEntry{Key: common.CreateDiskInstanceKey(project, zone, disk1, node), Op: opinfo}) {
					t.Errorf("Unexpected cache entry detected")
				}
			}

			// Unblock the poll operation
			if tc.signalPollOp {
				s := gcecloudprovider.Signal{}
				execute := <-readyToExecute
				execute <- s
			}

			err = <-resp
			if tc.expectCSIOpError && err == nil {
				t.Errorf("Expected error found none")
			}
			if !tc.expectCSIOpError && err != nil {
				t.Errorf("Unexpected error found")
			}
		})
	}
}

func TestHydrateCache(t *testing.T) {
	containsOp := func(key common.InstanceKey, op common.OpInfo, cache *common.OpsCache) bool {
		ops := cache.InstanceOps.GetOps(key)
		for _, o := range ops {
			if o.Name == op.Name && o.Type == op.Type {
				return true
			}
		}
		return false
	}
	tests := []struct {
		name                 string
		initialOps           []*compute.Operation
		expectedCacheEntries []InstanceOpCacheEntry
	}{
		{
			name: "no attach detach ops, no entries expected in cache",
			initialOps: []*compute.Operation{
				{
					Name:          "op-1",
					OperationType: "create",
					Status:        "DONE",
				},
				{
					Name:          "op-2",
					OperationType: "create",
					Status:        "DONE",
				},
			},
		},
		{
			name: "done attach detach ops, no entries expected in cache",
			initialOps: []*compute.Operation{
				{
					Name:          "op-1",
					OperationType: "attachDisk",
					Status:        "DONE",
				},
				{
					Name:          "op-2",
					OperationType: "detachDisk",
					Status:        "DONE",
				},
			},
		},
		{
			name: "in progress attach detach ops, entries expected in cache",
			initialOps: []*compute.Operation{
				{
					Name:          "op-1",
					OperationType: "attachDisk",
					Status:        "PENDING",
					TargetLink:    "https://www.googleapis.com/compute/v1/projects/testproject/zones/testzone/instances/testinstance",
				},
				{
					Name:          "op-2",
					OperationType: "detachDisk",
					Status:        "RUNNING",
					TargetLink:    "https://www.googleapis.com/compute/v1beta1/projects/testproject/zones/testzone/instances/testinstance",
				},
			},
			expectedCacheEntries: []InstanceOpCacheEntry{
				{
					Key: "testproject_testzone_testinstance",
					Op: common.OpInfo{
						Name: "op-1",
						Type: "attachDisk",
					},
				},
				{
					Key: "testproject_testzone_testinstance",
					Op: common.OpInfo{
						Name: "op-2",
						Type: "detachDisk",
					},
				},
			},
		},
		{
			name: "in progress attach detach ops for multiple instances, entries expected in cache",
			initialOps: []*compute.Operation{
				{
					Name:          "op-1",
					OperationType: "attachDisk",
					Status:        "PENDING",
					TargetLink:    "https://www.googleapis.com/compute/v1/projects/testproject/zones/testzone/instances/testinstance",
				},
				{
					Name:          "op-2",
					OperationType: "detachDisk",
					Status:        "RUNNING",
					TargetLink:    "https://www.googleapis.com/compute/v1beta1/projects/testproject/zones/testzone/instances/testinstance",
				},
				{
					Name:          "op-3",
					OperationType: "attachDisk",
					Status:        "PENDING",
					TargetLink:    "https://www.googleapis.com/compute/v1/projects/testproject/zones/testzone/instances/testinstance1",
				},
				{
					Name:          "op-4",
					OperationType: "detachDisk",
					Status:        "RUNNING",
					TargetLink:    "https://www.googleapis.com/compute/v1beta1/projects/testproject/zones/testzone/instances/testinstance1",
				},
			},
			expectedCacheEntries: []InstanceOpCacheEntry{
				{
					Key: "testproject_testzone_testinstance",
					Op: common.OpInfo{
						Name: "op-1",
						Type: "attachDisk",
					},
				},
				{
					Key: "testproject_testzone_testinstance",
					Op: common.OpInfo{
						Name: "op-2",
						Type: "detachDisk",
					},
				},
				{
					Key: "testproject_testzone_testinstance1",
					Op: common.OpInfo{
						Name: "op-3",
						Type: "attachDisk",
					},
				},
				{
					Key: "testproject_testzone_testinstance1",
					Op: common.OpInfo{
						Name: "op-4",
						Type: "detachDisk",
					},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fcp, err := gce.CreateFakeCloudProvider(project, zone, nil)
			if err != nil {
				t.Errorf("Failed to create fake cloud provider: %v", err)
			}
			fcp.InsertOps(tc.initialOps)
			gceDriver := initGCEDriverWithCloudProvider(t, fcp)
			gceDriver.cs.opsManager.HydrateOpsCache()
			if !gceDriver.cs.opsManager.IsReady() {
				t.Errorf("failed to initialize cache")
			}
			for _, e := range tc.expectedCacheEntries {
				if !containsOp(e.Key, e.Op, gceDriver.cs.opsManager.opsCache) {
					t.Errorf("expected entry not found")
				}
			}
		})
	}
}

func isUnavailableError(err error) bool {
	if err == nil {
		return false
	}

	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	return st.Code().String() == "Unavailable"
}

func testBackoffHelper(t *testing.T, testControllerPublish bool) {
	readyToExecute := make(chan chan gcecloudprovider.Signal, 1)
	disk1 := name + "1"
	cloudDisks := []*gce.CloudDisk{
		createZonalCloudDisk(disk1),
	}
	fcp, err := gce.CreateFakeCloudProvider(project, zone, cloudDisks)
	if err != nil {
		t.Fatalf("Failed to create fake cloud provider: %v", err)
	}
	fcpBlocking := &gce.FakeBlockingCloudProvider{
		FakeCloudProvider: fcp,
		ReadyToExecute:    readyToExecute,
	}
	instance := &compute.Instance{
		Name:  node,
		Disks: []*compute.AttachedDisk{},
	}
	if !testControllerPublish {
		instance.Disks = append(instance.Disks, &compute.AttachedDisk{DeviceName: disk1})
	}
	fcp.InsertInstance(instance, zone, node)

	driver := GetGCEDriver()
	tc := clock.NewFakeClock(time.Now())
	driver.cs = &GCEControllerServer{
		Driver:        driver,
		CloudProvider: fcpBlocking,
		seen:          map[string]int{},
		volumeLocks:   common.NewVolumeLocks(),
		nodeBackoff:   flowcontrol.NewFakeBackOff(nodeBackoffInitialDuration, nodeBackoffMaxDuration, tc),
		opsManager:    NewOpsManager(fcpBlocking),
	}
	driver.cs.opsManager.ready = true

	key := testNodeID
	step := 1 * time.Millisecond
	// Mock an active backoff condition on the node. This will setup a backoff duration of the 'nodeBackoffInitialDuration'.
	driver.cs.nodeBackoff.Next(key, tc.Now())
	pubreq := &csi.ControllerPublishVolumeRequest{
		VolumeId: testVolumeID + "1",
		NodeId:   testNodeID,
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}
	unpubreq := &csi.ControllerUnpublishVolumeRequest{
		VolumeId: testVolumeID + "1",
		NodeId:   testNodeID,
	}
	// For the first 199 ms, the backoff condition is true. All controller publish request will be denied with 'Unavailable' error code.
	for i := 0; i < 199; i++ {
		tc.Step(step)
		var err error
		if testControllerPublish {
			_, err = driver.cs.ControllerPublishVolume(context.Background(), pubreq)
		} else {
			_, err = driver.cs.ControllerUnpublishVolume(context.Background(), unpubreq)
		}
		if !isUnavailableError(err) {
			t.Errorf("unexpected error %v", err)
		}
	}

	// Mock clock tick for the 200th millisecond. So backoff condition is no longer true.
	tc.Step(step)
	// Mock a ControllerPublish error due to failure to poll for attach disk op.
	runPublishRequest := func(req *csi.ControllerPublishVolumeRequest) <-chan error {
		response := make(chan error)
		go func() {
			_, err := driver.cs.ControllerPublishVolume(context.Background(), req)
			response <- err
		}()
		return response
	}
	runUnpublishRequest := func(req *csi.ControllerUnpublishVolumeRequest) <-chan error {
		response := make(chan error)
		go func() {
			_, err := driver.cs.ControllerUnpublishVolume(context.Background(), req)
			response <- err
		}()
		return response
	}

	var respPublish <-chan error
	var respUnpublish <-chan error
	if testControllerPublish {
		respPublish = runPublishRequest(pubreq)
	} else {
		respUnpublish = runUnpublishRequest(unpubreq)
	}
	execute := <-readyToExecute
	s1 := gcecloudprovider.Signal{ReportTooManyRequestsError: true}
	execute <- s1
	if testControllerPublish {
		if err := <-respPublish; err == nil {
			t.Errorf("expected error")
		}
	} else {
		if err := <-respUnpublish; err == nil {
			t.Errorf("expected error")
		}
	}

	// The above failure should cause driver to call Backoff.Next() again and a backoff duration of 400 ms duration is set starting at the 200th millisecond.
	// For the 200-599 ms, the backoff condition is true, and new controller publish requests will be deined.
	for i := 0; i < 399; i++ {
		tc.Step(step)
		var err error
		if testControllerPublish {
			_, err = driver.cs.ControllerPublishVolume(context.Background(), pubreq)
		} else {
			_, err = driver.cs.ControllerUnpublishVolume(context.Background(), unpubreq)
		}

		if !isUnavailableError(err) {
			t.Errorf("unexpected error %v", err)
		}
	}

	// Mock clock tick for the 600th millisecond. So backoff condition is no longer true.
	tc.Step(step)
	// Now mock a successful ControllerPublish request.
	if testControllerPublish {
		respPublish = runPublishRequest(pubreq)
		if err := <-respPublish; err != nil {
			t.Errorf("unexpected error")
		}
	} else {
		respUnpublish = runUnpublishRequest(unpubreq)
		if err := <-respUnpublish; err != nil {
			t.Errorf("unexpected error")
		}
	}

	// Driver is expected to remove the node key from the backoff map.
	t1 := driver.cs.nodeBackoff.Get(key)
	if t1 != 0 {
		t.Error("unexpected delay")
	}
}

func TestControllerPublishUnpublishBackoff(t *testing.T) {
	testBackoffHelper(t, true /* publish */)
	testBackoffHelper(t, false /* unpublish */)
}
