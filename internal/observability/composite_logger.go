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

import "github.com/go-logr/logr"

// compositeLogSink is a logr.LogSink that multiplexes calls to multiple
// underlying sinks.
type compositeLogSink struct {
	sinks []logr.LogSink
}

// Init implements logr.LogSink.
func (c *compositeLogSink) Init(info logr.RuntimeInfo) {
	for _, s := range c.sinks {
		s.Init(info)
	}
}

// Enabled implements logr.LogSink. It returns true if any underlying sink is
// enabled.
func (c *compositeLogSink) Enabled(level int) bool {
	for _, s := range c.sinks {
		if s.Enabled(level) {
			return true
		}
	}
	return false
}

// Info implements logr.LogSink.
func (c *compositeLogSink) Info(level int, msg string, keysAndValues ...any) {
	for _, s := range c.sinks {
		s.Info(level, msg, keysAndValues...)
	}
}

// Error implements logr.LogSink.
func (c *compositeLogSink) Error(err error, msg string, keysAndValues ...any) {
	for _, s := range c.sinks {
		s.Error(err, msg, keysAndValues...)
	}
}

// WithValues implements logr.LogSink.
func (c *compositeLogSink) WithValues(keysAndValues ...any) logr.LogSink {
	newSinks := make([]logr.LogSink, len(c.sinks))
	for i, s := range c.sinks {
		newSinks[i] = s.WithValues(keysAndValues...)
	}
	return &compositeLogSink{sinks: newSinks}
}

// WithName implements logr.LogSink.
func (c *compositeLogSink) WithName(name string) logr.LogSink {
	newSinks := make([]logr.LogSink, len(c.sinks))
	for i, s := range c.sinks {
		newSinks[i] = s.WithName(name)
	}
	return &compositeLogSink{sinks: newSinks}
}

// NewCompositeLogger returns a LogSink that forwards calls to all provided
// sinks.
func NewCompositeLogger(sinks ...logr.LogSink) logr.LogSink {
	return &compositeLogSink{sinks: sinks}
}
