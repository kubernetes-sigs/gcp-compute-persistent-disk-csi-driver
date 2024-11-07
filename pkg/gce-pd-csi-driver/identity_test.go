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
	"testing"

	"context"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
)

func TestGetPluginInfo(t *testing.T) {
	vendorVersion := "test-vendor"
	gceDriver := GetGCEDriver()
	identityServer := NewIdentityServer(gceDriver)
	err := gceDriver.SetupGCEDriver(driver, vendorVersion, nil, nil, nil, identityServer, nil, nil)
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}

	resp, err := gceDriver.ids.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})
	if err != nil {
		t.Fatalf("GetPluginInfo returned unexpected error: %v", err)
	}

	if resp.GetName() != driver {
		t.Fatalf("Response name expected: %v, got: %v", driver, resp.GetName())
	}

	respVer := resp.GetVendorVersion()
	if respVer != vendorVersion {
		t.Fatalf("Vendor version expected: %v, got: %v", vendorVersion, respVer)
	}
}

func TestGetPluginCapabilities(t *testing.T) {
	gceDriver := GetGCEDriver()
	identityServer := NewIdentityServer(gceDriver)
	err := gceDriver.SetupGCEDriver(driver, "test-vendor", nil, nil, nil, identityServer, nil, nil)
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}

	resp, err := gceDriver.ids.GetPluginCapabilities(context.Background(), &csi.GetPluginCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("GetPluginCapabilities returned unexpected error: %v", err)
	}

	for _, capability := range resp.GetCapabilities() {
		if capability.GetVolumeExpansion() != nil {
			switch capability.GetVolumeExpansion().GetType() {
			case csi.PluginCapability_VolumeExpansion_ONLINE:
			case csi.PluginCapability_VolumeExpansion_OFFLINE:
			default:
				t.Fatalf("Unknown capability: %v", capability.GetVolumeExpansion().GetType())
			}
		}
		if capability.GetService() != nil {
			switch capability.GetService().GetType() {
			case csi.PluginCapability_Service_CONTROLLER_SERVICE:
			case csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS:
			default:
				t.Fatalf("Unknown capability: %v", capability.GetService().GetType())
			}
		}
	}
}

func TestProbe(t *testing.T) {
	gceDriver := GetGCEDriver()
	identityServer := NewIdentityServer(gceDriver)
	err := gceDriver.SetupGCEDriver(driver, "test-vendor", nil, nil, nil, identityServer, nil, nil)
	if err != nil {
		t.Fatalf("Failed to setup GCE Driver: %v", err)
	}

	_, err = gceDriver.ids.Probe(context.Background(), &csi.ProbeRequest{})
	if err != nil {
		t.Fatalf("Probe returned unexpected error: %v", err)
	}
}
