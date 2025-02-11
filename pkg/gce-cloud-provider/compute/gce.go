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

package gcecloudprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"time"

	"golang.org/x/oauth2/google"
	"golang.org/x/time/rate"
	"google.golang.org/api/option"
	"gopkg.in/gcfg.v1"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"

	"cloud.google.com/go/compute/metadata"
	rscmgr "cloud.google.com/go/resourcemanager/apiv3"
	"golang.org/x/oauth2"
	computebeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"
)

type Environment string

const (
	TokenURL                        = "https://accounts.google.com/o/oauth2/token"
	diskSourceURITemplateSingleZone = "projects/%s/zones/%s/disks/%s"       // {gce.projectID}/zones/{disk.Zone}/disks/{disk.Name}"
	diskSourceURITemplateRegional   = "projects/%s/regions/%s/disks/%s"     //{gce.projectID}/regions/{disk.Region}/disks/repd"
	diskTypeURITemplateSingleZone   = "projects/%s/zones/%s/diskTypes/%s"   // {gce.projectID}/zones/{disk.Zone}/diskTypes/{disk.Type}"
	diskTypeURITemplateRegional     = "projects/%s/regions/%s/diskTypes/%s" // {gce.projectID}/regions/{disk.Region}/diskTypes/{disk.Type}"

	regionURITemplate = "projects/%s/regions/%s"

	replicaZoneURITemplateSingleZone             = "projects/%s/zones/%s" // {gce.projectID}/zones/{disk.Zone}
	EnvironmentStaging               Environment = "staging"
	EnvironmentProduction            Environment = "production"

	// resourceManagerHostSubPath is the endpoint for tag requests.
	resourceManagerHostSubPath = "cloudresourcemanager.googleapis.com"

	// zonalOrRegionalComputeParentPathFmt is the string format for the full path of compute resource.
	// belonging to a zone or a region
	zonalOrRegionalComputeParentPathFmt = "//compute.googleapis.com/projects/%s/%s/%s/%s/%d"

	// globalComputeParentPathFmt is the string format for the full path of global compute resource.
	globalComputeParentPathFmt = "//compute.googleapis.com/projects/%s/global/%s/%d"

	// gcpTagsRequestRateLimit is the tag request rate limit per second.
	gcpTagsRequestRateLimit = 8

	// gcpTagsRequestTokenBucketSize is the burst/token bucket size used
	// for limiting API requests.
	gcpTagsRequestTokenBucketSize = 8
)

// ResourceType indicates the type of a compute resource.
type ResourceType string

var (
	// snapshotsType is the resource type of compute snapshots.
	snapshotsType ResourceType = "snapshots"
	// imagesType is the resource type of compute images.
	imagesType ResourceType = "images"
)

// CloudProvider only supports GCE v1/beta Disk APIs. See
// https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1524
// for how to add GCE alpha Disk support.
type CloudProvider struct {
	service     *compute.Service
	betaService *computebeta.Service
	tokenSource oauth2.TokenSource
	project     string
	zone        string

	zonesCache map[string][]string

	waitForAttachConfig WaitForAttachConfig

	tagsRateLimiter *rate.Limiter

	listInstancesConfig ListInstancesConfig

	enableHdHA bool
}

var _ GCECompute = &CloudProvider{}

type ConfigFile struct {
	Global ConfigGlobal `gcfg:"global"`
}

type ListInstancesConfig struct {
	Filters []string
}

type WaitForAttachConfig struct {
	// A set of disk types that should use the compute instances.get API instead of the
	// disks.get API. For certain disk types, using the instances.get API is preferred
	// based on the response characteristics of the API.
	UseInstancesAPIForDiskTypes []string
}

func (cfg WaitForAttachConfig) ShouldUseGetInstanceAPI(diskType string) bool {
	return slices.Contains(cfg.UseInstancesAPIForDiskTypes, diskType)
}

type ConfigGlobal struct {
	TokenURL  string `gcfg:"token-url"`
	TokenBody string `gcfg:"token-body"`
	ProjectId string `gcfg:"project-id"`
	Zone      string `gcfg:"zone"`
}

func CreateCloudProvider(ctx context.Context, vendorVersion string, configPath string, computeEndpoint *url.URL, computeEnvironment Environment, waitForAttachConfig WaitForAttachConfig, listInstancesConfig ListInstancesConfig) (*CloudProvider, error) {
	configFile, err := readConfig(configPath)
	if err != nil {
		return nil, err
	}
	// At this point configFile could still be nil.
	// Any following code that uses configFile should handle nil pointer gracefully.

	klog.V(2).Infof("Using GCE provider config %+v", configFile)

	tokenSource, err := generateTokenSource(ctx, configFile)
	if err != nil {
		return nil, err
	}

	svc, err := createCloudService(ctx, vendorVersion, tokenSource, computeEndpoint, computeEnvironment)
	if err != nil {
		return nil, err
	}
	klog.Infof("Compute endpoint for V1 version: %s", svc.BasePath)

	betasvc, err := createBetaCloudService(ctx, vendorVersion, tokenSource, computeEndpoint, computeEnvironment)
	if err != nil {
		return nil, err
	}
	klog.Infof("Compute endpoint for Beta version: %s", betasvc.BasePath)

	project, zone, err := getProjectAndZone(configFile)
	if err != nil {
		return nil, fmt.Errorf("Failed getting Project and Zone: %w", err)
	}

	return &CloudProvider{
		service:             svc,
		betaService:         betasvc,
		tokenSource:         tokenSource,
		project:             project,
		zone:                zone,
		zonesCache:          make(map[string]([]string)),
		waitForAttachConfig: waitForAttachConfig,
		listInstancesConfig: listInstancesConfig,
		// GCP has a rate limit of 600 requests per minute, restricting
		// here to 8 requests per second.
		tagsRateLimiter: common.NewLimiter(gcpTagsRequestRateLimit, gcpTagsRequestTokenBucketSize, true),
	}, nil

}

func generateTokenSource(ctx context.Context, configFile *ConfigFile) (oauth2.TokenSource, error) {
	if configFile != nil && configFile.Global.TokenURL != "" && configFile.Global.TokenURL != "nil" {
		// configFile.Global.TokenURL is defined
		// Use AltTokenSource

		tokenSource := NewAltTokenSource(configFile.Global.TokenURL, configFile.Global.TokenBody)
		klog.V(2).Infof("Using AltTokenSource %#v", tokenSource)
		return tokenSource, nil
	}

	// Use DefaultTokenSource

	tokenSource, err := google.DefaultTokenSource(
		ctx,
		compute.CloudPlatformScope,
		compute.ComputeScope)

	// DefaultTokenSource relies on GOOGLE_APPLICATION_CREDENTIALS env var being set.
	if gac, ok := os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS"); ok {
		klog.V(2).Infof("GOOGLE_APPLICATION_CREDENTIALS env var set %v", gac)
	} else {
		klog.Warningf("GOOGLE_APPLICATION_CREDENTIALS env var not set")
	}
	klog.V(2).Infof("Using DefaultTokenSource %#v", tokenSource)

	return tokenSource, err
}

func readConfig(configPath string) (*ConfigFile, error) {
	if configPath == "" {
		return nil, nil
	}

	reader, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("couldn't open cloud provider configuration at %s: %w", configPath, err)
	}
	defer reader.Close()

	cfg := &ConfigFile{}
	if err := gcfg.FatalOnly(gcfg.ReadInto(cfg, reader)); err != nil {
		return nil, fmt.Errorf("couldn't read cloud provider configuration at %s: %w", configPath, err)
	}
	return cfg, nil
}

func createBetaCloudService(ctx context.Context, vendorVersion string, tokenSource oauth2.TokenSource, computeEndpoint *url.URL, computeEnvironment Environment) (*computebeta.Service, error) {
	computeOpts, err := getComputeVersion(ctx, tokenSource, computeEndpoint, computeEnvironment, GCEAPIVersionBeta)
	if err != nil {
		klog.Errorf("Failed to get compute endpoint: %s", err)
	}
	service, err := computebeta.NewService(ctx, computeOpts...)
	if err != nil {
		return nil, err
	}
	service.UserAgent = fmt.Sprintf("GCE CSI Driver/%s (%s %s)", vendorVersion, runtime.GOOS, runtime.GOARCH)
	return service, nil
}

func createCloudService(ctx context.Context, vendorVersion string, tokenSource oauth2.TokenSource, computeEndpoint *url.URL, computeEnvironment Environment) (*compute.Service, error) {
	computeOpts, err := getComputeVersion(ctx, tokenSource, computeEndpoint, computeEnvironment, GCEAPIVersionV1)
	if err != nil {
		klog.Errorf("Failed to get compute endpoint: %s", err)
	}
	service, err := compute.NewService(ctx, computeOpts...)
	if err != nil {
		return nil, err
	}
	service.UserAgent = fmt.Sprintf("GCE CSI Driver/%s (%s %s)", vendorVersion, runtime.GOOS, runtime.GOARCH)
	return service, nil
}

func getComputeVersion(ctx context.Context, tokenSource oauth2.TokenSource, computeEndpoint *url.URL, computeEnvironment Environment, computeVersion GCEAPIVersion) ([]option.ClientOption, error) {
	client, err := newOauthClient(ctx, tokenSource)
	if err != nil {
		return nil, err
	}
	computeOpts := []option.ClientOption{option.WithHTTPClient(client)}

	if computeEndpoint != nil {
		computeEnvironmentSuffix := constructComputeEndpointPath(computeEnvironment, computeVersion)
		computeEndpoint.Path = computeEnvironmentSuffix
		endpoint := computeEndpoint.String()
		computeOpts = append(computeOpts, option.WithEndpoint(endpoint))
	}
	return computeOpts, nil
}

func constructComputeEndpointPath(env Environment, version GCEAPIVersion) string {
	prefix := ""
	if env == EnvironmentStaging {
		prefix = fmt.Sprintf("%s_", env)
	}
	return fmt.Sprintf("compute/%s%s/", prefix, version)
}

func createTagValuesClient(ctx context.Context, tokenSource oauth2.TokenSource, resourceManagerHostSubPath string) (*rscmgr.TagValuesClient, error) {
	client, err := newOauthClient(ctx, tokenSource)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("https://%s", resourceManagerHostSubPath)
	opts := []option.ClientOption{
		option.WithHTTPClient(client),
		option.WithEndpoint(endpoint),
	}
	return rscmgr.NewTagValuesRESTClient(ctx, opts...)
}

func createTagBindingsClient(ctx context.Context, tokenSource oauth2.TokenSource, location string, resourceManagerHostSubPath string) (*rscmgr.TagBindingsClient, error) {
	client, err := newOauthClient(ctx, tokenSource)
	if err != nil {
		return nil, err
	}

	var endpoint string
	if location != "" {
		endpoint = fmt.Sprintf("https://%s-%s", location, resourceManagerHostSubPath)
	} else {
		endpoint = fmt.Sprintf("https://%s", resourceManagerHostSubPath)
	}
	opts := []option.ClientOption{
		option.WithHTTPClient(client),
		option.WithEndpoint(endpoint),
	}
	return rscmgr.NewTagBindingsRESTClient(ctx, opts...)
}

func newOauthClient(ctx context.Context, tokenSource oauth2.TokenSource) (*http.Client, error) {
	if err := wait.PollImmediate(5*time.Second, 30*time.Second, func() (bool, error) {
		if _, err := tokenSource.Token(); err != nil {
			klog.Errorf("error fetching initial token: %v", err.Error())
			return false, nil
		}
		return true, nil
	}); err != nil {
		return nil, err
	}

	return oauth2.NewClient(ctx, tokenSource), nil
}

func getProjectAndZone(config *ConfigFile) (string, string, error) {
	var err error

	var zone string
	if config == nil || config.Global.Zone == "" {
		zone, err = metadata.Zone()
		if err != nil {
			return "", "", err
		}
		klog.V(2).Infof("Using GCP zone from the Metadata server: %q", zone)
	} else {
		zone = config.Global.Zone
		klog.V(2).Infof("Using GCP zone from the local GCE cloud provider config file: %q", zone)
	}

	var projectID string
	if config == nil || config.Global.ProjectId == "" {
		// Project ID is not available from the local GCE cloud provider config file.
		// This could happen if the driver is not running in the master VM.
		// Defaulting to project ID from the Metadata server.
		projectID, err = metadata.ProjectID()
		if err != nil {
			return "", "", err
		}
		klog.V(2).Infof("Using GCP project ID from the Metadata server: %q", projectID)
	} else {
		projectID = config.Global.ProjectId
		klog.V(2).Infof("Using GCP project ID from the local GCE cloud provider config file: %q", projectID)
	}

	return projectID, zone, nil
}

// isGCEError returns true if given error is a googleapi.Error with given
// reason (e.g. "resourceInUseByAnotherResource")
func IsGCEError(err error, reason string) bool {
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}

	for _, e := range apiErr.Errors {
		if e.Reason == reason {
			return true
		}
	}
	return false
}

// IsGCENotFoundError returns true if the error is a googleapi.Error with
// notFound reason
func IsGCENotFoundError(err error) bool {
	return IsGCEError(err, "notFound")
}

// IsInvalidError returns true if the error is a googleapi.Error with
// invalid reason
func IsGCEInvalidError(err error) bool {
	return IsGCEError(err, "invalid")
}
