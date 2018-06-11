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
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	gce "github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider"
	utils "github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pkg/utils"
	"golang.org/x/net/context"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TODO: Add noisy glog.V(5).Infof() EVERYWHERE
// TODO: Improve errors to only expose codes at top level
// TODO: Improve error prefix to explicitly state what function it is in.

type GCEControllerServer struct {
	Driver        *GCEDriver
	CloudProvider gce.GCECompute
}

var _ csi.ControllerServer = &GCEControllerServer{}

const (
	// MaxVolumeSizeInBytes is the maximum standard and ssd size of 64TB
	MaxVolumeSizeInBytes     int64 = 64 * 1024 * 1024 * 1024 * 1024
	MinimumVolumeSizeInBytes int64 = 5 * 1024 * 1024 * 1024
	MinimumDiskSizeInGb            = 5

	DiskTypeSSD      = "pd-ssd"
	DiskTypeStandard = "pd-standard"
	diskTypeDefault  = DiskTypeStandard

	attachableDiskTypePersistent = "PERSISTENT"
)

func getRequestCapacity(capRange *csi.CapacityRange) (capBytes int64) {
	// TODO: Take another look at these casts/caps. Make sure this func is correct
	if capRange == nil {
		capBytes = MinimumVolumeSizeInBytes
		return
	}

	if tcap := capRange.GetRequiredBytes(); tcap > 0 {
		capBytes = tcap
	} else if tcap = capRange.GetLimitBytes(); tcap > 0 {
		capBytes = tcap
	}
	// Too small, default
	if capBytes < MinimumVolumeSizeInBytes {
		capBytes = MinimumVolumeSizeInBytes
	}
	return
}

func (gceCS *GCEControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	// TODO: Check create zone against Driver zone. They must MATCH
	glog.Infof("CreateVolume called with request %v", *req)

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

	capBytes := getRequestCapacity(capacityRange)

	// TODO: Validate volume capabilities

	// TODO: Support replica zones and fs type. Can vendor in api-machinery stuff for sets etc.
	// Apply Parameters (case-insensitive). We leave validation of
	// the values to the cloud provider.
	diskType := "pd-standard"
	configuredZone := gceCS.CloudProvider.GetZone()
	for k, v := range req.GetParameters() {
		if k == "csiProvisionerSecretName" || k == "csiProvisionerSecretNamespace" {
			// These are hardcoded secrets keys required to function but not needed by GCE PD
			continue
		}
		switch strings.ToLower(k) {
		case "type":
			glog.Infof("Setting type: %v", v)
			diskType = v
		case "zone":
			configuredZone = v
		default:
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid option %q", k))
		}
	}

	createResp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes: capBytes,
			Id:            utils.CombineVolumeId(configuredZone, name),
			// TODO: Are there any attributes we need to add. These get sent to ControllerPublishVolume
			Attributes: nil,
		},
	}

	// Check for existing disk of same name in same zone
	exists, err := gceCS.CloudProvider.GetAndValidateExistingDisk(ctx, configuredZone,
		name, diskType,
		capacityRange.GetRequiredBytes(),
		capacityRange.GetLimitBytes())
	if err != nil {
		return nil, err
	}
	if exists {
		glog.Warningf("GCE PD %s already exists, reusing", name)
		return createResp, nil
	}

	sizeGb := utils.BytesToGb(capBytes)
	if sizeGb < MinimumDiskSizeInGb {
		sizeGb = MinimumDiskSizeInGb
	}
	diskToCreate := &compute.Disk{
		Name:        name,
		SizeGb:      sizeGb,
		Description: "Disk created by GCE-PD CSI Driver",
		Type:        gceCS.CloudProvider.GetDiskTypeURI(configuredZone, diskType),
	}

	insertOp, err := gceCS.CloudProvider.InsertDisk(ctx, configuredZone, diskToCreate)

	if err != nil {
		if gce.IsGCEError(err, "alreadyExists") {
			_, err := gceCS.CloudProvider.GetAndValidateExistingDisk(ctx, configuredZone,
				name, diskType,
				capacityRange.GetRequiredBytes(),
				capacityRange.GetLimitBytes())
			if err != nil {
				return nil, err
			}
			glog.Warningf("GCE PD %s already exists, reusing", name)
			return createResp, nil
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("unkown Insert disk error: %v", err))
	}

	err = gceCS.CloudProvider.WaitForOp(ctx, insertOp, configuredZone)

	if err != nil {
		if gce.IsGCEError(err, "alreadyExists") {
			_, err := gceCS.CloudProvider.GetAndValidateExistingDisk(ctx, configuredZone,
				name, diskType,
				capacityRange.GetRequiredBytes(),
				capacityRange.GetLimitBytes())
			if err != nil {
				return nil, err
			}
			glog.Warningf("GCE PD %s already exists after wait, reusing", name)
			return createResp, nil
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("unkown Insert disk operation error: %v", err))
	}

	glog.Infof("Completed creation of disk %v", name)
	return createResp, nil
}

func (gceCS *GCEControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	// TODO: Only allow deletion of volumes that were created by the driver
	// Assuming ID is of form {zone}/{id}
	glog.Infof("DeleteVolume called with request %v", *req)

	// Validate arguments
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume Volume ID must be provided")
	}

	zone, name, err := utils.SplitZoneNameId(volumeID)
	if err != nil {
		// Cannot find volume associated with this ID because can't even get the name or zone
		// This is a success according to the spec
		return &csi.DeleteVolumeResponse{}, nil
	}

	deleteOp, err := gceCS.CloudProvider.DeleteDisk(ctx, zone, name)
	if err != nil {
		if gce.IsGCEError(err, "resourceInUseByAnotherResource") {
			return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("Volume in use: %v", err))
		}
		if gce.IsGCEError(err, "notFound") {
			// Already deleted
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown Delete disk error: %v", err))
	}

	err = gceCS.CloudProvider.WaitForOp(ctx, deleteOp, zone)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown Delete disk operation error: %v", err))
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (gceCS *GCEControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	glog.Infof("ControllerPublishVolume called with request %v", *req)

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

	volumeZone, volumeName, err := utils.SplitZoneNameId(volumeID)
	if err != nil {
		return nil, err
	}

	// TODO: Check volume capability matches

	pubVolResp := &csi.ControllerPublishVolumeResponse{
		// TODO: Info gets sent to NodePublishVolume. Send something if necessary.
		PublishInfo: nil,
	}

	disk, err := gceCS.CloudProvider.GetDiskOrError(ctx, volumeZone, volumeName)
	if err != nil {
		return nil, err
	}
	instance, err := gceCS.CloudProvider.GetInstanceOrError(ctx, volumeZone, nodeID)
	if err != nil {
		return nil, err
	}

	readWrite := "READ_WRITE"
	if readOnly {
		readWrite = "READ_ONLY"
	}

	attached, err := diskIsAttachedAndCompatible(disk, instance, volumeCapability, readWrite)
	if err != nil {
		return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("Disk %v already published to node %v but incompatbile: %v", volumeName, nodeID, err))
	}
	if attached {
		// Volume is attached to node. Success!
		glog.Infof("Attach operation is successful. PD %q was already attached to node %q.", volumeName, nodeID)
		return pubVolResp, nil
	}

	source := gceCS.CloudProvider.GetDiskSourceURI(disk, volumeZone)

	attachedDiskV1 := &compute.AttachedDisk{
		DeviceName: disk.Name,
		Kind:       disk.Kind,
		Mode:       readWrite,
		Source:     source,
		Type:       attachableDiskTypePersistent,
	}

	glog.Infof("Attaching disk %#v to instance %v", attachedDiskV1, nodeID)
	attachOp, err := gceCS.CloudProvider.AttachDisk(ctx, volumeZone, nodeID, attachedDiskV1)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown Attach error: %v", err))
	}

	glog.Infof("Waiting for attach of disk %v to instance %v to complete...", disk.Name, nodeID)
	err = gceCS.CloudProvider.WaitForOp(ctx, attachOp, volumeZone)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown Attach operation error: %v", err))
	}

	glog.Infof("Disk %v attached to instance %v successfully", disk.Name, nodeID)
	return pubVolResp, nil
}

func (gceCS *GCEControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	glog.Infof("ControllerUnpublishVolume called with request %v", *req)

	// Validate arguments
	volumeID := req.GetVolumeId()
	nodeID := req.GetNodeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume Volume ID must be provided")
	}
	if len(nodeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume Node ID must be provided")
	}

	volumeZone, volumeName, err := utils.SplitZoneNameId(volumeID)
	if err != nil {
		return nil, err
	}

	disk, err := gceCS.CloudProvider.GetDiskOrError(ctx, volumeZone, volumeName)
	if err != nil {
		return nil, err
	}
	instance, err := gceCS.CloudProvider.GetInstanceOrError(ctx, volumeZone, nodeID)
	if err != nil {
		return nil, err
	}

	attached := diskIsAttached(disk, instance)

	if !attached {
		// Volume is not attached to node. Success!
		glog.Infof("Detach operation is successful. PD %q was not attached to node %q.", volumeName, nodeID)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	detachOp, err := gceCS.CloudProvider.DetachDisk(ctx, volumeZone, nodeID, volumeName)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown detach error: %v", err))
	}

	err = gceCS.CloudProvider.WaitForOp(ctx, detachOp, volumeZone)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("unknown detach operation error: %v", err))
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// TODO: This abstraction isn't great. We shouldn't need diskIsAttached AND diskIsAttachedAndCompatible to duplicate code
func diskIsAttached(volume *compute.Disk, instance *compute.Instance) bool {
	for _, disk := range instance.Disks {
		if disk.DeviceName == volume.Name {
			// Disk is attached to node
			return true
		}
	}
	return false
}

func diskIsAttachedAndCompatible(volume *compute.Disk, instance *compute.Instance, volumeCapability *csi.VolumeCapability, readWrite string) (bool, error) {
	for _, disk := range instance.Disks {
		if disk.DeviceName == volume.Name {
			// Disk is attached to node
			if disk.Mode != readWrite {
				return true, fmt.Errorf("disk mode does not match. Got %v. Want %v", disk.Mode, readWrite)
			}
			// TODO: Check volume_capability.
			return true, nil
		}
	}
	return false, nil
}

func (gceCS *GCEControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	// TODO: Factor out the volume capability functionality and use as validation in all other functions as well
	glog.V(5).Infof("Using default ValidateVolumeCapabilities")
	// Validate Arguments
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume Volume ID must be provided")
	}
	if req.GetVolumeCapabilities() == nil || len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume Volume Capabilities must be provided")
	}

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

	return &csi.ValidateVolumeCapabilitiesResponse{
		Supported: true,
	}, nil
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
	return nil, status.Error(codes.Unimplemented, "")
}

func (gceCS *GCEControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (gceCS *GCEControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
