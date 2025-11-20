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
	computev1 "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
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
	regionalLocationFmt = "^[a-z]+-[a-z]+[0-9]{1,2}$"

	// Full or partial URL of the machine type resource, in the format:
	//   zones/zone/machineTypes/machine-type
	machineTypePattern = "zones/[^/]+/machineTypes/([^/]+)$"

	// Full or partial URL of the zone resource, in the format:
	//   projects/{project}/zones/{zone}
	zoneURIPattern = "projects/[^/]+/zones/([^/]+)$"
	alphanums      = "bcdfghjklmnpqrstvwxz2456789"

	HyperdiskBalancedIopsPerGB         = 500
	HyperdiskBalancedMinIops           = 3000
	HyperdiskExtremeIopsPerGB          = 2
	HyperdiskThroughputThroughputPerGB = 10
	BytesInGB                          = 1024
)

var (
	// Full or partial URL of the machine type resource, in the format:
	//   zones/zone/machineTypes/machine-type
	machineTypeRegex = regexp.MustCompile(machineTypePattern)

	zoneURIRegex = regexp.MustCompile(zoneURIPattern)

	// userErrorCodeMap tells how API error types are translated to error codes.
	userErrorCodeMap = map[int]codes.Code{
		http.StatusForbidden:          codes.PermissionDenied,
		http.StatusBadRequest:         codes.InvalidArgument,
		http.StatusTooManyRequests:    codes.ResourceExhausted,
		http.StatusNotFound:           codes.NotFound,
		http.StatusConflict:           codes.FailedPrecondition,
		http.StatusPreconditionFailed: codes.FailedPrecondition,
	}

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
		return fmt.Sprintf(volIDZonalFmt, constants.UnspecifiedValue, constants.UnspecifiedValue, diskName)
	}
	return fmt.Sprintf(volIDRegionalFmt, constants.UnspecifiedValue, constants.UnspecifiedValue, diskName)
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
	klog.Errorf("%v: %s %v", code, msg, err.Error())
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

func ParseZoneFromURI(zoneURI string) (string, error) {
	zoneMatch := zoneURIRegex.FindStringSubmatch(zoneURI)
	if zoneMatch == nil {
		return "", fmt.Errorf("failed to parse zone URI. Expected projects/{project}/zones/{zone}. Got: %s", zoneURI)
	}
	return zoneMatch[1], nil
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
	splitId[volIDToplogyValue] = constants.MultiZoneValue
	return strings.Join(splitId, "/"), nil
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

// GetHyperdiskAttachLimit returns the hyperdisk attach limit based on machine type prefix and vCPUs
func GetHyperdiskAttachLimit(machineTypePrefix string, vCPUs int64) int64 {
	var limitMap []constants.MachineHyperdiskLimit

	switch machineTypePrefix {
	case "c4":
		limitMap = constants.C4MachineHyperdiskAttachLimitMap
	case "c4d":
		limitMap = constants.C4DMachineHyperdiskAttachLimitMap
	case "n4":
		limitMap = constants.N4MachineHyperdiskAttachLimitMap
	case "c4a":
		limitMap = constants.C4AMachineHyperdiskAttachLimitMap
	case "a4x":
		limitMap = constants.A4XMachineHyperdiskAttachLimitMap
	default:
		// Fallback to the most conservative Gen4 map for unknown types
		return MapNumber(vCPUs, constants.C4DMachineHyperdiskAttachLimitMap)
	}

	return MapNumber(vCPUs, limitMap)
}

// mapNumber maps the vCPUs to the appropriate hyperdisk limit
func MapNumber(vCPUs int64, limitMap []constants.MachineHyperdiskLimit) int64 {
	for _, limit := range limitMap {
		if vCPUs <= limit.Max {
			return limit.Value
		}
	}
	// Return the last value if vCPUs exceeds all max values
	if len(limitMap) > 0 {
		return limitMap[len(limitMap)-1].Value
	}
	return 15
}

// HasDiskTypeLabelKeyPrefix checks if the label key starts with the DiskTypeKeyPrefix.
func HasDiskTypeLabelKeyPrefix(labelKey string) bool {
	return strings.HasPrefix(labelKey, constants.DiskTypeKeyPrefix)
}

func DiskTypeLabelKey(diskType string) string {
	return fmt.Sprintf("%s/%s", constants.DiskTypeKeyPrefix, diskType)
}

// IsUpdateIopsThroughputValuesAllowed checks if a disk type is hyperdisk,
// which implies that IOPS and throughput values can be updated.
func IsUpdateIopsThroughputValuesAllowed(disk *computev1.Disk) bool {
	// Sample formats:
	// https://www.googleapis.com/compute/v1/{gce.projectID}/zones/{disk.Zone}/diskTypes/{disk.Type}"
	// https://www.googleapis.com/compute/v1/{gce.projectID}/regions/{disk.Region}/diskTypes/{disk.Type}"
	return strings.Contains(disk.Type, "hyperdisk")
}

// GetMinIopsThroughput calculates and returns the minimum required IOPS and throughput
// based on the existing disk configuration and the requested new GiB.
// The `needed` return value indicates whether either IOPS or throughput need to be updated.
// https://cloud.google.com/compute/docs/disks/hyperdisks#limits-disk
func GetMinIopsThroughput(disk *computev1.Disk, requestGb int64) (needed bool, minIops int64, minThroughput int64) {
	switch {
	case strings.Contains(disk.Type, "hyperdisk-balanced"):
		// This includes types "hyperdisk-balanced" and "hyperdisk-balanced-high-availability"
		return minIopsForBalanced(disk, requestGb)
	case strings.Contains(disk.Type, "hyperdisk-extreme"):
		return minIopsForExtreme(disk, requestGb)
	case strings.Contains(disk.Type, "hyperdisk-ml"):
		return minThroughputForML(disk, requestGb)
	case strings.Contains(disk.Type, "hyperdisk-throughput"):
		return minThroughputForThroughput(disk, requestGb)
	default:
		return false, 0, 0
	}
}

// minIopsForBalanced calculates and returns the minimum required IOPS and throughput
// for hyperdisk-balanced and hyperdisk-balanced-high-availability disks
func minIopsForBalanced(disk *computev1.Disk, requestGb int64) (needed bool, minIops int64, minThroughput int64) {
	minRequiredIops := requestGb * HyperdiskBalancedIopsPerGB
	if minRequiredIops > HyperdiskBalancedMinIops {
		minRequiredIops = HyperdiskBalancedMinIops
	}
	if disk.ProvisionedIops < minRequiredIops {
		return true, minRequiredIops, 0
	}
	return false, 0, 0
}

// minIopsForExtreme calculates and returns the minimum required IOPS and throughput
// for hyperdisk-extreme disks
func minIopsForExtreme(disk *computev1.Disk, requestGb int64) (needed bool, minIops int64, minThroughput int64) {
	minRequiredIops := requestGb * HyperdiskExtremeIopsPerGB
	if disk.ProvisionedIops < minRequiredIops {
		return true, minRequiredIops, 0
	}
	return false, 0, 0
}

// minThroughputForML calculates and returns the minimum required IOPS and throughput
// for hyperdisk-ml disks
func minThroughputForML(disk *computev1.Disk, requestGb int64) (needed bool, minIops int64, minThroughput int64) {
	minRequiredThroughput := int64(float64(requestGb) * 0.12)
	if disk.ProvisionedThroughput < minRequiredThroughput {
		return true, 0, minRequiredThroughput
	}
	return false, 0, 0
}

// minThroughputForThroughput calculates and returns the minimum required IOPS and throughput
// for hyperdisk-throughput disks
func minThroughputForThroughput(disk *computev1.Disk, requestGb int64) (needed bool, minIops int64, minThroughput int64) {
	minRequiredThroughput := requestGb * HyperdiskThroughputThroughputPerGB / BytesInGB
	if disk.ProvisionedThroughput < minRequiredThroughput {
		return true, 0, minRequiredThroughput
	}
	return false, 0, 0
}
