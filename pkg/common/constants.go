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

	// DiskTypeKeyPrefix is the prefix for the disk type label key used as part
	// of the Disk Topology feature.
	DiskTypeKeyPrefix = "disk-type.gke.io"

	// VolumeAttributes for Partition
	VolumeAttributePartition = "partition"

	UnspecifiedValue = "UNSPECIFIED"

	// Keyword indicating a 'multi-zone' volumeHandle. Replaces "zones" in the volumeHandle:
	// eg: projects/{project}/zones/multi-zone/disks/{name} vs.
	// projects/{project}/zones/{zone}/disks/{name}
	MultiZoneValue = "multi-zone"

	// Label that is set on a disk when it is used by a 'multi-zone' VolumeHandle
	MultiZoneLabel = "goog-gke-multi-zone"

	// GCE Access Modes that are valid for hyperdisks only.
	GCEReadOnlyManyAccessMode  = "READ_ONLY_MANY"
	GCEReadWriteManyAccessMode = "READ_WRITE_MANY"
	GCEReadWriteOnceAccessMode = "READ_WRITE_SINGLE"

	// Data cache mode
	DataCacheModeWriteBack    = "writeback"
	DataCacheModeWriteThrough = "writethrough"

	ContextDataCacheSize = "data-cache-size"
	ContextDataCacheMode = "data-cache-mode"

	// Keys in the publish context
	ContexLocalSsdCacheSize = "local-ssd-cache-size"
	// Node name for E2E tests
	TestNode = "test-node-csi-e2e"

	// Default LSSD count for datacache E2E tests
	LocalSSDCountForDataCache = 2

	// Node label for Data Cache (only applicable to GKE nodes)
	NodeLabelPrefix         = "cloud.google.com/%s"
	DataCacheLssdCountLabel = "gke-data-cache-disk"
	// Node label for attach limit override
	NodeRestrictionLabelPrefix = "node-restriction.kubernetes.io/%s"
	AttachLimitOverrideLabel   = "gke-volume-attach-limit-override"
)

// doc https://cloud.google.com/compute/docs/disks/hyperdisks#max-total-disks-per-vm
var Gen4MachineHyperdiskAttachLimitMap = []struct {
	max   int64
	value int64
}{
	{max: 8, value: 15},
	{max: 1024, value: 31},
}
