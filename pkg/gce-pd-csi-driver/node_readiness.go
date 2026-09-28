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

package gceGCEDriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	PDCSIReadyReason     = "CSIDriverReady"
	PDCSIReadyMessage    = "GCE PD CSI driver node service is registered and ready"
	PDCSINotReadyReason  = "CSIDriverNotReady"
	PDCSINotReadyMessage = "Waiting for GCE PD CSI driver node service to register with Kubelet"

	registrarPollInterval = 500 * time.Millisecond
	registrarHTTPTimeout  = 2 * time.Second
	patchRetryInterval    = 5 * time.Second
)

// NodeReadinessReporter polls csi-driver-registrar and patches the configured readiness condition on Node status.
type NodeReadinessReporter struct {
	nodeName      string
	conditionType v1.NodeConditionType
	healthzURL    string
	kubeClient    kubernetes.Interface
	httpClient    *http.Client
	pollInterval  time.Duration
	patchInterval time.Duration
}

// NewNodeReadinessReporter creates a NodeReadinessReporter that reports conditionType for the given node.
func NewNodeReadinessReporter(nodeName, conditionType, registrarEndpoint string, kubeClient kubernetes.Interface) (*NodeReadinessReporter, error) {
	if nodeName == "" {
		return nil, errors.New("node name is empty")
	}
	if conditionType == "" {
		return nil, errors.New("condition type is empty")
	}
	if registrarEndpoint == "" {
		return nil, errors.New("registrar endpoint is empty")
	}
	if kubeClient == nil {
		return nil, errors.New("kube client is nil")
	}

	return &NodeReadinessReporter{
		nodeName:      nodeName,
		conditionType: v1.NodeConditionType(conditionType),
		healthzURL:    fmt.Sprintf("http://%s/healthz", registrarEndpoint),
		kubeClient:    kubeClient,
		httpClient:    &http.Client{Timeout: registrarHTTPTimeout},
		pollInterval:  registrarPollInterval,
		patchInterval: patchRetryInterval,
	}, nil
}

// Run performs a one-shot readiness check: it makes a single attempt to set the readiness
// condition to False on startup without blocking, waits for the registrar healthz endpoint
// to return HTTP 200, and then retries patching the condition to True until it succeeds.
// Note: Hard crashes or ungraceful terminations leave the condition True until the container
// restarts and Run resets it to False.
func (r *NodeReadinessReporter) Run(ctx context.Context) {
	if err := r.patchConditionOnce(ctx, v1.ConditionFalse, PDCSINotReadyReason, PDCSINotReadyMessage); err != nil {
		klog.Warningf("Failed to set initial node %s condition %s=False (continuing): %v", r.nodeName, r.conditionType, err)
	}

	if err := r.waitForRegistrarHealthz(ctx); err != nil {
		klog.Warningf("Node readiness reporter stopped while waiting for registrar healthz: %v", err)
		return
	}

	if err := r.patchNodeReadinessCondition(ctx); err != nil {
		if ctx.Err() != nil {
			klog.Infof("Node readiness reporter stopped due to context cancellation: %v", err)
		} else {
			klog.Errorf("Node readiness reporter failed to patch node %s condition %s=True: %v", r.nodeName, r.conditionType, err)
		}
	}
}

func (r *NodeReadinessReporter) waitForRegistrarHealthz(ctx context.Context) error {
	return wait.PollUntilContextCancel(ctx, r.pollInterval, true, func(ctx context.Context) (bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.healthzURL, nil)
		if err != nil {
			// An invalid static URL will never succeed on retry so stop polling immediately.
			return false, err
		}

		resp, err := r.httpClient.Do(req)
		if err != nil {
			klog.V(4).Infof("Registrar healthz check %s failed: %v", r.healthzURL, err)
			return false, nil
		}
		defer resp.Body.Close()
		// Read and discard the response body so the TCP connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)

		if resp.StatusCode == http.StatusOK {
			klog.Infof("Registrar healthz check %s succeeded (HTTP 200)", r.healthzURL)
			return true, nil
		}

		klog.V(4).Infof("Registrar healthz check %s returned non-200 status: %d", r.healthzURL, resp.StatusCode)
		return false, nil
	})
}

// patchNodeReadinessCondition retries setting PDCSIReady=True at a fixed interval until it succeeds or ctx is cancelled.
func (r *NodeReadinessReporter) patchNodeReadinessCondition(ctx context.Context) error {
	return wait.PollUntilContextCancel(ctx, r.patchInterval, true, func(ctx context.Context) (bool, error) {
		if err := r.patchConditionOnce(ctx, v1.ConditionTrue, PDCSIReadyReason, PDCSIReadyMessage); err != nil {
			klog.Warningf("Failed to patch node %s status with condition %s=True (will retry in %v): %v", r.nodeName, r.conditionType, r.patchInterval, err)
			return false, nil
		}

		klog.Infof("Successfully patched node %s status with condition %s=True", r.nodeName, r.conditionType)
		return true, nil
	})
}

func (r *NodeReadinessReporter) patchConditionOnce(ctx context.Context, status v1.ConditionStatus, reason, message string) error {
	now := metav1.Now()
	patch := map[string]any{
		"status": map[string]any{
			"conditions": []v1.NodeCondition{
				{
					Type:               r.conditionType,
					Status:             status,
					LastHeartbeatTime:  now,
					LastTransitionTime: now,
					Reason:             reason,
					Message:            message,
				},
			},
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal node readiness condition patch: %w", err)
	}

	_, err = r.kubeClient.CoreV1().Nodes().Patch(
		ctx,
		r.nodeName,
		types.StrategicMergePatchType,
		patchBytes,
		metav1.PatchOptions{},
		"status",
	)
	return err
}
