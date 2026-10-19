package taint

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
)

func TestNodeTainter_Handle(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	decoder := admission.NewDecoder(scheme)

	tests := []struct {
		name          string
		node          *corev1.Node
		wantAllowed   bool
		wantPatches   []map[string]interface{}
		wantResultMsg string
	}{
		{
			name: "Should add taint to a clean node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "clean-node"},
				Spec:       corev1.NodeSpec{Taints: []corev1.Taint{}},
			},
			wantAllowed: true,
			wantPatches: []map[string]interface{}{
				{
					"op":   "add",
					"path": "/spec/taints",
					"value": []interface{}{
						map[string]interface{}{
							"effect": string(constants.StartupTaintEffect),
							"key":    constants.StartupTaintKey,
							"value":  constants.StartupTaintValue,
						},
					},
				},
			},
		},
		{
			name: "Should ignore node if taint already exists (Idempotency)",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "tainted-node"},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{Key: constants.StartupTaintKey, Value: constants.StartupTaintValue, Effect: constants.StartupTaintEffect},
					},
				},
			},
			wantAllowed:   true,
			wantPatches:   []map[string]interface{}{},
			wantResultMsg: "Node already has startup taint",
		},
		{
			name: "Should ignore node if Fail-Safe label is present",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "disabled-node",
					Labels: map[string]string{
						DisableTaintLabelKey: DisableTaintLabelValue,
					},
				},
				Spec: corev1.NodeSpec{Taints: []corev1.Taint{}},
			},
			wantAllowed:   true,
			wantPatches:   []map[string]interface{}{},
			wantResultMsg: "Fail-safe label detected",
		},
		{
			name: "Should preserve existing unrelated taints and append ours",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "busy-node"},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{Key: "other-taint", Effect: corev1.TaintEffectNoSchedule},
					},
				},
			},
			wantAllowed: true,
			wantPatches: []map[string]interface{}{
				{
					"op":   "add",
					"path": "/spec/taints/-",
					"value": map[string]interface{}{
						"effect": string(constants.StartupTaintEffect),
						"key":    constants.StartupTaintKey,
						"value":  constants.StartupTaintValue,
					},
				},
			},
		},
		{
			name: "Should append to node with multiple existing taints",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "multi-taint-node"},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{Key: "taint-1", Effect: corev1.TaintEffectNoSchedule},
						{Key: "taint-2", Effect: corev1.TaintEffectNoExecute},
						{Key: "taint-3", Effect: corev1.TaintEffectPreferNoSchedule},
					},
				},
			},
			wantAllowed: true,
			wantPatches: []map[string]interface{}{
				{
					"op":   "add",
					"path": "/spec/taints/-",
					"value": map[string]interface{}{
						"effect": string(constants.StartupTaintEffect),
						"key":    constants.StartupTaintKey,
						"value":  constants.StartupTaintValue,
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rawNode, err := json.Marshal(tc.node)
			if err != nil {
				t.Fatalf("Failed to marshal node: %v", err)
			}

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Object: runtime.RawExtension{Raw: rawNode},
				},
			}

			tainter := &NodeTainter{}
			tainter.InjectDecoder(&decoder)

			resp := tainter.Handle(context.Background(), req)

			if resp.Allowed != tc.wantAllowed {
				t.Errorf("Allowed = %v, want %v", resp.Allowed, tc.wantAllowed)
			}

			if tc.wantResultMsg != "" && resp.Result != nil {
				msg := resp.Result.Message
				reason := string(resp.Result.Reason)

				// Check if either the Message OR the Reason contains the expected text
				if !strings.Contains(msg, tc.wantResultMsg) && !strings.Contains(reason, tc.wantResultMsg) {
					t.Errorf("Result mismatch. Got Message='%s' Reason='%s'. Expected to contain: '%s'",
						msg, reason, tc.wantResultMsg)
				}
			}

			if len(tc.wantPatches) > 0 {
				var gotPatches []map[string]interface{}
				if len(resp.Patch) > 0 {
					if err := json.Unmarshal(resp.Patch, &gotPatches); err != nil {
						t.Fatalf("Failed to unmarshal patch response: %v", err)
					}
				}
				if len(gotPatches) != len(tc.wantPatches) {
					t.Fatalf("Got %d patches, want %d", len(gotPatches), len(tc.wantPatches))
				}
				for i, patch := range gotPatches {
					if diff := cmp.Diff(tc.wantPatches[i], patch); diff != "" {
						t.Errorf("Patch[%d] mismatch (-want +got):\n%s", i, diff)
					}
				}
			} else {
				if len(resp.Patch) > 0 && string(resp.Patch) != "null" && string(resp.Patch) != "[]" {
					t.Errorf("Expected NO patches, got %s", string(resp.Patch))
				}
			}
		})
	}
}

func TestNodeTainter_Handle_DecodeError(t *testing.T) {
	tainter := &NodeTainter{}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	decoder := admission.NewDecoder(scheme)
	tainter.InjectDecoder(&decoder)

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Object: runtime.RawExtension{Raw: []byte(`INVALID JSON`)},
		},
	}

	resp := tainter.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("Expected allowed=false for decode error, but got allowed=true")
	}

	if resp.Result == nil {
		t.Fatal("Expected resp.Result to be populated, got nil")
	}

	if resp.Result.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.Result.Code)
	}
}
