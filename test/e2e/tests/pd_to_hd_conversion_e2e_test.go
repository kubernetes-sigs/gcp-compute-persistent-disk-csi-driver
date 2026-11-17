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
	"context"
	"fmt"
	"os"
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
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GCE PD CSI Driver ControllerModifyVolume", func() {
	It("should modify IOPS and throughput on the same disk type without triggering conversion", func() {
		if !runCMVTests() {
			Skip("Not running ControllerModifyVolume tests, as RUN_CONTROLLER_MODIFY_VOLUME_TESTS is falsy")
		}
		if len(mwTestContexts) == 0 {
			Skip("Not running ControllerModifyVolume test, as no multiwriter test contexts are configured")
		}

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
		volume, err := client.CreateVolume(volName, params, defaultHdBSizeGb, &csi.TopologyRequirement{
			Requisite: []*csi.Topology{
				{Segments: map[string]string{constants.TopologyKeyZone: z}},
			},
		}, nil)
		Expect(err).To(BeNil(), "CreateVolume failed: %v", err)

		defer func() {
			err := client.DeleteVolume(volume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume failed: %v", err)
			project, key, err := common.VolumeIDToKey(volume.VolumeId)
			Expect(err).To(BeNil(), "Failed to parse volume ID: %v", err)
			_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to be deleted")
		}()

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

	It("should reject out-of-range IOPS and throughput on Hyperdisk Balanced", func() {
		if !runCMVTests() {
			Skip("Not running ControllerModifyVolume tests, as RUN_CONTROLLER_MODIFY_VOLUME_TESTS is falsy")
		}
		if len(mwTestContexts) == 0 {
			Skip("Not running ControllerModifyVolume test, as no multiwriter test contexts are configured")
		}

		testContext := getRandomMwTestContext()
		project, zone, _ := testContext.Instance.GetIdentity()
		testCases := []struct {
			name       string
			parameters map[string]string
		}{
			{
				name:       "IOPS above the supported maximum",
				parameters: map[string]string{"type": hdbDiskType, "iops": "100001"},
			},
			{
				name:       "throughput above the supported maximum",
				parameters: map[string]string{"type": hdbDiskType, "throughput": "100001Mi"},
			},
		}

		for _, testCase := range testCases {
			By(testCase.name)
			volName := testNamePrefix + string(uuid.NewUUID())
			volume, err := testContext.Client.CreateVolume(volName, map[string]string{
				parameters.ParameterKeyType:                          hdbDiskType,
				parameters.ParameterKeyProvisionedIOPSOnCreate:       provisionedIOPSOnCreateHdb,
				parameters.ParameterKeyProvisionedThroughputOnCreate: provisionedThroughputOnCreateHdb,
			}, defaultHdBSizeGb, &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{constants.TopologyKeyZone: zone}},
				},
			}, nil)
			Expect(err).To(BeNil(), "CreateVolume failed: %v", err)
			if err != nil {
				continue
			}

			func() {
				defer func() {
					Expect(testContext.Client.DeleteVolume(volume.VolumeId)).To(Succeed())
					_, key, err := common.VolumeIDToKey(volume.VolumeId)
					Expect(err).To(BeNil(), "Failed to parse volume ID: %v", err)
					_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
					Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to be deleted")
				}()

				disk, err := computeService.Disks.Get(project, zone, volName).Do()
				Expect(err).To(BeNil(), "Could not get disk before modification: %v", err)
				if err != nil {
					return
				}
				Expect(disk.Type).To(ContainSubstring(hdbDiskType))
				initialIOPS := disk.ProvisionedIops
				initialThroughput := disk.ProvisionedThroughput
				initialType := disk.Type

				err = testContext.Client.ControllerModifyVolume(volume.VolumeId, testCase.parameters)
				Expect(err).NotTo(BeNil(), "Expected GCE to reject out-of-range performance parameters")
				if err == nil {
					return
				}
				Expect(status.Code(err)).To(Equal(codes.InvalidArgument),
					"Expected InvalidArgument for out-of-range parameters, got: %v", err)

				disk, err = computeService.Disks.Get(project, zone, volName).Do()
				Expect(err).To(BeNil(), "Could not get disk after rejected modification: %v", err)
				if err != nil {
					return
				}
				Expect(disk.Type).To(Equal(initialType), "Disk type changed after rejected modification")
				Expect(disk.ProvisionedIops).To(Equal(initialIOPS), "IOPS changed after rejected modification")
				Expect(disk.ProvisionedThroughput).To(Equal(initialThroughput), "Throughput changed after rejected modification")
			}()
		}
	})
})

func requireConversionSuccess(err error, message string) {
	if err != nil {
		GinkgoT().Fatalf("%s: %v", message, err)
	}
}

var _ = Describe("PD to Hyperdisk disk type conversion", Ordered, func() {
	var (
		ctx           context.Context
		cancel        context.CancelFunc
		env           *testutils.DiskConversionEnvironment
		restoreDriver func(context.Context) error
	)

	BeforeAll(func() {
		if os.Getenv("RUN_PD_TO_HD_CONVERSION_E2E") != "true" {
			Skip("set RUN_PD_TO_HD_CONVERSION_E2E=true to run against an existing Kubernetes cluster")
		}
		ctx, cancel, env, restoreDriver = testutils.SetupDiskConversionE2E(GinkgoT())
	})

	AfterAll(func() {
		if cancel != nil {
			cancel()
		}
		if restoreDriver == nil {
			return
		}
		restoreCtx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		Expect(restoreDriver(restoreCtx)).To(Succeed(), "restore controller deployment")
	})

	It("converts a detached disk", func() {
		fixture := testutils.NewDiskConversionFixture(GinkgoT(), env)
		fixture.RequestConversion(GinkgoT(), ctx, 3000, 150)
		requireConversionSuccess(fixture.WaitForDiskType(ctx, "hyperdisk-balanced", 2*time.Hour), "wait for disk conversion")
		requireConversionSuccess(fixture.WaitForConversionComplete(ctx, "hyperdisk-balanced", 10*time.Minute), "wait for conversion completion")
	})

	It("queues conversion until the workload detaches", func() {
		fixture := testutils.NewDiskConversionFixture(GinkgoT(), env)
		fixture.StartPod(GinkgoT(), ctx)
		fixture.RequestConversion(GinkgoT(), ctx, 3000, 150)
		requireConversionSuccess(fixture.WaitForPVAnnotation(ctx, "pdcsi.gke.io/disk-type-conversion-operation", "Pending", 10*time.Minute),
			"conversion should be queued while the disk is attached")
		fixture.StopPod(GinkgoT(), ctx)
		requireConversionSuccess(fixture.WaitForDiskType(ctx, "hyperdisk-balanced", 2*time.Hour), "wait for disk conversion")
		requireConversionSuccess(fixture.WaitForConversionComplete(ctx, "hyperdisk-balanced", 10*time.Minute), "wait for conversion completion")
	})

	It("blocks attachment during conversion", func() {
		fixture := testutils.NewDiskConversionFixture(GinkgoT(), env)
		fixture.RequestConversion(GinkgoT(), ctx, 3000, 150)
		operation, err := fixture.WaitForActiveOperation(ctx, 10*time.Minute)
		if err != nil {
			GinkgoT().Fatalf("wait for active conversion operation: %v", err)
		}
		fixture.CreatePod(GinkgoT(), ctx)
		requireConversionSuccess(fixture.WaitForPodBlocked(ctx, operation, 2*time.Minute), "wait for attachment to be blocked")
		requireConversionSuccess(fixture.WaitForPodAndDisk(ctx, "hyperdisk-balanced", 2*time.Hour),
			fmt.Sprintf("disk should become attachable after conversion operation %s", operation))
		requireConversionSuccess(fixture.WaitForConversionComplete(ctx, "hyperdisk-balanced", 10*time.Minute), "wait for conversion completion")
	})

	It("resumes conversion after controller restart", func() {
		fixture := testutils.NewDiskConversionFixture(GinkgoT(), env)
		fixture.RequestConversion(GinkgoT(), ctx, 3000, 150)
		operation, err := fixture.WaitForActiveOperation(ctx, 10*time.Minute)
		if err != nil {
			GinkgoT().Fatalf("wait for active conversion operation: %v", err)
		}
		requireConversionSuccess(testutils.RestartDiskConversionController(ctx, env),
			fmt.Sprintf("restart controller while conversion %s is active", operation))
		requireConversionSuccess(fixture.VerifyConversionIntent(ctx, operation), "verify conversion intent after controller restart")
		requireConversionSuccess(fixture.WaitForDiskType(ctx, "hyperdisk-balanced", 2*time.Hour), "wait for disk conversion")
		requireConversionSuccess(fixture.WaitForConversionComplete(ctx, "hyperdisk-balanced", 10*time.Minute), "wait for conversion completion")
	})

	It("retries conversion after an instant snapshot is removed", func() {
		fixture := testutils.NewDiskConversionFixture(GinkgoT(), env)
		requireConversionSuccess(fixture.CreateInstantSnapshot(ctx), "create instant snapshot")
		fixture.RequestConversion(GinkgoT(), ctx, 3000, 150)
		requireConversionSuccess(fixture.WaitForSnapshotRetry(ctx, 15*time.Minute),
			"conversion should report a retriable snapshot conflict")
		fixture.DeleteInstantSnapshot(GinkgoT(), ctx)
		requireConversionSuccess(fixture.WaitForDiskType(ctx, "hyperdisk-balanced", 2*time.Hour), "wait for disk conversion")
		requireConversionSuccess(fixture.WaitForConversionComplete(ctx, "hyperdisk-balanced", 10*time.Minute), "wait for conversion completion")
	})

	It("cancels a queued conversion when the VAC is cleared", func() {
		fixture := testutils.NewDiskConversionFixture(GinkgoT(), env)
		fixture.StartPod(GinkgoT(), ctx)
		fixture.RequestConversion(GinkgoT(), ctx, 3000, 150)
		requireConversionSuccess(fixture.WaitForPVAnnotation(ctx, "pdcsi.gke.io/disk-type-conversion-operation", "Pending", 10*time.Minute),
			"conversion should be queued while the disk is attached")
		fixture.ClearVolumeAttributesClass(GinkgoT(), ctx)
		requireConversionSuccess(fixture.WaitForConversionCancelled(ctx, 5*time.Minute),
			"pending conversion should be cancelled after clearing the PVC VAC")
		fixture.StopPod(GinkgoT(), ctx)
		requireConversionSuccess(fixture.WaitForDiskType(ctx, "pd-balanced", 2*time.Minute),
			"disk type should remain unchanged after the queued conversion is cancelled")
	})
})
