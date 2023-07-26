/*
Copyright 2023 The Kubernetes Authors.


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

package gcecloudprovider

import (
	"testing"

	computebeta "google.golang.org/api/compute/v0.beta"
	computev1 "google.golang.org/api/compute/v1"
)

func CreateDiskWithConfidentialCompute(betaDisk bool, confidentialCompute bool, diskType string) *CloudDisk {
	if betaDisk {
		return &CloudDisk{
			betaDisk: &computebeta.Disk{
				EnableConfidentialCompute: confidentialCompute,
				Type:                      diskType,
			},
		}
	}
	return &CloudDisk{
		disk: &computev1.Disk{},
	}
}

func TestGetEnableConfidentialCompute(t *testing.T) {
	testCases := []struct {
		name                              string
		diskVersion                       *CloudDisk
		expectedEnableConfidentialCompute bool
	}{
		{
			name:                              "test betaDisk with enableConfidentialCompute=false",
			diskVersion:                       CreateDiskWithConfidentialCompute(true, false, "hyperdisk-balanced"),
			expectedEnableConfidentialCompute: false,
		},
		{
			name:                              "test betaDisk with enableConfidentialCompute=true",
			diskVersion:                       CreateDiskWithConfidentialCompute(true, true, "hyperdisk-balanced"),
			expectedEnableConfidentialCompute: true,
		},
		{
			name:                              "test disk withpit enableConfidentialCompute",
			diskVersion:                       CreateDiskWithConfidentialCompute(false, false, "hyperdisk-balanced"),
			expectedEnableConfidentialCompute: false,
		},
		{
			name:                              "test disk withpit enableConfidentialCompute",
			diskVersion:                       CreateDiskWithConfidentialCompute(false, false, "pd-standard"),
			expectedEnableConfidentialCompute: false,
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		confidentialCompute := tc.diskVersion.GetEnableConfidentialCompute()
		if confidentialCompute != tc.expectedEnableConfidentialCompute {
			t.Fatalf("Got confidentialCompute value %t expected %t", confidentialCompute, tc.expectedEnableConfidentialCompute)
		}
		if confidentialCompute != tc.expectedEnableConfidentialCompute {
			t.Fatalf("Got confidentialCompute value %t expected %t", confidentialCompute, tc.expectedEnableConfidentialCompute)
		}
	}
}
