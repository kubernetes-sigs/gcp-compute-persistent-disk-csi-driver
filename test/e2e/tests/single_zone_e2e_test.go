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

	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"

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
		// Create new driver and client
		// TODO: Should probably actual have some object that includes both client and instance so we can relate the two??
		Expect(testInstances).NotTo(BeEmpty())
		testContext, err := testutils.SetupNewDriverAndClient(testInstances[0])
		Expect(err).To(BeNil(), "Set up new Driver and Client failed with error")
		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volId, err := client.CreateVolume(volName, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// TODO: Validate Disk Created
		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))

		defer func() {
			// Delete Disk
			client.DeleteVolume(volId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// TODO: Validate Disk Deleted
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to not be found")
		}()

		// Attach Disk
		err = client.ControllerPublishVolume(volId, instance.GetName())
		Expect(err).To(BeNil(), "ControllerPublishVolume failed with error")

		defer func() {
			// Detach Disk
			err := client.ControllerUnpublishVolume(volId, instance.GetName())
			Expect(err).To(BeNil(), "ControllerUnpublishVolume failed with error")
		}()

		// Stage Disk
		stageDir := filepath.Join("/tmp/", volName, "stage")
		client.NodeStageVolume(volId, stageDir)
		Expect(err).To(BeNil(), "NodeStageVolume failed with error")

		defer func() {
			// Unstage Disk
			err := client.NodeUnstageVolume(volId, stageDir)
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

		// Write a file
		testFileContents := "test"
		testFile := filepath.Join(publishDir, "testfile")
		err = testutils.WriteFile(instance, testFile, testFileContents)
		Expect(err).To(BeNil(), "Failed to write file")

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

	})

	It("Should create disks in correct zones when topology is specified", func() {
		///
		Expect(testInstances).NotTo(BeEmpty())
		testContext, err := testutils.SetupNewDriverAndClient(testInstances[0])
		Expect(err).To(BeNil(), "Failed to set up new driver and client")
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
			volID, err := testContext.Client.CreateVolume(volName, topReq)
			Expect(err).To(BeNil(), "Failed to create volume")
			defer func() {
				err = testContext.Client.DeleteVolume(volID)
				Expect(err).To(BeNil(), "Failed to delete volume")
			}()

			_, err = computeService.Disks.Get(p, zone, volName).Do()
			Expect(err).To(BeNil(), "Could not find disk in correct zone")
		}

	})

	// Test volume already exists

	// Test volume with op pending
})

func Logf(format string, args ...interface{}) {
	fmt.Fprint(GinkgoWriter, args...)
}
