package gceGCEDriver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	computev1 "google.golang.org/api/compute/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/constants"
	gcecloudprovider "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
)

const (
	diskName = "test-disk"
	volumeID = "projects/test-project/zones/us-central1-a/disks/test-disk"

	testNamespace  = "default"
	testPVCName    = "test-pvc"
	testPVName     = "test-pv"
	testDriverName = "pd.csi.storage.gke.io"
)

func TestGetDiskMeta(t *testing.T) {
	tests := []struct {
		name         string
		disk         *computev1.Disk
		wantVolumeID string
		wantTags     map[string]string
		wantErr      bool
	}{
		{
			name: "valid disk with all required metadata",
			disk: &computev1.Disk{
				SelfLink: selfLinkPrefix + volumeID,
				Description: mustMarshal(map[string]string{
					pvcNamespaceKey: testNamespace,
					pvcNameKey:      testPVCName,
					pvNameKey:       testPVName,
					createdByKey:    testDriverName,
				}),
			},
			wantVolumeID: "projects/test-project/zones/us-central1-a/disks/test-disk",
			wantTags: map[string]string{
				pvcNamespaceKey: testNamespace,
				pvcNameKey:      testPVCName,
				pvNameKey:       testPVName,
				createdByKey:    testDriverName,
			},
			wantErr: false,
		},
		{
			name: "missing pv name",
			disk: &computev1.Disk{
				SelfLink: selfLinkPrefix + volumeID,
				Description: mustMarshal(map[string]string{
					pvcNamespaceKey: testNamespace,
					pvcNameKey:      testPVCName,
					createdByKey:    "gce-pd-csi-driver",
				}),
			},
			wantErr: true,
		},
		{
			name: "missing pvc name",
			disk: &computev1.Disk{
				SelfLink: selfLinkPrefix + volumeID,
				Description: mustMarshal(map[string]string{
					pvcNamespaceKey: testNamespace,
					pvNameKey:       testPVName,
					createdByKey:    "gce-pd-csi-driver",
				}),
			},
			wantErr: true,
		},
		{
			name: "missing pvc namespace",
			disk: &computev1.Disk{
				SelfLink: selfLinkPrefix + volumeID,
				Description: mustMarshal(map[string]string{
					pvcNameKey:   testPVCName,
					pvNameKey:    testPVName,
					createdByKey: "gce-pd-csi-driver",
				}),
			},
			wantErr: true,
		},
		{
			name: "invalid JSON in description",
			disk: &computev1.Disk{
				SelfLink:    selfLinkPrefix + volumeID,
				Description: "invalid json",
			},
			wantErr: true,
		},
		{
			name: "disk not managed by CSI driver",
			disk: &computev1.Disk{
				SelfLink: selfLinkPrefix + volumeID,
				Description: mustMarshal(map[string]string{
					pvcNamespaceKey: testNamespace,
					pvcNameKey:      testPVCName,
					pvNameKey:       testPVName,
					createdByKey:    "some-other-driver",
				}),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVolumeID, gotTags, err := getDiskMeta(tt.disk)
			if (err != nil) != tt.wantErr {
				t.Errorf("getDiskMeta() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if diff := cmp.Diff(tt.wantVolumeID, gotVolumeID); diff != "" {
					t.Errorf("getDiskMeta() volumeID mismatch (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tt.wantTags, gotTags); diff != "" {
					t.Errorf("getDiskMeta() tags mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestLabelFilter(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{
			name:  "cluster ID filter",
			key:   "cluster-id",
			value: "test-cluster",
			want:  "labels.cluster-id=test-cluster",
		},
		{
			name:  "status filter",
			key:   "status",
			value: "provisioning",
			want:  "labels.status=provisioning",
		},
		{
			name:  "empty value",
			key:   "key",
			value: "",
			want:  "labels.key=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := labelFilter(tt.key, tt.value)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("labelFilter() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func mustMarshal(data map[string]string) string {
	b, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestHandleDisk(t *testing.T) {
	tests := []struct {
		name               string
		disk               *computev1.Disk
		enableDiskDeletion bool
		initialObjects     []client.Object
		wantDisk           *gcecloudprovider.CloudDisk
	}{
		{
			name: "pv & pvc exist -> update disk status",
			disk: &computev1.Disk{
				Name:     diskName,
				SelfLink: selfLinkPrefix + volumeID,
				Description: mustMarshal(map[string]string{
					pvcNamespaceKey: testNamespace,
					pvcNameKey:      testPVCName,
					pvNameKey:       testPVName,
					createdByKey:    testDriverName,
				}),
				Labels: map[string]string{
					constants.VolumePublishStatus: constants.ProvisioningStatus,
				},
			},
			initialObjects: []client.Object{
				&v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: testPVName,
					},
				},
				&v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testPVCName,
						Namespace: testNamespace,
					},
				},
			},
			enableDiskDeletion: true,
			wantDisk: gcecloudprovider.CloudDiskFromV1(&computev1.Disk{
				Labels: map[string]string{
					constants.VolumePublishStatus: constants.ProvisionedStatus,
				},
			}),
		},
		{
			name: "pvc only -> do nothing",
			disk: &computev1.Disk{
				Name:     diskName,
				SelfLink: selfLinkPrefix + volumeID,
				Description: mustMarshal(map[string]string{
					pvcNamespaceKey: testNamespace,
					pvcNameKey:      testPVCName,
					pvNameKey:       testPVName,
					createdByKey:    testDriverName,
				}),
				Labels: map[string]string{
					constants.VolumePublishStatus: constants.ProvisioningStatus,
				},
			},
			initialObjects: []client.Object{
				&v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testPVCName,
						Namespace: testNamespace,
					},
				},
			},
			enableDiskDeletion: true,
			wantDisk: gcecloudprovider.CloudDiskFromV1(&computev1.Disk{
				Labels: map[string]string{
					constants.VolumePublishStatus: constants.ProvisioningStatus,
				},
			}),
		},
		{
			name: "no associated objects -> delete disk",
			disk: &computev1.Disk{
				Name:     diskName,
				SelfLink: selfLinkPrefix + volumeID,
				Description: mustMarshal(map[string]string{
					pvcNamespaceKey: testNamespace,
					pvcNameKey:      testPVCName,
					pvNameKey:       testPVName,
					createdByKey:    testDriverName,
				}),
				Labels: map[string]string{
					constants.VolumePublishStatus: constants.ProvisioningStatus,
				},
			},
			enableDiskDeletion: true,
			wantDisk:           nil,
		},
		{
			name: "no associated object with disk cleanup disabled -> do nothing",
			disk: &computev1.Disk{
				Name:     diskName,
				SelfLink: selfLinkPrefix + volumeID,
				Description: mustMarshal(map[string]string{
					pvcNamespaceKey: testNamespace,
					pvcNameKey:      testPVCName,
					pvNameKey:       testPVName,
					createdByKey:    testDriverName,
				}),
				Labels: map[string]string{
					constants.VolumePublishStatus: constants.ProvisioningStatus,
				},
			},
			enableDiskDeletion: false,
			wantDisk: gcecloudprovider.CloudDiskFromV1(&computev1.Disk{
				Labels: map[string]string{
					constants.VolumePublishStatus: constants.ProvisioningStatus,
				},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup k8s fake client
			builder := fake.NewClientBuilder()
			if len(tt.initialObjects) > 0 {
				builder.WithObjects(tt.initialObjects...)
			}
			kc := builder.Build()

			// Setup fake cloud provider
			var cloudDisks []*gcecloudprovider.CloudDisk
			if tt.disk != nil {
				cloudDisks = append(cloudDisks, gcecloudprovider.CloudDiskFromV1(tt.disk))
			}
			fcp, err := gcecloudprovider.CreateFakeCloudProvider("test-project", "us-central1-a", cloudDisks)
			if err != nil {
				t.Fatalf("failed to create fake cloud provider: %v", err)
			}

			gceCS := &GCEControllerServer{
				CloudProvider: fcp,
			}

			if err := handleDisk(ctx, tt.disk, kc, gceCS, tt.enableDiskDeletion); err != nil {
				t.Fatalf("handleDisk() error = %v", err)
			}

			project, volKey, err := common.VolumeIDToKey(volumeID)
			if err != nil {
				t.Fatalf("failed to parse volume ID: %v", err)
			}

			gotDisk, _ := fcp.GetDisk(ctx, project, volKey)
			if diff := cmp.Diff(tt.wantDisk, gotDisk, cmp.AllowUnexported(gcecloudprovider.CloudDisk{}), cmpopts.IgnoreFields(computev1.Disk{}, "Description", "Name", "SelfLink")); diff != "" {
				t.Errorf("disk mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
