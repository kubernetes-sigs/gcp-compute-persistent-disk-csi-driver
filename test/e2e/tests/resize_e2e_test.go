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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"
)

var _ = Describe("GCE PD CSI Driver", func() {
	It("Should online resize controller and node for an ext4 volume", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volume, err := client.CreateVolume(volName, nil, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: z},
					},
				},
			}, nil)
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
			client.DeleteVolume(volume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach Disk
		err = client.ControllerPublishVolumeReadWrite(volume.VolumeId, instance.GetNodeID(), false /* forceAttach */)
		Expect(err).To(BeNil(), "Controller publish volume failed")

		defer func() {
			// Detach Disk
			err = client.ControllerUnpublishVolume(volume.VolumeId, instance.GetNodeID())
			if err != nil {
				klog.Errorf("Failed to detach disk: %v", err)
			}
		}()

		// Stage Disk
		stageDir := filepath.Join("/tmp/", volName, "stage")
		err = client.NodeStageExt4Volume(volume.VolumeId, stageDir)
		Expect(err).To(BeNil(), "Node Stage volume failed")

		defer func() {
			// Unstage Disk
			err = client.NodeUnstageVolume(volume.VolumeId, stageDir)
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
		err = client.NodePublishVolume(volume.VolumeId, stageDir, publishDir)
		Expect(err).To(BeNil(), "Node publish volume failed")

		defer func() {
			// Unmount Disk
			err = client.NodeUnpublishVolume(volume.VolumeId, publishDir)
			if err != nil {
				klog.Errorf("NodeUnpublishVolume failed with error: %v", err)
			}
		}()

		// Verify pre-resize fs size
		sizeGb, err := testutils.GetFSSizeInGb(instance, publishDir)
		Expect(err).To(BeNil(), "Failed to get FSSize in GB")
		Expect(sizeGb).To(Equal(defaultSizeGb))

		// Resize controller
		var newSizeGb int64 = 10
		err = client.ControllerExpandVolume(volume.VolumeId, newSizeGb)

		Expect(err).To(BeNil(), "Controller expand volume failed")

		// Verify cloud size
		cloudDisk, err = computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Get cloud disk failed")
		Expect(cloudDisk.SizeGb).To(Equal(newSizeGb))

		// Resize node
		_, err = client.NodeExpandVolume(volume.VolumeId, publishDir, newSizeGb)
		Expect(err).To(BeNil(), "Node expand volume failed")

		// Verify disk size
		sizeGb, err = testutils.GetFSSizeInGb(instance, publishDir)
		Expect(err).To(BeNil(), "Failed to get FSSize in GB")
		Expect(sizeGb).To(Equal(newSizeGb))

	})

	It("Should offline resize controller and node for an ext4 volume", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volume, err := client.CreateVolume(volName, nil, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: z},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created & size
		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))

		defer func() {
			// Delete Disk
			client.DeleteVolume(volume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Volume should be attached/formatted/mounted/unmounted/detached
		err = testAttachWriteReadDetach(volume.VolumeId, volName, instance, client, false /* readOnly */)
		Expect(err).To(BeNil(), "Failed to go through volume lifecycle")

		// Resize controller
		var newSizeGb int64 = 10
		err = client.ControllerExpandVolume(volume.VolumeId, newSizeGb)

		Expect(err).To(BeNil(), "Controller expand volume failed")

		// Verify cloud size
		cloudDisk, err = computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Get cloud disk failed")
		Expect(cloudDisk.SizeGb).To(Equal(newSizeGb))

		// Attach and mount again
		err = client.ControllerPublishVolumeReadWrite(volume.VolumeId, instance.GetNodeID(), false /* forceAttach */)
		Expect(err).To(BeNil(), "Controller publish volume failed")

		defer func() {
			// Detach Disk
			err = client.ControllerUnpublishVolume(volume.VolumeId, instance.GetNodeID())
			if err != nil {
				klog.Errorf("Failed to detach disk: %v", err)
			}

		}()

		// Stage Disk
		stageDir := filepath.Join("/tmp/", volName, "stage")
		err = client.NodeStageExt4Volume(volume.VolumeId, stageDir)
		Expect(err).To(BeNil(), "Node Stage volume failed")

		defer func() {
			// Unstage Disk
			err = client.NodeUnstageVolume(volume.VolumeId, stageDir)
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
		err = client.NodePublishVolume(volume.VolumeId, stageDir, publishDir)
		Expect(err).To(BeNil(), "Node publish volume failed")

		defer func() {
			// Unmount Disk
			err = client.NodeUnpublishVolume(volume.VolumeId, publishDir)
			if err != nil {
				klog.Errorf("NodeUnpublishVolume failed with error: %v", err)
			}
		}()

		// Resize node
		_, err = client.NodeExpandVolume(volume.VolumeId, publishDir, newSizeGb)
		Expect(err).To(BeNil(), "Node expand volume failed")

		// Verify disk size
		sizeGb, err := testutils.GetFSSizeInGb(instance, publishDir)
		Expect(err).To(BeNil(), "Failed to get FSSize in GB")
		Expect(sizeGb).To(Equal(newSizeGb))

	})

	It("Should resize controller and node for an block volume", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volume, err := client.CreateVolume(volName, nil, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: z},
					},
				},
			}, nil)
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
			client.DeleteVolume(volume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach Disk
		err = client.ControllerPublishVolumeReadWrite(volume.VolumeId, instance.GetNodeID(), false /* forceAttach */)
		Expect(err).To(BeNil(), "Controller publish volume failed")

		defer func() {
			// Detach Disk
			err = client.ControllerUnpublishVolume(volume.VolumeId, instance.GetNodeID())
			if err != nil {
				klog.Errorf("Failed to detach disk: %v", err)
			}

		}()

		// Stage Disk
		stageDir := filepath.Join("/tmp/", volName, "stage")
		err = client.NodeStageBlockVolume(volume.VolumeId, stageDir)
		Expect(err).To(BeNil(), "Node Stage volume failed")

		defer func() {
			// Unstage Disk
			err = client.NodeUnstageVolume(volume.VolumeId, stageDir)
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
		err = client.NodePublishBlockVolume(volume.VolumeId, stageDir, publishDir)
		Expect(err).To(BeNil(), "Node publish volume failed")

		defer func() {
			// Unmount Disk
			err = client.NodeUnpublishVolume(volume.VolumeId, publishDir)
			if err != nil {
				klog.Errorf("NodeUnpublishVolume failed with error: %v", err)
			}
		}()

		// Verify pre-resize fs size
		sizeGb, err := testutils.GetBlockSizeInGb(instance, publishDir)
		Expect(err).To(BeNil(), "Failed to get block device size in GB")
		Expect(sizeGb).To(Equal(defaultSizeGb), "Old size should be equal")

		// Resize controller
		var newSizeGb int64 = 10
		err = client.ControllerExpandVolume(volume.VolumeId, newSizeGb)

		Expect(err).To(BeNil(), "Controller expand volume failed")

		// Verify cloud size
		cloudDisk, err = computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Get cloud disk failed")
		Expect(cloudDisk.SizeGb).To(Equal(newSizeGb))

		// Resize node
		resp, err := client.NodeExpandVolume(volume.VolumeId, publishDir, newSizeGb)
		Expect(err).To(BeNil(), "Node expand volume failed")
		Expect(resp.CapacityBytes).To(Equal(common.GbToBytes(newSizeGb)), "Node expand should not do anything")

		// Verify disk size
		sizeGb, err = testutils.GetBlockSizeInGb(instance, publishDir)
		Expect(err).To(BeNil(), "Failed to get block device size in GB")
		Expect(sizeGb).To(Equal(newSizeGb), "New size should be equal")

	})
})
