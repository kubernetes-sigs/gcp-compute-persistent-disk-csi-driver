package metrics

import (
	"context"

	"google.golang.org/grpc"
)

type MetricInterceptor struct {
	MetricsManager *MetricsManager
}

func (m *MetricInterceptor) unaryInterceptorInternal(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	requestMetadata := newRequestMetadata()
	newCtx := context.WithValue(ctx, requestMetadataKey, requestMetadata)
	result, err := handler(newCtx, req)
	m.MetricsManager.RecordOperationErrorMetrics(info.FullMethod, err, requestMetadata.diskType, requestMetadata.enableConfidentialStorage, requestMetadata.enableStoragePools)
	return result, err
}

func (m *MetricInterceptor) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return m.unaryInterceptorInternal
}
