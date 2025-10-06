/*
Copyright 2018 The Kubernetes Authors.

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
	"regexp"
	"strings"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
)

const (
	// Snapshot storage location format
	// Reference: https://cloud.google.com/storage/docs/locations
	// Example: us
	multiRegionalLocationFmt = "^[a-z]+$"
	// Example: us-east1
	regionalLocationFmt = "^[a-z]+-[a-z]+[0-9]{1,2}$"
)

var (
	multiRegionalPattern = regexp.MustCompile(multiRegionalLocationFmt)
	regionalPattern      = regexp.MustCompile(regionalLocationFmt)

	validDataCacheMode     = []string{constants.DataCacheModeWriteBack, constants.DataCacheModeWriteThrough}
	storagePoolFieldsRegex = regexp.MustCompile(`^projects/([^/]+)/zones/([^/]+)/storagePools/([^/]+)$`)
)

// ProcessStorageLocations trims and normalizes storage location to lower letters.
func ProcessStorageLocations(storageLocations string) ([]string, error) {
	normalizedLoc := strings.ToLower(strings.TrimSpace(storageLocations))
	if !multiRegionalPattern.MatchString(normalizedLoc) && !regionalPattern.MatchString(normalizedLoc) {
		return []string{}, fmt.Errorf("invalid location for snapshot: %q", storageLocations)
	}
	return []string{normalizedLoc}, nil
}

// ValidateSnapshotType validates the type
func ValidateSnapshotType(snapshotType string) error {
	switch snapshotType {
	case DiskSnapshotType, DiskImageType:
		return nil
	default:
		return fmt.Errorf("invalid snapshot type %s", snapshotType)
	}
}

// ParseStoragePools returns an error if none of the given storagePools
// (delimited by a comma) are in the format
// projects/project/zones/zone/storagePools/storagePool.
func ParseStoragePools(storagePools string) ([]StoragePool, error) {
	spSlice := strings.Split(storagePools, ",")
	parsedStoragePools := []StoragePool{}
	for _, sp := range spSlice {
		project, location, spName, err := fieldsFromStoragePoolResourceName(sp)
		if err != nil {
			return nil, err
		}
		spObj := StoragePool{Project: project, Zone: location, Name: spName, ResourceName: sp}
		parsedStoragePools = append(parsedStoragePools, spObj)

	}
	return parsedStoragePools, nil
}

// fieldsFromResourceName returns the project, zone, and Storage Pool name from the given
// Storage Pool resource name. The resource name must be in the format
// projects/project/zones/zone/storagePools/storagePool.
// All other formats are invalid, and an error will be returned.
func fieldsFromStoragePoolResourceName(resourceName string) (project, location, spName string, err error) {
	fieldMatches := storagePoolFieldsRegex.FindStringSubmatch(resourceName)
	//  Field matches should have 4 strings: [resourceName, project, zone, storagePool]. The first
	// match is the entire string.
	if len(fieldMatches) != 4 {
		err := fmt.Errorf("invalid Storage Pool resource name. Got %s, expected projects/project/zones/zone/storagePools/storagePool", resourceName)
		return "", "", "", err
	}
	project = fieldMatches[1]
	location = fieldMatches[2]
	spName = fieldMatches[3]
	return
}

func ValidateDataCacheMode(s string) error {
	if StringInSlice(s, validDataCacheMode) {
		return nil
	}
	return fmt.Errorf("invalid data-cache-mode %s. Only \"writeback\" and \"writethrough\" is a valid input", s)
}

func ValidateNonNegativeInt(n int64) error {
	if n <= 0 {
		return fmt.Errorf("Input should be set to > 0, got %d", n)
	}
	return nil
}

func StringInSlice(s string, list []string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func isValidDiskEncryptionKmsKey(DiskEncryptionKmsKey string) bool {
	// Validate key against default kmskey pattern
	kmsKeyPattern := regexp.MustCompile("projects/[^/]+/locations/([^/]+)/keyRings/[^/]+/cryptoKeys/[^/]+")
	return kmsKeyPattern.MatchString(DiskEncryptionKmsKey)
}
