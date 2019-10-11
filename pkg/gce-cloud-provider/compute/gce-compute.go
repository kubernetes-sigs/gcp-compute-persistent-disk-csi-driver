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
	"fmt"
	"strings"
	"time"

	"context"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	csi "github.com/container-storage-interface/spec/lib/go/csi"
	computealpha "google.golang.org/api/compute/v0.alpha"
	computev1 "google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

const (
	operationStatusDone            = "DONE"
	waitForSnapshotCreationTimeOut = 2 * time.Minute
	diskKind                       = "compute#disk"
)

type GCECompute interface {
	// Disk Methods
	GetDisk(ctx context.Context, volumeKey *meta.Key) (*CloudDisk, error)
	RepairUnderspecifiedVolumeKey(ctx context.Context, volumeKey *meta.Key) (*meta.Key, error)
	ValidateExistingDisk(ctx context.Context, disk *CloudDisk, diskType string, reqBytes, limBytes int64) error
	InsertDisk(ctx context.Context, volKey *meta.Key, diskType string, capBytes int64, capacityRange *csi.CapacityRange, replicaZones []string, snapshotID, diskEncryptionKmsKey string, multiWriter bool) error
	DeleteDisk(ctx context.Context, volumeKey *meta.Key) error
	AttachDisk(ctx context.Context, volKey *meta.Key, readWrite, diskType, instanceZone, instanceName string) error
	DetachDisk(ctx context.Context, deviceName string, instanceZone, instanceName string) error
	GetDiskSourceURI(volKey *meta.Key) string
	GetDiskTypeURI(volKey *meta.Key, diskType string) string
	WaitForAttach(ctx context.Context, volKey *meta.Key, instanceZone, instanceName string) error
	ResizeDisk(ctx context.Context, volKey *meta.Key, requestBytes int64) (int64, error)
	// Regional Disk Methods
	GetReplicaZoneURI(zone string) string
	// Instance Methods
	GetInstanceOrError(ctx context.Context, instanceZone, instanceName string) (*computev1.Instance, error)
	// Zone Methods
	ListZones(ctx context.Context, region string) ([]string, error)
	ListSnapshots(ctx context.Context, filter string, maxEntries int64, pageToken string) ([]*computev1.Snapshot, string, error)
	GetSnapshot(ctx context.Context, snapshotName string) (*computev1.Snapshot, error)
	CreateSnapshot(ctx context.Context, volKey *meta.Key, snapshotName string) (*computev1.Snapshot, error)
	DeleteSnapshot(ctx context.Context, snapshotName string) error
}

// RepairUnderspecifiedVolumeKey will query the cloud provider and check each zone for the disk specified
// by the volume key and return a volume key with a correct zone
func (cloud *CloudProvider) RepairUnderspecifiedVolumeKey(ctx context.Context, volumeKey *meta.Key) (*meta.Key, error) {
	klog.V(5).Infof("Repairing potentially underspecified volume key %v", volumeKey)
	region, err := common.GetRegionFromZones([]string{cloud.zone})
	if err != nil {
		return nil, fmt.Errorf("failed to get region from zones: %v", err)
	}
	switch volumeKey.Type() {
	case meta.Zonal:
		foundZone := ""
		if volumeKey.Zone == common.UnspecifiedValue {
			// list all zones, try to get disk in each zone
			zones, err := cloud.ListZones(ctx, region)
			if err != nil {
				return nil, err
			}
			for _, zone := range zones {
				_, err := cloud.getZonalDiskOrError(ctx, zone, volumeKey.Name)
				if err != nil {
					if IsGCENotFoundError(err) {
						// Couldn't find the disk in this zone so we keep
						// looking
						continue
					}
					// There is some miscellaneous error getting disk from zone
					// so we return error immediately
					return nil, err
				}
				if len(foundZone) > 0 {
					return nil, fmt.Errorf("found disk %s in more than one zone: %s and %s", volumeKey.Name, foundZone, zone)
				}
				foundZone = zone
			}

			if len(foundZone) == 0 {
				return nil, notFoundError()
			}
			volumeKey.Zone = foundZone
			return volumeKey, nil
		}
		return volumeKey, nil
	case meta.Regional:
		if volumeKey.Region == common.UnspecifiedValue {
			volumeKey.Region = region
		}
		return volumeKey, nil
	default:
		return nil, fmt.Errorf("key was neither zonal nor regional, got: %v", volumeKey.String())
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
		return nil, fmt.Errorf("failed to list zones in region %s: %v", region, err)
	}
	for _, zone := range zoneList.Items {
		zones = append(zones, zone.Name)
	}
	cloud.zonesCache[region] = zones
	return zones, nil

}

func (cloud *CloudProvider) ListSnapshots(ctx context.Context, filter string, maxEntries int64, pageToken string) ([]*computev1.Snapshot, string, error) {
	klog.V(5).Infof("Listing snapshots with filter: %s, max entries: %v, page token: %s", filter, maxEntries, pageToken)
	snapshots := []*computev1.Snapshot{}
	snapshotList, err := cloud.service.Snapshots.List(cloud.project).Filter(filter).MaxResults(maxEntries).PageToken(pageToken).Do()
	if err != nil {
		return snapshots, "", err
	}
	for _, snapshot := range snapshotList.Items {
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, snapshotList.NextPageToken, nil

}

func (cloud *CloudProvider) GetDisk(ctx context.Context, key *meta.Key) (*CloudDisk, error) {
	klog.V(5).Infof("Getting disk %v", key)
	switch key.Type() {
	case meta.Zonal:
		disk, err := cloud.getZonalDiskOrError(ctx, key.Zone, key.Name)
		return ZonalCloudDisk(disk), err
	case meta.Regional:
		disk, err := cloud.getRegionalDiskOrError(ctx, key.Region, key.Name)
		return RegionalCloudDisk(disk), err
	default:
		return nil, fmt.Errorf("key was neither zonal nor regional, got: %v", key.String())
	}
}

func (cloud *CloudProvider) getZonalDiskOrError(ctx context.Context, volumeZone, volumeName string) (*computev1.Disk, error) {
	svc := cloud.service
	project := cloud.project

	disk, err := svc.Disks.Get(project, volumeZone, volumeName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return disk, nil
}

func (cloud *CloudProvider) getRegionalDiskOrError(ctx context.Context, volumeRegion, volumeName string) (*computev1.Disk, error) {
	project := cloud.project
	disk, err := cloud.service.RegionDisks.Get(project, volumeRegion, volumeName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return disk, nil
}

func (cloud *CloudProvider) GetReplicaZoneURI(zone string) string {
	return cloud.service.BasePath + fmt.Sprintf(
		replicaZoneURITemplateSingleZone,
		cloud.project,
		zone)
}

func (cloud *CloudProvider) getRegionURI(region string) string {
	return cloud.service.BasePath + fmt.Sprintf(
		regionURITemplate,
		cloud.project,
		region)
}

func (cloud *CloudProvider) ValidateExistingDisk(ctx context.Context, resp *CloudDisk, diskType string, reqBytes, limBytes int64) error {
	klog.V(5).Infof("Validating existing disk %v with diskType: %s, reqested bytes: %v, limit bytes: %v", resp, diskType, reqBytes, limBytes)
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

	respType := strings.Split(resp.GetType(), "/")
	typeMatch := strings.TrimSpace(respType[len(respType)-1]) == strings.TrimSpace(diskType)
	typeDefault := diskType == "" && strings.TrimSpace(respType[len(respType)-1]) == "pd-standard"
	if !typeMatch && !typeDefault {
		return fmt.Errorf("disk already exists with incompatible type. Need %v. Got %v",
			diskType, respType[len(respType)-1])
	}
	return nil
}

func (cloud *CloudProvider) InsertDisk(ctx context.Context, volKey *meta.Key, diskType string, capBytes int64, capacityRange *csi.CapacityRange, replicaZones []string, snapshotID, diskEncryptionKmsKey string, multiWriter bool) error {
	klog.V(5).Infof("Inserting disk %v", volKey)
	switch volKey.Type() {
	case meta.Zonal:
		if multiWriter {
			return cloud.insertZonalAlphaDisk(ctx, volKey, diskType, capBytes, capacityRange, snapshotID, diskEncryptionKmsKey, multiWriter)
		} else {
			return cloud.insertZonalDisk(ctx, volKey, diskType, capBytes, capacityRange, snapshotID, diskEncryptionKmsKey)
		}
	case meta.Regional:
		if multiWriter {
			return cloud.insertRegionalAlphaDisk(ctx, volKey, diskType, capBytes, capacityRange, replicaZones, snapshotID, diskEncryptionKmsKey, multiWriter)
		} else {
			return cloud.insertRegionalDisk(ctx, volKey, diskType, capBytes, capacityRange, replicaZones, snapshotID, diskEncryptionKmsKey)
		}
	default:
		return fmt.Errorf("could not insert disk, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *CloudProvider) insertRegionalDisk(ctx context.Context, volKey *meta.Key, diskType string, capBytes int64, capacityRange *csi.CapacityRange, replicaZones []string, snapshotID, diskEncryptionKmsKey string) error {
	diskToCreate := &computev1.Disk{
		Name:        volKey.Name,
		SizeGb:      common.BytesToGb(capBytes),
		Description: "Regional disk created by GCE-PD CSI Driver",
		Type:        cloud.GetDiskTypeURI(volKey, diskType),
	}
	if snapshotID != "" {
		diskToCreate.SourceSnapshot = snapshotID
	}
	if len(replicaZones) != 0 {
		diskToCreate.ReplicaZones = replicaZones
	}
	if diskEncryptionKmsKey != "" {
		diskToCreate.DiskEncryptionKey = &computev1.CustomerEncryptionKey{
			KmsKeyName: diskEncryptionKmsKey,
		}
	}

	insertOp, err := cloud.service.RegionDisks.Insert(cloud.project, volKey.Region, diskToCreate).Context(ctx).Do()
	if err != nil {
		if IsGCEError(err, "alreadyExists") {
			disk, err := cloud.GetDisk(ctx, volKey)
			if err != nil {
				return err
			}
			err = cloud.ValidateExistingDisk(ctx, disk, diskType,
				int64(capacityRange.GetRequiredBytes()),
				int64(capacityRange.GetLimitBytes()))
			if err != nil {
				return err
			}
			klog.Warningf("GCE PD %s already exists, reusing", volKey.Name)
			return nil
		}
		return status.Error(codes.Internal, fmt.Sprintf("unkown Insert disk error: %v", err))
	}

	err = cloud.waitForRegionalOp(ctx, insertOp, volKey.Region)
	if err != nil {
		if IsGCEError(err, "alreadyExists") {
			disk, err := cloud.GetDisk(ctx, volKey)
			if err != nil {
				return err
			}
			err = cloud.ValidateExistingDisk(ctx, disk, diskType,
				int64(capacityRange.GetRequiredBytes()),
				int64(capacityRange.GetLimitBytes()))
			if err != nil {
				return err
			}
			klog.Warningf("GCE PD %s already exists after wait, reusing", volKey.Name)
			return nil
		}
		return fmt.Errorf("unkown Insert disk operation error: %v", err)
	}
	return nil
}

func (cloud *CloudProvider) insertRegionalAlphaDisk(ctx context.Context, volKey *meta.Key, diskType string, capBytes int64, capacityRange *csi.CapacityRange, replicaZones []string, snapshotID, diskEncryptionKmsKey string, multiWriter bool) error {
	diskToCreateAlpha := &computealpha.Disk{
		Name:        volKey.Name,
		SizeGb:      common.BytesToGb(capBytes),
		Description: "Regional disk created by GCE-PD CSI Driver",
		Type:        cloud.GetDiskTypeURI(volKey, diskType),
		MultiWriter: multiWriter,
	}
	if snapshotID != "" {
		diskToCreateAlpha.SourceSnapshot = snapshotID
	}
	if len(replicaZones) != 0 {
		diskToCreateAlpha.ReplicaZones = replicaZones
	}
	if diskEncryptionKmsKey != "" {
		diskToCreateAlpha.DiskEncryptionKey = &computealpha.CustomerEncryptionKey{
			KmsKeyName: diskEncryptionKmsKey,
		}
	}

	insertOp, err := cloud.alphaService.RegionDisks.Insert(cloud.project, volKey.Region, diskToCreateAlpha).Context(ctx).Do()
	if err != nil {
		if IsGCEError(err, "alreadyExists") {
			disk, err := cloud.GetDisk(ctx, volKey)
			if err != nil {
				return err
			}
			err = cloud.ValidateExistingDisk(ctx, disk, diskType,
				int64(capacityRange.GetRequiredBytes()),
				int64(capacityRange.GetLimitBytes()))
			if err != nil {
				return err
			}
			klog.Warningf("GCE PD %s already exists, reusing", volKey.Name)
			return nil
		}
		return status.Error(codes.Internal, fmt.Sprintf("unkown Insert disk error: %v", err))
	}

	err = cloud.waitForRegionalAlphaOp(ctx, insertOp, volKey.Region)
	if err != nil {
		if IsGCEError(err, "alreadyExists") {
			disk, err := cloud.GetDisk(ctx, volKey)
			if err != nil {
				return err
			}
			err = cloud.ValidateExistingDisk(ctx, disk, diskType,
				int64(capacityRange.GetRequiredBytes()),
				int64(capacityRange.GetLimitBytes()))
			if err != nil {
				return err
			}
			klog.Warningf("GCE PD %s already exists after wait, reusing", volKey.Name)
			return nil
		}
		return fmt.Errorf("unkown Insert disk operation error: %v", err)
	}
	return nil
}

func (cloud *CloudProvider) insertZonalDisk(ctx context.Context, volKey *meta.Key, diskType string, capBytes int64, capacityRange *csi.CapacityRange, snapshotID, diskEncryptionKmsKey string) error {
	diskToCreate := &computev1.Disk{
		Name:        volKey.Name,
		SizeGb:      common.BytesToGb(capBytes),
		Description: "Disk created by GCE-PD CSI Driver",
		Type:        cloud.GetDiskTypeURI(volKey, diskType),
	}

	if snapshotID != "" {
		diskToCreate.SourceSnapshot = snapshotID
	}

	if diskEncryptionKmsKey != "" {
		diskToCreate.DiskEncryptionKey = &computev1.CustomerEncryptionKey{
			KmsKeyName: diskEncryptionKmsKey,
		}
	}

	op, err := cloud.service.Disks.Insert(cloud.project, volKey.Zone, diskToCreate).Context(ctx).Do()

	if err != nil {
		if IsGCEError(err, "alreadyExists") {
			disk, err := cloud.GetDisk(ctx, volKey)
			if err != nil {
				return err
			}
			err = cloud.ValidateExistingDisk(ctx, disk, diskType,
				int64(capacityRange.GetRequiredBytes()),
				int64(capacityRange.GetLimitBytes()))
			if err != nil {
				return err
			}
			klog.Warningf("GCE PD %s already exists, reusing", volKey.Name)
			return nil
		}
		return fmt.Errorf("unkown Insert disk error: %v", err)
	}

	err = cloud.waitForZonalOp(ctx, op, volKey.Zone)

	if err != nil {
		if IsGCEError(err, "alreadyExists") {
			disk, err := cloud.GetDisk(ctx, volKey)
			if err != nil {
				return err
			}
			err = cloud.ValidateExistingDisk(ctx, disk, diskType,
				int64(capacityRange.GetRequiredBytes()),
				int64(capacityRange.GetLimitBytes()))
			if err != nil {
				return err
			}
			klog.Warningf("GCE PD %s already exists after wait, reusing", volKey.Name)
			return nil
		}
		return fmt.Errorf("unkown Insert disk operation error: %v", err)
	}
	return nil
}

func (cloud *CloudProvider) insertZonalAlphaDisk(ctx context.Context, volKey *meta.Key, diskType string, capBytes int64, capacityRange *csi.CapacityRange, snapshotID, diskEncryptionKmsKey string, multiWriter bool) error {
	diskToCreateAlpha := &computealpha.Disk{
		Name:        volKey.Name,
		SizeGb:      common.BytesToGb(capBytes),
		Description: "Disk created by GCE-PD CSI Driver",
		Type:        cloud.GetDiskTypeURI(volKey, diskType),
		MultiWriter: multiWriter,
	}

	if snapshotID != "" {
		diskToCreateAlpha.SourceSnapshot = snapshotID
	}

	if diskEncryptionKmsKey != "" {
		diskToCreateAlpha.DiskEncryptionKey = &computealpha.CustomerEncryptionKey{
			KmsKeyName: diskEncryptionKmsKey,
		}
	}

	op, err := cloud.alphaService.Disks.Insert(cloud.project, volKey.Zone, diskToCreateAlpha).Context(ctx).Do()

	if err != nil {
		if IsGCEError(err, "alreadyExists") {
			disk, err := cloud.GetDisk(ctx, volKey)
			if err != nil {
				return err
			}
			err = cloud.ValidateExistingDisk(ctx, disk, diskType,
				int64(capacityRange.GetRequiredBytes()),
				int64(capacityRange.GetLimitBytes()))
			if err != nil {
				return err
			}
			klog.Warningf("GCE PD %s already exists, reusing", volKey.Name)
			return nil
		}
		return fmt.Errorf("unkown Insert disk error: %v", err)
	}

	err = cloud.waitForZonalAlphaOp(ctx, op, volKey.Zone)

	if err != nil {
		if IsGCEError(err, "alreadyExists") {
			disk, err := cloud.GetDisk(ctx, volKey)
			if err != nil {
				return err
			}
			err = cloud.ValidateExistingDisk(ctx, disk, diskType,
				int64(capacityRange.GetRequiredBytes()),
				int64(capacityRange.GetLimitBytes()))
			if err != nil {
				return err
			}
			klog.Warningf("GCE PD %s already exists after wait, reusing", volKey.Name)
			return nil
		}
		return fmt.Errorf("unkown Insert disk operation error: %v", err)
	}
	return nil
}

func (cloud *CloudProvider) DeleteDisk(ctx context.Context, volKey *meta.Key) error {
	klog.V(5).Infof("Deleting disk: %v", volKey)
	switch volKey.Type() {
	case meta.Zonal:
		return cloud.deleteZonalDisk(ctx, volKey.Zone, volKey.Name)
	case meta.Regional:
		return cloud.deleteRegionalDisk(ctx, volKey.Region, volKey.Name)
	default:
		return fmt.Errorf("could not delete disk, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *CloudProvider) deleteZonalDisk(ctx context.Context, zone, name string) error {
	op, err := cloud.service.Disks.Delete(cloud.project, zone, name).Context(ctx).Do()
	if err != nil {
		if IsGCEError(err, "notFound") {
			// Already deleted
			return nil
		}
		return err
	}
	err = cloud.waitForZonalOp(ctx, op, zone)
	if err != nil {
		return err
	}
	return nil
}

func (cloud *CloudProvider) deleteRegionalDisk(ctx context.Context, region, name string) error {
	op, err := cloud.service.RegionDisks.Delete(cloud.project, region, name).Context(ctx).Do()
	if err != nil {
		if IsGCEError(err, "notFound") {
			// Already deleted
			return nil
		}
		return err
	}
	err = cloud.waitForRegionalOp(ctx, op, region)
	if err != nil {
		return err
	}
	return nil
}

func (cloud *CloudProvider) AttachDisk(ctx context.Context, volKey *meta.Key, readWrite, diskType, instanceZone, instanceName string) error {
	klog.V(5).Infof("Attaching disk %v to %s", volKey, instanceName)
	source := cloud.GetDiskSourceURI(volKey)

	deviceName, err := common.GetDeviceName(volKey)
	if err != nil {
		return fmt.Errorf("failed to get device name: %v", err)
	}
	attachedDiskV1 := &computev1.AttachedDisk{
		DeviceName: deviceName,
		Kind:       diskKind,
		Mode:       readWrite,
		Source:     source,
		Type:       diskType,
	}

	op, err := cloud.service.Instances.AttachDisk(cloud.project, instanceZone, instanceName, attachedDiskV1).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed cloud service attach disk call: %v", err)
	}
	err = cloud.waitForZonalOp(ctx, op, instanceZone)
	if err != nil {
		return fmt.Errorf("failed when waiting for zonal op: %v", err)
	}
	return nil
}

func (cloud *CloudProvider) DetachDisk(ctx context.Context, deviceName, instanceZone, instanceName string) error {
	klog.V(5).Infof("Detaching disk %v from %v", deviceName, instanceName)
	op, err := cloud.service.Instances.DetachDisk(cloud.project, instanceZone, instanceName, deviceName).Context(ctx).Do()
	if err != nil {
		return err
	}
	err = cloud.waitForZonalOp(ctx, op, instanceZone)
	if err != nil {
		return err
	}
	return nil
}

func (cloud *CloudProvider) GetDiskSourceURI(volKey *meta.Key) string {
	switch volKey.Type() {
	case Zonal:
		return cloud.getZonalDiskSourceURI(volKey.Name, volKey.Zone)
	case Regional:
		return cloud.getRegionalDiskSourceURI(volKey.Name, volKey.Region)
	default:
		return ""
	}
}

func (cloud *CloudProvider) getZonalDiskSourceURI(diskName, zone string) string {
	return cloud.service.BasePath + fmt.Sprintf(
		diskSourceURITemplateSingleZone,
		cloud.project,
		zone,
		diskName)
}

func (cloud *CloudProvider) getRegionalDiskSourceURI(diskName, region string) string {
	return cloud.service.BasePath + fmt.Sprintf(
		diskSourceURITemplateRegional,
		cloud.project,
		region,
		diskName)
}

func (cloud *CloudProvider) GetDiskTypeURI(volKey *meta.Key, diskType string) string {
	switch volKey.Type() {
	case meta.Zonal:
		return cloud.getZonalDiskTypeURI(volKey.Zone, diskType)
	case meta.Regional:
		return cloud.getRegionalDiskTypeURI(volKey.Region, diskType)
	default:
		return fmt.Sprintf("could get disk type URI, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *CloudProvider) getZonalDiskTypeURI(zone, diskType string) string {
	return cloud.service.BasePath + fmt.Sprintf(diskTypeURITemplateSingleZone, cloud.project, zone, diskType)
}

func (cloud *CloudProvider) getRegionalDiskTypeURI(region, diskType string) string {
	return cloud.service.BasePath + fmt.Sprintf(diskTypeURITemplateRegional, cloud.project, region, diskType)
}

func (cloud *CloudProvider) waitForZonalOp(ctx context.Context, op *computev1.Operation, zone string) error {
	svc := cloud.service
	project := cloud.project
	return wait.Poll(3*time.Second, 5*time.Minute, func() (bool, error) {
		pollOp, err := svc.ZoneOperations.Get(project, zone, op.Name).Context(ctx).Do()
		if err != nil {
			klog.Errorf("WaitForOp(op: %#v, zone: %#v) failed to poll the operation", op, zone)
			return false, err
		}
		done, err := opIsDone(pollOp)
		return done, err
	})
}

func (cloud *CloudProvider) waitForZonalAlphaOp(ctx context.Context, op *computealpha.Operation, zone string) error {
	svc := cloud.alphaService
	project := cloud.project
	return wait.Poll(3*time.Second, 5*time.Minute, func() (bool, error) {
		pollOp, err := svc.ZoneOperations.Get(project, zone, op.Name).Context(ctx).Do()
		if err != nil {
			klog.Errorf("WaitForOp(op: %#v, zone: %#v) failed to poll the operation", op, zone)
			return false, err
		}
		done, err := alphaOpIsDone(pollOp)
		return done, err
	})
}

func (cloud *CloudProvider) waitForRegionalOp(ctx context.Context, op *computev1.Operation, region string) error {
	return wait.Poll(3*time.Second, 5*time.Minute, func() (bool, error) {
		pollOp, err := cloud.service.RegionOperations.Get(cloud.project, region, op.Name).Context(ctx).Do()
		if err != nil {
			klog.Errorf("WaitForOp(op: %#v, region: %#v) failed to poll the operation", op, region)
			return false, err
		}
		done, err := opIsDone(pollOp)
		return done, err
	})
}

func (cloud *CloudProvider) waitForRegionalAlphaOp(ctx context.Context, op *computealpha.Operation, region string) error {
	return wait.Poll(3*time.Second, 5*time.Minute, func() (bool, error) {
		pollOp, err := cloud.alphaService.RegionOperations.Get(cloud.project, region, op.Name).Context(ctx).Do()
		if err != nil {
			klog.Errorf("WaitForOp(op: %#v, region: %#v) failed to poll the operation", op, region)
			return false, err
		}
		done, err := alphaOpIsDone(pollOp)
		return done, err
	})
}

func (cloud *CloudProvider) waitForGlobalOp(ctx context.Context, op *computev1.Operation) error {
	svc := cloud.service
	project := cloud.project
	return wait.Poll(3*time.Second, 5*time.Minute, func() (bool, error) {
		pollOp, err := svc.GlobalOperations.Get(project, op.Name).Context(ctx).Do()
		if err != nil {
			klog.Errorf("waitForGlobalOp(op: %#v) failed to poll the operation", op)
			return false, err
		}
		done, err := opIsDone(pollOp)
		return done, err
	})
}

func (cloud *CloudProvider) WaitForAttach(ctx context.Context, volKey *meta.Key, instanceZone, instanceName string) error {
	klog.V(5).Infof("Waiting for attach of disk %v to instance %v to complete...", volKey.Name, instanceName)
	start := time.Now()
	return wait.Poll(5*time.Second, 2*time.Minute, func() (bool, error) {
		klog.V(6).Infof("Polling for attach of disk %v to instance %v to complete for %v", volKey.Name, instanceName, time.Since(start))
		disk, err := cloud.GetDisk(ctx, volKey)
		if err != nil {
			return false, fmt.Errorf("GetDisk failed to get disk: %v", err)
		}

		if disk == nil {
			return false, fmt.Errorf("Disk %v could not be found", volKey.Name)
		}

		for _, user := range disk.GetUsers() {
			if strings.Contains(user, instanceName) && strings.Contains(user, instanceZone) {
				return true, nil
			}
		}
		return false, nil
	})
}

func opIsDone(op *computev1.Operation) (bool, error) {
	if op == nil || op.Status != operationStatusDone {
		return false, nil
	}
	if op.Error != nil && len(op.Error.Errors) > 0 && op.Error.Errors[0] != nil {
		return true, fmt.Errorf("operation %v failed (%v): %v", op.Name, op.Error.Errors[0].Code, op.Error.Errors[0].Message)
	}
	return true, nil
}

func alphaOpIsDone(op *computealpha.Operation) (bool, error) {
	if op == nil || op.Status != operationStatusDone {
		return false, nil
	}
	if op.Error != nil && len(op.Error.Errors) > 0 && op.Error.Errors[0] != nil {
		return true, fmt.Errorf("operation %v failed (%v): %v", op.Name, op.Error.Errors[0].Code, op.Error.Errors[0].Message)
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

func (cloud *CloudProvider) GetSnapshot(ctx context.Context, snapshotName string) (*computev1.Snapshot, error) {
	klog.V(5).Infof("Getting snapshot %v", snapshotName)
	svc := cloud.service
	project := cloud.project
	snapshot, err := svc.Snapshots.Get(project, snapshotName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (cloud *CloudProvider) DeleteSnapshot(ctx context.Context, snapshotName string) error {
	klog.V(5).Infof("Deleting snapshot %v", snapshotName)
	op, err := cloud.service.Snapshots.Delete(cloud.project, snapshotName).Context(ctx).Do()
	if err != nil {
		if IsGCEError(err, "notFound") {
			// Already deleted
			return nil
		}
		return err
	}
	err = cloud.waitForGlobalOp(ctx, op)
	if err != nil {
		return err
	}
	return nil
}

func (cloud *CloudProvider) CreateSnapshot(ctx context.Context, volKey *meta.Key, snapshotName string) (*computev1.Snapshot, error) {
	klog.V(5).Infof("Creating snapshot %s for volume %v", snapshotName, volKey)
	switch volKey.Type() {
	case meta.Zonal:
		return cloud.createZonalDiskSnapshot(ctx, volKey, snapshotName)
	case meta.Regional:
		return cloud.createRegionalDiskSnapshot(ctx, volKey, snapshotName)
	default:
		return nil, fmt.Errorf("could not create snapshot, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *CloudProvider) ResizeDisk(ctx context.Context, volKey *meta.Key, requestBytes int64) (int64, error) {
	klog.V(5).Infof("Resizing disk %v to size %v", volKey, requestBytes)
	cloudDisk, err := cloud.GetDisk(ctx, volKey)
	if err != nil {
		return -1, fmt.Errorf("failed to get disk: %v", err)
	}

	sizeGb := cloudDisk.GetSizeGb()
	requestGb := common.BytesToGb(requestBytes)

	// If disk is already of size equal or greater than requested size, we simply return
	if sizeGb >= requestGb {
		return requestBytes, nil
	}

	switch volKey.Type() {
	case meta.Zonal:
		return cloud.resizeZonalDisk(ctx, volKey, requestGb)
	case meta.Regional:
		return cloud.resizeRegionalDisk(ctx, volKey, requestGb)
	default:
		return -1, fmt.Errorf("could not resize disk, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *CloudProvider) resizeZonalDisk(ctx context.Context, volKey *meta.Key, requestGb int64) (int64, error) {
	resizeReq := &computev1.DisksResizeRequest{
		SizeGb: requestGb,
	}
	op, err := cloud.service.Disks.Resize(cloud.project, volKey.Zone, volKey.Name, resizeReq).Context(ctx).Do()
	if err != nil {
		return -1, fmt.Errorf("failed to resize zonal volume %v: %v", volKey.String(), err)
	}

	err = cloud.waitForZonalOp(ctx, op, volKey.Zone)
	if err != nil {
		return -1, fmt.Errorf("failed waiting for op for zonal resize for %s: %v", volKey.String(), err)
	}

	return requestGb, nil
}

func (cloud *CloudProvider) resizeRegionalDisk(ctx context.Context, volKey *meta.Key, requestGb int64) (int64, error) {
	resizeReq := &computev1.RegionDisksResizeRequest{
		SizeGb: requestGb,
	}

	op, err := cloud.service.RegionDisks.Resize(cloud.project, volKey.Region, volKey.Name, resizeReq).Context(ctx).Do()
	if err != nil {
		return -1, fmt.Errorf("failed to resize regional volume %v: %v", volKey.String(), err)
	}

	err = cloud.waitForRegionalOp(ctx, op, volKey.Region)
	if err != nil {
		return -1, fmt.Errorf("failed waiting for op for regional resize for %s: %v", volKey.String(), err)
	}

	return requestGb, nil
}

func (cloud *CloudProvider) createZonalDiskSnapshot(ctx context.Context, volKey *meta.Key, snapshotName string) (*computev1.Snapshot, error) {
	snapshotToCreate := &computev1.Snapshot{
		Name: snapshotName,
	}

	_, err := cloud.service.Disks.CreateSnapshot(cloud.project, volKey.Zone, volKey.Name, snapshotToCreate).Context(ctx).Do()

	if err != nil {
		return nil, err
	}

	return cloud.waitForSnapshotCreation(ctx, snapshotName)
}

func (cloud *CloudProvider) createRegionalDiskSnapshot(ctx context.Context, volKey *meta.Key, snapshotName string) (*computev1.Snapshot, error) {
	snapshotToCreate := &computev1.Snapshot{
		Name: snapshotName,
	}

	_, err := cloud.service.RegionDisks.CreateSnapshot(cloud.project, volKey.Region, volKey.Name, snapshotToCreate).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	return cloud.waitForSnapshotCreation(ctx, snapshotName)

}

func (cloud *CloudProvider) waitForSnapshotCreation(ctx context.Context, snapshotName string) (*computev1.Snapshot, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	timer := time.NewTimer(waitForSnapshotCreationTimeOut)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			klog.V(6).Infof("Checking GCE Snapshot %s.", snapshotName)
			snapshot, err := cloud.GetSnapshot(ctx, snapshotName)
			if err != nil {
				klog.Warningf("Error in getting snapshot %s, %v", snapshotName, err)
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
