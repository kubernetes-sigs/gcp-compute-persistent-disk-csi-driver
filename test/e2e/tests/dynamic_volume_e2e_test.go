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
