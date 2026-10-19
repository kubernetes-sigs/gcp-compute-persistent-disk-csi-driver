package taint

import (
	"context"
	"encoding/json"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
)

// Define the Taint details
const (

	// If this label is present on the Node with value "true", the taint will not be applied
	DisableTaintLabelKey   = "pd.csi.storage.gke.io/disable-startup-taint"
	DisableTaintLabelValue = "true"
)

// NodeTainter appends a specific taint to Nodes on creation
type NodeTainter struct {
	decoder admission.Decoder
}

// Handle serves as the admission logic
func (a *NodeTainter) Handle(ctx context.Context, req admission.Request) admission.Response {
	node := &corev1.Node{}
	err := a.decoder.Decode(req, node)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Sanity check, make sure labels is not nil
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	// Failsafe to skip tainting if the DisableTaintLabelKey=true
	if val, ok := node.Labels[DisableTaintLabelKey]; ok && val == DisableTaintLabelValue {
		return admission.Allowed("Fail-safe label detected: skipping startup taint")
	}

	if hasStartupTaint(node) {
		return admission.Allowed("Node already has startup taint")
	}

	// Apply taint via explicit JSON patch to avoid dropping unknown fields.
	taintObj := corev1.Taint{
		Key:    constants.StartupTaintKey,
		Value:  constants.StartupTaintValue,
		Effect: constants.StartupTaintEffect,
	}
	patches := buildTaintPatches(node.Spec.Taints, taintObj)

	return createPatchResponse(patches, "Applying startup taint")
}

// buildTaintPatches generates the JSON patch operations for adding the taint
func buildTaintPatches(existingTaints []corev1.Taint, taintObj corev1.Taint) []map[string]interface{} {
	hasExistingTaints := len(existingTaints) > 0
	var patches []map[string]interface{}

	if hasExistingTaints {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/taints/-",
			"value": taintObj,
		})
	} else {
		// Initialize null taint array.
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/taints",
			"value": []corev1.Taint{taintObj},
		})
	}

	return patches
}

// createPatchResponse constructs a patch-based admission response.
func createPatchResponse(patches []map[string]interface{}, message string) admission.Response {
	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	patchType := admissionv1.PatchTypeJSONPatch

	return admission.Response{
		AdmissionResponse: admissionv1.AdmissionResponse{
			Allowed:   true,
			Patch:     patchBytes,
			PatchType: &patchType,
			Result: &metav1.Status{
				Code:    http.StatusOK,
				Message: message,
			},
		},
	}
}

// InjectDecoder injects the decoder (required by controller-runtime)
func (a *NodeTainter) InjectDecoder(d *admission.Decoder) error {
	a.decoder = *d
	return nil
}

func hasStartupTaint(node *corev1.Node) bool {
	for _, t := range node.Spec.Taints {
		if t.Key == constants.StartupTaintKey && t.Effect == constants.StartupTaintEffect && t.Value == constants.StartupTaintValue {
			return true
		}
	}
	return false
}
