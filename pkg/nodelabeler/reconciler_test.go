package nodelabeler

import (
	"context"
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
			name: "should not remove labels that are already present",
			initialObjects: []runtime.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						Labels: map[string]string{
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
				instanceTypeLabel: "e2-medium",
				constants.DiskTypeKeyPrefix + "/pd-standard": "true",
				constants.DiskTypeKeyPrefix + "/pd-ssd":      "true",
				constants.DiskTypeKeyPrefix + "/pd-extreme":  "true",
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
