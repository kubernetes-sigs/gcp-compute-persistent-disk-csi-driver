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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"

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
	readyState               = "READY"
	standardDiskType         = "pd-standard"
	ssdDiskType              = "pd-ssd"
	defaultVolumeLimit int64 = 128
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
		err = testAttachWriteReadDetach(volID, volName, instance, client, false /* readOnly */)
		Expect(err).To(BeNil(), "Failed to go through volume lifecycle")

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

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		_, err := client.CreateVolume(volName, nil, defaultSizeGb,
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

		underSpecifiedID := common.GenerateUnderspecifiedVolumeID(volName, true /* isZonal */)

		defer func() {
			// Delete Disk
			client.DeleteVolume(underSpecifiedID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach Disk
		err = testAttachWriteReadDetach(underSpecifiedID, volName, instance, client, false /* readOnly */)
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
		cloudDisk, err := betaComputeService.RegionDisks.Get(p, region, volName).Do()
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
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

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
		snapshotID, err := client.CreateSnapshot(snapshotName, volId, nil)
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
		volId, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyReplicationType: "regional-pd",
		}, defaultRepdSizeGb, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := betaComputeService.RegionDisks.Get(p, region, volName).Do()
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
		snapshotID, err := controllerClient.CreateSnapshot(snapshotName, volId, nil)
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
			err = controllerClient.DeleteSnapshot(snapshotID)
			Expect(err).To(BeNil(), "DeleteSnapshot failed")

			// Validate Snapshot Deleted
			_, err = computeService.Snapshots.Get(p, snapshotName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected snapshot to not be found")
		}()
	})
})
