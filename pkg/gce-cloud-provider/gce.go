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
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/golang/glog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	TokenURL                        = "https://accounts.google.com/o/oauth2/token"
	diskSourceURITemplateSingleZone = "%s/zones/%s/disks/%s"              // {gce.projectID}/zones/{disk.Zone}/disks/{disk.Name}"
	diskTypeURITemplateSingleZone   = "projects/%s/zones/%s/diskTypes/%s" // projects/{gce.projectID}/zones/{disk.Zone}/diskTypes/{disk.Type}"

	gceComputeAPIEndpoint = "https://www.googleapis.com/compute/v1/"
)

type CloudProvider struct {
	service *compute.Service
	project string
	zone    string
}

func CreateCloudProvider() (*CloudProvider, error) {
	svc, err := createCloudService()
	if err != nil {
		return nil, err
	}
	// TODO: Use metadata server or flags to retrieve project and zone. Fallback on flag if necessary

	project, zone, err := getProjectAndZoneFromMetadata()
	if err != nil {
		return nil, fmt.Errorf("Failed getting Project and Zone from Metadata server: %v", err)
	}

	return &CloudProvider{
		service: svc,
		project: project,
		zone:    zone,
	}, nil

}

func createCloudService() (*compute.Service, error) {
	// TODO: support alternate methods of authentication
	svc, err := createCloudServiceWithDefaultServiceAccount()
	return svc, err
}

func createCloudServiceWithDefaultServiceAccount() (*compute.Service, error) {
	client, err := newDefaultOauthClient()
	if err != nil {
		return nil, err
	}
	service, err := compute.New(client)
	if err != nil {
		return nil, err
	}
	service.UserAgent = fmt.Sprintf("GCE CSI Driver/%s (%s %s)", "0.2.0", runtime.GOOS, runtime.GOARCH)
	return service, nil
}

func newDefaultOauthClient() (*http.Client, error) {
	// No compute token source, fallback on default
	tokenSource, err := google.DefaultTokenSource(
		oauth2.NoContext,
		compute.CloudPlatformScope,
		compute.ComputeScope)
	if gac, ok := os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS"); ok {
		glog.Infof("GOOGLE_APPLICATION_CREDENTIALS env var set %v", gac)
	} else {
		glog.Warningf("GOOGLE_APPLICATION_CREDENTIALS env var not set")
	}
	glog.Infof("Using DefaultTokenSource %#v", tokenSource)
	if err != nil {
		return nil, err
	}

	if err := wait.PollImmediate(5*time.Second, 30*time.Second, func() (bool, error) {
		if _, err := tokenSource.Token(); err != nil {
			glog.Errorf("error fetching initial token: %v", err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		return nil, err
	}

	return oauth2.NewClient(oauth2.NoContext, tokenSource), nil
}

func getProjectAndZoneFromMetadata() (string, string, error) {
	result, err := metadata.Get("instance/zone")
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(result, "/")
	if len(parts) != 4 {
		return "", "", fmt.Errorf("unexpected response: %s", result)
	}
	zone := parts[3]
	projectID, err := metadata.ProjectID()
	if err != nil {
		return "", "", err
	}
	return projectID, zone, nil
}

// isGCEError returns true if given error is a googleapi.Error with given
// reason (e.g. "resourceInUseByAnotherResource")
func IsGCEError(err error, reason string) bool {
	apiErr, ok := err.(*googleapi.Error)
	if !ok {
		return false
	}

	for _, e := range apiErr.Errors {
		if e.Reason == reason {
			return true
		}
	}
	return false
}
