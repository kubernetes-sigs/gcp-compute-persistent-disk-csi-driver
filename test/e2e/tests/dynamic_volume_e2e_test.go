/*
Copyright 2024 The Kubernetes Authors.

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
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/parameters"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GCE PD CSI Driver Dynamic Volumes HD Node Provisioning", func() {

	It("Should provision hyperdisk-balanced on HD-capable node when type=dynamic", func() {
		testContext := getRandomMwTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		volName := testNamePrefix + string(uuid.NewUUID())

		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-balanced",
			parameters.ParameterPDType:  "pd-balanced",
		}

		volume, err := client.CreateVolume(volName, params, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                        z,
							common.DiskTypeLabelKey("hyperdisk-balanced"): "true",
							common.DiskTypeLabelKey("pd-balanced"):        "true",
						},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                        z,
							common.DiskTypeLabelKey("hyperdisk-balanced"): "true",
							common.DiskTypeLabelKey("pd-balanced"):        "true",
						},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume failed: %v", err)

		defer func() {
			err := client.DeleteVolume(volume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume failed")
			project, key, err := common.VolumeIDToKey(volume.VolumeId)
			Expect(err).To(BeNil(), "Failed to parse volume ID")
			_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to be deleted")
		}()

		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from GCE API")
		Expect(cloudDisk.Status).To(Equal(readyState), "Disk not in READY state")
		Expect(cloudDisk.Name).To(Equal(volName))

		klog.Infof("Dynamic volume resolved to disk type: %s on HD-capable node %s", cloudDisk.Type, z)
		Expect(cloudDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected hyperdisk-balanced on HD-capable c3 node but got: %s", cloudDisk.Type)
	})

})

var _ = Describe("GCE PD CSI Driver Dynamic Volumes PD Fallback Provisioning", func() {

	It("Should fall back to pd-balanced on PD-only node when type=dynamic", func() {
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		volName := testNamePrefix + string(uuid.NewUUID())

		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-balanced",
			parameters.ParameterPDType:  "pd-balanced",
		}

		volume, err := client.CreateVolume(volName, params, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                 z,
							common.DiskTypeLabelKey("pd-balanced"): "true",
							common.DiskTypeLabelKey("pd-standard"): "true",
							common.DiskTypeLabelKey("pd-ssd"):      "true",
							common.DiskTypeLabelKey("pd-extreme"):  "true",
						},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                 z,
							common.DiskTypeLabelKey("pd-balanced"): "true",
							common.DiskTypeLabelKey("pd-standard"): "true",
							common.DiskTypeLabelKey("pd-ssd"):      "true",
							common.DiskTypeLabelKey("pd-extreme"):  "true",
						},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume failed: %v", err)

		defer func() {
			err := client.DeleteVolume(volume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume failed")
			project, key, err := common.VolumeIDToKey(volume.VolumeId)
			Expect(err).To(BeNil(), "Failed to parse volume ID")
			_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to be deleted")
		}()

		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from GCE API")
		Expect(cloudDisk.Status).To(Equal(readyState), "Disk not in READY state")
		Expect(cloudDisk.Name).To(Equal(volName))

		klog.Infof("Dynamic volume resolved to disk type: %s on PD-only node %s", cloudDisk.Type, z)
		Expect(cloudDisk.Type).To(ContainSubstring("pd-balanced"),
			"Expected pd-balanced fallback on PD-only n2d node but got: %s", cloudDisk.Type)
	})

})

var _ = Describe("GCE PD CSI Driver Dynamic Volumes Default Parameters Provisioning", func() {

	It("Should use built-in default disk types when no pd-type or hyperdisk-type is specified", func() {
		hdContext := getRandomMwTestContext()
		pdContext := getRandomTestContext()

		hdProject, hdZone, _ := hdContext.Instance.GetIdentity()
		pdProject, pdZone, _ := pdContext.Instance.GetIdentity()

		// Only type=dynamic — no pd-type or hyperdisk-type specified.
		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
		}

		hdVolName := testNamePrefix + string(uuid.NewUUID())
		hdVolume, err := hdContext.Client.CreateVolume(hdVolName, params, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                          hdZone,
							common.DiskTypeLabelKey("hyperdisk-balanced"):   "true",
							common.DiskTypeLabelKey("hyperdisk-throughput"): "true",
							common.DiskTypeLabelKey("pd-balanced"):          "true",
						},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                          hdZone,
							common.DiskTypeLabelKey("hyperdisk-balanced"):   "true",
							common.DiskTypeLabelKey("hyperdisk-throughput"): "true",
							common.DiskTypeLabelKey("pd-balanced"):          "true",
						},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume (HD) failed: %v", err)
		defer func() {
			err := hdContext.Client.DeleteVolume(hdVolume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume (HD) failed")
			project, key, err := common.VolumeIDToKey(hdVolume.VolumeId)
			Expect(err).To(BeNil())
			_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected HD disk to be deleted")
		}()

		hdDisk, err := computeService.Disks.Get(hdProject, hdZone, hdVolName).Do()
		Expect(err).To(BeNil(), "Could not get HD disk from GCE API")
		Expect(hdDisk.Status).To(Equal(readyState))
		klog.Infof("Default dynamic volume on HD node resolved to: %s", hdDisk.Type)
		Expect(hdDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected default hyperdisk-balanced on HD node but got: %s", hdDisk.Type)

		pdVolName := testNamePrefix + string(uuid.NewUUID())
		pdVolume, err := pdContext.Client.CreateVolume(pdVolName, params, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                 pdZone,
							common.DiskTypeLabelKey("pd-balanced"): "true",
							common.DiskTypeLabelKey("pd-standard"): "true",
							common.DiskTypeLabelKey("pd-ssd"):      "true",
							common.DiskTypeLabelKey("pd-extreme"):  "true",
						},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                 pdZone,
							common.DiskTypeLabelKey("pd-balanced"): "true",
							common.DiskTypeLabelKey("pd-standard"): "true",
							common.DiskTypeLabelKey("pd-ssd"):      "true",
							common.DiskTypeLabelKey("pd-extreme"):  "true",
						},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume (PD) failed: %v", err)
		defer func() {
			err := pdContext.Client.DeleteVolume(pdVolume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume (PD) failed")
			project, key, err := common.VolumeIDToKey(pdVolume.VolumeId)
			Expect(err).To(BeNil())
			_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected PD disk to be deleted")
		}()

		pdDisk, err := computeService.Disks.Get(pdProject, pdZone, pdVolName).Do()
		Expect(err).To(BeNil(), "Could not get PD disk from GCE API")
		Expect(pdDisk.Status).To(Equal(readyState))
		klog.Infof("Default dynamic volume on PD-only node resolved to: %s", pdDisk.Type)
		Expect(pdDisk.Type).To(ContainSubstring("pd-balanced"),
			"Expected default pd-balanced on PD-only node but got: %s", pdDisk.Type)
	})

})

var _ = Describe("GCE PD CSI Driver Dynamic Volumes Lifecycle", func() {

	It("Should create a dynamic volume with correct disk type and delete it with no orphaned resources", func() {
		testContext := getRandomMwTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		volName := testNamePrefix + string(uuid.NewUUID())

		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-balanced",
			parameters.ParameterPDType:  "pd-balanced",
		}

		volume, err := client.CreateVolume(volName, params, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                        z,
							common.DiskTypeLabelKey("hyperdisk-balanced"): "true",
							common.DiskTypeLabelKey("pd-balanced"):        "true",
						},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                        z,
							common.DiskTypeLabelKey("hyperdisk-balanced"): "true",
							common.DiskTypeLabelKey("pd-balanced"):        "true",
						},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume failed: %v", err)

		Expect(volume.VolumeId).NotTo(BeEmpty(), "VolumeId should not be empty")

		project, key, err := common.VolumeIDToKey(volume.VolumeId)
		Expect(err).To(BeNil(), "Failed to parse volume ID: %v", err)

		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "GCE disk not found after CreateVolume: %v", err)
		Expect(cloudDisk.Status).To(Equal(readyState), "Disk not in READY state")
		Expect(cloudDisk.Name).To(Equal(volName), "Disk name mismatch")

		klog.Infof("Dynamic volume created: name=%s type=%s zone=%s", cloudDisk.Name, cloudDisk.Type, z)
		Expect(cloudDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected hyperdisk-balanced but got: %s", cloudDisk.Type)

		err = client.DeleteVolume(volume.VolumeId)
		Expect(err).To(BeNil(), "DeleteVolume failed: %v", err)

		// Verify no orphaned resources remain.
		_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
		Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(),
			"Expected disk to be fully deleted but it still exists")

		klog.Infof("Dynamic volume deleted successfully: name=%s — no orphaned resources", volName)
	})

})

// Topology Aware Scheduling Test Suite
//
// Background:
// When use-allowed-disk-topology=true is set in a StorageClass, the GCE PD CSI
// driver adds a disk-type label (e.g. disk-type.gke.io/hyperdisk-balanced=true)
// to the AccessibleTopology of the CreateVolume response.
//
// The Kubernetes scheduler reads these topology labels to ensure pods are only
// placed on nodes that support the provisioned disk type. This prevents a pod
// from being scheduled on a node that cannot attach the volume.
//
// Prerequisites:
//   - The driver must be started with --disk-topology=true (already set in
//     test/e2e/utils/utils.go line 75).
//   - HD-capable node (c3-standard-4) is required for Step 1.
//   - PD-only node (n2d-standard-4) is required for Step 2 and Step 3.
//
// This single test covers three scenarios in sequence:
//
//	Step 1: hyperdisk-balanced + flag=true  → label must be present in topology
//	Step 2: pd-balanced        + flag=true  → label must be present in topology
//	Step 3: pd-balanced        + flag=false → label must be absent from topology
var _ = Describe("GCE PD CSI Driver Topology Aware Scheduling", func() {

	It("Should schedule pod only on nodes that support the provisioned disk type", func() {

		// -----------------------------------------------------------------
		// Step 1: HD-capable node — hyperdisk-balanced with flag enabled
		//
		// Goal: Verify that use-allowed-disk-topology=true causes the driver
		// to include disk-type.gke.io/hyperdisk-balanced=true in the
		// AccessibleTopology response for a hyperdisk-balanced volume.
		//
		// Node type: HD-capable (c3-standard-4) via getRandomMwTestContext().
		// hyperdisk-balanced cannot be provisioned on PD-only nodes.
		// -----------------------------------------------------------------
		By("Step 1: Creating hyperdisk-balanced volume with use-allowed-disk-topology=true on HD-capable node")

		Expect(testContexts).ToNot(BeEmpty())

		// getRandomMwTestContext() returns a context backed by an HD-capable
		// node (c3-standard-4) that supports hyperdisk-balanced volumes.
		hdContext := getRandomMwTestContext()
		_, hdZone, _ := hdContext.Instance.GetIdentity()
		klog.Infof("[Step 1] Using HD-capable node in zone: %s", hdZone)

		hdVolName := testNamePrefix + string(uuid.NewUUID())
		klog.Infof("[Step 1] Creating volume: %s", hdVolName)

		// ProvisionedIOPS and ProvisionedThroughput are mandatory for
		// hyperdisk-balanced — omitting them causes InvalidArgument error.
		// use-allowed-disk-topology=true tells the driver to add the
		// disk-type label to AccessibleTopology in the response.
		hdParams := map[string]string{
			parameters.ParameterKeyType:                          hdbDiskType,
			parameters.ParameterKeyProvisionedIOPSOnCreate:       provisionedIOPSOnCreateHdb,
			parameters.ParameterKeyProvisionedThroughputOnCreate: provisionedThroughputOnCreateHdb,
			parameters.ParameterKeyUseAllowedDiskTopology:        "true",
		}
		klog.Infof("[Step 1] Params: type=%s iops=%s throughput=%s use-allowed-disk-topology=true",
			hdbDiskType, provisionedIOPSOnCreateHdb, provisionedThroughputOnCreateHdb)

		hdVolume, err := hdContext.Client.CreateVolume(hdVolName, hdParams, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						// Only zone is passed — the driver derives and returns
						// the disk-type label in AccessibleTopology.
						Segments: map[string]string{constants.TopologyKeyZone: hdZone},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume (hyperdisk-balanced) failed: %v", err)
		klog.Infof("[Step 1] Volume created successfully: %s", hdVolume.VolumeId)

		defer func() {
			klog.Infof("[Step 1] Deleting volume: %s", hdVolume.VolumeId)
			Expect(hdContext.Client.DeleteVolume(hdVolume.VolumeId)).To(BeNil(),
				"DeleteVolume (hyperdisk-balanced) failed")
			klog.Infof("[Step 1] Volume deleted successfully")
		}()

		// The driver must return exactly one topology entry for a zonal volume.
		Expect(hdVolume.AccessibleTopology).To(HaveLen(1),
			"Expected exactly one accessible topology for a zonal volume")

		hdSegments := hdVolume.AccessibleTopology[0].Segments
		klog.Infof("[Step 1] AccessibleTopology segments returned by driver: %v", hdSegments)

		// Verify zone segment is correct.
		Expect(hdSegments).To(HaveKeyWithValue(constants.TopologyKeyZone, hdZone),
			"Topology should include the correct zone segment")

		// Core assertion: disk-type.gke.io/hyperdisk-balanced=true must be present.
		// DiskTypeLabelKey("hyperdisk-balanced") = "disk-type.gke.io/hyperdisk-balanced"
		Expect(hdSegments).To(HaveKeyWithValue(common.DiskTypeLabelKey(hdbDiskType), "true"),
			"Topology should include disk-type.gke.io/hyperdisk-balanced=true")
		klog.Infof("[Step 1] PASSED — disk-type.gke.io/%s=true present in AccessibleTopology", hdbDiskType)

		// -----------------------------------------------------------------
		// Step 2: PD-only node — pd-balanced with flag enabled
		//
		// Goal: Verify that use-allowed-disk-topology=true causes the driver
		// to include disk-type.gke.io/pd-balanced=true in AccessibleTopology
		// for a pd-balanced volume on a PD-only node.
		//
		// Node type: PD-only (n2d-standard-4) via getRandomTestContext().
		// This validates the PD code path separately from the HD path above.
		// -----------------------------------------------------------------
		By("Step 2: Creating pd-balanced volume with use-allowed-disk-topology=true on PD-only node")

		// getRandomTestContext() returns a context backed by a PD-only node
		// (n2d-standard-4). pd-balanced works on all node types.
		pdContext := getRandomTestContext()
		_, pdZone, _ := pdContext.Instance.GetIdentity()
		klog.Infof("[Step 2] Using PD-only node in zone: %s", pdZone)

		pdVolName := testNamePrefix + string(uuid.NewUUID())
		klog.Infof("[Step 2] Creating volume: %s", pdVolName)

		// pd-balanced does not require IOPS or throughput params.
		// use-allowed-disk-topology=true triggers disk-type label in response.
		pdParams := map[string]string{
			parameters.ParameterKeyType:                   "pd-balanced",
			parameters.ParameterKeyUseAllowedDiskTopology: "true",
		}
		klog.Infof("[Step 2] Params: type=pd-balanced use-allowed-disk-topology=true")

		pdVolume, err := pdContext.Client.CreateVolume(pdVolName, pdParams, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{constants.TopologyKeyZone: pdZone},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume (pd-balanced) failed: %v", err)
		klog.Infof("[Step 2] Volume created successfully: %s", pdVolume.VolumeId)

		defer func() {
			klog.Infof("[Step 2] Deleting volume: %s", pdVolume.VolumeId)
			Expect(pdContext.Client.DeleteVolume(pdVolume.VolumeId)).To(BeNil(),
				"DeleteVolume (pd-balanced) failed")
			klog.Infof("[Step 2] Volume deleted successfully")
		}()

		Expect(pdVolume.AccessibleTopology).To(HaveLen(1),
			"Expected exactly one accessible topology for a zonal volume")

		pdSegments := pdVolume.AccessibleTopology[0].Segments
		klog.Infof("[Step 2] AccessibleTopology segments returned by driver: %v", pdSegments)

		// Verify zone segment is correct.
		Expect(pdSegments).To(HaveKeyWithValue(constants.TopologyKeyZone, pdZone),
			"Topology should include the correct zone segment")

		// Core assertion: disk-type.gke.io/pd-balanced=true must be present.
		Expect(pdSegments).To(HaveKeyWithValue(common.DiskTypeLabelKey("pd-balanced"), "true"),
			"Topology should include disk-type.gke.io/pd-balanced=true")
		klog.Infof("[Step 2] PASSED — disk-type.gke.io/pd-balanced=true present in AccessibleTopology")

		// -----------------------------------------------------------------
		// Step 3: PD-only node — pd-balanced WITHOUT flag (negative case)
		//
		// Goal: Verify that when use-allowed-disk-topology is NOT set, the
		// driver returns only the zone segment — no disk-type label at all.
		//
		// Why this matters: The disk-type label must be opt-in. If the driver
		// always returns disk-type labels regardless of the flag, it could
		// break existing workloads or incorrectly restrict pod scheduling.
		// -----------------------------------------------------------------
		By("Step 3: Creating pd-balanced volume WITHOUT use-allowed-disk-topology — label must be absent")

		// Reuse the same PD-only context from Step 2.
		klog.Infof("[Step 3] Using PD-only node in zone: %s (reusing context from Step 2)", pdZone)

		noFlagVolName := testNamePrefix + string(uuid.NewUUID())
		klog.Infof("[Step 3] Creating volume: %s", noFlagVolName)

		// use-allowed-disk-topology is deliberately omitted.
		// The driver must NOT add disk-type labels in this case.
		noFlagParams := map[string]string{
			parameters.ParameterKeyType: "pd-balanced",
		}
		klog.Infof("[Step 3] Params: type=pd-balanced (use-allowed-disk-topology NOT set — default behaviour)")

		noFlagVolume, err := pdContext.Client.CreateVolume(noFlagVolName, noFlagParams, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{constants.TopologyKeyZone: pdZone},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume (no flag) failed: %v", err)
		klog.Infof("[Step 3] Volume created successfully: %s", noFlagVolume.VolumeId)

		defer func() {
			klog.Infof("[Step 3] Deleting volume: %s", noFlagVolume.VolumeId)
			Expect(pdContext.Client.DeleteVolume(noFlagVolume.VolumeId)).To(BeNil(),
				"DeleteVolume (no flag) failed")
			klog.Infof("[Step 3] Volume deleted successfully")
		}()

		// Volume must still have a topology entry — just without disk-type labels.
		Expect(noFlagVolume.AccessibleTopology).ToNot(BeEmpty(),
			"Volume should still have accessible topologies (zone only)")

		noFlagSegments := noFlagVolume.AccessibleTopology[0].Segments
		klog.Infof("[Step 3] AccessibleTopology segments returned by driver: %v", noFlagSegments)
		klog.Infof("[Step 3] Total segment keys returned: %d (expected: 1 — zone only)", len(noFlagSegments))

		// Zone segment must still be present.
		Expect(noFlagSegments).To(HaveKeyWithValue(constants.TopologyKeyZone, pdZone),
			"Topology should still include the zone segment")

		// Core negative assertion: disk-type.gke.io/pd-balanced must NOT exist.
		// If present, the driver is incorrectly leaking disk-type labels when
		// use-allowed-disk-topology was not requested.
		Expect(noFlagSegments).ToNot(HaveKey(common.DiskTypeLabelKey("pd-balanced")),
			"Topology must NOT include disk-type.gke.io/pd-balanced when use-allowed-disk-topology is not set")

		klog.Infof("[Step 3] PASSED — disk-type.gke.io/pd-balanced correctly absent from AccessibleTopology")
		klog.Infof("[Step 3] Only zone segment present as expected: %v", noFlagSegments)
	})

})

// Dynamic IOPS Provisioning on HD Test Suite
//
// Background:
// hyperdisk-balanced supports custom IOPS and throughput values set at
// provisioning time via StorageClass parameters:
//   - provisioned-iops-on-create:       sets the IOPS for the disk
//   - provisioned-throughput-on-create: sets the throughput (MiB/s) for the disk
//
// These values are applied at disk creation time and verified via the
// GCE Disks API (disk.ProvisionedIops, disk.ProvisionedThroughput).
//
// Node type: HD-capable (c3-standard-4) via getRandomMwTestContext().
// hyperdisk-balanced can only be attached to c3/c3d/c4 machine families —
// this is a GCE infrastructure constraint, not a test constraint.
//
// This single test covers two scenarios in sequence:
//
//	Step 1-4: Create with standard IOPS (3000) and throughput (150 MiB/s)
//	          → verify both values applied correctly via GCE API.
//	Step 5-7: Create with non-default custom IOPS (12345)
//	          → verify the driver passes user-specified value, not a default.
var _ = Describe("GCE PD CSI Driver Dynamic IOPS Provisioning", func() {

	It("Should apply specified IOPS and throughput values to hyperdisk-balanced on HD-capable node", func() {
		Expect(testContexts).ToNot(BeEmpty())

		// HD-capable node (c3-standard-4) required for hyperdisk-balanced.
		// getRandomMwTestContext() returns a context from hyperdiskTestContexts
		// which are nodes created with -hyperdisk-machine-type=c3-standard-4.
		testContext := getRandomMwTestContext()
		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		klog.Infof("[IOPS] Using HD-capable node in zone: %s project: %s", z, p)

		// -----------------------------------------------------------------
		// Step 1: Create hyperdisk-balanced with standard IOPS and throughput
		//
		// Simulates a dynamic StorageClass with:
		//   type: hyperdisk-balanced
		//   provisioned-iops-on-create: 3000
		//   provisioned-throughput-on-create: 150Mi
		// -----------------------------------------------------------------
		By("Step 1: Creating hyperdisk-balanced with standard IOPS (3000) and throughput (150 MiB/s)")

		volName1 := testNamePrefix + string(uuid.NewUUID())
		klog.Infof("[IOPS Step 1] Creating volume: %s", volName1)
		klog.Infof("[IOPS Step 1] Params: type=%s iops=%s throughput=%s",
			hdbDiskType, provisionedIOPSOnCreateHdb, provisionedThroughputOnCreateHdb)

		// provisionedIOPSOnCreateHdb = "3000", provisionedThroughputOnCreateHdb = "150Mi"
		// Both are mandatory for hyperdisk-balanced — omitting either causes
		// an InvalidArgument error from the CSI driver.
		params1 := map[string]string{
			parameters.ParameterKeyType:                          hdbDiskType,
			parameters.ParameterKeyProvisionedIOPSOnCreate:       provisionedIOPSOnCreateHdb,
			parameters.ParameterKeyProvisionedThroughputOnCreate: provisionedThroughputOnCreateHdb,
		}

		volume1, err := client.CreateVolume(volName1, params1, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{constants.TopologyKeyZone: z}},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume (standard IOPS) failed: %v", err)
		klog.Infof("[IOPS Step 1] Volume created: %s", volume1.VolumeId)

		defer func() {
			klog.Infof("[IOPS Step 1] Deleting volume: %s", volume1.VolumeId)
			err := client.DeleteVolume(volume1.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume (standard IOPS) failed")
			_, err = computeService.Disks.Get(p, z, volName1).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(),
				"Expected standard IOPS disk to be deleted")
			klog.Infof("[IOPS Step 1] Volume deleted and confirmed gone")
		}()

		// -----------------------------------------------------------------
		// Step 2: Verify disk exists in GCE with correct type and status
		// -----------------------------------------------------------------
		By("Step 2: Verifying disk exists in GCE with correct type and status")

		cloudDisk1, err := computeService.Disks.Get(p, z, volName1).Do()
		Expect(err).To(BeNil(), "Could not get disk from GCE API")
		klog.Infof("[IOPS Step 2] GCE disk: type=%s status=%s size=%d iops=%d throughput=%d",
			cloudDisk1.Type, cloudDisk1.Status, cloudDisk1.SizeGb,
			cloudDisk1.ProvisionedIops, cloudDisk1.ProvisionedThroughput)

		Expect(cloudDisk1.Status).To(Equal(readyState),
			"Disk should be in READY state")
		Expect(cloudDisk1.Type).To(ContainSubstring(hdbDiskType),
			"Disk type should be hyperdisk-balanced")
		Expect(cloudDisk1.SizeGb).To(Equal(defaultHdBSizeGb),
			"Disk size should be %d GiB", defaultHdBSizeGb)

		// -----------------------------------------------------------------
		// Step 3: Verify the specified IOPS value is applied
		//
		// Core assertion: ProvisionedIops on the GCE disk must match exactly
		// what was set in provisioned-iops-on-create parameter.
		// -----------------------------------------------------------------
		By("Step 3: Verifying standard IOPS value (3000) is applied to hyperdisk")

		klog.Infof("[IOPS Step 3] Expected IOPS: %d, Actual IOPS: %d",
			provisionedIOPSOnCreateHdbInt, cloudDisk1.ProvisionedIops)
		Expect(cloudDisk1.ProvisionedIops).To(Equal(provisionedIOPSOnCreateHdbInt),
			"ProvisionedIops should be %d as set in provisioned-iops-on-create",
			provisionedIOPSOnCreateHdbInt)
		klog.Infof("[IOPS Step 3] PASSED — IOPS %d correctly applied",
			cloudDisk1.ProvisionedIops)

		// -----------------------------------------------------------------
		// Step 4: Verify the specified throughput value is also applied
		// -----------------------------------------------------------------
		By("Step 4: Verifying standard throughput value (150 MiB/s) is applied to hyperdisk")

		klog.Infof("[IOPS Step 4] Expected throughput: %d MiB/s, Actual: %d MiB/s",
			provisionedThroughputOnCreateHdbInt, cloudDisk1.ProvisionedThroughput)
		Expect(cloudDisk1.ProvisionedThroughput).To(Equal(provisionedThroughputOnCreateHdbInt),
			"ProvisionedThroughput should be %d MiB/s as set in provisioned-throughput-on-create",
			provisionedThroughputOnCreateHdbInt)
		klog.Infof("[IOPS Step 4] PASSED — throughput %d MiB/s correctly applied",
			cloudDisk1.ProvisionedThroughput)

		// -----------------------------------------------------------------
		// Step 5: Create hyperdisk-balanced with non-default custom IOPS
		//
		// Uses provisionedIOPSOnCreate (12345) — a deliberately unusual value
		// that cannot match any GCE default, proving the driver honours the
		// exact user-specified value from the StorageClass parameter rather
		// than substituting a built-in default.
		// -----------------------------------------------------------------
		By("Step 5: Creating hyperdisk-balanced with non-default custom IOPS (12345)")

		volName2 := testNamePrefix + string(uuid.NewUUID())
		klog.Infof("[IOPS Step 5] Creating volume: %s", volName2)
		klog.Infof("[IOPS Step 5] Params: type=%s iops=%s (non-default) throughput=%s",
			hdbDiskType, provisionedIOPSOnCreate, provisionedThroughputOnCreateHdb)

		// provisionedIOPSOnCreate = "12345" — chosen specifically because it
		// is not a round number and cannot be a coincidental default value.
		params2 := map[string]string{
			parameters.ParameterKeyType:                          hdbDiskType,
			parameters.ParameterKeyProvisionedIOPSOnCreate:       provisionedIOPSOnCreate,
			parameters.ParameterKeyProvisionedThroughputOnCreate: provisionedThroughputOnCreateHdb,
		}

		volume2, err := client.CreateVolume(volName2, params2, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{constants.TopologyKeyZone: z}},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume (non-default IOPS) failed: %v", err)
		klog.Infof("[IOPS Step 5] Volume created: %s", volume2.VolumeId)

		defer func() {
			klog.Infof("[IOPS Step 5] Deleting volume: %s", volume2.VolumeId)
			err := client.DeleteVolume(volume2.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume (non-default IOPS) failed")
			_, err = computeService.Disks.Get(p, z, volName2).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(),
				"Expected non-default IOPS disk to be deleted")
			klog.Infof("[IOPS Step 5] Volume deleted and confirmed gone")
		}()

		// -----------------------------------------------------------------
		// Step 6: Verify disk exists in GCE with correct type and status
		// -----------------------------------------------------------------
		By("Step 6: Verifying disk exists in GCE with correct type and status")

		cloudDisk2, err := computeService.Disks.Get(p, z, volName2).Do()
		Expect(err).To(BeNil(), "Could not get disk from GCE API")
		klog.Infof("[IOPS Step 6] GCE disk: type=%s status=%s iops=%d throughput=%d",
			cloudDisk2.Type, cloudDisk2.Status,
			cloudDisk2.ProvisionedIops, cloudDisk2.ProvisionedThroughput)

		Expect(cloudDisk2.Status).To(Equal(readyState))
		Expect(cloudDisk2.Type).To(ContainSubstring(hdbDiskType))

		// -----------------------------------------------------------------
		// Step 7: Verify non-default custom IOPS value is applied exactly
		// -----------------------------------------------------------------
		By("Step 7: Verifying non-default custom IOPS value (12345) is applied exactly")

		// The driver must pass the exact user-specified IOPS value to the
		// GCE API — not substitute any default. If ProvisionedIops is 3000
		// or any other value, it means the driver ignored the parameter.
		klog.Infof("[IOPS Step 7] Expected IOPS: %d, Actual IOPS: %d",
			provisionedIOPSOnCreateInt, cloudDisk2.ProvisionedIops)
		Expect(cloudDisk2.ProvisionedIops).To(Equal(provisionedIOPSOnCreateInt),
			"ProvisionedIops should be %d (non-default custom value) — driver must not substitute defaults",
			provisionedIOPSOnCreateInt)
		klog.Infof("[IOPS Step 7] PASSED — non-default IOPS %d correctly applied",
			cloudDisk2.ProvisionedIops)

		klog.Infof("[IOPS] ALL STEPS PASSED — both standard and custom IOPS values correctly applied to hyperdisk-balanced")
	})

})

// Multi-Zone Dynamic Provisioning Test
//
// Background:
// In a multi-zone Kubernetes cluster with WaitForFirstConsumer volume binding,
// the scheduler picks a node first, then the CSI driver provisions the disk
// in the same zone as the scheduled node. This ensures the pod can always
// attach the volume — cross-zone attachment is not supported for zonal PDs.
//
// In this CSI e2e framework (no real pods/PVCs), WaitForFirstConsumer binding
// is simulated by passing the target zone in the Preferred topology of the
// TopologyRequirement. The driver must provision the disk in that preferred
// zone, just as it would if a pod had been scheduled there.
//
// This test:
//
//	Step 1: Discover two distinct zones from testContexts (simulates multi-zone cluster).
//	Step 2: Provision a pd-balanced volume with Preferred topology set to zone-1
//	        (simulates WaitForFirstConsumer — pod scheduled in zone-1).
//	        Verify the disk is created in zone-1 via the GCE API.
//	Step 3: Provision a pd-balanced volume with Preferred topology set to zone-2
//	        (simulates pod rescheduled to zone-2).
//	        Verify the disk is created in zone-2 via the GCE API.
//	Step 4: Cleanup — delete both volumes and confirm disks are gone.
var _ = Describe("GCE PD CSI Driver Multi-Zone Dynamic Provisioning", func() {

	It("Should provision disk in the correct zone matching where the pod is scheduled", func() {

		// Require at least 2 test contexts to have a meaningful multi-zone test.
		Expect(len(testContexts)).To(BeNumerically(">", 1),
			"Need at least 2 test contexts for multi-zone test")

		// -----------------------------------------------------------------
		// Step 1: Discover two distinct zones from testContexts
		//
		// testContexts contains one context per node across all zones.
		// We build a zoneToContext map to get one context per unique zone,
		// then pick zone-1 and zone-2 for the test.
		// This mirrors the pattern used in multi_zone_e2e_test.go.
		// -----------------------------------------------------------------
		By("Step 1: Discovering two distinct zones from test contexts")

		// Build zone -> context map, one entry per unique zone.
		zoneToContext := map[string]interface {
			GetIdentity() (string, string, string)
		}{}
		zones := []string{}
		for _, tc := range testContexts {
			_, z, _ := tc.Instance.GetIdentity()
			if _, ok := zoneToContext[z]; !ok {
				zoneToContext[z] = tc.Instance
				zones = append(zones, z)
			}
			if len(zones) == 2 {
				break
			}
		}

		Expect(len(zones)).To(Equal(2),
			"Must have test contexts in at least 2 zones for multi-zone provisioning test")

		zone1 := zones[0]
		zone2 := zones[1]

		// Use first context as the controller client for all CreateVolume calls.
		controllerContext := testContexts[0]
		p, _, _ := controllerContext.Instance.GetIdentity()

		klog.Infof("[Step 1] Discovered zones: zone1=%s zone2=%s project=%s", zone1, zone2, p)

		// -----------------------------------------------------------------
		// Step 2: Provision volume in zone-1
		//
		// Simulates: pod scheduled on a node in zone-1 by the Kubernetes
		// scheduler (WaitForFirstConsumer). The CSI driver must provision
		// the disk in zone-1 so the pod can attach it.
		//
		// We pass zone-1 as both Requisite and Preferred topology.
		// Requisite = allowed zones, Preferred = most desired zone.
		// The driver picks the Preferred zone when available.
		// -----------------------------------------------------------------
		By("Step 2: Provisioning pd-balanced volume with Preferred topology in zone-1")

		volName1 := testNamePrefix + string(uuid.NewUUID())
		klog.Infof("[Step 2] Creating volume %s with Preferred zone: %s", volName1, zone1)

		// Parameters: pd-balanced is supported on all node types.
		// No special flags needed — this tests pure zone-selection behaviour.
		params := map[string]string{
			parameters.ParameterKeyType: "pd-balanced",
		}

		volume1, err := controllerContext.Client.CreateVolume(volName1, params, defaultSizeGb,
			&csi.TopologyRequirement{
				// Requisite: the set of zones the driver is allowed to use.
				Requisite: []*csi.Topology{
					{Segments: map[string]string{constants.TopologyKeyZone: zone1}},
					{Segments: map[string]string{constants.TopologyKeyZone: zone2}},
				},
				// Preferred: the zone where the pod was scheduled.
				// WaitForFirstConsumer sets this to the node's zone.
				// The driver MUST honour this and provision in zone-1.
				Preferred: []*csi.Topology{
					{Segments: map[string]string{constants.TopologyKeyZone: zone1}},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume in zone-1 failed: %v", err)
		klog.Infof("[Step 2] Volume created: %s", volume1.VolumeId)

		defer func() {
			klog.Infof("[Step 2] Deleting volume: %s", volume1.VolumeId)
			err := controllerContext.Client.DeleteVolume(volume1.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume zone-1 failed")

			// Verify disk is actually gone from GCE.
			_, key, err := common.VolumeIDToKey(volume1.VolumeId)
			Expect(err).To(BeNil())
			_, err = computeService.Disks.Get(p, key.Zone, key.Name).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(),
				"Expected zone-1 disk to be deleted but it still exists")
			klog.Infof("[Step 2] Volume deleted and confirmed gone from GCE")
		}()

		// Verify via GCE API that disk was created in zone-1.
		// This is the core assertion: disk zone must match the Preferred zone
		// (i.e. the zone where the pod would have been scheduled).
		klog.Infof("[Step 2] Verifying disk exists in GCE zone: %s", zone1)
		disk1, err := computeService.Disks.Get(p, zone1, volName1).Do()
		Expect(err).To(BeNil(),
			"Disk not found in zone-1 (%s) — driver did not honour Preferred topology", zone1)
		Expect(disk1.Status).To(Equal(readyState),
			"Disk in zone-1 is not in READY state")
		Expect(disk1.Type).To(ContainSubstring("pd-balanced"),
			"Disk type does not match requested pd-balanced")

		klog.Infof("[Step 2] PASSED — disk %s created in correct zone: %s (type: %s)",
			volName1, zone1, disk1.Type)

		// Also verify AccessibleTopology in the response matches zone-1.
		Expect(volume1.AccessibleTopology).ToNot(BeEmpty(),
			"Volume should have accessible topologies")
		responseZone1 := volume1.AccessibleTopology[0].Segments[constants.TopologyKeyZone]
		Expect(responseZone1).To(Equal(zone1),
			"AccessibleTopology zone in response does not match Preferred zone-1")
		klog.Infof("[Step 2] AccessibleTopology zone in response: %s (matches zone-1)", responseZone1)

		// -----------------------------------------------------------------
		// Step 3: Provision volume in zone-2
		//
		// Simulates: a different pod scheduled on a node in zone-2.
		// Verifies the driver correctly picks zone-2 when it is Preferred,
		// demonstrating zone-aware dynamic provisioning works across zones.
		// -----------------------------------------------------------------
		By("Step 3: Provisioning pd-balanced volume with Preferred topology in zone-2")

		volName2 := testNamePrefix + string(uuid.NewUUID())
		klog.Infof("[Step 3] Creating volume %s with Preferred zone: %s", volName2, zone2)

		volume2, err := controllerContext.Client.CreateVolume(volName2, params, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{constants.TopologyKeyZone: zone1}},
					{Segments: map[string]string{constants.TopologyKeyZone: zone2}},
				},
				// Preferred zone is now zone-2 — driver must provision there.
				Preferred: []*csi.Topology{
					{Segments: map[string]string{constants.TopologyKeyZone: zone2}},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume in zone-2 failed: %v", err)
		klog.Infof("[Step 3] Volume created: %s", volume2.VolumeId)

		defer func() {
			klog.Infof("[Step 3] Deleting volume: %s", volume2.VolumeId)
			err := controllerContext.Client.DeleteVolume(volume2.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume zone-2 failed")

			// Verify disk is actually gone from GCE.
			_, key, err := common.VolumeIDToKey(volume2.VolumeId)
			Expect(err).To(BeNil())
			_, err = computeService.Disks.Get(p, key.Zone, key.Name).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(),
				"Expected zone-2 disk to be deleted but it still exists")
			klog.Infof("[Step 3] Volume deleted and confirmed gone from GCE")
		}()

		// Verify via GCE API that disk was created in zone-2.
		klog.Infof("[Step 3] Verifying disk exists in GCE zone: %s", zone2)
		disk2, err := computeService.Disks.Get(p, zone2, volName2).Do()
		Expect(err).To(BeNil(),
			"Disk not found in zone-2 (%s) — driver did not honour Preferred topology", zone2)
		Expect(disk2.Status).To(Equal(readyState),
			"Disk in zone-2 is not in READY state")
		Expect(disk2.Type).To(ContainSubstring("pd-balanced"),
			"Disk type does not match requested pd-balanced")

		klog.Infof("[Step 3] PASSED — disk %s created in correct zone: %s (type: %s)",
			volName2, zone2, disk2.Type)

		// Also verify AccessibleTopology in the response matches zone-2.
		Expect(volume2.AccessibleTopology).ToNot(BeEmpty(),
			"Volume should have accessible topologies")
		responseZone2 := volume2.AccessibleTopology[0].Segments[constants.TopologyKeyZone]
		Expect(responseZone2).To(Equal(zone2),
			"AccessibleTopology zone in response does not match Preferred zone-2")
		klog.Infof("[Step 3] AccessibleTopology zone in response: %s (matches zone-2)", responseZone2)

		klog.Infof("[DONE] Both disks provisioned in correct zones — zone1=%s zone2=%s", zone1, zone2)
	})

})
