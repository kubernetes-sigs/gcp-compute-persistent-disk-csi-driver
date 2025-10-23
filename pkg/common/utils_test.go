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

package common

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"syscall"
	"testing"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/gax-go/v2/apierror"
	computev1 "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/parameters"
)

const (
	volIDZoneFmt   = "projects/%s/zones/%s/disks/%s"
	volIDRegionFmt = "projects/%s/regions/%s/disks/%s"
	testDiskName   = "test-disk"
)

func TestBytesToGbRoundDown(t *testing.T) {
	testCases := []struct {
		name  string
		bytes int64
		expGB int64
	}{
		{
			name:  "normal 5gb",
			bytes: 5368709120,
			expGB: 5,
		},
		{
			name:  "slightly less than 5gb",
			bytes: 5368709119,
			expGB: 4,
		},
		{
			name:  "slightly more than 5gb",
			bytes: 5368709121,
			expGB: 5,
		},
		{
			name:  "zero",
			bytes: 0,
			expGB: 0,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		gotGB := BytesToGbRoundDown(tc.bytes)

		if gotGB != tc.expGB {
			t.Errorf("got GB %v, expected %v", gotGB, tc.expGB)
		}

	}
}

func TestBytesToGbRoundUp(t *testing.T) {
	testCases := []struct {
		name  string
		bytes int64
		expGB int64
	}{
		{
			name:  "normal 5gb",
			bytes: 5368709120,
			expGB: 5,
		},
		{
			name:  "slightly less than 5gb",
			bytes: 5368709119,
			expGB: 5,
		},
		{
			name:  "slightly more than 5gb",
			bytes: 5368709121,
			expGB: 6,
		},
		{
			name:  "1.5Gi",
			bytes: 1610612736,
			expGB: 2,
		},
		{
			name:  "zero",
			bytes: 0,
			expGB: 0,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		gotGB := BytesToGbRoundUp(tc.bytes)

		if gotGB != tc.expGB {
			t.Errorf("got GB %v, expected %v", gotGB, tc.expGB)
		}

	}
}

func TestGbToBytes(t *testing.T) {
	testCases := []struct {
		name     string
		gb       int64
		expBytes int64
	}{
		{
			name:     "5Gb",
			gb:       5,
			expBytes: 5368709120,
		},
		{
			name:     "0gb",
			gb:       0,
			expBytes: 0,
		},
		{
			name:     "1gb",
			gb:       1,
			expBytes: 1024 * 1024 * 1024,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		gotBytes := GbToBytes(tc.gb)
		if gotBytes != tc.expBytes {
			t.Errorf("got bytes: %v, expected: %v", gotBytes, tc.expBytes)
		}

	}
}

func TestVolumeIDToKey(t *testing.T) {
	testName := "test-name"
	testZone := "test-zone"
	testProject := "test-project"
	testCrossProject := "test-cross-project"
	testRegion := "test-region"

	testCases := []struct {
		name       string
		volID      string
		expProject string
		expKey     *meta.Key
		expErr     bool
	}{
		{
			name:       "normal zonal",
			volID:      fmt.Sprintf(volIDZoneFmt, testProject, testZone, testName),
			expKey:     meta.ZonalKey(testName, testZone),
			expProject: testProject,
		},
		{
			name:       "cross project",
			volID:      fmt.Sprintf(volIDZoneFmt, testCrossProject, testZone, testName),
			expKey:     meta.ZonalKey(testName, testZone),
			expProject: testCrossProject,
		},
		{
			name:       "normal regional",
			volID:      fmt.Sprintf(volIDRegionFmt, testProject, testRegion, testName),
			expKey:     meta.RegionalKey(testName, testRegion),
			expProject: testProject,
		},
		{
			name:   "malformed",
			volID:  "wrong",
			expErr: true,
		},
		{
			name:   "malformed but right length",
			volID:  "this/is/wrong/but/right/num",
			expErr: true,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		project, gotKey, err := VolumeIDToKey(tc.volID)
		if err == nil && tc.expErr {
			t.Errorf("Expected error but got none")
		}
		if err != nil {
			if !tc.expErr {
				t.Errorf("Did not expect error but got: %v", err)
			}
			continue
		}

		if !reflect.DeepEqual(gotKey, tc.expKey) {
			t.Errorf("Got key %v, but expected %v, from volume ID %v", gotKey, tc.expKey, tc.volID)
		}

		if project != tc.expProject {
			t.Errorf("Got project %v, but expected %v, from volume ID %v", project, tc.expProject, tc.volID)
		}
	}

}

func TestNodeIDToZoneAndName(t *testing.T) {
	testProject := "test-project"
	testName := "test-name"
	testZone := "test-zone"

	testCases := []struct {
		name    string
		nodeID  string
		expZone string
		expName string
		expErr  bool
	}{
		{
			name:    "normal",
			nodeID:  CreateNodeID(testProject, testZone, testName),
			expZone: testZone,
			expName: testName,
		},
		{
			name:   "malformed",
			nodeID: "wrong",
			expErr: true,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		zone, name, err := NodeIDToZoneAndName(tc.nodeID)
		if err == nil && tc.expErr {
			t.Errorf("Expected error but got none")
		}
		if err != nil {
			if !tc.expErr {
				t.Errorf("Did not expect error but got: %v", err)
			}
			continue
		}

		if !(zone == tc.expZone && name == tc.expName) {
			t.Errorf("got wrong zone/name %s/%s, expected %s/%s", zone, name, tc.expZone, tc.expName)
		}

	}
}

func TestGetRegionFromZones(t *testing.T) {
	testCases := []struct {
		name      string
		zones     []string
		expRegion string
		expErr    bool
	}{
		{
			name:      "single zone success",
			zones:     []string{"us-central1-c"},
			expRegion: "us-central1",
		},
		{
			name:      "multi zone success",
			zones:     []string{"us-central1-b", "us-central1-c"},
			expRegion: "us-central1",
		},
		{
			name:   "multi different zone fail",
			zones:  []string{"us-central1-c", "us-asia1-b"},
			expErr: true,
		},
		{
			name:   "empty zones",
			expErr: true,
		},
		{
			name:   "malformed zone",
			zones:  []string{"blah/blooh"},
			expErr: true,
		},
		{
			name:      "tpc zone",
			zones:     []string{"u-europe-central2-a"},
			expRegion: "u-europe-central2",
		},
		{
			name:   "malformed tpc zone",
			zones:  []string{"us-europe-central2-a"},
			expErr: true,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		region, err := GetRegionFromZones(tc.zones)
		if err == nil && tc.expErr {
			t.Errorf("Expected error but got none")
		}
		if err != nil {
			if !tc.expErr {
				t.Errorf("Did not expect error but got: %v", err)
			}
			continue
		}

		if region != tc.expRegion {
			t.Errorf("Got region: %v, expected: %v", region, tc.expRegion)
		}

	}
}

func TestKeyToVolumeID(t *testing.T) {
	testName := "test-name"
	testZone := "test-zone"
	testProject := "test-project"
	testRegion := "test-region"

	testCases := []struct {
		name   string
		key    *meta.Key
		expID  string
		expErr bool
	}{
		{
			name:  "normal zonal",
			key:   meta.ZonalKey(testName, testZone),
			expID: fmt.Sprintf(volIDZoneFmt, testProject, testZone, testName),
		},
		{
			name:  "normal regional",
			key:   meta.RegionalKey(testName, testRegion),
			expID: fmt.Sprintf(volIDRegionFmt, testProject, testRegion, testName),
		},
		{
			name:   "malformed / unsupported global",
			key:    meta.GlobalKey(testName),
			expErr: true,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		gotID, err := KeyToVolumeID(tc.key, testProject)
		if err == nil && tc.expErr {
			t.Errorf("Expected error but got none")
		}
		if err != nil {
			if !tc.expErr {
				t.Errorf("Did not expect error but got: %v", err)
			}
			continue
		}

		if !reflect.DeepEqual(gotID, tc.expID) {
			t.Errorf("Got ID %v, but expected %v, from volume key %v", gotID, tc.expID, tc.key)
		}
	}

}

func TestParseMachineType(t *testing.T) {
	tests := []struct {
		desc                string
		inputMachineTypeUrl string
		expectedMachineType string
		expectError         bool
	}{
		{
			desc:                "full URL machine type",
			inputMachineTypeUrl: "https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-c/machineTypes/c3-highcpu-4",
			expectedMachineType: "c3-highcpu-4",
		},
		{
			desc:                "partial URL machine type",
			inputMachineTypeUrl: "zones/us-central1-c/machineTypes/n2-standard-4",
			expectedMachineType: "n2-standard-4",
		},
		{
			desc:                "custom partial URL machine type",
			inputMachineTypeUrl: "zones/us-central1-c/machineTypes/e2-custom-2-4096",
			expectedMachineType: "e2-custom-2-4096",
		},
		{
			desc:                "incorrect URL",
			inputMachineTypeUrl: "https://www.googleapis.com/compute/v1/projects/psch-gke-dev/zones/us-central1-c",
			expectError:         true,
		},
		{
			desc:                "incorrect partial URL",
			inputMachineTypeUrl: "zones/us-central1-c/machineTypes/",
			expectError:         true,
		},
		{
			desc:                "missing zone",
			inputMachineTypeUrl: "zones//machineTypes/n2-standard-4",
			expectError:         true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			actualMachineFamily, err := ParseMachineType(tc.inputMachineTypeUrl)
			if err != nil && !tc.expectError {
				t.Errorf("Got error %v parsing machine type %s; expect no error", err, tc.inputMachineTypeUrl)
			}
			if err == nil && tc.expectError {
				t.Errorf("Got no error parsing machine type %s; expect an error", tc.inputMachineTypeUrl)
			}
			if err == nil && actualMachineFamily != tc.expectedMachineType {
				t.Errorf("Got %s parsing machine type; expect %s", actualMachineFamily, tc.expectedMachineType)
			}
		})
	}
}

func TestCodeForError(t *testing.T) {
	getGoogleAPIWrappedError := func(err error) *googleapi.Error {
		apierr, _ := apierror.ParseError(err, false)
		wrappedError := &googleapi.Error{}
		wrappedError.Wrap(apierr)

		return wrappedError
	}
	getAPIError := func(err error) *apierror.APIError {
		apierror, _ := apierror.ParseError(err, true)
		return apierror
	}
	testCases := []struct {
		name     string
		inputErr error
		expCode  codes.Code
	}{
		{
			name:     "Not googleapi.Error",
			inputErr: errors.New("I am not a googleapi.Error"),
			expCode:  codes.Internal,
		},
		{
			name:     "User error",
			inputErr: &googleapi.Error{Code: http.StatusBadRequest, Message: "User error with bad request"},
			expCode:  codes.InvalidArgument,
		},
		{
			name: "googleapi.Error that wraps apierror.APIError of http kind",
			inputErr: getGoogleAPIWrappedError(&googleapi.Error{
				Code:    404,
				Message: "data requested not found error",
			}),
			expCode: codes.NotFound,
		},
		{
			name: "googleapi.Error that wraps apierror.APIError of http kind status conflict",
			inputErr: getGoogleAPIWrappedError(&googleapi.Error{
				Code:    409,
				Message: "status conflict error",
			}),
			expCode: codes.FailedPrecondition,
		},
		{
			name: "googleapi.Error that wraps apierror.APIError of status kind",
			inputErr: getGoogleAPIWrappedError(status.New(
				codes.Internal, "Internal status error",
			).Err()),
			expCode: codes.Internal,
		},
		{
			name: "apierror.APIError of http kind",
			inputErr: getAPIError(&googleapi.Error{
				Code:    404,
				Message: "data requested not found error",
			}),
			expCode: codes.NotFound,
		},
		{
			name: "apierror.APIError of status kind",
			inputErr: getAPIError(status.New(
				codes.Canceled, "Internal status error",
			).Err()),
			expCode: codes.Canceled,
		},
		{
			name:     "googleapi.Error but not a user error",
			inputErr: &googleapi.Error{Code: http.StatusInternalServerError, Message: "Internal error"},
			expCode:  codes.Internal,
		},
		{
			name:     "context canceled error",
			inputErr: context.Canceled,
			expCode:  codes.Canceled,
		},
		{
			name:     "context deadline exceeded error",
			inputErr: context.DeadlineExceeded,
			expCode:  codes.DeadlineExceeded,
		},
		{
			name:     "connection reset error",
			inputErr: fmt.Errorf("failed to getDisk: connection reset by peer"),
			expCode:  codes.Unavailable,
		},
		{
			name:     "wrapped connection reset error",
			inputErr: fmt.Errorf("received error: %v", syscall.ECONNRESET),
			expCode:  codes.Unavailable,
		},
		{
			name:     "status error with Aborted error code",
			inputErr: status.Error(codes.Aborted, "aborted error"),
			expCode:  codes.Aborted,
		},
		{
			name:     "nil error",
			inputErr: nil,
			expCode:  codes.Internal,
		},
		{
			name:     "user multiattach error",
			inputErr: fmt.Errorf("The disk resource 'projects/foo/disk/bar' is already being used by 'projects/foo/instances/1'"),
			expCode:  codes.InvalidArgument,
		},
		{
			name:     "TemporaryError that wraps googleapi error",
			inputErr: &TemporaryError{code: codes.Unavailable, err: &googleapi.Error{Code: http.StatusBadRequest, Message: "User error with bad request"}},
			expCode:  codes.Unavailable,
		},
		{
			name:     "TemporaryError that wraps fmt.Errorf, which wraps googleapi error",
			inputErr: &TemporaryError{code: codes.Aborted, err: fmt.Errorf("got error: %w", &googleapi.Error{Code: http.StatusBadRequest, Message: "User error with bad request"})},
			expCode:  codes.Aborted,
		},
		{
			name:     "TemporaryError that wraps status error",
			inputErr: &TemporaryError{code: codes.Aborted, err: status.Error(codes.Aborted, "aborted error")},
			expCode:  codes.Aborted,
		},
		{
			name:     "TemporaryError that wraps context canceled error",
			inputErr: &TemporaryError{code: codes.Aborted, err: context.Canceled},
			expCode:  codes.Aborted,
		},
		{
			name:     "CMEK precondition failed error, under layers of wrapping",
			inputErr: fmt.Errorf("unknown Insert disk error: %w", &googleapi.Error{Code: http.StatusPreconditionFailed, Message: "CMEK policy violated"}),
			expCode:  codes.FailedPrecondition,
		},
	}

	for _, tc := range testCases {
		errCode := CodeForError(tc.inputErr)
		if errCode != tc.expCode {
			t.Errorf("test %v failed: got %v, expected %v", tc.name, errCode, tc.expCode)
		}
	}
}

func TestIsContextError(t *testing.T) {
	cases := []struct {
		name            string
		err             error
		expectedErrCode codes.Code
		expectError     bool
	}{
		{
			name:            "deadline exceeded error",
			err:             context.DeadlineExceeded,
			expectedErrCode: codes.DeadlineExceeded,
		},
		{
			name:            "contains 'context deadline exceeded'",
			err:             fmt.Errorf("got error: %w", context.DeadlineExceeded),
			expectedErrCode: codes.DeadlineExceeded,
		},
		{
			name:            "context canceled error",
			err:             context.Canceled,
			expectedErrCode: codes.Canceled,
		},
		{
			name:            "contains 'context canceled'",
			err:             fmt.Errorf("got error: %w", context.Canceled),
			expectedErrCode: codes.Canceled,
		},
		{
			name:        "does not contain 'context canceled' or 'context deadline exceeded'",
			err:         fmt.Errorf("unknown error"),
			expectError: true,
		},
		{
			name:        "nil error",
			err:         nil,
			expectError: true,
		},
	}

	for _, test := range cases {
		errCode, err := isContextError(test.err)
		if test.expectError {
			if err == nil {
				t.Errorf("test %v failed, expected error, got %v", test.name, errCode)
			}
		} else if errCode != test.expectedErrCode {
			t.Errorf("test %v failed: got %v, expected %v", test.name, errCode, test.expectedErrCode)
		}
	}
}

func TestIsUserMultiAttachError(t *testing.T) {
	cases := []struct {
		errorString  string
		expectedCode codes.Code
		expectCode   bool
	}{
		{
			errorString:  "The disk resource 'projects/foo/disk/bar' is already being used by 'projects/foo/instance/biz'",
			expectedCode: codes.InvalidArgument,
			expectCode:   true,
		},
		{
			errorString: "The disk resource is ok!",
			expectCode:  false,
		},
	}
	for _, test := range cases {
		code, err := isUserMultiAttachError(fmt.Errorf("%v", test.errorString))
		if test.expectCode {
			if err != nil || code != test.expectedCode {
				t.Errorf("Failed with non-nil error %v or bad code %v: %s", err, code, test.errorString)
			}
		} else if err == nil {
			t.Errorf("Expected error for test but got none: %s", test.errorString)
		}
	}
}

func TestZones(t *testing.T) {
	testcases := []struct {
		name          string
		storagePools  []parameters.StoragePool
		expectedZones []string
		expectedErr   bool
	}{
		{
			name: "StoragePools_WithValidResourceNames_ReturnsZones",
			storagePools: []parameters.StoragePool{
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
			expectedZones: []string{"us-central1-a", "us-central1-b"},
		},
		{
			name: "StoragePools_WithDuplicateZone_ReturnsError",
			storagePools: []parameters.StoragePool{
				{
					Project:      "my-project",
					Zone:         "us-central1-a",
					Name:         "storagePool-1",
					ResourceName: "projects/my-project/zones/us-central1-a/storagePools/storagePool-1",
				},
				{
					Project:      "my-project",
					Zone:         "us-central1-a",
					Name:         "storagePool-2",
					ResourceName: "projects/my-project/zones/us-central1-a/storagePools/storagePool-2",
				},
			},
			expectedErr: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			zones, err := parameters.StoragePoolZones(tc.storagePools)
			input := fmt.Sprintf("StoragePoolZones(%q)", tc.storagePools)
			gotErr := err != nil
			if gotErr != tc.expectedErr {
				t.Errorf("%s error presence = %v, expected error presence = %v", input, gotErr, tc.expectedErr)
			}
			if diff := cmp.Diff(tc.expectedZones, zones); diff != "" {
				t.Errorf("%s: -want err, +got err\n%s", input, diff)
			}
		})
	}
}

func TestStoragePoolInZone(t *testing.T) {
	testcases := []struct {
		name                string
		storagePools        []parameters.StoragePool
		zone                string
		expectedStoragePool *parameters.StoragePool
		expectedErr         bool
	}{
		{
			name: "ValidStoragePools_ReturnsStoragePoolInZone",
			storagePools: []parameters.StoragePool{
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
			zone: "us-central1-a",
			expectedStoragePool: &parameters.StoragePool{
				Project:      "my-project",
				Zone:         "us-central1-a",
				Name:         "storagePool-1",
				ResourceName: "projects/my-project/zones/us-central1-a/storagePools/storagePool-1",
			},
		},
		{
			name: "StoragePoolNotInZone_ReturnsNil",
			storagePools: []parameters.StoragePool{
				{
					Project:      "my-project",
					Zone:         "us-central1-a",
					Name:         "storagePool-1",
					ResourceName: "projects/my-project/zones/us-central1-a/storagePools/storagePool-1",
				},
			},
			zone:                "us-central1-b",
			expectedStoragePool: nil,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			sp := parameters.StoragePoolInZone(tc.storagePools, tc.zone)
			input := fmt.Sprintf("StoragePoolInZone(%q)", tc.storagePools)
			if diff := cmp.Diff(tc.expectedStoragePool, sp); diff != "" {
				t.Errorf("%s: -want, +got \n%s", input, diff)
			}
		})
	}
}

func TestUnorderedSlicesEqual(t *testing.T) {
	testcases := []struct {
		name                string
		slice1              []string
		slice2              []string
		expectedSlicesEqual bool
	}{
		{
			name:                "OrderedSlicesEqual_ReturnsTrue",
			slice1:              []string{"us-central1-a", "us-central1-b"},
			slice2:              []string{"us-central1-a", "us-central1-b"},
			expectedSlicesEqual: true,
		},
		{
			name:                "UnorderedSlicesEqual_ReturnsTrue",
			slice1:              []string{"us-central1-a", "us-central1-b"},
			slice2:              []string{"us-central1-b", "us-central1-a"},
			expectedSlicesEqual: true,
		},
		{
			name:                "SlicesNotEqualSameLength_ReturnsFalse",
			slice1:              []string{"us-central1-a", "us-central1-b"},
			slice2:              []string{"us-central1-a", "us-central1-a"},
			expectedSlicesEqual: false,
		},
		{
			name:                "SlicesNotEqualDifferentLength_ReturnsFalse",
			slice1:              []string{"us-central1-a"},
			slice2:              []string{},
			expectedSlicesEqual: false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			slicesEqual := UnorderedSlicesEqual(tc.slice1, tc.slice2)
			input := fmt.Sprintf("UnorderedSlicesEqual(%v, %v)", tc.slice1, tc.slice2)
			if diff := cmp.Diff(tc.expectedSlicesEqual, slicesEqual); diff != "" {
				t.Errorf("%s: -want, +got \n%s", input, diff)
			}
		})
	}
}

func TestParseZoneFromURI(t *testing.T) {
	testcases := []struct {
		name      string
		zoneURI   string
		wantZone  string
		expectErr bool
	}{
		{
			name:     "ParseZoneFromURI_FullURI",
			zoneURI:  "https://www.googleapis.com/compute/v1/projects/psch-gke-dev/zones/us-east4-a",
			wantZone: "us-east4-a",
		},
		{
			name:     "ParseZoneFromURI_ProjectZoneString",
			zoneURI:  "projects/psch-gke-dev/zones/us-east4-a",
			wantZone: "us-east4-a",
		},
		{
			name:      "ParseZoneFromURI_Malformed",
			zoneURI:   "projects/psch-gke-dev/regions/us-east4",
			expectErr: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			gotZone, err := ParseZoneFromURI(tc.zoneURI)
			if err != nil && !tc.expectErr {
				t.Fatalf("Unexpected error: %v", err)
			}
			if err == nil && tc.expectErr {
				t.Fatalf("Expected err, but none was returned. Zone result: %v", gotZone)
			}
			if gotZone != tc.wantZone {
				t.Errorf("ParseZoneFromURI(%v): got %v, want %v", tc.zoneURI, gotZone, tc.wantZone)
			}
		})
	}
}

func TestNewCombinedError(t *testing.T) {
	testcases := []struct {
		name     string
		errors   []error
		wantCode codes.Code
	}{
		{
			name:     "single generic error",
			errors:   []error{fmt.Errorf("my internal error")},
			wantCode: codes.Internal,
		},
		{
			name:     "single retryable error",
			errors:   []error{&googleapi.Error{Code: http.StatusTooManyRequests, Message: "Resource Exhausted"}},
			wantCode: codes.ResourceExhausted,
		},
		{
			name:     "multi generic error",
			errors:   []error{fmt.Errorf("my internal error"), fmt.Errorf("my other internal error")},
			wantCode: codes.Internal,
		},
		{
			name:     "multi retryable error",
			errors:   []error{fmt.Errorf("my internal error"), &googleapi.Error{Code: http.StatusTooManyRequests, Message: "Resource Exhausted"}},
			wantCode: codes.ResourceExhausted,
		},
		{
			name:     "multi retryable error",
			errors:   []error{fmt.Errorf("my internal error"), &googleapi.Error{Code: http.StatusGatewayTimeout, Message: "connection reset by peer"}, fmt.Errorf("my other internal error")},
			wantCode: codes.Unavailable,
		},
		{
			name:     "multi retryable error",
			errors:   []error{fmt.Errorf("The disk resource is already being used"), &googleapi.Error{Code: http.StatusGatewayTimeout, Message: "connection reset by peer"}},
			wantCode: codes.Unavailable,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			gotCode := CodeForError(NewCombinedError("message", tc.errors))
			if gotCode != tc.wantCode {
				t.Errorf("NewCombinedError(%v): got %v, want %v", tc.errors, gotCode, tc.wantCode)
			}
		})
	}
}
func TestIsUpdateIopsThroughputValuesAllowed(t *testing.T) {
	testcases := []struct {
		name         string
		diskType     string
		expectResult bool
	}{
		{
			name:         "Hyperdisk returns true",
			diskType:     "hyperdisk-balanced",
			expectResult: true,
		},
		{
			name:         "PD disk returns true",
			diskType:     "pd-ssd",
			expectResult: false,
		},
		{
			name:         "Unknown disk type",
			diskType:     "not-a-disk-type-we-know",
			expectResult: false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			disk := &computev1.Disk{
				Name: "test-disk",
				Type: tc.diskType,
			}
			gotResult := IsUpdateIopsThroughputValuesAllowed(disk)
			if gotResult != tc.expectResult {
				t.Errorf("IsUpdateIopsThroughputValuesAllowed: got %v, want %v", gotResult, tc.expectResult)
			}
		})
	}
}

func TestGetMinIopsThroughput(t *testing.T) {
	testcases := []struct {
		name                string
		existingDisk        *computev1.Disk
		reqGb               int64
		expectResult        bool
		expectMinIops       int64
		expectMinThroughput int64
	}{
		{
			name: "Hyperdisk Balanced 4 GiB to 5GiB",
			existingDisk: &computev1.Disk{
				Name:                  testDiskName,
				Type:                  "hyperdisk-balanced",
				ProvisionedIops:       2000,
				ProvisionedThroughput: 140,
				SizeGb:                4,
			},
			reqGb:               5,
			expectResult:        true,
			expectMinIops:       2500,
			expectMinThroughput: 0, // 0 indicates no change to throughput
		},
		{
			name: "Hyperdisk Balanced 5 GiB to 6GiB",
			existingDisk: &computev1.Disk{
				Name:                  testDiskName,
				Type:                  "hyperdisk-balanced",
				ProvisionedIops:       2500,
				ProvisionedThroughput: 145,
				SizeGb:                5,
			},
			reqGb:               6,
			expectResult:        true,
			expectMinIops:       3000,
			expectMinThroughput: 0, // 0 indicates no change to throughput
		},
		{
			name: "Hyperdisk Balanced 6 GiB to 10GiB - no adjustment",
			existingDisk: &computev1.Disk{
				Name:                  testDiskName,
				Type:                  "hyperdisk-balanced",
				ProvisionedIops:       3000,
				ProvisionedThroughput: 145,
				SizeGb:                6,
			},
			reqGb:               10,
			expectResult:        false,
			expectMinIops:       0, // 0 indicates no change to iops
			expectMinThroughput: 0, // 0 indicates no change to throughput
		},
		{
			name: "Hyperdisk Extreme with min IOPS value as 2 will adjust IOPs",
			existingDisk: &computev1.Disk{
				Name:            testDiskName,
				Type:            "hyperdisk-extreme",
				ProvisionedIops: 128,
				SizeGb:          64,
			},
			reqGb:               65,
			expectResult:        true,
			expectMinIops:       130,
			expectMinThroughput: 0, // 0 indicates no change to throughput
		},
		{
			name: "Hyperdisk Extreme 64GiB to 70 GiB - no adjustment",
			existingDisk: &computev1.Disk{
				Name:            testDiskName,
				Type:            "hyperdisk-extreme",
				ProvisionedIops: 3000,
				SizeGb:          64,
			},
			reqGb:               70,
			expectResult:        false,
			expectMinIops:       0, // 0 indicates no change to iops
			expectMinThroughput: 0, // 0 indicates no change to throughput
		},
		{
			name: "Hyperdisk ML with min throughput per GB will adjust throughput",
			existingDisk: &computev1.Disk{
				Name:                  testDiskName,
				Type:                  "hyperdisk-ml",
				ProvisionedThroughput: 400,
				SizeGb:                3334,
			},
			reqGb:               3400,
			expectResult:        true,
			expectMinThroughput: 408,
		},
		{
			name: "Hyperdisk ML 64GiB to 100 GiB - no adjustment",
			existingDisk: &computev1.Disk{
				Name:                  testDiskName,
				Type:                  "hyperdisk-ml",
				ProvisionedThroughput: 6400,
				SizeGb:                64,
			},
			reqGb:               100,
			expectResult:        false,
			expectMinIops:       0, // 0 indicates no change to iops
			expectMinThroughput: 0, // 0 indicates no change to throughput
		},
		{
			name: "Hyperdisk throughput with min throughput per GB will adjust throughput",
			existingDisk: &computev1.Disk{
				Name:                  testDiskName,
				Type:                  "hyperdisk-throughput",
				ProvisionedThroughput: 20,
				SizeGb:                2048,
			},
			reqGb:               3072,
			expectResult:        true,
			expectMinIops:       0,
			expectMinThroughput: 30,
		},
		{
			name: "Hyperdisk throughput 2TiB to 4TiB - no adjustment",
			existingDisk: &computev1.Disk{
				Name:                  testDiskName,
				Type:                  "hyperdisk-throughput",
				ProvisionedThroughput: 567,
				SizeGb:                2048,
			},
			reqGb:               4096,
			expectResult:        false,
			expectMinIops:       0, // 0 indicates no change to iops
			expectMinThroughput: 0, // 0 indicates no change to throughput
		},
		{
			name: "Unknown disk type, no need to update",
			existingDisk: &computev1.Disk{
				Name: testDiskName,
				Type: "unknown-type",
			},
			reqGb:               5,
			expectResult:        false,
			expectMinIops:       0,
			expectMinThroughput: 0, // 0 indicates no change to throughput
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			gotNeeded, gotMinIops, gotMinThroughput := GetMinIopsThroughput(tc.existingDisk, tc.reqGb)
			if gotNeeded != tc.expectResult {
				t.Errorf("GetMinIopsThroughput: got %v, want %v", gotNeeded, tc.expectResult)
			}

			if gotMinIops != tc.expectMinIops {
				t.Errorf("GetMinIopsThroughput Iops: got %v, want %v", gotMinIops, tc.expectMinIops)
			}

			if gotMinThroughput != tc.expectMinThroughput {
				t.Errorf("GetMinIopsThroughput Throughput: got %v, want %v", gotMinThroughput, tc.expectMinThroughput)
			}
		})
	}
}
