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

package main

import (
	"strings"
	"testing"
)

func TestVerifyReleaseSeries(t *testing.T) {
	tests := []struct {
		name      string
		tag       string
		metadata  string
		wantError string
	}{
		{
			name: "stable release has a matching series",
			tag:  "v1.1.0",
			metadata: `apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3
kind: Metadata
releaseSeries:
- major: 1
  minor: 1
  contract: v1beta2
`,
		},
		{
			name: "prerelease has a matching series",
			tag:  "v1.2.0-beta.1",
			metadata: `releaseSeries:
- major: 1
  minor: 2
  contract: v1beta2
`,
		},
		{
			name: "matching major with wrong minor fails",
			tag:  "v1.1.0",
			metadata: `releaseSeries:
- major: 1
  minor: 0
  contract: v1beta2
`,
			wantError: "release series 1.1",
		},
		{
			name:      "invalid tag fails",
			tag:       "dev",
			metadata:  "releaseSeries: []\n",
			wantError: "valid semantic version",
		},
		{
			name:      "invalid metadata fails",
			tag:       "v1.1.0",
			metadata:  "releaseSeries: [\n",
			wantError: "parse metadata",
		},
		{
			name:      "empty release series fails",
			tag:       "v1.0.0",
			metadata:  "releaseSeries: []\n",
			wantError: "no release series",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyReleaseSeries(tt.tag, []byte(tt.metadata))
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("verifyReleaseSeries() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("verifyReleaseSeries() error = %v, want an error containing %q", err, tt.wantError)
			}
		})
	}
}
