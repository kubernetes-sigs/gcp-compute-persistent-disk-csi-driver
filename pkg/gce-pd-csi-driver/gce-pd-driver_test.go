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
	"time"

	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
)

func initGCEDriver(t *testing.T, cloudDisks []*gce.CloudDisk) *GCEDriver {
	fakeCloudProvider, err := gce.CreateFakeCloudProvider(project, zone, cloudDisks)
	if err != nil {
		t.Fatalf("Failed to create fake cloud provider: %v", err)
	}
	return initGCEDriverWithCloudProvider(t, fakeCloudProvider)
}

func initBlockingGCEDriver(t *testing.T, cloudDisks []*gce.CloudDisk, readyToExecute chan chan gce.Signal) *GCEDriver {
	fakeCloudProvider, err := gce.CreateFakeCloudProvider(project, zone, cloudDisks)
	if err != nil {
		t.Fatalf("Failed to create fake cloud provider: %v", err)
	}
	fakeBlockingBlockProvider := &gce.FakeBlockingCloudProvider{
		FakeCloudProvider: fakeCloudProvider,
		ReadyToExecute:    readyToExecute,
	}
	return initGCEDriverWithCloudProvider(t, fakeBlockingBlockProvider)
}

func controllerServerForTest(cloudProvider gce.GCECompute) *GCEControllerServer {
	gceDriver := GetGCEDriver()
	errorBackoffInitialDuration := 200 * time.Millisecond
	errorBackoffMaxDuration := 5 * time.Minute
	fallbackRequisiteZones := []string{}
	enableStoragePools := false
	enableDataCache := false
	multiZoneVolumeHandleConfig := MultiZoneVolumeHandleConfig{}
	listVolumesConfig := ListVolumesConfig{}
	provisionableDisksConfig := ProvisionableDisksConfig{
		SupportsIopsChange:       []string{"hyperdisk-balanced", "hyperdisk-extreme"},
		SupportsThroughputChange: []string{"hyperdisk-balanced", "hyperdisk-throughput", "hyperdisk-ml"},
	}

	return NewControllerServer(gceDriver, cloudProvider, errorBackoffInitialDuration, errorBackoffMaxDuration, fallbackRequisiteZones, enableStoragePools, enableDataCache, multiZoneVolumeHandleConfig, listVolumesConfig, provisionableDisksConfig)
}

func initGCEDriverWithCloudProvider(t *testing.T, cloudProvider gce.GCECompute) *GCEDriver {
	vendorVersion := "test-vendor"
	gceDriver := GetGCEDriver()
	controllerServer := controllerServerForTest(cloudProvider)
	err := gceDriver.SetupGCEDriver(driver, vendorVersion, nil, nil, nil, controllerServer, nil)
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}
	return gceDriver
}
