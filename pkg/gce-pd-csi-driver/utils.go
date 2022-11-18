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
	"errors"
	"fmt"

	"context"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

const (
	fsTypeXFS = "xfs"
)

var ProbeCSIFullMethod = "/csi.v1.Identity/Probe"

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
		klog.Errorf("%s returned with error: %w", info.FullMethod, err)
	} else {
		klog.V(4).Infof("%s returned with response: %s", info.FullMethod, resp)
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
		return fmt.Errorf("%v access mode is not supported for for PD", am.GetMode())
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

func collectMountOptions(fsType string, mntFlags []string) []string {
	var options []string

	for _, opt := range mntFlags {
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
