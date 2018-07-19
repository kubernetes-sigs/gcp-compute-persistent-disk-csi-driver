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
	remote "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/binremote"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	network        = "unix"
	testNamePrefix = "gcepd-csi-e2e-"

	defaultSizeGb    int64 = 5
	readyState             = "READY"
	standardDiskType       = "pd-standard"
	ssdDiskType            = "pd-ssd"
)

var (
	client   *utils.CsiClient
	instance *remote.InstanceInfo
	//gceCloud *gce.CloudProvider
	nodeID string
)

var _ = BeforeSuite(func() {
	var err error
	// TODO(dyzz): better defaults
	nodeID = "gce-pd-csi-e2e-us-central1-c"
	port := "2000"
	if *runInProw {
		*project, *serviceAccount = utils.SetupProwConfig()
	}

	Expect(*project).ToNot(BeEmpty(), "Project should not be empty")
	Expect(*serviceAccount).ToNot(BeEmpty(), "Service account should not be empty")

	instance, err = utils.SetupInstanceAndDriver(*project, "us-central1-c", nodeID, port, *serviceAccount)
	Expect(err).To(BeNil())

	client = utils.CreateCSIClient(fmt.Sprintf("localhost:%s", port))
})

var _ = AfterSuite(func() {
	// Close the client
	err := client.CloseConn()
	if err != nil {
		Logf("Failed to close the client")
	} else {
		Logf("Closed the client")
	}

	// instance.DeleteInstance()
})

var _ = Describe("GCE PD CSI Driver", func() {

	BeforeEach(func() {
		err := client.AssertCSIConnection()
		Expect(err).To(BeNil(), "Failed to assert csi client connection: %v", err)
	})

	It("Should create->attach->stage->mount volume and check if it is writable, then unmount->unstage->detach->delete and check disk is deleted", func() {
		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volId, err := client.CreateVolume(volName)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// TODO: Validate Disk Created
		/*cloudDisk, err := gceCloud.GetDiskOrError(context.Background(), gceCloud.GetZone(), volName)
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))*/

		defer func() {
			// Delete Disk
			client.DeleteVolume(volId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// TODO: Validate Disk Deleted
			/*_, err = gceCloud.GetDiskOrError(context.Background(), gceCloud.GetZone(), volName)
			serverError, ok := status.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(serverError.Code()).To(Equal(codes.NotFound))*/
		}()

		// Attach Disk
		err = client.ControllerPublishVolume(volId, nodeID)
		Expect(err).To(BeNil(), "ControllerPublishVolume failed with error")

		defer func() {
			// Detach Disk
			err := client.ControllerUnpublishVolume(volId, nodeID)
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
			err = utils.RmAll(instance, filepath.Join("/tmp/", volName))
			Expect(err).To(BeNil(), "Failed to remove temp directory")
		}()

		// Mount Disk
		publishDir := filepath.Join("/tmp/", volName, "mount")
		err = client.NodePublishVolume(volId, stageDir, publishDir)
		Expect(err).To(BeNil(), "NodePublishVolume failed with error")
		err = utils.ForceChmod(instance, filepath.Join("/tmp/", volName), "777")
		Expect(err).To(BeNil(), "Chmod failed with error")

		// Write a file
		testFileContents := "test"
		testFile := filepath.Join(publishDir, "testfile")
		err = utils.WriteFile(instance, testFile, testFileContents)
		Expect(err).To(BeNil(), "Failed to write file")

		// Unmount Disk
		err = client.NodeUnpublishVolume(volId, publishDir)
		Expect(err).To(BeNil(), "NodeUnpublishVolume failed with error")

		// Mount disk somewhere else
		secondPublishDir := filepath.Join("/tmp/", volName, "secondmount")
		err = client.NodePublishVolume(volId, stageDir, secondPublishDir)
		Expect(err).To(BeNil(), "NodePublishVolume failed with error")
		err = utils.ForceChmod(instance, filepath.Join("/tmp/", volName), "777")
		Expect(err).To(BeNil(), "Chmod failed with error")

		// Read File
		secondTestFile := filepath.Join(secondPublishDir, "testfile")
		readContents, err := utils.ReadFile(instance, secondTestFile)
		Expect(err).To(BeNil(), "ReadFile failed with error")
		Expect(strings.TrimSpace(string(readContents))).To(Equal(testFileContents))

		// Unmount Disk
		err = client.NodeUnpublishVolume(volId, secondPublishDir)
		Expect(err).To(BeNil(), "NodeUnpublishVolume failed with error")

	})

	// Test volume already exists

	// Test volume with op pending
})

func Logf(format string, args ...interface{}) {
	fmt.Fprint(GinkgoWriter, args...)
}
