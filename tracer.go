// Package tracer provides zero-configuration distributed tracing for RanuPod workloads.
// It auto-discovers OpenTelemetry and Datadog configuration injected by RanuPod.
package tracer

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	ServiceName string
	Environment string
	Endpoint    string
	Insecure    bool
	Protocol    string // "http" or "grpc"
}

// Start initializes the global tracer and returns a shutdown function.
// Usage in main():
//
//	defer tracer.Start()()
func Start(opts ...func(*Config)) func() {
	cfg := Config{
		ServiceName: os.Getenv("OTEL_SERVICE_NAME"),
		Environment: os.Getenv("DD_ENV"),
		Endpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		Insecure:    true,
		Protocol:    "http",
	}

	if cfg.ServiceName == "" {
		cfg.ServiceName = os.Getenv("DD_SERVICE")
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "ranupod-app"
	}

	if cfg.Environment == "" {
		cfg.Environment = os.Getenv("ENV")
	}
	if cfg.Environment == "" {
		cfg.Environment = "production"
	}

	// Default fallback to in-cluster Alloy gateway
	if cfg.Endpoint == "" {
		agentHost := os.Getenv("DD_AGENT_HOST")
		if agentHost != "" {
			cfg.Endpoint = fmt.Sprintf("http://%s:4318", agentHost)
		} else {
			cfg.Endpoint = "http://alloy-otlp.observability.svc:4318"
		}
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	ctx := context.Background()

	// 1. Build Resource attributes
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.DeploymentEnvironment(cfg.Environment),
			attribute.String("ranupod.managed", "true"),
		),
	)
	if err != nil {
		res = resource.Default()
	}

	// 2. Build Exporter
	cleanEndpoint := cfg.Endpoint
	cleanEndpoint = strings.TrimPrefix(cleanEndpoint, "http://")
	cleanEndpoint = strings.TrimPrefix(cleanEndpoint, "https://")

	var exp sdktrace.SpanExporter
	if cfg.Protocol == "grpc" || strings.HasSuffix(cfg.Endpoint, ":4317") {
		exp, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cleanEndpoint),
			otlptracegrpc.WithInsecure(),
		)
	} else {
		exp, err = otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(cleanEndpoint),
			otlptracehttp.WithInsecure(),
		)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "[RanuPod Tracer] Warning: Failed to initialize exporter: %v\n", err)
		return func() {}
	}

	// 3. Register Tracer Provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(2*time.Second)),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Return cleanup function
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(shutdownCtx)
	}
}

// Middleware creates standard HTTP middleware that traces every inbound request.
func Middleware(serviceName string) func(http.Handler) http.Handler {
	tr := otel.GetTracerProvider().Tracer("ranupod/http")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)

			ctx, span := tr.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPRequestMethodKey.String(r.Method),
					semconv.URLPath(r.URL.Path),
					semconv.UserAgentOriginal(r.UserAgent()),
				),
			)
			defer span.End()

			rw := &responseWriterInterceptor{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rw, r.WithContext(ctx))

			span.SetAttributes(semconv.HTTPResponseStatusCode(rw.statusCode))
		})
	}
}

type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriterInterceptor) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
