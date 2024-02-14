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
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"google.golang.org/api/googleapi"
)

type mockTokenSource struct{}

func (*mockTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken:  "access",
		TokenType:    "Bearer",
		RefreshToken: "refresh",
		Expiry:       time.Now().Add(1 * time.Hour),
	}, nil
}
func TestIsGCEError(t *testing.T) {
	testCases := []struct {
		name          string
		inputErr      error
		reason        string
		expIsGCEError bool
	}{
		{
			name:          "Not googleapi.Error",
			inputErr:      errors.New("I am not a googleapi.Error"),
			reason:        "notFound",
			expIsGCEError: false,
		},
		{
			name: "googleapi.Error not found error",
			inputErr: &googleapi.Error{
				Code: http.StatusNotFound,
				Errors: []googleapi.ErrorItem{
					{
						Reason: "notFound",
					},
				},
				Message: "Not found",
			},
			reason:        "notFound",
			expIsGCEError: true,
		},
		{
			name: "wrapped googleapi.Error",
			inputErr: fmt.Errorf("encountered not found: %w", &googleapi.Error{
				Code: http.StatusNotFound,
				Errors: []googleapi.ErrorItem{
					{
						Reason: "notFound",
					},
				},
				Message: "Not found",
			},
			),
			reason:        "notFound",
			expIsGCEError: true,
		},
		{
			name:          "nil error",
			inputErr:      nil,
			reason:        "notFound",
			expIsGCEError: false,
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test: %v", tc.name)
		isGCEError := IsGCEError(tc.inputErr, tc.reason)
		if tc.expIsGCEError != isGCEError {
			t.Fatalf("Got isGCEError '%t', expected '%t'", isGCEError, tc.expIsGCEError)
		}
	}
}

func TestGetComputeVersion(t *testing.T) {
	testCases := []struct {
		name               string
		computeEndpoint    string
		computeEnvironment Environment
		computeVersion     Version
		expectedEndpoint   string
		expectError        bool
	}{

		{
			name:               "check for production environment",
			computeEndpoint:    "https://compute.googleapis.com",
			computeEnvironment: "production",
			computeVersion:     "v1",
			expectedEndpoint:   "https://compute.googleapis.com/compute/v1/",
			expectError:        false,
		},
		{
			name:               "check for incorrect endpoint",
			computeEndpoint:    "https://compute.googleapis",
			computeEnvironment: "prod",
			computeVersion:     "v1",
			expectError:        true,
		},
		{
			name:               "check for staging environment",
			computeEndpoint:    "https://compute.googleapis.com",
			computeEnvironment: environmentStaging,
			computeVersion:     "v1",
			expectedEndpoint:   "compute/staging_v1/",
			expectError:        false,
		},
		{
			name:               "check for random string as endpoint",
			computeEndpoint:    "compute-googleapis",
			computeEnvironment: "prod",
			computeVersion:     "v1",
			expectedEndpoint:   "compute/v1/",
			expectError:        true,
		},
	}
	for _, tc := range testCases {
		ctx := context.Background()
		_, err := getComputeVersion(ctx, &mockTokenSource{}, tc.computeEndpoint, tc.computeEnvironment, tc.computeVersion)
		if err != nil && !tc.expectError {
			t.Fatalf("Got error %v, expected endpoint %s", err, tc.expectedEndpoint)
		}
	}

}
