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

	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	computebeta "google.golang.org/api/compute/v0.beta"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/pkg/cloudprovider/providers/gce/cloud/meta"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

const (
	operationStatusDone = "DONE"
)

type GCECompute interface {
	// Disk Methods
	GetDisk(ctx context.Context, volumeKey *meta.Key) (*CloudDisk, error)
	ValidateExistingDisk(ctx context.Context, disk *CloudDisk, diskType string, reqBytes, limBytes int64) error
	InsertDisk(ctx context.Context, volKey *meta.Key, diskType string, capBytes int64, capacityRange *csi.CapacityRange, replicaZones []string) error
	DeleteDisk(ctx context.Context, volumeKey *meta.Key) error
	AttachDisk(ctx context.Context, disk *CloudDisk, volKey *meta.Key, readWrite, diskType, instanceZone, instanceName string) error
	DetachDisk(ctx context.Context, volKey *meta.Key, instanceZone, instanceName string) error
	GetDiskSourceURI(disk *CloudDisk, volKey *meta.Key) string
	GetDiskTypeURI(volKey *meta.Key, diskType string) string
	WaitForAttach(ctx context.Context, volKey *meta.Key, instanceZone, instanceName string) error
	// Regional Disk Methods
	GetReplicaZoneURI(zone string) string
	// Instance Methods
	GetInstanceOrError(ctx context.Context, instanceZone, instanceName string) (*compute.Instance, error)
	// Zone Methods
	ListZones(ctx context.Context, region string) ([]string, error)
}

func (cloud *CloudProvider) ListZones(ctx context.Context, region string) ([]string, error) {
	zones := []string{}
	zoneList, err := cloud.service.Zones.List(cloud.project).Filter(fmt.Sprintf("region eq .*%s$", region)).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list zones in region %s: %v", region, err)
	}
	for _, zone := range zoneList.Items {
		zones = append(zones, zone.Name)
	}
	return zones, nil

}

func (cloud *CloudProvider) GetDisk(ctx context.Context, key *meta.Key) (*CloudDisk, error) {
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

func (cloud *CloudProvider) getZonalDiskOrError(ctx context.Context, volumeZone, volumeName string) (*compute.Disk, error) {
	svc := cloud.service
	project := cloud.project
	glog.V(4).Infof("Getting disk %v from zone %v", volumeName, volumeZone)
	disk, err := svc.Disks.Get(project, volumeZone, volumeName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("Got disk %v from zone %v", volumeName, volumeZone)
	return disk, nil
}

func (cloud *CloudProvider) getRegionalDiskOrError(ctx context.Context, volumeRegion, volumeName string) (*computebeta.Disk, error) {
	project := cloud.project
	glog.V(4).Infof("Getting disk %v from region %v", volumeName, volumeRegion)
	disk, err := cloud.betaService.RegionDisks.Get(project, volumeRegion, volumeName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("Got disk %v from region %v", volumeName, volumeRegion)
	return disk, nil
}

func (cloud *CloudProvider) GetReplicaZoneURI(zone string) string {
	return cloud.betaService.BasePath + fmt.Sprintf(
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

	// Volume exists with matching name, capacity, type.
	glog.V(4).Infof("Compatible disk already exists. Reusing existing.")
	return nil
}

func (cloud *CloudProvider) InsertDisk(ctx context.Context, volKey *meta.Key, diskType string, capBytes int64, capacityRange *csi.CapacityRange, replicaZones []string) error {
	switch volKey.Type() {
	case meta.Zonal:
		return cloud.insertZonalDisk(ctx, volKey, diskType, capBytes, capacityRange)
	case meta.Regional:
		return cloud.insertRegionalDisk(ctx, volKey, diskType, capBytes, capacityRange, replicaZones)
	default:
		return fmt.Errorf("could not insert disk, key was neither zonal nor regional, instead got: %v", volKey.String())
	}
}

func (cloud *CloudProvider) insertRegionalDisk(ctx context.Context, volKey *meta.Key, diskType string, capBytes int64, capacityRange *csi.CapacityRange, replicaZones []string) error {
	diskToCreateBeta := &computebeta.Disk{
		Name:        volKey.Name,
		SizeGb:      common.BytesToGb(capBytes),
		Description: "Regional disk created by GCE-PD CSI Driver",
		Type:        cloud.GetDiskTypeURI(volKey, diskType),
	}
	if len(replicaZones) != 0 {
		diskToCreateBeta.ReplicaZones = replicaZones
	}

	insertOp, err := cloud.betaService.RegionDisks.Insert(cloud.project, volKey.Region, diskToCreateBeta).Context(ctx).Do()
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
			glog.Warningf("GCE PD %s already exists, reusing", volKey.Name)
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
			glog.Warningf("GCE PD %s already exists after wait, reusing", volKey.Name)
			return nil
		}
		return fmt.Errorf("unkown Insert disk operation error: %v", err)
	}
	return nil
}

func (cloud *CloudProvider) insertZonalDisk(ctx context.Context, volKey *meta.Key, diskType string, capBytes int64, capacityRange *csi.CapacityRange) error {
	diskToCreate := &compute.Disk{
		Name:        volKey.Name,
		SizeGb:      common.BytesToGb(capBytes),
		Description: "Disk created by GCE-PD CSI Driver",
		Type:        cloud.GetDiskTypeURI(volKey, diskType),
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
			glog.Warningf("GCE PD %s already exists, reusing", volKey.Name)
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
			glog.Warningf("GCE PD %s already exists after wait, reusing", volKey.Name)
			return nil
		}
		return fmt.Errorf("unkown Insert disk operation error: %v", err)
	}
	return nil
}

func (cloud *CloudProvider) DeleteDisk(ctx context.Context, volKey *meta.Key) error {
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
	op, err := cloud.betaService.RegionDisks.Delete(cloud.project, region, name).Context(ctx).Do()
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

func (cloud *CloudProvider) AttachDisk(ctx context.Context, disk *CloudDisk, volKey *meta.Key, readWrite, diskType, instanceZone, instanceName string) error {
	source := cloud.GetDiskSourceURI(disk, volKey)

	attachedDiskV1 := &compute.AttachedDisk{
		DeviceName: disk.GetName(),
		Kind:       disk.GetKind(),
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

func (cloud *CloudProvider) DetachDisk(ctx context.Context, volKey *meta.Key, instanceZone, instanceName string) error {
	op, err := cloud.service.Instances.DetachDisk(cloud.project, instanceZone, instanceName, volKey.Name).Context(ctx).Do()
	if err != nil {
		return err
	}
	err = cloud.waitForZonalOp(ctx, op, instanceZone)
	if err != nil {
		return err
	}
	return nil
}

func (cloud *CloudProvider) GetDiskSourceURI(disk *CloudDisk, volKey *meta.Key) string {
	switch disk.Type() {
	case Zonal:
		return cloud.getZonalDiskSourceURI(disk.ZonalDisk, volKey.Zone)
	case Regional:
		return cloud.getRegionalDiskSourceURI(disk.RegionalDisk, volKey.Region)
	default:
		return ""
	}
}

func (cloud *CloudProvider) getZonalDiskSourceURI(disk *compute.Disk, zone string) string {
	return cloud.service.BasePath + fmt.Sprintf(
		diskSourceURITemplateSingleZone,
		cloud.project,
		zone,
		disk.Name)
}

func (cloud *CloudProvider) getRegionalDiskSourceURI(disk *computebeta.Disk, region string) string {
	return cloud.betaService.BasePath + fmt.Sprintf(
		diskSourceURITemplateRegional,
		cloud.project,
		region,
		disk.Name)
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
	return cloud.betaService.BasePath + fmt.Sprintf(diskTypeURITemplateRegional, cloud.project, region, diskType)
}

func (cloud *CloudProvider) waitForZonalOp(ctx context.Context, op *compute.Operation, zone string) error {
	svc := cloud.service
	project := cloud.project
	return wait.Poll(3*time.Second, 5*time.Minute, func() (bool, error) {
		pollOp, err := svc.ZoneOperations.Get(project, zone, op.Name).Context(ctx).Do()
		if err != nil {
			glog.Errorf("WaitForOp(op: %#v, zone: %#v) failed to poll the operation", op, zone)
			return false, err
		}
		done, err := opIsDone(pollOp)
		return done, err
	})
}

func (cloud *CloudProvider) waitForRegionalOp(ctx context.Context, op *computebeta.Operation, region string) error {
	return wait.Poll(3*time.Second, 5*time.Minute, func() (bool, error) {
		pollOp, err := cloud.betaService.RegionOperations.Get(cloud.project, region, op.Name).Context(ctx).Do()
		if err != nil {
			glog.Errorf("WaitForOp(op: %#v, region: %#v) failed to poll the operation", op, region)
			return false, err
		}
		done, err := regionalOpIsDone(pollOp)
		return done, err
	})
}

func (cloud *CloudProvider) WaitForAttach(ctx context.Context, volKey *meta.Key, instanceZone, instanceName string) error {
	return wait.Poll(5*time.Second, 2*time.Minute, func() (bool, error) {
		disk, err := cloud.GetDisk(ctx, volKey)
		if err != nil {
			glog.Errorf("GetDisk failed to get disk: %v", err)
			return false, err
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

func opIsDone(op *compute.Operation) (bool, error) {
	if op == nil || op.Status != operationStatusDone {
		return false, nil
	}
	if op.Error != nil && len(op.Error.Errors) > 0 && op.Error.Errors[0] != nil {
		return true, fmt.Errorf("operation %v failed (%v): %v", op.Name, op.Error.Errors[0].Code, op.Error.Errors[0].Message)
	}
	return true, nil
}

func regionalOpIsDone(op *computebeta.Operation) (bool, error) {
	if op == nil || op.Status != operationStatusDone {
		return false, nil
	}
	if op.Error != nil && len(op.Error.Errors) > 0 && op.Error.Errors[0] != nil {
		return true, fmt.Errorf("operation %v failed (%v): %v", op.Name, op.Error.Errors[0].Code, op.Error.Errors[0].Message)
	}
	return true, nil
}

func (cloud *CloudProvider) GetInstanceOrError(ctx context.Context, instanceZone, instanceName string) (*compute.Instance, error) {
	svc := cloud.service
	project := cloud.project
	glog.V(4).Infof("Getting instance %v from zone %v", instanceName, instanceZone)

	instance, err := svc.Instances.Get(project, instanceZone, instanceName).Do()
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("Got instance %v from zone %v", instanceName, instanceZone)
	return instance, nil
}
