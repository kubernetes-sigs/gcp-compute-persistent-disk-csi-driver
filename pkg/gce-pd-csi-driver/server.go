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

package gceGCEDriver

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/metrics"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
)

// Defines Non blocking GRPC server interfaces
type NonBlockingGRPCServer interface {
	// Start services at the endpoint
	Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer)
	// Waits for the service to stop
	Wait()
	// Stops the service gracefully
	Stop()
	// Stops the service forcefully
	ForceStop()
}

func NewNonBlockingGRPCServer(enableOtelTracing bool, metricsManager *metrics.MetricsManager) NonBlockingGRPCServer {
	return &nonBlockingGRPCServer{otelTracing: enableOtelTracing, metricsManager: metricsManager}
}

// NonBlocking server
type nonBlockingGRPCServer struct {
	wg             sync.WaitGroup
	server         *grpc.Server
	otelTracing    bool
	metricsManager *metrics.MetricsManager
}

func (s *nonBlockingGRPCServer) Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {

	s.wg.Add(1)

	go s.serve(endpoint, ids, cs, ns)

	return
}

func (s *nonBlockingGRPCServer) Wait() {
	s.wg.Wait()
}

func (s *nonBlockingGRPCServer) Stop() {
	s.server.GracefulStop()
}

func (s *nonBlockingGRPCServer) ForceStop() {
	s.server.Stop()
}

func (s *nonBlockingGRPCServer) serve(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	interceptors := []grpc.UnaryServerInterceptor{logGRPC}
	if s.metricsManager != nil {
		metricsInterceptor := metrics.MetricInterceptor{
			MetricsManager: s.metricsManager,
		}
		interceptors = append(interceptors, metricsInterceptor.UnaryInterceptor())
	}
	if s.otelTracing {
		interceptors = append(interceptors, otelgrpc.UnaryServerInterceptor())
	}
	grpcInterceptor := grpc.ChainUnaryInterceptor(interceptors...)

	opts := []grpc.ServerOption{
		grpcInterceptor,
	}

	u, err := url.Parse(endpoint)

	if err != nil {
		klog.Fatal(err.Error())
	}

	var addr string
	if u.Scheme == "unix" {
		addr = u.Path
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			klog.Fatalf("Failed to remove %s, error: %s", addr, err.Error())
		}

		listenDir := filepath.Dir(addr)
		if _, err := os.Stat(listenDir); err != nil {
			if os.IsNotExist(err) {
				klog.Fatalf("Expected Kubelet plugin watcher to create parent dir %s but did not find such a dir", listenDir)
			} else {
				klog.Fatalf("Failed to stat %s, error: %s", listenDir, err.Error())
			}
		}

	} else if u.Scheme == "tcp" {
		addr = u.Host
	} else {
		klog.Fatalf("%v endpoint scheme not supported", u.Scheme)
	}

	klog.V(4).Infof("Start listening with scheme %v, addr %v", u.Scheme, addr)
	listener, err := net.Listen(u.Scheme, addr)
	if err != nil {
		klog.Fatalf("Failed to listen: %v", err.Error())
	}

	server := grpc.NewServer(opts...)
	s.server = server

	if ids != nil {
		csi.RegisterIdentityServer(server, ids)
	}
	if cs != nil {
		csi.RegisterControllerServer(server, cs)
	}
	if ns != nil {
		csi.RegisterNodeServer(server, ns)
	}

	klog.V(4).Infof("Listening for connections on address: %#v", listener.Addr())

	if err := server.Serve(listener); err != nil {
		klog.Fatalf("Failed to serve: %v", err.Error())
	}

}
