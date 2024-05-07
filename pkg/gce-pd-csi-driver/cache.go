package gceGCEDriver

import (
	"fmt"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"

	"k8s.io/klog/v2"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

const cacheSuffix = "csi-fast"
const mainLvSuffix = "csi-main"

func SetupCaching(devicePath string, req *csi.NodeStageVolumeRequest, nodeId string) (string, error) {
	volumeId := req.GetVolumeId()
	volumeGroupName := getVolumeGroupName(nodeId)
	mainDevicePath := "/dev/" + volumeGroupName + "/" + getLvName(mainLvSuffix, volumeId)
	klog.V(2).Infof("====== Start LVM PoC NodeStageVolume Steps ======")
	klog.V(2).Infof("====== volumeGroupName is %v ======", volumeGroupName)

	klog.V(2).Infof("====== vgscan before vgcreate ======")
	args := []string{}
	info, err := common.RunCommand("vgscan", args...)
	if err != nil {
		klog.Errorf("vgscan error %v: %s", err, info)
	}
	klog.V(2).Infof("====== vgscan info %v  ======", string(info))
	klog.V(2).Infof("====== vgscan info contains volumeGroupName or not %v ======", strings.Contains(string(info), volumeGroupName))
	// Check if the required volume group already exists
	if strings.Contains(string(info), volumeGroupName) {
		klog.V(2).Infof("============= VG exists, now check if PD is part of VG============")

		// Clean up Volume Group before adding the PD
		reduceVolumeGroup(volumeGroupName)
	} else {
		err := createVg(volumeGroupName, devicePath)
		if err != nil {
			return mainDevicePath, err
		}
		// VG doesn't exist so throw error
		klog.Errorf("No VG on Node, ensure Volume group on node")
	}

	// Verify if the PD is already in the required volume group
	// args = []string{
	// 	"--select",
	// 	"vg_name",
	// 	volumeGroupName
	// }
	// vgPvs, err := common.RunCommand("pvs", args...)
	// if err != nil {
	// 	klog.Errorf("Errored while checking Physical Volume(PV)s for the volume group %v: %s", err, vgPvs)
	// }
	// if strings.Contains(string(vgPvs), devicePath) {
	// 	klog.V(2).Infof("====Physical Volume(PV) already exists in the Volume Group=====")
	// } else {

	// Check if the Physical Volume(PV) is part of some other volume group
	args = []string{
		"--select",
		"pv_name=" + devicePath,
		"-o",
		"vg_name",
	}
	info, err = common.RunCommand("pvs", args...)
	if err != nil {
		klog.Errorf("errored while checking physical volume details %v: %s", err, info)
		// On error info contains the error message which we cannot use for further steps
	}

	klog.V(2).Infof("==========Got Volume group details from PV %s=======", info)

	infoString := strings.TrimSpace(strings.ReplaceAll(string(info), "\n", " "))
	infoString = strings.ReplaceAll(infoString, ".", "")
	infoString = strings.ReplaceAll(infoString, "\"", "")
	infoSlice := strings.Split(strings.TrimSpace(infoString), " ")
	klog.V(2).Info("============ infoSlice %s =============", infoSlice[(len(infoSlice)-1)], infoSlice)
	vgNameForPv := strings.TrimSpace(infoSlice[(len(infoSlice) - 1)])
	klog.V(2).Infof("============ Physical volume is part of Volume group: %v=======", vgNameForPv)
	if vgNameForPv == volumeGroupName {
		klog.V(2).Infof("====Physical Volume(PV) already exists in the Volume Group=====")
	} else if vgNameForPv != "VG" && vgNameForPv != "" {

		klog.V(2).Infof("=========Deactivate VG %s========", vgNameForPv)
		info, err = common.RunCommand("vgchange", []string{"-an", vgNameForPv}...)
		if err != nil {
			klog.Errorf("Errored while deactivating VG %v: err: %v: %s", vgNameForPv, err, info)
		}

		reduceVolumeGroup(vgNameForPv)
		klog.V(2).Infof("==========Merge VG %v to Node VG %v==========", vgNameForPv, volumeGroupName)
		info, err = common.RunCommand("vgmerge", []string{volumeGroupName, vgNameForPv}...)
		if err != nil {
			klog.Errorf("Errored while merging Volume group %s into %s %v: %s", vgNameForPv, volumeGroupName, err, info)
		}

		klog.V(2).Infof("==========Remove VG from node %v ==========", vgNameForPv)
		info, err = common.RunCommand("cgremove", []string{vgNameForPv, "-y"}...)
		if err != nil {
			klog.Errorf("Errored while removing Volume group %s: info:%s, error:%v", vgNameForPv, err, info)
		}

	} else {
		klog.V(2).Infof("==========Extend Node VG %v for PV %v==========", volumeGroupName, devicePath)
		info, err := common.RunCommand("vgextend", []string{volumeGroupName, devicePath}...)
		if err != nil {
			klog.Errorf("Errored while extending VGs %v: %s", err, info)
		}
	}

	mainLvName := getLvName(mainLvSuffix, volumeId)
	// Create LV if not already created
	args = []string{
		"--select",
		"vg_name=" + volumeGroupName,
		"-o",
		"lv_name",
	}
	lvList, err := common.RunCommand("lvs", args...)
	if err != nil {
		klog.V(2).Infof("====== lvs error %v: %s ======", err, info)
		return mainDevicePath, fmt.Errorf("lv list error %w: %s", err, info)
	}
	klog.Info("=============== Got LVs %s on Volume group %s ============", lvList, volumeGroupName)
	if !strings.Contains(string(lvList), mainLvName) {
		// lvcreate -n main -l 100%PVS cachegroup /dev/sdb
		klog.V(2).Infof("====== lvcreate main cache layer ======")
		args = []string{
			"--yes",
			"-n",
			mainLvName,
			"-l",
			"100%PVS",
			volumeGroupName,
			devicePath,
		}
		info, err = common.RunCommand("lvcreate", args...)
		if err != nil {
			klog.V(2).Infof("====== lvcreate error %v: %s ======", err, info)
			return mainDevicePath, fmt.Errorf("lvcreate error %w: %s", err, info)
		}

	}
	// Replace this with RAIDed Local SSD
	cachePoolName := "dev/nvme0n1"
	// Verify caching is setup for PD
	args = []string{
		"--select",
		"lv_name=" + mainLvName,
		"-o",
		"pool_lv",
	}
	poolName, err := common.RunCommand("lvs", args...)
	if err != nil {
		klog.V(2).Infof("====== lvcreate error %v: %s ======", err, info)
		return mainDevicePath, fmt.Errorf("lvcreate error %w: %s", err, info)
	}
	cacheLvName := getLvName(cacheSuffix, volumeId)
	if strings.Contains(string(poolName), "csi-fast") {
		// Validate that cache is setup for required size
		klog.V(2).Infof("================Validate Cache is setup for correct size and mode===============")
	} else {
		fastCacheSize := req.GetPublishContext()[contexLocalSsdCacheSize] + "Gi"
		klog.V(2).Infof("====== fastCacheSize is %v ======", fastCacheSize)
		// lvcreate -n fast -L 50G cachegroup /dev/nvme0n1
		klog.V(2).Infof("====== lvcreate fast cache layer again with the VolumeGroup %v======", volumeGroupName)
		args = []string{
			"--yes",
			"-n",
			cacheLvName,
			"-L",
			fastCacheSize,
			volumeGroupName,
			cachePoolName,
		}
		info, err = common.RunCommand("lvcreate", args...)
		if err != nil {
			klog.V(2).Infof("====== lvcreate error %v: %s ======", err, info)
			return mainDevicePath, fmt.Errorf("lvcreate error %w: %s", err, info)
		}

		// Once caching is setup, link the PD to cache
		// lvconvert --type cache --cachevol fast --zero y --cachemode writethrough cachegroup/main --force -y
		klog.V(2).Infof("====== lvconvert fast and main to cache ======")
		args = []string{
			"--type",
			"cache",
			"--cachevol",
			cacheLvName,
			"--zero",
			"y",
			"--cachemode",
			req.GetPublishContext()[contextDataCacheMode],
			volumeGroupName + "/" + mainLvName,
			"--force",
			"-y",
		}
		info, err = common.RunCommand("lvconvert", args...)
		if err != nil {
			klog.V(2).Infof("====== lvconvert error %v: %s ======", err, info)
			return mainDevicePath, fmt.Errorf("lvconvert error %w: %s", err, info)
		}
	}

	// activate all the LVs in the Volume group
	klog.V(2).Infof("====== Activate Volume group %s ======", volumeGroupName)
	info, err = common.RunCommand("vgchange", []string{"-ay", volumeGroupName}...)
	if err != nil {
		klog.Errorf("Failed to activate VG %v %v:%s", volumeGroupName, err, info)
	}

	return mainDevicePath, nil
}

func CleanupCache(volumeId string, nodeId string) error {

	volumeGroupName := getVolumeGroupName(nodeId)
	klog.V(2).Infof("=============Deactivating volume %s/%s=====", volumeGroupName, volumeId)
	args := []string{
		"-an",
		"/dev/" + volumeGroupName + "/" + getLvName(mainLvSuffix, volumeId),
	}
	info, err := common.RunCommand("lvchange", args...)
	if err != nil {
		klog.Errorf("Errored while deactivating the disk  %v: %s", err, info)
	}
	args = []string{
		"--uncache",
		volumeGroupName + "/" + getLvName(mainLvSuffix, volumeId),
	}
	info, err = common.RunCommand("lvconvert", args...)
	if err != nil {
		klog.Errorf("Errored while uncaching the disk  %v: %s", err, info)
		return fmt.Errorf("errored while uncaching the disk %w: %s", err, info)
	}
	reduceVolumeGroup(volumeGroupName)
	return nil
}

func getVolumeGroupName(nodePath string) string {
	nodeSlice := strings.Split(nodePath, "/")
	nodeId := nodeSlice[len(nodeSlice)-1]
	return fmt.Sprintf("csi-vg-%s", nodeId)
}

func getLvName(suffix string, volumeId string) string {
	pvcNameStringSlice := strings.Split(volumeId, "/")
	pvcName := pvcNameStringSlice[len(pvcNameStringSlice)-1]
	return fmt.Sprintf("%s-%s", suffix, pvcName)
}

func createVg(volumeGroupName string, devicePath string) error {
	// No existing volume group
	klog.V(2).Infof("====== vgcreate ======")
	args := []string{
		"--zero",
		"y",
		volumeGroupName,
		"/dev/nvme0n1",
	}
	info, err := common.RunCommand("vgcreate", args...)
	if err != nil {
		klog.Errorf("vgcreate error %v: %s", err, info)
		return fmt.Errorf("vgcreate error %w: %s", err, info)
	}

	klog.V(2).Infof("====== vgscan after vgcreate ======")
	args = []string{}
	info, err = common.RunCommand("vgscan", args...)
	if err != nil {
		klog.Errorf("vgscan error %v: %s", err, info)
	} else {
		klog.V(2).Infof("====== vgscan info %s  ======", info)
	}
	return nil
}

func reduceVolumeGroup(volumeGroupName string) {
	klog.V(2).Infof("=========Cleanup VG========")
	args := []string{
		"--removemissing",
		"--force",
		volumeGroupName,
	}
	info, err := common.RunCommand("vgreduce", args...)
	if err != nil {
		klog.Errorf("Errored while cleaning up volume group %v: %s", err, info)
	}
}
