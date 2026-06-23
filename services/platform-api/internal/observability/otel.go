// Package observability wires OpenTelemetry traces and metrics for the
// platform-api service. It is intentionally self-contained: Init sets up
// OTLP/gRPC exporters pointing at the in-cluster collector and installs
// global TracerProvider/MeterProvider, returning a shutdown func.
//
// If OTEL_EXPORTER_OTLP_ENDPOINT is empty, Init is a no-op (telemetry
// disabled) and the returned shutdown func does nothing. This keeps the
// binary bootable in dev and in any environment without a collector.
package observability

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// defaultEndpoint is the in-cluster OTLP/gRPC collector address used when
// OTEL_EXPORTER_OTLP_ENDPOINT is set but does not itself carry a value
// (handled by the env lookup below) — kept here for documentation. The
// effective default lives in the deployment env so ops can override it.
const defaultEndpoint = "http://otel-collector.observability.svc.cluster.local:4317"

// noop is returned when telemetry is disabled so callers can always defer
// the shutdown func unconditionally.
func noop(context.Context) error { return nil }

// Init configures OpenTelemetry traces and metrics over OTLP/gRPC and sets
// the resulting providers as global. It returns a shutdown func that flushes
// and stops the providers; call it (e.g. via defer) before the process exits.
//
// The collector endpoint is read from OTEL_EXPORTER_OTLP_ENDPOINT. When that
// variable is empty, telemetry is disabled and a no-op shutdown is returned.
func Init(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		// Telemetry disabled — no collector configured.
		return noop, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return noop, err
	}

	// otlp*grpc exporters take a bare host:port and a TLS/insecure toggle,
	// not a URL scheme. Strip http:// or https:// and pick insecure for the
	// in-cluster plaintext collector.
	addr, insecure := parseEndpoint(endpoint)

	// ─── Traces ─────────────────────────────────────────────────────────
	traceOpts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(addr)}
	if insecure {
		traceOpts = append(traceOpts, otlptracegrpc.WithInsecure())
	}
	traceExp, err := otlptracegrpc.New(ctx, traceOpts...)
	if err != nil {
		return noop, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// ─── Metrics ────────────────────────────────────────────────────────
	metricOpts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(addr)}
	if insecure {
		metricOpts = append(metricOpts, otlpmetricgrpc.WithInsecure())
	}
	metricExp, err := otlpmetricgrpc.New(ctx, metricOpts...)
	if err != nil {
		// Tear down the already-installed tracer provider so we don't leak it.
		_ = tp.Shutdown(ctx)
		return noop, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
	)
	otel.SetMeterProvider(mp)

	shutdown := func(ctx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return errors.Join(tp.Shutdown(shutdownCtx), mp.Shutdown(shutdownCtx))
	}
	return shutdown, nil
}

// parseEndpoint splits an OTLP endpoint into the host:port form the gRPC
// exporters expect and a flag indicating whether to dial without TLS.
// A plain http:// scheme (or no scheme) maps to insecure; https:// keeps TLS.
func parseEndpoint(endpoint string) (addr string, insecure bool) {
	switch {
	case strings.HasPrefix(endpoint, "http://"):
		return strings.TrimPrefix(endpoint, "http://"), true
	case strings.HasPrefix(endpoint, "https://"):
		return strings.TrimPrefix(endpoint, "https://"), false
	default:
		// Bare host:port — assume in-cluster plaintext collector.
		return endpoint, true
	}
}
