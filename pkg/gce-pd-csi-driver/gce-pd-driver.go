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

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	"k8s.io/mount-utils"
	common "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	metadataservice "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/metadata"
	mountmanager "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"
)

type GCEDriver struct {
	name              string
	vendorVersion     string
	extraVolumeLabels map[string]string

	ids *GCEIdentityServer
	ns  *GCENodeServer
	cs  *GCEControllerServer

	vcap  []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability
	nscap []*csi.NodeServiceCapability
}

func GetGCEDriver() *GCEDriver {
	return &GCEDriver{}
}

func (gceDriver *GCEDriver) SetupGCEDriver(name, vendorVersion string, extraVolumeLabels map[string]string, identityServer *GCEIdentityServer, controllerServer *GCEControllerServer, nodeServer *GCENodeServer) error {
	if name == "" {
		return fmt.Errorf("Driver name missing")
	}

	// Adding Capabilities
	vcam := []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	}
	gceDriver.AddVolumeCapabilityAccessModes(vcam)
	csc := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		csi.ControllerServiceCapability_RPC_PUBLISH_READONLY,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
	}
	gceDriver.AddControllerServiceCapabilities(csc)
	ns := []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
		csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
	}
	gceDriver.AddNodeServiceCapabilities(ns)

	gceDriver.name = name
	gceDriver.vendorVersion = vendorVersion
	gceDriver.extraVolumeLabels = extraVolumeLabels
	gceDriver.ids = identityServer
	gceDriver.cs = controllerServer
	gceDriver.ns = nodeServer

	return nil
}

func (gceDriver *GCEDriver) AddVolumeCapabilityAccessModes(vc []csi.VolumeCapability_AccessMode_Mode) error {
	var vca []*csi.VolumeCapability_AccessMode
	for _, c := range vc {
		klog.V(4).Infof("Enabling volume access mode: %v", c.String())
		vca = append(vca, NewVolumeCapabilityAccessMode(c))
	}
	gceDriver.vcap = vca
	return nil
}

func (gceDriver *GCEDriver) AddControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) error {
	var csc []*csi.ControllerServiceCapability
	for _, c := range cl {
		klog.V(4).Infof("Enabling controller service capability: %v", c.String())
		csc = append(csc, NewControllerServiceCapability(c))
	}
	gceDriver.cscap = csc
	return nil
}

func (gceDriver *GCEDriver) AddNodeServiceCapabilities(nl []csi.NodeServiceCapability_RPC_Type) error {
	var nsc []*csi.NodeServiceCapability
	for _, n := range nl {
		klog.V(4).Infof("Enabling node service capability: %v", n.String())
		nsc = append(nsc, NewNodeServiceCapability(n))
	}
	gceDriver.nscap = nsc
	return nil
}

func (gceDriver *GCEDriver) ValidateControllerServiceRequest(c csi.ControllerServiceCapability_RPC_Type) error {
	if c == csi.ControllerServiceCapability_RPC_UNKNOWN {
		return nil
	}

	for _, cap := range gceDriver.cscap {
		if c == cap.GetRpc().Type {
			return nil
		}
	}

	return status.Error(codes.InvalidArgument, "Invalid controller service request")
}

func NewIdentityServer(gceDriver *GCEDriver) *GCEIdentityServer {
	return &GCEIdentityServer{
		Driver: gceDriver,
	}
}

func NewNodeServer(gceDriver *GCEDriver, mounter *mount.SafeFormatAndMount, deviceUtils mountmanager.DeviceUtils, meta metadataservice.MetadataService, statter mountmanager.Statter) *GCENodeServer {
	return &GCENodeServer{
		Driver:          gceDriver,
		Mounter:         mounter,
		DeviceUtils:     deviceUtils,
		MetadataService: meta,
		volumeLocks:     common.NewVolumeLocks(),
		VolumeStatter:   statter,
	}
}

func NewControllerServer(gceDriver *GCEDriver, cloudProvider gce.GCECompute) *GCEControllerServer {
	return &GCEControllerServer{
		Driver:        gceDriver,
		CloudProvider: cloudProvider,
		seen:          map[string]int{},
		volumeLocks:   common.NewVolumeLocks(),
	}
}

func (gceDriver *GCEDriver) Run(endpoint string) {
	klog.V(4).Infof("Driver: %v", gceDriver.name)

	//Start the nonblocking GRPC
	s := NewNonBlockingGRPCServer()
	// TODO(#34): Only start specific servers based on a flag.
	// In the future have this only run specific combinations of servers depending on which version this is.
	// The schema for that was in util. basically it was just s.start but with some nil servers.

	s.Start(endpoint, gceDriver.ids, gceDriver.cs, gceDriver.ns)
	s.Wait()
}
