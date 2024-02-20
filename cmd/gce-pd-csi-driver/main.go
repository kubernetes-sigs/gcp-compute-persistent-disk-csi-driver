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
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/deviceutils"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	metadataservice "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/metadata"
	driver "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-pd-csi-driver"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/metrics"
	mountmanager "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/mount-manager"
)

var (
	cloudConfigFilePath  = flag.String("cloud-config", "", "Path to GCE cloud provider config")
	endpoint             = flag.String("endpoint", "unix:/tmp/csi.sock", "CSI endpoint")
	runControllerService = flag.Bool("run-controller-service", true, "If set to false then the CSI driver does not activate its controller service (default: true)")
	runNodeService       = flag.Bool("run-node-service", true, "If set to false then the CSI driver does not activate its node service (default: true)")
	httpEndpoint         = flag.String("http-endpoint", "", "The TCP network address where the prometheus metrics endpoint will listen (example: `:8080`). The default is empty string, which means metrics endpoint is disabled.")
	metricsPath          = flag.String("metrics-path", "/metrics", "The HTTP path where prometheus metrics will be exposed. Default is `/metrics`.")
	grpcLogCharCap       = flag.Int("grpc-log-char-cap", 10000, "The maximum amount of characters logged for every grpc responses")
	enableOtelTracing    = flag.Bool("enable-otel-tracing", false, "If set, enable opentelemetry tracing for the driver. The tracing is disabled by default. Configure the exporter endpoint with OTEL_EXPORTER_OTLP_ENDPOINT and other env variables, see https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/#general-sdk-configuration.")

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

	maxProcs                = flag.Int("maxprocs", 1, "GOMAXPROCS override")
	maxConcurrentFormat     = flag.Int("max-concurrent-format", 1, "The maximum number of concurrent format exec calls")
	concurrentFormatTimeout = flag.Duration("concurrent-format-timeout", 1*time.Minute, "The maximum duration of a format operation before its concurrency token is released")

	maxConcurrentFormatAndMount = flag.Int("max-concurrent-format-and-mount", 1, "If set then format and mount operations are serialized on each node. This is stronger than max-concurrent-format as it includes fsck and other mount operations")
	formatAndMountTimeout       = flag.Duration("format-and-mount-timeout", 1*time.Minute, "The maximum duration of a format and mount operation before another such operation will be started. Used only if --serialize-format-and-mount")
	fallbackRequisiteZonesFlag  = flag.String("fallback-requisite-zones", "", "Comma separated list of requisite zones that will be used if there are not sufficient zones present in requisite topologies when provisioning a disk")
	enableStoragePoolsFlag      = flag.Bool("enable-storage-pools", false, "If set to true, the CSI Driver will allow volumes to be provisioned in Storage Pools")

	multiZoneVolumeHandleDiskTypesFlag = flag.String("multi-zone-volume-handle-disk-types", "", "Comma separated list of allowed disk types that can use the multi-zone volumeHandle. Used only if --multi-zone-volume-handle-enable")
	multiZoneVolumeHandleEnableFlag    = flag.Bool("multi-zone-volume-handle-enable", false, "If set to true, the multi-zone volumeHandle feature will be enabled")

	computeEnvironment        gce.Environment = gce.EnvironmentProduction
	computeEndpoint           *url.URL
	allowedComputeEnvironment = []gce.Environment{gce.EnvironmentStaging, gce.EnvironmentProduction}

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
	enumFlag(&computeEnvironment, "compute-environment", allowedComputeEnvironment, "Operating compute environment")
	urlFlag(&computeEndpoint, "compute-endpoint", "Compute endpoint")
	klog.InitFlags(flag.CommandLine)
	flag.Set("logtostderr", "true")
}

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	klog.Infof("Operating compute environment set to: %s and computeEndpoint is set to: %v", computeEnvironment, computeEndpoint)
	handle()
	os.Exit(0)
}

func handle() {
	var err error

	runtime.GOMAXPROCS(*maxProcs)
	klog.Infof("Sys info: NumCPU: %v MAXPROC: %v", runtime.NumCPU(), runtime.GOMAXPROCS(0))

	if version == "" {
		klog.Fatalf("version must be set at compile time")
	}
	klog.V(2).Infof("Driver vendor version %v", version)

	// Start tracing as soon as possible
	if *enableOtelTracing {
		exporter, err := driver.InitOtelTracing()
		if err != nil {
			klog.Fatalf("Failed to initialize otel tracing: %v", err.Error())
		}
		// Exporter will flush traces on shutdown
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := exporter.Shutdown(ctx); err != nil {
				klog.Errorf("Could not shutdown otel exporter: %v", err.Error())
			}
		}()
	}

	if *runControllerService && *httpEndpoint != "" {
		mm := metrics.NewMetricsManager()
		mm.InitializeHttpHandler(*httpEndpoint, *metricsPath)
		mm.RegisterPDCSIMetric()

		if metrics.IsGKEComponentVersionAvailable() {
			mm.EmitGKEComponentVersion()
		}
	}

	if len(*extraVolumeLabelsStr) > 0 && !*runControllerService {
		klog.Fatalf("Extra volume labels provided but not running controller")
	}
	extraVolumeLabels, err := common.ConvertLabelsStringToMap(*extraVolumeLabelsStr)
	if err != nil {
		klog.Fatalf("Bad extra volume labels: %v", err.Error())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize driver
	gceDriver := driver.GetGCEDriver()

	// Initialize identity server
	identityServer := driver.NewIdentityServer(gceDriver)

	// Initialize requisite zones
	fallbackRequisiteZones := strings.Split(*fallbackRequisiteZonesFlag, ",")

	// Initialize multi-zone disk types
	multiZoneVolumeHandleDiskTypes := strings.Split(*multiZoneVolumeHandleDiskTypesFlag, ",")
	multiZoneVolumeHandleConfig := driver.MultiZoneVolumeHandleConfig{
		Enable:    *multiZoneVolumeHandleEnableFlag,
		DiskTypes: multiZoneVolumeHandleDiskTypes,
	}

	// Initialize requirements for the controller service
	var controllerServer *driver.GCEControllerServer
	if *runControllerService {
		cloudProvider, err := gce.CreateCloudProvider(ctx, version, *cloudConfigFilePath, computeEndpoint, computeEnvironment)
		if err != nil {
			klog.Fatalf("Failed to get cloud provider: %v", err.Error())
		}
		initialBackoffDuration := time.Duration(*errorBackoffInitialDurationMs) * time.Millisecond
		maxBackoffDuration := time.Duration(*errorBackoffMaxDurationMs) * time.Millisecond
		controllerServer = driver.NewControllerServer(gceDriver, cloudProvider, initialBackoffDuration, maxBackoffDuration, fallbackRequisiteZones, *enableStoragePoolsFlag, multiZoneVolumeHandleConfig)
	} else if *cloudConfigFilePath != "" {
		klog.Warningf("controller service is disabled but cloud config given - it has no effect")
	}

	// Initialize requirements for the node service
	var nodeServer *driver.GCENodeServer
	if *runNodeService {
		mounter, err := mountmanager.NewSafeMounter(*maxConcurrentFormat, *concurrentFormatTimeout)
		if err != nil {
			klog.Fatalf("Failed to get safe mounter: %v", err.Error())
		}
		deviceUtils := deviceutils.NewDeviceUtils()
		statter := mountmanager.NewStatter(mounter)
		meta, err := metadataservice.NewMetadataService()
		if err != nil {
			klog.Fatalf("Failed to set up metadata service: %v", err.Error())
		}
		nodeServer = driver.NewNodeServer(gceDriver, mounter, deviceUtils, meta, statter)
		if *maxConcurrentFormatAndMount > 0 {
			nodeServer = nodeServer.WithSerializedFormatAndMount(*formatAndMountTimeout, *maxConcurrentFormatAndMount)
		}
	}

	err = gceDriver.SetupGCEDriver(driverName, version, extraVolumeLabels, identityServer, controllerServer, nodeServer)
	if err != nil {
		klog.Fatalf("Failed to initialize GCE CSI Driver: %v", err.Error())
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

	gceDriver.Run(*endpoint, *grpcLogCharCap, *enableOtelTracing)
}

func enumFlag(target *gce.Environment, name string, allowedComputeEnvironment []gce.Environment, usage string) {
	flag.Func(name, usage, func(flagValue string) error {
		for _, allowedValue := range allowedComputeEnvironment {
			if gce.Environment(flagValue) == allowedValue {
				*target = gce.Environment(flagValue)
				return nil
			}
		}
		errMsg := fmt.Sprintf(`must be one of %v`, allowedComputeEnvironment)
		return errors.New(errMsg)
	})

}

func urlFlag(target **url.URL, name string, usage string) {
	flag.Func(name, usage, func(flagValue string) error {
		computeURL, err := url.ParseRequestURI(flagValue)
		if err == nil {
			*target = computeURL
			return nil
		}
		klog.Infof("Error parsing endpoint compute endpoint %v", err)
		return err
	})
}
