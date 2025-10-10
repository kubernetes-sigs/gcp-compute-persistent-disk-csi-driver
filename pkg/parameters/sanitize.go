package parameters

import (
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

func selectDiskType(dp *DiskParameters, topologies []*csi.Topology) {
	// Collect disk type labels from the first topology if at least one exists.
	var supportedDisks []string
	if len(topologies) > 0 {
		for key := range topologies[0].Segments {
			diskType := common.DiskTypeFromLabelKey(key)
			supportedDisks = append(supportedDisks, diskType)
		}
	}

	selectedDiskType := ""
	for _, supportedDisk := range supportedDisks {
		// Choose either disk type if it is supported.
		if supportedDisk == dp.hdType || supportedDisk == dp.pdType {

			// If we are finding the first supported disk, set it without considering the preference.
			if selectedDiskType == "" {
				selectedDiskType = supportedDisk
				continue
			}

			// If we have already found a supported disk, we should only
			// override if our override matches the user's stated preference.
			if diskTypeMatchesPreference(supportedDisk, dp.preference) {
				selectedDiskType = supportedDisk
				continue
			}
		}
	}

	// If we found nothing supported, just choose the preference.
	if selectedDiskType == "" {
		if dp.preference == pd {
			selectedDiskType = dp.pdType
		} else if dp.preference == hd {
			selectedDiskType = dp.hdType
		}
	}

	dp.DiskType = selectedDiskType
}

func diskTypeMatchesPreference(diskType string, preference diskTypePreference) bool {
	if preference == pd {
		return diskTypeIsPd(diskType)
	} else if preference == hd {
		return diskTypeIsHyperdisk(diskType)
	}

	klog.Warningf("Disk type %q is not expected %q or %q, returning false", diskType, pd, hd)
	return false
}

func diskTypeIsHyperdisk(diskType string) bool {
	return strings.HasPrefix(diskType, "hyperdisk-")
}

func diskTypeIsPd(diskType string) bool {
	return strings.HasPrefix(diskType, "pd-")
}

// sanitizeDiskParameters sanitizes the disk parameters according to the disk
// type.  This prevents values from being passed to the eventual GCE API calls
// that would cause those calls to error.  This is a prerequisite for a generic
// volume, as parameters will be specified for two different disk types but
// where those parameters are valid for only one of the types.
func sanitizeDiskParameters(dp *DiskParameters) {
	klog.Infof("Sanitizing disk parameters for disk type %s, parameters before: %+v", dp.DiskType, dp)
	for _, sanitizer := range sanitizers {
		sanitizer(dp)
	}
	klog.Infof("Sanitizing disk parameters for disk type %s, parameters after: %+v", dp.DiskType, dp)
}

// sanitizer is a function that sanitizes a specific field of the disk parameters based on the DiskType parameter.
type sanitizer func(dp *DiskParameters)

// sanitizers cover all parameters that are valid for only of PD or HD.
var sanitizers = map[string]sanitizer{
	"ReplicationType": func(dp *DiskParameters) {
		// Replication type supported in PD only
		if diskTypeIsPd(dp.DiskType) {
			dp.ReplicationType = replicationTypeNone
		}
	},
	"ProvisionedIOPSOnCreate": func(dp *DiskParameters) {
		if diskTypeIsPd(dp.DiskType) {
			dp.ProvisionedIOPSOnCreate = 0
		}
	},
	"ProvisionedThroughputOnCreate": func(dp *DiskParameters) {
		if diskTypeIsPd(dp.DiskType) {
			dp.ProvisionedThroughputOnCreate = 0
		}
	},
	// Does ForceAttach need to be sanitized?
	"StoragePools": func(dp *DiskParameters) {
		if diskTypeIsPd(dp.DiskType) {
			dp.StoragePools = nil
		}
	},
	"MultiZoneProvisioning": func(dp *DiskParameters) {
		if diskTypeIsPd(dp.DiskType) {
			dp.MultiZoneProvisioning = false
		}
	},
}
