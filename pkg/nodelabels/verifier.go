package nodelabels

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
)

// resyncInterval is the interval at which the informer resyncs.  Should the
// informer cache miss an event, the resync will correct the cache.
const resyncInterval = 10 * time.Minute

// Verifier checks node labels for GKE topology labels.
type Verifier struct {
	nodeLister listers.NodeLister
}

// NewVerifier creates an informer for listing nodes and returns a
// lister.  It waits for the informer to sync and connects the factory's stop
// channel to the context's done channel to prevent goroutine leaks when the
// binary is stopped.
func NewVerifier(ctx context.Context, clientset kubernetes.Interface) (*Verifier, error) {
	stopCh := make(chan struct{})
	// Should the context be cancelled upstream, we want to stop the informer.
	// Documentation suggests that this prevents goroutine leaks.
	go func() {
		<-ctx.Done()
		close(stopCh)
	}()

	factory := informers.NewSharedInformerFactory(clientset, resyncInterval)
	nodeInformer := factory.Core().V1().Nodes().Informer()
	nodeLister := factory.Core().V1().Nodes().Lister()

	factory.Start(stopCh)
	synced := cache.WaitForCacheSync(stopCh, nodeInformer.HasSynced)
	if !synced {
		return nil, fmt.Errorf("failed to sync node informer")
	}

	return &Verifier{
		nodeLister: nodeLister,
	}, nil
}

func (v *Verifier) AllNodesHaveDiskSupportLabel() (bool, error) {
	nodes, err := v.nodeLister.List(labels.Everything())
	if err != nil {
		return false, fmt.Errorf("failed to list nodes: %v", err)
	}
	return allNodesHaveDiskSupportLabel(nodes), nil
}

func allNodesHaveDiskSupportLabel(nodes []*corev1.Node) bool {
	if len(nodes) == 0 {
		return false
	}

	for _, node := range nodes {
		if !nodeHasDiskSupportLabel(node) {
			klog.V(4).Infof("Node %s does not have disk support label", node.GetName())
			return false
		}
	}
	return true
}

func nodeHasDiskSupportLabel(node *corev1.Node) bool {
	labels := node.GetLabels()
	for key, _ := range labels {
		if isNonZoneGKETopologyLabel(key) {
			return true
		}
	}
	return false
}

func isNonZoneGKETopologyLabel(key string) bool {
	return common.IsGKETopologyLabel(key) && key != common.TopologyKeyZone
}
