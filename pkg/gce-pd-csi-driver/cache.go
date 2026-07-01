package gceGCEDriver

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	fsnotify "github.com/fsnotify/fsnotify"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/k8sclient"
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

func getVgUuid(vgName string) (string, error) {
	args := []string{
		"--noheadings",
		"-o",
		"vg_uuid",
		vgName,
	}
	output, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgs", args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func setupCaching(devicePath string, req *csi.NodeStageVolumeRequest, nodeId string) (string, error) {

	// The device path may have changed after rebooting, so we need to fetch the path again
	raidedLocalSsdPath, err := fetchRAIDedLocalSsdPath()
	if err != nil {
		return "", err
	}

	volumeId := req.GetVolumeId()
	volumeGroupName := getVolumeGroupName(nodeId)
	mainLvName := getLvName(mainLvSuffix, volumeId)
	mainDevicePath := "/dev/" + volumeGroupName + "/" + mainLvName

	// Refresh LVM device cache to ensure newly attached GCE PD is fully scanned (Comment: Refresh LVM cache)
	_, _ = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "pvscan", []string{"--cache"}...)
	_, _ = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgscan", []string{"--cache"}...)

	// Resolve any stale duplicate Volume Group name conflicts from preemption events
	hasConflict, staleVgUuid, err := detectPreemptionVgConflict(volumeGroupName, devicePath)
	if err != nil {
		klog.Errorf("Failed to check duplicate VG conflict for PV %v: %v", devicePath, err)
	}
	if hasConflict {
		err = recoverPreemptionVgConflict(volumeGroupName, mainLvName, devicePath, staleVgUuid)
		if err != nil {
			return "", err
		}
	}

	klog.V(4).Infof("Volume group available on node %v ", volumeGroupName)
	vgExists := checkVgExists(volumeGroupName)
	if vgExists {
		// Clean up Volume Group before adding the PD
		reduceVolumeGroup(volumeGroupName, true)

		// Ensure the local SSD cache PV is registered in the Volume Group
		err = ensureLocalSsdInVolumeGroup(volumeGroupName, raidedLocalSsdPath)
		if err != nil {
			return mainDevicePath, err
		}
	} else {
		err := createVg(volumeGroupName, raidedLocalSsdPath)
		if err != nil {
			return mainDevicePath, err
		}
	}

	// Check if the Physical Volume(PV) is part of some other volume group
	vgNameForPv, _, err := getVgInfoForPv(devicePath)
	if err != nil {
		return "", fmt.Errorf("failed to get VG name for PV %s: %w", devicePath, err)
	}
	klog.V(4).Infof("Physical volume is part of Volume group: %v", vgNameForPv)
	if vgNameForPv == volumeGroupName {
		klog.V(4).Infof("Physical Volume(PV) already exists in the Volume Group %v", volumeGroupName)
	} else if vgNameForPv != "" {
		err = mergeStaleVgIntoHost(vgNameForPv, volumeGroupName, mainLvName)
		if err != nil {
			return "", err
		}
	} else {
		_, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgextend", []string{volumeGroupName, devicePath}...)
		if err != nil {
			return "", fmt.Errorf("Errored while extending Volume group to add PV %v, error: %w", devicePath, err)
		}
	}

	// Create LV if not already created
	args := []string{
		"--select",
		"vg_name=" + volumeGroupName,
		"-o",
		"lv_name",
	}
	lvList, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvs", args...)
	if err != nil {
		return mainDevicePath, fmt.Errorf("Errored while checking logical volume for the device %s %w", devicePath, err)
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
		_, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvcreate", args...)
		if err != nil {
			return mainDevicePath, fmt.Errorf("Errored setting up logical volume for the volume %s %w", devicePath, err)
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
		cacheSize := req.GetPublishContext()[constants.ContextDataCacheSize]
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
		args = []string{
			"--select",
			"vg_name=" + volumeGroupName,
			"-o",
			"lv_name",
		}
		info, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvs", args...)
		if err != nil {
			return mainDevicePath, fmt.Errorf("Errored while checking logical volume list %w", err)
		}
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
			_, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvcreate", args...)
			if err != nil {
				if strings.Contains(err.Error(), "insufficient free space") {
					return mainDevicePath, status.Error(codes.InvalidArgument, fmt.Sprintf("Error setting up cache: %v", err.Error()))
				}
				return mainDevicePath, fmt.Errorf("Errored while creating cache %w", err)
			}
		}

		// Once caching is setup, link the PD to cache
		args = []string{
			"--type",
			"cache",
			"--cachevol",
			cacheLvName,
			"--zero",
			"n",
			"--cachemode",
			req.GetPublishContext()[constants.ContextDataCacheMode],
			volumeGroupName + "/" + mainLvName,
			"--chunksize",
			chunkSize,
			"--force",
			"-y",
		}
		_, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvconvert", args...)
		if err != nil {
			return mainDevicePath, fmt.Errorf("Errored while setting up caching for volume %s %w", devicePath, err)
		}
	}

	// activate all the LVs in the Volume group
	_, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgchange", []string{"-ay", volumeGroupName}...)
	if err != nil {
		// The logical volumes would not be accessible if the group is not activated
		return mainDevicePath, fmt.Errorf("Failed to activate volume group %v %w", volumeGroupName, err)
	}
	return mainDevicePath, nil
}

// detectPreemptionVgConflict checks if a stale duplicate Volume Group name conflict exists on the GCE PD.
// A conflict occurs when a worker node is preempted and replaced: the GCE PD retains the old VG metadata/name,
// which matches the new VG name initialized on the replacement node's local SSDs, but has a different VG UUID.
// Returns true, the stale VG UUID, and nil if a conflict is detected; false, "", and nil/error otherwise.
func detectPreemptionVgConflict(volumeGroupName, devicePath string) (bool, string, error) {
	vgNameForPv, vgUuidForPv, err := getVgInfoForPv(devicePath)
	if err != nil {
		return false, "", err
	}
	if vgNameForPv != volumeGroupName || vgUuidForPv == "" {
		klog.V(4).Infof("No duplicate VG name conflict detected for %s on PV %s", volumeGroupName, devicePath)
		return false, "", nil
	}

	// Get the active VG UUID on the host to distinguish same-node reattachments from true conflicts
	activeVgUuid, activeVgErr := getVgUuid(volumeGroupName)
	if activeVgErr != nil {
		klog.V(4).Infof("Failed to get active VG UUID for %s (it might not exist yet): %v. Proceeding with duplicate VG recovery.", volumeGroupName, activeVgErr)
	}
	if activeVgUuid == vgUuidForPv {
		klog.Infof("No duplicate VG conflict: GCE PD's VG UUID %s matches the active host VG UUID. Skipping recovery.", vgUuidForPv)
		return false, "", nil
	}

	return true, vgUuidForPv, nil
}

// recoverPreemptionVgConflict resolves the duplicate Volume Group name conflict on the GCE PD.
// It isolates LVM commands to the GCE PD using a device filter, deactivates the stale VG,
// dissociates the cache pool, prunes missing PV references, renames the stale VG, and
// regenerates its UUID to completely break the conflict.
func recoverPreemptionVgConflict(volumeGroupName, mainLvName, devicePath, staleVgUuid string) error {
	klog.Infof("Resolving duplicate VG name conflict for %s using stale VG UUID %s", volumeGroupName, staleVgUuid)
	configFilter := fmt.Sprintf("devices { filter = [ \"a|%s|\", \"r|.*|\" ] }", devicePath)

	// Step 1: Deactivate the stale VG first under the device filter.
	// If the GCE PD was automatically activated upon attachment, LVM will reject metadata edits
	// because the logical volumes are "open" or "in use" (kernel-active). Deactivating them takes them offline safely.
	klog.Infof("Step 1: Deactivating stale VG %s using device filter", volumeGroupName)
	if err := deactivateVolumeGroup(volumeGroupName, configFilter); err != nil {
		klog.Warningf("Failed to deactivate stale VG %s: %v. Attempting to proceed with recovery.", volumeGroupName, err)
	}

	// Step 2: Cleanly decouple (uncache) the caching layout first, BEFORE running vgreduce.
	// WARNING: If we run vgreduce --removemissing first on a partial cached LV (missing local SSD),
	// LVM will forcefully destroy/delete the entire logical volume, resulting in complete data loss!
	// Dissociating the cache first preserves the origin volume (and user data) intact on the GCE PD.
	isCached, err := isLvCachedOnDevice(volumeGroupName, mainLvName, devicePath)
	if err != nil {
		klog.Errorf("Failed to check if stale volume %s/%s is cached: %v", volumeGroupName, mainLvName, err)
	}
	if isCached {
		klog.Infof("Step 2: Dissociating (uncaching) stale volume %s/%s using device filter to preserve data", volumeGroupName, mainLvName)
		if err := uncacheLogicalVolume(volumeGroupName, mainLvName, configFilter); err != nil {
			return fmt.Errorf("failed to uncache stale volume %s/%s during preemption recovery: %w", volumeGroupName, mainLvName, err)
		}
	} else {
		klog.Infof("Stale volume %s/%s is not cached, skipping uncache step during preemption recovery", volumeGroupName, mainLvName)
	}

	// Step 3: Clean up missing PVs on the stale VG using the device filter.
	// Since the cache has been dissociated, this safely removes the missing local SSD references from the VG metadata.
	klog.Infof("Step 3: Cleaning up missing PVs on duplicate VG %s using device filter", volumeGroupName)
	if err := reduceStaleVolumeGroup(volumeGroupName, configFilter); err != nil {
		return fmt.Errorf("failed to reduce stale VG %s during preemption recovery: %w", volumeGroupName, err)
	}

	// Compute unique temporary name for the stale VG.
	shortUuid := staleVgUuid
	if len(shortUuid) > 8 {
		shortUuid = shortUuid[:8]
	}
	tempVgName := fmt.Sprintf("csi-vg-stale-%s", shortUuid)

	// Step 4: Rename the stale VG using the device filter to resolve the duplicate name conflict.
	klog.Infof("Step 4: Renaming stale VG %s to %s using device filter", volumeGroupName, tempVgName)
	if err := renameVolumeGroup(volumeGroupName, tempVgName, configFilter); err != nil {
		return fmt.Errorf("failed to rename stale VG %s to %s during preemption recovery: %w", volumeGroupName, tempVgName, err)
	}

	// Step 5: Regenerate the UUID of the renamed VG to break the duplicate VGID conflict.
	klog.Infof("Step 5: Regenerating UUID for stale VG %s to resolve duplicate VGID conflict", tempVgName)
	if err := regenerateVolumeGroupUuid(tempVgName, configFilter); err != nil {
		klog.Warningf("Failed to regenerate UUID for stale VG %s: %v. Continuing recovery.", tempVgName, err)
	}

	return nil
}

// deactivateVolumeGroup deactivates the Volume Group under the specified device filter to release kernel locks.
func deactivateVolumeGroup(vgName, configFilter string) error {
	deactivateArgs := []string{"-an", vgName, "--config", configFilter}
	_, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgchange", deactivateArgs...)
	return err
}

// uncacheLogicalVolume dissociates (uncaches) the cache pool from the logical volume under the specified device filter.
func uncacheLogicalVolume(vgName, lvName, configFilter string) error {
	uncacheArgs := []string{"--uncache", vgName + "/" + lvName, "--force", "-y", "--config", configFilter}
	_, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvconvert", uncacheArgs...)
	return err
}

// reduceStaleVolumeGroup prunes missing physical volumes (references to local SSDs) from the VG metadata under the specified device filter.
func reduceStaleVolumeGroup(vgName, configFilter string) error {
	reduceArgs := []string{"--removemissing", vgName, "--force", "--config", configFilter}
	_, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgreduce", reduceArgs...)
	return err
}

// renameVolumeGroup renames the Volume Group under the specified device filter to resolve the duplicate naming conflict.
func renameVolumeGroup(oldName, newName, configFilter string) error {
	renameArgs := []string{oldName, newName, "--config", configFilter}
	_, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgrename", renameArgs...)
	return err
}

// regenerateVolumeGroupUuid regenerates the Volume Group UUID under the specified device filter to resolve the duplicate VGID conflict.
func regenerateVolumeGroupUuid(vgName, configFilter string) error {
	vgchangeArgs := []string{"-u", vgName, "--config", configFilter}
	_, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgchange", vgchangeArgs...)
	return err
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
	node, err := k8sclient.GetNodeWithRetry(ctx, nodeName)
	if err != nil {
		return 0, err
	}

	if val, found := node.GetLabels()[fmt.Sprintf(constants.NodeLabelPrefix, constants.DataCacheLssdCountLabel)]; found {
		dataCacheCount, err := strconv.Atoi(val)
		if err != nil {
			return 0, fmt.Errorf("Error getting Data Cache's LSSD count from node label: %v", err)
		}
		klog.V(4).Infof("Number of local SSDs requested for Data Cache: %v", dataCacheCount)
		return dataCacheCount, nil
	}
	return 0, nil
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
	// Compile regex to match GCE Local SSD model names (e.g., "nvme_card" or "nvme_card1")
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

// getVgInfoForPv queries LVM metadata for the physical volume (PV) path under a device-specific filter.
// It returns the associated Volume Group (VG) name, VG UUID, and any error.
func getVgInfoForPv(pvPath string) (string, string, error) {
	resolvedPvPath, err := filepath.EvalSymlinks(pvPath)
	if err != nil {
		resolvedPvPath = pvPath // Fallback
	}

	configFilter := fmt.Sprintf("devices { filter = [ \"a|%s|\", \"r|.*|\" ] }", resolvedPvPath)
	args := []string{
		"--noheadings",
		"-o",
		"vg_name,vg_uuid,pv_name",
		"--config",
		configFilter,
	}
	info, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "pvs", args...)
	if err != nil {
		return "", "", fmt.Errorf("failed to run pvs: %w", err)
	}
	lines := strings.Split(string(info), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		// The fields slice must contain at least 3 elements representing:
		// fields[0] = vg_name, fields[1] = vg_uuid, fields[2] = pv_name
		if len(fields) >= 3 {
			resolvedFieldName, err := filepath.EvalSymlinks(fields[2])
			if err != nil {
				resolvedFieldName = fields[2]
			}
			if resolvedFieldName == resolvedPvPath {
				vgName := fields[0]
				vgUuid := fields[1]
				if vgName == "" || vgName == "-" {
					vgName = ""
				}
				if vgUuid == "" || vgUuid == "-" {
					vgUuid = ""
				}
				return vgName, vgUuid, nil
			}
		}
	}
	return "", "", nil
}

// isLvCachedOnDevice checks if the logical volume (LV) in the volume group (VG) is cached.
// It queries LVM using 'lvs' with a custom field selection, returning true if the volume
// is associated with a cache pool ('csi-fast'), and false otherwise.
func isLvCachedOnDevice(vgName, lvName, pvPath string) (bool, error) {
	resolvedPvPath, err := filepath.EvalSymlinks(pvPath)
	if err != nil {
		resolvedPvPath = pvPath
	}

	configFilter := fmt.Sprintf("devices { filter = [ \"a|%s|\", \"r|.*|\" ] }", resolvedPvPath)
	args := []string{
		"--noheadings",
		"--select", "lv_name=" + lvName + " && vg_name=" + vgName,
		"-o", "pool_lv",
		"--config", configFilter,
	}
	output, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvs", args...)
	if err != nil {
		// If lvs exits with a warning/error (which it always does when a PV is missing),
		// we still want to parse the output. If the VG/LV genuinely doesn't exist,
		// the output will be empty and we will correctly return false.
		klog.V(4).Infof("lvs returned warning/error while checking cache on %s: %v. Parsing output anyway.", pvPath, err)
	}
	return strings.Contains(string(output), "csi-fast"), nil
}

// mergeStaleVgIntoHost deactivates, uncaches, and reduces a stale Volume Group on the GCE PD,
// and merges its physical volume into the host's active Volume Group pool.
// Encapsulates stale VG merging logic.
func mergeStaleVgIntoHost(vgNameForPv, volumeGroupName, mainLvName string) error {
	klog.Infof("Stale VG %s found on GCE PD. Activating it first to perform cleanup.", vgNameForPv)
	// Ensure the stale VG is active so we can modify its metadata (Comment: Activate stale VG for cleanup)
	_, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgchange", []string{"-ay", vgNameForPv}...)
	if err != nil {
		klog.Errorf("Errored while activating stale VG %v: err: %v", vgNameForPv, err)
	}

	_, isCached := isCachingSetup(mainLvName)
	if isCached {
		klog.Infof("Stale volume %s/%s is cached. Dissociating (uncaching) cache pool to preserve data.", vgNameForPv, mainLvName)
		args := []string{
			"--uncache",
			vgNameForPv + "/" + mainLvName,
			"--force",
			"-y", // force remove cache without flushing data
		}
		_, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "lvconvert", args...)
		if err != nil {
			return fmt.Errorf("errored while uncaching main LV: %w", err)
		}
	}

	// Clean up the stale Volume Group to remove the missing local SSD PV references while it is active
	reduceVolumeGroup(vgNameForPv, true)

	// Deactivate ONLY the source VG to release its locks before running vgmerge (Comment: Release locks on source VG only)
	klog.Infof("Deactivating source VG %s to release locks before merging...", vgNameForPv)
	_, _ = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgchange", []string{"-an", vgNameForPv}...)

	// Merge the GCE PD's VG into the host's VG (destination VG remains active/in-use!)
	_, err = common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgmerge", []string{volumeGroupName, vgNameForPv}...)
	if err != nil {
		return fmt.Errorf("Errored while merging the PV Volume group %s into %s %w", vgNameForPv, volumeGroupName, err)
	}
	return nil
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

// ensureLocalSsdInVolumeGroup checks if the local SSD cache physical volume is registered
// in the host Volume Group, and extends the VG to add it back if it is missing (e.g. after a node preemption reset).
func ensureLocalSsdInVolumeGroup(volumeGroupName, raidedLocalSsdPath string) error {
	ssdVgName, _, err := getVgInfoForPv(raidedLocalSsdPath)
	if err != nil {
		klog.Warningf("Local SSD PV %s is not registered in LVM (it might have been wiped): %v. Re-adding it to Volume Group %s.", raidedLocalSsdPath, err, volumeGroupName)
	}
	if ssdVgName != volumeGroupName {
		klog.Infof("Extending Volume Group %s to include local SSD cache PV %s", volumeGroupName, raidedLocalSsdPath)
		info, err := common.RunCommand("" /* pipedCmd */, nil /* pipedCmdArg */, "vgextend", []string{volumeGroupName, raidedLocalSsdPath}...)
		if err != nil {
			return fmt.Errorf("failed to extend VG %s with local SSD PV %s: %w (output: %s)", volumeGroupName, raidedLocalSsdPath, err, string(info))
		}
	}
	return nil
}

func reduceVolumeGroup(volumeGroupName string, force bool) {
	if !checkVgExists(volumeGroupName) {
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

// cacheSize is always in GiB
func fetchChunkSizeKiB(cacheSize string) (string, error) {
	var chunkSize float64

	cacheSize = strings.TrimSuffix(cacheSize, "GiB")
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

func StartWatcher(ctx context.Context, nodeName string) {
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
	go watchDiskDetaches(ctx, watcher, nodeName, errorCh)

	select {
	case err := <-errorCh:
		klog.Errorf("watcher encountered an error: %v", err)
	}
}

func watchDiskDetaches(ctx context.Context, watcher *fsnotify.Watcher, nodeName string, errorCh chan error) error {
	for {
		select {
		case <-ctx.Done():
			klog.Infof("Context done, stopping watcher")
			return nil
		// watch for errors
		case err := <-watcher.Errors:
			errorCh <- fmt.Errorf("disk update event errored: %v", err)
		// watch for events
		case <-watcher.Events:
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
		"-o", "pv_name,pv_size",
		"--noheadings",
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
