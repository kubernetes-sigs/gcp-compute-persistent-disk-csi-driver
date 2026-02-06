package taint

import (
	"context"
	"encoding/json"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Define the Taint details
const (
	StartupTaintKey    = "pd.csi.storage.gke.io/startup"
	StartupTaintValue  = "true"
	StartupTaintEffect = corev1.TaintEffectNoSchedule

	// If this label is present on the Node with value "true", the taint will not be applied
	DisableTaintLabelKey   = "pd.csi.storage.gke.io/disable-startup-taint"
	DisableTaintLabelValue = "true"
)

// NodeTainter appends a specific taint to Nodes on creation
type NodeTainter struct {
	Client  client.Client
	decoder admission.Decoder
}

// Handle serves as the admission logic
func (a *NodeTainter) Handle(ctx context.Context, req admission.Request) admission.Response {
	node := &corev1.Node{}
	err := a.decoder.Decode(req, node)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Failsafe to skip tainting if the DisableTaintLabelKey=true
	if val, ok := node.Labels[DisableTaintLabelKey]; ok && val == DisableTaintLabelValue {
		return admission.Allowed("Fail-safe label detected: skipping startup taint")
	}

	if hasStartupTaint(node) {
		return admission.Allowed("Node already has startup taint")
	}

	// Apply taint
	node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
		Key:    StartupTaintKey,
		Value:  StartupTaintValue,
		Effect: StartupTaintEffect,
	})
	marshaledNode, err := json.Marshal(node)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledNode)
}

// InjectDecoder injects the decoder (required by controller-runtime)
func (a *NodeTainter) InjectDecoder(d *admission.Decoder) error {
	a.decoder = *d
	return nil
}

func hasStartupTaint(node *corev1.Node) bool {
	for _, t := range node.Spec.Taints {
		if t.Key == StartupTaintKey && t.Effect == StartupTaintEffect {
			return true
		}
	}
	return false
}
