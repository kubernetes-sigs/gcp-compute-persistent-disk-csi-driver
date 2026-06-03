/*
Copyright 2026 The Kubernetes Authors.

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

// Package main is the GCE PD CSI default StorageClass rollback handler entrypoint.
package main

import (
	"context"
	"flag"
	"os"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/scdefaultrestorer"
)

func main() {
	var config scdefaultrestorer.Config

	flag.StringVar(&config.StorageClassName, "storageclass-name", "standard-rwo", "Target StorageClass to restore default annotation for")
	flag.DurationVar(&config.APITimeout, "api-timeout", 30*time.Second, "Maximum time to wait for API server readiness")

	klog.InitFlags(nil)
	flag.Parse()

	klog.InfoS("Starting MAP default storageclass rollback handler")

	restorer := scdefaultrestorer.New(config)
	if err := restorer.Run(context.Background()); err != nil {
		klog.ErrorS(err, "Failed to restore storage class defaults")
		os.Exit(1)
	}
}
