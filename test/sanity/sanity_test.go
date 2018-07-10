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

package sanitytest

import (
	"testing"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"

	sanity "github.com/kubernetes-csi/csi-test/pkg/sanity"
	compute "google.golang.org/api/compute/v1"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider"
	driver "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-pd-csi-driver"
)

func TestSanity(t *testing.T) {
	// Set up variables
	driverName := "test-driver"
	nodeID := "io.kubernetes.storage.mock"
	project := "test-project"
	zone := "test-zone"
	vendorVersion := "test-version"
	endpoint := "unix:/tmp/csi.sock"
	mountPath := "/tmp/csi/mount"
	stagePath := "/tmp/csi/stage"
	// Set up driver and env
	gceDriver := driver.GetGCEDriver()

	cloudProvider, err := gce.FakeCreateCloudProvider(project, zone)
	if err != nil {
		t.Fatalf("Failed to get cloud provider: %v", err)
	}

	mounter, err := mountmanager.CreateFakeMounter()
	if err != nil {
		t.Fatalf("Failed to get mounter %v", err)
	}

	//Initialize GCE Driver
	err = gceDriver.SetupGCEDriver(cloudProvider, mounter, driverName, nodeID, vendorVersion)
	if err != nil {
		t.Fatalf("Failed to initialize GCE CSI Driver: %v", err)
	}

	instance := &compute.Instance{
		Name:  nodeID,
		Disks: []*compute.AttachedDisk{},
	}
	cloudProvider.InsertInstance(instance, nodeID)

	go func() {
		gceDriver.Run(endpoint)
	}()

	// Run test
	config := &sanity.Config{
		TargetPath:  mountPath,
		StagingPath: stagePath,
		Address:     endpoint,
	}
	sanity.Test(t, config)

}
