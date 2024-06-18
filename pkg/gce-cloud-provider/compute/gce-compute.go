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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	rscmgr "cloud.google.com/go/resourcemanager/apiv3"
	rscmgrpb "cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/apierror"
	"golang.org/x/oauth2"
	computebeta "google.golang.org/api/compute/v0.beta"
	computev1 "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

const (
	operationStatusDone            = "DONE"
	waitForSnapshotCreationTimeOut = 2 * time.Minute
	waitForImageCreationTimeOut    = 5 * time.Minute
	diskKind                       = "compute#disk"
	cryptoKeyVerDelimiter          = "/cryptoKeyVersions"
	// Example message: "[pd-standard] features are not compatible for creating instance"
	pdDiskTypeUnsupportedPattern = `\[([a-z-]+)\] features are not compatible for creating instance`
)

var hyperdiskTypes = []string{"hyperdisk-extreme", "hyperdisk-throughput", "hyperdisk-balanced"}
var pdDiskTypeUnsupportedRegex = regexp.MustCompile(pdDiskTypeUnsupportedPattern)

type GCEAPIVersion string

const (
	// V1 key type
	GCEAPIVersionV1 GCEAPIVersion = "v1"
	// Beta key type
	GCEAPIVersionBeta GCEAPIVersion = "beta"
)

var GCEAPIVersions = []GCEAPIVersion{GCEAPIVersionBeta, GCEAPIVersionV1}

// AttachDiskBackoff is backoff used to wait for AttachDisk to complete.
// Default values are similar to Poll every 5 seconds with 2 minute timeout.
var AttachDiskBackoff = wait.Backoff{
	Duration: 5 * time.Second,
	Factor:   0.0,
	Jitter:   0.0,
	Steps:    24,
	Cap:      0}

// WaitForOpBackoff is backoff used to wait for Global, Regional or Zonal operation to complete.
// Default values are similar to Poll every 3 seconds with 5 minute timeout.
var WaitForOpBackoff = wait.Backoff{
	Duration: 3 * time.Second,
	Factor:   0.0,
	Jitter:   0.0,
	Steps:    100,
	Cap:      0}

// Custom error type to propagate error messages up to clients.
type UnsupportedDiskError struct {
	DiskType string
}

func (udErr *UnsupportedDiskError) Error() string {
	return ""
}

type GCECompute interface {
	// Metadata information
	GetDefaultProject() string
	GetDefaultZone() string
	// Disk Methods
	GetDisk(ctx context.Context, project string, volumeKey *meta.Key, gceAPIVersion GCEAPIVersion) (*CloudDisk, error)
	RepairUnderspecifiedVolumeKey(ctx context.Context, project string, volumeKey *meta.Key) (string, *meta.Key, error)
	ValidateExistingDisk(ctx context.Context, disk *CloudDisk, params common.DiskParameters, reqBytes, limBytes int64, multiWriter bool) error
	InsertDisk(ctx context.Context, project string, volKey *meta.Key, params common.DiskParameters, capBytes int64, capacityRange *csi.CapacityRange, replicaZones []string, snapshotID string, volumeContentSourceVolumeID string, multiWriter bool, accessMode string) error
	DeleteDisk(ctx context.Context, project string, volumeKey *meta.Key) error
	UpdateDisk(ctx context.Context, project string, volKey *meta.Key, existingDisk *CloudDisk, params common.ModifyVolumeParameters) error
	AttachDisk(ctx context.Context, project string, volKey *meta.Key, readWrite, diskType, instanceZone, instanceName string, forceAttach bool) error
	DetachDisk(ctx context.Context, project, deviceName, instanceZone, instanceName string) error
	SetDiskAccessMode(ctx context.Context, project string, volKey *meta.Key, accessMode string) error
	ListCompatibleDiskTypeZones(ctx context.Context, project string, zones []string, diskType string) ([]string, error)
	GetDiskSourceURI(project string, volKey *meta.Key) string
	GetDiskTypeURI(project string, volKey *meta.Key, diskType string) string
	WaitForAttach(ctx context.Context, project string, volKey *meta.Key, diskType, instanceZone, instanceName string) error
	ResizeDisk(ctx context.Context, project string, volKey *meta.Key, requestBytes int64) (int64, error)
	ListDisks(ctx context.Context, fields []googleapi.Field) ([]*computev1.Disk, string, error)
	ListDisksWithFilter(ctx context.Context, fields []googleapi.Field, filter string) ([]*computev1.Disk, string, error)
	ListInstances(ctx context.Context, fields []googleapi.Field) ([]*computev1.Instance, string, error)
	// Regional Disk Methods
	GetReplicaZoneURI(project string, zone string) string
	// Instance Methods
	GetInstanceOrError(ctx context.Context, instanceZone, instanceName string) (*computev1.Instance, error)
	// Zone Methods
	ListZones(ctx context.Context, region string) ([]string, error)
	ListSnapshots(ctx context.Context, filter string) ([]*computev1.Snapshot, string, error)
	GetSnapshot(ctx context.Context, project, snapshotName string) (*computev1.Snapshot, error)
	CreateSnapshot(ctx context.Context, project string, volKey *meta.Key, snapshotName string, snapshotParams common.SnapshotParameters) (*computev1.Snapshot, error)
	DeleteSnapshot(ctx context.Context, project, snapshotName string) error
	ListImages(ctx context.Context, filter string) ([]*computev1.Image, string, error)
	GetImage(ctx context.Context, project, imageName string) (*computev1.Image, error)
	CreateImage(ctx context.Context, project string, volKey *meta.Key, imageName string, snapshotParams common.SnapshotParameters) (*computev1.Image, error)
	DeleteImage(ctx context.Context, project, imageName string) error
}

// GetDefaultProject returns the project that was used to instantiate this GCE client.
func (cloud *CloudProvider) GetDefaultProject() string {
	return cloud.project
}

// GetDefaultZone returns the zone that was used to instantiate this GCE client.
func (cloud *CloudProvider) GetDefaultZone() string {
	return cloud.zone
}

// ListDisks lists disks based on maxEntries and pageToken only in the project
// and region that the driver is running in.
func (cloud *CloudProvider) ListDisks(ctx context.Context, fields []googleapi.Field) ([]*computev1.Disk, string, error) {
	filter := ""
	return cloud.listDisksInternal(ctx, fields, filter)
}

func (cloud *CloudProvider) ListDisksWithFilter(ctx context.Context, fields []googleapi.Field, filter string) ([]*computev1.Disk, string, error) {
	return cloud.listDisksInternal(ctx, fields, filter)
}

func (cloud *CloudProvider) listDisksInternal(ctx context.Context, fields []googleapi.Field, filter string) ([]*computev1.Disk, string, error) {
	region, err := common.GetRegionFromZones([]string{cloud.zone})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get region from zones: %w", err)
	}
	zones, err := cloud.ListZones(ctx, region)
	if err != nil {
		return nil, "", err
	}
	items := []*computev1.Disk{}

	// listing out regional disks in the region
	rlCall := cloud.service.RegionDisks.List(cloud.project, region)
	rlCall.Fields(fields...)
	rlCall.Filter(filter)
	nextPageToken := "pageToken"
	for nextPageToken != "" {
		rDiskList, err := rlCall.Do()
		if err != nil {
			return nil, "", err
		}
		items = append(items, rDiskList.Items...)
		nextPageToken = rDiskList.NextPageToken
		rlCall.PageToken(nextPageToken)
	}

	// listing out zonal disks in all zones of the region
	for _, zone := range zones {
		lCall := cloud.service.Disks.List(cloud.project, zone)
		lCall.Fields(fields...)
		lCall.Filter(filter)
		nextPageToken := "pageToken"
		for nextPageToken != "" {
			diskList, err := lCall.Do()
			if err != nil {
				return nil, "", err
			}
			items = append(items, diskList.Items...)
			nextPageToken = diskList.NextPageToken
			lCall.PageToken(nextPageToken)
		}
	}
	return items, "", nil
}

// ListInstances lists instances based on maxEntries and pageToken for the project and region
// that the driver is running in. Filters from cloud.listInstancesConfig.Filters are applied
// to the request.
func (cloud *CloudProvider) ListInstances(ctx context.Context, fields []googleapi.Field) ([]*computev1.Instance, string, error) {
	region, err := common.GetRegionFromZones([]string{cloud.zone})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get region from zones: %w", err)
	}
	zones, err := cloud.ListZones(ctx, region)
	if err != nil {
		return nil, "", err
	}
	items := []*computev1.Instance{}

	for _, zone := range zones {
		lCall := cloud.service.Instances.List(cloud.project, zone)
		for _, filter := range cloud.listInstancesConfig.Filters {
			lCall = lCall.Filter(filter)
		}
		lCall = lCall.Fields(fields...)
		nextPageToken := "pageToken"
		for nextPageToken != "" {
			instancesList, err := lCall.Do()
			if err != nil {
				return nil, "", err
			}
			items = append(items, instancesList.Items...)
			nextPageToken = instancesList.NextPageToken
			lCall.PageToken(nextPageToken)
		}
	}

	return items, "", nil
}

// RepairUnderspecifiedVolumeKey will query the cloud provider and check each zone for the disk specified
// by the volume key and return a volume key with a correct zone
func (cloud *CloudProvider) RepairUnderspecifiedVolumeKey(ctx context.Context, project string, volumeKey *meta.Key) (string, *meta.Key, error) {
	klog.V(5).Infof("Repairing potentially underspecified volume key %v", volumeKey)
	if project == common.UnspecifiedValue {
		project = cloud.project
	}
	region, err := common.GetRegionFromZones([]string{cloud.zone})
	if err != nil {
		return "", nil, fmt.Errorf("failed to get region from zones: %w", err)
	}
	switch volumeKey.Type() {
	case meta.Zonal:
		foundZone := ""
		if volumeKey.Zone == common.UnspecifiedValue {
			// list all zones, try to get disk in each zone
			zones, err := cloud.ListZones(ctx, region)
			if err != nil {
				return "", nil, err
			}
			for _, zone := range zones {
				_, err := cloud.getZonalDiskOrError(ctx, project, zone, volumeKey.Name)
				if err != nil {
					if IsGCENotFoundError(err) {
						// Couldn't find the disk in this zone so we keep
						// looking
						continue
					}
					// There is some miscellaneous error getting disk from zone
					// so we return error immediately
					return "", nil, err
				}
				if len(foundZone) > 0 {
					return "", nil, fmt.Errorf("found disk %s in more than one zone: %s and %s", volumeKey.Name, foundZone, zone)
				}
				foundZone = zone
			}

			if len(foundZone) == 0 {
				return "", nil, notFoundError()
			}
			volumeKey.Zone = foundZone
			return project, volumeKey, nil
		}
		return project, volumeKey, nil
	case meta.Regional:
		if volumeKey.Region == common.UnspecifiedValue {
			volumeKey.Region = region
		}
		return project, volumeKey, nil
	default:
		return "", nil, fmt.Errorf("key was neither zonal nor regional, got: %v", volumeKey.String())
	}
}

func (cloud *CloudProvider) ListZones(ctx context.Context, region string) ([]string, error) {
	klog.V(5).Infof("Listing zones in region: %v", region)
	if len(cloud.zonesCache[region]) > 0 {
		return cloud.zonesCache[region], nil
	}
	zones := []string{}
	zoneList, err := cloud.service.Zones.List(cloud.project).Filter(fmt.Sprintf("region eq .*%s$", region)).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list zones in region %s: %w", region, err)
	}
	for _, zone := range zoneList.Items {
		zones = append(zones, zone.Name)
	}
	cloud.zonesCache[region] = zones
	return zones, nil

}

func (cloud *CloudProvider) ListSnapshots(ctx context.Context, filter string) ([]*computev1.Snapshot, string, error) {
	klog.V(5).Infof("Listing snapshots with filter: %s", filter)
	items := []*computev1.Snapshot{}
	lCall := cloud.service.Snapshots.List(cloud.project).Filter(filter)
	nextPageToken := "pageToken"
	for nextPageToken != "" {
		snapshotList, err := lCall.Do()
		if err != nil {
			return nil, "", err
		}
		items = append(items, snapshotList.Items...)
	}
	return items, "", nil
}

func (cloud *CloudProvider) GetDisk(ctx context.Context, project string, key *meta.Key, gceAPIVersion GCEAPIVersion) (*CloudDisk, error) {
	klog.V(5).Infof("Getting disk %v", key)

	// Override GCEAPIVersion as hyperdisk is only available in beta and we cannot get the disk-type with get disk call.
	gceAPIVersion = GCEAPIVersionBeta
	switch key.Type() {
	case meta.Zonal:
		if gceAPIVersion == GCEAPIVersionBeta {
			disk, err := cloud.getZonalBetaDiskOrError(ctx, project, key.Zone, key.Name)
			return CloudDiskFromBeta(disk), err
		} else {
			disk, err := cloud.getZonalDiskOrError(ctx, project, key.Zone, key.Name)
			return CloudDiskFromV1(disk), err
		}
	case meta.Regional:
		if gceAPIVersion == GCEAPIVersionBeta {
			disk, err := cloud.getRegionalBetaDiskOrError(ctx, project, key.Region, key.Name)
			return CloudDiskFromBeta(disk), err
		} else {
			disk, err := cloud.getRegionalDiskOrError(ctx, project, key.Region, key.Name)
			return CloudDiskFromV1(disk), err
		}
	default:
		return nil, fmt.Errorf("key was neither zonal nor regional, got: %v", key.String())
	}
}

func (cloud *CloudProvider) getZonalDiskOrError(ctx context.Context, project, volumeZone, volumeName string) (*computev1.Disk, error) {
	disk, err := cloud.service.Disks.Get(project, volumeZone, volumeName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return disk, nil
}

func (cloud *CloudProvider) getRegionalDiskOrError(ctx context.Context, project, volumeRegion, volumeName string) (*computev1.Disk, error) {
	disk, err := cloud.service.RegionDisks.Get(project, volumeRegion, volumeName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return disk, nil
}

func (cloud *CloudProvider) getZonalBetaDiskOrError(ctx context.Context, project, volumeZone, volumeName string) (*computebeta.Disk, error) {
	disk, err := cloud.betaService.Disks.Get(project, volumeZone, volumeName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return disk, nil
}

func (cloud *CloudProvider) getRegionalBetaDiskOrError(ctx context.Context, project, volumeRegion, volumeName string) (*computebeta.Disk, error) {
	disk, err := cloud.betaService.RegionDisks.Get(project, volumeRegion, volumeName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return disk, nil
}

func (cloud *CloudProvider) GetReplicaZoneURI(project, zone string) string {
	return cloud.service.BasePath + fmt.Sprintf(
		replicaZoneURITemplateSingleZone,
		project,
		zone)
}

func (cloud *CloudProvider) getRegionURI(project, region string) string {
	return cloud.service.BasePath + fmt.Sprintf(
		regionURITemplate,
		project,
		region)
}

func (cloud *CloudProvider) ValidateExistingDisk(ctx context.Context, resp *CloudDisk, params common.DiskParameters, reqBytes, limBytes int64, multiWriter bool) error {
	klog.V(5).Infof("Validating existing disk %v with diskType: %s, reqested bytes: %v, limit bytes: %v", resp, params.DiskType, reqBytes, limBytes)
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

	// We are assuming here that a multiWriter disk could be used as non-multiWriter
	if multiWriter && !resp.GetMultiWriter() {
		return fmt.Errorf("disk already exists with incompatible capability. Need MultiWriter. Got non-MultiWriter")
	}

	return ValidateDiskParameters(resp, params)
}

// ValidateDiskParameters takes a CloudDisk and returns true if the parameters
// specified validly describe the disk provided, and false otherwise.
func ValidateDiskParameters(disk *CloudDisk, params common.DiskParameters) error {
	if disk.GetPDType() != params.DiskType {
		return fmt.Errorf("actual pd type %s did not match the expected param %s", disk.GetPDType(), params.DiskType)
	}

	locationType := disk.LocationType()
	if (params.ReplicationType == "none" && locationType != meta.Zonal) || (params.ReplicationType == "regional-pd" && locationType != meta.Regional) {
		return fmt.Errorf("actual disk replication type %v did not match expected param %s", locationType, params.ReplicationType)
	}

	if !KmsKeyEqual(
		disk.GetKMSKeyName(), /* fetchedKMSKey */
		params.DiskEncryptionKMSKey /* storageClassKMSKey */) {
		return fmt.Errorf("actual disk KMS key name %s did not match expected param %s", disk.GetKMSKeyName(), params.DiskEncryptionKMSKey)
	}

	return nil
}

func (cloud *CloudProvider) InsertDisk(ctx context.Context, project string, volKey *meta.Key, params common.DiskParameters, capBytes int64, capacityRange *csi.CapacityRange, replicaZones []string, snapshotID string, volumeContentSourceVolumeID string, multiWriter bool, accessMode string) error {
	klog.V(5).Infof("Inserting disk %v", volKey)

	description, err := encodeTags(params.Tags)
	if err != nil {
		return err
	}

	switch volKey.Type() {
	case meta.Zonal:
		if description == "" {
			description = "Disk created by GCE-PD CSI Driver"
		}
		return cloud.insertZonalDisk(ctx, project, volKey, params, capBytes, capacityRange, snapshotID, volumeContentSourceVolumeID, description, multiWriter, accessMode)
	case meta.Regional:
		if description == "" {
			description = "Regional disk created by GCE-PD CSI Driver"
		}
		return cloud.insertRegionalDisk(ctx, project, volKey, params, capBytes, capacityRange, replicaZones, snapshotID, volumeContentSourceVolumeID, description, multiWriter)
	default:
		return fmt.Errorf("could not insert disk, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *CloudProvider) UpdateDisk(ctx context.Context, project string, volKey *meta.Key, existingDisk *CloudDisk, params common.ModifyVolumeParameters) error {

	klog.V(5).Infof("Updating disk %v", volKey)
	// hyperdisks are zonal disks
	// pd-disks do not support modification of IOPS and Throughput
	return cloud.updateZonalDisk(ctx, project, volKey, existingDisk, params)
}

func (cloud *CloudProvider) updateZonalDisk(ctx context.Context, project string, volKey *meta.Key, existingDisk *CloudDisk, params common.ModifyVolumeParameters) error {

	updatedDisk := &computev1.Disk{
		Name:                  existingDisk.GetName(),
		ProvisionedIops:       params.IOPS,
		ProvisionedThroughput: params.Throughput,
	}

	diskUpdateOp := cloud.service.Disks.Update(project, volKey.Zone, volKey.Name, updatedDisk)
	diskUpdateOp.Paths("provisionedIops", "provisionedThroughput")
	_, err := diskUpdateOp.Context(ctx).Do()

	if err != nil {
		return fmt.Errorf("error updating disk %v: %w", volKey, err)
	}

	return nil
}

func convertV1CustomerEncryptionKeyToBeta(v1Key *computev1.CustomerEncryptionKey) *computebeta.CustomerEncryptionKey {
	return &computebeta.CustomerEncryptionKey{
		KmsKeyName:      v1Key.KmsKeyName,
		RawKey:          v1Key.RawKey,
		Sha256:          v1Key.Sha256,
		ForceSendFields: v1Key.ForceSendFields,
		NullFields:      v1Key.NullFields,
	}
}

func convertV1DiskParamsToBeta(v1DiskParams *computev1.DiskParams) *computebeta.DiskParams {
	resourceManagerTags := make(map[string]string)
	for k, v := range v1DiskParams.ResourceManagerTags {
		resourceManagerTags[k] = v
	}
	return &computebeta.DiskParams{
		ResourceManagerTags: resourceManagerTags,
	}
}

func convertV1DiskToBetaDisk(v1Disk *computev1.Disk) *computebeta.Disk {
	var dek *computebeta.CustomerEncryptionKey = nil

	if v1Disk.DiskEncryptionKey != nil {
		dek = convertV1CustomerEncryptionKeyToBeta(v1Disk.DiskEncryptionKey)
	}

	var params *computebeta.DiskParams = nil
	if v1Disk.Params != nil {
		params = convertV1DiskParamsToBeta(v1Disk.Params)
	}

	// Note: this is an incomplete list. It only includes the fields we use for disk creation.
	betaDisk := &computebeta.Disk{
		Name:              v1Disk.Name,
		SizeGb:            v1Disk.SizeGb,
		Description:       v1Disk.Description,
		Type:              v1Disk.Type,
		SourceSnapshot:    v1Disk.SourceSnapshot,
		SourceImage:       v1Disk.SourceImage,
		SourceImageId:     v1Disk.SourceImageId,
		SourceSnapshotId:  v1Disk.SourceSnapshotId,
		SourceDisk:        v1Disk.SourceDisk,
		ReplicaZones:      v1Disk.ReplicaZones,
		DiskEncryptionKey: dek,
		Zone:              v1Disk.Zone,
		Region:            v1Disk.Region,
		Status:            v1Disk.Status,
		SelfLink:          v1Disk.SelfLink,
		Params:            params,
		AccessMode:        v1Disk.AccessMode,
	}

	// Hyperdisk doesn't currently support multiWriter (https://cloud.google.com/compute/docs/disks/hyperdisks#limitations),
	// but if multiWriter + hyperdisk is supported in the future, we want the PDCSI driver to support this feature without
	// any additional code change.
	if v1Disk.ProvisionedIops > 0 {
		betaDisk.ProvisionedIops = v1Disk.ProvisionedIops
	}
	if v1Disk.ProvisionedThroughput > 0 {
		betaDisk.ProvisionedThroughput = v1Disk.ProvisionedThroughput
	}
	betaDisk.StoragePool = v1Disk.StoragePool

	return betaDisk
}

func (cloud *CloudProvider) insertRegionalDisk(
	ctx context.Context,
	project string,
	volKey *meta.Key,
	params common.DiskParameters,
	capBytes int64,
	capacityRange *csi.CapacityRange,
	replicaZones []string,
	snapshotID string,
	volumeContentSourceVolumeID string,
	description string,
	multiWriter bool) error {
	var (
		err           error
		opName        string
		gceAPIVersion = GCEAPIVersionV1
	)

	if multiWriter {
		gceAPIVersion = GCEAPIVersionBeta
	}

	diskToCreate := &computev1.Disk{
		Name:        volKey.Name,
		SizeGb:      common.BytesToGbRoundUp(capBytes),
		Description: description,
		Type:        cloud.GetDiskTypeURI(cloud.project, volKey, params.DiskType),
		Labels:      params.Labels,
	}
	if snapshotID != "" {
		_, snapshotType, _, err := common.SnapshotIDToProjectKey(snapshotID)
		if err != nil {
			return err
		}
		switch snapshotType {
		case common.DiskSnapshotType:
			diskToCreate.SourceSnapshot = snapshotID
		case common.DiskImageType:
			diskToCreate.SourceImage = snapshotID
		default:
			return fmt.Errorf("invalid snapshot type in snapshot ID: %s", snapshotType)
		}
	}
	if volumeContentSourceVolumeID != "" {
		diskToCreate.SourceDisk = volumeContentSourceVolumeID
	}

	if len(replicaZones) != 0 {
		diskToCreate.ReplicaZones = replicaZones
	}
	if params.DiskEncryptionKMSKey != "" {
		diskToCreate.DiskEncryptionKey = &computev1.CustomerEncryptionKey{
			KmsKeyName: params.DiskEncryptionKMSKey,
		}
	}
	resourceTags, err := getResourceManagerTags(ctx, cloud.tokenSource, params.ResourceTags)
	if err != nil {
		return err
	}

	if len(resourceTags) > 0 {
		diskToCreate.Params = &computev1.DiskParams{
			ResourceManagerTags: resourceTags,
		}
	}

	if gceAPIVersion == GCEAPIVersionBeta {
		var insertOp *computebeta.Operation
		betaDiskToCreate := convertV1DiskToBetaDisk(diskToCreate)
		betaDiskToCreate.MultiWriter = multiWriter
		insertOp, err = cloud.betaService.RegionDisks.Insert(project, volKey.Region, betaDiskToCreate).Context(ctx).Do()
		if insertOp != nil {
			opName = insertOp.Name
		}
	} else {
		var insertOp *computev1.Operation
		insertOp, err = cloud.service.RegionDisks.Insert(project, volKey.Region, diskToCreate).Context(ctx).Do()
		if insertOp != nil {
			opName = insertOp.Name
		}
	}
	if err != nil {
		if IsGCEError(err, "alreadyExists") {
			disk, err := cloud.GetDisk(ctx, project, volKey, gceAPIVersion)
			if err != nil {
				// failed to GetDisk, however the Disk may already exist
				// the error code should be non-Final
				return common.NewTemporaryError(codes.Unavailable, fmt.Errorf("error when getting disk: %w", err))
			}
			err = cloud.ValidateExistingDisk(ctx, disk, params,
				int64(capacityRange.GetRequiredBytes()),
				int64(capacityRange.GetLimitBytes()),
				multiWriter)
			if err != nil {
				return err
			}
			klog.Warningf("GCE PD %s already exists, reusing", volKey.Name)
			return nil
		}
		// if the error code is considered "final", RegionDisks.Insert might not be retried
		return fmt.Errorf("unknown Insert Regional disk error: %w", err)
	}
	klog.V(5).Infof("InsertDisk operation %s for disk %s", opName, diskToCreate.Name)

	err = cloud.waitForRegionalOp(ctx, project, opName, volKey.Region)
	// failed to wait for Op to finish, however, the Op possibly is still running as expected
	// the error code returned should be non-final
	if err != nil {
		if IsGCEError(err, "alreadyExists") {
			disk, err := cloud.GetDisk(ctx, project, volKey, gceAPIVersion)
			if err != nil {
				return common.NewTemporaryError(codes.Unavailable, fmt.Errorf("error when getting disk: %w", err))
			}
			err = cloud.ValidateExistingDisk(ctx, disk, params,
				int64(capacityRange.GetRequiredBytes()),
				int64(capacityRange.GetLimitBytes()),
				multiWriter)
			if err != nil {
				return err
			}
			klog.Warningf("GCE PD %s already exists after wait, reusing", volKey.Name)
			return nil
		}
		return common.NewTemporaryError(codes.Unavailable, fmt.Errorf("unknown error when polling the operation: %w", err))
	}
	return nil
}

func (cloud *CloudProvider) insertZonalDisk(
	ctx context.Context,
	project string,
	volKey *meta.Key,
	params common.DiskParameters,
	capBytes int64,
	capacityRange *csi.CapacityRange,
	snapshotID string,
	volumeContentSourceVolumeID string,
	description string,
	multiWriter bool,
	accessMode string) error {
	var (
		err           error
		opName        string
		gceAPIVersion = GCEAPIVersionV1
	)
	if multiWriter {
		gceAPIVersion = GCEAPIVersionBeta
	}

	diskToCreate := &computev1.Disk{
		Name:        volKey.Name,
		SizeGb:      common.BytesToGbRoundUp(capBytes),
		Description: description,
		Type:        cloud.GetDiskTypeURI(project, volKey, params.DiskType),
		Labels:      params.Labels,
	}

	if params.ProvisionedIOPSOnCreate > 0 {
		diskToCreate.ProvisionedIops = params.ProvisionedIOPSOnCreate
	}
	if params.ProvisionedThroughputOnCreate > 0 {
		diskToCreate.ProvisionedThroughput = params.ProvisionedThroughputOnCreate
	}

	if params.StoragePools != nil {
		sp := common.StoragePoolInZone(params.StoragePools, volKey.Zone)
		if sp == nil {
			return status.Errorf(codes.InvalidArgument, "cannot create disk in zone %q: no Storage Pools exist in zone", volKey.Zone)
		}
		diskToCreate.StoragePool = sp.ResourceName
	}

	if snapshotID != "" {
		_, snapshotType, _, err := common.SnapshotIDToProjectKey(snapshotID)
		if err != nil {
			return err
		}
		switch snapshotType {
		case common.DiskSnapshotType:
			diskToCreate.SourceSnapshot = snapshotID
		case common.DiskImageType:
			diskToCreate.SourceImage = snapshotID
		default:
			return fmt.Errorf("invalid snapshot type in snapshot ID: %s", snapshotType)
		}
	}
	if volumeContentSourceVolumeID != "" {
		diskToCreate.SourceDisk = volumeContentSourceVolumeID
	}

	if params.DiskEncryptionKMSKey != "" {
		diskToCreate.DiskEncryptionKey = &computev1.CustomerEncryptionKey{
			KmsKeyName: params.DiskEncryptionKMSKey,
		}
	}
	diskToCreate.EnableConfidentialCompute = params.EnableConfidentialCompute

	resourceTags, err := getResourceManagerTags(ctx, cloud.tokenSource, params.ResourceTags)
	if err != nil {
		return err
	}

	if len(resourceTags) > 0 {
		diskToCreate.Params = &computev1.DiskParams{
			ResourceManagerTags: resourceTags,
		}
	}
	diskToCreate.AccessMode = accessMode

	if gceAPIVersion == GCEAPIVersionBeta {
		var insertOp *computebeta.Operation
		betaDiskToCreate := convertV1DiskToBetaDisk(diskToCreate)
		betaDiskToCreate.MultiWriter = multiWriter
		insertOp, err = cloud.betaService.Disks.Insert(project, volKey.Zone, betaDiskToCreate).Context(ctx).Do()
		if insertOp != nil {
			opName = insertOp.Name
		}
	} else {
		var insertOp *computev1.Operation
		insertOp, err = cloud.service.Disks.Insert(project, volKey.Zone, diskToCreate).Context(ctx).Do()
		if insertOp != nil {
			opName = insertOp.Name
		}
	}

	if err != nil {
		if IsGCEError(err, "alreadyExists") {
			disk, err := cloud.GetDisk(ctx, project, volKey, gceAPIVersion)
			if err != nil {
				// failed to GetDisk, however the Disk may already exist
				// the error code should be non-Final
				return common.NewTemporaryError(codes.Unavailable, fmt.Errorf("error when getting disk: %w", err))
			}
			err = cloud.ValidateExistingDisk(ctx, disk, params,
				int64(capacityRange.GetRequiredBytes()),
				int64(capacityRange.GetLimitBytes()),
				multiWriter)
			if err != nil {
				return err
			}
			klog.Warningf("GCE PD %s already exists, reusing", volKey.Name)
			return nil
		}
		// if the error code is considered "final", Disks.Insert might not be retried
		return fmt.Errorf("unknown Insert disk error: %w", err)
	}
	klog.V(5).Infof("InsertDisk operation %s for disk %s", opName, diskToCreate.Name)

	err = cloud.waitForZonalOp(ctx, project, opName, volKey.Zone)

	if err != nil {
		// failed to wait for Op to finish, however, the Op possibly is still running as expected
		// the error code returned should be non-final
		if IsGCEError(err, "alreadyExists") {
			disk, err := cloud.GetDisk(ctx, project, volKey, gceAPIVersion)
			if err != nil {
				return common.NewTemporaryError(codes.Unavailable, fmt.Errorf("error when getting disk: %w", err))
			}
			err = cloud.ValidateExistingDisk(ctx, disk, params,
				int64(capacityRange.GetRequiredBytes()),
				int64(capacityRange.GetLimitBytes()),
				multiWriter)
			if err != nil {
				return err
			}
			klog.Warningf("GCE PD %s already exists after wait, reusing", volKey.Name)
			return nil
		}
		return common.NewTemporaryError(codes.Unavailable, fmt.Errorf("unknown error when polling the operation: %w", err))
	}
	return nil
}

func (cloud *CloudProvider) DeleteDisk(ctx context.Context, project string, volKey *meta.Key) error {
	klog.V(5).Infof("Deleting disk: %v", volKey)
	switch volKey.Type() {
	case meta.Zonal:
		return cloud.deleteZonalDisk(ctx, project, volKey.Zone, volKey.Name)
	case meta.Regional:
		return cloud.deleteRegionalDisk(ctx, project, volKey.Region, volKey.Name)
	default:
		return fmt.Errorf("could not delete disk, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *CloudProvider) deleteZonalDisk(ctx context.Context, project, zone, name string) error {
	op, err := cloud.service.Disks.Delete(project, zone, name).Context(ctx).Do()
	if err != nil {
		if IsGCEError(err, "notFound") {
			// Already deleted
			return nil
		}
		return err
	}
	klog.V(5).Infof("DeleteDisk operation %s for disk %s", op.Name, name)

	err = cloud.waitForZonalOp(ctx, project, op.Name, zone)
	if err != nil {
		return err
	}
	return nil
}

func (cloud *CloudProvider) deleteRegionalDisk(ctx context.Context, project, region, name string) error {
	op, err := cloud.service.RegionDisks.Delete(project, region, name).Context(ctx).Do()
	if err != nil {
		if IsGCEError(err, "notFound") {
			// Already deleted
			return nil
		}
		return err
	}
	klog.V(5).Infof("DeleteDisk operation %s for disk %s", op.Name, name)

	err = cloud.waitForRegionalOp(ctx, project, op.Name, region)
	if err != nil {
		return err
	}
	return nil
}

func (cloud *CloudProvider) AttachDisk(ctx context.Context, project string, volKey *meta.Key, readWrite, diskType, instanceZone, instanceName string, forceAttach bool) error {
	klog.V(5).Infof("Attaching disk %v to %s", volKey, instanceName)
	source := cloud.GetDiskSourceURI(project, volKey)

	deviceName, err := common.GetDeviceName(volKey)
	if err != nil {
		return fmt.Errorf("failed to get device name: %w", err)
	}
	attachedDiskV1 := &computev1.AttachedDisk{
		DeviceName: deviceName,
		Kind:       diskKind,
		Mode:       readWrite,
		Source:     source,
		Type:       diskType,
		// This parameter is ignored in the call, the ForceAttach decorator
		// (query parameter) is the important one. We'll set it in both places
		// in case that behavior changes.
		ForceAttach: forceAttach,
	}

	op, err := cloud.service.Instances.AttachDisk(project, instanceZone, instanceName, attachedDiskV1).Context(ctx).ForceAttach(forceAttach).Do()
	if err != nil {
		return fmt.Errorf("failed cloud service attach disk call: %w", err)
	}
	klog.V(5).Infof("AttachDisk operation %s for disk %s", op.Name, attachedDiskV1.DeviceName)

	err = cloud.waitForZonalOp(ctx, project, op.Name, instanceZone)
	if err != nil {
		return fmt.Errorf("failed when waiting for zonal op: %w", err)
	}
	return nil
}

func (cloud *CloudProvider) DetachDisk(ctx context.Context, project, deviceName, instanceZone, instanceName string) error {
	klog.V(5).Infof("Detaching disk %v from %v", deviceName, instanceName)
	op, err := cloud.service.Instances.DetachDisk(project, instanceZone, instanceName, deviceName).Context(ctx).Do()
	if err != nil {
		return err
	}
	klog.V(5).Infof("DetachDisk operation %s for disk %s", op.Name, deviceName)

	err = cloud.waitForZonalOp(ctx, project, op.Name, instanceZone)
	if err != nil {
		return err
	}
	return nil
}

func (cloud *CloudProvider) SetDiskAccessMode(ctx context.Context, project string, volKey *meta.Key, accessMode string) error {
	diskMask := &computev1.Disk{
		AccessMode: accessMode,
		Name:       volKey.Name,
	}
	switch volKey.Type() {
	case meta.Zonal:
		op, err := cloud.service.Disks.Update(project, volKey.Zone, volKey.Name, diskMask).Context(ctx).Paths("accessMode").Do()
		if err != nil {
			return fmt.Errorf("failed to set access mode for zonal volume %v: %w", volKey, err)
		}
		klog.V(5).Infof("SetDiskAccessMode operation %s for disk %s", op.Name, volKey.Name)

		err = cloud.waitForZonalOp(ctx, project, op.Name, volKey.Zone)
		if err != nil {
			return fmt.Errorf("failed waiting for op for zonal disk update for %v: %w", volKey, err)
		}
	case meta.Regional:
		op, err := cloud.service.RegionDisks.Update(project, volKey.Region, volKey.Name, diskMask).Context(ctx).Paths("accessMode").Do()
		if err != nil {
			return fmt.Errorf("failed to set access mode for regional volume %v: %w", volKey, err)
		}
		klog.V(5).Infof("SetDiskAccessMode operation %s for disk %s", op.Name, volKey.Name)

		err = cloud.waitForRegionalOp(ctx, project, op.Name, volKey.Region)
		if err != nil {
			return fmt.Errorf("failed waiting for op for regional disk update for %v: %w", volKey, err)
		}
	default:
		return fmt.Errorf("volume key %v not zonal nor regional", volKey.Name)
	}

	return nil
}

func (cloud *CloudProvider) ListCompatibleDiskTypeZones(ctx context.Context, project string, zones []string, diskType string) ([]string, error) {
	diskTypeFilter := fmt.Sprintf("name=%s", diskType)
	filters := []string{diskTypeFilter}
	diskTypeListCall := cloud.service.DiskTypes.AggregatedList(project).Context(ctx).Filter(strings.Join(filters, " "))

	supportedZones := []string{}
	nextPageToken := "pageToken"
	for nextPageToken != "" {
		diskTypeList, err := diskTypeListCall.Do()
		if err != nil {
			return nil, err
		}
		for _, item := range diskTypeList.Items {
			for _, diskType := range item.DiskTypes {
				zone, err := common.ParseZoneFromURI(diskType.Zone)
				if err != nil {
					klog.Warningf("Failed to parse zone %q from diskTypes API: %v", diskType.Zone, err)
					continue
				}
				if slices.Contains(zones, zone) {
					supportedZones = append(supportedZones, zone)
				}
			}
		}

		nextPageToken = diskTypeList.NextPageToken
		diskTypeListCall.PageToken(nextPageToken)
	}

	return supportedZones, nil
}

func (cloud *CloudProvider) GetDiskSourceURI(project string, volKey *meta.Key) string {
	switch volKey.Type() {
	case meta.Zonal:
		return cloud.getZonalDiskSourceURI(project, volKey.Name, volKey.Zone)
	case meta.Regional:
		return cloud.getRegionalDiskSourceURI(project, volKey.Name, volKey.Region)
	default:
		return ""
	}
}

func (cloud *CloudProvider) getZonalDiskSourceURI(project, diskName, zone string) string {
	return cloud.service.BasePath + fmt.Sprintf(
		diskSourceURITemplateSingleZone,
		project,
		zone,
		diskName)
}

func (cloud *CloudProvider) getRegionalDiskSourceURI(project, diskName, region string) string {
	return cloud.service.BasePath + fmt.Sprintf(
		diskSourceURITemplateRegional,
		project,
		region,
		diskName)
}

func (cloud *CloudProvider) GetDiskTypeURI(project string, volKey *meta.Key, diskType string) string {
	switch volKey.Type() {
	case meta.Zonal:
		return cloud.getZonalDiskTypeURI(project, volKey.Zone, diskType)
	case meta.Regional:
		return cloud.getRegionalDiskTypeURI(project, volKey.Region, diskType)
	default:
		return fmt.Sprintf("could get disk type URI, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *CloudProvider) getZonalDiskTypeURI(project string, zone, diskType string) string {
	return cloud.service.BasePath + fmt.Sprintf(diskTypeURITemplateSingleZone, project, zone, diskType)
}

func (cloud *CloudProvider) getRegionalDiskTypeURI(project string, region, diskType string) string {
	return cloud.service.BasePath + fmt.Sprintf(diskTypeURITemplateRegional, project, region, diskType)
}

func (cloud *CloudProvider) waitForZonalOp(ctx context.Context, project, opName string, zone string) error {
	// The v1 API can query for v1, alpha, or beta operations.
	return wait.ExponentialBackoff(WaitForOpBackoff, func() (bool, error) {
		pollOp, err := cloud.service.ZoneOperations.Get(project, zone, opName).Context(ctx).Do()
		if err != nil {
			klog.Errorf("WaitForOp(op: %s, zone: %#v) failed to poll the operation", opName, zone)
			return false, err
		}
		done, err := opIsDone(pollOp)
		return done, err
	})
}

func (cloud *CloudProvider) waitForRegionalOp(ctx context.Context, project, opName string, region string) error {
	// The v1 API can query for v1, alpha, or beta operations.
	return wait.ExponentialBackoff(WaitForOpBackoff, func() (bool, error) {
		pollOp, err := cloud.service.RegionOperations.Get(project, region, opName).Context(ctx).Do()
		if err != nil {
			klog.Errorf("WaitForOp(op: %s, region: %#v) failed to poll the operation", opName, region)
			return false, err
		}
		done, err := opIsDone(pollOp)
		return done, err
	})
}

func (cloud *CloudProvider) waitForGlobalOp(ctx context.Context, project, opName string) error {
	return wait.ExponentialBackoff(WaitForOpBackoff, func() (bool, error) {
		pollOp, err := cloud.service.GlobalOperations.Get(project, opName).Context(ctx).Do()
		if err != nil {
			klog.Errorf("waitForGlobalOp(op: %s) failed to poll the operation", opName)
			return false, err
		}
		done, err := opIsDone(pollOp)
		return done, err
	})
}

func (cloud *CloudProvider) waitForAttachOnInstance(ctx context.Context, project string, volKey *meta.Key, instanceZone, instanceName string) error {
	klog.V(5).Infof("Waiting for attach of disk %v to instance %v to complete...", volKey.Name, instanceName)
	start := time.Now()
	return wait.ExponentialBackoff(AttachDiskBackoff, func() (bool, error) {
		klog.V(6).Infof("Polling instances.get for attach of disk %v to instance %v to complete for %v", volKey.Name, instanceName, time.Since(start))
		instance, err := cloud.GetInstanceOrError(ctx, instanceZone, instanceName)
		if err != nil {
			return false, fmt.Errorf("GetInstance failed to get instance: %w", err)
		}

		if instance == nil {
			return false, fmt.Errorf("instance %v could not be found", instanceName)
		}

		for _, disk := range instance.Disks {
			deviceName, err := common.GetDeviceName(volKey)
			if err != nil {
				return false, fmt.Errorf("failed to get disk device name for %s: %w", volKey, err)
			}

			if deviceName == disk.DeviceName {
				return true, nil
			}
		}
		return false, nil
	})
}

func (cloud *CloudProvider) waitForAttachOnDisk(ctx context.Context, project string, volKey *meta.Key, instanceZone, instanceName string) error {
	klog.V(5).Infof("Waiting for attach of disk %v to instance %v to complete...", volKey.Name, instanceName)
	start := time.Now()
	return wait.ExponentialBackoff(AttachDiskBackoff, func() (bool, error) {
		klog.V(6).Infof("Polling disks.get for attach of disk %v to instance %v to complete for %v", volKey.Name, instanceName, time.Since(start))
		disk, err := cloud.GetDisk(ctx, project, volKey, GCEAPIVersionV1)
		if err != nil {
			return false, fmt.Errorf("GetDisk failed to get disk: %w", err)
		}

		if disk == nil {
			return false, fmt.Errorf("disk %v could not be found", volKey.Name)
		}

		for _, user := range disk.GetUsers() {
			if strings.Contains(user, instanceName) && strings.Contains(user, instanceZone) {
				return true, nil
			}
		}
		return false, nil
	})
}

func (cloud *CloudProvider) WaitForAttach(ctx context.Context, project string, volKey *meta.Key, diskType, instanceZone, instanceName string) error {
	if cloud.waitForAttachConfig.ShouldUseGetInstanceAPI(diskType) {
		return cloud.waitForAttachOnInstance(ctx, project, volKey, instanceZone, instanceName)
	} else {
		return cloud.waitForAttachOnDisk(ctx, project, volKey, instanceZone, instanceName)
	}
}

func wrapOpErr(name string, opErr *computev1.OperationErrorErrors) error {
	if opErr == nil {
		return nil
	}

	if opErr.Code == "UNSUPPORTED_OPERATION" {
		if diskType := pdDiskTypeUnsupportedRegex.FindStringSubmatch(opErr.Message); diskType != nil {
			return &UnsupportedDiskError{
				DiskType: diskType[1],
			}
		}
	}
	grpcErrCode := codeForGCEOpError(*opErr)
	return status.Errorf(grpcErrCode, "operation %v failed (%v): %v", name, opErr.Code, opErr.Message)
}

// codeForGCEOpError return the grpc error code for the passed in
// gce operation error. All of these error codes are filtered out from our SLO,
// but will be monitored by the stockout reporting dashboard.
func codeForGCEOpError(err computev1.OperationErrorErrors) codes.Code {
	userErrors := map[string]codes.Code{
		"RESOURCE_NOT_FOUND":                        codes.NotFound,
		"RESOURCE_ALREADY_EXISTS":                   codes.AlreadyExists,
		"RESOURCE_IN_USE_BY_ANOTHER_RESOURCE":       codes.InvalidArgument,
		"OPERATION_CANCELED_BY_USER":                codes.Aborted,
		"QUOTA_EXCEEDED":                            codes.ResourceExhausted,
		"ZONE_RESOURCE_POOL_EXHAUSTED":              codes.Unavailable,
		"ZONE_RESOURCE_POOL_EXHAUSTED_WITH_DETAILS": codes.Unavailable,
		"REGION_QUOTA_EXCEEDED":                     codes.ResourceExhausted,
		"RATE_LIMIT_EXCEEDED":                       codes.ResourceExhausted,
		"INVALID_USAGE":                             codes.InvalidArgument,
		"UNSUPPORTED_OPERATION":                     codes.InvalidArgument,
	}
	if code, ok := userErrors[err.Code]; ok {
		return code
	}
	return codes.Internal
}

func opIsDone(op *computev1.Operation) (bool, error) {
	if op == nil || op.Status != operationStatusDone {
		return false, nil
	}
	if op.Error != nil && len(op.Error.Errors) > 0 && op.Error.Errors[0] != nil {
		return true, wrapOpErr(op.Name, op.Error.Errors[0])
	}
	return true, nil
}

func (cloud *CloudProvider) GetInstanceOrError(ctx context.Context, instanceZone, instanceName string) (*computev1.Instance, error) {
	klog.V(5).Infof("Getting instance %v from zone %v", instanceName, instanceZone)
	svc := cloud.service
	project := cloud.project
	instance, err := svc.Instances.Get(project, instanceZone, instanceName).Do()
	if err != nil {
		return nil, err
	}
	return instance, nil
}

func (cloud *CloudProvider) GetSnapshot(ctx context.Context, project, snapshotName string) (*computev1.Snapshot, error) {
	klog.V(5).Infof("Getting snapshot %v", snapshotName)
	svc := cloud.service
	snapshot, err := svc.Snapshots.Get(project, snapshotName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (cloud *CloudProvider) DeleteSnapshot(ctx context.Context, project, snapshotName string) error {
	klog.V(5).Infof("Deleting snapshot %v", snapshotName)
	op, err := cloud.service.Snapshots.Delete(project, snapshotName).Context(ctx).Do()
	if err != nil {
		if IsGCEError(err, "notFound") {
			// Already deleted
			return nil
		}
		return err
	}
	err = cloud.waitForGlobalOp(ctx, project, op.Name)
	if err != nil {
		return err
	}
	return nil
}

func (cloud *CloudProvider) CreateSnapshot(ctx context.Context, project string, volKey *meta.Key, snapshotName string, snapshotParams common.SnapshotParameters) (*computev1.Snapshot, error) {
	klog.V(5).Infof("Creating snapshot %s for volume %v", snapshotName, volKey)

	description, err := encodeTags(snapshotParams.Tags)
	if err != nil {
		return nil, err
	}

	switch volKey.Type() {
	case meta.Zonal:
		if description == "" {
			description = "Snapshot created by GCE-PD CSI Driver"
		}
		return cloud.createZonalDiskSnapshot(ctx, project, volKey, snapshotName, snapshotParams, description)
	case meta.Regional:
		if description == "" {
			description = "Regional Snapshot created by GCE-PD CSI Driver"
		}
		return cloud.createRegionalDiskSnapshot(ctx, project, volKey, snapshotName, snapshotParams, description)
	default:
		return nil, fmt.Errorf("could not create snapshot, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *CloudProvider) CreateImage(ctx context.Context, project string, volKey *meta.Key, imageName string, snapshotParams common.SnapshotParameters) (*computev1.Image, error) {
	klog.V(5).Infof("Creating image %s for source %v", imageName, volKey)

	description, err := encodeTags(snapshotParams.Tags)
	if err != nil {
		return nil, err
	}

	if description == "" {
		description = "Image created by GCE-PD CSI Driver"
	}

	diskID, err := common.KeyToVolumeID(volKey, project)
	if err != nil {
		return nil, err
	}
	image := &computev1.Image{
		SourceDisk:       diskID,
		Family:           snapshotParams.ImageFamily,
		Name:             imageName,
		StorageLocations: snapshotParams.StorageLocations,
		Description:      description,
		Labels:           snapshotParams.Labels,
	}

	_, err = cloud.service.Images.Insert(project, image).Context(ctx).ForceCreate(true).Do()
	if err != nil {
		return nil, err
	}

	newImage, err := cloud.waitForImageCreation(ctx, project, imageName)

	if err == nil {
		err = cloud.attachTagsToResource(ctx, snapshotParams.ResourceTags, project, newImage.Id, imagesType, "", false, resourceManagerHostSubPath)
	}

	return newImage, err
}

func (cloud *CloudProvider) waitForImageCreation(ctx context.Context, project, imageName string) (*computev1.Image, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	timer := time.NewTimer(waitForImageCreationTimeOut)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			klog.V(6).Infof("Checking GCE Image %s.", imageName)
			image, err := cloud.GetImage(ctx, project, imageName)
			if err != nil {
				klog.Warningf("Error in getting image %s, %v", imageName, err.Error())
			} else if image != nil {
				if image.Status != "PENDING" {
					klog.V(6).Infof("Image %s status is %s", imageName, image.Status)
					return image, nil
				} else {
					klog.V(6).Infof("Image %s is still pending", imageName)
				}
			}
		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for image %s to be created", imageName)
		}
	}
}

func (cloud *CloudProvider) GetImage(ctx context.Context, project, imageName string) (*computev1.Image, error) {
	klog.V(5).Infof("Getting image %v", imageName)
	image, err := cloud.service.Images.Get(project, imageName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return image, nil
}

func (cloud *CloudProvider) ListImages(ctx context.Context, filter string) ([]*computev1.Image, string, error) {
	klog.V(5).Infof("Listing images with filter: %s", filter)
	var items []*computev1.Image
	lCall := cloud.service.Images.List(cloud.project).Context(ctx).Filter(filter)
	nextPageToken := "pageToken"
	for nextPageToken != "" {
		imageList, err := lCall.Do()
		if err != nil {
			return nil, "", err
		}
		items = append(items, imageList.Items...)
	}
	return items, "", nil
}

func (cloud *CloudProvider) DeleteImage(ctx context.Context, project, imageName string) error {
	klog.V(5).Infof("Deleting image %v", imageName)
	op, err := cloud.service.Images.Delete(cloud.project, imageName).Context(ctx).Do()
	if err != nil {
		if IsGCEError(err, "notFound") {
			return nil
		}
		return err
	}
	err = cloud.waitForGlobalOp(ctx, project, op.Name)
	if err != nil {
		return err
	}
	return nil
}

// ResizeDisk takes in the requested disk size in bytes and returns the resized
// size in Gi
// TODO(#461) The whole driver could benefit from standardized usage of the
// k8s.io/apimachinery/quantity package for better size handling
func (cloud *CloudProvider) ResizeDisk(ctx context.Context, project string, volKey *meta.Key, requestBytes int64) (int64, error) {
	klog.V(5).Infof("Resizing disk %v to size %v", volKey, requestBytes)
	cloudDisk, err := cloud.GetDisk(ctx, project, volKey, GCEAPIVersionV1)
	if err != nil {
		return -1, fmt.Errorf("failed to get disk: %w", err)
	}

	sizeGb := cloudDisk.GetSizeGb()
	requestGb := common.BytesToGbRoundUp(requestBytes)

	// If disk is already of size equal or greater than requested size, we
	// simply return the found size
	if sizeGb >= requestGb {
		return sizeGb, nil
	}

	switch volKey.Type() {
	case meta.Zonal:
		return cloud.resizeZonalDisk(ctx, project, volKey, requestGb)
	case meta.Regional:
		return cloud.resizeRegionalDisk(ctx, project, volKey, requestGb)
	default:
		return -1, fmt.Errorf("could not resize disk, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *CloudProvider) resizeZonalDisk(ctx context.Context, project string, volKey *meta.Key, requestGb int64) (int64, error) {
	resizeReq := &computev1.DisksResizeRequest{
		SizeGb: requestGb,
	}
	op, err := cloud.service.Disks.Resize(project, volKey.Zone, volKey.Name, resizeReq).Context(ctx).Do()
	if err != nil {
		return -1, fmt.Errorf("failed to resize zonal volume %v: %w", volKey.String(), err)
	}
	klog.V(5).Infof("ResizeDisk operation %s for disk %s", op.Name, volKey.Name)

	err = cloud.waitForZonalOp(ctx, project, op.Name, volKey.Zone)
	if err != nil {
		return -1, fmt.Errorf("failed waiting for op for zonal resize for %s: %w", volKey.String(), err)
	}

	return requestGb, nil
}

func (cloud *CloudProvider) resizeRegionalDisk(ctx context.Context, project string, volKey *meta.Key, requestGb int64) (int64, error) {
	resizeReq := &computev1.RegionDisksResizeRequest{
		SizeGb: requestGb,
	}

	op, err := cloud.service.RegionDisks.Resize(project, volKey.Region, volKey.Name, resizeReq).Context(ctx).Do()
	if err != nil {
		return -1, fmt.Errorf("failed to resize regional volume %v: %w", volKey.String(), err)
	}
	klog.V(5).Infof("ResizeDisk operation %s for disk %s", op.Name, volKey.Name)

	err = cloud.waitForRegionalOp(ctx, project, op.Name, volKey.Region)
	if err != nil {
		return -1, fmt.Errorf("failed waiting for op for regional resize for %s: %w", volKey.String(), err)
	}

	return requestGb, nil
}

func (cloud *CloudProvider) createZonalDiskSnapshot(ctx context.Context, project string, volKey *meta.Key, snapshotName string, snapshotParams common.SnapshotParameters, description string) (*computev1.Snapshot, error) {
	snapshotToCreate := &computev1.Snapshot{
		Name:             snapshotName,
		StorageLocations: snapshotParams.StorageLocations,
		Description:      description,
		Labels:           snapshotParams.Labels,
	}

	_, err := cloud.service.Disks.CreateSnapshot(project, volKey.Zone, volKey.Name, snapshotToCreate).Context(ctx).Do()

	if err != nil {
		return nil, err
	}

	snapshot, err := cloud.waitForSnapshotCreation(ctx, project, snapshotName)

	if err == nil {
		err = cloud.attachTagsToResource(ctx, snapshotParams.ResourceTags, project, snapshot.Id, snapshotsType, "", false, resourceManagerHostSubPath)
	}

	return snapshot, err
}

func (cloud *CloudProvider) createRegionalDiskSnapshot(ctx context.Context, project string, volKey *meta.Key, snapshotName string, snapshotParams common.SnapshotParameters, description string) (*computev1.Snapshot, error) {
	snapshotToCreate := &computev1.Snapshot{
		Name:             snapshotName,
		StorageLocations: snapshotParams.StorageLocations,
		Description:      description,
		Labels:           snapshotParams.Labels,
	}

	_, err := cloud.service.RegionDisks.CreateSnapshot(project, volKey.Region, volKey.Name, snapshotToCreate).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	snapshot, err := cloud.waitForSnapshotCreation(ctx, project, snapshotName)

	if err == nil {
		err = cloud.attachTagsToResource(ctx, snapshotParams.ResourceTags, project, snapshot.Id, snapshotsType, "", false, resourceManagerHostSubPath)
	}

	return snapshot, err

}

func (cloud *CloudProvider) waitForSnapshotCreation(ctx context.Context, project, snapshotName string) (*computev1.Snapshot, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	timer := time.NewTimer(waitForSnapshotCreationTimeOut)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			klog.V(6).Infof("Checking GCE Snapshot %s.", snapshotName)
			snapshot, err := cloud.GetSnapshot(ctx, project, snapshotName)
			if err != nil {
				klog.Warningf("Error in getting snapshot %s, %v", snapshotName, err.Error())
			} else if snapshot != nil {
				if snapshot.Status != "CREATING" {
					klog.V(6).Infof("Snapshot %s status is %s", snapshotName, snapshot.Status)
					return snapshot, nil
				} else {
					klog.V(6).Infof("Snapshot %s is still creating ...", snapshotName)
				}
			}
		case <-timer.C:
			return nil, fmt.Errorf("Timeout waiting for snapshot %s to be created.", snapshotName)
		}
	}
}

// getResourceManagerTags returns the map of tag keys and values. The tag keys are in the form `tagKeys/{tag_key_id}`
// and the tag values are in the format `tagValues/456`.
func getResourceManagerTags(ctx context.Context, tokenSource oauth2.TokenSource, tagsMap map[string]string) (map[string]string, error) {
	if len(tagsMap) <= 0 {
		return nil, nil
	}

	tagValuesClient, err := createTagValuesClient(ctx, tokenSource, resourceManagerHostSubPath)
	if err != nil {
		return nil, err
	}
	defer tagValuesClient.Close()

	tagKeyValueMap := make(map[string]string, len(tagsMap))
	for tagParentIDKey, tagValue := range tagsMap {
		getTagValuesReq := &rscmgrpb.GetNamespacedTagValueRequest{
			Name: fmt.Sprintf("%s/%s", tagParentIDKey, tagValue),
		}
		value, err := tagValuesClient.GetNamespacedTagValue(ctx, getTagValuesReq, getRetryCallOptions()...)
		if err != nil {
			return nil, err
		}
		tagKeyValueMap[value.Parent] = value.Name
	}

	return tagKeyValueMap, nil
}

// getRetryCallOptions returns a list of additional call options. If the
// call encounters TooManyRequests error then it will be retried with an
// exponential backoff.
func getRetryCallOptions() []gax.CallOption {
	return []gax.CallOption{
		gax.WithRetry(func() gax.Retryer {
			return gax.OnHTTPCodes(gax.Backoff{
				Initial:    90 * time.Second,
				Max:        5 * time.Minute,
				Multiplier: 2,
			},
				http.StatusTooManyRequests)
		}),
	}
}

// getFilteredTagsMap returns the map of tag keys and the tag values to apply on the resources after
// filtering the tags already existing on a given resource.
func getFilteredTagsMap(ctx context.Context, client *rscmgr.TagBindingsClient, parent string, tagsMap map[string]string) map[string]string {
	if len(tagsMap) <= 0 {
		klog.Infof("getFilteredTagsMap: tags map is empty for %s compute", parent)
		return nil
	}

	filteredTagsMap := make(map[string]string, len(tagsMap))
	for key, value := range tagsMap {
		filteredTagsMap[key] = value
	}

	listBindingsReq := &rscmgrpb.ListEffectiveTagsRequest{
		Parent: parent,
	}
	bindings := client.ListEffectiveTags(ctx, listBindingsReq)
	for i := 0; i < 50; i++ {
		binding, err := bindings.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil || binding == nil {
			klog.Errorf("failed to list effective tags on %s compute resource: %v: %v", parent, binding, err)
			break
		}
		namespacedTagKey := binding.GetNamespacedTagKey()
		if _, exists := filteredTagsMap[namespacedTagKey]; exists {
			delete(filteredTagsMap, namespacedTagKey)
			klog.V(1).Infof("getFilteredTagsMap: skipping tag %s already exists on the %s compute resource", namespacedTagKey, parent)
		}
	}

	return filteredTagsMap
}

// attachTagsToResource attaches tags to a compute resource. As GCP has a rate limit of 600
// requests per minute, the tags list is filtered so that only the tags which are not attached
// to the resource are attached. The calls to create the tag binding resource for the tags are
// rate limited to 8 requests per second.
func (cloud *CloudProvider) attachTagsToResource(
	ctx context.Context,
	tagsMap map[string]string,
	project string,
	resourceID uint64,
	resourceType ResourceType,
	location string,
	isZonal bool,
	resourceManagerHostSubPath string) error {
	if len(tagsMap) <= 0 {
		return nil
	}

	tagBindingsClient, err := createTagBindingsClient(ctx, cloud.tokenSource, location, resourceManagerHostSubPath)
	if err != nil || tagBindingsClient == nil {
		return fmt.Errorf("failed to create tag binding client for adding tags to %d compute %s: %w", resourceID, resourceType, err)
	}
	defer tagBindingsClient.Close()

	var fullResourceID string
	if location != "" {
		if isZonal {
			fullResourceID = fmt.Sprintf(zonalOrRegionalComputeParentPathFmt, project, "zones", location, resourceType, resourceID)
		} else {
			fullResourceID = fmt.Sprintf(zonalOrRegionalComputeParentPathFmt, project, "regions", location, resourceType, resourceID)
		}
	} else {
		fullResourceID = fmt.Sprintf(globalComputeParentPathFmt, project, resourceType, resourceID)
	}

	filteredTagsMap := getFilteredTagsMap(ctx, tagBindingsClient, fullResourceID, tagsMap)

	errFlag := false
	for tagParentIDKey, tagValue := range filteredTagsMap {
		if err := cloud.tagsRateLimiter.Wait(ctx); err != nil {
			errFlag = true
			klog.Errorf("rate limiting request to add %s tag to %d compute %s failed: %v", tagValue, resourceID, resourceType, err)
			continue
		}

		tagBindingReq := &rscmgrpb.CreateTagBindingRequest{
			TagBinding: &rscmgrpb.TagBinding{
				Parent:                 fullResourceID,
				TagValueNamespacedName: fmt.Sprintf("%s/%s", tagParentIDKey, tagValue),
			},
		}

		result, err := tagBindingsClient.CreateTagBinding(ctx, tagBindingReq, getRetryCallOptions()...)
		if err != nil {
			e, ok := err.(*apierror.APIError)
			if ok && e.HTTPCode() == http.StatusConflict {
				klog.Infof("tag binding %d: %s/%s already exists", resourceID, tagParentIDKey, tagValue)
				continue
			}
			errFlag = true
			klog.Errorf("request to add %s/%s tag to %d compute %s failed: %v", tagParentIDKey, tagValue, resourceID, resourceType, err)
			continue
		}

		if _, err = result.Wait(ctx); err != nil {
			errFlag = true
			klog.Errorf("failed to add %s/%s tag to %d compute %s: %v", tagParentIDKey, tagValue, resourceID, resourceType, err)
		}
	}
	if errFlag {
		return fmt.Errorf("failed to add tags to %d compute %s", resourceID, resourceType)
	}
	return nil
}

// kmsKeyEqual returns true if fetchedKMSKey and storageClassKMSKey refer to the same key.
// fetchedKMSKey - key returned by the server
//
//	example: projects/{0}/locations/{1}/keyRings/{2}/cryptoKeys/{3}/cryptoKeyVersions/{4}
//
// storageClassKMSKey - key as provided by the client
//
//	example: projects/{0}/locations/{1}/keyRings/{2}/cryptoKeys/{3}
//
// cryptoKeyVersions should be disregarded if the rest of the key is identical.
func KmsKeyEqual(fetchedKMSKey, storageClassKMSKey string) bool {
	return removeCryptoKeyVersion(fetchedKMSKey) == removeCryptoKeyVersion(storageClassKMSKey)
}

func removeCryptoKeyVersion(kmsKey string) string {
	i := strings.LastIndex(kmsKey, cryptoKeyVerDelimiter)
	if i > 0 {
		return kmsKey[:i]
	}
	return kmsKey
}

// encodeTags encodes requested volume or snapshot tags into JSON string, suitable for putting into
// the PD or Snapshot Description field.
func encodeTags(tags map[string]string) (string, error) {
	if len(tags) == 0 {
		// No tags -> empty JSON
		return "", nil
	}

	enc, err := json.Marshal(tags)
	if err != nil {
		return "", fmt.Errorf("failed to encodeTags %v: %w", tags, err)
	}
	return string(enc), nil
}

func containsBetaDiskType(betaDiskTypes []string, diskType string) bool {
	for _, betaDiskType := range betaDiskTypes {
		if betaDiskType == diskType {
			return true
		}
	}

	return false
}
