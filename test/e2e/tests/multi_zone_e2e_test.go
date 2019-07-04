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
	"path/filepath"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/klog"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"
	remote "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote"
)

var _ = Describe("GCE PD CSI Driver Multi-Zone", func() {
	BeforeEach(func() {
		Expect(len(testContexts)).To(BeNumerically(">", 1))
	})

	It("Should get reasonable topology from nodes with NodeGetInfo", func() {
		for _, testContext := range testContexts {
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

		Expect(testContexts).NotTo(BeEmpty())

		zoneToContext := map[string]*remote.TestContext{}
		zones := []string{}

		for _, tc := range testContexts {
			_, z, _ := tc.Instance.GetIdentity()
			// Zone hasn't been seen before
			if _, ok := zoneToContext[z]; !ok {
				zoneToContext[z] = tc
				zones = append(zones, z)
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
		volID, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyReplicationType: "regional-pd",
		}, defaultRepdSizeGb, &csi.TopologyRequirement{
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
		Expect(cloudDisk.SizeGb).To(Equal(defaultRepdSizeGb))
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
			controllerClient.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
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
			testAttachWriteReadDetach(volID, volName, testContext.Instance, testContext.Client, readOnly)
			i = i + 1
		}

	})

})

func testAttachWriteReadDetach(volID string, volName string, instance *remote.InstanceInfo, client *remote.CsiClient, readOnly bool) error {
	var err error

	klog.Infof("Starting testAttachWriteReadDetach with volume %v node %v with readonly %v\n", volID, instance.GetNodeID(), readOnly)
	// Attach Disk
	err = client.ControllerPublishVolume(volID, instance.GetNodeID())
	if err != nil {
		return fmt.Errorf("ControllerPublishVolume failed with error for disk %v on node %v: %v", volID, instance.GetNodeID(), err)
	}

	defer func() {
		// Detach Disk
		err = client.ControllerUnpublishVolume(volID, instance.GetNodeID())
		if err != nil {
			klog.Errorf("Failed to detach disk: %v", err)
		}

	}()

	// Stage Disk
	stageDir := filepath.Join("/tmp/", volName, "stage")
	err = client.NodeStageVolume(volID, stageDir)
	if err != nil {
		return fmt.Errorf("NodeStageVolume failed with error: %v", err)
	}

	defer func() {
		// Unstage Disk
		err = client.NodeUnstageVolume(volID, stageDir)
		if err != nil {
			klog.Errorf("Failed to unstage volume: %v", err)
		}
		fp := filepath.Join("/tmp/", volName)
		err = testutils.RmAll(instance, fp)
		if err != nil {
			klog.Errorf("Failed to rm file path %s: %v", fp, err)
		}
	}()

	// Mount Disk
	publishDir := filepath.Join("/tmp/", volName, "mount")
	err = client.NodePublishVolume(volID, stageDir, publishDir)
	if err != nil {
		return fmt.Errorf("NodePublishVolume failed with error: %v", err)
	}
	err = testutils.ForceChmod(instance, filepath.Join("/tmp/", volName), "777")
	if err != nil {
		return fmt.Errorf("Chmod failed with error: %v", err)
	}
	testFileContents := "test"
	if !readOnly {
		// Write a file
		testFile := filepath.Join(publishDir, "testfile")
		err = testutils.WriteFile(instance, testFile, testFileContents)
		if err != nil {
			return fmt.Errorf("Failed to write file: %v", err)
		}
	}

	// Unmount Disk
	err = client.NodeUnpublishVolume(volID, publishDir)
	if err != nil {
		return fmt.Errorf("NodeUnpublishVolume failed with error: %v", err)
	}

	// Mount disk somewhere else
	secondPublishDir := filepath.Join("/tmp/", volName, "secondmount")
	err = client.NodePublishVolume(volID, stageDir, secondPublishDir)
	if err != nil {
		return fmt.Errorf("NodePublishVolume failed with error: %v", err)
	}
	err = testutils.ForceChmod(instance, filepath.Join("/tmp/", volName), "777")
	if err != nil {
		return fmt.Errorf("Chmod failed with error: %v", err)
	}

	// Read File
	secondTestFile := filepath.Join(secondPublishDir, "testfile")
	readContents, err := testutils.ReadFile(instance, secondTestFile)
	if err != nil {
		return fmt.Errorf("ReadFile failed with error: %v", err)
	}
	Expect(strings.TrimSpace(string(readContents))).To(Equal(testFileContents))

	// Unmount Disk
	err = client.NodeUnpublishVolume(volID, secondPublishDir)
	if err != nil {
		return fmt.Errorf("NodeUnpublishVolume failed with error: %v", err)
	}

	klog.Infof("Completed testAttachWriteReadDetach with volume %v node %v\n", volID, instance.GetNodeID())
	return nil
}
