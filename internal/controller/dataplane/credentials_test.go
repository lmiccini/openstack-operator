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
	"encoding/json"
	"testing"
	"time"

	dataplanev1 "github.com/openstack-k8s-operators/openstack-operator/api/dataplane/v1beta1"
)

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

func TestComputeDeploymentSummary(t *testing.T) {
	tests := []struct {
		name               string
		trackingData       *SecretTrackingData
		totalNodes         int
		expectedUpdated    int
		expectedAllUpdated bool
	}{
		{
			name: "all nodes updated",
			trackingData: &SecretTrackingData{
				NodeStatus: map[string]NodeSecretStatus{
					"node1": {AllSecretsUpdated: true},
					"node2": {AllSecretsUpdated: true},
					"node3": {AllSecretsUpdated: true},
				},
			},
			totalNodes:         3,
			expectedUpdated:    3,
			expectedAllUpdated: true,
		},
		{
			name: "partial nodes updated",
			trackingData: &SecretTrackingData{
				NodeStatus: map[string]NodeSecretStatus{
					"node1": {AllSecretsUpdated: true},
					"node2": {AllSecretsUpdated: false},
					"node3": {AllSecretsUpdated: true},
				},
			},
			totalNodes:         3,
			expectedUpdated:    2,
			expectedAllUpdated: false,
		},
		{
			name: "no nodes updated",
			trackingData: &SecretTrackingData{
				NodeStatus: map[string]NodeSecretStatus{
					"node1": {AllSecretsUpdated: false},
					"node2": {AllSecretsUpdated: false},
				},
			},
			totalNodes:         2,
			expectedUpdated:    0,
			expectedAllUpdated: false,
		},
		{
			name: "empty tracking data",
			trackingData: &SecretTrackingData{
				NodeStatus: map[string]NodeSecretStatus{},
			},
			totalNodes:         0,
			expectedUpdated:    0,
			expectedAllUpdated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := computeDeploymentSummary(tt.trackingData, tt.totalNodes, "test-configmap")

			if summary.UpdatedNodes != tt.expectedUpdated {
				t.Errorf("computeDeploymentSummary() UpdatedNodes = %d, want %d",
					summary.UpdatedNodes, tt.expectedUpdated)
			}

			if summary.AllNodesUpdated != tt.expectedAllUpdated {
				t.Errorf("computeDeploymentSummary() AllNodesUpdated = %v, want %v",
					summary.AllNodesUpdated, tt.expectedAllUpdated)
			}

			if summary.TotalNodes != tt.totalNodes {
				t.Errorf("computeDeploymentSummary() TotalNodes = %d, want %d",
					summary.TotalNodes, tt.totalNodes)
			}

			if summary.ConfigMapName != "test-configmap" {
				t.Errorf("computeDeploymentSummary() ConfigMapName = %s, want test-configmap",
					summary.ConfigMapName)
			}

			if summary.LastUpdateTime == nil {
				t.Error("computeDeploymentSummary() LastUpdateTime is nil, want timestamp")
			}
		})
	}
}

func TestSecretTrackingDataJSONSerialization(t *testing.T) {
	now := time.Now()
	original := &SecretTrackingData{
		Secrets: map[string]SecretVersionInfo{
			"secret1": {
				CurrentHash:       "hash123",
				PreviousHash:      "hash456",
				NodesWithCurrent:  []string{"node1", "node2"},
				NodesWithPrevious: []string{"node3"},
				LastChanged:       now,
			},
			"secret2": {
				CurrentHash:      "hash789",
				NodesWithCurrent: []string{"node1", "node2", "node3"},
				LastChanged:      now,
			},
		},
		NodeStatus: map[string]NodeSecretStatus{
			"node1": {
				AllSecretsUpdated:  true,
				SecretsWithCurrent: []string{"secret1", "secret2"},
			},
			"node2": {
				AllSecretsUpdated:  true,
				SecretsWithCurrent: []string{"secret1", "secret2"},
			},
			"node3": {
				AllSecretsUpdated:   false,
				SecretsWithCurrent:  []string{"secret2"},
				SecretsWithPrevious: []string{"secret1"},
			},
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal SecretTrackingData: %v", err)
	}

	// Unmarshal back
	var restored SecretTrackingData
	if err := json.Unmarshal(jsonData, &restored); err != nil {
		t.Fatalf("Failed to unmarshal SecretTrackingData: %v", err)
	}

	// Verify secrets
	if len(restored.Secrets) != len(original.Secrets) {
		t.Errorf("Restored secrets count = %d, want %d", len(restored.Secrets), len(original.Secrets))
	}

	secret1 := restored.Secrets["secret1"]
	if secret1.CurrentHash != "hash123" {
		t.Errorf("Restored secret1 CurrentHash = %s, want hash123", secret1.CurrentHash)
	}
	if secret1.PreviousHash != "hash456" {
		t.Errorf("Restored secret1 PreviousHash = %s, want hash456", secret1.PreviousHash)
	}
	if len(secret1.NodesWithCurrent) != 2 {
		t.Errorf("Restored secret1 NodesWithCurrent count = %d, want 2", len(secret1.NodesWithCurrent))
	}

	// Verify node status
	if len(restored.NodeStatus) != len(original.NodeStatus) {
		t.Errorf("Restored node status count = %d, want %d", len(restored.NodeStatus), len(original.NodeStatus))
	}

	node1 := restored.NodeStatus["node1"]
	if !node1.AllSecretsUpdated {
		t.Error("Restored node1 AllSecretsUpdated = false, want true")
	}
	if len(node1.SecretsWithCurrent) != 2 {
		t.Errorf("Restored node1 SecretsWithCurrent count = %d, want 2", len(node1.SecretsWithCurrent))
	}
}

func TestGetSecretTrackingConfigMapName(t *testing.T) {
	tests := []struct {
		nodesetName string
		expected    string
	}{
		{
			nodesetName: "compute-nodes",
			expected:    "compute-nodes-secret-tracking",
		},
		{
			nodesetName: "edpm",
			expected:    "edpm-secret-tracking",
		},
		{
			nodesetName: "my-nodeset-123",
			expected:    "my-nodeset-123-secret-tracking",
		},
	}

	for _, tt := range tests {
		t.Run(tt.nodesetName, func(t *testing.T) {
			result := getSecretTrackingConfigMapName(tt.nodesetName)
			if result != tt.expected {
				t.Errorf("getSecretTrackingConfigMapName(%q) = %q, want %q",
					tt.nodesetName, result, tt.expected)
			}
		})
	}
}

func TestSecretRotationTracking(t *testing.T) {
	// Simulate secret rotation scenario
	trackingData := &SecretTrackingData{
		Secrets:    make(map[string]SecretVersionInfo),
		NodeStatus: make(map[string]NodeSecretStatus),
	}

	now := time.Now()

	// Step 1: Initial secret deployment
	trackingData.Secrets["rabbitmq-secret"] = SecretVersionInfo{
		CurrentHash:      "hash-v1",
		NodesWithCurrent: []string{"node1", "node2"},
		LastChanged:      now,
	}

	// Verify initial state
	secret := trackingData.Secrets["rabbitmq-secret"]
	if secret.CurrentHash != "hash-v1" {
		t.Errorf("Initial CurrentHash = %s, want hash-v1", secret.CurrentHash)
	}
	if secret.PreviousHash != "" {
		t.Errorf("Initial PreviousHash = %s, want empty", secret.PreviousHash)
	}

	// Step 2: Simulate rotation - hash changes
	secret.PreviousHash = secret.CurrentHash
	secret.NodesWithPrevious = secret.NodesWithCurrent
	secret.CurrentHash = "hash-v2"
	secret.NodesWithCurrent = []string{"node1"} // Only node1 has new version
	secret.LastChanged = now.Add(1 * time.Hour)
	trackingData.Secrets["rabbitmq-secret"] = secret

	// Verify rotation state
	secret = trackingData.Secrets["rabbitmq-secret"]
	if secret.CurrentHash != "hash-v2" {
		t.Errorf("After rotation CurrentHash = %s, want hash-v2", secret.CurrentHash)
	}
	if secret.PreviousHash != "hash-v1" {
		t.Errorf("After rotation PreviousHash = %s, want hash-v1", secret.PreviousHash)
	}
	if len(secret.NodesWithPrevious) != 2 {
		t.Errorf("After rotation NodesWithPrevious count = %d, want 2", len(secret.NodesWithPrevious))
	}
	if len(secret.NodesWithCurrent) != 1 {
		t.Errorf("After rotation NodesWithCurrent count = %d, want 1", len(secret.NodesWithCurrent))
	}

	// Step 3: Second node gets new version
	secret.NodesWithCurrent = append(secret.NodesWithCurrent, "node2")
	trackingData.Secrets["rabbitmq-secret"] = secret

	// All nodes now have current version, should clear previous
	if len(secret.NodesWithCurrent) == 2 {
		secret.PreviousHash = ""
		secret.NodesWithPrevious = []string{}
		trackingData.Secrets["rabbitmq-secret"] = secret
	}

	// Verify cleanup after full rotation
	secret = trackingData.Secrets["rabbitmq-secret"]
	if secret.PreviousHash != "" {
		t.Errorf("After full rotation PreviousHash = %s, want empty", secret.PreviousHash)
	}
	if len(secret.NodesWithPrevious) != 0 {
		t.Errorf("After full rotation NodesWithPrevious count = %d, want 0", len(secret.NodesWithPrevious))
	}
	if len(secret.NodesWithCurrent) != 2 {
		t.Errorf("After full rotation NodesWithCurrent count = %d, want 2", len(secret.NodesWithCurrent))
	}
}
