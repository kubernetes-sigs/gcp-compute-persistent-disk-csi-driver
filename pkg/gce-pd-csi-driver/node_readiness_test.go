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
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	testNodeName      = "gke-test-node-1"
	testKernelVersion = "6.6.137+"
)

// TestNodeReadinessReporter tests registrar healthz polling, retry backoffs, and node condition patching/skipping.
func TestNodeReadinessReporter(t *testing.T) {
	testCases := []struct {
		name               string
		nodeName           string
		nilClient          bool
		failHealthz        int32
		failPatch          int32
		failGet            int32
		initialReady       bool
		cancelBefore200    bool
		expectErr          bool
		expectReady        bool
		expectedPatchCalls int32
	}{
		{
			name:               "normal - registrar 503 then ready",
			nodeName:           testNodeName,
			failHealthz:        2,
			expectReady:        true,
			expectedPatchCalls: 1,
		},
		{
			name:               "transient patch error retried successfully",
			nodeName:           testNodeName,
			failPatch:          2,
			expectReady:        true,
			expectedPatchCalls: 3,
		},
		{
			name:               "already ready condition skips patch",
			nodeName:           testNodeName,
			initialReady:       true,
			expectReady:        true,
			expectedPatchCalls: 0,
		},
		{
			name:               "get error falls back to patch",
			nodeName:           testNodeName,
			failGet:            1,
			expectReady:        true,
			expectedPatchCalls: 1,
		},
		{
			name:            "context cancellation before registrar ready",
			nodeName:        testNodeName,
			failHealthz:     1000,
			cancelBefore200: true,
			expectReady:     false,
		},
		{
			name:      "missing node name",
			expectErr: true,
		},
		{
			name:      "nil kube client",
			nodeName:  testNodeName,
			nilClient: true,
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var healthzCalls, patchCalls, getCalls atomic.Int32

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			// Sentinel timestamp to verify LastTransitionTime is preserved.
			oldTransitionTime := metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))

			fakeClient := fake.NewClientset(testNode(tc.initialReady, oldTransitionTime))
			fakeClient.PrependReactor("patch", "nodes", failNTimes(tc.failPatch, &patchCalls))
			fakeClient.PrependReactor("get", "nodes", failNTimes(tc.failGet, &getCalls))

			var client kubernetes.Interface = fakeClient
			if tc.nilClient {
				client = nil
			}

			// Mock registrar healthz endpoint.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callNum := healthzCalls.Add(1)
				if tc.cancelBefore200 && callNum >= 2 {
					cancel()
				}
				if callNum <= tc.failHealthz {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			reporter, err := newTestReporter(tc.nodeName, client, server.URL)
			if err != nil && !tc.expectErr {
				t.Fatalf("Got unexpected error: %v", err)
			}
			if err == nil && tc.expectErr {
				t.Fatalf("Expected error, got %v", err)
			}
			if tc.expectErr {
				return
			}

			reporter.Run(ctx)

			// Verify the patch count and the resulting node condition.
			if got := patchCalls.Load(); got != tc.expectedPatchCalls {
				t.Errorf("Expected %d patch calls, got %d", tc.expectedPatchCalls, got)
			}

			updatedNode, err := fakeClient.CoreV1().Nodes().Get(context.Background(), testNodeName, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Failed to get node %s: %v", testNodeName, err)
			}

			cond := getNodeCondition(updatedNode, PDCSIReadyConditionType)
			if tc.expectReady {
				if cond == nil || cond.Status != v1.ConditionTrue || cond.Reason != PDCSIReadyReason {
					t.Fatalf("Expected condition (%s=%s, reason=%s), got %+v", PDCSIReadyConditionType, v1.ConditionTrue, PDCSIReadyReason, cond)
				}
				if updatedNode.Status.NodeInfo.KernelVersion != testKernelVersion {
					t.Errorf("Expected NodeInfo.KernelVersion %q to be preserved, got %q", testKernelVersion, updatedNode.Status.NodeInfo.KernelVersion)
				}
				// A skipped patch must preserve the original LastTransitionTime.
				if tc.initialReady && !cond.LastTransitionTime.Time.Equal(oldTransitionTime.Time) {
					t.Errorf("Expected LastTransitionTime %v to be preserved, got %v", oldTransitionTime, cond.LastTransitionTime)
				}
			} else if cond != nil {
				t.Errorf("Expected no %s condition, got %+v", PDCSIReadyConditionType, cond)
			}
		})
	}
}

func newTestReporter(nodeName string, client kubernetes.Interface, healthzURL string) (*NodeReadinessReporter, error) {
	reporter, err := NewNodeReadinessReporter(nodeName, client)
	if err != nil {
		return nil, err
	}
	reporter.healthzURL = healthzURL
	reporter.pollInterval = 5 * time.Millisecond
	reporter.patchInterval = 5 * time.Millisecond
	return reporter, nil
}

// failNTimes returns a reactor that counts every matching call into calls and fails the
// first n of them with a transient error, letting the fake client handle the rest normally.
func failNTimes(n int32, calls *atomic.Int32) k8stesting.ReactionFunc {
	return func(action k8stesting.Action) (bool, runtime.Object, error) {
		if calls.Add(1) <= n {
			return true, nil, apierrors.NewInternalError(errors.New("transient apiserver error"))
		}
		return false, nil, nil
	}
}

// testNode builds the initial node, optionally with PDCSIReady=True already set at readySince.
func testNode(ready bool, readySince metav1.Time) *v1.Node {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: testNodeName},
		Status:     v1.NodeStatus{NodeInfo: v1.NodeSystemInfo{KernelVersion: testKernelVersion}},
	}
	if ready {
		node.Status.Conditions = []v1.NodeCondition{{
			Type:               PDCSIReadyConditionType,
			Status:             v1.ConditionTrue,
			Reason:             PDCSIReadyReason,
			Message:            PDCSIReadyMessage,
			LastHeartbeatTime:  readySince,
			LastTransitionTime: readySince,
		}}
	}
	return node
}
