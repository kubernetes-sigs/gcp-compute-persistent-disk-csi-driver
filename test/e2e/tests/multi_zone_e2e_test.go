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
	"path/filepath"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"
	remote "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote"
)

var _ = Describe("GCE PD CSI Driver Multi-Zone", func() {
	BeforeEach(func() {
		Expect(len(testInstances)).To(BeNumerically(">", 1))
		// TODO: Check whether the instances are in different zones???
		// I Think there should be a better way of guaranteeing this. Like a map from zone to instance for testInstances (?)
	})

	It("Should get reasonable topology from nodes with NodeGetInfo", func() {
		for _, instance := range testInstances {
			testContext, err := testutils.GCEClientAndDriverSetup(instance)
			Expect(err).To(BeNil(), "Set up new Driver and Client failed with error")
			defer func() {
				err := remote.TeardownDriverAndClient(testContext)
				Expect(err).To(BeNil(), "Teardown Driver and Client failed with error")
			}()

			resp, err := testContext.Client.NodeGetInfo()
			Expect(err).To(BeNil())

			// Get Cloud Instance
			p, z, n := testContext.Instance.GetIdentity()
			cloudInstance, err := computeService.Instances.Get(p, z, n).Do()
			Expect(err).To(BeNil())
			Expect(cloudInstance).ToNot(BeNil())

			// Check topology matches
			segments := resp.GetAccessibleTopology().GetSegments()
			Expect(segments).ToNot(BeNil())

			Expect(segments[common.TopologyKeyZone]).To(Equal(z))
			Expect(len(segments)).To(Equal(1))
		}

	})

	It("Should successfully run through entire lifecycle of an RePD volume on instances in 2 zones", func() {
		// Create new driver and client

		Expect(testInstances).NotTo(BeEmpty())

		zoneToContext := map[string]*remote.TestContext{}
		zones := []string{}

		for _, i := range testInstances {
			_, z, _ := i.GetIdentity()
			// Zone hasn't been seen before
			if _, ok := zoneToContext[z]; !ok {
				c, err := testutils.GCEClientAndDriverSetup(i)
				Expect(err).To(BeNil(), "Set up new Driver and Client failed with error")
				zoneToContext[z] = c
				zones = append(zones, z)

				defer func() {
					err := remote.TeardownDriverAndClient(c)
					Expect(err).To(BeNil(), "Teardown Driver and Client failed with error")
				}()
			}
			if len(zoneToContext) == 2 {
				break
			}
		}

		Expect(len(zoneToContext)).To(Equal(2), "Must have instances in exactly 2 zones")

		controllerContext := zoneToContext[zones[0]]
		controllerClient := controllerContext.Client
		controllerInstance := controllerContext.Instance

		p, _, _ := controllerInstance.GetIdentity()

		region, err := common.GetRegionFromZones(zones)
		Expect(err).To(BeNil(), "Failed to get region from zones")

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volId, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyReplicationType: "regional-pd",
		}, defaultSizeGb, &csi.TopologyRequirement{
			Requisite: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: zones[0]},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: zones[1]},
				},
			},
		})
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := betaComputeService.RegionDisks.Get(p, region, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))
		Expect(len(cloudDisk.ReplicaZones)).To(Equal(2))
		zonesSet := sets.NewString(zones...)
		for _, replicaZone := range cloudDisk.ReplicaZones {
			tokens := strings.Split(replicaZone, "/")
			actualZone := tokens[len(tokens)-1]
			Expect(zonesSet.Has(actualZone)).To(BeTrue(), "Expected zone %v to exist in zone set %v", actualZone, zones)
		}

		defer func() {
			// Delete Disk
			controllerClient.DeleteVolume(volId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// TODO: Validate Disk Deleted
			_, err = betaComputeService.RegionDisks.Get(p, region, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// For each of the two instances
		i := 0
		for _, testContext := range zoneToContext {
			readOnly := false
			if i >= 1 {
				readOnly = true
			}
			testAttachWriteReadDetach(volId, volName, testContext.Instance, testContext.Client, readOnly)
			i = i + 1
		}

	})

})

func testAttachWriteReadDetach(volId string, volName string, instance *remote.InstanceInfo, client *remote.CsiClient, readOnly bool) {
	var err error

	Logf("Starting testAttachWriteReadDetach with volume %v node %v with readonly %v\n", volId, instance.GetNodeID(), readOnly)
	// Attach Disk
	err = client.ControllerPublishVolume(volId, instance.GetNodeID())
	Expect(err).To(BeNil(), "ControllerPublishVolume failed with error for disk %v on node %v", volId, instance.GetNodeID())

	defer func() {

		// Detach Disk
		err = client.ControllerUnpublishVolume(volId, instance.GetNodeID())
		Expect(err).To(BeNil(), "ControllerUnpublishVolume failed with error")
	}()

	// Stage Disk
	stageDir := filepath.Join("/tmp/", volName, "stage")
	client.NodeStageVolume(volId, stageDir)
	Expect(err).To(BeNil(), "NodeStageVolume failed with error")

	defer func() {
		// Unstage Disk
		err = client.NodeUnstageVolume(volId, stageDir)
		Expect(err).To(BeNil(), "NodeUnstageVolume failed with error")
		err = testutils.RmAll(instance, filepath.Join("/tmp/", volName))
		Expect(err).To(BeNil(), "Failed to remove temp directory")
	}()

	// Mount Disk
	publishDir := filepath.Join("/tmp/", volName, "mount")
	err = client.NodePublishVolume(volId, stageDir, publishDir)
	Expect(err).To(BeNil(), "NodePublishVolume failed with error")
	err = testutils.ForceChmod(instance, filepath.Join("/tmp/", volName), "777")
	Expect(err).To(BeNil(), "Chmod failed with error")
	testFileContents := "test"
	if !readOnly {
		// Write a file
		testFile := filepath.Join(publishDir, "testfile")
		err = testutils.WriteFile(instance, testFile, testFileContents)
		Expect(err).To(BeNil(), "Failed to write file")
	}

	// Unmount Disk
	err = client.NodeUnpublishVolume(volId, publishDir)
	Expect(err).To(BeNil(), "NodeUnpublishVolume failed with error")

	// Mount disk somewhere else
	secondPublishDir := filepath.Join("/tmp/", volName, "secondmount")
	err = client.NodePublishVolume(volId, stageDir, secondPublishDir)
	Expect(err).To(BeNil(), "NodePublishVolume failed with error")
	err = testutils.ForceChmod(instance, filepath.Join("/tmp/", volName), "777")
	Expect(err).To(BeNil(), "Chmod failed with error")

	// Read File
	secondTestFile := filepath.Join(secondPublishDir, "testfile")
	readContents, err := testutils.ReadFile(instance, secondTestFile)
	Expect(err).To(BeNil(), "ReadFile failed with error")
	Expect(strings.TrimSpace(string(readContents))).To(Equal(testFileContents))

	// Unmount Disk
	err = client.NodeUnpublishVolume(volId, secondPublishDir)
	Expect(err).To(BeNil(), "NodeUnpublishVolume failed with error")

	Logf("Completed testAttachWriteReadDetach with volume %v node %v\n", volId, instance.GetNodeID())
}
