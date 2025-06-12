package tenancy

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func TestGetProjectNumberFromLabels(t *testing.T) {
	testCases := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{
			name:     "Valid tenant project label: returns tenant project number",
			labels:   map[string]string{TenantProjectLabelKey: "t987987987"},
			expected: "987987987",
		},
		{
			name:     "Invalid tenant project label: returns empty string",
			labels:   map[string]string{TenantProjectLabelKey: "1234567890"},
			expected: "",
		},
		{
			name:     "No tenant project label: returns empty string",
			labels:   map[string]string{"foo": "bar"},
			expected: "",
		},
		{
			name:     "Empty labels: returns empty string",
			labels:   map[string]string{},
			expected: "",
		},
		{
			name:     "Nil labels: returns empty string",
			labels:   nil,
			expected: "",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pr := NewMetadataResolver(true, "project-number", "cluster-name")
			got, _ := pr.getTenantProjectNumberFromLabels(tc.labels)
			if got != tc.expected {
				t.Errorf("GetProjectNumberFromLabels(%v): %s, want %s", tc.labels, got, tc.expected)
			}
		})
	}
}

func TestGetMetadataFromLabels(t *testing.T) {
	testCases := []struct {
		name          string
		isMultiTenant bool
		labels        map[string]string
		want          Metadata
	}{
		{
			name:          "Valid tenant name and project labels in multi-tenant mode: returns tenant project number",
			isMultiTenant: true,
			labels: map[string]string{
				TenantNameLabelKey:    "tenant-name",
				TenantProjectLabelKey: "t123456798",
			},
			want: Metadata{
				TenantName:    "tenant-name",
				ProjectNumber: "123456798",
			},
		},
		{
			name:          "No tenant name in multi-tenant mode: defaults to cluster info",
			isMultiTenant: true,
			labels: map[string]string{
				TenantProjectLabelKey: "t123456798",
			},
			want: Metadata{
				TenantName:    "cluster-name",
				ProjectNumber: "123456789",
			},
		},
		{
			name:          "No tenant project label in multi-tenant mode: defaults to empty info",
			isMultiTenant: true,
			labels: map[string]string{
				TenantNameLabelKey: "tenant-name",
			},
			want: Metadata{
				TenantName:    "",
				ProjectNumber: "",
			},
		},
		{
			name:          "Empty labels in multi-tenant mode: defaults to cluster info",
			isMultiTenant: true,
			labels:        map[string]string{},
			want: Metadata{
				TenantName:    "cluster-name",
				ProjectNumber: "123456789",
			},
		},
		{
			name:          "Nil labels in multi-tenant mode: defaults to cluster info",
			labels:        nil,
			isMultiTenant: true,
			want: Metadata{
				TenantName:    "cluster-name",
				ProjectNumber: "123456789",
			},
		},
		{
			name:          "Valid tenant name and project labels in single-tenant mode: defaults to cluster info",
			isMultiTenant: false,
			labels: map[string]string{
				TenantNameLabelKey:    "tenant-name",
				TenantProjectLabelKey: "t123456798",
			},
			want: Metadata{
				TenantName:    "cluster-name",
				ProjectNumber: "123456789",
			},
		},
		{
			name:          "Empty labels in single-tenant mode: defaults to cluster info",
			isMultiTenant: false,
			labels:        map[string]string{},
			want: Metadata{
				TenantName:    "cluster-name",
				ProjectNumber: "123456789",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pr := NewMetadataResolver(tc.isMultiTenant, "123456789", "cluster-name")
			got := pr.GetMetadataFromLabels(tc.labels)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("GetMetadataFromLabels(%v) returned diff (-want/+got):\n%s", tc.labels, diff)
			}
		})
	}
}

func TestGetMetadataFromTenantCR(t *testing.T) {
	tenantCRYaml := `
apiVersion: tenancy.gke.io/v1
kind: Tenant
metadata:
  name: t123123456456-default
spec:
  projectNumber: 123123456456
`
	var tenantCR unstructured.Unstructured
	err := yaml.Unmarshal([]byte(tenantCRYaml), &tenantCR)
	if err != nil {
		t.Fatalf("Failed to convert %s to unstructured.Unstructured: %v", tenantCRYaml, err)
	}

	got, err := GetMetadataFromTenantCR(&tenantCR)
	if err != nil {
		t.Fatalf("GetMetadataFromTenantCR(%v) failed: %v", tenantCR, err)
	}
	want := Metadata{
		TenantName:    "t123123456456-default",
		ProjectNumber: "123123456456",
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("GetMetadataFromTenantCR(%v) returned diff (-want/+got):\n%s", tenantCR, diff)
	}
}

func TestGetMetadataFromTenantCR_Errors(t *testing.T) {
	testCases := []struct {
		desc     string
		tenantCR interface{}
	}{
		{
			desc:     "Not a *unstructured.Unstructured object type",
			tenantCR: "wrong-type",
		},
		{
			desc:     "spec missing from object",
			tenantCR: &unstructured.Unstructured{Object: map[string]interface{}{}},
		},
		{
			desc: "spec type is not map[string]interface{}",
			tenantCR: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": "wrong-type",
				},
			},
		},
		{
			desc: "spec.projectNumber missing from object",
			tenantCR: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{},
				},
			},
		},
		{
			desc: "spec.projectNumber is not an int64",
			tenantCR: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"projectNumber": "wrong-type",
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := GetMetadataFromTenantCR(tc.tenantCR)
			if err == nil {
				t.Fatalf("GetMetadataFromTenantCR(%v) = nil, want err", tc.tenantCR)
			}
		})
	}
}

func TestMetadataIsEqual(t *testing.T) {
	testCases := []struct {
		desc     string
		base     Metadata
		expected Metadata
		want     bool
	}{
		{
			desc: "OK: all fields are equal",
			want: true,
			base: Metadata{
				ProjectNumber: "1234",
				TenantName:    "tenant-name",
			},
			expected: Metadata{
				ProjectNumber: "1234",
				TenantName:    "tenant-name",
			},
		},
		{
			desc: "OK: only project number set",
			want: true,
			base: Metadata{
				ProjectNumber: "1234",
			},
			expected: Metadata{
				ProjectNumber: "1234",
			},
		},
		{
			desc: "OK: only tenant name set",
			want: true,
			base: Metadata{
				TenantName: "tenant-name",
			},
			expected: Metadata{
				TenantName: "tenant-name",
			},
		},
		{
			desc:     "OK: empty",
			want:     true,
			base:     Metadata{},
			expected: Metadata{},
		},
		{
			desc: "Different project numbers",
			want: false,
			base: Metadata{
				ProjectNumber: "1234",
			},
			expected: Metadata{
				ProjectNumber: "5678",
			},
		},
		{
			desc: "Different tenant names",
			want: false,
			base: Metadata{
				TenantName: "tenant-name",
			},
			expected: Metadata{
				TenantName: "different-name",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if got := tc.base.Equal(tc.expected); got != tc.want {
				t.Errorf("(%v).IsEqualTo(%v): %v, want %v", tc.base, tc.expected, got, tc.want)
			}
		})
	}
}
