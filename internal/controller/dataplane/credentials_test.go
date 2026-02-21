/*

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

package dataplane

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	dataplanev1 "github.com/openstack-k8s-operators/openstack-operator/api/dataplane/v1beta1"
)

func TestDetectRabbitMQSecretsInDeployment(t *testing.T) {
	tests := []struct {
		name        string
		deployment  *dataplanev1.OpenStackDataPlaneDeployment
		secrets     []corev1.Secret
		wantSecrets []string // Expected rabbitmq-user-* secret names
	}{
		{
			name:        "nil deployment returns empty",
			deployment:  nil,
			secrets:     []corev1.Secret{},
			wantSecrets: []string{},
		},
		{
			name: "detect rabbitmq-user from transport_url in config",
			deployment: &dataplanev1.OpenStackDataPlaneDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test",
				},
				Status: dataplanev1.OpenStackDataPlaneDeploymentStatus{
					SecretHashes: map[string]string{
						"nova-cell1-compute-config": "hash123",
					},
				},
			},
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nova-cell1-compute-config",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"01-nova.conf": []byte("transport_url=rabbit://novacell1-11:password@host:5671/?ssl=1"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rabbitmq-user-nova-cell1-transport-novacell1-11-user",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"username": []byte("novacell1-11"),
						"password": []byte("password123"),
					},
				},
			},
			wantSecrets: []string{"rabbitmq-user-nova-cell1-transport-novacell1-11-user"},
		},
		{
			name: "skip non-transport_url secrets",
			deployment: &dataplanev1.OpenStackDataPlaneDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test",
				},
				Status: dataplanev1.OpenStackDataPlaneDeploymentStatus{
					SecretHashes: map[string]string{
						"nova-cell1-compute-config": "hash1",
						"cert-my-cert":              "hash2",
					},
				},
			},
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nova-cell1-compute-config",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"01-nova.conf": []byte("transport_url=rabbit://novacell1-11:password@host:5671/?ssl=1"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cert-my-cert",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"cert.pem": []byte("certificate data"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rabbitmq-user-nova-cell1-transport-novacell1-11-user",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"username": []byte("novacell1-11"),
						"password": []byte("password123"),
					},
				},
			},
			wantSecrets: []string{"rabbitmq-user-nova-cell1-transport-novacell1-11-user"},
		},
		{
			name: "detect multiple rabbitmq-user from multiple configs",
			deployment: &dataplanev1.OpenStackDataPlaneDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test",
				},
				Status: dataplanev1.OpenStackDataPlaneDeploymentStatus{
					SecretHashes: map[string]string{
						"nova-cell1-compute-config":         "hash1",
						"neutron-ovn-metadata-agent-config": "hash2",
					},
				},
			},
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nova-cell1-compute-config",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"01-nova.conf": []byte("transport_url=rabbit://novacell1-11:password@host:5671/?ssl=1"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "neutron-ovn-metadata-agent-config",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"neutron.conf": []byte("transport_url=rabbit://neutron-5:password@host:5671/?ssl=1"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rabbitmq-user-nova-cell1-transport-novacell1-11-user",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"username": []byte("novacell1-11"),
						"password": []byte("password123"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rabbitmq-user-neutron-transport-neutron-5-user",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"username": []byte("neutron-5"),
						"password": []byte("password456"),
					},
				},
			},
			wantSecrets: []string{
				"rabbitmq-user-nova-cell1-transport-novacell1-11-user",
				"rabbitmq-user-neutron-transport-neutron-5-user",
			},
		},
		{
			name: "skip if rabbitmq-user secret not found",
			deployment: &dataplanev1.OpenStackDataPlaneDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test",
				},
				Status: dataplanev1.OpenStackDataPlaneDeploymentStatus{
					SecretHashes: map[string]string{
						"nova-cell1-compute-config": "hash123",
					},
				},
			},
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nova-cell1-compute-config",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"01-nova.conf": []byte("transport_url=rabbit://novacell1-11:password@host:5671/?ssl=1"),
					},
				},
				// rabbitmq-user secret doesn't exist
			},
			wantSecrets: []string{}, // Should skip gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create scheme and add types
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			_ = dataplanev1.AddToScheme(scheme)

			// Create objects for fake client
			objs := []runtime.Object{}
			if tt.deployment != nil {
				objs = append(objs, tt.deployment)
			}
			for i := range tt.secrets {
				objs = append(objs, &tt.secrets[i])
			}

			// Create fake client
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objs...).
				Build()

			// Create reconciler
			reconciler := &OpenStackDataPlaneNodeSetReconciler{
				Client: fakeClient,
			}

			// Call detectRabbitMQSecretsInDeployment
			secrets, err := reconciler.detectRabbitMQSecretsInDeployment(context.TODO(), tt.deployment, "test")
			if err != nil {
				t.Fatalf("detectRabbitMQSecretsInDeployment() error = %v", err)
			}

			// Check we got expected number of secrets
			if len(secrets) != len(tt.wantSecrets) {
				t.Errorf("detectRabbitMQSecretsInDeployment() returned %d secrets, want %d", len(secrets), len(tt.wantSecrets))
			}

			// Check each expected secret exists
			for _, wantSecret := range tt.wantSecrets {
				if _, found := secrets[wantSecret]; !found {
					t.Errorf("detectRabbitMQSecretsInDeployment() missing secret %q", wantSecret)
				}
			}

			// Check no unexpected secrets
			for secretName := range secrets {
				found := false
				for _, wantSecret := range tt.wantSecrets {
					if secretName == wantSecret {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("detectRabbitMQSecretsInDeployment() returned unexpected secret %q", secretName)
				}
			}

			// Verify hashes are computed correctly
			for secretName, secretInfo := range secrets {
				// Find the corresponding secret
				var secret *corev1.Secret
				for i := range tt.secrets {
					if tt.secrets[i].Name == secretName {
						secret = &tt.secrets[i]
						break
					}
				}
				if secret == nil {
					continue
				}

				// Compute expected hash
				expectedHash, err := util.ObjectHash(secret.Data)
				if err != nil {
					t.Fatalf("failed to compute expected hash: %v", err)
				}

				if secretInfo.secretHash != expectedHash {
					t.Errorf("secret %q has hash %q, want %q", secretName, secretInfo.secretHash, expectedHash)
				}
			}
		})
	}
}

func TestGetNodesCoveredByDeployment(t *testing.T) {
	tests := []struct {
		name            string
		ansibleLimit    string
		nodesetNodes    []string
		expectedCovered []string
	}{
		{
			name:            "nil nodeset returns empty",
			ansibleLimit:    "",
			nodesetNodes:    nil,
			expectedCovered: []string{},
		},
		{
			name:            "empty AnsibleLimit returns all nodes",
			ansibleLimit:    "",
			nodesetNodes:    []string{"compute-0", "compute-1"},
			expectedCovered: []string{"compute-0", "compute-1"},
		},
		{
			name:            "wildcard returns all nodes",
			ansibleLimit:    "*",
			nodesetNodes:    []string{"compute-0", "compute-1", "compute-2"},
			expectedCovered: []string{"compute-0", "compute-1", "compute-2"},
		},
		{
			name:            "exact match returns single node",
			ansibleLimit:    "compute-0",
			nodesetNodes:    []string{"compute-0", "compute-1", "compute-2"},
			expectedCovered: []string{"compute-0"},
		},
		{
			name:            "comma-separated returns multiple nodes",
			ansibleLimit:    "compute-0,compute-2",
			nodesetNodes:    []string{"compute-0", "compute-1", "compute-2"},
			expectedCovered: []string{"compute-0", "compute-2"},
		},
		{
			name:            "wildcard pattern returns matching nodes",
			ansibleLimit:    "compute-*",
			nodesetNodes:    []string{"compute-0", "compute-1", "other-node"},
			expectedCovered: []string{"compute-0", "compute-1"},
		},
		{
			name:            "mixed exact and wildcard",
			ansibleLimit:    "compute-0,other-*",
			nodesetNodes:    []string{"compute-0", "compute-1", "other-node", "other-2"},
			expectedCovered: []string{"compute-0", "other-node", "other-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create deployment
			var deployment *dataplanev1.OpenStackDataPlaneDeployment
			if tt.nodesetNodes != nil {
				deployment = &dataplanev1.OpenStackDataPlaneDeployment{
					Spec: dataplanev1.OpenStackDataPlaneDeploymentSpec{
						AnsibleLimit: tt.ansibleLimit,
					},
				}
			}

			// Create nodeset
			var nodeset *dataplanev1.OpenStackDataPlaneNodeSet
			if tt.nodesetNodes != nil {
				nodes := make(map[string]dataplanev1.NodeSection)
				for _, nodeName := range tt.nodesetNodes {
					nodes[nodeName] = dataplanev1.NodeSection{}
				}
				nodeset = &dataplanev1.OpenStackDataPlaneNodeSet{
					Spec: dataplanev1.OpenStackDataPlaneNodeSetSpec{
						Nodes: nodes,
					},
				}
			}

			// Call function
			covered := getNodesCoveredByDeployment(deployment, nodeset)

			// Check result
			if len(covered) != len(tt.expectedCovered) {
				t.Errorf("getNodesCoveredByDeployment() returned %d nodes, want %d", len(covered), len(tt.expectedCovered))
			}

			// Check each expected node is covered
			for _, expectedNode := range tt.expectedCovered {
				found := false
				for _, node := range covered {
					if node == expectedNode {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("getNodesCoveredByDeployment() missing expected node %q", expectedNode)
				}
			}
		})
	}
}

func TestGetAllNodeNames(t *testing.T) {
	tests := []struct {
		name          string
		nodeset       *dataplanev1.OpenStackDataPlaneNodeSet
		expectedNodes []string
	}{
		{
			name:          "nil nodeset returns empty",
			nodeset:       nil,
			expectedNodes: []string{},
		},
		{
			name: "single node",
			nodeset: &dataplanev1.OpenStackDataPlaneNodeSet{
				Spec: dataplanev1.OpenStackDataPlaneNodeSetSpec{
					Nodes: map[string]dataplanev1.NodeSection{
						"compute-0": {},
					},
				},
			},
			expectedNodes: []string{"compute-0"},
		},
		{
			name: "multiple nodes",
			nodeset: &dataplanev1.OpenStackDataPlaneNodeSet{
				Spec: dataplanev1.OpenStackDataPlaneNodeSetSpec{
					Nodes: map[string]dataplanev1.NodeSection{
						"compute-0": {},
						"compute-1": {},
						"compute-2": {},
					},
				},
			},
			expectedNodes: []string{"compute-0", "compute-1", "compute-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := getAllNodeNames(tt.nodeset)

			if len(nodes) != len(tt.expectedNodes) {
				t.Errorf("getAllNodeNames() returned %d nodes, want %d", len(nodes), len(tt.expectedNodes))
			}

			// Check each expected node exists
			for _, expectedNode := range tt.expectedNodes {
				found := false
				for _, node := range nodes {
					if node == expectedNode {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("getAllNodeNames() missing expected node %q", expectedNode)
				}
			}
		})
	}
}

func TestExtractTransportURLFromSecretName(t *testing.T) {
	tests := []struct {
		name              string
		secretName        string
		expectedTransport string
	}{
		{
			name:              "nova cell1 transport",
			secretName:        "rabbitmq-user-nova-cell1-transport-cell1user1-user",
			expectedTransport: "nova-cell1-transport",
		},
		{
			name:              "nova cell1 transport with different user",
			secretName:        "rabbitmq-user-nova-cell1-transport-cell1user2-user",
			expectedTransport: "nova-cell1-transport",
		},
		{
			name:              "neutron transport",
			secretName:        "rabbitmq-user-neutron-transport-neutronuser-user",
			expectedTransport: "neutron-transport",
		},
		{
			name:              "nova api transport",
			secretName:        "rabbitmq-user-nova-api-transport-apiuser-user",
			expectedTransport: "nova-api-transport",
		},
		{
			name:              "invalid prefix",
			secretName:        "invalid-nova-cell1-transport-cell1user1-user",
			expectedTransport: "",
		},
		{
			name:              "no user suffix",
			secretName:        "rabbitmq-user-nova-cell1-transport-cell1user1",
			expectedTransport: "",
		},
		{
			name:              "single character username",
			secretName:        "rabbitmq-user-nova-cell1-transport-a-user",
			expectedTransport: "nova-cell1-transport",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTransportURLFromSecretName(tt.secretName)
			if result != tt.expectedTransport {
				t.Errorf("extractTransportURLFromSecretName(%q) = %q, want %q",
					tt.secretName, result, tt.expectedTransport)
			}
		})
	}
}
