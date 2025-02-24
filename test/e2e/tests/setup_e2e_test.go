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
	"flag"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	cloudkms "cloud.google.com/go/kms/apiv1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	computealpha "google.golang.org/api/compute/v0.alpha"
	computebeta "google.golang.org/api/compute/v0.beta"
	compute "google.golang.org/api/compute/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"
	remote "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote"
)

var (
	project                   = flag.String("project", "", "Project to run tests in")
	serviceAccount            = flag.String("service-account", "", "Service account to bring up instance with")
	vmNamePrefix              = flag.String("vm-name-prefix", "gce-pd-csi-e2e", "VM name prefix")
	architecture              = flag.String("arch", "amd64", "Architecture pd csi driver build on")
	minCpuPlatform            = flag.String("min-cpu-platform", "rome", "Minimum CPU architecture")
	zones                     = flag.String("zones", "us-east4-a,us-east4-c", "Zones to run tests in. If there are multiple zones, separate each by comma")
	machineType               = flag.String("machine-type", "n2-standard-2", "Type of machine to provision instance on")
	imageURL                  = flag.String("image-url", "projects/debian-cloud/global/images/family/debian-11", "OS image url to get image from")
	runInProw                 = flag.Bool("run-in-prow", false, "If true, use a Boskos loaned project and special CI service accounts and ssh keys")
	deleteInstances           = flag.Bool("delete-instances", false, "Delete the instances after tests run")
	cloudtopHost              = flag.Bool("cloudtop-host", false, "The local host is cloudtop, a kind of googler machine with special requirements to access GCP")
	extraDriverFlags          = flag.String("extra-driver-flags", "", "Extra flags to pass to the driver")
	enableConfidentialCompute = flag.Bool("enable-confidential-compute", false, "Create VMs with confidential compute mode. This uses NVMe devices")
	// Multi-writer is only supported on M3, C3, and N4
	// https://cloud.google.com/compute/docs/disks/sharing-disks-between-vms#hd-multi-writer
	hdMachineType = flag.String("hyperdisk-machine-type", "c3-standard-4", "Type of machine to provision instance on")

	testContexts        = []*remote.TestContext{}
	computeService      *compute.Service
	computeAlphaService *computealpha.Service
	computeBetaService  *computebeta.Service
	kmsClient           *cloudkms.KeyManagementClient
)

func init() {
	klog.InitFlags(flag.CommandLine)
}

func TestE2E(t *testing.T) {
	flag.Parse()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Google Compute Engine Persistent Disk Container Storage Interface Driver Tests")
}

var _ = BeforeSuite(func() {
	var err error
	numberOfInstancesPerZone := 2
	zones := strings.Split(*zones, ",")
	tcc := make(chan *remote.TestContext, len(zones)*numberOfInstancesPerZone)
	defer close(tcc)

	rand.Seed(time.Now().UnixNano())

	computeService, err = remote.GetComputeClient()
	Expect(err).To(BeNil())

	computeAlphaService, err = remote.GetComputeAlphaClient()
	Expect(err).To(BeNil())

	computeBetaService, err = remote.GetComputeBetaClient()
	Expect(err).To(BeNil())

	// Create the KMS client.
	kmsClient, err = cloudkms.NewKeyManagementClient(context.Background())
	Expect(err).To(BeNil())

	if *runInProw {
		*project, *serviceAccount = testutils.SetupProwConfig("gce-project")
	}

	Expect(*project).ToNot(BeEmpty(), "Project should not be empty")
	Expect(*serviceAccount).ToNot(BeEmpty(), "Service account should not be empty")

	klog.Infof("Running in project %v with service account %v", *project, *serviceAccount)

	setupContext := func(zone string) {
		var wg sync.WaitGroup
		// Create 2 instances for each zone as we need 2 instances each zone for certain test cases
		for j := 0; j < numberOfInstancesPerZone; j++ {
			wg.Add(1)
			go func(curZone string, randInt int) {
				defer GinkgoRecover()
				defer wg.Done()
				tcc <- NewDefaultTestContext(curZone, strconv.Itoa(randInt))
			}(zone, j)
		}
	}

	for _, zone := range zones {
		setupContext(zone)
	}

	for i := 0; i < len(zones)*numberOfInstancesPerZone; i++ {
		tc := <-tcc
		testContexts = append(testContexts, tc)
		klog.Infof("Added TestContext for node %s", tc.Instance.GetName())
	}
})

var _ = AfterSuite(func() {
	for _, tc := range testContexts {
		err := remote.TeardownDriverAndClient(tc)
		Expect(err).To(BeNil(), "Teardown Driver and Client failed with error")
		if *deleteInstances {
			tc.Instance.DeleteInstance()
		}
	}
})

func notEmpty(v string) bool {
	return v != ""
}

func getDriverConfig() testutils.DriverConfig {
	return testutils.DriverConfig{
		ExtraFlags: slices.Filter(nil, strings.Split(*extraDriverFlags, ","), notEmpty),
		Zones:      strings.Split(*zones, ","),
	}
}

func NewDefaultTestContext(zone string, instanceNumber string) *remote.TestContext {
	return NewTestContext(zone, *minCpuPlatform, *machineType, instanceNumber)
}

func NewTestContext(zone, minCpuPlatform, machineType string, instanceNumber string) *remote.TestContext {
	nodeID := fmt.Sprintf("%s-%s-%s-%s", *vmNamePrefix, zone, machineType, instanceNumber)
	klog.Infof("Setting up node %s", nodeID)

	instanceConfig := remote.InstanceConfig{
		Project:                   *project,
		Architecture:              *architecture,
		MinCpuPlatform:            minCpuPlatform,
		Zone:                      zone,
		Name:                      nodeID,
		MachineType:               machineType,
		ServiceAccount:            *serviceAccount,
		ImageURL:                  *imageURL,
		CloudtopHost:              *cloudtopHost,
		EnableConfidentialCompute: *enableConfidentialCompute,
		ComputeService:            computeService,
		LocalSSDCount:             common.LocalSSDCountForDataCache,
	}

	if machineType == *hdMachineType {
		// Machine type is defaulted to c3-standard-2 which doesn't support LSSD and we don't need LSSD for HdHA test context
		instanceConfig.LocalSSDCount = 0
	}
	i, err := remote.SetupInstance(instanceConfig)
	if err != nil {
		klog.Fatalf("Failed to setup instance %v: %v", nodeID, err)
	}

	err = testutils.MkdirAll(i, "/lib/udev_containerized")
	if err != nil {
		klog.Fatalf("Failed to make scsi_id containerized directory: %v", err)
	}

	err = testutils.CopyFile(i, "/lib/udev/scsi_id", "/lib/udev_containerized/scsi_id")
	if err != nil {
		klog.Fatalf("Failed to copy scsi_id to containerized directory: %v", err)
	}

	err = testutils.CopyFile(i, "/lib/udev/google_nvme_id", "/lib/udev_containerized/google_nvme_id")
	if err != nil {
		klog.Fatalf("Failed to copy google_nvme_id to containerized directory: %v", err)
	}
	pkgs := []string{"lvm2", "mdadm", "grep", "coreutils"}
	err = testutils.InstallDependencies(i, pkgs)
	if err != nil {
		klog.Fatalf("Failed to install dependency package on node %v: error : %v", i.GetNodeID(), err)
	}

	err = testutils.SetupDataCachingConfig(i)
	if err != nil {
		klog.Fatalf("Failed to setup data cache required config error %v", err)
	}
	klog.Infof("Creating new driver and client for node %s", i.GetName())
	tc, err := testutils.GCEClientAndDriverSetup(i, getDriverConfig())
	if err != nil {
		klog.Fatalf("Failed to set up TestContext for instance %v: %v", i.GetName(), err)
	}

	klog.Infof("Finished creating TestContext for node %s", tc.Instance.GetName())
	return tc
}

func getRandomTestContext() *remote.TestContext {
	Expect(testContexts).ToNot(BeEmpty())
	rn := rand.Intn(len(testContexts))
	return testContexts[rn]
}
