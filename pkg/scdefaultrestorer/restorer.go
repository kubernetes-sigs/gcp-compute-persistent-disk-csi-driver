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
	"os"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/version"
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
	StorageClassName         string
	KubeconfigPath           string
	SCDefaultingByMAPEnabled string
	EmulatedVersion          string
	KubeconfigTimeout        time.Duration
	APITimeout               time.Duration
}

type Restorer struct {
	config Config
}

func New(cfg Config) *Restorer {
	return &Restorer{config: cfg}
}

func (r *Restorer) Run(ctx context.Context) error {
	// Check if SC defaulting by MAP is disabled - only proceed if it is
	if strings.ToLower(r.config.SCDefaultingByMAPEnabled) != "false" {
		klog.InfoS("Skipping default annotation check: SC defaulting by MAP is not disabled", "value", r.config.SCDefaultingByMAPEnabled)
		return nil
	}

	// Check if cluster version is 1.36+
	is136Plus, err := r.isVersion136Plus(r.config.EmulatedVersion)
	if err != nil {
		klog.ErrorS(err, "Failed to parse emulated version", "version", r.config.EmulatedVersion)
		return fmt.Errorf("invalid emulated version %q: %w", r.config.EmulatedVersion, err)
	}
	if !is136Plus {
		klog.InfoS("Skipping: cluster version is below 1.36", "version", r.config.EmulatedVersion)
		return nil
	}

	clientset, err := r.buildClientWithWait(ctx)
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

func (r *Restorer) isVersion136Plus(ver string) (bool, error) {
	parsedVersion, err := version.ParseGeneric(ver)
	if err != nil {
		return false, err
	}
	return parsedVersion.Major() > 1 || (parsedVersion.Major() == 1 && parsedVersion.Minor() >= 36), nil
}

func (r *Restorer) waitForKubeconfig(ctx context.Context) error {
	klog.InfoS("Waiting for kubeconfig file", "path", r.config.KubeconfigPath)

	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, r.config.KubeconfigTimeout, true, func(ctx context.Context) (bool, error) {
		_, err := os.Stat(r.config.KubeconfigPath)
		return err == nil, nil
	})

	if err != nil {
		return fmt.Errorf("timeout waiting for kubeconfig: %w", err)
	}
	return nil
}

func (r *Restorer) buildClientWithWait(ctx context.Context) (*kubernetes.Clientset, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			kubeConfig, err := config.GetConfig()
			if err == nil {
				// Config built successfully, attempt to create the clientset
				clientset, clientErr := kubernetes.NewForConfig(kubeConfig)
				if clientErr == nil {
					return clientset, nil
				}
				err = clientErr
			}

			klog.V(4).Infof("Waiting for kubeconfig to be ready: %v", err)
			time.Sleep(2 * time.Second)
		}
	}
}

func (r *Restorer) waitForAPIServer(ctx context.Context, client kubernetes.Interface) error {
	klog.Info("Waiting for API server to be ready")

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
		klog.Warningf("StorageClass not found, skipping restoration: %s", targetSCName)
		return nil
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
