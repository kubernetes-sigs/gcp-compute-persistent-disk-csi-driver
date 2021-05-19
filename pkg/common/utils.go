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
	"fmt"
	"regexp"
	"strings"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"k8s.io/apimachinery/pkg/util/sets"
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

	// Node ID Expected Format
	// "projects/{projectName}/zones/{zoneName}/disks/{diskName}"
	nodeIDFmt           = "projects/%s/zones/%s/instances/%s"
	nodeIDZoneValue     = 3
	nodeIDNameValue     = 5
	nodeIDTotalElements = 6

	regionalDeviceNameSuffix = "_regional"
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

func VolumeIDToKey(id string) (*meta.Key, error) {
	splitId := strings.Split(id, "/")
	if len(splitId) != volIDTotalElements {
		return nil, fmt.Errorf("failed to get id components. Expected projects/{project}/zones/{zone}/disks/{name}. Got: %s", id)
	}
	if splitId[volIDToplogyKey] == "zones" {
		return meta.ZonalKey(splitId[volIDDiskNameValue], splitId[volIDToplogyValue]), nil
	} else if splitId[volIDToplogyKey] == "regions" {
		return meta.RegionalKey(splitId[volIDDiskNameValue], splitId[volIDToplogyValue]), nil
	} else {
		return nil, fmt.Errorf("could not get id components, expected either zones or regions, got: %v", splitId[volIDToplogyKey])
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

func SnapshotIDToProjectKey(id string) (string, string, error) {
	splitId := strings.Split(id, "/")
	if len(splitId) != snapshotTotalElements {
		return "", "", fmt.Errorf("failed to get id components. Expected projects/{project}/global/snapshot/{name}. Got: %s", id)
	}
	if splitId[snapshotTopologyKey] == "global" {
		return splitId[1], splitId[snapshotTotalElements-1], nil
	} else {
		return "", "", fmt.Errorf("could not get id components, expected global, got: %v", splitId[snapshotTopologyKey])
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
