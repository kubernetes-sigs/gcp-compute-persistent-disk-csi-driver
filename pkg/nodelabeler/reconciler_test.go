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

package nodelabeler

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
)

const (
	testConfigMapName      = "machine-pd-compatibility"
	testConfigMapNamespace = "gce-pd-csi-driver"
)

func TestReconcile(t *testing.T) {
	testCases := []struct {
		name           string
		initialObjects []runtime.Object
		req            reconcile.Request
		expectedLabels map[string]string
		expectError    bool
	}{
		{
			name: "should add labels to node",
			initialObjects: []runtime.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						Labels: map[string]string{
							instanceTypeLabel: "e2-medium",
						},
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testConfigMapName,
						Namespace: testConfigMapNamespace,
					},
					Data: map[string]string{
						"machine-pd-compatibility.json": `{ 
							"e2": { "pd-standard": true, "pd-ssd": true } 
						}`,
					},
				},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-node"},
			},
			expectedLabels: map[string]string{
				instanceTypeLabel: "e2-medium",
				constants.DiskTypeKeyPrefix + "/pd-standard": "true",
				constants.DiskTypeKeyPrefix + "/pd-ssd":      "true",
			},
			expectError: false,
		},
		{
			name: "should remove pd labels that are not in compatibility map",
			initialObjects: []runtime.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						Labels: map[string]string{
							"foo":             "bar",
							instanceTypeLabel: "e2-medium",
							constants.DiskTypeKeyPrefix + "/pd-standard": "true",
							constants.DiskTypeKeyPrefix + "/pd-ssd":      "true",
							constants.DiskTypeKeyPrefix + "/pd-extreme":  "true",
						},
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testConfigMapName,
						Namespace: testConfigMapNamespace,
					},
					Data: map[string]string{
						"machine-pd-compatibility.json": `{ 
							"e2": { "pd-standard": true, "pd-ssd": true } 
						}`,
					},
				},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-node"},
			},
			expectedLabels: map[string]string{
				"foo":             "bar",
				instanceTypeLabel: "e2-medium",
				constants.DiskTypeKeyPrefix + "/pd-standard": "true",
				constants.DiskTypeKeyPrefix + "/pd-ssd":      "true",
			},
			expectError: false,
		},
		{
			name: "should do nothing if node not found",
			initialObjects: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: testConfigMapName, Namespace: testConfigMapNamespace},
					Data:       map[string]string{"machine-pd-compatibility.json": `{}`},
				},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "non-existent-node"},
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			corev1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tc.initialObjects...).Build()

			reconciler := &Reconciler{
				Client:             fakeClient,
				ConfigMapName:      testConfigMapName,
				ConfigMapNamespace: testConfigMapNamespace,
			}

			if err := reconciler.UpdateMachinePDCompatibility(context.Background()); err != nil {
				if len(tc.initialObjects) > 1 {
					t.Fatalf("UpdateDiskCompatibilityMapFromConfigMap() failed: %v", err)
				}
			}

			if _, err := reconciler.Reconcile(context.Background(), tc.req); (err != nil) != tc.expectError {
				t.Fatalf("Reconcile() error = %v, wantErr %v", err, tc.expectError)
			}

			if tc.expectedLabels != nil {
				updatedNode := &corev1.Node{}
				if err := fakeClient.Get(context.Background(), tc.req.NamespacedName, updatedNode); err != nil {
					t.Fatalf("Failed to get updated node: %v", err)
				}
				if diff := cmp.Diff(tc.expectedLabels, updatedNode.Labels); diff != "" {
					t.Errorf("Node labels mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestUpdateMachinePDCompatibility(t *testing.T) {
	testCases := []struct {
		name          string
		configMap     *corev1.ConfigMap
		expectedMap   map[string][]string
		expectErr     bool
		reconciler    *Reconciler
		configMapName string
	}{
		{
			name: "valid configmap",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: "test-ns",
				},
				Data: map[string]string{
					machinePDCompatibilityKey: `{"e2":{"pd-balanced":true,"pd-standard":true},"n2":{"pd-standard":true,"pd-ssd":true}}`,
				},
			},
			expectedMap: map[string][]string{
				"n2": {"pd-standard", "pd-ssd"},
				"e2": {"pd-balanced", "pd-standard"},
			},
			expectErr: false,
			reconciler: &Reconciler{
				ConfigMapName:      "test-cm",
				ConfigMapNamespace: "test-ns",
			},
			configMapName: "test-cm",
		},
		{
			name: "configmap missing key",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: "test-ns",
				},
				Data: map[string]string{},
			},
			expectedMap: nil,
			expectErr:   false,
			reconciler: &Reconciler{
				ConfigMapName:      "test-cm",
				ConfigMapNamespace: "test-ns",
			},
			configMapName: "test-cm",
		},
		{
			name: "invalid json",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: "test-ns",
				},
				Data: map[string]string{
					machinePDCompatibilityKey: `{"n2":}`,
				},
			},
			expectedMap: nil,
			expectErr:   true,
			reconciler: &Reconciler{
				ConfigMapName:      "test-cm",
				ConfigMapNamespace: "test-ns",
			},
			configMapName: "test-cm",
		},
		{
			name: "configmap not found",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: "test-ns",
				},
			},
			expectedMap: nil,
			expectErr:   true,
			reconciler: &Reconciler{
				ConfigMapName:      "test-cm",
				ConfigMapNamespace: "test-ns",
			},
			configMapName: "wrong-name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithObjects(tc.configMap).Build()
			tc.reconciler.Client = fakeClient
			tc.reconciler.ConfigMapName = tc.configMapName
			err := tc.reconciler.UpdateMachinePDCompatibility(context.Background())
			if (err != nil) != tc.expectErr {
				t.Errorf("UpdateMachinePDCompatibility() error = %v, wantErr %v", err, tc.expectErr)
				return
			}

			if tc.expectedMap != nil {
				expectedJSONBytes, _ := json.Marshal(tc.expectedMap)
				gotJSONBytes, _ := json.Marshal(tc.reconciler.compatibility)
				var want, got map[string][]string
				json.Unmarshal(expectedJSONBytes, &want)
				json.Unmarshal(gotJSONBytes, &got)
				for _, v := range want {
					sort.Strings(v)
				}
				for _, v := range got {
					sort.Strings(v)
				}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("UpdateMachinePDCompatibility() mismatch (-want +got):\n%s", diff)
				}
			} else if tc.reconciler.compatibility != nil {
				t.Errorf("UpdateMachinePDCompatibility() compatibility map = %v, want nil", tc.reconciler.compatibility)
			}
		})
	}
}
