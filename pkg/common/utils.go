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
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/cloudprovider/providers/gce/cloud/meta"
)

const (
	// Volume ID Expected Format
	// "projects/{projectName}/zones/{zoneName}/disks/{diskName}"
	// "projects/{projectName}/regions/{regionName}/disks/{diskName}"
	// OR
	// "projects/{projectName}/zones/{zoneName}/disks/{diskName}/partitions/{partition}"
	// "projects/{projectName}/regions/{regionName}/disks/{diskName}/partitions/{partition}"

	volIDToplogyKey                 = 2
	volIDToplogyValue               = 3
	volIDDiskNameValue              = 5
	volIDPartitionKey               = 6
	volIDPartitionValue             = 7
	volIDTotalElements              = 6
	volIDTotalElementsWithPartition = 8

	// Node ID Expected Format
	// "{zoneName}/{instanceName}"
	nodeIDZoneValue     = 0
	nodeIDNameValue     = 1
	nodeIDTotalElements = 2
)

func BytesToGb(bytes int64) int64 {
	// TODO: Throw an error when div to 0
	return bytes / (1024 * 1024 * 1024)
}

func GbToBytes(Gb int64) int64 {
	// TODO: Check for overflow
	return Gb * 1024 * 1024 * 1024
}

func VolumeIDToKey(id string) (*meta.Key, string, error) {
	splitID := strings.Split(id, "/")
	partition := ""
	if !(len(splitID) == volIDTotalElements || len(splitID) == volIDTotalElementsWithPartition) {
		return nil, partition, fmt.Errorf("failed to get id components. Expected projects/{project}/zones/{zone}/disks/{name}(/partition/{partition}). Got: %s", id)
	}

	if len(splitID) == volIDTotalElementsWithPartition {
		if splitID[volIDPartitionKey] == "partitions" {
			partition = splitID[volIDPartitionValue]
		} else {
			return nil, partition, fmt.Errorf("could not get id components, expected partitions, got: %v", splitID[volIDPartitionKey])
		}
	}

	if splitID[volIDToplogyKey] == "zones" {
		return meta.ZonalKey(splitID[volIDDiskNameValue], splitID[volIDToplogyValue]), partition, nil
	} else if splitID[volIDToplogyKey] == "regions" {
		return meta.RegionalKey(splitID[volIDDiskNameValue], splitID[volIDToplogyValue]), partition, nil
	} else {
		return nil, partition, fmt.Errorf("could not get id components, expected either zones or regions, got: %v", splitID[volIDToplogyKey])
	}

}

func NodeIDToZoneAndName(id string) (string, string, error) {
	splitID := strings.Split(id, "/")
	if len(splitID) != nodeIDTotalElements {
		return "", "", fmt.Errorf("failed to get id components. expected {zone}/{name}. Got: %s", id)
	}
	return splitID[nodeIDZoneValue], splitID[nodeIDNameValue], nil
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
