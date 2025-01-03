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
	"math/rand"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/types/known/timestamppb"

	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog/v2"
	clock "k8s.io/utils/clock/testing"
	"k8s.io/utils/strings/slices"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	gcecloudprovider "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
)

const (
	project                      = "test-project"
	zone                         = "country-region-zone"
	secondZone                   = "country-region-fakesecondzone"
	node                         = "test-node"
	driver                       = "test-driver"
	name                         = "test-name"
	parameterConfidentialCompute = "EnableConfidentialCompute"
	testDiskEncryptionKmsKey     = "projects/KMS_PROJECT_ID/locations/REGION/keyRings/KEY_RING/cryptoKeys/KEY"
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

	testVolumeID           = fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, name)
	underspecifiedVolumeID = fmt.Sprintf("projects/UNSPECIFIED/zones/UNSPECIFIED/disks/%s", name)
	multiZoneVolumeID      = fmt.Sprintf("projects/%s/zones/multi-zone/disks/%s", project, name)

	region, _      = common.GetRegionFromZones([]string{zone})
	testRegionalID = fmt.Sprintf("projects/%s/regions/%s/disks/%s", project, region, name)
	testSnapshotID = fmt.Sprintf("projects/%s/global/snapshots/%s", project, name)
	testImageID    = fmt.Sprintf("projects/%s/global/images/%s", project, name)
	testNodeID     = fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone, node)

	errorBackoffInitialDuration = 200 * time.Millisecond
	errorBackoffMaxDuration     = 5 * time.Minute
	defaultConfidentialStorage  = "false"
)

func TestCreateSnapshotArguments(t *testing.T) {
	thetime, _ := time.Parse(time.RFC3339, gce.Timestamp)
	tp := timestamppb.New(thetime)
	if err := tp.CheckValid(); err != nil {
		t.Fatalf("Unable to conver time to timestamp: %v", err)
	}

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
			name: "success disk image of zonal disk",
			req: &csi.CreateSnapshotRequest{
				Name:           name,
				SourceVolumeId: testVolumeID,
				Parameters:     map[string]string{common.ParameterKeyStorageLocations: " US-WEST2", common.ParameterKeySnapshotType: "images"},
			},
			seedDisks: []*gce.CloudDisk{
				createZonalCloudDisk(name),
			},
			expSnapshot: &csi.Snapshot{
				SnapshotId:     testImageID,
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
		{
			name: "success with resource-tags parameter",
			req: &csi.CreateSnapshotRequest{
				Name:           name,
				SourceVolumeId: testVolumeID,
				Parameters:     map[string]string{"resource-tags": "parent1/key1/value1,parent2/key2/value2"},
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
			name: "fail with malformed resource-tags parameter",
			req: &csi.CreateSnapshotRequest{
				Name:           name,
				SourceVolumeId: testVolumeID,
				Parameters:     map[string]string{"resource-tags": "parent1/key1/value1,parent2/key2/"},
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
			t.Errorf("Err: %v", errStr)
		}
	}
}

func TestUnsupportedMultiZoneCreateSnapshot(t *testing.T) {
	testCase := struct {
		name       string
		req        *csi.CreateSnapshotRequest
		expErrCode codes.Code
	}{
		name: "failed create snapshot for multi-zone PV", // Example values
		req: &csi.CreateSnapshotRequest{
			Name:           name,
			SourceVolumeId: multiZoneVolumeID,
		},
		expErrCode: codes.InvalidArgument,
	}

	t.Logf("test case: %s", testCase.name)

	gceDriver := initGCEDriver(t, nil)
	gceDriver.cs.multiZoneVolumeHandleConfig = MultiZoneVolumeHandleConfig{
		Enable: true,
	}

	// Start Test
	_, err := gceDriver.cs.CreateSnapshot(context.Background(), testCase.req)
	if err != nil {
		serverError, ok := status.FromError(err)
		if !ok {
			t.Fatalf("Could not get error status code from err: %v", serverError)
		}
		if serverError.Code() != testCase.expErrCode {
			t.Fatalf("Expected error code: %v, got: %v. err : %v", testCase.expErrCode, serverError.Code(), err)
		}
	} else {
		t.Fatalf("Expected error: %v, got no error", testCase.expErrCode)
	}
}

func TestUnsupportedMultiZoneControllerExpandVolume(t *testing.T) {
	testCase := struct {
		name       string
		req        *csi.ControllerExpandVolumeRequest
		expErrCode codes.Code
	}{
		name: "failed create snapshot for multi-zone PV", // Example values
		req: &csi.ControllerExpandVolumeRequest{
			VolumeId: multiZoneVolumeID,
		},
		expErrCode: codes.InvalidArgument,
	}

	t.Logf("test case: %s", testCase.name)

	gceDriver := initGCEDriver(t, nil)
	gceDriver.cs.multiZoneVolumeHandleConfig = MultiZoneVolumeHandleConfig{
		Enable: true,
	}

	// Start Test
	_, err := gceDriver.cs.ControllerExpandVolume(context.Background(), testCase.req)
	if err != nil {
		serverError, ok := status.FromError(err)
		if !ok {
			t.Fatalf("Could not get error status code from err: %v", serverError)
		}
		if serverError.Code() != testCase.expErrCode {
			t.Fatalf("Expected error code: %v, got: %v. err : %v", testCase.expErrCode, serverError.Code(), err)
		}
	} else {
		t.Fatalf("Expected error: %v, got no error", testCase.expErrCode)
	}
}

func TestDeleteSnapshot(t *testing.T) {
	testCases := []struct {
		name       string
		req        *csi.DeleteSnapshotRequest
		expErrCode codes.Code
	}{
		{
			name: "valid snapshot delete",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: testSnapshotID,
			},
		},
		{
			name: "valid image delete",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: testImageID,
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
	}
}

func TestListSnapshotsArguments(t *testing.T) {
	testCases := []struct {
		name          string
		req           *csi.ListSnapshotsRequest
		numSnapshots  int
		numImages     int
		expectedCount int
		expErrCode    codes.Code
	}{
		{
			name: "valid",
			req: &csi.ListSnapshotsRequest{
				SnapshotId: testSnapshotID + "0",
			},
			numSnapshots:  3,
			numImages:     2,
			expectedCount: 1,
		},
		{
			name: "valid image",
			req: &csi.ListSnapshotsRequest{
				SnapshotId: testImageID + "0",
			},
			numSnapshots:  3,
			numImages:     2,
			expectedCount: 1,
		},
		{
			name: "invalid id",
			req: &csi.ListSnapshotsRequest{
				SnapshotId: testSnapshotID + "/foo",
			},
			expectedCount: 0,
		},
		{
			name: "invalid image id",
			req: &csi.ListSnapshotsRequest{
				SnapshotId: testImageID + "/foo",
			},
			expectedCount: 0,
		},
		{
			name: "invalid snapshot name",
			req: &csi.ListSnapshotsRequest{
				SnapshotId: testSnapshotID + "-invalid-snapshot-",
			},
			expectedCount: 0,
			expErrCode:    codes.InvalidArgument,
		},
		{
			name: "invalid image name",
			req: &csi.ListSnapshotsRequest{
				SnapshotId: testImageID + "-invalid-image-",
			},
			expectedCount: 0,
			expErrCode:    codes.InvalidArgument,
		},
		{
			name: "no id",
			req: &csi.ListSnapshotsRequest{
				SnapshotId: "",
			},
			numSnapshots:  2,
			numImages:     3,
			expectedCount: 5,
		},
		{
			name: "with invalid token",
			req: &csi.ListSnapshotsRequest{
				StartingToken: "invalid",
			},
			expectedCount: 0,
			expErrCode:    codes.Aborted,
		},
		{
			name: "negative entries",
			req: &csi.ListSnapshotsRequest{
				MaxEntries: -1,
			},

			expErrCode: codes.InvalidArgument,
		},
		{
			name: "max enries",
			req: &csi.ListSnapshotsRequest{
				MaxEntries: 4,
			},
			numSnapshots:  2,
			numImages:     3,
			expectedCount: 4,
		},
	}

	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)

		disks := []*gce.CloudDisk{}
		for i := 0; i < tc.numSnapshots+tc.numImages; i++ {
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
				Parameters:     map[string]string{common.ParameterKeySnapshotType: common.DiskSnapshotType},
			}
			_, err := gceDriver.cs.CreateSnapshot(context.Background(), createReq)
			if err != nil {
				t.Errorf("error %v", err)
			}
		}

		for i := 0; i < tc.numImages; i++ {
			volumeID := fmt.Sprintf("%s%d", testVolumeID, i)
			nameID := fmt.Sprintf("%s%d", name, i)
			createReq := &csi.CreateSnapshotRequest{
				Name:           nameID,
				SourceVolumeId: volumeID,
				Parameters:     map[string]string{common.ParameterKeySnapshotType: common.DiskImageType},
			}
			_, err := gceDriver.cs.CreateSnapshot(context.Background(), createReq)
			if err != nil {
				t.Errorf("error %v", err)
			}
		}

		// Start Test
		resp, err := gceDriver.cs.ListSnapshots(context.Background(), tc.req)
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
		if (snapshots == nil || len(snapshots) == 0) && tc.numSnapshots == 0 {
			continue
		}

		if snapshots == nil || len(snapshots) == 0 {
			// If one is nil or empty but not both
			t.Fatalf("Expected snapshots number %v, got no snapshot", tc.numSnapshots)
		}
		if len(snapshots) != tc.expectedCount {
			errStr := fmt.Sprintf("Expected snapshot number to equal: %v", tc.numSnapshots)
			t.Errorf("Err: %v", errStr)
		}
	}
}

func TestCreateVolumeArguments(t *testing.T) {
	testCases := []struct {
		name               string
		req                *csi.CreateVolumeRequest
		enableStoragePools bool
		expVol             *csi.Volume
		expErrCode         codes.Code
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
		{
			name: "success with provisionedIops parameter",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         map[string]string{"labels": "key1=value1,key2=value2", "provisioned-iops-on-create": "10000"},
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				VolumeId:           testVolumeID,
				VolumeContext:      nil,
				AccessibleTopology: stdTopology,
			},
		},
		{
			name: "fail with malformed provisionedIops parameter",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         map[string]string{"labels": "key1=value1,key2=value2", "provisioned-iops-on-create": "dsfo3"},
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "success with provisionedThroughput parameter",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         map[string]string{"labels": "key1=value1,key2=value2", "provisioned-throughput-on-create": "10000"},
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				VolumeId:           testVolumeID,
				VolumeContext:      nil,
				AccessibleTopology: stdTopology,
			},
		},
		{
			name: "fail with malformed provisionedThroughput parameter",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         map[string]string{"labels": "key1=value1,key2=value2", "provisioned-throughput-on-create": "dsfo3"},
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "success with storage pools parameter",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         map[string]string{"storage-pools": "projects/test-project/zones/us-central1-a/storagePools/storagePool-1", "type": "hyperdisk-balanced"},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			enableStoragePools: true,
			expVol: &csi.Volume{
				CapacityBytes: common.GbToBytes(20),
				VolumeId:      "projects/test-project/zones/us-central1-a/disks/test-name",
				VolumeContext: nil,
				AccessibleTopology: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
				},
			},
		},
		{
			name: "fail with storage pools parameter, enableStoragePools is false",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         map[string]string{"storage-pools": "projects/test-project/zones/us-central1-a/storagePools/storagePool-1", "type": "hyperdisk-balanced"},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			enableStoragePools: false,
			expErrCode:         codes.InvalidArgument,
		},
		{
			name: "fail with invalid storage pools parameter",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         map[string]string{"storage-pools": "zones/us-central1-a/storagePools/storagePool-1", "type": "hyperdisk-balanced"},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			enableStoragePools: true,
			expErrCode:         codes.InvalidArgument,
		},
		{
			name: "success with resource-tags parameter",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         map[string]string{"resource-tags": "parent1/key1/value1,parent2/key2/value2"},
			},
			expVol: &csi.Volume{
				CapacityBytes:      common.GbToBytes(20),
				VolumeId:           testVolumeID,
				VolumeContext:      nil,
				AccessibleTopology: stdTopology,
			},
		},
		{
			name: "fail with malformed resource-tags parameter",
			req: &csi.CreateVolumeRequest{
				Name:               name,
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters:         map[string]string{"resource-tags": "parent1/key1/value1,parent2/key2/"},
			},
			expErrCode: codes.InvalidArgument,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		gceDriver := initGCEDriver(t, nil)
		gceDriver.cs.enableStoragePools = tc.enableStoragePools
		// Start Test
		resp, err := gceDriver.cs.CreateVolume(context.Background(), tc.req)
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
			t.Errorf("Err: %v", errStr)
		}
	}
}

func TestMultiZoneVolumeCreation(t *testing.T) {
	testCases := []struct {
		name               string
		req                *csi.CreateVolumeRequest
		enableStoragePools bool
		fallbackZones      []string
		expZones           []string
		expErrCode         codes.Code
	}{
		{
			name: "success single ROX multi-zone disk",
			req: &csi.CreateVolumeRequest{
				Name:          "test-name",
				CapacityRange: stdCapRange,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
						},
					},
				},
				Parameters: map[string]string{
					common.ParameterKeyType:                        "hyperdisk-ml",
					common.ParameterKeyEnableMultiZoneProvisioning: "true",
				},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: testSnapshotID,
						},
					},
				},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			expZones: []string{"us-central1-a"},
		},
		{
			name: "single ROX multi-zone disk empty topology fallback zones",
			req: &csi.CreateVolumeRequest{
				Name:          "test-name",
				CapacityRange: stdCapRange,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
						},
					},
				},
				Parameters: map[string]string{
					common.ParameterKeyType:                        "hyperdisk-ml",
					common.ParameterKeyEnableMultiZoneProvisioning: "true",
				},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: testSnapshotID,
						},
					},
				},
				AccessibilityRequirements: &csi.TopologyRequirement{},
			},
			fallbackZones: []string{zone, secondZone},
			expZones:      []string{zone, secondZone},
		},
		{
			name: "success triple ROX multi-zone disk",
			req: &csi.CreateVolumeRequest{
				Name:          "test-name",
				CapacityRange: stdCapRange,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
						},
					},
				},
				Parameters: map[string]string{
					common.ParameterKeyType:                        "hyperdisk-ml",
					common.ParameterKeyEnableMultiZoneProvisioning: "true",
				},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: testSnapshotID,
						},
					},
				},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
						},
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
						},
					},
				},
			},
			expZones: []string{"us-central1-a", "us-central1-b", "us-central1-c"},
		},
		{
			name: "success triple rwo multi-zone disk",
			req: &csi.CreateVolumeRequest{
				Name:          "test-name",
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
				},
				Parameters: map[string]string{
					common.ParameterKeyType:                        "hyperdisk-ml",
					common.ParameterKeyEnableMultiZoneProvisioning: "true",
				},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
						},
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
						},
					},
				},
			},
			expZones: []string{"us-central1-a", "us-central1-b", "us-central1-c"},
		},
		{
			name: "err single ROX multi-zone no topology",
			req: &csi.CreateVolumeRequest{
				Name:          "test-name",
				CapacityRange: stdCapRange,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
						},
					},
				},
				Parameters: map[string]string{
					common.ParameterKeyType:                        "hyperdisk-ml",
					common.ParameterKeyEnableMultiZoneProvisioning: "true",
				},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: testSnapshotID,
						},
					},
				},
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "err rwo access mode",
			req: &csi.CreateVolumeRequest{
				Name:          "test-name",
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
				},
				Parameters: map[string]string{
					common.ParameterKeyType:                        "hyperdisk-ml",
					common.ParameterKeyEnableMultiZoneProvisioning: "true",
				},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: testSnapshotID,
						},
					},
				},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "err no content source",
			req: &csi.CreateVolumeRequest{
				Name:          "test-name",
				CapacityRange: stdCapRange,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
						},
					},
				},
				Parameters: map[string]string{
					common.ParameterKeyType:                        "hyperdisk-ml",
					common.ParameterKeyEnableMultiZoneProvisioning: "true",
				},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "err cloning not supported",
			req: &csi.CreateVolumeRequest{
				Name:          "test-name",
				CapacityRange: stdCapRange,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
						},
					},
				},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: testVolumeID,
						},
					},
				},
				Parameters: map[string]string{
					common.ParameterKeyType:                        "hyperdisk-ml",
					common.ParameterKeyEnableMultiZoneProvisioning: "true",
				},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			expErrCode: codes.InvalidArgument,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		fcp, err := gce.CreateFakeCloudProvider(project, zone, nil)
		if err != nil {
			t.Fatalf("Failed to create fake cloud provider: %v", err)
		}
		// Setup new driver each time so no interference
		gceDriver := initGCEDriverWithCloudProvider(t, fcp)
		gceDriver.cs.multiZoneVolumeHandleConfig.DiskTypes = []string{"hyperdisk-ml"}
		gceDriver.cs.multiZoneVolumeHandleConfig.Enable = true
		gceDriver.cs.fallbackRequisiteZones = tc.fallbackZones

		if tc.req.VolumeContentSource.GetType() != nil {
			snapshotParams, err := common.ExtractAndDefaultSnapshotParameters(nil, gceDriver.name, nil)
			if err != nil {
				t.Errorf("Got error extracting snapshot parameters: %v", err)
			}
			if snapshotParams.SnapshotType == common.DiskSnapshotType {
				fcp.CreateSnapshot(context.Background(), project, meta.ZonalKey(name, common.MultiZoneValue), name, snapshotParams)
			} else {
				t.Fatalf("No volume source mentioned in snapshot parameters %v", snapshotParams)
			}
		}

		// Start Test
		resp, err := gceDriver.cs.CreateVolume(context.Background(), tc.req)
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

		topologies := make([]*csi.Topology, 0, len(tc.expZones))
		for _, zone := range tc.expZones {
			topologies = append(topologies, &csi.Topology{
				Segments: map[string]string{common.TopologyKeyZone: zone},
			})
		}

		expVol := &csi.Volume{
			CapacityBytes:      common.GbToBytes(20),
			VolumeId:           fmt.Sprintf("projects/%s/zones/multi-zone/disks/%s", project, name),
			VolumeContext:      nil,
			AccessibleTopology: topologies,
			ContentSource:      tc.req.VolumeContentSource,
		}

		// Make sure responses match
		vol := resp.GetVolume()
		if vol == nil {
			// If one is nil but not both
			t.Fatalf("Expected volume %v, got nil volume", expVol)
		}

		klog.Warningf("Got accessible topology: %v", vol.GetAccessibleTopology())

		sortTopologies := func(t1, t2 *csi.Topology) bool {
			return t1.Segments[common.TopologyKeyZone] < t2.Segments[common.TopologyKeyZone]
		}

		// Custom comparers to compare two volumes
		contentSourceComparer := cmp.Comparer(func(a, b *csi.VolumeContentSource) bool {
			if a == nil {
				return b == nil
			}
			if b == nil {
				return false
			}
			if vcsA, ok := a.Type.(*csi.VolumeContentSource_Snapshot); ok {
				if vcsB, valid := b.Type.(*csi.VolumeContentSource_Snapshot); valid {
					return vcsA.Snapshot.SnapshotId == vcsB.Snapshot.SnapshotId
				}
				return false
			}
			if vcsA, ok := a.Type.(*csi.VolumeContentSource_Volume); ok {
				if vcsB, valid := b.Type.(*csi.VolumeContentSource_Volume); valid {
					return vcsA.Volume.VolumeId == vcsB.Volume.VolumeId
				}
				return false
			}
			return false
		})
		topComparer := cmp.Comparer(func(a, b *csi.Topology) bool {
			return cmp.Diff(a.Segments, b.Segments) == ""
		})
		volComparer := cmp.Comparer(func(a, b *csi.Volume) bool {
			if a == nil {
				return b == nil
			}
			if b == nil {
				return false
			}
			topEqual := cmp.Diff(a.AccessibleTopology, b.AccessibleTopology, cmpopts.SortSlices(sortTopologies), topComparer) == ""
			vcEqual := cmp.Diff(a.VolumeContext, b.VolumeContext) == ""
			csEqual := cmp.Diff(a.ContentSource, b.ContentSource, contentSourceComparer) == ""
			return a.CapacityBytes == b.CapacityBytes && a.VolumeId == b.VolumeId && vcEqual && topEqual && csEqual
		})
		if diff := cmp.Diff(expVol, vol, volComparer); diff != "" {
			t.Errorf("Accessible topologies mismatch (-want +got):\n%s", diff)
		}

		for _, zone := range tc.expZones {
			volumeKey := meta.ZonalKey(name, zone)
			disk, err := fcp.GetDisk(context.Background(), project, volumeKey, gce.GCEAPIVersionBeta)
			if err != nil {
				t.Fatalf("Get Disk failed for created disk with error: %v", err)
			}
			if disk.GetLabels()[common.MultiZoneLabel] != "true" {
				t.Fatalf("Expect %s disk to have %s label, got: %v", volumeKey, common.MultiZoneLabel, disk.GetLabels())
			}
		}
	}
}

type FakeCloudProviderInsertDiskErr struct {
	*gce.FakeCloudProvider
	insertDiskErrors map[string]error
}

func NewFakeCloudProviderInsertDiskErr(project, zone string) (*FakeCloudProviderInsertDiskErr, error) {
	provider, err := gce.CreateFakeCloudProvider(project, zone, nil)
	if err != nil {
		return nil, err
	}
	return &FakeCloudProviderInsertDiskErr{
		FakeCloudProvider: provider,
		insertDiskErrors:  map[string]error{},
	}, nil
}

func (cloud *FakeCloudProviderInsertDiskErr) AddDiskForErr(volKey *meta.Key, err error) {
	cloud.insertDiskErrors[volKey.String()] = err
}

func (cloud *FakeCloudProviderInsertDiskErr) InsertDisk(ctx context.Context, project string, volKey *meta.Key, params common.DiskParameters, capBytes int64, capacityRange *csi.CapacityRange, replicaZones []string, snapshotID string, volumeContentSourceVolumeID string, multiWriter bool, accessMode string) error {
	if err, ok := cloud.insertDiskErrors[volKey.String()]; ok {
		return err
	}

	return cloud.FakeCloudProvider.InsertDisk(ctx, project, volKey, params, capBytes, capacityRange, replicaZones, snapshotID, volumeContentSourceVolumeID, multiWriter, accessMode)
}

func TestMultiZoneVolumeCreationErrHandling(t *testing.T) {
	testCases := []struct {
		name           string
		req            *csi.CreateVolumeRequest
		insertDiskErrs map[*meta.Key]error
		expErrCode     codes.Code
		wantDisks      []*meta.Key
	}{
		{
			name: "ResourceExhausted errors",
			req: &csi.CreateVolumeRequest{
				Name:          "test-name",
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
				},
				Parameters: map[string]string{
					common.ParameterKeyType:                        "hyperdisk-ml",
					common.ParameterKeyEnableMultiZoneProvisioning: "true",
				},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
						},
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			insertDiskErrs: map[*meta.Key]error{
				meta.ZonalKey(name, "us-central1-b"): &googleapi.Error{Code: http.StatusTooManyRequests, Message: "Resource Exhausted"},
			},
			expErrCode: codes.ResourceExhausted,
			wantDisks: []*meta.Key{
				meta.ZonalKey(name, "us-central1-a"),
				meta.ZonalKey(name, "us-central1-c"),
			},
		},
		{
			name: "Unavailable errors",
			req: &csi.CreateVolumeRequest{
				Name:          "test-name",
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
				},
				Parameters: map[string]string{
					common.ParameterKeyType:                        "hyperdisk-ml",
					common.ParameterKeyEnableMultiZoneProvisioning: "true",
				},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
						},
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			insertDiskErrs: map[*meta.Key]error{
				meta.ZonalKey(name, "us-central1-b"): &googleapi.Error{Code: http.StatusGatewayTimeout, Message: "connection reset by peer"},
				meta.ZonalKey(name, "us-central1-c"): &googleapi.Error{Code: http.StatusTooManyRequests, Message: "Resource Exhausted"},
			},
			expErrCode: codes.Unavailable,
			wantDisks: []*meta.Key{
				meta.ZonalKey(name, "us-central1-a"),
			},
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		fcp, err := NewFakeCloudProviderInsertDiskErr(project, zone)
		if err != nil {
			t.Fatalf("Failed to create fake cloud provider: %v", err)
		}
		// Setup new driver each time so no interference
		gceDriver := initGCEDriverWithCloudProvider(t, fcp)
		gceDriver.cs.multiZoneVolumeHandleConfig.DiskTypes = []string{"hyperdisk-ml"}
		gceDriver.cs.multiZoneVolumeHandleConfig.Enable = true

		for volKey, err := range tc.insertDiskErrs {
			fcp.AddDiskForErr(volKey, err)
		}

		// Start Test
		_, err = gceDriver.cs.CreateVolume(context.Background(), tc.req)

		if err == nil {
			t.Errorf("Expected error: %v, got no error", tc.expErrCode)
		}

		serverError, ok := status.FromError(err)
		if !ok {
			t.Errorf("Could not get error status code from err: %v", serverError)
		}
		if serverError.Code() != tc.expErrCode {
			t.Errorf("Expected error code: %v, got: %v. err : %v", tc.expErrCode, serverError.Code(), err)
		}

		for _, volKey := range tc.wantDisks {
			disk, err := fcp.GetDisk(context.Background(), project, volKey, gce.GCEAPIVersionV1)
			if err != nil {
				t.Errorf("Unexpected err fetching disk %v: %v", volKey, err)
			}
			if disk == nil {
				t.Errorf("Expected disk for %v but got nil", volKey)
			}
		}
	}
}

func TestCreateVolumeWithVolumeAttributeClassParameters(t *testing.T) {
	// When volume attribute class specifies iops / throughput they should take precedence over storage class parameters
	testCases := []struct {
		name          string
		req           *csi.CreateVolumeRequest
		expIops       int64
		expThroughput int64
		wantErr       bool
		expErrCode    codes.Code
	}{
		{
			name: "VolumeAttributesClass parameters should take precedence over storage class parameters",
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
				},
				Parameters: map[string]string{
					common.ParameterKeyType:                          "hyperdisk-balanced",
					common.ParameterKeyProvisionedIOPSOnCreate:       "10000",
					common.ParameterKeyProvisionedThroughputOnCreate: "500Mi",
				},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
				MutableParameters: map[string]string{"iops": "20000", "throughput": "600Mi"},
			},
			expIops:       20000,
			expThroughput: 600,
			wantErr:       false,
		},
		{
			name: "VolumeAttributesClass parameters should throw an error for incompatible disk types",
			req: &csi.CreateVolumeRequest{
				Name:          "pd-ssd-vol",
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
				},
				Parameters: map[string]string{
					common.ParameterKeyType:                          "pd-ssd",
					common.ParameterKeyProvisionedIOPSOnCreate:       "10000",
					common.ParameterKeyProvisionedThroughputOnCreate: "500Mi",
				},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
				MutableParameters: map[string]string{"iops": "20000", "throughput": "600Mi"},
			},
			expIops:       0,
			expThroughput: 0,
			wantErr:       true,
			expErrCode:    codes.InvalidArgument,
		},
	}

	for _, tc := range testCases {
		var d []*gce.CloudDisk
		fcp, err := gce.CreateFakeCloudProvider(project, zone, d)
		gceDriver := initGCEDriverWithCloudProvider(t, fcp)

		if err != nil {
			t.Fatalf("Failed to create fake cloud provider: %v", err)
		}

		createVolReq := tc.req

		resp, err := gceDriver.cs.CreateVolume(context.Background(), createVolReq)
		if tc.wantErr {
			if status.Code(err) != tc.expErrCode {
				t.Fatalf("Expected error code: %v, got: %v. err : %v", tc.expErrCode, status.Code(err), err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Failed to create volume: %v", err)
		}

		volumeId := resp.GetVolume().VolumeId
		project, volumeKey, err := common.VolumeIDToKey(volumeId)
		if err != nil {
			t.Fatalf("Failed to convert volume id to key: %v", err)
		}

		disk, err := fcp.GetDisk(context.Background(), project, volumeKey, gce.GCEAPIVersionBeta)

		if err != nil {
			t.Fatalf("Failed to get disk: %v", err)
		}
		if disk != nil {
			if disk.GetProvisionedIops() != tc.expIops {
				t.Errorf("Expected IOPS to be %d, got: %v", tc.expIops, disk.GetProvisionedIops())
			}
			if disk.GetProvisionedThroughput() != tc.expThroughput {
				t.Errorf("Expected Throughput to be %d, got: %v", tc.expThroughput, disk.GetProvisionedThroughput())
			}
		}
	}

}

func TestVolumeModifyOperation(t *testing.T) {
	testCases := []struct {
		name          string
		req           *csi.ControllerModifyVolumeRequest
		diskType      string
		params        *common.DiskParameters
		expIops       int64
		expThroughput int64
		expErrMessage string
	}{
		{
			name: "Update volume with valid parameters",
			req: &csi.ControllerModifyVolumeRequest{
				VolumeId:          testVolumeID,
				MutableParameters: map[string]string{"iops": "20000", "throughput": "600Mi"},
			},
			diskType: "hyperdisk-balanced",
			params: &common.DiskParameters{
				DiskType:                      "hyperdisk-balanced",
				ProvisionedIOPSOnCreate:       10000,
				ProvisionedThroughputOnCreate: 500,
			},
			expIops:       20000,
			expThroughput: 600,
			expErrMessage: "",
		},
		{
			name: "Update volume with invalid parameters",
			req: &csi.ControllerModifyVolumeRequest{
				VolumeId:          testVolumeID,
				MutableParameters: map[string]string{"iops": "0", "throughput": "0Mi"},
			},
			diskType: "hyperdisk-balanced",
			params: &common.DiskParameters{
				DiskType:                      "hyperdisk-balanced",
				ProvisionedIOPSOnCreate:       10000,
				ProvisionedThroughputOnCreate: 500,
			},
			expIops:       10000,
			expThroughput: 500,
			expErrMessage: "no IOPS or Throughput or SizeGb specified for disk",
		},
		{
			name: "Modify volume with unsupported sizeGb",
			req: &csi.ControllerModifyVolumeRequest{
				VolumeId:          testVolumeID,
				MutableParameters: map[string]string{"sizeGb": "800"},
			},
			diskType: "hyperdisk-balanced",
			params: &common.DiskParameters{
				DiskType:                      "hyperdisk-balanced",
				ProvisionedIOPSOnCreate:       10000,
				ProvisionedThroughputOnCreate: 500,
			},
			expIops:       10000,
			expThroughput: 500,
			expErrMessage: "parameters contain unknown parameter: sizeGb",
		},
		{
			name: "Update volume with valid parameters but invalid disk type",
			req: &csi.ControllerModifyVolumeRequest{
				VolumeId:          testVolumeID,
				MutableParameters: map[string]string{"iops": "20000", "throughput": "600Mi"},
			},
			diskType: "pd-ssd",
			params: &common.DiskParameters{
				DiskType: "pd-ssd",
			},
			expIops:       0,
			expThroughput: 0,
			expErrMessage: fmt.Sprintf("modifications not supported for disk type %s", "pd-ssd"),
		},
	}

	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Arrange
		fcp, err := gce.CreateFakeCloudProvider(project, zone, nil)

		if err != nil {
			t.Fatalf("Failed to create mock cloud provider: %v", err)
		}

		gceDriver := initGCEDriverWithCloudProvider(t, fcp)
		project, volKey, err := common.VolumeIDToKey(testVolumeID)
		if err != nil {
			t.Fatalf("Failed convert key: %v", err)
		}

		err = fcp.InsertDisk(context.Background(), project, volKey, *tc.params, 200000, nil, nil, "", "", false, "")
		if err != nil {
			t.Fatalf("Failed to insert disk: %v", err)
		}
		// Act
		_, err = gceDriver.cs.ControllerModifyVolume(context.Background(), tc.req)

		// Assert
		if err != nil {
			msg := err.Error()
			if !strings.ContainsAny(msg, tc.expErrMessage) {
				t.Errorf("Failed to modify volume: %v", err)
			}
		}

		modifiedVol, err := fcp.GetDisk(context.Background(), project, volKey, gce.GCEAPIVersionBeta)

		if err != nil {
			t.Errorf("Failed to get volume: %v", err)
		}

		diskIops := modifiedVol.GetProvisionedIops()
		throughput := modifiedVol.GetProvisionedThroughput()

		if diskIops != tc.expIops && throughput != tc.expThroughput {
			t.Errorf("Failed to modify volume: %v", err)
		}
	}
}

type FakeCloudProviderUpdateDiskErr struct {
	*gce.FakeCloudProvider
	updateDiskErrors map[string]error
}

func NewFakeCloudProviderUpdateDiskErr(project, zone string) (*FakeCloudProviderUpdateDiskErr, error) {
	provider, err := gce.CreateFakeCloudProvider(project, zone, nil)
	if err != nil {
		return nil, err
	}
	return &FakeCloudProviderUpdateDiskErr{
		FakeCloudProvider: provider,
		updateDiskErrors:  map[string]error{},
	}, nil
}

func (cloud *FakeCloudProviderUpdateDiskErr) AddDiskForErr(volKey *meta.Key, err error) {
	cloud.updateDiskErrors[volKey.String()] = err
}

func (cloud *FakeCloudProviderUpdateDiskErr) UpdateDisk(ctx context.Context, project string, volKey *meta.Key, existingDisk *gcecloudprovider.CloudDisk, params common.ModifyVolumeParameters) error {
	if err, ok := cloud.updateDiskErrors[volKey.String()]; ok {
		return err
	}

	return cloud.FakeCloudProvider.UpdateDisk(ctx, project, volKey, existingDisk, params)
}

type modifyVolumeErrorTest struct {
	expErrCode int
	wantReason bool
	reason     string
}

func TestVolumeModifyErrorHandling(t *testing.T) {
	testCases := []struct {
		name               string
		modifyVolumeErrors map[*meta.Key]error
		createReq          *csi.CreateVolumeRequest
		modifyReq          *csi.ControllerModifyVolumeRequest
		expErr             *modifyVolumeErrorTest
	}{
		{
			name:      "disk notFound errors",
			modifyReq: &csi.ControllerModifyVolumeRequest{},
			expErr: &modifyVolumeErrorTest{
				wantReason: true,
				reason:     "notFound",
			},
		},
		{
			name: "Too Many Requests errors",
			createReq: &csi.CreateVolumeRequest{
				Name: name,
				Parameters: map[string]string{
					common.ParameterKeyType:                          "hyperdisk-balanced",
					common.ParameterKeyProvisionedIOPSOnCreate:       "3000",
					common.ParameterKeyProvisionedThroughputOnCreate: "150Mi",
				},
				VolumeCapabilities: stdVolCaps,
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			modifyReq: &csi.ControllerModifyVolumeRequest{
				MutableParameters: map[string]string{"iops": "3001", "throughput": "151Mi"},
			},
			modifyVolumeErrors: map[*meta.Key]error{
				meta.ZonalKey(name, "us-central1-a"): &googleapi.Error{
					Code:    http.StatusTooManyRequests,
					Message: "too many IOPS/Throughput modifications in a 6 hour window",
				},
			},
			expErr: &modifyVolumeErrorTest{
				expErrCode: http.StatusTooManyRequests,
			},
		},
		{
			name: "InvalidArgument errors",
			createReq: &csi.CreateVolumeRequest{
				Name: name,
				Parameters: map[string]string{
					common.ParameterKeyType:                          "hyperdisk-balanced",
					common.ParameterKeyProvisionedIOPSOnCreate:       "3000",
					common.ParameterKeyProvisionedThroughputOnCreate: "150Mi",
				},
				VolumeCapabilities: stdVolCaps,
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			modifyReq: &csi.ControllerModifyVolumeRequest{
				MutableParameters: map[string]string{"iops": "10000", "throughput": "2400Mi"},
			},
			modifyVolumeErrors: map[*meta.Key]error{
				meta.ZonalKey(name, "us-central1-a"): &googleapi.Error{Code: int(codes.InvalidArgument), Message: "InvalidArgument"},
			},
			expErr: &modifyVolumeErrorTest{
				expErrCode: int(codes.InvalidArgument),
			},
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		fcp, err := NewFakeCloudProviderUpdateDiskErr(project, zone)
		if err != nil {
			t.Fatalf("Failed to create mock cloud provider")
		}
		gceDriver := initGCEDriverWithCloudProvider(t, fcp)

		for volKey, err := range tc.modifyVolumeErrors {
			fcp.AddDiskForErr(volKey, err)
		}

		volId := testVolumeID
		if tc.createReq != nil {
			fmt.Printf("Creating volume")
			resp, err := gceDriver.cs.CreateVolume(context.Background(), tc.createReq)
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
			volId = resp.GetVolume().VolumeId
		}

		tc.modifyReq.VolumeId = volId
		_, err = gceDriver.cs.ControllerModifyVolume(context.Background(), tc.modifyReq)
		if err == nil {
			t.Errorf("Expected err: %v, got no error", tc.expErr.expErrCode)
		}

		var e *googleapi.Error
		if ok := errors.As(err, &e); ok {
			if e.Code != tc.expErr.expErrCode {
				t.Errorf("Expected error: %v, got: %v", tc.expErr.expErrCode, e.Code)
			}
			if tc.expErr.wantReason && !googleapiErrContainsReason(e, tc.expErr.reason) {
				t.Errorf("Expected error to contain reason %s", tc.expErr.reason)
			}
		} else {
			t.Errorf("Expected error %v to be a googleapi error", err)
		}
	}
}

func TestListVolumePagination(t *testing.T) {
	testCases := []struct {
		name            string
		diskCount       int
		maxEntries      int32
		expectedEntries []int
	}{
		{
			name:            "no pagination (implicit)",
			diskCount:       325,
			expectedEntries: []int{325},
		},
		{
			name:            "no pagination (explicit)",
			diskCount:       2500,
			maxEntries:      2500,
			expectedEntries: []int{2500},
		},
		{
			name:            "pagination (implicit)",
			diskCount:       1327,
			expectedEntries: []int{500, 500, 327},
		},
		{
			name:            "pagination (explicit)",
			diskCount:       723,
			maxEntries:      200,
			expectedEntries: []int{200, 200, 200, 123},
		},
		{
			name:            "pagination (explicit)",
			diskCount:       3253,
			maxEntries:      1000,
			expectedEntries: []int{1000, 1000, 1000, 253},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup new driver each time so no interference
			var d []*gce.CloudDisk
			for i := 0; i < tc.diskCount; i++ {
				// Create diskCount dummy disks
				name := fmt.Sprintf("disk-%v", i)
				d = append(d, gce.CloudDiskFromV1(&compute.Disk{
					Name:     name,
					SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/project/zones/zone/disk/%s", name),
				}))
			}
			gceDriver := initGCEDriver(t, d)
			tok := ""
			for i, expectedEntry := range tc.expectedEntries {
				lvr := &csi.ListVolumesRequest{
					MaxEntries:    tc.maxEntries,
					StartingToken: tok,
				}
				resp, err := gceDriver.cs.ListVolumes(context.TODO(), lvr)
				if err != nil {
					t.Fatalf("Got error %v", err)
					return
				}

				if len(resp.Entries) != expectedEntry {
					t.Fatalf("Got %v entries, expected %v on call # %d", len(resp.Entries), expectedEntry, i+1)
				}

				tok = resp.NextToken
			}
			if len(tok) != 0 {
				t.Fatalf("Expected no more entries, but got NextToken in response: %s", tok)
			}
		})
	}
}

func TestListAttachedVolumePagination(t *testing.T) {
	testCases := []struct {
		name            string
		diskCount       int
		maxEntries      int32
		expectedEntries []int
	}{
		{
			name:            "no pagination (implicit)",
			diskCount:       325,
			expectedEntries: []int{325},
		},
		{
			name:            "no pagination (explicit)",
			diskCount:       2500,
			maxEntries:      2500,
			expectedEntries: []int{2500},
		},
		{
			name:            "pagination (implicit)",
			diskCount:       1327,
			expectedEntries: []int{500, 500, 327},
		},
		{
			name:            "pagination (explicit)",
			diskCount:       723,
			maxEntries:      200,
			expectedEntries: []int{200, 200, 200, 123},
		},
		{
			name:            "pagination (explicit)",
			diskCount:       3253,
			maxEntries:      1000,
			expectedEntries: []int{1000, 1000, 1000, 253},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup new driver each time so no interference
			var d []*gce.CloudDisk
			fakeCloudProvider, err := gce.CreateFakeCloudProvider(project, zone, d)
			if err != nil {
				t.Fatalf("Failed to create fake cloud provider: %v", err)
			}
			for i := 0; i < tc.diskCount; i++ {
				// Create diskCount dummy instances, each with a dynamically attached disk
				instanceName := fmt.Sprintf("instance-%v", i)
				diskName := fmt.Sprintf("pvc-%v", i)
				instance := compute.Instance{
					Name:     name,
					SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s", project, zone, instanceName),
					Disks: []*compute.AttachedDisk{
						{
							DeviceName: diskName,
							Source:     fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s", project, zone, diskName),
						},
					},
				}
				fakeCloudProvider.InsertInstance(&instance, zone, instanceName)
			}
			gceDriver := initGCEDriverWithCloudProvider(t, fakeCloudProvider)
			// Use attached disks (instances.list) API
			gceDriver.cs.listVolumesConfig.UseInstancesAPIForPublishedNodes = true

			tok := ""
			for i, expectedEntry := range tc.expectedEntries {
				lvr := &csi.ListVolumesRequest{
					MaxEntries:    tc.maxEntries,
					StartingToken: tok,
				}
				resp, err := gceDriver.cs.ListVolumes(context.TODO(), lvr)
				if err != nil {
					t.Fatalf("Got error %v", err)
					return
				}

				if len(resp.Entries) != expectedEntry {
					t.Fatalf("Got %v entries, expected %v on call # %d", len(resp.Entries), expectedEntry, i+1)
				}

				tok = resp.NextToken
			}
			if len(tok) != 0 {
				t.Fatalf("Expected no more entries, but got NextToken in response: %s", tok)
			}
		})
	}
}

func TestListVolumeArgs(t *testing.T) {
	diskCount := 500
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
				name := fmt.Sprintf("disk-%v", i)
				d = append(d, gce.CloudDiskFromV1(&compute.Disk{
					Name:     name,
					SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/project/zones/zone/disk/%s", name),
				}))
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

func TestListVolumeResponse(t *testing.T) {
	zone1 := "us-central1-a"
	zone2 := "us-central1-b"
	testCases := []struct {
		name            string
		disks           []compute.Disk
		instances       []compute.Instance
		expectedEntries []*csi.ListVolumesResponse_Entry
	}{
		{
			name: "unattached disk",
			disks: []compute.Disk{
				{
					Name:     "pv-1",
					SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s", project, zone, "pv-1"),
				},
			},
			expectedEntries: []*csi.ListVolumesResponse_Entry{
				{
					Volume: &csi.Volume{
						VolumeId: fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, "pv-1"),
					},
					Status: &csi.ListVolumesResponse_VolumeStatus{
						PublishedNodeIds: []string{},
					},
				},
			},
		},
		{
			name: "attached disk",
			disks: []compute.Disk{
				{
					Name:     "pv-1",
					SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s", project, zone, "pv-1"),
				},
			},
			instances: []compute.Instance{
				{
					Name:     "node-1",
					SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s", project, zone, "node-1"),
					Disks: []*compute.AttachedDisk{
						{
							DeviceName: "pv-1",
							Source:     fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s", project, zone, "pv-1"),
						},
					},
				},
			},
			expectedEntries: []*csi.ListVolumesResponse_Entry{
				{
					Volume: &csi.Volume{
						VolumeId: fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, "pv-1"),
					},
					Status: &csi.ListVolumesResponse_VolumeStatus{
						PublishedNodeIds: []string{fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone, "node-1")},
					},
				},
			},
		},
		{
			name: "attached regional disk",
			disks: []compute.Disk{
				{
					Name:     "pv-1",
					SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/regions/%s/disks/%s", project, region, "pv-1"),
				},
			},
			instances: []compute.Instance{
				{
					Name:     "node-1",
					SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s", project, zone, "node-1"),
					Disks: []*compute.AttachedDisk{
						{
							DeviceName: "pv-1",
							Source:     fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/regions/%s/disks/%s", project, region, "pv-1"),
						},
					},
				},
			},
			expectedEntries: []*csi.ListVolumesResponse_Entry{
				{
					Volume: &csi.Volume{
						VolumeId: fmt.Sprintf("projects/%s/regions/%s/disks/%s", project, region, "pv-1"),
					},
					Status: &csi.ListVolumesResponse_VolumeStatus{
						PublishedNodeIds: []string{fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone, "node-1")},
					},
				},
			},
		},
		{
			name: "multi-zone attached disk",
			disks: []compute.Disk{
				{
					Name:     fmt.Sprintf("%s-pv-1", zone1),
					SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s", project, zone1, "pv-1"),
					Labels:   map[string]string{common.MultiZoneLabel: "true"},
				},
				{
					Name:     fmt.Sprintf("%s-pv-1", zone2),
					SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s", project, zone2, "pv-1"),
					Labels:   map[string]string{common.MultiZoneLabel: "true"},
				},
			},
			instances: []compute.Instance{
				{
					Name:     "node-1",
					SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s", project, zone1, "node-1"),
					Disks: []*compute.AttachedDisk{
						{
							DeviceName: "pv-1",
							Source:     fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s", project, zone1, "pv-1"),
						},
					},
					Zone: zone1,
				},
				{
					Name:     "node-2",
					SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s", project, zone2, "node-2"),
					Disks: []*compute.AttachedDisk{
						{
							DeviceName: "pv-1",
							Source:     fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s", project, zone2, "pv-1"),
						},
					},
					Zone: zone2,
				},
			},
			expectedEntries: []*csi.ListVolumesResponse_Entry{
				{
					Volume: &csi.Volume{
						VolumeId: fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone1, "pv-1"),
					},
					Status: &csi.ListVolumesResponse_VolumeStatus{
						PublishedNodeIds: []string{fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone1, "node-1")},
					},
				},
				{
					Volume: &csi.Volume{
						VolumeId: fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone2, "pv-1"),
					},
					Status: &csi.ListVolumesResponse_VolumeStatus{
						PublishedNodeIds: []string{fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone2, "node-2")},
					},
				},
				{
					Volume: &csi.Volume{
						VolumeId: fmt.Sprintf("projects/%s/zones/multi-zone/disks/%s", project, "pv-1"),
					},
					Status: &csi.ListVolumesResponse_VolumeStatus{
						PublishedNodeIds: []string{
							fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone1, "node-1"),
							fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone2, "node-2"),
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var d []*gce.CloudDisk
			for _, disk := range tc.disks {
				d = append(d, gce.CloudDiskFromV1(&disk))
			}
			fakeCloudProvider, err := gce.CreateFakeCloudProvider(project, zone, d)
			if err != nil {
			}
			for _, instance := range tc.instances {
				fakeCloudProvider.InsertInstance(&instance, instance.Zone, instance.Name)
			}
			// Setup new driver each time so no interference
			gceDriver := initGCEDriverWithCloudProvider(t, fakeCloudProvider)
			gceDriver.cs.multiZoneVolumeHandleConfig = MultiZoneVolumeHandleConfig{
				Enable: true,
			}

			resp, err := gceDriver.cs.ListVolumes(context.TODO(), &csi.ListVolumesRequest{})
			if err != nil {
				t.Fatalf("ListVolumes unexpected error: %v", err)
			}

			if diff := cmp.Diff(tc.expectedEntries, resp.Entries, cmp.Transformer("EntryToVolumeId", entryToVolumeId), cmpopts.SortSlices(less)); diff != "" {
				t.Errorf("ListVolumes: -want err, +got err\n%s", diff)
			}
		})
	}
}

func entryToVolumeId(e *csi.ListVolumesResponse_Entry) string {
	return e.Volume.VolumeId
}

func less(a, b fmt.Stringer) bool {
	return a.String() < b.String()
}

func TestCreateVolumeWithVolumeSourceFromSnapshot(t *testing.T) {
	testCases := []struct {
		name            string
		project         string
		volKey          *meta.Key
		snapshotType    string
		snapshotOnCloud bool
		expErrCode      codes.Code
	}{
		{
			name:            "success with data source of snapshot type",
			project:         "test-project",
			volKey:          meta.ZonalKey("my-disk", zone),
			snapshotType:    common.DiskSnapshotType,
			snapshotOnCloud: true,
		},
		{
			name:            "fail with data source of snapshot type that doesn't exist",
			project:         "test-project",
			volKey:          meta.ZonalKey("my-disk", zone),
			snapshotType:    common.DiskSnapshotType,
			snapshotOnCloud: false,
			expErrCode:      codes.NotFound,
		},
		{
			name:            "success with data source of snapshot type",
			project:         "test-project",
			volKey:          meta.ZonalKey("my-disk", zone),
			snapshotType:    common.DiskImageType,
			snapshotOnCloud: true,
		},
		{
			name:            "fail with data source of snapshot type that doesn't exist",
			project:         "test-project",
			volKey:          meta.ZonalKey("my-disk", zone),
			snapshotType:    common.DiskImageType,
			snapshotOnCloud: false,
			expErrCode:      codes.NotFound,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		gceDriver := initGCEDriver(t, nil)

		snapshotParams, err := common.ExtractAndDefaultSnapshotParameters(nil, gceDriver.name, nil)
		if err != nil {
			t.Errorf("Got error extracting snapshot parameters: %v", err)
		}

		// Start Test
		var snapshotID string
		switch tc.snapshotType {
		case common.DiskSnapshotType:
			snapshotID = testSnapshotID
			if tc.snapshotOnCloud {
				gceDriver.cs.CloudProvider.CreateSnapshot(context.Background(), tc.project, tc.volKey, name, snapshotParams)
			}
		case common.DiskImageType:
			snapshotID = testImageID
			if tc.snapshotOnCloud {
				gceDriver.cs.CloudProvider.CreateImage(context.Background(), tc.project, tc.volKey, name, snapshotParams)
			}
		default:
			t.Errorf("Unknown snapshot type: %v", tc.snapshotType)
		}

		req := &csi.CreateVolumeRequest{
			Name:               "test-name",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCaps,
			VolumeContentSource: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{
					Snapshot: &csi.VolumeContentSource_SnapshotSource{
						SnapshotId: snapshotID,
					},
				},
			},
		}

		resp, err := gceDriver.cs.CreateVolume(context.Background(), req)
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

func TestCloningLocationRequirements(t *testing.T) {
	testSourceVolumeName := "test-volume-source-name"
	testZonalVolumeSourceID := fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, testSourceVolumeName)
	testRegionalVolumeSourceID := fmt.Sprintf("projects/%s/regions/%s/disks/%s", project, region, testSourceVolumeName)

	testCases := []struct {
		name                         string
		sourceVolumeID               string
		nilVolumeContentSource       bool
		reqParameters                map[string]string
		requestCapacityRange         *csi.CapacityRange
		replicationType              string
		expectedLocationRequirements *locationRequirements
		expectedErr                  bool
	}{
		{
			name:                 "success zonal disk clone of zonal source disk",
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			reqParameters: map[string]string{
				common.ParameterKeyReplicationType: replicationTypeNone,
			},
			replicationType:              replicationTypeNone,
			expectedLocationRequirements: &locationRequirements{srcVolRegion: region, srcVolZone: zone, srcReplicationType: replicationTypeNone, cloneReplicationType: replicationTypeNone},
			expectedErr:                  false,
		},
		{
			name:                 "success regional disk clone of regional source disk",
			sourceVolumeID:       testRegionalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			reqParameters: map[string]string{
				common.ParameterKeyReplicationType: replicationTypeRegionalPD,
			},
			replicationType:              replicationTypeRegionalPD,
			expectedLocationRequirements: &locationRequirements{srcVolRegion: region, srcVolZone: "", srcReplicationType: replicationTypeRegionalPD, cloneReplicationType: replicationTypeRegionalPD},
			expectedErr:                  false,
		},
		{
			name:                 "success regional disk clone of zonal data source",
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			reqParameters: map[string]string{
				common.ParameterKeyReplicationType: replicationTypeRegionalPD,
			},
			replicationType:              replicationTypeRegionalPD,
			expectedLocationRequirements: &locationRequirements{srcVolRegion: region, srcVolZone: zone, srcReplicationType: replicationTypeNone, cloneReplicationType: replicationTypeRegionalPD},
			expectedErr:                  false,
		},
		{
			name:                   "non-cloning CreateVolumeRequest",
			nilVolumeContentSource: true,
			requestCapacityRange:   stdCapRange,
			reqParameters: map[string]string{
				common.ParameterKeyReplicationType: replicationTypeRegionalPD,
			},
			replicationType:              replicationTypeRegionalPD,
			expectedLocationRequirements: nil,
			expectedErr:                  false,
		},
		{
			name:                 "failure invalid volumeID",
			sourceVolumeID:       fmt.Sprintf("projects/%s/disks/%s", project, testSourceVolumeName),
			requestCapacityRange: stdCapRange,
			reqParameters: map[string]string{
				common.ParameterKeyReplicationType: replicationTypeNone,
			},
			replicationType:              replicationTypeNone,
			expectedLocationRequirements: nil,
			expectedErr:                  true,
		},
	}

	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		req := &csi.CreateVolumeRequest{
			Name:               name,
			CapacityRange:      tc.requestCapacityRange,
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
		if tc.nilVolumeContentSource {
			req.VolumeContentSource = nil
		}

		locationRequirements, err := cloningLocationRequirements(req, tc.replicationType)

		if err != nil != tc.expectedErr {
			t.Fatalf("Got error %v, expected error %t", err, tc.expectedErr)
		}
		input := fmt.Sprintf("cloningLocationRequirements(%v, %s", req, tc.replicationType)
		if fmt.Sprintf("%v", tc.expectedLocationRequirements) != fmt.Sprintf("%v", locationRequirements) {
			t.Fatalf("%s returned unexpected diff got: %v, want %v", input, locationRequirements, tc.expectedLocationRequirements)
		}
	}
}

func TestCreateVolumeWithVolumeSourceFromVolume(t *testing.T) {
	testSourceVolumeName := "test-volume-source-name"
	testCloneVolumeName := "test-volume-clone"
	testZonalVolumeSourceID := fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, testSourceVolumeName)
	testRegionalVolumeSourceID := fmt.Sprintf("projects/%s/regions/%s/disks/%s", project, region, testSourceVolumeName)
	testSecondZonalVolumeSourceID := fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, "different-zone1", testSourceVolumeName)
	zonalParams := map[string]string{
		common.ParameterKeyType: "test-type", common.ParameterKeyReplicationType: replicationTypeNone,
		common.ParameterKeyDiskEncryptionKmsKey: "encryption-key",
	}
	regionalParams := map[string]string{
		common.ParameterKeyType: "test-type", common.ParameterKeyReplicationType: replicationTypeRegionalPD,
		common.ParameterKeyDiskEncryptionKmsKey: "encryption-key",
	}
	requisiteTopology := []*csi.Topology{
		{
			Segments: map[string]string{common.TopologyKeyZone: zone},
		},
		{
			Segments: map[string]string{common.TopologyKeyZone: secondZone},
		},
	}

	requisiteAllRegionZonesTopology := []*csi.Topology{
		{
			Segments: map[string]string{common.TopologyKeyZone: "country-region-fakethirdzone"},
		},
		{
			Segments: map[string]string{common.TopologyKeyZone: zone},
		},
		{
			Segments: map[string]string{common.TopologyKeyZone: secondZone},
		},
	}

	prefTopology := []*csi.Topology{
		{
			Segments: map[string]string{common.TopologyKeyZone: zone},
		},
		{
			Segments: map[string]string{common.TopologyKeyZone: secondZone},
		},
	}

	testCases := []struct {
		name                 string
		volumeOnCloud        bool
		expErrCode           codes.Code
		expErrMsg            string
		sourceVolumeID       string
		reqParameters        map[string]string
		sourceReqParameters  map[string]string
		sourceCapacityRange  *csi.CapacityRange
		requestCapacityRange *csi.CapacityRange
		sourceTopology       *csi.TopologyRequirement
		requestTopology      *csi.TopologyRequirement
		enableStoragePools   bool
		expCloneKey          *meta.Key
		// Accessible topologies validates that the replica zones are valid for regional disk clones.
		expAccessibleTop []*csi.Topology
	}{

		{
			name:                 "success zonal -> zonal cloning, nil topology: immediate binding w/ no allowedTopologies",
			volumeOnCloud:        true,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        zonalParams,
			sourceReqParameters:  zonalParams,
			// Source volume will be in the zone that is the first element of preferred topologies (country-region-zone)
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: nil,
			expCloneKey:     &meta.Key{Name: testCloneVolumeName, Zone: zone, Region: ""},
			expAccessibleTop: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-zone"},
				},
			},
		},
		{
			name:                 "success zonal -> zonal cloning, req = allowedTopologies, pref = req w/ randomly selected zone as first element: immediate binding w/ allowedTopologies",
			volumeOnCloud:        true,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        zonalParams,
			sourceReqParameters:  zonalParams,
			// Source volume will be in the zone that is the first element of preferred topologies (country-region-zone)
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: secondZone},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: zone},
					},
				},
			},
			expCloneKey: &meta.Key{Name: testCloneVolumeName, Zone: zone, Region: ""},
			expAccessibleTop: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-zone"},
				},
			},
		},
		{
			name:                 "success zonal -> zonal cloning, req = allowedTopologies, pref = req w/ src zone as first element: delayed binding w/ allowedTopologies",
			volumeOnCloud:        true,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        zonalParams,
			sourceReqParameters:  zonalParams,
			// Source volume will be in the zone that is the first element of preferred topologies (country-region-zone)
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			expCloneKey: &meta.Key{Name: testCloneVolumeName, Zone: zone, Region: ""},
			expAccessibleTop: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-zone"},
				},
			},
		},
		{
			name:                 "fail cloning with storage pools",
			volumeOnCloud:        true,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			enableStoragePools:   true,
			reqParameters: map[string]string{
				common.ParameterKeyType:                 "test-type",
				common.ParameterKeyReplicationType:      replicationTypeNone,
				common.ParameterKeyDiskEncryptionKmsKey: "encryption-key",
				common.ParameterKeyStoragePools:         "projects/test-project/zones/country-region-zone/storagePools/storagePool-1",
			},
			sourceReqParameters: zonalParams,
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			expErrCode: codes.InvalidArgument,
			expErrMsg:  "storage pools do not support disk clones",
		},
		{
			name:                 "success zonal -> zonal cloning, req = all zones in region, pref = req w/ src zone as first element: delayed binding without allowedTopologies",
			volumeOnCloud:        true,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        zonalParams,
			sourceReqParameters:  zonalParams,
			// Source volume will be in the zone that is the first element of preferred topologies (country-region-zone)
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteAllRegionZonesTopology,
				Preferred: prefTopology,
			},
			expCloneKey: &meta.Key{Name: testCloneVolumeName, Zone: zone, Region: ""},
			expAccessibleTop: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-zone"},
				},
			},
		},
		{
			name:                 "success zonal -> regional cloning, nil topology: immediate binding w/ no allowedTopologies",
			volumeOnCloud:        true,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        regionalParams,
			sourceReqParameters:  zonalParams,
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: nil,
			expCloneKey:     &meta.Key{Name: testCloneVolumeName, Zone: "", Region: "country-region"},
			expAccessibleTop: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-zone"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-fakesecondzone"},
				},
			},
		},
		{
			name:                 "success zonal -> regional cloning, req = allowedTopologies, pref = req w/ randomly selected zone as first element: immediate binding w/ allowedTopologies",
			volumeOnCloud:        true,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        regionalParams,
			sourceReqParameters:  zonalParams,
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: secondZone},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: zone},
					},
				},
			},
			expCloneKey: &meta.Key{Name: testCloneVolumeName, Zone: "", Region: "country-region"},
			expAccessibleTop: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-zone"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-fakesecondzone"},
				},
			},
		},
		{
			name:                 "success zonal -> regional cloning, req = allowedTopologies, pref = req w/ src zone as first element: delayed binding w/ allowedTopologies",
			volumeOnCloud:        true,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        regionalParams,
			sourceReqParameters:  zonalParams,
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			expCloneKey: &meta.Key{Name: testCloneVolumeName, Zone: "", Region: "country-region"},
			expAccessibleTop: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-zone"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-fakesecondzone"},
				},
			},
		},
		{
			name:                 "success zonal -> regional cloning, req = all zones in region, pref = req w/ src zone as first element: delayed binding without allowedTopologies",
			volumeOnCloud:        true,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        regionalParams,
			sourceReqParameters:  zonalParams,
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteAllRegionZonesTopology,
				Preferred: prefTopology,
			},
			expCloneKey: &meta.Key{Name: testCloneVolumeName, Zone: "", Region: "country-region"},
			expAccessibleTop: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-zone"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-fakesecondzone"},
				},
			},
		},
		{
			name:                 "success regional -> regional cloning, nil topology: immediate binding w/ no allowedTopologies",
			volumeOnCloud:        true,
			sourceVolumeID:       testRegionalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        regionalParams,
			sourceReqParameters:  regionalParams,
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: nil,
			expCloneKey:     &meta.Key{Name: testCloneVolumeName, Zone: "", Region: "country-region"},
			expAccessibleTop: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-zone"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-fakesecondzone"},
				},
			},
		},
		{
			name:                 "success regional -> regional cloning, req = allowedTopologies, pref = req w/ randomly selected zone as first element: immediate binding w/ allowedTopologies",
			volumeOnCloud:        true,
			sourceVolumeID:       testRegionalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        regionalParams,
			sourceReqParameters:  regionalParams,
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: secondZone},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: zone},
					},
				},
			},
			expCloneKey: &meta.Key{Name: testCloneVolumeName, Zone: "", Region: "country-region"},
			expAccessibleTop: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-zone"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-fakesecondzone"},
				},
			},
		},
		{
			name:                 "success regional -> regional cloning, req = allowedTopologies, pref = req w/ src zone as first element: delayed binding w/ allowedTopologies",
			volumeOnCloud:        true,
			sourceVolumeID:       testRegionalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        regionalParams,
			sourceReqParameters:  regionalParams,
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			expCloneKey: &meta.Key{Name: testCloneVolumeName, Zone: "", Region: "country-region"},
			expAccessibleTop: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-zone"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-fakesecondzone"},
				},
			},
		},
		{
			name:                 "success regional -> regional cloning, req = all zones in region, pref = req w/ src zone as first element: delayed binding without allowedTopologies",
			volumeOnCloud:        true,
			sourceVolumeID:       testRegionalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        regionalParams,
			sourceReqParameters:  regionalParams,
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			expCloneKey: &meta.Key{Name: testCloneVolumeName, Zone: "", Region: "country-region"},
			expAccessibleTop: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-zone"},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: "country-region-fakesecondzone"},
				},
			},
		},
		{
			name:                 "fail regional disk clone with no matching replica zone of zonal data source",
			volumeOnCloud:        true,
			expErrCode:           codes.InvalidArgument,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        regionalParams,
			sourceReqParameters:  zonalParams,
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "different-zone1"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "different-zone2"},
					},
				},
			},
		},
		{
			name:                 "fail zonal disk clone with different disk type",
			volumeOnCloud:        true,
			expErrCode:           codes.InvalidArgument,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        zonalParams,
			sourceReqParameters: map[string]string{
				common.ParameterKeyType: "different-type",
			},
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
		},
		{
			name:                 "fail zonal disk clone with different DiskEncryptionKMSKey",
			volumeOnCloud:        true,
			expErrCode:           codes.InvalidArgument,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        zonalParams,
			sourceReqParameters: map[string]string{
				common.ParameterKeyType: "test-type", common.ParameterKeyReplicationType: replicationTypeNone,
				common.ParameterKeyDiskEncryptionKmsKey: "different-encryption-key",
			},
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
		},
		{
			name:                 "fail zonal disk clone with different zone",
			volumeOnCloud:        true,
			expErrCode:           codes.InvalidArgument,
			sourceVolumeID:       testSecondZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        zonalParams,
			sourceReqParameters:  zonalParams,
			sourceTopology: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "different-zone1"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "different-zone2"},
					},
				},
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
		},
		{
			name:                 "fail zonal disk clone of regional data source",
			volumeOnCloud:        true,
			expErrCode:           codes.InvalidArgument,
			sourceVolumeID:       testRegionalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        zonalParams,
			sourceReqParameters:  regionalParams,
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
		},

		{
			name:                 "fail zonal source disk does not exist",
			volumeOnCloud:        false,
			expErrCode:           codes.NotFound,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        stdParams,
			sourceReqParameters:  stdParams,
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
		},
		{
			name:                 "fail invalid source disk volume id format",
			volumeOnCloud:        false,
			expErrCode:           codes.InvalidArgument,
			sourceVolumeID:       testZonalVolumeSourceID + "/invalid/format",
			requestCapacityRange: stdCapRange,
			sourceCapacityRange:  stdCapRange,
			reqParameters:        stdParams,
			sourceReqParameters:  stdParams,
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
		},
		{
			name:                 "fail zonal disk clone with smaller disk capacity",
			volumeOnCloud:        true,
			expErrCode:           codes.InvalidArgument,
			sourceVolumeID:       testZonalVolumeSourceID,
			requestCapacityRange: stdCapRange,
			sourceCapacityRange: &csi.CapacityRange{
				RequiredBytes: common.GbToBytes(40),
			},
			reqParameters:       zonalParams,
			sourceReqParameters: zonalParams,
			sourceTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
			requestTopology: &csi.TopologyRequirement{
				Requisite: requisiteTopology,
				Preferred: prefTopology,
			},
		},
	}

	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		gceDriver := initGCEDriver(t, nil)
		gceDriver.cs.enableStoragePools = tc.enableStoragePools

		req := &csi.CreateVolumeRequest{
			Name:               testCloneVolumeName,
			CapacityRange:      tc.requestCapacityRange,
			VolumeCapabilities: stdVolCaps,
			Parameters:         tc.reqParameters,
			VolumeContentSource: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: tc.sourceVolumeID,
					},
				},
			},
			AccessibilityRequirements: tc.requestTopology,
		}

		sourceVolumeRequest := &csi.CreateVolumeRequest{
			Name:                      testSourceVolumeName,
			CapacityRange:             tc.sourceCapacityRange,
			VolumeCapabilities:        stdVolCaps,
			Parameters:                tc.sourceReqParameters,
			AccessibilityRequirements: tc.sourceTopology,
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
		if tc.expErrMsg != "" && tc.expErrMsg != err.Error() {
			t.Fatalf("Got error: %v, expected error: %v", err.Error(), tc.expErrMsg)
		}

		// Make sure the response has the source volume.
		respVolume := resp.GetVolume()
		if respVolume.ContentSource == nil || respVolume.ContentSource.Type == nil ||
			respVolume.ContentSource.GetVolume() == nil || respVolume.ContentSource.GetVolume().VolumeId == "" {
			t.Fatalf("Expected volume content source to have volume ID, got none")
		}
		// Validate that the cloned volume is in the region/zone that we expect
		cloneVolID := respVolume.VolumeId
		_, cloneVolKey, err := common.VolumeIDToKey(cloneVolID)
		if err != nil {
			t.Fatalf("failed to get key from volume id %q: %v", cloneVolID, err)
		}
		if cloneVolKey.String() != tc.expCloneKey.String() {
			t.Fatalf("got clone volume key: %q, expected clone volume key: %q", cloneVolKey.String(), tc.expCloneKey.String())
		}
		if !accessibleTopologiesEqual(respVolume.AccessibleTopology, tc.expAccessibleTop) {
			t.Fatalf("got accessible topology: %q, expected accessible topology: %q", fmt.Sprintf("%+v", respVolume.AccessibleTopology), fmt.Sprintf("%+v", tc.expAccessibleTop))

		}
	}
}

func sortTopologies(in []*csi.Topology) {
	sort.Slice(in, func(i, j int) bool {
		return in[i].Segments[common.TopologyKeyZone] < in[j].Segments[common.TopologyKeyZone]
	})
}

func accessibleTopologiesEqual(got []*csi.Topology, expected []*csi.Topology) bool {
	sortTopologies(got)
	sortTopologies(expected)
	return fmt.Sprintf("%+v", got) == fmt.Sprintf("%+v", expected)
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
		Name:     name,
		SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/project/zones/zone/name/%s", name),
	})
}

func createZonalCloudDiskWithZone(name, zone string) *gce.CloudDisk {
	return gce.CloudDiskFromV1(&compute.Disk{
		Name:     name,
		SelfLink: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/project/zones/zone/name/%s", name),
		Zone:     zone,
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

func TestMultiZoneDeleteVolume(t *testing.T) {
	testCases := []struct {
		name      string
		seedDisks []*gce.CloudDisk
		req       *csi.DeleteVolumeRequest
		expErr    bool
	}{
		{
			name: "single-zone",
			seedDisks: []*gce.CloudDisk{
				createZonalCloudDiskWithZone(name, zone),
			},
			req: &csi.DeleteVolumeRequest{
				VolumeId: fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, common.MultiZoneValue, name),
			},
		},
		{
			name: "multi-zone",
			seedDisks: []*gce.CloudDisk{
				createZonalCloudDiskWithZone(name, zone),
				createZonalCloudDiskWithZone(name, secondZone),
			},
			req: &csi.DeleteVolumeRequest{
				VolumeId: fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, common.MultiZoneValue, name),
			},
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		// Setup new driver each time so no interference
		fcp, err := gce.CreateFakeCloudProvider(project, zone, tc.seedDisks)
		if err != nil {
			t.Fatalf("Failed to create fake cloud provider: %v", err)
		}
		// Setup new driver each time so no interference
		gceDriver := initGCEDriverWithCloudProvider(t, fcp)
		gceDriver.cs.multiZoneVolumeHandleConfig.DiskTypes = []string{"hyperdisk-ml"}
		gceDriver.cs.multiZoneVolumeHandleConfig.Enable = true
		_, err = gceDriver.cs.DeleteVolume(context.Background(), tc.req)
		if err == nil && tc.expErr {
			t.Errorf("Expected error but got none")
		}
		if err != nil && !tc.expErr {
			t.Errorf("Did not expect error but got: %v", err)
		}

		disks, _, _ := fcp.ListDisks(context.TODO(), []googleapi.Field{})
		if len(disks) > 0 {
			t.Errorf("Expected all disks to be deleted. Got: %v", disks)
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

func TestPickZonesInRegion(t *testing.T) {
	testCases := []struct {
		name     string
		region   string
		zones    []string
		expZones []string
	}{
		{
			name:     "all zones in region",
			region:   "us-central1",
			zones:    []string{"us-central1-a", "us-central1-b", "us-central1-c"},
			expZones: []string{"us-central1-a", "us-central1-b", "us-central1-c"},
		},
		{
			name:     "removes zones not in region",
			region:   "us-central1",
			zones:    []string{"us-central1-a", "us-central1-b", "us-central1-c", "us-east1-a, us-west1-a"},
			expZones: []string{"us-central1-a", "us-central1-b", "us-central1-c"},
		},
		{
			name:     "region not in zones",
			region:   "us-west1",
			zones:    []string{"us-central1-a", "us-central1-b", "us-central1-c"},
			expZones: []string{},
		},
		{
			name:     "empty zones",
			region:   "us-central1",
			zones:    []string{},
			expZones: []string{},
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		gotZones := pickZonesInRegion(tc.region, tc.zones)
		if !sets.NewString(gotZones...).Equal(sets.NewString(tc.expZones...)) {
			t.Errorf("Got zones: %v, expected: %v", gotZones, tc.expZones)
		}
	}
}

func TestPrependZone(t *testing.T) {
	testCases := []struct {
		name     string
		zone     string
		zones    []string
		expZones []string
	}{
		{
			name:     "zone already at index 0",
			zone:     "us-central1-a",
			zones:    []string{"us-central1-a", "us-central1-b", "us-central1-c"},
			expZones: []string{"us-central1-a", "us-central1-b", "us-central1-c"},
		},
		{
			name:     "zone at index 1",
			zone:     "us-central1-b",
			zones:    []string{"us-central1-a", "us-central1-b", "us-central1-c"},
			expZones: []string{"us-central1-b", "us-central1-a", "us-central1-c"},
		},
		{
			name:     "zone not in zones",
			zone:     "us-central1-f",
			zones:    []string{"us-central1-a", "us-central1-b", "us-central1-c"},
			expZones: []string{"us-central1-f", "us-central1-a", "us-central1-b", "us-central1-c"},
		},
		{
			name:     "empty zones",
			zone:     "us-central1-a",
			zones:    []string{},
			expZones: []string{"us-central1-a"},
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		gotZones := prependZone(tc.zone, tc.zones)
		if !zonesEqual(gotZones, tc.expZones) {
			t.Errorf("Got zones: %v, expected: %v", gotZones, tc.expZones)
		}
	}
}

func TestPickZonesFromTopology(t *testing.T) {
	testCases := []struct {
		name                   string
		top                    *csi.TopologyRequirement
		locReq                 *locationRequirements
		numZones               int
		fallbackRequisiteZones []string
		expZones               []string
		expErr                 bool
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
			name: "success: preferred, locationRequirements[region:us-central1, zone:us-central1-a, srcReplicationType:none, cloneReplicationType:none]",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
				},
			},
			locReq:   &locationRequirements{srcVolRegion: "us-central1", srcVolZone: "us-central1-a", srcReplicationType: replicationTypeNone, cloneReplicationType: replicationTypeNone},
			numZones: 1,
			expZones: []string{"us-central1-a"},
		},
		{
			name: "success: requisite, locationRequirements[region:us-central1, zone:us-central1-a, srcReplicationType:none, cloneReplicationType:regional-pd]",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-f"},
					},
				},
				Preferred: []*csi.Topology{},
			},
			locReq:   &locationRequirements{srcVolRegion: "us-central1", srcVolZone: "us-central1-c", srcReplicationType: replicationTypeNone, cloneReplicationType: replicationTypeRegionalPD},
			numZones: 2,
			expZones: []string{"us-central1-c", "us-central1-f"},
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
			name: "success: preferred and requisite, locationRequirements[region:us-central1, zone:us-central1-a, srcReplicationType:regional-pd, cloneReplicationType:regional-pd]",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-d"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-f"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-west1-a"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-east1-a"},
					},
				},
			},
			locReq:   &locationRequirements{srcVolRegion: "us-central1", srcVolZone: "us-central1-a", srcReplicationType: replicationTypeRegionalPD, cloneReplicationType: replicationTypeRegionalPD},
			numZones: 5,
			expZones: []string{"us-central1-b", "us-central1-a", "us-central1-c", "us-central1-d", "us-central1-f"},
		},
		{
			name: "success: preferred and requisite, locationRequirements[region:us-central1, zone:us-central1-a, srcReplicationType:none, cloneReplicationType:regional-pd]",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-d"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-f"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-west1-a"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-east1-a"},
					},
				},
			},
			locReq:   &locationRequirements{srcVolRegion: "us-central1", srcVolZone: "us-central1-a", srcReplicationType: replicationTypeNone, cloneReplicationType: replicationTypeRegionalPD},
			numZones: 5,
			expZones: []string{"us-central1-a", "us-central1-b", "us-central1-c", "us-central1-d", "us-central1-f"},
		},
		{
			name: "success: preferred and requisite, locationRequirements[region:us-central1, zone:us-central1-a, srcReplicationType:none, cloneReplicationType:regional-pd], 3 zones {a, b, c}",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-f"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
				},
			},
			locReq:   &locationRequirements{srcVolRegion: "us-central1", srcVolZone: "us-central1-a", srcReplicationType: replicationTypeNone, cloneReplicationType: replicationTypeRegionalPD},
			numZones: 3,
			expZones: []string{"us-central1-a", "us-central1-c", "us-central1-b"},
		},
		{
			name: "success: preferred and requisite, locationRequirements[region:us-central1, zone:us-central1-a, srcReplicationType:none, cloneReplicationType:regional-pd], 3 zones {b, c, f}",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-f"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
				},
			},
			locReq:   &locationRequirements{srcVolRegion: "us-central1", srcVolZone: "us-central1-b", srcReplicationType: replicationTypeNone, cloneReplicationType: replicationTypeRegionalPD},
			numZones: 3,
			expZones: []string{"us-central1-b", "us-central1-c", "us-central1-f"},
		},
		{
			name: "success: preferred and requisite, locationRequirements[region:us-central1, zone:us-central1-a, srcReplicationType:none, cloneReplicationType:regional-pd], fallback topologies specified but unused",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
				},
			},
			fallbackRequisiteZones: []string{"us-central1-a", "us-central1-f", "us-central1-g"},
			locReq:                 &locationRequirements{srcVolRegion: "us-central1", srcVolZone: "us-central1-a", srcReplicationType: replicationTypeNone, cloneReplicationType: replicationTypeRegionalPD},
			numZones:               2,
			expZones:               []string{"us-central1-a", "us-central1-b"},
		},
		{
			name: "success: preferred and requisite, locationRequirements[region:us-central1, zone:us-central1-a, srcReplicationType:none, cloneReplicationType:regional-pd], fallback topologies specified",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-west1-b"},
					},
				},
			},
			fallbackRequisiteZones: []string{"us-central1-a", "us-central1-b", "us-central1-c"},
			locReq:                 &locationRequirements{srcVolRegion: "us-central1", srcVolZone: "us-central1-b", srcReplicationType: replicationTypeNone, cloneReplicationType: replicationTypeRegionalPD},
			numZones:               2,
			expZones:               []string{"us-central1-b", "us-central1-c"},
		},
		{
			name: "success: preferred and requisite, locationRequirements[region:us-central1, zone:us-central1-a, srcReplicationType:regional-pd, cloneReplicationType:regional-pd], fallback topologies specified",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{},
				Preferred: []*csi.Topology{
					// This is a bit contrived, a real regional PD should have two zones
					// This only has one, so we can test that a second is pulled from
					// fallbackRequisiteZones.
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
				},
			},
			fallbackRequisiteZones: []string{"us-central1-a", "us-central1-b", "us-central1-c", "us-central1-f"},
			locReq:                 &locationRequirements{srcVolRegion: "us-central1", srcVolZone: "us-central1-b", srcReplicationType: replicationTypeRegionalPD, cloneReplicationType: replicationTypeRegionalPD},
			numZones:               2,
			expZones:               []string{"us-central1-b", "us-central1-c"},
		},
		{
			name: "success: preferred and requisite, fallback topologies specified",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
				},
			},
			fallbackRequisiteZones: []string{"us-central1-a", "us-central1-b", "us-central1-c"},
			numZones:               2,
			expZones:               []string{"us-central1-b", "us-central1-c"},
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
			name: "fail: not enough topologies, fallback topologies specified",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
				},
			},
			fallbackRequisiteZones: []string{"us-central1-a", "us-central1-b", "us-central1-c"},
			numZones:               4,
			expErr:                 true,
		},
		{
			name: "fail: no topologies that match locationRequirment, locationRequirements[region:us-east1, zone:us-east1-a, replicationType:none]",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
				},
			},
			locReq:   &locationRequirements{srcVolRegion: "us-east1", srcVolZone: "us-east1-a", cloneReplicationType: replicationTypeNone},
			numZones: 1,
			expErr:   true,
		},
		{
			name: "fail: no topologies that match locationRequirment, locationRequirements[region:us-east1, zone:us-east1-a, replicationType:regional-pd]",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
				},
			},
			locReq:   &locationRequirements{srcVolRegion: "us-east1", srcVolZone: "us-east1-a", cloneReplicationType: replicationTypeRegionalPD},
			numZones: 2,
			expErr:   true,
		},
		{
			name: "fail: not enough topologies, locationRequirements[region:us-central1, zone:us-central1-a, replicationType:regional-pd]",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
				},
			},
			locReq:   &locationRequirements{srcVolRegion: "us-central1", srcVolZone: "us-central1-a", cloneReplicationType: replicationTypeRegionalPD},
			numZones: 4,
			expErr:   true,
		},
		{
			name: "success: only requisite, all zones",
			top: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-c"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-a"},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: "us-central1-b"},
					},
				},
			},
			numZones: 3,
			expZones: []string{"us-central1-b", "us-central1-c", "us-central1-a"},
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
		// Apply a deterministic seed to make the test that calls rand.Intn stable.
		rand.Seed(8)
		t.Logf("test case: %s", tc.name)
		gotZones, err := pickZonesFromTopology(tc.top, tc.numZones, tc.locReq, tc.fallbackRequisiteZones)
		if err != nil && !tc.expErr {
			t.Errorf("got error: %v, but did not expect error", err)
		}
		if err == nil && tc.expErr {
			t.Errorf("got no error, but expected error")
		}
		if !slices.Equal(gotZones, tc.expZones) {
			t.Errorf("Expected zones: %v, but got: %v", tc.expZones, gotZones)
		}
	}
}

func zonesEqual(gotZones, expectedZones []string) bool {
	if len(gotZones) != len(expectedZones) {
		return false
	}
	for i := 0; i < len(gotZones); i++ {
		if gotZones[i] != expectedZones[i] {
			return false
		}
	}
	return true
}

func TestVolumeOperationConcurrency(t *testing.T) {
	readyToExecute := make(chan chan gce.Signal, 1)
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
			t.Log(err)
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
	execVol2CreateSnapshot <- gce.Signal{}
	if err := <-vol2CreateSnapshotResp; err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// To clean up, allow the vol1CreateSnapshotA to complete
	execVol1CreateSnapshotA <- gce.Signal{}
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

type backoffDriverConfig struct {
	mockMissingInstance bool
	mockMissingDisk     bool

	clock          *clock.FakeClock
	attachedDisks  []*compute.AttachedDisk
	readyToExecute chan chan gce.Signal
}

func newFakeCSIErrorBackoff(tc *clock.FakeClock) *csiErrorBackoff {
	backoff := flowcontrol.NewFakeBackOff(errorBackoffInitialDuration, errorBackoffMaxDuration, tc)
	return &csiErrorBackoff{backoff, make(map[csiErrorBackoffId]codes.Code)}
}

func TestControllerUnpublishBackoff(t *testing.T) {
	for desc, tc := range map[string]struct {
		config *backoffDriverConfig
	}{
		"success": {},
		"missing instance": {
			config: &backoffDriverConfig{
				mockMissingInstance: true,
			},
		},
	} {
		t.Run(desc, func(t *testing.T) {
			if tc.config == nil {
				tc.config = &backoffDriverConfig{}
			}
			tc.config.clock = clock.NewFakeClock(time.Now())
			tc.config.attachedDisks = []*compute.AttachedDisk{{DeviceName: name}}
			tc.config.readyToExecute = make(chan chan gce.Signal)
			driver := backoffDriver(t, tc.config)

			backoffId := driver.cs.errorBackoff.backoffId(testNodeID, testVolumeID)
			step := 1 * time.Millisecond

			runUnpublishRequest := func(req *csi.ControllerUnpublishVolumeRequest, reportError bool) error {
				response := make(chan error)
				go func() {
					_, err := driver.cs.ControllerUnpublishVolume(context.Background(), req)
					response <- err
				}()
				go func() {
					executeChan := <-tc.config.readyToExecute
					executeChan <- gce.Signal{ReportError: reportError}
				}()
				return <-response
			}

			// Mock an active backoff condition on the node.
			driver.cs.errorBackoff.next(backoffId, codes.Unavailable)

			tc.config.clock.Step(step)
			// A requst for a a different volume should succeed. This volume is not
			// mounted on the node, so no GCE call will be made (ie, runUnpublishRequest
			// doesn't need to be called, the request can be called directly).
			differentUnpubReq := &csi.ControllerUnpublishVolumeRequest{
				VolumeId: testVolumeID + "-different",
				NodeId:   testNodeID,
			}
			if _, err := driver.cs.ControllerUnpublishVolume(context.Background(), differentUnpubReq); err != nil {
				t.Errorf("expected no error on different unpublish, got %v", err)
			}

			unpubreq := &csi.ControllerUnpublishVolumeRequest{
				VolumeId: testVolumeID,
				NodeId:   testNodeID,
			}
			// For the first 199 ms, the backoff condition is true. All controller publish
			// request will be denied with the same unavailable error code as was set on
			// the original error.
			for i := 0; i < 199; i++ {
				var err error
				_, err = driver.cs.ControllerUnpublishVolume(context.Background(), unpubreq)
				if !isUnavailableError(err) {
					t.Errorf("unexpected error %v", err)
				}
				tc.config.clock.Step(step)
			}

			// At the 200th millisecond, the backoff condition is no longer true. The driver should return a success code, and the backoff condition should be cleared.
			if tc.config.mockMissingInstance {
				_, err := driver.cs.ControllerUnpublishVolume(context.Background(), unpubreq)
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
				// Driver is expected to remove the node key from the backoff map.
				t1 := driver.cs.errorBackoff.backoff.Get(string(backoffId))
				if t1 != 0 {
					t.Error("unexpected delay")
				}
				return
			}

			// Mock an error. This will produce an Internal error, which is different from
			// the default error and what's used in the failure above, so that the correct
			// error code can be confirmed.
			if err := runUnpublishRequest(unpubreq, true); err == nil {
				t.Errorf("expected error")
			}

			// The above failure should cause driver to call backoff.next() again and a
			// backoff duration of 400 ms duration is set starting at the 200th
			// millisecond.  For the 200-599 ms, the backoff condition is true, with an
			// internal error this time, and new controller publish requests will be
			// denied.
			for i := 0; i < 399; i++ {
				tc.config.clock.Step(step)
				var err error
				_, err = driver.cs.ControllerUnpublishVolume(context.Background(), unpubreq)
				if !isInternalError(err) {
					t.Errorf("unexpected error %v", err)
				}
			}

			// Mock clock tick for the 600th millisecond. So backoff condition is no longer true.
			tc.config.clock.Step(step)
			// Now mock a successful ControllerUnpublish request, where DetachDisk call succeeds.
			if err := runUnpublishRequest(unpubreq, false); err != nil {
				t.Errorf("unexpected error")
			}

			// Driver is expected to remove the node key from the backoff map.
			t1 := driver.cs.errorBackoff.backoff.Get(string(backoffId))
			if t1 != 0 {
				t.Error("unexpected delay")
			}
		})
	}
}

func TestControllerUnpublishSucceedsIfNotFound(t *testing.T) {
	for desc, tc := range map[string]struct {
		volumeID string
	}{
		"full volume ID": {
			volumeID: testVolumeID,
		},
		"underspecified volume ID": {
			volumeID: underspecifiedVolumeID,
		},
	} {
		t.Run(desc, func(t *testing.T) {
			driver := backoffDriver(t, &backoffDriverConfig{
				mockMissingDisk: true,
				clock:           clock.NewFakeClock(time.Now()),
				attachedDisks:   []*compute.AttachedDisk{{DeviceName: name}},
			})

			req := &csi.ControllerUnpublishVolumeRequest{
				VolumeId: tc.volumeID,
				NodeId:   testNodeID,
			}

			_, err := driver.cs.ControllerUnpublishVolume(context.Background(), req)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestControllerPublishBackoff(t *testing.T) {
	for desc, tc := range map[string]struct {
		config      *backoffDriverConfig
		forceAttach bool
	}{
		"success":      {},
		"force attach": {forceAttach: true},
		"missing instance": {
			config: &backoffDriverConfig{
				mockMissingInstance: true,
			},
		},
	} {
		t.Run(desc, func(t *testing.T) {
			if tc.config == nil {
				tc.config = &backoffDriverConfig{}
			}
			tc.config.clock = clock.NewFakeClock(time.Now())
			tc.config.readyToExecute = make(chan chan gce.Signal)
			driver := backoffDriver(t, tc.config)

			backoffId := driver.cs.errorBackoff.backoffId(testNodeID, testVolumeID)
			step := 1 * time.Millisecond

			// Mock an active bakcoff condition on the node.
			driver.cs.errorBackoff.next(backoffId, codes.Unavailable)

			// A detach request for a different disk should succeed. As this disk is not
			// on the instance, the detach will succeed without calling the gce detach
			// disk api so we don't have to go through the blocking cloud provider and
			// and make the request directly.
			if _, err := driver.cs.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{VolumeId: testVolumeID + "different", NodeId: testNodeID}); err != nil {
				t.Errorf("expected no error on different unpublish, got %v", err)
			}

			var volumeContext map[string]string
			if tc.forceAttach {
				volumeContext = map[string]string{
					contextForceAttach: "true",
				}
			}
			pubreq := &csi.ControllerPublishVolumeRequest{
				VolumeId: testVolumeID,
				NodeId:   testNodeID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				VolumeContext: volumeContext,
			}
			// For the first 199 ms, the backoff condition is true. All controller publish request will be denied with 'Unavailable' error code.
			for i := 0; i < 199; i++ {
				tc.config.clock.Step(step)
				var err error
				_, err = driver.cs.ControllerPublishVolume(context.Background(), pubreq)
				if !isUnavailableError(err) {
					t.Errorf("unexpected error %v", err)
				}
			}

			// Mock clock tick for the 200th millisecond. So backoff condition is no longer true.
			tc.config.clock.Step(step)
			runPublishRequest := func(req *csi.ControllerPublishVolumeRequest, reportError bool) error {
				response := make(chan error)
				go func() {
					_, err := driver.cs.ControllerPublishVolume(context.Background(), req)
					response <- err
				}()
				go func() {
					executeChan := <-tc.config.readyToExecute
					executeChan <- gce.Signal{ReportError: reportError}
				}()
				return <-response
			}

			// For a missing instance the driver should return error code, and the backoff condition should be set.
			if tc.config.mockMissingInstance {
				_, err := driver.cs.ControllerPublishVolume(context.Background(), pubreq)
				if err == nil {
					t.Errorf("unexpected error %v", err)
				}

				t1 := driver.cs.errorBackoff.backoff.Get(string(backoffId))
				if t1 == 0 {
					t.Error("expected delay, got none")
				}
				return
			}

			// Mock an error
			if err := runPublishRequest(pubreq, true); err == nil {
				t.Errorf("expected error")
			}

			// The above failure should cause driver to call backoff.next() again and a
			// backoff duration of 400 ms duration is set starting at the 200th
			// millisecond.  For the 200-599 ms, the backoff condition is true, with an
			// internal error this time, and new controller publish requests will be
			// denied.
			for i := 0; i < 399; i++ {
				tc.config.clock.Step(step)
				var err error
				_, err = driver.cs.ControllerPublishVolume(context.Background(), pubreq)
				if !isInternalError(err) {
					t.Errorf("unexpected error %v", err)
				}
			}

			// Mock clock tick for the 600th millisecond. So backoff condition is no longer true.
			tc.config.clock.Step(step)
			// Now mock a successful ControllerUnpublish request, where DetachDisk call succeeds.
			if err := runPublishRequest(pubreq, false); err != nil {
				t.Errorf("unexpected error")
			}

			if tc.forceAttach {
				instance, err := driver.cs.CloudProvider.GetInstanceOrError(context.Background(), zone, node)
				if err != nil {
					t.Fatalf("%s instance not found: %v", node, err)
				}
				for _, disk := range instance.Disks {
					if !disk.ForceAttach {
						t.Errorf("Expected %s to be force attached", disk.DeviceName)
					}
				}
			}

			// Driver is expected to remove the node key from the backoff map.
			t1 := driver.cs.errorBackoff.backoff.Get(string(backoffId))
			if t1 != 0 {
				t.Error("unexpected delay")
			}
		})
	}
}

func TestGetResource(t *testing.T) {
	testCases := []struct {
		name  string
		in    string
		want  string
		error bool
	}{
		{
			name: "v1 full standard w/ endpoint prefix",
			in:   "https://www.googleapis.com/compute/v1/projects/project/zones/zone/disks/disk",
			want: "projects/project/zones/zone/disks/disk",
		},
		{
			name: "beta full standard w/ endpoint prefix",
			in:   "https://www.googleapis.com/compute/beta/projects/project/zones/zone/disks/disk",
			want: "projects/project/zones/zone/disks/disk",
		},
		{
			name: "alpha full standard w/ endpoint prefix",
			in:   "https://www.googleapis.com/compute/alpha/projects/project/zones/zone/disks/disk",
			want: "projects/project/zones/zone/disks/disk",
		},
		{
			name:  "no prefix",
			in:    "projects/project/zones/zone/disks/disk",
			error: true,
		},
		{
			name:  "no prefix + project omitted",
			in:    "zones/zone/disks/disk",
			error: true,
		},
		{
			name: "Compute prefix, www google api",
			in:   "https://www.compute.googleapis.com/compute/v1/projects/project/zones/zone/disks/disk",
			want: "projects/project/zones/zone/disks/disk",
		},
		{
			name: "Compute prefix, partner api",
			in:   "https://www.compute.partnerapis.com/compute/v1/projects/project/zones/zone/disks/disk",
			want: "projects/project/zones/zone/disks/disk",
		},
		{
			name: "Compute, alternate googleapis host",
			in:   "https://content-compute.googleapis.com/compute/v1/projects/project/zones/zone/disks/disk",
			want: "projects/project/zones/zone/disks/disk",
		},
		{
			name: "Compute, partner host",
			in:   "https://compute.blahapis.com/compute/v1/projects/project/zones/zone/disks/disk",
			want: "projects/project/zones/zone/disks/disk",
		},
		{
			name: "Alternate partner host with mtls domain",
			in:   "https://content-compute.us-central1.rep.mtls.googleapis.com/compute/v1/projects/project/zones/zone/disks/disk",
			want: "projects/project/zones/zone/disks/disk",
		},
		{
			name: "Partner beta api",
			in:   "https://www.PARTNERapis.com/compute/beta/projects/project/zones/zone/disks/disk",
			want: "projects/project/zones/zone/disks/disk",
		},
		{
			name: "Partner alpha api",
			in:   "https://www.partnerapis.com/compute/alpha/projects/project/zones/zone/disks/disk",
			want: "projects/project/zones/zone/disks/disk",
		},
		{
			name: "alpha project",
			in:   "https://www.googleapis.com/compute/alpha/projects/alphaproject/zones/zone/disks/disk",
			want: "projects/alphaproject/zones/zone/disks/disk",
		},
		{
			name: "beta project",
			in:   "https://www.googleapis.com/compute/alpha/projects/betabeta/zones/zone/disks/disk",
			want: "projects/betabeta/zones/zone/disks/disk",
		},
		{
			name: "v1 project",
			in:   "https://www.googleapis.com/compute/alpha/projects/projectv1/zones/zone/disks/disk",
			want: "projects/projectv1/zones/zone/disks/disk",
		},
		{
			name: "random host",
			in:   "https://npr.org/compute/v1/projects/project/zones/zone/disks/disk",
			want: "projects/project/zones/zone/disks/disk",
		},
		{
			name:  "no prefix",
			in:    "projects/project/zones/zone/disks/disk",
			error: true,
		},
		{
			name:  "bad scheme",
			in:    "ftp://www.googleapis.com/compute/v1/projects/project/zones/zone/disks/disk",
			error: true,
		},
		{
			name:  "insecure scheme",
			in:    "http://www.googleapis.com/compute/v1/projects/project/zones/zone/disks/disk",
			error: true,
		},
		{
			name:  "bad service",
			in:    "https://www.googleapis.com/computers/v1/projects/project/zones/zone/disks/disk",
			error: true,
		},
		{
			name:  "bad version",
			in:    "https://www..googleapis.com/compute/zeta/projects/project/zones/zone/disks/disk",
			error: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := getResourceId(tc.in)
			if tc.error {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error %v", err)
				} else if got != tc.want {
					t.Errorf("Expected cleaned self link: %v, got: %v", tc.want, got)
				}
			}
		})
	}
}

func TestCreateConfidentialVolume(t *testing.T) {
	// Define test cases
	testCases := []struct {
		name       string
		volKey     *meta.Key
		req        *csi.CreateVolumeRequest
		diskStatus string
		expErrCode codes.Code
	}{
		{
			name:       "create confidential volume from snapshot",
			volKey:     meta.ZonalKey("my-disk", zone),
			diskStatus: "READY",
			req: &csi.CreateVolumeRequest{
				Name:               "test-volume",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters: map[string]string{
					common.ParameterKeyEnableConfidentialCompute: "true",
					common.ParameterKeyDiskEncryptionKmsKey:      testDiskEncryptionKmsKey,
					common.ParameterKeyType:                      "hyperdisk-balanced",
				},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: testSnapshotID,
						},
					},
				},
			},
		},
		{
			name:       "create volume from snapshot with confidential-compute disabled",
			volKey:     meta.ZonalKey("my-disk", zone),
			diskStatus: "READY",
			req: &csi.CreateVolumeRequest{
				Name:               "test-volume",
				CapacityRange:      stdCapRange,
				VolumeCapabilities: stdVolCaps,
				Parameters: map[string]string{
					common.ParameterKeyEnableConfidentialCompute: "false",
					common.ParameterKeyType:                      "hyperdisk-balanced",
				},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: testSnapshotID,
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		t.Run(tc.name, func(t *testing.T) {
			fcp, err := gce.CreateFakeCloudProvider(project, zone, nil)
			if err != nil {
				t.Fatalf("Failed to create fake cloud provider: %v", err)
			}
			// Setup new driver each time so no interference
			gceDriver := initGCEDriverWithCloudProvider(t, fcp)

			if tc.req.VolumeContentSource.GetType() != nil {
				snapshotParams, err := common.ExtractAndDefaultSnapshotParameters(nil, gceDriver.name, nil)
				if err != nil {
					t.Errorf("Got error extracting snapshot parameters: %v", err)
				}
				if snapshotParams.SnapshotType == common.DiskSnapshotType {
					fcp.CreateSnapshot(context.Background(), project, tc.volKey, name, snapshotParams)
				} else {
					t.Fatalf("No volume source mentioned in snapshot parameters %v", snapshotParams)
				}
			}

			// Start Test
			resp, err := gceDriver.cs.CreateVolume(context.Background(), tc.req)
			if err != nil {
				serverError, ok := status.FromError(err)
				if !ok {
					t.Fatalf("Could not get error status code from err: %v", serverError)
				}
				t.Errorf("Recieved error %v", serverError)
			}

			volumeId := resp.GetVolume().VolumeId
			project, volumeKey, err := common.VolumeIDToKey(volumeId)
			createdDisk, err := fcp.GetDisk(context.Background(), project, volumeKey, gce.GCEAPIVersionBeta)
			if err != nil {
				t.Fatalf("Get Disk failed for created disk with error: %v", err)
			}
			val, ok := tc.req.Parameters[common.ParameterKeyEnableConfidentialCompute]
			if ok && val != strconv.FormatBool(createdDisk.GetEnableConfidentialCompute()) {
				t.Fatalf("Confidential disk parameter does not match with created disk: %v Got error %v", createdDisk.GetEnableConfidentialCompute(), err)
			}
			t.Logf("Created disk for confidentialCompute %v with parametrs, %v", createdDisk.GetEnableConfidentialCompute(), tc.req.Parameters)
		})
	}
}

func backoffDriver(t *testing.T, config *backoffDriverConfig) *GCEDriver {
	var cloudDisks []*gce.CloudDisk
	if !config.mockMissingDisk {
		cloudDisks = append(cloudDisks, createZonalCloudDisk(name))
	}
	fcp, err := gce.CreateFakeCloudProvider(project, zone, cloudDisks)
	if err != nil {
		t.Fatalf("Failed to create fake cloud provider: %v", err)
	}
	instance := &compute.Instance{
		Name:  node,
		Disks: config.attachedDisks,
	}
	if !config.mockMissingInstance {
		fcp.InsertInstance(instance, zone, node)
	}

	driver := GetGCEDriver()
	driver.cs = &GCEControllerServer{
		Driver:            driver,
		volumeEntriesSeen: map[string]int{},
		volumeLocks:       common.NewVolumeLocks(),
		errorBackoff:      newFakeCSIErrorBackoff(config.clock),
	}

	driver.cs.CloudProvider = fcp
	if config.readyToExecute != nil {
		driver.cs.CloudProvider = &gce.FakeBlockingCloudProvider{
			FakeCloudProvider: fcp,
			ReadyToExecute:    config.readyToExecute,
		}
	}
	return driver
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

func isInternalError(err error) bool {
	if err == nil {
		return false
	}

	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	return st.Code().String() == "Internal"
}

func googleapiErrContainsReason(err *googleapi.Error, reason string) bool {
	for _, errItem := range err.Errors {
		if strings.Contains(errItem.Reason, reason) {
			return true
		}
	}
	return false
}
