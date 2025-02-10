package gceGCEDriver

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
	klog.V(2).Infof("============================== Start LVM PoC NodeStageVolume Steps ==============================")
	klog.V(2).Infof("============================== volumeGroupName is %v ==============================", volumeGroupName)

	info, err := common.RunCommand("grep", raidedLocalSsdName, "ls", raidedLssdPrefix)
	if err != nil {
		klog.Errorf("================== failed while listing raided devices, err: %v, output:%v ===============", err, info)
	}
	infoString := strings.TrimSpace(string(info))
	klog.V(2).Infof("=================== Got Raided LSSD name %v ===================", infoString)
	raidedLocalSsdPath = raidedLssdPrefix + infoString

	klog.V(2).Infof("============================== vgscan before vgcreate ==============================")
	args := []string{}
	info, err = common.RunCommand("" /* pipedCmd */, "" /* pipedCmdArg */, "vgscan", args...)
	if err != nil {
		klog.Errorf("vgscan error %v: %s", err, info)
	}
	klog.V(2).Infof("============================== vgscan info contains volumeGroupName or not %v ==============================", strings.Contains(string(info), volumeGroupName))
	// Check if the required volume group already exists
	if strings.Contains(string(info), volumeGroupName) {
		klog.V(2).Infof("============================== VG exists, now check if PD is part of VG ==============================")

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

		reduceVolumeGroup(vgNameForPv, false)
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

			reduceVolumeGroup(vgNameForPv, false)
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
		fastCacheSize := req.GetPublishContext()[common.ContextDataCacheSize]
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

func ValidateDataCacheConfig(dataCacheMode string, datacacheSize string, ctx context.Context, nodeName string) error {
	if dataCacheMode != "" && datacacheSize != "" {
		isAlreadyRaided, err := isRaided()
		if err != nil {
			return fmt.Errorf("Local SSDs are not setup for caching; got error: %v", err)
		}
		if !isAlreadyRaided {
			return fmt.Errorf("Local SSDs are not setup for caching")
		}
		return nil
	}
	klog.Infof("Data cache is not enabled for PVC")
	return nil
}

func GetDataCacheCountFromNodeLabel(ctx context.Context, nodeName string) (int, error) {
	if nodeName == common.TestNode {
		return common.LocalSSDCountForDataCache, nil
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return 0, err
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return 0, err
	}
	node, err := kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		// We could retry, but this error will also crashloop the driver which may be as good a way to retry as any.
		return 0, err
	}
	if val, found := node.GetLabels()[fmt.Sprintf(common.NodeLabelPrefix, common.DataCacheLssdCountLabel)]; found {
		dataCacheCount, err := strconv.Atoi(val)
		if err != nil {
			return 0, fmt.Errorf("Error getting Datacache's LSSD count from node label: %v", err)
		}
		return dataCacheCount, nil
	}
	return 0, fmt.Errorf("Cannot get Datacache's LSSD count from node label")
}

func FetchRaidedLssdCountForDatacache() (int, error) {
	args := []string{
		"--detail",
		raidedLssdPrefix + raidedLocalSsdName,
	}
	info, err := common.RunCommand("grep", "Raid Devices", "mdadm", args...)
	if err != nil {
		return 0, fmt.Errorf("Error getting RAIDed devices for Datacache")
	}
	if len(info) != 0 {
		raidedDeviceInfo := strings.Split(strings.TrimSpace(string(info)), ":")
		// raidedDeviceInfo should be in "Raid Devices : X" format
		raidedDeviceCount, _ := strconv.Atoi(strings.TrimSpace(raidedDeviceInfo[1]))
		return raidedDeviceCount, nil
	}
	return 0, nil
}

func FetchRaidedLssds() ([]string, error) {
	raidedLssdList := []string{}

	args := []string{
		"--detail",
		"--scan",
		"--export",
	}

	info, err := common.RunCommand("grep", "/dev", "mdadm", args...)
	if err != nil {
		return nil, fmt.Errorf("error fetching RAIDed LSSDs: %v; err:%v", info, err)
	}

	if len(info) != 0 {
		infoList := strings.Split(strings.TrimSpace(string(info)), "\n")
		for _, ssd := range infoList {
			ssdInfo := strings.TrimSpace(ssd)
			klog.V(2).Infof("=========================== Got RAIDed SSD info %v ====================", ssdInfo)
			// SSD name comes after "=" on each output line (e.g. MD_DEVICE_dev_nvme3n1_DEV=/dev/nvme3n1)
			ssdName := strings.Split(ssdInfo, "=")[1]
			raidedLssdList = append(raidedLssdList, ssdName)
		}
	}

	klog.V(2).Infof("============================== Raided NVME list %v ==============================", raidedLssdList)

	return raidedLssdList, nil
}

func FetchAllLssds() ([]string, error) {
	diskList := []string{}

	info, err := common.RunCommand("" /* pipedCmd */, "" /* pipeCmdArg */, "lsblk", []string{"-o", "NAME,MODEL", "-p", "-d", "-n"}...)
	if err != nil {
		return nil, fmt.Errorf("errored while fetching NVME disks info: %v; err:%v", info, err)
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

	return diskList, nil
}

func cleanupCache(volumeId string, nodeId string) error {

	volumeGroupName := getVolumeGroupName(nodeId)
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

func reduceVolumeGroup(volumeGroupName string, force bool) {
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

func RaidLocalSsds(availableLssds []string) error {
	isAlreadyRaided, err := isRaided()
	if err != nil {
		klog.V(2).Infof("============================== Errored while scanning for available LocalSSDs err:%v; continuing Raiding ==============================", err)
	} else if isAlreadyRaided {
		klog.V(2).Infof("============================== Local SSDs are already RAIDed ==============================")
		return nil
	}

	args := []string{
		"--create",
		raidedLssdPrefix + raidedLocalSsdName,
		"-l" + raidMode,
		// Force RAIDing as sometime it might fail for caution if there is just 1 LSSD present as 1 LSSD need not be RAIDed
		"--force",
		"-n",
		strconv.Itoa(len(availableLssds)),
	}
	args = append(args, availableLssds...)
	info, err := common.RunCommand("" /* pipedCmd */, "" /* pipeCmdArg */, "mdadm", args...)
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

	raidedDataCacheCount, err := FetchRaidedLssdCountForDatacache()
	if err != nil {
		return err
	}
	if raidedDataCacheCount != len(availableLssds) {
		return fmt.Errorf("Local SSDs reserved do not match the requested count")
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
