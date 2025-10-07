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
	"testing"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/parameters"
)

var (
	stdVolCap = &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
	stdVolCaps = []*csi.VolumeCapability{
		stdVolCap,
	}
)

func createVolumeCapabilities(am csi.VolumeCapability_AccessMode_Mode) []*csi.VolumeCapability {
	return []*csi.VolumeCapability{
		createVolumeCapability(am),
	}
}

func createVolumeCapability(am csi.VolumeCapability_AccessMode_Mode) *csi.VolumeCapability {
	return &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: am,
		},
	}
}

func createBlockVolumeCapabilities(am csi.VolumeCapability_AccessMode_Mode) []*csi.VolumeCapability {
	return []*csi.VolumeCapability{
		createBlockVolumeCapability(am),
	}
}

func createBlockVolumeCapability(am csi.VolumeCapability_AccessMode_Mode) *csi.VolumeCapability {
	return &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: am,
		},
	}
}

func TestValidateVolumeCapabilities(t *testing.T) {
	testCases := []struct {
		name   string
		vc     []*csi.VolumeCapability
		expErr bool
	}{
		{
			name: "success with empty capabilities",
			vc:   []*csi.VolumeCapability{},
		},
		{
			name: "fail with capabilities no access mode",
			vc: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
				},
			},
			expErr: true,
		},
		{
			name: "fail with capabilities no mode",
			vc: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{},
				},
			},
			expErr: true,
		},
		{
			name: "fail with capabilities no access type",
			vc: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			expErr: true,
		},
		{
			name: "success with mount/SINGLE_NODE_WRITER capabilities",
			vc:   createVolumeCapabilities(csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER),
		},
		{
			name: "success with mount/SINGLE_NODE_READER_ONLY capabilities",
			vc:   createVolumeCapabilities(csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY),
		},
		{
			name: "success with mount/MULTI_NODE_READER_ONLY capabilities",
			vc:   createVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY),
		},
		{
			name:   "fail with mount/MULTI_NODE_SINGLE_WRITER capabilities",
			vc:     createVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER),
			expErr: true,
		},
		{
			name:   "fail with mount/MULTI_NODE_MULTI_WRITER capabilities",
			vc:     createVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER),
			expErr: true,
		},
		{
			name:   "fail with mount/UNKNOWN capabilities",
			vc:     createVolumeCapabilities(csi.VolumeCapability_AccessMode_UNKNOWN),
			expErr: true,
		},
		{
			name: "success with block capabilities",
			vc:   createBlockVolumeCapabilities(csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER),
		},
		{
			name: "success with block/MULTI_NODE_MULTI_WRITER capabilities",
			vc:   createBlockVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER),
		},
		{
			name:   "fail with block/MULTI_NODE_SINGLE_WRITER capabilities",
			vc:     createBlockVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER),
			expErr: true,
		},
		{
			name: "success with reader + writer capabilities",
			vc: []*csi.VolumeCapability{
				createVolumeCapability(csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY),
				createVolumeCapability(csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER),
			},
		},
		{
			name: "success with different reader capabilities",
			vc: []*csi.VolumeCapability{
				createVolumeCapability(csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY),
				createVolumeCapability(csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY),
			},
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		err := validateVolumeCapabilities(tc.vc)
		if tc.expErr && err == nil {
			t.Fatalf("Expected error but didn't get any")
		}
		if !tc.expErr && err != nil {
			t.Fatalf("Did not expect error but got: %v", err)
		}
	}
}

func TestGetMultiWriterFromCapabilities(t *testing.T) {
	testCases := []struct {
		name   string
		vc     []*csi.VolumeCapability
		expVal bool
		expErr bool
	}{
		{
			name:   "false with empty capabilities",
			vc:     []*csi.VolumeCapability{},
			expVal: false,
		},
		{
			name: "fail with capabilities no access mode",
			vc: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
				},
			},
			expErr: true,
		},
		{
			name:   "false with mount/SINGLE_NODE_WRITER capabilities",
			vc:     createVolumeCapabilities(csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER),
			expVal: false,
		},
		{
			name:   "true with block/MULTI_NODE_MULTI_WRITER capabilities",
			vc:     createBlockVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER),
			expVal: true,
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		val, err := getMultiWriterFromCapabilities(tc.vc)
		if tc.expErr && err == nil {
			t.Fatalf("Expected error but didn't get any")
		}
		if !tc.expErr && err != nil {
			t.Fatalf("Did not expect error but got: %v", err)
		}
		if err != nil {
			if tc.expVal != val {
				t.Fatalf("Expected '%t' but got '%t'", tc.expVal, val)
			}
		}
	}
}

func TestGetReadOnlyFromCapabilities(t *testing.T) {
	testCases := []struct {
		name   string
		vc     []*csi.VolumeCapability
		expVal bool
		expErr bool
	}{
		{
			name:   "false with empty capabilities",
			vc:     []*csi.VolumeCapability{},
			expVal: false,
		},
		{
			name: "fail with capabilities no access mode",
			vc: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
				},
			},
			expErr: true,
		},
		{
			name:   "false with SINGLE_NODE_WRITER capabilities",
			vc:     createVolumeCapabilities(csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER),
			expVal: false,
		},
		{
			name:   "true with MULTI_NODE_READER_ONLY capabilities",
			vc:     createBlockVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY),
			expVal: true,
		},
		{
			name:   "true with SINGLE_NODE_READER_ONLY capabilities",
			vc:     createVolumeCapabilities(csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY),
			expVal: true,
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		val, err := getReadOnlyFromCapabilities(tc.vc)
		if tc.expErr && err == nil {
			t.Fatalf("Expected error but didn't get any")
		}
		if !tc.expErr && err != nil {
			t.Fatalf("Did not expect error but got: %v", err)
		}
		if err != nil {
			if tc.expVal != val {
				t.Fatalf("Expected '%t' but got '%t'", tc.expVal, val)
			}
		}
	}
}

func TestValidateStoragePools(t *testing.T) {
	testCases := []struct {
		name       string
		req        *csi.CreateVolumeRequest
		params     parameters.DiskParameters
		project    string
		expErr     error
		enableHdHA bool
	}{
		{
			name: "success with storage pools not enabled",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
			},
			params: parameters.DiskParameters{
				DiskType: "hyperdisk-balanced",
			},
			expErr: nil,
		},
		{
			name: "success with nil CreateVolumeReq",
			req:  nil,
			params: parameters.DiskParameters{
				DiskType: "hyperdisk-balanced",
			},
			expErr: nil,
		},
		{
			name: "fail storage pools with confidential storage",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
			},
			params: parameters.DiskParameters{
				DiskType: "hyperdisk-balanced",
				StoragePools: []parameters.StoragePool{
					{
						Project:      "test-project",
						Zone:         "us-central1-a",
						Name:         "storagePool-1",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
					{
						Project:      "test-project",
						Zone:         "us-central1-b",
						Name:         "storagePool-2",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
				},
				EnableConfidentialCompute: true,
			},
			project: "test-project",
			expErr:  fmt.Errorf("storage pools do not support confidential storage"),
		},
		{
			name: "fail storage pools with disk type other than HdB/HdT",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
			},
			params: parameters.DiskParameters{
				DiskType: "pd-balanced",
				StoragePools: []parameters.StoragePool{
					{
						Project:      "test-project",
						Zone:         "us-central1-a",
						Name:         "storagePool-1",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
					{
						Project:      "test-project",
						Zone:         "us-central1-b",
						Name:         "storagePool-2",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
				},
			},
			project: "test-project",
			expErr:  fmt.Errorf("invalid disk-type: \"pd-balanced\". storage pools only support hyperdisk-balanced or hyperdisk-throughput"),
		},
		{
			name: "fail storage pools with regional PD",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
			},
			params: parameters.DiskParameters{
				DiskType:        "hyperdisk-balanced",
				ReplicationType: "regional-pd",
				StoragePools: []parameters.StoragePool{
					{
						Project:      "test-project",
						Zone:         "us-central1-a",
						Name:         "storagePool-1",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
					{
						Project:      "test-project",
						Zone:         "us-central1-b",
						Name:         "storagePool-2",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
				},
			},
			project: "test-project",
			expErr:  fmt.Errorf("storage pools do not support regional disks"),
		},
		{
			name: "fail storage pools with HdHA, even when HdHA is allowed",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
			},
			params: parameters.DiskParameters{
				DiskType: "hyperdisk-balanced-high-availability",
				StoragePools: []parameters.StoragePool{
					{
						Project:      "test-project",
						Zone:         "us-central1-a",
						Name:         "storagePool-1",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
					{
						Project:      "test-project",
						Zone:         "us-central1-b",
						Name:         "storagePool-2",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
				},
			},
			project:    "test-project",
			expErr:     fmt.Errorf("invalid disk-type: \"hyperdisk-balanced-high-availability\". storage pools only support hyperdisk-balanced or hyperdisk-throughput"),
			enableHdHA: true,
		},
		{
			name: "fail storage pools with HdHA when HdHA is not allowed",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
			},
			params: parameters.DiskParameters{
				DiskType: "hyperdisk-balanced-high-availability",
				StoragePools: []parameters.StoragePool{
					{
						Project:      "test-project",
						Zone:         "us-central1-a",
						Name:         "storagePool-1",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
					{
						Project:      "test-project",
						Zone:         "us-central1-b",
						Name:         "storagePool-2",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
				},
			},
			project: "test-project",
			expErr:  fmt.Errorf("invalid disk-type: \"hyperdisk-balanced-high-availability\". storage pools only support hyperdisk-balanced or hyperdisk-throughput"),
		},
		{
			name: "fail storage pools with disk clones",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: "projects/test-project/zones/us-central1-a/disks/disk-1",
						},
					},
				},
			},
			params: parameters.DiskParameters{
				DiskType: "hyperdisk-balanced",
				StoragePools: []parameters.StoragePool{
					{
						Project:      "test-project",
						Zone:         "us-central1-a",
						Name:         "storagePool-1",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
					{
						Project:      "test-project",
						Zone:         "us-central1-b",
						Name:         "storagePool-2",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
				},
			},
			project: "test-project",
			expErr:  fmt.Errorf("storage pools do not support disk clones"),
		},
		{
			name: "fail storage pools zones, requisite zones mismatch",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-a"},
						},
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-b"},
						},
					},
				},
			},
			params: parameters.DiskParameters{
				DiskType: "hyperdisk-balanced",
				StoragePools: []parameters.StoragePool{
					{
						Project:      "test-project",
						Zone:         "us-central1-a",
						Name:         "storagePool-1",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
					{
						Project:      "test-project",
						Zone:         "us-central1-b",
						Name:         "storagePool-2",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
				},
			},
			project: "test-project",
			expErr:  fmt.Errorf("failed to validate storage pools zones: requisite topologies must match storage pools zones. requisite zones: [us-central1-a], storage pools zones: [us-central1-a us-central1-b]"),
		},
		{
			name: "fail storage pools cross-project usage",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-a"},
						},
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-b"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-a"},
						},
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-b"},
						},
					},
				},
			},
			params: parameters.DiskParameters{
				DiskType: "hyperdisk-balanced",
				StoragePools: []parameters.StoragePool{
					{
						Project:      "test-project",
						Zone:         "us-central1-a",
						Name:         "storagePool-1",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
					{
						Project:      "test-project",
						Zone:         "us-central1-b",
						Name:         "storagePool-2",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
				},
			},
			project: "other-project",
			expErr:  fmt.Errorf("failed to validate storage pools projects: cross-project storage pools usage is not supported. Trying to CreateVolume in project \"other-project\" with storage pools in projects [test-project]"),
		},
		{
			name: "success validateStoragePools",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-a"},
						},
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-b"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-a"},
						},
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-b"},
						},
					},
				},
			},
			params: parameters.DiskParameters{
				DiskType: "hyperdisk-balanced",
				StoragePools: []parameters.StoragePool{
					{
						Project:      "test-project",
						Zone:         "us-central1-a",
						Name:         "storagePool-1",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
					{
						Project:      "test-project",
						Zone:         "us-central1-b",
						Name:         "storagePool-2",
						ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
					},
				},
			},
			project: "test-project",
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		input := "validateStoragePools()"
		err := validateStoragePools(tc.req, tc.params, tc.project)
		if tc.expErr != nil && err == nil {
			t.Fatalf("%s didn't get any error, but expected error %v", input, tc.expErr)
		}
		if tc.expErr == nil && err != nil {
			t.Fatalf("%s got error %v, but didn't expect any error", input, err)
		}
		if err != nil && tc.expErr != nil {
			if diff := cmp.Diff(err.Error(), tc.expErr.Error()); diff != "" {
				t.Errorf("%s: -want, +got \n%s", input, diff)
			}
		}
	}
}

func TestValidateStoragePoolZones(t *testing.T) {
	testCases := []struct {
		name         string
		req          *csi.CreateVolumeRequest
		storagePools []parameters.StoragePool
		expErr       error
	}{
		{
			name: "fail with 2 storage pools in 1 zone",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			storagePools: []parameters.StoragePool{
				{
					Project:      "test-project",
					Zone:         "us-central1-a",
					Name:         "storagePool-1",
					ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
				},
				{
					Project:      "test-project",
					Zone:         "us-central1-a",
					Name:         "storagePool-2",
					ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
				},
			},
			expErr: fmt.Errorf("found multiple storage pools in zone us-central1-a. Only one storage pool per zone is allowed"),
		},
		{
			name: "fail with requisite topology with no segments",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{{}},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			storagePools: []parameters.StoragePool{
				{
					Project:      "test-project",
					Zone:         "us-central1-a",
					Name:         "storagePool-1",
					ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
				},
			},
			expErr: fmt.Errorf("topologies specified but no segments"),
		},
		{
			name: "fail with requisite zones does not match storage pools zones",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-a"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-a"},
						},
					},
				},
			},
			storagePools: []parameters.StoragePool{
				{
					Project:      "test-project",
					Zone:         "us-central1-b",
					Name:         "storagePool-1",
					ResourceName: "projects/test-project/zones/us-central1-b/storagePools/storagePool-1",
				},
			},
			expErr: fmt.Errorf("requisite topologies must match storage pools zones. requisite zones: [us-central1-a], storage pools zones: [us-central1-b]"),
		},
		{
			name: "success validateStoragePoolZones",
			req: &csi.CreateVolumeRequest{
				Name: "test-name",
				AccessibilityRequirements: &csi.TopologyRequirement{
					Requisite: []*csi.Topology{
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-a"},
						},
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-b"},
						},
					},
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-a"},
						},
						{
							Segments: map[string]string{constants.TopologyKeyZone: "us-central1-b"},
						},
					},
				},
			},
			storagePools: []parameters.StoragePool{
				{
					Project:      "test-project",
					Zone:         "us-central1-a",
					Name:         "storagePool-1",
					ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
				},
				{
					Project:      "test-project",
					Zone:         "us-central1-b",
					Name:         "storagePool-2",
					ResourceName: "projects/test-project/zones/us-central1-a/storagePools/storagePool-1",
				},
			},
			expErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		err := validateStoragePoolZones(tc.req, tc.storagePools)
		input := "validateStoragePoolZones()"
		if tc.expErr != nil && err == nil {
			t.Fatalf("%s didn't get any error, but expected error %v", input, tc.expErr)
		}
		if tc.expErr == nil && err != nil {
			t.Fatalf("%s got error %v, but didn't expect any error", input, err)
		}
		if err != nil && tc.expErr != nil {
			if diff := cmp.Diff(err.Error(), tc.expErr.Error()); diff != "" {
				t.Errorf("%s: -want, +got \n%s", input, diff)
			}
		}
	}
}

func TestGetHyperdiskAccessModeFromCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name    string
		vcs     []*csi.VolumeCapability
		want    string
		wantErr bool
	}{
		{
			name:    "error with nil vcs",
			wantErr: true,
		},
		{
			name:    "error with no vcs",
			vcs:     []*csi.VolumeCapability{},
			wantErr: true,
		},
		{
			name: "error with nil access mode",
			vcs: []*csi.VolumeCapability{
				{},
			},
			wantErr: true,
		},
		{
			name: "error with unsupported CSI access mode",
			vcs: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "success getting ROX",
			vcs: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
					},
				},
			},
			want: constants.GCEReadOnlyManyAccessMode,
		},
		{
			name: "success getting RWO",
			vcs: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			want: constants.GCEReadWriteOnceAccessMode,
		},
		{
			name: "success getting RWX",
			vcs: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
			},
			want: constants.GCEReadWriteManyAccessMode,
		},
	} {
		t.Logf("Running test: %v", tc.name)
		am, err := getHyperdiskAccessModeFromCapabilities(tc.vcs)
		if err != nil {
			if !tc.wantErr {
				t.Errorf("unexpected error: %v", err)
			}
			continue
		}
		if am != tc.want {
			t.Errorf("want %s, got %s", tc.want, am)
		}
	}
}

func TestIsDataCacheEnabledNodePool(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		nodeName             string
		wantDataCacheEnabled bool
		dataCacheFlag        bool
		wantErr              bool
	}{
		{
			// Valid nod ename tries to fetch the data cache count from node labels resulting in an error
			name:                 "node name is provided",
			nodeName:             "gke-node-some-name",
			dataCacheFlag:        true,
			wantDataCacheEnabled: true,
			wantErr:              true,
		},
		{
			name:                 "no node name provided",
			nodeName:             "",
			dataCacheFlag:        true,
			wantDataCacheEnabled: false,
		},
		{
			name:                 "test node",
			nodeName:             constants.TestNode,
			dataCacheFlag:        true,
			wantDataCacheEnabled: true,
		},
		{
			name:                 "node name provided but data cache feature disabled",
			nodeName:             "",
			dataCacheFlag:        false,
			wantDataCacheEnabled: false,
		},
	} {
		t.Logf("Running test: %v", tc.name)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		gotDataCacheEnabled, err := IsDataCacheEnabledNodePool(ctx, tc.nodeName, tc.dataCacheFlag)
		if err != nil {
			if !tc.wantErr {
				t.Errorf("unexpected error, got %v", err)
			}
			continue
		}
		if gotDataCacheEnabled != tc.wantDataCacheEnabled {
			t.Errorf("want %t, got %t", tc.wantDataCacheEnabled, gotDataCacheEnabled)
		}
	}
}
