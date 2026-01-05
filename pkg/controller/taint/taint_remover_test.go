package taint

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileNodeTaint_Reconcile(t *testing.T) {
	nodeName := "test-node"

	taintedNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: StartupTaintKey, Effect: corev1.TaintEffectNoSchedule},
				{Key: "other-taint", Effect: corev1.TaintEffectNoSchedule},
			},
		},
	}

	untaintedNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "other-taint", Effect: corev1.TaintEffectNoSchedule},
			},
		},
	}

	csiNodeWithoutDriver := &storagev1.CSINode{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Spec: storagev1.CSINodeSpec{
			Drivers: []storagev1.CSINodeDriver{
				{Name: "other.driver.io", NodeID: "other-id"},
			},
		},
	}

	csiNodeWithDriver := &storagev1.CSINode{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Spec: storagev1.CSINodeSpec{
			Drivers: []storagev1.CSINodeDriver{
				{Name: "other.driver.io", NodeID: "other-id"},
				{Name: DriverName, NodeID: "my-id"},
			},
		},
	}

	tests := []struct {
		name          string
		existingObjs  []client.Object
		req           reconcile.Request
		expectTaint   bool
		expectedError bool
	}{
		{
			name:          "CSINode Not Found (Do nothing)",
			existingObjs:  []client.Object{taintedNode},
			req:           reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}},
			expectTaint:   true, // Taint remains because CSINode doesn't exist
			expectedError: false,
		},
		{
			name:          "CSINode exists, Driver NOT registered",
			existingObjs:  []client.Object{taintedNode, csiNodeWithoutDriver},
			req:           reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}},
			expectTaint:   true, // Taint remains because driver isn't ready
			expectedError: false,
		},
		{
			name:          "CSINode exists, Driver Registered -> Remove Taint",
			existingObjs:  []client.Object{taintedNode, csiNodeWithDriver},
			req:           reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}},
			expectTaint:   false, // Taint should be removed
			expectedError: false,
		},
		{
			name:          "Node already untainted (Idempotent)",
			existingObjs:  []client.Object{untaintedNode, csiNodeWithDriver},
			req:           reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}},
			expectTaint:   false,
			expectedError: false,
		},
		{
			name:          "Node Not Found (Defensive check)",
			existingObjs:  []client.Object{csiNodeWithDriver},
			req:           reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}},
			expectTaint:   false, // Irrelevant check, but shouldn't crash
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().
				WithObjects(tt.existingObjs...)
			cl := builder.Build()

			r := &ReconcileNodeTaint{client: cl}
			_, err := r.Reconcile(context.TODO(), tt.req)
			if (err != nil) != tt.expectedError {
				t.Errorf("Reconcile() error = %v, expectedError %v", err, tt.expectedError)
				return
			}

			node := &corev1.Node{}
			err = cl.Get(context.TODO(), types.NamespacedName{Name: nodeName}, node)
			if err == nil {
				hasTaint := hasStartupTaint(node)
				if hasTaint != tt.expectTaint {
					t.Errorf("Node Taint State mismatch. Got Taint: %v, Expected: %v", hasTaint, tt.expectTaint)
				}
				// make sure we didn't wipe other taints
				if !hasOtherTaint(node) {
					t.Errorf("Reconcile wiped out unrelated taints. Expected 'other-taint' to persist.")
				}
			}
		})
	}
}

func hasOtherTaint(node *corev1.Node) bool {
	for _, t := range node.Spec.Taints {
		if t.Key == "other-taint" {
			return true
		}
	}
	return false
}
