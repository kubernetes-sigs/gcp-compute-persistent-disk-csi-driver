package nodelabeler

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ConfigMapEventHandler struct {
	Client     client.Client
	Reconciler *Reconciler
}

func (h *ConfigMapEventHandler) Map(ctx context.Context, obj client.Object) []reconcile.Request {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		klog.Errorf("Expected a ConfigMap object but got %T", obj)
		return nil
	}
	if err := h.Reconciler.UpdateMachinePDCompatibility(ctx); err != nil {
		klog.Errorf("Failed to update disk compatibility map from ConfigMap %s/%s: %v", cm.Namespace, cm.Name, err)
		return nil
	}

	var nodes corev1.NodeList
	if err := h.Client.List(ctx, &nodes); err != nil {
		klog.Errorf("Failed to list nodes for configmap change reconciliation: %v", err)
		return nil
	}
	requests := make([]reconcile.Request, len(nodes.Items))
	for i, node := range nodes.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: node.Name,
			},
		}
	}
	klog.Infof("ConfigMap changed, enqueuing reconciliation for %d nodes", len(requests))
	return requests
}
