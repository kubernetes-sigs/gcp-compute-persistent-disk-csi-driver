/*
Copyright 2026 The Kubernetes Authors.

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
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/parameters"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	compute "google.golang.org/api/compute/v1"
)

// These tests cover how the driver handles a disk type in a
// VolumeAttributesClass. They run against real disks, but the driver runs
// outside a cluster, so it has no PersistentVolume to record a conversion's
// progress on. A conversion itself can therefore not run here; these tests
// cover what the driver decides before one would start, and how CreateVolume
// applies the class's type.
var _ = Describe("GCE PD CSI Driver disk type in VolumeAttributesClass", func() {
	It("should modify IOPS and throughput on the same disk type without triggering conversion", func() {
		skipUnlessDiskTypeCMVTestsCanRun()

		testContext := getRandomMwTestContext()
		client := testContext.Client
		p, z, _ := testContext.Instance.GetIdentity()
		volName := testNamePrefix + string(uuid.NewUUID())

		initialIOPS := provisionedIOPSOnCreateHdbInt
		initialThroughput := provisionedThroughputOnCreateHdbInt
		updatedIOPS := int64(3013)
		updatedThroughput := int64(181)

		params := map[string]string{
			parameters.ParameterKeyType:                          hdbDiskType,
			parameters.ParameterKeyProvisionedIOPSOnCreate:       provisionedIOPSOnCreateHdb,
			parameters.ParameterKeyProvisionedThroughputOnCreate: provisionedThroughputOnCreateHdb,
		}
		volume, err := client.CreateVolume(volName, params, defaultHdBSizeGb, zoneTopology(z), nil)
		Expect(err).To(BeNil(), "CreateVolume failed: %v", err)

		defer deleteVolumeAndVerify(client, volume.VolumeId)

		disk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk before modification: %v", err)
		Expect(disk.Status).To(Equal(readyState))
		Expect(disk.Type).To(ContainSubstring(hdbDiskType))
		Expect(disk.ProvisionedIops).To(Equal(initialIOPS))
		Expect(disk.ProvisionedThroughput).To(Equal(initialThroughput))
		initialDiskType := disk.Type

		err = client.ControllerModifyVolume(volume.VolumeId, map[string]string{
			"type":       hdbDiskType,
			"iops":       "3013",
			"throughput": "181Mi",
		})
		Expect(err).To(BeNil(), "ControllerModifyVolume should update performance without starting a conversion: %v", err)

		err = wait.Poll(10*time.Second, 5*time.Minute, func() (bool, error) {
			disk, err := computeService.Disks.Get(p, z, volName).Do()
			if err != nil {
				return false, err
			}
			return disk.ProvisionedIops == updatedIOPS && disk.ProvisionedThroughput == updatedThroughput, nil
		})
		Expect(err).To(BeNil(), "Timed out waiting for IOPS and throughput to update: %v", err)

		disk, err = computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk after modification: %v", err)
		Expect(disk.Type).To(Equal(initialDiskType), "Disk type changed during performance modification")
		Expect(disk.ProvisionedIops).To(Equal(updatedIOPS))
		Expect(disk.ProvisionedThroughput).To(Equal(updatedThroughput))

		klog.Infof("Updated IOPS and throughput on %s without changing disk type %s", volName, disk.Type)
	})

	// The type is compared after it is trimmed and lowercased, so a class
	// that spells the disk's own type differently does not start a
	// conversion, and a class that changes nothing is reported as applied.
	DescribeTable("should treat a type that matches the disk as already applied",
		func(requestedType string) {
			skipUnlessDiskTypeCMVTestsCanRun()

			testContext := getRandomMwTestContext()
			client := testContext.Client
			p, z, _ := testContext.Instance.GetIdentity()

			volName, volID := createAndValidateUniqueZonalDisk(client, p, z, hdbDiskType)
			defer deleteVolumeAndVerify(client, volID)
			before, err := computeService.Disks.Get(p, z, volName).Do()
			Expect(err).To(BeNil(), "Could not get disk before modification: %v", err)

			err = client.ControllerModifyVolume(volID, map[string]string{"type": requestedType})
			Expect(err).To(BeNil(), "ControllerModifyVolume with the disk's own type should succeed: %v", err)

			expectDiskUnchanged(p, z, volName, before)
		},
		Entry("with the type as the disk reports it", hdbDiskType),
		Entry("with the type in another case and padded with spaces", " Hyperdisk-Balanced "),
	)

	// With --enable-disk-type-conversion off, the default, a class that asks
	// for another type is rejected outright, so the external-resizer marks it
	// Infeasible. No part of the request is applied: IOPS or throughput sent
	// with the type must not be applied to the disk on their own.
	DescribeTable("should reject a disk type change while disk type conversion is disabled",
		func(sourceType string, mutableParams map[string]string) {
			skipUnlessDiskTypeCMVTestsCanRun()

			testContext := getRandomMwTestContext()
			checkSkipDiskType(testContext, sourceType)
			client := testContext.Client
			p, z, _ := testContext.Instance.GetIdentity()

			volName, volID := createAndValidateUniqueZonalDisk(client, p, z, sourceType)
			defer deleteVolumeAndVerify(client, volID)
			before, err := computeService.Disks.Get(p, z, volName).Do()
			Expect(err).To(BeNil(), "Could not get disk before modification: %v", err)

			err = client.ControllerModifyVolume(volID, mutableParams)
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument), "Expected InvalidArgument, got: %v", err)
			Expect(err.Error()).To(ContainSubstring("disk type conversion is not enabled"))

			expectDiskUnchanged(p, z, volName, before)
		},
		Entry("from pd-balanced to hyperdisk-balanced", balancedDiskType,
			map[string]string{"type": hdbDiskType}),
		Entry("from pd-balanced to hyperdisk-balanced with IOPS and throughput", balancedDiskType,
			map[string]string{"type": hdbDiskType, "iops": "3000", "throughput": "150Mi"}),
		Entry("from hyperdisk-balanced to pd-balanced", hdbDiskType,
			map[string]string{"type": balancedDiskType}),
	)

	DescribeTable("should reject invalid mutable parameters without changing the disk",
		func(mutableParams map[string]string) {
			skipUnlessDiskTypeCMVTestsCanRun()

			testContext := getRandomMwTestContext()
			client := testContext.Client
			p, z, _ := testContext.Instance.GetIdentity()

			volName, volID := createAndValidateUniqueZonalDisk(client, p, z, hdbDiskType)
			defer deleteVolumeAndVerify(client, volID)
			before, err := computeService.Disks.Get(p, z, volName).Do()
			Expect(err).To(BeNil(), "Could not get disk before modification: %v", err)

			err = client.ControllerModifyVolume(volID, mutableParams)
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument), "Expected InvalidArgument, got: %v", err)

			expectDiskUnchanged(p, z, volName, before)
		},
		Entry("with an empty type", map[string]string{"type": "  "}),
		Entry("with zero IOPS", map[string]string{"type": hdbDiskType, "iops": "0"}),
		Entry("with zero throughput", map[string]string{"type": hdbDiskType, "throughput": "0Mi"}),
		Entry("with an IOPS value that is not a number", map[string]string{"iops": "fast"}),
		Entry("with an unknown parameter", map[string]string{"type": hdbDiskType, "tier": "gold"}),
	)

	// The CSI spec has mutable_parameters take precedence over the
	// StorageClass parameters, so a claim created with a class that names a
	// type gets a disk of that type rather than one that would immediately
	// need converting, and IOPS and throughput are checked against that type.
	DescribeTable("should create the disk with the VolumeAttributesClass type",
		func(storageClassType string, mutableParams map[string]string, wantIOPS, wantThroughput int64) {
			skipUnlessDiskTypeCMVTestsCanRun()

			testContext := getRandomMwTestContext()
			client := testContext.Client
			p, z, _ := testContext.Instance.GetIdentity()
			volName := testNamePrefix + string(uuid.NewUUID())

			volume, err := client.CreateVolumeWithMutableParameters(volName,
				map[string]string{parameters.ParameterKeyType: storageClassType},
				mutableParams, defaultHdBSizeGb, zoneTopology(z))
			Expect(err).To(BeNil(), "CreateVolume failed: %v", err)
			defer deleteVolumeAndVerify(client, volume.VolumeId)

			disk, err := computeService.Disks.Get(p, z, volName).Do()
			Expect(err).To(BeNil(), "Could not get disk: %v", err)
			Expect(disk.Status).To(Equal(readyState))
			Expect(gce.CloudDiskFromV1(disk).GetPDType()).To(Equal(mutableParams["type"]),
				"Disk should have the VolumeAttributesClass type, not the StorageClass type %s", storageClassType)
			if wantIOPS != 0 {
				Expect(disk.ProvisionedIops).To(Equal(wantIOPS))
			}
			if wantThroughput != 0 {
				Expect(disk.ProvisionedThroughput).To(Equal(wantThroughput))
			}
		},
		Entry("from a pd-balanced StorageClass", balancedDiskType,
			map[string]string{"type": hdbDiskType}, int64(0), int64(0)),
		Entry("from a pd-balanced StorageClass, with IOPS and throughput only Hyperdisk can take", balancedDiskType,
			map[string]string{"type": hdbDiskType, "iops": "4000", "throughput": "200Mi"}, int64(4000), int64(200)),
		Entry("from a hyperdisk-balanced StorageClass", hdbDiskType,
			map[string]string{"type": balancedDiskType}, int64(0), int64(0)),
	)

	It("should reject IOPS that the VolumeAttributesClass type cannot take at create time", func() {
		skipUnlessDiskTypeCMVTestsCanRun()

		testContext := getRandomMwTestContext()
		client := testContext.Client
		p, z, _ := testContext.Instance.GetIdentity()
		volName := testNamePrefix + string(uuid.NewUUID())

		// The StorageClass type could take the IOPS, but the class's type,
		// which takes precedence, cannot.
		_, err := client.CreateVolumeWithMutableParameters(volName,
			map[string]string{parameters.ParameterKeyType: hdbDiskType},
			map[string]string{"type": balancedDiskType, "iops": "4000"},
			defaultHdBSizeGb, zoneTopology(z))
		Expect(status.Code(err)).To(Equal(codes.InvalidArgument), "Expected InvalidArgument, got: %v", err)

		_, err = computeService.Disks.Get(p, z, volName).Do()
		Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "No disk should have been created, got: %v", err)
	})
})

// skipUnlessDiskTypeCMVTestsCanRun skips a test that needs ControllerModifyVolume
// and Hyperdisk, or that needs disk type conversion to be off.
//
// With --enable-disk-type-conversion on, every ControllerModifyVolume first
// looks up the volume's PersistentVolume, which a driver running outside a
// cluster cannot do, so none of these tests can pass.
func skipUnlessDiskTypeCMVTestsCanRun() {
	if !runCMVTests() {
		Skip("Not running ControllerModifyVolume tests, as RUN_CONTROLLER_MODIFY_VOLUME_TESTS is falsy")
	}
	if len(mwTestContexts) == 0 {
		Skip("Not running disk type tests, as no Hyperdisk capable (multiwriter) test contexts are configured")
	}
	if diskTypeConversionEnabled() {
		Skip("Not running disk type tests, as the driver runs with --enable-disk-type-conversion and has no cluster to record conversions in")
	}
}

func diskTypeConversionEnabled() bool {
	for _, flag := range strings.Split(*extraDriverFlags, ",") {
		flag = strings.TrimLeft(strings.TrimSpace(flag), "-")
		if flag == "enable-disk-type-conversion" || flag == "enable-disk-type-conversion=true" {
			return true
		}
	}
	return false
}

func zoneTopology(zone string) *csi.TopologyRequirement {
	return &csi.TopologyRequirement{
		Requisite: []*csi.Topology{
			{Segments: map[string]string{constants.TopologyKeyZone: zone}},
		},
	}
}

// expectDiskUnchanged checks that the disk still has the type and provisioned
// performance it had before a rejected request.
func expectDiskUnchanged(project, zone, volName string, before *compute.Disk) {
	after, err := computeService.Disks.Get(project, zone, volName).Do()
	Expect(err).To(BeNil(), "Could not get disk after modification: %v", err)
	Expect(after.Status).To(Equal(readyState))
	Expect(after.Type).To(Equal(before.Type), "Disk type changed")
	Expect(after.ProvisionedIops).To(Equal(before.ProvisionedIops), "Provisioned IOPS changed")
	Expect(after.ProvisionedThroughput).To(Equal(before.ProvisionedThroughput), "Provisioned throughput changed")
}

func deleteVolumeAndVerify(client *remote.CsiClient, volID string) {
	err := client.DeleteVolume(volID)
	Expect(err).To(BeNil(), "DeleteVolume failed: %v", err)
	project, key, err := common.VolumeIDToKey(volID)
	Expect(err).To(BeNil(), "Failed to parse volume ID: %v", err)
	_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
	Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to be deleted")
}
