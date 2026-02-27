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

// Package credentials handles cloudscale.ch API credential retrieval.
package credentials

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TokenKey is the key in the Secret data that holds the API token.
const TokenKey = "token"

// GetToken retrieves the cloudscale.ch API token from a Secret.
// If secretRef.Namespace is empty, clusterNamespace is used.
func GetToken(ctx context.Context, c client.Client, secretRef corev1.ObjectReference, clusterNamespace string) (string, error) {
	namespace := secretRef.Namespace
	if namespace == "" {
		namespace = clusterNamespace
	}

	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: secretRef.Name, Namespace: namespace}

	if err := c.Get(ctx, key, secret); err != nil {
		return "", fmt.Errorf("failed to get credentials secret %s/%s: %w", namespace, secretRef.Name, err)
	}

	token, ok := secret.Data[TokenKey]
	if !ok {
		return "", fmt.Errorf("secret %s/%s does not contain key %q", namespace, secretRef.Name, TokenKey)
	}

	tokenStr := string(token)
	if tokenStr == "" {
		return "", fmt.Errorf("secret %s/%s has empty token", namespace, secretRef.Name)
	}

	return tokenStr, nil
}
