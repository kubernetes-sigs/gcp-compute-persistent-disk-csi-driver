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
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"
	remote "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote"
)

const (
	noMachineType = "none"
)

var (
	project                   = flag.String("project", "", "Project to run tests in")
	serviceAccount            = flag.String("service-account", "", "Service account to bring up instance with")
	vmNamePrefix              = flag.String("vm-name-prefix", "gce-pd-csi-e2e", "VM name prefix")
	architecture              = flag.String("arch", "amd64", "Architecture pd csi driver build on")
	minCpuPlatform            = flag.String("min-cpu-platform", "AMD Rome", "Minimum CPU architecture")
	mwMinCpuPlatform          = flag.String("min-cpu-platform-mw", "Intel Sapphire Rapids", "Minimum CPU architecture for multiwriter tests")
	zonesFlag                 = flag.String("zones", "us-east4-a,us-east4-c", "Zones to run tests in. If there are multiple zones, separate each by comma")
	subnetwork                = flag.String("subnetwork", "", "Subnetwork to use. Must already exist in the region of a zone used for an instance. Ignored if empty")
	machineType               = flag.String("machine-type", "n2d-standard-4", "Type of machine to provision instance on")
	instancesPerZone          = flag.Int("instances-per-zone", 3, "Number of instances per zone that will be provisioned")
	diskTypeDefault           = flag.String("disk-type-default", "pd-balanced", "Default disk type to use for --machine-type if not specified by the test.")
	supportedDiskTypesFlag    = flag.String("supported-disk-types", "pd-standard,pd-balanced,pd-extreme,pd-ssd", "Supported disk types for --machine-type (comma-separated)")
	imageURL                  = flag.String("image-url", "projects/ubuntu-os-cloud/global/images/family/ubuntu-minimal-2404-lts-amd64", "OS image url to get image from. The default is ubuntu, a COS example is projects/cos-cloud/global/images/cos-stable-121-18867-584-23 (from gcloud compute images describe-from-family cos-stable --project cos-cloud --format 'value(selfLink)')")
	runInProw                 = flag.Bool("run-in-prow", false, "If true, use a Boskos loaned project and special CI service accounts and ssh keys")
	deleteInstances           = flag.Bool("delete-instances", false, "Delete the instances after tests run")
	cloudtopHost              = flag.Bool("cloudtop-host", false, "The local host is cloudtop, a kind of googler machine with special requirements to access GCP")
	extraDriverFlags          = flag.String("extra-driver-flags", "", "Extra flags to pass to the driver")
	enableConfidentialCompute = flag.Bool("enable-confidential-compute", false, "Create VMs with confidential compute mode. This uses NVMe devices")
	localSsdCount             = flag.Int("local-ssd-count", 2, "The number of local ssds to create. This can determine what tests are run. It is ignored if the machine type determines the number of ssds")

	// Multi-writer is only supported on M3, C3, and N4
	// https://cloud.google.com/compute/docs/disks/sharing-disks-between-vms#hd-multi-writer
	// This is also used as a proxy for hyperdisk compatible machine types (gen3+).
	mwMachineType = flag.String("machine-type-mw", "c3-standard-4", "Type of multiwriter machine to provision instance on, or `none' to skip. These will not be configured with local ssd")

	testContexts        []*remote.TestContext
	mwTestContexts      []*remote.TestContext
	computeService      *compute.Service
	computeAlphaService *computealpha.Service
	computeBetaService  *computebeta.Service
	kmsClient           *cloudkms.KeyManagementClient

	zones              []string
	supportedDiskTypes []string
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
	numberOfInstancesPerZone := *instancesPerZone
	if numberOfInstancesPerZone < 3 {
		klog.Infof("Using %d instances, but some tests require 3. Be careful!", numberOfInstancesPerZone)
	}
	zones = strings.Split(*zonesFlag, ",")
	supportedDiskTypes = strings.Split(*supportedDiskTypesFlag, ",")

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

	testContexts = make([]*remote.TestContext, numberOfInstancesPerZone*len(zones))
	if *mwMachineType != noMachineType {
		mwTestContexts = make([]*remote.TestContext, len(zones))
	}
	var wg sync.WaitGroup
	setupContext := func(idx int, zone string) {
		for j := 0; j < numberOfInstancesPerZone; j++ {
			wg.Add(1)
			go func(curZone string, randInt int) {
				defer GinkgoRecover()
				defer wg.Done()
				tc := NewDefaultTestContext(curZone, strconv.Itoa(randInt))
				k := j + idx*numberOfInstancesPerZone
				testContexts[k] = tc
				klog.Infof("Added TestContext for node %s at %d", tc.Instance.GetName(), k)
			}(zone, j)
		}
		if mwTestContexts != nil {
			wg.Add(1)
			go func(curZone string) {
				defer GinkgoRecover()
				defer wg.Done()
				tc := NewTestContext(curZone, *mwMinCpuPlatform, *mwMachineType, 0, "0")
				mwTestContexts[idx] = tc
				klog.Infof("Added hyperdisk TestContext for node %s at %d", tc.Instance.GetName(), idx)
			}(zone)
		}
	}

	for i, zone := range zones {
		setupContext(i, zone)
	}
	wg.Wait()
})

var _ = AfterSuite(func() {
	for _, tc := range testContexts {
		err := remote.TeardownDriverAndClient(tc)
		Expect(err).To(BeNil(), "Teardown Driver and Client failed with error")
		if *deleteInstances {
			tc.Instance.DeleteInstance()
		}
	}
	for _, mwTc := range mwTestContexts {
		err := remote.TeardownDriverAndClient(mwTc)
		Expect(err).To(BeNil(), "Multiwriter Teardown Driver and Client failed with error")
		if *deleteInstances {
			mwTc.Instance.DeleteInstance()
		}
	}
})

func notEmpty(v string) bool {
	return v != ""
}

func getDriverConfig() testutils.DriverConfig {
	return testutils.DriverConfig{
		ExtraFlags: slices.Filter(nil, strings.Split(*extraDriverFlags, ","), notEmpty),
		Zones:      zones,
	}
}

func NewDefaultTestContext(zone string, instanceNumber string) *remote.TestContext {
	return NewTestContext(zone, *minCpuPlatform, *machineType, int64(*localSsdCount), instanceNumber)
}

func NewTestContext(zone, minCpuPlatform, machineType string, localSsdCount int64, instanceNumber string) *remote.TestContext {
	nodeID := fmt.Sprintf("%s-%s-%s-%s", *vmNamePrefix, zone, machineType, instanceNumber)
	klog.Infof("Setting up node %s", nodeID)

	// Avoid local SSD on e2 & e4 instance types. Technically the user should pass in the correct --local-ssd-count
	// flag, but this isn't always done for image qualification suites.
	if strings.HasPrefix(machineType, "e2-") || strings.HasPrefix(machineType, "e4-") {
		localSsdCount = 0
	}
	instanceConfig := remote.InstanceConfig{
		Project:                   *project,
		Architecture:              *architecture,
		MinCpuPlatform:            minCpuPlatform,
		Zone:                      zone,
		Name:                      nodeID,
		MachineType:               machineType,
		DiskTypeDefault:           *diskTypeDefault,
		SupportedDiskTypes:        supportedDiskTypes,
		ServiceAccount:            *serviceAccount,
		ImageURL:                  *imageURL,
		CloudtopHost:              *cloudtopHost,
		EnableConfidentialCompute: *enableConfidentialCompute,
		ComputeService:            computeService,
		LocalSSDCount:             localSsdCount,
		Subnetwork:                *subnetwork,
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
	tc.TestZones = zones

	// This must be done after driver setup as we rely on driver scripts being installed.
	if err = testutils.CleanUpAnyExistingLVMInstances(i); err != nil {
		klog.Fatalf("failed to clean up existing LVM instances on %v: %v", i.GetName(), err)
	}

	klog.Infof("Finished creating TestContext for node %s (%s/%d)", tc.Instance.GetName(), tc.Instance.MachineType(), tc.Instance.GetLocalSSDCount())
	return tc
}

func getRandomTestContext() *remote.TestContext {
	Expect(testContexts).ToNot(BeEmpty())
	rn := rand.Intn(len(testContexts))
	return testContexts[rn]
}

func getRandomMwTestContext() *remote.TestContext {
	Expect(mwTestContexts).ToNot(BeEmpty())
	rn := rand.Intn(len(mwTestContexts))
	return mwTestContexts[rn]
}
