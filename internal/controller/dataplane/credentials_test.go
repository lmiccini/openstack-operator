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
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	dataplanev1 "github.com/openstack-k8s-operators/openstack-operator/api/dataplane/v1beta1"
)

func TestDetectServicesInDeployment(t *testing.T) {
	tests := []struct {
		name                  string
		deployment            *dataplanev1.OpenStackDataPlaneDeployment
		secrets               []corev1.Secret
		transportURLs         []rabbitmqv1.TransportURL
		wantServices          []string
		wantRabbitMQUserNames map[string]string // service -> rabbitmq user secret name
	}{
		{
			name:                  "nil deployment returns empty",
			deployment:            nil,
			secrets:               []corev1.Secret{},
			transportURLs:         []rabbitmqv1.TransportURL{},
			wantServices:          []string{},
			wantRabbitMQUserNames: map[string]string{},
		},
		{
			name: "detect nova service with dedicated user",
			deployment: &dataplanev1.OpenStackDataPlaneDeployment{
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
						"01-nova.conf": []byte("transport_url = rabbit://novacell1-2:password@rabbitmq:5672/"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rabbitmq-user-nova-cell1-transport-novacell1-2-user",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"username": []byte("novacell1-2"),
						"password": []byte("password123"),
					},
				},
			},
			transportURLs: []rabbitmqv1.TransportURL{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nova-cell1-transport",
						Namespace: "test",
					},
					Status: rabbitmqv1.TransportURLStatus{
						RabbitmqUsername: "novacell1-2",
						RabbitmqUserRef:  "nova-cell1-transport-novacell1-2-user",
					},
				},
			},
			wantServices: []string{"nova"},
			wantRabbitMQUserNames: map[string]string{
				"nova": "rabbitmq-user-nova-cell1-transport-novacell1-2-user",
			},
		},
		{
			name: "skip neutron service using default_user",
			deployment: &dataplanev1.OpenStackDataPlaneDeployment{
				Status: dataplanev1.OpenStackDataPlaneDeploymentStatus{
					SecretHashes: map[string]string{
						"neutron-dhcp-agent-neutron-config": "hash456",
					},
				},
			},
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "neutron-dhcp-agent-neutron-config",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"10-neutron-dhcp.conf": []byte("transport_url = rabbit://default_user:pass@host:5672/"),
					},
				},
			},
			transportURLs: []rabbitmqv1.TransportURL{
				// No TransportURL with dedicated user for neutron
			},
			wantServices:          []string{}, // Should be skipped
			wantRabbitMQUserNames: map[string]string{},
		},
		{
			name: "detect multiple services with dedicated users",
			deployment: &dataplanev1.OpenStackDataPlaneDeployment{
				Status: dataplanev1.OpenStackDataPlaneDeploymentStatus{
					SecretHashes: map[string]string{
						"nova-cell1-compute-config":         "nova-hash",
						"ironic-neutron-agent-config-data":  "ironic-hash",
						"neutron-dhcp-agent-neutron-config": "neutron-hash",
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
						"01-nova.conf": []byte("transport_url = rabbit://novacell1-1:pass@host:5672/"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rabbitmq-user-nova-cell1-transport-novacell1-1-user",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"username": []byte("novacell1-1"),
						"password": []byte("pass"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ironic-neutron-agent-config-data",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"01-ironic.conf": []byte("transport_url = rabbit://ironic-1:pass@host:5672/"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rabbitmq-user-ironic-transport-ironic-1-user",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"username": []byte("ironic-1"),
						"password": []byte("pass"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "neutron-dhcp-agent-neutron-config",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"10-neutron-dhcp.conf": []byte("transport_url = rabbit://default_user:pass@host:5672/"),
					},
				},
			},
			transportURLs: []rabbitmqv1.TransportURL{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nova-cell1-transport",
						Namespace: "test",
					},
					Status: rabbitmqv1.TransportURLStatus{
						RabbitmqUsername: "novacell1-1",
						RabbitmqUserRef:  "nova-cell1-transport-novacell1-1-user",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ironic-transport",
						Namespace: "test",
					},
					Status: rabbitmqv1.TransportURLStatus{
						RabbitmqUsername: "ironic-1",
						RabbitmqUserRef:  "ironic-transport-ironic-1-user",
					},
				},
			},
			wantServices: []string{"nova", "ironic"}, // neutron skipped
			wantRabbitMQUserNames: map[string]string{
				"nova":   "rabbitmq-user-nova-cell1-transport-novacell1-1-user",
				"ironic": "rabbitmq-user-ironic-transport-ironic-1-user",
			},
		},
		{
			name: "ignore non-service config secrets",
			deployment: &dataplanev1.OpenStackDataPlaneDeployment{
				Status: dataplanev1.OpenStackDataPlaneDeploymentStatus{
					SecretHashes: map[string]string{
						"nova-db-secret":                 "db-hash",
						"ceilometer-compute-config-data": "config-hash",
						"random-secret":                  "random-hash",
					},
				},
			},
			secrets:               []corev1.Secret{},
			transportURLs:         []rabbitmqv1.TransportURL{},
			wantServices:          []string{},
			wantRabbitMQUserNames: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create scheme and fake client
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			_ = dataplanev1.AddToScheme(scheme)
			_ = rabbitmqv1.AddToScheme(scheme)

			// Build client objects
			objs := []runtime.Object{}
			for i := range tt.secrets {
				objs = append(objs, &tt.secrets[i])
			}
			for i := range tt.transportURLs {
				objs = append(objs, &tt.transportURLs[i])
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objs...).
				Build()

			reconciler := &OpenStackDataPlaneNodeSetReconciler{
				Client: fakeClient,
			}

			services, err := reconciler.detectServicesInDeployment(context.TODO(), tt.deployment, "test")
			if err != nil {
				t.Fatalf("detectServicesInDeployment() error = %v", err)
			}

			if len(services) != len(tt.wantServices) {
				t.Errorf("detectServicesInDeployment() returned %d services, want %d", len(services), len(tt.wantServices))
			}

			for _, wantService := range tt.wantServices {
				if _, ok := services[wantService]; !ok {
					t.Errorf("detectServicesInDeployment() missing service %q", wantService)
				}
			}

			for service, wantUserName := range tt.wantRabbitMQUserNames {
				if info, ok := services[service]; !ok {
					t.Errorf("detectServicesInDeployment() missing service %q", service)
				} else if info.rabbitmqUserName != wantUserName {
					t.Errorf("detectServicesInDeployment() service %q has rabbitmqUserName %q, want %q",
						service, info.rabbitmqUserName, wantUserName)
				}
			}
		})
	}
}

func TestGetNodesCoveredByDeployment(t *testing.T) {
	nodeset := &dataplanev1.OpenStackDataPlaneNodeSet{
		Spec: dataplanev1.OpenStackDataPlaneNodeSetSpec{
			Nodes: map[string]dataplanev1.NodeSection{
				"compute-0": {},
				"compute-1": {},
				"compute-2": {},
			},
		},
	}

	tests := []struct {
		name         string
		ansibleLimit string
		nodeset      *dataplanev1.OpenStackDataPlaneNodeSet
		wantNodes    []string
	}{
		{
			name:         "nil nodeset returns empty",
			ansibleLimit: "",
			nodeset:      nil,
			wantNodes:    []string{},
		},
		{
			name:         "nil deployment returns empty",
			ansibleLimit: "",
			nodeset:      nodeset,
			wantNodes:    []string{},
		},
		{
			name:         "empty AnsibleLimit returns all nodes",
			ansibleLimit: "",
			nodeset:      nodeset,
			wantNodes:    []string{"compute-0", "compute-1", "compute-2"},
		},
		{
			name:         "wildcard returns all nodes",
			ansibleLimit: "*",
			nodeset:      nodeset,
			wantNodes:    []string{"compute-0", "compute-1", "compute-2"},
		},
		{
			name:         "exact match returns single node",
			ansibleLimit: "compute-0",
			nodeset:      nodeset,
			wantNodes:    []string{"compute-0"},
		},
		{
			name:         "comma-separated returns multiple nodes",
			ansibleLimit: "compute-0,compute-1",
			nodeset:      nodeset,
			wantNodes:    []string{"compute-0", "compute-1"},
		},
		{
			name:         "wildcard prefix returns matching nodes",
			ansibleLimit: "compute-*",
			nodeset:      nodeset,
			wantNodes:    []string{"compute-0", "compute-1", "compute-2"},
		},
		{
			name:         "whitespace in list is trimmed",
			ansibleLimit: "compute-0, compute-1 ,compute-2",
			nodeset:      nodeset,
			wantNodes:    []string{"compute-0", "compute-1", "compute-2"},
		},
		{
			name:         "non-matching limit returns empty",
			ansibleLimit: "controller-0",
			nodeset:      nodeset,
			wantNodes:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var deployment *dataplanev1.OpenStackDataPlaneDeployment
			if tt.name == "nil deployment returns empty" {
				deployment = nil
			} else {
				deployment = &dataplanev1.OpenStackDataPlaneDeployment{
					Spec: dataplanev1.OpenStackDataPlaneDeploymentSpec{
						AnsibleLimit: tt.ansibleLimit,
					},
				}
			}

			nodes := getNodesCoveredByDeployment(deployment, tt.nodeset)

			if len(nodes) != len(tt.wantNodes) {
				t.Errorf("getNodesCoveredByDeployment() returned %d nodes, want %d", len(nodes), len(tt.wantNodes))
			}

			for _, wantNode := range tt.wantNodes {
				found := false
				for _, node := range nodes {
					if node == wantNode {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("getNodesCoveredByDeployment() missing node %q", wantNode)
				}
			}
		})
	}
}

func TestUpdateServiceCredentialStatus(t *testing.T) {
	// Compute the expected hash for the mock RabbitMQUser secret
	mockRabbitMQUserData := map[string][]byte{
		"username": []byte("novacell1-2"),
		"password": []byte("password123"),
	}
	expectedHash, _ := util.ObjectHash(mockRabbitMQUserData)

	tests := []struct {
		name                string
		existingStatus      map[string]dataplanev1.ServiceCredentialInfo
		deploymentReady     bool
		deploymentSecrets   map[string]string
		ansibleLimit        string
		totalNodes          int
		wantService         string
		wantUpdatedNodes    []string
		wantAllNodesUpdated bool
	}{
		{
			name:            "deployment not ready does not update",
			existingStatus:  nil,
			deploymentReady: false,
			deploymentSecrets: map[string]string{
				"nova-cell1-compute-config": "hash123",
			},
			ansibleLimit: "",
			totalNodes:   3,
		},
		{
			name:            "all nodes updated with no AnsibleLimit",
			existingStatus:  nil,
			deploymentReady: true,
			deploymentSecrets: map[string]string{
				"nova-cell1-compute-config": "hash123",
			},
			ansibleLimit:        "",
			totalNodes:          3,
			wantService:         "nova",
			wantUpdatedNodes:    []string{"compute-0", "compute-1", "compute-2"},
			wantAllNodesUpdated: true,
		},
		{
			name:            "partial nodes with AnsibleLimit",
			existingStatus:  nil,
			deploymentReady: true,
			deploymentSecrets: map[string]string{
				"nova-cell1-compute-config": "hash123",
			},
			ansibleLimit:        "compute-0,compute-1",
			totalNodes:          3,
			wantService:         "nova",
			wantUpdatedNodes:    []string{"compute-0", "compute-1"},
			wantAllNodesUpdated: false,
		},
		{
			name: "accumulate nodes across deployments",
			existingStatus: map[string]dataplanev1.ServiceCredentialInfo{
				"nova": {
					SecretName:      "rabbitmq-user-nova-cell1-transport-novacell1-2-user",
					SecretHash:      expectedHash,
					UpdatedNodes:    []string{"compute-0"},
					TotalNodes:      3,
					AllNodesUpdated: false,
				},
			},
			deploymentReady: true,
			deploymentSecrets: map[string]string{
				"nova-cell1-compute-config": "hash123",
			},
			ansibleLimit:        "compute-1",
			totalNodes:          3,
			wantService:         "nova",
			wantUpdatedNodes:    []string{"compute-0", "compute-1"},
			wantAllNodesUpdated: false,
		},
		{
			name: "nodeset scaled up updates TotalNodes",
			existingStatus: map[string]dataplanev1.ServiceCredentialInfo{
				"nova": {
					SecretName:      "rabbitmq-user-nova-cell1-transport-novacell1-2-user",
					SecretHash:      expectedHash,
					UpdatedNodes:    []string{"compute-0", "compute-1"},
					TotalNodes:      2, // Old size
					AllNodesUpdated: true,
				},
			},
			deploymentReady: true,
			deploymentSecrets: map[string]string{
				"nova-cell1-compute-config": "hash123",
			},
			ansibleLimit:        "compute-2", // Deploy to new node
			totalNodes:          3,           // Scaled to 3
			wantService:         "nova",
			wantUpdatedNodes:    []string{"compute-0", "compute-1", "compute-2"},
			wantAllNodesUpdated: true, // All 3 nodes now updated
		},
		{
			name: "nodeset scaled down updates TotalNodes",
			existingStatus: map[string]dataplanev1.ServiceCredentialInfo{
				"nova": {
					SecretName:      "rabbitmq-user-nova-cell1-transport-novacell1-2-user",
					SecretHash:      expectedHash,
					UpdatedNodes:    []string{"compute-0", "compute-1", "compute-2"},
					TotalNodes:      3, // Old size
					AllNodesUpdated: true,
				},
			},
			deploymentReady: true,
			deploymentSecrets: map[string]string{
				"nova-cell1-compute-config": "hash123",
			},
			ansibleLimit:        "", // Deploy to all (now only 2)
			totalNodes:          2,  // Scaled down to 2
			wantService:         "nova",
			wantUpdatedNodes:    []string{"compute-0", "compute-1", "compute-2"}, // Keeps old list
			wantAllNodesUpdated: false,                                           // 3 > 2, so not all updated
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set timestamps for testing
			// RabbitMQUser secret created first
			secretCreationTime := metav1.Now()
			// Deployment created 1 minute later
			deploymentCreationTime := metav1.NewTime(secretCreationTime.Add(1 * time.Minute))

			// Create mock secret for nova-cell1-compute-config
			mockConfigSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nova-cell1-compute-config",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"01-nova.conf": []byte("transport_url = rabbit://novacell1-2:password@rabbitmq:5672/"),
				},
			}

			// Create mock RabbitMQUser secret
			mockRabbitMQUserSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rabbitmq-user-nova-cell1-transport-novacell1-2-user",
					Namespace:         "test",
					CreationTimestamp: secretCreationTime,
				},
				Data: map[string][]byte{
					"username": []byte("novacell1-2"),
					"password": []byte("password123"),
				},
			}

			// Create mock TransportURL
			mockTransportURL := &rabbitmqv1.TransportURL{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nova-cell1-transport",
					Namespace: "test",
				},
				Status: rabbitmqv1.TransportURLStatus{
					RabbitmqUsername: "novacell1-2",
					RabbitmqUserRef:  "nova-cell1-transport-novacell1-2-user",
				},
			}

			// Create scheme and fake client
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			_ = dataplanev1.AddToScheme(scheme)
			_ = rabbitmqv1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(mockConfigSecret, mockRabbitMQUserSecret, mockTransportURL).
				Build()

			reconciler := &OpenStackDataPlaneNodeSetReconciler{
				Client: fakeClient,
			}

			// Build nodeset with correct number of nodes based on totalNodes
			nodes := map[string]dataplanev1.NodeSection{}
			for i := 0; i < tt.totalNodes; i++ {
				nodeName := fmt.Sprintf("compute-%d", i)
				nodes[nodeName] = dataplanev1.NodeSection{}
			}

			nodeset := &dataplanev1.OpenStackDataPlaneNodeSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nodeset",
					Namespace: "test",
				},
				Spec: dataplanev1.OpenStackDataPlaneNodeSetSpec{
					Nodes: nodes,
				},
				Status: dataplanev1.OpenStackDataPlaneNodeSetStatus{
					ServiceCredentialStatus: tt.existingStatus,
				},
			}

			deployment := &dataplanev1.OpenStackDataPlaneDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-deployment",
					Namespace:         "test",
					CreationTimestamp: deploymentCreationTime,
				},
				Spec: dataplanev1.OpenStackDataPlaneDeploymentSpec{
					AnsibleLimit: tt.ansibleLimit,
				},
				Status: dataplanev1.OpenStackDataPlaneDeploymentStatus{
					SecretHashes: tt.deploymentSecrets,
				},
			}

			if tt.deploymentReady {
				deployment.Status.Conditions.MarkTrue("DeploymentReady", "Ready")
			}

			err := reconciler.updateServiceCredentialStatus(context.TODO(), nodeset, deployment)
			if err != nil {
				t.Errorf("updateServiceCredentialStatus() error = %v", err)
				return
			}

			if !tt.deploymentReady {
				// Should not have updated status
				if nodeset.Status.ServiceCredentialStatus != nil {
					t.Errorf("updateServiceCredentialStatus() updated status when deployment not ready")
				}
				return
			}

			if tt.wantService == "" {
				return
			}

			credInfo, ok := nodeset.Status.ServiceCredentialStatus[tt.wantService]
			if !ok {
				t.Errorf("updateServiceCredentialStatus() did not create %q service status", tt.wantService)
				return
			}

			if len(credInfo.UpdatedNodes) != len(tt.wantUpdatedNodes) {
				t.Errorf("updateServiceCredentialStatus() UpdatedNodes length = %d, want %d",
					len(credInfo.UpdatedNodes), len(tt.wantUpdatedNodes))
			}

			for _, wantNode := range tt.wantUpdatedNodes {
				found := false
				for _, node := range credInfo.UpdatedNodes {
					if node == wantNode {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("updateServiceCredentialStatus() missing node %q in UpdatedNodes", wantNode)
				}
			}

			if credInfo.AllNodesUpdated != tt.wantAllNodesUpdated {
				t.Errorf("updateServiceCredentialStatus() AllNodesUpdated = %v, want %v",
					credInfo.AllNodesUpdated, tt.wantAllNodesUpdated)
			}
		})
	}
}
