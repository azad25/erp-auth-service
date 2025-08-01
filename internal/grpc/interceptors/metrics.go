package interceptors

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// MetricsInterceptor provides Prometheus metrics for gRPC requests
type MetricsInterceptor struct {
	requestsTotal     *prometheus.CounterVec
	requestDuration   *prometheus.HistogramVec
	requestsInFlight  *prometheus.GaugeVec
	requestSize       *prometheus.HistogramVec
	responseSize      *prometheus.HistogramVec
}

// NewMetricsInterceptor creates a new metrics interceptor
func NewMetricsInterceptor(registry *prometheus.Registry) *MetricsInterceptor {
	mi := &MetricsInterceptor{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "grpc_server_requests_total",
				Help: "Total number of gRPC requests",
			},
			[]string{"method", "code"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "grpc_server_request_duration_seconds",
				Help:    "Duration of gRPC requests",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~32s
			},
			[]string{"method", "code"},
		),
		requestsInFlight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "grpc_server_requests_in_flight",
				Help: "Number of gRPC requests currently being processed",
			},
			[]string{"method"},
		),
		requestSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "grpc_server_request_size_bytes",
				Help:    "Size of gRPC request messages",
				Buckets: prometheus.ExponentialBuckets(64, 2, 16), // 64B to ~4MB
			},
			[]string{"method"},
		),
		responseSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "grpc_server_response_size_bytes",
				Help:    "Size of gRPC response messages",
				Buckets: prometheus.ExponentialBuckets(64, 2, 16), // 64B to ~4MB
			},
			[]string{"method"},
		),
	}
	
	// Register metrics
	registry.MustRegister(
		mi.requestsTotal,
		mi.requestDuration,
		mi.requestsInFlight,
		mi.requestSize,
		mi.responseSize,
	)
	
	return mi
}

// UnaryServerInterceptor returns a gRPC unary server interceptor for metrics
func (mi *MetricsInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		method := info.FullMethod
		
		// Track in-flight requests
		mi.requestsInFlight.WithLabelValues(method).Inc()
		defer mi.requestsInFlight.WithLabelValues(method).Dec()
		
		// Record request size (approximate)
		if reqSize := mi.estimateMessageSize(req); reqSize > 0 {
			mi.requestSize.WithLabelValues(method).Observe(float64(reqSize))
		}
		
		// Execute handler with timing
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)
		
		// Get status code
		code := status.Code(err).String()
		
		// Record metrics
		mi.requestsTotal.WithLabelValues(method, code).Inc()
		mi.requestDuration.WithLabelValues(method, code).Observe(duration.Seconds())
		
		// Record response size (approximate)
		if resp != nil {
			if respSize := mi.estimateMessageSize(resp); respSize > 0 {
				mi.responseSize.WithLabelValues(method).Observe(float64(respSize))
			}
		}
		
		return resp, err
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor for metrics
func (mi *MetricsInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		method := info.FullMethod
		
		// Track in-flight requests
		mi.requestsInFlight.WithLabelValues(method).Inc()
		defer mi.requestsInFlight.WithLabelValues(method).Dec()
		
		// Execute handler with timing
		start := time.Now()
		err := handler(srv, stream)
		duration := time.Since(start)
		
		// Get status code
		code := status.Code(err).String()
		
		// Record metrics
		mi.requestsTotal.WithLabelValues(method, code).Inc()
		mi.requestDuration.WithLabelValues(method, code).Observe(duration.Seconds())
		
		return err
	}
}

// estimateMessageSize provides a rough estimate of message size
// This is a simplified implementation - in production you might want more accurate sizing
func (mi *MetricsInterceptor) estimateMessageSize(msg interface{}) int {
	if msg == nil {
		return 0
	}
	
	// This is a very rough estimate based on the string representation
	// In a real implementation, you might want to use proto.Size() for protobuf messages
	msgStr := ""
	switch v := msg.(type) {
	case string:
		msgStr = v
	default:
		// For other types, we'll use a rough estimate
		return 256 // Default estimate
	}
	
	return len(msgStr)
}

// RecordTokenValidationDuration records the duration of token validation
func (mi *MetricsInterceptor) RecordTokenValidationDuration(duration time.Duration) {
	mi.requestDuration.WithLabelValues("ValidateToken", "OK").Observe(duration.Seconds())
}

// GetMetrics returns the current metrics for external access
func (mi *MetricsInterceptor) GetMetrics() map[string]prometheus.Collector {
	return map[string]prometheus.Collector{
		"requests_total":     mi.requestsTotal,
		"request_duration":   mi.requestDuration,
		"requests_in_flight": mi.requestsInFlight,
		"request_size":       mi.requestSize,
		"response_size":      mi.responseSize,
	}
}