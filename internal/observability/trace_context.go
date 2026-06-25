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
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// traceContextSink wraps a base logr.LogSink and adds trace_id and
// span_id to every log line so logs can be correlated to traces.
// The sink also records Error logs on the
// associated span (RecordError + SetStatus); this gives failed reconciles a
// failed span in the trace UI without callers having to do anything special.
//
// trace_id and span_id are injected at log time rather than via WithValues
// so that nested StartSpanWithLogger calls don't accumulate duplicate keys.
type traceContextSink struct {
	base logr.LogSink
	span trace.Span
}

var _ logr.LogSink = &traceContextSink{}

func newTraceContextSink(base logr.LogSink, span trace.Span) logr.LogSink {
	return &traceContextSink{base: base, span: span}
}

// Init implements logr.LogSink. base is already initialized by the original
// logr.New call that produced it; logr's contract is that Init runs at most
// once per sink, so we do not forward.
func (s *traceContextSink) Init(logr.RuntimeInfo) {}

// Enabled implements logr.LogSink.
func (s *traceContextSink) Enabled(level int) bool { return s.base.Enabled(level) }

// Info implements logr.LogSink.
func (s *traceContextSink) Info(level int, msg string, keysAndValues ...any) {
	s.base.Info(level, msg, s.withTraceKVs(keysAndValues)...)
}

// Error implements logr.LogSink, additionally recording the error on the span
// and marking the span status as Error.
func (s *traceContextSink) Error(err error, msg string, keysAndValues ...any) {
	s.span.RecordError(err)
	s.span.SetStatus(codes.Error, msg)
	s.base.Error(err, msg, s.withTraceKVs(keysAndValues)...)
}

// WithValues implements logr.LogSink.
func (s *traceContextSink) WithValues(keysAndValues ...any) logr.LogSink {
	return &traceContextSink{base: s.base.WithValues(keysAndValues...), span: s.span}
}

// WithName implements logr.LogSink.
func (s *traceContextSink) WithName(name string) logr.LogSink {
	return &traceContextSink{base: s.base.WithName(name), span: s.span}
}

// withTraceKVs prepends trace_id and span_id keys to the supplied k/v pairs.
func (s *traceContextSink) withTraceKVs(kvs []any) []any {
	sc := s.span.SpanContext()
	if !sc.IsValid() {
		return kvs
	}
	out := make([]any, 0, len(kvs)+4)
	out = append(out, "trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String())
	out = append(out, kvs...)
	return out
}
