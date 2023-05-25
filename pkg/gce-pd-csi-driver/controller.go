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
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"google.golang.org/protobuf/types/known/timestamppb"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/metrics"
)

const (
	errorBackoffInitialDuration = 200 * time.Millisecond
	errorBackoffMaxDuration     = 5 * time.Minute
)

type GCEControllerServer struct {
	Driver        *GCEDriver
	CloudProvider gce.GCECompute
	Metrics       metrics.MetricsManager

	disks []*compute.Disk
	seen  map[string]int

	snapshots      []*csi.ListSnapshotsResponse_Entry
	snapshotTokens map[string]int

	// A map storing all volumes with ongoing operations so that additional
	// operations for that same volume (as defined by Volume Key) return an
	// Aborted error
	volumeLocks *common.VolumeLocks

	// There are several kinds of errors that are immediately retried by either
	// the CSI sidecars or the k8s control plane. The retries consume GCP api
	// quota, eg by doing ListVolumes, and so backoff needs to be used to
	// prevent quota exhaustion.
	//
	// Examples of these errors are the per-instance GCE operation queue getting
	// full (typically only 32 operations in flight at a time are allowed), and
	// disks being deleted out from under a PV causing unpublish errors.
	//
	// While we need to backoff, we also need some semblance of fairness. In
	// particular, volume unpublish retries happen very quickly, and with
	// a single backoff per node these retries can prevent any other operation
	// from making progess, even if it would succeed. Hence we track errors on
	// node and disk pairs, backing off only for calls matching such a
	// pair.
	//
	// An implication is that in the full operation queue situation, requests
	// for new disks will not backoff the first time. This is acceptible as a
	// single spurious call will not cause problems for quota exhaustion or make
	// the operation queue problem worse. This is well compensated by giving
	// disks where no problems are ocurring a chance to be processed.
	//
	// errorBackoff keeps track of any active backoff condition on a given node,
	// and the time when retry of controller publish/unpublish is permissible. A
	// node and disk pair is marked with backoff when any error is encountered
	// by the driver during controller publish/unpublish calls.  If the
	// controller eventually allows controller publish/publish requests for
	// volumes (because the backoff time expired), and those requests fail, the
	// next backoff retry time will be updated on every failure and capped at
	// 'errorBackoffMaxDuration'. Also, any successful controller
	// publish/unpublish call will clear the backoff condition for a node and
	// disk.
	errorBackoff *csiErrorBackoff
}

type csiErrorBackoff struct {
	backoff *flowcontrol.Backoff
}
type csiErrorBackoffId string

type workItem struct {
	ctx          context.Context
	publishReq   *csi.ControllerPublishVolumeRequest
	unpublishReq *csi.ControllerUnpublishVolumeRequest
}

// locationRequirements are additional location topology requirements that must be respected when creating a volume.
type locationRequirements struct {
	srcVolRegion         string
	srcVolZone           string
	srcReplicationType   string
	cloneReplicationType string
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
	diskNotFound              = ""
	// The maximum number of entries that we can include in the
	// ListVolumesResposne
	// In reality, the limit here is 4MB (based on gRPC client response limits),
	// but 500 is a good proxy (gives ~8KB of data per ListVolumesResponse#Entry)
	// See https://github.com/grpc/grpc/blob/master/include/grpc/impl/codegen/grpc_types.h#L503)
	maxListVolumesResponseEntries = 500
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

// cloningLocationRequirements returns additional location requirements to be applied to the given create volume requests topology.
// If the CreateVolumeRequest will use volume cloning, location requirements in compliance with the volume cloning limitations
// will be returned: https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/volume-cloning#limitations.
func cloningLocationRequirements(req *csi.CreateVolumeRequest, cloneReplicationType string) (*locationRequirements, error) {
	if !useVolumeCloning(req) {
		return nil, nil
	}
	// If we are using volume cloning, this will be set.
	volSrc := req.VolumeContentSource.GetVolume()
	volSrcVolID := volSrc.GetVolumeId()

	_, sourceVolKey, err := common.VolumeIDToKey(volSrcVolID)
	if err != nil {
		return nil, fmt.Errorf("volume ID is invalid: %w", err)
	}

	isZonalSrcVol := sourceVolKey.Type() == meta.Zonal
	if isZonalSrcVol {
		region, err := common.GetRegionFromZones([]string{sourceVolKey.Zone})
		if err != nil {
			return nil, fmt.Errorf("failed to get region from zones: %w", err)
		}
		sourceVolKey.Region = region
	}

	srcReplicationType := replicationTypeNone
	if !isZonalSrcVol {
		srcReplicationType = replicationTypeRegionalPD
	}

	return &locationRequirements{srcVolZone: sourceVolKey.Zone, srcVolRegion: sourceVolKey.Region, srcReplicationType: srcReplicationType, cloneReplicationType: cloneReplicationType}, nil
}

// useVolumeCloning returns true if the create volume request should be created with volume cloning.
func useVolumeCloning(req *csi.CreateVolumeRequest) bool {
	return req.VolumeContentSource != nil && req.VolumeContentSource.GetVolume() != nil
}

func (gceCS *GCEControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	var err error
	diskTypeForMetric := ""
	defer func() {
		if err != nil {
			gceCS.Metrics.RecordOperationErrorMetrics("CreateVolume", err, diskTypeForMetric)
		}
	}()
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
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume Request Capacity is invalid: %v", err.Error())
	}

	err = validateVolumeCapabilities(volumeCapabilities)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "VolumeCapabilities is invalid: %v", err.Error())
	}

	// Apply Parameters (case-insensitive). We leave validation of
	// the values to the cloud provider.
	params, err := common.ExtractAndDefaultParameters(req.GetParameters(), gceCS.Driver.name, gceCS.Driver.extraVolumeLabels)
	diskTypeForMetric = params.DiskType
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to extract parameters: %v", err.Error())
	}
	// Determine multiWriter
	gceAPIVersion := gce.GCEAPIVersionV1
	multiWriter, _ := getMultiWriterFromCapabilities(volumeCapabilities)
	if multiWriter {
		gceAPIVersion = gce.GCEAPIVersionBeta
	}

	var locationTopReq *locationRequirements
	if useVolumeCloning(req) {
		locationTopReq, err = cloningLocationRequirements(req, params.ReplicationType)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "failed to get location requirements: %v", err.Error())
		}
	}

	// Determine the zone or zones+region of the disk
	var zones []string
	var volKey *meta.Key
	switch params.ReplicationType {
	case replicationTypeNone:
		zones, err = pickZones(ctx, gceCS, req.GetAccessibilityRequirements(), 1, locationTopReq)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "CreateVolume failed to pick zones for disk: %v", err.Error())
		}
		if len(zones) != 1 {
			return nil, status.Errorf(codes.Internal, "Failed to pick exactly 1 zone for zonal disk, got %v instead", len(zones))
		}
		volKey = meta.ZonalKey(name, zones[0])

	case replicationTypeRegionalPD:
		zones, err = pickZones(ctx, gceCS, req.GetAccessibilityRequirements(), 2, locationTopReq)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "CreateVolume failed to pick zones for disk: %v", err.Error())
		}
		region, err := common.GetRegionFromZones(zones)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "CreateVolume failed to get region from zones: %v", err.Error())
		}
		volKey = meta.RegionalKey(name, region)
	default:
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume replication type '%s' is not supported", params.ReplicationType)
	}

	volumeID, err := common.KeyToVolumeID(volKey, gceCS.CloudProvider.GetDefaultProject())
	if err != nil {
		return nil, common.LoggedError("Failed to convert volume key to volume ID: ", err)
	}
	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, common.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)

	// Validate if disk already exists
	existingDisk, err := gceCS.CloudProvider.GetDisk(ctx, gceCS.CloudProvider.GetDefaultProject(), volKey, gceAPIVersion)
	diskTypeForMetric = metrics.GetDiskType(existingDisk)
	if err != nil {
		if !gce.IsGCEError(err, "notFound") {
			return nil, common.LoggedError("CreateVolume, failed to getDisk when validating: ", err)
		}
	}
	if err == nil {
		// There was no error so we want to validate the disk that we find
		err = gceCS.CloudProvider.ValidateExistingDisk(ctx, existingDisk, params,
			int64(capacityRange.GetRequiredBytes()),
			int64(capacityRange.GetLimitBytes()),
			multiWriter)
		if err != nil {
			return nil, status.Errorf(codes.AlreadyExists, "CreateVolume disk already exists with same name and is incompatible: %v", err.Error())
		}

		ready, err := isDiskReady(existingDisk)
		if err != nil {
			return nil, common.LoggedError("CreateVolume disk "+volKey.String()+" had error checking ready status: ", err)
		}
		if !ready {
			return nil, status.Errorf(codes.Internal, "CreateVolume existing disk %v is not ready", volKey)
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
				return nil, common.LoggedError("CreateVolume failed to get snapshot "+snapshotID+": ", err)
			} else if len(sl.Entries) == 0 {
				return nil, status.Errorf(codes.NotFound, "CreateVolume source snapshot %s does not exist", snapshotID)
			}
		}

		if content.GetVolume() != nil {
			volumeContentSourceVolumeID = content.GetVolume().GetVolumeId()
			// Verify that the source VolumeID is in the correct format.
			project, sourceVolKey, err := common.VolumeIDToKey(volumeContentSourceVolumeID)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "CreateVolume source volume id is invalid: %v", err.Error())
			}

			// Verify that the volume in VolumeContentSource exists.
			diskFromSourceVolume, err := gceCS.CloudProvider.GetDisk(ctx, project, sourceVolKey, gceAPIVersion)
			if err != nil {
				if gce.IsGCEError(err, "notFound") {
					return nil, status.Errorf(codes.NotFound, "CreateVolume source volume %s does not exist", volumeContentSourceVolumeID)
				} else {
					return nil, common.LoggedError("CreateVolume, getDisk error when validating: ", err)
				}
			}

			// Verify the disk type and encryption key of the clone are the same as that of the source disk.
			if diskFromSourceVolume.GetPDType() != params.DiskType || !gce.KmsKeyEqual(diskFromSourceVolume.GetKMSKeyName(), params.DiskEncryptionKMSKey) {
				return nil, status.Errorf(codes.InvalidArgument, "CreateVolume Parameters %v do not match source volume Parameters", params)
			}
			// Verify the disk capacity range are the same or greater as that of the source disk.
			if diskFromSourceVolume.GetSizeGb() > common.BytesToGbRoundDown(capBytes) {
				return nil, status.Errorf(codes.InvalidArgument, "CreateVolume disk CapacityRange %d is less than source volume CapacityRange %d", common.BytesToGbRoundDown(capBytes), diskFromSourceVolume.GetSizeGb())
			}

			if params.ReplicationType == replicationTypeNone {
				// For zonal->zonal disk clones, verify the zone is the same as that of the source disk.
				if sourceVolKey.Zone != volKey.Zone {
					return nil, status.Errorf(codes.InvalidArgument, "CreateVolume disk zone %s does not match source volume zone %s", volKey.Zone, sourceVolKey.Zone)
				}
				// regional->zonal disk clones are not allowed.
				if diskFromSourceVolume.LocationType() == meta.Regional {
					return nil, status.Errorf(codes.InvalidArgument, "Cannot create a zonal disk clone from a regional disk")
				}
			}

			if params.ReplicationType == replicationTypeNone {
				// For regional->regional disk clones, verify the region is the same as that of the source disk.
				if diskFromSourceVolume.LocationType() == meta.Regional && sourceVolKey.Region != volKey.Region {
					return nil, status.Errorf(codes.InvalidArgument, "CreateVolume disk region %s does not match source volume region %s", volKey.Region, sourceVolKey.Region)
				}
				// For zonal->regional disk clones, verify one of the replica zones matches the source disk zone.
				if diskFromSourceVolume.LocationType() == meta.Zonal && !containsZone(zones, sourceVolKey.Zone) {
					return nil, status.Errorf(codes.InvalidArgument, "CreateVolume regional disk replica zones %v do not match source volume zone %s", zones, sourceVolKey.Zone)
				}
			}

			// Verify the source disk is ready.
			ready, err := isDiskReady(diskFromSourceVolume)
			if err != nil {
				return nil, common.LoggedError("CreateVolume disk from source volume "+sourceVolKey.String()+"  had error checking ready status: ", err)
			}
			if !ready {
				return nil, status.Errorf(codes.Internal, "CreateVolume disk from source volume %v is not ready", sourceVolKey)
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
			return nil, status.Errorf(codes.Internal, "CreateVolume failed to get a single zone for creating zonal disk, instead got: %v", zones)
		}
		disk, err = createSingleZoneDisk(ctx, gceCS.CloudProvider, name, zones, params, capacityRange, capBytes, snapshotID, volumeContentSourceVolumeID, multiWriter)
		if err != nil {
			return nil, common.LoggedError("CreateVolume failed to create single zonal disk "+name+": ", err)
		}
	case replicationTypeRegionalPD:
		if len(zones) != 2 {
			return nil, status.Errorf(codes.Internal, "CreateVolume failed to get a 2 zones for creating regional disk, instead got: %v", zones)
		}
		disk, err = createRegionalDisk(ctx, gceCS.CloudProvider, name, zones, params, capacityRange, capBytes, snapshotID, volumeContentSourceVolumeID, multiWriter)
		if err != nil {
			return nil, common.LoggedError("CreateVolume failed to create regional disk "+name+": ", err)
		}
	default:
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume replication type '%s' is not supported", params.ReplicationType)
	}

	ready, err := isDiskReady(disk)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateVolume disk %v had error checking ready status: %v", volKey, err.Error())
	}
	if !ready {
		return nil, status.Errorf(codes.Internal, "CreateVolume disk %v is not ready", volKey)
	}

	klog.V(4).Infof("CreateVolume succeeded for disk %v", volKey)
	return generateCreateVolumeResponse(disk, zones), nil

}

func (gceCS *GCEControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	var err error
	diskTypeForMetric := ""
	defer func() {
		if err != nil {
			gceCS.Metrics.RecordOperationErrorMetrics("DeleteVolume", err, diskTypeForMetric)
		}
	}()
	// Validate arguments
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume Volume ID must be provided")
	}

	project, volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		// Cannot find volume associated with this ID because VolumeID is not in
		// correct format, this is a success according to the Spec
		klog.Warningf("DeleteVolume treating volume as deleted because volume id %s is invalid: %v", volumeID, err.Error())
		return &csi.DeleteVolumeResponse{}, nil
	}

	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			klog.Warningf("DeleteVolume treating volume as deleted because cannot find volume %v: %v", volumeID, err.Error())
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, common.LoggedError("DeleteVolume error repairing underspecified volume key: ", err)
	}

	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, common.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)
	disk, _ := gceCS.CloudProvider.GetDisk(ctx, project, volKey, gce.GCEAPIVersionV1)
	diskTypeForMetric = metrics.GetDiskType(disk)
	err = gceCS.CloudProvider.DeleteDisk(ctx, project, volKey)
	if err != nil {
		return nil, common.LoggedError("Failed to delete disk: ", err)
	}

	klog.V(4).Infof("DeleteVolume succeeded for disk %v", volKey)
	return &csi.DeleteVolumeResponse{}, nil
}

func (gceCS *GCEControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	var err error
	diskTypeForMetric := ""
	defer func() {
		if err != nil {
			gceCS.Metrics.RecordOperationErrorMetrics("ControllerPublishVolume", err, diskTypeForMetric)
		}
	}()
	// Only valid requests will be accepted
	_, _, err = gceCS.validateControllerPublishVolumeRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	backoffId := gceCS.errorBackoff.backoffId(req.NodeId, req.VolumeId)
	if gceCS.errorBackoff.blocking(backoffId) {
		return nil, status.Errorf(codes.Unavailable, "ControllerPublish not permitted on node %q due to backoff condition", req.NodeId)
	}

	resp, err, diskTypeForMetric := gceCS.executeControllerPublishVolume(ctx, req)
	if err != nil {
		klog.Infof("For node %s adding backoff due to error for volume %s: %v", req.NodeId, req.VolumeId, err.Error())
		gceCS.errorBackoff.next(backoffId)
	} else {
		klog.Infof("For node %s clear backoff due to successful publish of volume %v", req.NodeId, req.VolumeId)
		gceCS.errorBackoff.reset(backoffId)
	}
	return resp, err
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
		return "", nil, status.Errorf(codes.InvalidArgument, "ControllerPublishVolume volume ID is invalid: %v", err.Error())
	}

	// TODO(#253): Check volume capability matches for ALREADY_EXISTS
	if err = validateVolumeCapability(volumeCapability); err != nil {
		return "", nil, status.Errorf(codes.InvalidArgument, "VolumeCapabilities is invalid: %v", err.Error())
	}

	return project, volKey, nil
}

func parseMachineType(machineTypeUrl string) string {
	machineType, parseErr := common.ParseMachineType(machineTypeUrl)
	if parseErr != nil {
		// Parse errors represent an unexpected API change with instance.MachineType; log a warning.
		klog.Warningf("ParseMachineType(%v): %v", machineTypeUrl, parseErr)
	}
	return machineType
}

func (gceCS *GCEControllerServer) executeControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error, string) {
	diskType := ""
	project, volKey, err := gceCS.validateControllerPublishVolumeRequest(ctx, req)
	if err != nil {
		return nil, err, diskType
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
			return nil, status.Errorf(codes.NotFound, "ControllerPublishVolume could not find volume with ID %v: %v", volumeID, err.Error()), diskType
		}
		return nil, common.LoggedError("ControllerPublishVolume error repairing underspecified volume key: ", err), diskType
	}

	// Acquires the lock for the volume on that node only, because we need to support the ability
	// to publish the same volume onto different nodes concurrently
	lockingVolumeID := fmt.Sprintf("%s/%s", nodeID, volumeID)
	if acquired := gceCS.volumeLocks.TryAcquire(lockingVolumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, common.VolumeOperationAlreadyExistsFmt, lockingVolumeID), diskType
	}
	defer gceCS.volumeLocks.Release(lockingVolumeID)
	disk, err := gceCS.CloudProvider.GetDisk(ctx, project, volKey, gce.GCEAPIVersionV1)
	diskType = metrics.GetDiskType(disk)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "Could not find disk %v: %v", volKey.String(), err.Error()), diskType
		}
		return nil, status.Errorf(codes.Internal, "Failed to getDisk: %v", err.Error()), diskType
	}
	instanceZone, instanceName, err := common.NodeIDToZoneAndName(nodeID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not split nodeID: %v", err.Error()), diskType
	}
	instance, err := gceCS.CloudProvider.GetInstanceOrError(ctx, instanceZone, instanceName)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "Could not find instance %v: %v", nodeID, err.Error()), diskType
		}
		return nil, status.Errorf(codes.Internal, "Failed to get instance: %v", err.Error()), diskType
	}

	readWrite := "READ_WRITE"
	if readOnly {
		readWrite = "READ_ONLY"
	}

	deviceName, err := common.GetDeviceName(volKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting device name: %v", err.Error()), diskType
	}

	attached, err := diskIsAttachedAndCompatible(deviceName, instance, volumeCapability, readWrite)
	if err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "Disk %v already published to node %v but incompatible: %v", volKey.Name, nodeID, err.Error()), diskType
	}
	if attached {
		// Volume is attached to node. Success!
		klog.V(4).Infof("ControllerPublishVolume succeeded for disk %v to instance %v, already attached.", volKey, nodeID)
		return pubVolResp, nil, diskType
	}
	instanceZone, instanceName, err = common.NodeIDToZoneAndName(nodeID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "could not split nodeID: %v", err.Error()), diskType
	}
	err = gceCS.CloudProvider.AttachDisk(ctx, project, volKey, readWrite, attachableDiskTypePersistent, instanceZone, instanceName)
	if err != nil {
		var udErr *gce.UnsupportedDiskError
		if errors.As(err, &udErr) {
			// If we encountered an UnsupportedDiskError, rewrite the error message to be more user friendly.
			// The error message from GCE is phrased around disk create on VM creation, not runtime attach.
			machineType := parseMachineType(instance.MachineType)
			return nil, status.Errorf(codes.InvalidArgument, "'%s' is not a compatible disk type with the machine type %s, please review the GCP online documentation for available persistent disk options", udErr.DiskType, machineType), diskType
		}
		return nil, status.Errorf(codes.Internal, "Failed to Attach: %v", err.Error()), diskType
	}

	err = gceCS.CloudProvider.WaitForAttach(ctx, project, volKey, instanceZone, instanceName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Errored during WaitForAttach: %v", err.Error()), diskType
	}

	klog.V(4).Infof("ControllerPublishVolume succeeded for disk %v to instance %v", volKey, nodeID)
	return pubVolResp, nil, diskType
}

func (gceCS *GCEControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	var err error
	diskTypeForMetric := ""
	defer func() {
		if err != nil {
			gceCS.Metrics.RecordOperationErrorMetrics("ControllerUnpublishVolume", err, diskTypeForMetric)
		}
	}()
	_, _, err = gceCS.validateControllerUnpublishVolumeRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	err = status.Errorf(codes.InvalidArgument, "error message")
	// Only valid requests will be queued
	backoffId := gceCS.errorBackoff.backoffId(req.NodeId, req.VolumeId)
	if gceCS.errorBackoff.blocking(backoffId) {
		return nil, status.Errorf(codes.Unavailable, "ControllerUnpublish not permitted on node %q due to backoff condition", req.NodeId)
	}
	resp, err, diskTypeForMetric := gceCS.executeControllerUnpublishVolume(ctx, req)
	if err != nil {
		klog.Infof("For node %s adding backoff due to error for volume %s", req.NodeId, req.VolumeId)
		gceCS.errorBackoff.next(backoffId)
	} else {
		klog.Infof("For node %s clear backoff due to successful unpublish of volume %v", req.NodeId, req.VolumeId)
		gceCS.errorBackoff.reset(backoffId)
	}
	return resp, err
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
		return "", nil, status.Errorf(codes.InvalidArgument, "ControllerUnpublishVolume Volume ID is invalid: %v", err.Error())
	}

	return project, volKey, nil
}

func (gceCS *GCEControllerServer) executeControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error, string) {
	var diskType string
	project, volKey, err := gceCS.validateControllerUnpublishVolumeRequest(ctx, req)

	if err != nil {
		return nil, err, diskType
	}

	volumeID := req.GetVolumeId()
	nodeID := req.GetNodeId()
	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			klog.Warningf("Treating volume %v as unpublished because it could not be found", volumeID)
			return &csi.ControllerUnpublishVolumeResponse{}, nil, diskType
		}
		return nil, common.LoggedError("ControllerUnpublishVolume error repairing underspecified volume key: ", err), diskType
	}

	// Acquires the lock for the volume on that node only, because we need to support the ability
	// to unpublish the same volume from different nodes concurrently
	lockingVolumeID := fmt.Sprintf("%s/%s", nodeID, volumeID)
	if acquired := gceCS.volumeLocks.TryAcquire(lockingVolumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, common.VolumeOperationAlreadyExistsFmt, lockingVolumeID), diskType
	}
	defer gceCS.volumeLocks.Release(lockingVolumeID)
	diskToUnpublish, _ := gceCS.CloudProvider.GetDisk(ctx, project, volKey, gce.GCEAPIVersionV1)
	diskType = metrics.GetDiskType(diskToUnpublish)
	instanceZone, instanceName, err := common.NodeIDToZoneAndName(nodeID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "could not split nodeID: %v", err.Error()), diskType
	}
	instance, err := gceCS.CloudProvider.GetInstanceOrError(ctx, instanceZone, instanceName)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			// Node not existing on GCE means that disk has been detached
			klog.Warningf("Treating volume %v as unpublished because node %v could not be found", volKey.String(), instanceName)
			return &csi.ControllerUnpublishVolumeResponse{}, nil, diskType
		}
		return nil, status.Errorf(codes.Internal, "error getting instance: %v", err.Error()), diskType
	}

	deviceName, err := common.GetDeviceName(volKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting device name: %v", err.Error()), diskType
	}

	attached := diskIsAttached(deviceName, instance)

	if !attached {
		// Volume is not attached to node. Success!
		klog.V(4).Infof("ControllerUnpublishVolume succeeded for disk %v from node %v. Already not attached.", volKey, nodeID)
		return &csi.ControllerUnpublishVolumeResponse{}, nil, diskType
	}
	err = gceCS.CloudProvider.DetachDisk(ctx, project, deviceName, instanceZone, instanceName)
	if err != nil {
		return nil, common.LoggedError("Failed to detach: ", err), diskType
	}

	klog.V(4).Infof("ControllerUnpublishVolume succeeded for disk %v from node %v", volKey, nodeID)
	return &csi.ControllerUnpublishVolumeResponse{}, nil, diskType
}

func (gceCS *GCEControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	var err error
	diskTypeForMetric := ""
	defer func() {
		if err != nil {
			gceCS.Metrics.RecordOperationErrorMetrics("ValidateVolumeCapabilities", err, diskTypeForMetric)
		}
	}()
	if req.GetVolumeCapabilities() == nil || len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities must be provided")
	}
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID must be provided")
	}
	project, volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Volume ID is invalid: %v", err.Error())
	}
	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "ValidateVolumeCapabilities could not find volume with ID %v: %v", volumeID, err.Error())
		}
		return nil, common.LoggedError("ValidateVolumeCapabilities error repairing underspecified volume key: ", err)
	}

	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, common.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)

	disk, err := gceCS.CloudProvider.GetDisk(ctx, project, volKey, gce.GCEAPIVersionV1)
	diskTypeForMetric = metrics.GetDiskType(disk)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "Could not find disk %v: %v", volKey.Name, err.Error())
		}
		return nil, common.LoggedError("Failed to getDisk: ", err)
	}

	// Check Volume Context is Empty
	if len(req.GetVolumeContext()) != 0 {
		return generateFailedValidationMessage("VolumeContext expected to be empty but got %v", req.GetVolumeContext()), nil
	}

	// Check volume capabilities supported by PD. These are the same for any PD
	if err := validateVolumeCapabilities(req.GetVolumeCapabilities()); err != nil {
		return generateFailedValidationMessage("VolumeCapabilities not valid: %v", err.Error()), nil
	}

	// Validate the disk parameters match the disk we GET
	params, err := common.ExtractAndDefaultParameters(req.GetParameters(), gceCS.Driver.name, gceCS.Driver.extraVolumeLabels)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to extract parameters: %v", err.Error())
	}
	if err := gce.ValidateDiskParameters(disk, params); err != nil {
		return generateFailedValidationMessage("Parameters %v do not match given disk %s: %v", req.GetParameters(), disk.GetName(), err.Error()), nil
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
		return nil, status.Errorf(codes.InvalidArgument,
			"ListVolumes got max entries request %v. GCE only supports values >0", req.MaxEntries)
	}

	offset := 0
	var ok bool
	if req.StartingToken == "" {
		diskList, _, err := gceCS.CloudProvider.ListDisks(ctx)
		if err != nil {
			if gce.IsGCEInvalidError(err) {
				return nil, status.Errorf(codes.Aborted, "ListVolumes error with invalid request: %v", err.Error())
			}
			return nil, common.LoggedError("Failed to list disk: ", err)
		}
		gceCS.disks = diskList
		gceCS.seen = map[string]int{}
	} else {
		offset, ok = gceCS.seen[req.StartingToken]
		if !ok {
			return nil, status.Errorf(codes.Aborted, "ListVolumes error with invalid startingToken: %s", req.StartingToken)
		}
	}

	var maxEntries int = int(req.MaxEntries)
	if maxEntries == 0 {
		maxEntries = maxListVolumesResponseEntries
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
	var err error
	diskTypeForMetric := ""
	defer func() {
		if err != nil {
			gceCS.Metrics.RecordOperationErrorMetrics("CreateSnapshot", err, diskTypeForMetric)
		}
	}()
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
		return nil, status.Errorf(codes.InvalidArgument, "CreateSnapshot Volume ID is invalid: %v", err.Error())
	}

	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, common.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)

	// Check if volume exists
	disk, err := gceCS.CloudProvider.GetDisk(ctx, project, volKey, gce.GCEAPIVersionV1)
	diskTypeForMetric = metrics.GetDiskType(disk)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "CreateSnapshot could not find disk %v: %v", volKey.String(), err.Error())
		}
		return nil, common.LoggedError("CreateSnapshot, failed to getDisk: ", err)
	}

	snapshotParams, err := common.ExtractAndDefaultSnapshotParameters(req.GetParameters(), gceCS.Driver.name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid snapshot parameters: %v", err.Error())
	}

	var snapshot *csi.Snapshot
	switch snapshotParams.SnapshotType {
	case common.DiskSnapshotType:
		snapshot, err = gceCS.createPDSnapshot(ctx, project, volKey, req.Name, snapshotParams)
		if err != nil {
			return nil, err
		}
	case common.DiskImageType:
		snapshot, err = gceCS.createImage(ctx, project, volKey, req.Name, snapshotParams)
		if err != nil {
			return nil, err
		}
	default:
		return nil, status.Errorf(codes.InvalidArgument, "Invalid snapshot type: %s", snapshotParams.SnapshotType)
	}

	klog.V(4).Infof("CreateSnapshot succeeded for snapshot %s on volume %s", snapshot.SnapshotId, volumeID)
	return &csi.CreateSnapshotResponse{Snapshot: snapshot}, nil
}

func (gceCS *GCEControllerServer) createPDSnapshot(ctx context.Context, project string, volKey *meta.Key, snapshotName string, snapshotParams common.SnapshotParameters) (*csi.Snapshot, error) {
	volumeID, err := common.KeyToVolumeID(volKey, project)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid volume key: %v", volKey)
	}
	// Check if PD snapshot already exists
	var snapshot *compute.Snapshot
	snapshot, err = gceCS.CloudProvider.GetSnapshot(ctx, project, snapshotName)
	if err != nil {
		if !gce.IsGCEError(err, "notFound") {
			return nil, status.Errorf(codes.Internal, "Failed to get snapshot: %v", err.Error())
		}
		// If we could not find the snapshot, we create a new one
		snapshot, err = gceCS.CloudProvider.CreateSnapshot(ctx, project, volKey, snapshotName, snapshotParams)
		if err != nil {
			if gce.IsGCEError(err, "notFound") {
				return nil, status.Errorf(codes.NotFound, "Could not find volume with ID %v: %v", volKey.String(), err.Error())
			}
			return nil, common.LoggedError("Failed to create snapshot: ", err)
		}
	}

	err = gceCS.validateExistingSnapshot(snapshot, volKey)
	if err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "Error in creating snapshot: %v", err.Error())
	}

	timestamp, err := parseTimestamp(snapshot.CreationTimestamp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to covert creation timestamp: %v", err.Error())
	}

	ready, err := isCSISnapshotReady(snapshot.Status)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Snapshot had error checking ready status: %v", err.Error())
	}

	return &csi.Snapshot{
		SizeBytes:      common.GbToBytes(snapshot.DiskSizeGb),
		SnapshotId:     cleanSelfLink(snapshot.SelfLink),
		SourceVolumeId: volumeID,
		CreationTime:   timestamp,
		ReadyToUse:     ready,
	}, nil
}

func (gceCS *GCEControllerServer) createImage(ctx context.Context, project string, volKey *meta.Key, imageName string, snapshotParams common.SnapshotParameters) (*csi.Snapshot, error) {
	volumeID, err := common.KeyToVolumeID(volKey, project)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid volume key: %v", volKey)
	}

	// Check if image already exists
	var image *compute.Image
	image, err = gceCS.CloudProvider.GetImage(ctx, project, imageName)
	if err != nil {
		if !gce.IsGCEError(err, "notFound") {
			return nil, common.LoggedError("Failed to get image: ", err)
		}
		// create a new image
		image, err = gceCS.CloudProvider.CreateImage(ctx, project, volKey, imageName, snapshotParams)
		if err != nil {
			if gce.IsGCEError(err, "notFound") {
				return nil, status.Errorf(codes.NotFound, "Could not find volume with ID %v: %v", volKey.String(), err.Error())
			}
			return nil, common.LoggedError("Failed to create image: ", err)
		}
	}

	err = gceCS.validateExistingImage(image, volKey)
	if err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "Error in creating snapshot: %v", err.Error())
	}

	timestamp, err := parseTimestamp(image.CreationTimestamp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to covert creation timestamp: %v", err.Error())
	}

	ready, err := isImageReady(image.Status)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to check image status: %v", err.Error())
	}

	return &csi.Snapshot{
		SizeBytes:      common.GbToBytes(image.DiskSizeGb),
		SnapshotId:     cleanSelfLink(image.SelfLink),
		SourceVolumeId: volumeID,
		CreationTime:   timestamp,
		ReadyToUse:     ready,
	}, nil
}

func (gceCS *GCEControllerServer) validateExistingImage(image *compute.Image, volKey *meta.Key) error {
	if image == nil {
		return fmt.Errorf("disk does not exist")
	}

	_, sourceKey, err := common.VolumeIDToKey(cleanSelfLink(image.SourceDisk))
	if err != nil {
		return fmt.Errorf("fail to get source disk key %s, %w", image.SourceDisk, err)
	}

	if sourceKey.String() != volKey.String() {
		return fmt.Errorf("image already exists with same name but with a different disk source %s, expected disk source %s", sourceKey.String(), volKey.String())
	}

	klog.V(5).Infof("Compatible image %s exists with source disk %s.", image.Name, image.SourceDisk)
	return nil
}

func parseTimestamp(creationTimestamp string) (*timestamp.Timestamp, error) {
	t, err := time.Parse(time.RFC3339, creationTimestamp)
	if err != nil {
		return nil, err
	}

	timestamp, err := ptypes.TimestampProto(t)
	if err != nil {
		return nil, err
	}
	return timestamp, nil
}
func isImageReady(status string) (bool, error) {
	// Possible status:
	//   "DELETING"
	//   "FAILED"
	//   "PENDING"
	//   "READY"
	switch status {
	case "DELETING":
		klog.V(4).Infof("image status is DELETING")
		return true, nil
	case "FAILED":
		return false, fmt.Errorf("image status is FAILED")
	case "PENDING":
		return false, nil
	case "READY":
		return true, nil
	default:
		return false, fmt.Errorf("unknown image status %s", status)
	}
}

func (gceCS *GCEControllerServer) validateExistingSnapshot(snapshot *compute.Snapshot, volKey *meta.Key) error {
	if snapshot == nil {
		return fmt.Errorf("disk does not exist")
	}

	_, sourceKey, err := common.VolumeIDToKey(cleanSelfLink(snapshot.SourceDisk))
	if err != nil {
		return fmt.Errorf("fail to get source disk key %s, %w", snapshot.SourceDisk, err)
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
	var err error
	diskTypeForMetric := ""
	defer func() {
		if err != nil {
			gceCS.Metrics.RecordOperationErrorMetrics("DeleteSnapshot", err, diskTypeForMetric)
		}
	}()
	// Validate arguments
	snapshotID := req.GetSnapshotId()
	if len(snapshotID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeleteSnapshot Snapshot ID must be provided")
	}

	project, snapshotType, key, err := common.SnapshotIDToProjectKey(snapshotID)
	if err != nil {
		// Cannot get snapshot ID from the passing request
		// This is a success according to the spec
		klog.Warningf("Snapshot id does not have the correct format %s", snapshotID)
		return &csi.DeleteSnapshotResponse{}, nil
	}

	switch snapshotType {
	case common.DiskSnapshotType:
		err = gceCS.CloudProvider.DeleteSnapshot(ctx, project, key)
		if err != nil {
			return nil, common.LoggedError("Failed to DeleteSnapshot: ", err)
		}
	case common.DiskImageType:
		err = gceCS.CloudProvider.DeleteImage(ctx, project, key)
		if err != nil {
			return nil, common.LoggedError("Failed to DeleteImage error: ", err)
		}
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unknown snapshot type %s", snapshotType)
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

func (gceCS *GCEControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	// case 1: SnapshotId is not empty, return snapshots that match the snapshot id.
	if len(req.GetSnapshotId()) != 0 {
		return gceCS.getSnapshotByID(ctx, req.GetSnapshotId())
	}

	// case 2: no SnapshotId is set, so we return all the snapshots that satify the reqeust.
	var maxEntries int = int(req.MaxEntries)
	if maxEntries < 0 {
		return nil, status.Errorf(codes.InvalidArgument,
			"ListSnapshots got max entries request %v. GCE only supports values >0", maxEntries)
	}

	var offset int
	var length int
	var ok bool
	var nextToken string
	if req.StartingToken == "" {
		snapshotList, err := gceCS.getSnapshots(ctx, req)
		if err != nil {
			if gce.IsGCEInvalidError(err) {
				return nil, status.Errorf(codes.Aborted, "ListSnapshots error with invalid request: %v", err.Error())
			}
			return nil, common.LoggedError("Failed to list snapshots: ", err)
		}
		gceCS.snapshots = snapshotList
		gceCS.snapshotTokens = map[string]int{}
	} else {
		offset, ok = gceCS.snapshotTokens[req.StartingToken]
		if !ok {
			return nil, status.Errorf(codes.Aborted, "ListSnapshots error with invalid startingToken: %s", req.StartingToken)
		}
	}

	if maxEntries == 0 {
		maxEntries = len(gceCS.snapshots)
	}
	if maxEntries < len(gceCS.snapshots)-offset {
		length = maxEntries
		nextToken = string(uuid.NewUUID())
		gceCS.snapshotTokens[nextToken] = length + offset
	} else {
		length = len(gceCS.snapshots) - offset
	}

	return &csi.ListSnapshotsResponse{
		Entries:   gceCS.snapshots[offset : offset+length],
		NextToken: nextToken,
	}, nil
}

func (gceCS *GCEControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	var err error
	diskTypeForMetric := ""
	defer func() {
		if err != nil {
			gceCS.Metrics.RecordOperationErrorMetrics("ControllerExpandVolume", err, diskTypeForMetric)
		}
	}()
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerExpandVolume volume ID must be provided")
	}
	capacityRange := req.GetCapacityRange()
	reqBytes, err := getRequestCapacity(capacityRange)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "ControllerExpandVolume capacity range is invalid: %v", err.Error())
	}

	project, volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "ControllerExpandVolume Volume ID is invalid: %v", err.Error())
	}
	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "ControllerExpandVolume could not find volume with ID %v: %v", volumeID, err.Error())
		}
		return nil, common.LoggedError("ControllerExpandVolume error repairing underspecified volume key: ", err)
	}
	sourceDisk, err := gceCS.CloudProvider.GetDisk(ctx, project, volKey, gce.GCEAPIVersionV1)
	diskTypeForMetric = metrics.GetDiskType(sourceDisk)
	resizedGb, err := gceCS.CloudProvider.ResizeDisk(ctx, project, volKey, reqBytes)
	if err != nil {
		return nil, common.LoggedError("ControllerExpandVolume failed to resize disk: ", err)
	}

	klog.V(4).Infof("ControllerExpandVolume succeeded for disk %v to size %v", volKey, resizedGb)
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         common.GbToBytes(resizedGb),
		NodeExpansionRequired: true,
	}, nil
}

func (gceCS *GCEControllerServer) getSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) ([]*csi.ListSnapshotsResponse_Entry, error) {
	var snapshots []*compute.Snapshot
	var images []*compute.Image
	var filter string
	var err error
	if len(req.GetSourceVolumeId()) != 0 {
		filter = fmt.Sprintf("sourceDisk eq .*%s$", req.SourceVolumeId)
	}
	snapshots, _, err = gceCS.CloudProvider.ListSnapshots(ctx, filter)
	if err != nil {
		if gce.IsGCEError(err, "invalid") {
			return nil, status.Errorf(codes.Aborted, "Invalid error: %v", err.Error())
		}
		return nil, common.LoggedError("Failed to list snapshot: ", err)
	}

	images, _, err = gceCS.CloudProvider.ListImages(ctx, filter)
	if err != nil {
		if gce.IsGCEError(err, "invalid") {
			return nil, status.Errorf(codes.Aborted, "Invalid error: %v", err.Error())
		}
		return nil, common.LoggedError("Failed to list image: ", err)
	}

	entries := []*csi.ListSnapshotsResponse_Entry{}

	for _, snapshot := range snapshots {
		entry, err := generateDiskSnapshotEntry(snapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to generate snapshot entry: %w", err)
		}
		entries = append(entries, entry)
	}

	for _, image := range images {
		entry, err := generateDiskImageEntry(image)
		if err != nil {
			return nil, fmt.Errorf("failed to generate image entry: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (gceCS *GCEControllerServer) getSnapshotByID(ctx context.Context, snapshotID string) (*csi.ListSnapshotsResponse, error) {
	project, snapshotType, key, err := common.SnapshotIDToProjectKey(snapshotID)
	if err != nil {
		// Cannot get snapshot ID from the passing request
		klog.Warningf("invalid snapshot id format %s", snapshotID)
		return &csi.ListSnapshotsResponse{}, nil
	}

	var entries []*csi.ListSnapshotsResponse_Entry
	switch snapshotType {
	case common.DiskSnapshotType:
		snapshot, err := gceCS.CloudProvider.GetSnapshot(ctx, project, key)
		if err != nil {
			if gce.IsGCEError(err, "notFound") {
				// return empty list if no snapshot is found
				return &csi.ListSnapshotsResponse{}, nil
			}
			return nil, common.LoggedError("Failed to list snapshot: ", err)
		}
		e, err := generateDiskSnapshotEntry(snapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to generate snapshot entry: %w", err)
		}
		entries = []*csi.ListSnapshotsResponse_Entry{e}
	case common.DiskImageType:
		image, err := gceCS.CloudProvider.GetImage(ctx, project, key)
		if err != nil {
			if gce.IsGCEError(err, "notFound") {
				// return empty list if no snapshot is found
				return &csi.ListSnapshotsResponse{}, nil
			}
		}
		e, err := generateImageEntry(image)
		if err != nil {
			return nil, fmt.Errorf("failed to generate image entry: %w", err)
		}
		entries = []*csi.ListSnapshotsResponse_Entry{e}
	}

	//entries[0] = entry
	listSnapshotResp := &csi.ListSnapshotsResponse{
		Entries: entries,
	}
	return listSnapshotResp, nil
}

func generateDiskSnapshotEntry(snapshot *compute.Snapshot) (*csi.ListSnapshotsResponse_Entry, error) {
	t, _ := time.Parse(time.RFC3339, snapshot.CreationTimestamp)

	tp := timestamppb.New(t)
	if err := tp.CheckValid(); err != nil {
		return nil, fmt.Errorf("Failed to covert creation timestamp: %w", err)
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

func generateDiskImageEntry(image *compute.Image) (*csi.ListSnapshotsResponse_Entry, error) {
	t, _ := time.Parse(time.RFC3339, image.CreationTimestamp)

	tp := timestamppb.New(t)
	if err := tp.CheckValid(); err != nil {
		return nil, fmt.Errorf("failed to covert creation timestamp: %w", err)
	}

	ready, _ := isImageReady(image.Status)

	entry := &csi.ListSnapshotsResponse_Entry{
		Snapshot: &csi.Snapshot{
			SizeBytes:      common.GbToBytes(image.DiskSizeGb),
			SnapshotId:     cleanSelfLink(image.SelfLink),
			SourceVolumeId: cleanSelfLink(image.SourceDisk),
			CreationTime:   tp,
			ReadyToUse:     ready,
		},
	}
	return entry, nil
}

func generateImageEntry(image *compute.Image) (*csi.ListSnapshotsResponse_Entry, error) {
	timestamp, err := parseTimestamp(image.CreationTimestamp)
	if err != nil {
		return nil, fmt.Errorf("Failed to covert creation timestamp: %w", err)
	}

	// ignore the error intentionally here since we are just listing images
	ready, _ := isImageReady(image.Status)

	entry := &csi.ListSnapshotsResponse_Entry{
		Snapshot: &csi.Snapshot{
			SizeBytes:      common.GbToBytes(image.DiskSizeGb),
			SnapshotId:     cleanSelfLink(image.SelfLink),
			SourceVolumeId: cleanSelfLink(image.SourceDisk),
			CreationTime:   timestamp,
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

// pickZonesInRegion will remove any zones that are not in the given region.
func pickZonesInRegion(region string, zones []string) []string {
	refinedZones := []string{}
	for _, zone := range zones {
		if strings.Contains(zone, region) {
			refinedZones = append(refinedZones, zone)
		}
	}
	return refinedZones
}

func prependZone(zone string, zones []string) []string {
	newZones := []string{zone}
	for i := 0; i < len(zones); i++ {
		// Do not add a zone if it is equal to the zone that is already prepended to newZones.
		if zones[i] != zone {
			newZones = append(newZones, zones[i])
		}
	}
	return newZones
}

func pickZonesFromTopology(top *csi.TopologyRequirement, numZones int, locationTopReq *locationRequirements) ([]string, error) {
	reqZones, err := getZonesFromTopology(top.GetRequisite())
	if err != nil {
		return nil, fmt.Errorf("could not get zones from requisite topology: %w", err)
	}
	prefZones, err := getZonesFromTopology(top.GetPreferred())
	if err != nil {
		return nil, fmt.Errorf("could not get zones from preferred topology: %w", err)
	}

	if locationTopReq != nil {
		srcVolZone := locationTopReq.srcVolZone
		switch locationTopReq.cloneReplicationType {
		// For zonal -> zonal cloning, the source disk zone must match the destination disk zone.
		case replicationTypeNone:
			// If the source volume zone is not in the topology requirement, we return an error.
			if !slices.Contains(prefZones, srcVolZone) && !slices.Contains(reqZones, srcVolZone) {
				volumeCloningReq := fmt.Sprintf("clone zone must match source disk zone: %s", srcVolZone)
				return nil, fmt.Errorf("failed to find zone from topology %v: %s", top, volumeCloningReq)
			}
			return []string{srcVolZone}, nil
		// For zonal or regional -> regional disk cloning, the source disk region must match the destination disk region.
		case replicationTypeRegionalPD:
			srcVolRegion := locationTopReq.srcVolRegion
			prefZones = pickZonesInRegion(srcVolRegion, prefZones)
			reqZones = pickZonesInRegion(srcVolRegion, reqZones)

			if len(prefZones) == 0 && len(reqZones) == 0 {
				volumeCloningReq := fmt.Sprintf("clone zone must reside in source disk region %s", srcVolRegion)
				return nil, fmt.Errorf("failed to find zone from topology %v: %s", top, volumeCloningReq)
			}

			// For zonal -> regional disk cloning, one of the replicated zones must match the source zone.
			if locationTopReq.srcReplicationType == replicationTypeNone {
				if !slices.Contains(prefZones, srcVolZone) && !slices.Contains(reqZones, srcVolZone) {
					volumeCloningReq := fmt.Sprintf("one of the replica zones of the clone must match the source disk zone: %s", srcVolZone)
					return nil, fmt.Errorf("failed to find zone from topology %v: %s", top, volumeCloningReq)
				}
				prefZones = prependZone(srcVolZone, prefZones)
			}
		}
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
			return nil, fmt.Errorf("could not get zone from preferred topology: %w", err)
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

func pickZones(ctx context.Context, gceCS *GCEControllerServer, top *csi.TopologyRequirement, numZones int, locationTopReq *locationRequirements) ([]string, error) {
	var zones []string
	var err error
	if top != nil {
		zones, err = pickZonesFromTopology(top, numZones, locationTopReq)
		if err != nil {
			return nil, fmt.Errorf("failed to pick zones from topology: %w", err)
		}
	} else {
		existingZones := []string{gceCS.CloudProvider.GetDefaultZone()}
		// We set existingZones to the source volume zone so that for zonal -> zonal cloning, the clone is provisioned
		// in the same zone as the source volume, and for zonal -> regional, one of the replicated zones will always
		// be the zone of the source volume. For regional -> regional cloning, the srcVolZone will not be set, so we
		// just use the default zone.
		if locationTopReq != nil && locationTopReq.srcReplicationType == replicationTypeNone {
			existingZones = []string{locationTopReq.srcVolZone}
		}
		// If topology is nil, then the Immediate binding mode was used without setting allowedTopologies in the storageclass.
		zones, err = getDefaultZonesInRegion(ctx, gceCS, existingZones, numZones)
		if err != nil {
			return nil, fmt.Errorf("failed to get default %v zones in region: %w", numZones, err)
		}
		klog.Warningf("No zones have been specified in either topology or params, picking default zone: %v", zones)

	}
	return zones, nil
}

func getDefaultZonesInRegion(ctx context.Context, gceCS *GCEControllerServer, existingZones []string, numZones int) ([]string, error) {
	region, err := common.GetRegionFromZones(existingZones)
	if err != nil {
		return nil, fmt.Errorf("failed to get region from zones: %w", err)
	}
	needToGet := numZones - len(existingZones)
	totZones, err := gceCS.CloudProvider.ListZones(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("failed to list zones from cloud provider: %w", err)
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
	imageID := disk.GetImageId()
	diskID := disk.GetSourceDiskId()
	if diskID != "" || snapshotID != "" || imageID != "" {
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
		if imageID != "" {
			contentSource = &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{
					Snapshot: &csi.VolumeContentSource_SnapshotSource{
						SnapshotId: imageID,
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
		return nil, fmt.Errorf("failed to get region from zones: %w", err)
	}

	fullyQualifiedReplicaZones := []string{}
	for _, replicaZone := range zones {
		fullyQualifiedReplicaZones = append(
			fullyQualifiedReplicaZones, cloudProvider.GetReplicaZoneURI(project, replicaZone))
	}

	err = cloudProvider.InsertDisk(ctx, project, meta.RegionalKey(name, region), params, capBytes, capacityRange, fullyQualifiedReplicaZones, snapshotID, volumeContentSourceVolumeID, multiWriter)
	if err != nil {
		return nil, fmt.Errorf("failed to insert regional disk: %w", err)
	}

	gceAPIVersion := gce.GCEAPIVersionV1
	if multiWriter {
		gceAPIVersion = gce.GCEAPIVersionBeta
	}

	disk, err := cloudProvider.GetDisk(ctx, project, meta.RegionalKey(name, region), gceAPIVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk after creating regional disk: %w", err)
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
		return nil, fmt.Errorf("failed to insert zonal disk: %w", err)
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

func newCsiErrorBackoff() *csiErrorBackoff {
	return &csiErrorBackoff{flowcontrol.NewBackOff(errorBackoffInitialDuration, errorBackoffMaxDuration)}
}

func (_ *csiErrorBackoff) backoffId(nodeId, volumeId string) csiErrorBackoffId {
	return csiErrorBackoffId(fmt.Sprintf("%s:%s", nodeId, volumeId))
}

func (b *csiErrorBackoff) blocking(id csiErrorBackoffId) bool {
	blk := b.backoff.IsInBackOffSinceUpdate(string(id), b.backoff.Clock.Now())
	return blk
}

func (b *csiErrorBackoff) next(id csiErrorBackoffId) {
	b.backoff.Next(string(id), b.backoff.Clock.Now())
}

func (b *csiErrorBackoff) reset(id csiErrorBackoffId) {
	b.backoff.Reset(string(id))
}
