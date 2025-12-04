package gceGCEDriver

import (
	"context"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/go-cmp/cmp"
	computev1 "google.golang.org/api/compute/v1"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/parameters"
)

const (
	pdTestDiskType = "pd-fake"
	hdTestDiskType = "hyperdisk-fake"

	testProject = "fake-project"
	testZone    = "us-central1-f"
)

var (
	fakeDiskPD = gce.CloudDiskFromV1(&computev1.Disk{
		Name: "pd-source-disk",
		Type: pdTestDiskType,
	})
	fakeDiskHD = gce.CloudDiskFromV1(&computev1.Disk{
		Name: "hd-source-disk",
		Type: hdTestDiskType,
	})
	fakeDiskInvalid = gce.CloudDiskFromV1(&computev1.Disk{
		Name: "invalid-source-disk",
		Type: "invalid-type",
	})
)

func TestSelectDisk(t *testing.T) {
	tests := []struct {
		desc    string
		req     *csi.CreateVolumeRequest
		want    string
		wantErr bool
	}{
		{
			desc: "no topologies select hd by default",
			req: &csi.CreateVolumeRequest{
				Parameters: map[string]string{
					parameters.ParameterPDType: pdTestDiskType,
					parameters.ParameterHDType: hdTestDiskType,
				},
			},
			want: hdTestDiskType,
		},
		{
			desc: "disk type override",
			req: &csi.CreateVolumeRequest{
				Parameters: map[string]string{
					parameters.ParameterPDType:         pdTestDiskType,
					parameters.ParameterHDType:         hdTestDiskType,
					parameters.ParameterDiskPreference: parameters.ParameterPDType,
				},
			},
			want: pdTestDiskType,
		},
		{
			desc: "topologies only support pd",
			req: &csi.CreateVolumeRequest{
				Parameters: map[string]string{
					parameters.ParameterPDType: pdTestDiskType,
					parameters.ParameterHDType: hdTestDiskType,
				},
				AccessibilityRequirements: &csi.TopologyRequirement{
					Preferred: []*csi.Topology{
						{
							Segments: map[string]string{
								common.DiskTypeLabelKey(pdTestDiskType): "true",
							},
						},
					},
				},
			},
			want: pdTestDiskType,
		},
		{
			desc: "source volume specified",
			req: &csi.CreateVolumeRequest{
				Parameters: map[string]string{
					parameters.ParameterPDType: pdTestDiskType,
					parameters.ParameterHDType: hdTestDiskType,
				},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: fmt.Sprintf("projects/%s/zones/%s/disks/%s", testProject, testZone, fakeDiskPD.GetName())},
					},
				},
			},
			want: pdTestDiskType,
		},
		{
			desc:    "fail parameters missing",
			req:     &csi.CreateVolumeRequest{},
			wantErr: true,
		},
		{
			desc: "fail source volume missing",
			req: &csi.CreateVolumeRequest{
				Parameters: map[string]string{
					parameters.ParameterPDType: pdTestDiskType,
					parameters.ParameterHDType: hdTestDiskType,
				},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: fmt.Sprintf("projects/%s/zones/%s/disks/%s", testProject, testZone, fakeDiskInvalid.GetName())},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			fcp, err := gce.CreateFakeCloudProvider(testProject, testZone, []*gce.CloudDisk{fakeDiskPD})
			if err != nil {
				t.Fatalf("Failed to create fake cloud provider: %v", err)
			}
			got, err := SelectDisk(context.Background(), tc.req, fcp)
			if (err != nil) != tc.wantErr {
				t.Fatalf("SelectDisk() wantErr = %v, gotErr = %v", tc.wantErr, err)
			}

			if got != tc.want {
				t.Errorf("SelectDisk() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGetDiskTypes(t *testing.T) {
	tests := []struct {
		desc       string
		parameters map[string]string
		want       *dynamicDiskTypes
		wantErr    bool
	}{
		{
			desc: "successful extraction",
			parameters: map[string]string{
				parameters.ParameterPDType: pdTestDiskType,
				parameters.ParameterHDType: hdTestDiskType,
			},
			want: &dynamicDiskTypes{
				PD:      pdTestDiskType,
				HD:      hdTestDiskType,
				Default: hdTestDiskType,
			},
		},
		{
			desc: "pd default override",
			parameters: map[string]string{
				parameters.ParameterPDType:         pdTestDiskType,
				parameters.ParameterHDType:         hdTestDiskType,
				parameters.ParameterDiskPreference: parameters.ParameterHDType,
			},
			want: &dynamicDiskTypes{
				PD:      pdTestDiskType,
				HD:      hdTestDiskType,
				Default: hdTestDiskType,
			},
		},
		{
			desc: "hd default override",
			parameters: map[string]string{
				parameters.ParameterPDType:         pdTestDiskType,
				parameters.ParameterHDType:         hdTestDiskType,
				parameters.ParameterDiskPreference: parameters.ParameterHDType,
			},
			want: &dynamicDiskTypes{
				PD:      pdTestDiskType,
				HD:      hdTestDiskType,
				Default: hdTestDiskType,
			},
		},
		{
			desc: "invalid type preference",
			parameters: map[string]string{
				parameters.ParameterPDType:         pdTestDiskType,
				parameters.ParameterHDType:         hdTestDiskType,
				parameters.ParameterDiskPreference: "fake-preference",
			},
			wantErr: true,
		},
		{
			desc: "missing pd type",
			parameters: map[string]string{
				parameters.ParameterHDType: hdTestDiskType,
			},
			want: &dynamicDiskTypes{
				PD:      defaultTypePD,
				HD:      hdTestDiskType,
				Default: hdTestDiskType,
			},
		},
		{
			desc: "missing hd type",
			parameters: map[string]string{
				parameters.ParameterPDType: pdTestDiskType,
			},
			want: &dynamicDiskTypes{
				PD:      pdTestDiskType,
				HD:      defaultTypeHD,
				Default: defaultTypeHD,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := getDynamicDiskTypes(tc.parameters)
			if (err != nil) != tc.wantErr {
				t.Fatalf("getDiskTypes() wantErr = %v, gotErr = %v", tc.wantErr, err)
			}

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("Unexpected disk types (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIsSourceVolumeSpecified(t *testing.T) {
	tests := []struct {
		desc         string
		sourceVolume *csi.VolumeContentSource
		want         bool
	}{
		{
			desc: "nil volume content source",
			want: false,
		},
		{
			desc: "snapshot source specified",
			sourceVolume: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{},
			},
			want: false,
		},
		{
			desc: "source volume specified",
			sourceVolume: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: "projects/fake-project/zones/us-central1-f/disks/fake-disk"},
				},
			},
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got := isSourceVolumeSpecified(tc.sourceVolume)
			if got != tc.want {
				t.Errorf("isSourceVolumeSpecified() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGetTypeFromSourceVolume(t *testing.T) {
	tests := []struct {
		desc         string
		sourceVolume *csi.VolumeContentSource
		want         string
		wantErr      bool
	}{
		{
			desc: "source volume select pd type",
			sourceVolume: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: fmt.Sprintf("projects/%s/zones/%s/disks/%s", testProject, testZone, fakeDiskPD.GetName())},
				},
			},
			want: pdTestDiskType,
		},
		{
			desc: "source volume select hd type",
			sourceVolume: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: fmt.Sprintf("projects/%s/zones/%s/disks/%s", testProject, testZone, fakeDiskHD.GetName())},
				},
			},
			want: hdTestDiskType,
		},
		{
			desc: "fail source type does not match parameters",
			sourceVolume: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: fmt.Sprintf("projects/%s/zones/%s/disks/%s", testProject, testZone, fakeDiskInvalid.GetName())},
				},
			},
			wantErr: true,
		},
		{
			desc: "fail source volume missing",
			sourceVolume: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: fmt.Sprintf("projects/%s/zones/%s/disks/%s", testProject, testZone, "non-existent-disk")},
				},
			},
			wantErr: true,
		},
		{
			desc: "fail invalid volume id format",
			sourceVolume: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: "invalid-volume-id"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			fcp, err := gce.CreateFakeCloudProvider(testProject, testZone, []*gce.CloudDisk{fakeDiskPD, fakeDiskHD, fakeDiskInvalid})
			if err != nil {
				t.Fatalf("Failed to create fake cloud provider: %v", err)
			}
			got, err := getTypeFromSourceVolume(context.Background(), tc.sourceVolume, fcp, &dynamicDiskTypes{
				PD:      pdTestDiskType,
				HD:      hdTestDiskType,
				Default: hdTestDiskType,
			})
			if (err != nil) != tc.wantErr {
				t.Fatalf("getTypeFromSourceVolume() wantErr = %v, gotErr = %v", tc.wantErr, err)
			}

			if got != tc.want {
				t.Errorf("getTypeFromSourceVolume() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSelectDiskTypeFromTopology(t *testing.T) {
	// This is not an actual value, and used only by this test to verify default behavior,
	const defaultDiskType = "default-type"

	tests := []struct {
		desc       string
		topologies *csi.TopologyRequirement
		want       string
	}{
		{
			desc: "only hd supported",
			topologies: &csi.TopologyRequirement{
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							common.DiskTypeLabelKey(hdTestDiskType): "true",
						},
					},
				},
			},
			want: hdTestDiskType,
		},
		{
			desc: "only pd supported",
			topologies: &csi.TopologyRequirement{
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							common.DiskTypeLabelKey(pdTestDiskType): "true",
						},
					},
				},
			},
			want: pdTestDiskType,
		},
		{
			desc: "both are supported",
			topologies: &csi.TopologyRequirement{
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							common.DiskTypeLabelKey(pdTestDiskType): "true",
							common.DiskTypeLabelKey(hdTestDiskType): "true",
						},
					},
				},
			},
			want: defaultDiskType,
		},
		{
			desc: "none are supported",
			topologies: &csi.TopologyRequirement{
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{},
					},
				},
			},
			want: defaultDiskType,
		},
		{
			desc: "no topologies",
			want: defaultDiskType,
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got := selectDiskTypeFromTopologies(tc.topologies, &dynamicDiskTypes{
				PD:      pdTestDiskType,
				HD:      hdTestDiskType,
				Default: defaultDiskType,
			})
			if got != tc.want {
				t.Errorf("selectDiskTypeFromTopologies() = %v, want %v", got, tc.want)
			}
		})
	}
}
