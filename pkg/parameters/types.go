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

// DiskParameters contains normalized and defaulted disk parameters
type DiskParameters struct {
	// Values: pd-standard, pd-balanced, pd-ssd, or any other PD disk type. Not validated.
	// Default: pd-standard
	DiskType string
	// Values: any PD type. Not validated.
	// Default: ""
	PDType string
	// Values: any HD type. Not validated.
	// Default: ""
	HDType string
	// Values: pdType, hdType
	// Default: hdType
	DiskTypePreference string
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
	AccessMode string
	// Values: {bool}
	// Default: false
	UseAllowedDiskTopology bool
}

func (dp *DiskParameters) IsRegional() bool {
	return dp.ReplicationType == "regional-pd" || dp.DiskType == DiskTypeHdHA
}

func (dp *DiskParameters) IsDiskDynamic() bool {
	return dp.DiskType == DynamicVolumeType
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
	DriverName           string
	EnableStoragePools   bool
	EnableMultiZone      bool
	EnableHdHA           bool
	EnableDiskTopology   bool
	EnableDataCache      bool
	EnableDynamicVolumes bool
	ExtraVolumeLabels    map[string]string
	ExtraTags            map[string]string
}

func (pp *ParameterProcessor) isHDHADisabled() bool {
	return !pp.EnableHdHA
}

func (pp *ParameterProcessor) isStoragePoolDisabled() bool {
	return !pp.EnableStoragePools
}

func (pp *ParameterProcessor) isDataCacheDisabled() bool {
	return !pp.EnableDataCache
}

func (pp *ParameterProcessor) isMultiZoneDisabled() bool {
	return !pp.EnableMultiZone
}

func (pp *ParameterProcessor) isDiskTopologyDisabled() bool {
	return !pp.EnableDiskTopology
}

func (pp *ParameterProcessor) isDynamicVolumesDisabled() bool {
	return !pp.EnableDynamicVolumes
}

type ModifyVolumeParameters struct {
	IOPS       *int64
	Throughput *int64
}
