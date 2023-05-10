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
	"fmt"
	"net/http"
	"os"

	"k8s.io/component-base/metrics"
	"k8s.io/klog/v2"
)

const (
	// envGKEPDCSIVersion is an environment variable set in the PDCSI controller manifest
	// with the current version of the GKE component.
	envGKEPDCSIVersion = "GKE_PDCSI_VERSION"
)

var (
	// This metric is exposed only from the controller driver component when GKE_PDCSI_VERSION env variable is set.
	gkeComponentVersion = metrics.NewGaugeVec(&metrics.GaugeOpts{
		Name: "component_version",
		Help: "Metric to expose the version of the PDCSI GKE component.",
	}, []string{"component_version"})
)

type metricsManager struct {
	registry metrics.KubeRegistry
}

func NewMetricsManager() metricsManager {
	mm := metricsManager{
		registry: metrics.NewKubeRegistry(),
	}
	return mm
}

func (mm *metricsManager) GetRegistry() metrics.KubeRegistry {
	return mm.registry
}

func (mm *metricsManager) registerComponentVersionMetric() {
	mm.registry.MustRegister(gkeComponentVersion)
}

func (mm *metricsManager) recordComponentVersionMetric() error {
	v := getEnvVar(envGKEPDCSIVersion)
	if v == "" {
		klog.V(2).Info("Skip emitting component version metric")
		return fmt.Errorf("Failed to register GKE component version metric, env variable %v not defined", envGKEPDCSIVersion)
	}

	gkeComponentVersion.WithLabelValues(v).Set(1.0)
	klog.Infof("Recorded GKE component version : %v", v)
	return nil
}

func (mm *metricsManager) EmitGKEComponentVersion() error {
	mm.registerComponentVersionMetric()
	if err := mm.recordComponentVersionMetric(); err != nil {
		return err
	}

	return nil
}

// Server represents any type that could serve HTTP requests for the metrics
// endpoint.
type Server interface {
	Handle(pattern string, handler http.Handler)
}

// RegisterToServer registers an HTTP handler for this metrics manager to the
// given server at the specified address/path.
func (mm *metricsManager) registerToServer(s Server, metricsPath string) {
	s.Handle(metricsPath, metrics.HandlerFor(
		mm.GetRegistry(),
		metrics.HandlerOpts{
			ErrorHandling: metrics.ContinueOnError}))
}

// InitializeHttpHandler sets up a server and creates a handler for metrics.
func (mm *metricsManager) InitializeHttpHandler(address, path string) {
	mux := http.NewServeMux()
	mm.registerToServer(mux, path)
	go func() {
		klog.Infof("Metric server listening at %q", address)
		if err := http.ListenAndServe(address, mux); err != nil {
			klog.Fatalf("Failed to start metric server at specified address (%q) and path (%q): %w", address, path, err)
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
