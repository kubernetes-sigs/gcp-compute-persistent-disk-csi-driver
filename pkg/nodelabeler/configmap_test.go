package nodelabeler

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestConfigMapEventHandler_Map(t *testing.T) {
	testCases := []struct {
		name         string
		nodes        []client.Object
		configMap    *corev1.ConfigMap
		expectedReqs []reconcile.Request
	}{
		{
			name: "nodes exist",
			nodes: []client.Object{
				&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
				&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
			},
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: "test-ns",
				},
				Data: map[string]string{
					"machine-pd-compatibility.json": `{"n2":{"pd-standard":true}}`,
				},
			},
			expectedReqs: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: "node1"}},
				{NamespacedName: types.NamespacedName{Name: "node2"}},
			},
		},
		{
			name:  "no nodes exist",
			nodes: []client.Object{},
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: "test-ns",
				},
				Data: map[string]string{
					"machine-pd-compatibility.json": `{"n2":{"pd-standard":true}}`,
				},
			},
			expectedReqs: []reconcile.Request{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			allObjects := append(tc.nodes, tc.configMap)
			fakeClient := fake.NewClientBuilder().WithObjects(allObjects...).Build()

			reconciler := &Reconciler{
				Client:             fakeClient,
				ConfigMapName:      "test-cm",
				ConfigMapNamespace: "test-ns",
			}

			eventHandler := &ConfigMapEventHandler{
				Client:     fakeClient,
				Reconciler: reconciler,
			}

			got := eventHandler.Map(context.Background(), tc.configMap)
			if diff := cmp.Diff(tc.expectedReqs, got); diff != "" {
				t.Errorf("Map() returned diff (-want +got):\n%s", diff)
			}
		})
	}
}
