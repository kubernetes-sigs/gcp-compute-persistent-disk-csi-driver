package parameters

import "strings"

// Map of sanitizers for parameters that apply to only a subset of disk types. Each sanitizeer
// should only be responsible for a single parameter.
var sanitizers = map[string]sanitizer{
	"ReplicationType": func(dp *DiskParameters) {
		// Only sanitize replication type if parameter has been specified.
		if isHD(dp.DiskType) && dp.ReplicationType != "" {
			dp.ReplicationType = replicationTypeNone
		}
	},
	"ProvisionedIOPSOnCreate": func(dp *DiskParameters) {
		if isPD(dp.DiskType) && dp.DiskType != "pd-extreme" {
			dp.ProvisionedIOPSOnCreate = 0
		}
	},
	"ProvisionedThroughputOnCreate": func(dp *DiskParameters) {
		if isPD(dp.DiskType) {
			dp.ProvisionedThroughputOnCreate = 0
		}
	},
	"StoragePools": func(dp *DiskParameters) {
		if isPD(dp.DiskType) {
			dp.StoragePools = nil
		}
	},
	"ForceAttach": func(dp *DiskParameters) {
		if isHD(dp.DiskType) {
			dp.ForceAttach = false
		}
	},
}

func isHD(diskType string) bool {
	return strings.HasPrefix(diskType, "hyperdisk-")
}

func isPD(diskType string) bool {
	return strings.HasPrefix(diskType, "pd-")
}

type sanitizer func(dp *DiskParameters)

// sanitizeDiskParameters removes any parameters that are not applicable to the specified disk type,
// and is intended to only be used for dynamic volumes.
func sanitizeDiskParameters(dp *DiskParameters) {
	if dp == nil {
		return
	}
	for _, sanitizer := range sanitizers {
		sanitizer(dp)
	}
}
