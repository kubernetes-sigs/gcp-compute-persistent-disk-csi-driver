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
	"os"
	"runtime"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"gopkg.in/gcfg.v1"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2"
	computealpha "google.golang.org/api/compute/v0.alpha"
	computebeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	TokenURL                        = "https://accounts.google.com/o/oauth2/token"
	diskSourceURITemplateSingleZone = "projects/%s/zones/%s/disks/%s"       // {gce.projectID}/zones/{disk.Zone}/disks/{disk.Name}"
	diskSourceURITemplateRegional   = "projects/%s/regions/%s/disks/%s"     //{gce.projectID}/regions/{disk.Region}/disks/repd"
	diskTypeURITemplateSingleZone   = "projects/%s/zones/%s/diskTypes/%s"   // {gce.projectID}/zones/{disk.Zone}/diskTypes/{disk.Type}"
	diskTypeURITemplateRegional     = "projects/%s/regions/%s/diskTypes/%s" // {gce.projectID}/regions/{disk.Region}/diskTypes/{disk.Type}"

	regionURITemplate = "projects/%s/regions/%s"

	replicaZoneURITemplateSingleZone = "projects/%s/zones/%s" // {gce.projectID}/zones/{disk.Zone}
)

type CloudProvider struct {
	service      *compute.Service
	betaService  *computebeta.Service
	alphaService *computealpha.Service
	project      string
	zone         string

	zonesCache map[string][]string
}

var _ GCECompute = &CloudProvider{}

type ConfigFile struct {
	Global ConfigGlobal `gcfg:"global"`
}

type ConfigGlobal struct {
	TokenURL  string `gcfg:"token-url"`
	TokenBody string `gcfg:"token-body"`
	ProjectId string `gcfg:"project-id"`
	Zone      string `gcfg:"zone"`
}

func CreateCloudProvider(ctx context.Context, vendorVersion string, configPath string, computeEndpoint string) (*CloudProvider, error) {
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

	svc, err := createCloudService(ctx, vendorVersion, tokenSource, computeEndpoint)
	if err != nil {
		return nil, err
	}

	betasvc, err := createBetaCloudService(ctx, vendorVersion, tokenSource, computeEndpoint)
	if err != nil {
		return nil, err
	}

	alphasvc, err := createAlphaCloudService(ctx, vendorVersion, tokenSource, computeEndpoint)
	if err != nil {
		return nil, err
	}

	project, zone, err := getProjectAndZone(configFile)
	if err != nil {
		return nil, fmt.Errorf("Failed getting Project and Zone: %w", err)
	}

	return &CloudProvider{
		service:      svc,
		betaService:  betasvc,
		alphaService: alphasvc,
		project:      project,
		zone:         zone,
		zonesCache:   make(map[string]([]string)),
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

func createBetaCloudService(ctx context.Context, vendorVersion string, tokenSource oauth2.TokenSource, computeEndpoint string) (*computebeta.Service, error) {
	client, err := newOauthClient(ctx, tokenSource)
	if err != nil {
		return nil, err
	}

	computeOpts := []option.ClientOption{option.WithHTTPClient(client)}
	if computeEndpoint != "" {
		betaEndpoint := fmt.Sprintf("%s/compute/beta/", computeEndpoint)
		computeOpts = append(computeOpts, option.WithEndpoint(betaEndpoint))
	}
	service, err := computebeta.NewService(ctx, computeOpts...)
	if err != nil {
		return nil, err
	}
	service.UserAgent = fmt.Sprintf("GCE CSI Driver/%s (%s %s)", vendorVersion, runtime.GOOS, runtime.GOARCH)
	return service, nil
}

func createAlphaCloudService(ctx context.Context, vendorVersion string, tokenSource oauth2.TokenSource, computeEndpoint string) (*computealpha.Service, error) {
	client, err := newOauthClient(ctx, tokenSource)
	if err != nil {
		return nil, err
	}

	computeOpts := []option.ClientOption{option.WithHTTPClient(client)}
	if computeEndpoint != "" {
		alphaEndpoint := fmt.Sprintf("%s/compute/alpha/", computeEndpoint)
		computeOpts = append(computeOpts, option.WithEndpoint(alphaEndpoint))
	}
	service, err := computealpha.NewService(ctx, computeOpts...)
	if err != nil {
		return nil, err
	}
	service.UserAgent = fmt.Sprintf("GCE CSI Driver/%s (%s %s)", vendorVersion, runtime.GOOS, runtime.GOARCH)
	return service, nil
}

func createCloudService(ctx context.Context, vendorVersion string, tokenSource oauth2.TokenSource, computeEndpoint string) (*compute.Service, error) {
	svc, err := createCloudServiceWithDefaultServiceAccount(ctx, vendorVersion, tokenSource, computeEndpoint)
	return svc, err
}

func createCloudServiceWithDefaultServiceAccount(ctx context.Context, vendorVersion string, tokenSource oauth2.TokenSource, computeEndpoint string) (*compute.Service, error) {
	client, err := newOauthClient(ctx, tokenSource)
	if err != nil {
		return nil, err
	}

	computeOpts := []option.ClientOption{option.WithHTTPClient(client)}
	if computeEndpoint != "" {
		v1Endpoint := fmt.Sprintf("%s/compute/v1/", computeEndpoint)
		computeOpts = append(computeOpts, option.WithEndpoint(v1Endpoint))
	}
	service, err := compute.NewService(ctx, computeOpts...)
	if err != nil {
		return nil, err
	}
	service.UserAgent = fmt.Sprintf("GCE CSI Driver/%s (%s %s)", vendorVersion, runtime.GOOS, runtime.GOARCH)
	return service, nil
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
