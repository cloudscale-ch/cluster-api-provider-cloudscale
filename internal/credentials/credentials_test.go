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

package credentials

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

func TestGetToken(t *testing.T) {

	tests := []struct {
		name      string
		secret    *corev1.Secret
		namespace string
		secretRef corev1.ObjectReference
		wantToken string
		wantErr   bool
	}{
		{
			name: "valid secret with token",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloudscale-credentials",
					Namespace: "default",
				},
				Data: map[string][]byte{
					TokenKey: []byte("test-api-token"),
				},
			},
			namespace: "default",
			secretRef: corev1.ObjectReference{
				Name:      "cloudscale-credentials",
				Namespace: "default",
			},
			wantToken: "test-api-token",
			wantErr:   false,
		},
		{
			name: "secret without token key",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloudscale-credentials",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"wrong-key": []byte("test-api-token"),
				},
			},
			namespace: "default",
			secretRef: corev1.ObjectReference{
				Name:      "cloudscale-credentials",
				Namespace: "default",
			},
			wantToken: "",
			wantErr:   true,
		},
		{
			name: "secret with empty token",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloudscale-credentials",
					Namespace: "default",
				},
				Data: map[string][]byte{
					TokenKey: []byte(""),
				},
			},
			namespace: "default",
			secretRef: corev1.ObjectReference{
				Name:      "cloudscale-credentials",
				Namespace: "default",
			},
			wantToken: "",
			wantErr:   true,
		},
		{
			name:      "secret not found",
			secret:    nil,
			namespace: "default",
			secretRef: corev1.ObjectReference{
				Name:      "missing-secret",
				Namespace: "default",
			},
			wantToken: "",
			wantErr:   true,
		},
		{
			name: "uses cluster namespace when secretRef namespace is empty",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cloudscale-credentials",
					Namespace: "my-cluster-ns",
				},
				Data: map[string][]byte{
					TokenKey: []byte("test-api-token"),
				},
			},
			namespace: "my-cluster-ns",
			secretRef: corev1.ObjectReference{
				Name:      "cloudscale-credentials",
				Namespace: "", // Empty, should use cluster namespace
			},
			wantToken: "test-api-token",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			var objs []client.Object
			if tt.secret != nil {
				objs = append(objs, tt.secret)
			}
			cl := testutils.NewFakeClient(objs...)

			token, err := GetToken(context.Background(), cl, tt.secretRef, tt.namespace)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(token).To(Equal(tt.wantToken))
		})
	}
}
