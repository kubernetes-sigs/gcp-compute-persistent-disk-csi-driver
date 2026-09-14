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
	PDCSIReadyConditionType = v1.NodeConditionType("node.gke.io/PDCSIReady")
	PDCSIReadyReason        = "CSIDriverReady"
	PDCSIReadyMessage       = "GCE PD CSI driver node service is registered and ready"
	registrarHealthzURL     = "http://127.0.0.1:9931/healthz"

	registrarPollInterval = 500 * time.Millisecond
	registrarHTTPTimeout  = 2 * time.Second
	patchRetryInterval    = 5 * time.Second
)

// NodeReadinessReporter polls csi-driver-registrar and patches PDCSIReady=True on Node status once ready.
type NodeReadinessReporter struct {
	nodeName      string
	healthzURL    string
	kubeClient    kubernetes.Interface
	httpClient    *http.Client
	pollInterval  time.Duration
	patchInterval time.Duration
}

// NewNodeReadinessReporter creates a NodeReadinessReporter for the given node.
func NewNodeReadinessReporter(nodeName string, kubeClient kubernetes.Interface) (*NodeReadinessReporter, error) {
	if nodeName == "" {
		return nil, errors.New("node name is empty")
	}
	if kubeClient == nil {
		return nil, errors.New("kube client is nil")
	}

	return &NodeReadinessReporter{
		nodeName:      nodeName,
		healthzURL:    registrarHealthzURL,
		kubeClient:    kubeClient,
		httpClient:    &http.Client{Timeout: registrarHTTPTimeout},
		pollInterval:  registrarPollInterval,
		patchInterval: patchRetryInterval,
	}, nil
}

// Run polls the registrar healthz endpoint until HTTP 200 is returned, then issues a one-shot
// StrategicMergePatch setting node.gke.io/PDCSIReady=True on the Node status, unless it is already set.
func (r *NodeReadinessReporter) Run(ctx context.Context) {
	if err := r.waitForRegistrarHealthz(ctx); err != nil {
		klog.Warningf("Node readiness reporter stopped while waiting for registrar healthz: %v", err)
		return
	}

	// Skip the patch if the condition is already True to avoid redundant Node writes
	if alreadyTrue, err := r.conditionAlreadyTrue(ctx); err != nil {
		klog.Warningf("Could not read node %s to check condition %s before patching (will patch anyway): %v", r.nodeName, PDCSIReadyConditionType, err)
	} else if alreadyTrue {
		klog.Infof("Node %s already has condition %s=True; skipping patch", r.nodeName, PDCSIReadyConditionType)
		return
	}

	if err := r.patchNodeReadinessCondition(ctx); err != nil {
		if ctx.Err() != nil {
			klog.Infof("Node readiness reporter stopped due to context cancellation: %v", err)
		} else {
			klog.Errorf("Node readiness reporter failed to patch node %s condition %s=True: %v", r.nodeName, PDCSIReadyConditionType, err)
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

// patchNodeReadinessCondition retries the status patch at a fixed interval until it succeeds or ctx is cancelled.
func (r *NodeReadinessReporter) patchNodeReadinessCondition(ctx context.Context) error {
	now := metav1.Now()
	patch := map[string]any{
		"status": map[string]any{
			"conditions": []v1.NodeCondition{
				{
					Type:               PDCSIReadyConditionType,
					Status:             v1.ConditionTrue,
					LastHeartbeatTime:  now,
					LastTransitionTime: now,
					Reason:             PDCSIReadyReason,
					Message:            PDCSIReadyMessage,
				},
			},
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal node readiness condition patch: %w", err)
	}

	return wait.PollUntilContextCancel(ctx, r.patchInterval, true, func(ctx context.Context) (bool, error) {
		_, err := r.kubeClient.CoreV1().Nodes().Patch(
			ctx,
			r.nodeName,
			types.StrategicMergePatchType,
			patchBytes,
			metav1.PatchOptions{},
			"status",
		)
		if err != nil {
			klog.Warningf("Failed to patch node %s status with condition %s=True (will retry in %v): %v", r.nodeName, PDCSIReadyConditionType, r.patchInterval, err)
			return false, nil
		}

		klog.Infof("Successfully patched node %s status with condition %s=True", r.nodeName, PDCSIReadyConditionType)
		return true, nil
	})
}

// conditionAlreadyTrue reports whether the node already has PDCSIReady=True.
func (r *NodeReadinessReporter) conditionAlreadyTrue(ctx context.Context) (bool, error) {
	node, err := r.kubeClient.CoreV1().Nodes().Get(ctx, r.nodeName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	cond := getNodeCondition(node, PDCSIReadyConditionType)
	return cond != nil && cond.Status == v1.ConditionTrue, nil
}

// getNodeCondition returns the node condition of the given type, or nil if absent.
func getNodeCondition(node *v1.Node, condType v1.NodeConditionType) *v1.NodeCondition {
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == condType {
			return &node.Status.Conditions[i]
		}
	}
	return nil
}
