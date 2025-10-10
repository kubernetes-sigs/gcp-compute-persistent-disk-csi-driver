package parameters

type StoragePool struct {
	Project      string
	Zone         string
	Name         string
	ResourceName string
}

type DataCacheParameters struct {
	// Values: {string} in int64 form
	// Default: ""
	DataCacheSize string
	// Values: writethrough, writeback
	// Default: writethrough
	DataCacheMode string
}

type diskTypePreference string

const (
	pd diskTypePreference = DiskTypePreferencePd
	hd diskTypePreference = DiskTypePreferenceHd
)

// DiskParameters contains normalized and defaulted disk parameters
type DiskParameters struct {
	// Values: pd-standard, pd-balanced, pd-ssd, or any other PD disk type. Not validated.
	// Default: pd-standard
	DiskType   string
	pdType     string
	hdType     string
	preference diskTypePreference // If user doesn't specify this, set it to hd.
	// Values: "none", regional-pd
	// Default: "none"
	ReplicationType string
	// Values: {string}
	// Default: ""
	DiskEncryptionKMSKey string
	// Values: {map[string]string}
	// Default: ""
	Tags map[string]string
	// Values: {map[string]string}
	// Default: ""
	Labels map[string]string
	// Values: {int64}
	// Default: none
	ProvisionedIOPSOnCreate int64
	// Values: {int64}
	// Default: none
	ProvisionedThroughputOnCreate int64
	// Values: {bool}
	// Default: false
	EnableConfidentialCompute bool
	// Values: {bool}
	// Default: false
	ForceAttach bool
	// Values: {[]string}
	// Default: ""
	StoragePools []StoragePool
	// Values: {map[string]string}
	// Default: ""
	ResourceTags map[string]string
	// Values: {bool}
	// Default: false
	MultiZoneProvisioning bool
	// Values: READ_WRITE_SINGLE, READ_ONLY_MANY, READ_WRITE_MANY
	// Default: READ_WRITE_SINGLE

	// JULIAN: This is unused in the codebase AFAICT.  is that intentional?  Should this be removed?
	AccessMode string
	// Values: {bool}
	// Default: false
	UseAllowedDiskTopology bool
}

func (dp *DiskParameters) IsRegional() bool {
	return dp.ReplicationType == "regional-pd" || dp.DiskType == DiskTypeHdHA
}

// SnapshotParameters contains normalized and defaulted parameters for snapshots
type SnapshotParameters struct {
	StorageLocations []string
	SnapshotType     string
	ImageFamily      string
	Tags             map[string]string
	Labels           map[string]string
	ResourceTags     map[string]string
}

type ParameterProcessor struct {
	DriverName         string
	EnableStoragePools bool
	EnableMultiZone    bool
	EnableHdHA         bool
	EnableDiskTopology bool
	ExtraVolumeLabels  map[string]string
	EnableDataCache    bool
	ExtraTags          map[string]string
}

type ModifyVolumeParameters struct {
	IOPS       *int64
	Throughput *int64
}
