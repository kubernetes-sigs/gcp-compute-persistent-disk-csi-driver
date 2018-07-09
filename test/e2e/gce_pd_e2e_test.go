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

package e2e

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"

	csipb "github.com/container-storage-interface/spec/lib/go/csi/v0"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider"
	driver "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-pd-csi-driver"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	endpoint       = "unix://tmp/csi.sock"
	addr           = "/tmp/csi.sock"
	network        = "unix"
	testNamePrefix = "gcepd-csi-e2e-"

	defaultSizeGb    int64 = 5
	readyState             = "READY"
	standardDiskType       = "pd-standard"
	ssdDiskType            = "pd-ssd"
)

var (
	client   *csiClient
	gceCloud *gce.CloudProvider
	nodeID   string

	stdVolCap = &csipb.VolumeCapability{
		AccessType: &csipb.VolumeCapability_Mount{
			Mount: &csipb.VolumeCapability_MountVolume{},
		},
		AccessMode: &csipb.VolumeCapability_AccessMode{
			Mode: csipb.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
	stdVolCaps = []*csipb.VolumeCapability{
		stdVolCap,
	}
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Google Compute Engine Persistent Disk Container Storage Interface Driver Tests")
}

var _ = BeforeSuite(func() {
	var err error
	// TODO(dyzz): better defaults
	driverName := "testdriver"
	nodeID = "gce-pd-csi-e2e"

	// TODO(dyzz): Start a driver
	gceDriver := driver.GetGCEDriver()
	gceCloud, err = gce.CreateCloudProvider(gceDriver.GetVendorVersion())

	Expect(err).To(BeNil(), "Failed to get cloud provider: %v", err)

	// TODO(dyzz): Change this to a fake mounter
	mounter, err := mountmanager.CreateMounter()

	Expect(err).To(BeNil(), "Failed to get mounter %v", err)

	//Initialize GCE Driver
	err = gceDriver.SetupGCEDriver(gceCloud, mounter, driverName, nodeID)
	Expect(err).To(BeNil(), "Failed to initialize GCE CSI Driver: %v", err)

	go func() {
		gceDriver.Run(endpoint)
	}()

	client = createCSIClient()

	Expect(err).To(BeNil(), "Failed to create cloud service")
	// TODO: This is a hack to make sure the driver is fully up before running the tests, theres probably a better way to do this.
	time.Sleep(20 * time.Second)
})

var _ = AfterSuite(func() {
	// Close the client
	err := client.conn.Close()
	if err != nil {
		Logf("Failed to close the client")
	} else {
		Logf("Closed the client")
	}
	// TODO(dyzz): Clean up driver and other things
})

var _ = Describe("GCE PD CSI Driver", func() {

	BeforeEach(func() {
		err := client.assertCSIConnection()
		Expect(err).To(BeNil(), "Failed to assert csi client connection: %v", err)
	})

	It("Should create->attach->stage->mount volume and check if it is writable, then unmount->unstage->detach->delete and check disk is deleted", func() {
		// Create Disk
		volName := testNamePrefix + string(uuid.NewUUID())
		volId, err := CreateVolume(volName)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

		// Validate Disk Created
		cloudDisk, err := gceCloud.GetDiskOrError(context.Background(), gceCloud.GetZone(), volName)
		Expect(err).To(BeNil(), "Could not get disk from cloud directly")
		Expect(cloudDisk.Type).To(ContainSubstring(standardDiskType))
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultSizeGb))
		Expect(cloudDisk.Name).To(Equal(volName))

		defer func() {
			// Delete Disk
			DeleteVolume(volId)
			Expect(err).To(BeNil(), "DeleteVolume failed")

			// Validate Disk Deleted
			_, err = gceCloud.GetDiskOrError(context.Background(), gceCloud.GetZone(), volName)
			serverError, ok := status.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(serverError.Code()).To(Equal(codes.NotFound))
		}()

		// Attach Disk
		err = ControllerPublishVolume(volId, nodeID)
		Expect(err).To(BeNil(), "ControllerPublishVolume failed with error")

		defer func() {
			// Detach Disk
			err := ControllerUnpublishVolume(volId, nodeID)
			Expect(err).To(BeNil(), "ControllerUnpublishVolume failed with error")
		}()

		// Stage Disk
		stageDir := filepath.Join("/tmp/", volName, "stage")
		NodeStageVolume(volId, stageDir)
		Expect(err).To(BeNil(), "NodeStageVolume failed with error")

		defer func() {
			// Unstage Disk
			err := NodeUnstageVolume(volId, stageDir)
			Expect(err).To(BeNil(), "NodeUnstageVolume failed with error")
		}()

		// Mount Disk
		publishDir := filepath.Join("/tmp/", volName, "mount")
		err = NodePublishVolume(volId, stageDir, publishDir)
		Expect(err).To(BeNil(), "NodePublishVolume failed with error")

		// Write a file
		testFile := filepath.Join(publishDir, "testfile")
		f, err := os.Create(testFile)
		Expect(err).To(BeNil(), "Opening file %s failed with error", testFile)

		testString := "test-string"
		f.WriteString(testString)
		f.Sync()
		f.Close()

		// Unmount Disk
		err = NodeUnpublishVolume(volId, publishDir)
		Expect(err).To(BeNil(), "NodeUnpublishVolume failed with error")

		// Mount disk somewhere else
		secondPublishDir := filepath.Join("/tmp/", volName, "secondmount")
		err = NodePublishVolume(volId, stageDir, secondPublishDir)
		Expect(err).To(BeNil(), "NodePublishVolume failed with error")

		// Read File
		fileContent, err := ioutil.ReadFile(filepath.Join(secondPublishDir, "testfile"))
		Expect(err).To(BeNil(), "ReadFile failed with error")
		Expect(string(fileContent)).To(Equal(testString))

		// Unmount Disk
		err = NodeUnpublishVolume(volId, secondPublishDir)
		Expect(err).To(BeNil(), "NodeUnpublishVolume failed with error")

	})
})

func Logf(format string, args ...interface{}) {
	fmt.Fprint(GinkgoWriter, args...)
}

type csiClient struct {
	conn       *grpc.ClientConn
	idClient   csipb.IdentityClient
	nodeClient csipb.NodeClient
	ctrlClient csipb.ControllerClient
}

func createCSIClient() *csiClient {
	return &csiClient{}
}

func (c *csiClient) assertCSIConnection() error {
	if c.conn == nil {
		conn, err := grpc.Dial(
			addr,
			grpc.WithInsecure(),
			grpc.WithDialer(func(target string, timeout time.Duration) (net.Conn, error) {
				return net.Dial(network, target)
			}),
		)
		if err != nil {
			return err
		}
		c.conn = conn
		c.idClient = csipb.NewIdentityClient(conn)
		c.nodeClient = csipb.NewNodeClient(conn)
		c.ctrlClient = csipb.NewControllerClient(conn)
	}
	return nil
}

func CreateVolume(volName string) (string, error) {
	cvr := &csipb.CreateVolumeRequest{
		Name:               volName,
		VolumeCapabilities: stdVolCaps,
	}
	cresp, err := client.ctrlClient.CreateVolume(context.Background(), cvr)
	if err != nil {
		return "", err
	}
	return cresp.GetVolume().GetId(), nil
}

func DeleteVolume(volId string) error {
	dvr := &csipb.DeleteVolumeRequest{
		VolumeId: volId,
	}
	_, err := client.ctrlClient.DeleteVolume(context.Background(), dvr)
	return err
}

func ControllerPublishVolume(volId, nodeId string) error {
	cpreq := &csipb.ControllerPublishVolumeRequest{
		VolumeId:         volId,
		NodeId:           nodeId,
		VolumeCapability: stdVolCap,
		Readonly:         false,
	}
	_, err := client.ctrlClient.ControllerPublishVolume(context.Background(), cpreq)
	return err
}

func ControllerUnpublishVolume(volId, nodeId string) error {
	cupreq := &csipb.ControllerUnpublishVolumeRequest{
		VolumeId: volId,
		NodeId:   nodeId,
	}
	_, err := client.ctrlClient.ControllerUnpublishVolume(context.Background(), cupreq)
	return err
}

func NodeStageVolume(volId, stageDir string) error {
	nodeStageReq := &csipb.NodeStageVolumeRequest{
		VolumeId:          volId,
		StagingTargetPath: stageDir,
		VolumeCapability:  stdVolCap,
	}
	_, err := client.nodeClient.NodeStageVolume(context.Background(), nodeStageReq)
	return err
}

func NodeUnstageVolume(volId, stageDir string) error {
	nodeUnstageReq := &csipb.NodeUnstageVolumeRequest{
		VolumeId:          volId,
		StagingTargetPath: stageDir,
	}
	_, err := client.nodeClient.NodeUnstageVolume(context.Background(), nodeUnstageReq)
	return err
}

func NodeUnpublishVolume(volumeId, publishDir string) error {
	nodeUnpublishReq := &csipb.NodeUnpublishVolumeRequest{
		VolumeId:   volumeId,
		TargetPath: publishDir,
	}
	_, err := client.nodeClient.NodeUnpublishVolume(context.Background(), nodeUnpublishReq)
	return err
}

func NodePublishVolume(volumeId, stageDir, publishDir string) error {
	nodePublishReq := &csipb.NodePublishVolumeRequest{
		VolumeId:          volumeId,
		StagingTargetPath: stageDir,
		TargetPath:        publishDir,
		VolumeCapability:  stdVolCap,
		Readonly:          false,
	}
	_, err := client.nodeClient.NodePublishVolume(context.Background(), nodePublishReq)
	return err
}
