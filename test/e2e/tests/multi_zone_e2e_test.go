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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"
	remote "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote"
)

type verifyArgs struct {
	publishDir, stageDir string
}

type verifyFunc func(*verifyArgs) error

type detacherFunc func()

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

		Expect(len(zoneToContext)).To(Equal(2), "Must have instances in 2 zones")

		controllerContext := zoneToContext[zones[0]]
		controllerClient := controllerContext.Client
		controllerInstance := controllerContext.Instance

		p, _, _ := controllerInstance.GetIdentity()

		region, err := common.GetRegionFromZones(zones)
		Expect(err).To(BeNil(), "Failed to get region from zones")

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volume, err := controllerClient.CreateVolume(volName, map[string]string{
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
		}, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := computeService.RegionDisks.Get(p, region, volName).Do()
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
			controllerClient.DeleteVolume(volume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.RegionDisks.Get(p, region, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// For each of the two instances
		i := 0
		for _, testContext := range zoneToContext {
			readOnly := false
			if i >= 1 {
				readOnly = true
			}
			err = testAttachWriteReadDetach(volume.VolumeId, volName, testContext.Instance, testContext.Client, readOnly)
			Expect(err).To(BeNil(), "failed volume lifecycle checks")
			i = i + 1
		}
	})

	It("Should create a RePD instance, write to it, force-attach it to another instance, and read the same data", func() {
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

		Expect(len(zoneToContext)).To(Equal(2), "Must have instances in 2 zones")

		controllerContext := zoneToContext[zones[0]]
		controllerClient := controllerContext.Client
		controllerInstance := controllerContext.Instance

		p, _, _ := controllerInstance.GetIdentity()

		region, err := common.GetRegionFromZones(zones)
		Expect(err).To(BeNil(), "Failed to get region from zones")

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volume, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyReplicationType: "regional-pd",
			common.ParameterAvailabilityClass:  common.ParameterRegionalHardFailoverClass,
		}, defaultRepdSizeGb, &csi.TopologyRequirement{
			Requisite: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: zones[0]},
				},
				{
					Segments: map[string]string{common.TopologyKeyZone: zones[1]},
				},
			},
		}, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := computeService.RegionDisks.Get(p, region, volName).Do()
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
		Expect(volume.VolumeContext).To(HaveKeyWithValue("force-attach", "true"))

		detachers := []detacherFunc{}

		defer func() {
			// Perform any detaches
			for _, fn := range detachers {
				fn()
			}

			// Delete Disk
			controllerClient.DeleteVolume(volume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.RegionDisks.Get(p, region, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach disk to instance in the first zone.
		tc0 := zoneToContext[zones[0]]
		err, detacher, args := testAttachAndMount(volume.VolumeId, volName, tc0.Instance, tc0.Client, false /* useBlock */, false /* forceAttach */)
		detachers = append(detachers, detacher)
		Expect(err).To(BeNil(), "failed attach in zone 0")
		testFileName := filepath.Join(args.publishDir, "force-attach-test")
		testFileContents := "force attach test"
		err = testutils.WriteFile(tc0.Instance, testFileName, testFileContents)
		Expect(err).To(BeNil(), "failed write in zone 0")
		_, err = tc0.Instance.SSH("sync") // Sync so force detach doesn't lose data.
		Expect(err).To(BeNil(), "failed sync")

		readContents, err := testutils.ReadFile(tc0.Instance, testFileName)
		Expect(err).To(BeNil(), "failed read in zone 0")
		Expect(strings.TrimSpace(string(readContents))).To(BeIdenticalTo(testFileContents), "content mismatch in zone 0")

		// Now force attach to the second instance without detaching.
		tc1 := zoneToContext[zones[1]]
		err, detacher, args = testAttachAndMount(volume.VolumeId, volName, tc1.Instance, tc1.Client, false /* useBlock */, true /* forceAttach */)
		detachers = append(detachers, detacher)
		Expect(err).To(BeNil(), "failed force attach in zone 1")
		readContents, err = testutils.ReadFile(tc1.Instance, testFileName)
		Expect(err).To(BeNil(), "failed read in zone 1")
		Expect(strings.TrimSpace(string(readContents))).To(BeIdenticalTo(testFileContents), "content mismatch in zone 1")
	})
})

func testAttachWriteReadDetach(volID string, volName string, instance *remote.InstanceInfo, client *remote.CsiClient, readOnly bool) error {
	var testFileContents = "test"
	writeFile := func(a *verifyArgs) error {
		if !readOnly {
			// Write a file
			testFile := filepath.Join(a.publishDir, "testfile")
			err := testutils.WriteFile(instance, testFile, testFileContents)
			if err != nil {
				return fmt.Errorf("Failed to write file: %v", err.Error())
			}
		}
		return nil
	}

	verifyReadFile := func(a *verifyArgs) error {
		// Read File
		secondTestFile := filepath.Join(a.publishDir, "testfile")
		readContents, err := testutils.ReadFile(instance, secondTestFile)
		if err != nil {
			return fmt.Errorf("ReadFile failed with error: %v", err.Error())
		}
		if strings.TrimSpace(string(readContents)) != testFileContents {
			return fmt.Errorf("wanted test file content: %s, got content: %s", testFileContents, readContents)
		}
		return nil
	}
	return testLifecycleWithVerify(volID, volName, instance, client, readOnly, false /* fs */, writeFile, verifyReadFile)
}

func testAttachAndMount(volID string, volName string, instance *remote.InstanceInfo, client *remote.CsiClient, useBlock, forceAttach bool) (error, func(), *verifyArgs) {
	// Attach Disk
	err := client.ControllerPublishVolume(volID, instance.GetNodeID(), forceAttach)
	if err != nil {
		return fmt.Errorf("ControllerPublishVolume failed with error for disk %v on node %v: %v", volID, instance.GetNodeID(), err.Error()), nil, nil
	}

	detach := func() {
		// Detach Disk
		err = client.ControllerUnpublishVolume(volID, instance.GetNodeID())
		if err != nil {
			klog.Errorf("Failed to detach disk: %v", err)
		}
	}

	// Stage Disk
	stageDir := filepath.Join("/tmp/", volName, "stage")
	if useBlock {
		err = client.NodeStageBlockVolume(volID, stageDir)
	} else {
		err = client.NodeStageExt4Volume(volID, stageDir)
	}

	if err != nil {
		detach()
		return fmt.Errorf("NodeStageExt4Volume failed with error: %w", err), nil, nil
	}

	unstageAndDetach := func() {
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

		detach()
	}

	// Mount Disk
	publishDir := filepath.Join("/tmp/", volName, "mount")

	if useBlock {
		err = client.NodePublishBlockVolume(volID, stageDir, publishDir)
	} else {
		err = client.NodePublishVolume(volID, stageDir, publishDir)
	}

	if err != nil {
		unstageAndDetach()
		return fmt.Errorf("NodePublishVolume failed with error: %v", err.Error()), nil, nil
	}
	err = testutils.ForceChmod(instance, filepath.Join("/tmp/", volName), "777")
	if err != nil {
		unstageAndDetach()
		return fmt.Errorf("Chmod failed with error: %v", err.Error()), nil, nil
	}

	args := &verifyArgs{
		publishDir: publishDir,
		stageDir:   stageDir,
	}

	return nil, unstageAndDetach, args
}

func testLifecycleWithVerify(volID string, volName string, instance *remote.InstanceInfo, client *remote.CsiClient, readOnly, useBlock bool, firstMountVerify, secondMountVerify verifyFunc) error {
	klog.Infof("Starting testAttachWriteReadDetach with volume %v node %v with readonly %v\n", volID, instance.GetNodeID(), readOnly)
	err, detacher, args := testAttachAndMount(volID, volName, instance, client, useBlock, false /* forceAttach */)
	if err != nil {
		return fmt.Errorf("failed to attach and mount: %w", err)
	}
	defer detacher()

	err = firstMountVerify(args)
	if err != nil {
		return fmt.Errorf("failed to verify after first mount to %s: %w", args.publishDir, err)
	}

	// Unmount Disk
	err = client.NodeUnpublishVolume(volID, args.publishDir)
	if err != nil {
		return fmt.Errorf("NodeUnpublishVolume failed with error: %v", err.Error())
	}

	if secondMountVerify != nil {
		// Mount disk somewhere else
		secondPublishDir := filepath.Join("/tmp/", volName, "secondmount")
		if useBlock {
			err = client.NodePublishBlockVolume(volID, args.stageDir, secondPublishDir)
		} else {
			err = client.NodePublishVolume(volID, args.stageDir, secondPublishDir)
		}
		if err != nil {
			return fmt.Errorf("NodePublishVolume failed with error: %v", err.Error())
		}
		err = testutils.ForceChmod(instance, filepath.Join("/tmp/", volName), "777")
		if err != nil {
			return fmt.Errorf("Chmod failed with error: %v", err)
		}

		b := verifyArgs{
			publishDir: secondPublishDir,
		}
		err = secondMountVerify(&b)
		if err != nil {
			return fmt.Errorf("failed to verify after second mount to %s: %v", args.publishDir, err.Error())
		}

		// Unmount Disk
		err = client.NodeUnpublishVolume(volID, secondPublishDir)
		if err != nil {
			return fmt.Errorf("NodeUnpublishVolume failed with error: %v", err.Error())
		}
	}

	klog.Infof("Completed testAttachWriteReadDetach with volume %v node %v\n", volID, instance.GetNodeID())
	return nil
}
