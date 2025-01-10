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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	compute "google.golang.org/api/compute/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
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

func runMultiZoneTests() bool {
	runMultiZoneTestsStr, ok := os.LookupEnv("RUN_MULTI_ZONE_TESTS")
	if !ok {
		return false
	}

	runMultiZoneTests, err := strconv.ParseBool(runMultiZoneTestsStr)
	if err != nil {
		return false
	}

	return runMultiZoneTests
}

func checkSkipMultiZoneTests() {
	// TODO: Remove this once hyperdisk-ml SKU is supported
	// If you want to run these tests, set the env variable: RUN_MULTI_ZONE_TESTS=true
	if !runMultiZoneTests() {
		Skip("Not running multi-zone tests, as RUN_MULTI_ZONE_TESTS is falsy")
	}
}

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

	It("Should attach ROX 'multi-zone' PV instances to two separate VMs", func() {
		checkSkipMultiZoneTests()

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

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		_, volID0 := createAndValidateZonalDisk(controllerClient, p, zones[0], "hyperdisk-ml", volName)
		_, volID1 := createAndValidateZonalDisk(controllerClient, p, zones[1], "hyperdisk-ml", volName)

		labelsMap := map[string]string{
			common.MultiZoneLabel: "true",
		}
		disk1, err := computeService.Disks.Get(p, zones[0], volName).Do()
		Expect(err).To(BeNil(), "Could not get disk")
		disk1Op, err := computeService.Disks.SetLabels(p, zones[0], volName, &compute.ZoneSetLabelsRequest{
			LabelFingerprint: disk1.LabelFingerprint,
			Labels:           labelsMap,
		}).Do()
		Expect(err).To(BeNil(), "Could not set disk labels")
		_, err = computeService.ZoneOperations.Wait(p, zones[0], disk1Op.Name).Do()
		Expect(err).To(BeNil(), "Could not set disk labels")

		disk2, err := computeService.Disks.Get(p, zones[1], volName).Do()
		Expect(err).To(BeNil(), "Could not get disk")
		disk2Op, err := computeService.Disks.SetLabels(p, zones[1], volName, &compute.ZoneSetLabelsRequest{
			LabelFingerprint: disk2.LabelFingerprint,
			Labels:           labelsMap,
		}).Do()
		Expect(err).To(BeNil(), "Could not set disk labels")
		_, err = computeService.ZoneOperations.Wait(p, zones[1], disk2Op.Name).Do()
		Expect(err).To(BeNil(), "Could not set disk labels")

		defer deleteDisk(controllerClient, p, zones[0], volID0, volName)
		defer deleteDisk(controllerClient, p, zones[1], volID1, volName)

		// Attach Disk
		volID := fmt.Sprintf("projects/%s/zones/multi-zone/disks/%s", p, volName)

		// Attach disk to instance in the first zone.
		tc0 := zoneToContext[zones[0]]
		tc1 := zoneToContext[zones[1]]

		nodeID0 := tc0.Instance.GetNodeID()
		nodeID1 := tc1.Instance.GetNodeID()

		err = controllerClient.ControllerPublishVolumeReadOnly(volID, nodeID0)
		Expect(err).To(BeNil(), "Failed to attach and mount vol1")

		err = controllerClient.ControllerPublishVolumeReadOnly(volID, nodeID1)
		Expect(err).To(BeNil(), "Failed to attach and mount vol2")

		// List Volumes
		volsToNodes, err := controllerClient.ListVolumes()
		Expect(err).To(BeNil(), "Failed ListVolumes")

		// Verify List Volumes
		Expect(volsToNodes[volID0]).To(ContainElements(nodeID0), "Find find node in attach nodes for vol")
		Expect(volsToNodes[volID1]).To(ContainElements(nodeID1), "Find find node in attach nodes for vol")
		Expect(volsToNodes[volID]).To(ContainElements(nodeID0, nodeID1), "Couldn't find node in attached nodes for vol")

		// Detach disk
		err = controllerClient.ControllerUnpublishVolume(volID, nodeID0)
		Expect(err).To(BeNil(), "Failed to detach vol1")

		err = controllerClient.ControllerUnpublishVolume(volID, nodeID1)
		Expect(err).To(BeNil(), "Failed to detach vol2")
	})

	It("Should create RWO 'multi-zone' PV instances from a previously created disk", func() {
		checkSkipMultiZoneTests()

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

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		_, volID0 := createAndValidateZonalDisk(controllerClient, p, zones[0], "hyperdisk-ml", volName)

		labelsMap := map[string]string{
			common.MultiZoneLabel: "true",
		}
		disk1, err := computeService.Disks.Get(p, zones[0], volName).Do()
		Expect(err).To(BeNil(), "Could not get disk")
		disk1Op, err := computeService.Disks.SetLabels(p, zones[0], volName, &compute.ZoneSetLabelsRequest{
			LabelFingerprint: disk1.LabelFingerprint,
			Labels:           labelsMap,
		}).Do()
		Expect(err).To(BeNil(), "Could not set disk labels")
		_, err = computeService.ZoneOperations.Wait(p, zones[0], disk1Op.Name).Do()
		Expect(err).To(BeNil(), "Could not set disk labels")

		defer deleteDisk(controllerClient, p, zones[0], volID0, volName)

		// Create multi-zone Disk
		resp, err := controllerClient.CreateVolumeWithCaps(volName, map[string]string{
			common.ParameterKeyEnableMultiZoneProvisioning: "true",
			common.ParameterKeyType:                        "hyperdisk-ml",
		}, defaultHdmlSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: zones[1]},
					},
				},
			},
			[]*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			nil)
		Expect(err).To(BeNil(), "Error creating multi-zone volume")
		topology := resp.GetAccessibleTopology()
		Expect(len(topology)).To(Equal(2))
		gotZones := []string{topology[0].Segments[common.TopologyKeyZone], topology[1].Segments[common.TopologyKeyZone]}
		Expect(gotZones).To(ConsistOf(zones[0], zones[1]))

		volID := fmt.Sprintf("projects/%s/zones/multi-zone/disks/%s", p, volName)
		defer func() {
			// Delete Disk
			err := controllerClient.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, zones[0], volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found. Err: %v", err)
			_, err = computeService.Disks.Get(p, zones[1], volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found. Err: %v", err)
		}()

		disk1, err = computeService.Disks.Get(p, zones[0], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[0], volName)
		disk2, err := computeService.Disks.Get(p, zones[1], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[1], volName)

		// Validate disks are RWO
		Expect(disk1.AccessMode).To(Equal("READ_WRITE_SINGLE"))
		Expect(disk2.AccessMode).To(Equal("READ_WRITE_SINGLE"))
	})

	It("Should create ROX 'multi-zone' PV from existing snapshot", func() {
		checkSkipMultiZoneTests()
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

		tc0 := zoneToContext[zones[0]]
		tc1 := zoneToContext[zones[1]]

		snapshotVolName, snapshotVolID := createAndValidateUniqueZonalDisk(controllerClient, p, zones[0], standardDiskType)

		underSpecifiedID := common.GenerateUnderspecifiedVolumeID(snapshotVolName, true /* isZonal */)

		defer func() {
			// Delete Disk
			err := controllerClient.DeleteVolume(underSpecifiedID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, zones[0], snapshotVolName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach Disk
		err := testAttachWriteReadDetach(underSpecifiedID, snapshotVolName, tc0.Instance, controllerClient, false /* readOnly */)
		Expect(err).To(BeNil(), "Failed to go through volume lifecycle")

		// Create Snapshot
		snapshotName := testNamePrefix + string(uuid.NewUUID())
		snapshotID, err := controllerClient.CreateSnapshot(snapshotName, snapshotVolID, nil)
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

		// Create multi-zone Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		_, err = controllerClient.CreateVolumeWithCaps(volName, map[string]string{
			common.ParameterKeyEnableMultiZoneProvisioning: "true",
			common.ParameterKeyType:                        "hyperdisk-ml",
		}, defaultHdmlSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: zones[0]},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: zones[1]},
					},
				},
			},
			[]*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
					},
				},
			},
			&csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{
					Snapshot: &csi.VolumeContentSource_SnapshotSource{
						SnapshotId: snapshotID,
					},
				},
			})
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		volID := fmt.Sprintf("projects/%s/zones/multi-zone/disks/%s", p, volName)
		defer func() {
			// Delete Disk
			err := controllerClient.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, zones[0], volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found. Err: %v", err)
			_, err = computeService.Disks.Get(p, zones[1], volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found. Err: %v", err)
		}()

		disk1, err := computeService.Disks.Get(p, zones[0], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[0], volName)
		disk2, err := computeService.Disks.Get(p, zones[1], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[1], volName)

		// Validate disks are ROX
		Expect(disk1.AccessMode).To(Equal("READ_ONLY_MANY"))
		Expect(disk2.AccessMode).To(Equal("READ_ONLY_MANY"))

		// Attach Disk to node1 and validate contents
		err = testAttachWriteReadDetach(volID, volName, tc0.Instance, tc0.Client, true /* readonly */)
		Expect(err).To(BeNil(), "Failed to attach/read/detach on vol1")

		// Attach Disk to node1 and validate contents
		err = testAttachWriteReadDetach(volID, volName, tc1.Instance, tc1.Client, true /* readonly */)
		Expect(err).To(BeNil(), "Failed to attach/read/detach on vol2")

		disk1, err = computeService.Disks.Get(p, zones[0], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[0], volName)
		disk2, err = computeService.Disks.Get(p, zones[1], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[1], volName)

		// Validate disks have multi-zone labels
		Expect(disk1.Labels[common.MultiZoneLabel]).To(Equal("true"))
		Expect(disk2.Labels[common.MultiZoneLabel]).To(Equal("true"))

		// Validate disks are ROX
		Expect(disk1.AccessMode).To(Equal("READ_ONLY_MANY"))
		Expect(disk2.AccessMode).To(Equal("READ_ONLY_MANY"))
	})

	It("Should create ROX 'multi-zone' PV from existing snapshot with no topology", func() {
		checkSkipMultiZoneTests()
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

		tc0 := zoneToContext[zones[0]]
		tc1 := zoneToContext[zones[1]]

		snapshotVolName, snapshotVolID := createAndValidateUniqueZonalDisk(controllerClient, p, zones[0], standardDiskType)

		underSpecifiedID := common.GenerateUnderspecifiedVolumeID(snapshotVolName, true /* isZonal */)

		defer func() {
			// Delete Disk
			err := controllerClient.DeleteVolume(underSpecifiedID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, zones[0], snapshotVolName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach Disk
		err := testAttachWriteReadDetach(underSpecifiedID, snapshotVolName, tc0.Instance, controllerClient, false /* readOnly */)
		Expect(err).To(BeNil(), "Failed to go through volume lifecycle")

		// Create Snapshot
		snapshotName := testNamePrefix + string(uuid.NewUUID())
		snapshotID, err := controllerClient.CreateSnapshot(snapshotName, snapshotVolID, nil)
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

		// Create multi-zone Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		_, err = controllerClient.CreateVolumeWithCaps(volName, map[string]string{
			common.ParameterKeyEnableMultiZoneProvisioning: "true",
			common.ParameterKeyType:                        "hyperdisk-ml",
		}, defaultHdmlSizeGb,
			&csi.TopologyRequirement{},
			[]*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
					},
				},
			},
			&csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{
					Snapshot: &csi.VolumeContentSource_SnapshotSource{
						SnapshotId: snapshotID,
					},
				},
			})
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		volID := fmt.Sprintf("projects/%s/zones/multi-zone/disks/%s", p, volName)
		defer func() {
			// Delete Disk
			err := controllerClient.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, zones[0], volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found. Err: %v", err)
			_, err = computeService.Disks.Get(p, zones[1], volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found. Err: %v", err)
		}()

		disk1, err := computeService.Disks.Get(p, zones[0], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[0], volName)
		disk2, err := computeService.Disks.Get(p, zones[1], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[1], volName)

		// Validate disks are ROX
		Expect(disk1.AccessMode).To(Equal("READ_ONLY_MANY"))
		Expect(disk2.AccessMode).To(Equal("READ_ONLY_MANY"))

		// Attach Disk to node1 and validate contents
		err = testAttachWriteReadDetach(volID, volName, tc0.Instance, tc0.Client, true /* readonly */)
		Expect(err).To(BeNil(), "Failed to attach/read/detach on vol1")

		// Attach Disk to node1 and validate contents
		err = testAttachWriteReadDetach(volID, volName, tc1.Instance, tc1.Client, true /* readonly */)
		Expect(err).To(BeNil(), "Failed to attach/read/detach on vol2")

		disk1, err = computeService.Disks.Get(p, zones[0], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[0], volName)
		disk2, err = computeService.Disks.Get(p, zones[1], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[1], volName)

		// Validate disks have multi-zone labels
		Expect(disk1.Labels[common.MultiZoneLabel]).To(Equal("true"))
		Expect(disk2.Labels[common.MultiZoneLabel]).To(Equal("true"))

		// Validate disks are ROX
		Expect(disk1.AccessMode).To(Equal("READ_ONLY_MANY"))
		Expect(disk2.AccessMode).To(Equal("READ_ONLY_MANY"))
	})

	It("Should create ROX 'multi-zone' PV from existing disk image", func() {
		checkSkipMultiZoneTests()
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

		tc0 := zoneToContext[zones[0]]
		tc1 := zoneToContext[zones[1]]

		snapshotVolName, snapshotVolID := createAndValidateUniqueZonalDisk(controllerClient, p, zones[0], standardDiskType)

		underSpecifiedID := common.GenerateUnderspecifiedVolumeID(snapshotVolName, true /* isZonal */)

		defer func() {
			// Delete Disk
			err := controllerClient.DeleteVolume(underSpecifiedID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, zones[0], snapshotVolName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach Disk
		err := testAttachWriteReadDetach(underSpecifiedID, snapshotVolName, tc0.Instance, controllerClient, false /* readOnly */)
		Expect(err).To(BeNil(), "Failed to go through volume lifecycle")

		// Create Disk Image
		imageName := testNamePrefix + string(uuid.NewUUID())
		snapshotParams := map[string]string{
			"snapshot-type": "images",
		}
		snapshotID, err := controllerClient.CreateSnapshot(imageName, snapshotVolID, snapshotParams)
		klog.Infof("Created image snapshot with snapshotID: %s", snapshotID)
		Expect(err).To(BeNil(), "CreateSnapshot failed with error: %v", err)

		// Validate Disk Image Created
		image, err := computeService.Images.Get(p, imageName).Do()
		Expect(err).To(BeNil(), "Could not get disk image from cloud directly")
		Expect(image.Name).To(Equal(imageName))

		err = wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
			image, err := computeService.Images.Get(p, imageName).Do()
			Expect(err).To(BeNil(), "Could not get disk image from cloud directly")
			if image.Status == "READY" {
				return true, nil
			}
			return false, nil
		})
		Expect(err).To(BeNil(), "Could not wait for disk image be ready")

		// Create multi-zone Disk
		volName := testNamePrefix + string(uuid.NewUUID())

		_, err = controllerClient.CreateVolumeWithCaps(volName, map[string]string{
			common.ParameterKeyEnableMultiZoneProvisioning: "true",
			common.ParameterKeyType:                        "hyperdisk-ml",
		}, defaultHdmlSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: zones[0]},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: zones[1]},
					},
				},
			},
			[]*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
					},
				},
			},
			&csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{
					Snapshot: &csi.VolumeContentSource_SnapshotSource{
						SnapshotId: snapshotID,
					},
				},
			})
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		volID := fmt.Sprintf("projects/%s/zones/multi-zone/disks/%s", p, volName)
		defer func() {
			// Delete Disk
			err := controllerClient.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, zones[0], volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found. Err: %v", err)
			_, err = computeService.Disks.Get(p, zones[1], volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found. Err: %v", err)
		}()

		disk1, err := computeService.Disks.Get(p, zones[0], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[0], volName)
		disk2, err := computeService.Disks.Get(p, zones[1], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[1], volName)

		// Validate disks have multi-zone labels
		Expect(disk1.Labels[common.MultiZoneLabel]).To(Equal("true"))
		Expect(disk2.Labels[common.MultiZoneLabel]).To(Equal("true"))

		// Validate disks are ROX
		Expect(disk1.AccessMode).To(Equal("READ_ONLY_MANY"))
		Expect(disk2.AccessMode).To(Equal("READ_ONLY_MANY"))

		// Attach Disk to node1
		err = testAttachWriteReadDetach(volID, volName, tc0.Instance, tc0.Client, true /* readonly */)
		Expect(err).To(BeNil(), "Failed to attach/read/detach on vol1")

		// Attach Disk to node1
		err = testAttachWriteReadDetach(volID, volName, tc1.Instance, tc1.Client, true /* readonly */)
		Expect(err).To(BeNil(), "Failed to attach/read/detach on vol2")
	})

	It("Should create RWO 'multi-zone' PV that has empty disks", func() {
		checkSkipMultiZoneTests()
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

		// Attach disk to instance in the first zone.
		tc0 := zoneToContext[zones[0]]
		tc1 := zoneToContext[zones[1]]

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		_, err := controllerClient.CreateVolumeWithCaps(volName, map[string]string{
			common.ParameterKeyEnableMultiZoneProvisioning: "true",
			common.ParameterKeyType:                        "hyperdisk-ml",
		}, defaultHdmlSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: zones[0]},
					},
					{
						Segments: map[string]string{common.TopologyKeyZone: zones[1]},
					},
				},
			},
			[]*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		volID := fmt.Sprintf("projects/%s/zones/multi-zone/disks/%s", p, volName)
		defer func() {
			// Delete Disk
			err := controllerClient.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, zones[0], volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found. Err: %v", err)
			_, err = computeService.Disks.Get(p, zones[1], volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found. Err: %v", err)
		}()

		disk1, err := computeService.Disks.Get(p, zones[0], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[0], volName)
		disk2, err := computeService.Disks.Get(p, zones[1], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[1], volName)

		// Validate disks have multi-zone labels
		Expect(disk1.Labels[common.MultiZoneLabel]).To(Equal("true"))
		Expect(disk2.Labels[common.MultiZoneLabel]).To(Equal("true"))

		// Validate disks are RWO
		Expect(disk1.AccessMode).To(Equal("READ_WRITE_SINGLE"))
		Expect(disk2.AccessMode).To(Equal("READ_WRITE_SINGLE"))

		// Validate underlying disks can be used
		volID0 := fmt.Sprintf("projects/%s/zones/%s/disks/%s", p, zones[0], volName)
		volID1 := fmt.Sprintf("projects/%s/zones/%s/disks/%s", p, zones[1], volName)

		err = testAttachWriteReadDetach(volID0, volName, tc0.Instance, tc0.Client, false /* readonly */)
		Expect(err).To(BeNil(), "Failed to attach/write/read/detach on vol1")

		err = testAttachWriteReadDetach(volID1, volName, tc1.Instance, tc1.Client, false /* readonly */)
		Expect(err).To(BeNil(), "Failed to attach/write/read/detach on vol2")

		// Validate disks can be used in multi-zone mode on both nodes
		volIDMultiZone := fmt.Sprintf("projects/%s/zones/multi-zone/disks/%s", p, volName)
		err = testAttachWriteReadDetach(volIDMultiZone, volName, tc0.Instance, tc0.Client, true /* readonly */)
		Expect(err).To(BeNil(), "Failed to attach/read/detach on vol1")

		err = testAttachWriteReadDetach(volIDMultiZone, volName, tc1.Instance, tc1.Client, true /* readonly */)
		Expect(err).To(BeNil(), "Failed to attach/read/detach on vol2")

		// Validate disks are ROX now
		disk1, err = computeService.Disks.Get(p, zones[0], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[0], volName)
		disk2, err = computeService.Disks.Get(p, zones[1], volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk %v/%v", zones[1], volName)

		Expect(disk1.AccessMode).To(Equal("READ_ONLY_MANY"))
		Expect(disk2.AccessMode).To(Equal("READ_ONLY_MANY"))

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
		err, detacher, args := testAttachAndMount(volume.VolumeId, volName, tc0.Instance, tc0.Client, attachAndMountArgs{
			readOnly:    false,
			useBlock:    false,
			forceAttach: false,
		})
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
		err, detacher, _ = testAttachAndMount(volume.VolumeId, volName, tc1.Instance, tc1.Client, attachAndMountArgs{
			readOnly:    false,
			useBlock:    false,
			forceAttach: true,
		})
		detachers = append(detachers, detacher)
		Expect(err).To(BeNil(), "failed force attach in zone 1")
		readContents, err = testutils.ReadFile(tc1.Instance, testFileName)
		Expect(err).To(BeNil(), "failed read in zone 1")
		Expect(strings.TrimSpace(string(readContents))).To(BeIdenticalTo(testFileContents), "content mismatch in zone 1")
	})
})

func deleteDisk(controllerClient *remote.CsiClient, p, zone, volID, volName string) {
	// Delete Disk
	err := controllerClient.DeleteVolume(volID)
	Expect(err).To(BeNil(), "DeleteVolume failed")

	// Validate Disk Deleted
	_, err = computeService.Disks.Get(p, zone, volName).Do()
	Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
}

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

type attachAndMountArgs struct {
	readOnly    bool
	useBlock    bool
	forceAttach bool
}

func testAttachAndMount(volID string, volName string, instance *remote.InstanceInfo, client *remote.CsiClient, args attachAndMountArgs) (error, func(), *verifyArgs) {
	// Attach Disk
	var err error
	if args.readOnly {
		err = client.ControllerPublishVolumeReadOnly(volID, instance.GetNodeID())
	} else {
		err = client.ControllerPublishVolumeReadWrite(volID, instance.GetNodeID(), args.forceAttach)
	}
	if err != nil {
		return fmt.Errorf("ControllerPublishVolume failed with error for disk %v on node %v: %v", volID, instance.GetNodeID(), err.Error()), nil, nil
	}

	detach := func() {
		// Detach Disk
		if err := client.ControllerUnpublishVolume(volID, instance.GetNodeID()); err != nil {
			klog.Errorf("Failed to detach disk: %v", err)
		}
	}

	// Stage Disk
	stageDir := filepath.Join("/tmp/", volName, "stage")
	if args.useBlock {
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
		if err := client.NodeUnstageVolume(volID, stageDir); err != nil {
			klog.Errorf("Failed to unstage volume: %v", err)
		}
		fp := filepath.Join("/tmp/", volName)
		if err := testutils.RmAll(instance, fp); err != nil {
			klog.Errorf("Failed to rm file path %s: %v", fp, err)
		}

		detach()
	}

	// Mount Disk
	publishDir := filepath.Join("/tmp/", volName, "mount")

	if args.useBlock {
		err = client.NodePublishBlockVolume(volID, stageDir, publishDir)
	} else {
		err = client.NodePublishVolume(volID, stageDir, publishDir)
	}

	if err != nil {
		unstageAndDetach()
		return fmt.Errorf("NodePublishVolume failed with error: %v", err.Error()), nil, nil
	}

	unpublish := func() {
		// Unpublish Disk
		if err := client.NodeUnpublishVolume(volID, publishDir); err != nil {
			klog.Errorf("Failed to unpublish volume: %v", err)
		}
	}
	unpublishUnstageAndDetach := func() {
		unpublish()
		unstageAndDetach()
	}

	err = testutils.ForceChmod(instance, filepath.Join("/tmp/", volName), "777", !args.readOnly /* recursive */)
	if err != nil {
		unpublishUnstageAndDetach()
		return fmt.Errorf("Chmod failed with error: %v", err.Error()), nil, nil
	}

	returnArgs := &verifyArgs{
		publishDir: publishDir,
		stageDir:   stageDir,
	}

	return nil, unpublishUnstageAndDetach, returnArgs
}

func testLifecycleWithVerify(volID string, volName string, instance *remote.InstanceInfo, client *remote.CsiClient, readOnly, useBlock bool, firstMountVerify, secondMountVerify verifyFunc) error {
	klog.Infof("Starting testAttachWriteReadDetach with volume %v node %v with readonly %v\n", volID, instance.GetNodeID(), readOnly)
	err, detacher, args := testAttachAndMount(volID, volName, instance, client, attachAndMountArgs{
		readOnly:    readOnly,
		useBlock:    useBlock,
		forceAttach: false,
	})
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
		err = testutils.ForceChmod(instance, filepath.Join("/tmp/", volName), "777", !readOnly /* recursive */)
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
