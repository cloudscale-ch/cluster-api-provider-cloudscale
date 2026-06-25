//go:build e2e

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

package e2e

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	ctrl "sigs.k8s.io/controller-runtime"

	infrav1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
)

var (
	// Test suite configuration
	ctx                      = context.Background()
	e2eConfig                *clusterctl.E2EConfig
	clusterctlConfigPath     string
	bootstrapClusterProvider bootstrap.ClusterProvider
	bootstrapClusterProxy    framework.ClusterProxy

	// cloudscale API client and resource snapshot for leak detection
	cloudscaleClient *cloudscale.Client
	preTestSnapshot  *resourceSnapshot

	// Command line flags
	configPath         string
	artifactFolder     string
	skipCleanup        bool
	useExistingCluster bool

	// Scheme for the test
	scheme = runtime.NewScheme()
)

func init() {
	flag.StringVar(&configPath, "e2e.config", "", "Path to the e2e config file")
	flag.StringVar(&artifactFolder, "e2e.artifacts-folder", "", "Folder where test artifacts should be stored")
	flag.BoolVar(&skipCleanup, "e2e.skip-resource-cleanup", false, "If true, the resource cleanup after tests will be skipped")
	flag.BoolVar(&useExistingCluster, "e2e.use-existing-cluster", false, "If true, use an existing cluster for e2e tests")

	// Register schemes
	_ = clientgoscheme.AddToScheme(scheme) // Standard k8s types (apps/v1, core/v1, etc.)
	_ = clusterv1.AddToScheme(scheme)
	_ = infrav1beta2.AddToScheme(scheme)
	_ = bootstrapv1.AddToScheme(scheme)
	_ = controlplanev1.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)
	_ = clusterctlv1.AddToScheme(scheme)
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	ctrl.SetLogger(klog.Background())

	RunSpecs(t, "cluster-api-provider-cloudscale e2e suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	// This runs only on the first Ginkgo node
	Expect(configPath).To(BeAnExistingFile(), "E2E config file is required: --e2e.config=<path>")

	By("Loading e2e config")
	e2eConfig = clusterctl.LoadE2EConfig(ctx, clusterctl.LoadE2EConfigInput{
		ConfigPath: configPath,
	})
	Expect(e2eConfig).NotTo(BeNil(), "Failed to load e2e config")

	By("Validating required environment variables")
	apiToken := os.Getenv("CLOUDSCALE_API_TOKEN")
	Expect(apiToken).NotTo(BeEmpty(), "CLOUDSCALE_API_TOKEN environment variable is required")

	// Add secrets to e2eConfig variables so CreateRepository includes them in clusterctl.yaml
	e2eConfig.Variables["CLOUDSCALE_API_TOKEN"] = apiToken

	sshKey := os.Getenv("CLOUDSCALE_SSH_PUBLIC_KEY")
	Expect(sshKey).NotTo(BeEmpty(), "CLOUDSCALE_SSH_PUBLIC_KEY environment variable is required")
	e2eConfig.Variables["CLOUDSCALE_SSH_PUBLIC_KEY"] = sshKey

	cloudscaleClient = newCloudscaleClient(apiToken)

	// Optional: Pre-existing network for private networking tests.
	// If not set, tests requiring a pre-existing network will be skipped.
	if networkUUID := os.Getenv("CLOUDSCALE_NETWORK_UUID"); networkUUID != "" {
		e2eConfig.Variables["CLOUDSCALE_NETWORK_UUID"] = networkUUID
	}

	By("Taking pre-test snapshot of cloudscale infrastructure resources")
	var err error
	preTestSnapshot, err = takeResourceSnapshot(ctx, cloudscaleClient)
	Expect(err).NotTo(HaveOccurred(), "Failed to snapshot cloudscale resources")

	By("Setting up artifacts folder")
	if artifactFolder == "" {
		artifactFolder = filepath.Join(os.TempDir(), "capcs-e2e-artifacts")
	}
	Expect(os.MkdirAll(artifactFolder, 0750)).To(Succeed())

	By("Creating a clusterctl local repository")
	clusterctlConfigPath = clusterctl.CreateRepository(ctx, clusterctl.CreateRepositoryInput{
		E2EConfig:        e2eConfig,
		RepositoryFolder: filepath.Join(artifactFolder, "repository"),
	})

	By("Setting up bootstrap cluster")
	bootstrapClusterProvider, bootstrapClusterProxy = setupBootstrapCluster(e2eConfig, scheme, useExistingCluster)

	By("Initializing management cluster with providers")
	clusterctl.InitManagementClusterAndWatchControllerLogs(ctx,
		clusterctl.InitManagementClusterAndWatchControllerLogsInput{
			ClusterProxy:            bootstrapClusterProxy,
			ClusterctlConfigPath:    clusterctlConfigPath,
			InfrastructureProviders: e2eConfig.InfrastructureProviders(),
			// CoreProvider, BootstrapProviders, ControlPlaneProviders use defaults (cluster-api, kubeadm, kubeadm)
			// If providers are already installed (use-existing-cluster), init is skipped automatically
			LogFolder: filepath.Join(artifactFolder, "clusters", bootstrapClusterProxy.GetName()),
		},
		e2eConfig.GetIntervals("", "wait-controllers")...)

	return []byte(bootstrapClusterProxy.GetKubeconfigPath())
}, func(data []byte) {
	// This runs on all Ginkgo nodes
	Expect(configPath).To(BeAnExistingFile(), "E2E config file is required")

	e2eConfig = clusterctl.LoadE2EConfig(ctx, clusterctl.LoadE2EConfigInput{
		ConfigPath: configPath,
	})
	Expect(e2eConfig).NotTo(BeNil())

	// Re-inject env-only variables lost when LoadE2EConfig overwrites e2eConfig.
	if networkUUID := os.Getenv("CLOUDSCALE_NETWORK_UUID"); networkUUID != "" {
		e2eConfig.Variables["CLOUDSCALE_NETWORK_UUID"] = networkUUID
	}

	if artifactFolder == "" {
		artifactFolder = filepath.Join(os.TempDir(), "capcs-e2e-artifacts")
	}

	kubeconfigPath := string(data)
	Expect(kubeconfigPath).ToNot(BeEmpty(), "Kubeconfig path was not passed from the first node")
	bootstrapClusterProxy = framework.NewClusterProxy("bootstrap", kubeconfigPath, scheme,
		framework.WithMachineLogCollector(CloudscaleLogCollector{}),
	)
})

var _ = SynchronizedAfterSuite(func() {
	// This runs on all Ginkgo nodes - nothing to do here
}, func() {
	// This runs only on the first Ginkgo node
	if !skipCleanup && cloudscaleClient != nil && preTestSnapshot != nil {
		By("Checking for leaked cloudscale infrastructure resources")
		Expect(checkForLeakedResources(ctx, cloudscaleClient, preTestSnapshot)).To(Succeed(),
			"Infrastructure resources leaked during test run")
	}

	By("Tearing down the management cluster")
	if !skipCleanup && bootstrapClusterProvider != nil {
		bootstrapClusterProvider.Dispose(ctx)
	}
})

// setupBootstrapCluster creates or uses an existing bootstrap cluster
func setupBootstrapCluster(config *clusterctl.E2EConfig, scheme *runtime.Scheme, useExisting bool) (bootstrap.ClusterProvider, framework.ClusterProxy) {
	var clusterProvider bootstrap.ClusterProvider
	var clusterProxy framework.ClusterProxy

	if useExisting {
		By("Using existing cluster")
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			kubeconfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}
		clusterProxy = framework.NewClusterProxy("bootstrap", kubeconfigPath, scheme,
			framework.WithMachineLogCollector(CloudscaleLogCollector{}),
		)
	} else {
		By("Creating a Kind bootstrap cluster")
		clusterProvider = bootstrap.CreateKindBootstrapClusterAndLoadImages(ctx,
			bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
				Name:               config.ManagementClusterName,
				RequiresDockerSock: true,
				Images:             config.Images,
			})
		Expect(clusterProvider).NotTo(BeNil(), "Failed to create Kind cluster")

		kubeconfigPath := clusterProvider.GetKubeconfigPath()
		Expect(kubeconfigPath).To(BeAnExistingFile(), "Kubeconfig should exist")

		clusterProxy = framework.NewClusterProxy("bootstrap", kubeconfigPath, scheme,
			framework.WithMachineLogCollector(CloudscaleLogCollector{}),
		)
	}

	return clusterProvider, clusterProxy
}
