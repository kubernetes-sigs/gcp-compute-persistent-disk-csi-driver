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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	mountmanager "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"google.golang.org/api/iterator"
	kmspb "google.golang.org/genproto/googleapis/cloud/kms/v1"
	fieldmask "google.golang.org/genproto/protobuf/field_mask"
)

const (
	testNamePrefix = "gcepd-csi-e2e-"

	defaultSizeGb      int64 = 5
	defaultRepdSizeGb  int64 = 200
	defaultMwSizeGb    int64 = 200
	readyState               = "READY"
	standardDiskType         = "pd-standard"
	defaultVolumeLimit int64 = 127

	defaultEpsilon = 500000000 // 500M
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
		volName, volID := createAndValidateUniqueZonalDisk(client, p, z)

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

	It("Should automatically fix symlink errors between /dev/sdx and /dev/by-id if disk is not found", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		// Set-up instance to have scsi_id where we expect it

		// Create Disk
		volName, volID := createAndValidateUniqueZonalDisk(client, p, z)

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach Disk
		err := client.ControllerPublishVolume(volID, instance.GetNodeID())
		Expect(err).To(BeNil(), "ControllerPublishVolume failed with error for disk %v on node %v: %v", volID, instance.GetNodeID())

		defer func() {
			// Detach Disk
			err = client.ControllerUnpublishVolume(volID, instance.GetNodeID())
			if err != nil {
				klog.Errorf("Failed to detach disk: %v", err)
			}

		}()

		// MESS UP THE symlink
		devicePaths := mountmanager.NewDeviceUtils().GetDiskByIdPaths(volName, "")
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
				klog.Errorf("Failed to unstage volume: %v", err)
			}
			fp := filepath.Join("/tmp/", volName)
			err = testutils.RmAll(instance, fp)
			if err != nil {
				klog.Errorf("Failed to rm file path %s: %v", fp, err)
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

	It("Should complete entire disk lifecycle with underspecified volume ID", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		volName, _ := createAndValidateUniqueZonalDisk(client, p, z)

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
		}, defaultRepdSizeGb, nil)
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
			_, err = computeService.RegionDisks.Get(p, region, volName).Do()
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
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		volName, volID := createAndValidateUniqueZonalDisk(client, p, z)

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

	It("Should create CMEK key, go through volume lifecycle, validate behavior on key revoke and restore", func() {
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

		// Defer deletion of all key versions
		// https://cloud.google.com/kms/docs/destroy-restore
		defer func() {

			for _, keyVersion := range keyVersions {
				destroyKeyReq := &kmspb.DestroyCryptoKeyVersionRequest{
					Name: keyVersion,
				}
				_, err = kmsClient.DestroyCryptoKeyVersion(ctx, destroyKeyReq)
				Expect(err).To(BeNil(), "Failed to destroy crypto key version: %v", keyVersion)
			}

		}()

		// Go through volume lifecycle using CMEK-ed PD
		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volID, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyDiskEncryptionKmsKey: key.Name,
		}, defaultSizeGb,
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
			err = controllerClient.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Test disk works
		err = testAttachWriteReadDetach(volID, volName, controllerInstance, controllerClient, false /* readOnly */)
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
		err = testAttachWriteReadDetach(volID, volName, controllerInstance, controllerClient, false /* readOnly */)
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

		// Make sure attach of PD succeeds
		err = testAttachWriteReadDetach(volID, volName, controllerInstance, controllerClient, false /* readOnly */)
		Expect(err).To(BeNil(), "Failed to go through volume lifecycle after restoring CMEK key")
	})

	It("Should create disks, attach them places, and verify List returns correct results", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		nodeID := testContext.Instance.GetNodeID()

		_, volID := createAndValidateUniqueZonalDisk(client, p, z)
		defer deleteVolumeOrError(client, volID, p)

		_, secondVolID := createAndValidateUniqueZonalDisk(client, p, z)
		defer deleteVolumeOrError(client, secondVolID, p)

		// Attach volID to current instance
		err := client.ControllerPublishVolume(volID, nodeID)
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
		volID, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyReplicationType: "regional-pd",
		}, defaultRepdSizeGb, nil)
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
			tokens := strings.Split(replicaZone, "/")
			actualZone := tokens[len(tokens)-1]
			gotRegion, err := common.GetRegionFromZones([]string{actualZone})
			Expect(err).To(BeNil(), "failed to get region from actual zone %v", actualZone)
			Expect(gotRegion).To(Equal(region), "Got region from replica zone that did not match supplied region")
		}

		// Create Snapshot
		snapshotName := testNamePrefix + string(uuid.NewUUID())
		snapshotID, err := controllerClient.CreateSnapshot(snapshotName, volID, nil)
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
			err := controllerClient.DeleteVolume(volID)
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

		volName, volID := createAndValidateUniqueZonalDisk(client, p, z)

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		verifyVolumeStats := func(a verifyArgs) error {
			available, capacity, used, inodesFree, inodes, inodesUsed, err := client.NodeGetVolumeStats(volID, a.publishDir)
			if err != nil {
				return fmt.Errorf("failed to get node volume stats: %v", err)
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

		volName, volID := createAndValidateUniqueZonalDisk(client, p, z)

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		verifyVolumeStats := func(a verifyArgs) error {
			available, capacity, used, inodesFree, inodes, inodesUsed, err := client.NodeGetVolumeStats(volID, a.publishDir)
			if err != nil {
				return fmt.Errorf("failed to get node volume stats: %v", err)
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
		volName, volID := createAndValidateUniqueZonalMultiWriterDisk(client, p, zone)

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
		volName, volID := createAndValidateUniqueZonalMultiWriterDisk(client, p, z)

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
		writeFunc := func(a verifyArgs) error {
			err := testutils.WriteBlock(instance, a.publishDir, testFileContents)
			if err != nil {
				return fmt.Errorf("Failed to write file: %v", err)
			}
			return nil
		}
		verifyReadFunc := func(a verifyArgs) error {
			readContents, err := testutils.ReadBlock(instance, a.publishDir, len(testFileContents))
			if err != nil {
				return fmt.Errorf("ReadFile failed with error: %v", err)
			}
			if strings.TrimSpace(string(readContents)) != testFileContents {
				return fmt.Errorf("wanted test file content: %s, got content: %s", testFileContents, readContents)
			}
			return nil
		}
		err := testLifecycleWithVerify(volID, volName, instance, client, false /* readOnly */, true /* block */, writeFunc, verifyReadFunc)
		Expect(err).To(BeNil(), "Failed to go through volume lifecycle")
	})

	It("Should successfully create disk with PVC/PV tags", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		controllerInstance := testContext.Instance
		controllerClient := testContext.Client

		p, z, _ := controllerInstance.GetIdentity()

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volID, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyPVCName:      "test-pvc",
			common.ParameterKeyPVCNamespace: "test-pvc-namespace",
			common.ParameterKeyPVName:       "test-pv-name",
		}, defaultSizeGb, nil /* topReq */)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))
		Expect(cloudDisk.Description).To(Equal("{\"kubernetes.io/created-for/pv/name\":\"test-pv-name\",\"kubernetes.io/created-for/pvc/name\":\"test-pvc\",\"kubernetes.io/created-for/pvc/namespace\":\"test-pvc-namespace\",\"storage.gke.io/created-by\":\"pd.csi.storage.gke.io\"}"))
		defer func() {
			// Delete Disk
			controllerClient.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()
	})
})

func equalWithinEpsilon(a, b, epsiolon int64) bool {
	if a > b {
		return a-b < epsiolon
	}
	return b-a < epsiolon
}

func createAndValidateUniqueZonalDisk(client *remote.CsiClient, project, zone string) (volName, volID string) {
	// Create Disk
	var err error
	volName = testNamePrefix + string(uuid.NewUUID())
	volID, err = client.CreateVolume(volName, nil, defaultSizeGb,
		&csi.TopologyRequirement{
			Requisite: []*csi.Topology{
				{
					Segments: map[string]string{common.TopologyKeyZone: zone},
				},
			},
		})
	Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

	// Validate Disk Created
	cloudDisk, err := computeService.Disks.Get(project, zone, volName).Do()
	Expect(err).To(BeNil(), "Could not get disk from cloud directly")
	Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
	Expect(cloudDisk.Status).To(Equal(readyState))
	Expect(cloudDisk.SizeGb).To(Equal(defaultSizeGb))
	Expect(cloudDisk.Name).To(Equal(volName))
	return
}

func deleteVolumeOrError(client *remote.CsiClient, volID, project string) {
	// Delete Disk
	err := client.DeleteVolume(volID)
	Expect(err).To(BeNil(), "DeleteVolume failed")

	// Validate Disk Deleted
	key, err := common.VolumeIDToKey(volID)
	Expect(err).To(BeNil(), "Failed to conver volume ID To key")
	_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
	Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
}

func createAndValidateUniqueZonalMultiWriterDisk(client *remote.CsiClient, project, zone string) (string, string) {
	// Create Disk
	volName := testNamePrefix + string(uuid.NewUUID())
	volID, err := client.CreateVolumeWithCaps(volName, nil, defaultMwSizeGb,
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
		})
	Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

	// Validate Disk Created
	cloudDisk, err := computeAlphaService.Disks.Get(project, zone, volName).Do()
	Expect(err).To(BeNil(), "Could not get disk from cloud directly")
	Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
	Expect(cloudDisk.Status).To(Equal(readyState))
	Expect(cloudDisk.SizeGb).To(Equal(defaultMwSizeGb))
	Expect(cloudDisk.Name).To(Equal(volName))
	Expect(cloudDisk.MultiWriter).To(Equal(true))

	return volName, volID
}
