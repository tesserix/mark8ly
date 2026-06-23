// Package observability wires OpenTelemetry traces and metrics for the
// otto service. Everything is exported over OTLP/gRPC to the in-cluster
// collector. It is intentionally self-contained: no other otto package
// depends on it beyond a single Init call in main.
package observability

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// defaultEndpoint is the in-cluster OTLP/gRPC collector address used when
// OTEL_EXPORTER_OTLP_ENDPOINT is unset but observability is requested.
const defaultEndpoint = "http://otel-collector.observability.svc.cluster.local:4317"

// Init sets up global OpenTelemetry trace and metric providers exporting
// over OTLP/gRPC. The returned shutdown func flushes and tears down both
// providers; callers should defer it.
//
// Behaviour is driven by OTEL_EXPORTER_OTLP_ENDPOINT:
//   - empty string  -> observability is disabled (no-op shutdown, nil err)
//   - any other value -> exporters target that endpoint
//
// As a convenience, the default in-cluster collector endpoint is applied
// when the env var is explicitly set to the literal "default" — otherwise
// the provided value is used verbatim. An unset env var means disabled so
// local runs and tests stay quiet without a collector.
func Init(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		// Disabled: nothing wired, harmless shutdown.
		return func(context.Context) error { return nil }, nil
	}
	if endpoint == "default" {
		endpoint = defaultEndpoint
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: build resource: %w", err)
	}

	// otlptracegrpc/otlpmetricgrpc parse the standard OTEL_EXPORTER_OTLP_*
	// env vars themselves; we also pass the endpoint explicitly so an
	// http:// scheme cleanly maps to an insecure in-cluster connection.
	traceExp, err := otlptracegrpc.New(ctx, grpcTraceOpts(endpoint)...)
	if err != nil {
		return nil, fmt.Errorf("observability: trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	metricExp, err := otlpmetricgrpc.New(ctx, grpcMetricOpts(endpoint)...)
	if err != nil {
		// Roll back the tracer provider so we don't leak a live exporter
		// on a partial init.
		_ = tp.Shutdown(ctx)
		return nil, fmt.Errorf("observability: metric exporter: %w", err)
	}
	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(metricExp,
			metric.WithInterval(30*time.Second),
		)),
	)
	otel.SetMeterProvider(mp)

	shutdown := func(ctx context.Context) error {
		var first error
		if err := tp.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
		if err := mp.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
		return first
	}
	return shutdown, nil
}

func grpcTraceOpts(endpoint string) []otlptracegrpc.Option {
	target, insecure := stripScheme(endpoint)
	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(target)}
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	return opts
}

func grpcMetricOpts(endpoint string) []otlpmetricgrpc.Option {
	target, insecure := stripScheme(endpoint)
	opts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(target)}
	if insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}
	return opts
}

// stripScheme turns a URL-style endpoint (http://host:4317) into the
// host:port the gRPC exporters expect, reporting whether TLS should be
// disabled. An http:// scheme (the in-cluster collector) implies insecure;
// https:// keeps TLS; a bare host:port is left untouched (secure default).
func stripScheme(endpoint string) (target string, insecure bool) {
	switch {
	case len(endpoint) > 7 && endpoint[:7] == "http://":
		return endpoint[7:], true
	case len(endpoint) > 8 && endpoint[:8] == "https://":
		return endpoint[8:], false
	default:
		return endpoint, false
	}
}
