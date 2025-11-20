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
	neturl "net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	csi "github.com/container-storage-interface/spec/lib/go/csi"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/convert"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/metrics"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/parameters"
)

type GCEControllerServer struct {
	Driver        *GCEDriver
	CloudProvider gce.GCECompute
	Metrics       metrics.MetricsManager

	volumeEntries     []*csi.ListVolumesResponse_Entry
	volumeEntriesSeen map[string]int

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

	// Requisite zones to fallback to when provisioning a disk.
	// If there are an insufficient number of zones available in the union
	// of preferred/requisite topology, this list is used instead of
	// the passed in requisite topology.
	// The main use case of this field is to support Regional Persistent Disk
	// provisioning in GKE Autopilot, where a GKE cluster to
	// be scaled down to 1 zone.
	fallbackRequisiteZones []string

	// If set to true, the CSI Driver will allow volumes to be provisioned in Storage Pools.
	enableStoragePools bool

	// If set to true, the CSI Driver will allow Hyperdisk-balanced High Availability disks
	// to be provisioned.
	enableHdHA bool

	// If set to true, the CSI Driver will allow volumes to be provisioned with data cache configuration
	enableDataCache bool

	multiZoneVolumeHandleConfig MultiZoneVolumeHandleConfig

	listVolumesConfig ListVolumesConfig

	provisionableDisksConfig ProvisionableDisksConfig

	// Embed UnimplementedControllerServer to ensure the driver returns Unimplemented for any
	// new RPC methods that might be introduced in future versions of the spec.
	csi.UnimplementedControllerServer

	EnableDiskTopology       bool
	EnableDiskSizeValidation bool
}

type GCEControllerServerArgs struct {
	EnableDiskTopology       bool
	EnableDiskSizeValidation bool
	EnableDynamicVolumes     bool
}

type MultiZoneVolumeHandleConfig struct {
	// A set of supported disk types that are compatible with multi-zone volumeHandles.
	// The disk type is only validated on ControllerPublish.
	// Other operations that interacti with volumeHandle (ListVolumes/ControllerUnpublish)
	// don't validate the disk type. This ensures existing published multi-zone volumes
	// are listed and unpublished correctly. This allows this flag
	// to be ratcheted to be more restricted without affecting volumes that are already
	// published.
	DiskTypes []string

	// If set to true, the CSI driver will enable the multi-zone volumeHandle feature.
	// If set to false, volumeHandles that contain 'multi-zone' will not be translated
	// to their respective attachment zone (based on the node), which will result in
	// an "Unknown zone" error on ControllerPublish/ControllerUnpublish.
	Enable bool
}

type ListVolumesConfig struct {
	UseInstancesAPIForPublishedNodes bool
}

type ProvisionableDisksConfig struct {
	SupportsIopsChange       []string
	SupportsThroughputChange []string
}

func (c ListVolumesConfig) listDisksFields() []googleapi.Field {
	if c.UseInstancesAPIForPublishedNodes {
		// If we are using the instances.list API in ListVolumes,
		// don't include the users field in the response, as an optimization.
		// We rely on instances.list items.disks for attachment pairings.
		return listDisksFieldsWithoutUsers
	}

	return listDisksFieldsWithUsers
}

type csiErrorBackoffId string

type csiErrorBackoff struct {
	backoff    *flowcontrol.Backoff
	errorCodes map[csiErrorBackoffId]codes.Code
}

type workItem struct {
	ctx          context.Context
	publishReq   *csi.ControllerPublishVolumeRequest
	unpublishReq *csi.ControllerUnpublishVolumeRequest
}

// locationRequirements are additional location topology requirements that must be respected when creating a volume.
type locationRequirements struct {
	srcVolRegion    string
	srcVolZone      string
	srcIsRegional   bool
	cloneIsRegional bool
}

// PDCSIContext is the extracted VolumeContext from controller requests.
type PDCSIContext struct {
	ForceAttach bool
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
	// The maximum number of entries that we can include in the
	// ListVolumesResposne
	// In reality, the limit here is 4MB (based on gRPC client response limits),
	// but 500 is a good proxy (gives ~8KB of data per ListVolumesResponse#Entry)
	// See https://github.com/grpc/grpc/blob/master/include/grpc/impl/codegen/grpc_types.h#L503)
	maxListVolumesResponseEntries = 500

	// Keys in the volume context.
	contextForceAttach = "force-attach"

	resourceApiScheme  = "https"
	resourceApiService = "compute"
	resourceProject    = "projects"

	listDisksUsersField = googleapi.Field("items/users")
)

var (
	validResourceApiVersions = map[string]bool{"v1": true, "alpha": true, "beta": true, "staging_v1": true, "staging_beta": true, "staging_alpha": true}

	// By default GCE returns a lot of data for each instance. Request only a subset of the fields.
	listInstancesFields = []googleapi.Field{
		"items/disks/deviceName",
		"items/disks/source",
		"items/selfLink",
		"nextPageToken",
	}

	// By default GCE returns a lot of data for each disk. Request only a subset of the fields.
	listDisksFieldsWithoutUsers = []googleapi.Field{
		"items/labels",
		"items/selfLink",
		"nextPageToken",
	}
	listDisksFieldsWithUsers      = append(listDisksFieldsWithoutUsers, "items/users")
	disksWithModifiableAccessMode = []string{parameters.DiskTypeHdML}
	disksWithUnsettableAccessMode = map[string]bool{
		parameters.DiskTypeHdE: true,
		parameters.DiskTypeHdT: true,
	}
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
func cloningLocationRequirements(req *csi.CreateVolumeRequest, cloneIsRegional bool) (*locationRequirements, error) {
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

	return &locationRequirements{srcVolZone: sourceVolKey.Zone, srcVolRegion: sourceVolKey.Region, srcIsRegional: !isZonalSrcVol, cloneIsRegional: cloneIsRegional}, nil
}

// useVolumeCloning returns true if the create volume request should be created with volume cloning.
func useVolumeCloning(req *csi.CreateVolumeRequest) bool {
	return req.VolumeContentSource != nil && req.VolumeContentSource.GetVolume() != nil
}

func (gceCS *GCEControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	response, err := gceCS.createVolumeInternal(ctx, req)
	if err != nil && req != nil {
		klog.V(4).Infof("CreateVolume succeeded for volume %v", req.Name)
	}

	return response, err
}

func (gceCS *GCEControllerServer) createVolumeInternal(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	var err error
	// Apply Parameters (case-insensitive). We leave validation of
	// the values to the cloud provider.
	params, dataCacheParams, err := gceCS.parameterProcessor().ExtractAndDefaultParameters(req.GetParameters())
	metrics.UpdateRequestMetadataFromParams(ctx, params)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to extract parameters: %v", err.Error())
	}

	// Validate arguments
	volumeCapabilities := req.GetVolumeCapabilities()
	capacityRange := req.GetCapacityRange()
	if len(req.GetName()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Name must be provided")
	}
	if len(volumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Volume capabilities must be provided")
	}

	// Validate request capacity early
	if _, err := getRequestCapacity(capacityRange); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume Request Capacity is invalid: %v", err.Error())
	}

	err = validateVolumeCapabilities(volumeCapabilities)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "VolumeCapabilities is invalid: %v", err.Error())
	}
	// https://github.com/container-storage-interface/spec/blob/master/spec.md#createvolume
	// mutable_parameters MUST take precedence over the values from parameters.
	mutableParams := req.GetMutableParameters()
	// If the disk type does not support dynamic provisioning, throw an error
	supportsIopsChange := gceCS.diskSupportsIopsChange(params.DiskType)
	supportsThroughputChange := gceCS.diskSupportsThroughputChange(params.DiskType)
	if len(mutableParams) > 0 {
		if !supportsIopsChange && !supportsThroughputChange {
			return nil, status.Errorf(codes.InvalidArgument, "Disk type %s does not support dynamic provisioning", params.DiskType)
		}
		p, err := parameters.ExtractModifyVolumeParameters(mutableParams)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "Invalid mutable parameters: %v", err)
		}
		if p.IOPS != nil {
			if !supportsIopsChange {
				return nil, status.Errorf(codes.InvalidArgument, "Cannot specify IOPS for disk type %s", params.DiskType)
			}
			params.ProvisionedIOPSOnCreate = *p.IOPS
		}
		if p.Throughput != nil {
			if !supportsThroughputChange {
				return nil, status.Errorf(codes.InvalidArgument, "Cannot specify throughput for disk type %s", params.DiskType)
			}
			params.ProvisionedThroughputOnCreate = *p.Throughput
		}
	}

	// Validate multiwriter
	if _, err := getMultiWriterFromCapabilities(volumeCapabilities); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "VolumeCapabilities is invalid: %v", err.Error())
	}
	err = validateStoragePools(req, params, gceCS.CloudProvider.GetDefaultProject())
	if err != nil {
		// Reassign error so that all errors are reported as InvalidArgument to RecordOperationErrorMetrics.
		err = status.Errorf(codes.InvalidArgument, "CreateVolume failed to validate storage pools: %v", err)
		return nil, err
	}

	// Validate VolumeContentSource is set when access mode is read only
	readonly, _ := getReadOnlyFromCapabilities(volumeCapabilities)

	if readonly && req.GetVolumeContentSource() == nil && params.DiskType != parameters.DiskTypeHdML {
		return nil, status.Error(codes.InvalidArgument, "VolumeContentSource must be provided when AccessMode is set to read only")
	}

	if readonly && params.DiskType == parameters.DiskTypeHdHA {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid access mode for disk type %s", parameters.DiskTypeHdHA)
	}

	// Hyperdisk-throughput and hyperdisk-extreme do not support attaching to multiple VMs.
	isMultiAttach, err := getMultiAttachementFromCapabilities(volumeCapabilities)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume failed to parse volume capabilities: %v", err)
	}
	if isMultiAttach && disksWithUnsettableAccessMode[params.DiskType] {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid access mode for disk type %s", params.DiskType)
	}

	// Validate multi-zone provisioning configuration
	err = gceCS.validateMultiZoneProvisioning(req, params)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume failed to validate multi-zone provisioning request: %v", err)
	}

	// Verify that the regional availability class is only used on regional disks.
	if params.ForceAttach && !params.IsRegional() {
		return nil, status.Errorf(codes.InvalidArgument, "invalid availabilty class for zonal disk")
	}

	if gceCS.multiZoneVolumeHandleConfig.Enable && params.MultiZoneProvisioning {
		// Create multi-zone disk, that may have up to N disks.
		return gceCS.createMultiZoneDisk(ctx, req, params, dataCacheParams, gceCS.enableDataCache)
	}

	// Create single device zonal or regional disk
	return gceCS.createSingleDeviceDisk(ctx, req, params, dataCacheParams, gceCS.enableDataCache)
}

func (gceCS *GCEControllerServer) getSupportedZonesForPDType(ctx context.Context, zones []string, diskType string) ([]string, error) {
	project := gceCS.CloudProvider.GetDefaultProject()
	zones, err := gceCS.CloudProvider.ListCompatibleDiskTypeZones(ctx, project, zones, diskType)
	if err != nil {
		return nil, err
	}
	return zones, nil
}

func (gceCS *GCEControllerServer) getMultiZoneProvisioningZones(ctx context.Context, req *csi.CreateVolumeRequest, params parameters.DiskParameters) ([]string, error) {
	top := req.GetAccessibilityRequirements()
	if top == nil {
		return nil, status.Errorf(codes.InvalidArgument, "no topology specified")
	}
	prefZones, err := getZonesFromTopology(top.GetPreferred())
	if err != nil {
		return nil, fmt.Errorf("could not get zones from preferred topology: %w", err)
	}
	reqZones, err := getZonesFromTopology(top.GetRequisite())
	if err != nil {
		return nil, fmt.Errorf("could not get zones from requisite topology: %w", err)
	}
	prefSet := sets.NewString(prefZones...)
	reqSet := sets.NewString(reqZones...)
	prefAndReqSet := prefSet.Union(reqSet)
	availableZones := prefAndReqSet.List()
	if prefAndReqSet.Len() == 0 {
		// If there are no specified zones, this means that there were no aggregate
		// zones (eg: no nodes running) in the cluster
		availableZones = gceCS.fallbackRequisiteZones
	}

	supportedZones, err := gceCS.getSupportedZonesForPDType(ctx, availableZones, params.DiskType)
	if err != nil {
		return nil, fmt.Errorf("could not get supported zones for disk type %v from zone list %v: %w", params.DiskType, prefAndReqSet.List(), err)
	}

	// It's possible that the provided requisite zones shifted since the last time that
	// CreateVolume was called (eg: due to a node being removed in a zone)
	// Ensure that we combine the supportedZones with any existing zones to get the full set.
	existingZones, err := gceCS.getZonesWithDiskNameAndType(ctx, req.Name, params.DiskType)
	if err != nil {
		return nil, common.LoggedError(fmt.Sprintf("failed to check existing list of zones for request: %v", req.Name), err)
	}

	supportedSet := sets.NewString(supportedZones...)
	existingSet := sets.NewString(existingZones...)
	combinedZones := existingSet.Union(supportedSet)

	return combinedZones.List(), nil
}

func (gceCS *GCEControllerServer) createMultiZoneDisk(ctx context.Context, req *csi.CreateVolumeRequest, params parameters.DiskParameters, dataCacheParams parameters.DataCacheParameters, enableDataCache bool) (*csi.CreateVolumeResponse, error) {
	var err error
	// For multi-zone, we either select:
	// 1) The zones specified in requisite topology requirements
	// 2) All zones in the region that are compatible with the selected disk type
	zones, err := gceCS.getMultiZoneProvisioningZones(ctx, req, params)
	if err != nil {
		return nil, err
	}

	multiZoneVolKey := meta.ZonalKey(req.GetName(), constants.MultiZoneValue)
	volumeID, err := common.KeyToVolumeID(multiZoneVolKey, gceCS.CloudProvider.GetDefaultProject())
	if err != nil {
		return nil, err
	}
	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, constants.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)

	// If creating an empty disk (content source nil), always create RWO disks (when supported)
	// This allows disks to be created as underlying RWO disks, so they can be hydrated.
	accessMode := constants.GCEReadWriteOnceAccessMode
	if req.GetVolumeContentSource() != nil {
		accessMode = constants.GCEReadOnlyManyAccessMode
	}

	createDiskErrs := []error{}
	createdDisks := make([]*gce.CloudDisk, 0, len(zones))
	for _, zone := range zones {
		volKey := meta.ZonalKey(req.GetName(), zone)
		klog.V(4).Infof("Creating single zone disk for zone %q and volume: %v", zone, volKey)
		disk, err := gceCS.createSingleDisk(ctx, req, params, volKey, []string{zone}, accessMode)
		if err != nil {
			createDiskErrs = append(createDiskErrs, err)
			continue
		}

		createdDisks = append(createdDisks, disk)
	}
	if len(createDiskErrs) > 0 {
		return nil, common.LoggedError("Failed to create multi-zone disk: ", errors.Join(createDiskErrs...))
	}

	if len(createdDisks) == 0 {
		return nil, status.Errorf(codes.Internal, "could not create any disks for request: %v", req)
	}

	// Use the first response as a template
	volumeId := fmt.Sprintf("projects/%s/zones/%s/disks/%s", gceCS.CloudProvider.GetDefaultProject(), constants.MultiZoneValue, req.GetName())
	klog.V(4).Infof("CreateVolume succeeded for multi-zone disks in zones %s: %v", zones, multiZoneVolKey)

	return gceCS.generateCreateVolumeResponseWithVolumeId(createdDisks[0], zones, params, dataCacheParams, enableDataCache, volumeId), nil
}

func (gceCS *GCEControllerServer) getZonesWithDiskNameAndType(ctx context.Context, name string, diskType string) ([]string, error) {
	zoneOnlyFields := []googleapi.Field{"items/zone", "items/type"}
	nameAndRegionFilter := fmt.Sprintf("name=%s", name)
	disksWithZone, _, err := gceCS.CloudProvider.ListDisksWithFilter(ctx, zoneOnlyFields, nameAndRegionFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing zones for disk name %v: %w", name, err)
	}
	zones := []string{}
	for _, disk := range disksWithZone {
		if !strings.Contains(disk.Type, diskType) || disk.Zone == "" {
			continue
		}
		diskZone, err := common.ParseZoneFromURI(disk.Zone)
		if err != nil {
			klog.Warningf("Malformed zone URI %v for disk %v from ListDisks call. Skipping", disk.Zone, name)
			continue
		}
		zones = append(zones, diskZone)
	}
	return zones, nil
}

func (gceCS *GCEControllerServer) updateAccessModeIfNecessary(ctx context.Context, project string, volKey *meta.Key, disk *gce.CloudDisk, readonly bool) error {
	if !slices.Contains(disksWithModifiableAccessMode, disk.GetPDType()) {
		// If this isn't a disk that has access mode (eg: Hyperdisk ML), return
		// So far, HyperdiskML is the only disk type that allows the disk type to be modified.
		return nil
	}
	if !readonly {
		// Only update the access mode if we're converting from ReadWrite to ReadOnly
		return nil
	}

	if disk.GetAccessMode() == constants.GCEReadOnlyManyAccessMode {
		// If the access mode is already readonly, return
		return nil
	}

	return gceCS.CloudProvider.SetDiskAccessMode(ctx, project, volKey, constants.GCEReadOnlyManyAccessMode)
}

func (gceCS *GCEControllerServer) createSingleDeviceDisk(ctx context.Context, req *csi.CreateVolumeRequest, params parameters.DiskParameters, dataCacheParams parameters.DataCacheParameters, enableDataCache bool) (*csi.CreateVolumeResponse, error) {
	var err error
	var locationTopReq *locationRequirements
	if useVolumeCloning(req) {
		locationTopReq, err = cloningLocationRequirements(req, params.IsRegional())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "failed to get location requirements: %v", err.Error())
		}
	}

	// Determine the zone or zones+region of the disk
	var zones []string
	var volKey *meta.Key
	if params.IsRegional() {
		zones, err = gceCS.pickZones(ctx, req.GetAccessibilityRequirements(), 2, locationTopReq)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "CreateVolume failed to pick zones for disk: %v", err.Error())
		}
		region, err := common.GetRegionFromZones(zones)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "CreateVolume failed to get region from zones: %v", err.Error())
		}
		volKey = meta.RegionalKey(req.GetName(), region)
	} else if params.ReplicationType == replicationTypeNone {
		zones, err = gceCS.pickZones(ctx, req.GetAccessibilityRequirements(), 1, locationTopReq)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "CreateVolume failed to pick zones for disk: %v", err.Error())
		}
		if len(zones) != 1 {
			return nil, status.Errorf(codes.Internal, "Failed to pick exactly 1 zone for zonal disk, got %v instead", len(zones))
		}
		volKey = meta.ZonalKey(req.GetName(), zones[0])
	} else {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume replication type '%s' is not supported", params.ReplicationType)
	}

	volumeID, err := common.KeyToVolumeID(volKey, gceCS.CloudProvider.GetDefaultProject())
	if err != nil {
		return nil, common.LoggedError("Failed to convert volume key to volume ID: ", err)
	}
	accessMode, err := getAccessMode(req, params)
	if err != nil {
		return nil, common.LoggedError("Failed to get access mode: ", err)
	}

	// If creating an empty disk (content source nil), always create RWO disks (when supported)
	// This allows disks to be created as underlying RWO disks, so they can be hydrated.
	readonly, _ := getReadOnlyFromCapabilities(req.GetVolumeCapabilities())
	if readonly && req.GetVolumeContentSource() == nil && params.DiskType == parameters.DiskTypeHdML {
		accessMode = constants.GCEReadWriteOnceAccessMode
	}

	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, constants.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)

	disk, err := gceCS.createSingleDisk(ctx, req, params, volKey, zones, accessMode)
	if err != nil {
		return nil, common.LoggedError("CreateVolume failed: ", err)
	}

	return gceCS.generateCreateVolumeResponseWithVolumeId(disk, zones, params, dataCacheParams, enableDataCache, volumeID), nil
}

func getAccessMode(req *csi.CreateVolumeRequest, params parameters.DiskParameters) (string, error) {
	readonly, _ := getReadOnlyFromCapabilities(req.GetVolumeCapabilities())
	if common.IsHyperdisk(params.DiskType) {
		if am, err := getHyperdiskAccessModeFromCapabilities(req.GetVolumeCapabilities()); err != nil {
			return "", err
		} else if disksWithUnsettableAccessMode[params.DiskType] {
			// Disallow multi-attach for HdT and HdE. These checks were done in `createVolumeInternal`,
			// but repeating them here future-proves us from possible refactors.
			if am != constants.GCEReadWriteOnceAccessMode {
				return "", status.Errorf(codes.Internal, "")
			}
		} else {
			return am, nil
		}
	}

	if readonly && slices.Contains(disksWithModifiableAccessMode, params.DiskType) {
		return constants.GCEReadOnlyManyAccessMode, nil
	}

	return "", nil
}

func (gceCS *GCEControllerServer) createSingleDisk(ctx context.Context, req *csi.CreateVolumeRequest, params parameters.DiskParameters, volKey *meta.Key, zones []string, accessMode string) (*gce.CloudDisk, error) {
	capacityRange := req.GetCapacityRange()
	capBytes, _ := getRequestCapacity(capacityRange)

	multiWriter := false
	if !common.IsHyperdisk(params.DiskType) {
		multiWriter, _ = getMultiWriterFromCapabilities(req.GetVolumeCapabilities())
	}

	// Validate if disk already exists
	existingDisk, err := gceCS.CloudProvider.GetDisk(ctx, gceCS.CloudProvider.GetDefaultProject(), volKey)
	if err != nil {
		if !gce.IsGCEError(err, "notFound") {
			// failed to GetDisk, however the Disk may already be created, the error code should be non-Final
			return nil, common.LoggedError("CreateVolume, failed to getDisk when validating: ", status.Error(codes.Unavailable, err.Error()))
		}
	}
	if err == nil {
		// There was no error so we want to validate the disk that we find
		err = gce.ValidateExistingDisk(ctx, existingDisk, params,
			int64(capacityRange.GetRequiredBytes()),
			int64(capacityRange.GetLimitBytes()),
			multiWriter, accessMode)
		if err != nil {
			return nil, status.Errorf(codes.AlreadyExists, "CreateVolume disk already exists with same name and is incompatible: %v", err.Error())
		}

		ready, err := isDiskReady(existingDisk)
		if err != nil {
			return nil, status.Errorf(codes.Aborted, "CreateVolume disk %q had error checking ready status: %v", volKey.String(), err.Error())
		}
		if !ready {
			return nil, status.Errorf(codes.Aborted, "CreateVolume existing disk %v is not ready", volKey)
		}

		// If there is no validation error, immediately return success
		klog.V(4).Infof("CreateVolume succeeded for disk %v, it already exists and was compatible", volKey)
		return existingDisk, nil
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
			diskFromSourceVolume, err := gceCS.CloudProvider.GetDisk(ctx, project, sourceVolKey)
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
				return nil, status.Errorf(codes.Aborted, "CreateVolume disk from source volume %q had error checking ready status: %v", sourceVolKey.String(), err.Error())
			}
			if !ready {
				return nil, status.Errorf(codes.Aborted, "CreateVolume disk from source volume %v is not ready", sourceVolKey)
			}
		}
	}

	// Create the disk
	var disk *gce.CloudDisk
	name := req.GetName()

	if params.IsRegional() {
		if len(zones) != 2 {
			return nil, status.Errorf(codes.Internal, "CreateVolume failed to get a 2 zones for creating regional disk, instead got: %v", zones)
		}
		disk, err = createRegionalDisk(ctx, gceCS.CloudProvider, name, zones, params, capacityRange, capBytes, snapshotID, volumeContentSourceVolumeID, multiWriter, accessMode)
		if err != nil {
			return nil, common.LoggedError("CreateVolume failed to create regional disk "+name+": ", err)
		}
	} else if params.ReplicationType == replicationTypeNone {
		if len(zones) != 1 {
			return nil, status.Errorf(codes.Internal, "CreateVolume failed to get a single zone for creating zonal disk, instead got: %v", zones)
		}
		disk, err = createSingleZoneDisk(ctx, gceCS.CloudProvider, name, zones, params, capacityRange, capBytes, snapshotID, volumeContentSourceVolumeID, multiWriter, accessMode)
		if err != nil {
			return nil, common.LoggedError("CreateVolume failed to create single zonal disk "+name+": ", err)
		}
	} else {
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
	return disk, nil
}

func (gceCS *GCEControllerServer) diskSupportsIopsChange(diskType string) bool {
	for _, disk := range gceCS.provisionableDisksConfig.SupportsIopsChange {
		if disk == diskType {
			return true
		}
	}
	return false
}

func (gceCS *GCEControllerServer) diskSupportsThroughputChange(diskType string) bool {
	for _, disk := range gceCS.provisionableDisksConfig.SupportsThroughputChange {
		if disk == diskType {
			return true
		}
	}
	return false
}

func (gceCS *GCEControllerServer) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	var err error

	volumeID := req.GetVolumeId()
	klog.V(4).Infof("Modifying Volume ID: %s", volumeID)

	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}

	project, volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		// Cannot find volume associated with this ID because VolumeID is not in the correct format
		err = status.Errorf(codes.NotFound, "volume ID is invalid: %v", err.Error())
		return nil, err
	}

	volumeModifyParams, err := parameters.ExtractModifyVolumeParameters(req.GetMutableParameters())
	if err != nil {
		klog.Errorf("Failed to extract parameters for volume %s: %v", volumeID, err)
		err = status.Errorf(codes.InvalidArgument, "Invalid parameters: %v", err)
		return nil, err
	}
	klog.V(4).Infof("Modify Volume Parameters for %s: %v", volumeID, volumeModifyParams)

	existingDisk, err := gceCS.CloudProvider.GetDisk(ctx, project, volKey)
	metrics.UpdateRequestMetadataFromDisk(ctx, existingDisk)

	if err != nil {
		err = fmt.Errorf("Failed to get volume: %w", err)
		return nil, err
	}

	if existingDisk == nil || existingDisk.GetSelfLink() == "" {
		err = status.Errorf(codes.Internal, "failed to get volume : %s", volumeID)
		return nil, err
	}

	// Check if the disk supports dynamic IOPS/Throughput provisioning
	diskType := existingDisk.GetPDType()
	supportsIopsChange := gceCS.diskSupportsIopsChange(diskType)
	supportsThroughputChange := gceCS.diskSupportsThroughputChange(diskType)
	if !supportsIopsChange && !supportsThroughputChange {
		err = status.Errorf(codes.InvalidArgument, "Failed to modify volume: modifications not supported for disk type %s", diskType)
		return nil, err
	}
	if !supportsIopsChange && volumeModifyParams.IOPS != nil {
		err = status.Errorf(codes.InvalidArgument, "Cannot specify IOPS for disk type %s", diskType)
		return nil, err
	}
	if !supportsThroughputChange && volumeModifyParams.Throughput != nil {
		err = status.Errorf(codes.InvalidArgument, "Cannot specify throughput for disk type %s", diskType)
		return nil, err
	}

	err = gceCS.CloudProvider.UpdateDisk(ctx, project, volKey, existingDisk, volumeModifyParams)
	if err != nil {
		klog.Errorf("Failed to modify volume %s: %v", volumeID, err)
		err = fmt.Errorf("Failed to modify volume %s: %w", volumeID, err)
		return nil, err
	}

	return &csi.ControllerModifyVolumeResponse{}, nil
}

func (gceCS *GCEControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	var err error
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

	volumeIsMultiZone := isMultiZoneVolKey(volKey)
	if gceCS.multiZoneVolumeHandleConfig.Enable && volumeIsMultiZone {
		// Delete multi-zone disk, that may have up to N disks.
		return gceCS.deleteMultiZoneDisk(ctx, req, project, volKey)
	}

	// Delete zonal or regional disk
	return gceCS.deleteSingleDeviceDisk(ctx, req, project, volKey)
}

func getGCEApiVersion(multiWriter bool) gce.GCEAPIVersion {
	if multiWriter {
		return gce.GCEAPIVersionBeta
	}

	return gce.GCEAPIVersionV1
}

func (gceCS *GCEControllerServer) deleteMultiZoneDisk(ctx context.Context, req *csi.DeleteVolumeRequest, project string, volKey *meta.Key) (*csi.DeleteVolumeResponse, error) {
	// List disks with same name
	var err error
	existingZones := []string{gceCS.CloudProvider.GetDefaultZone()}
	zones, err := getDefaultZonesInRegion(ctx, gceCS, existingZones)
	if err != nil {
		return nil, fmt.Errorf("failed to list default zones: %w", err)
	}

	volumeID := req.GetVolumeId()
	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, constants.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)

	deleteDiskErrs := []error{}
	for _, zone := range zones {
		zonalVolKey := &meta.Key{
			Name:   volKey.Name,
			Region: volKey.Region,
			Zone:   zone,
		}
		disk, _ := gceCS.CloudProvider.GetDisk(ctx, project, zonalVolKey)
		// TODO: Consolidate the parameters here, rather than taking the last.
		metrics.UpdateRequestMetadataFromDisk(ctx, disk)
		err := gceCS.CloudProvider.DeleteDisk(ctx, project, zonalVolKey)
		if err != nil {
			deleteDiskErrs = append(deleteDiskErrs, gceCS.CloudProvider.DeleteDisk(ctx, project, volKey))
		}
	}

	if len(deleteDiskErrs) > 0 {
		return nil, common.LoggedError("Failed to delete multi-zone disk: ", errors.Join(deleteDiskErrs...))
	}

	klog.V(4).Infof("DeleteVolume succeeded for disk %v", volKey)
	return &csi.DeleteVolumeResponse{}, nil
}

func (gceCS *GCEControllerServer) deleteSingleDeviceDisk(ctx context.Context, req *csi.DeleteVolumeRequest, project string, volKey *meta.Key) (*csi.DeleteVolumeResponse, error) {
	var err error
	volumeID := req.GetVolumeId()
	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey, "")
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			klog.Warningf("DeleteVolume treating volume as deleted because cannot find volume %v: %v", volumeID, err.Error())
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, common.LoggedError("DeleteVolume error repairing underspecified volume key: ", err)
	}

	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, constants.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)
	disk, _ := gceCS.CloudProvider.GetDisk(ctx, project, volKey)
	metrics.UpdateRequestMetadataFromDisk(ctx, disk)
	err = gceCS.CloudProvider.DeleteDisk(ctx, project, volKey)
	if err != nil {
		return nil, common.LoggedError("Failed to delete disk: ", err)
	}

	klog.V(4).Infof("DeleteVolume succeeded for disk %v", volKey)
	return &csi.DeleteVolumeResponse{}, nil
}

func (gceCS *GCEControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	var err error
	// Only valid requests will be accepted
	_, _, _, err = gceCS.validateControllerPublishVolumeRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	backoffId := gceCS.errorBackoff.backoffId(req.NodeId, req.VolumeId)
	if gceCS.errorBackoff.blocking(backoffId) {
		return nil, status.Errorf(gceCS.errorBackoff.code(backoffId), "ControllerPublish not permitted on node %q due to backoff condition", req.NodeId)
	}

	resp, err, disk := gceCS.executeControllerPublishVolume(ctx, req)
	metrics.UpdateRequestMetadataFromDisk(ctx, disk)
	if err != nil {
		klog.Infof("For node %s adding backoff due to error for volume %s: %v", req.NodeId, req.VolumeId, err)
		gceCS.errorBackoff.next(backoffId, common.CodeForError(err))
	} else {
		klog.Infof("For node %s clear backoff due to successful publish of volume %v", req.NodeId, req.VolumeId)
		gceCS.errorBackoff.reset(backoffId)
	}
	return resp, err
}

func (gceCS *GCEControllerServer) validateControllerPublishVolumeRequest(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (string, *meta.Key, *PDCSIContext, error) {
	// Validate arguments
	volumeID := req.GetVolumeId()
	nodeID := req.GetNodeId()
	volumeCapability := req.GetVolumeCapability()
	if len(volumeID) == 0 {
		return "", nil, nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume ID must be provided")
	}
	if len(nodeID) == 0 {
		return "", nil, nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Node ID must be provided")
	}
	if volumeCapability == nil {
		return "", nil, nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume capability must be provided")
	}

	project, volKey, err := common.VolumeIDToKey(volumeID)
	if err != nil {
		return "", nil, nil, status.Errorf(codes.InvalidArgument, "ControllerPublishVolume volume ID is invalid: %v", err.Error())
	}

	// TODO(#253): Check volume capability matches for ALREADY_EXISTS
	if err = validateVolumeCapability(volumeCapability); err != nil {
		return "", nil, nil, status.Errorf(codes.InvalidArgument, "VolumeCapabilities is invalid: %v", err.Error())
	}

	var pdcsiContext *PDCSIContext
	if pdcsiContext, err = extractVolumeContext(req.VolumeContext); err != nil {
		return "", nil, nil, status.Errorf(codes.InvalidArgument, "Invalid volume context: %v", err.Error())
	}

	return project, volKey, pdcsiContext, nil
}

func parseMachineType(machineTypeUrl string) string {
	machineType, parseErr := common.ParseMachineType(machineTypeUrl)
	if parseErr != nil {
		// Parse errors represent an unexpected API change with instance.MachineType; log a warning.
		klog.Warningf("ParseMachineType(%v): %v", machineTypeUrl, parseErr)
	}
	return machineType
}

func convertMultiZoneVolKeyToZoned(volumeKey *meta.Key, instanceZone string) *meta.Key {
	volumeKey.Zone = instanceZone
	return volumeKey
}

func (gceCS *GCEControllerServer) validateMultiZoneDisk(volumeID string, disk *gce.CloudDisk) error {
	if !slices.Contains(gceCS.multiZoneVolumeHandleConfig.DiskTypes, disk.GetPDType()) {
		return status.Errorf(codes.InvalidArgument, "Multi-zone volumeID %q points to disk with unsupported disk type %q: %v", volumeID, disk.GetPDType(), disk.GetSelfLink())
	}
	if _, ok := disk.GetLabels()[constants.MultiZoneLabel]; !ok {
		return status.Errorf(codes.InvalidArgument, "Multi-zone volumeID %q points to disk that is missing label %q: %v", volumeID, constants.MultiZoneLabel, disk.GetSelfLink())
	}
	return nil
}

func (gceCS *GCEControllerServer) validateMultiZoneProvisioning(req *csi.CreateVolumeRequest, params parameters.DiskParameters) error {
	if !gceCS.multiZoneVolumeHandleConfig.Enable {
		return nil
	}
	if !params.MultiZoneProvisioning {
		return nil
	}

	// For volume populator, we want to allow multiple RWO disks to be created
	// with the same name, so they can be hydrated across multiple zones.

	// We don't have support volume cloning from an existing PVC
	if useVolumeCloning(req) {
		return fmt.Errorf("%q parameter does not support volume cloning", parameters.ParameterKeyEnableMultiZoneProvisioning)
	}

	if readonly, _ := getReadOnlyFromCapabilities(req.GetVolumeCapabilities()); !readonly && req.GetVolumeContentSource() != nil {
		return fmt.Errorf("%q parameter does not support specifying volume content source in readwrite mode", parameters.ParameterKeyEnableMultiZoneProvisioning)
	}

	if !slices.Contains(gceCS.multiZoneVolumeHandleConfig.DiskTypes, params.DiskType) {
		return fmt.Errorf("%q parameter with unsupported disk type: %v", parameters.ParameterKeyEnableMultiZoneProvisioning, params.DiskType)
	}

	return nil
}

func isMultiZoneVolKey(volumeKey *meta.Key) bool {
	return volumeKey.Type() == meta.Zonal && volumeKey.Zone == constants.MultiZoneValue
}

func (gceCS *GCEControllerServer) executeControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error, *gce.CloudDisk) {
	project, volKey, pdcsiContext, err := gceCS.validateControllerPublishVolumeRequest(ctx, req)
	if err != nil {
		return nil, err, nil
	}

	volumeID := req.GetVolumeId()
	readOnly := req.GetReadonly()
	nodeID := req.GetNodeId()
	volumeCapability := req.GetVolumeCapability()

	pubVolResp := &csi.ControllerPublishVolumeResponse{
		PublishContext: map[string]string{},
	}

	// Set data cache publish context
	if gceCS.enableDataCache && req.GetVolumeContext() != nil {
		if req.GetVolumeContext()[constants.ContextDataCacheSize] != "" {
			pubVolResp.PublishContext = map[string]string{}
			pubVolResp.PublishContext[constants.ContextDataCacheSize] = req.GetVolumeContext()[constants.ContextDataCacheSize]
			pubVolResp.PublishContext[constants.ContextDataCacheMode] = req.GetVolumeContext()[constants.ContextDataCacheMode]
		}
	}

	instanceZone, instanceName, err := common.NodeIDToZoneAndName(nodeID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not split nodeID: %v", err.Error()), nil
	}

	volumeIsMultiZone := isMultiZoneVolKey(volKey)
	if gceCS.multiZoneVolumeHandleConfig.Enable && volumeIsMultiZone {
		// Only allow read-only attachment for "multi-zone" volumes
		if !readOnly {
			return nil, status.Errorf(codes.InvalidArgument, "'multi-zone' volume only supports 'readOnly': %v", volumeID), nil
		}

		volKey = convertMultiZoneVolKeyToZoned(volKey, instanceZone)
	}

	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey, "")
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "ControllerPublishVolume could not find volume with ID %v: %v", volumeID, err.Error()), nil
		}
		return nil, common.LoggedError("ControllerPublishVolume error repairing underspecified volume key: ", err), nil
	}

	// Acquires the lock for the volume on that node only, because we need to support the ability
	// to publish the same volume onto different nodes concurrently
	lockingVolumeID := fmt.Sprintf("%s/%s", nodeID, volumeID)
	if acquired := gceCS.volumeLocks.TryAcquire(lockingVolumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, constants.VolumeOperationAlreadyExistsFmt, lockingVolumeID), nil
	}
	defer gceCS.volumeLocks.Release(lockingVolumeID)
	disk, err := gceCS.CloudProvider.GetDisk(ctx, project, volKey)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "Could not find disk %v: %v", volKey.String(), err.Error()), disk
		}
		return nil, common.LoggedError("Failed to getDisk: ", err), disk
	}
	if gceCS.EnableDiskSizeValidation && pubVolResp.GetPublishContext() != nil {
		pubVolResp.PublishContext[constants.ContextDiskSizeGB] = strconv.FormatInt(disk.GetSizeGb(), 10)
	}
	instance, err := gceCS.CloudProvider.GetInstanceOrError(ctx, project, instanceZone, instanceName)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "Could not find instance %v: %v", nodeID, err.Error()), disk
		}
		return nil, common.LoggedError("Failed to get instance: ", err), disk
	}

	if gceCS.multiZoneVolumeHandleConfig.Enable && volumeIsMultiZone {
		if err := gceCS.validateMultiZoneDisk(volumeID, disk); err != nil {
			return nil, err, disk
		}
	}

	readWrite := "READ_WRITE"
	if readOnly {
		readWrite = "READ_ONLY"
	}

	deviceName, err := common.GetDeviceName(volKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting device name: %v", err.Error()), disk
	}

	attached, err := diskIsAttachedAndCompatible(deviceName, instance, volumeCapability, readWrite)
	if err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "Disk %v already published to node %v but incompatible: %v", volKey.Name, nodeID, err.Error()), disk
	}
	if attached {
		// Volume is attached to node. Success!
		klog.V(4).Infof("ControllerPublishVolume succeeded for disk %v to instance %v, already attached.", volKey, nodeID)
		return pubVolResp, nil, disk
	}
	if err := gceCS.updateAccessModeIfNecessary(ctx, project, volKey, disk, readOnly); err != nil {
		return nil, common.LoggedError("Failed to update access mode: ", err), disk
	}
	instanceZone, instanceName, err = common.NodeIDToZoneAndName(nodeID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "could not split nodeID: %v", err.Error()), disk
	}
	err = gceCS.CloudProvider.AttachDisk(ctx, project, volKey, readWrite, attachableDiskTypePersistent, instanceZone, instanceName, pdcsiContext.ForceAttach)
	if err != nil {
		var udErr *gce.UnsupportedDiskError
		if errors.As(err, &udErr) {
			// If we encountered an UnsupportedDiskError, rewrite the error message to be more user friendly.
			// The error message from GCE is phrased around disk create on VM creation, not runtime attach.
			machineType := parseMachineType(instance.MachineType)
			return nil, status.Errorf(codes.InvalidArgument, "'%s' is not a compatible disk type with the machine type %s, please review the GCP online documentation for available persistent disk options", udErr.DiskType, machineType), disk
		}
		return nil, common.LoggedError("Failed to Attach: ", err), disk
	}

	err = gceCS.CloudProvider.WaitForAttach(ctx, project, volKey, disk.GetPDType(), instanceZone, instanceName)
	if err != nil {
		return nil, common.LoggedError("Errored during WaitForAttach: ", err), disk
	}

	klog.V(4).Infof("ControllerPublishVolume succeeded for disk %v to instance %v", volKey, nodeID)
	return pubVolResp, nil, disk
}

func (gceCS *GCEControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	var err error
	_, _, err = gceCS.validateControllerUnpublishVolumeRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	err = status.Errorf(codes.InvalidArgument, "error message")
	// Only valid requests will be queued
	backoffId := gceCS.errorBackoff.backoffId(req.NodeId, req.VolumeId)
	if gceCS.errorBackoff.blocking(backoffId) {
		return nil, status.Errorf(gceCS.errorBackoff.code(backoffId), "ControllerUnpublish not permitted on node %q due to backoff condition", req.NodeId)
	}
	resp, err, disk := gceCS.executeControllerUnpublishVolume(ctx, req)
	metrics.UpdateRequestMetadataFromDisk(ctx, disk)
	if err != nil {
		klog.Infof("For node %s adding backoff due to error for volume %s: %v", req.NodeId, req.VolumeId, err)
		gceCS.errorBackoff.next(backoffId, common.CodeForError(err))
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

func (gceCS *GCEControllerServer) executeControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error, *gce.CloudDisk) {
	project, volKey, err := gceCS.validateControllerUnpublishVolumeRequest(ctx, req)

	if err != nil {
		return nil, err, nil
	}

	volumeID := req.GetVolumeId()
	nodeID := req.GetNodeId()

	instanceZone, instanceName, err := common.NodeIDToZoneAndName(nodeID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "could not split nodeID: %v", err.Error()), nil
	}

	if gceCS.multiZoneVolumeHandleConfig.Enable && isMultiZoneVolKey(volKey) {
		volKey = convertMultiZoneVolKeyToZoned(volKey, instanceZone)
	}

	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey, instanceZone)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			klog.Warningf("Treating volume %v as unpublished because it could not be found", volumeID)
			return &csi.ControllerUnpublishVolumeResponse{}, nil, nil
		}
		return nil, common.LoggedError("ControllerUnpublishVolume error repairing underspecified volume key: ", err), nil
	}

	// Acquires the lock for the volume on that node only, because we need to support the ability
	// to unpublish the same volume from different nodes concurrently
	lockingVolumeID := fmt.Sprintf("%s/%s", nodeID, volumeID)
	if acquired := gceCS.volumeLocks.TryAcquire(lockingVolumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, constants.VolumeOperationAlreadyExistsFmt, lockingVolumeID), nil
	}
	defer gceCS.volumeLocks.Release(lockingVolumeID)
	diskToUnpublish, _ := gceCS.CloudProvider.GetDisk(ctx, project, volKey)
	instance, err := gceCS.CloudProvider.GetInstanceOrError(ctx, project, instanceZone, instanceName)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			// Node not existing on GCE means that disk has been detached
			klog.Warningf("Treating volume %v as unpublished because node %v could not be found", volKey.String(), instanceName)
			return &csi.ControllerUnpublishVolumeResponse{}, nil, diskToUnpublish
		}
		return nil, common.LoggedError("error getting instance: ", err), diskToUnpublish
	}

	deviceName, err := common.GetDeviceName(volKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting device name: %v", err.Error()), diskToUnpublish
	}

	attached := diskIsAttached(deviceName, instance)

	if !attached {
		// Volume is not attached to node. Success!
		klog.V(4).Infof("ControllerUnpublishVolume succeeded for disk %v from node %v. Already not attached.", volKey, nodeID)
		return &csi.ControllerUnpublishVolumeResponse{}, nil, diskToUnpublish
	}
	err = gceCS.CloudProvider.DetachDisk(ctx, project, deviceName, instanceZone, instanceName)
	if err != nil {
		return nil, common.LoggedError("Failed to detach: ", err), diskToUnpublish
	}

	klog.V(4).Infof("ControllerUnpublishVolume succeeded for disk %v from node %v", volKey, nodeID)
	return &csi.ControllerUnpublishVolumeResponse{}, nil, diskToUnpublish
}

func (gceCS *GCEControllerServer) parameterProcessor() *parameters.ParameterProcessor {
	return &parameters.ParameterProcessor{
		DriverName:         gceCS.Driver.name,
		EnableStoragePools: gceCS.enableStoragePools,
		EnableMultiZone:    gceCS.multiZoneVolumeHandleConfig.Enable,
		EnableHdHA:         gceCS.enableHdHA,
		EnableDiskTopology: gceCS.EnableDiskTopology,
		ExtraVolumeLabels:  gceCS.Driver.extraVolumeLabels,
		EnableDataCache:    gceCS.enableDataCache,
		ExtraTags:          gceCS.Driver.extraTags,
	}
}

func (gceCS *GCEControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	var err error
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

	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey, "")
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "ValidateVolumeCapabilities could not find volume with ID %v: %v", volumeID, err.Error())
		}
		return nil, common.LoggedError("ValidateVolumeCapabilities error repairing underspecified volume key: ", err)
	}

	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, constants.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)

	disk, err := gceCS.CloudProvider.GetDisk(ctx, project, volKey)
	metrics.UpdateRequestMetadataFromDisk(ctx, disk)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "Could not find disk %v: %v", volKey.Name, err.Error())
		}
		return nil, common.LoggedError("Failed to getDisk: ", err)
	}

	// Check volume capabilities supported by PD. These are the same for any PD
	if err := validateVolumeCapabilities(req.GetVolumeCapabilities()); err != nil {
		return generateFailedValidationMessage("VolumeCapabilities not valid: %v", err.Error()), nil
	}

	// Validate the disk parameters match the disk we GET
	params, _, err := gceCS.parameterProcessor().ExtractAndDefaultParameters(req.GetParameters())
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

func (gceCS *GCEControllerServer) listVolumeEntries(ctx context.Context) ([]*csi.ListVolumesResponse_Entry, error) {
	diskList, _, err := gceCS.CloudProvider.ListDisks(ctx, gceCS.listVolumesConfig.listDisksFields())
	if err != nil {
		return nil, err
	}

	var instanceList []*compute.Instance = nil
	if gceCS.listVolumesConfig.UseInstancesAPIForPublishedNodes {
		instanceList, _, err = gceCS.CloudProvider.ListInstances(ctx, listInstancesFields)
		if err != nil {
			return nil, err
		}
	}
	return gceCS.disksAndInstancesToVolumeEntries(diskList, instanceList), nil
}

func (gceCS *GCEControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	// https://cloud.google.com/compute/docs/reference/beta/disks/list
	if req.MaxEntries < 0 {
		return nil, status.Errorf(codes.InvalidArgument,
			"ListVolumes got max entries request %v. GCE only supports values >0", req.MaxEntries)
	}

	offsetLow := 0
	var ok bool
	if req.StartingToken == "" {
		volumeEntries, err := gceCS.listVolumeEntries(ctx)
		if err != nil {
			if gce.IsGCEInvalidError(err) {
				return nil, status.Errorf(codes.Aborted, "ListVolumes error with invalid request: %v", err.Error())
			}
			return nil, common.LoggedError("Failed to list volumes: ", err)
		}

		gceCS.volumeEntries = volumeEntries
		gceCS.volumeEntriesSeen = map[string]int{}
	} else {
		offsetLow, ok = gceCS.volumeEntriesSeen[req.StartingToken]
		if !ok {
			return nil, status.Errorf(codes.Aborted, "ListVolumes error with invalid startingToken: %s", req.StartingToken)
		}
	}

	var maxEntries int = int(req.MaxEntries)
	if maxEntries == 0 {
		maxEntries = maxListVolumesResponseEntries
	}

	nextToken := ""
	offsetHigh := offsetLow + maxEntries
	if offsetHigh < len(gceCS.volumeEntries) {
		nextToken = string(uuid.NewUUID())
		gceCS.volumeEntriesSeen[nextToken] = offsetHigh
	} else {
		offsetHigh = len(gceCS.volumeEntries)
	}

	return &csi.ListVolumesResponse{
		Entries:   gceCS.volumeEntries[offsetLow:offsetHigh],
		NextToken: nextToken,
	}, nil
}

// isMultiZoneDisk returns the multi-zone volumeId of a disk if it is
// "multi-zone", otherwise returns an empty string
// The second parameter indiciates if it is a "multi-zone" disk
func isMultiZoneDisk(diskRsrc string, diskLabels map[string]string) (string, bool) {
	isMultiZoneDisk := false
	for l := range diskLabels {
		if l == constants.MultiZoneLabel {
			isMultiZoneDisk = true
		}
	}
	if !isMultiZoneDisk {
		return "", false
	}

	multiZoneVolumeId, err := common.VolumeIdAsMultiZone(diskRsrc)
	if err != nil {
		klog.Warningf("Error converting multi-zone volume handle for disk %s, skipped: %v", diskRsrc, err)
		return "", false
	}
	return multiZoneVolumeId, true
}

// disksAndInstancesToVolumeEntries converts a list of disks and instances to a list
// of CSI ListVolumeResponse entries.
// It appends "multi-zone" volumeHandles at the end. These are volumeHandles which
// map to multiple volumeHandles in different zones
func (gceCS *GCEControllerServer) disksAndInstancesToVolumeEntries(disks []*compute.Disk, instances []*compute.Instance) []*csi.ListVolumesResponse_Entry {
	nodesByVolumeId := map[string][]string{}
	multiZoneVolumeIdsByVolumeId := map[string]string{}
	for _, d := range disks {
		volumeId, err := getResourceId(d.SelfLink)
		if err != nil {
			klog.Warningf("Bad ListVolumes disk resource %s, skipped: %v (%+v)", d.SelfLink, err, d)
			continue
		}

		instanceIds := make([]string, len(d.Users))
		for _, u := range d.Users {
			instanceId, err := getResourceId(u)
			if err != nil {
				klog.Warningf("Bad ListVolumes user %s, skipped: %v", u, err)
			} else {
				instanceIds = append(instanceIds, instanceId)
			}
		}

		nodesByVolumeId[volumeId] = instanceIds

		if gceCS.multiZoneVolumeHandleConfig.Enable {
			if multiZoneVolumeId, isMultiZone := isMultiZoneDisk(volumeId, d.Labels); isMultiZone {
				multiZoneVolumeIdsByVolumeId[volumeId] = multiZoneVolumeId
				nodesByVolumeId[multiZoneVolumeId] = append(nodesByVolumeId[multiZoneVolumeId], instanceIds...)
			}
		}
	}

	entries := []*csi.ListVolumesResponse_Entry{}
	for _, instance := range instances {
		instanceId, err := getResourceId(instance.SelfLink)
		if err != nil {
			klog.Warningf("Bad ListVolumes instance resource %s, skipped: %v (%+v)", instance.SelfLink, err, instance)
			continue
		}
		for _, disk := range instance.Disks {
			volumeId, err := getResourceId(disk.Source)
			if err != nil {
				klog.Warningf("Bad ListVolumes instance disk source %s, skipped: %v (%+v)", disk.Source, err, instance)
				continue
			}

			nodesByVolumeId[volumeId] = append(nodesByVolumeId[volumeId], instanceId)
			if multiZoneVolumeId, isMultiZone := multiZoneVolumeIdsByVolumeId[volumeId]; isMultiZone {
				nodesByVolumeId[multiZoneVolumeId] = append(nodesByVolumeId[multiZoneVolumeId], instanceId)
			}
		}
	}

	for volumeId, nodeIds := range nodesByVolumeId {
		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId: volumeId,
			},
			Status: &csi.ListVolumesResponse_VolumeStatus{
				PublishedNodeIds: nodeIds,
			},
		})
	}
	return entries
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

	volumeIsMultiZone := isMultiZoneVolKey(volKey)
	if gceCS.multiZoneVolumeHandleConfig.Enable && volumeIsMultiZone {
		return nil, status.Errorf(codes.InvalidArgument, "CreateSnapshot for volume %v failed. Snapshots are not supported with the multi-zone PV volumeHandle feature", volumeID)
	}

	if acquired := gceCS.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, constants.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer gceCS.volumeLocks.Release(volumeID)

	// Check if volume exists
	disk, err := gceCS.CloudProvider.GetDisk(ctx, project, volKey)
	metrics.UpdateRequestMetadataFromDisk(ctx, disk)
	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "CreateSnapshot could not find disk %v: %v", volKey.String(), err.Error())
		}
		return nil, common.LoggedError("CreateSnapshot, failed to getDisk: ", err)
	}

	snapshotParams, err := parameters.ExtractAndDefaultSnapshotParameters(req.GetParameters(), gceCS.Driver.name, gceCS.Driver.extraTags)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid snapshot parameters: %v", err.Error())
	}

	var snapshot *csi.Snapshot
	switch snapshotParams.SnapshotType {
	case parameters.DiskSnapshotType:
		snapshot, err = gceCS.createPDSnapshot(ctx, project, volKey, req.Name, snapshotParams)
		if err != nil {
			return nil, err
		}
	case parameters.DiskImageType:
		if disk.LocationType() == meta.Regional {
			return nil, status.Errorf(codes.InvalidArgument, "Cannot create backup type %s for regional disk %s", parameters.DiskImageType, disk.GetName())
		}
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

func (gceCS *GCEControllerServer) createPDSnapshot(ctx context.Context, project string, volKey *meta.Key, snapshotName string, snapshotParams parameters.SnapshotParameters) (*csi.Snapshot, error) {
	volumeID, err := common.KeyToVolumeID(volKey, project)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid volume key: %v", volKey)
	}

	// Check if PD snapshot already exists
	var snapshot *compute.Snapshot
	snapshot, err = gceCS.CloudProvider.GetSnapshot(ctx, project, snapshotName)
	if err != nil {
		if !gce.IsGCEError(err, "notFound") {
			return nil, common.LoggedError("Failed to get snapshot: ", err)
		}
		// If we could not find the snapshot, we create a new one
		snapshot, err = gceCS.CloudProvider.CreateSnapshot(ctx, project, volKey, snapshotName, snapshotParams)
		if err != nil {
			if gce.IsGCEError(err, "notFound") {
				return nil, status.Errorf(codes.NotFound, "Could not find volume with ID %v: %v", volKey.String(), err.Error())
			}

			// Identified as incorrect error handling
			if gce.IsSnapshotAlreadyExistsError(err) {
				return nil, status.Errorf(codes.AlreadyExists, "Snapshot already exists: %v", err.Error())
			}

			// Identified as incorrect error handling
			if gce.IsGCPOrgViolationError(err) {
				return nil, status.Errorf(codes.FailedPrecondition, "Violates GCP org policy: %v", err.Error())
			}
			return nil, common.LoggedError("Failed to create snapshot: ", err)
		}
	}
	snapshotId, err := getResourceId(snapshot.SelfLink)
	if err != nil {
		return nil, common.LoggedError(fmt.Sprintf("Cannot extract resource id from snapshot %s", snapshot.SelfLink), err)
	}

	err = gceCS.validateExistingSnapshot(snapshot, volKey)
	if err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "Error in creating snapshot: %v", err.Error())
	}

	timestamp, err := parseTimestamp(snapshot.CreationTimestamp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to covert creation timestamp: %v", err.Error())
	}

	ready, err := isCSISnapshotReady(snapshot)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Snapshot had error checking ready status: %v", err.Error())
	}

	return &csi.Snapshot{
		SizeBytes:      common.GbToBytes(snapshot.DiskSizeGb),
		SnapshotId:     snapshotId,
		SourceVolumeId: volumeID,
		CreationTime:   timestamp,
		ReadyToUse:     ready,
	}, nil
}

func (gceCS *GCEControllerServer) createImage(ctx context.Context, project string, volKey *meta.Key, imageName string, snapshotParams parameters.SnapshotParameters) (*csi.Snapshot, error) {
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
	imageId, err := getResourceId(image.SelfLink)
	if err != nil {
		return nil, common.LoggedError(fmt.Sprintf("Cannot extract resource id from snapshot %s", image.SelfLink), err)
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
		SnapshotId:     imageId,
		SourceVolumeId: volumeID,
		CreationTime:   timestamp,
		ReadyToUse:     ready,
	}, nil
}

func (gceCS *GCEControllerServer) validateExistingImage(image *compute.Image, volKey *meta.Key) error {
	if image == nil {
		return fmt.Errorf("disk does not exist")
	}

	sourceId, err := getResourceId(image.SourceDisk)
	if err != nil {
		return fmt.Errorf("failed to get source id from %s: %w", image.SourceDisk, err)
	}
	_, sourceKey, err := common.VolumeIDToKey(sourceId)
	if err != nil {
		return fmt.Errorf("failed to get source disk key %s: %w", image.SourceDisk, err)
	}

	if sourceKey.String() != volKey.String() {
		return fmt.Errorf("image already exists with same name but with a different disk source %s, expected disk source %s", sourceKey.String(), volKey.String())
	}

	klog.V(5).Infof("Compatible image %s exists with source disk %s.", image.Name, image.SourceDisk)
	return nil
}

func parseTimestamp(creationTimestamp string) (*timestamppb.Timestamp, error) {
	t, err := time.Parse(time.RFC3339, creationTimestamp)
	if err != nil {
		return nil, err
	}

	timestamp := timestamppb.New(t)
	if err := timestamp.CheckValid(); err != nil {
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

	sourceId, err := getResourceId(snapshot.SourceDisk)
	if err != nil {
		return fmt.Errorf("failed to get source id from %s: %w", snapshot.SourceDisk, err)
	}
	_, sourceKey, err := common.VolumeIDToKey(sourceId)
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

func isCSISnapshotReady(snapshot *compute.Snapshot) (bool, error) {
	klog.V(4).Infof("snapshot %s is %s", snapshot.SelfLink, snapshot.Status)
	switch snapshot.Status {
	case "READY":
		return true, nil
	case "FAILED":
		return false, fmt.Errorf("snapshot %s status is FAILED", snapshot.SelfLink)
	default:
		return false, nil
	}
}

func (gceCS *GCEControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	var err error
	// Validate arguments
	snapshotID := req.GetSnapshotId()
	if len(snapshotID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeleteSnapshot Snapshot ID must be provided")
	}

	project, snapshotType, key, err := common.SnapshotIDToProjectKey(snapshotID)
	if err != nil {
		// Cannot get snapshot ID from the passing request
		// This is a success according to the spec
		klog.Warningf("Snapshot id does not have the correct format %s: %v", snapshotID, err)
		return &csi.DeleteSnapshotResponse{}, nil
	}

	switch snapshotType {
	case parameters.DiskSnapshotType:
		err = gceCS.CloudProvider.DeleteSnapshot(ctx, project, key)
		if err != nil {
			return nil, common.LoggedError("Failed to DeleteSnapshot: ", err)
		}
	case parameters.DiskImageType:
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
	project, volKey, err = gceCS.CloudProvider.RepairUnderspecifiedVolumeKey(ctx, project, volKey, "")

	if err != nil {
		if gce.IsGCENotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "ControllerExpandVolume could not find volume with ID %v: %v", volumeID, err.Error())
		}
		return nil, common.LoggedError("ControllerExpandVolume error repairing underspecified volume key: ", err)
	}

	volumeIsMultiZone := isMultiZoneVolKey(volKey)
	if gceCS.multiZoneVolumeHandleConfig.Enable && volumeIsMultiZone {
		return nil, status.Errorf(codes.InvalidArgument, "ControllerExpandVolume is not supported with the multi-zone PVC volumeHandle feature. Please re-create the volume %v from source if you want a larger size", volumeID)
	}

	sourceDisk, err := gceCS.CloudProvider.GetDisk(ctx, project, volKey)
	metrics.UpdateRequestMetadataFromDisk(ctx, sourceDisk)
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
	case parameters.DiskSnapshotType:
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
	case parameters.DiskImageType:
		image, err := gceCS.CloudProvider.GetImage(ctx, project, key)
		if err != nil {
			if gce.IsGCEError(err, "notFound") {
				// return empty list if no snapshot is found
				return &csi.ListSnapshotsResponse{}, nil
			}
			return nil, common.LoggedError("Failed to get image snapshot: ", err)
		}
		e, err := generateDiskImageEntry(image)
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

	snapshotId, err := getResourceId(snapshot.SelfLink)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot id from %s: %w", snapshot.SelfLink, err)
	}
	sourceId, err := getResourceId(snapshot.SourceDisk)
	if err != nil {
		return nil, fmt.Errorf("failed to get source id from %s: %w", snapshot.SourceDisk, err)
	}

	// We ignore the error intentionally here since we are just listing snapshots
	// TODO: If the snapshot is in "FAILED" state we need to think through what this
	// should actually look like.
	ready, _ := isCSISnapshotReady(snapshot)

	entry := &csi.ListSnapshotsResponse_Entry{
		Snapshot: &csi.Snapshot{
			SizeBytes:      common.GbToBytes(snapshot.DiskSizeGb),
			SnapshotId:     snapshotId,
			SourceVolumeId: sourceId,
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

	imageId, err := getResourceId(image.SelfLink)
	if err != nil {
		return nil, fmt.Errorf("cannot get image id from %s: %w", image.SelfLink, err)
	}
	sourceId, err := getResourceId(image.SourceDisk)
	if err != nil {
		return nil, fmt.Errorf("cannot get source id from %s: %w", image.SourceDisk, err)
	}

	ready, _ := isImageReady(image.Status)

	entry := &csi.ListSnapshotsResponse_Entry{
		Snapshot: &csi.Snapshot{
			SizeBytes:      common.GbToBytes(image.DiskSizeGb),
			SnapshotId:     imageId,
			SourceVolumeId: sourceId,
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

func pickZonesFromTopology(top *csi.TopologyRequirement, numZones int, locationTopReq *locationRequirements, fallbackRequisiteZones []string) ([]string, error) {
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
		if locationTopReq.cloneIsRegional {
			// For zonal or regional -> regional disk cloning, the source disk region must match the destination disk region.
			srcVolRegion := locationTopReq.srcVolRegion
			prefZones = pickZonesInRegion(srcVolRegion, prefZones)
			reqZones = pickZonesInRegion(srcVolRegion, reqZones)

			if len(prefZones) == 0 && len(reqZones) == 0 {
				volumeCloningReq := fmt.Sprintf("clone zone must reside in source disk region %s", srcVolRegion)
				return nil, fmt.Errorf("failed to find zone from topology %v: %s", top, volumeCloningReq)
			}

			// For zonal -> regional disk cloning, one of the replicated zones must match the source zone.
			if !locationTopReq.srcIsRegional {
				if !slices.Contains(prefZones, srcVolZone) && !slices.Contains(reqZones, srcVolZone) {
					volumeCloningReq := fmt.Sprintf("one of the replica zones of the clone must match the source disk zone: %s", srcVolZone)
					return nil, fmt.Errorf("failed to find zone from topology %v: %s", top, volumeCloningReq)
				}
				prefZones = prependZone(srcVolZone, prefZones)
			}
		} else {
			// For zonal -> zonal cloning, the source disk zone must match the destination disk zone.
			if !slices.Contains(prefZones, srcVolZone) && !slices.Contains(reqZones, srcVolZone) {
				// If the source volume zone is not in the topology requirement, we return an error.
				volumeCloningReq := fmt.Sprintf("clone zone must match source disk zone: %s", srcVolZone)
				return nil, fmt.Errorf("failed to find zone from topology %v: %s", top, volumeCloningReq)
			}
			return []string{srcVolZone}, nil
		}
	}

	if numZones <= len(prefZones) {
		return prefZones[0:numZones], nil
	}

	remainingNumZones := numZones - len(prefZones)
	// Take all of the remaining zones from requisite zones
	reqSet := sets.NewString(reqZones...)
	prefSet := sets.NewString(prefZones...)
	remainingZones := reqSet.Difference(prefSet)

	if remainingZones.Len() < remainingNumZones {
		fallbackSet := sets.NewString(fallbackRequisiteZones...)
		remainingFallbackZones := fallbackSet.Difference(prefSet)
		if remainingFallbackZones.Len() >= remainingNumZones {
			remainingZones = remainingFallbackZones
		} else {
			return nil, fmt.Errorf("need %v zones from topology, only got %v unique zones", numZones, reqSet.Union(prefSet).Len())
		}
	}

	allZones := prefSet.Union(remainingZones).List()
	sort.Strings(allZones)
	var shiftIndex int
	if len(prefZones) == 0 {
		// Random shift the requisite zones, since there is no preferred start.
		shiftIndex = rand.Intn(len(allZones))
	} else {
		shiftIndex = slices.Index(allZones, prefZones[0])
	}
	shiftedZones := append(allZones[shiftIndex:], allZones[:shiftIndex]...)
	sortedShiftedReqZones := slices.Filter(nil, shiftedZones, func(v string) bool { return !prefSet.Has(v) })
	zones := make([]string, 0, numZones)
	zones = append(zones, prefZones...)
	zones = append(zones, sortedShiftedReqZones...)
	return zones[:numZones], nil
}

func getZonesFromTopology(topList []*csi.Topology) ([]string, error) {
	zones := []string{}
	for _, top := range topList {
		if top.GetSegments() == nil {
			return nil, fmt.Errorf("topologies specified but no segments")
		}

		// GCE PD cloud provider Create has no restrictions so just create in top preferred zone
		zone, err := getZoneFromSegment(top.GetSegments())
		if err != nil {
			return nil, fmt.Errorf("could not get zone from topology: %w", err)
		}
		zones = append(zones, zone)
	}
	return zones, nil
}

func getZoneFromSegment(seg map[string]string) (string, error) {
	var zone string
	for k, v := range seg {
		switch k {
		case constants.TopologyKeyZone:
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

func (gceCS *GCEControllerServer) pickZones(ctx context.Context, top *csi.TopologyRequirement, numZones int, locationTopReq *locationRequirements) ([]string, error) {
	var zones []string
	var err error
	if top != nil {
		zones, err = pickZonesFromTopology(top, numZones, locationTopReq, gceCS.fallbackRequisiteZones)
		if err != nil {
			return nil, fmt.Errorf("failed to pick zones from topology: %w", err)
		}
	} else {
		existingZones := []string{gceCS.CloudProvider.GetDefaultZone()}
		// We set existingZones to the source volume zone so that for zonal -> zonal cloning, the clone is provisioned
		// in the same zone as the source volume, and for zonal -> regional, one of the replicated zones will always
		// be the zone of the source volume. For regional -> regional cloning, the srcVolZone will not be set, so we
		// just use the default zone.
		if locationTopReq != nil && !locationTopReq.srcIsRegional {
			existingZones = []string{locationTopReq.srcVolZone}
		}
		// If topology is nil, then the Immediate binding mode was used without setting allowedTopologies in the storageclass.
		zones, err = getNumDefaultZonesInRegion(ctx, gceCS, existingZones, numZones)
		if err != nil {
			return nil, fmt.Errorf("failed to get default %v zones in region: %w", numZones, err)
		}
		klog.Warningf("No zones have been specified in either topology or params, picking default zone: %v", zones)

	}
	return zones, nil
}

func getDefaultZonesInRegion(ctx context.Context, gceCS *GCEControllerServer, existingZones []string) ([]string, error) {
	region, err := common.GetRegionFromZones(existingZones)
	if err != nil {
		return nil, fmt.Errorf("failed to get region from zones: %w", err)
	}
	totZones, err := gceCS.CloudProvider.ListZones(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("failed to list zones from cloud provider: %w", err)
	}
	return totZones, nil
}

func getNumDefaultZonesInRegion(ctx context.Context, gceCS *GCEControllerServer, existingZones []string, numZones int) ([]string, error) {
	needToGet := numZones - len(existingZones)
	totZones, err := getDefaultZonesInRegion(ctx, gceCS, existingZones)
	if err != nil {
		return nil, err
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

func paramsToVolumeContext(params parameters.DiskParameters) map[string]string {
	context := map[string]string{}
	if params.ForceAttach {
		context[contextForceAttach] = "true"
	}
	if len(context) > 0 {
		return context
	}
	return nil
}

func extractVolumeContext(context map[string]string) (*PDCSIContext, error) {
	info := &PDCSIContext{}
	// Note that sidecars may inject values in the context (eg,
	// csiProvisionerIdentity). So we don't validate that all keys are known.
	for key, val := range context {
		switch key {
		case contextForceAttach:
			b, err := convert.ConvertStringToBool(val)
			if err != nil {
				return nil, fmt.Errorf("bad volume context force attach: %w", err)
			}
			info.ForceAttach = b
		}
	}
	return info, nil
}

func (gceCS *GCEControllerServer) generateCreateVolumeResponseWithVolumeId(disk *gce.CloudDisk, zones []string, params parameters.DiskParameters, dataCacheParams parameters.DataCacheParameters, enableDataCache bool, volumeId string) *csi.CreateVolumeResponse {
	tops := []*csi.Topology{}
	for _, zone := range zones {
		top := &csi.Topology{
			Segments: map[string]string{
				constants.TopologyKeyZone: zone,
			},
		}

		// The addition of the disk type label requires both the feature to be
		// enabled on the PDCSI binary via the `--disk-topology=true` flag AND
		// the StorageClass to have the `use-allowed-disk-topology` parameter
		// set to `true`.
		if gceCS.EnableDiskTopology && params.UseAllowedDiskTopology {
			top.Segments[common.DiskTypeLabelKey(params.DiskType)] = "true"
		}

		tops = append(tops, top)
	}

	realDiskSizeBytes := common.GbToBytes(disk.GetSizeGb())
	createResp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes:      realDiskSizeBytes,
			VolumeId:           volumeId,
			VolumeContext:      paramsToVolumeContext(params),
			AccessibleTopology: tops,
		},
	}
	// Set data cache volume context
	if enableDataCache && dataCacheParams != (parameters.DataCacheParameters{}) {
		if createResp.Volume.VolumeContext == nil {
			createResp.Volume.VolumeContext = map[string]string{}
		}
		createResp.Volume.VolumeContext[constants.ContextDataCacheMode] = dataCacheParams.DataCacheMode
		createResp.Volume.VolumeContext[constants.ContextDataCacheSize] = dataCacheParams.DataCacheSize
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

func getResourceId(resourceLink string) (string, error) {
	url, err := neturl.Parse(resourceLink)
	if err != nil {
		return "", fmt.Errorf("could not parse resource %s: %w", resourceLink, err)
	}
	if url.Scheme != resourceApiScheme {
		return "", fmt.Errorf("unexpected API scheme for resource %s", resourceLink)
	}

	// Note that the resource host can basically be anything, if we are running in
	// a distributed cloud or trusted partner environment.

	// The path should be /compute/VERSION/project/....
	elts := strings.Split(url.Path, "/")
	if len(elts) < 4 {
		return "", fmt.Errorf("short resource path %s", resourceLink)
	}
	if elts[1] != resourceApiService {
		return "", fmt.Errorf("bad resource service %s in %s", elts[1], resourceLink)
	}
	if _, ok := validResourceApiVersions[elts[2]]; !ok {
		return "", fmt.Errorf("bad version %s in %s", elts[2], resourceLink)
	}
	if elts[3] != resourceProject {
		return "", fmt.Errorf("expected %v to start with %s in resource %s", elts[3:], resourceProject, resourceLink)
	}
	return strings.Join(elts[3:], "/"), nil
}

func createRegionalDisk(ctx context.Context, cloudProvider gce.GCECompute, name string, zones []string, params parameters.DiskParameters, capacityRange *csi.CapacityRange, capBytes int64, snapshotID string, volumeContentSourceVolumeID string, multiWriter bool, accessMode string) (*gce.CloudDisk, error) {
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

	err = cloudProvider.InsertDisk(ctx, project, meta.RegionalKey(name, region), params, capBytes, capacityRange, fullyQualifiedReplicaZones, snapshotID, volumeContentSourceVolumeID, multiWriter, accessMode)
	if err != nil {
		return nil, fmt.Errorf("failed to insert regional disk: %w", err)
	}

	// failed to GetDisk, however the Disk may already be created, the error code should be non-Final
	disk, err := cloudProvider.GetDisk(ctx, project, meta.RegionalKey(name, region))
	if err != nil {
		return nil, common.NewTemporaryError(codes.Unavailable, fmt.Errorf("failed to get disk after creating regional disk: %w", err))
	}
	return disk, nil
}

func createSingleZoneDisk(ctx context.Context, cloudProvider gce.GCECompute, name string, zones []string, params parameters.DiskParameters, capacityRange *csi.CapacityRange, capBytes int64, snapshotID string, volumeContentSourceVolumeID string, multiWriter bool, accessMode string) (*gce.CloudDisk, error) {
	project := cloudProvider.GetDefaultProject()
	if len(zones) != 1 {
		return nil, fmt.Errorf("got wrong number of zones for zonal create volume: %v", len(zones))
	}
	diskZone := zones[0]
	err := cloudProvider.InsertDisk(ctx, project, meta.ZonalKey(name, diskZone), params, capBytes, capacityRange, nil, snapshotID, volumeContentSourceVolumeID, multiWriter, accessMode)
	if err != nil {
		return nil, fmt.Errorf("failed to insert zonal disk: %w", err)
	}

	// failed to GetDisk, however the Disk may already be created, the error code should be non-Final
	disk, err := cloudProvider.GetDisk(ctx, project, meta.ZonalKey(name, diskZone))
	if err != nil {
		return nil, common.NewTemporaryError(codes.Unavailable, fmt.Errorf("failed to get disk after creating zonal disk: %w", err))
	}
	return disk, nil
}

func newCsiErrorBackoff(initialDuration, errorBackoffMaxDuration time.Duration) *csiErrorBackoff {
	return &csiErrorBackoff{flowcontrol.NewBackOff(initialDuration, errorBackoffMaxDuration), make(map[csiErrorBackoffId]codes.Code)}
}

func (_ *csiErrorBackoff) backoffId(nodeId, volumeId string) csiErrorBackoffId {
	return csiErrorBackoffId(fmt.Sprintf("%s:%s", nodeId, volumeId))
}

func (b *csiErrorBackoff) blocking(id csiErrorBackoffId) bool {
	blk := b.backoff.IsInBackOffSinceUpdate(string(id), b.backoff.Clock.Now())
	return blk
}

func (b *csiErrorBackoff) code(id csiErrorBackoffId) codes.Code {
	if code, ok := b.errorCodes[id]; ok {
		return code
	}
	// If we haven't recorded a code, return unavailable, which signals a problem with the driver
	// (ie, next() wasn't called correctly).
	klog.Errorf("using default code for %s", id)
	return codes.Internal
}

func (b *csiErrorBackoff) next(id csiErrorBackoffId, code codes.Code) {
	b.backoff.Next(string(id), b.backoff.Clock.Now())
	b.errorCodes[id] = code
}

func (b *csiErrorBackoff) reset(id csiErrorBackoffId) {
	b.backoff.Reset(string(id))
	delete(b.errorCodes, id)
}
