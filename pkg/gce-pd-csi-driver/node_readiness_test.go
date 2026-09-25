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
	"strings"
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
	// Deliberately non-GKE to verify the condition type is configurable.
	testConditionType = v1.NodeConditionType("example.com/PDCSIReady")
)

// TestNodeReadinessReporter tests startup False patching, registrar healthz polling, and True condition retry.
func TestNodeReadinessReporter(t *testing.T) {
	testCases := []struct {
		name               string
		nodeName           string
		emptyConditionType bool
		emptyEndpoint      bool
		nilClient          bool
		failHealthz        int32
		failPatch          int32
		initialReady       bool
		cancelBefore200    bool
		expectErr          bool
		expectedStatus     v1.ConditionStatus
		expectedReason     string
		expectedPatchCalls int32
	}{
		{
			name:               "normal - patches False on startup, registrar 503 then patches True",
			nodeName:           testNodeName,
			failHealthz:        2,
			expectedStatus:     v1.ConditionTrue,
			expectedReason:     PDCSIReadyReason,
			expectedPatchCalls: 2,
		},
		{
			name:               "startup False patch error does not block True patch",
			nodeName:           testNodeName,
			failPatch:          1,
			expectedStatus:     v1.ConditionTrue,
			expectedReason:     PDCSIReadyReason,
			expectedPatchCalls: 2,
		},
		{
			name:               "transient True patch error retried successfully",
			nodeName:           testNodeName,
			failPatch:          3,
			expectedStatus:     v1.ConditionTrue,
			expectedReason:     PDCSIReadyReason,
			expectedPatchCalls: 4,
		},
		{
			name:               "context cancellation before registrar ready clears stale True to False",
			nodeName:           testNodeName,
			initialReady:       true,
			failHealthz:        1000,
			cancelBefore200:    true,
			expectedStatus:     v1.ConditionFalse,
			expectedReason:     PDCSINotReadyReason,
			expectedPatchCalls: 1,
		},
		{
			name:      "missing node name",
			expectErr: true,
		},
		{
			name:               "missing condition type",
			nodeName:           testNodeName,
			emptyConditionType: true,
			expectErr:          true,
		},
		{
			name:          "missing registrar endpoint",
			nodeName:      testNodeName,
			emptyEndpoint: true,
			expectErr:     true,
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
			var healthzCalls, patchCalls atomic.Int32

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			fakeClient := fake.NewClientset(testNode(tc.initialReady))
			fakeClient.PrependReactor("patch", "nodes", failNTimes(tc.failPatch, &patchCalls))

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

			endpoint := strings.TrimPrefix(server.URL, "http://")
			if tc.emptyEndpoint {
				endpoint = ""
			}
			conditionType := string(testConditionType)
			if tc.emptyConditionType {
				conditionType = ""
			}
			reporter, err := newTestReporter(tc.nodeName, conditionType, client, endpoint)
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

			cond := getNodeCondition(updatedNode, testConditionType)
			if cond == nil || cond.Status != tc.expectedStatus || cond.Reason != tc.expectedReason {
				t.Fatalf("Expected condition (%s=%s, reason=%s), got %+v", testConditionType, tc.expectedStatus, tc.expectedReason, cond)
			}
			if updatedNode.Status.NodeInfo.KernelVersion != testKernelVersion {
				t.Errorf("Expected NodeInfo.KernelVersion %q to be preserved, got %q", testKernelVersion, updatedNode.Status.NodeInfo.KernelVersion)
			}
		})
	}
}

func newTestReporter(nodeName, conditionType string, client kubernetes.Interface, registrarEndpoint string) (*NodeReadinessReporter, error) {
	reporter, err := NewNodeReadinessReporter(nodeName, conditionType, registrarEndpoint, client)
	if err != nil {
		return nil, err
	}
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

// testNode builds the initial node, optionally with PDCSIReady=True already set.
func testNode(ready bool) *v1.Node {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: testNodeName},
		Status:     v1.NodeStatus{NodeInfo: v1.NodeSystemInfo{KernelVersion: testKernelVersion}},
	}
	if ready {
		now := metav1.Now()
		node.Status.Conditions = []v1.NodeCondition{{
			Type:               testConditionType,
			Status:             v1.ConditionTrue,
			Reason:             PDCSIReadyReason,
			Message:            PDCSIReadyMessage,
			LastHeartbeatTime:  now,
			LastTransitionTime: now,
		}}
	}
	return node
}

func getNodeCondition(node *v1.Node, condType v1.NodeConditionType) *v1.NodeCondition {
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == condType {
			return &node.Status.Conditions[i]
		}
	}
	return nil
}
