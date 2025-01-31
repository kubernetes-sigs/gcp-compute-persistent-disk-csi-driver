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

const (
	// Keys for Topology. This key will be shared amongst drivers from GCP
	TopologyKeyZone = "topology.gke.io/zone"

	// VolumeAttributes for Partition
	VolumeAttributePartition = "partition"

	UnspecifiedValue = "UNSPECIFIED"

	// Keyword indicating a 'multi-zone' volumeHandle. Replaces "zones" in the volumeHandle:
	// eg: projects/{project}/zones/multi-zone/disks/{name} vs.
	// projects/{project}/zones/{zone}/disks/{name}
	MultiZoneValue = "multi-zone"

	// Label that is set on a disk when it is used by a 'multi-zone' VolumeHandle
	MultiZoneLabel = "goog-gke-multi-zone"

	// Data cache mode
	DataCacheModeWriteBack    = "writeback"
	DataCacheModeWriteThrough = "writethrough"

	ContextDataCacheSize = "data-cache-size"
	ContextDataCacheMode = "data-cache-mode"
	// If disk is created newly then the content source would be nil or empty
	ContextDiskSource = "disk-content-source"

	// Keys in the publish context
	ContextLocalSsdCacheSize = "local-ssd-cache-size"
	// Node name for E2E tests
	TestNode = "test-node-csi-e2e"
)
