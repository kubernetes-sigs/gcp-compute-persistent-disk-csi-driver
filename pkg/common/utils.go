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

package common

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	volumehelpers "k8s.io/cloud-provider/volume/helpers"
	"k8s.io/klog/v2"
)

const (
	// Volume ID Expected Format
	// "projects/{projectName}/zones/{zoneName}/disks/{diskName}"
	volIDZonalFmt = "projects/%s/zones/%s/disks/%s"
	// "projects/{projectName}/regions/{regionName}/disks/{diskName}"
	volIDRegionalFmt   = "projects/%s/regions/%s/disks/%s"
	volIDToplogyKey    = 2
	volIDToplogyValue  = 3
	volIDDiskNameValue = 5
	volIDTotalElements = 6

	// Snapshot ID
	snapshotTotalElements = 5
	snapshotTopologyKey   = 2
	snapshotProjectKey    = 1

	// Node ID Expected Format
	// "projects/{projectName}/zones/{zoneName}/disks/{diskName}"
	nodeIDFmt           = "projects/%s/zones/%s/instances/%s"
	nodeIDProjectValue  = 1
	nodeIDZoneValue     = 3
	nodeIDNameValue     = 5
	nodeIDTotalElements = 6

	regionalDeviceNameSuffix = "_regional"

	// Snapshot storage location format
	// Reference: https://cloud.google.com/storage/docs/locations
	// Example: us
	multiRegionalLocationFmt = "^[a-z]+$"
	// Example: us-east1
	regionalLocationFmt = "^[a-z]+-[a-z]+[0-9]$"

	// Full or partial URL of the machine type resource, in the format:
	//   zones/zone/machineTypes/machine-type
	machineTypePattern = "zones/[^/]+/machineTypes/([^/]+)$"
)

var (
	multiRegionalPattern = regexp.MustCompile(multiRegionalLocationFmt)
	regionalPattern      = regexp.MustCompile(regionalLocationFmt)

	// Full or partial URL of the machine type resource, in the format:
	//   zones/zone/machineTypes/machine-type
	machineTypeRegex = regexp.MustCompile(machineTypePattern)

	storagePoolFieldsRegex = regexp.MustCompile(`^projects/([^/]+)/zones/([^/]+)/storagePools/([^/]+)$`)

	// userErrorCodeMap tells how API error types are translated to error codes.
	userErrorCodeMap = map[int]codes.Code{
		http.StatusForbidden:       codes.PermissionDenied,
		http.StatusBadRequest:      codes.InvalidArgument,
		http.StatusTooManyRequests: codes.ResourceExhausted,
		http.StatusNotFound:        codes.NotFound,
	}
)

func BytesToGbRoundDown(bytes int64) int64 {
	// TODO: Throw an error when div to 0
	return bytes / (1024 * 1024 * 1024)
}

func BytesToGbRoundUp(bytes int64) int64 {
	re := bytes / (1024 * 1024 * 1024)
	if (bytes % (1024 * 1024 * 1024)) != 0 {
		re++
	}
	return re
}

func GbToBytes(Gb int64) int64 {
	// TODO: Check for overflow
	return Gb * 1024 * 1024 * 1024
}

func VolumeIDToKey(id string) (string, *meta.Key, error) {
	splitId := strings.Split(id, "/")
	if len(splitId) != volIDTotalElements {
		return "", nil, fmt.Errorf("failed to get id components. Expected projects/{project}/zones/{zone}/disks/{name}. Got: %s", id)
	}
	if splitId[volIDToplogyKey] == "zones" {
		return splitId[nodeIDProjectValue], meta.ZonalKey(splitId[volIDDiskNameValue], splitId[volIDToplogyValue]), nil
	} else if splitId[volIDToplogyKey] == "regions" {
		return splitId[nodeIDProjectValue], meta.RegionalKey(splitId[volIDDiskNameValue], splitId[volIDToplogyValue]), nil
	} else {
		return "", nil, fmt.Errorf("could not get id components, expected either zones or regions, got: %v", splitId[volIDToplogyKey])
	}
}

func KeyToVolumeID(volKey *meta.Key, project string) (string, error) {
	switch volKey.Type() {
	case meta.Zonal:
		return fmt.Sprintf(volIDZonalFmt, project, volKey.Zone, volKey.Name), nil
	case meta.Regional:
		return fmt.Sprintf(volIDRegionalFmt, project, volKey.Region, volKey.Name), nil
	default:
		return "", fmt.Errorf("volume key %v neither zonal nor regional", volKey.String())
	}
}

func GenerateUnderspecifiedVolumeID(diskName string, isZonal bool) string {
	if isZonal {
		return fmt.Sprintf(volIDZonalFmt, UnspecifiedValue, UnspecifiedValue, diskName)
	}
	return fmt.Sprintf(volIDRegionalFmt, UnspecifiedValue, UnspecifiedValue, diskName)
}

func SnapshotIDToProjectKey(id string) (string, string, string, error) {
	splitId := strings.Split(id, "/")
	if len(splitId) != snapshotTotalElements {
		return "", "", "", fmt.Errorf("failed to get id components. Expected projects/{project}/global/{snapshots|images}/{name}. Got: %s", id)
	}
	if splitId[snapshotTopologyKey] == "global" {
		return splitId[snapshotProjectKey], splitId[snapshotTotalElements-2], splitId[snapshotTotalElements-1], nil
	} else {
		return "", "", "", fmt.Errorf("could not get id components, expected global, got: %v", splitId[snapshotTopologyKey])
	}
}

func NodeIDToZoneAndName(id string) (string, string, error) {
	splitId := strings.Split(id, "/")
	if len(splitId) != nodeIDTotalElements {
		return "", "", fmt.Errorf("failed to get id components. expected projects/{project}/zones/{zone}/instances/{name}. Got: %s", id)
	}
	return splitId[nodeIDZoneValue], splitId[nodeIDNameValue], nil
}

func GetRegionFromZones(zones []string) (string, error) {
	regions := sets.String{}
	if len(zones) < 1 {
		return "", fmt.Errorf("no zones specified")
	}
	for _, zone := range zones {
		// Zone expected format {locale}-{region}-{zone}
		splitZone := strings.Split(zone, "-")
		if len(splitZone) != 3 {
			return "", fmt.Errorf("zone in unexpected format, expected: {locale}-{region}-{zone}, got: %v", zone)
		}
		regions.Insert(strings.Join(splitZone[0:2], "-"))
	}
	if regions.Len() != 1 {
		return "", fmt.Errorf("multiple or no regions gotten from zones, got: %v", regions)
	}
	return regions.UnsortedList()[0], nil
}

func GetDeviceName(volKey *meta.Key) (string, error) {
	switch volKey.Type() {
	case meta.Zonal:
		return volKey.Name, nil
	case meta.Regional:
		return volKey.Name + regionalDeviceNameSuffix, nil
	default:
		return "", fmt.Errorf("volume key %v neither zonal nor regional", volKey.Name)
	}
}

func CreateNodeID(project, zone, name string) string {
	return fmt.Sprintf(nodeIDFmt, project, zone, name)
}

func CreateZonalVolumeID(project, zone, name string) string {
	return fmt.Sprintf(volIDZonalFmt, project, zone, name)
}

// ConvertLabelsStringToMap converts the labels from string to map
// example: "key1=value1,key2=value2" gets converted into {"key1": "value1", "key2": "value2"}
// See https://cloud.google.com/compute/docs/labeling-resources#label_format for details.
func ConvertLabelsStringToMap(labels string) (map[string]string, error) {
	const labelsDelimiter = ","
	const labelsKeyValueDelimiter = "="

	labelsMap := make(map[string]string)
	if labels == "" {
		return labelsMap, nil
	}

	regexKey, _ := regexp.Compile(`^\p{Ll}[\p{Ll}0-9_-]{0,62}$`)
	checkLabelKeyFn := func(key string) error {
		if !regexKey.MatchString(key) {
			return fmt.Errorf("label value %q is invalid (should start with lowercase letter / lowercase letter, digit, _ and - chars are allowed / 1-63 characters", key)
		}
		return nil
	}

	regexValue, _ := regexp.Compile(`^[\p{Ll}0-9_-]{0,63}$`)
	checkLabelValueFn := func(value string) error {
		if !regexValue.MatchString(value) {
			return fmt.Errorf("label value %q is invalid (lowercase letter, digit, _ and - chars are allowed / 0-63 characters", value)
		}

		return nil
	}

	keyValueStrings := strings.Split(labels, labelsDelimiter)
	for _, keyValue := range keyValueStrings {
		keyValue := strings.Split(keyValue, labelsKeyValueDelimiter)

		if len(keyValue) != 2 {
			return nil, fmt.Errorf("labels %q are invalid, correct format: 'key1=value1,key2=value2'", labels)
		}

		key := strings.TrimSpace(keyValue[0])
		if err := checkLabelKeyFn(key); err != nil {
			return nil, err
		}

		value := strings.TrimSpace(keyValue[1])
		if err := checkLabelValueFn(value); err != nil {
			return nil, err
		}

		labelsMap[key] = value
	}

	const maxNumberOfLabels = 64
	if len(labelsMap) > maxNumberOfLabels {
		return nil, fmt.Errorf("more than %d labels is not allowed, given: %d", maxNumberOfLabels, len(labelsMap))
	}

	return labelsMap, nil
}

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

// ConvertStringToInt64 converts a string to int64
func ConvertStringToInt64(str string) (int64, error) {
	quantity, err := resource.ParseQuantity(str)
	if err != nil {
		return -1, err
	}
	return volumehelpers.RoundUpToB(quantity)
}

// ConvertMiStringToInt64 converts a GiB string to int64
func ConvertMiStringToInt64(str string) (int64, error) {
	quantity, err := resource.ParseQuantity(str)
	if err != nil {
		return -1, err
	}
	return volumehelpers.RoundUpToMiB(quantity)
}

// ConvertStringToBool converts a string to a boolean.
func ConvertStringToBool(str string) (bool, error) {
	switch strings.ToLower(str) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("Unexpected boolean string %s", str)
}

// ConvertStringToAvailabilityClass converts a string to an availability class string.
func ConvertStringToAvailabilityClass(str string) (string, error) {
	switch strings.ToLower(str) {
	case ParameterNoAvailabilityClass:
		return ParameterNoAvailabilityClass, nil
	case ParameterRegionalHardFailoverClass:
		return ParameterRegionalHardFailoverClass, nil
	}
	return "", fmt.Errorf("Unexpected boolean string %s", str)
}

// ParseMachineType returns an extracted machineType from a URL, or empty if not found.
// machineTypeUrl: Full or partial URL of the machine type resource, in the format:
//
//	zones/zone/machineTypes/machine-type
func ParseMachineType(machineTypeUrl string) (string, error) {
	machineType := machineTypeRegex.FindStringSubmatch(machineTypeUrl)
	if machineType == nil {
		return "", fmt.Errorf("failed to parse machineTypeUrl. Expected suffix: zones/{zone}/machineTypes/{machine-type}. Got: %s", machineTypeUrl)
	}
	return machineType[1], nil
}

// CodeForError returns the grpc error code that maps to the http error code for the
// passed in user googleapi error or context error. Returns codes.Internal if the given
// error is not a googleapi error caused by the user. userErrorCodeMap is used for
// encoding most errors.
func CodeForError(sourceError error) codes.Code {
	if sourceError == nil {
		return codes.Internal
	}

	if code, err := isUserMultiAttachError(sourceError); err == nil {
		return code
	}
	if code, err := existingErrorCode(sourceError); err == nil {
		return code
	}
	if code, err := isContextError(sourceError); err == nil {
		return code
	}

	var apiErr *googleapi.Error
	if !errors.As(sourceError, &apiErr) {
		return codes.Internal
	}
	if code, ok := userErrorCodeMap[apiErr.Code]; ok {
		return code
	}

	return codes.Internal
}

// isContextError returns the grpc error code DeadlineExceeded if the passed in error
// contains the "context deadline exceeded" string and returns the grpc error code
// Canceled if the error contains the "context canceled" string. It returns and error if
// err isn't a context error.
func isContextError(err error) (codes.Code, error) {
	if err == nil {
		return codes.Unknown, fmt.Errorf("null error")
	}

	errStr := err.Error()
	if strings.Contains(errStr, context.DeadlineExceeded.Error()) {
		return codes.DeadlineExceeded, nil
	}
	if strings.Contains(errStr, context.Canceled.Error()) {
		return codes.Canceled, nil
	}
	return codes.Unknown, fmt.Errorf("Not a context error: %w", err)
}

// isUserMultiAttachError returns an InvalidArgument if the error is
// multi-attach detected from the API server. If we get this error from the API
// server, it means that the kubelet doesn't know about the multiattch so it is
// due to user configuration.
func isUserMultiAttachError(err error) (codes.Code, error) {
	if strings.Contains(err.Error(), "The disk resource") && strings.Contains(err.Error(), "is already being used") {
		return codes.InvalidArgument, nil
	}
	return codes.Unknown, fmt.Errorf("Not a user multiattach error: %w", err)
}

func existingErrorCode(err error) (codes.Code, error) {
	if err == nil {
		return codes.Unknown, fmt.Errorf("null error")
	}
	if status, ok := status.FromError(err); ok {
		return status.Code(), nil
	}
	return codes.Unknown, fmt.Errorf("no existing error code for %w", err)
}

func LoggedError(msg string, err error) error {
	klog.Errorf(msg+"%v", err.Error())
	return status.Errorf(CodeForError(err), msg+"%v", err.Error())
}

func isValidDiskEncryptionKmsKey(DiskEncryptionKmsKey string) bool {
	// Validate key against default kmskey pattern
	kmsKeyPattern := regexp.MustCompile("projects/[^/]+/locations/([^/]+)/keyRings/[^/]+/cryptoKeys/[^/]+")
	return kmsKeyPattern.MatchString(DiskEncryptionKmsKey)
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

// StoragePoolZones returns the unique zones of the given storage pool resource names.
// Returns an error if multiple storage pools in 1 zone are found.
func StoragePoolZones(storagePools []StoragePool) ([]string, error) {
	zonesSet := sets.String{}
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

func StoragePoolInZone(storagePools []StoragePool, zone string) *StoragePool {
	for _, pool := range storagePools {
		if zone == pool.Zone {
			return &pool
		}
	}
	return nil
}

func UnorderedSlicesEqual(slice1 []string, slice2 []string) bool {
	set1 := sets.NewString(slice1...)
	set2 := sets.NewString(slice2...)
	spZonesNotInReq := set1.Difference(set2)
	if spZonesNotInReq.Len() != 0 {
		return false
	}
	return true
}
