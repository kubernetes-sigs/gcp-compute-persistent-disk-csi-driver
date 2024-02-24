/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"fmt"
	"strings"

	"k8s.io/klog/v2"
)

const (
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

	// Parameters for Data Cache
	ParameterKeyDataCacheSize               = "data-cache-size"
	ParameterKeyDataCacheMode               = "data-cache-mode"
	ParameterKeyResourceTags                = "resource-tags"
	ParameterKeyEnableMultiZoneProvisioning = "enable-multi-zone-provisioning"

	// Parameters for VolumeSnapshotClass
	ParameterKeyStorageLocations = "storage-locations"
	ParameterKeySnapshotType     = "snapshot-type"
	ParameterKeyImageFamily      = "image-family"
	DiskSnapshotType             = "snapshots"
	DiskImageType                = "images"
	replicationTypeNone          = "none"

	// Parameters for AvailabilityClass
	ParameterNoAvailabilityClass       = "none"
	ParameterRegionalHardFailoverClass = "regional-hard-failover"

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

	// Keys for tags to put in the provisioned snapshot description
	tagKeyCreatedForSnapshotName        = "kubernetes.io/created-for/volumesnapshot/name"
	tagKeyCreatedForSnapshotNamespace   = "kubernetes.io/created-for/volumesnapshot/namespace"
	tagKeyCreatedForSnapshotContentName = "kubernetes.io/created-for/volumesnapshotcontent/name"
)

type DataCacheParameters struct {
	// Values: {string}
	// Default: ""
	// Example: "25Gi"
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

type StoragePool struct {
	Project      string
	Zone         string
	Name         string
	ResourceName string
}

type ParameterProcessor struct {
	DriverName         string
	EnableStoragePools bool
	EnableMultiZone    bool
}

type ModifyVolumeParameters struct {
	IOPS       *int64
	Throughput *int64
}

// ExtractAndDefaultParameters will take the relevant parameters from a map and
// put them into a well defined struct making sure to default unspecified fields.
// extraVolumeLabels are added as labels; if there are also labels specified in
// parameters, any matching extraVolumeLabels will be overridden.
func (pp *ParameterProcessor) ExtractAndDefaultParameters(parameters map[string]string, extraVolumeLabels map[string]string, enableDataCache bool, extraTags map[string]string) (DiskParameters, DataCacheParameters, error) {
	p := DiskParameters{
		DiskType:             "pd-standard",           // Default
		ReplicationType:      replicationTypeNone,     // Default
		DiskEncryptionKMSKey: "",                      // Default
		Tags:                 make(map[string]string), // Default
		Labels:               make(map[string]string), // Default
		ResourceTags:         make(map[string]string), // Default
	}

	// Set data cache feature default
	d := DataCacheParameters{}
	if enableDataCache {
		d.DataCacheMode = "writethrough"
	}

	for k, v := range extraVolumeLabels {
		p.Labels[k] = v
	}

	for k, v := range extraTags {
		p.ResourceTags[k] = v
	}

	for k, v := range parameters {
		if k == "csiProvisionerSecretName" || k == "csiProvisionerSecretNamespace" {
			// These are hardcoded secrets keys required to function but not needed by GCE PD
			continue
		}
		switch strings.ToLower(k) {
		case ParameterKeyType:
			if v != "" {
				p.DiskType = strings.ToLower(v)
			}
		case ParameterKeyReplicationType:
			if v != "" {
				p.ReplicationType = strings.ToLower(v)
			}
		case ParameterKeyDiskEncryptionKmsKey:
			// Resource names (e.g. "keyRings", "cryptoKeys", etc.) are case sensitive, so do not change case
			p.DiskEncryptionKMSKey = v
		case ParameterKeyPVCName:
			p.Tags[tagKeyCreatedForClaimName] = v
		case ParameterKeyPVCNamespace:
			p.Tags[tagKeyCreatedForClaimNamespace] = v
		case ParameterKeyPVName:
			p.Tags[tagKeyCreatedForVolumeName] = v
		case ParameterKeyLabels:
			paramLabels, err := ConvertLabelsStringToMap(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contain invalid labels parameter: %w", err)
			}
			// Override any existing labels with those from this parameter.
			for labelKey, labelValue := range paramLabels {
				p.Labels[labelKey] = labelValue
			}
		case ParameterKeyProvisionedIOPSOnCreate:
			paramProvisionedIOPSOnCreate, err := ConvertStringToInt64(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contain invalid provisionedIOPSOnCreate parameter: %w", err)
			}
			p.ProvisionedIOPSOnCreate = paramProvisionedIOPSOnCreate
		case ParameterKeyProvisionedThroughputOnCreate:
			paramProvisionedThroughputOnCreate, err := ConvertMiStringToInt64(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contain invalid provisionedThroughputOnCreate parameter: %w", err)
			}
			if paramProvisionedThroughputOnCreate < 0 {
				return p, d, fmt.Errorf("parameter provisionedThroughputOnCreate cannot be negative")
			}
			p.ProvisionedThroughputOnCreate = paramProvisionedThroughputOnCreate
		case ParameterAvailabilityClass:
			paramAvailabilityClass, err := ConvertStringToAvailabilityClass(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contain invalid availability class parameter: %w", err)
			}
			if paramAvailabilityClass == ParameterRegionalHardFailoverClass {
				p.ForceAttach = true
			}
		case ParameterKeyEnableConfidentialCompute:
			paramEnableConfidentialCompute, err := ConvertStringToBool(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contain invalid value for enable-confidential-storage parameter: %w", err)
			}

			if paramEnableConfidentialCompute {
				// DiskEncryptionKmsKey is needed to enable confidentialStorage
				if val, ok := parameters[ParameterKeyDiskEncryptionKmsKey]; !ok || !isValidDiskEncryptionKmsKey(val) {
					return p, d, fmt.Errorf("Valid %v is required to enable ConfidentialStorage", ParameterKeyDiskEncryptionKmsKey)
				}
			}

			p.EnableConfidentialCompute = paramEnableConfidentialCompute
		case ParameterKeyStoragePools:
			if !pp.EnableStoragePools {
				return p, d, fmt.Errorf("parameters contains invalid option %q", ParameterKeyStoragePools)
			}
			storagePools, err := ParseStoragePools(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contains invalid value for %s parameter %q: %w", ParameterKeyStoragePools, v, err)
			}
			p.StoragePools = storagePools
		case ParameterKeyDataCacheSize:
			if !enableDataCache {
				return p, d, fmt.Errorf("parameters contains invalid option %q", ParameterKeyDataCacheSize)
			}
			// TODO: need to parse or validate the string
			d.DataCacheSize = v
			klog.V(2).Infof("====== Data cache size is %v ======", v)
		case ParameterKeyDataCacheMode:
			if !enableDataCache {
				return p, d, fmt.Errorf("parameters contains invalid option %q", ParameterKeyDataCacheSize)
			}
			d.DataCacheMode = v
			klog.V(2).Infof("====== Data cache mode is %v ======", v)
		case ParameterKeyResourceTags:
			if err := extractResourceTagsParameter(v, p.ResourceTags); err != nil {
				return p, d, err
			}
		case ParameterKeyEnableMultiZoneProvisioning:
			if !pp.EnableMultiZone {
				return p, d, fmt.Errorf("parameters contains invalid option %q", ParameterKeyEnableMultiZoneProvisioning)
			}
			paramEnableMultiZoneProvisioning, err := ConvertStringToBool(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contain invalid value for %s parameter: %w", ParameterKeyEnableMultiZoneProvisioning, err)
			}

			p.MultiZoneProvisioning = paramEnableMultiZoneProvisioning
			if paramEnableMultiZoneProvisioning {
				p.Labels[MultiZoneLabel] = "true"
			}
		default:
			return p, d, fmt.Errorf("parameters contains invalid option %q", k)
		}
	}
	if len(p.Tags) > 0 {
		p.Tags[tagKeyCreatedBy] = pp.DriverName
	}
	return p, d, nil
}

func ExtractAndDefaultSnapshotParameters(parameters map[string]string, driverName string, extraTags map[string]string) (SnapshotParameters, error) {
	p := SnapshotParameters{
		StorageLocations: []string{},
		SnapshotType:     DiskSnapshotType,
		Tags:             make(map[string]string), // Default
		Labels:           make(map[string]string), // Default
		ResourceTags:     make(map[string]string), // Default
	}

	for k, v := range extraTags {
		p.ResourceTags[k] = v
	}

	for k, v := range parameters {
		switch strings.ToLower(k) {
		case ParameterKeyStorageLocations:
			normalizedStorageLocations, err := ProcessStorageLocations(v)
			if err != nil {
				return p, err
			}
			p.StorageLocations = normalizedStorageLocations
		case ParameterKeySnapshotType:
			err := ValidateSnapshotType(v)
			if err != nil {
				return p, err
			}
			p.SnapshotType = v
		case ParameterKeyImageFamily:
			p.ImageFamily = v
		case ParameterKeyVolumeSnapshotName:
			p.Tags[tagKeyCreatedForSnapshotName] = v
		case ParameterKeyVolumeSnapshotNamespace:
			p.Tags[tagKeyCreatedForSnapshotNamespace] = v
		case ParameterKeyVolumeSnapshotContentName:
			p.Tags[tagKeyCreatedForSnapshotContentName] = v
		case ParameterKeyLabels:
			paramLabels, err := ConvertLabelsStringToMap(v)
			if err != nil {
				return p, fmt.Errorf("parameters contain invalid labels parameter: %w", err)
			}
			// Override any existing labels with those from this parameter.
			for labelKey, labelValue := range paramLabels {
				p.Labels[labelKey] = labelValue
			}
		case ParameterKeyResourceTags:
			if err := extractResourceTagsParameter(v, p.ResourceTags); err != nil {
				return p, err
			}
		default:
			return p, fmt.Errorf("parameters contains invalid option %q", k)
		}
	}
	if len(p.Tags) > 0 {
		p.Tags[tagKeyCreatedBy] = driverName
	}
	return p, nil
}

func extractResourceTagsParameter(tagsString string, resourceTags map[string]string) error {
	paramResourceTags, err := ConvertTagsStringToMap(tagsString)
	if err != nil {
		return fmt.Errorf("parameters contain invalid %s parameter: %w", ParameterKeyResourceTags, err)
	}
	// Override any existing resource tags with those from this parameter.
	for tagParentIDKey, tagValue := range paramResourceTags {
		resourceTags[tagParentIDKey] = tagValue
	}
	return nil
}

func ExtractModifyVolumeParameters(parameters map[string]string) (ModifyVolumeParameters, error) {

	modifyVolumeParams := ModifyVolumeParameters{}

	for key, value := range parameters {
		switch strings.ToLower(key) {
		case "iops":
			iops, err := ConvertStringToInt64(value)
			if err != nil {
				return ModifyVolumeParameters{}, fmt.Errorf("parameters contain invalid iops parameter: %w", err)
			}
			modifyVolumeParams.IOPS = &iops
		case "throughput":
			throughput, err := ConvertMiStringToInt64(value)
			if err != nil {
				return ModifyVolumeParameters{}, fmt.Errorf("parameters contain invalid throughput parameter: %w", err)
			}
			modifyVolumeParams.Throughput = &throughput
		default:
			return ModifyVolumeParameters{}, fmt.Errorf("parameters contain unknown parameter: %s", key)
		}
	}
	return modifyVolumeParams, nil
}
