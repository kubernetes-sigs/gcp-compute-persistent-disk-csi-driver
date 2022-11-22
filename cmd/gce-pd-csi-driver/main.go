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
	"runtime"
	"time"

	"k8s.io/klog"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	metadataservice "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/metadata"
	driver "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-pd-csi-driver"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/metrics"
	mountmanager "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"
)

var (
	cloudConfigFilePath  = flag.String("cloud-config", "", "Path to GCE cloud provider config")
	endpoint             = flag.String("endpoint", "unix:/tmp/csi.sock", "CSI endpoint")
	computeEndpoint      = flag.String("compute-endpoint", "", "If set, used as the endpoint for the GCE API.")
	runControllerService = flag.Bool("run-controller-service", true, "If set to false then the CSI driver does not activate its controller service (default: true)")
	runNodeService       = flag.Bool("run-node-service", true, "If set to false then the CSI driver does not activate its node service (default: true)")
	httpEndpoint         = flag.String("http-endpoint", "", "The TCP network address where the prometheus metrics endpoint will listen (example: `:8080`). The default is empty string, which means metrics endpoint is disabled.")
	metricsPath          = flag.String("metrics-path", "/metrics", "The HTTP path where prometheus metrics will be exposed. Default is `/metrics`.")

	errorBackoffInitialDurationMs = flag.Int("backoff-initial-duration-ms", 200, "The amount of ms for the initial duration of the backoff condition for controller publish/unpublish CSI operations. Default is 200.")
	errorBackoffMaxDurationMs     = flag.Int("backoff-max-duration-ms", 300000, "The amount of ms for the max duration of the backoff condition for controller publish/unpublish CSI operations. Default is 300000 (5m).")
	extraVolumeLabelsStr          = flag.String("extra-labels", "", "Extra labels to attach to each PD created. It is a comma separated list of key value pairs like '<key1>=<value1>,<key2>=<value2>'. See https://cloud.google.com/compute/docs/labeling-resources for details")

	attachDiskBackoffDuration = flag.Duration("attach-disk-backoff-duration", 5*time.Second, "Duration for attachDisk backoff")
	attachDiskBackoffFactor   = flag.Float64("attach-disk-backoff-factor", 0.0, "Factor for attachDisk backoff")
	attachDiskBackoffJitter   = flag.Float64("attach-disk-backoff-jitter", 0.0, "Jitter for attachDisk backoff")
	attachDiskBackoffSteps    = flag.Int("attach-disk-backoff-steps", 24, "Steps for attachDisk backoff")
	attachDiskBackoffCap      = flag.Duration("attach-disk-backoff-cap", 0, "Cap for attachDisk backoff")
	waitForOpBackoffDuration  = flag.Duration("wait-op-backoff-duration", 3*time.Second, "Duration for wait for operation backoff")
	waitForOpBackoffFactor    = flag.Float64("wait-op-backoff-factor", 0.0, "Factor for wait for operation backoff")
	waitForOpBackoffJitter    = flag.Float64("wait-op-backoff-jitter", 0.0, "Jitter for wait for operation backoff")
	waitForOpBackoffSteps     = flag.Int("wait-op-backoff-steps", 100, "Steps for wait for operation backoff")
	waitForOpBackoffCap       = flag.Duration("wait-op-backoff-cap", 0, "Cap for wait for operation backoff")

	maxprocs = flag.Int("maxprocs", 1, "GOMAXPROCS override")

	version string
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

	runtime.GOMAXPROCS(*maxprocs)
	klog.Infof("Sys info: NumCPU: %v MAXPROC: %v", runtime.NumCPU(), runtime.GOMAXPROCS(0))

	if version == "" {
		klog.Fatalf("version must be set at compile time")
	}
	klog.V(2).Infof("Driver vendor version %v", version)

	if *runControllerService && *httpEndpoint != "" && metrics.IsGKEComponentVersionAvailable() {
		mm := metrics.NewMetricsManager()
		mm.InitializeHttpHandler(*httpEndpoint, *metricsPath)
		mm.EmitGKEComponentVersion()
	}

	if len(*extraVolumeLabelsStr) > 0 && !*runControllerService {
		klog.Fatalf("Extra volume labels provided but not running controller")
	}
	extraVolumeLabels, err := common.ConvertLabelsStringToMap(*extraVolumeLabelsStr)
	if err != nil {
		klog.Fatalf("Bad extra volume labels: %v", err)
	}

	gceDriver := driver.GetGCEDriver()

	//Initialize GCE Driver
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//Initialize identity server
	identityServer := driver.NewIdentityServer(gceDriver)

	//Initialize requirements for the controller service
	var controllerServer *driver.GCEControllerServer
	if *runControllerService {
		cloudProvider, err := gce.CreateCloudProvider(ctx, version, *cloudConfigFilePath, *computeEndpoint)
		if err != nil {
			klog.Fatalf("Failed to get cloud provider: %v", err)
		}
		initialBackoffDuration := time.Duration(*errorBackoffInitialDurationMs) * time.Millisecond
		maxBackoffDuration := time.Duration(*errorBackoffMaxDurationMs) * time.Microsecond
		controllerServer = driver.NewControllerServer(gceDriver, cloudProvider, initialBackoffDuration, maxBackoffDuration)
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

	err = gceDriver.SetupGCEDriver(driverName, version, extraVolumeLabels, identityServer, controllerServer, nodeServer)
	if err != nil {
		klog.Fatalf("Failed to initialize GCE CSI Driver: %v", err)
	}

	gce.AttachDiskBackoff.Duration = *attachDiskBackoffDuration
	gce.AttachDiskBackoff.Factor = *attachDiskBackoffFactor
	gce.AttachDiskBackoff.Jitter = *attachDiskBackoffJitter
	gce.AttachDiskBackoff.Steps = *attachDiskBackoffSteps
	gce.AttachDiskBackoff.Cap = *attachDiskBackoffCap

	gce.WaitForOpBackoff.Duration = *waitForOpBackoffDuration
	gce.WaitForOpBackoff.Factor = *waitForOpBackoffFactor
	gce.WaitForOpBackoff.Jitter = *waitForOpBackoffJitter
	gce.WaitForOpBackoff.Steps = *waitForOpBackoffSteps
	gce.WaitForOpBackoff.Cap = *waitForOpBackoffCap

	gceDriver.Run(*endpoint)
}
