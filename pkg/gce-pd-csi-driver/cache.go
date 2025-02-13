package gceGCEDriver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	fsnotify "github.com/fsnotify/fsnotify"

	"k8s.io/klog/v2"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

const (
	cacheSuffix        = "csi-fast"
	mainLvSuffix       = "csi-main"
	raidedLocalSsdName = "csi-driver-data-cache"
	raidMode           = "0"
	raidedLssdPrefix   = "/dev/md/"
)

var raidedLocalSsdPath = raidedLssdPrefix + raidedLocalSsdName

func setupCaching(devicePath string, req *csi.NodeStageVolumeRequest, nodeId string) (string, error) {
	volumeId := req.GetVolumeId()
	volumeGroupName := GetVolumeGroupName(nodeId)
	mainDevicePath := "/dev/" + volumeGroupName + "/" + getLvName(mainLvSuffix, volumeId)
	mainLvName := getLvName(mainLvSuffix, volumeId)
	klog.V(2).Infof("============================== Start LVM PoC NodeStageVolume Steps ==============================")
	klog.V(2).Infof("============================== volumeGroupName is %v ==============================", volumeGroupName)

	info, err := common.RunCommand("grep", raidedLocalSsdName, "ls", raidedLssdPrefix)
	if err != nil {
		klog.Errorf("================== failed while listing raided devices, err: %v, output:%v ===============", err, info)
	}
	infoString := strings.TrimSpace(string(info))
	klog.V(2).Infof("=================== Got Raided LSSD name %v ===================", infoString)
	raidedLocalSsdPath = raidedLssdPrefix + infoString

	klog.V(2).Infof("============================== VG exists, now check if PD is part of VG ==============================")
	// Check if the Physical Volume(PV) is part of some other volume group
	args := []string{
		"--select",
		"pv_name=" + devicePath,
		"-o",
		"vg_name",
	}
	info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "pvs", args...)
	if err != nil {
		klog.Errorf("errored while checking physical volume details %v: %s", err, info)
		// On error info contains the error message which we cannot use for further steps
		info = nil
	}

	infoString = strings.TrimSpace(strings.ReplaceAll(string(info), "\n", " "))
	infoString = strings.ReplaceAll(infoString, ".", "")
	infoString = strings.ReplaceAll(infoString, "\"", "")
	infoSlice := strings.Split(strings.TrimSpace(infoString), " ")
	vgNameForPv := strings.TrimSpace(infoSlice[(len(infoSlice) - 1)])
	klog.V(2).Infof("============================== Physical volume is part of Volume group: %v ==============================", vgNameForPv)
	if vgNameForPv == volumeGroupName {
		klog.V(2).Infof("============================== Physical Volume(PV) already exists in the Volume Group ==============================")
	} else if vgNameForPv != "VG" && vgNameForPv != "" {

		klog.V(2).Infof("============================== Deactivate VG %s ==============================", vgNameForPv)
		info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgchange", []string{"-an", vgNameForPv}...)
		if err != nil {
			klog.Errorf("Errored while deactivating VG %v: err: %v: %s", vgNameForPv, err, info)
		}

		ReduceVolumeGroup(vgNameForPv, false)
		_, isCached := isCachingSetup(mainLvName)
		// We will continue to uncache even if it errors to check caching as it is not a terminal issue.

		if isCached {
			klog.Infof("============================== Uncaching the LV %v==============================", mainLvName)
			// Uncache LV
			args = []string{
				"--uncache",
				vgNameForPv + "/" + mainLvName,
				"--force",
				"-y", // force remove cache without flushing data
			}
			info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "lvconvert", args...)
			if err != nil {
				klog.Errorf("errored while uncaching main LV. %v: %s", err, info)
			}

			ReduceVolumeGroup(vgNameForPv, false)
		}
		klog.V(2).Infof("============================== Merge VG %v to Node VG %v ==============================", vgNameForPv, volumeGroupName)
		info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgmerge", []string{volumeGroupName, vgNameForPv}...)
		if err != nil {
			klog.Errorf("Errored while merging Volume group %s into %s %v: %s", vgNameForPv, volumeGroupName, err, info)
		}

	} else {
		klog.V(2).Infof("============================== Extend Node VG %v for PV %v ==============================", volumeGroupName, devicePath)
		info, err := common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgextend", []string{volumeGroupName, devicePath}...)
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
	lvList, err := common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "lvs", args...)
	if err != nil {
		return mainDevicePath, fmt.Errorf("lv list error %w: %s", err, info)
	}
	klog.Infof("==============================Got LVs %s on Volume group %s ==============================", string(lvList), volumeGroupName)
	if !strings.Contains(string(lvList), mainLvName) {
		// lvcreate -n main -l 100%PVS cachegroup /dev/sdb
		klog.V(2).Infof("============================== lvcreate main cache layer ==============================")
		args = []string{
			"--yes",
			"-n",
			mainLvName,
			"-l",
			"100%PVS",
			volumeGroupName,
			devicePath,
		}
		info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "lvcreate", args...)
		if err != nil {
			return mainDevicePath, fmt.Errorf("lvcreate error %w: %s", err, info)
		}

	}
	err, isCached := isCachingSetup(mainLvName)
	if err != nil {
		klog.Errorf("faild to check if caching ius setup for LV. Continuing to setup caching.")
	}
	cacheLvName := getLvName(cacheSuffix, volumeId)
	if isCached {
		// Validate that cache is setup for required size
		klog.V(2).Infof("==============================Assuming valid data cache size and mode, resizing is not supported==============================")
	} else {
		fastCacheSize := req.GetPublishContext()[common.ContexLocalSsdCacheSize]
		chunkSize := "960" // Cannot use default chunk size(64KiB) as it errors on maxChunksAllowed. Unit - KiB
		klog.V(2).Infof("============================== fastCacheSize is %v GiB ==============================", fastCacheSize)
		klog.V(2).Infof("============================== lvcreate fast cache layer again with the VolumeGroup %v==============================", volumeGroupName)
		args = []string{
			"--yes",
			"-n",
			cacheLvName,
			"-L",
			// ConvertGiStringToInt64 converts the input size to GiB so default to "g" for cache size - LVM g|G is GiB.
			fastCacheSize + "g",
			volumeGroupName,
			raidedLocalSsdPath,
		}
		info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "lvcreate", args...)
		if err != nil {
			klog.V(2).Infof("============================== lvcreate error %v: %s ==============================", err, info)
			return mainDevicePath, fmt.Errorf("lvcreate error %w: %s", err, info)
		}

		// Once caching is setup, link the PD to cache
		klog.V(2).Infof("============================== lvconvert fast and main to cache ==============================")
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
		info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "lvconvert", args...)
		if err != nil {
			klog.V(2).Infof("============================== lvconvert error %v: %s ==============================", err, info)
			return mainDevicePath, fmt.Errorf("lvconvert error %w: %s", err, info)
		}
	}

	// activate all the LVs in the Volume group
	klog.V(2).Infof("============================== Activate Volume group %s ==============================", volumeGroupName)
	info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgchange", []string{"-ay", volumeGroupName}...)
	if err != nil {
		klog.Errorf("Failed to activate VG %v %v:%s", volumeGroupName, err, info)
	}

	return mainDevicePath, nil
}

func CheckVgExists(volumeGroupName string) bool {
	args := []string{}
	info, err := common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgscan", args...)
	if err != nil {
		klog.Errorf("vgscan error %v: %s", err, info)
		return false
	}
	// Check if the required volume group already exists
	return strings.Contains(string(info), volumeGroupName)
}

func cleanupCache(volumeId string, nodeId string) error {

	volumeGroupName := GetVolumeGroupName(nodeId)
	if !CheckVgExists(volumeGroupName) {
		// If volume group doesn't exist then there's nothing to uncache
		return nil
	}
	mainLvName := getLvName(mainLvSuffix, volumeId)
	klog.V(2).Infof("============================== Deactivating volume %s/%s ==============================", volumeGroupName, mainLvName)
	args := []string{
		"-an",
		"/dev/" + volumeGroupName + "/" + mainLvName,
	}
	info, err := common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "lvchange", args...)
	if err != nil {
		klog.Errorf("Errored while deactivating the disk  %v: %s", err, info)
	}
	args = []string{
		"--uncache",
		volumeGroupName + "/" + mainLvName,
		"-y",
	}
	info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "lvconvert", args...)
	if err != nil {
		return fmt.Errorf("errored while uncaching the disk %w: %s", err, info)
	}
	return nil
}

func GetVolumeGroupName(nodePath string) string {
	nodeSlice := strings.Split(nodePath, "/")
	nodeId := nodeSlice[len(nodeSlice)-1]
	nodeHash := common.ShortString(nodeId)
	return fmt.Sprintf("csi-vg-%s", nodeHash)
}

func getLvName(suffix string, volumeId string) string {
	pvcNameStringSlice := strings.Split(volumeId, "/")
	pvcName := pvcNameStringSlice[len(pvcNameStringSlice)-1]
	return fmt.Sprintf("%s-%s", suffix, pvcName)
}

func CreateVg(volumeGroupName string, raidedLocalSsds string) error {
	klog.V(2).Infof("============================== vgcreate ==============================")
	args := []string{
		"--zero",
		"y",
		volumeGroupName,
		raidedLocalSsds,
		"-v",
	}
	info, err := common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgcreate", args...)
	if err != nil {
		klog.Errorf("vgcreate error %v: %s", err, info)
		return fmt.Errorf("vgcreate error %w: %s", err, info)
	}
	klog.Infof("Volume group creation succeeded for %v", volumeGroupName)

	klog.V(2).Infof("============================== vgscan after vgcreate ==============================")
	args = []string{}
	info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgscan", args...)
	if err != nil {
		klog.Errorf("vgscan error %v: %s", err, info)
	} else {
		klog.V(2).Infof("============================== vgscan info %s  ==============================", info)
	}
	return nil
}

func ReduceVolumeGroup(volumeGroupName string, force bool) {
	klog.V(2).Infof("============================== Cleanup VG %s ==============================", volumeGroupName)
	args := []string{
		"--removemissing",
		volumeGroupName,
	}
	if force {
		args = append(args, "--force")
	}
	info, err := common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgreduce", args...)
	if err != nil {
		klog.Errorf("Errored while cleaning up volume group %v: %s", err, info)
	}
}

func RaidLocalSsds() error {
	isAlreadyRaided, err := isRaided()
	if err != nil {
		klog.V(2).Infof("============================== Errored while scanning for available LocalSSDs err:%v; continuing Raiding ==============================", err)
	} else if isAlreadyRaided {
		klog.V(2).Infof("============================== Local SSDs are already RAIDed ==============================")
		return nil
	}
	diskList := []string{}
	info, err := common.RunCommand("" /* pipedCmd */, "" /* pipeCmdArg */, "lsblk", []string{"-o", "NAME,MODEL", "-p", "-d", "-n"}...)
	if err != nil {
		return fmt.Errorf("errored while fetching NVME disks info: %v; err:%v", info, err)
	}
	infoList := strings.Split(strings.TrimSpace(string(info)), "\n")
	klog.Infof("============================== Got NVME disks %v ==============================", infoList)
	re, err := regexp.Compile("nvme_card([0-9]+)?$")
	if err != nil {
		klog.V(2).ErrorS(err, "Errored while compiling to check PD or LSSD")
	}
	for _, ssd := range infoList {
		klog.V(2).Infof("=========================== Checking for SSD %v ====================", ssd)
		ssd = strings.TrimSpace(ssd)
		if strings.HasPrefix(ssd, "/dev/nvme") {
			ssdDetails := strings.Split(ssd, " ")
			klog.V(2).Infof("=========================== Got SSD details %v ====================", ssdDetails)
			lssd := re.MatchString(ssdDetails[1])
			klog.Infof("=================== ssdDetails1 %v and compile string result %v", ssdDetails[1], lssd)
			if lssd {
				diskList = append(diskList, strings.TrimSpace(ssdDetails[0]))
			}
		}
	}
	klog.V(2).Infof("============================== NVME list %v ==============================", diskList)
	nvmeDiskCount := len(diskList)
	nvmeDiskList := strings.Join(diskList, " ")
	if nvmeDiskCount == 0 {
		klog.Infof("No NVME disks found for RAIDing")
		return nil
	}
	klog.V(2).Infof("============================== nvmeDiskCount %v; nvmeDiskList: %v; diskList %v ==============================", nvmeDiskCount, nvmeDiskList, diskList)
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
	info, err = common.RunCommand("" /* pipedCmd */, "" /* pipeCmdArg */, "mdadm", args...)
	if err != nil {
		return fmt.Errorf("errored while RAIDing LSSDs info: %v; err:%v", info, err)
	}
	// Validate if Raided successfully
	isAlreadyRaided, err = isRaided()
	if err != nil {
		klog.V(2).Infof("============================== Errored while scanning for available raided LocalSSDs err:%v ==============================", err)
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
	info, err := common.RunCommand("" /* pipedCmd */, "" /* pipeCmdArg */, "mdadm", args...)
	if err != nil {
		return false, fmt.Errorf("errored while scanning for raided LSSD %v: %s", err, info)
	}
	klog.V(2).Infof("============================== Got LSSDs %v ==============================", string(info))
	if info != nil && strings.Contains(string(info), raidedLocalSsdName) {
		return true, nil
	}
	return false, nil
}

func isCachingSetup(mainLvName string) (error, bool) {
	// Verify caching is setup for PD
	klog.V(2).Infof("============================== Verifying if caching is setup for %v ==============================", mainLvName)
	args := []string{
		"--select",
		"lv_name=" + mainLvName,
		"-o",
		"pool_lv",
	}
	poolName, err := common.RunCommand("" /* pipedCmd */, "" /* pipeCmdArg */, "lvs", args...)
	if err != nil {
		return fmt.Errorf("lvs error %w", err), false
	}
	if strings.Contains(string(poolName), "csi-fast") {
		return nil, true
	}
	return nil, false
}

func StartWatcher(nodeName string) {
	dirToWatch := "/dev/"
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		klog.V(2).ErrorS(err, "Errored while creating watcher")
	}
	defer watcher.Close()

	// out of the box fsnotify can watch a single file, or a single directory
	if err := watcher.Add(dirToWatch); err != nil {
		klog.V(2).ErrorS(err, "Errored while adding watcher directory")
	}
	errorCh := make(chan error, 1)
	// Handle the error received from the watcher goroutine
	go watchDiskDetaches(watcher, nodeName, errorCh)

	select {
	case err := <-errorCh:
		klog.Errorf("Watcher encountered an error: %v", err)
	}
}

func watchDiskDetaches(watcher *fsnotify.Watcher, nodeName string, errorCh chan error) error {
	for {
		select {
		// watch for errors
		case err := <-watcher.Errors:
			errorCh <- fmt.Errorf("Disk update event errored: %v", err)
		// watch for events
		case event := <-watcher.Events:
			// In case of an event i.e. creation or deletion of any new PV, we update the VG metadata.
			// This might include some non-LVM changes, no harm in updating metadata multiple times.
			ReduceVolumeGroup(GetVolumeGroupName(nodeName), true)
			klog.V(2).Infof("Disk attach/detach event %#v\n", event)
		}
	}
}

func ValidateRaidedLSSDinVG(vgName string) error {
	args := []string{
		"--noheadings",
		"-o",
		"pv_name",
		"--select",
		"vg_name=" + vgName,
	}
	info, err := common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "pvs", args...)
	if err != nil {
		return fmt.Errorf("errored while checking physical volume details %v: %s", err, info)
		// On error info contains the error message which we cannot use for further steps
	}

	klog.V(2).Infof("==================== Got PVs %v in VG %v ====================", strings.TrimSpace(string(info)), vgName)
	if !strings.Contains(string(info), "/dev/md127") {
		info, err := common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgextend", []string{vgName, "/dev/md127"}...)
		if err != nil {
			klog.Errorf("Errored while extending VGs %v: %s", err, info)
		}
	}
	return nil
}
