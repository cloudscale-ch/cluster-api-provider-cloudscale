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

// StartSpanWithLogger starts a new OTel span and returns a context, logger, and
// done function. The returned logger is a composite that writes to both the
// standard logger and the span as events.
func StartSpanWithLogger(
	ctx context.Context,
	spanName string,
	attrs ...attribute.KeyValue,
) (context.Context, logr.Logger, func()) {
	tracer := otel.Tracer("capcs")
	ctx, span := tracer.Start(ctx, spanName, trace.WithAttributes(attrs...))

	baseLogger := logf.FromContext(ctx)
	sink := NewCompositeLogger(baseLogger.GetSink(), NewSpanLogSink(span))
	logger := logr.New(sink).WithName(spanName)
	ctx = logr.NewContext(ctx, logger)

	return ctx, logger, func() { span.End() }
}
