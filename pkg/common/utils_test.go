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
	"testing"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	volIDZoneFmt   = "projects/%s/zones/%s/disks/%s"
	volIDRegionFmt = "projects/%s/regions/%s/disks/%s"
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

func TestConvertLabelsStringToMap(t *testing.T) {
	t.Run("parsing labels string into map", func(t *testing.T) {
		testCases := []struct {
			name           string
			labels         string
			expectedOutput map[string]string
			expectedError  bool
		}{
			{
				name:           "should return empty map when labels string is empty",
				labels:         "",
				expectedOutput: map[string]string{},
				expectedError:  false,
			},
			{
				name:   "single label string",
				labels: "key=value",
				expectedOutput: map[string]string{
					"key": "value",
				},
				expectedError: false,
			},
			{
				name:   "multiple label string",
				labels: "key1=value1,key2=value2",
				expectedOutput: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				expectedError: false,
			},
			{
				name:   "multiple labels string with whitespaces gets trimmed",
				labels: "key1=value1, key2=value2",
				expectedOutput: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				expectedError: false,
			},
			{
				name:           "malformed labels string (no keys and values)",
				labels:         ",,",
				expectedOutput: nil,
				expectedError:  true,
			},
			{
				name:           "malformed labels string (incorrect format)",
				labels:         "foo,bar",
				expectedOutput: nil,
				expectedError:  true,
			},
			{
				name:           "malformed labels string (missing key)",
				labels:         "key1=value1,=bar",
				expectedOutput: nil,
				expectedError:  true,
			},
			{
				name:           "malformed labels string (missing key and value)",
				labels:         "key1=value1,=bar,=",
				expectedOutput: nil,
				expectedError:  true,
			},
		}

		for _, tc := range testCases {
			t.Logf("test case: %s", tc.name)
			output, err := ConvertLabelsStringToMap(tc.labels)
			if tc.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if err != nil {
				if !tc.expectedError {
					t.Errorf("Did not expect error but got: %v", err)
				}
				continue
			}

			if !reflect.DeepEqual(output, tc.expectedOutput) {
				t.Errorf("Got labels %v, but expected %v", output, tc.expectedOutput)
			}
		}
	})

	t.Run("checking google requirements", func(t *testing.T) {
		testCases := []struct {
			name          string
			labels        string
			expectedError bool
		}{
			{
				name: "64 labels at most",
				labels: `k1=v,k2=v,k3=v,k4=v,k5=v,k6=v,k7=v,k8=v,k9=v,k10=v,k11=v,k12=v,k13=v,k14=v,k15=v,k16=v,k17=v,k18=v,k19=v,k20=v,
                         k21=v,k22=v,k23=v,k24=v,k25=v,k26=v,k27=v,k28=v,k29=v,k30=v,k31=v,k32=v,k33=v,k34=v,k35=v,k36=v,k37=v,k38=v,k39=v,k40=v,
                         k41=v,k42=v,k43=v,k44=v,k45=v,k46=v,k47=v,k48=v,k49=v,k50=v,k51=v,k52=v,k53=v,k54=v,k55=v,k56=v,k57=v,k58=v,k59=v,k60=v,
                         k61=v,k62=v,k63=v,k64=v,k65=v`,
				expectedError: true,
			},
			{
				name:          "label key must start with lowercase char (# case)",
				labels:        "#k=v",
				expectedError: true,
			},
			{
				name:          "label key must start with lowercase char (_ case)",
				labels:        "_k=v",
				expectedError: true,
			},
			{
				name:          "label key must start with lowercase char (- case)",
				labels:        "-k=v",
				expectedError: true,
			},
			{
				name:          "label key can only contain lowercase chars, digits, _ and -)",
				labels:        "k*=v",
				expectedError: true,
			},
			{
				name:          "label key may not have over 63 characters",
				labels:        "abcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghij1234=v",
				expectedError: true,
			},
			{
				name:          "label key cannot contain . and /",
				labels:        "kubernetes.io/created-for/pvc/namespace=v",
				expectedError: true,
			},
			{
				name:          "label value can only contain lowercase chars, digits, _ and -)",
				labels:        "k1=###",
				expectedError: true,
			},
			{
				name:          "label value may not have over 63 characters",
				labels:        "abcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghij1234=v",
				expectedError: true,
			},
			{
				name:          "label value cannot contain . and /",
				labels:        "kubernetes_io_created-for_pvc_namespace=v./",
				expectedError: true,
			},
			{
				name:          "label key can have up to 63 characters",
				labels:        "abcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghij123=v",
				expectedError: false,
			},
			{
				name:          "label value can have up to 63 characters",
				labels:        "abcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghij123=v",
				expectedError: false,
			},
			{
				name:          "label key can contain _ and -",
				labels:        "kubernetes_io_created-for_pvc_namespace=v",
				expectedError: false,
			},
			{
				name:          "label value can contain _ and -",
				labels:        "k=my_value-2",
				expectedError: false,
			},
		}

		for _, tc := range testCases {
			t.Logf("test case: %s", tc.name)
			_, err := ConvertLabelsStringToMap(tc.labels)

			if tc.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tc.expectedError && err != nil {
				t.Errorf("Did not expect error but got: %v", err)
			}
		}
	})

}

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
			// Zones are not valid bucket/snapshot locations.
			"single zone",
			"us-east1a",
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

func TestConvertStringToInt64(t *testing.T) {
	tests := []struct {
		desc        string
		inputStr    string
		expInt64    int64
		expectError bool
	}{
		{
			desc:        "valid number string",
			inputStr:    "10000",
			expInt64:    10000,
			expectError: false,
		},
		{
			desc:        "test higher number",
			inputStr:    "15000",
			expInt64:    15000,
			expectError: false,
		},
		{
			desc:        "round M to number",
			inputStr:    "1M",
			expInt64:    1000000,
			expectError: false,
		},
		{
			desc:        "round m to number",
			inputStr:    "1m",
			expInt64:    1,
			expectError: false,
		},
		{
			desc:        "round k to number",
			inputStr:    "1k",
			expInt64:    1000,
			expectError: false,
		},
		{
			desc:        "invalid empty string",
			inputStr:    "",
			expInt64:    0,
			expectError: true,
		},
		{
			desc:        "invalid string",
			inputStr:    "ew%65",
			expInt64:    0,
			expectError: true,
		},
		{
			desc:        "invalid KiB string",
			inputStr:    "10KiB",
			expInt64:    10000,
			expectError: true,
		},
		{
			desc:        "invalid GB string",
			inputStr:    "10GB",
			expInt64:    0,
			expectError: true,
		},
		{
			desc:        "round Ki to number",
			inputStr:    "1Ki",
			expInt64:    1024,
			expectError: false,
		},
		{
			desc:        "round k to number",
			inputStr:    "10k",
			expInt64:    10000,
			expectError: false,
		},
		{
			desc:        "round Mi to number",
			inputStr:    "10Mi",
			expInt64:    10485760,
			expectError: false,
		},
		{
			desc:        "round M to number",
			inputStr:    "10M",
			expInt64:    10000000,
			expectError: false,
		},
		{
			desc:        "round G to number",
			inputStr:    "10G",
			expInt64:    10000000000,
			expectError: false,
		},
		{
			desc:        "round Gi to number",
			inputStr:    "100Gi",
			expInt64:    107374182400,
			expectError: false,
		},
		{
			desc:        "round decimal to number",
			inputStr:    "1.2Gi",
			expInt64:    1288490189,
			expectError: false,
		},
		{
			desc:        "round big value to number",
			inputStr:    "8191Pi",
			expInt64:    9222246136947933184,
			expectError: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			actualInt64, err := ConvertStringToInt64(tc.inputStr)
			if err != nil && !tc.expectError {
				t.Errorf("Got error %v converting string to int64 %s; expect no error", err, tc.inputStr)
			}
			if err == nil && tc.expectError {
				t.Errorf("Got no error converting string to int64 %s; expect an error", tc.inputStr)
			}
			if err == nil && actualInt64 != tc.expInt64 {
				t.Errorf("Got %d for converting string to int64; expect %d", actualInt64, tc.expInt64)
			}
		})
	}
}

func TestConvertMiStringToInt64(t *testing.T) {
	tests := []struct {
		desc        string
		inputStr    string
		expInt64    int64
		expectError bool
	}{
		{
			desc:        "valid number string",
			inputStr:    "10000",
			expInt64:    1,
			expectError: false,
		},
		{
			desc:        "round Ki to MiB",
			inputStr:    "1000Ki",
			expInt64:    1,
			expectError: false,
		},
		{
			desc:        "round k to MiB",
			inputStr:    "1000k",
			expInt64:    1,
			expectError: false,
		},
		{
			desc:        "round Mi to MiB",
			inputStr:    "1000Mi",
			expInt64:    1000,
			expectError: false,
		},
		{
			desc:        "round M to MiB",
			inputStr:    "1000M",
			expInt64:    954,
			expectError: false,
		},
		{
			desc:        "round G to MiB",
			inputStr:    "1000G",
			expInt64:    953675,
			expectError: false,
		},
		{
			desc:        "round Gi to MiB",
			inputStr:    "10000Gi",
			expInt64:    10240000,
			expectError: false,
		},
		{
			desc:        "round decimal to MiB",
			inputStr:    "1.2Gi",
			expInt64:    1229,
			expectError: false,
		},
		{
			desc:        "round big value to MiB",
			inputStr:    "8191Pi",
			expInt64:    8795019280384,
			expectError: false,
		},
		{
			desc:        "invalid empty string",
			inputStr:    "",
			expInt64:    0,
			expectError: true,
		},
		{
			desc:        "invalid KiB string",
			inputStr:    "10KiB",
			expInt64:    10000,
			expectError: true,
		},
		{
			desc:        "invalid GB string",
			inputStr:    "10GB",
			expInt64:    0,
			expectError: true,
		},
		{
			desc:        "invalid string",
			inputStr:    "ew%65",
			expInt64:    0,
			expectError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			actualInt64, err := ConvertMiStringToInt64(tc.inputStr)
			if err != nil && !tc.expectError {
				t.Errorf("Got error %v converting string to int64 %s; expect no error", err, tc.inputStr)
			}
			if err == nil && tc.expectError {
				t.Errorf("Got no error converting string to int64 %s; expect an error", tc.inputStr)
			}
			if err == nil && actualInt64 != tc.expInt64 {
				t.Errorf("Got %d for converting string to int64; expect %d", actualInt64, tc.expInt64)
			}
		})
	}
}

func TestConvertStringToBool(t *testing.T) {
	tests := []struct {
		desc        string
		inputStr    string
		expected    bool
		expectError bool
	}{
		{
			desc:        "valid true",
			inputStr:    "true",
			expected:    true,
			expectError: false,
		},
		{
			desc:        "valid mixed case true",
			inputStr:    "True",
			expected:    true,
			expectError: false,
		},
		{
			desc:        "valid false",
			inputStr:    "false",
			expected:    false,
			expectError: false,
		},
		{
			desc:        "valid mixed case false",
			inputStr:    "False",
			expected:    false,
			expectError: false,
		},
		{
			desc:        "invalid",
			inputStr:    "yes",
			expected:    false,
			expectError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := ConvertStringToBool(tc.inputStr)
			if err != nil && !tc.expectError {
				t.Errorf("Got error %v converting string to bool %s; expect no error", err, tc.inputStr)
			}
			if err == nil && tc.expectError {
				t.Errorf("Got no error converting string to bool %s; expect an error", tc.inputStr)
			}
			if err == nil && got != tc.expected {
				t.Errorf("Got %v for converting string to bool; expect %v", got, tc.expected)
			}
		})
	}
}

func TestConvertStringToAvailabilityClass(t *testing.T) {
	tests := []struct {
		desc        string
		inputStr    string
		expected    string
		expectError bool
	}{
		{
			desc:        "valid none",
			inputStr:    "none",
			expected:    ParameterNoAvailabilityClass,
			expectError: false,
		},
		{
			desc:        "valid mixed case none",
			inputStr:    "None",
			expected:    ParameterNoAvailabilityClass,
			expectError: false,
		},
		{
			desc:        "valid failover",
			inputStr:    "regional-hard-failover",
			expected:    ParameterRegionalHardFailoverClass,
			expectError: false,
		},
		{
			desc:        "valid mixed case failover",
			inputStr:    "Regional-Hard-Failover",
			expected:    ParameterRegionalHardFailoverClass,
			expectError: false,
		},
		{
			desc:        "invalid",
			inputStr:    "yes",
			expected:    "",
			expectError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := ConvertStringToAvailabilityClass(tc.inputStr)
			if err != nil && !tc.expectError {
				t.Errorf("Got error %v converting string to availablity class %s; expect no error", err, tc.inputStr)
			}
			if err == nil && tc.expectError {
				t.Errorf("Got no error converting string to availablity class %s; expect an error", tc.inputStr)
			}
			if err == nil && got != tc.expected {
				t.Errorf("Got %v for converting string to availablity class; expect %v", got, tc.expected)
			}
		})
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
			name:     "status error with Aborted error code",
			inputErr: status.Error(codes.Aborted, "aborted error"),
			expCode:  codes.Aborted,
		},
		{
			name:     "nil error",
			inputErr: nil,
			expCode:  codes.Internal,
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
