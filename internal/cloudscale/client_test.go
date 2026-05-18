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

package cloudscale

import (
	"fmt"
	"net/url"
	"os"
	"testing"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v9"
	. "github.com/onsi/gomega"
)

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error returns false", nil, false},
		{"non-ErrorResponse error returns false", fmt.Errorf("something went wrong"), false},
		{"ErrorResponse with 404 returns true", &cloudscalesdk.ErrorResponse{StatusCode: 404}, true},
		{"ErrorResponse with 500 returns false", &cloudscalesdk.ErrorResponse{StatusCode: 500}, false},
		{"ErrorResponse with 403 returns false", &cloudscalesdk.ErrorResponse{StatusCode: 403}, false},
		{"wrapped ErrorResponse with 404 returns true", fmt.Errorf("outer: %w", &cloudscalesdk.ErrorResponse{StatusCode: 404}), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := IsNotFound(tt.err)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error returns false", nil, false},
		{
			"url.Error with Timeout=true returns true",
			&url.Error{Op: "Post", URL: "https://api.example.com/v1/servers", Err: os.ErrDeadlineExceeded},
			true,
		},
		{
			"url.Error with Timeout=false returns false",
			&url.Error{Op: "Get", URL: "https://api.example.com/v1/servers", Err: fmt.Errorf("connection refused")},
			false,
		},
		{
			"wrapped url.Error with Timeout=true returns true",
			fmt.Errorf("outer: %w", &url.Error{Op: "Post", URL: "https://api.example.com/v1/servers", Err: os.ErrDeadlineExceeded}),
			true,
		},
		{
			"os.ErrDeadlineExceeded returns true",
			os.ErrDeadlineExceeded,
			true,
		},
		{
			"generic error returns false",
			fmt.Errorf("some other error"),
			false,
		},
		{
			"ErrorResponse with 500 returns false",
			&cloudscalesdk.ErrorResponse{StatusCode: 500},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := IsTimeoutError(tt.err)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}
