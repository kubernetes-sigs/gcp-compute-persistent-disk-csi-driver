package tenancy

import (
	"fmt"
	"sync"
	"time"

	computev1 "google.golang.org/api/compute/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// TenantsInformer is an interface that wraps a cache.SharedIndexInformer
// and watches tenancy.gke.io/tenants objects.
type TenantsInformer interface {
	AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error)
	Run(<-chan struct{})
	HasSynced() bool
}

// TenantLifecycleHandler defines callbacks for tenant lifecycle events.
// TenantMetadata should be the struct returned by GetMetadataFromTenantCR.
type TenantLifecycleHandler struct {
	AddFunc    func(tenantMeta *Metadata, zone string) (*computev1.Service, error)
	DeleteFunc func(tenantMeta *Metadata)
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
func NewTenantsInformer(isMultiTenantCluster bool, kubeConfig *rest.Config) (TenantsInformer, error) {
	if !isMultiTenantCluster {
		return NewNoopTenantsInformer(), nil
	}

	dynamicClient, err := newDynamicClientForConfig(kubeConfig)
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

func RegisterTenantEventHandlers(ti TenantsInformer, handler TenantLifecycleHandler, zone string, tenantServiceMap map[string]*computev1.Service, mutex *sync.Mutex) error {
	if ti == nil {
		return fmt.Errorf("TenantsInformer cannot be nil")
	}

	_, err := ti.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			klog.Infof("Tenant CR created: %v", obj)
			tenantMeta, err := GetMetadataFromTenantCR(obj)
			if err != nil {
				klog.Errorf("Error extracting tenant metadata from CR: %v", err)
				return
			}

			mutex.Lock()
			defer mutex.Unlock()
			if _, ok := tenantServiceMap[tenantMeta.ProjectNumber]; ok {
				klog.Infof("Tenant GCE client already exists for tenant project number %s, skipping GCE client instantiation.", tenantMeta.ProjectNumber)
				mutex.Unlock()
				return
			}

			svc, err := handler.AddFunc(&tenantMeta, zone)
			if err != nil {
				klog.Errorf("Error in AddFunc callback for tenant %s (project %s): %v", tenantMeta.TenantName, tenantMeta.ProjectNumber, err)
				return
			}

			if svc != nil {
				tenantServiceMap[tenantMeta.ProjectNumber] = svc
				klog.Infof("Successfully processed AddFunc for tenant %s (project %s) and updated service map.", tenantMeta.TenantName, tenantMeta.ProjectNumber)
			}
		},
		DeleteFunc: func(obj any) {
			klog.Infof("Tenant CR deleted: %v", obj)
			tenantMeta, err := GetMetadataFromTenantCR(obj)
			if err != nil {
				klog.Errorf("Error while extracting tenant metadata on delete: %v", err)
				return
			}

			mutex.Lock()
			defer mutex.Unlock()
			if _, ok := tenantServiceMap[tenantMeta.ProjectNumber]; ok {
				delete(tenantServiceMap, tenantMeta.ProjectNumber)
				klog.Infof("Deleted GCE client for tenant project number %s from map.", tenantMeta.ProjectNumber)
			} else {
				klog.Warningf("Attempted to delete GCE client for tenant project %s, but it was not found in the map.", tenantMeta.ProjectNumber)
			}

			if handler.DeleteFunc != nil {
				handler.DeleteFunc(&tenantMeta)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler to tenant informer: %w", err)
	}
	return nil
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
