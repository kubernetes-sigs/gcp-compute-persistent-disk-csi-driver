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
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"

	"context"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	csi "github.com/container-storage-interface/spec/lib/go/csi"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	metadataservice "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/metadata"
)

type GCEControllerServer struct {
	Driver          *GCEDriver
	CloudProvider   gce.GCECompute
	MetadataService metadataservice.MetadataService
}

var _ csi.ControllerServer = &GCEControllerServer{}

const (
	// MaxVolumeSizeInBytes is the maximum standard and ssd size of 64TB
	MaxVolumeSizeInBytes     int64 = 64 * 1024 * 1024 * 1024 * 1024
	MinimumVolumeSizeInBytes int64 = 1 * 1024 * 1024 * 1024
	MinimumDiskSizeInGb            = 1

	DiskTypeSSD      = "pd-ssd"
	DiskTypeStandard = "pd-standard"
	diskTypeDefault  = DiskTypeStandard

	attachableDiskTypePersistent = "PERSISTENT"

	replicationTypeNone       = "none"
	replicationTypeRegionalPD = "regional-pd"
)

func (gceCS *GCEControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	var err error
	klog.V(4).Infof("CreateVolume called with request %v", *req)

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
	diskType := "pd-standard"
	// Start process for creating a new disk
	replicationType := replicationTypeNone
	diskEncryptionKmsKey := ""
	for k, v := range req.GetParameters() {
		if k == "csiProvisionerSecretName" || k == "csiProvisionerSecretNamespace" {
			// These are hardcoded secrets keys required to function but not needed by GCE PD
			continue
		}
		switch strings.ToLower(k) {
		case common.ParameterKeyType:
			klog.V(4).Infof("Setting type: %v", v)
			diskType = v
		case common.ParameterKeyReplicationType:
			replicationType = strings.ToLower(v)
		case common.ParameterKeyDiskEncryptionKmsKey:
			// Resource names (e.g. "keyRings", "cryptoKeys", etc.) are case sensitive, so do not change case
			diskEncryptionKmsKey = v
		default:
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume invalid option %q", k))
		}
	}
	// Determine the zone or zones+region of the disk
	var zones []string
	var volKey *meta.Key
	switch replicationType {
	case replicationTypeNone:
		zones, err = pickZones(gceCS, req.GetAccessibilityRequirements(), 1)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume failed to pick zones for disk: %v", err))
		}
		if len(zones) != 1 {
			return nil, status.Errorf(codes.Internal, fmt.Sprintf("Failed to pick exactly 1 zone for zonal disk, got %v instead", len(zones)))
		}
		volKey = meta.ZonalKey(name, zones[0])

	case replicationTypeRegionalPD:
		zones, err = pickZones(gceCS, req.GetAccessibilityRequirements(), 2)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume failed to pick zones for disk: %v", err))
		}
		region, err := common.GetRegionFromZones(zones)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume failed to get region from zones: %v", err))
		}
		volKey = meta.RegionalKey(name, region)
	default:
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume replication type '%s' is not supported", replicationType))
	}

	// Validate if disk already exists
	existingDisk, err := gceCS.CloudProvider.GetDisk(ctx, volKey)
	if err != nil {
		if !gce.IsGCEError(err, "notFound") {
			return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume unknown get disk error when validating: %v", err))
		}
	}
	if err == nil {
		// There was no error so we want to validate the disk that we find
		err = gceCS.CloudProvider.ValidateExistingDisk(ctx, existingDisk, diskType,
			int64(capacityRange.GetRequiredBytes()),
			int64(capacityRange.GetLimitBytes()))
		if err != nil {
			return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("CreateVolume disk already exists with same name and is incompatible: %v", err))
		}
		// If there is no validation error, immediately return success
		return generateCreateVolumeResponse(existingDisk, capBytes, zones), nil
	}

	snapshotId := ""
	content := req.GetVolumeContentSource()
	if content != nil {
		if content.GetSnapshot() != nil {
			// TODO(#161): Add support for Volume Source (cloning) introduced in CSI v1.0.0
			snapshotId = content.GetSnapshot().GetSnapshotId()

			// Verify that snapshot exists
			sl, err := gceCS.getSnapshotById(ctx, snapshotId)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "CreateVolume failed to get snapshot %s: %v", snapshotId, err)
			} else if len(sl.Entries) == 0 {
				return nil, status.Errorf(codes.NotFound, "CreateVolume source snapshot %s does not exist", snapshotId)
			}
		}
	}

	// Create the disk
	var disk *gce.CloudDisk
	switch replicationType {
	case replicationTypeNone:
		if len(zones) != 1 {
			return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume failed to get a single zone for creating zonal disk, instead got: %v", zones))
		}
		disk, err = createSingleZoneDisk(ctx, gceCS.CloudProvider, name, zones, diskType, capacityRange, capBytes, snapshotId, diskEncryptionKmsKey)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume failed to create single zonal disk %#v: %v", name, err))
		}
	case replicationTypeRegionalPD:
		if len(zones) != 2 {
			return nil, status.Errorf(codes.Internal, fmt.Sprintf("CreateVolume failed to get a 2 zones for creating regional disk, instead got: %v", zones))
		}
		disk, err = createRegionalDisk(ctx, gceCS.CloudProvider, name, zones, diskType, capacityRange, capBytes, snapshotId, diskEncryptionKmsKey)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume failed to create regional disk %#v: %v", name, err))
		}
	default:
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume replication type '%s' is not supported", replicationType))
	}
	return generateCreateVolumeResponse(disk, capBytes, zones), nil

}

func (gceCS *GCEControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.V(4).Infof("DeleteVolume called with request %v", *req)

	// Validate arguments
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume Volume ID must be provided")
	}

	volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		// Cannot find volume associated with this ID because can't even get the name or zone
		// This is a success according to the spec
		return &csi.DeleteVolumeResponse{}, nil
	}

	volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, volKey)
	if err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("Could not find volume with ID %v: %v", volumeID, err))
	}

	err = gceCS.CloudProvider.DeleteDisk(ctx, volKey)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown Delete disk error: %v", err))
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (gceCS *GCEControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	klog.V(4).Infof("ControllerPublishVolume called with request %v", *req)

	// Validate arguments
	volumeID := req.GetVolumeId()
	readOnly := req.GetReadonly()
	nodeID := req.GetNodeId()
	volumeCapability := req.GetVolumeCapability()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume ID must be provided")
	}
	if len(nodeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Node ID must be provided")
	}
	if volumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume capability must be provided")
	}

	volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("Could not find volume with ID %v: %v", volumeID, err))
	}

	volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, volKey)
	if err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("Could not find volume with ID %v: %v", volumeID, err))
	}

	// TODO(#253): Check volume capability matches for ALREADY_EXISTS
	if err = validateVolumeCapability(volumeCapability); err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("VolumeCapabilities is invalid: %v", err))
	}

	pubVolResp := &csi.ControllerPublishVolumeResponse{
		PublishContext: nil,
	}

	_, err = gceCS.CloudProvider.GetDisk(ctx, volKey)
	if err != nil {
		if gce.IsGCEError(err, "notFound") {
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
		if gce.IsGCEError(err, "notFound") {
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
		klog.V(4).Infof("Attach operation is successful. PD %q was already attached to node %q.", volKey.Name, nodeID)
		return pubVolResp, nil
	}
	instanceZone, instanceName, err = common.NodeIDToZoneAndName(nodeID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("could not split nodeID: %v", err))
	}
	err = gceCS.CloudProvider.AttachDisk(ctx, volKey, readWrite, attachableDiskTypePersistent, instanceZone, instanceName)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown Attach error: %v", err))
	}

	klog.V(4).Infof("Waiting for attach of disk %v to instance %v to complete...", volKey.Name, nodeID)

	err = gceCS.CloudProvider.WaitForAttach(ctx, volKey, instanceZone, instanceName)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown WaitForAttach error: %v", err))
	}

	klog.V(4).Infof("Disk %v attached to instance %v successfully", volKey.Name, nodeID)
	return pubVolResp, nil
}

func (gceCS *GCEControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	klog.V(4).Infof("ControllerUnpublishVolume called with request %v", *req)

	// Validate arguments
	volumeID := req.GetVolumeId()
	nodeID := req.GetNodeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume Volume ID must be provided")
	}
	if len(nodeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume Node ID must be provided")
	}

	volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return nil, err
	}

	instanceZone, instanceName, err := common.NodeIDToZoneAndName(nodeID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("could not split nodeID: %v", err))
	}
	instance, err := gceCS.CloudProvider.GetInstanceOrError(ctx, instanceZone, instanceName)
	if err != nil {
		return nil, err
	}

	deviceName, err := common.GetDeviceName(volKey)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("error getting device name: %v", err))
	}

	attached := diskIsAttached(deviceName, instance)

	if !attached {
		// Volume is not attached to node. Success!
		klog.V(4).Infof("Detach operation is successful. PD %q was not attached to node %q.", volKey.Name, nodeID)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	err = gceCS.CloudProvider.DetachDisk(ctx, deviceName, instanceZone, instanceName)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown detach error: %v", err))
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (gceCS *GCEControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	// TODO(#162): Implement ValidateVolumeCapabilities

	klog.V(5).Infof("Using default ValidateVolumeCapabilities")
	// Validate Arguments
	if req.GetVolumeCapabilities() == nil || len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume Capabilities must be provided")
	}
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume ID must be provided")
	}
	volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("Volume ID is of improper format, got %v", volumeID))
	}
	_, err = gceCS.CloudProvider.GetDisk(ctx, volKey)
	if err != nil {
		if gce.IsGCEError(err, "notFound") {
			return nil, status.Error(codes.NotFound, fmt.Sprintf("Could not find disk %v: %v", volKey.Name, err))
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("Unknown get disk error: %v", err))
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Message: "ValidateVolumeCapabilities is currently unimplemented for CSI v1.0.0",
	}, nil
	/*
		for _, c := range req.GetVolumeCapabilities() {
			found := false
			for _, c1 := range gceCS.Driver.vcap {
				if c1.Mode == c.GetAccessMode().Mode {
					found = true
				}
			}
			if !found {
				return &csi.ValidateVolumeCapabilitiesResponse{
					Supported: false,
					Message:   "Driver does not support mode:" + c.GetAccessMode().Mode.String(),
				}, status.Error(codes.InvalidArgument, "Driver does not support mode:"+c.GetAccessMode().Mode.String())
			}
			// TODO: Ignoring mount & block types for now.
		}

		for _, top := range req.GetAccessibleTopology() {
			for k, v := range top.GetSegments() {
				switch k {
				case common.TopologyKeyZone:
					switch volKey.Type() {
					case meta.Zonal:
						if v == volKey.Zone {
							// Accessible zone matches with storage zone
							return &csi.ValidateVolumeCapabilitiesResponse{
								Supported: true,
							}, nil
						}
					case meta.Regional:
						// TODO: This should more accurately check the disks replica Zones but that involves
						// GET-ing the disk
						region, err := common.GetRegionFromZones([]string{v})
						if err != nil {
							return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("ValidateVolumeCapabilities could not extract topology region from zone %v: %v", v, err))
						}
						if region == volKey.Region {
							// Accessible region matches with storage region
							return &csi.ValidateVolumeCapabilitiesResponse{
								Supported: true,
							}, nil
						}
					default:
						// Accessible zone does not match
						return &csi.ValidateVolumeCapabilitiesResponse{
							Supported: false,
							Message:   fmt.Sprintf("Volume %s is not accesible from topology %s:%s", volumeID, k, v),
						}, nil
					}
				default:
					return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities unknown topology segment key")
				}
			}
		}

		return &csi.ValidateVolumeCapabilitiesResponse{
			Supported: true,
		}, nil
	*/
}

func (gceCS *GCEControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	// https://cloud.google.com/compute/docs/reference/beta/disks/list
	// List volumes in the whole region? In only the zone that this controller is running?
	return nil, status.Error(codes.Unimplemented, "")
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
	klog.V(4).Infof("CreateSnapshot called with request %v", *req)

	// Validate arguments
	volumeID := req.GetSourceVolumeId()
	if len(req.Name) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Snapshot name must be provided")
	}
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateSnapshot Source Volume ID must be provided")
	}
	volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("Could not find volume with ID %v: %v", volumeID, err))
	}

	// Check if snapshot already exists
	var snapshot *compute.Snapshot
	snapshot, err = gceCS.CloudProvider.GetSnapshot(ctx, req.Name)
	if err != nil {
		if !gce.IsGCEError(err, "notFound") {
			return nil, status.Error(codes.Internal, fmt.Sprintf("Unknown get snapshot error: %v", err))
		}
		// If we could not find the snapshot, we create a new one
		snapshot, err = gceCS.CloudProvider.CreateSnapshot(ctx, volKey, req.Name)
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
	return createResp, nil
}

func (gceCS *GCEControllerServer) validateExistingSnapshot(snapshot *compute.Snapshot, volKey *meta.Key) error {
	if snapshot == nil {
		return fmt.Errorf("disk does not exist")
	}

	sourceKey, err := common.VolumeIDToKey(cleanSelfLink(snapshot.SourceDisk))
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
	klog.V(4).Infof("DeleteSnapshot called with request %v", *req)

	// Validate arguments
	snapshotID := req.GetSnapshotId()
	if len(snapshotID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeleteSnapshot Snapshot ID must be provided")
	}

	key, err := common.SnapshotIDToKey(snapshotID)
	if err != nil {
		// Cannot get snapshot ID from the passing request
		// This is a success according to the spec
		klog.Warningf("Snapshot id does not have the correct format %s", snapshotID)
		return &csi.DeleteSnapshotResponse{}, nil
	}

	err = gceCS.CloudProvider.DeleteSnapshot(ctx, key)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown Delete snapshot error: %v", err))
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

func (gceCS *GCEControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	klog.V(4).Infof("ListSnapshots called with request %v", *req)

	// case 1: SnapshotId is not empty, return snapshots that match the snapshot id.
	if len(req.GetSnapshotId()) != 0 {
		return gceCS.getSnapshotById(ctx, req.GetSnapshotId())
	}

	// case 2: no SnapshotId is set, so we return all the snapshots that satify the reqeust.
	return gceCS.getSnapshots(ctx, req)
}

func (gceCS *GCEControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerExpandVolume is not yet implemented")
}

func (gceCS *GCEControllerServer) getSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	snapshots := []*compute.Snapshot{}
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

func (gceCS *GCEControllerServer) getSnapshotById(ctx context.Context, snapshotId string) (*csi.ListSnapshotsResponse, error) {
	key, err := common.SnapshotIDToKey(snapshotId)
	if err != nil {
		// Cannot get snapshot ID from the passing request
		klog.Warningf("invalid snapshot id format %s", snapshotId)
		return &csi.ListSnapshotsResponse{}, nil
	}

	snapshot, err := gceCS.CloudProvider.GetSnapshot(ctx, key)
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

func pickZones(gceCS *GCEControllerServer, top *csi.TopologyRequirement, numZones int) ([]string, error) {
	var zones []string
	var err error
	if top != nil {
		zones, err = pickZonesFromTopology(top, numZones)
		if err != nil {
			return nil, fmt.Errorf("failed to pick zones from topology: %v", err)
		}
	} else {
		zones, err = getDefaultZonesInRegion(gceCS, []string{gceCS.MetadataService.GetZone()}, numZones)
		if err != nil {
			return nil, fmt.Errorf("failed to get default %v zones in region: %v", numZones, err)
		}
		klog.Warningf("No zones have been specified in either topology or params, picking default zone: %v", zones)

	}
	return zones, nil
}

func getDefaultZonesInRegion(gceCS *GCEControllerServer, existingZones []string, numZones int) ([]string, error) {
	region, err := common.GetRegionFromZones(existingZones)
	if err != nil {
		return nil, fmt.Errorf("failed to get region from zones: %v", err)
	}
	needToGet := numZones - len(existingZones)
	totZones, err := gceCS.CloudProvider.ListZones(context.Background(), region)
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

func generateCreateVolumeResponse(disk *gce.CloudDisk, capBytes int64, zones []string) *csi.CreateVolumeResponse {
	tops := []*csi.Topology{}
	for _, zone := range zones {
		tops = append(tops, &csi.Topology{
			Segments: map[string]string{common.TopologyKeyZone: zone},
		})
	}
	createResp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes:      capBytes,
			VolumeId:           cleanSelfLink(disk.GetSelfLink()),
			VolumeContext:      nil,
			AccessibleTopology: tops,
		},
	}
	snapshotId := disk.GetSnapshotId()
	if snapshotId != "" {
		source := &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Snapshot{
				Snapshot: &csi.VolumeContentSource_SnapshotSource{
					SnapshotId: snapshotId,
				},
			},
		}
		createResp.Volume.ContentSource = source

	}
	return createResp
}

func cleanSelfLink(selfLink string) string {
	temp := strings.TrimPrefix(selfLink, gce.GCEComputeAPIEndpoint)
	return strings.TrimPrefix(temp, gce.GCEComputeBetaAPIEndpoint)
}

func createRegionalDisk(ctx context.Context, cloudProvider gce.GCECompute, name string, zones []string, diskType string, capacityRange *csi.CapacityRange, capBytes int64, snapshotId, diskEncryptionKmsKey string) (*gce.CloudDisk, error) {
	region, err := common.GetRegionFromZones(zones)
	if err != nil {
		return nil, fmt.Errorf("failed to get region from zones: %v", err)
	}

	fullyQualifiedReplicaZones := []string{}
	for _, replicaZone := range zones {
		fullyQualifiedReplicaZones = append(
			fullyQualifiedReplicaZones, cloudProvider.GetReplicaZoneURI(replicaZone))
	}

	err = cloudProvider.InsertDisk(ctx, meta.RegionalKey(name, region), diskType, capBytes, capacityRange, fullyQualifiedReplicaZones, snapshotId, diskEncryptionKmsKey)
	if err != nil {
		return nil, fmt.Errorf("failed to insert regional disk: %v", err)
	}

	klog.V(4).Infof("Completed creation of disk %v", name)
	disk, err := cloudProvider.GetDisk(ctx, meta.RegionalKey(name, region))
	if err != nil {
		return nil, fmt.Errorf("failed to get disk after creating regional disk: %v", err)
	}
	klog.Warningf("GCE PD %s already exists after wait, reusing", name)
	return disk, nil
}

func createSingleZoneDisk(ctx context.Context, cloudProvider gce.GCECompute, name string, zones []string, diskType string, capacityRange *csi.CapacityRange, capBytes int64, snapshotId, diskEncryptionKmsKey string) (*gce.CloudDisk, error) {
	if len(zones) != 1 {
		return nil, fmt.Errorf("got wrong number of zones for zonal create volume: %v", len(zones))
	}
	diskZone := zones[0]
	err := cloudProvider.InsertDisk(ctx, meta.ZonalKey(name, diskZone), diskType, capBytes, capacityRange, nil, snapshotId, diskEncryptionKmsKey)
	if err != nil {
		return nil, fmt.Errorf("failed to insert zonal disk: %v", err)
	}

	klog.V(4).Infof("Completed creation of disk %v", name)
	disk, err := cloudProvider.GetDisk(ctx, meta.ZonalKey(name, diskZone))
	if err != nil {
		return nil, err
	}
	klog.Warningf("GCE PD %s already exists after wait, reusing", name)
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
