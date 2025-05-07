package tenancy

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
)

const (
	// TenantProjectLabelKey is the label key for the tenant project number.
	// All K8s objects that belong to a tenant must have this label.
	// The value of this label is the tenant project number.
	// Format: `t<tenant_project_number>`
	TenantProjectLabelKey = "tenancy.gke.io/project"

	// TenantNameLabelKey is the label key for the tenant name.
	// All K8s objects that belong to a tenant must have this label.
	// The value of this label is the tenant name.
	TenantNameLabelKey = "tenancy.gke.io/tenant"
)

// MetadataResolver is a helper class that resolves multitenancy-related
// metadata of K8s objects, such as the project number that an object belongs
// to.
type MetadataResolver struct {
	// isMultiTenant indicates whether the cluster is running in multi-tenant
	// mode.
	isMultiTenant bool
	// customerProjectNumber is:
	//    - If running in multi-tenant mode, the cluster supervisor project.
	//    - If running in single-tenant mode, the customer project number.
	customerProjectNumber string

	// Name of the cluster.
	clusterName string
}

// Metadata contains tenant metadata about K8s objects.
// If a cluster doesn't belong to a tenant (e.g., if the cluster is
// not multi-tenant), the metadata defaults to a single-tenant metadata.
type Metadata struct {
	// Project number that a K8s object belongs to. If the object doesn't belong
	// to a tenant, this will be the cluster project number.
	ProjectNumber string
	// Name of a tenant that a k8s object belongs to. If the object doesn't belong
	// to a tenant, this will be the cluster name.
	TenantName string
}

// NewMetadataResolver creates a new MetadataResolver.
//
// customerProjectNumber is:
//   - If running in multi-tenant mode, the cluster supervisor project.
//   - If running in single-tenant mode, the customer project number.
func NewMetadataResolver(isMultiTenant bool, customerProjectNumber, clusterName string) *MetadataResolver {
	return &MetadataResolver{
		isMultiTenant:         isMultiTenant,
		customerProjectNumber: customerProjectNumber,
		clusterName:           clusterName,
	}
}

// getTenantProjectNumberFromLabels returns the tenant project number from the
// given K8s object labels or returns an empty string if the label is invalid
// or the object is not bound to a tenant. The function also returns a boolean
// to indicate whether the tenant project number was extracted.
func (m *MetadataResolver) getTenantProjectNumberFromLabels(labels map[string]string) (string, bool) {
	tenantProjectLabelValue, ok := labels[TenantProjectLabelKey]
	if !ok {
		return "", false
	}
	// Check if the tenant project label value is valid.
	if len(tenantProjectLabelValue) == 0 || tenantProjectLabelValue[0] != 't' {
		return "", false
	}
	return tenantProjectLabelValue[1:], true
}

// getTenantNameFromLabels returns the tenant name from the given K8s
// object labels or returns an empty string if the object is not bound to a
// tenant (label is missing). The function also returns a boolean to indicate
// whether the tenant name was extracted.
func (m *MetadataResolver) getTenantNameFromLabels(labels map[string]string) (string, bool) {
	// In multi-tenancy mode, all k8s objects that belong to tenants must have
	// this label. If it doesn't exist, the object is not bound to a tenant.
	tenantProjectLabelValue, ok := labels[TenantNameLabelKey]
	return tenantProjectLabelValue, ok
}

// GetMetadataFromLabels retrieves tenancy metadata information from a K8s
// object labels. If tenancy labels are not present, it defaults to the cluster
// project metadata. If tenancy labels are invalid, it returns an empty metadat
// object.
func (m *MetadataResolver) GetMetadataFromLabels(labels map[string]string) Metadata {
	if !m.isMultiTenant {
		return Metadata{
			ProjectNumber: m.customerProjectNumber,
			TenantName:    m.clusterName,
		}
	}
	tenantName, ok := m.getTenantNameFromLabels(labels)
	if !ok {
		return Metadata{
			ProjectNumber: m.customerProjectNumber,
			TenantName:    m.clusterName,
		}
	}
	projectNumber, ok := m.getTenantProjectNumberFromLabels(labels)
	if !ok {
		// This should never happen, given the guaranteed made by MT, but just
		// in case, print a warning and return an empty metadata object.
		// TODO: b/409994644 - Return an error and drop the object instead of
		// sending it to UAS using the wrong site ID.
		klog.Warningf("Failed to extract project number for tenant %s from labels %v", tenantName, labels)
		return Metadata{
			ProjectNumber: "",
			TenantName:    "",
		}
	}
	return Metadata{
		ProjectNumber: projectNumber,
		TenantName:    tenantName,
	}
}

func GetMetadataFromTenantCR(obj interface{}) (Metadata, error) {
	unstructuredObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return Metadata{}, fmt.Errorf("failed to cast %v of type %T to a unstructured.Unstructured type", obj, obj)
	}
	spec, ok := unstructuredObj.Object["spec"].(map[string]interface{})
	if !ok {
		return Metadata{}, fmt.Errorf("failed to cast spec %v of type %T to map[string]interface", unstructuredObj.Object["spec"], unstructuredObj.Object["spec"])
	}
	projectNumber, ok := spec["projectNumber"].(int64)
	if !ok {
		return Metadata{}, fmt.Errorf("failed to cast spec.projectNumber of type %T to int64", spec["project_number"])
	}
	return Metadata{
		ProjectNumber: fmt.Sprintf("%d", projectNumber),
		TenantName:    unstructuredObj.GetName(),
	}, nil
}

func (m *Metadata) Equal(c Metadata) bool {
	if m.ProjectNumber != c.ProjectNumber {
		return false
	}
	if m.TenantName != c.TenantName {
		return false
	}
	return true
}
