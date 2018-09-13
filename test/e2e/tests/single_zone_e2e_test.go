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

package tests

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"

	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	testNamePrefix = "gcepd-csi-e2e-"

	defaultSizeGb    int64 = 5
	readyState             = "READY"
	standardDiskType       = "pd-standard"
	ssdDiskType            = "pd-ssd"
)

var _ = Describe("GCE PD CSI Driver", func() {

	It("Should create->attach->stage->mount volume and check if it is writable, then unmount->unstage->detach->delete and check disk is deleted", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volID, err := client.CreateVolume(volName, nil, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: z},
					},
				},
			})
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))

		defer func() {
			// Delete Disk
			client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach Disk
		testAttachWriteReadDetach(volID, volName, instance, client, false /* readOnly */)

	})

	It("Should create disks in correct zones when topology is specified", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		p, _, _ := testContext.Instance.GetIdentity()

		zones := []string{"us-central1-c", "us-central1-b", "us-central1-a"}

		for _, zone := range zones {
			volName := testNamePrefix + string(uuid.NewUUID())
			topReq := &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: zone},
					},
				},
			}
			volID, err := testContext.Client.CreateVolume(volName, nil, defaultSizeGb, topReq)
			Expect(err).To(BeNil(), "Failed to create volume")
			defer func() {
				err = testContext.Client.DeleteVolume(volID)
				Expect(err).To(BeNil(), "Failed to delete volume")
			}()

			_, err = computeService.Disks.Get(p, zone, volName).Do()
			Expect(err).To(BeNil(), "Could not find disk in correct zone")
		}

	})

	It("Should successfully create RePD in two zones in the drivers region when none are specified", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		controllerInstance := testContext.Instance
		controllerClient := testContext.Client

		p, z, _ := controllerInstance.GetIdentity()

		region, err := common.GetRegionFromZones([]string{z})
		Expect(err).To(BeNil(), "Failed to get region from zones")

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volID, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyReplicationType: "regional-pd",
		}, defaultSizeGb, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := betaComputeService.RegionDisks.Get(p, region, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))
		Expect(len(cloudDisk.ReplicaZones)).To(Equal(2))
		for _, replicaZone := range cloudDisk.ReplicaZones {
			tokens := strings.Split(replicaZone, "/")
			actualZone := tokens[len(tokens)-1]
			gotRegion, err := common.GetRegionFromZones([]string{actualZone})
			Expect(err).To(BeNil(), "failed to get region from actual zone %v", actualZone)
			Expect(gotRegion).To(Equal(region), "Got region from replica zone that did not match supplied region")
		}
		defer func() {
			// Delete Disk
			controllerClient.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = betaComputeService.RegionDisks.Get(p, region, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()
	})

	It("Should create and delete disk with default zone", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volID, err := client.CreateVolume(volName, nil, defaultSizeGb, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))

		defer func() {
			// Delete Disk
			client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()
	})

	// Test volume already exists idempotency

	// Test volume with op pending

	It("Should create and delete snapshot for the volume with default zone", func() {
		// Create new driver and client
		Expect(testInstances).NotTo(BeEmpty())
		testContext, err := testutils.GCEClientAndDriverSetup(testInstances[0])
		Expect(err).To(BeNil(), "Set up new Driver and Client failed with error")
		defer func() {
			err := remote.TeardownDriverAndClient(testContext)
			Expect(err).To(BeNil(), "Teardown Driver and Client failed with error")
		}()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volId, err := client.CreateVolume(volName, nil, defaultSizeGb, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))

		// Create Snapshot
		snapshotName := testNamePrefix + string(uuid.NewUUID())
		snapshotId, err := client.CreateSnapshot(snapshotName, volId, nil)
		Expect(err).To(BeNil(), "CreateSnapshot failed with error: %v", err)

		// Validate Snapshot Created
		snapshot, err := computeService.Snapshots.Get(p, snapshotName).Do()
		Expect(err).To(BeNil(), "Could not get snapshot from cloud directly")
		Expect(snapshot.Name).To(Equal(snapshotName))

		err = wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
			snapshot, err := computeService.Snapshots.Get(p, snapshotName).Do()
			Expect(err).To(BeNil(), "Could not get snapshot from cloud directly")
			if snapshot.Status == "READY" {
				return true, nil
			}
			return false, nil
		})
		Expect(err).To(BeNil(), "Could not wait for snapshot be ready")

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")

			// Delete Snapshot
			err = client.DeleteSnapshot(snapshotId)
			Expect(err).To(BeNil(), "DeleteSnapshot failed")

			// Validate Snapshot Deleted
			_, err = computeService.Snapshots.Get(p, snapshotName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected snapshot to not be found")
		}()
	})

	It("Should create and delete snapshot for RePD in two zones ", func() {
		// Create new driver and client
		Expect(testInstances).NotTo(BeEmpty())
		testContext, err := testutils.GCEClientAndDriverSetup(testInstances[0])
		Expect(err).To(BeNil(), "Failed to set up new driver and client")
		defer func() {
			err := remote.TeardownDriverAndClient(testContext)
			Expect(err).To(BeNil(), "Teardown Driver and Client failed with error")
		}()

		controllerInstance := testContext.Instance
		controllerClient := testContext.Client

		p, z, _ := controllerInstance.GetIdentity()

		region, err := common.GetRegionFromZones([]string{z})
		Expect(err).To(BeNil(), "Failed to get region from zones")

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volId, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyReplicationType: "regional-pd",
		}, defaultSizeGb, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := betaComputeService.RegionDisks.Get(p, region, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))
		Expect(len(cloudDisk.ReplicaZones)).To(Equal(2))
		for _, replicaZone := range cloudDisk.ReplicaZones {
			tokens := strings.Split(replicaZone, "/")
			actualZone := tokens[len(tokens)-1]
			gotRegion, err := common.GetRegionFromZones([]string{actualZone})
			Expect(err).To(BeNil(), "failed to get region from actual zone %v", actualZone)
			Expect(gotRegion).To(Equal(region), "Got region from replica zone that did not match supplied region")
		}

		// Create Snapshot
		snapshotName := testNamePrefix + string(uuid.NewUUID())
		snapshotId, err := controllerClient.CreateSnapshot(snapshotName, volId, nil)
		Expect(err).To(BeNil(), "CreateSnapshot failed with error: %v", err)

		// Validate Snapshot Created
		snapshot, err := computeService.Snapshots.Get(p, snapshotName).Do()
		Expect(err).To(BeNil(), "Could not get snapshot from cloud directly")
		Expect(snapshot.Name).To(Equal(snapshotName))

		err = wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
			snapshot, err := computeService.Snapshots.Get(p, snapshotName).Do()
			Expect(err).To(BeNil(), "Could not get snapshot from cloud directly")
			if snapshot.Status == "READY" {
				return true, nil
			}
			return false, nil
		})
		Expect(err).To(BeNil(), "Could not wait for snapshot be ready")

		defer func() {
			// Delete Disk
			err := controllerClient.DeleteVolume(volId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = betaComputeService.RegionDisks.Get(p, region, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")

			// Delete Snapshot
			err = controllerClient.DeleteSnapshot(snapshotId)
			Expect(err).To(BeNil(), "DeleteSnapshot failed")

			// Validate Snapshot Deleted
			_, err = computeService.Snapshots.Get(p, snapshotName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected snapshot to not be found")
		}()
	})
})

func Logf(format string, args ...interface{}) {
	fmt.Fprintf(GinkgoWriter, format, args...)
}
