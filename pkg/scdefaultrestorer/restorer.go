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

package scdefaultrestorer

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	defaultClassAnnotation = "storageclass.kubernetes.io/is-default-class"
	// markerAnnotation is applied to StorageClasses when their default annotation is
	// disabled and defaulting is handled by the Mutating Admission Policy (MAP). It tracks that the default annotation
	// was disabled by the system, allowing us to rollback and restore the default
	// annotation if required on the clusters.
	markerAnnotation = "pdcsi.storage.gke.io/install-default-using-map"
)

type Config struct {
	StorageClassName string
	APITimeout       time.Duration
}

type Restorer struct {
	config Config
}

func New(cfg Config) *Restorer {
	return &Restorer{config: cfg}
}

func (r *Restorer) Run(ctx context.Context) error {
	kubeConfig, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to build Kubernetes client: %w", err)
	}

	// Wait for API server to be ready
	if err := r.waitForAPIServer(ctx, clientset); err != nil {
		return err
	}

	// Restore the default annotation on StorageClass
	return r.restoreDefaultAnnotation(ctx, clientset)
}

func (r *Restorer) waitForAPIServer(ctx context.Context, client kubernetes.Interface) error {
	klog.InfoS("Waiting for API server to be ready")

	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, r.config.APITimeout, true, func(ctx context.Context) (bool, error) {
		result := client.Discovery().RESTClient().Get().AbsPath("/healthz").Do(ctx)
		if result.Error() != nil {
			return false, nil
		}
		var statusCode int
		result.StatusCode(&statusCode)
		return statusCode == 200, nil
	})

	if err != nil {
		return fmt.Errorf("timeout waiting for API server: %w", err)
	}
	return nil
}

func (r *Restorer) restoreDefaultAnnotation(ctx context.Context, client kubernetes.Interface) error {
	scClient := client.StorageV1().StorageClasses()
	targetSCName := r.config.StorageClassName

	targetSC, err := scClient.Get(ctx, targetSCName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Warningf("StorageClass not found, skipping restoration: %s", targetSCName)
			return nil
		}
		return fmt.Errorf("failed to get StorageClass %s: %w", targetSCName, err)
	}

	annotations := targetSC.GetAnnotations()
	if annotations == nil || annotations[markerAnnotation] != "true" {
		klog.InfoS("Marker annotation not found, no restoration needed", "storageclass", targetSCName)
		return nil
	}

	klog.InfoS("Marker annotation found, checking for other defaults", "storageclass", targetSCName)

	// List all storage classes to check for other defaults
	scList, err := scClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list storage classes: %w", err)
	}

	// Find any other StorageClasses marked as default
	var otherDefaults []string
	for _, sc := range scList.Items {
		if sc.Name == targetSCName {
			continue
		}
		if sc.Annotations != nil && sc.Annotations[defaultClassAnnotation] == "true" {
			otherDefaults = append(otherDefaults, sc.Name)
		}
	}

	// Prepare patch: either restore as default or just remove marker
	var patchBytes []byte
	if len(otherDefaults) == 0 {
		klog.InfoS("No other defaults found, restoring as default", "storageclass", targetSCName)
		patchBytes = []byte(fmt.Sprintf(`{"metadata": {"annotations":{%q:"true", %q: null}}}`, defaultClassAnnotation, markerAnnotation))
	} else {
		klog.InfoS("Other defaults found, removing marker only", "storageclass", targetSCName, "otherDefaults", otherDefaults)
		patchBytes = []byte(fmt.Sprintf(`{"metadata": {"annotations":{%q: null}}}`, markerAnnotation))
	}

	_, err = scClient.Patch(ctx, targetSCName, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to patch storage class %s: %w", targetSCName, err)
	}

	klog.InfoS("Successfully restored storage class default configuration", "storageclass", targetSCName)
	return nil
}
