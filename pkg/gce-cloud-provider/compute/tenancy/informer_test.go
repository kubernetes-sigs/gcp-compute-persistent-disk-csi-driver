package tenancy

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

func TestNewTenantsInformer_MultiTenantCluster(t *testing.T) {
	originalNewDynamicClientForConfig := newDynamicClientForConfig
	t.Cleanup(func() {
		newDynamicClientForConfig = originalNewDynamicClientForConfig
	})
	newDynamicClientForConfig = func(*rest.Config) (dynamic.Interface, error) {
		return dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), nil
	}
	informer, err := NewTenantsInformer(true)
	if err != nil {
		t.Fatalf("NewTenantsInformer(true, &rest.Config{}, 1h) failed: %v", err)
	}
	if _, ok := informer.(cache.SharedIndexInformer); !ok {
		t.Errorf("NewTenantsInformer expected to return informer of type cache.SharedIndexInformer for multi-tenant clusters")
	}
}
func TestNewTenantsInformer_SingleTenantCluster(t *testing.T) {
	originalNewDynamicClientForConfig := newDynamicClientForConfig
	t.Cleanup(func() {
		newDynamicClientForConfig = originalNewDynamicClientForConfig
	})
	newDynamicClientForConfig = func(*rest.Config) (dynamic.Interface, error) {
		return dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), nil
	}
	informer, err := NewTenantsInformer(false)
	if err != nil {
		t.Fatalf("NewTenantsInformer(false, &rest.Config{}, 1h) failed: %v", err)
	}
	if _, ok := informer.(*noopTenantsInformer); !ok {
		t.Errorf("NewTenantsInformer expected to return informer of type *noopTenantsInformer for single-tenant clusters")
	}
}
