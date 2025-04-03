package gceGCEDriver

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	fsnotify "github.com/fsnotify/fsnotify"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

const (
	cacheSuffix                = "csi-fast"
	mainLvSuffix               = "csi-main"
	raidedLocalSsdName         = "csi-driver-data-cache"
	raidMode                   = "0"
	maxAllowedChunks   int64   = 1000000 // This is the max allowed chunks for LVM
	GiB                float64 = 1024 * 1024 * 1024
	KiB                float64 = 1024
)

var (
	maxChunkSize float64 = 1 * GiB   // Max allowed chunk size as per LVM documentation
	minChunkSize float64 = 160 * KiB // This is randomly selected, we need a multiple of 32KiB, the default size would be too small for caching https://man7.org/linux/man-pages/man8/lvcreate.8.html (--chunksize)
)

func fetchRAIDedLocalSsdPath() (string, error) {
	args := []string{
		"--detail",
		"--scan",
	}
	info, err := common.RunCommand("grep", []string{raidedLocalSsdName}, "mdadm", args...)
	if err != nil || len(info) == 0 {
		return "", fmt.Errorf("Error getting RAIDed device path for Data Cache %v, output:%v", err, string(info))
	}
	infoString := strings.TrimSpace(string(info))
	infoSlice := strings.Fields(infoString)

	// We want to get the second element in the array (sample: ARRAY /dev/md126 metadata=1.2 name=csi-driver-data-cache UUID=*),
	//  which is the path to the RAIDed device
	return infoSlice[1], nil
}

func setupCaching(devicePath string, req *csi.NodeStageVolumeRequest, nodeId string) (string, error) {

	// The device path may have changed after rebooting, so we need to fetch the path again
	raidedLocalSsdPath, err := fetchRAIDedLocalSsdPath()
	if err != nil {
		return "", err
	}

	volumeId := req.GetVolumeId()
	volumeGroupName := getVolumeGroupName(nodeId)
	mainDevicePath := "/dev/" + volumeGroupName + "/" + getLvName(mainLvSuffix, volumeId)
	mainLvName := getLvName(mainLvSuffix, volumeId)
	klog.V(4).Infof("Volume group available on node %v ", volumeGroupName)
	vgExists := checkVgExists(volumeGroupName)
	if vgExists {
		// Clean up Volume Group before adding the PD
		reduceVolumeGroup(volumeGroupName, true)
	} else {
		err := createVg(volumeGroupName, raidedLocalSsdPath)
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
	info, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "pvs", args...)
	if err != nil {
		klog.Errorf("errored while checking physical volume details %v: %s", err, info)
		// On error info contains the error message which we cannot use for further steps
		info = nil
	}

	infoString := strings.TrimSpace(strings.ReplaceAll(string(info), "\n", " "))
	infoString = strings.ReplaceAll(infoString, ".", "")
	infoString = strings.ReplaceAll(infoString, "\"", "")
	infoSlice := strings.Split(strings.TrimSpace(infoString), " ")
	vgNameForPv := strings.TrimSpace(infoSlice[(len(infoSlice) - 1)])
	klog.V(4).Infof("Physical volume is part of Volume group: %v", vgNameForPv)
	if vgNameForPv == volumeGroupName {
		klog.V(4).Infof("Physical Volume(PV) already exists in the Volume Group %v", volumeGroupName)
	} else if vgNameForPv != "VG" && vgNameForPv != "" {

		info, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgchange", []string{"-an", vgNameForPv}...)
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
			info, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvconvert", args...)
			if err != nil {
				return "", fmt.Errorf("errored while uncaching main LV. %v: %s", err, info)
			}
			// CLean up volume group to remove any dangling PV refrences
			reduceVolumeGroup(vgNameForPv, false)
		}
		info, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgmerge", []string{volumeGroupName, vgNameForPv}...)
		if err != nil {
			return "", fmt.Errorf("Errored while merging the PV Volume group %s into %s %v: %s", vgNameForPv, volumeGroupName, err, info)
		}

	} else {
		info, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgextend", []string{volumeGroupName, devicePath}...)
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
	lvList, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvs", args...)
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
		info, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvcreate", args...)
		if err != nil {
			return mainDevicePath, fmt.Errorf("Errored setting up logical volume for the volume %s %w: %s", devicePath, err, info)
		}

	}
	err, isCached := isCachingSetup(mainLvName)
	if err != nil {
		klog.Errorf("failed to check if caching is setup for LV, continuing to setup caching.")
	}
	cacheLvName := getLvName(cacheSuffix, volumeId)
	if isCached {
		// Validate that cache is setup for required size
		klog.V(4).Infof("Assuming valid data cache size and mode, resizing cache is not supported")
	} else {
		cacheSize := req.GetPublishContext()[common.ContextDataCacheSize]
		maxChunkSizeStr := strconv.FormatInt(int64(maxChunkSize/KiB), 10)
		var chunkSize string
		cachePvSize, err := fetchPvSizeGiB()
		if err != nil {
			klog.Errorf("Errored while fetching PV size, got %v, falling back to default chunkSize of %v", err, maxChunkSize)
			chunkSize = maxChunkSizeStr
		} else {
			chunkSize, err = fetchChunkSizeKiB(cachePvSize)
			if err != nil {
				klog.Errorf("Errored to fetch cache size, verify the data-cache-size is valid: got %v, error: %q", chunkSize, err)
				chunkSize = maxChunkSizeStr
			}
		}
		// Check if LV exists
		info, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvs", args...)
		lvExists := strings.Contains(string(info), cacheLvName)
		if !lvExists {
			args = []string{
				"--yes",
				"-n",
				cacheLvName,
				"-L",
				// ConvertGiStringToInt64 converts the input size to GiB so default to "g" for cache size - LVM g|G is GiB.
				cacheSize + "g",
				volumeGroupName,
				raidedLocalSsdPath,
			}
			info, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvcreate", args...)
			if err != nil {
				if strings.Contains(err.Error(), "insufficient free space") {
					return mainDevicePath, status.Error(codes.InvalidArgument, fmt.Sprintf("Error setting up cache: %v", err.Error()))
				}
				return mainDevicePath, fmt.Errorf("Errored while creating cache %w: %s", err, info)
			}
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
			chunkSize,
			"--force",
			"-y",
		}
		info, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvconvert", args...)
		if err != nil {
			return mainDevicePath, fmt.Errorf("Errored while setting up caching for volume %s %w: %s", devicePath, err, info)
		}
	}

	// activate all the LVs in the Volume group
	info, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgchange", []string{"-ay", volumeGroupName}...)
	if err != nil {
		// The logical volumes would not be accessible if the group is not activated
		return mainDevicePath, fmt.Errorf("Failed to activate volume group %v %v:%s", volumeGroupName, err, info)
	}
	return mainDevicePath, nil
}

func ValidateDataCacheConfig(dataCacheMode string, dataCacheSize string, ctx context.Context) error {
	if dataCacheMode != "" && dataCacheSize != "" {
		isAlreadyRaided, err := IsRaided()
		if err != nil {
			return fmt.Errorf("Local SSDs are not setup for caching; got error: %v", err)
		}
		if !isAlreadyRaided {
			return fmt.Errorf("Local SSDs are not setup for caching")
		}
		return nil
	}
	return fmt.Errorf("Data Cache is not enabled for PVC (data-cache-size: %v, data-cache-mode: %v). Please set both parameters in StorageClass to enable caching", dataCacheSize, dataCacheMode)
}

func GetDataCacheCountFromNodeLabel(ctx context.Context, nodeName string) (int, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return 0, err
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return 0, err
	}
	node, err := getNodeWithRetry(ctx, kubeClient, nodeName)
	if err != nil {
		return 0, err
	}
	if val, found := node.GetLabels()[fmt.Sprintf(common.NodeLabelPrefix, common.DataCacheLssdCountLabel)]; found {
		dataCacheCount, err := strconv.Atoi(val)
		if err != nil {
			return 0, fmt.Errorf("Error getting Data Cache's LSSD count from node label: %v", err)
		}
		klog.V(4).Infof("Number of local SSDs requested for Data Cache: %v", dataCacheCount)
		return dataCacheCount, nil
	}
	return 0, nil
}

func getNodeWithRetry(ctx context.Context, kubeClient *kubernetes.Clientset, nodeName string) (*v1.Node, error) {
	var nodeObj *v1.Node
	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   2.0,
		Steps:    5,
	}
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func() (bool, error) {
		node, err := kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("Error getting node %s: %v, retrying...\n", nodeName, err)
			return false, nil
		}
		nodeObj = node
		klog.V(4).Infof("Successfully retrieved node info %s\n", nodeName)
		return true, nil
	})

	if err != nil {
		klog.Errorf("Failed to get node %s after retries: %v\n", nodeName, err)
	}
	return nodeObj, err
}

func FetchRaidedLssdCountForDatacache() (int, error) {
	raidedPath, err := fetchRAIDedLocalSsdPath()
	if err != nil {
		return 0, err
	}
	args := []string{
		"--detail",
		raidedPath,
	}
	info, err := common.RunCommand("grep", []string{"Raid Devices"}, "mdadm", args...)
	if err != nil {
		return 0, fmt.Errorf("Error getting RAIDed devices for Data Cache")
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

	info, err := common.RunCommand("grep", []string{"/dev"}, "mdadm", args...)
	if err != nil {
		return nil, fmt.Errorf("error fetching RAIDed LSSDs: %v; err:%v", info, err)
	}

	if len(info) != 0 {
		infoList := strings.Split(strings.TrimSpace(string(info)), "\n")
		for _, ssd := range infoList {
			ssdInfo := strings.TrimSpace(ssd)
			// SSD name comes after "=" on each output line (e.g. MD_DEVICE_dev_nvme3n1_DEV=/dev/nvme3n1)
			ssdName := strings.Split(ssdInfo, "=")[1]
			raidedLssdList = append(raidedLssdList, ssdName)
		}
	}

	klog.V(4).Infof("Raided NVME list %v", raidedLssdList)

	return raidedLssdList, nil
}

func FetchAllLssds() ([]string, error) {
	diskList := []string{}

	info, err := common.RunCommand("" /* pipedCmd */, nil /* pipeCmdArg */, "lsblk", []string{"-o", "NAME,MODEL", "-p", "-d", "-n"}...)
	if err != nil {
		return nil, fmt.Errorf("errored while fetching NVME disks info: %v; err:%v", info, err)
	}
	infoList := strings.Split(strings.TrimSpace(string(info)), "\n")
	re, err := regexp.Compile("nvme_card([0-9]+)?$")
	if err != nil {
		klog.V(4).ErrorS(err, "Errored while compiling to check PD or LSSD")
	}
	for _, ssd := range infoList {
		ssd = strings.TrimSpace(ssd)
		if strings.HasPrefix(ssd, "/dev/nvme") {
			ssdDetails := strings.Fields(ssd)
			lssd := re.MatchString(ssdDetails[1])
			if lssd {
				diskList = append(diskList, strings.TrimSpace(ssdDetails[0]))
			}
		}
	}

	klog.V(4).Infof("NVME list %v", diskList)

	return diskList, nil
}

func FetchLSSDsWihtEmptyMountPoint() ([]string, error) {
	info, err := common.RunCommand("grep", []string{"-E", `^\S+\s*$`} /* pipeCmdArg */, "lsblk", []string{"-o", "NAME,MOUNTPOINT", "-pdn"}...)
	if err != nil {
		return nil, fmt.Errorf("Error while fetching disks with no mount point: %v; err:%v", info, err)
	}
	infoList := strings.Split(string(info), "\n")
	diskList := []string{}
	for _, ssd := range infoList {
		diskList = append(diskList, strings.TrimSpace(ssd))
	}
	return diskList, nil
}

func checkVgExists(volumeGroupName string) bool {
	args := []string{}
	info, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgscan", args...)
	if err != nil {
		klog.Errorf("Errored while checking if volume group exists %v: %s", err, info)
		return false
	}
	// Check if the required volume group already exists
	klog.Infof("check vg exists output: %v, volumeGroupName: %v", string(info), volumeGroupName)
	return strings.Contains(string(info), volumeGroupName)
}

func cleanupCache(volumeId string, nodeId string) error {

	volumeGroupName := getVolumeGroupName(nodeId)
	if !checkVgExists(volumeGroupName) {
		klog.V(4).Infof("Volume group %s not found, no cache clean up needed", volumeGroupName)
		// If volume group doesn't exist then there's nothing to uncache
		return nil
	}
	reduceVolumeGroup(volumeGroupName, true)
	mainLvName := getLvName(mainLvSuffix, volumeId)
	if !checkLvExists(mainLvName) {
		klog.V(4).Infof("Logical volume %s not found, assuming caching wasn't setup for the PVC %s or is cleaned up", mainLvName, volumeId)
		// If logical volume doesn't exist then there's nothing to uncache
		return nil
	}
	args := []string{
		"-an",
		"/dev/" + volumeGroupName + "/" + mainLvName,
	}
	info, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvchange", args...)
	if err != nil {
		return fmt.Errorf("Failed to deactivate volume for uncaching %s %v: %s", volumeId, err, info)
	}
	args = []string{
		"--uncache",
		volumeGroupName + "/" + mainLvName,
		"-y",
	}
	info, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvconvert", args...)
	if err != nil {
		return fmt.Errorf("Failed to uncache volume %s %w: %s", volumeId, err, info)
	}
	return nil
}

func checkLvExists(lvName string) bool {
	args := []string{}
	info, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvscan", args...)
	if err != nil {
		klog.Errorf("Errored while checking if logical volume exists for %s %v: %s", lvName, err, info)
		return false
	}
	// Check if the required logical volume already exists
	return strings.Contains(string(info), lvName)
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

func createVg(volumeGroupName string, raidedLocalSsds string) error {
	args := []string{
		"--zero",
		"y",
		volumeGroupName,
		raidedLocalSsds,
		"-v",
	}
	info, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgcreate", args...)
	if err != nil {
		return fmt.Errorf("Volume group creation failed %w: %s", err, info)
	}
	klog.V(4).Infof("Volume group creation succeeded for %v", volumeGroupName)

	args = []string{}
	info, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgscan", args...)
	if err != nil {
		klog.Errorf("Failed to scan for volume group post creation, continuing: %v: %s", err, info)
	}
	return nil
}

func reduceVolumeGroup(volumeGroupName string, force bool) {
	if !checkVgExists(volumeGroupName) {
		klog.V(2).Infof("Volume group %v not found, no further action needed", volumeGroupName)
		return
	}
	args := []string{
		"--removemissing",
		volumeGroupName,
	}
	if force {
		args = append(args, "--force")
	}
	info, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgreduce", args...)
	if err != nil {
		klog.Errorf("Errored while cleaning up volume group %v: %s", err, info)
	}
}

func RaidLocalSsds(availableLssds []string) error {
	args := []string{
		"--create",
		raidedLocalSsdName,
		"-l" + raidMode,
		// Force RAIDing as sometime it might fail for caution if there is just 1 LSSD present as 1 LSSD need not be RAIDed
		"--force",
		"-n",
		strconv.Itoa(len(availableLssds)),
	}
	args = append(args, availableLssds...)
	info, err := common.RunCommand("" /* pipedCmd */, nil /* pipeCmdArg */, "mdadm", args...)
	if err != nil {
		return fmt.Errorf("errored while RAIDing LSSDs info: %v; err:%v", info, err)
	}
	// Validate if Raided successfully
	isAlreadyRaided, err := IsRaided()
	if err != nil {
		klog.V(4).Infof("Errored while scanning for available raided LocalSSDs err:%v=", err)
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

func IsRaided() (bool, error) {
	args := []string{
		"--detail",
		"--scan",
	}
	info, err := common.RunCommand("" /* pipedCmd */, nil /* pipeCmdArg */, "mdadm", args...)
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
	poolName, err := common.RunCommand("" /* pipedCmd */, nil /* pipeCmdArg */, "lvs", args...)
	if err != nil {
		return fmt.Errorf("Failed to check if caching is setup %w", err), false
	}
	if strings.Contains(string(poolName), "csi-fast") {
		return nil, true
	}
	return nil, false
}

func fetchChunkSizeKiB(cacheSize string) (string, error) {
	var chunkSize float64

	cacheSizeInt, err := strconv.ParseInt(cacheSize, 10, 64)
	if err != nil {
		return "0", err
	}
	// Chunksize should be divisible by 32Kib so we need (chunksize/32*1024)*32*1024
	chunkSize = (float64(cacheSizeInt) * GiB) / float64(maxAllowedChunks)
	chunkSize = math.Round(chunkSize/(32*KiB)) * (32 * KiB)
	chunkSize = math.Min(math.Max(chunkSize, minChunkSize), maxChunkSize) / KiB
	// default chunk size unit KiB
	return strconv.FormatInt(int64(chunkSize), 10) + "KiB", nil
}

func InitializeDataCacheNode(nodeId string) error {
	raidedLocalSsdPath, err := fetchRAIDedLocalSsdPath()
	if err != nil {
		return err
	}
	volumeGroupName := getVolumeGroupName(nodeId)

	vgExists := checkVgExists(volumeGroupName)
	// Check if the required volume group already exists
	if vgExists {
		// Clean up Volume Group before adding the PD
		reduceVolumeGroup(volumeGroupName, true)

		// validate that raidedLSSD is part of VG
		err = validateRaidedLSSDinVG(volumeGroupName, raidedLocalSsdPath)
		if err != nil {
			return fmt.Errorf("failed validate local ssd in vg %v: %v", volumeGroupName, err)
		}
	} else {
		err := createVg(volumeGroupName, raidedLocalSsdPath)
		if err != nil {
			return err
		}
	}
	return nil
}

func StartWatcher(nodeName string) {
	dirToWatch := "/dev/"
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		klog.V(2).ErrorS(err, "errored while creating watcher")
	}
	klog.V(2).Infof("Watcher started for directory %v", dirToWatch)
	defer watcher.Close()

	// out of the box fsnotify can watch a single file, or a single directory
	if err := watcher.Add(dirToWatch); err != nil {
		klog.V(2).ErrorS(err, "errored while adding watcher directory")
	}
	errorCh := make(chan error, 1)
	// Handle the error received from the watcher goroutine
	go watchDiskDetaches(watcher, nodeName, errorCh)

	select {
	case err := <-errorCh:
		klog.Errorf("watcher encountered an error: %v", err)
	}
}

func watchDiskDetaches(watcher *fsnotify.Watcher, nodeName string, errorCh chan error) error {
	for {
		select {
		// watch for errors
		case err := <-watcher.Errors:
			errorCh <- fmt.Errorf("disk update event errored: %v", err)
		// watch for events
		case event := <-watcher.Events:
			// In case of an event i.e. creation or deletion of any new PV, we update the VG metadata.
			// This might include some non-LVM changes, no harm in updating metadata multiple times.
			args := []string{
				"--updatemetadata",
				getVolumeGroupName(nodeName),
			}
			_, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgck", args...)
			if err != nil {
				klog.Errorf("Error updating volume group's metadata: %v", err)
			}
			reduceVolumeGroup(getVolumeGroupName(nodeName), true)
			klog.V(6).Infof("disk attach/detach event %#v\n", event)
		}
	}
}

func validateRaidedLSSDinVG(vgName string, lssdPath string) error {
	args := []string{
		"--noheadings",
		"-o",
		"pv_name",
		"--select",
		"vg_name=" + vgName,
	}
	info, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "pvs", args...)
	if err != nil {
		return fmt.Errorf("errored while checking physical volume details %v: %s", err, info)
		// On error info contains the error message which we cannot use for further steps
	}

	if !strings.Contains(string(info), lssdPath) {
		return addRaidedLSSDToVg(vgName, lssdPath)
	}
	return nil
}

func addRaidedLSSDToVg(vgName, lssdPath string) error {
	info, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgextend", []string{vgName, lssdPath}...)
	if err != nil {
		return fmt.Errorf("errored while extending VGs %v: %s", err, info)
	}
	return nil
}

func fetchPvSizeGiB() (string, error) {
	args := []string{
		"--select",
		"-o",
		"--noheadings",
		"pv_size",
		"--units=b",
	}
	// RAIDed device is always registered with its /dev/md127 equivalent in VG so cannot check it directly based on the RAIDed LSSD path which could be /dev/md/csi-driver-data-cache
	info, err := common.RunCommand("grep" /* pipedCmd */, []string{"/dev/md"} /* pipedCmdArg */, "pvs", args...)
	if err != nil {
		return "", fmt.Errorf("errored while fetching PV size %v: %s", err, info)
	}
	infoString := strings.TrimSpace(string(info))
	infoSlice := strings.Fields(infoString)
	pvSize, err := fetchNumberGiB(infoSlice)
	if err != nil {
		return "", fmt.Errorf("Error fetching PV size for cache %v", err)
	}
	return pvSize, nil

}

func fetchNumberGiB(infoSlice []string) (string, error) {
	re, err := regexp.Compile("^[0-9]+B$")
	if err != nil {
		return "", fmt.Errorf("Failed to compile regex match %v", err)
	}
	var pvSize string
	for _, i := range infoSlice {
		if re.MatchString(i) {
			pvSize, err = strings.TrimSuffix(i, "B"), nil
			if err != nil {
				return "", fmt.Errorf("Failed to extract PV size %v", err)
			}
			break
		}
	}
	pvSizeInt, err := strconv.ParseFloat(pvSize, 64)
	if err != nil {
		return "", fmt.Errorf("Error while fetching PV size for cache %v", err)
	}
	return strconv.FormatInt(int64(math.Ceil(pvSizeInt/GiB)), 10) + "GiB", nil
}
