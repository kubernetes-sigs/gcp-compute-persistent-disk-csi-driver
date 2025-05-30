/*
Copyright 2019 The Kubernetes Authors.

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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"k8s.io/client-go/util/flowcontrol"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute/tenancy"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
)

const (
	// Max QPS to allow through to the token URL.
	tokenURLQPS = .05 // back off to once every 20 seconds when failing
	// Maximum burst of requests to token URL before limiting.
	tokenURLBurst = 3
)

// TODO(#276) add metrics around token requests once the driver integrates with Prometheus.

// AltTokenSource is the structure holding the data for the functionality needed to generates tokens
type AltTokenSource struct {
	oauthClient *http.Client
	tokenURL    string
	tokenBody   string
	throttle    flowcontrol.RateLimiter
}

// Token returns a token which may be used for authentication
func (a *AltTokenSource) Token() (*oauth2.Token, error) {
	a.throttle.Accept()
	return a.token()
}

func (a *AltTokenSource) token() (*oauth2.Token, error) {
	req, err := http.NewRequest("POST", a.tokenURL, strings.NewReader(a.tokenBody))
	if err != nil {
		return nil, err
	}
	res, err := a.oauthClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	var tok struct {
		AccessToken string    `json:"accessToken"`
		ExpireTime  time.Time `json:"expireTime"`
	}
	if err := json.NewDecoder(res.Body).Decode(&tok); err != nil {
		return nil, err
	}
	return &oauth2.Token{
		AccessToken: tok.AccessToken,
		Expiry:      tok.ExpireTime,
	}, nil
}

// NewAltTokenSource constructs a new alternate token source for generating tokens.
func NewAltTokenSource(tokenURL, tokenBody string) oauth2.TokenSource {
	client := oauth2.NewClient(oauth2.NoContext, google.ComputeTokenSource(""))
	a := &AltTokenSource{
		oauthClient: client,
		tokenURL:    tokenURL,
		tokenBody:   tokenBody,
		throttle:    flowcontrol.NewTokenBucketRateLimiter(tokenURLQPS, tokenURLBurst),
	}
	return oauth2.ReuseTokenSource(nil, a)
}

func NewTenantTokenSource(tenantMeta *tenancy.Metadata, region, existingTokenURL, existingTokenBody string) (oauth2.TokenSource, error) {
	tenantTokenUrl, err := getTenantTokenURL(tenantMeta, existingTokenURL)
	if err != nil {
		return nil, err
	}
	tenantTokenBody, err := getTenantTokenBody(tenantMeta, existingTokenBody)
	if err != nil {
		return nil, err
	}
	return NewAltTokenSource(tenantTokenUrl, tenantTokenBody), nil
}

func getTenantTokenURL(tenantMeta *tenancy.Metadata, existingTokenURL string) (string, error) {
	location := extractLocationFromTokenURL(existingTokenURL)
	if location == "" {
		return "", fmt.Errorf("could not extract location from existing token URL: %s", existingTokenURL)
	}

	tokenURLParts := strings.SplitN(existingTokenURL, "/projects/", 2)
	if len(tokenURLParts) != 2 {
		return "", fmt.Errorf("invalid existing token URL format: %s, cannot extract base URL", existingTokenURL)
	}
	baseURL := tokenURLParts[0]

	// Format: {BASE_URL}/projects/{TENANT_PROJECT_NUMBER}/locations/{TENANT_LOCATION}/tenants/{TENANT_ID}:generateTenantToken
	formatString := "%s/projects/%s/locations/%s/tenants/%s:generateTenantToken"
	tokenURL := fmt.Sprintf(formatString, baseURL, tenantMeta.ProjectNumber, location, tenantMeta.TenantName)
	return tokenURL, nil
}

// extractLocationFromTokenURL extracts the location from a GKE token URL.
// Example input: https://gkeauth.googleapis.com/v1/projects/654321/locations/us-central1/clusters/example-cluster:generateToken
// Returns: us-central1
func extractLocationFromTokenURL(tokenURL string) string {
	parts := strings.Split(tokenURL, "/")
	for i, part := range parts {
		if part == "locations" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func getTenantTokenBody(tenantMeta *tenancy.Metadata, existingTokenBody string) (string, error) {
	// Check if the token body is a quoted JSON string
	// Quoted example: "{\"projectNumber\":12345,\"clusterId\":\"example-cluster\"}"
	// Non-quoted example: {"projectNumber":12345,"clusterId":"example-cluster"}
	isQuoted := len(existingTokenBody) > 0 && existingTokenBody[0] == '"' && existingTokenBody[len(existingTokenBody)-1] == '"'

	var jsonStr string
	if isQuoted {
		var err error
		jsonStr, err = strconv.Unquote(existingTokenBody)
		if err != nil {
			return "", fmt.Errorf("error unquoting TokenBody: %v", err)
		}
	} else {
		jsonStr = existingTokenBody
	}

	var bodyMap map[string]any

	if err := json.Unmarshal([]byte(jsonStr), &bodyMap); err != nil {
		return "", fmt.Errorf("error unmarshaling TokenBody: %v", err)
	}

	bodyMap["projectNumber"] = tenantMeta.ProjectNumber

	newTokenBodyBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return "", fmt.Errorf("error marshaling TokenBody: %v", err)
	}

	if isQuoted {
		// Re-quote the JSON string if the original was quoted
		return strconv.Quote(string(newTokenBodyBytes)), nil
	}

	return string(newTokenBodyBytes), nil
}
