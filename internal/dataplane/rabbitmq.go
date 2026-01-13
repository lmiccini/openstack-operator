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
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetNovaCellRabbitMqUserFromSecret extracts the RabbitMQ username from a nova-cellX-compute-config secret
// Returns the username extracted from transport_url in the secret
func GetNovaCellRabbitMqUserFromSecret(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
	cellName string,
) (string, error) {
	// List all secrets in the namespace
	secretList := &corev1.SecretList{}
	err := h.GetClient().List(ctx, secretList, client.InNamespace(namespace))
	if err != nil {
		return "", fmt.Errorf("failed to list secrets: %w", err)
	}

	// Pattern to match nova-cellX-compute-config secrets
	// Supports both split secrets (nova-cell1-compute-config-0) and non-split (nova-cell1-compute-config)
	secretPattern := regexp.MustCompile(`^nova-(` + cellName + `)-compute-config(-\d+)?$`)

	for _, secret := range secretList.Items {
		matches := secretPattern.FindStringSubmatch(secret.Name)
		if matches == nil {
			continue
		}

		// Extract transport_url from secret data
		transportURLBytes, ok := secret.Data["transport_url"]
		if !ok {
			// Try to extract from config files as fallback (in case it's embedded)
			// Check both custom.conf and 01-nova.conf
			for _, configKey := range []string{"custom.conf", "01-nova.conf"} {
				customConfig, hasCustom := secret.Data[configKey]
				if !hasCustom {
					continue
				}
				// Try to extract from custom config
				username := extractUsernameFromCustomConfig(string(customConfig))
				if username != "" {
					return username, nil
				}
			}
			continue
		}

		// Parse transport_url to extract username
		username, err := parseUsernameFromTransportURL(string(transportURLBytes))
		if err != nil {
			h.GetLogger().Info("Failed to parse transport_url", "secret", secret.Name, "error", err)
			continue
		}

		if username != "" {
			return username, nil
		}
	}

	return "", fmt.Errorf("no RabbitMQ username found for cell %s", cellName)
}

// parseUsernameFromTransportURL extracts the username from a RabbitMQ transport URL
// Format: rabbit://username:password@host:port/vhost
// Also supports: rabbit+tls://username:password@host1:port1,host2:port2/vhost
func parseUsernameFromTransportURL(transportURL string) (string, error) {
	// Handle empty URLs
	if transportURL == "" {
		return "", fmt.Errorf("empty transport URL")
	}

	// Parse the URL
	// First, replace rabbit:// or rabbit+tls:// with http:// for URL parsing
	tempURL := strings.Replace(transportURL, "rabbit://", "http://", 1)
	tempURL = strings.Replace(tempURL, "rabbit+tls://", "http://", 1)

	parsedURL, err := url.Parse(tempURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	// Extract username from UserInfo
	if parsedURL.User == nil {
		return "", fmt.Errorf("no user info in transport URL")
	}

	username := parsedURL.User.Username()
	if username == "" {
		return "", fmt.Errorf("empty username in transport URL")
	}

	return username, nil
}

// extractUsernameFromCustomConfig attempts to extract RabbitMQ username from custom config
// This is a fallback for cases where transport_url is embedded in the config file
func extractUsernameFromCustomConfig(customConfig string) string {
	// Look for transport_url in the config
	// Format: transport_url = rabbit://username:password@...
	transportURLPattern := regexp.MustCompile(`transport_url\s*=\s*rabbit[^:]*://([^:]+):`)
	matches := transportURLPattern.FindStringSubmatch(customConfig)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
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
