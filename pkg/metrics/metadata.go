package metrics

import (
	"context"
	"strconv"

	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/parameters"
)

const (
	// envGKEPDCSIVersion is an environment variable set in the PDCSI controller manifest
	// with the current version of the GKE component.
	requestMetadataKey = "requestMetadata"
)

// RequestMetadata represents metadata about a gRPC CSI request
type RequestMetadata struct {
	diskType                  string
	enableConfidentialStorage string
	enableStoragePools        string
}

func newRequestMetadata() *RequestMetadata {
	return &RequestMetadata{
		diskType:                  DefaultDiskTypeForMetric,
		enableConfidentialStorage: DefaultEnableConfidentialCompute,
		enableStoragePools:        DefaultEnableStoragePools,
	}
}

// MetadataFromContext returns a mutable from a request context
func MetadataFromContext(ctx context.Context) *RequestMetadata {
	requestMetadata, _ := ctx.Value(requestMetadataKey).(*RequestMetadata)
	return requestMetadata
}

func UpdateRequestMetadataFromParams(ctx context.Context, params parameters.DiskParameters) {
	metadata := MetadataFromContext(ctx)
	if metadata != nil {
		metadata.diskType = params.DiskType
		metadata.enableConfidentialStorage = strconv.FormatBool(params.EnableConfidentialCompute)
		hasStoragePools := len(params.StoragePools) > 0
		metadata.enableStoragePools = strconv.FormatBool(hasStoragePools)
	}
}

func UpdateRequestMetadataFromDisk(ctx context.Context, disk *gce.CloudDisk) {
	metadata := MetadataFromContext(ctx)
	if metadata != nil && disk != nil {
		metadata.diskType = disk.GetPDType()
		metadata.enableConfidentialStorage = strconv.FormatBool(disk.GetEnableConfidentialCompute())
		metadata.enableStoragePools = strconv.FormatBool(disk.GetEnableStoragePools())
	}
}
