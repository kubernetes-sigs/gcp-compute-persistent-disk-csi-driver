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
	"context"
	"testing"

	computebeta "google.golang.org/api/compute/v0.beta"
	computev1 "google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common/constants"
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

func TestValidateExistingDisk(t *testing.T) {
	hyperdisk := "hyperdisk-balanced"
	pd := "pd-balanced"
	for _, tc := range []struct {
		name        string
		reqBytes    int64
		limBytes    int64
		multiWriter bool
		accessMode  string
		disk        *computebeta.Disk
		diskType    string
		wantErr     bool
	}{
		{
			name:     "invalid reqbytes - too big",
			reqBytes: common.GbToBytes(10),
			disk: &computebeta.Disk{
				SizeGb: 5,
			},
			wantErr: true,
		},
		{
			name:     "valid reqbytes",
			reqBytes: common.GbToBytes(5),
			disk: &computebeta.Disk{
				SizeGb: 8,
			},
		},
		{
			name:     "invalid limbytes",
			limBytes: common.GbToBytes(5),
			disk: &computebeta.Disk{
				SizeGb: 10,
			},
			wantErr: true,
		},
		{
			name:     "valid limbytes",
			limBytes: common.GbToBytes(5),
			disk: &computebeta.Disk{
				SizeGb: 3,
			},
		},
		{
			name:        "valid pd with same multi-writer config",
			multiWriter: true,
			disk: &computebeta.Disk{
				MultiWriter: true,
			},
			diskType: pd,
		},
		{
			name:        "valid pd with compatible multi-writer config",
			multiWriter: false,
			disk: &computebeta.Disk{
				MultiWriter: true,
			},
			diskType: pd,
		},
		{
			name:        "invalid pd with incompatible multi-writer config",
			multiWriter: true,
			disk: &computebeta.Disk{
				MultiWriter: false,
			},
			diskType: pd,
			wantErr:  true,
		},
		{
			name:       "valid hyperdisk with same access mode config",
			accessMode: constants.GCEReadWriteManyAccessMode,
			disk: &computebeta.Disk{
				AccessMode: constants.GCEReadWriteManyAccessMode,
			},
			diskType: hyperdisk,
		},
		{
			name:       "valid hyperdisk with compatible access mode config - ROX can use RWX",
			accessMode: constants.GCEReadOnlyManyAccessMode,
			disk: &computebeta.Disk{
				AccessMode: constants.GCEReadWriteManyAccessMode,
			},
			diskType: hyperdisk,
		},
		{
			name:       "valid hyperdisk with compatible access mode config - RWO can use RWX",
			accessMode: constants.GCEReadWriteOnceAccessMode,
			disk: &computebeta.Disk{
				AccessMode: constants.GCEReadWriteManyAccessMode,
			},
			diskType: hyperdisk,
		},
		{
			name:       "invalid hyperdisk with incompatible access mode config - ROX cannot use RWO",
			accessMode: constants.GCEReadOnlyManyAccessMode,
			disk: &computebeta.Disk{
				AccessMode: constants.GCEReadWriteOnceAccessMode,
			},
			diskType: hyperdisk,
			wantErr:  true,
		},
		{
			name:       "invalid hyperdisk with incompatible access mode config - RWO cannot use ROX",
			accessMode: constants.GCEReadWriteOnceAccessMode,
			disk: &computebeta.Disk{
				AccessMode: constants.GCEReadOnlyManyAccessMode,
			},
			diskType: hyperdisk,
			wantErr:  true,
		},
		{
			name:       "invalid hyperdisk with incompatible access mode config - RWX cannot use ROX",
			accessMode: constants.GCEReadWriteManyAccessMode,
			disk: &computebeta.Disk{
				AccessMode: constants.GCEReadOnlyManyAccessMode,
			},
			diskType: hyperdisk,
			wantErr:  true,
		},
		{
			name:       "invalid access mode",
			accessMode: "RANDOM_ERROR",
			disk: &computebeta.Disk{
				AccessMode: constants.GCEReadOnlyManyAccessMode,
			},
			diskType: hyperdisk,
			wantErr:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Bootstrap correct disk
			d := tc.disk
			d.Type = tc.diskType
			d.Zone = "zone"

			// Bootstrap params. We don't care about these as they are already tested in previous unit test.
			params := common.DiskParameters{
				DiskType: tc.diskType,
			}

			err := ValidateExistingDisk(context.Background(), CloudDiskFromBeta(tc.disk), params, tc.reqBytes, tc.limBytes, tc.multiWriter, tc.accessMode)
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Errorf("want error: %v, got error: %v", tc.wantErr, err)
			}
		})
	}
}

func TestCodeForGCEOpError(t *testing.T) {
	testCases := []struct {
		name     string
		inputErr computev1.OperationErrorErrors
		expCode  codes.Code
	}{
		{
			name:     "RESOURCE_NOT_FOUND error",
			inputErr: computev1.OperationErrorErrors{Code: "RESOURCE_NOT_FOUND"},
			expCode:  codes.NotFound,
		},
		{
			name:     "RESOURCE_ALREADY_EXISTS error",
			inputErr: computev1.OperationErrorErrors{Code: "RESOURCE_ALREADY_EXISTS"},
			expCode:  codes.AlreadyExists,
		},
		{
			name:     "OPERATION_CANCELED_BY_USER error",
			inputErr: computev1.OperationErrorErrors{Code: "OPERATION_CANCELED_BY_USER"},
			expCode:  codes.Canceled,
		},
		{
			name:     "QUOTA_EXCEEDED error",
			inputErr: computev1.OperationErrorErrors{Code: "QUOTA_EXCEEDED"},
			expCode:  codes.ResourceExhausted,
		},
		{
			name:     "ZONE_RESOURCE_POOL_EXHAUSTED error",
			inputErr: computev1.OperationErrorErrors{Code: "ZONE_RESOURCE_POOL_EXHAUSTED"},
			expCode:  codes.Unavailable,
		},
		{
			name:     "ZONE_RESOURCE_POOL_EXHAUSTED_WITH_DETAILS error",
			inputErr: computev1.OperationErrorErrors{Code: "ZONE_RESOURCE_POOL_EXHAUSTED_WITH_DETAILS"},
			expCode:  codes.Unavailable,
		},
		{
			name:     "REGION_QUOTA_EXCEEDED error",
			inputErr: computev1.OperationErrorErrors{Code: "REGION_QUOTA_EXCEEDED"},
			expCode:  codes.ResourceExhausted,
		},
		{
			name:     "RATE_LIMIT_EXCEEDED error",
			inputErr: computev1.OperationErrorErrors{Code: "RATE_LIMIT_EXCEEDED"},
			expCode:  codes.ResourceExhausted,
		},
		{
			name:     "INVALID_USAGE error",
			inputErr: computev1.OperationErrorErrors{Code: "INVALID_USAGE"},
			expCode:  codes.InvalidArgument,
		},
		{
			name:     "RESOURCE_IN_USE_BY_ANOTHER_RESOURCE error",
			inputErr: computev1.OperationErrorErrors{Code: "RESOURCE_IN_USE_BY_ANOTHER_RESOURCE"},
			expCode:  codes.InvalidArgument,
		},
		{
			name:     "UNSUPPORTED_OPERATION error",
			inputErr: computev1.OperationErrorErrors{Code: "UNSUPPORTED_OPERATION"},
			expCode:  codes.InvalidArgument,
		},
		{
			name:     "RESOURCE_OPERATION_RATE_EXCEEDED error",
			inputErr: computev1.OperationErrorErrors{Code: "RESOURCE_OPERATION_RATE_EXCEEDED"},
			expCode:  codes.ResourceExhausted,
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		errCode := codeForGCEOpError(tc.inputErr)
		if errCode != tc.expCode {
			t.Errorf("test %v failed: got %v, expected %v", tc.name, errCode, tc.expCode)
		}
	}
}
