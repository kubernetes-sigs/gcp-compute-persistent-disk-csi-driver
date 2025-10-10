package parameters

const (
	// Disk Params
	ParameterAccessMode = "access-mode"

	// Parameters for StorageClass
	ParameterKeyType                          = "type"
	ParameterKeyReplicationType               = "replication-type"
	ParameterKeyDiskEncryptionKmsKey          = "disk-encryption-kms-key"
	ParameterKeyLabels                        = "labels"
	ParameterKeyProvisionedIOPSOnCreate       = "provisioned-iops-on-create"
	ParameterKeyProvisionedThroughputOnCreate = "provisioned-throughput-on-create"
	ParameterAvailabilityClass                = "availability-class"
	ParameterKeyEnableConfidentialCompute     = "enable-confidential-storage"
	ParameterKeyStoragePools                  = "storage-pools"
	ParameterKeyUseAllowedDiskTopology        = "use-allowed-disk-topology"
	ParameterKeyPdType                        = "pd-type"
	ParameterKeyHdType                        = "hyperdisk-type"
	ParameterKeyDiskTypePreference            = "disk-type-preference"

	// Keys for disk type preference
	DiskTypePreferencePd = ParameterKeyPdType
	DiskTypePreferenceHd = ParameterKeyHdType

	// Parameters for Data Cache
	ParameterKeyDataCacheSize               = "data-cache-size"
	ParameterKeyDataCacheMode               = "data-cache-mode"
	ParameterKeyResourceTags                = "resource-tags"
	ParameterKeyEnableMultiZoneProvisioning = "enable-multi-zone-provisioning"

	// Parameters for VolumeSnapshotClass
	ParameterKeyStorageLocations = "storage-locations"
	ParameterKeySnapshotType     = "snapshot-type"
	ParameterKeyImageFamily      = "image-family"
	replicationTypeNone          = "none"

	// Keys for PV and PVC parameters as reported by external-provisioner
	ParameterKeyPVCName      = "csi.storage.k8s.io/pvc/name"
	ParameterKeyPVCNamespace = "csi.storage.k8s.io/pvc/namespace"
	ParameterKeyPVName       = "csi.storage.k8s.io/pv/name"

	// Keys for tags to put in the provisioned disk description
	tagKeyCreatedForClaimNamespace = "kubernetes.io/created-for/pvc/namespace"
	tagKeyCreatedForClaimName      = "kubernetes.io/created-for/pvc/name"
	tagKeyCreatedForVolumeName     = "kubernetes.io/created-for/pv/name"
	tagKeyCreatedBy                = "storage.gke.io/created-by"

	// Keys for Snapshot and SnapshotContent parameters as reported by external-snapshotter
	ParameterKeyVolumeSnapshotName        = "csi.storage.k8s.io/volumesnapshot/name"
	ParameterKeyVolumeSnapshotNamespace   = "csi.storage.k8s.io/volumesnapshot/namespace"
	ParameterKeyVolumeSnapshotContentName = "csi.storage.k8s.io/volumesnapshotcontent/name"

	// Parameters for AvailabilityClass
	ParameterNoAvailabilityClass       = "none"
	ParameterRegionalHardFailoverClass = "regional-hard-failover"

	// Keys for tags to put in the provisioned snapshot description
	tagKeyCreatedForSnapshotName        = "kubernetes.io/created-for/volumesnapshot/name"
	tagKeyCreatedForSnapshotNamespace   = "kubernetes.io/created-for/volumesnapshot/namespace"
	tagKeyCreatedForSnapshotContentName = "kubernetes.io/created-for/volumesnapshotcontent/name"

	// Hyperdisk disk types
	DiskTypeHdHA = "hyperdisk-balanced-high-availability"
	DiskTypeHdT  = "hyperdisk-throughput"
	DiskTypeHdE  = "hyperdisk-extreme"
	DiskTypeHdML = "hyperdisk-ml"

	// Parameters for VolumeSnapshotClass
	DiskSnapshotType = "snapshots"
	DiskImageType    = "images"
)
