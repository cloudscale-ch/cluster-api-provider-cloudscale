/*
Copyright 2026 cloudscale.ch.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package observability

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

// InitTracing initializes an OpenTelemetry tracer provider with an OTLP gRPC
// exporter. The OTLP endpoint is read from the OTEL_EXPORTER_OTLP_ENDPOINT
// environment variable (defaults to localhost:4317 if unset).
func InitTracing(ctx context.Context, log logr.Logger, serviceName, version string, sampleRate float64) (func(), error) {
	if sampleRate < 0.0 || sampleRate > 1.0 {
		return nil, fmt.Errorf("tracing-sample-rate must be between 0.0 and 1.0, got %f", sampleRate)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			attribute.String("version", version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create opentelemetry resource: %w", err)
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	sampler := sdktrace.AlwaysSample()
	if sampleRate < 1.0 {
		sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRate))
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Error(err, "Failed to shut down tracer provider")
		}
	}
	return shutdown, nil
}
