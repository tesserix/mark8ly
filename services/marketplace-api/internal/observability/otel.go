// Package observability wires OpenTelemetry traces and metrics for the
// marketplace-api binary. It exports over OTLP gRPC to the in-cluster
// collector, controlled entirely by env so it stays a no-op in local dev
// (and any environment that doesn't set OTEL_EXPORTER_OTLP_ENDPOINT).
package observability

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// defaultEndpoint is the in-cluster OTLP gRPC collector address used when
// OTEL_EXPORTER_OTLP_ENDPOINT is set to a non-empty value but callers want
// a sensible default. Init reads the env var; this constant documents the
// expected wiring (see the Helm values for the deployed override).
const defaultEndpoint = "http://otel-collector.observability.svc.cluster.local:4317"

// Init bootstraps the global TracerProvider and MeterProvider and returns a
// shutdown func that flushes and stops both providers.
//
// If OTEL_EXPORTER_OTLP_ENDPOINT is empty, OpenTelemetry is disabled: Init
// returns a no-op shutdown and a nil error so the caller can always defer
// shutdown() unconditionally.
func Init(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		// Disabled — no exporters, no global providers installed.
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	// otlptracegrpc/otlpmetricgrpc parse an endpoint URL: a "http://" scheme
	// selects an insecure (plaintext) connection, "https://" enables TLS.
	traceOpts := []otlptracegrpc.Option{otlptracegrpc.WithEndpointURL(endpoint)}
	metricOpts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpointURL(endpoint)}

	traceExp, err := otlptracegrpc.New(ctx, traceOpts...)
	if err != nil {
		return nil, fmt.Errorf("otlp trace exporter: %w", err)
	}

	metricExp, err := otlpmetricgrpc.New(ctx, metricOpts...)
	if err != nil {
		// Don't leak the trace exporter if metrics fail to start.
		_ = traceExp.Shutdown(ctx)
		return nil, fmt.Errorf("otlp metric exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	shutdown := func(ctx context.Context) error {
		// Bound shutdown so a stuck collector can't hang process exit.
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var errs []error
		if err := tp.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		if err := mp.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			return fmt.Errorf("otel shutdown: %v", errs)
		}
		return nil
	}

	return shutdown, nil
}
