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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// spanLogSink is a logr.LogSink implementation that writes log data to an
// OpenTelemetry span as events.
type spanLogSink struct {
	span trace.Span
	name string
	vals []any
}

// Init implements logr.LogSink.
func (s *spanLogSink) Init(_ logr.RuntimeInfo) {}

// Enabled implements logr.LogSink.
func (s *spanLogSink) Enabled(_ int) bool { return true }

// Info implements logr.LogSink, writing an event to the span.
func (s *spanLogSink) Info(_ int, msg string, keysAndValues ...any) {
	attrs := kvsToAttrs(append(s.vals, keysAndValues...)...)
	s.span.AddEvent(
		fmt.Sprintf("[INFO | %s] %s", s.name, msg),
		trace.WithTimestamp(time.Now()),
		trace.WithAttributes(attrs...),
	)
}

// Error implements logr.LogSink, recording the error and writing an event to
// the span.
func (s *spanLogSink) Error(err error, msg string, keysAndValues ...any) {
	attrs := kvsToAttrs(append(s.vals, keysAndValues...)...)
	s.span.RecordError(err)
	s.span.AddEvent(
		fmt.Sprintf("[ERROR | %s] %s (%s)", s.name, msg, err),
		trace.WithTimestamp(time.Now()),
		trace.WithAttributes(attrs...),
	)
}

// WithValues implements logr.LogSink.
func (s spanLogSink) WithValues(keysAndValues ...any) logr.LogSink {
	vals := make([]any, len(s.vals)+len(keysAndValues))
	copy(vals, s.vals)
	copy(vals[len(s.vals):], keysAndValues)
	s.vals = vals
	return &s
}

// WithName implements logr.LogSink.
func (s spanLogSink) WithName(name string) logr.LogSink {
	s.name = name
	return &s
}

// NewSpanLogSink returns a LogSink that writes log events to the given span.
func NewSpanLogSink(span trace.Span) logr.LogSink {
	return &spanLogSink{span: span}
}

// kvsToAttrs converts key-value pairs (from a logr call) to OTel attributes.
func kvsToAttrs(kvs ...any) []attribute.KeyValue {
	var attrs []attribute.KeyValue
	for i := 0; i+1 < len(kvs); i += 2 {
		k := fmt.Sprint(kvs[i])
		v := fmt.Sprint(kvs[i+1])
		attrs = append(attrs, attribute.String(k, v))
	}
	return attrs
}
