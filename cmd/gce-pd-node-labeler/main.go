/*
Copyright 2025 The Kubernetes Authors.

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

// Package main is the GCE PD Node Labeler entrypoint.
package main

import (
	"flag"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/nodelabeler"
)

var (
	configmapName      = flag.String("configmap-name", "machine-pd-compatibility", "Name of the ConfigMap to use for machine & PD compatibility information.")
	configmapNamespace = flag.String("configmap-namespace", "gce-pd-csi-driver", "Namespace of the ConfigMap to use for machine & PD compatibility information.")
	httpEndpoint       = flag.String("http-endpoint", ":22015", "HTTP endpoint for health checks.")
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	log.SetLogger(klog.NewKlogr())
	ctx := signals.SetupSignalHandler()

	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{
		HealthProbeBindAddress: *httpEndpoint,
	})
	if err != nil {
		klog.Fatalf("Failed to create manager: %v", err)
	}

	if err := mgr.AddHealthzCheck("healthz/node-labeler", healthz.Ping); err != nil {
		klog.Fatalf("Failed to set up health check: %v", err)
	}

	reconciler := &nodelabeler.Reconciler{
		Client:             mgr.GetClient(),
		ConfigMapName:      *configmapName,
		ConfigMapNamespace: *configmapNamespace,
	}

	eventHandler := &nodelabeler.ConfigMapEventHandler{
		Client:     mgr.GetClient(),
		Reconciler: reconciler,
	}

	err = builder.ControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Watches(
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      *configmapName,
					Namespace: *configmapNamespace,
				},
			},
			handler.EnqueueRequestsFromMapFunc(eventHandler.Map),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				return obj.GetName() == *configmapName && obj.GetNamespace() == *configmapNamespace
			})),
		).
		Complete(reconciler)
	if err != nil {
		klog.Fatalf("Failed to build controller: %v", err)
	}

	klog.Info("Starting manager")
	if err := mgr.Start(ctx); err != nil {
		klog.Fatalf("Failed to start manager: %v", err)
	}
}
