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
	"time"

	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	kmspb "google.golang.org/genproto/googleapis/cloud/kms/v1"
	fieldmask "google.golang.org/genproto/protobuf/field_mask"
)

var _ = Describe("GCE PD CSI Driver pd-extreme", func() {

	It("Should create and delete pd-extreme disk", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		params := map[string]string{
			common.ParameterKeyType:                    extremeDiskType,
			common.ParameterKeyProvisionedIOPSOnCreate: provisionedIOPSOnCreate,
		}
		volID, err := client.CreateVolume(volName, params, defaultExtremeSizeGb, nil, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(extremeDiskType))
		Expect(cloudDisk.ProvisionedIops).To(Equal(provisionedIOPSOnCreateInt))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultExtremeSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()
	})

	It("Should create and delete pd-extreme disk with labels", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		params := map[string]string{
			common.ParameterKeyLabels:                  "key1=value1,key2=value2",
			common.ParameterKeyType:                    extremeDiskType,
			common.ParameterKeyProvisionedIOPSOnCreate: provisionedIOPSOnCreate,
		}
		volID, err := client.CreateVolume(volName, params, defaultExtremeSizeGb, nil, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(extremeDiskType))
		Expect(cloudDisk.ProvisionedIops).To(Equal(provisionedIOPSOnCreateInt))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultExtremeSizeGb))
		Expect(cloudDisk.Labels).To(Equal(map[string]string{
			"key1": "value1",
			"key2": "value2",
			// The label below is added as an --extra-label driver command line argument.
			testutils.DiskLabelKey: testutils.DiskLabelValue,
		}))
		Expect(cloudDisk.Name).To(Equal(volName))

		defer func() {
			// Delete Disk
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()
	})

	It("Should create CMEK key, go through volume lifecycle, validate behavior on key revoke and restore for pd-extreme", func() {
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

		// Go through volume lifecycle using CMEK-ed PD
		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volID, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyDiskEncryptionKmsKey:    key.Name,
			common.ParameterKeyType:                    extremeDiskType,
			common.ParameterKeyProvisionedIOPSOnCreate: provisionedIOPSOnCreate,
		}, defaultExtremeSizeGb,
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
		Expect(cloudDisk.Type).To(ContainSubstring(extremeDiskType))
		Expect(cloudDisk.ProvisionedIops).To(Equal(provisionedIOPSOnCreateInt))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultExtremeSizeGb))
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

		// The controller publish failure in above step would set a backoff condition on the node. Wait suffcient amount of time for the driver to accept new controller publish requests.
		time.Sleep(time.Second)
		// Make sure attach of PD succeeds
		err = testAttachWriteReadDetach(volID, volName, controllerInstance, controllerClient, false /* readOnly */)
		Expect(err).To(BeNil(), "Failed to go through volume lifecycle after restoring CMEK key")
	})

	It("Should successfully create pd-extreme disk with PVC/PV tags", func() {
		Expect(testContexts).ToNot(BeEmpty())
		testContext := getRandomTestContext()

		controllerInstance := testContext.Instance
		controllerClient := testContext.Client

		p, z, _ := controllerInstance.GetIdentity()

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volID, err := controllerClient.CreateVolume(volName, map[string]string{
			common.ParameterKeyPVCName:                 "test-pvc",
			common.ParameterKeyPVCNamespace:            "test-pvc-namespace",
			common.ParameterKeyPVName:                  "test-pv-name",
			common.ParameterKeyType:                    extremeDiskType,
			common.ParameterKeyProvisionedIOPSOnCreate: provisionedIOPSOnCreate,
		}, defaultExtremeSizeGb, nil /* topReq */, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(extremeDiskType))
		Expect(cloudDisk.ProvisionedIops).To(Equal(provisionedIOPSOnCreateInt))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultExtremeSizeGb))
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
