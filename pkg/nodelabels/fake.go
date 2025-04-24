package nodelabels

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func NewFakeVerifier(t *testing.T, nodes []*corev1.Node) *Verifier {
	ctx := context.Background()
	clientset := fakeKubeClient(nodes)

	verifier, err := NewVerifier(ctx, clientset)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}

	return verifier
}

func fakeKubeClient(nodes []*corev1.Node) kubernetes.Interface {
	// Convert the list of nodes to a slice of runtime.Object
	var objects []runtime.Object
	for _, node := range nodes {
		objects = append(objects, node)
	}

	// Create a fake clientset with the predefined objects
	clientset := fake.NewSimpleClientset(objects...)

	return clientset
}
