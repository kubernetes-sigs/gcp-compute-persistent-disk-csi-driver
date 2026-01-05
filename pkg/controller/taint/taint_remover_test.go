package taint

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
)

func TestReconcileNodeTaint_Reconcile(t *testing.T) {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = storagev1.AddToScheme(s)

	nodeName := "test-node"

	// Helper function to dynamically create nodes with specific ages and OS labels
	newTaintedNode := func(age time.Duration, os string) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:              nodeName,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
				Labels:            map[string]string{"kubernetes.io/os": os},
			},
			Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					{Key: constants.StartupTaintKey, Effect: constants.StartupTaintEffect, Value: constants.StartupTaintValue},
					{Key: "other-taint", Effect: corev1.TaintEffectNoSchedule},
				},
			},
		}
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
		expectRequeue bool
	}{
		{
			name:          "Bootstrap Race: Node is new, CSINode missing -> Requeue and wait",
			existingObjs:  []client.Object{newTaintedNode(1*time.Minute, "linux")},
			req:           reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}},
			expectTaint:   true, // Taint stays
			expectedError: false,
			expectRequeue: true, // Must requeue to wait for driver
		},
		{
			name:          "Orphan Cleanup: Node is old, CSINode still missing -> Remove Taint",
			existingObjs:  []client.Object{newTaintedNode(20*time.Minute, "linux")}, // Over 10m limit
			req:           reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}},
			expectTaint:   false, // Orphan cleanup rips it off
			expectedError: false,
			expectRequeue: false,
		},
		{
			name:          "CSINode exists, Driver NOT registered -> Requeue and wait",
			existingObjs:  []client.Object{newTaintedNode(10*time.Minute, "linux"), csiNodeWithoutDriver},
			req:           reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}},
			expectTaint:   true,
			expectedError: false,
			expectRequeue: true, // Must requeue to wait for our specific driver
		},
		{
			name:          "CSINode exists, Driver Registered -> Remove Taint",
			existingObjs:  []client.Object{newTaintedNode(10*time.Minute, "linux"), csiNodeWithDriver},
			req:           reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}},
			expectTaint:   false,
			expectedError: false,
			expectRequeue: false,
		},
		{
			name:          "Node already untainted (Idempotent)",
			existingObjs:  []client.Object{untaintedNode, csiNodeWithDriver},
			req:           reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}},
			expectTaint:   false,
			expectedError: false,
			expectRequeue: false,
		},
		{
			name:          "Node Not Found (Defensive check)",
			existingObjs:  []client.Object{csiNodeWithDriver},
			req:           reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}},
			expectTaint:   false, // Irrelevant check, but shouldn't crash
			expectedError: false,
			expectRequeue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(tt.existingObjs...)
			cl := builder.Build()

			r := &ReconcileNodeTaint{client: cl}
			res, err := r.Reconcile(context.TODO(), tt.req)

			// 1. Check Errors
			if (err != nil) != tt.expectedError {
				t.Errorf("Reconcile() error = %v, expectedError %v", err, tt.expectedError)
				return
			}

			// 2. Check Requeue Logic
			if (res.Requeue || res.RequeueAfter > 0) != tt.expectRequeue {
				t.Errorf("Expected Requeue: %v, got RequeueAfter: %v", tt.expectRequeue, res.RequeueAfter)
			}

			// 3. Check Node Taint State
			node := &corev1.Node{}
			err = cl.Get(context.TODO(), types.NamespacedName{Name: nodeName}, node)
			if err == nil {
				hasTaint := hasStartupTaint(node)
				if hasTaint != tt.expectTaint {
					t.Errorf("Node Taint State mismatch. Got Taint: %v, Expected: %v", hasTaint, tt.expectTaint)
				}
				// Make sure we didn't wipe other taints
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
