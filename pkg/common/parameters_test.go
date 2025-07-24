/*
Copyright 2020 The Kubernetes Authors.

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

package common

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestExtractAndDefaultParameters(t *testing.T) {
	tests := []struct {
		name                  string
		parameters            map[string]string
		labels                map[string]string
		enableStoragePools    bool
		enableDataCache       bool
		enableMultiZone       bool
		enableHdHA            bool
		extraTags             map[string]string
		expectParams          DiskParameters
		expectDataCacheParams DataCacheParameters
		expectErr             bool
	}{
		{
			name:       "defaults",
			parameters: map[string]string{},
			labels:     map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "",
				Tags:                 map[string]string{},
				Labels:               map[string]string{},
				ResourceTags:         map[string]string{},
			},
		},
		{
			name:       "specified empties",
			parameters: map[string]string{ParameterKeyType: "", ParameterKeyReplicationType: "", ParameterKeyDiskEncryptionKmsKey: "", ParameterKeyLabels: "", ParameterKeyResourceTags: ""},
			labels:     map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "",
				Tags:                 map[string]string{},
				Labels:               map[string]string{},
				ResourceTags:         map[string]string{},
			},
		},
		{
			name:       "random keys",
			parameters: map[string]string{ParameterKeyType: "", "foo": "", ParameterKeyDiskEncryptionKmsKey: "", ParameterKeyLabels: ""},
			labels:     map[string]string{},
			expectErr:  true,
		},
		{
			name:       "values from parameters",
			parameters: map[string]string{ParameterKeyType: "pd-ssd", ParameterKeyReplicationType: "regional-pd", ParameterKeyDiskEncryptionKmsKey: "foo/key", ParameterKeyLabels: "key1=value1,key2=value2", ParameterKeyResourceTags: "parent1/key1/value1,parent2/key2/value2"},
			labels:     map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-ssd",
				ReplicationType:      "regional-pd",
				DiskEncryptionKMSKey: "foo/key",
				Tags:                 map[string]string{},
				Labels: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				ResourceTags: map[string]string{
					"parent1/key1": "value1",
					"parent2/key2": "value2",
				},
			},
		},
		{
			name:       "values from parameters, checking pd-extreme",
			parameters: map[string]string{ParameterKeyType: "pd-extreme", ParameterKeyReplicationType: "none", ParameterKeyDiskEncryptionKmsKey: "foo/key", ParameterKeyLabels: "key1=value1,key2=value2", ParameterKeyResourceTags: "parent1/key1/value1,parent2/key2/value2", ParameterKeyProvisionedIOPSOnCreate: "10k"},
			labels:     map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-extreme",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "foo/key",
				Tags:                 map[string]string{},
				Labels: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				ResourceTags: map[string]string{
					"parent1/key1": "value1",
					"parent2/key2": "value2",
				},
				ProvisionedIOPSOnCreate: 10000,
			},
		},
		{
			name:       "values from parameters, checking hyperdisk-throughput",
			parameters: map[string]string{ParameterKeyType: "hyperdisk-throughput", ParameterKeyReplicationType: "none", ParameterKeyDiskEncryptionKmsKey: "foo/key", ParameterKeyLabels: "key1=value1,key2=value2", ParameterKeyResourceTags: "parent1/key1/value1,parent2/key2/value2", ParameterKeyProvisionedThroughputOnCreate: "1000Mi"},
			labels:     map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "hyperdisk-throughput",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "foo/key",
				Tags:                 map[string]string{},
				Labels: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				ResourceTags: map[string]string{
					"parent1/key1": "value1",
					"parent2/key2": "value2",
				},
				ProvisionedThroughputOnCreate: 1000,
			},
		},
		{
			name:       "values from parameters, checking balanced pd",
			parameters: map[string]string{ParameterKeyType: "pd-balanced", ParameterKeyReplicationType: "regional-pd", ParameterKeyDiskEncryptionKmsKey: "foo/key"},
			labels:     map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-balanced",
				ReplicationType:      "regional-pd",
				DiskEncryptionKMSKey: "foo/key",
				Tags:                 map[string]string{},
				Labels:               map[string]string{},
				ResourceTags:         map[string]string{},
			},
		},
		{
			name:       "partial spec",
			parameters: map[string]string{ParameterKeyDiskEncryptionKmsKey: "foo/key"},
			labels:     map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "foo/key",
				Tags:                 map[string]string{},
				Labels:               map[string]string{},
				ResourceTags:         map[string]string{},
			},
		},
		{
			name:       "tags",
			parameters: map[string]string{ParameterKeyPVCName: "testPVCName", ParameterKeyPVCNamespace: "testPVCNamespace", ParameterKeyPVName: "testPVName"},
			labels:     map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "",
				Tags:                 map[string]string{tagKeyCreatedForClaimName: "testPVCName", tagKeyCreatedForClaimNamespace: "testPVCNamespace", tagKeyCreatedForVolumeName: "testPVName", tagKeyCreatedBy: "testDriver"},
				Labels:               map[string]string{},
				ResourceTags:         map[string]string{},
			},
		},
		{
			name:       "extra labels",
			parameters: map[string]string{},
			labels:     map[string]string{"label-1": "label-value-1", "label-2": "label-value-2"},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "",
				Tags:                 map[string]string{},
				Labels:               map[string]string{"label-1": "label-value-1", "label-2": "label-value-2"},
				ResourceTags:         map[string]string{},
			},
		},
		{
			name:       "parameter and extra labels",
			parameters: map[string]string{ParameterKeyLabels: "key1=value1,key2=value2"},
			labels:     map[string]string{"label-1": "label-value-1", "label-2": "label-value-2"},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "",
				Tags:                 map[string]string{},
				Labels:               map[string]string{"key1": "value1", "key2": "value2", "label-1": "label-value-1", "label-2": "label-value-2"},
				ResourceTags:         map[string]string{},
			},
		},
		{
			name:       "parameter and extra labels, overlapping",
			parameters: map[string]string{ParameterKeyLabels: "label-1=value-a,key1=value1"},
			labels:     map[string]string{"label-1": "label-value-1", "label-2": "label-value-2"},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "",
				Tags:                 map[string]string{},
				Labels:               map[string]string{"key1": "value1", "label-1": "value-a", "label-2": "label-value-2"},
				ResourceTags:         map[string]string{},
			},
		},
		{
			name:       "extra tags",
			parameters: map[string]string{},
			extraTags:  map[string]string{"parent1/key1": "value1", "parent2/key2": "value2"},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "",
				Tags:                 map[string]string{},
				Labels:               map[string]string{},
				ResourceTags: map[string]string{
					"parent1/key1": "value1",
					"parent2/key2": "value2",
				},
			},
		},
		{
			name:       "resource-tags parameter and extra tags",
			parameters: map[string]string{ParameterKeyResourceTags: "parent3/key3/value3"},
			extraTags:  map[string]string{"parent1/key1": "value1", "parent2/key2": "value2"},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "",
				Tags:                 map[string]string{},
				Labels:               map[string]string{},
				ResourceTags: map[string]string{
					"parent1/key1": "value1",
					"parent2/key2": "value2",
					"parent3/key3": "value3",
				},
			},
		},
		{
			name:       "resource-tags parameter and extra labels, overlapping",
			parameters: map[string]string{ParameterKeyResourceTags: "parent1/key1/value-a"},
			extraTags:  map[string]string{"parent1/key1": "value-b", "parent2/key2": "value2"},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "",
				Tags:                 map[string]string{},
				Labels:               map[string]string{},
				ResourceTags: map[string]string{
					"parent1/key1": "value-a",
					"parent2/key2": "value2",
				},
			},
		},
		{
			name:       "availability class parameters",
			parameters: map[string]string{ParameterAvailabilityClass: ParameterRegionalHardFailoverClass},
			expectParams: DiskParameters{
				DiskType:        "pd-standard",
				ReplicationType: "none",
				ForceAttach:     true,
				Tags:            map[string]string{},
				Labels:          map[string]string{},
				ResourceTags:    map[string]string{},
			},
		},
		{
			name:       "no force attach parameters",
			parameters: map[string]string{ParameterAvailabilityClass: ParameterNoAvailabilityClass},
			expectParams: DiskParameters{
				DiskType:        "pd-standard",
				ReplicationType: "none",
				Tags:            map[string]string{},
				Labels:          map[string]string{},
				ResourceTags:    map[string]string{},
			},
		},
		{
			name:               "storage pool parameters",
			enableStoragePools: true,
			parameters:         map[string]string{ParameterKeyType: "hyperdisk-balanced", ParameterKeyStoragePools: "projects/my-project/zones/us-central1-a/storagePools/storagePool-1,projects/my-project/zones/us-central1-b/storagePools/storagePool-2"},
			labels:             map[string]string{},
			expectParams: DiskParameters{
				DiskType:        "hyperdisk-balanced",
				ReplicationType: "none",
				Tags:            map[string]string{},
				Labels:          map[string]string{},
				ResourceTags:    map[string]string{},
				StoragePools: []StoragePool{
					{
						Project:      "my-project",
						Zone:         "us-central1-a",
						Name:         "storagePool-1",
						ResourceName: "projects/my-project/zones/us-central1-a/storagePools/storagePool-1",
					},
					{
						Project:      "my-project",
						Zone:         "us-central1-b",
						Name:         "storagePool-2",
						ResourceName: "projects/my-project/zones/us-central1-b/storagePools/storagePool-2",
					},
				},
			},
		},
		{
			name:               "invalid storage pool parameters, starts with /projects instead of projects",
			enableStoragePools: true,
			parameters:         map[string]string{ParameterKeyType: "hyperdisk-balanced", ParameterKeyStoragePools: "/projects/my-project/zones/us-central1-a/storagePools/storagePool-1"},
			labels:             map[string]string{},
			expectErr:          true,
		},
		{
			name:               "invalid storage pool parameters, missing projects",
			enableStoragePools: true,
			parameters:         map[string]string{ParameterKeyType: "hyperdisk-balanced", ParameterKeyStoragePools: "zones/us-central1-a/storagePools/storagePool-1"},
			labels:             map[string]string{},
			expectErr:          true,
		},
		{
			name:               "invalid storage pool parameters, missing zones",
			enableStoragePools: true,
			parameters:         map[string]string{ParameterKeyType: "hyperdisk-balanced", ParameterKeyStoragePools: "projects/my-project/storagePools/storagePool-1"},
			labels:             map[string]string{},
			expectErr:          true,
		},
		{
			name:               "invalid storage pool parameters, duplicate projects",
			enableStoragePools: true,
			parameters:         map[string]string{ParameterKeyType: "hyperdisk-balanced", ParameterKeyStoragePools: "projects/my-project/projects/my-project/storagePools/storagePool-1"},
			labels:             map[string]string{},
			expectErr:          true,
		},
		{
			name:               "invalid storage pool parameters, duplicate zones",
			enableStoragePools: true,
			parameters:         map[string]string{ParameterKeyType: "hyperdisk-balanced", ParameterKeyStoragePools: "zones/us-central1-a/zones/us-central1-a/storagePools/storagePool-1"},
			labels:             map[string]string{},
			expectErr:          true,
		},
		{
			name:               "invalid storage pool parameters, duplicate storagePools",
			enableStoragePools: true,
			parameters:         map[string]string{ParameterKeyType: "hyperdisk-balanced", ParameterKeyStoragePools: "projects/my-project/storagePools/us-central1-a/storagePools/storagePool-1"},
			labels:             map[string]string{},
			expectErr:          true,
		},
		{
			name:               "storage pool parameters, enableStoragePools is false",
			enableStoragePools: false,
			parameters:         map[string]string{ParameterKeyType: "hyperdisk-balanced", ParameterKeyStoragePools: "projects/my-project/zones/us-central1-a/storagePools/storagePool-1,projects/my-project/zones/us-central1-b/storagePools/storagePool-2"},
			labels:             map[string]string{},
			expectErr:          true,
		},
		{
			name:            "data cache parameters - set default cache mode",
			enableDataCache: true,
			parameters:      map[string]string{ParameterKeyType: "pd-balanced", ParameterKeyReplicationType: "none", ParameterKeyDiskEncryptionKmsKey: "foo/key", ParameterKeyLabels: "key1=value1,key2=value2", ParameterKeyDataCacheSize: "1234Gi"},
			labels:          map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-balanced",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "foo/key",
				Tags:                 map[string]string{},
				Labels: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				ResourceTags: map[string]string{},
			},
			expectDataCacheParams: DataCacheParameters{
				DataCacheMode: DataCacheModeWriteThrough,
				DataCacheSize: "1234",
			},
		},
		{
			name:            "data cache parameters",
			enableDataCache: true,
			parameters:      map[string]string{ParameterKeyType: "pd-balanced", ParameterKeyReplicationType: "none", ParameterKeyDiskEncryptionKmsKey: "foo/key", ParameterKeyLabels: "key1=value1,key2=value2", ParameterKeyDataCacheSize: "1234Gi", ParameterKeyDataCacheMode: DataCacheModeWriteBack},
			labels:          map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-balanced",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "foo/key",
				Tags:                 map[string]string{},
				Labels: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				ResourceTags: map[string]string{},
			},
			expectDataCacheParams: DataCacheParameters{
				DataCacheMode: DataCacheModeWriteBack,
				DataCacheSize: "1234",
			},
		},
		{
			name:            "data cache parameters - enableDataCache is false",
			enableDataCache: false,
			parameters:      map[string]string{ParameterKeyType: "pd-balanced", ParameterKeyReplicationType: "none", ParameterKeyDiskEncryptionKmsKey: "foo/key", ParameterKeyLabels: "key1=value1,key2=value2", ParameterKeyDataCacheSize: "1234Gi", ParameterKeyDataCacheMode: DataCacheModeWriteBack},
			labels:          map[string]string{},
			expectErr:       true,
		},
		{
			name:            "multi-zone-enable parameters, multi-zone label is set, multi-zone feature enabled",
			parameters:      map[string]string{ParameterKeyType: "hyperdisk-ml", ParameterKeyEnableMultiZoneProvisioning: "true"},
			labels:          map[string]string{MultiZoneLabel: "true"},
			enableMultiZone: true,
			expectParams: DiskParameters{
				DiskType:              "hyperdisk-ml",
				ReplicationType:       "none",
				Tags:                  map[string]string{},
				Labels:                map[string]string{MultiZoneLabel: "true"},
				ResourceTags:          map[string]string{},
				MultiZoneProvisioning: true,
			},
		},
		{
			name:            "multi-zone-enable parameters, multi-zone label is false, multi-zone feature enabled",
			parameters:      map[string]string{ParameterKeyType: "hyperdisk-ml", ParameterKeyEnableMultiZoneProvisioning: "false"},
			enableMultiZone: true,
			expectParams: DiskParameters{
				DiskType:        "hyperdisk-ml",
				ReplicationType: "none",
				Tags:            map[string]string{},
				ResourceTags:    map[string]string{},
				Labels:          map[string]string{},
			},
		},
		{
			name:            "multi-zone-enable parameters, invalid value, multi-zone feature enabled",
			parameters:      map[string]string{ParameterKeyType: "hyperdisk-ml", ParameterKeyEnableMultiZoneProvisioning: "unknown"},
			enableMultiZone: true,
			expectErr:       true,
		},
		{
			name:       "multi-zone-enable parameters, multi-zone label is set, multi-zone feature disabled",
			parameters: map[string]string{ParameterKeyType: "hyperdisk-ml", ParameterKeyEnableMultiZoneProvisioning: "true"},
			expectErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pp := ParameterProcessor{
				DriverName:         "testDriver",
				EnableStoragePools: tc.enableStoragePools,
				EnableMultiZone:    tc.enableMultiZone,
			}
			p, d, err := pp.ExtractAndDefaultParameters(tc.parameters, tc.labels, tc.enableDataCache, tc.extraTags)
			if gotErr := err != nil; gotErr != tc.expectErr {
				t.Fatalf("ExtractAndDefaultParameters(%+v) = %v; expectedErr: %v", tc.parameters, err, tc.expectErr)
			}
			if err != nil {
				return
			}

			if diff := cmp.Diff(tc.expectParams, p); diff != "" {
				t.Errorf("ExtractAndDefaultParameters(%+v): -want, +got \n%s", tc.parameters, diff)
			}

			if diff := cmp.Diff(tc.expectDataCacheParams, d); diff != "" {
				t.Errorf("ExtractAndDefaultParameters(%+v) for data cache params: -want, +got \n%s", tc.parameters, diff)
			}
		})
	}
}

// Currently the storage-locations parameter is tested in utils_test/TestSnapshotStorageLocations.
// Here we just test other parameters.
func TestSnapshotParameters(t *testing.T) {
	tests := []struct {
		desc                    string
		extraTags               map[string]string
		parameters              map[string]string
		expectedSnapshotParames SnapshotParameters
		expectError             bool
	}{
		{
			desc: "valid parameter",
			parameters: map[string]string{
				ParameterKeyStorageLocations:          "ASIA ",
				ParameterKeySnapshotType:              "images",
				ParameterKeyImageFamily:               "test-family",
				ParameterKeyVolumeSnapshotName:        "snapshot-name",
				ParameterKeyVolumeSnapshotContentName: "snapshot-content-name",
				ParameterKeyVolumeSnapshotNamespace:   "snapshot-namespace",
				ParameterKeyLabels:                    "label-1=value-a,key1=value1",
				ParameterKeyResourceTags:              "parent1/key1/value1,parent2/key2/value2",
			},
			expectedSnapshotParames: SnapshotParameters{
				StorageLocations: []string{"asia"},
				SnapshotType:     DiskImageType,
				ImageFamily:      "test-family",
				Tags: map[string]string{
					tagKeyCreatedForSnapshotName:        "snapshot-name",
					tagKeyCreatedForSnapshotContentName: "snapshot-content-name",
					tagKeyCreatedForSnapshotNamespace:   "snapshot-namespace",
					tagKeyCreatedBy:                     "test-driver",
				},
				Labels:       map[string]string{"label-1": "value-a", "key1": "value1"},
				ResourceTags: map[string]string{"parent1/key1": "value1", "parent2/key2": "value2"},
			},
			expectError: false,
		},
		{
			desc:       "nil parameter",
			parameters: nil,
			expectedSnapshotParames: SnapshotParameters{
				StorageLocations: []string{},
				SnapshotType:     DiskSnapshotType,
				Tags:             make(map[string]string),
				Labels:           map[string]string{},
				ResourceTags:     map[string]string{},
			},
			expectError: false,
		},
		{
			desc:        "invalid snapshot type",
			parameters:  map[string]string{ParameterKeySnapshotType: "invalid-type"},
			expectError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			p, err := ExtractAndDefaultSnapshotParameters(tc.parameters, "test-driver", tc.extraTags)
			if err != nil && !tc.expectError {
				t.Errorf("Got error %v; expect no error", err)
			}
			if err == nil && tc.expectError {
				t.Error("Got no error; expect an error")
			}
			if err == nil && !reflect.DeepEqual(p, tc.expectedSnapshotParames) {
				t.Errorf("Got ExtractAndDefaultSnapshotParameters(%+v) = %+v; expect %+v", tc.parameters, p, tc.expectedSnapshotParames)
			}
		})
	}
}
