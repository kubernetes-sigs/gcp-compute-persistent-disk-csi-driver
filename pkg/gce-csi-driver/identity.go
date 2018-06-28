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
	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GCEIdentityServer struct {
	Driver *GCEDriver
}

// GetPluginInfo(context.Context, *GetPluginInfoRequest) (*GetPluginInfoResponse, error)
func (gceIdentity *GCEIdentityServer) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	glog.V(5).Infof("Using default GetPluginInfo")

	if gceIdentity.Driver.name == "" {
		return nil, status.Error(codes.Unavailable, "Driver name not configured")
	}

	return &csi.GetPluginInfoResponse{
		Name: gceIdentity.Driver.name,
	}, nil
}

func (gceIdentity *GCEIdentityServer) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	glog.V(5).Infof("Using default GetPluginCapabilities")
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
		},
	}, nil
}

func (gceIdentity *GCEIdentityServer) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	glog.Infof("Probe called with args: %#v", req)
	return &csi.ProbeResponse{}, nil
}
