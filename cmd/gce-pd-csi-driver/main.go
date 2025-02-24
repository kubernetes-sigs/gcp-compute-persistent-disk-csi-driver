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
	"k8s.io/utils/strings/slices"
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
	enableDataCacheFlag         = flag.Bool("enable-data-cache", false, "If set to true, the CSI Driver will allow volumes to be provisioned with data cache configuration")
	nodeName                    = flag.String("node-name", "", "The node this driver is running on")

	multiZoneVolumeHandleDiskTypesFlag = flag.String("multi-zone-volume-handle-disk-types", "", "Comma separated list of allowed disk types that can use the multi-zone volumeHandle. Used only if --multi-zone-volume-handle-enable")
	multiZoneVolumeHandleEnableFlag    = flag.Bool("multi-zone-volume-handle-enable", false, "If set to true, the multi-zone volumeHandle feature will be enabled")

	computeEnvironment        gce.Environment = gce.EnvironmentProduction
	computeEndpoint           *url.URL
	allowedComputeEnvironment = []gce.Environment{gce.EnvironmentStaging, gce.EnvironmentProduction}

	useInstanceAPIOnWaitForAttachDiskTypesFlag     = flag.String("use-instance-api-to-poll-attachment-disk-types", "", "Comma separated list of disk types that should use instances.get API when polling for disk attach during ControllerPublish")
	useInstanceAPIForListVolumesPublishedNodesFlag = flag.Bool("use-instance-api-to-list-volumes-published-nodes", false, "Enables using the instances.list API to determine published_node_ids in ListVolumes. When false (default), the disks.list API is used")
	instancesListFiltersFlag                       = flag.String("instances-list-filters", "", "Comma separated list of filters to use when calling the instances.list API. By default instances.list fetches all instances in a region")

	extraTagsStr = flag.String("extra-tags", "", "Extra tags to attach to each Compute Disk, Image, Snapshot created. It is a comma separated list of parent id, key and value like '<parent_id1>/<tag_key1>/<tag_value1>,...,<parent_idN>/<tag_keyN>/<tag_valueN>'. parent_id is the Organization or the Project ID or Project name where the tag key and the tag value resources exist. A maximum of 50 tags bindings is allowed for a resource. See https://cloud.google.com/resource-manager/docs/tags/tags-overview, https://cloud.google.com/resource-manager/docs/tags/tags-creating-and-managing for details")

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
	stringEnumFlag(&computeEnvironment, "compute-environment", allowedComputeEnvironment, "Operating compute environment")
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

	if len(*extraTagsStr) > 0 && !*runControllerService {
		klog.Fatalf("Extra tags provided but not running controller")
	}
	extraTags, err := common.ConvertTagsStringToMap(*extraTagsStr)
	if err != nil {
		klog.Fatalf("Bad extra tags: %v", err.Error())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize driver
	gceDriver := driver.GetGCEDriver()

	// Initialize identity server
	identityServer := driver.NewIdentityServer(gceDriver)

	// Initialize requisite zones
	fallbackRequisiteZones := parseCSVFlag(*fallbackRequisiteZonesFlag)

	// Initialize multi-zone disk types
	multiZoneVolumeHandleDiskTypes := parseCSVFlag(*multiZoneVolumeHandleDiskTypesFlag)
	multiZoneVolumeHandleConfig := driver.MultiZoneVolumeHandleConfig{
		Enable:    *multiZoneVolumeHandleEnableFlag,
		DiskTypes: multiZoneVolumeHandleDiskTypes,
	}

	// Initialize waitForAttach config
	useInstanceAPIOnWaitForAttachDiskTypes := parseCSVFlag(*useInstanceAPIOnWaitForAttachDiskTypesFlag)
	waitForAttachConfig := gce.WaitForAttachConfig{
		UseInstancesAPIForDiskTypes: useInstanceAPIOnWaitForAttachDiskTypes,
	}

	// Initialize listVolumes config
	instancesListFilters := parseCSVFlag(*instancesListFiltersFlag)
	listInstancesConfig := gce.ListInstancesConfig{
		Filters: instancesListFilters,
	}
	listVolumesConfig := driver.ListVolumesConfig{
		UseInstancesAPIForPublishedNodes: *useInstanceAPIForListVolumesPublishedNodesFlag,
	}

	// Initialize requirements for the controller service
	var controllerServer *driver.GCEControllerServer
	if *runControllerService {
		cloudProvider, err := gce.CreateCloudProvider(ctx, version, *cloudConfigFilePath, computeEndpoint, computeEnvironment, waitForAttachConfig, listInstancesConfig)
		if err != nil {
			klog.Fatalf("Failed to get cloud provider: %v", err.Error())
		}
		initialBackoffDuration := time.Duration(*errorBackoffInitialDurationMs) * time.Millisecond
		maxBackoffDuration := time.Duration(*errorBackoffMaxDurationMs) * time.Millisecond
		controllerServer = driver.NewControllerServer(gceDriver, cloudProvider, initialBackoffDuration, maxBackoffDuration, fallbackRequisiteZones, *enableStoragePoolsFlag, *enableDataCacheFlag, multiZoneVolumeHandleConfig, listVolumesConfig)
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
		nsArgs := driver.NodeServerArgs{
			EnableDataCache: *enableDataCacheFlag,
		}
		nodeServer = driver.NewNodeServer(gceDriver, mounter, deviceUtils, meta, statter, nsArgs)
		if *maxConcurrentFormatAndMount > 0 {
			nodeServer = nodeServer.WithSerializedFormatAndMount(*formatAndMountTimeout, *maxConcurrentFormatAndMount)
		}
		if *enableDataCacheFlag {
			if nodeName == nil || *nodeName == "" {
				klog.Errorf("Data Cache enabled, but --node-name not passed")
			}
			if err := setupDataCache(ctx, *nodeName, nodeServer.MetadataService.GetName()); err != nil {
				klog.Errorf("DataCache setup failed: %v", err)
			}
			go driver.StartWatcher(*nodeName)
		}
	}

	err = gceDriver.SetupGCEDriver(driverName, version, extraVolumeLabels, extraTags, identityServer, controllerServer, nodeServer)
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

func notEmpty(v string) bool {
	return v != ""
}

func parseCSVFlag(list string) []string {
	return slices.Filter(nil, strings.Split(list, ","), notEmpty)
}

type enumConverter[T any] interface {
	convert(v string) (T, error)
	eq(a, b T) bool
}

type stringConverter[T ~string] struct{}

func (s stringConverter[T]) convert(v string) (T, error) {
	return T(v), nil
}

func (s stringConverter[T]) eq(a, b T) bool {
	return a == b
}

func stringEnumFlag[T ~string](target *T, name string, allowed []T, usage string) {
	enumFlag(target, name, stringConverter[T]{}, allowed, usage)
}

func enumFlag[T any](target *T, name string, converter enumConverter[T], allowed []T, usage string) {
	flag.Func(name, usage, func(flagValue string) error {
		tValue, err := converter.convert(flagValue)
		if err != nil {
			return err
		}
		for _, allowedValue := range allowed {
			if converter.eq(allowedValue, tValue) {
				*target = tValue
				return nil
			}
		}
		errMsg := fmt.Sprintf(`must be one of %v`, allowedComputeEnvironment)
		return errors.New(errMsg)
	})
}

func urlFlag(target **url.URL, name string, usage string) {
	flag.Func(name, usage, func(flagValue string) error {
		if flagValue == "" {
			return nil
		}
		computeURL, err := url.ParseRequestURI(flagValue)
		if err == nil {
			*target = computeURL
			return nil
		}
		klog.Errorf("Error parsing endpoint compute endpoint %v", err)
		return err
	})
}

func fetchLssdsForRaiding(lssdCount int) ([]string, error) {
	allLssds, err := driver.FetchAllLssds()
	if err != nil {
		return nil, fmt.Errorf("Error listing all LSSDs %v", err)
	}

	raidedLssds, err := driver.FetchRaidedLssds()
	if err != nil {
		return nil, fmt.Errorf("Error listing RAIDed LSSDs %v", err)
	}

	unRaidedLssds := []string{}
	for _, l := range allLssds {
		if !slices.Contains(raidedLssds, l) {
			unRaidedLssds = append(unRaidedLssds, l)
		}
		if len(unRaidedLssds) == lssdCount {
			break
		}
	}

	LSSDsWithEmptyMountPoint, err := driver.FetchLSSDsWihtEmptyMountPoint()
	if err != nil {
		return nil, fmt.Errorf("Error listing LSSDs with empty mountpoint: %v", err)
	}

	// We need to ensure the disks to be used for Datacache are both unRAIDed & not containing mountpoints for ephemeral storage already
	availableLssds := slices.Filter(nil, unRaidedLssds, func(e string) bool {
		return slices.Contains(LSSDsWithEmptyMountPoint, e)
	})

	if len(availableLssds) == 0 {
		return nil, fmt.Errorf("No LSSDs available to set up caching")
	}

	if len(availableLssds) < lssdCount {
		return nil, fmt.Errorf("Not enough LSSDs available to set up caching. Available LSSDs: %v, wanted LSSDs: %v", len(availableLssds), lssdCount)
	}
	return availableLssds, nil
}

func setupDataCache(ctx context.Context, nodeName string) error {
	isAlreadyRaided, err := driver.IsRaided()
	if err != nil {
		klog.V(2).Infof("Errored while scanning for available LocalSSDs err:%v; continuing Raiding", err)
	} else if isAlreadyRaided {
		klog.V(2).Infof("Local SSDs are already RAIDed. Skipping Datacache setup.")
		return nil
	}

	lssdCount := common.LocalSSDCountForDataCache
	if nodeName != common.TestNode {
		var err error
		lssdCount, err = driver.GetDataCacheCountFromNodeLabel(ctx, nodeName)
		if lssdCount == 0 {
			klog.Infof("Datacache is not enabled on node %v", nodeName)
			return nil
		}
		if err != nil {
			return err
		}
	}
	lssdNames, err := fetchLssdsForRaiding(lssdCount)
	if err != nil {
		klog.Fatalf("Failed to get sufficient SSDs for Datacache's caching setup: %v", err)
	}
	klog.V(2).Infof("Raiding local ssds to setup data cache: %v", lssdNames)
	if err := driver.RaidLocalSsds(lssdNames); err != nil {
		return fmt.Errorf("Failed to Raid local SSDs, unable to setup data caching, got error %v", err)
	}

	// Initializing data cache node (VG checks w/ raided lssd)
	if err := driver.InitializeDataCacheNode(nodeId); err != nil {
		return err
	}

	klog.V(4).Infof("LSSD caching is setup for the Data Cache enabled node %s", nodeName)
	return nil
}
