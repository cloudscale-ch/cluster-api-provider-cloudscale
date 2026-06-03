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

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// StartSpanWithLogger starts a new OTel span and returns a context, logger,
// and done function. When tracing is enabled, the returned logger emits
// trace_id and span_id on every log line for log/trace correlation, and
// logger.Error() additionally records the error on the span. When tracing is
// disabled, the noop SpanContext is invalid and the logger is returned
// unchanged.
func StartSpanWithLogger(
	ctx context.Context,
	spanName string,
	attrs ...attribute.KeyValue,
) (context.Context, logr.Logger, func()) {
	tracer := otel.Tracer("capcs")
	ctx, span := tracer.Start(ctx, spanName, trace.WithAttributes(attrs...))

	// Strip any parent traceContextSink so nested spans don't accumulate
	// duplicate trace_id/span_id values or RecordError onto every ancestor
	// span.
	baseSink := logf.FromContext(ctx).GetSink()
	if w, ok := baseSink.(*traceContextSink); ok {
		baseSink = w.base
	}

	logger := logr.New(baseSink)
	if span.SpanContext().IsValid() {
		logger = logr.New(newTraceContextSink(baseSink, span))
	}
	ctx = logr.NewContext(ctx, logger)

	return ctx, logger, func() { span.End() }
}
