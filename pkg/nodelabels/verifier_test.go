package nodelabels

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

const fakeDiskType = "fake-disk-type"

// Tests the allNodesHaveDiskSupportLabel function
func TestAllNodesHaveDiskSupportLabel(t *testing.T) {
	tests := []struct {
		name     string
		nodes    []*corev1.Node
		expected bool
	}{
		{
			name:     "empty node list",
			nodes:    []*corev1.Node{},
			expected: false,
		},
		{
			name: "all nodes have disk support label",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							common.TopologyLabelKey(fakeDiskType): "us-central1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							common.TopologyLabelKey(fakeDiskType): "us-west1",
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "some nodes missing disk support label",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							common.TopologyLabelKey(fakeDiskType): "us-central1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"some-other-label": "value",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "only zone labels are not sufficient",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							common.TopologyKeyZone: "us-central1-a",
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := allNodesHaveDiskSupportLabel(tc.nodes)
			if result != tc.expected {
				t.Errorf("allNodesHaveDiskSupportLabel() = %v, want %v", result, tc.expected)
			}
		})
	}
}
