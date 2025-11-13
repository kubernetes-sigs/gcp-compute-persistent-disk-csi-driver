package parameters

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSanitizeDiskParameters(t *testing.T) {
	tests := []struct {
		desc       string
		parameters *DiskParameters
		want       *DiskParameters
	}{
		{
			desc:       "nil parameters",
			parameters: nil,
		},
		{
			desc:       "empty parameters unchanged",
			parameters: &DiskParameters{},
			want:       &DiskParameters{},
		},
		{
			desc: "general parameters unchanged PD",
			parameters: &DiskParameters{
				DiskType:                  "pd-balanced",
				DiskEncryptionKMSKey:      "fake-kms-key",
				Tags:                      map[string]string{"fake-tag": "fake-resource-tag-value"},
				Labels:                    map[string]string{"fake-label": "fake-label-value"},
				EnableConfidentialCompute: true,
				ResourceTags:              map[string]string{"fake-resource-tag": "fake-resource-tag-value"},
				AccessMode:                "READ_WRITE_SINGLE",
				UseAllowedDiskTopology:    true,
			},
			want: &DiskParameters{
				DiskType:                  "pd-balanced",
				DiskEncryptionKMSKey:      "fake-kms-key",
				Tags:                      map[string]string{"fake-tag": "fake-resource-tag-value"},
				Labels:                    map[string]string{"fake-label": "fake-label-value"},
				EnableConfidentialCompute: true,
				ResourceTags:              map[string]string{"fake-resource-tag": "fake-resource-tag-value"},
				AccessMode:                "READ_WRITE_SINGLE",
				UseAllowedDiskTopology:    true,
			},
		},
		{
			desc: "general parameters unchanged HD",
			parameters: &DiskParameters{
				DiskType:                  "hyperdisk-balanced",
				DiskEncryptionKMSKey:      "fake-kms-key",
				Tags:                      map[string]string{"fake-tag": "fake-resource-tag-value"},
				Labels:                    map[string]string{"fake-label": "fake-label-value"},
				EnableConfidentialCompute: true,
				ResourceTags:              map[string]string{"fake-resource-tag": "fake-resource-tag-value"},
				AccessMode:                "READ_WRITE_SINGLE",
				UseAllowedDiskTopology:    true,
			},
			want: &DiskParameters{
				DiskType:                  "hyperdisk-balanced",
				DiskEncryptionKMSKey:      "fake-kms-key",
				Tags:                      map[string]string{"fake-tag": "fake-resource-tag-value"},
				Labels:                    map[string]string{"fake-label": "fake-label-value"},
				EnableConfidentialCompute: true,
				ResourceTags:              map[string]string{"fake-resource-tag": "fake-resource-tag-value"},
				AccessMode:                "READ_WRITE_SINGLE",
				UseAllowedDiskTopology:    true,
			},
		},
		{
			// For replication type, the sanitized value is "none" instead of an empty string.
			desc: "ReplicationType sanitized for HD",
			parameters: &DiskParameters{
				DiskType:        "hyperdisk-balanced",
				ReplicationType: "regional-pd",
			},
			want: &DiskParameters{
				DiskType:        "hyperdisk-balanced",
				ReplicationType: "none",
			},
		},
		{
			desc: "ReplicationType unchanged for PD",
			parameters: &DiskParameters{
				DiskType:        "pd-balanced",
				ReplicationType: "regional",
			},
			want: &DiskParameters{
				DiskType:        "pd-balanced",
				ReplicationType: "regional",
			},
		},
		{
			desc: "ProvisionedIOPSOnCreate sanitized for PD",
			parameters: &DiskParameters{
				DiskType:                "pd-balanced",
				ProvisionedIOPSOnCreate: 1000,
			},
			want: &DiskParameters{
				DiskType: "pd-balanced",
			},
		},
		{
			desc: "ProvisionedIOPSOnCreate unchanged for HD",
			parameters: &DiskParameters{
				DiskType:                "hyperdisk-balanced",
				ProvisionedIOPSOnCreate: 5000,
			},
			want: &DiskParameters{
				DiskType:                "hyperdisk-balanced",
				ProvisionedIOPSOnCreate: 5000,
			},
		},
		{
			desc: "ProvisionedIOPSOnCreate unchanged for pd-extreme",
			parameters: &DiskParameters{
				DiskType:                "pd-extreme",
				ProvisionedIOPSOnCreate: 5000,
			},
			want: &DiskParameters{
				DiskType:                "pd-extreme",
				ProvisionedIOPSOnCreate: 5000,
			},
		},
		{
			desc: "ProvisionedThroughputOnCreate sanitized for PD",
			parameters: &DiskParameters{
				DiskType:                      "pd-standard",
				ProvisionedThroughputOnCreate: 200,
			},
			want: &DiskParameters{
				DiskType: "pd-standard",
			},
		},
		{
			desc: "ProvisionedThroughputOnCreate unchanged for HD",
			parameters: &DiskParameters{
				DiskType:                      "hyperdisk-balanced",
				ProvisionedThroughputOnCreate: 200,
			},
			want: &DiskParameters{
				DiskType:                      "hyperdisk-balanced",
				ProvisionedThroughputOnCreate: 200,
			},
		},
		{
			desc: "StoragePools sanitized for PD",
			parameters: &DiskParameters{
				DiskType:     "pd-balanced",
				StoragePools: []StoragePool{{Name: "fake-pool"}},
			},
			want: &DiskParameters{
				DiskType: "pd-balanced",
			},
		},
		{
			desc: "StoragePools unchanged for HD",
			parameters: &DiskParameters{
				DiskType:     "hyperdisk-balanced",
				StoragePools: []StoragePool{{Name: "fake-pool"}},
			},
			want: &DiskParameters{
				DiskType:     "hyperdisk-balanced",
				StoragePools: []StoragePool{{Name: "fake-pool"}},
			},
		},
		{
			desc: "ForceAttach sanitized for HD",
			parameters: &DiskParameters{
				DiskType:    "hyperdisk-balanced",
				ForceAttach: true,
			},
			want: &DiskParameters{
				DiskType: "hyperdisk-balanced",
			},
		},
		{
			desc: "ForceAttach unchanged for PD",
			parameters: &DiskParameters{
				DiskType:    "pd-balanced",
				ForceAttach: true,
			},
			want: &DiskParameters{
				DiskType:    "pd-balanced",
				ForceAttach: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			SanitizeDiskParameters(tc.parameters)
			got := tc.parameters
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("sanitizeDiskParameters() mismatch (-want +got):\n%s", diff)
			}

		})
	}

}
