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

package v1beta2

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testenv"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
	// +kubebuilder:scaffold:imports
)

var (
	ctx       context.Context
	cancel    context.CancelFunc
	k8sClient client.Client
	cfg       *rest.Config
	testEnv   *envtest.Environment
)

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	testEnv, cfg, k8sClient, err = testenv.StartEnvTest(
		func() error {
			return infrastructurev1beta2.AddToScheme(scheme.Scheme)
		},
		[]string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		[]string{filepath.Join("..", "..", "..", "config", "webhook")},
		filepath.Join("..", "..", "..", "bin", "k8s"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	// start webhook server using Manager.
	webhookInstallOptions := &testEnv.WebhookInstallOptions
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    webhookInstallOptions.LocalServingHost,
			Port:    webhookInstallOptions.LocalServingPort,
			CertDir: webhookInstallOptions.LocalServingCertDir,
		}),
		LeaderElection: false,
		Metrics:        metricsserver.Options{BindAddress: "0"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create manager: %v\n", err)
		os.Exit(1)
	}

	err = SetupCloudscaleClusterWebhookWithManager(mgr, testutils.NewTestRegionInfo())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup cluster webhook: %v\n", err)
		os.Exit(1)
	}

	err = SetupCloudscaleMachineWebhookWithManager(mgr, testutils.NewTestFlavorInfo())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup machine webhook: %v\n", err)
		os.Exit(1)
	}

	err = SetupCloudscaleMachineTemplateWebhookWithManager(mgr, testutils.NewTestFlavorInfo())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup machine template webhook: %v\n", err)
		os.Exit(1)
	}

	// +kubebuilder:scaffold:webhook

	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Manager exited with error: %v\n", err)
		}
	}()

	// wait for the webhook server to get ready.
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec
		if dialErr == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	code := m.Run()

	cancel()
	_ = testEnv.Stop()
	os.Exit(code)
}
