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

package gceGCEDriver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
)

var (
	stdVolCap = &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
	stdVolCaps = []*csi.VolumeCapability{
		stdVolCap,
	}
)

func createVolumeCapabilities(am csi.VolumeCapability_AccessMode_Mode) []*csi.VolumeCapability {
	return []*csi.VolumeCapability{
		createVolumeCapability(am),
	}
}

func createVolumeCapability(am csi.VolumeCapability_AccessMode_Mode) *csi.VolumeCapability {
	return &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: am,
		},
	}
}

func createBlockVolumeCapabilities(am csi.VolumeCapability_AccessMode_Mode) []*csi.VolumeCapability {
	return []*csi.VolumeCapability{
		createBlockVolumeCapability(am),
	}
}

func createBlockVolumeCapability(am csi.VolumeCapability_AccessMode_Mode) *csi.VolumeCapability {
	return &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: am,
		},
	}
}

func TestValidateVolumeCapabilities(t *testing.T) {
	testCases := []struct {
		name   string
		vc     []*csi.VolumeCapability
		expErr bool
	}{
		{
			name: "success with empty capabilities",
			vc:   []*csi.VolumeCapability{},
		},
		{
			name: "fail with capabilities no access mode",
			vc: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
				},
			},
			expErr: true,
		},
		{
			name: "fail with capabilities no mode",
			vc: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{},
				},
			},
			expErr: true,
		},
		{
			name: "fail with capabilities no access type",
			vc: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			expErr: true,
		},
		{
			name: "success with mount/SINGLE_NODE_WRITER capabilities",
			vc:   createVolumeCapabilities(csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER),
		},
		{
			name: "success with mount/SINGLE_NODE_READER_ONLY capabilities",
			vc:   createVolumeCapabilities(csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY),
		},
		{
			name: "success with mount/MULTI_NODE_READER_ONLY capabilities",
			vc:   createVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY),
		},
		{
			name:   "fail with mount/MULTI_NODE_SINGLE_WRITER capabilities",
			vc:     createVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER),
			expErr: true,
		},
		{
			name:   "fail with mount/MULTI_NODE_MULTI_WRITER capabilities",
			vc:     createVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER),
			expErr: true,
		},
		{
			name:   "fail with mount/UNKNOWN capabilities",
			vc:     createVolumeCapabilities(csi.VolumeCapability_AccessMode_UNKNOWN),
			expErr: true,
		},
		{
			name: "success with block capabilities",
			vc:   createBlockVolumeCapabilities(csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER),
		},
		{
			name: "success with block/MULTI_NODE_MULTI_WRITER capabilities",
			vc:   createBlockVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER),
		},
		{
			name:   "fail with block/MULTI_NODE_SINGLE_WRITER capabilities",
			vc:     createBlockVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER),
			expErr: true,
		},
		{
			name: "success with reader + writer capabilities",
			vc: []*csi.VolumeCapability{
				createVolumeCapability(csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY),
				createVolumeCapability(csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER),
			},
		},
		{
			name: "success with different reader capabilities",
			vc: []*csi.VolumeCapability{
				createVolumeCapability(csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY),
				createVolumeCapability(csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY),
			},
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		err := validateVolumeCapabilities(tc.vc)
		if tc.expErr && err == nil {
			t.Fatalf("Expected error but didn't get any")
		}
		if !tc.expErr && err != nil {
			t.Fatalf("Did not expect error but got: %v", err)
		}
	}
}

func TestGetMultiWriterFromCapabilities(t *testing.T) {
	testCases := []struct {
		name   string
		vc     []*csi.VolumeCapability
		expVal bool
		expErr bool
	}{
		{
			name:   "false with empty capabilities",
			vc:     []*csi.VolumeCapability{},
			expVal: false,
		},
		{
			name: "fail with capabilities no access mode",
			vc: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
				},
			},
			expErr: true,
		},
		{
			name:   "false with mount/SINGLE_NODE_WRITER capabilities",
			vc:     createVolumeCapabilities(csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER),
			expVal: false,
		},
		{
			name:   "true with block/MULTI_NODE_MULTI_WRITER capabilities",
			vc:     createBlockVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER),
			expVal: true,
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		val, err := getMultiWriterFromCapabilities(tc.vc)
		if tc.expErr && err == nil {
			t.Fatalf("Expected error but didn't get any")
		}
		if !tc.expErr && err != nil {
			t.Fatalf("Did not expect error but got: %v", err)
		}
		if err != nil {
			if tc.expVal != val {
				t.Fatalf("Expected '%t' but got '%t'", tc.expVal, val)
			}
		}
	}
}

func TestGetReadOnlyFromCapabilities(t *testing.T) {
	testCases := []struct {
		name   string
		vc     []*csi.VolumeCapability
		expVal bool
		expErr bool
	}{
		{
			name:   "false with empty capabilities",
			vc:     []*csi.VolumeCapability{},
			expVal: false,
		},
		{
			name: "fail with capabilities no access mode",
			vc: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
				},
			},
			expErr: true,
		},
		{
			name:   "false with SINGLE_NODE_WRITER capabilities",
			vc:     createVolumeCapabilities(csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER),
			expVal: false,
		},
		{
			name:   "true with MULTI_NODE_READER_ONLY capabilities",
			vc:     createBlockVolumeCapabilities(csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY),
			expVal: true,
		},
		{
			name:   "true with SINGLE_NODE_READER_ONLY capabilities",
			vc:     createVolumeCapabilities(csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY),
			expVal: true,
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		val, err := getReadOnlyFromCapabilities(tc.vc)
		if tc.expErr && err == nil {
			t.Fatalf("Expected error but didn't get any")
		}
		if !tc.expErr && err != nil {
			t.Fatalf("Did not expect error but got: %v", err)
		}
		if err != nil {
			if tc.expVal != val {
				t.Fatalf("Expected '%t' but got '%t'", tc.expVal, val)
			}
		}
	}
}

func TestCodeForError(t *testing.T) {
	internalErrorCode := codes.Internal
	userErrorCode := codes.InvalidArgument
	testCases := []struct {
		name     string
		inputErr error
		expCode  *codes.Code
	}{
		{
			name:     "Not googleapi.Error",
			inputErr: errors.New("I am not a googleapi.Error"),
			expCode:  &internalErrorCode,
		},
		{
			name:     "User error",
			inputErr: &googleapi.Error{Code: http.StatusBadRequest, Message: "User error with bad request"},
			expCode:  &userErrorCode,
		},
		{
			name:     "googleapi.Error but not a user error",
			inputErr: &googleapi.Error{Code: http.StatusInternalServerError, Message: "Internal error"},
			expCode:  &internalErrorCode,
		},
		{
			name:     "context canceled error",
			inputErr: context.Canceled,
			expCode:  errCodePtr(codes.Canceled),
		},
		{
			name:     "context deadline exceeded error",
			inputErr: context.DeadlineExceeded,
			expCode:  errCodePtr(codes.DeadlineExceeded),
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		actualCode := *CodeForError(tc.inputErr)
		if *tc.expCode != actualCode {
			t.Fatalf("Expected error code '%v' but got '%v'", tc.expCode, actualCode)
		}
	}
}

func TestIsContextError(t *testing.T) {
	cases := []struct {
		name            string
		err             error
		expectedErrCode *codes.Code
	}{
		{
			name:            "deadline exceeded error",
			err:             context.DeadlineExceeded,
			expectedErrCode: errCodePtr(codes.DeadlineExceeded),
		},
		{
			name:            "contains 'context deadline exceeded'",
			err:             fmt.Errorf("got error: %w", context.DeadlineExceeded),
			expectedErrCode: errCodePtr(codes.DeadlineExceeded),
		},
		{
			name:            "context canceled error",
			err:             context.Canceled,
			expectedErrCode: errCodePtr(codes.Canceled),
		},
		{
			name:            "contains 'context canceled'",
			err:             fmt.Errorf("got error: %w", context.Canceled),
			expectedErrCode: errCodePtr(codes.Canceled),
		},
		{
			name:            "does not contain 'context canceled' or 'context deadline exceeded'",
			err:             fmt.Errorf("unknown error"),
			expectedErrCode: nil,
		},
		{
			name:            "nil error",
			err:             nil,
			expectedErrCode: nil,
		},
	}

	for _, test := range cases {
		errCode := isContextError(test.err)
		if (test.expectedErrCode == nil) != (errCode == nil) {
			t.Errorf("test %v failed: got %v, expected %v", test.name, errCode, test.expectedErrCode)
		}
		if test.expectedErrCode != nil && *errCode != *test.expectedErrCode {
			t.Errorf("test %v failed: got %v, expected %v", test.name, errCode, test.expectedErrCode)
		}
	}
}
