/*
Copyright 2020 The Kubernetes Authors.

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

package metrics

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/component-base/metrics"
	"k8s.io/klog/v2"
)

const (
	// envGKEPDCSIVersion is an environment variable set in the PDCSI controller manifest
	// with the current version of the GKE component.
	envGKEPDCSIVersion  = "GKE_PDCSI_VERSION"
	hyperdiskDriverName = "hyperdisk.csi.storage.gke.io"
	pdcsiDriverName     = "pd.csi.storage.gke.io"
)

var (
	// This metric is exposed only from the controller driver component when GKE_PDCSI_VERSION env variable is set.
	gkeComponentVersion = metrics.NewGaugeVec(&metrics.GaugeOpts{
		Name: "component_version",
		Help: "Metric to expose the version of the PDCSI GKE component.",
	}, []string{"component_version"})

	pdcsiOperationErrorsMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Subsystem:      "csidriver",
			Name:           "pdcsi_operation_errors",
			Help:           "CSI server side error metrics",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{"driver_name", "method_name", "grpc_status_code", "disk_type"},
	)
)

type MetricsManager struct {
	registry metrics.KubeRegistry
}

func NewMetricsManager() MetricsManager {
	mm := MetricsManager{
		registry: metrics.NewKubeRegistry(),
	}
	return mm
}

func (mm *MetricsManager) GetRegistry() metrics.KubeRegistry {
	return mm.registry
}

func (mm *MetricsManager) registerComponentVersionMetric() {
	mm.registry.MustRegister(gkeComponentVersion)
}

func (mm *MetricsManager) RegisterHyperdiskMetric() {
	mm.registry.MustRegister(pdcsiOperationErrorsMetric)
}

func (mm *MetricsManager) recordComponentVersionMetric() error {
	v := getEnvVar(envGKEPDCSIVersion)
	if v == "" {
		klog.V(2).Info("Skip emitting component version metric")
		return fmt.Errorf("Failed to register GKE component version metric, env variable %v not defined", envGKEPDCSIVersion)
	}

	gkeComponentVersion.WithLabelValues(v).Set(1.0)
	klog.Infof("Recorded GKE component version : %v", v)
	return nil
}

func (mm *MetricsManager) RecordOperationErrorMetrics(
	operationName string,
	operationErr error,
	diskType string) {
	var driverName string
	if strings.Contains(diskType, "hyperdisk") {
		driverName = hyperdiskDriverName
	}
	if strings.Contains(diskType, "pd") {
		driverName = pdcsiDriverName
	}
	pdcsiOperationErrorsMetric.WithLabelValues(driverName, "/csi.v1.Controller/"+operationName, LoggedError(operationErr).Error(), diskType).Set(1.0)
}

func (mm *MetricsManager) EmitGKEComponentVersion() error {
	mm.registerComponentVersionMetric()
	if err := mm.recordComponentVersionMetric(); err != nil {
		return err
	}

	return nil
}

// CodeForError returns a pointer to the grpc error code that maps to the http
// error code for the passed in user googleapi error. Returns codes.Internal if
// the given error is not a googleapi error caused by the user. The following
// http error codes are considered user errors:
// (1) http 400 Bad Request, returns grpc InvalidArgument,
// (2) http 403 Forbidden, returns grpc PermissionDenied,
// (3) http 404 Not Found, returns grpc NotFound
// (4) http 429 Too Many Requests, returns grpc ResourceExhausted
func CodeForError(err error) *codes.Code {
	internalErrorCode := codes.Internal
	// Upwrap the error
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return &internalErrorCode
	}

	userErrors := map[int]codes.Code{
		http.StatusForbidden:       codes.PermissionDenied,
		http.StatusBadRequest:      codes.InvalidArgument,
		http.StatusTooManyRequests: codes.ResourceExhausted,
		http.StatusNotFound:        codes.NotFound,
	}
	if code, ok := userErrors[apiErr.Code]; ok {
		return &code
	}
	return &internalErrorCode
}

func LoggedError(err error) error {
	return status.Errorf(*CodeForError(err), "error code"+err.Error())
}

// Server represents any type that could serve HTTP requests for the metrics
// endpoint.
type Server interface {
	Handle(pattern string, handler http.Handler)
}

// RegisterToServer registers an HTTP handler for this metrics manager to the
// given server at the specified address/path.
func (mm *MetricsManager) registerToServer(s Server, metricsPath string) {
	s.Handle(metricsPath, metrics.HandlerFor(
		mm.GetRegistry(),
		metrics.HandlerOpts{
			ErrorHandling: metrics.ContinueOnError}))
}

// InitializeHttpHandler sets up a server and creates a handler for metrics.
func (mm *MetricsManager) InitializeHttpHandler(address, path string) {
	mux := http.NewServeMux()
	mm.registerToServer(mux, path)
	go func() {
		klog.Infof("Metric server listening at %q", address)
		if err := http.ListenAndServe(address, mux); err != nil {
			klog.Fatalf("Failed to start metric server at specified address (%q) and path (%q): %v", address, path, err.Error())
		}
	}()
}

func getEnvVar(envVarName string) string {
	v, ok := os.LookupEnv(envVarName)
	if !ok {
		klog.Warningf("%q env not set", envVarName)
		return ""
	}

	return v
}

func IsGKEComponentVersionAvailable() bool {
	if getEnvVar(envGKEPDCSIVersion) == "" {
		return false
	}

	return true
}
