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
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/deviceutils"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/iterator"
	kmspb "google.golang.org/genproto/googleapis/cloud/kms/v1"
	fieldmask "google.golang.org/genproto/protobuf/field_mask"
)

const (
	testNamePrefix = "gcepd-csi-e2e-"

	defaultSizeGb                     int64 = 5
	defaultExtremeSizeGb              int64 = 500
	defaultHdTSizeGb                  int64 = 2048
	defaultRepdSizeGb                 int64 = 200
	defaultMwSizeGb                   int64 = 200
	defaultVolumeLimit                int64 = 127
	readyState                              = "READY"
	standardDiskType                        = "pd-standard"
	extremeDiskType                         = "pd-extreme"
	hdtDiskType                             = "hyperdisk-throughput"
	provisionedIOPSOnCreate                 = "12345"
	provisionedIOPSOnCreateInt              = int64(12345)
	provisionedIOPSOnCreateDefaultInt       = int64(100000)
	provisionedThroughputOnCreate           = "66Mi"
	provisionedThroughputOnCreateInt        = int64(66)
	defaultEpsilon                          = 500000000 // 500M
)

var _ = Describe("GCE PD CSI Driver", func() {

	It("Should get reasonable volume limits from nodes with NodeGetInfo", func() {
		testContext := getRandomTestContext()
		resp, err := testContext.Client.NodeGetInfo()
		Expect(err).To(BeNil())
		volumeLimit := resp.GetMaxVolumesPerNode()
		Expect(volumeLimit).To(Equal(defaultVolumeLimit))
	})

	It("Should create->attach->stage->mount volume and check if it is writable, then unmount->unstage->detach->delete and check disk is deleted", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		// Create Disk
		volName, volID := createAndValidateUniqueZonalDisk(client, p, z, standardDiskType)

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach Disk
		err := testAttachWriteReadDetach(volID, volName, instance, client, false /* readOnly */)
		Expect(err).To(BeNil(), "Failed to go through volume lifecycle")
	})

	It("Should automatically fix the symlink between /dev/* and /dev/by-id if the disk does not match", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		// Create Disk
		volName, volID := createAndValidateUniqueZonalDisk(client, p, z, standardDiskType)

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach Disk
		err := client.ControllerPublishVolumeReadWrite(volID, instance.GetNodeID(), false /* forceAttach */)
		Expect(err).To(BeNil(), "ControllerPublishVolume failed with error for disk %v on node %v: %v", volID, instance.GetNodeID())

		defer func() {
			// Detach Disk
			err = client.ControllerUnpublishVolume(volID, instance.GetNodeID())
			if err != nil {
				klog.Errorf("Failed to detach disk: %w", err)
			}

		}()

		// MESS UP THE symlink
		devicePaths := deviceutils.NewDeviceUtils().GetDiskByIdPaths(volName, "")
		for _, devicePath := range devicePaths {
			err = testutils.RmAll(instance, devicePath)
			Expect(err).To(BeNil(), "failed to remove /dev/by-id folder")
			err = testutils.Symlink(instance, "/dev/null", devicePath)
			Expect(err).To(BeNil(), "failed to add invalid symlink /dev/by-id folder")
		}

		// Stage Disk
		stageDir := filepath.Join("/tmp/", volName, "stage")
		err = client.NodeStageExt4Volume(volID, stageDir)
		Expect(err).To(BeNil(), "failed to repair /dev/by-id symlink and stage volume")

		// Validate that the link is correct
		var validated bool
		for _, devicePath := range devicePaths {
			validated, err = testutils.ValidateLogicalLinkIsDisk(instance, devicePath, volName)
			Expect(err).To(BeNil(), "failed to validate link %s is disk %s: %v", stageDir, volName, err)
			if validated {
				break
			}
		}
		Expect(validated).To(BeTrue(), "could not find device in %v that links to volume %s", devicePaths, volName)

		defer func() {
			// Unstage Disk
			err = client.NodeUnstageVolume(volID, stageDir)
			if err != nil {
				klog.Errorf("Failed to unstage volume: %w", err)
			}
			fp := filepath.Join("/tmp/", volName)
			err = testutils.RmAll(instance, fp)
			if err != nil {
				klog.Errorf("Failed to rm file path %s: %w", fp, err)
			}
		}()
	})

	It("Should automatically add a symlink between /dev/* and /dev/by-id if disk is not found", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		// Create Disk
		volName, volID := createAndValidateUniqueZonalDisk(client, p, z, standardDiskType)

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach Disk
		err := client.ControllerPublishVolumeReadWrite(volID, instance.GetNodeID(), false /* forceAttach */)
		Expect(err).To(BeNil(), "ControllerPublishVolume failed with error for disk %v on node %v: %v", volID, instance.GetNodeID())

		defer func() {
			// Detach Disk
			err = client.ControllerUnpublishVolume(volID, instance.GetNodeID())
			if err != nil {
				klog.Errorf("Failed to detach disk: %w", err)
			}

		}()

		// DELETE THE symlink
		devicePaths := deviceutils.NewDeviceUtils().GetDiskByIdPaths(volName, "")
		for _, devicePath := range devicePaths {
			err = testutils.RmAll(instance, devicePath)
			Expect(err).To(BeNil(), "failed to remove /dev/by-id folder")
		}

		// Stage Disk
		stageDir := filepath.Join("/tmp/", volName, "stage")
		err = client.NodeStageExt4Volume(volID, stageDir)
		Expect(err).To(BeNil(), "failed to repair /dev/by-id symlink and stage volume")

		// Validate that the link is correct
		var validated bool
		for _, devicePath := range devicePaths {
			validated, err = testutils.ValidateLogicalLinkIsDisk(instance, devicePath, volName)
			Expect(err).To(BeNil(), "failed to validate link %s is disk %s: %v", stageDir, volName, err)
			if validated {
				break
			}
		}
		Expect(validated).To(BeTrue(), "could not find device in %v that links to volume %s", devicePaths, volName)

		defer func() {
			// Unstage Disk
			err = client.NodeUnstageVolume(volID, stageDir)
			if err != nil {
				klog.Errorf("Failed to unstage volume: %w", err)
			}
			fp := filepath.Join("/tmp/", volName)
			err = testutils.RmAll(instance, fp)
			if err != nil {
				klog.Errorf("Failed to rm file path %s: %w", fp, err)
			}
		}()
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
			volume, err := testContext.Client.CreateVolume(volName, nil, defaultSizeGb, topReq, nil)
			Expect(err).To(BeNil(), "Failed to create volume")
			defer func() {
				err = testContext.Client.DeleteVolume(volume.VolumeId)
				Expect(err).To(BeNil(), "Failed to delete volume")
			}()

			_, err = computeService.Disks.Get(p, zone, volName).Do()
			Expect(err).To(BeNil(), "Could not find disk in correct zone")
		}
	})

	DescribeTable("Should complete entire disk lifecycle with underspecified volume ID",
		func(diskType string) {
			testContext := getRandomTestContext()

			p, z, _ := testContext.Instance.GetIdentity()
			client := testContext.Client
			instance := testContext.Instance

			volName, _ := createAndValidateUniqueZonalDisk(client, p, z, diskType)

			underSpecifiedID := common.GenerateUnderspecifiedVolumeID(volName, true /* isZonal */)

			defer func() {
				// Delete Disk
				err := client.DeleteVolume(underSpecifiedID)
				Expect(err).To(BeNil(), "DeleteVolume failed")

				// Validate Disk Deleted
				_, err = computeService.Disks.Get(p, z, volName).Do()
				Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
			}()

			// Attach Disk
			err := testAttachWriteReadDetach(underSpecifiedID, volName, instance, client, false /* readOnly */)
			Expect(err).To(BeNil(), "Failed to go through volume lifecycle")
		},
		Entry("on pd-standard", standardDiskType),
		Entry("on pd-extreme", extremeDiskType),
		Entry("on hyperdisk-throughput", hdtDiskType),
	)

	DescribeTable("Should complete publish/unpublish lifecycle with underspecified volume ID and missing volume",
		func(diskType string) {
			testContext := getRandomTestContext()

			p, z, _ := testContext.Instance.GetIdentity()
			client := testContext.Client
			instance := testContext.Instance

			// Create Disk
			volName, _ := createAndValidateUniqueZonalDisk(client, p, z, diskType)
			underSpecifiedID := common.GenerateUnderspecifiedVolumeID(volName, true /* isZonal */)

			defer func() {
				// Detach Disk
				err := instance.DetachDisk(volName)
				Expect(err).To(BeNil(), "DetachDisk failed")

				// Delete Disk
				err = client.DeleteVolume(underSpecifiedID)
				Expect(err).To(BeNil(), "DeleteVolume failed")

				// Validate Disk Deleted
				_, err = computeService.Disks.Get(p, z, volName).Do()
				Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")

				// Unpublish Disk
				err = client.ControllerUnpublishVolume(underSpecifiedID, instance.GetNodeID())
				Expect(err).To(BeNil(), "ControllerUnpublishVolume failed")
			}()

			// Attach Disk
			err := client.ControllerPublishVolumeReadWrite(underSpecifiedID, instance.GetNodeID(), false /* forceAttach */)
			Expect(err).To(BeNil(), "ControllerPublishVolume failed")
		},
		Entry("on pd-standard", standardDiskType),
		Entry("on pd-extreme", extremeDiskType),
	)

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
		volume, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyReplicationType: "regional-pd",
		}, defaultRepdSizeGb, nil, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := computeService.RegionDisks.Get(p, region, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultRepdSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))
		Expect(len(cloudDisk.ReplicaZones)).To(Equal(2))
		for _, replicaZone := range cloudDisk.ReplicaZones {
			actualZone := zoneFromURL(replicaZone)
			gotRegion, err := common.GetRegionFromZones([]string{actualZone})
			Expect(err).To(BeNil(), "failed to get region from actual zone %v", actualZone)
			Expect(gotRegion).To(Equal(region), "Got region from replica zone that did not match supplied region")
		}
		defer func() {
			// Delete Disk
			controllerClient.DeleteVolume(volume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.RegionDisks.Get(p, region, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()
	})

	DescribeTable("Should create and delete disk with default zone",
		func(diskType string) {
			Expect(testContexts).ToNot(BeEmpty())
			testContext := getRandomTestContext()

			p, z, _ := testContext.Instance.GetIdentity()
			client := testContext.Client

			// Create Disk
			disk := typeToDisk[diskType]
			volName := testNamePrefix + string(uuid.NewUUID())

			diskSize := defaultSizeGb
			if diskType == extremeDiskType {
				diskSize = defaultExtremeSizeGb
			}

			volume, err := client.CreateVolume(volName, disk.params, diskSize, nil, nil)

			Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

			// Validate Disk Created
			cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
			Expect(err).To(BeNil(), "Could not get disk from cloud directly")
			Expect(cloudDisk.Status).To(Equal(readyState))
			Expect(cloudDisk.SizeGb).To(Equal(diskSize))
			Expect(cloudDisk.Name).To(Equal(volName))
			disk.validate(cloudDisk)

			defer func() {
				// Delete Disk
				client.DeleteVolume(volume.VolumeId)
				Expect(err).To(BeNil(), "DeleteVolume failed")

				// Validate Disk Deleted
				_, err = computeService.Disks.Get(p, z, volName).Do()
				Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
			}()
		},
		Entry("on pd-standard", standardDiskType),
		Entry("on pd-extreme", extremeDiskType),
	)

	DescribeTable("Should create and delete pd-extreme disk with default iops",
		func(diskType string) {
			Expect(testContexts).ToNot(BeEmpty())
			testContext := getRandomTestContext()

			p, z, _ := testContext.Instance.GetIdentity()
			client := testContext.Client

			// Create Disk
			diskParams := map[string]string{
				common.ParameterKeyType: diskType,
			}
			volName := testNamePrefix + string(uuid.NewUUID())

			diskSize := defaultExtremeSizeGb

			volume, err := client.CreateVolume(volName, diskParams, diskSize, nil, nil)

			Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

			// Validate Disk Created
			cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
			Expect(err).To(BeNil(), "Could not get disk from cloud directly")
			Expect(cloudDisk.Status).To(Equal(readyState))
			Expect(cloudDisk.SizeGb).To(Equal(defaultExtremeSizeGb))
			Expect(cloudDisk.Type).To(ContainSubstring(extremeDiskType))
			Expect(cloudDisk.ProvisionedIops).To(Equal(provisionedIOPSOnCreateDefaultInt))
			Expect(cloudDisk.Name).To(Equal(volName))

			defer func() {
				// Delete Disk
				client.DeleteVolume(volume.VolumeId)
				Expect(err).To(BeNil(), "DeleteVolume failed")

				// Validate Disk Deleted
				_, err = computeService.Disks.Get(p, z, volName).Do()
				Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
			}()
		},
		Entry("on pd-extreme", extremeDiskType),
	)

	DescribeTable("Should create and delete disk with labels",
		func(diskType string) {
			Expect(testContexts).ToNot(BeEmpty())
			testContext := getRandomTestContext()

			p, z, _ := testContext.Instance.GetIdentity()
			client := testContext.Client

			// Create Disk
			disk := typeToDisk[diskType]
			volName := testNamePrefix + string(uuid.NewUUID())
			params := merge(disk.params, map[string]string{
				common.ParameterKeyLabels: "key1=value1,key2=value2",
			})

			diskSize := defaultSizeGb
			if diskType == extremeDiskType {
				diskSize = defaultExtremeSizeGb
			}
			volume, err := client.CreateVolume(volName, params, diskSize, nil, nil)
			Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

			// Validate Disk Created
			cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
			Expect(err).To(BeNil(), "Could not get disk from cloud directly")
			Expect(cloudDisk.Status).To(Equal(readyState))
			Expect(cloudDisk.SizeGb).To(Equal(diskSize))
			Expect(cloudDisk.Labels).To(Equal(map[string]string{
				"key1": "value1",
				"key2": "value2",
				// The label below is added as an --extra-label driver command line argument.
				testutils.DiskLabelKey: testutils.DiskLabelValue,
			}))
			Expect(cloudDisk.Name).To(Equal(volName))
			disk.validate(cloudDisk)

			defer func() {
				// Delete Disk
				err := client.DeleteVolume(volume.VolumeId)
				Expect(err).To(BeNil(), "DeleteVolume failed")

				// Validate Disk Deleted
				_, err = computeService.Disks.Get(p, z, volName).Do()
				Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
			}()
		},
		Entry("on pd-standard", standardDiskType),
		Entry("on pd-extreme", extremeDiskType),
	)

	It("Should create and delete snapshot for the volume with default zone", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		volName, volID := createAndValidateUniqueZonalDisk(client, p, z, standardDiskType)

		// Create Snapshot
		snapshotName := testNamePrefix + string(uuid.NewUUID())
		snapshotID, err := client.CreateSnapshot(snapshotName, volID, nil)
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
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")

			// Delete Snapshot
			err = client.DeleteSnapshot(snapshotID)
			Expect(err).To(BeNil(), "DeleteSnapshot failed")

			// Validate Snapshot Deleted
			_, err = computeService.Snapshots.Get(p, snapshotName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected snapshot to not be found")
		}()
	})

	DescribeTable("Should create CMEK key, go through volume lifecycle, validate behavior on key revoke and restore",
		func(diskType string) {
			ctx := context.Background()
			Expect(testContexts).ToNot(BeEmpty())
			testContext := getRandomTestContext()

			controllerInstance := testContext.Instance
			controllerClient := testContext.Client

			p, z, _ := controllerInstance.GetIdentity()
			locationID := "global"

			// The resource name of the key rings.
			parentName := fmt.Sprintf("projects/%s/locations/%s", p, locationID)
			keyRingId := "gce-pd-csi-test-ring"

			key, keyVersions := setupKeyRing(ctx, parentName, keyRingId)

			// Defer deletion of all key versions
			// https://cloud.google.com/kms/docs/destroy-restore
			defer func() {
				for _, keyVersion := range keyVersions {
					destroyKeyReq := &kmspb.DestroyCryptoKeyVersionRequest{
						Name: keyVersion,
					}
					_, err := kmsClient.DestroyCryptoKeyVersion(ctx, destroyKeyReq)
					Expect(err).To(BeNil(), "Failed to destroy crypto key version: %v", keyVersion)
				}
			}()

			// Go through volume lifecycle using CMEK-ed PD Create Disk
			disk := typeToDisk[diskType]
			volName := testNamePrefix + string(uuid.NewUUID())
			params := merge(disk.params, map[string]string{
				common.ParameterKeyDiskEncryptionKmsKey: key.Name,
			})
			topology := &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: z},
					},
				},
			}

			diskSize := defaultSizeGb
			if diskType == extremeDiskType {
				diskSize = defaultExtremeSizeGb
			}
			volume, err := controllerClient.CreateVolume(volName, params, diskSize, topology, nil)
			Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

			// Validate Disk Created
			cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
			Expect(err).To(BeNil(), "Could not get disk from cloud directly")
			Expect(cloudDisk.Status).To(Equal(readyState))
			Expect(cloudDisk.SizeGb).To(Equal(diskSize))
			Expect(cloudDisk.Name).To(Equal(volName))
			disk.validate(cloudDisk)

			defer func() {
				// Delete Disk
				err = controllerClient.DeleteVolume(volume.VolumeId)
				Expect(err).To(BeNil(), "DeleteVolume failed")

				// Validate Disk Deleted
				_, err = computeService.Disks.Get(p, z, volName).Do()
				Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
			}()

			// Test disk works
			err = testAttachWriteReadDetach(volume.VolumeId, volName, controllerInstance, controllerClient, false /* readOnly */)
			Expect(err).To(BeNil(), "Failed to go through volume lifecycle before revoking CMEK key")

			// Revoke CMEK key
			// https://cloud.google.com/kms/docs/enable-disable

			for _, keyVersion := range keyVersions {
				disableReq := &kmspb.UpdateCryptoKeyVersionRequest{
					CryptoKeyVersion: &kmspb.CryptoKeyVersion{
						Name:  keyVersion,
						State: kmspb.CryptoKeyVersion_DISABLED,
					},
					UpdateMask: &fieldmask.FieldMask{
						Paths: []string{"state"},
					},
				}
				_, err = kmsClient.UpdateCryptoKeyVersion(ctx, disableReq)
				Expect(err).To(BeNil(), "Failed to disable crypto key")
			}

			// Make sure attach of PD fails
			err = testAttachWriteReadDetach(volume.VolumeId, volName, controllerInstance, controllerClient, false /* readOnly */)
			Expect(err).ToNot(BeNil(), "Volume lifecycle should have failed, but succeeded")

			// Restore CMEK key
			for _, keyVersion := range keyVersions {
				enableReq := &kmspb.UpdateCryptoKeyVersionRequest{
					CryptoKeyVersion: &kmspb.CryptoKeyVersion{
						Name:  keyVersion,
						State: kmspb.CryptoKeyVersion_ENABLED,
					},
					UpdateMask: &fieldmask.FieldMask{
						Paths: []string{"state"},
					},
				}
				_, err = kmsClient.UpdateCryptoKeyVersion(ctx, enableReq)
				Expect(err).To(BeNil(), "Failed to enable crypto key")
			}

			// The controller publish failure in above step would set a backoff condition on the node. Wait suffcient amount of time for the driver to accept new controller publish requests.
			time.Sleep(time.Second)
			// Make sure attach of PD succeeds
			err = testAttachWriteReadDetach(volume.VolumeId, volName, controllerInstance, controllerClient, false /* readOnly */)
			Expect(err).To(BeNil(), "Failed to go through volume lifecycle after restoring CMEK key")
		},
		Entry("on pd-standard", standardDiskType),
		Entry("on pd-extreme", extremeDiskType),
	)

	It("Should create disks, attach them places, and verify List returns correct results", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		nodeID := testContext.Instance.GetNodeID()

		_, volID := createAndValidateUniqueZonalDisk(client, p, z, standardDiskType)
		defer deleteVolumeOrError(client, volID)

		_, secondVolID := createAndValidateUniqueZonalDisk(client, p, z, standardDiskType)
		defer deleteVolumeOrError(client, secondVolID)

		// Attach volID to current instance
		err := client.ControllerPublishVolumeReadWrite(volID, nodeID, false /* forceAttach */)
		Expect(err).To(BeNil(), "Failed ControllerPublishVolume")
		defer client.ControllerUnpublishVolume(volID, nodeID)

		// List Volumes
		volsToNodes, err := client.ListVolumes()
		Expect(err).To(BeNil(), "Failed ListVolumes")

		// Verify
		Expect(volsToNodes[volID]).ToNot(BeNil(), "Couldn't find attached nodes for vol")
		Expect(volsToNodes[volID]).To(ContainElement(nodeID), "Couldn't find node in attached nodes for vol")
		Expect(volsToNodes[secondVolID]).To(BeNil(), "Second vol ID attached nodes not nil")
	})

	It("Should create and delete snapshot for RePD in two zones ", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		controllerInstance := testContext.Instance
		controllerClient := testContext.Client

		p, z, _ := controllerInstance.GetIdentity()

		region, err := common.GetRegionFromZones([]string{z})
		Expect(err).To(BeNil(), "Failed to get region from zones")

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volume, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyReplicationType: "regional-pd",
		}, defaultRepdSizeGb, nil, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := computeService.RegionDisks.Get(p, region, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultRepdSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))
		Expect(len(cloudDisk.ReplicaZones)).To(Equal(2))
		for _, replicaZone := range cloudDisk.ReplicaZones {
			actualZone := zoneFromURL(replicaZone)
			gotRegion, err := common.GetRegionFromZones([]string{actualZone})
			Expect(err).To(BeNil(), "failed to get region from actual zone %v", actualZone)
			Expect(gotRegion).To(Equal(region), "Got region from replica zone that did not match supplied region")
		}

		// Create Snapshot
		snapshotName := testNamePrefix + string(uuid.NewUUID())
		snapshotID, err := controllerClient.CreateSnapshot(snapshotName, volume.VolumeId, nil)
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
			err := controllerClient.DeleteVolume(volume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.RegionDisks.Get(p, region, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")

			// Delete Snapshot
			err = controllerClient.DeleteSnapshot(snapshotID)
			Expect(err).To(BeNil(), "DeleteSnapshot failed")

			// Validate Snapshot Deleted
			_, err = computeService.Snapshots.Get(p, snapshotName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected snapshot to not be found")
		}()
	})

	It("Should get correct VolumeStats for Block", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		volName, volID := createAndValidateUniqueZonalDisk(client, p, z, standardDiskType)

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		verifyVolumeStats := func(a *verifyArgs) error {
			available, capacity, used, inodesFree, inodes, inodesUsed, err := client.NodeGetVolumeStats(volID, a.publishDir)
			if err != nil {
				return fmt.Errorf("failed to get node volume stats: %v", err.Error())
			}
			if available != 0 || capacity != common.GbToBytes(defaultSizeGb) || used != 0 ||
				inodesFree != 0 || inodes != 0 || inodesUsed != 0 {
				return fmt.Errorf("got: available %v, capacity %v, used %v, inodesFree %v, inodes %v, inodesUsed %v -- expected: capacity = %v, available = 0, used = 0, inodesFree = 0, inodes = 0 , inodesUsed = 0",
					available, capacity, used, inodesFree, inodes, inodesUsed, common.GbToBytes(defaultSizeGb))
			}
			return nil
		}

		// Attach Disk
		err := testLifecycleWithVerify(volID, volName, instance, client, false /* readOnly */, true /* block */, verifyVolumeStats, nil)
		Expect(err).To(BeNil(), "Failed to go through volume lifecycle")
	})

	It("Should get correct VolumeStats", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		volName, volID := createAndValidateUniqueZonalDisk(client, p, z, standardDiskType)

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		verifyVolumeStats := func(a *verifyArgs) error {
			available, capacity, used, inodesFree, inodes, inodesUsed, err := client.NodeGetVolumeStats(volID, a.publishDir)
			if err != nil {
				return fmt.Errorf("failed to get node volume stats: %v", err.Error())
			}
			if !equalWithinEpsilon(available, common.GbToBytes(defaultSizeGb), defaultEpsilon) || !equalWithinEpsilon(capacity, common.GbToBytes(defaultSizeGb), defaultEpsilon) || !equalWithinEpsilon(used, 0, defaultEpsilon) ||
				inodesFree == 0 || inodes == 0 || inodesUsed == 0 {
				return fmt.Errorf("got: available %v, capacity %v, used %v, inodesFree %v, inodes %v, inodesUsed %v -- expected: available ~= %v, capacity ~= %v, used = 0, inodesFree != 0, inodes != 0 , inodesUsed != 0",
					available, capacity, used, inodesFree, inodes, inodesUsed, common.GbToBytes(defaultSizeGb), common.GbToBytes(defaultSizeGb))
			}
			return nil
		}

		// Attach Disk
		err := testLifecycleWithVerify(volID, volName, instance, client, false /* readOnly */, false /* fs */, verifyVolumeStats, nil)
		Expect(err).To(BeNil(), "Failed to go through volume lifecycle")
	})

	// Pending while multi-writer feature is in Alpha
	PIt("Should create and delete multi-writer disk", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		p, _, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		// Hardcode to us-east1-a while feature is in alpha
		zone := "us-east1-a"

		// Create and Validate Disk
		volName, volID := createAndValidateUniqueZonalMultiWriterDisk(client, p, zone, standardDiskType)

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeAlphaService.Disks.Get(p, zone, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()
	})

	// Pending while multi-writer feature is in Alpha
	PIt("Should complete entire disk lifecycle with multi-writer disk", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		// Create and Validate Disk
		volName, volID := createAndValidateUniqueZonalMultiWriterDisk(client, p, z, standardDiskType)

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach Disk
		testFileContents := "test"
		writeFunc := func(a *verifyArgs) error {
			err := testutils.WriteBlock(instance, a.publishDir, testFileContents)
			if err != nil {
				return fmt.Errorf("Failed to write file: %v", err.Error())
			}
			return nil
		}
		verifyReadFunc := func(a *verifyArgs) error {
			readContents, err := testutils.ReadBlock(instance, a.publishDir, len(testFileContents))
			if err != nil {
				return fmt.Errorf("ReadFile failed with error: %v", err.Error())
			}
			if strings.TrimSpace(string(readContents)) != testFileContents {
				return fmt.Errorf("wanted test file content: %s, got content: %s", testFileContents, readContents)
			}
			return nil
		}
		err := testLifecycleWithVerify(volID, volName, instance, client, false /* readOnly */, true /* block */, writeFunc, verifyReadFunc)
		Expect(err).To(BeNil(), "Failed to go through volume lifecycle")
	})

	DescribeTable("Should successfully create disk with PVC/PV tags",
		func(diskType string) {
			Expect(testContexts).ToNot(BeEmpty())
			testContext := getRandomTestContext()

			controllerInstance := testContext.Instance
			controllerClient := testContext.Client

			diskSize := defaultSizeGb
			if diskType == extremeDiskType {
				diskSize = defaultExtremeSizeGb
			}

			p, z, _ := controllerInstance.GetIdentity()

			// Create Disk
			disk := typeToDisk[diskType]
			volName := testNamePrefix + string(uuid.NewUUID())
			params := merge(disk.params, map[string]string{
				common.ParameterKeyPVCName:      "test-pvc",
				common.ParameterKeyPVCNamespace: "test-pvc-namespace",
				common.ParameterKeyPVName:       "test-pv-name",
			})
			volume, err := controllerClient.CreateVolume(volName, params, diskSize, nil /* topReq */, nil)
			Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

			// Validate Disk Created
			cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
			Expect(err).To(BeNil(), "Could not get disk from cloud directly")
			Expect(cloudDisk.Status).To(Equal(readyState))
			Expect(cloudDisk.SizeGb).To(Equal(diskSize))
			Expect(cloudDisk.Name).To(Equal(volName))
			Expect(cloudDisk.Description).To(Equal("{\"kubernetes.io/created-for/pv/name\":\"test-pv-name\",\"kubernetes.io/created-for/pvc/name\":\"test-pvc\",\"kubernetes.io/created-for/pvc/namespace\":\"test-pvc-namespace\",\"storage.gke.io/created-by\":\"pd.csi.storage.gke.io\"}"))
			disk.validate(cloudDisk)

			defer func() {
				// Delete Disk
				controllerClient.DeleteVolume(volume.VolumeId)
				Expect(err).To(BeNil(), "DeleteVolume failed")

				// Validate Disk Deleted
				_, err = computeService.Disks.Get(p, z, volName).Do()
				Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
			}()
		},
		Entry("on pd-standard", standardDiskType),
		Entry("on pd-extreme", extremeDiskType),
	)

	// Use the region of the test location.
	It("Should successfully create snapshot with storage locations", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		// Create Disk
		volName, volID := createAndValidateUniqueZonalDisk(client, p, z, standardDiskType)

		// Create Snapshot
		snapshotName := testNamePrefix + string(uuid.NewUUID())

		// Convert GCP zone to region, e.g. us-central1-a => us-central1
		// This is safe because we hardcode the zones.
		snapshotLocation := z[:len(z)-2]

		snapshotParams := map[string]string{
			common.ParameterKeyStorageLocations:          snapshotLocation,
			common.ParameterKeyVolumeSnapshotName:        "test-volumesnapshot-name",
			common.ParameterKeyVolumeSnapshotNamespace:   "test-volumesnapshot-namespace",
			common.ParameterKeyVolumeSnapshotContentName: "test-volumesnapshotcontent-name",
		}
		snapshotID, err := client.CreateSnapshot(snapshotName, volID, snapshotParams)
		Expect(err).To(BeNil(), "CreateSnapshot failed with error: %v", err)

		// Validate Snapshot Created
		snapshot, err := computeService.Snapshots.Get(p, snapshotName).Do()
		Expect(err).To(BeNil(), "Could not get snapshot from cloud directly")
		Expect(snapshot.Name).To(Equal(snapshotName))
		Expect(snapshot.Description).To(Equal("{\"kubernetes.io/created-for/volumesnapshot/name\":\"test-volumesnapshot-name\",\"kubernetes.io/created-for/volumesnapshot/namespace\":\"test-volumesnapshot-namespace\",\"kubernetes.io/created-for/volumesnapshotcontent/name\":\"test-volumesnapshotcontent-name\",\"storage.gke.io/created-by\":\"pd.csi.storage.gke.io\"}"))

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
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")

			// Delete Snapshot
			err = client.DeleteSnapshot(snapshotID)
			Expect(err).To(BeNil(), "DeleteSnapshot failed")

			// Validate Snapshot Deleted
			_, err = computeService.Snapshots.Get(p, snapshotName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected snapshot to not be found")
		}()
	})

	// Use the region of the test location.
	It("Should successfully create snapshot backed by disk image", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		// Create Disk
		volName, volID := createAndValidateUniqueZonalDisk(client, p, z, standardDiskType)

		// Create Snapshot
		snapshotName := testNamePrefix + string(uuid.NewUUID())
		testImageFamily := "test-family"

		snapshotParams := map[string]string{common.ParameterKeySnapshotType: common.DiskImageType, common.ParameterKeyImageFamily: testImageFamily}
		snapshotID, err := client.CreateSnapshot(snapshotName, volID, snapshotParams)
		Expect(err).To(BeNil(), "CreateSnapshot failed with error: %v", err)

		// Validate Snapshot Created
		snapshot, err := computeService.Images.Get(p, snapshotName).Do()
		Expect(err).To(BeNil(), "Could not get snapshot from cloud directly")
		Expect(snapshot.Name).To(Equal(snapshotName))

		err = wait.Poll(10*time.Second, 5*time.Minute, func() (bool, error) {
			snapshot, err := computeService.Images.Get(p, snapshotName).Do()
			Expect(err).To(BeNil(), "Could not get snapshot from cloud directly")
			if snapshot.Status == "READY" {
				return true, nil
			}
			return false, nil
		})
		Expect(err).To(BeNil(), "Could not wait for snapshot be ready")

		// Check Snapshot Type
		snapshot, err = computeService.Images.Get(p, snapshotName).Do()
		Expect(err).To(BeNil(), "Could not get snapshot from cloud directly")
		_, snapshotType, _, err := common.SnapshotIDToProjectKey(cleanSelfLink(snapshot.SelfLink))
		Expect(err).To(BeNil(), "Failed to parse snapshot ID")
		Expect(snapshotType).To(Equal(common.DiskImageType), "Expected images type in snapshot ID")

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")

			// Delete Snapshot
			err = client.DeleteSnapshot(snapshotID)
			Expect(err).To(BeNil(), "DeleteSnapshot failed")

			// Validate Snapshot Deleted
			_, err = computeService.Images.Get(p, snapshotName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected snapshot to not be found")
		}()
	})

	It("Should successfully create zonal PD from a zonal PD VolumeContentSource", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		controllerInstance := testContext.Instance
		controllerClient := testContext.Client

		p, z, _ := controllerInstance.GetIdentity()

		// Create Source Disk
		_, srcVolID := createAndValidateUniqueZonalDisk(controllerClient, p, z, standardDiskType)

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volume, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyReplicationType: "none",
		}, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{common.TopologyKeyZone: z},
					},
				},
			},
			&csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: srcVolID,
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
		// Validate the the clone disk zone matches the source disk zone.
		_, srcKey, err := common.VolumeIDToKey(srcVolID)
		Expect(err).To(BeNil(), "Could not get source volume key from id")
		Expect(zoneFromURL(cloudDisk.Zone)).To(Equal(srcKey.Zone))
		defer func() {
			// Delete Disk
			controllerClient.DeleteVolume(volume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()
	})

	It("Should successfully create RePD from a zonal PD VolumeContentSource", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		controllerInstance := testContext.Instance
		controllerClient := testContext.Client

		p, z, _ := controllerInstance.GetIdentity()

		region, err := common.GetRegionFromZones([]string{z})
		Expect(err).To(BeNil(), "Failed to get region from zones")

		// Create Source Disk
		srcVolName := testNamePrefix + string(uuid.NewUUID())
		srcVolume, err := controllerClient.CreateVolume(srcVolName, map[string]string{
			common.ParameterKeyReplicationType: "none",
		}, defaultRepdSizeGb, nil, nil)
		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volume, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyReplicationType: "regional-pd",
		}, defaultRepdSizeGb, nil,
			&csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: srcVolume.VolumeId,
					},
				},
			})

		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := computeService.RegionDisks.Get(p, region, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultRepdSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))
		Expect(len(cloudDisk.ReplicaZones)).To(Equal(2))
		replicaZonesCompatible := false
		_, srcKey, err := common.VolumeIDToKey(srcVolume.VolumeId)
		Expect(err).To(BeNil(), "Could not get source volume key from id")
		for _, replicaZone := range cloudDisk.ReplicaZones {
			actualZone := zoneFromURL(replicaZone)
			if actualZone == srcKey.Zone {
				replicaZonesCompatible = true
			}
			gotRegion, err := common.GetRegionFromZones([]string{actualZone})
			Expect(err).To(BeNil(), "failed to get region from actual zone %v", actualZone)
			Expect(gotRegion).To(Equal(region), "Got region from replica zone that did not match supplied region")
		}
		// Validate that one of the replicaZones of the clone matches the zone of the source disk.
		Expect(replicaZonesCompatible).To(Equal(true))
		defer func() {
			// Delete Disk
			controllerClient.DeleteVolume(volume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.RegionDisks.Get(p, region, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()
	})

	It("Should successfully create RePD from a RePD VolumeContentSource", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		controllerInstance := testContext.Instance
		controllerClient := testContext.Client

		p, z, _ := controllerInstance.GetIdentity()

		region, err := common.GetRegionFromZones([]string{z})
		Expect(err).To(BeNil(), "Failed to get region from zones")

		// Create Source Disk
		srcVolName := testNamePrefix + string(uuid.NewUUID())
		srcVolume, err := controllerClient.CreateVolume(srcVolName, map[string]string{
			common.ParameterKeyReplicationType: "regional-pd",
		}, defaultRepdSizeGb, nil, nil)
		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volume, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyReplicationType: "regional-pd",
		}, defaultRepdSizeGb, nil,
			&csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: srcVolume.VolumeId,
					},
				},
			})

		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := computeService.RegionDisks.Get(p, region, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultRepdSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))
		Expect(len(cloudDisk.ReplicaZones)).To(Equal(2))
		// Validate that the replicaZones of the clone match the replicaZones of the source disk.
		srcCloudDisk, err := computeService.RegionDisks.Get(p, region, srcVolName).Do()
		Expect(err).To(BeNil(), "Could not get source disk from cloud directly")
		Expect(srcCloudDisk.ReplicaZones).To(Equal(cloudDisk.ReplicaZones))
		for _, replicaZone := range cloudDisk.ReplicaZones {
			actualZone := zoneFromURL(replicaZone)
			gotRegion, err := common.GetRegionFromZones([]string{actualZone})
			Expect(err).To(BeNil(), "failed to get region from actual zone %v", actualZone)
			Expect(gotRegion).To(Equal(region), "Got region from replica zone that did not match supplied region")
		}
		defer func() {
			// Delete Disk
			controllerClient.DeleteVolume(volume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.RegionDisks.Get(p, region, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()
	})

	It("Should pass if valid compute endpoint is passed in", func() {
		// gets instance set up w/o compute-endpoint set from test setup
		_, err := getRandomTestContext().Client.ListVolumes()
		Expect(err).To(BeNil(), "no error expected when passed valid compute url")

		zone := "us-central1-c"
		nodeID := fmt.Sprintf("gce-pd-csi-e2e-%s", zone)
		i, err := remote.SetupInstance(*project, *architecture, zone, nodeID, *machineType, *serviceAccount, *imageURL, computeService)

		if err != nil {
			klog.Fatalf("Failed to setup instance %v: %w", nodeID, err)
		}

		klog.Infof("Creating new driver and client for node %s\n", i.GetName())

		// Create new driver and client w/ valid, passed-in endpoint
		tcValid, err := testutils.GCEClientAndDriverSetup(i, "https://compute.googleapis.com")
		if err != nil {
			klog.Fatalf("Failed to set up Test Context for instance %v: %w", i.GetName(), err)
		}
		_, err = tcValid.Client.ListVolumes()

		Expect(err).To(BeNil(), "no error expected when passed valid compute url")
	})

	type multiZoneTestConfig struct {
		diskType          string
		readOnly          bool
		hasMultiZoneLabel bool
		wantErrSubstring  string
	}

	DescribeTable("Unsupported 'multi-zone' PV ControllerPublish attempts",
		func(cfg multiZoneTestConfig) {
			Expect(testContexts).ToNot(BeEmpty())
			testContext := getRandomTestContext()

			controllerInstance := testContext.Instance
			controllerClient := testContext.Client

			p, z, _ := controllerInstance.GetIdentity()

			volName := testNamePrefix + string(uuid.NewUUID())
			_, diskVolumeId := createAndValidateZonalDisk(controllerClient, p, z, cfg.diskType, volName)
			defer deleteDisk(controllerClient, p, z, diskVolumeId, volName)

			if cfg.hasMultiZoneLabel {
				labelsMap := map[string]string{
					common.MultiZoneLabel: "true",
				}
				disk, err := computeService.Disks.Get(p, z, volName).Do()
				Expect(err).To(BeNil(), "Could not get disk")
				diskOp, err := computeService.Disks.SetLabels(p, z, volName, &compute.ZoneSetLabelsRequest{
					LabelFingerprint: disk.LabelFingerprint,
					Labels:           labelsMap,
				}).Do()
				Expect(err).To(BeNil(), "Could not set disk labels")
				_, err = computeService.ZoneOperations.Wait(p, z, diskOp.Name).Do()
				Expect(err).To(BeNil(), "Could not set disk labels")
			}

			// Attach Disk
			volID := fmt.Sprintf("projects/%s/zones/multi-zone/disks/%s", p, volName)
			nodeID := testContext.Instance.GetNodeID()

			err := controllerClient.ControllerPublishVolume(volID, nodeID, false /* forceAttach */, cfg.readOnly)
			Expect(err).ToNot(BeNil(), "Unexpected success attaching disk")
			Expect(err.Error()).To(ContainSubstring(cfg.wantErrSubstring), "Expected err")
		},
		Entry("with unsupported ROX mode", multiZoneTestConfig{diskType: standardDiskType, readOnly: false, hasMultiZoneLabel: true, wantErrSubstring: "'multi-zone' volume only supports 'readOnly'"}),
		Entry("with missing multi-zone label", multiZoneTestConfig{diskType: standardDiskType, readOnly: true, hasMultiZoneLabel: false, wantErrSubstring: "points to disk that is missing label \"goog-gke-multi-zone\""}),
		Entry("with unsupported disk-type pd-extreme", multiZoneTestConfig{diskType: extremeDiskType, readOnly: true, hasMultiZoneLabel: true, wantErrSubstring: "points to disk with unsupported disk type"}),
	)
})

func equalWithinEpsilon(a, b, epsiolon int64) bool {
	if a > b {
		return a-b < epsiolon
	}
	return b-a < epsiolon
}

func createAndValidateUniqueZonalDisk(client *remote.CsiClient, project, zone string, diskType string) (string, string) {
	volName := testNamePrefix + string(uuid.NewUUID())
	return createAndValidateZonalDisk(client, project, zone, diskType, volName)
}

func createAndValidateZonalDisk(client *remote.CsiClient, project, zone string, diskType string, volName string) (string, string) {
	// Create Disk
	disk := typeToDisk[diskType]

	diskSize := defaultSizeGb
	switch diskType {
	case extremeDiskType:
		diskSize = defaultExtremeSizeGb
	case hdtDiskType:
		diskSize = defaultHdTSizeGb
	}
	volume, err := client.CreateVolume(volName, disk.params, diskSize,
		&csi.TopologyRequirement{
			Requisite: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: zone},
				},
			},
		}, nil)
	Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

	// Validate Disk Created
	cloudDisk, err := computeService.Disks.Get(project, zone, volName).Do()
	Expect(err).To(BeNil(), "Could not get disk from cloud directly")
	Expect(cloudDisk.Status).To(Equal(readyState))
	Expect(cloudDisk.SizeGb).To(Equal(diskSize))
	Expect(cloudDisk.Name).To(Equal(volName))
	disk.validate(cloudDisk)

	return volName, volume.VolumeId
}

func deleteVolumeOrError(client *remote.CsiClient, volID string) {
	// Delete Disk
	err := client.DeleteVolume(volID)
	Expect(err).To(BeNil(), "DeleteVolume failed")

	// Validate Disk Deleted
	project, key, err := common.VolumeIDToKey(volID)
	Expect(err).To(BeNil(), "Failed to conver volume ID To key")
	_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
	Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
}

func createAndValidateUniqueZonalMultiWriterDisk(client *remote.CsiClient, project, zone string, diskType string) (string, string) {
	// Create Disk
	disk := typeToDisk[diskType]
	volName := testNamePrefix + string(uuid.NewUUID())
	volume, err := client.CreateVolumeWithCaps(volName, disk.params, defaultMwSizeGb,
		&csi.TopologyRequirement{
			Requisite: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: zone},
				},
			},
		},
		[]*csi.VolumeCapability{
			{
				AccessType: &csi.VolumeCapability_Block{
					Block: &csi.VolumeCapability_BlockVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
				},
			},
		}, nil)
	Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

	// Validate Disk Created
	cloudDisk, err := computeService.Disks.Get(project, zone, volName).Do()
	Expect(err).To(BeNil(), "Failed to get cloud disk")
	Expect(cloudDisk.Status).To(Equal(readyState))
	Expect(cloudDisk.SizeGb).To(Equal(defaultMwSizeGb))
	Expect(cloudDisk.Name).To(Equal(volName))
	disk.validate(cloudDisk)

	alphaDisk, err := computeAlphaService.Disks.Get(project, zone, volName).Do()
	Expect(err).To(BeNil(), "Failed to get cloud disk using alpha API")
	Expect(alphaDisk.MultiWriter).To(Equal(true))

	return volName, volume.VolumeId
}

func cleanSelfLink(selfLink string) string {
	r, _ := regexp.Compile("https:\\/\\/www.*apis.com\\/[a-z]+\\/(v1|beta|alpha)\\/")
	return r.ReplaceAllString(selfLink, "")
}

// Returns the zone from the URL with the format https://compute.googleapis.com/compute/v1/projects/{project}/zones/{zone}.
// Returns the empty string if the zone cannot be abstracted from the URL.
func zoneFromURL(url string) string {
	tokens := strings.Split(url, "/")
	if len(tokens) == 0 {
		return ""
	}
	return tokens[len(tokens)-1]
}

func setupKeyRing(ctx context.Context, parentName string, keyRingId string) (*kmspb.CryptoKey, []string) {
	// Create KeyRing
	ringReq := &kmspb.CreateKeyRingRequest{
		Parent:    parentName,
		KeyRingId: keyRingId,
	}
	keyRing, err := kmsClient.CreateKeyRing(ctx, ringReq)
	if !gce.IsGCEError(err, "alreadyExists") {
		getKeyRingReq := &kmspb.GetKeyRingRequest{
			Name: fmt.Sprintf("%s/keyRings/%s", parentName, keyRingId),
		}
		keyRing, err = kmsClient.GetKeyRing(ctx, getKeyRingReq)

	}
	Expect(err).To(BeNil(), "Failed to create or get key ring %v", keyRingId)

	// Create CryptoKey in KeyRing
	keyId := "test-key-" + string(uuid.NewUUID())
	keyReq := &kmspb.CreateCryptoKeyRequest{
		Parent:      keyRing.Name,
		CryptoKeyId: keyId,
		CryptoKey: &kmspb.CryptoKey{
			Purpose: kmspb.CryptoKey_ENCRYPT_DECRYPT,
			VersionTemplate: &kmspb.CryptoKeyVersionTemplate{
				Algorithm: kmspb.CryptoKeyVersion_GOOGLE_SYMMETRIC_ENCRYPTION,
			},
		},
	}
	key, err := kmsClient.CreateCryptoKey(ctx, keyReq)
	Expect(err).To(BeNil(), "Failed to create crypto key %v in key ring %v", keyId, keyRing.Name)

	keyVersions := []string{}
	keyVersionReq := &kmspb.ListCryptoKeyVersionsRequest{
		Parent: key.Name,
	}

	it := kmsClient.ListCryptoKeyVersions(ctx, keyVersionReq)

	for {
		keyVersion, err := it.Next()
		if err == iterator.Done {
			break
		}
		Expect(err).To(BeNil(), "Failed to list crypto key versions")

		keyVersions = append(keyVersions, keyVersion.Name)
	}
	return key, keyVersions
}

type disk struct {
	params   map[string]string
	validate func(disk *compute.Disk)
}

var typeToDisk = map[string]*disk{
	standardDiskType: {
		params: map[string]string{
			common.ParameterKeyType: standardDiskType,
		},
		validate: func(disk *compute.Disk) {
			Expect(disk.Type).To(ContainSubstring(standardDiskType))
		},
	},
	extremeDiskType: {
		params: map[string]string{
			common.ParameterKeyType:                    extremeDiskType,
			common.ParameterKeyProvisionedIOPSOnCreate: provisionedIOPSOnCreate,
		},
		validate: func(disk *compute.Disk) {
			Expect(disk.Type).To(ContainSubstring(extremeDiskType))
			Expect(disk.ProvisionedIops).To(Equal(provisionedIOPSOnCreateInt))
		},
	},
	hdtDiskType: {
		params: map[string]string{
			common.ParameterKeyType:                          hdtDiskType,
			common.ParameterKeyProvisionedThroughputOnCreate: provisionedThroughputOnCreate,
		},
		validate: func(disk *compute.Disk) {
			Expect(disk.Type).To(ContainSubstring(hdtDiskType))
			Expect(disk.ProvisionedThroughput).To(Equal(provisionedThroughputOnCreateInt))
		},
	},
}

func merge(a, b map[string]string) map[string]string {
	res := map[string]string{}
	for k, v := range a {
		res[k] = v
	}
	for k, v := range b {
		res[k] = v
	}
	return res
}
