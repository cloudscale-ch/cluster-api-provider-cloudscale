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

package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testenv"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
	// +kubebuilder:scaffold:imports
)

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	testEnv, cfg, k8sClient, err = testenv.StartEnvTest(
		func() error {
			return infrastructurev1beta2.AddToScheme(clientgoscheme.Scheme)
		},
		[]string{filepath.Join("..", "..", "config", "crd", "bases")},
		nil,
		filepath.Join("..", "..", "bin", "k8s"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	cancel()
	_ = testEnv.Stop()
	os.Exit(code)
}

func newTestReconciler(objs ...client.Object) *CloudscaleClusterReconciler {
	return &CloudscaleClusterReconciler{
		Client:   testutils.NewFakeClient(objs...),
		recorder: events.NewFakeRecorder(10),
	}
}
