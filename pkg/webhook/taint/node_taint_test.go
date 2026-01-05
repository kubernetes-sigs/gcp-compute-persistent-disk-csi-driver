package taint

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestNodeTainter_Handle(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	decoder := admission.NewDecoder(scheme)

	tests := []struct {
		name          string
		node          *corev1.Node
		wantAllowed   bool
		wantPatches   []jsonpatch.JsonPatchOperation
		wantResultMsg string
	}{
		{
			name: "Should add taint to a clean node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "clean-node"},
				Spec:       corev1.NodeSpec{Taints: []corev1.Taint{}},
			},
			wantAllowed: true,
			wantPatches: []jsonpatch.JsonPatchOperation{
				{
					Operation: "add",
					Path:      "/spec/taints",
					Value: []interface{}{
						map[string]interface{}{
							"effect": string(StartupTaintEffect),
							"key":    StartupTaintKey,
							"value":  StartupTaintValue,
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
						{Key: StartupTaintKey, Value: StartupTaintValue, Effect: StartupTaintEffect},
					},
				},
			},
			wantAllowed:   true,
			wantPatches:   []jsonpatch.JsonPatchOperation{},
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
			wantPatches:   []jsonpatch.JsonPatchOperation{},
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
			wantPatches: []jsonpatch.JsonPatchOperation{
				{
					Operation: "add",
					Path:      "/spec/taints/1",
					Value: map[string]interface{}{
						"effect": string(StartupTaintEffect),
						"key":    StartupTaintKey,
						"value":  StartupTaintValue,
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
				if len(resp.Patches) != len(tc.wantPatches) {
					t.Fatalf("Got %d patches, want %d", len(resp.Patches), len(tc.wantPatches))
				}
				for i, patch := range resp.Patches {
					if diff := cmp.Diff(tc.wantPatches[i], patch); diff != "" {
						t.Errorf("Patch[%d] mismatch (-want +got):\n%s", i, diff)
					}
				}
			} else {
				if len(resp.Patches) > 0 {
					t.Errorf("Expected NO patches, got %v", resp.Patches)
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
