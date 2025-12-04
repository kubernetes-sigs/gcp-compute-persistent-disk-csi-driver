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

package parameters

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/convert"
)

// ExtractAndDefaultParameters will take the relevant parameters from a map and
// put them into a well defined struct making sure to default unspecified fields.
func (pp *ParameterProcessor) ExtractAndDefaultParameters(parameters map[string]string) (DiskParameters, DataCacheParameters, error) {
	p := DiskParameters{
		DiskType:             "pd-standard",           // Default
		ReplicationType:      replicationTypeNone,     // Default
		DiskEncryptionKMSKey: "",                      // Default
		Tags:                 make(map[string]string), // Default
		Labels:               make(map[string]string), // Default
		ResourceTags:         make(map[string]string), // Default
	}

	// Set data cache mode default
	d := DataCacheParameters{}
	if pp.EnableDataCache && parameters[ParameterKeyDataCacheSize] != "" {
		d.DataCacheMode = constants.DataCacheModeWriteThrough
	}

	for k, v := range pp.ExtraVolumeLabels {
		p.Labels[k] = v
	}

	for k, v := range pp.ExtraTags {
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
				if pp.isHDHADisabled() && p.DiskType == DiskTypeHdHA {
					return p, d, fmt.Errorf("parameters contain invalid disk type %s", DiskTypeHdHA)
				}
				if p.DiskType == DynamicVolumeType {
					if pp.isDynamicVolumesDisabled() {
						return p, d, fmt.Errorf("parameters contain invalid disk type %s, must enable dynamic volumes", DynamicVolumeType)
					}
				}

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
			paramLabels, err := convert.ConvertLabelsStringToMap(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contain invalid labels parameter: %w", err)
			}
			// Override any existing labels with those from this parameter.
			for labelKey, labelValue := range paramLabels {
				p.Labels[labelKey] = labelValue
			}
		case ParameterKeyProvisionedIOPSOnCreate:
			paramProvisionedIOPSOnCreate, err := convert.ConvertStringToInt64(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contain invalid provisionedIOPSOnCreate parameter: %w", err)
			}
			p.ProvisionedIOPSOnCreate = paramProvisionedIOPSOnCreate
		case ParameterKeyProvisionedThroughputOnCreate:
			paramProvisionedThroughputOnCreate, err := convert.ConvertMiStringToInt64(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contain invalid provisionedThroughputOnCreate parameter: %w", err)
			}
			if paramProvisionedThroughputOnCreate < 0 {
				return p, d, fmt.Errorf("parameter provisionedThroughputOnCreate cannot be negative")
			}
			p.ProvisionedThroughputOnCreate = paramProvisionedThroughputOnCreate
		case ParameterAvailabilityClass:
			paramAvailabilityClass, err := convertStringToAvailabilityClass(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contain invalid availability class parameter: %w", err)
			}
			if paramAvailabilityClass == ParameterRegionalHardFailoverClass {
				p.ForceAttach = true
			}
		case ParameterKeyEnableConfidentialCompute:
			paramEnableConfidentialCompute, err := convert.ConvertStringToBool(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contain invalid value for enable-confidential-storage parameter: %w", err)
			}

			if paramEnableConfidentialCompute {
				// DiskEncryptionKmsKey is needed to enable confidentialStorage
				if val, ok := parameters[ParameterKeyDiskEncryptionKmsKey]; !ok || !isValidDiskEncryptionKmsKey(val) {
					return p, d, fmt.Errorf("valid %v is required to enable ConfidentialStorage", ParameterKeyDiskEncryptionKmsKey)
				}
			}

			p.EnableConfidentialCompute = paramEnableConfidentialCompute
		case ParameterKeyStoragePools:
			if pp.isStoragePoolDisabled() {
				return p, d, fmt.Errorf("parameters contains invalid option %q", ParameterKeyStoragePools)
			}
			storagePools, err := ParseStoragePools(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contains invalid value for %s parameter %q: %w", ParameterKeyStoragePools, v, err)
			}
			p.StoragePools = storagePools
		case ParameterKeyDataCacheSize:
			if pp.isDataCacheDisabled() {
				return p, d, fmt.Errorf("data caching enabled: %v; parameters contains invalid option %q", pp.EnableDataCache, ParameterKeyDataCacheSize)
			}

			paramDataCacheSize, err := convert.ConvertGiStringToInt64(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contain invalid dataCacheSize parameter: %w", err)
			}
			if err := ValidateNonNegativeInt(paramDataCacheSize); err != nil {
				return p, d, fmt.Errorf("parameters contains invalid option: %s: %w", ParameterKeyDataCacheSize, err)
			}
			d.DataCacheSize = strconv.FormatInt(paramDataCacheSize, 10)
		case ParameterKeyDataCacheMode:
			if pp.isDataCacheDisabled() {
				return p, d, fmt.Errorf("data caching enabled %v; parameters contains invalid option %q", pp.EnableDataCache, ParameterKeyDataCacheMode)
			}
			if err := ValidateDataCacheMode(v); err != nil {
				return p, d, fmt.Errorf("parameters contains invalid option: %s: %w", ParameterKeyDataCacheMode, err)
			}
			d.DataCacheMode = v
		case ParameterKeyResourceTags:
			if err := extractResourceTagsParameter(v, p.ResourceTags); err != nil {
				return p, d, err
			}
		case ParameterKeyEnableMultiZoneProvisioning:
			if pp.isMultiZoneDisabled() {
				return p, d, fmt.Errorf("parameters contains invalid option %q", ParameterKeyEnableMultiZoneProvisioning)
			}
			paramEnableMultiZoneProvisioning, err := convert.ConvertStringToBool(v)
			if err != nil {
				return p, d, fmt.Errorf("parameters contain invalid value for %s parameter: %w", ParameterKeyEnableMultiZoneProvisioning, err)
			}

			p.MultiZoneProvisioning = paramEnableMultiZoneProvisioning
			if paramEnableMultiZoneProvisioning {
				p.Labels[constants.MultiZoneLabel] = "true"
			}
		case ParameterAccessMode:
			if v != "" {
				p.AccessMode = v
			}
		case ParameterKeyUseAllowedDiskTopology:
			if pp.isDiskTopologyDisabled() {
				klog.Warningf("parameters contains invalid option %q when disk topology is not enabled", ParameterKeyUseAllowedDiskTopology)
				continue
			}

			paramUseAllowedDiskTopology, err := convert.ConvertStringToBool(v)
			if err != nil {
				klog.Warningf("failed to convert %s parameter with value %q to bool: %v", ParameterKeyUseAllowedDiskTopology, v, err)
				continue
			}

			p.UseAllowedDiskTopology = paramUseAllowedDiskTopology
		case ParameterHDType, ParameterPDType:
			if pp.isDynamicVolumesDisabled() {
				return p, d, fmt.Errorf("parameters contains invalid option %q, must enable dynamic volumes", k)
			}
			if strings.ToLower(parameters[ParameterKeyType]) != DynamicVolumeType {
				return p, d, fmt.Errorf("must specify %q parameter as %q when %q is specified", ParameterKeyType, DynamicVolumeType, k)
			}
		case ParameterDiskPreference:
			if pp.isDynamicVolumesDisabled() {
				return p, d, fmt.Errorf("parameters contains invalid option %q, must enable dynamic volumes", k)
			}
			if v != "" {
				preference := strings.ToLower(v)
				if preference != ParameterHDType && preference != ParameterPDType {
					return p, d, fmt.Errorf("must specify disk type preference as either %q or %q", ParameterHDType, ParameterPDType)
				}
			}
			if strings.ToLower(parameters[ParameterKeyType]) != DynamicVolumeType {
				return p, d, fmt.Errorf("must specify %q parameter as %q when using dynamic volume", ParameterKeyType, DynamicVolumeType)
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
			paramLabels, err := convert.ConvertLabelsStringToMap(v)
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
	paramResourceTags, err := convert.ConvertTagsStringToMap(tagsString)
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
			iops, err := convert.ConvertStringToInt64(value)
			if err != nil {
				return ModifyVolumeParameters{}, fmt.Errorf("parameters contain invalid iops parameter: %w", err)
			}
			modifyVolumeParams.IOPS = &iops
		case "throughput":
			throughput, err := convert.ConvertMiStringToInt64(value)
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

// convertStringToAvailabilityClass converts a string to an availability class string.
func convertStringToAvailabilityClass(str string) (string, error) {
	switch strings.ToLower(str) {
	case ParameterNoAvailabilityClass:
		return ParameterNoAvailabilityClass, nil
	case ParameterRegionalHardFailoverClass:
		return ParameterRegionalHardFailoverClass, nil
	}
	return "", fmt.Errorf("unexpected boolean string %s", str)
}

// StoragePoolZones returns the unique zones of the given storage pool resource names.
// Returns an error if multiple storage pools in 1 zone are found.
func StoragePoolZones(storagePools []StoragePool) ([]string, error) {
	zonesSet := sets.New[string]()
	var zones []string
	for _, sp := range storagePools {
		if zonesSet.Has(sp.Zone) {
			return nil, fmt.Errorf("found multiple storage pools in zone %s. Only one storage pool per zone is allowed", sp.Zone)
		}
		zonesSet.Insert(sp.Zone)
		zones = append(zones, sp.Zone)
	}
	return zones, nil
}

// StoragePoolInZone returns the storage pool in the given zone.
func StoragePoolInZone(storagePools []StoragePool, zone string) *StoragePool {
	for _, pool := range storagePools {
		if zone == pool.Zone {
			return &pool
		}
	}
	return nil
}
