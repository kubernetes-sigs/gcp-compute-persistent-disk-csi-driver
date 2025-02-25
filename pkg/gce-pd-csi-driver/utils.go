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

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

const (
	fsTypeXFS = "xfs"
)

var (
	ProbeCSIFullMethod = "/csi.v1.Identity/Probe"

	csiAccessModeToHyperdiskMode = map[csi.VolumeCapability_AccessMode_Mode]string{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER:      common.GCEReadWriteOnceAccessMode,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY:  common.GCEReadOnlyManyAccessMode,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER: common.GCEReadWriteManyAccessMode,
	}

	supportedMultiAttachAccessModes = map[csi.VolumeCapability_AccessMode_Mode]bool{
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY:  true,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER: true,
	}
)

func NewVolumeCapabilityAccessMode(mode csi.VolumeCapability_AccessMode_Mode) *csi.VolumeCapability_AccessMode {
	return &csi.VolumeCapability_AccessMode{Mode: mode}
}

func NewControllerServiceCapability(cap csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
	return &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: &csi.ControllerServiceCapability_RPC{
				Type: cap,
			},
		},
	}
}

func NewNodeServiceCapability(cap csi.NodeServiceCapability_RPC_Type) *csi.NodeServiceCapability {
	return &csi.NodeServiceCapability{
		Type: &csi.NodeServiceCapability_Rpc{
			Rpc: &csi.NodeServiceCapability_RPC{
				Type: cap,
			},
		},
	}
}

func logGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if info.FullMethod == ProbeCSIFullMethod {
		return handler(ctx, req)
	}
	// Note that secrets are not included in any RPC message. In the past protosanitizer and other log
	// stripping was shown to cause a significant increase of CPU usage (see
	// https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/issues/356#issuecomment-550529004).
	klog.V(4).Infof("%s called with request: %s", info.FullMethod, req)
	resp, err := handler(ctx, req)
	if err != nil {
		klog.Errorf("%s returned with error: %v", info.FullMethod, err.Error())
	} else {
		cappedStr := fmt.Sprintf("%v", resp)
		if len(cappedStr) > maxLogChar {
			cappedStr = cappedStr[:maxLogChar] + fmt.Sprintf(" [response body too large, log capped to %d chars]", maxLogChar)
		}
		klog.V(4).Infof("%s returned with response: %s", info.FullMethod, cappedStr)
	}
	return resp, err
}

func validateVolumeCapabilities(vcs []*csi.VolumeCapability) error {
	isMnt := false
	isBlk := false

	if vcs == nil {
		return errors.New("volume capabilities is nil")
	}

	for _, vc := range vcs {
		if err := validateVolumeCapability(vc); err != nil {
			return err
		}
		if blk := vc.GetBlock(); blk != nil {
			isBlk = true
		}
		if mnt := vc.GetMount(); mnt != nil {
			isMnt = true
		}
	}

	if isBlk && isMnt {
		return errors.New("both mount and block volume capabilities specified")
	}

	return nil
}

func validateVolumeCapability(vc *csi.VolumeCapability) error {
	if err := validateAccessMode(vc.GetAccessMode()); err != nil {
		return err
	}
	blk := vc.GetBlock()
	mnt := vc.GetMount()
	mod := vc.GetAccessMode().GetMode()
	if mnt == nil && blk == nil {
		return errors.New("must specify an access type")
	}
	if mnt != nil && blk != nil {
		return errors.New("specified both mount and block access types")
	}
	if mnt != nil && mod == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
		return errors.New("specified multi writer with mount access type")
	}
	return nil
}

func validateAccessMode(am *csi.VolumeCapability_AccessMode) error {
	if am == nil {
		return errors.New("access mode is nil")
	}

	switch am.GetMode() {
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER:
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY:
	case csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY:
	case csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER:
	default:
		return fmt.Errorf("%v access mode is not supported for GCE disks", am.GetMode())
	}
	return nil
}

func validateStoragePools(req *csi.CreateVolumeRequest, params common.DiskParameters, project string) error {
	storagePoolsEnabled := params.StoragePools != nil
	if !storagePoolsEnabled || req == nil {
		return nil
	}

	if params.EnableConfidentialCompute {
		return fmt.Errorf("storage pools do not support confidential storage")
	}

	if !(params.DiskType == "hyperdisk-balanced" || params.DiskType == "hyperdisk-throughput") {
		return fmt.Errorf("invalid disk-type: %q. storage pools only support hyperdisk-balanced or hyperdisk-throughput", params.DiskType)
	}

	if params.IsRegional() {
		return fmt.Errorf("storage pools do not support regional disks")
	}

	if useVolumeCloning(req) {
		return fmt.Errorf("storage pools do not support disk clones")
	}

	// Check that requisite zones matches the storage pools zones.
	// This means allowedTopologies was set properly by GCW.
	if err := validateStoragePoolZones(req, params.StoragePools); err != nil {
		return fmt.Errorf("failed to validate storage pools zones: %v", err)
	}

	// Check that Storage Pools are in same project as the GCEClient that creates the volume.
	if err := validateStoragePoolProjects(project, params.StoragePools); err != nil {
		return fmt.Errorf("failed to validate storage pools projects: %v", err)
	}

	return nil
}

func validateStoragePoolZones(req *csi.CreateVolumeRequest, storagePools []common.StoragePool) error {
	storagePoolZones, err := common.StoragePoolZones(storagePools)
	if err != nil {
		return err
	}
	reqZones, err := getZonesFromTopology(req.GetAccessibilityRequirements().GetRequisite())
	if err != nil {
		return err
	}
	if !common.UnorderedSlicesEqual(storagePoolZones, reqZones) {
		return fmt.Errorf("requisite topologies must match storage pools zones. requisite zones: %v, storage pools zones: %v", reqZones, storagePoolZones)
	}
	return nil
}

func validateStoragePoolProjects(project string, storagePools []common.StoragePool) error {
	spProjects := sets.String{}
	for _, sp := range storagePools {
		if sp.Project != project {
			spProjects.Insert(sp.Project)
			return fmt.Errorf("cross-project storage pools usage is not supported. Trying to CreateVolume in project %q with storage pools in projects %v", project, spProjects.UnsortedList())
		}
	}
	return nil
}

func getMultiWriterFromCapability(vc *csi.VolumeCapability) (bool, error) {
	if vc.GetAccessMode() == nil {
		return false, errors.New("access mode is nil")
	}
	mode := vc.GetAccessMode().GetMode()
	return (mode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER), nil
}

func getMultiWriterFromCapabilities(vcs []*csi.VolumeCapability) (bool, error) {
	if vcs == nil {
		return false, errors.New("volume capabilities is nil")
	}
	for _, vc := range vcs {
		multiWriter, err := getMultiWriterFromCapability(vc)
		if err != nil {
			return false, err
		}
		if multiWriter {
			return true, nil
		}
	}
	return false, nil
}

func getReadOnlyFromCapability(vc *csi.VolumeCapability) (bool, error) {
	if vc.GetAccessMode() == nil {
		return false, errors.New("access mode is nil")
	}
	mode := vc.GetAccessMode().GetMode()
	return (mode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY ||
		mode == csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY), nil
}

func getReadOnlyFromCapabilities(vcs []*csi.VolumeCapability) (bool, error) {
	if vcs == nil {
		return false, errors.New("volume capabilities is nil")
	}
	for _, vc := range vcs {
		readOnly, err := getReadOnlyFromCapability(vc)
		if err != nil {
			return false, err
		}
		if readOnly {
			return true, nil
		}
	}
	return false, nil
}

func getMultiAttachementFromCapabilities(vcs []*csi.VolumeCapability) (bool, error) {
	if vcs == nil {
		return false, errors.New("volume capabilities is nil")
	}
	for _, vc := range vcs {
		if vc.GetAccessMode() == nil {
			return false, errors.New("access mode is nil")
		}
		mode := vc.GetAccessMode().GetMode()
		if isMultiAttach, ok := supportedMultiAttachAccessModes[mode]; !ok {
			return false, nil
		} else {
			return isMultiAttach, nil
		}
	}
	return false, nil
}

func getHyperdiskAccessModeFromCapabilities(vcs []*csi.VolumeCapability) (string, error) {
	if vcs == nil {
		return "", errors.New("volume capabilities is nil")
	}
	for _, vc := range vcs {
		if vc.GetAccessMode() == nil {
			return "", errors.New("access mode is nil")
		}
		mode := vc.GetAccessMode().GetMode()
		if am, ok := csiAccessModeToHyperdiskMode[mode]; !ok {
			return "", errors.New("found unsupported access mode for hyperdisk")
		} else {
			return am, nil
		}
	}
	return "", errors.New("volume capabilities is nil")
}

func collectMountOptions(fsType string, mntFlags []string) []string {
	var options []string

	for _, opt := range mntFlags {
		if readAheadKBMountFlagRegex.FindString(opt) != "" {
			// The read_ahead_kb flag is a special flag that isn't
			// passed directly as an option to the mount command.
			continue
		}
		options = append(options, opt)
	}

	// By default, xfs does not allow mounting of two volumes with the same filesystem uuid.
	// Force ignore this uuid to be able to mount volume + its clone / restored snapshot on the same node.
	if fsType == fsTypeXFS {
		options = append(options, "nouuid")
	}
	return options
}

func containsZone(zones []string, zone string) bool {
	for _, z := range zones {
		if z == zone {
			return true
		}
	}

	return false
}
