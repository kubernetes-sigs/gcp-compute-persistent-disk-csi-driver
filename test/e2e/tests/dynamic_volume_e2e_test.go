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
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/parameters"
	testutils "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/e2e/utils"

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

// Pod Migration with Dynamic PVC
//
// Verifies that data written on node A persists after the pod is migrated to node B,
// and that the disk type remains unchanged throughout.
var _ = Describe("GCE PD CSI Driver Dynamic Volumes Pod Migration", func() {

	It("Should persist data and retain disk type after pod migration from node A to node B", func() {
		Expect(testContexts).To(HaveLen(2),
			"Need at least 2 PD nodes for pod migration test")

		tcA := testContexts[0]
		tcB := testContexts[1]

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
					{Segments: map[string]string{"topology.gke.io/zone": zA}},
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
					{Segments: map[string]string{"topology.gke.io/zone": z}},
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
		volume, err := client.CreateVolume(volName, params, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{"topology.gke.io/zone": z}},
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

	It("Should restore a snapshot into a new dynamic volume with the correct resolved disk type", func() {
		testContext := getRandomMwTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		// Create source dynamic volume.
		srcVolName := testNamePrefix + string(uuid.NewUUID())
		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-balanced",
			parameters.ParameterPDType:  "pd-balanced",
		}

		srcVolume, err := client.CreateVolume(srcVolName, params, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{"topology.gke.io/zone": z}},
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

		// Take a snapshot.
		snapshotName := testNamePrefix + string(uuid.NewUUID())
		snapshotID, err := client.CreateSnapshot(snapshotName, srcVolID, nil)
		Expect(err).To(BeNil(), "CreateSnapshot failed: %v", err)

		defer func() {
			err := client.DeleteSnapshot(snapshotID)
			Expect(err).To(BeNil(), "DeleteSnapshot failed")
		}()

		err = waitForSnapshotReady(p, snapshotName)
		Expect(err).To(BeNil(), "Snapshot did not reach READY state: %v", err)

		// Restore: create a new dynamic volume from the snapshot.
		restoredVolName := testNamePrefix + string(uuid.NewUUID())
		restoredVolume, err := client.CreateVolume(restoredVolName, params, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{"topology.gke.io/zone": z}},
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
			},
			&csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{
					Snapshot: &csi.VolumeContentSource_SnapshotSource{
						SnapshotId: snapshotID,
					},
				},
			})
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
	})

})

var _ = Describe("GCE PD CSI Driver Dynamic Volumes Restore Snapshot to Larger Size", func() {

	It("Should restore a snapshot into a larger dynamic volume with correct disk type and size", func() {
		testContext := getRandomMwTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		// Create source dynamic volume at base size.
		srcVolName := testNamePrefix + string(uuid.NewUUID())
		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-balanced",
			parameters.ParameterPDType:  "pd-balanced",
		}

		srcVolume, err := client.CreateVolume(srcVolName, params, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{"topology.gke.io/zone": z}},
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
		Expect(err).To(BeNil(), "CreateVolume (source) failed: %v", err)
		srcVolID := srcVolume.VolumeId

		defer func() {
			err := client.DeleteVolume(srcVolID)
			Expect(err).To(BeNil(), "DeleteVolume (source) failed")
			_, err = computeService.Disks.Get(p, z, srcVolName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected source disk to be deleted")
		}()

		srcDisk, err := computeService.Disks.Get(p, z, srcVolName).Do()
		Expect(err).To(BeNil(), "Could not get source disk from GCE API")
		Expect(srcDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected hyperdisk-balanced source disk but got: %s", srcDisk.Type)

		// Take a snapshot.
		snapshotName := testNamePrefix + string(uuid.NewUUID())
		snapshotID, err := client.CreateSnapshot(snapshotName, srcVolID, nil)
		Expect(err).To(BeNil(), "CreateSnapshot failed: %v", err)

		defer func() {
			err := client.DeleteSnapshot(snapshotID)
			Expect(err).To(BeNil(), "DeleteSnapshot failed")
		}()

		err = waitForSnapshotReady(p, snapshotName)
		Expect(err).To(BeNil(), "Snapshot did not reach READY state: %v", err)

		// Restore into a larger volume (2x the source size).
		expandedSizeGb := defaultHdBSizeGb * 2
		restoredVolName := testNamePrefix + string(uuid.NewUUID())
		restoredVolume, err := client.CreateVolume(restoredVolName, params, expandedSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{"topology.gke.io/zone": z}},
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
			},
			&csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{
					Snapshot: &csi.VolumeContentSource_SnapshotSource{
						SnapshotId: snapshotID,
					},
				},
			})
		Expect(err).To(BeNil(), "CreateVolume (restore to larger size) failed: %v", err)

		defer func() {
			err := client.DeleteVolume(restoredVolume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume (restored) failed")
			_, err = computeService.Disks.Get(p, z, restoredVolName).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected restored disk to be deleted")
		}()

		// Verify restored disk has correct disk type and expanded size.
		restoredDisk, err := computeService.Disks.Get(p, z, restoredVolName).Do()
		Expect(err).To(BeNil(), "Could not get restored disk from GCE API")
		Expect(restoredDisk.Status).To(Equal(readyState), "Restored disk not in READY state")

		klog.Infof("Restored disk: name=%s type=%s sizeGb=%d",
			restoredDisk.Name, restoredDisk.Type, restoredDisk.SizeGb)
		Expect(restoredDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected hyperdisk-balanced on restored disk but got: %s", restoredDisk.Type)
		Expect(restoredDisk.SizeGb).To(Equal(expandedSizeGb),
			"Expected restored disk size %dGb but got %dGb", expandedSizeGb, restoredDisk.SizeGb)
	})

})
