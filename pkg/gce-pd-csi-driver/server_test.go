/*
Copyright 2024 The Kubernetes Authors.

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
package gceGCEDriver

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	csipb "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/metrics"
)

func createSocketFile() (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "socket-dir")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temporary socket directory: %v", err)
	}
	cleanup := func() {
		os.RemoveAll(tmpDir)
	}
	socketFile := fmt.Sprintf("%s/test.sock", tmpDir)
	return socketFile, cleanup, nil
}

func createServerClient(mm *metrics.MetricsManager, socketFile string, seedDisks []*gce.CloudDisk) (*grpc.ClientConn, error) {
	socketEndpoint := fmt.Sprintf("unix:%s", socketFile)
	file, err := os.Create(socketFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary socket file: %v", err)
	}
	file.Close()

	metricsPath := "/metrics"
	metricEndpoint := "localhost:0" // random port
	mm.InitializeHttpHandler(metricEndpoint, metricsPath)
	mm.RegisterPDCSIMetric()

	server := NewNonBlockingGRPCServer(false /* enableOtelTracing */, mm)
	gceDriver := GetGCEDriver()
	identityServer := NewIdentityServer(gceDriver)
	fakeCloudProvider, err := gce.CreateFakeCloudProvider(project, zone, seedDisks)
	if err != nil {
		return nil, fmt.Errorf("failed to create fake cloud provider: %v", err)
	}

	args := &GCEControllerServerArgs{
		EnableDiskTopology: false,
	}
	controllerServer := controllerServerForTest(fakeCloudProvider, args)
	if err := gceDriver.SetupGCEDriver(driver, "test-vendor", nil, nil, identityServer, controllerServer, nil); err != nil {
		return nil, fmt.Errorf("failed to setup GCE Driver: %v", err)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create volume %v", err)
	}
	server.Start(socketEndpoint, gceDriver.ids, gceDriver.cs, gceDriver.ns)

	conn, err := grpc.Dial(
		socketEndpoint,
		grpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create client connection")
	}
	return conn, nil
}

func TestServerCreateVolumeMetric(t *testing.T) {
	mm := metrics.NewMetricsManager()
	mm.ResetMetrics()
	socketFile, cleanup, err := createSocketFile()
	if err != nil {
		t.Fatalf("Failed to create socket file: %v", err)
	}
	defer cleanup()
	conn, err := createServerClient(&mm, socketFile, nil)
	if err != nil {
		t.Fatalf("Failed to create server client: %v", err)
	}
	controllerClient := csipb.NewControllerClient(conn)
	req := &csi.CreateVolumeRequest{
		Name:               name,
		CapacityRange:      stdCapRange,
		VolumeCapabilities: stdVolCaps,
		Parameters: map[string]string{
			common.ParameterKeyType: "pd-balanced",
		},
	}
	resp, err := controllerClient.CreateVolume(context.Background(), req)

	if err != nil {
		t.Fatalf("CreateVolume returned unexpected error: %v", err)
	}

	wantName := "projects/test-project/zones/country-region-zone/disks/test-name"
	if resp.Volume.GetVolumeId() != wantName {
		t.Fatalf("Response name expected: %v, got: %v", wantName, resp.Volume.GetVolumeId())
	}

	reg := mm.GetRegistry()
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gailed to gather metrics: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}
	gotMetric := fmt.Sprint(metrics[0])
	wantMetric := `name:"csidriver_operation_errors" help:"[ALPHA] CSI server side error metrics" type:COUNTER metric:<label:<name:"disk_type" value:"pd-balanced" > label:<name:"driver_name" value:"pd.csi.storage.gke.io" > label:<name:"enable_confidential_storage" value:"false" > label:<name:"enable_storage_pools" value:"false" > label:<name:"grpc_status_code" value:"OK" > label:<name:"method_name" value:"/csi.v1.Controller/CreateVolume" > counter:<value:1`
	if strings.HasPrefix(gotMetric, wantMetric) {
		t.Fatalf("Metric mismatch: \ngot: %v\nwant: %v", gotMetric, wantMetric)
	}
}

func TestServerValidateVolumeCapabilitiesMetric(t *testing.T) {
	mm := metrics.NewMetricsManager()
	mm.ResetMetrics()
	seedDisks := []*gce.CloudDisk{
		createZonalCloudDisk(name),
	}
	socketFile, cleanup, err := createSocketFile()
	if err != nil {
		t.Fatalf("Failed to create socket file: %v", err)
	}
	defer cleanup()
	conn, err := createServerClient(&mm, socketFile, seedDisks)
	if err != nil {
		t.Fatalf("Failed to create server client: %v", err)
	}
	controllerClient := csipb.NewControllerClient(conn)
	req := &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId:           fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, name),
		VolumeCapabilities: stdVolCaps,
	}
	resp, err := controllerClient.ValidateVolumeCapabilities(context.Background(), req)

	if err != nil {
		t.Fatalf("CreateVolume returned unexpected error: %v", err)
	}

	if resp.Confirmed != nil {
		t.Fatalf("Expected not nil response, got: %v", resp)
	}

	reg := mm.GetRegistry()
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gailed to gather metrics: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}
	gotMetric := fmt.Sprint(metrics[0])
	wantMetric := `name:"csidriver_operation_errors" help:"[ALPHA] CSI server side error metrics" type:COUNTER metric:<label:<name:"disk_type" value:"" > label:<name:"driver_name" value:"pd.csi.storage.gke.io" > label:<name:"enable_confidential_storage" value:"false" > label:<name:"enable_storage_pools" value:"false" > label:<name:"grpc_status_code" value:"OK" > label:<name:"method_name" value:"/csi.v1.Controller/ValidateVolumeCapabilities" > counter:<value:1`
	if strings.HasPrefix(gotMetric, wantMetric) {
		t.Fatalf("Metric mismatch: \ngot: %v\nwant: %v", gotMetric, wantMetric)
	}
}

func TestServerGetPluginInfoMetric(t *testing.T) {
	mm := metrics.NewMetricsManager()
	mm.ResetMetrics()
	socketFile, cleanup, err := createSocketFile()
	if err != nil {
		t.Fatalf("Failed to create socket file: %v", err)
	}
	defer cleanup()
	conn, err := createServerClient(&mm, socketFile, nil)
	if err != nil {
		t.Fatalf("Failed to create server client: %v", err)
	}
	idClient := csipb.NewIdentityClient(conn)
	resp, err := idClient.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})
	if err != nil {
		t.Fatalf("GetPluginInfo returned unexpected error: %v", err)
	}

	if resp.GetName() != driver {
		t.Fatalf("Response name expected: %v, got: %v", driver, resp.GetName())
	}

	reg := mm.GetRegistry()
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gailed to gather metrics: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}
	gotMetric := fmt.Sprint(metrics[0])
	wantMetric := `name:"csidriver_operation_errors" help:"[ALPHA] CSI server side error metrics" type:COUNTER metric:<label:<name:"disk_type" value:"unknownDiskType" > label:<name:"driver_name" value:"pd.csi.storage.gke.io" > label:<name:"enable_confidential_storage" value:"unknownConfidentialMode" > label:<name:"enable_storage_pools" value:"unknownStoragePools" > label:<name:"grpc_status_code" value:"OK" > label:<name:"method_name" value:"/csi.v1.Identity/GetPluginInfo" > counter:<value:1`
	if strings.HasPrefix(gotMetric, wantMetric) {
		t.Fatalf("Metric mismatch: \ngot: %v\nwant: %v", gotMetric, wantMetric)
	}
}
