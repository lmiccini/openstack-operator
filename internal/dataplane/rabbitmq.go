/*
Copyright 2023.

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

package deployment

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetNovaCellRabbitMqUserFromSecret extracts the RabbitMQUser CR name from a nova-cellX secret
// Returns the RabbitMQUser CR name, or empty string if using default RabbitMQ user
func GetNovaCellRabbitMqUserFromSecret(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
	cellName string,
) (string, error) {
	// The nova-operator creates a secret named "nova-<cellName>" for each cell
	// This secret contains rabbitmq_user_name which is the name of the RabbitMQUser CR
	secretName := fmt.Sprintf("nova-%s", cellName)

	secret := &corev1.Secret{}
	err := h.GetClient().Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      secretName,
	}, secret)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return "", fmt.Errorf("nova cell secret %s not found for cell %s", secretName, cellName)
		}
		return "", fmt.Errorf("failed to get nova cell secret %s: %w", secretName, err)
	}

	// Extract the RabbitMQUser CR name from the secret
	// If the field is not present or empty, it means using default RabbitMQ user (no dedicated RabbitMQUser CR)
	rabbitmqUserNameBytes, ok := secret.Data["rabbitmq_user_name"]
	if !ok {
		return "", fmt.Errorf("rabbitmq_user_name not found in secret %s - nova-operator may need to be updated", secretName)
	}

	rabbitmqUserName := string(rabbitmqUserNameBytes)
	// Empty string is valid - it means using default RabbitMQ user without a dedicated CR
	return rabbitmqUserName, nil
}

// GetNovaComputeConfigCellNames returns a list of cell names from nova-cellX-compute-config secrets
// referenced in the NodeSet's dataSources
func GetNovaComputeConfigCellNames(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
) ([]string, error) {
	// List all secrets in the namespace
	secretList := &corev1.SecretList{}
	err := h.GetClient().List(ctx, secretList, client.InNamespace(namespace))
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	cellNames := []string{}
	// Pattern to match nova-cellX-compute-config secrets
	secretPattern := regexp.MustCompile(`^nova-(cell\d+)-compute-config(-\d+)?$`)

	for _, secret := range secretList.Items {
		matches := secretPattern.FindStringSubmatch(secret.Name)
		if matches == nil {
			continue
		}

		cellName := matches[1] // Extract cell name (e.g., "cell1")
		// Avoid duplicates
		found := false
		for _, cn := range cellNames {
			if cn == cellName {
				found = true
				break
			}
		}
		if !found {
			cellNames = append(cellNames, cellName)
		}
	}

	return cellNames, nil
}

// ExtractCellNameFromSecretName extracts the cell name from a secret name
// Example: "nova-cell1-compute-config" -> "cell1"
// Example: "nova-cell1-compute-config-0" -> "cell1"
func ExtractCellNameFromSecretName(secretName string) string {
	pattern := regexp.MustCompile(`nova-(cell\d+)-compute-config`)
	matches := pattern.FindStringSubmatch(secretName)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// GetNovaCellSecretsLastModified returns a map of nova-cellX-compute-config secret names
// to their last modification timestamps
func GetNovaCellSecretsLastModified(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
) (map[string]time.Time, error) {
	// List all secrets in the namespace
	secretList := &corev1.SecretList{}
	err := h.GetClient().List(ctx, secretList, client.InNamespace(namespace))
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	secretTimes := make(map[string]time.Time)
	// Pattern to match nova-cellX-compute-config secrets
	secretPattern := regexp.MustCompile(`^nova-(cell\d+)-compute-config(-\d+)?$`)

	for _, secret := range secretList.Items {
		matches := secretPattern.FindStringSubmatch(secret.Name)
		if matches == nil {
			continue
		}

		// Use the resource version change time if available, otherwise creation time
		modTime := secret.CreationTimestamp.Time
		if secret.ManagedFields != nil {
			for _, field := range secret.ManagedFields {
				if field.Time != nil && field.Time.After(modTime) {
					modTime = field.Time.Time
				}
			}
		}

		secretTimes[secret.Name] = modTime
	}

	return secretTimes, nil
}
