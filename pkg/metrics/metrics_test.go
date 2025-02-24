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

package metrics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
)

const (
	hyperdiskBalanced = "hyperdisk-balanced"
)

func CreateDiskWithConfidentialCompute(confidentialCompute bool, diskType string) *gce.CloudDisk {
	return gce.CloudDiskFromV1(&compute.Disk{
		EnableConfidentialCompute: confidentialCompute,
		Type:                      diskType,
	})
}

func CreateDiskWithStoragePool(storagePool string, diskType string) *gce.CloudDisk {
	return gce.CloudDiskFromV1(&compute.Disk{
		StoragePool: storagePool,
		Type:        diskType,
	})
}

func TestGetMetricParameters(t *testing.T) {
	testCases := []struct {
		name                              string
		disk                              *gce.CloudDisk
		expectedEnableConfidentialCompute string
		expectedDiskType                  string
		expectedEnableStoragePools        string
	}{
		{
			name:                              "test disk with enableConfidentialCompute=false",
			disk:                              CreateDiskWithConfidentialCompute(false, hyperdiskBalanced),
			expectedEnableConfidentialCompute: "false",
			expectedDiskType:                  hyperdiskBalanced,
			expectedEnableStoragePools:        "false",
		},
		{
			name:                              "test disk with enableConfidentialCompute=true",
			disk:                              CreateDiskWithConfidentialCompute(true, hyperdiskBalanced),
			expectedEnableConfidentialCompute: "true",
			expectedDiskType:                  hyperdiskBalanced,
			expectedEnableStoragePools:        "false",
		},
		{
			name:                              "test disk with storage pool projects/my-project/zone/us-central1-a/storagePools/sp1",
			disk:                              CreateDiskWithStoragePool("projects/my-project/zone/us-central1-a/storagePools/sp1", hyperdiskBalanced),
			expectedEnableConfidentialCompute: "false",
			expectedDiskType:                  hyperdiskBalanced,
			expectedEnableStoragePools:        "true",
		},
		{
			name:                              "test disk with no storage pool",
			disk:                              CreateDiskWithStoragePool("", hyperdiskBalanced),
			expectedEnableConfidentialCompute: "false",
			expectedDiskType:                  hyperdiskBalanced,
			expectedEnableStoragePools:        "false",
		},
		{
			name:                              "test nil disk",
			disk:                              nil,
			expectedEnableConfidentialCompute: DefaultEnableConfidentialCompute,
			expectedDiskType:                  DefaultDiskTypeForMetric,
			expectedEnableStoragePools:        DefaultEnableStoragePools,
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		ctx := context.TODO()
		requestMetadata := newRequestMetadata()
		newCtx := context.WithValue(ctx, requestMetadataKey, requestMetadata)
		UpdateRequestMetadataFromDisk(newCtx, tc.disk)
		if requestMetadata.enableConfidentialStorage != tc.expectedEnableConfidentialCompute {
			t.Fatalf("Got confidentialCompute value %q expected %q", requestMetadata.enableConfidentialStorage, tc.expectedEnableConfidentialCompute)
		}
		if requestMetadata.diskType != tc.expectedDiskType {
			t.Fatalf("Got diskType value %q expected %q", requestMetadata.enableConfidentialStorage, tc.expectedDiskType)
		}
		if requestMetadata.enableStoragePools != tc.expectedEnableStoragePools {
			t.Fatalf("Got enableStoragePools value %q expected %q", requestMetadata.enableStoragePools, tc.expectedEnableStoragePools)
		}
	}
}

func TestErrorCodeLabelValue(t *testing.T) {
	testCases := []struct {
		name          string
		operationErr  error
		wantErrorCode string
	}{
		{
			name:          "Not googleapi.Error",
			operationErr:  errors.New("I am not a googleapi.Error"),
			wantErrorCode: "Internal",
		},
		{
			name:          "User error",
			operationErr:  &googleapi.Error{Code: http.StatusBadRequest, Message: "User error with bad request"},
			wantErrorCode: "InvalidArgument",
		},
		{
			name:          "googleapi.Error but not a user error",
			operationErr:  &googleapi.Error{Code: http.StatusInternalServerError, Message: "Internal error"},
			wantErrorCode: "Internal",
		},
		{
			name:          "context canceled error",
			operationErr:  context.Canceled,
			wantErrorCode: "Canceled",
		},
		{
			name:          "context deadline exceeded error",
			operationErr:  context.DeadlineExceeded,
			wantErrorCode: "DeadlineExceeded",
		},
		{
			name:          "status error with Aborted error code",
			operationErr:  status.Error(codes.Aborted, "aborted error"),
			wantErrorCode: "Aborted",
		},
		{
			name:          "user multiattach error",
			operationErr:  fmt.Errorf("The disk resource 'projects/foo/disk/bar' is already being used by 'projects/foo/instances/1'"),
			wantErrorCode: "InvalidArgument",
		},
		{
			name:          "TemporaryError that wraps googleapi error",
			operationErr:  common.NewTemporaryError(codes.Unavailable, &googleapi.Error{Code: http.StatusBadRequest, Message: "User error with bad request"}),
			wantErrorCode: "InvalidArgument",
		},
		{
			name:          "TemporaryError that wraps fmt.Errorf, which wraps googleapi error",
			operationErr:  common.NewTemporaryError(codes.Aborted, fmt.Errorf("got error: %w", &googleapi.Error{Code: http.StatusBadRequest, Message: "User error with bad request"})),
			wantErrorCode: "InvalidArgument",
		},
		{
			name:          "TemporaryError that wraps status error",
			operationErr:  common.NewTemporaryError(codes.Aborted, status.Error(codes.InvalidArgument, "User error with bad request")),
			wantErrorCode: "InvalidArgument",
		},
		{
			name:          "TemporaryError that wraps multiattach error",
			operationErr:  common.NewTemporaryError(codes.Unavailable, fmt.Errorf("The disk resource 'projects/foo/disk/bar' is already being used by 'projects/foo/instances/1'")),
			wantErrorCode: "InvalidArgument",
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		errCode := errorCodeLabelValue(tc.operationErr)
		if diff := cmp.Diff(tc.wantErrorCode, errCode); diff != "" {
			t.Errorf("%s: -want err, +got err\n%s", tc.name, diff)
		}
	}
}

func TestMountOperationError(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "no error",
			want: "OK",
		},
		{
			name: "unknown error",
			err:  fmt.Errorf("fake error"),
			want: "UnknownError",
		},
		{
			name: "mount error",
			err:  mount.NewMountError(mount.FormatFailed, "file system format failed"),
			want: string(mount.FormatFailed),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := mountErrorType(tc.err)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("%s: -want err, +got err\n%s", tc.name, diff)
			}
		})
	}
}
