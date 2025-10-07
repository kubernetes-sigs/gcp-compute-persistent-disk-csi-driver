package parameters

import (
	"fmt"
	"reflect"
	"testing"
)

func TestSnapshotStorageLocations(t *testing.T) {
	tests := []struct {
		desc                        string
		locationString              string
		expectedNormalizedLocations []string
		expectError                 bool
	}{
		{
			"valid multi-region",
			"   uS ",
			[]string{"us"},
			false,
		},
		{
			"valid region",
			"  US-EAST1",
			[]string{"us-east1"},
			false,
		},
		{
			"valid region in large continent",
			"europe-west12",
			[]string{"europe-west12"},
			false,
		},
		{
			// Zones are not valid bucket/snapshot locations.
			"single zone",
			"us-east1-a",
			[]string{},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			normalizedLocations, err := ProcessStorageLocations(tc.locationString)
			if err != nil && !tc.expectError {
				t.Errorf("Got error %v processing storage locations %q; expect no error", err, tc.locationString)
			}
			if err == nil && tc.expectError {
				t.Errorf("Got no error processing storage locations %q; expect an error", tc.locationString)
			}
			if err == nil && !reflect.DeepEqual(normalizedLocations, tc.expectedNormalizedLocations) {
				t.Errorf("Got %v for normalized storage locations; expect %v", normalizedLocations, tc.expectedNormalizedLocations)
			}
		})
	}
}

func TestStringInSlice(t *testing.T) {
	testCases := []struct {
		name            string
		inputStr        string
		inputSlice      []string
		expectedInSlice bool
	}{
		{
			name:            "string is in the slice",
			inputStr:        "in slice",
			inputSlice:      []string{"in slice", "other string"},
			expectedInSlice: true,
		},
		{
			name:       "string is NOT in the slice",
			inputStr:   "not in slice",
			inputSlice: []string{"other string"},
		},
	}

	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		actualResult := StringInSlice(tc.inputStr, tc.inputSlice)
		if actualResult != tc.expectedInSlice {
			t.Errorf("Expect value is %v but got  %v. inputStr is %s, inputSlice is %v", tc.expectedInSlice, actualResult, tc.inputStr, tc.inputSlice)
		}
	}
}

func TestValidateDataCacheMode(t *testing.T) {
	testCases := []struct {
		name        string
		inputStr    string
		expectError bool
	}{
		{
			name:     "valid input - writethrough",
			inputStr: "writethrough",
		},
		{
			name:     "valid input - writeback",
			inputStr: "writeback",
		},
		{
			name:        "invalid input",
			inputStr:    "write-back not valid",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		err := ValidateDataCacheMode(tc.inputStr)
		if err != nil && !tc.expectError {
			t.Errorf("Got error %v  validate data cache mode %s; expect no error", err, tc.inputStr)
		}

		if err == nil && tc.expectError {
			t.Errorf("Got no error validate data cache mode %s; expect an error", tc.inputStr)
		}
	}

}

func TestValidateNonNegativeInt(t *testing.T) {
	testCases := []struct {
		name        string
		cacheSize   int64
		expectError bool
	}{
		{
			name:      "valid input - positive cache size",
			cacheSize: 100000,
		},
		{
			name:        "invalid input - cachesize 0",
			cacheSize:   0,
			expectError: true,
		},
		{
			name:        "invalid input - negative cache size",
			cacheSize:   -100,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		err := ValidateNonNegativeInt(tc.cacheSize)
		if err != nil && !tc.expectError {
			t.Errorf("Got error %v  validate data cache mode %d; expect no error", err, tc.cacheSize)
		}

		if err == nil && tc.expectError {
			t.Errorf("Got no error validate data cache mode %d; expect an error", tc.cacheSize)
		}
	}

}

func TestIsValidDiskEncryptionKmsKey(t *testing.T) {
	cases := []struct {
		diskEncryptionKmsKey string
		expectedIsValid      bool
	}{
		{
			diskEncryptionKmsKey: "projects/my-project/locations/us-central1/keyRings/TestKeyRing/cryptoKeys/test-key",
			expectedIsValid:      true,
		},
		{
			diskEncryptionKmsKey: "projects/my-project/locations/global/keyRings/TestKeyRing/cryptoKeys/test-key",
			expectedIsValid:      true,
		},
		{
			diskEncryptionKmsKey: "projects/my-project/locations/keyRings/TestKeyRing/cryptoKeys/test-key",
			expectedIsValid:      false,
		},
	}
	for _, tc := range cases {
		isValid := isValidDiskEncryptionKmsKey(tc.diskEncryptionKmsKey)
		if tc.expectedIsValid != isValid {
			t.Errorf("test failed: the provided key %s expected to be %v bu tgot %v", tc.diskEncryptionKmsKey, tc.expectedIsValid, isValid)
		}
	}
}

func TestFieldsFromResourceName(t *testing.T) {
	testcases := []struct {
		name            string
		resourceName    string
		expectedProject string
		expectedZone    string
		expectedName    string
		expectedErr     bool
	}{
		{
			name:            "StoragePool_WithValidResourceName_ReturnsFields",
			resourceName:    "projects/my-project/zones/us-central1-a/storagePools/storagePool-1",
			expectedProject: "my-project",
			expectedZone:    "us-central1-a",
			expectedName:    "storagePool-1",
		},
		{
			name:         "StoragePool_WithFullResourceURL_ReturnsError",
			resourceName: "https://www.googleapis.com/compute/v1/projects/project/zones/zone/storagePools/storagePool",
			expectedErr:  true,
		},
		{
			name:         "StoragePool_WithMissingProject_ReturnsError",
			resourceName: "zones/us-central1-a/storagePools/storagePool-1",
			expectedErr:  true,
		},
		{
			name:         "StoragePool_WithMissingZone_ReturnsError",
			resourceName: "projects/my-project/storagePools/storagePool-1",
			expectedErr:  true,
		},
		{
			name:         "StoragePool_WithMissingStoragePoolName_ReturnsError",
			resourceName: "projects/my-project/zones/us-central1-a/storagePool-1",
			expectedErr:  true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			project, zone, name, err := fieldsFromStoragePoolResourceName(tc.resourceName)
			input := fmt.Sprintf("fieldsFromStoragePoolResourceName(%q)", tc.resourceName)
			gotErr := err != nil
			if gotErr != tc.expectedErr {
				t.Errorf("%s error presence = %v, expected error presence = %v", input, gotErr, tc.expectedErr)
			}
			if project != tc.expectedProject || zone != tc.expectedZone || name != tc.expectedName {
				t.Errorf("%s returned {project: %q, zone: %q, name: %q}, expected {project: %q, zone: %q, name: %q}", input, project, zone, name, tc.expectedProject, tc.expectedZone, tc.expectedName)
			}
		})
	}
}
