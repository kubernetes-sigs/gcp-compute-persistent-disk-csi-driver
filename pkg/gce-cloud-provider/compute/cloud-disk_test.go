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

	"github.com/google/go-cmp/cmp"
	computealpha "google.golang.org/api/compute/v0.alpha"
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
			name:                              "test disk without enableConfidentialCompute",
			diskVersion:                       CreateDiskWithConfidentialCompute(false, false, "hyperdisk-balanced"),
			expectedEnableConfidentialCompute: false,
		},
		{
			name:                              "test disk without enableConfidentialCompute",
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

func TestGetEnableStoragePools(t *testing.T) {
	testCases := []struct {
		name                       string
		cloudDisk                  *CloudDisk
		expectedEnableStoragePools bool
	}{
		{
			name: "v1 disk returns false",
			cloudDisk: &CloudDisk{
				disk: &computev1.Disk{},
			},
			expectedEnableStoragePools: false,
		},
		{
			name: "beta disk returns false",
			cloudDisk: &CloudDisk{
				betaDisk: &computebeta.Disk{},
			},
			expectedEnableStoragePools: false,
		},
		{
			name: "alpha disk without storage pool returns false",
			cloudDisk: &CloudDisk{
				alphaDisk: &computealpha.Disk{},
			},
			expectedEnableStoragePools: false,
		},
		{
			name: "alpha disk with storage pool returns true",
			cloudDisk: &CloudDisk{
				alphaDisk: &computealpha.Disk{
					StoragePool: "projects/my-project/zones/us-central1-a/storagePools/storagePool-1",
				},
			},
			expectedEnableStoragePools: true,
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		input := "GetEnableStoragePools()"
		enableStoragePools := tc.cloudDisk.GetEnableStoragePools()
		if enableStoragePools != tc.expectedEnableStoragePools {
			t.Fatalf("%s got confidentialCompute value %t expected %t", input, enableStoragePools, tc.expectedEnableStoragePools)
		}
		if enableStoragePools != tc.expectedEnableStoragePools {
			t.Fatalf("%s got confidentialCompute value %t expected %t", input, enableStoragePools, tc.expectedEnableStoragePools)
		}
	}
}

func TestGetLabels(t *testing.T) {
	testCases := []struct {
		name       string
		cloudDisk  *CloudDisk
		wantLabels map[string]string
	}{
		{
			name: "v1 disk labels",
			cloudDisk: &CloudDisk{
				disk: &computev1.Disk{
					Labels: map[string]string{"foo": "v1", "goog-gke-multi-zone": "true"},
				},
			},
			wantLabels: map[string]string{"foo": "v1", "goog-gke-multi-zone": "true"},
		},
		{
			name: "beta disk labels",
			cloudDisk: &CloudDisk{
				betaDisk: &computebeta.Disk{
					Labels: map[string]string{"bar": "beta", "goog-gke-multi-zone": "true"},
				},
			},
			wantLabels: map[string]string{"bar": "beta", "goog-gke-multi-zone": "true"},
		},
		{
			name: "alpha disk without storage pool returns false",
			cloudDisk: &CloudDisk{
				alphaDisk: &computealpha.Disk{
					Labels: map[string]string{"baz": "alpha", "goog-gke-multi-zone": "true"},
				},
			},
			wantLabels: map[string]string{"baz": "alpha", "goog-gke-multi-zone": "true"},
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		gotLabels := tc.cloudDisk.GetLabels()
		if diff := cmp.Diff(tc.wantLabels, gotLabels); diff != "" {
			t.Errorf("GetLabels() returned unexpected difference (-want +got):\n%s", diff)
		}
	}
}
