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
	"fmt"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/parameters"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"
	remote "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/remote"

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

var _ = Describe("GCE PD CSI Driver Disk Type Selection", func() {

	// Verifies that when disk-type-preference is set with hyperdisk-type on a node
	// that supports both hyperdisk and PD, the driver selects hyperdisk-balanced
	// over pd-balanced.
	It("Should prefer hyperdisk-balanced over pd-balanced on a node supporting both HD and PD", func() {
		// getRandomMwTestContext() returns a c3-standard-4 VM which supports both hyperdisk and PD
		testContext := getRandomMwTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		volName := testNamePrefix + string(uuid.NewUUID())

		// disk-type-preference: hyperdisk-type is set, so driver must prefer hyperdisk-balanced
		// over pd-balanced on a node that supports both
		params := map[string]string{
			parameters.ParameterKeyType:        parameters.DynamicVolumeType,
			parameters.ParameterHDType:         "hyperdisk-balanced",
			parameters.ParameterPDType:         "pd-balanced",
			parameters.ParameterDiskPreference: parameters.ParameterHDType,
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
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

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

		klog.Infof("Disk type selected on HD+PD capable node %s: %s", z, cloudDisk.Type)
		Expect(cloudDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected hyperdisk-balanced to be preferred over pd-balanced on HD+PD capable node, got: %s", cloudDisk.Type)
	})

	// Verifies that when disk-type-preference is set with pd-type on a node that supports
	// both hyperdisk and PD, the driver selects pd-balanced even though HD is available.
	It("Should prefer pd-balanced over hyperdisk-balanced on a node supporting both HD and PD when pd-type is set", func() {
		// getRandomMwTestContext() returns a c3-standard-4 VM which supports both hyperdisk and PD
		testContext := getRandomMwTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		volName := testNamePrefix + string(uuid.NewUUID())

		// disk-type-preference=pd-type forces the driver to default to pd-balanced
		// even on a node that supports both HD and PD
		params := map[string]string{
			parameters.ParameterKeyType:        parameters.DynamicVolumeType,
			parameters.ParameterHDType:         "hyperdisk-balanced",
			parameters.ParameterPDType:         "pd-balanced",
			parameters.ParameterDiskPreference: parameters.ParameterPDType,
		}

		volume, err := client.CreateVolume(volName, params, defaultSizeGb,
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
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

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

		klog.Infof("Disk type selected on HD+PD capable node %s: %s", z, cloudDisk.Type)
		Expect(cloudDisk.Type).To(ContainSubstring("pd-balanced"),
			"Expected pd-balanced to be selected over hyperdisk on HD+PD capable node, got: %s", cloudDisk.Type)
	})

	// Verifies that when hyperdisk-type is set to hyperdisk-throughput in the dynamic
	// StorageClass, the driver provisions hyperdisk-throughput (not hyperdisk-balanced)
	// on a node compatible with hyperdisk-throughput.
	It("Should provision hyperdisk-throughput when hyperdisk-type is set to hyperdisk-throughput", func() {
		// getRandomMwTestContext() returns a c3-standard-4 VM compatible with hyperdisk-throughput
		testContext := getRandomMwTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		volName := testNamePrefix + string(uuid.NewUUID())

		// hyperdisk-type explicitly set to hyperdisk-throughput — driver must use this
		// exact type instead of defaulting to hyperdisk-balanced
		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-throughput",
			parameters.ParameterPDType:  "pd-balanced",
		}

		// defaultHdTSizeGb is the minimum size for hyperdisk-throughput
		volume, err := client.CreateVolume(volName, params, defaultHdTSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                          z,
							common.DiskTypeLabelKey("hyperdisk-throughput"): "true",
							common.DiskTypeLabelKey("pd-balanced"):          "true",
						},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                          z,
							common.DiskTypeLabelKey("hyperdisk-throughput"): "true",
							common.DiskTypeLabelKey("pd-balanced"):          "true",
						},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

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

		klog.Infof("Disk type provisioned on HD-capable node %s: %s", z, cloudDisk.Type)
		Expect(cloudDisk.Type).To(ContainSubstring("hyperdisk-throughput"),
			"Expected hyperdisk-throughput from custom hyperdisk-type param, got: %s", cloudDisk.Type)
	})

	// Verifies that when pd-type is set to pd-ssd in the dynamic StorageClass,
	// the driver provisions pd-ssd when HD is not available on the scheduled node.
	It("Should provision pd-ssd when pd-type is set to pd-ssd and HD is not available on the node", func() {
		// getRandomTestContext() returns an n2-standard-4 VM which is PD-only (no hyperdisk support)
		testContext := getRandomTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		volName := testNamePrefix + string(uuid.NewUUID())

		// No disk-type-preference is set. The driver reads Preferred topology labels
		// to determine supported disk types. The PD-only node has pd-ssd label but no
		// hyperdisk-balanced label, so the driver selects pd-ssd via topology.
		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-balanced",
			parameters.ParameterPDType:  "pd-ssd",
		}

		// Both Requisite and Preferred carry only pd-ssd label — hyperdisk-balanced is
		// intentionally absent to simulate a PD-only node so the driver falls back to pd-ssd.
		volume, err := client.CreateVolume(volName, params, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":            z,
							common.DiskTypeLabelKey("pd-ssd"): "true",
						},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":            z,
							common.DiskTypeLabelKey("pd-ssd"): "true",
						},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume failed with error: %v", err)

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

		klog.Infof("Disk type provisioned on PD-only node %s: %s", z, cloudDisk.Type)
		Expect(cloudDisk.Type).To(ContainSubstring("pd-ssd"),
			"Expected pd-ssd on PD-only node detected via topology, got: %s", cloudDisk.Type)
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

var _ = Describe("GCE PD CSI Driver Dynamic Volumes Pod Migration", func() {

	It("Should persist data and retain disk type after pod migration from node A to node B", func() {
		var tcA, tcB *remote.TestContext
		for i := 0; i < len(testContexts); i++ {
			for j := i + 1; j < len(testContexts); j++ {
				_, zoneA, _ := testContexts[i].Instance.GetIdentity()
				_, zoneB, _ := testContexts[j].Instance.GetIdentity()
				if zoneA == zoneB {
					tcA = testContexts[i]
					tcB = testContexts[j]
					break
				}
			}
			if tcA != nil {
				break
			}
		}
		Expect(tcA).NotTo(BeNil(), "Need at least 2 nodes in the same zone for zonal pod migration")

		pA, zA, _ := tcA.Instance.GetIdentity()
		instanceA := tcA.Instance
		clientA := tcA.Client

		instanceB := tcB.Instance
		clientB := tcB.Client

		volName := testNamePrefix + string(uuid.NewUUID())

		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-balanced",
			parameters.ParameterPDType:  "pd-balanced",
		}

		// Create dynamic volume on node A's zone — PD-only topology resolves to pd-balanced.
		volume, err := clientA.CreateVolume(volName, params, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                 zA,
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
							"topology.gke.io/zone":                 zA,
							common.DiskTypeLabelKey("pd-balanced"): "true",
							common.DiskTypeLabelKey("pd-standard"): "true",
							common.DiskTypeLabelKey("pd-ssd"):      "true",
							common.DiskTypeLabelKey("pd-extreme"):  "true",
						},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume failed: %v", err)
		volID := volume.VolumeId

		defer func() {
			err := clientA.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")
			project, key, err := common.VolumeIDToKey(volID)
			Expect(err).To(BeNil(), "Failed to parse volume ID")
			_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to be deleted")
		}()

		// Verify disk type resolved to pd-balanced.
		cloudDisk, err := computeService.Disks.Get(pA, zA, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from GCE API")
		Expect(cloudDisk.Status).To(Equal(readyState), "Disk not in READY state")
		initialDiskType := cloudDisk.Type
		klog.Infof("Dynamic volume created: name=%s type=%s", volName, initialDiskType)
		Expect(initialDiskType).To(ContainSubstring("pd-balanced"),
			"Expected pd-balanced but got: %s", initialDiskType)

		mountArgs := attachAndMountArgs{readOnly: false, useBlock: false, forceAttach: false}

		// Attach, stage, and mount on node A — then write a test file.
		err, cleanupA, argsA := testAttachAndMount(volID, volName, instanceA, clientA, mountArgs)
		Expect(err).To(BeNil(), "Failed to attach and mount on node A: %v", err)

		writeFile, _ := testWriteAndReadFile(instanceA, false)
		Expect(writeFile(argsA)).To(BeNil(), "Failed to write file on node A")

		// Simulate pod deletion: unpublish → unstage → detach from node A.
		err = clientA.NodeUnpublishVolume(volID, argsA.publishDir)
		Expect(err).To(BeNil(), "NodeUnpublishVolume failed on node A")
		err = clientA.NodeUnstageVolume(volID, argsA.stageDir)
		Expect(err).To(BeNil(), "NodeUnstageVolume failed on node A")
		_ = testutils.RmAll(instanceA, filepath.Join("/tmp/", volName))
		cleanupA()

		// Simulate pod rescheduling: attach, stage, and mount on node B.
		err, cleanupB, stageDir := testAttach(volID, volName, instanceB, clientB, mountArgs)
		Expect(err).To(BeNil(), "Failed to attach disk to node B: %v", err)
		defer cleanupB()

		err, _, argsB := testMount(volID, volName, instanceB, clientB, mountArgs, stageDir)
		Expect(err).To(BeNil(), "Failed to mount disk on node B: %v", err)
		defer func() {
			_ = clientB.NodeUnpublishVolume(volID, argsB.publishDir)
		}()

		// Verify file written on node A is readable on node B.
		_, readFile := testWriteAndReadFile(instanceB, false)
		Expect(readFile(argsB)).To(BeNil(), "Data not persisted after pod migration to node B")

		// Verify disk type is unchanged after migration.
		cloudDiskAfter, err := computeService.Disks.Get(pA, zA, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk after migration")
		Expect(cloudDiskAfter.Type).To(Equal(initialDiskType),
			"Disk type changed after migration: was %s, now %s", initialDiskType, cloudDiskAfter.Type)

		klog.Infof("Pod migration verified: data persisted and disk type unchanged (%s)", cloudDiskAfter.Type)
	})

})

// Pod Restart with Dynamic Volume
//
// Verifies that after a pod restart the dynamic volume is reattached correctly
// and data written before the restart is still intact.
var _ = Describe("GCE PD CSI Driver Dynamic Volumes Pod Restart", func() {

	It("Should reattach dynamic volume correctly and maintain data integrity after pod restart", func() {
		testContext := getRandomMwTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		instance := testContext.Instance
		client := testContext.Client

		volName := testNamePrefix + string(uuid.NewUUID())

		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-balanced",
			parameters.ParameterPDType:  "pd-balanced",
		}

		// Create dynamic volume.
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
		volID := volume.VolumeId

		defer func() {
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")
			project, key, err := common.VolumeIDToKey(volID)
			Expect(err).To(BeNil(), "Failed to parse volume ID")
			_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to be deleted")
		}()

		// Verify disk resolved to hyperdisk-balanced.
		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from GCE API")
		Expect(cloudDisk.Status).To(Equal(readyState), "Disk not in READY state")
		klog.Infof("Dynamic volume created: name=%s type=%s", volName, cloudDisk.Type)
		Expect(cloudDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected hyperdisk-balanced but got: %s", cloudDisk.Type)

		mountArgs := attachAndMountArgs{readOnly: false, useBlock: false, forceAttach: false}

		// First mount (pod start): attach → stage → mount → write file.
		err, _, args := testAttachAndMount(volID, volName, instance, client, mountArgs)
		Expect(err).To(BeNil(), "Failed to attach and mount (first): %v", err)

		writeFile, _ := testWriteAndReadFile(instance, false)
		Expect(writeFile(args)).To(BeNil(), "Failed to write file before pod restart")

		// Simulate pod restart: unpublish → unstage → detach.
		err = client.NodeUnpublishVolume(volID, args.publishDir)
		Expect(err).To(BeNil(), "NodeUnpublishVolume failed during pod restart")
		err = client.NodeUnstageVolume(volID, args.stageDir)
		Expect(err).To(BeNil(), "NodeUnstageVolume failed during pod restart")
		_ = testutils.RmAll(instance, filepath.Join("/tmp/", volName))
		err = detach(volID, instance, client)
		Expect(err).To(BeNil(), "Detach failed during pod restart")

		// Second mount (pod restart): reattach → stage → mount → read file.
		err, cleanup, stageDir := testAttach(volID, volName, instance, client, mountArgs)
		Expect(err).To(BeNil(), "Failed to reattach disk after pod restart: %v", err)
		defer cleanup()

		err, _, argsRestart := testMount(volID, volName, instance, client, mountArgs, stageDir)
		Expect(err).To(BeNil(), "Failed to mount disk after pod restart: %v", err)
		defer func() {
			_ = client.NodeUnpublishVolume(volID, argsRestart.publishDir)
		}()

		// Verify data written before restart is intact.
		_, readFile := testWriteAndReadFile(instance, false)
		err = readFile(argsRestart)
		if err != nil {
			Expect(err).To(BeNil(), fmt.Sprintf("Data integrity check failed after pod restart: %v", err))
		}

		klog.Infof("Pod restart verified: volume reattached and data integrity confirmed for %s", volName)
	})

})

// Snapshot/restore GCE round trips scale with disk size, so these tests use a
// smaller source size than defaultHdBSizeGb (100) to keep suite runtime down.
const (
	snapshotSourceSizeGb        int64 = 50
	snapshotRestoreLargerSizeGb int64 = 60
)

// waitForSnapshotReady polls until the GCP snapshot reaches READY status.
func waitForSnapshotReady(project, snapshotName string) error {
	return wait.Poll(10*time.Second, 5*time.Minute, func() (bool, error) {
		snap, err := computeService.Snapshots.Get(project, snapshotName).Do()
		if err != nil {
			return false, err
		}
		return snap.Status == "READY", nil
	})
}

var _ = Describe("GCE PD CSI Driver Dynamic Volumes Snapshot of HD Volume", func() {

	It("Should create a snapshot of a hyperdisk-balanced dynamic volume successfully", func() {
		testContext := getRandomMwTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		volName := testNamePrefix + string(uuid.NewUUID())

		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-balanced",
			parameters.ParameterPDType:  "pd-balanced",
		}

		// Create dynamic volume — should resolve to hyperdisk-balanced on HD node.
		volume, err := client.CreateVolume(volName, params, snapshotSourceSizeGb,
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
		volID := volume.VolumeId

		defer func() {
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to be deleted")
		}()

		// Verify source disk is hyperdisk-balanced.
		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from GCE API")
		Expect(cloudDisk.Status).To(Equal(readyState), "Disk not in READY state")
		Expect(cloudDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected hyperdisk-balanced source disk but got: %s", cloudDisk.Type)

		// Take a snapshot of the dynamic volume.
		snapshotName := testNamePrefix + string(uuid.NewUUID())
		snapshotID, err := client.CreateSnapshot(snapshotName, volID, nil)
		Expect(err).To(BeNil(), "CreateSnapshot failed: %v", err)

		defer func() {
			err := client.DeleteSnapshot(snapshotID)
			Expect(err).To(BeNil(), "DeleteSnapshot failed")
			_, err = computeService.Snapshots.Get(p, snapshotName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected snapshot to be deleted")
		}()

		// Verify snapshot exists and reaches READY state.
		snapshot, err := computeService.Snapshots.Get(p, snapshotName).Do()
		Expect(err).To(BeNil(), "Could not get snapshot from GCE API")
		Expect(snapshot.Name).To(Equal(snapshotName))

		err = waitForSnapshotReady(p, snapshotName)
		Expect(err).To(BeNil(), "Snapshot did not reach READY state: %v", err)

		// Verify the snapshot references the correct source disk.
		snapshot, err = computeService.Snapshots.Get(p, snapshotName).Do()
		Expect(err).To(BeNil(), "Could not re-fetch snapshot")
		Expect(snapshot.SourceDiskId).NotTo(BeEmpty(), "Snapshot should reference a source disk")

		klog.Infof("Snapshot created: name=%s status=%s sourceDisk=%s",
			snapshot.Name, snapshot.Status, snapshot.SourceDisk)
		Expect(snapshot.SourceDisk).To(ContainSubstring(volName),
			"Snapshot source disk should reference the dynamic volume")
	})

})

var _ = Describe("GCE PD CSI Driver Dynamic Volumes Restore from Snapshot", func() {

	It("Should restore a snapshot into a same-size and a larger dynamic volume with the correct resolved disk type", func() {
		testContext := getRandomMwTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		// Create one source dynamic volume and one snapshot, shared by both
		// restore assertions below, to avoid two redundant disk+snapshot round trips.
		srcVolName := testNamePrefix + string(uuid.NewUUID())
		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-balanced",
			parameters.ParameterPDType:  "pd-balanced",
		}
		topology := &csi.TopologyRequirement{
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
		}

		srcVolume, err := client.CreateVolume(srcVolName, params, snapshotSourceSizeGb, topology, nil)
		Expect(err).To(BeNil(), "CreateVolume (source) failed: %v", err)
		srcVolID := srcVolume.VolumeId

		defer func() {
			err := client.DeleteVolume(srcVolID)
			Expect(err).To(BeNil(), "DeleteVolume (source) failed")
			_, err = computeService.Disks.Get(p, z, srcVolName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected source disk to be deleted")
		}()

		// Verify source disk resolved to hyperdisk-balanced.
		srcDisk, err := computeService.Disks.Get(p, z, srcVolName).Do()
		Expect(err).To(BeNil(), "Could not get source disk from GCE API")
		Expect(srcDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected hyperdisk-balanced source disk but got: %s", srcDisk.Type)

		// Take a single snapshot to restore from in both assertions below.
		snapshotName := testNamePrefix + string(uuid.NewUUID())
		snapshotID, err := client.CreateSnapshot(snapshotName, srcVolID, nil)
		Expect(err).To(BeNil(), "CreateSnapshot failed: %v", err)

		defer func() {
			err := client.DeleteSnapshot(snapshotID)
			Expect(err).To(BeNil(), "DeleteSnapshot failed")
		}()

		err = waitForSnapshotReady(p, snapshotName)
		Expect(err).To(BeNil(), "Snapshot did not reach READY state: %v", err)

		contentSource := &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Snapshot{
				Snapshot: &csi.VolumeContentSource_SnapshotSource{
					SnapshotId: snapshotID,
				},
			},
		}

		By("Restoring the snapshot into a same-size dynamic volume")

		restoredVolName := testNamePrefix + string(uuid.NewUUID())
		restoredVolume, err := client.CreateVolume(restoredVolName, params, snapshotSourceSizeGb, topology, contentSource)
		Expect(err).To(BeNil(), "CreateVolume (restore) failed: %v", err)

		defer func() {
			err := client.DeleteVolume(restoredVolume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume (restored) failed")
			_, err = computeService.Disks.Get(p, z, restoredVolName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected restored disk to be deleted")
		}()

		// Verify restored disk has a concrete disk type — not "dynamic".
		restoredDisk, err := computeService.Disks.Get(p, z, restoredVolName).Do()
		Expect(err).To(BeNil(), "Could not get restored disk from GCE API")
		Expect(restoredDisk.Status).To(Equal(readyState), "Restored disk not in READY state")

		klog.Infof("Restored disk type: %s", restoredDisk.Type)
		Expect(restoredDisk.Type).NotTo(ContainSubstring("dynamic"),
			"Restored disk type should be resolved, not 'dynamic'")
		Expect(restoredDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected restored disk to be hyperdisk-balanced but got: %s", restoredDisk.Type)

		By("Restoring the same snapshot into a larger dynamic volume")

		expandedSizeGb := snapshotRestoreLargerSizeGb
		largerVolName := testNamePrefix + string(uuid.NewUUID())
		largerVolume, err := client.CreateVolume(largerVolName, params, expandedSizeGb, topology, contentSource)
		Expect(err).To(BeNil(), "CreateVolume (restore to larger size) failed: %v", err)

		defer func() {
			err := client.DeleteVolume(largerVolume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume (restored larger) failed")
			_, err = computeService.Disks.Get(p, z, largerVolName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected restored larger disk to be deleted")
		}()

		// Verify restored disk has correct disk type and expanded size.
		largerDisk, err := computeService.Disks.Get(p, z, largerVolName).Do()
		Expect(err).To(BeNil(), "Could not get restored larger disk from GCE API")
		Expect(largerDisk.Status).To(Equal(readyState), "Restored larger disk not in READY state")

		klog.Infof("Restored disk: name=%s type=%s sizeGb=%d",
			largerDisk.Name, largerDisk.Type, largerDisk.SizeGb)
		Expect(largerDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected hyperdisk-balanced on restored disk but got: %s", largerDisk.Type)
		Expect(largerDisk.SizeGb).To(Equal(expandedSizeGb),
			"Expected restored disk size %dGb but got %dGb", expandedSizeGb, largerDisk.SizeGb)
	})

})

// No Matching Node Labels
//
// When Preferred topology has no disk-type.gke.io/* labels (only zone),
// selectDiskTypeFromTopologies() finds no HD or PD match and falls back
// to the built-in default disk type (hyperdisk-balanced).
var _ = Describe("GCE PD CSI Driver Dynamic Volumes No Matching Node Labels", func() {

	It("Should fall back to default disk type when Preferred topology has no disk-type labels", func() {
		testContext := getRandomMwTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		volName := testNamePrefix + string(uuid.NewUUID())

		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-balanced",
			parameters.ParameterPDType:  "pd-balanced",
		}

		// Preferred topology has only a zone label — no disk-type.gke.io/* labels.
		// Driver falls back to dts.Default (hyperdisk-balanced).
		volume, err := client.CreateVolume(volName, params, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{"topology.gke.io/zone": z}},
				},
				Preferred: []*csi.Topology{
					{Segments: map[string]string{"topology.gke.io/zone": z}},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume failed: %v", err)
		volID := volume.VolumeId

		defer func() {
			err := client.DeleteVolume(volID)
			Expect(err).To(BeNil(), "DeleteVolume failed")
			project, key, err := common.VolumeIDToKey(volID)
			Expect(err).To(BeNil(), "Failed to parse volume ID")
			_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected disk to be deleted")
		}()

		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from GCE API")
		Expect(cloudDisk.Status).To(Equal(readyState), "Disk not in READY state")

		klog.Infof("No-label fallback resolved to disk type: %s", cloudDisk.Type)
		Expect(cloudDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected default hyperdisk-balanced fallback but got: %s", cloudDisk.Type)
	})

})

// Mixed Node Pool Disk Selection
//
// In a cluster with both HD-capable and PD-only nodes, verifies that the driver
// selects the correct disk type based on the topology of the node the pod is scheduled on.
var _ = Describe("GCE PD CSI Driver Dynamic Volumes Mixed Node Pool Disk Selection", func() {

	It("Should select hyperdisk-balanced on HD node and pd-balanced on PD node in a mixed pool", func() {
		hdContext := getRandomMwTestContext()
		pdContext := getRandomTestContext()

		hdProject, hdZone, _ := hdContext.Instance.GetIdentity()
		pdProject, pdZone, _ := pdContext.Instance.GetIdentity()

		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-balanced",
			parameters.ParameterPDType:  "pd-balanced",
		}

		// --- HD node: pod scheduled on c3-standard-4 → expect hyperdisk-balanced ---
		hdVolName := testNamePrefix + string(uuid.NewUUID())
		hdVolume, err := hdContext.Client.CreateVolume(hdVolName, params, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                        hdZone,
							common.DiskTypeLabelKey("hyperdisk-balanced"): "true",
							common.DiskTypeLabelKey("pd-balanced"):        "true",
						},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                        hdZone,
							common.DiskTypeLabelKey("hyperdisk-balanced"): "true",
							common.DiskTypeLabelKey("pd-balanced"):        "true",
						},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume (HD node) failed: %v", err)
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
		klog.Infof("Mixed pool — HD node resolved to: %s", hdDisk.Type)
		Expect(hdDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected hyperdisk-balanced on HD node but got: %s", hdDisk.Type)

		// --- PD node: pod scheduled on n2d-standard-4 → expect pd-balanced ---
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
		Expect(err).To(BeNil(), "CreateVolume (PD node) failed: %v", err)
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
		klog.Infof("Mixed pool — PD node resolved to: %s", pdDisk.Type)
		Expect(pdDisk.Type).To(ContainSubstring("pd-balanced"),
			"Expected pd-balanced on PD node but got: %s", pdDisk.Type)
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

// Online Resize Test Suite
//
// Background:
// Online resize means expanding a volume while it is mounted and the pod is
// still running — no pod restart required. The CSI flow is:
//  1. ControllerExpandVolume — expands the disk at the GCE API level.
//  2. NodeExpandVolume       — expands the filesystem to use the new space.
//
// After both steps, the new size must be visible inside the mounted filesystem
// without any pod restart.
//
// Note on filesystem size vs disk size:
// GetFSSizeInGb uses "df -BG" which reports usable filesystem space in GiB,
// rounded down. For large disks (e.g. 100 GiB HD), ext4 metadata overhead
// causes df to report ~98 GiB. For small disks (e.g. 5 GiB PD), the overhead
// is negligible and df reports the exact size. We use BeNumerically for HD
// sizes and Equal for small PD sizes to match this behaviour.
//
// This file covers:
//
//	TC-RESIZE-01: Online resize of a hyperdisk-balanced (HD) dynamic volume.
//	TC-RESIZE-02: Online resize of a pd-balanced (PD fallback) dynamic volume.
var _ = Describe("GCE PD CSI Driver Online Resize", func() {

	// -------------------------------------------------------------------------
	// TC-RESIZE-01: Online Resize of HD Dynamic Volume
	//
	// Goal: Expand a mounted hyperdisk-balanced PVC while the pod is running.
	// Verify the new size is reflected inside the pod filesystem without restart.
	//
	// Node type: HD-capable (c3-standard-4) via getRandomMwTestContext().
	// Initial size: defaultHdBSizeGb (100 GiB)
	// Expanded size: 200 GiB
	//
	// Why BeNumerically: df -BG rounds down, so 100 GiB disk reports ~98 GiB
	// and 200 GiB disk reports ~196 GiB due to ext4 metadata overhead.
	// We allow up to 5 GiB tolerance for large HD disks.
	// -------------------------------------------------------------------------
	It("Should online resize a mounted hyperdisk-balanced volume without pod restart [TC-RESIZE-01]", func() {
		Expect(testContexts).ToNot(BeEmpty())

		// HD-capable node required for hyperdisk-balanced.
		testContext := getRandomMwTestContext()
		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		klog.Infof("[TC-RESIZE-01] Using HD-capable node in zone: %s", z)

		// -----------------------------------------------------------------
		// Step 1: Create hyperdisk-balanced volume
		// ProvisionedIOPS and ProvisionedThroughput are mandatory for HD.
		// -----------------------------------------------------------------
		By("Step 1: Creating hyperdisk-balanced volume")
		volName := testNamePrefix + string(uuid.NewUUID())
		klog.Infof("[TC-RESIZE-01] Creating volume: %s", volName)

		params := map[string]string{
			parameters.ParameterKeyType:                          hdbDiskType,
			parameters.ParameterKeyProvisionedIOPSOnCreate:       provisionedIOPSOnCreateHdb,
			parameters.ParameterKeyProvisionedThroughputOnCreate: provisionedThroughputOnCreateHdb,
		}

		volume, err := client.CreateVolume(volName, params, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{constants.TopologyKeyZone: z}},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume (hyperdisk-balanced) failed: %v", err)
		klog.Infof("[TC-RESIZE-01] Volume created: %s", volume.VolumeId)

		// Verify disk exists in GCE with correct initial size.
		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from GCE API")
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultHdBSizeGb),
			"Initial GCE disk size should be %d GiB", defaultHdBSizeGb)
		klog.Infof("[TC-RESIZE-01] Disk verified in GCE: size=%d GiB status=%s",
			cloudDisk.SizeGb, cloudDisk.Status)

		defer func() {
			klog.Infof("[TC-RESIZE-01] Deleting volume: %s", volume.VolumeId)
			client.DeleteVolume(volume.VolumeId)
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(),
				"Expected disk to be deleted")
			klog.Infof("[TC-RESIZE-01] Volume deleted and confirmed gone")
		}()

		// -----------------------------------------------------------------
		// Step 2: Attach (ControllerPublish) the disk to the node
		// -----------------------------------------------------------------
		By("Step 2: Attaching disk to node")
		err = client.ControllerPublishVolumeReadWrite(volume.VolumeId,
			instance.GetNodeID(), false /* forceAttach */)
		Expect(err).To(BeNil(), "ControllerPublishVolume failed")
		klog.Infof("[TC-RESIZE-01] Disk attached to node: %s", instance.GetNodeID())

		defer func() {
			klog.Infof("[TC-RESIZE-01] Detaching disk from node")
			err = client.ControllerUnpublishVolume(volume.VolumeId, instance.GetNodeID())
			if err != nil {
				klog.Errorf("[TC-RESIZE-01] Failed to detach disk: %v", err)
			}
		}()

		// -----------------------------------------------------------------
		// Step 3: Stage the disk (formats and mounts to staging path)
		// -----------------------------------------------------------------
		By("Step 3: Staging disk")
		stageDir := filepath.Join("/tmp/", volName, "stage")
		err = client.NodeStageExt4Volume(volume.VolumeId, stageDir,
			false /* setupDataCache */)
		Expect(err).To(BeNil(), "NodeStageVolume failed")
		klog.Infof("[TC-RESIZE-01] Disk staged at: %s", stageDir)

		defer func() {
			klog.Infof("[TC-RESIZE-01] Unstaging disk")
			err = client.NodeUnstageVolume(volume.VolumeId, stageDir)
			if err != nil {
				klog.Errorf("[TC-RESIZE-01] Failed to unstage volume: %v", err)
			}
			fp := filepath.Join("/tmp/", volName)
			err = testutils.RmAll(instance, fp)
			if err != nil {
				klog.Errorf("[TC-RESIZE-01] Failed to rm %s: %v", fp, err)
			}
		}()

		// -----------------------------------------------------------------
		// Step 4: Mount the disk (simulates pod starting to use the volume)
		// -----------------------------------------------------------------
		By("Step 4: Mounting disk (simulating pod start)")
		publishDir := filepath.Join("/tmp/", volName, "mount")
		err = client.NodePublishVolume(volume.VolumeId, stageDir, publishDir)
		Expect(err).To(BeNil(), "NodePublishVolume failed")
		klog.Infof("[TC-RESIZE-01] Disk mounted at: %s", publishDir)

		defer func() {
			klog.Infof("[TC-RESIZE-01] Unmounting disk")
			err = client.NodeUnpublishVolume(volume.VolumeId, publishDir)
			if err != nil {
				klog.Errorf("[TC-RESIZE-01] NodeUnpublishVolume failed: %v", err)
			}
		}()

		// -----------------------------------------------------------------
		// Step 5: Verify pre-resize filesystem size
		// df -BG rounds down for large disks due to ext4 metadata overhead,
		// so 100 GiB disk reports ~98 GiB. We allow up to 5 GiB tolerance.
		// -----------------------------------------------------------------
		By("Step 5: Verifying pre-resize filesystem size")
		preSizeGb, err := testutils.GetFSSizeInGb(instance, publishDir)
		Expect(err).To(BeNil(), "Failed to get filesystem size")
		klog.Infof("[TC-RESIZE-01] Pre-resize filesystem size: %d GiB (disk: %d GiB, overhead: %d GiB)",
			preSizeGb, defaultHdBSizeGb, defaultHdBSizeGb-preSizeGb)
		// Allow up to 5 GiB difference for ext4 overhead on large HD disks.
		Expect(preSizeGb).To(BeNumerically(">=", defaultHdBSizeGb-5),
			"Pre-resize filesystem size should be within 5 GiB of disk size %d GiB", defaultHdBSizeGb)
		Expect(preSizeGb).To(BeNumerically("<=", defaultHdBSizeGb),
			"Pre-resize filesystem size should not exceed disk size %d GiB", defaultHdBSizeGb)
		klog.Infof("[TC-RESIZE-01] Pre-resize size verified")

		// -----------------------------------------------------------------
		// Step 6: ControllerExpandVolume — expand disk at GCE API level
		// The disk is expanded while the pod is still running (online resize).
		// This is equivalent to the PVC resize request in Kubernetes.
		// -----------------------------------------------------------------
		By("Step 6: Expanding volume at controller level (online — pod still running)")
		var newSizeGb int64 = 200 // expand from 100 GiB to 200 GiB
		klog.Infof("[TC-RESIZE-01] Expanding volume: %d GiB → %d GiB",
			defaultHdBSizeGb, newSizeGb)

		err = client.ControllerExpandVolume(volume.VolumeId, newSizeGb)
		Expect(err).To(BeNil(), "ControllerExpandVolume failed")

		// Verify GCE disk reflects the new size immediately.
		cloudDisk, err = computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk after ControllerExpand")
		Expect(cloudDisk.SizeGb).To(Equal(newSizeGb),
			"GCE disk size should be %d GiB after ControllerExpand", newSizeGb)
		klog.Infof("[TC-RESIZE-01] GCE disk size after ControllerExpand: %d GiB",
			cloudDisk.SizeGb)

		// -----------------------------------------------------------------
		// Step 7: NodeExpandVolume — expand filesystem to use the new space
		// This is the online filesystem resize — happens while pod is running.
		// After this step the new space must be visible inside the pod.
		// -----------------------------------------------------------------
		By("Step 7: Expanding filesystem at node level (online — pod still running)")
		_, err = client.NodeExpandVolume(volume.VolumeId, publishDir, newSizeGb)
		Expect(err).To(BeNil(), "NodeExpandVolume failed")
		klog.Infof("[TC-RESIZE-01] NodeExpandVolume completed")

		// -----------------------------------------------------------------
		// Step 8: Verify new filesystem size inside pod without restart
		// Core assertion: pod sees expanded size without being restarted.
		// Again using BeNumerically — 200 GiB disk reports ~196 GiB via df.
		// -----------------------------------------------------------------
		By("Step 8: Verifying new filesystem size is visible inside pod without restart")
		postSizeGb, err := testutils.GetFSSizeInGb(instance, publishDir)
		Expect(err).To(BeNil(), "Failed to get filesystem size after resize")
		klog.Infof("[TC-RESIZE-01] Post-resize filesystem size: %d GiB (disk: %d GiB, overhead: %d GiB)",
			postSizeGb, newSizeGb, newSizeGb-postSizeGb)
		// Allow up to 5 GiB difference for ext4 overhead on large HD disks.
		Expect(postSizeGb).To(BeNumerically(">=", newSizeGb-5),
			"Post-resize filesystem size should be within 5 GiB of %d GiB", newSizeGb)
		Expect(postSizeGb).To(BeNumerically("<=", newSizeGb),
			"Post-resize filesystem size should not exceed %d GiB", newSizeGb)
		klog.Infof("[TC-RESIZE-01] PASSED — hyperdisk-balanced volume resized %d→%d GiB online",
			defaultHdBSizeGb, newSizeGb)
	})

	// -------------------------------------------------------------------------
	// TC-RESIZE-02: Online Resize of PD Fallback Volume
	//
	// Goal: Expand a mounted pd-balanced PVC while the pod is running.
	// Verify the new size is reflected inside the pod filesystem without restart.
	//
	// Node type: PD-only (n2d-standard-4) via getRandomTestContext().
	// Initial size: defaultSizeGb (5 GiB)
	// Expanded size: 10 GiB
	//
	// Why Equal: For small PD disks (5→10 GiB), df -BG reports exact size
	// because ext4 overhead is negligible relative to disk size. This matches
	// the existing resize_e2e_test.go pattern.
	// -------------------------------------------------------------------------
	It("Should online resize a mounted pd-balanced volume without pod restart [TC-RESIZE-02]", func() {
		Expect(testContexts).ToNot(BeEmpty())

		// PD-only node — pd-balanced works on all node types.
		testContext := getRandomTestContext()
		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client
		instance := testContext.Instance

		klog.Infof("[TC-RESIZE-02] Using PD-only node in zone: %s", z)

		// -----------------------------------------------------------------
		// Step 1: Create pd-balanced volume
		// pd-balanced is the fallback disk type when HD is not available.
		// -----------------------------------------------------------------
		By("Step 1: Creating pd-balanced volume")
		volName := testNamePrefix + string(uuid.NewUUID())
		klog.Infof("[TC-RESIZE-02] Creating volume: %s", volName)

		params := map[string]string{
			parameters.ParameterKeyType: "pd-balanced",
		}

		volume, err := client.CreateVolume(volName, params, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{constants.TopologyKeyZone: z}},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume (pd-balanced) failed: %v", err)
		klog.Infof("[TC-RESIZE-02] Volume created: %s", volume.VolumeId)

		// Verify disk exists in GCE with correct initial size.
		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Could not get disk from GCE API")
		Expect(cloudDisk.Status).To(Equal(readyState))
		Expect(cloudDisk.SizeGb).To(Equal(defaultSizeGb),
			"Initial GCE disk size should be %d GiB", defaultSizeGb)
		klog.Infof("[TC-RESIZE-02] Disk verified in GCE: size=%d GiB status=%s",
			cloudDisk.SizeGb, cloudDisk.Status)

		defer func() {
			klog.Infof("[TC-RESIZE-02] Deleting volume: %s", volume.VolumeId)
			client.DeleteVolume(volume.VolumeId)
			_, err = computeService.Disks.Get(p, z, volName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(),
				"Expected disk to be deleted")
			klog.Infof("[TC-RESIZE-02] Volume deleted and confirmed gone")
		}()

		// -----------------------------------------------------------------
		// Step 2: Attach disk to node
		// -----------------------------------------------------------------
		By("Step 2: Attaching disk to node")
		err = client.ControllerPublishVolumeReadWrite(volume.VolumeId,
			instance.GetNodeID(), false /* forceAttach */)
		Expect(err).To(BeNil(), "ControllerPublishVolume failed")
		klog.Infof("[TC-RESIZE-02] Disk attached to node: %s", instance.GetNodeID())

		defer func() {
			klog.Infof("[TC-RESIZE-02] Detaching disk from node")
			err = client.ControllerUnpublishVolume(volume.VolumeId, instance.GetNodeID())
			if err != nil {
				klog.Errorf("[TC-RESIZE-02] Failed to detach disk: %v", err)
			}
		}()

		// -----------------------------------------------------------------
		// Step 3: Stage the disk
		// -----------------------------------------------------------------
		By("Step 3: Staging disk")
		stageDir := filepath.Join("/tmp/", volName, "stage")
		err = client.NodeStageExt4Volume(volume.VolumeId, stageDir,
			false /* setupDataCache */)
		Expect(err).To(BeNil(), "NodeStageVolume failed")
		klog.Infof("[TC-RESIZE-02] Disk staged at: %s", stageDir)

		defer func() {
			klog.Infof("[TC-RESIZE-02] Unstaging disk")
			err = client.NodeUnstageVolume(volume.VolumeId, stageDir)
			if err != nil {
				klog.Errorf("[TC-RESIZE-02] Failed to unstage volume: %v", err)
			}
			fp := filepath.Join("/tmp/", volName)
			err = testutils.RmAll(instance, fp)
			if err != nil {
				klog.Errorf("[TC-RESIZE-02] Failed to rm %s: %v", fp, err)
			}
		}()

		// -----------------------------------------------------------------
		// Step 4: Mount the disk (simulates pod starting to use the volume)
		// -----------------------------------------------------------------
		By("Step 4: Mounting disk (simulating pod start)")
		publishDir := filepath.Join("/tmp/", volName, "mount")
		err = client.NodePublishVolume(volume.VolumeId, stageDir, publishDir)
		Expect(err).To(BeNil(), "NodePublishVolume failed")
		klog.Infof("[TC-RESIZE-02] Disk mounted at: %s", publishDir)

		defer func() {
			klog.Infof("[TC-RESIZE-02] Unmounting disk")
			err = client.NodeUnpublishVolume(volume.VolumeId, publishDir)
			if err != nil {
				klog.Errorf("[TC-RESIZE-02] NodeUnpublishVolume failed: %v", err)
			}
		}()

		// -----------------------------------------------------------------
		// Step 5: Verify pre-resize filesystem size
		// For small PD disks, df -BG reports exact size (no rounding needed).
		// -----------------------------------------------------------------
		By("Step 5: Verifying pre-resize filesystem size")
		preSizeGb, err := testutils.GetFSSizeInGb(instance, publishDir)
		Expect(err).To(BeNil(), "Failed to get filesystem size")
		klog.Infof("[TC-RESIZE-02] Pre-resize filesystem size: %d GiB (disk: %d GiB)",
			preSizeGb, defaultSizeGb)
		Expect(preSizeGb).To(Equal(defaultSizeGb),
			"Pre-resize filesystem size should be %d GiB", defaultSizeGb)
		klog.Infof("[TC-RESIZE-02] Pre-resize size verified")

		// -----------------------------------------------------------------
		// Step 6: ControllerExpandVolume — expand disk while pod is running
		// -----------------------------------------------------------------
		By("Step 6: Expanding volume at controller level (online — pod still running)")
		var newSizeGb int64 = 10 // expand from 5 GiB to 10 GiB
		klog.Infof("[TC-RESIZE-02] Expanding volume: %d GiB → %d GiB",
			defaultSizeGb, newSizeGb)

		err = client.ControllerExpandVolume(volume.VolumeId, newSizeGb)
		Expect(err).To(BeNil(), "ControllerExpandVolume failed")

		// Verify GCE disk reflects the new size.
		cloudDisk, err = computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "Failed to get disk after ControllerExpand")
		Expect(cloudDisk.SizeGb).To(Equal(newSizeGb),
			"GCE disk size should be %d GiB after ControllerExpand", newSizeGb)
		klog.Infof("[TC-RESIZE-02] GCE disk size after ControllerExpand: %d GiB",
			cloudDisk.SizeGb)

		// -----------------------------------------------------------------
		// Step 7: NodeExpandVolume — expand filesystem while pod is running
		// -----------------------------------------------------------------
		By("Step 7: Expanding filesystem at node level (online — pod still running)")
		_, err = client.NodeExpandVolume(volume.VolumeId, publishDir, newSizeGb)
		Expect(err).To(BeNil(), "NodeExpandVolume failed")
		klog.Infof("[TC-RESIZE-02] NodeExpandVolume completed")

		// -----------------------------------------------------------------
		// Step 8: Verify new size is visible inside pod without restart
		// For small PD disks, df -BG reports exact size — using Equal.
		// -----------------------------------------------------------------
		By("Step 8: Verifying new filesystem size is visible inside pod without restart")
		postSizeGb, err := testutils.GetFSSizeInGb(instance, publishDir)
		Expect(err).To(BeNil(), "Failed to get filesystem size after resize")
		klog.Infof("[TC-RESIZE-02] Post-resize filesystem size: %d GiB (disk: %d GiB)",
			postSizeGb, newSizeGb)
		Expect(postSizeGb).To(Equal(newSizeGb),
			"Post-resize filesystem size should be %d GiB", newSizeGb)
		klog.Infof("[TC-RESIZE-02] PASSED — pd-balanced volume resized %d→%d GiB online",
			defaultSizeGb, newSizeGb)
	})

})
