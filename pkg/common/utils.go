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
	"hash/fnv"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"github.com/googleapis/gax-go/v2/apierror"
	"golang.org/x/time/rate"
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

	// Full or partial URL of the zone resource, in the format:
	//   projects/{project}/zones/{zone}
	zoneURIPattern = "projects/[^/]+/zones/([^/]+)$"
	alphanums      = "bcdfghjklmnpqrstvwxz2456789"
)

var (
	multiRegionalPattern = regexp.MustCompile(multiRegionalLocationFmt)
	regionalPattern      = regexp.MustCompile(regionalLocationFmt)

	// Full or partial URL of the machine type resource, in the format:
	//   zones/zone/machineTypes/machine-type
	machineTypeRegex = regexp.MustCompile(machineTypePattern)

	storagePoolFieldsRegex = regexp.MustCompile(`^projects/([^/]+)/zones/([^/]+)/storagePools/([^/]+)$`)

	zoneURIRegex = regexp.MustCompile(zoneURIPattern)

	// userErrorCodeMap tells how API error types are translated to error codes.
	userErrorCodeMap = map[int]codes.Code{
		http.StatusForbidden:       codes.PermissionDenied,
		http.StatusBadRequest:      codes.InvalidArgument,
		http.StatusTooManyRequests: codes.ResourceExhausted,
		http.StatusNotFound:        codes.NotFound,
		http.StatusConflict:        codes.FailedPrecondition,
	}

	validDataCacheMode = []string{DataCacheModeWriteBack, DataCacheModeWriteThrough}

	// Regular expressions for validating parent_id, key and value of a resource tag.
	regexParent = regexp.MustCompile(`(^[1-9][0-9]{0,31}$)|(^[a-z][a-z0-9-]{4,28}[a-z0-9]$)`)
	regexKey    = regexp.MustCompile(`^[a-zA-Z0-9]([0-9A-Za-z_.-]{0,61}[a-zA-Z0-9])?$`)
	regexValue  = regexp.MustCompile(`^[a-zA-Z0-9]([0-9A-Za-z_.@%=+:,*#&()\[\]{}\-\s]{0,61}[a-zA-Z0-9])?$`)

	csiRetryableErrorCodes = []codes.Code{codes.Canceled, codes.DeadlineExceeded, codes.Unavailable, codes.Aborted, codes.ResourceExhausted}
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
	const tpcPrefix = "u"
	regions := sets.String{}
	if len(zones) < 1 {
		return "", fmt.Errorf("no zones specified")
	}
	for _, zone := range zones {
		// Zone expected format {locale}-{region}-{zone}
		// TPC zone expected format is u-{}-{}-{}, e.g. u-europe-central2-a. See go/tpc-region-naming.
		splitZone := strings.Split(zone, "-")
		if len(splitZone) == 3 {
			regions.Insert(strings.Join(splitZone[0:2], "-"))
		} else if len(splitZone) == 4 && splitZone[0] == tpcPrefix {
			regions.Insert(strings.Join(splitZone[0:3], "-"))
		} else {
			return "", fmt.Errorf("zone in unexpected format, expected: {locale}-{region}-{zone} or u-{locale}-{region}-{zone}, got: %v", zone)
		}
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

// ConvertTagsStringToMap converts the tags from string to Tag slice
// example: "parent_id1/tag_key1/tag_value1,parent_id2/tag_key2/tag_value2" gets
// converted into {"parent_id1/tag_key1":"tag_value1", "parent_id2/tag_key2":"tag_value2"}
// See https://cloud.google.com/resource-manager/docs/tags/tags-overview,
// https://cloud.google.com/resource-manager/docs/tags/tags-creating-and-managing for details
func ConvertTagsStringToMap(tags string) (map[string]string, error) {
	const tagsDelimiter = ","
	const tagsParentIDKeyValueDelimiter = "/"

	tagsMap := make(map[string]string)
	if tags == "" {
		return nil, nil
	}

	checkTagParentIDFn := func(tag, parentID string) error {
		if !regexParent.MatchString(parentID) {
			return fmt.Errorf("tag parent_id %q for tag %q is invalid. parent_id can have a maximum of 32 characters and cannot be empty. parent_id can be either OrganizationID or ProjectID. OrganizationID must consist of decimal numbers, and cannot have leading zeroes and ProjectID must be 6 to 30 characters in length, can only contain lowercase letters, numbers, and hyphens, and must start with a letter, and cannot end with a hyphen", parentID, tag)
		}
		return nil
	}

	checkTagKeyFn := func(tag, key string) error {
		if !regexKey.MatchString(key) {
			return fmt.Errorf("tag key %q for tag %q is invalid. Tag key can have a maximum of 63 characters and cannot be empty. Tag key must begin and end with an alphanumeric character, and must contain only uppercase, lowercase alphanumeric characters, and the following special characters `._-`", key, tag)
		}
		return nil
	}

	checkTagValueFn := func(tag, value string) error {
		if !regexValue.MatchString(value) {
			return fmt.Errorf("tag value %q for tag %q is invalid. Tag value can have a maximum of 63 characters and cannot be empty. Tag value must begin and end with an alphanumeric character, and must contain only uppercase, lowercase alphanumeric characters, and the following special characters `_-.@%%=+:,*#&(){}[]` and spaces", value, tag)
		}

		return nil
	}

	checkTagParentIDKey := sets.String{}
	parentIDkeyValueStrings := strings.Split(tags, tagsDelimiter)
	for _, parentIDkeyValueString := range parentIDkeyValueStrings {
		parentIDKeyValue := strings.Split(parentIDkeyValueString, tagsParentIDKeyValueDelimiter)

		if len(parentIDKeyValue) != 3 {
			return nil, fmt.Errorf("tag %q is invalid, correct format: 'parent_id1/key1/value1,parent_id2/key2/value2'", parentIDkeyValueString)
		}

		parentID := strings.TrimSpace(parentIDKeyValue[0])
		if err := checkTagParentIDFn(parentIDkeyValueString, parentID); err != nil {
			return nil, err
		}

		key := strings.TrimSpace(parentIDKeyValue[1])
		if err := checkTagKeyFn(parentIDkeyValueString, key); err != nil {
			return nil, err
		}

		value := strings.TrimSpace(parentIDKeyValue[2])
		if err := checkTagValueFn(parentIDkeyValueString, value); err != nil {
			return nil, err
		}

		parentIDKeyStr := fmt.Sprintf("%s/%s", parentID, key)
		if checkTagParentIDKey.Has(parentIDKeyStr) {
			return nil, fmt.Errorf("tag parent_id & key combination %q exists more than once", parentIDKeyStr)
		}
		checkTagParentIDKey.Insert(parentIDKeyStr)

		tagsMap[parentIDKeyStr] = value
	}

	// The maximum number of tags allowed per resource is 50. For more details check the following:
	// https://cloud.google.com/resource-manager/docs/tags/tags-creating-and-managing#attaching
	// https://cloud.google.com/resource-manager/docs/limits#tag-limits
	const maxNumberOfTags = 50
	if len(tagsMap) > maxNumberOfTags {
		return nil, fmt.Errorf("more than %d tags is not allowed, given: %d", maxNumberOfTags, len(tagsMap))
	}

	return tagsMap, nil
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

// ConvertGiStringToInt64 converts a GiB string to int64
func ConvertGiStringToInt64(str string) (int64, error) {
	quantity, err := resource.ParseQuantity(str)
	if err != nil {
		return -1, err
	}
	return volumehelpers.RoundUpToGiB(quantity)
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
	if code, err := isConnectionResetError(sourceError); err == nil {
		return code
	}
	if code, err := isGoogleAPIError(sourceError); err == nil {
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

// isConnectionResetError returns the grpc error code Unavailable if the
// passed in error contains the "connection reset by peer" string.
func isConnectionResetError(err error) (codes.Code, error) {
	if err == nil {
		return codes.Unknown, fmt.Errorf("null error")
	}

	errStr := err.Error()
	if strings.Contains(errStr, "connection reset by peer") {
		return codes.Unavailable, nil
	}
	return codes.Unknown, fmt.Errorf("Not a connection reset error: %w", err)
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

// existingErrorCode returns the existing gRPC Status error code for the given error, if one exists,
// or an error if one doesn't exist. Since github.com/googleapis/gax-go/v2/apierror now wraps googleapi
// errors (returned from GCE API calls), and sets their status error code to Unknown, we now have to
// make sure we only return existing error codes from errors that are either TemporaryErrors, or errors
// that do not wrap googleAPI errors. Otherwise, we will return Unknown for all GCE API calls that
// return googleapi errors.
func existingErrorCode(err error) (codes.Code, error) {
	if err == nil {
		return codes.Unknown, fmt.Errorf("null error")
	}
	var tmpError *TemporaryError
	// This explicitly checks our error is a temporary error before extracting its
	// status, as there can be other errors that can qualify as statusable
	// while not necessarily being temporary.
	if errors.As(err, &tmpError) {
		if status, ok := status.FromError(err); ok {
			return status.Code(), nil
		}
	}
	// We want to make sure we catch other error types that are statusable.
	// (eg. grpc-go/internal/status/status.go Error struct that wraps a status)
	var googleErr *googleapi.Error
	if !errors.As(err, &googleErr) {
		if status, ok := status.FromError(err); ok {
			return status.Code(), nil
		}
	}

	return codes.Unknown, fmt.Errorf("no existing error code for %w", err)
}

// isGoogleAPIError returns the gRPC status code for the given googleapi error by mapping
// the googleapi error's HTTP code to the corresponding gRPC error code. If the error is
// wrapped in an APIError (github.com/googleapis/gax-go/v2/apierror), it maps the wrapped
// googleAPI error's HTTP code to the corresponding gRPC error code. Returns an error if
// the given error is not a googleapi error.
func isGoogleAPIError(err error) (codes.Code, error) {
	var googleErr *googleapi.Error
	if !errors.As(err, &googleErr) {
		return codes.Unknown, fmt.Errorf("error %w is not a googleapi.Error", err)
	}
	var sourceCode int
	var apiErr *apierror.APIError
	if errors.As(err, &apiErr) {
		// When googleapi.Err is used as a wrapper, we return the error code of the wrapped contents.
		sourceCode = apiErr.HTTPCode()
	} else {
		// Rely on error code in googleapi.Err when it is our primary error.
		sourceCode = googleErr.Code
	}
	// Map API error code to user error code.
	if code, ok := userErrorCodeMap[sourceCode]; ok {
		return code, nil
	}

	return codes.Unknown, fmt.Errorf("googleapi.Error %w does not map to any known errors", err)
}

func loggedErrorForCode(msg string, code codes.Code, err error) error {
	klog.Errorf(msg+"%v", err.Error())
	return status.Errorf(code, msg+"%v", err.Error())
}

func LoggedError(msg string, err error) error {
	return loggedErrorForCode(msg, CodeForError(err), err)
}

// NewCombinedError tries to return an appropriate wrapped error that captures
// useful information as an error code
// If there are multiple errors, it extracts the first "retryable" error
// as interpreted by the CSI sidecar.
func NewCombinedError(msg string, errs []error) error {
	// If there is only one error, return it as the single error code
	if len(errs) == 1 {
		LoggedError(msg, errs[0])
	}

	for _, err := range errs {
		code := CodeForError(err)
		if slices.Contains(csiRetryableErrorCodes, code) {
			// Return this as a TemporaryError to lock-in the retryable code
			// This will invoke the "existing" error code check in CodeForError
			return NewTemporaryError(code, fmt.Errorf("%s: %w", msg, err))
		}
	}

	// None of these error codes were retryable. Just return a combined error
	// The first matching error (based on our CodeForError) logic will be returned.
	return LoggedError(msg, errors.Join(errs...))
}

func isValidDiskEncryptionKmsKey(DiskEncryptionKmsKey string) bool {
	// Validate key against default kmskey pattern
	kmsKeyPattern := regexp.MustCompile("projects/[^/]+/locations/([^/]+)/keyRings/[^/]+/cryptoKeys/[^/]+")
	return kmsKeyPattern.MatchString(DiskEncryptionKmsKey)
}

func ParseZoneFromURI(zoneURI string) (string, error) {
	zoneMatch := zoneURIRegex.FindStringSubmatch(zoneURI)
	if zoneMatch == nil {
		return "", fmt.Errorf("failed to parse zone URI. Expected projects/{project}/zones/{zone}. Got: %s", zoneURI)
	}
	return zoneMatch[1], nil
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

func VolumeIdAsMultiZone(volumeId string) (string, error) {
	splitId := strings.Split(volumeId, "/")
	if len(splitId) != volIDTotalElements {
		return "", fmt.Errorf("failed to get id components. Expected projects/{project}/zones/{zone}/disks/{name}. Got: %s", volumeId)
	}
	if splitId[volIDToplogyKey] != "zones" {
		return "", fmt.Errorf("expected id to be zonal. Got: %s", volumeId)
	}
	splitId[volIDToplogyValue] = MultiZoneValue
	return strings.Join(splitId, "/"), nil
}

func StringInSlice(s string, list []string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
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

// NewLimiter returns a token bucket based request rate limiter after initializing
// the passed values for limit, burst (or token bucket) size. If opted for emptyBucket
// all initial tokens are reserved for the first burst.
func NewLimiter(limit, burst int, emptyBucket bool) *rate.Limiter {
	limiter := rate.NewLimiter(rate.Every(time.Second/time.Duration(limit)), burst)

	if emptyBucket {
		limiter.AllowN(time.Now(), burst)
	}

	return limiter
}

func IsHyperdisk(diskType string) bool {
	return strings.HasPrefix(diskType, "hyperdisk-")
}

// shortString is inspired by k8s.io/apimachinery/pkg/util/rand.SafeEncodeString, but takes data from a hash.
func ShortString(s string) string {
	hasher := fnv.New128a()
	hasher.Write([]byte(s))
	sum := hasher.Sum([]byte{})
	const sz = 8
	short := make([]byte, sz)
	for i := 0; i < sz; i++ {
		short[i] = alphanums[int(sum[i])%len(alphanums)]
	}
	return string(short)
}

// MapNumber is a function to map input cpu number to the Hyperdisk attach limit
func MapNumber(num int64) int64 {
	for _, r := range Gen4MachineHyperdiskAttachLimitMap {
		if num <= r.max {
			return r.value
		}
	}
	return 0
}

func IsGKETopologyLabel(key string) bool {
	return strings.HasPrefix(key, TopologyKeyPrefix)
}

func TopologyLabelKey(diskType string) string {
	return fmt.Sprintf("%s/%s", TopologyKeyPrefix, diskType)
}
