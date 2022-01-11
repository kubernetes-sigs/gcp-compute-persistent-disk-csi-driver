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

package gceGCEDriver

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
)

type GCEControllerServer struct {
	Driver        *GCEDriver
	CloudProvider gce.GCECompute

	disks []*compute.Disk
	seen  map[string]int

	// A map storing all volumes with ongoing operations so that additional
	// operations for that same volume (as defined by Volume Key) return an
	// Aborted error
	volumeLocks *common.VolumeLocks

	// queue is a rate limited work queue for Controller Publish/Unpublish
	// Volume calls
	queue workqueue.RateLimitingInterface

	// publishErrorsSeenOnNode is a list of nodes with attach/detach
	// operation failures so those nodes shall be rate limited for all
	// the attach/detach operations until there is an attach / detach
	// operation succeeds
	publishErrorsSeenOnNode map[string]bool
}

type workItem struct {
	ctx          context.Context
	publishReq   *csi.ControllerPublishVolumeRequest
	unpublishReq *csi.ControllerUnpublishVolumeRequest
}

var _ csi.ControllerServer = &GCEControllerServer{}

const (
	// MaxVolumeSizeInBytes is the maximum standard and ssd size of 64TB
	MaxVolumeSizeInBytes     int64 = 64 * 1024 * 1024 * 1024 * 1024
	MinimumVolumeSizeInBytes int64 = 1 * 1024 * 1024 * 1024
	MinimumDiskSizeInGb            = 1

	attachableDiskTypePersistent = "PERSISTENT"

	replicationTypeNone       = "none"
	replicationTypeRegionalPD = "regional-pd"
)

func isDiskReady(disk *gce.CloudDisk) (bool, error) {
	status := disk.GetStatus()
	switch status {
	case "READY":
		return true, nil
	case "FAILED":
		return false, fmt.Errorf("Disk %s status is FAILED", disk.GetName())
	case "CREATING":
		klog.V(4).Infof("Disk %s status is CREATING", disk.GetName())
		return false, nil
	case "DELETING":
		klog.V(4).Infof("Disk %s status is DELETING", disk.GetName())
		return false, nil
	case "RESTORING":
		klog.V(4).Infof("Disk %s status is RESTORING", disk.GetName())
		return false, nil
	default:
		klog.V(4).Infof("Disk %s status is: %s", disk.GetName(), status)
	}

	return false, nil
}

func (gceCS *GCEControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	var err error
	// Validate arguments
	volumeCapabilities := req.GetVolumeCapabilities()
	name := req.GetName()
	capacityRange := req.GetCapacityRange()
	if len(name) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Name must be provided")
	}
	if volumeCapabilities == nil || len(volumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Volume capabilities must be provided")
	}

	capBytes, err := getRequestCapacity(capacityRange)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume Request Capacity is invalid: %v", err))
	}

	err = validateVolumeCapabilities(volumeCapabilities)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("VolumeCapabilities is invalid: %v", err))
	}

	// Apply Parameters (case-insensitive). We leave validation of
	// the values to the cloud provider.
	params, err := common.ExtractAndDefaultParameters(req.GetParameters(), gceCS.Driver.name, gceCS.Driver.extraVolumeLabels)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to extract parameters: %v", err)
	}
	// Determine multiWriter
	gceAPIVersion := gce.GCEAPIVersionV1
	multiWriter, _ := getMultiWriterFromCapabilities(volumeCapabilities)
	if multiWriter {
		gceAPIVersion = gce.GCEAPIVersionBeta
	}
	// Determine the zone or zones+region of the disk
	var zones []string
	var volKey *meta.Key
	switch params.ReplicationType {
	case replicationTypeNone:
		zones, err = pickZones(ctx, gceCS, req.GetAccessibilityRequirements(), 1)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume failed to pick zones for disk: %v", err))
		}
		if len(zones) != 1 {
			return nil, status.Errorf(codes.Internal, fmt.Sprintf("Failed to pick exactly 1 zone for zonal disk, got %v instead", len(zones)))
		}
		volKey = meta.ZonalKey(name, zones[0])

	case replicationTypeRegionalPD:
		zones, err = pickZones(ctx, gceCS, req.GetAccessibilityRequirements(), 2)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume failed to pick zones for disk: %v", err))
		}
		region, err := common.GetRegionFromZones(zones)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume failed to get region from zones: %v", err))
		}
		volKey = meta.RegionalKey(name, region)
	default:
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume replication type '%s' is not supported", params.ReplicationType))
	}

	volumeID, err := common.KeyToVolumeID(volKey, gceCS.CloudProvider.GetDefaultProject())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to convert volume key to volume ID: %v", err)
	}
	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, common.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)

	// Validate if disk already exists
	existingDisk, err := gceCS.CloudProvider.GetDisk(ctx, gceCS.CloudProvider.GetDefaultProject(), volKey, gceAPIVersion)
	if err != nil {
		if !gce.IsGCEError(err, "notFound") {
			return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume unknown get disk error when validating: %v", err))
		}
	}
	if err == nil {
		// There was no error so we want to validate the disk that we find
		err = gceCS.CloudProvider.ValidateExistingDisk(ctx, existingDisk, params,
			int64(capacityRange.GetRequiredBytes()),
			int64(capacityRange.GetLimitBytes()),
			multiWriter)
		if err != nil {
			return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("CreateVolume disk already exists with same name and is incompatible: %v", err))
		}

		ready, err := isDiskReady(existingDisk)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume disk %v had error checking ready status: %v", volKey, err))
		}
		if !ready {
			return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume existing disk %v is not ready", volKey))
		}

		// If there is no validation error, immediately return success
		klog.V(4).Infof("CreateVolume succeeded for disk %v, it already exists and was compatible", volKey)
		return generateCreateVolumeResponse(existingDisk, zones), nil
	}

	snapshotID := ""
	volumeContentSourceVolumeID := ""
	content := req.GetVolumeContentSource()
	if content != nil {
		if content.GetSnapshot() != nil {
			snapshotID = content.GetSnapshot().GetSnapshotId()

			// Verify that snapshot exists
			sl, err := gceCS.getSnapshotByID(ctx, snapshotID)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "CreateVolume failed to get snapshot %s: %v", snapshotID, err)
			} else if len(sl.Entries) == 0 {
				return nil, status.Errorf(codes.NotFound, "CreateVolume source snapshot %s does not exist", snapshotID)
			}
		}

		if content.GetVolume() != nil {
			volumeContentSourceVolumeID = content.GetVolume().GetVolumeId()
			// Verify that the source VolumeID is in the correct format.
			project, sourceVolKey, err := common.VolumeIDToKey(volumeContentSourceVolumeID)
			if err != nil {
				return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume source volume id is invalid: %v", err))
			}

			// Verify that the volume in VolumeContentSource exists.
			diskFromSourceVolume, err := gceCS.CloudProvider.GetDisk(ctx, project, sourceVolKey, gceAPIVersion)
			if err != nil {
				if gce.IsGCEError(err, "notFound") {
					return nil, status.Errorf(codes.NotFound, "CreateVolume source volume %s does not exist", volumeContentSourceVolumeID)
				} else {
					return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume unknown get disk error when validating: %v", err))
				}
			}

			// Verify the disk type and encryption key of the clone are the same as that of the source disk.
			if diskFromSourceVolume.GetPDType() != params.DiskType || !kmsKeyEqual(diskFromSourceVolume.GetKMSKeyName(), params.DiskEncryptionKMSKey) {
				return nil, status.Errorf(codes.InvalidArgument, `CreateVolume source volume Parameters do not match CreateVolumeRequest Parameters %v`, params)
			}

			if params.ReplicationType == replicationTypeNone {
				// For zonal->zonal disk clones, verify the zone is the same as that of the source disk.
				if sourceVolKey.Zone != volKey.Zone {
					return nil, status.Errorf(codes.InvalidArgument, `CreateVolume source volume zone %s does not match CreateVolume disk zone %s`, sourceVolKey.Zone, volKey.Zone)
				}
				// regional->zonal disk clones are not allowed.
				if diskFromSourceVolume.LocationType() == meta.Regional {
					return nil, status.Errorf(codes.InvalidArgument, `Cannot create a zonal disk clone from a regional disk`)
				}
			}

			if params.ReplicationType == replicationTypeNone {
				// For regional->regional disk clones, verify the region is the same as that of the source disk.
				if diskFromSourceVolume.LocationType() == meta.Regional && sourceVolKey.Region != volKey.Region {
					return nil, status.Errorf(codes.InvalidArgument, `CreateVolume source volume region %s does not match CreateVolume disk region %s`, sourceVolKey.Region, volKey.Region)
				}
				// For zonal->regional disk clones, verify one of the replica zones matches the source disk zone.
				if diskFromSourceVolume.LocationType() == meta.Zonal && !contains(zones, sourceVolKey.Zone) {
					return nil, status.Errorf(codes.InvalidArgument, `CreateVolume source volume zone %s does not match one of the replica zones %v of the CreateVolume regional disk`, sourceVolKey.Zone, zones)
				}
			}

			// Verify the source disk is ready.
			ready, err := isDiskReady(diskFromSourceVolume)
			if err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume disk from source volume %v had error checking ready status: %v", sourceVolKey, err))
			}
			if !ready {
				return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume disk from source volume %v is not ready", sourceVolKey))
			}
		}
	} else { // if VolumeContentSource is nil, validate access mode is not read only
		if readonly, _ := getReadOnlyFromCapabilities(volumeCapabilities); readonly {
			return nil, status.Error(codes.InvalidArgument, "VolumeContentSource must be provided when AccessMode is set to read only")
		}
	}

	// Create the disk
	var disk *gce.CloudDisk
	switch params.ReplicationType {
	case replicationTypeNone:
		if len(zones) != 1 {
			return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume failed to get a single zone for creating zonal disk, instead got: %v", zones))
		}
		disk, err = createSingleZoneDisk(ctx, gceCS.CloudProvider, name, zones, params, capacityRange, capBytes, snapshotID, volumeContentSourceVolumeID, multiWriter)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume failed to create single zonal disk %#v: %v", name, err))
		}
	case replicationTypeRegionalPD:
		if len(zones) != 2 {
			return nil, status.Errorf(codes.Internal, fmt.Sprintf("CreateVolume failed to get a 2 zones for creating regional disk, instead got: %v", zones))
		}
		disk, err = createRegionalDisk(ctx, gceCS.CloudProvider, name, zones, params, capacityRange, capBytes, snapshotID, volumeContentSourceVolumeID, multiWriter)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume failed to create regional disk %#v: %v", name, err))
		}
	default:
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume replication type '%s' is not supported", params.ReplicationType))
	}

	ready, err := isDiskReady(disk)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume disk %v had error checking ready status: %v", volKey, err))
	}
	if !ready {
		return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume disk %v is not ready", volKey))
	}

	klog.V(4).Infof("CreateVolume succeeded for disk %v", volKey)
	return generateCreateVolumeResponse(disk, zones), nil

}

func (gceCS *GCEControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	// Validate arguments
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume Volume ID must be provided")
	}

	project, volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		// Cannot find volume associated with this ID because VolumeID is not in
		// correct format, this is a success according to the Spec
		klog.Warningf("DeleteVolume treating volume as deleted because volume id %s is invalid: %v", volumeID, err)
		return &csi.DeleteVolumeResponse{}, nil
	}

	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			klog.Warningf("DeleteVolume treating volume as deleted because cannot find volume %v: %v", volumeID, err)
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal, "DeleteVolume error repairing underspecified volume key: %v", err)
	}

	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, common.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)

	err = gceCS.CloudProvider.DeleteDisk(ctx, project, volKey)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown Delete disk error: %v", err))
	}

	klog.V(4).Infof("DeleteVolume succeeded for disk %v", volKey)
	return &csi.DeleteVolumeResponse{}, nil
}

// Run starts the GCEControllerServer.
func (gceCS *GCEControllerServer) Run() {
	go wait.Until(gceCS.worker, 1*time.Second, wait.NeverStop)
}

func (gceCS *GCEControllerServer) worker() {
	// Runs until workqueue is shut down
	for gceCS.processNextWorkItem() {
	}
}

func (gceCS *GCEControllerServer) processNextWorkItem() bool {
	item, quit := gceCS.queue.Get()
	if quit {
		return false
	}
	defer gceCS.queue.Done(item)

	workItem, ok := item.(*workItem)
	if !ok {
		gceCS.queue.AddRateLimited(item)
		return true
	}

	if workItem.publishReq != nil {
		_, err := gceCS.executeControllerPublishVolume(workItem.ctx, workItem.publishReq)

		if err != nil {
			klog.Errorf("ControllerPublishVolume failed with error: %v", err)
		}
	}

	if workItem.unpublishReq != nil {
		_, err := gceCS.executeControllerUnpublishVolume(workItem.ctx, workItem.unpublishReq)

		if err != nil {
			klog.Errorf("ControllerUnpublishVolume failed with error: %v", err)
		}
	}

	gceCS.queue.Forget(item)
	return true
}

func (gceCS *GCEControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	// Only valid requests will be queued
	_, _, err := gceCS.validateControllerPublishVolumeRequest(ctx, req)

	if err != nil {
		return nil, err
	}

	// If the node is not marked, proceed the request
	if _, found := gceCS.publishErrorsSeenOnNode[req.NodeId]; !found {
		return gceCS.executeControllerPublishVolume(ctx, req)
	}

	// Node is marked so queue up the request
	gceCS.queue.AddRateLimited(&workItem{
		ctx:        ctx,
		publishReq: req,
	})
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (gceCS *GCEControllerServer) validateControllerPublishVolumeRequest(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (string, *meta.Key, error) {
	// Validate arguments
	volumeID := req.GetVolumeId()
	nodeID := req.GetNodeId()
	volumeCapability := req.GetVolumeCapability()
	if len(volumeID) == 0 {
		return "", nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume ID must be provided")
	}
	if len(nodeID) == 0 {
		return "", nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Node ID must be provided")
	}
	if volumeCapability == nil {
		return "", nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume capability must be provided")
	}

	project, volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return "", nil, status.Error(codes.InvalidArgument, fmt.Sprintf("ControllerPublishVolume volume ID is invalid: %v", err))
	}

	// TODO(#253): Check volume capability matches for ALREADY_EXISTS
	if err = validateVolumeCapability(volumeCapability); err != nil {
		return "", nil, status.Error(codes.InvalidArgument, fmt.Sprintf("VolumeCapabilities is invalid: %v", err))
	}

	return project, volKey, nil
}

func (gceCS *GCEControllerServer) executeControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	project, volKey, err := gceCS.validateControllerPublishVolumeRequest(ctx, req)

	if err != nil {
		return nil, err
	}

	volumeID := req.GetVolumeId()
	readOnly := req.GetReadonly()
	nodeID := req.GetNodeId()
	volumeCapability := req.GetVolumeCapability()

	pubVolResp := &csi.ControllerPublishVolumeResponse{
		PublishContext: nil,
	}

	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "ControllerPublishVolume could not find volume with ID %v: %v", volumeID, err)
		}
		return nil, status.Errorf(codes.Internal, "ControllerPublishVolume error repairing underspecified volume key: %v", err)
	}

	// Acquires the lock for the volume on that node only, because we need to support the ability
	// to publish the same volume onto different nodes concurrently
	lockingVolumeID := fmt.Sprintf("%s/%s", nodeID, volumeID)
	if acquired := gceCS.volumeLocks.TryAcquire(lockingVolumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, common.VolumeOperationAlreadyExistsFmt, lockingVolumeID)
	}
	defer gceCS.volumeLocks.Release(lockingVolumeID)

	_, err = gceCS.CloudProvider.GetDisk(ctx, project, volKey, gce.GCEAPIVersionV1)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Error(codes.NotFound, fmt.Sprintf("Could not find disk %v: %v", volKey.String(), err))
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("Unknown get disk error: %v", err))
	}
	instanceZone, instanceName, err := common.NodeIDToZoneAndName(nodeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("could not split nodeID: %v", err))
	}
	instance, err := gceCS.CloudProvider.GetInstanceOrError(ctx, instanceZone, instanceName)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Error(codes.NotFound, fmt.Sprintf("Could not find instance %v: %v", nodeID, err))
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("Unknown get instance error: %v", err))
	}

	readWrite := "READ_WRITE"
	if readOnly {
		readWrite = "READ_ONLY"
	}

	deviceName, err := common.GetDeviceName(volKey)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("error getting device name: %v", err))
	}

	attached, err := diskIsAttachedAndCompatible(deviceName, instance, volumeCapability, readWrite)
	if err != nil {
		return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("Disk %v already published to node %v but incompatbile: %v", volKey.Name, nodeID, err))
	}
	if attached {
		// Volume is attached to node. Success!
		klog.V(4).Infof("ControllerPublishVolume succeeded for disk %v to instance %v, already attached.", volKey, nodeID)
		return pubVolResp, nil
	}
	instanceZone, instanceName, err = common.NodeIDToZoneAndName(nodeID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("could not split nodeID: %v", err))
	}
	err = gceCS.CloudProvider.AttachDisk(ctx, project, volKey, readWrite, attachableDiskTypePersistent, instanceZone, instanceName)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown Attach error: %v", err))
	}

	err = gceCS.CloudProvider.WaitForAttach(ctx, project, volKey, instanceZone, instanceName)
	if err != nil {
		// Mark the node and rate limit all the following attach/detach
		// operations for this node
		gceCS.publishErrorsSeenOnNode[nodeID] = true
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown WaitForAttach error: %v", err))
	}

	// Attach succeeds so unmark the node
	delete(gceCS.publishErrorsSeenOnNode, nodeID)

	klog.V(4).Infof("ControllerPublishVolume succeeded for disk %v to instance %v", volKey, nodeID)
	return pubVolResp, nil
}

func (gceCS *GCEControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	// Only valid requests will be queued
	_, _, err := gceCS.validateControllerUnpublishVolumeRequest(ctx, req)

	if err != nil {
		return nil, err
	}

	// If the node is not marked, proceed the request
	if _, found := gceCS.publishErrorsSeenOnNode[req.NodeId]; !found {
		return gceCS.executeControllerUnpublishVolume(ctx, req)
	}

	// Node is marked so queue up the request
	gceCS.queue.AddRateLimited(&workItem{
		ctx:          ctx,
		unpublishReq: req,
	})

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (gceCS *GCEControllerServer) validateControllerUnpublishVolumeRequest(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (string, *meta.Key, error) {
	// Validate arguments
	volumeID := req.GetVolumeId()
	nodeID := req.GetNodeId()
	if len(volumeID) == 0 {
		return "", nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume Volume ID must be provided")
	}
	if len(nodeID) == 0 {
		return "", nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume Node ID must be provided")
	}

	project, volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return "", nil, status.Error(codes.InvalidArgument, fmt.Sprintf("ControllerUnpublishVolume Volume ID is invalid: %v", err))
	}

	return project, volKey, nil
}

func (gceCS *GCEControllerServer) executeControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	project, volKey, err := gceCS.validateControllerUnpublishVolumeRequest(ctx, req)

	if err != nil {
		return nil, err
	}

	volumeID := req.GetVolumeId()
	nodeID := req.GetNodeId()
	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "ControllerUnpublishVolume could not find volume with ID %v: %v", volumeID, err)
		}
		return nil, status.Errorf(codes.Internal, "ControllerUnpublishVolume error repairing underspecified volume key: %v", err)
	}

	// Acquires the lock for the volume on that node only, because we need to support the ability
	// to unpublish the same volume from different nodes concurrently
	lockingVolumeID := fmt.Sprintf("%s/%s", nodeID, volumeID)
	if acquired := gceCS.volumeLocks.TryAcquire(lockingVolumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, common.VolumeOperationAlreadyExistsFmt, lockingVolumeID)
	}
	defer gceCS.volumeLocks.Release(lockingVolumeID)

	instanceZone, instanceName, err := common.NodeIDToZoneAndName(nodeID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("could not split nodeID: %v", err))
	}
	instance, err := gceCS.CloudProvider.GetInstanceOrError(ctx, instanceZone, instanceName)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			// Node not existing on GCE means that disk has been detached
			klog.Warningf("Treating volume %v as unpublished because node %v could not be found", volKey.String(), instanceName)
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("error getting instance: %v", err))
	}

	deviceName, err := common.GetDeviceName(volKey)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("error getting device name: %v", err))
	}

	attached := diskIsAttached(deviceName, instance)

	if !attached {
		// Volume is not attached to node. Success!
		klog.V(4).Infof("ControllerUnpublishVolume succeeded for disk %v from node %v. Already not attached.", volKey, nodeID)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	err = gceCS.CloudProvider.DetachDisk(ctx, project, deviceName, instanceZone, instanceName)
	if err != nil {
		// Mark the node and rate limit all the following attach/detach
		// operations for this node
		gceCS.publishErrorsSeenOnNode[nodeID] = true
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown detach error: %v", err))
	}

	// Detach succeeds so unmark the node
	delete(gceCS.publishErrorsSeenOnNode, nodeID)

	klog.V(4).Infof("ControllerUnpublishVolume succeeded for disk %v from node %v", volKey, nodeID)
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (gceCS *GCEControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.GetVolumeCapabilities() == nil || len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities must be provided")
	}
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID must be provided")
	}
	project, volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Volume ID is invalid: %v", err))
	}

	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "ValidateVolumeCapabilities could not find volume with ID %v: %v", volumeID, err)
		}
		return nil, status.Errorf(codes.Internal, "ValidateVolumeCapabilities error repairing underspecified volume key: %v", err)
	}

	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, common.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)

	disk, err := gceCS.CloudProvider.GetDisk(ctx, project, volKey, gce.GCEAPIVersionV1)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Error(codes.NotFound, fmt.Sprintf("Could not find disk %v: %v", volKey.Name, err))
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("Unknown get disk error: %v", err))
	}

	// Check Volume Context is Empty
	if len(req.GetVolumeContext()) != 0 {
		return generateFailedValidationMessage("VolumeContext expected to be empty but got %v", req.GetVolumeContext()), nil
	}

	// Check volume capabilities supported by PD. These are the same for any PD
	if err := validateVolumeCapabilities(req.GetVolumeCapabilities()); err != nil {
		return generateFailedValidationMessage("VolumeCapabilities not valid: %v", err), nil
	}

	// Validate the disk parameters match the disk we GET
	params, err := common.ExtractAndDefaultParameters(req.GetParameters(), gceCS.Driver.name, gceCS.Driver.extraVolumeLabels)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to extract parameters: %v", err)
	}
	if err := gce.ValidateDiskParameters(disk, params); err != nil {
		return generateFailedValidationMessage("Parameters %v do not match given disk %s: %v", req.GetParameters(), disk.GetName(), err), nil
	}

	// Ignore secrets
	if len(req.GetSecrets()) != 0 {
		return generateFailedValidationMessage("Secrets expected to be empty but got %v", req.GetSecrets()), nil
	}

	// All valid, return success
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeContext:      req.GetVolumeContext(),
			VolumeCapabilities: req.GetVolumeCapabilities(),
			Parameters:         req.GetParameters(),
		},
	}, nil
}

// TODO(#809): Implement logic and advertise related capabilities.
func (gceCS *GCEControllerServer) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerGetVolume is not implemented")
}

func generateFailedValidationMessage(format string, a ...interface{}) *csi.ValidateVolumeCapabilitiesResponse {
	return &csi.ValidateVolumeCapabilitiesResponse{
		Message: fmt.Sprintf(format, a...),
	}
}

func (gceCS *GCEControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	// https://cloud.google.com/compute/docs/reference/beta/disks/list
	if req.MaxEntries < 0 {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf(
			"ListVolumes got max entries request %v. GCE only supports values >0", req.MaxEntries))
	}

	offset := 0
	var ok bool
	if req.StartingToken == "" {
		diskList, _, err := gceCS.CloudProvider.ListDisks(ctx)
		if err != nil {
			if gce.IsGCEInvalidError(err) {
				return nil, status.Error(codes.Aborted, fmt.Sprintf("ListVolumes error with invalid request: %v", err))
			}
			return nil, status.Error(codes.Internal, fmt.Sprintf("Unknown list disk error: %v", err))
		}
		gceCS.disks = diskList
		gceCS.seen = map[string]int{}
	} else {
		offset, ok = gceCS.seen[req.StartingToken]
		if !ok {
			return nil, status.Error(codes.Aborted, fmt.Sprintf("ListVolumes error with invalid startingToken: %s", req.StartingToken))
		}
	}

	var maxEntries int = int(req.MaxEntries)
	if maxEntries == 0 {
		maxEntries = len(gceCS.disks)
	}

	entries := []*csi.ListVolumesResponse_Entry{}
	for i := 0; i+offset < len(gceCS.disks) && i < maxEntries; i++ {
		d := gceCS.disks[i+offset]
		users := []string{}
		for _, u := range d.Users {
			users = append(users, cleanSelfLink(u))
		}
		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId: cleanSelfLink(d.SelfLink),
			},
			Status: &csi.ListVolumesResponse_VolumeStatus{
				PublishedNodeIds: users,
			},
		})
	}

	nextToken := ""
	if len(entries)+offset < len(gceCS.disks) {
		nextToken = string(uuid.NewUUID())
		gceCS.seen[nextToken] = len(entries) + offset
	}

	return &csi.ListVolumesResponse{
		Entries:   entries,
		NextToken: nextToken,
	}, nil
}

func (gceCS *GCEControllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	// https://cloud.google.com/compute/quotas
	// DISKS_TOTAL_GB.
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerGetCapabilities implements the default GRPC callout.
func (gceCS *GCEControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: gceCS.Driver.cscap,
	}, nil
}

func (gceCS *GCEControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	// Validate arguments
	volumeID := req.GetSourceVolumeId()
	if len(req.Name) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Snapshot name must be provided")
	}
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateSnapshot Source Volume ID must be provided")
	}
	project, volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateSnapshot Volume ID is invalid: %v", err))
	}

	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, common.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)

	// Check if volume exists
	_, err = gceCS.CloudProvider.GetDisk(ctx, project, volKey, gce.GCEAPIVersionV1)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Error(codes.NotFound, fmt.Sprintf("CreateSnapshot could not find disk %v: %v", volKey.String(), err))
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("CreateSnapshot unknown get disk error: %v", err))
	}

	// Check if snapshot already exists
	var snapshot *compute.Snapshot
	snapshot, err = gceCS.CloudProvider.GetSnapshot(ctx, project, req.Name)
	if err != nil {
		if !gce.IsGCEError(err, "notFound") {
			return nil, status.Error(codes.Internal, fmt.Sprintf("Unknown get snapshot error: %v", err))
		}
		// If we could not find the snapshot, we create a new one
		snapshotParams, err := common.ExtractAndDefaultSnapshotParameters(req.GetParameters())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Invalid snapshot parameters: %v", err))
		}
		snapshot, err = gceCS.CloudProvider.CreateSnapshot(ctx, project, volKey, req.Name, snapshotParams)
		if err != nil {
			if gce.IsGCEError(err, "notFound") {
				return nil, status.Error(codes.NotFound, fmt.Sprintf("Could not find volume with ID %v: %v", volKey.String(), err))
			}
			return nil, status.Error(codes.Internal, fmt.Sprintf("Unknown create snapshot error: %v", err))
		}
	}

	err = gceCS.validateExistingSnapshot(snapshot, volKey)
	if err != nil {
		return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("Error in creating snapshot: %v", err))
	}
	t, err := time.Parse(time.RFC3339, snapshot.CreationTimestamp)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to covert creation timestamp: %v", err))
	}

	tp, err := ptypes.TimestampProto(t)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to covert creation timestamp: %v", err))
	}

	ready, err := isCSISnapshotReady(snapshot.Status)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("Snapshot had error checking ready status: %v", err))
	}

	createResp := &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SizeBytes:      common.GbToBytes(snapshot.DiskSizeGb),
			SnapshotId:     cleanSelfLink(snapshot.SelfLink),
			SourceVolumeId: volumeID,
			CreationTime:   tp,
			ReadyToUse:     ready,
		},
	}
	klog.V(4).Infof("CreateSnapshot succeeded for snapshot %s on volume %s", cleanSelfLink(snapshot.SelfLink), volumeID)
	return createResp, nil
}

func (gceCS *GCEControllerServer) validateExistingSnapshot(snapshot *compute.Snapshot, volKey *meta.Key) error {
	if snapshot == nil {
		return fmt.Errorf("disk does not exist")
	}

	_, sourceKey, err := common.VolumeIDToKey(cleanSelfLink(snapshot.SourceDisk))
	if err != nil {
		return fmt.Errorf("fail to get source disk key %s, %v", snapshot.SourceDisk, err)
	}

	if sourceKey.String() != volKey.String() {
		return fmt.Errorf("snapshot already exists with same name but with a different disk source %s, expected disk source %s", sourceKey.String(), volKey.String())
	}
	// Snapshot exists with matching source disk.
	klog.V(5).Infof("Compatible snapshot %s exists with source disk %s.", snapshot.Name, snapshot.SourceDisk)
	return nil
}

func isCSISnapshotReady(status string) (bool, error) {
	switch status {
	case "READY":
		return true, nil
	case "FAILED":
		return false, fmt.Errorf("snapshot status is FAILED")
	case "DELETING":
		klog.V(4).Infof("snapshot is in DELETING")
		fallthrough
	default:
		return false, nil
	}
}

func (gceCS *GCEControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	// Validate arguments
	snapshotID := req.GetSnapshotId()
	if len(snapshotID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeleteSnapshot Snapshot ID must be provided")
	}

	project, key, err := common.SnapshotIDToProjectKey(snapshotID)
	if err != nil {
		// Cannot get snapshot ID from the passing request
		// This is a success according to the spec
		klog.Warningf("Snapshot id does not have the correct format %s", snapshotID)
		return &csi.DeleteSnapshotResponse{}, nil
	}

	err = gceCS.CloudProvider.DeleteSnapshot(ctx, project, key)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown Delete snapshot error: %v", err))
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

func (gceCS *GCEControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	// case 1: SnapshotId is not empty, return snapshots that match the snapshot id.
	if len(req.GetSnapshotId()) != 0 {
		return gceCS.getSnapshotByID(ctx, req.GetSnapshotId())
	}

	// case 2: no SnapshotId is set, so we return all the snapshots that satify the reqeust.
	return gceCS.getSnapshots(ctx, req)
}

func (gceCS *GCEControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerExpandVolume volume ID must be provided")
	}
	capacityRange := req.GetCapacityRange()
	reqBytes, err := getRequestCapacity(capacityRange)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("ControllerExpandVolume capacity range is invalid: %v", err))
	}

	project, volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("ControllerExpandVolume Volume ID is invalid: %v", err))
	}

	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "ControllerExpandVolume could not find volume with ID %v: %v", volumeID, err)
		}
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume error repairing underspecified volume key: %v", err)
	}

	resizedGb, err := gceCS.CloudProvider.ResizeDisk(ctx, project, volKey, reqBytes)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("ControllerExpandVolume failed to resize disk: %v", err))
	}

	klog.V(4).Infof("ControllerExpandVolume succeeded for disk %v to size %v", volKey, resizedGb)
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         common.GbToBytes(resizedGb),
		NodeExpansionRequired: true,
	}, nil
}

func (gceCS *GCEControllerServer) getSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	var snapshots []*compute.Snapshot
	var nextToken string
	var err error
	if len(req.GetSourceVolumeId()) != 0 {
		snapshots, nextToken, err = gceCS.CloudProvider.ListSnapshots(ctx, fmt.Sprintf("sourceDisk eq .*%s$", req.SourceVolumeId), int64(req.MaxEntries), req.StartingToken)
	} else {
		snapshots, nextToken, err = gceCS.CloudProvider.ListSnapshots(ctx, "", int64(req.MaxEntries), req.StartingToken)
	}
	if err != nil {
		if gce.IsGCEError(err, "invalid") {
			return nil, status.Error(codes.Aborted, fmt.Sprintf("Invalid error: %v", err))
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("Unknown list snapshot error: %v", err))
	}
	entries := []*csi.ListSnapshotsResponse_Entry{}

	for _, snapshot := range snapshots {
		entry, err := generateSnapshotEntry(snapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to generate snapshot entry: %v", err)
		}
		entries = append(entries, entry)
	}
	listSnapshotResp := &csi.ListSnapshotsResponse{
		Entries:   entries,
		NextToken: nextToken,
	}
	return listSnapshotResp, nil
}

func (gceCS *GCEControllerServer) getSnapshotByID(ctx context.Context, snapshotID string) (*csi.ListSnapshotsResponse, error) {
	project, key, err := common.SnapshotIDToProjectKey(snapshotID)
	if err != nil {
		// Cannot get snapshot ID from the passing request
		klog.Warningf("invalid snapshot id format %s", snapshotID)
		return &csi.ListSnapshotsResponse{}, nil
	}

	snapshot, err := gceCS.CloudProvider.GetSnapshot(ctx, project, key)
	if err != nil {
		if gce.IsGCEError(err, "notFound") {
			// return empty list if no snapshot is found
			return &csi.ListSnapshotsResponse{}, nil
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("Unknown list snapshot error: %v", err))
	}
	e, err := generateSnapshotEntry(snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to generate snapshot entry: %v", err)
	}

	entries := []*csi.ListSnapshotsResponse_Entry{e}
	//entries[0] = entry
	listSnapshotResp := &csi.ListSnapshotsResponse{
		Entries: entries,
	}
	return listSnapshotResp, nil
}

func generateSnapshotEntry(snapshot *compute.Snapshot) (*csi.ListSnapshotsResponse_Entry, error) {
	t, _ := time.Parse(time.RFC3339, snapshot.CreationTimestamp)

	tp, err := ptypes.TimestampProto(t)
	if err != nil {
		return nil, fmt.Errorf("Failed to covert creation timestamp: %v", err)
	}

	// We ignore the error intentionally here since we are just listing snapshots
	// TODO: If the snapshot is in "FAILED" state we need to think through what this
	// should actually look like.
	ready, _ := isCSISnapshotReady(snapshot.Status)

	entry := &csi.ListSnapshotsResponse_Entry{
		Snapshot: &csi.Snapshot{
			SizeBytes:      common.GbToBytes(snapshot.DiskSizeGb),
			SnapshotId:     cleanSelfLink(snapshot.SelfLink),
			SourceVolumeId: cleanSelfLink(snapshot.SourceDisk),
			CreationTime:   tp,
			ReadyToUse:     ready,
		},
	}
	return entry, nil
}

func getRequestCapacity(capRange *csi.CapacityRange) (int64, error) {
	var capBytes int64
	// Default case where nothing is set
	if capRange == nil {
		capBytes = MinimumVolumeSizeInBytes
		return capBytes, nil
	}

	rBytes := capRange.GetRequiredBytes()
	rSet := rBytes > 0
	lBytes := capRange.GetLimitBytes()
	lSet := lBytes > 0

	if lSet && rSet && lBytes < rBytes {
		return 0, fmt.Errorf("Limit bytes %v is less than required bytes %v", lBytes, rBytes)
	}
	if lSet && lBytes < MinimumVolumeSizeInBytes {
		return 0, fmt.Errorf("Limit bytes %v is less than minimum volume size: %v", lBytes, MinimumVolumeSizeInBytes)
	}

	// If Required set just set capacity to that which is Required
	if rSet {
		capBytes = rBytes
	}

	// Limit is more than Required, but larger than Minimum. So we just set capcity to Minimum
	// Too small, default
	if capBytes < MinimumVolumeSizeInBytes {
		capBytes = MinimumVolumeSizeInBytes
	}
	return capBytes, nil
}

func diskIsAttached(deviceName string, instance *compute.Instance) bool {
	for _, disk := range instance.Disks {
		if disk.DeviceName == deviceName {
			// Disk is attached to node
			return true
		}
	}
	return false
}

func diskIsAttachedAndCompatible(deviceName string, instance *compute.Instance, volumeCapability *csi.VolumeCapability, readWrite string) (bool, error) {
	for _, disk := range instance.Disks {
		if disk.DeviceName == deviceName {
			// Disk is attached to node
			if disk.Mode != readWrite {
				return true, fmt.Errorf("disk mode does not match. Got %v. Want %v", disk.Mode, readWrite)
			}
			// TODO(#253): Check volume capability matches for ALREADY_EXISTS
			return true, nil
		}
	}
	return false, nil
}

func pickZonesFromTopology(top *csi.TopologyRequirement, numZones int) ([]string, error) {
	reqZones, err := getZonesFromTopology(top.GetRequisite())
	if err != nil {
		return nil, fmt.Errorf("could not get zones from requisite topology: %v", err)
	}
	prefZones, err := getZonesFromTopology(top.GetPreferred())
	if err != nil {
		return nil, fmt.Errorf("could not get zones from preferred topology: %v", err)
	}

	if numZones <= len(prefZones) {
		return prefZones[0:numZones], nil
	} else {
		zones := sets.String{}
		// Add all preferred zones into zones
		zones.Insert(prefZones...)
		remainingNumZones := numZones - len(prefZones)
		// Take all of the remaining zones from requisite zones
		reqSet := sets.NewString(reqZones...)
		prefSet := sets.NewString(prefZones...)
		remainingZones := reqSet.Difference(prefSet)

		if remainingZones.Len() < remainingNumZones {
			return nil, fmt.Errorf("need %v zones from topology, only got %v unique zones", numZones, reqSet.Union(prefSet).Len())
		}
		// Add the remaining number of zones into the set
		nSlice, err := pickRandAndConsecutive(remainingZones.List(), remainingNumZones)
		if err != nil {
			return nil, err
		}
		zones.Insert(nSlice...)
		return zones.List(), nil
	}
}

func getZonesFromTopology(topList []*csi.Topology) ([]string, error) {
	zones := []string{}
	for _, top := range topList {
		if top.GetSegments() == nil {
			return nil, fmt.Errorf("preferred topologies specified but no segments")
		}

		// GCE PD cloud provider Create has no restrictions so just create in top preferred zone
		zone, err := getZoneFromSegment(top.GetSegments())
		if err != nil {
			return nil, fmt.Errorf("could not get zone from preferred topology: %v", err)
		}
		zones = append(zones, zone)
	}
	return zones, nil
}

func getZoneFromSegment(seg map[string]string) (string, error) {
	var zone string
	for k, v := range seg {
		switch k {
		case common.TopologyKeyZone:
			zone = v
		default:
			return "", fmt.Errorf("topology segment has unknown key %v", k)
		}
	}
	if len(zone) == 0 {
		return "", fmt.Errorf("topology specified but could not find zone in segment: %v", seg)
	}
	return zone, nil
}

func pickZones(ctx context.Context, gceCS *GCEControllerServer, top *csi.TopologyRequirement, numZones int) ([]string, error) {
	var zones []string
	var err error
	if top != nil {
		zones, err = pickZonesFromTopology(top, numZones)
		if err != nil {
			return nil, fmt.Errorf("failed to pick zones from topology: %v", err)
		}
	} else {
		zones, err = getDefaultZonesInRegion(ctx, gceCS, []string{gceCS.CloudProvider.GetDefaultZone()}, numZones)
		if err != nil {
			return nil, fmt.Errorf("failed to get default %v zones in region: %v", numZones, err)
		}
		klog.Warningf("No zones have been specified in either topology or params, picking default zone: %v", zones)

	}
	return zones, nil
}

func getDefaultZonesInRegion(ctx context.Context, gceCS *GCEControllerServer, existingZones []string, numZones int) ([]string, error) {
	region, err := common.GetRegionFromZones(existingZones)
	if err != nil {
		return nil, fmt.Errorf("failed to get region from zones: %v", err)
	}
	needToGet := numZones - len(existingZones)
	totZones, err := gceCS.CloudProvider.ListZones(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("failed to list zones from cloud provider: %v", err)
	}
	remainingZones := sets.NewString(totZones...).Difference(sets.NewString(existingZones...))
	l := remainingZones.List()
	if len(l) < needToGet {
		return nil, fmt.Errorf("not enough remaining zones in %v to get %v zones out", l, needToGet)
	}
	// add l and zones
	ret := append(existingZones, l[0:needToGet]...)
	if len(ret) != numZones {
		return nil, fmt.Errorf("did some math wrong, need %v zones, but got %v", numZones, ret)
	}
	return ret, nil
}

func generateCreateVolumeResponse(disk *gce.CloudDisk, zones []string) *csi.CreateVolumeResponse {
	tops := []*csi.Topology{}
	for _, zone := range zones {
		tops = append(tops, &csi.Topology{
			Segments: map[string]string{common.TopologyKeyZone: zone},
		})
	}
	realDiskSizeBytes := common.GbToBytes(disk.GetSizeGb())
	createResp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes:      realDiskSizeBytes,
			VolumeId:           cleanSelfLink(disk.GetSelfLink()),
			VolumeContext:      nil,
			AccessibleTopology: tops,
		},
	}
	snapshotID := disk.GetSnapshotId()
	diskID := disk.GetSourceDiskId()
	if diskID != "" || snapshotID != "" {
		contentSource := &csi.VolumeContentSource{}
		if snapshotID != "" {
			contentSource = &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{
					Snapshot: &csi.VolumeContentSource_SnapshotSource{
						SnapshotId: snapshotID,
					},
				},
			}
		}
		if diskID != "" {
			contentSource = &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: diskID,
					},
				},
			}
		}
		createResp.Volume.ContentSource = contentSource
	}
	return createResp
}

func cleanSelfLink(selfLink string) string {
	temp := strings.TrimPrefix(selfLink, gce.GCEComputeAPIEndpoint)
	temp = strings.TrimPrefix(temp, gce.GCEComputeBetaAPIEndpoint)
	return strings.TrimPrefix(temp, gce.GCEComputeAlphaAPIEndpoint)
}

func createRegionalDisk(ctx context.Context, cloudProvider gce.GCECompute, name string, zones []string, params common.DiskParameters, capacityRange *csi.CapacityRange, capBytes int64, snapshotID string, volumeContentSourceVolumeID string, multiWriter bool) (*gce.CloudDisk, error) {
	project := cloudProvider.GetDefaultProject()
	region, err := common.GetRegionFromZones(zones)
	if err != nil {
		return nil, fmt.Errorf("failed to get region from zones: %v", err)
	}

	fullyQualifiedReplicaZones := []string{}
	for _, replicaZone := range zones {
		fullyQualifiedReplicaZones = append(
			fullyQualifiedReplicaZones, cloudProvider.GetReplicaZoneURI(project, replicaZone))
	}

	err = cloudProvider.InsertDisk(ctx, project, meta.RegionalKey(name, region), params, capBytes, capacityRange, fullyQualifiedReplicaZones, snapshotID, volumeContentSourceVolumeID, multiWriter)
	if err != nil {
		return nil, fmt.Errorf("failed to insert regional disk: %v", err)
	}

	gceAPIVersion := gce.GCEAPIVersionV1
	if multiWriter {
		gceAPIVersion = gce.GCEAPIVersionBeta
	}

	disk, err := cloudProvider.GetDisk(ctx, project, meta.RegionalKey(name, region), gceAPIVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk after creating regional disk: %v", err)
	}
	return disk, nil
}

func createSingleZoneDisk(ctx context.Context, cloudProvider gce.GCECompute, name string, zones []string, params common.DiskParameters, capacityRange *csi.CapacityRange, capBytes int64, snapshotID string, volumeContentSourceVolumeID string, multiWriter bool) (*gce.CloudDisk, error) {
	project := cloudProvider.GetDefaultProject()
	if len(zones) != 1 {
		return nil, fmt.Errorf("got wrong number of zones for zonal create volume: %v", len(zones))
	}
	diskZone := zones[0]
	err := cloudProvider.InsertDisk(ctx, project, meta.ZonalKey(name, diskZone), params, capBytes, capacityRange, nil, snapshotID, volumeContentSourceVolumeID, multiWriter)
	if err != nil {
		return nil, fmt.Errorf("failed to insert zonal disk: %v", err)
	}

	gceAPIVersion := gce.GCEAPIVersionV1
	if multiWriter {
		gceAPIVersion = gce.GCEAPIVersionBeta
	}
	disk, err := cloudProvider.GetDisk(ctx, project, meta.ZonalKey(name, diskZone), gceAPIVersion)
	if err != nil {
		return nil, err
	}
	return disk, nil
}

func pickRandAndConsecutive(slice []string, n int) ([]string, error) {
	if n > len(slice) {
		return nil, fmt.Errorf("n: %v is greater than length of provided slice: %v", n, slice)
	}
	sort.Strings(slice)
	start := rand.Intn(len(slice))
	ret := []string{}
	for i := 0; i < n; i++ {
		idx := (start + i) % len(slice)
		ret = append(ret, slice[idx])
	}
	return ret, nil
}
