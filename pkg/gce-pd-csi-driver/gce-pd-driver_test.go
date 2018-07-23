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

	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	metadataservice "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/metadata"
)

func initGCEDriver(t *testing.T) *GCEDriver {
	vendorVersion := "test-vendor"
	gceDriver := GetGCEDriver()
	fakeCloudProvider, err := gce.FakeCreateCloudProvider(project, zone)
	if err != nil {
		t.Fatalf("Failed to create fake cloud provider: %v", err)
	}
	err = gceDriver.SetupGCEDriver(fakeCloudProvider, nil, nil, metadataservice.NewFakeService(), driver, node, vendorVersion)
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}
	return gceDriver
}
