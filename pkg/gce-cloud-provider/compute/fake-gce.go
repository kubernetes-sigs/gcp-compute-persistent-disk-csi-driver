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

package gcecloudprovider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	csi "github.com/container-storage-interface/spec/lib/go/csi"
	computev1 "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
)

const (
	DiskSizeGb                = 10
	Timestamp                 = "2018-09-05T15:17:08.270-07:00"
	BasePath                  = "https://www.googleapis.com/compute/v1/projects/"
	snapshotURITemplateGlobal = "%s/global/snapshots/%s" //{gce.projectID}/global/snapshots/{snapshot.Name}"
)

type FakeCloudProvider struct {
	project string
	zone    string

	disks      map[string]*CloudDisk
	pageTokens map[string]sets.String
	instances  map[string]*computev1.Instance
	snapshots  map[string]*computev1.Snapshot

	// marker to set disk status during InsertDisk operation.
	mockDiskStatus string
}

var _ GCECompute = &FakeCloudProvider{}

func CreateFakeCloudProvider(project, zone string, cloudDisks []*CloudDisk) (*FakeCloudProvider, error) {
	fcp := &FakeCloudProvider{
		project:    project,
		zone:       zone,
		disks:      map[string]*CloudDisk{},
		instances:  map[string]*computev1.Instance{},
		snapshots:  map[string]*computev1.Snapshot{},
		pageTokens: map[string]sets.String{},
		// A newly created disk is marked READY by default.
		mockDiskStatus: "READY",
	}
	for _, d := range cloudDisks {
		fcp.disks[d.GetName()] = d
	}
	return fcp, nil
}

func (cloud *FakeCloudProvider) GetDefaultProject() string {
	return cloud.project
}

func (cloud *FakeCloudProvider) GetDefaultZone() string {
	return cloud.zone
}

func (cloud *FakeCloudProvider) RepairUnderspecifiedVolumeKey(ctx context.Context, volumeKey *meta.Key) (*meta.Key, error) {
	switch volumeKey.Type() {
	case meta.Zonal:
		if volumeKey.Zone != common.UnspecifiedValue {
			return volumeKey, nil
		}
		for name, d := range cloud.disks {
			if name == volumeKey.Name {
				volumeKey.Zone = d.GetZone()
				return volumeKey, nil
			}
		}
		return nil, notFoundError()
	case meta.Regional:
		if volumeKey.Region != common.UnspecifiedValue {
			return volumeKey, nil
		}
		r, err := common.GetRegionFromZones([]string{cloud.zone})
		if err != nil {
			return nil, fmt.Errorf("failed to get region from zones: %v", err)
		}
		volumeKey.Region = r
		return volumeKey, nil
	default:
		return nil, fmt.Errorf("Volume key %v not zonal nor regional", volumeKey.Name)
	}
}

func (cloud *FakeCloudProvider) ListZones(ctx context.Context, region string) ([]string, error) {
	return []string{cloud.zone, "country-region-fakesecondzone"}, nil
}

func (cloud *FakeCloudProvider) ListDisks(ctx context.Context, maxEntries int64, pageToken string) ([]*computev1.Disk, string, error) {
	// Ignore page tokens for now
	var seen sets.String
	var ok bool
	var count int64 = 0
	var newToken string
	d := []*computev1.Disk{}

	if pageToken != "" {
		seen, ok = cloud.pageTokens[pageToken]
		if !ok {
			return nil, "", invalidError()
		}
	} else {
		seen = sets.NewString()
	}

	if maxEntries == 0 {
		maxEntries = 500
	}

	for name, cd := range cloud.disks {
		// Only return v1 disks for simplicity
		if !seen.Has(name) {
			d = append(d, cd.disk)
			seen.Insert(name)
			count++
		}

		if count >= maxEntries {
			newToken = string(uuid.NewUUID())
			cloud.pageTokens[newToken] = seen
			break
		}
	}

	return d, newToken, nil
}

func (cloud *FakeCloudProvider) ListSnapshots(ctx context.Context, filter string, maxEntries int64, pageToken string) ([]*computev1.Snapshot, string, error) {
	var sourceDisk string
	snapshots := []*computev1.Snapshot{}
	if len(filter) > 0 {
		filterSplits := strings.Fields(filter)
		if len(filterSplits) != 3 || filterSplits[0] != "sourceDisk" {
			return nil, "", invalidError()
		}
		sourceDisk = filterSplits[2]
	}
	for _, snapshot := range cloud.snapshots {
		if len(sourceDisk) > 0 {
			if snapshot.SourceDisk == sourceDisk {
				continue
			}
		}
		snapshots = append(snapshots, snapshot)
	}

	var (
		ulenSnapshots = len(snapshots)
		startingToken int
	)

	if len(pageToken) > 0 {
		i, err := strconv.ParseUint(pageToken, 10, 32)
		if err != nil {
			return nil, "", invalidError()
		}
		startingToken = int(i)
	}

	if startingToken > ulenSnapshots {
		return nil, "", invalidError()
	}

	// Discern the number of remaining entries.
	rem := ulenSnapshots - startingToken

	// If maxEntries is 0 or greater than the number of remaining entries then
	// set maxEntries to the number of remaining entries.
	max := int(maxEntries)
	if max == 0 || max > rem {
		max = rem
	}

	results := []*computev1.Snapshot{}
	j := startingToken
	for i := 0; i < max; i++ {
		results = append(results, snapshots[j])
		j++
	}

	var nextToken string
	if j < ulenSnapshots {
		nextToken = fmt.Sprintf("%d", j)
	}
	return results, nextToken, nil
}

// Disk Methods
func (cloud *FakeCloudProvider) GetDisk(ctx context.Context, volKey *meta.Key, api GCEAPIVersion) (*CloudDisk, error) {
	disk, ok := cloud.disks[volKey.Name]
	if !ok {
		return nil, notFoundError()
	}
	return disk, nil
}

func (cloud *FakeCloudProvider) ValidateExistingDisk(ctx context.Context, resp *CloudDisk, params common.DiskParameters, reqBytes, limBytes int64, multiWriter bool) error {
	if resp == nil {
		return fmt.Errorf("disk does not exist")
	}
	requestValid := common.GbToBytes(resp.GetSizeGb()) >= reqBytes || reqBytes == 0
	responseValid := common.GbToBytes(resp.GetSizeGb()) <= limBytes || limBytes == 0
	if !requestValid || !responseValid {
		return fmt.Errorf(
			"disk already exists with incompatible capacity. Need %v (Required) < %v (Existing) < %v (Limit)",
			reqBytes, common.GbToBytes(resp.GetSizeGb()), limBytes)
	}

	respType := strings.Split(resp.GetPDType(), "/")
	typeMatch := strings.TrimSpace(respType[len(respType)-1]) == strings.TrimSpace(params.DiskType)
	typeDefault := params.DiskType == "" && strings.TrimSpace(respType[len(respType)-1]) == "pd-standard"
	if !typeMatch && !typeDefault {
		return fmt.Errorf("disk already exists with incompatible type. Need %v. Got %v",
			params.DiskType, respType[len(respType)-1])
	}

	// We are assuming here that a multiWriter disk could be used as non-multiWriter
	if multiWriter && !resp.GetMultiWriter() {
		return fmt.Errorf("disk already exists with incompatible capability. Need MultiWriter. Got non-MultiWriter")
	}

	klog.V(4).Infof("Compatible disk already exists")
	return ValidateDiskParameters(resp, params)
}

func (cloud *FakeCloudProvider) InsertDisk(ctx context.Context, volKey *meta.Key, params common.DiskParameters, capBytes int64, capacityRange *csi.CapacityRange, replicaZones []string, snapshotID string, multiWriter bool) error {
	if disk, ok := cloud.disks[volKey.Name]; ok {
		err := cloud.ValidateExistingDisk(ctx, disk, params,
			int64(capacityRange.GetRequiredBytes()),
			int64(capacityRange.GetLimitBytes()),
			multiWriter)
		if err != nil {
			return err
		}
	}

	computeDisk := &computev1.Disk{
		Name:             volKey.Name,
		SizeGb:           common.BytesToGb(capBytes),
		Description:      "Disk created by GCE-PD CSI Driver",
		Type:             cloud.GetDiskTypeURI(volKey, params.DiskType),
		SourceSnapshotId: snapshotID,
		Status:           cloud.mockDiskStatus,
	}
	if params.DiskEncryptionKMSKey != "" {
		computeDisk.DiskEncryptionKey = &computev1.CustomerEncryptionKey{
			KmsKeyName: params.DiskEncryptionKMSKey,
		}
	}
	switch volKey.Type() {
	case meta.Zonal:
		computeDisk.Zone = volKey.Zone
		computeDisk.SelfLink = fmt.Sprintf("projects/%s/zones/%s/disks/%s", cloud.project, volKey.Zone, volKey.Name)
	case meta.Regional:
		computeDisk.Region = volKey.Region
		computeDisk.SelfLink = fmt.Sprintf("projects/%s/regions/%s/disks/%s", cloud.project, volKey.Region, volKey.Name)
	default:
		return fmt.Errorf("could not create disk, key was neither zonal nor regional, instead got: %v", volKey.String())
	}

	cloud.disks[volKey.Name] = CloudDiskFromV1(computeDisk)
	return nil
}

func (cloud *FakeCloudProvider) DeleteDisk(ctx context.Context, volKey *meta.Key) error {
	if _, ok := cloud.disks[volKey.Name]; !ok {
		return notFoundError()
	}
	delete(cloud.disks, volKey.Name)
	return nil
}

func (cloud *FakeCloudProvider) AttachDisk(ctx context.Context, volKey *meta.Key, readWrite, diskType, instanceZone, instanceName string) error {
	source := cloud.GetDiskSourceURI(volKey)

	attachedDiskV1 := &computev1.AttachedDisk{
		DeviceName: volKey.Name,
		Kind:       diskKind,
		Mode:       readWrite,
		Source:     source,
		Type:       diskType,
	}
	instance, ok := cloud.instances[instanceName]
	if !ok {
		return fmt.Errorf("Failed to get instance %v", instanceName)
	}
	instance.Disks = append(instance.Disks, attachedDiskV1)
	return nil
}

func (cloud *FakeCloudProvider) DetachDisk(ctx context.Context, deviceName, instanceZone, instanceName string) error {
	instance, ok := cloud.instances[instanceName]
	if !ok {
		return fmt.Errorf("Failed to get instance %v", instanceName)
	}
	found := -1
	for i, disk := range instance.Disks {
		if disk.DeviceName == deviceName {
			found = i
			break
		}
	}
	instance.Disks[found] = instance.Disks[len(instance.Disks)-1]
	instance.Disks = instance.Disks[:len(instance.Disks)-1]
	return nil
}

func (cloud *FakeCloudProvider) GetDiskTypeURI(volKey *meta.Key, diskType string) string {
	switch volKey.Type() {
	case meta.Zonal:
		return cloud.getZonalDiskTypeURI(volKey.Zone, diskType)
	case meta.Regional:
		return cloud.getRegionalDiskTypeURI(volKey.Region, diskType)
	default:
		return fmt.Sprintf("could not get disk type uri, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *FakeCloudProvider) getZonalDiskTypeURI(zone, diskType string) string {
	return fmt.Sprintf(diskTypeURITemplateSingleZone, cloud.project, zone, diskType)
}

func (cloud *FakeCloudProvider) getRegionalDiskTypeURI(region, diskType string) string {
	return fmt.Sprintf(diskTypeURITemplateRegional, cloud.project, region, diskType)
}

func (cloud *FakeCloudProvider) WaitForAttach(ctx context.Context, volKey *meta.Key, instanceZone, instanceName string) error {
	return nil
}

// Regional Disk Methods
func (cloud *FakeCloudProvider) GetReplicaZoneURI(zone string) string {
	return ""
}

// Instance Methods
func (cloud *FakeCloudProvider) InsertInstance(instance *computev1.Instance, instanceZone, instanceName string) {
	cloud.instances[instanceName] = instance
	return
}

func (cloud *FakeCloudProvider) GetInstanceOrError(ctx context.Context, instanceZone, instanceName string) (*computev1.Instance, error) {
	instance, ok := cloud.instances[instanceName]
	if !ok {
		return nil, notFoundError()
	}
	return instance, nil
}

// Snapshot Methods
func (cloud *FakeCloudProvider) GetSnapshot(ctx context.Context, snapshotName string) (*computev1.Snapshot, error) {
	snapshot, ok := cloud.snapshots[snapshotName]
	if !ok {
		return nil, notFoundError()
	}
	snapshot.Status = "READY"
	return snapshot, nil
}

func (cloud *FakeCloudProvider) CreateSnapshot(ctx context.Context, volKey *meta.Key, snapshotName string) (*computev1.Snapshot, error) {
	if snapshot, ok := cloud.snapshots[snapshotName]; ok {
		return snapshot, nil
	}

	snapshotToCreate := &computev1.Snapshot{
		Name:              snapshotName,
		DiskSizeGb:        int64(DiskSizeGb),
		CreationTimestamp: Timestamp,
		Status:            "UPLOADING",
		SelfLink:          cloud.getGlobalSnapshotURI(snapshotName),
	}
	switch volKey.Type() {
	case meta.Zonal:
		snapshotToCreate.SourceDisk = cloud.getZonalDiskSourceURI(volKey.Name, volKey.Zone)
	case meta.Regional:
		snapshotToCreate.SourceDisk = cloud.getRegionalDiskSourceURI(volKey.Name, volKey.Region)
	default:
		return nil, fmt.Errorf("could not create snapshot, disk key was neither zonal nor regional, instead got: %v", volKey.String())
	}

	cloud.snapshots[snapshotName] = snapshotToCreate
	return snapshotToCreate, nil
}

func (cloud *FakeCloudProvider) ResizeDisk(ctx context.Context, volKey *meta.Key, requestBytes int64) (int64, error) {
	disk, ok := cloud.disks[volKey.Name]
	if !ok {
		return -1, notFoundError()
	}

	disk.setSizeGb(common.BytesToGb(requestBytes))

	return common.BytesToGb(requestBytes), nil

}

// Snapshot Methods
func (cloud *FakeCloudProvider) DeleteSnapshot(ctx context.Context, snapshotName string) error {
	delete(cloud.snapshots, snapshotName)
	return nil
}

func (cloud *FakeCloudProvider) ValidateExistingSnapshot(resp *computev1.Snapshot, volKey *meta.Key) error {
	if resp == nil {
		return fmt.Errorf("disk does not exist")
	}

	diskSource := cloud.GetDiskSourceURI(volKey)
	if resp.SourceDisk != diskSource {
		return status.Error(codes.AlreadyExists, fmt.Sprintf("snapshot already exists with same name but with a different disk source %s, expected disk source %s", diskSource, resp.SourceDisk))
	}
	// Snapshot exists with matching source disk.
	klog.V(4).Infof("Compatible snapshot already exists. Reusing existing.")
	return nil
}

func (cloud *FakeCloudProvider) GetDiskSourceURI(volKey *meta.Key) string {
	switch volKey.Type() {
	case meta.Zonal:
		return cloud.getZonalDiskSourceURI(volKey.Name, volKey.Zone)
	case meta.Regional:
		return cloud.getRegionalDiskSourceURI(volKey.Name, volKey.Region)
	default:
		return ""
	}
}

func (cloud *FakeCloudProvider) getZonalDiskSourceURI(diskName, zone string) string {
	return BasePath + fmt.Sprintf(
		diskSourceURITemplateSingleZone,
		cloud.project,
		zone,
		diskName)
}

func (cloud *FakeCloudProvider) getRegionalDiskSourceURI(diskName, region string) string {
	return BasePath + fmt.Sprintf(
		diskSourceURITemplateRegional,
		cloud.project,
		region,
		diskName)
}

func (cloud *FakeCloudProvider) getGlobalSnapshotURI(snapshotName string) string {
	return BasePath + fmt.Sprintf(
		snapshotURITemplateGlobal,
		cloud.project,
		snapshotName)
}

func (cloud *FakeCloudProvider) UpdateDiskStatus(s string) {
	cloud.mockDiskStatus = s
}

type FakeBlockingCloudProvider struct {
	*FakeCloudProvider
	ReadyToExecute chan chan struct{}
}

// FakeBlockingCloudProvider's method adds functionality to finely control the order of execution of CreateSnapshot calls.
// Upon starting a CreateSnapshot, it passes a chan 'executeCreateSnapshot' into readyToExecute, then blocks on executeCreateSnapshot.
// The test calling this function can block on readyToExecute to ensure that the operation has started and
// allowed the CreateSnapshot to continue by passing a struct into executeCreateSnapshot.
func (cloud *FakeBlockingCloudProvider) CreateSnapshot(ctx context.Context, volKey *meta.Key, snapshotName string) (*computev1.Snapshot, error) {
	executeCreateSnapshot := make(chan struct{})
	cloud.ReadyToExecute <- executeCreateSnapshot
	<-executeCreateSnapshot
	return cloud.FakeCloudProvider.CreateSnapshot(ctx, volKey, snapshotName)
}

func notFoundError() *googleapi.Error {
	return &googleapi.Error{
		Errors: []googleapi.ErrorItem{
			{
				Reason: "notFound",
			},
		},
	}
}

func invalidError() *googleapi.Error {
	return &googleapi.Error{
		Errors: []googleapi.ErrorItem{
			{
				Reason: "invalid",
			},
		},
	}
}
