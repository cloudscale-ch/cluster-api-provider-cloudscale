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
	"errors"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// recordingSink is a logr.LogSink that captures the last Info/Error call and
// surfaces a configurable Enabled(level) result, so tests can assert what
// the wrapping sink forwarded to it. WithValues/WithName intentionally
// return the same instance (unlike a correct logr.LogSink) so that any
// derived logger still writes to the same recordingSink the test holds.
type recordingSink struct {
	enabled  func(level int) bool
	lastInfo *infoCall
	lastErr  *errorCall
}

type infoCall struct {
	level int
	msg   string
	kvs   []any
}

type errorCall struct {
	err error
	msg string
	kvs []any
}

func newRecordingSink() *recordingSink {
	return &recordingSink{enabled: func(int) bool { return true }}
}

func (r *recordingSink) Init(logr.RuntimeInfo) {}
func (r *recordingSink) Enabled(level int) bool {
	return r.enabled(level)
}
func (r *recordingSink) Info(level int, msg string, kvs ...any) {
	r.lastInfo = &infoCall{level: level, msg: msg, kvs: append([]any(nil), kvs...)}
}
func (r *recordingSink) Error(err error, msg string, kvs ...any) {
	r.lastErr = &errorCall{err: err, msg: msg, kvs: append([]any(nil), kvs...)}
}
func (r *recordingSink) WithValues(_ ...any) logr.LogSink { return r }
func (r *recordingSink) WithName(_ string) logr.LogSink   { return r }

// sdkTracer returns a tracer provider backed by the SDK with synchronous
// in-memory export, so span events/status are observable immediately after
// End().
func sdkTracer(t *testing.T) (trace.TracerProvider, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return tp, exporter
}

// globalSDKTracer registers an SDK-backed TracerProvider with synchronous
// in-memory export as the global OTel provider for the test's duration, so
// StartSpanWithLogger (which calls otel.Tracer) picks it up. Tests using it
// must not run in parallel.
func globalSDKTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	tp, exporter := sdkTracer(t)
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
	})
	return exporter
}

// countKV returns the number of times key occurs in a logr-style kvs slice
// and the first value for it (nil if absent).
func countKV(kvs []any, key string) (any, int) {
	var found any
	count := 0
	for i := 0; i+1 < len(kvs); i += 2 {
		if k, ok := kvs[i].(string); ok && k == key {
			if count == 0 {
				found = kvs[i+1]
			}
			count++
		}
	}
	return found, count
}

// expectTraceKVs asserts that kvs contains exactly one trace_id and one
// span_id, both matching span's SpanContext.
func expectTraceKVs(g Gomega, kvs []any, span trace.Span) {
	traceID, traceCount := countKV(kvs, "trace_id")
	g.Expect(traceCount).To(Equal(1), "trace_id should appear exactly once")
	g.Expect(traceID).To(Equal(span.SpanContext().TraceID().String()))

	spanID, spanCount := countKV(kvs, "span_id")
	g.Expect(spanCount).To(Equal(1), "span_id should appear exactly once")
	g.Expect(spanID).To(Equal(span.SpanContext().SpanID().String()))
}

func TestTraceContextSink_InfoAddsTraceIDs(t *testing.T) {
	g := NewWithT(t)
	tp, _ := sdkTracer(t)
	_, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	base := newRecordingSink()
	sink := newTraceContextSink(base, span)

	sink.Info(0, "hello", "k", "v")

	g.Expect(base.lastInfo).ToNot(BeNil(), "base sink should receive the Info call")
	g.Expect(base.lastInfo.msg).To(Equal("hello"))
	expectTraceKVs(g, base.lastInfo.kvs, span)

	userVal, _ := countKV(base.lastInfo.kvs, "k")
	g.Expect(userVal).To(Equal("v"), "user kv should be preserved")
}

func TestTraceContextSink_InfoNoopSpanLeavesKVsUnchanged(t *testing.T) {
	g := NewWithT(t)
	_, span := tracenoop.NewTracerProvider().Tracer("test").Start(context.Background(), "op")

	base := newRecordingSink()
	sink := newTraceContextSink(base, span)

	sink.Info(0, "hello", "k", "v")

	g.Expect(base.lastInfo).ToNot(BeNil())
	_, traceCount := countKV(base.lastInfo.kvs, "trace_id")
	_, spanCount := countKV(base.lastInfo.kvs, "span_id")
	g.Expect(traceCount).To(BeZero(), "noop span: trace_id should be absent")
	g.Expect(spanCount).To(BeZero(), "noop span: span_id should be absent")
}

func TestTraceContextSink_ErrorRecordsAndPrependsIDs(t *testing.T) {
	g := NewWithT(t)
	tp, exporter := sdkTracer(t)
	_, span := tp.Tracer("test").Start(context.Background(), "op")

	base := newRecordingSink()
	sink := newTraceContextSink(base, span)

	boom := errors.New("boom")
	sink.Error(boom, "failed", "k", "v")
	span.End()

	g.Expect(base.lastErr).ToNot(BeNil(), "base sink should receive the Error call")
	g.Expect(base.lastErr.err).To(MatchError(boom))
	g.Expect(base.lastErr.msg).To(Equal("failed"))
	expectTraceKVs(g, base.lastErr.kvs, span)

	spans := exporter.GetSpans()
	g.Expect(spans).To(HaveLen(1))
	g.Expect(spans[0].Events).To(HaveLen(1), "expected exception event on span")
	g.Expect(spans[0].Status.Code).To(Equal(codes.Error))
	g.Expect(spans[0].Status.Description).To(Equal("failed"))
}

func TestTraceContextSink_WithValuesAndWithNamePreserveSpan(t *testing.T) {
	g := NewWithT(t)
	tp, _ := sdkTracer(t)
	_, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	base := newRecordingSink()
	sink := newTraceContextSink(base, span)

	derived := sink.WithValues("scope", "x").WithName("named")
	derived.Info(0, "hi")

	w, ok := derived.(*traceContextSink)
	g.Expect(ok).To(BeTrue(), "derived sink should still be a *traceContextSink, got %T", derived)
	g.Expect(w.span).To(BeIdenticalTo(span), "derived sink should keep the original span reference")

	// The base recordingSink wrapped by the derived sink should also have
	// received the IDs through the inner Info call.
	inner, ok := w.base.(*recordingSink)
	g.Expect(ok).To(BeTrue(), "expected derived sink to still wrap a *recordingSink, got %T", w.base)
	g.Expect(inner.lastInfo).ToNot(BeNil(), "derived sink should forward Info to base")
	_, traceCount := countKV(inner.lastInfo.kvs, "trace_id")
	g.Expect(traceCount).To(Equal(1), "derived sink should still inject trace_id exactly once")
}

func TestTraceContextSink_EnabledDelegates(t *testing.T) {
	g := NewWithT(t)
	tp, _ := sdkTracer(t)
	_, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	base := newRecordingSink()
	base.enabled = func(level int) bool { return level <= 1 }
	sink := newTraceContextSink(base, span)

	g.Expect(sink.Enabled(0)).To(BeTrue())
	g.Expect(sink.Enabled(1)).To(BeTrue())
	g.Expect(sink.Enabled(2)).To(BeFalse())
}

func TestStartSpanWithLogger_SingleSpanInjectsIDsOnce(t *testing.T) {
	g := NewWithT(t)
	globalSDKTracer(t)

	base := newRecordingSink()
	ctx := logr.NewContext(context.Background(), logr.New(base))

	_, logger, done := StartSpanWithLogger(ctx, "outer")
	logger.Info("hello")
	done()

	g.Expect(base.lastInfo).ToNot(BeNil())
	_, traceCount := countKV(base.lastInfo.kvs, "trace_id")
	_, spanCount := countKV(base.lastInfo.kvs, "span_id")
	g.Expect(traceCount).To(Equal(1))
	g.Expect(spanCount).To(Equal(1))
}

func TestStartSpanWithLogger_NestedSpansEmitChildIDsOnce(t *testing.T) {
	g := NewWithT(t)
	globalSDKTracer(t)

	base := newRecordingSink()
	ctx := logr.NewContext(context.Background(), logr.New(base))

	parentCtx, parentLogger, parentDone := StartSpanWithLogger(ctx, "parent")
	parentSpan := trace.SpanFromContext(parentCtx)

	childCtx, childLogger, childDone := StartSpanWithLogger(parentCtx, "child")
	childSpan := trace.SpanFromContext(childCtx)

	childLogger.Info("from child")

	g.Expect(base.lastInfo).ToNot(BeNil(), "child logger should have written to base sink")
	expectTraceKVs(g, base.lastInfo.kvs, childSpan)

	childDone()

	// After the child ends, logging via the parent's logger uses the parent's span_id.
	base.lastInfo = nil
	parentLogger.Info("from parent")
	g.Expect(base.lastInfo).ToNot(BeNil(), "parent logger should have written to base sink")
	parentSpanID, parentCount := countKV(base.lastInfo.kvs, "span_id")
	g.Expect(parentCount).To(Equal(1))
	g.Expect(parentSpanID).To(Equal(parentSpan.SpanContext().SpanID().String()))

	parentDone()
}

func TestStartSpanWithLogger_ErrorDoesNotCascadeToParent(t *testing.T) {
	g := NewWithT(t)
	exporter := globalSDKTracer(t)

	base := newRecordingSink()
	ctx := logr.NewContext(context.Background(), logr.New(base))

	parentCtx, _, parentDone := StartSpanWithLogger(ctx, "parent")
	parentSpanID := trace.SpanFromContext(parentCtx).SpanContext().SpanID()

	_, childLogger, childDone := StartSpanWithLogger(parentCtx, "child")

	childLogger.Error(errors.New("boom"), "failed")
	childDone()
	parentDone()

	spans := exporter.GetSpans()
	g.Expect(spans).To(HaveLen(2))

	var child, parent *tracetest.SpanStub
	for i := range spans {
		if spans[i].SpanContext.SpanID() == parentSpanID {
			parent = &spans[i]
		} else {
			child = &spans[i]
		}
	}
	g.Expect(child).ToNot(BeNil(), "could not identify child span by SpanID; exported=%+v", spans)
	g.Expect(parent).ToNot(BeNil(), "could not identify parent span by SpanID; exported=%+v", spans)

	g.Expect(child.Status.Code).To(Equal(codes.Error), "child span should be marked Error")
	g.Expect(child.Events).To(HaveLen(1), "child span should carry the exception event")
	g.Expect(parent.Status.Code).To(Equal(codes.Unset), "parent span status should not cascade")
	g.Expect(parent.Events).To(BeEmpty(), "parent span events should not cascade")
}
