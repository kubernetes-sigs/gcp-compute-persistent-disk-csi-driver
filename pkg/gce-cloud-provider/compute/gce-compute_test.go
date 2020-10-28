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

package gcecloudprovider

import (
	"testing"

	computev1 "google.golang.org/api/compute/v1"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

func TestValidateDiskParameters(t *testing.T) {
	testCases := []struct {
		fetchedKMSKey      string
		storageClassKMSKey string
		expectErr          bool
	}{
		{
			fetchedKMSKey:      "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/test-key/cryptoKeyVersions/8",
			storageClassKMSKey: "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/test-key",
			expectErr:          false,
		},
		{
			fetchedKMSKey:      "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/test-key",
			storageClassKMSKey: "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/test-key",
			expectErr:          false,
		},
		{
			fetchedKMSKey:      "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/garbage/cryptoKeyVersions/8",
			storageClassKMSKey: "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/test-key",
			expectErr:          true,
		},
		{
			fetchedKMSKey:      "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/garbage",
			storageClassKMSKey: "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/test-key",
			expectErr:          true,
		},
		{
			fetchedKMSKey:      "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/test-key",
			storageClassKMSKey: "projects/my-project/locations/us-west1/keyRings/TestKeyRing/cryptoKeys/test-key",
			expectErr:          true,
		},
		{
			fetchedKMSKey:      "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/foobar/cryptoKeyVersions/8",
			storageClassKMSKey: "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/foo",
			expectErr:          true,
		},
		{
			fetchedKMSKey:      "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/foobar",
			storageClassKMSKey: "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/foo",
			expectErr:          true,
		},
	}

	for i, tc := range testCases {
		// Arrange
		existingDisk := CloudDiskFromV1(&computev1.Disk{
			Id:                546559531467326555,
			CreationTimestamp: "2020-07-24T17:20:06.292-07:00",
			Name:              "test-disk",
			SizeGb:            500,
			Zone:              "https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-c",
			Status:            "READY",
			SelfLink:          "https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-c/disks/test-disk",
			Type:              "https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-c/diskTypes/pd-standard",
			DiskEncryptionKey: &computev1.CustomerEncryptionKey{
				KmsKeyName: tc.fetchedKMSKey,
			},
			LabelFingerprint:       "42WmSpB8rSM=",
			PhysicalBlockSizeBytes: 4096,
			Kind:                   "compute#disk",
		})

		storageClassParams := common.DiskParameters{
			DiskType:             "pd-standard",
			ReplicationType:      "none",
			DiskEncryptionKMSKey: tc.storageClassKMSKey,
		}

		// Act
		err := ValidateDiskParameters(existingDisk, storageClassParams)

		// Assert
		if !tc.expectErr && err != nil {
			t.Fatalf("Test case #%v: ValidateDiskParameters did not expect error, but got %v", i, err)
		}
		if tc.expectErr && err == nil {
			t.Fatalf("Test case #%v: ValidateDiskParameters expected error, but got no error", i)
		}
	}
}
