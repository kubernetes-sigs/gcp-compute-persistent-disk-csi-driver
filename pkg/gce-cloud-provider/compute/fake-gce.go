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
	"strings"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	csi "github.com/container-storage-interface/spec/lib/go/csi"
	computev1 "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	DiskSizeGb                = 10
	Timestamp                 = "2018-09-05T15:17:08.270-07:00"
	BasePath                  = "https://www.googleapis.com/compute/v1/projects/"
	snapshotURITemplateGlobal = "%s/global/snapshots/%s" //{gce.projectID}/global/snapshots/{snapshot.Name}"
	imageURITemplateGlobal    = "%s/global/images/%s"    //{gce.projectID}/global/images/{image.Name}"
)

type FakeCloudProvider struct {
	project string
	zone    string

	disks      map[string]*CloudDisk
	pageTokens map[string]sets.String
	instances  map[string]*computev1.Instance
	snapshots  map[string]*computev1.Snapshot
	images     map[string]*computev1.Image

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
		images:     map[string]*computev1.Image{},
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

func (cloud *FakeCloudProvider) RepairUnderspecifiedVolumeKey(ctx context.Context, project string, volumeKey *meta.Key) (string, *meta.Key, error) {
	if project == common.UnspecifiedValue {
		project = cloud.project
	}
	switch volumeKey.Type() {
	case meta.Zonal:
		if volumeKey.Zone != common.UnspecifiedValue {
			return project, volumeKey, nil
		}
		for name, d := range cloud.disks {
			if name == volumeKey.Name {
				volumeKey.Zone = d.GetZone()
				return project, volumeKey, nil
			}
		}
		return "", nil, notFoundError()
	case meta.Regional:
		if volumeKey.Region != common.UnspecifiedValue {
			return project, volumeKey, nil
		}
		r, err := common.GetRegionFromZones([]string{cloud.zone})
		if err != nil {
			return "", nil, fmt.Errorf("failed to get region from zones: %v", err)
		}
		volumeKey.Region = r
		return project, volumeKey, nil
	default:
		return "", nil, fmt.Errorf("Volume key %v not zonal nor regional", volumeKey.Name)
	}
}

func (cloud *FakeCloudProvider) ListZones(ctx context.Context, region string) ([]string, error) {
	return []string{cloud.zone, "country-region-fakesecondzone"}, nil
}

func (cloud *FakeCloudProvider) ListDisks(ctx context.Context) ([]*computev1.Disk, string, error) {
	d := []*computev1.Disk{}
	for _, cd := range cloud.disks {
		d = append(d, cd.disk)
	}
	return d, "", nil
}

func (cloud *FakeCloudProvider) ListSnapshots(ctx context.Context, filter string) ([]*computev1.Snapshot, string, error) {
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

	return snapshots, "", nil
}

// Disk Methods
func (cloud *FakeCloudProvider) GetDisk(ctx context.Context, project string, volKey *meta.Key, api GCEAPIVersion) (*CloudDisk, error) {
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

func (cloud *FakeCloudProvider) InsertDisk(ctx context.Context, project string, volKey *meta.Key, params common.DiskParameters, capBytes int64, capacityRange *csi.CapacityRange, replicaZones []string, snapshotID string, volumeContentSourceVolumeID string, multiWriter bool) error {
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
		Name:         volKey.Name,
		SizeGb:       common.BytesToGbRoundUp(capBytes),
		Description:  "Disk created by GCE-PD CSI Driver",
		Type:         cloud.GetDiskTypeURI(project, volKey, params.DiskType),
		SourceDiskId: volumeContentSourceVolumeID,
		Status:       cloud.mockDiskStatus,
		Labels:       params.Labels,
	}

	if snapshotID != "" {
		_, snapshotType, _, err := common.SnapshotIDToProjectKey(snapshotID)
		if err != nil {
			return err
		}
		switch snapshotType {
		case common.DiskSnapshotType:
			computeDisk.SourceSnapshotId = snapshotID
		case common.DiskImageType:
			computeDisk.SourceImageId = snapshotID
		default:
			return fmt.Errorf("invalid snapshot type in snapshot ID: %s", snapshotType)
		}
	}

	if params.DiskEncryptionKMSKey != "" {
		computeDisk.DiskEncryptionKey = &computev1.CustomerEncryptionKey{
			KmsKeyName: params.DiskEncryptionKMSKey,
		}
	}
	switch volKey.Type() {
	case meta.Zonal:
		computeDisk.Zone = volKey.Zone
		computeDisk.SelfLink = fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, volKey.Zone, volKey.Name)
	case meta.Regional:
		computeDisk.Region = volKey.Region
		computeDisk.SelfLink = fmt.Sprintf("projects/%s/regions/%s/disks/%s", project, volKey.Region, volKey.Name)
	default:
		return fmt.Errorf("could not create disk, key was neither zonal nor regional, instead got: %v", volKey.String())
	}

	cloud.disks[volKey.Name] = CloudDiskFromV1(computeDisk)
	return nil
}

func (cloud *FakeCloudProvider) DeleteDisk(ctx context.Context, project string, volKey *meta.Key) error {
	delete(cloud.disks, volKey.Name)
	return nil
}

func (cloud *FakeCloudProvider) AttachDisk(ctx context.Context, project string, volKey *meta.Key, readWrite, diskType, instanceZone, instanceName string) error {
	source := cloud.GetDiskSourceURI(project, volKey)

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

func (cloud *FakeCloudProvider) DetachDisk(ctx context.Context, project, deviceName, instanceZone, instanceName string) error {
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

func (cloud *FakeCloudProvider) GetDiskTypeURI(project string, volKey *meta.Key, diskType string) string {
	switch volKey.Type() {
	case meta.Zonal:
		return cloud.getZonalDiskTypeURI(project, volKey.Zone, diskType)
	case meta.Regional:
		return cloud.getRegionalDiskTypeURI(project, volKey.Region, diskType)
	default:
		return fmt.Sprintf("could not get disk type uri, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *FakeCloudProvider) getZonalDiskTypeURI(project, zone, diskType string) string {
	return fmt.Sprintf(diskTypeURITemplateSingleZone, project, zone, diskType)
}

func (cloud *FakeCloudProvider) getRegionalDiskTypeURI(project, region, diskType string) string {
	return fmt.Sprintf(diskTypeURITemplateRegional, project, region, diskType)
}

func (cloud *FakeCloudProvider) WaitForAttach(ctx context.Context, project string, volKey *meta.Key, instanceZone, instanceName string) error {
	return nil
}

// Regional Disk Methods
func (cloud *FakeCloudProvider) GetReplicaZoneURI(project, zone string) string {
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
func (cloud *FakeCloudProvider) GetSnapshot(ctx context.Context, project, snapshotName string) (*computev1.Snapshot, error) {
	snapshot, ok := cloud.snapshots[snapshotName]
	if !ok {
		return nil, notFoundError()
	}
	snapshot.Status = "READY"
	return snapshot, nil
}

func (cloud *FakeCloudProvider) CreateSnapshot(ctx context.Context, project string, volKey *meta.Key, snapshotName string, snapshotParams common.SnapshotParameters) (*computev1.Snapshot, error) {
	if snapshot, ok := cloud.snapshots[snapshotName]; ok {
		return snapshot, nil
	}

	snapshotToCreate := &computev1.Snapshot{
		Name:              snapshotName,
		DiskSizeGb:        int64(DiskSizeGb),
		CreationTimestamp: Timestamp,
		Status:            "UPLOADING",
		SelfLink:          cloud.getGlobalSnapshotURI(project, snapshotName),
		StorageLocations:  snapshotParams.StorageLocations,
	}
	switch volKey.Type() {
	case meta.Zonal:
		snapshotToCreate.SourceDisk = cloud.getZonalDiskSourceURI(project, volKey.Name, volKey.Zone)
	case meta.Regional:
		snapshotToCreate.SourceDisk = cloud.getRegionalDiskSourceURI(project, volKey.Name, volKey.Region)
	default:
		return nil, fmt.Errorf("could not create snapshot, disk key was neither zonal nor regional, instead got: %v", volKey.String())
	}

	cloud.snapshots[snapshotName] = snapshotToCreate
	return snapshotToCreate, nil
}

func (cloud *FakeCloudProvider) ResizeDisk(ctx context.Context, project string, volKey *meta.Key, requestBytes int64) (int64, error) {
	disk, ok := cloud.disks[volKey.Name]
	if !ok {
		return -1, notFoundError()
	}

	requestSizGb := common.BytesToGbRoundUp(requestBytes)

	disk.setSizeGb(requestSizGb)

	return requestSizGb, nil

}

// Snapshot Methods
func (cloud *FakeCloudProvider) DeleteSnapshot(ctx context.Context, project, snapshotName string) error {
	delete(cloud.snapshots, snapshotName)
	return nil
}

func (cloud *FakeCloudProvider) ListImages(ctx context.Context, filter string) ([]*computev1.Image, string, error) {
	var sourceDisk string
	images := []*computev1.Image{}
	if len(filter) > 0 {
		filterSplits := strings.Fields(filter)
		if len(filterSplits) != 3 || filterSplits[0] != "sourceDisk" {
			return nil, "", invalidError()
		}
		sourceDisk = filterSplits[2]
	}
	for _, image := range cloud.images {
		if len(sourceDisk) > 0 {
			if image.SourceDisk == sourceDisk {
				continue
			}
		}
		images = append(images, image)
	}

	return images, "", nil
}

func (cloud *FakeCloudProvider) GetImage(ctx context.Context, project, imageName string) (*computev1.Image, error) {
	image, ok := cloud.images[imageName]
	if !ok {
		return nil, notFoundError()
	}
	image.Status = "READY"
	return image, nil
}

func (cloud *FakeCloudProvider) CreateImage(ctx context.Context, project string, volKey *meta.Key, imageName string, snapshotParams common.SnapshotParameters) (*computev1.Image, error) {
	if image, ok := cloud.images[imageName]; ok {
		return image, nil
	}

	imageToCreate := &computev1.Image{
		CreationTimestamp: Timestamp,
		DiskSizeGb:        int64(DiskSizeGb),
		Family:            snapshotParams.ImageFamily,
		Name:              imageName,
		SelfLink:          cloud.getGlobalImageURI(project, imageName),
		SourceType:        "RAW",
		Status:            "PENDING",
		StorageLocations:  snapshotParams.StorageLocations,
	}

	switch volKey.Type() {
	case meta.Zonal:
		imageToCreate.SourceDisk = cloud.getZonalDiskSourceURI(project, volKey.Name, volKey.Zone)
	case meta.Regional:
		imageToCreate.SourceDisk = cloud.getRegionalDiskSourceURI(project, volKey.Name, volKey.Region)
	default:
		return nil, fmt.Errorf("could not create image, disk key was neither zonal nor regional, instead got: %v", volKey.String())
	}

	cloud.images[imageName] = imageToCreate
	return imageToCreate, nil
}

func (cloud *FakeCloudProvider) DeleteImage(ctx context.Context, project, imageName string) error {
	delete(cloud.images, imageName)
	return nil
}

func (cloud *FakeCloudProvider) ValidateExistingSnapshot(resp *computev1.Snapshot, volKey *meta.Key) error {
	if resp == nil {
		return fmt.Errorf("disk does not exist")
	}

	diskSource := cloud.GetDiskSourceURI(cloud.project, volKey)
	if resp.SourceDisk != diskSource {
		return status.Error(codes.AlreadyExists, fmt.Sprintf("snapshot already exists with same name but with a different disk source %s, expected disk source %s", diskSource, resp.SourceDisk))
	}
	// Snapshot exists with matching source disk.
	klog.V(4).Infof("Compatible snapshot already exists. Reusing existing.")
	return nil
}

func (cloud *FakeCloudProvider) GetDiskSourceURI(project string, volKey *meta.Key) string {
	switch volKey.Type() {
	case meta.Zonal:
		return cloud.getZonalDiskSourceURI(project, volKey.Name, volKey.Zone)
	case meta.Regional:
		return cloud.getRegionalDiskSourceURI(project, volKey.Name, volKey.Region)
	default:
		return ""
	}
}

func (cloud *FakeCloudProvider) getZonalDiskSourceURI(project, diskName, zone string) string {
	return BasePath + fmt.Sprintf(
		diskSourceURITemplateSingleZone,
		project,
		zone,
		diskName)
}

func (cloud *FakeCloudProvider) getRegionalDiskSourceURI(project, diskName, region string) string {
	return BasePath + fmt.Sprintf(
		diskSourceURITemplateRegional,
		project,
		region,
		diskName)
}

func (cloud *FakeCloudProvider) getGlobalSnapshotURI(project, snapshotName string) string {
	return BasePath + fmt.Sprintf(
		snapshotURITemplateGlobal,
		project,
		snapshotName)
}

func (cloud *FakeCloudProvider) getGlobalImageURI(project, imageName string) string {
	return BasePath + fmt.Sprintf(
		imageURITemplateGlobal,
		project,
		imageName)
}

func (cloud *FakeCloudProvider) UpdateDiskStatus(s string) {
	cloud.mockDiskStatus = s
}

type FakeBlockingCloudProvider struct {
	*FakeCloudProvider
	ReadyToExecute chan chan Signal
}

// FakeBlockingCloudProvider's method adds functionality to finely control the order of execution of CreateSnapshot calls.
// Upon starting a CreateSnapshot, it passes a chan 'executeCreateSnapshot' into readyToExecute, then blocks on executeCreateSnapshot.
// The test calling this function can block on readyToExecute to ensure that the operation has started and
// allowed the CreateSnapshot to continue by passing a struct into executeCreateSnapshot.
func (cloud *FakeBlockingCloudProvider) CreateSnapshot(ctx context.Context, project string, volKey *meta.Key, snapshotName string, snapshotParams common.SnapshotParameters) (*computev1.Snapshot, error) {
	executeCreateSnapshot := make(chan Signal)
	cloud.ReadyToExecute <- executeCreateSnapshot
	<-executeCreateSnapshot
	return cloud.FakeCloudProvider.CreateSnapshot(ctx, project, volKey, snapshotName, snapshotParams)
}

func (cloud *FakeBlockingCloudProvider) CreateImage(ctx context.Context, project string, volKey *meta.Key, imageName string, snapshotParams common.SnapshotParameters) (*computev1.Image, error) {
	executeCreateSnapshot := make(chan Signal)
	cloud.ReadyToExecute <- executeCreateSnapshot
	<-executeCreateSnapshot
	return cloud.FakeCloudProvider.CreateImage(ctx, project, volKey, imageName, snapshotParams)
}

func (cloud *FakeBlockingCloudProvider) DetachDisk(ctx context.Context, project, deviceName, instanceZone, instanceName string) error {
	execute := make(chan Signal)
	cloud.ReadyToExecute <- execute
	val := <-execute
	if val.ReportError {
		return fmt.Errorf("force mock error for DetachDisk device %s", deviceName)
	}
	return cloud.FakeCloudProvider.DetachDisk(ctx, project, deviceName, instanceZone, instanceName)
}

func (cloud *FakeBlockingCloudProvider) AttachDisk(ctx context.Context, project string, volKey *meta.Key, readWrite, diskType, instanceZone, instanceName string) error {
	execute := make(chan Signal)
	cloud.ReadyToExecute <- execute
	val := <-execute
	if val.ReportError {
		return fmt.Errorf("force mock error for AttachDisk: volkey %s", volKey)
	}
	return cloud.FakeCloudProvider.AttachDisk(ctx, project, volKey, readWrite, diskType, instanceZone, instanceName)
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

type Signal struct {
	ReportError   bool
	ReportRunning bool
}
