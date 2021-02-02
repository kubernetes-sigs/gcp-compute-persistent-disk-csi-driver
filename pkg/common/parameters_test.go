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
				Tags:                 make(map[string]string),
				Labels:               make(map[string]string),
			},
		},
		{
			name:       "specified empties",
			parameters: map[string]string{ParameterKeyType: "", ParameterKeyReplicationType: "", ParameterKeyDiskEncryptionKmsKey: ""},
			labels:     map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-standard",
				ReplicationType:      "none",
				DiskEncryptionKMSKey: "",
				Tags:                 make(map[string]string),
				Labels:               make(map[string]string),
			},
		},
		{
			name:       "random keys",
			parameters: map[string]string{ParameterKeyType: "", "foo": "", ParameterKeyDiskEncryptionKmsKey: ""},
			labels:     map[string]string{},
			expectErr:  true,
		},
		{
			name:       "real values",
			parameters: map[string]string{ParameterKeyType: "pd-ssd", ParameterKeyReplicationType: "regional-pd", ParameterKeyDiskEncryptionKmsKey: "foo/key"},
			labels:     map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-ssd",
				ReplicationType:      "regional-pd",
				DiskEncryptionKMSKey: "foo/key",
				Tags:                 make(map[string]string),
				Labels:               make(map[string]string),
			},
		},
		{
			name:       "real values, checking balanced pd",
			parameters: map[string]string{ParameterKeyType: "pd-balanced", ParameterKeyReplicationType: "regional-pd", ParameterKeyDiskEncryptionKmsKey: "foo/key"},
			labels:     map[string]string{},
			expectParams: DiskParameters{
				DiskType:             "pd-balanced",
				ReplicationType:      "regional-pd",
				DiskEncryptionKMSKey: "foo/key",
				Tags:                 make(map[string]string),
				Labels:               make(map[string]string),
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
				Tags:                 make(map[string]string),
				Labels:               make(map[string]string),
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
				Labels:               make(map[string]string),
			},
		},
		{
			name:       "labels",
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
