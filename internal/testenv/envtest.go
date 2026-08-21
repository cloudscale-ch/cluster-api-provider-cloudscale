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

package testenv

import (
	"fmt"
	"os"
	"path/filepath"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// StartEnvTest creates and starts an envtest.Environment with the given CRD and
// webhook paths, and returns the environment, REST config, and controller-runtime client.
//
// The caller is responsible for calling env.Stop() in a cleanup hook.
func StartEnvTest(
	schemeAddFn func() error,
	crdPaths []string,
	webhookPaths []string,
	binBase string,
) (*envtest.Environment, *rest.Config, client.Client, error) {
	logf.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(true)))

	if err := schemeAddFn(); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to add scheme: %w", err)
	}

	env := &envtest.Environment{
		CRDDirectoryPaths:     crdPaths,
		ErrorIfCRDPathMissing: true,
	}

	if len(webhookPaths) > 0 {
		env.WebhookInstallOptions = envtest.WebhookInstallOptions{
			Paths: webhookPaths,
		}
	}

	if binDir := FirstFoundEnvTestBinaryDir(binBase); binDir != "" {
		env.BinaryAssetsDirectory = binDir
	}

	cfg, err := env.Start()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to start envtest: %w", err)
	}
	if cfg == nil {
		return nil, nil, nil, fmt.Errorf("envtest config is nil")
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: clientgoscheme.Scheme})
	if err != nil {
		_ = env.Stop()
		return nil, nil, nil, fmt.Errorf("failed to create client: %w", err)
	}

	return env, cfg, k8sClient, nil
}

// FirstFoundEnvTestBinaryDir locates the first binary-version directory under basePath.
// This replicates the KUBEBUILDER_ASSETS lookup so that IDE test runs work.
func FirstFoundEnvTestBinaryDir(basePath string) string {
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read envtest binary directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
