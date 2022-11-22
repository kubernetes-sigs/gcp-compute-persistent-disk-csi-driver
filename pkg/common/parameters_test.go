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
)

func TestExtractAndDefaultParameters(t *testing.T) {
	tests := []struct {
		name         string
		parameters   map[string]string
		labels       map[string]string
		expectParams DiskParameters
		expectErr    bool
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
			},
		},
		{
			name:       "specified empties",
			parameters: map[string]string{ParameterKeyType: "", ParameterKeyReplicationType: "", ParameterKeyDiskEncryptionKmsKey: "", ParameterKeyLabels: ""},
			labels:     map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "",
				Tags:                 map[string]string{},
				Labels:               map[string]string{},
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
			parameters: map[string]string{ParameterKeyType: "pd-ssd", ParameterKeyReplicationType: "regional-pd", ParameterKeyDiskEncryptionKmsKey: "foo/key", ParameterKeyLabels: "key1=value1,key2=value2"},
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
				Labels:               map[string]string{labelKeyCreatedForClaimName: "testPVCName", labelKeyCreatedForClaimNamespace: "testPVCNamespace", labelKeyCreatedForVolumeName: "testPVName"},
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
			},
		},
		{
			name:       "PVC labels",
			parameters: map[string]string{ParameterKeyPVCName: "testPVCName", ParameterKeyPVCNamespace: "testPVCNamespace", ParameterKeyPVName: "testPVName"},
			labels:     map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "",
				Tags:                 map[string]string{tagKeyCreatedForClaimName: "testPVCName", tagKeyCreatedForClaimNamespace: "testPVCNamespace", tagKeyCreatedForVolumeName: "testPVName", tagKeyCreatedBy: "testDriver"},
				Labels:               map[string]string{labelKeyCreatedForClaimName: "testPVCName", labelKeyCreatedForClaimNamespace: "testPVCNamespace", labelKeyCreatedForVolumeName: "testPVName"},
			},
		},
		{
			name:       "PVC labels-override",
			parameters: map[string]string{ParameterKeyPVCName: "testPVCName", ParameterKeyPVCNamespace: "testPVCNamespace", ParameterKeyPVName: "testPVName"},
			labels:     map[string]string{labelKeyCreatedForClaimNamespace: "test-override"},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "",
				Tags:                 map[string]string{tagKeyCreatedForClaimName: "testPVCName", tagKeyCreatedForClaimNamespace: "testPVCNamespace", tagKeyCreatedForVolumeName: "testPVName", tagKeyCreatedBy: "testDriver"},
				Labels:               map[string]string{labelKeyCreatedForClaimName: "testPVCName", labelKeyCreatedForClaimNamespace: "testPVCNamespace", labelKeyCreatedForVolumeName: "testPVName"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ExtractAndDefaultParameters(tc.parameters, "testDriver", tc.labels)
			if gotErr := err != nil; gotErr != tc.expectErr {
				t.Fatalf("ExtractAndDefaultParameters(%+v) = %v; expectedErr: %v", tc.parameters, err, tc.expectErr)
			}
			if err != nil {
				return
			}

			if !reflect.DeepEqual(p, tc.expectParams) {
				t.Errorf("ExtractAndDefaultParameters(%+v) = %v; expected params: %v", tc.parameters, p, tc.expectParams)
			}
		})
	}
}

// Currently the storage-locations parameter is tested in utils_test/TestSnapshotStorageLocations.
// Here we just test other parameters.
func TestSnapshotParameters(t *testing.T) {
	tests := []struct {
		desc                    string
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
				Labels: map[string]string{"label-1": "value-a", "key1": "value1"},
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
			p, err := ExtractAndDefaultSnapshotParameters(tc.parameters, "test-driver")
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
