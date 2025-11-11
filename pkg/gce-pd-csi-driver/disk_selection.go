package gceGCEDriver

import (
	"context"
	"fmt"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
)

const (
	pdTypeParam         = "pdType"
	hdTypeParam         = "hdType"
	typePreferenceParam = "diskTypePreference"

	preferenceForPD = "pd-type"
	preferenceForHD = "hd-type"
)

func SelectDisk(ctx context.Context, req *csi.CreateVolumeRequest, gce gce.GCECompute) (string, error) {
	// This should never happen in practice.
	if req == nil {
		return "", fmt.Errorf("CreateVolumeRequest is nil")
	}

	dts, err := getDynamicDiskTypes(req.GetParameters())
	if err != nil {
		return "", fmt.Errorf("failed to get disk types from request parameters: %v", err)
	}

	if isSourceVolumeSpecified(req.GetVolumeContentSource()) {
		return getTypeFromSourceVolume(ctx, req.GetVolumeContentSource(), gce, dts)
	}

	return selectDiskTypeFromTopologies(req.GetAccessibilityRequirements(), dts), nil
}

type dynamicDiskTypes struct {
	PD      string
	HD      string
	Default string
}

// Extract disk types from the CSI CreateVolumeRequest.
func getDynamicDiskTypes(parameters map[string]string) (*dynamicDiskTypes, error) {
	if parameters == nil {
		return nil, fmt.Errorf("request parameters are nil")
	}

	pdType := strings.ToLower(parameters[pdTypeParam])
	if pdType == "" {
		return nil, fmt.Errorf("missing required parameter %q", pdTypeParam)
	}
	hdType := strings.ToLower(parameters[hdTypeParam])
	if hdType == "" {
		return nil, fmt.Errorf("missing required parameter %q", hdTypeParam)
	}

	// Determine default disk type based on preference parameter. If the parameter is
	// unspecfied than default to hdType.
	defaultDiskType := hdType
	if diskTypePreference, hasParameter := parameters[typePreferenceParam]; hasParameter {
		switch strings.ToLower(diskTypePreference) {
		case preferenceForPD:
			defaultDiskType = pdType
		case preferenceForHD:
			defaultDiskType = hdType
		default:
			return nil, fmt.Errorf("invalid disk type preference %q, must be %q or %q", diskTypePreference, preferenceForPD, preferenceForHD)
		}
	}

	return &dynamicDiskTypes{
		PD:      pdType,
		HD:      hdType,
		Default: defaultDiskType,
	}, nil
}

func isSourceVolumeSpecified(vcs *csi.VolumeContentSource) bool {
	switch {
	case vcs == nil:
		return false
	case vcs.GetVolume() == nil:
		return false
	default:
		return true
	}
}

func getTypeFromSourceVolume(ctx context.Context, vcs *csi.VolumeContentSource, gce gce.GCECompute, dts *dynamicDiskTypes) (string, error) {
	volumeContentSourceVolumeID := vcs.GetVolume().GetVolumeId()
	// Verify that the source VolumeID is in the correct format.
	project, sourceVolKey, err := common.VolumeIDToKey(volumeContentSourceVolumeID)
	if err != nil {
		return "", fmt.Errorf("failed to get source volume key: %v", err)
	}

	// Verify that the volume in VolumeContentSource exists, and it's disk type
	// match the specified dynamic disk types.
	d, err := gce.GetDisk(ctx, project, sourceVolKey)
	if err != nil {
		return "", fmt.Errorf("failed to get disk type from source volume: %v", err)
	}
	sourceDiskType := d.GetPDType()
	if sourceDiskType != dts.HD && sourceDiskType != dts.PD {
		return "", fmt.Errorf("source volume has invalid disk type %q, must be %q or %q", sourceDiskType, dts.PD, dts.HD)
	}

	return d.GetPDType(), nil
}

func selectDiskTypeFromTopologies(topologies *csi.TopologyRequirement, dts *dynamicDiskTypes) string {
	if len(topologies.GetPreferred()) > 0 {
		t := topologies.GetPreferred()[0]

		labels := t.GetSegments()
		supportedDiskTypes := map[string]bool{}
		for key := range labels {
			if common.HasDiskTypeLabelKeyPrefix(key) {
				supportedDiskTypes[common.DiskTypeFromLabel(key)] = true
			}
		}

		isHDSupported := supportedDiskTypes[dts.HD]
		isPDSupported := supportedDiskTypes[dts.PD]

		if isHDSupported && !isPDSupported {
			return dts.HD
		}
		if isPDSupported && !isHDSupported {
			return dts.PD
		}
	}

	return dts.Default
}
