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
	"fmt"
	"reflect"
	"testing"

	"k8s.io/kubernetes/pkg/cloudprovider/providers/gce/cloud/meta"
)

const (
	volIDZoneFmt   = "projects/%s/zones/%s/disks/%s"
	volIDRegionFmt = "projects/%s/regions/%s/disks/%s"
	nodeIDFmt      = "%s/%s"
)

func TestBytesToGb(t *testing.T) {
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
		gotGB := BytesToGb(tc.bytes)

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
	testRegion := "test-region"

	testCases := []struct {
		name         string
		volID        string
		expPartition string
		expKey       *meta.Key
		expErr       bool
	}{
		{
			name:   "normal zonal",
			volID:  fmt.Sprintf(volIDZoneFmt, testProject, testZone, testName),
			expKey: meta.ZonalKey(testName, testZone),
		},
		{
			name:   "normal regional",
			volID:  fmt.Sprintf(volIDRegionFmt, testProject, testRegion, testName),
			expKey: meta.RegionalKey(testName, testRegion),
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
		{
			name:         "zonal with partition",
			volID:        fmt.Sprintf(volIDZoneFmt, testProject, testZone, testName) + fmt.Sprintf("/partitions/%s", "1"),
			expPartition: "1",
			expKey:       meta.ZonalKey(testName, testZone),
		},
		{
			name:         "regional with partition",
			volID:        fmt.Sprintf(volIDRegionFmt, testProject, testRegion, testName) + fmt.Sprintf("/partitions/%s", "1"),
			expPartition: "1",
			expKey:       meta.RegionalKey(testName, testRegion),
		},
		{
			name:   "malformed with partition",
			volID:  fmt.Sprintf(volIDRegionFmt, testRegion, testZone, testName) + fmt.Sprintf("/bloblab/%s", "1"),
			expErr: true,
		},
	}
	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		gotKey, gotPartition, err := VolumeIDToKey(tc.volID)
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
		if gotPartition != tc.expPartition {
			t.Errorf("Got partition %v, but expected %v, from volume ID %v", gotPartition, tc.expPartition, tc.volID)
		}
	}

}

func TestNodeIDToZoneAndName(t *testing.T) {
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
			nodeID:  fmt.Sprintf(nodeIDFmt, testZone, testName),
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
