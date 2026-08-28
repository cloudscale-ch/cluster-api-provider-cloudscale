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
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/cloudscale-ch/cloudscale-go-sdk/v10/instrumentation"
	"go.opentelemetry.io/otel"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/controller"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/observability"
	webhookv1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/webhook/v1beta2"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	version  = "dev"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(infrastructurev1beta2.AddToScheme(scheme))
	_ = clusterv1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var clusterConcurrency int
	var machineConcurrency int
	var watchFilter string
	var tlsOpts []func(*tls.Config)
	var enableTracing bool
	var tracingSampleRate float64
	var profilerAddress string
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.IntVar(&clusterConcurrency, "cluster-concurrency", 1,
		"Maximum concurrent reconciles for CloudscaleCluster controller (1-4)")
	flag.IntVar(&machineConcurrency, "machine-concurrency", 1,
		"Maximum concurrent reconciles for CloudscaleMachine controller (1-10)")
	flag.StringVar(&watchFilter, "watch-filter", "",
		fmt.Sprintf("Label value that the controller watches to reconcile cluster-api objects. Label key is always %s. "+
			"If unspecified, the controller watches for all cluster-api objects.", clusterv1.WatchLabel))
	flag.BoolVar(&enableTracing, "enable-tracing", false, "Enable OpenTelemetry tracing")
	flag.Float64Var(&tracingSampleRate, "tracing-sample-rate", 0.1,
		"Trace sampling rate, between 0.0 and 1.0 (1.0 = always sample)")
	flag.StringVar(&profilerAddress, "profiler-address", "",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if clusterConcurrency < 1 || clusterConcurrency > 4 {
		return fmt.Errorf("invalid flag: --cluster-concurrency must be between 1 and 4, got %d", clusterConcurrency)
	}
	if machineConcurrency < 1 || machineConcurrency > 10 {
		return fmt.Errorf("invalid flag: --machine-concurrency must be between 1 and 10, got %d", machineConcurrency)
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("Disabling HTTP/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{
		TLSOpts: webhookTLSOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "cloudscale.infrastructure.cluster.x-k8s.io",
		PprofBindAddress:       profilerAddress,
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		return fmt.Errorf("failed to start manager: %w", err)
	}

	ctx := ctrl.SetupSignalHandler()

	if enableTracing {
		shutdown, err := observability.InitTracing(ctx, setupLog, "capcs", version, tracingSampleRate)
		if err != nil {
			return fmt.Errorf("failed to initialize tracing: %w", err)
		}
		defer shutdown()
	}

	// Wrap the transport with SDK instrumentation so all cloudscale API calls
	// emit Prometheus metrics and OpenTelemetry spans.
	//
	// The wrapped transport is shared for all cloudscale API clients to enable connection pooling and HTTP/2 multiplexing
	// across reconciles.
	instrumentedTransport := instrumentation.InstrumentedTransport(cloudscale.NewTransport(), instrumentation.Options{
		PrometheusRegistry: ctrlmetrics.Registry,
		Tracer:             otel.Tracer("cloudscale-go-sdk"),
	})

	// Fetch region information for controllers and webhooks
	regionInfo, flavorInfo, err := fetchAPIInfo(instrumentedTransport, version)
	if err != nil {
		return fmt.Errorf("failed to fetch API info: %w", err)
	}
	setupLog.Info("fetched region information", "regions", regionInfo.GetAllRegions())
	setupLog.Info("fetched flavor information", "flavors", len(flavorInfo.GetAllFlavors()))

	if err := (&controller.CloudscaleClusterReconciler{
		Client:                  mgr.GetClient(),
		Scheme:                  mgr.GetScheme(),
		WatchFilter:             watchFilter,
		Transport:               instrumentedTransport,
		Version:                 version,
		MaxConcurrentReconciles: clusterConcurrency,
	}).SetupWithManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to create controller CloudscaleCluster: %w", err)
	}
	if err := (&controller.CloudscaleMachineReconciler{
		Client:                  mgr.GetClient(),
		Scheme:                  mgr.GetScheme(),
		WatchFilter:             watchFilter,
		Transport:               instrumentedTransport,
		Version:                 version,
		MaxConcurrentReconciles: machineConcurrency,
	}).SetupWithManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to create controller CloudscaleMachine: %w", err)
	}
	if err := (&controller.CloudscaleMachineTemplateReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		FlavorInfo: flavorInfo,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to create controller CloudscaleMachineTemplate: %w", err)
	}

	webhooksEnabled := os.Getenv("ENABLE_WEBHOOKS") != "false"

	if webhooksEnabled {
		if err := webhookv1beta2.SetupCloudscaleClusterWebhookWithManager(mgr, regionInfo); err != nil {
			return fmt.Errorf("failed to setup webhook validation webhook CloudscaleCluster: %w", err)
		}
		if err := webhookv1beta2.SetupCloudscaleMachineWebhookWithManager(mgr, flavorInfo); err != nil {
			return fmt.Errorf("failed to setup webhook validation webhook CloudscaleMachine: %w", err)
		}
		if err := webhookv1beta2.SetupCloudscaleMachineTemplateWebhookWithManager(mgr, flavorInfo); err != nil {
			return fmt.Errorf("failed to setup webhook validation webhook CloudscaleMachineTemplate: %w", err)
		}
		if err := webhookv1beta2.SetupCloudscaleClusterTemplateWebhookWithManager(mgr, regionInfo); err != nil {
			return fmt.Errorf("failed to setup webhook validation webhook CloudscaleClusterTemplate: %w", err)
		}
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to set up ready check: %w", err)
	}

	setupLog.Info("Starting manager", "version", version)
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("failed to run manager: %w", err)
	}
	return nil
}

// fetchAPIInfo fetches region and flavor information from cloudscale.ch API.
// Requires CLOUDSCALE_API_TOKEN environment variable.
func fetchAPIInfo(transport http.RoundTripper, version string) (*cloudscale.RegionInfo, *cloudscale.FlavorInfo, error) {
	token := os.Getenv("CLOUDSCALE_API_TOKEN")
	if token == "" {
		return nil, nil, fmt.Errorf("CLOUDSCALE_API_TOKEN environment variable is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := cloudscale.NewClient(token, version, transport)

	var regionInfo *cloudscale.RegionInfo
	var flavorInfo *cloudscale.FlavorInfo

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		regions, err := client.Regions.List(ctx)
		if err != nil {
			return fmt.Errorf("failed to list regions: %w", err)
		}
		regionInfo = cloudscale.NewRegionInfo(regions)
		return nil
	})
	g.Go(func() error {
		flavors, err := client.Flavors.List(ctx)
		if err != nil {
			return fmt.Errorf("failed to list flavors: %w", err)
		}
		flavorInfo = cloudscale.NewFlavorInfo(flavors)
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, nil, err
	}

	return regionInfo, flavorInfo, nil
}
