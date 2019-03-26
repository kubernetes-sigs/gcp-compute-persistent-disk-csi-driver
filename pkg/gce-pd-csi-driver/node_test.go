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
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	metadataservice "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/metadata"
	mountmanager "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"
)

func getTestGCEDriver(t *testing.T) *GCEDriver {
	gceDriver := GetGCEDriver()
	err := gceDriver.SetupGCEDriver(nil, mountmanager.NewFakeSafeMounter(), mountmanager.NewFakeDeviceUtils(), metadataservice.NewFakeService(), driver, "test-vendor")
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}
	return gceDriver
}

func TestNodeGetVolumeLimits(t *testing.T) {

	gceDriver := getTestGCEDriver(t)
	ns := gceDriver.ns
	req := &csi.NodeGetInfoRequest{}

	testCases := []struct {
		name           string
		machineType    string
		expVolumeLimit int64
	}{
		{
			name:           "Predifined standard machine",
			machineType:    "n1-standard-1",
			expVolumeLimit: volumeLimit128,
		},
		{
			name:           "Predifined micro machine",
			machineType:    "f1-micro",
			expVolumeLimit: volumeLimit16,
		},
		{
			name:           "Predifined small machine",
			machineType:    "g1-small",
			expVolumeLimit: volumeLimit16,
		},
		{
			name:           "Custom machine with 1GiB Mem",
			machineType:    "custom-1-1024",
			expVolumeLimit: volumeLimit128,
		},
		{
			name:           "Custom machine with 4GiB Mem",
			machineType:    "custom-2-4096",
			expVolumeLimit: volumeLimit128,
		},
	}

	for _, tc := range testCases {
		t.Logf("Test case: %s", tc.name)
		metadataservice.SetMachineType(tc.machineType)
		res, err := ns.NodeGetInfo(context.Background(), req)
		if err != nil {
			t.Fatalf("Failed to get node info: %v", err)
		} else {
			volumeLimit := res.GetMaxVolumesPerNode()
			if volumeLimit != tc.expVolumeLimit {
				t.Fatalf("Expected volume limit: %v, got %v, for machine-type: %v",
					tc.expVolumeLimit, volumeLimit, tc.machineType)
			}
			t.Logf("Get node info: %v", res)
		}
	}
}
