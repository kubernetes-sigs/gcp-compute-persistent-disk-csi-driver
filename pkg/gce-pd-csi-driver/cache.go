package gceGCEDriver

import (
	"fmt"
	"strconv"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"

	"k8s.io/klog/v2"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

const (
	cacheSuffix        = "csi-fast"
	mainLvSuffix       = "csi-main"
	raidedLocalSsdName = "csi-driver-data-cache"
	raidMode           = "0"
	raidedLssdPrefix   = "/dev/md/"
	raidedLocalSsdPath = raidedLssdPrefix + raidedLocalSsdName
)

func setupCaching(devicePath string, req *csi.NodeStageVolumeRequest, nodeId string) (string, error) {
	volumeId := req.GetVolumeId()
	volumeGroupName := getVolumeGroupName(nodeId)
	mainDevicePath := "/dev/" + volumeGroupName + "/" + getLvName(mainLvSuffix, volumeId)
	mainLvName := getLvName(mainLvSuffix, volumeId)
	klog.V(2).Infof("====== Start LVM PoC NodeStageVolume Steps ======")
	klog.V(2).Infof("====== volumeGroupName is %v ======", volumeGroupName)

	klog.V(2).Infof("====== vgscan before vgcreate ======")
	args := []string{}
	info, err := common.RunCommand("vgscan", args...)
	if err != nil {
		klog.Errorf("vgscan error %v: %s", err, info)
	}
	klog.V(2).Infof("====== vgscan info contains volumeGroupName or not %v ======", strings.Contains(string(info), volumeGroupName))
	// Check if the required volume group already exists
	if strings.Contains(string(info), volumeGroupName) {
		klog.V(2).Infof("============= VG exists, now check if PD is part of VG============")

		// Clean up Volume Group before adding the PD
		reduceVolumeGroup(volumeGroupName, true)
	} else {
		err := createVg(volumeGroupName, devicePath, raidedLocalSsdPath)
		if err != nil {
			return mainDevicePath, err
		}
	}

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

		reduceVolumeGroup(vgNameForPv, false)
		// Uncache LV
		args = []string{
			"--uncache",
			vgNameForPv + "/" + mainLvName,
			"--force",
		}
		info, err = common.RunCommand("lvconvert", args...)
		if err != nil {
			klog.Errorf("errored while uncaching main LV %v: %s", err, info)
			// On error info contains the error message which we cannot use for further steps
		}

		reduceVolumeGroup(vgNameForPv, false)
		klog.V(2).Infof("==========Merge VG %v to Node VG %v==========", vgNameForPv, volumeGroupName)
		info, err = common.RunCommand("vgmerge", []string{volumeGroupName, vgNameForPv}...)
		if err != nil {
			klog.Errorf("Errored while merging Volume group %s into %s %v: %s", vgNameForPv, volumeGroupName, err, info)
		}

	} else {
		klog.V(2).Infof("==========Extend Node VG %v for PV %v==========", volumeGroupName, devicePath)
		info, err := common.RunCommand("vgextend", []string{volumeGroupName, devicePath}...)
		if err != nil {
			klog.Errorf("Errored while extending VGs %v: %s", err, info)
		}
	}

	// Create LV if not already created
	args = []string{
		"--select",
		"vg_name=" + volumeGroupName,
		"-o",
		"lv_name",
	}
	lvList, err := common.RunCommand("lvs", args...)
	if err != nil {
		return mainDevicePath, fmt.Errorf("lv list error %w: %s", err, info)
	}
	klog.Infof("=============== Got LVs %s on Volume group %s ============", string(lvList), volumeGroupName)
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
			return mainDevicePath, fmt.Errorf("lvcreate error %w: %s", err, info)
		}

	}
	cachePoolName := raidedLocalSsdPath
	// Verify caching is setup for PD
	args = []string{
		"--select",
		"lv_name=" + mainLvName,
		"-o",
		"pool_lv",
	}
	poolName, err := common.RunCommand("lvs", args...)
	if err != nil {
		return mainDevicePath, fmt.Errorf("lvcreate error %w: %s", err, info)
	}
	cacheLvName := getLvName(cacheSuffix, volumeId)
	if strings.Contains(string(poolName), "csi-fast") {
		// Validate that cache is setup for required size
		klog.V(2).Infof("================Validate Cache is setup for correct size and mode===============")
	} else {
		fastCacheSize := req.GetPublishContext()[common.ContexLocalSsdCacheSize]
		chunkSize := "960" // Cannot use default chunk size(64KiB) as it errors on maxChunksAllowed. Unit - KiB
		klog.V(2).Infof("====== fastCacheSize is %v ======", fastCacheSize)
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
		klog.V(2).Infof("====== lvconvert fast and main to cache ======")
		args = []string{
			"--type",
			"cache",
			"--cachevol",
			cacheLvName,
			"--zero",
			"y",
			"--cachemode",
			req.GetPublishContext()[common.ContextDataCacheMode],
			volumeGroupName + "/" + mainLvName,
			"--chunksize",
			string(chunkSize),
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

func cleanupCache(volumeId string, nodeId string) error {

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
		return fmt.Errorf("errored while uncaching the disk %w: %s", err, info)
	}
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

func createVg(volumeGroupName string, devicePath string, raidedLocalSsds string) error {
	// No existing volume group
	klog.V(2).Infof("====== vgcreate ======")
	args := []string{
		"--zero",
		"y",
		volumeGroupName,
		raidedLocalSsds,
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

func reduceVolumeGroup(volumeGroupName string, force bool) {
	klog.V(2).Infof("=========Cleanup VG========")
	args := []string{
		"--removemissing",
		volumeGroupName,
	}
	if force {
		args = append(args, "--force")
	}
	info, err := common.RunCommand("vgreduce", args...)
	if err != nil {
		klog.Errorf("Errored while cleaning up volume group %v: %s", err, info)
	}
}

func RaidLocalSsds() error {
	isAlreadyRaided, err := isRaided()
	if err != nil {
		klog.V(2).Infof("======Errored while scanning for available LocalSSDs err:%v; continuing Raiding=======", err)
	} else if isAlreadyRaided {
		klog.V(2).Infof("===============Local SSDs are already RAIDed==============")
		return nil
	}
	info, err := common.RunCommand("nvme", []string{"list", "-o", "json"}...)
	if err != nil {
		return fmt.Errorf("errored while scanning available NVME disks info: %v; err:%v", info, err)
	}
	infoString := strings.TrimSpace(strings.ReplaceAll(string(info), "\n", " "))
	klog.V(2).Infof("=============NVME list %v ============", infoString)
	infoString = strings.ReplaceAll(infoString, "\"", "")
	infoString = strings.ReplaceAll(infoString, " :", ":")
	infoString = strings.ReplaceAll(infoString, ": ", ":")
	infoString = strings.ReplaceAll(infoString, ",", " ")
	infoSlice := strings.Split(infoString, " ")
	diskList := []string{}
	for _, diskInfo := range infoSlice {
		diskName := strings.TrimSpace(diskInfo)
		if strings.Contains(diskName, "DevicePath") {
			diskName := strings.TrimSpace(strings.Split(diskName, ":")[1])
			diskList = append(diskList, diskName)
		}
	}
	nvmeDiskCount := len(diskList)
	nvmeDiskList := strings.Join(diskList, " ")
	klog.V(2).Infof("========= nvmeDiskCount %v; nvmeDiskList: %v; diskList %v================", nvmeDiskCount, nvmeDiskList, diskList)
	args := []string{
		"--create",
		raidedLssdPrefix + raidedLocalSsdName,
		"-l" + raidMode,
		// Force RAIDing as sometime it might fail for caution if there is just 1 LSSD present as 1 LSSD need not be RAIDed
		"--force",
		"-n",
		strconv.Itoa(nvmeDiskCount),
	}
	args = append(args, diskList...)
	info, err = common.RunCommand("mdadm", args...)
	if err != nil {
		return fmt.Errorf("errored while RAIDing LSSDs info: %v; err:%v", info, err)
	}
	// Validate if Raided successfully
	isAlreadyRaided, err = isRaided()
	if err != nil {
		klog.V(2).Infof("======Errored while scanning for available raided LocalSSDs err:%v=======", err)
	}
	if !isAlreadyRaided {
		return fmt.Errorf("failed raiding, raided device not found on scanning")
	}
	return nil
}

func isRaided() (bool, error) {
	args := []string{
		"--detail",
		"--scan",
	}
	info, err := common.RunCommand("mdadm", args...)
	if err != nil {
		return false, fmt.Errorf("errored while scanning for raided LSSD %v: %s", err, info)
	}
	klog.V(2).Infof("=========== Got LSSDs %v===========", string(info))
	if info != nil && strings.Contains(string(info), raidedLocalSsdName) {
		return true, nil
	}
	return false, nil
}
