package tenancy

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// TenantsInformer is an interface that wraps a cache.SharedIndexInformer
// and watches tenancy.gke.io/tenants objects.
type TenantsInformer interface {
	AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error)
	Run(<-chan struct{})
	HasSynced() bool
}

// newDynamicClientForConfig creates a new dynamic client for the given
// configuration.
//
// The value of newDynamicClientForConfig must only be overwritten in tests
var newDynamicClientForConfig = func(inConfig *rest.Config) (dynamic.Interface, error) {
	return dynamic.NewForConfig(inConfig)
}
var defaultResyncPeriod = 10 * time.Minute

// NewTenantsInformer creates a new TenantsInformer that watches
// tenancy.gke.io/tenants objects.
//
// After creating a new TenantsInformer, you must call Run() to start it.
func NewTenantsInformer(isMultiTenantCluster bool) (TenantsInformer, error) {
	if !isMultiTenantCluster {
		return NewNoopTenantsInformer(), nil
	}

	kc := GetKubeConfig()
	dynamicClient, err := newDynamicClientForConfig(kc)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client for CRD: %w", err)
	}
	gvr := schema.GroupVersionResource{
		Group:    "tenancy.gke.io",
		Version:  "v1",
		Resource: "tenants",
	}
	dynamicFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, defaultResyncPeriod)
	return dynamicFactory.ForResource(gvr).Informer(), nil
}

// noopTenantsInformer is a TenantsInformer that does nothing and
// always returns true for HasSynced.
// This should only be used when GKE multi-tenancy is disabled or in tests.
type noopTenantsInformer struct{}

func NewNoopTenantsInformer() TenantsInformer {
	return &noopTenantsInformer{}
}

// HasSynced always returns true.
func (*noopTenantsInformer) HasSynced() bool {
	return true
}

// AddEventHandler always returns nil and a nil error.
func (*noopTenantsInformer) AddEventHandler(_ cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

// Run does nothing.
func (*noopTenantsInformer) Run(_ <-chan struct{}) {}
