package gceGCEDriver

import (
	"fmt"
	"regexp"
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
)

var raidedLocalSsdPath = raidedLssdPrefix + raidedLocalSsdName

func setupCaching(devicePath string, req *csi.NodeStageVolumeRequest, nodeId string) (string, error) {
	volumeId := req.GetVolumeId()
	volumeGroupName := getVolumeGroupName(nodeId)
	mainDevicePath := "/dev/" + volumeGroupName + "/" + getLvName(mainLvSuffix, volumeId)
	mainLvName := getLvName(mainLvSuffix, volumeId)
	klog.V(2).Infof("Volume group available on node %v ", volumeGroupName)

	info, err := common.RunCommand("grep", raidedLocalSsdName, "ls", raidedLssdPrefix)
	if err != nil {
		klog.Errorf("failed while listing raided devices, err: %v, output:%v", err, info)
	}
	infoString := strings.TrimSpace(string(info))
	raidedLocalSsdPath = raidedLssdPrefix + infoString

	vgExists := checkVgExists(volumeGroupName)
	if vgExists {
		// Clean up Volume Group before adding the PD
		reduceVolumeGroup(volumeGroupName, true)
	} else {
		err := createVg(volumeGroupName, devicePath, raidedLocalSsdPath)
		if err != nil {
			return mainDevicePath, err
		}
	}

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
	if vgNameForPv == volumeGroupName {
		klog.V(2).Infof("Physical Volume(PV) already exists in the Volume Group %v", volumeGroupName)
	} else if vgNameForPv != "VG" && vgNameForPv != "" {
		info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgchange", []string{"-an", vgNameForPv}...)
		if err != nil {
			klog.Errorf("Errored while deactivating VG %v: err: %v: %s", vgNameForPv, err, info)
		}
		// CLean up volume group to remove any dangling PV refrences
		reduceVolumeGroup(vgNameForPv, false)
		_, isCached := isCachingSetup(mainLvName)
		// We will continue to uncache even if it errors to check caching as it is not a terminal issue.
		if isCached {
			// Uncache LV
			args = []string{
				"--uncache",
				vgNameForPv + "/" + mainLvName,
				"--force",
				"-y", // force remove cache without flushing data
			}
			info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "lvconvert", args...)
			if err != nil {
				return "", fmt.Errorf("errored while uncaching main LV. %v: %s", err, info)
			}
			// CLean up volume group to remove any dangling PV refrences
			reduceVolumeGroup(vgNameForPv, false)
		}
		info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgmerge", []string{volumeGroupName, vgNameForPv}...)
		if err != nil {
			return "", fmt.Errorf("Errored while merging the PV Volume group %s into %s %v: %s", vgNameForPv, volumeGroupName, err, info)
		}

	} else {
		info, err := common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgextend", []string{volumeGroupName, devicePath}...)
		if err != nil {
			return "", fmt.Errorf("Errored while extending Volume group to add PV %v, error: %v: %s", devicePath, err, info)
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
		return mainDevicePath, fmt.Errorf("Errored while checking logical volume for the device %s %w: %s", devicePath, err, info)
	}
	if !strings.Contains(string(lvList), mainLvName) {
		args = []string{
			"--yes",
			"-n",
			mainLvName,
			"-l",
			"100%PVS", // Use 100% of the PV
			volumeGroupName,
			devicePath,
		}
		info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "lvcreate", args...)
		if err != nil {
			return mainDevicePath, fmt.Errorf("Errored setting up logical volume for the volume %s %w: %s", devicePath, err, info)
		}

	}
	err, isCached := isCachingSetup(mainLvName)
	if err != nil {
		klog.Errorf("faild to check if caching ius setup for LV, continuing to setup caching.")
	}
	cacheLvName := getLvName(cacheSuffix, volumeId)
	if isCached {
		// Validate that cache is setup for required size
		klog.V(2).Infof("Assuming valid data cache size and mode, resizing cache is not supported")
	} else {
		fastCacheSize := req.GetPublishContext()[common.ContexLocalSsdCacheSize]
		chunkSize := "960" // Cannot use default chunk size(64KiB) as it errors on maxChunksAllowed. Unit - KiB
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
			return mainDevicePath, fmt.Errorf("Errored while creating cache %w: %s", err, info)
		}

		// Once caching is setup, link the PD to cache
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
			return mainDevicePath, fmt.Errorf("Errored while setting up caching for volume %s %w: %s", devicePath, err, info)
		}
	}

	// activate all the LVs in the Volume group
	info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgchange", []string{"-ay", volumeGroupName}...)
	if err != nil {
		// The logical volumes would not be accessible if the group is not activated
		return mainDevicePath, fmt.Errorf("Failed to activate volume group %v %v:%s", volumeGroupName, err, info)
	}
	return mainDevicePath, nil
}

func checkVgExists(volumeGroupName string) bool {
	args := []string{}
	info, err := common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgscan", args...)
	if err != nil {
		klog.Errorf("Errored while checking if volume group exists %v: %s", err, info)
		return false
	}
	// Check if the required volume group already exists
	return strings.Contains(string(info), volumeGroupName)
}

func cleanupCache(volumeId string, nodeId string) error {

	volumeGroupName := getVolumeGroupName(nodeId)
	if !checkVgExists(volumeGroupName) {
		// If volume group doesn't exist then there's nothing to uncache
		return nil
	}
	mainLvName := getLvName(mainLvSuffix, volumeId)
	args := []string{
		"-an",
		"/dev/" + volumeGroupName + "/" + mainLvName,
	}
	info, err := common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "lvchange", args...)
	if err != nil {
		return fmt.Errorf("Failed to deactivate volume for uncaching %s %v: %s", volumeId, err, info)
	}
	args = []string{
		"--uncache",
		volumeGroupName + "/" + mainLvName,
		"-y",
	}
	info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "lvconvert", args...)
	if err != nil {
		return fmt.Errorf("Failed to uncache volume %s %w: %s", volumeId, err, info)
	}
	return nil
}

func getVolumeGroupName(nodePath string) string {
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

func createVg(volumeGroupName string, devicePath string, raidedLocalSsds string) error {
	klog.V(2).Infof(" vgcreate=")
	args := []string{
		"--zero",
		"y",
		volumeGroupName,
		raidedLocalSsds,
		"-v",
	}
	info, err := common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgcreate", args...)
	if err != nil {
		return fmt.Errorf("Volume group creation failed %w: %s", err, info)
	}
	klog.Infof("Volume group creation succeeded for %v", volumeGroupName)

	args = []string{}
	info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgscan", args...)
	if err != nil {
		klog.Errorf("Failed to scan for volume group post creation, continuing: %v: %s", err, info)
	}
	return nil
}

func reduceVolumeGroup(volumeGroupName string, force bool) {
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
		klog.V(2).Infof("Errored while scanning for available LocalSSDs err:%v; continuing Raiding", err)
	} else if isAlreadyRaided {
		klog.V(2).Infof("Local SSDs are already RAIDed, no further action needed here")
		return nil
	}
	diskList := []string{}
	info, err := common.RunCommand("" /* pipedCmd */, "" /* pipeCmdArg */, "lsblk", []string{"-o", "NAME,MODEL", "-p", "-d", "-n"}...)
	if err != nil {
		return fmt.Errorf("Failed to fetch LSSD info: %v; err:%v", info, err)
	}
	infoList := strings.Split(strings.TrimSpace(string(info)), "\n")
	re, err := regexp.Compile("nvme_card([0-9]+)?$")
	if err != nil {
		return fmt.Errorf("Errored while compiling to check PD or LSSD %s", err)
	}
	for _, ssd := range infoList {
		ssd = strings.TrimSpace(ssd)
		if strings.HasPrefix(ssd, "/dev/nvme") {
			ssdDetails := strings.Split(ssd, " ")
			lssd := re.MatchString(ssdDetails[1])
			if lssd {
				diskList = append(diskList, strings.TrimSpace(ssdDetails[0]))
			}
		}
	}
	nvmeDiskCount := len(diskList)
	if nvmeDiskCount == 0 {
		return fmt.Errorf("No local SSDs found for raiding")
	}
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
		klog.V(2).Infof("Errored while scanning for available raided LocalSSDs err:%v=", err)
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
	if info != nil && strings.Contains(string(info), raidedLocalSsdName) {
		return true, nil
	}
	return false, nil
}

func isCachingSetup(mainLvName string) (error, bool) {
	// Verify caching is setup for PD
	args := []string{
		"--select",
		"lv_name=" + mainLvName,
		"-o",
		"pool_lv",
	}
	poolName, err := common.RunCommand("" /* pipedCmd */, "" /* pipeCmdArg */, "lvs", args...)
	if err != nil {
		return fmt.Errorf("Failed to check if caching is setup %w", err), false
	}
	if strings.Contains(string(poolName), "csi-fast") {
		return nil, true
	}
	return nil, false
}
