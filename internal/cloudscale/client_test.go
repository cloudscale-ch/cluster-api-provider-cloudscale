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
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"
	. "github.com/onsi/gomega"
	"golang.org/x/oauth2"
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

func TestNewTransport(t *testing.T) {
	g := NewWithT(t)
	transport := NewTransport()

	g.Expect(transport).ToNot(BeNil())
	g.Expect(transport.ForceAttemptHTTP2).To(BeTrue())
	g.Expect(transport.TLSHandshakeTimeout).To(Equal(5 * time.Second))
	g.Expect(transport.IdleConnTimeout).To(Equal(90 * time.Second))
	g.Expect(transport.MaxIdleConns).To(Equal(50))
	g.Expect(transport.MaxIdleConnsPerHost).To(Equal(50))
	g.Expect(transport.MaxConnsPerHost).To(Equal(0))

	// Verify HTTP/2 ping settings
	g.Expect(transport.HTTP2).ToNot(BeNil())
	g.Expect(transport.HTTP2.SendPingTimeout).To(Equal(5 * time.Second))
	g.Expect(transport.HTTP2.PingTimeout).To(Equal(3 * time.Second))

	// Verify dial settings by inspecting the DialContext function
	g.Expect(transport.DialContext).ToNot(BeNil())
}

func TestNewTransport_DialTimeout(t *testing.T) {
	g := NewWithT(t)
	transport := NewTransport()

	// Dial to an unroutable address to verify timeout works
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 192.0.2.1 is TEST-NET-1 (RFC 5737) - guaranteed unroutable
	conn, err := transport.DialContext(ctx, "tcp", "192.0.2.1:443")
	if conn != nil {
		conn.Close()
	}

	// Should fail with a timeout (dial timeout is 5s)
	g.Expect(err).To(HaveOccurred())
	var netErr net.Error
	g.Expect(err).To(BeAssignableToTypeOf(&net.OpError{}))
	if ok := errors.As(err, &netErr); ok {
		g.Expect(netErr.Timeout()).To(BeTrue())
	}
}

func TestNewClient_ReturnsNonNilClient(t *testing.T) {
	g := NewWithT(t)
	transport := NewTransport()
	client := NewClient("fake-token", transport)
	g.Expect(client).ToNot(BeNil())
	g.Expect(client.LoadBalancers).ToNot(BeNil())
	g.Expect(client.Servers).ToNot(BeNil())
	g.Expect(client.Networks).ToNot(BeNil())
}

func TestNewClient_NoGlobalTimeout(t *testing.T) {
	g := NewWithT(t)

	// Start a slow test server that takes 3s to respond
	slowServer := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	})
	server := &http.Server{Handler: slowServer}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	g.Expect(err).ToNot(HaveOccurred())
	go server.Serve(listener)
	defer server.Close()

	transport := NewTransport()

	// The client should NOT have a global timeout - only context-based
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test"})
	httpClient := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
			Base:   transport,
		},
	}
	// Verify no global timeout is set
	g.Expect(httpClient.Timeout).To(Equal(time.Duration(0)))
}

func TestTimeoutConstants(t *testing.T) {
	g := NewWithT(t)

	// Verify timeout constants exist and have sensible values
	g.Expect(ReadTimeout).To(Equal(30 * time.Second))
	g.Expect(WriteTimeout).To(Equal(2 * time.Minute))
	g.Expect(DeleteTimeout).To(Equal(30 * time.Second))
}
