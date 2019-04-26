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

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
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
	glog.V(3).Infof("GRPC call: %s", info.FullMethod)
	glog.V(5).Infof("GRPC request: %+v", protosanitizer.StripSecrets(req))
	resp, err := handler(ctx, req)
	if err != nil {
		glog.Errorf("GRPC error: %v", err)
	} else {
		glog.V(5).Infof("GRPC response: %+v", protosanitizer.StripSecrets(resp))
	}
	return resp, err
}

func validateVolumeCapabilities(vcs []*csi.VolumeCapability) error {
	if vcs == nil {
		return errors.New("volume capabilities is nil")
	}
	for _, vc := range vcs {
		if err := validateVolumeCapability(vc); err != nil {
			return err
		}
	}
	return nil
}

func validateVolumeCapability(vc *csi.VolumeCapability) error {
	if err := validateAccessModes(vc.GetAccessMode()); err != nil {
		return err
	}
	if blk := vc.GetBlock(); blk != nil {
		// TODO(#64): Block volume support
		return errors.New("Block volume support is not yet implemented")
	}
	if mnt := vc.GetMount(); mnt == nil {
		// TODO(#64): Change error message after block volume support
		return errors.New("Must specify an access type of Mount")
	}
	return nil
}

func validateAccessModes(am *csi.VolumeCapability_AccessMode) error {
	if am == nil {
		return errors.New("access mode is nil")
	}

	switch am.GetMode() {
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER:
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY:
	case csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY:
	case csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER:
		return errors.New("MULTI_NODE_MULTI_WRITER access mode is not yet supported for PD")
	default:
		return fmt.Errorf("%v access mode is not supported for for PD", am.GetMode())
	}
	return nil
}
