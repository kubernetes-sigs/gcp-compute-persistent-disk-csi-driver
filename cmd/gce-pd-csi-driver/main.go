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

// Package main is the GCE PD CSI Driver entrypoint.
package main

import (
	"context"
	"flag"
	"math/rand"
	"os"
	"time"

	"k8s.io/klog"

	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	metadataservice "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/metadata"
	driver "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-pd-csi-driver"
	mountmanager "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"
)

var (
	cloudConfigFilePath  = flag.String("cloud-config", "", "Path to GCE cloud provider config")
	endpoint             = flag.String("endpoint", "unix:/tmp/csi.sock", "CSI endpoint")
	runControllerService = flag.Bool("run-controller-service", true, "If set to false then the CSI driver does not activate its controller service (default: true)")
	runNodeService       = flag.Bool("run-node-service", true, "If set to false then the CSI driver does not activate its node service (default: true)")
	version              string
)

const (
	driverName = "pd.csi.storage.gke.io"
)

func init() {
	// klog verbosity guide for this package
	// Use V(2) for one time config information
	// Use V(4) for general debug information logging
	// Use V(5) for GCE Cloud Provider Call informational logging
	// Use V(6) for extra repeated/polling information
	klog.InitFlags(flag.CommandLine)
	flag.Set("logtostderr", "true")
}

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	handle()
	os.Exit(0)
}

func handle() {
	var err error

	if version == "" {
		klog.Fatalf("version must be set at compile time")
	}
	klog.V(2).Infof("Driver vendor version %v", version)

	gceDriver := driver.GetGCEDriver()

	//Initialize GCE Driver
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//Initialize identity server
	identityServer := driver.NewIdentityServer(gceDriver)

	//Initialize requirements for the controller service
	var controllerServer *driver.GCEControllerServer
	if *runControllerService {
		cloudProvider, err := gce.CreateCloudProvider(ctx, version, *cloudConfigFilePath)
		if err != nil {
			klog.Fatalf("Failed to get cloud provider: %v", err)
		}
		controllerServer = driver.NewControllerServer(gceDriver, cloudProvider)
	} else if *cloudConfigFilePath != "" {
		klog.Warningf("controller service is disabled but cloud config given - it has no effect")
	}

	//Initialize requirements for the node service
	var nodeServer *driver.GCENodeServer
	if *runNodeService {
		mounter, err := mountmanager.NewSafeMounter()
		if err != nil {
			klog.Fatalf("Failed to get safe mounter: %v", err)
		}
		deviceUtils := mountmanager.NewDeviceUtils()
		statter := mountmanager.NewStatter(mounter)
		meta, err := metadataservice.NewMetadataService()
		if err != nil {
			klog.Fatalf("Failed to set up metadata service: %v", err)
		}
		nodeServer = driver.NewNodeServer(gceDriver, mounter, deviceUtils, meta, statter)
	}

	err = gceDriver.SetupGCEDriver(driverName, version, identityServer, controllerServer, nodeServer)
	if err != nil {
		klog.Fatalf("Failed to initialize GCE CSI Driver: %v", err)
	}

	gceDriver.Run(*endpoint)
}
