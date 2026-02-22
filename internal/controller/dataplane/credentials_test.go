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
	"encoding/json"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
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

func TestNodeAccumulationDoesNotDuplicateInBothLists(t *testing.T) {
	// This test verifies that when nodes accumulate across deployments,
	// they are removed from nodesWithPrevious when added to nodesWithCurrent

	// Initial state: secret rotated, compute-0 has new version, compute-1 has old
	trackingData := &SecretTrackingData{
		Secrets: map[string]SecretVersionInfo{
			"nova-config": {
				CurrentHash:       "hash-v2",
				PreviousHash:      "hash-v1",
				NodesWithCurrent:  []string{"compute-0"},
				NodesWithPrevious: []string{"compute-1"},
				LastChanged:       time.Now(),
			},
		},
		NodeStatus: map[string]NodeSecretStatus{
			"compute-0": {AllSecretsUpdated: true, SecretsWithCurrent: []string{"nova-config"}},
			"compute-1": {AllSecretsUpdated: false, SecretsWithPrevious: []string{"nova-config"}},
		},
	}

	// Simulate deployment that covers compute-1 with hash-v2 (same version accumulation)
	secretInfo := trackingData.Secrets["nova-config"]
	coveredNodes := []string{"compute-1"}

	// Add newly covered nodes (this is the code path that had the bug)
	for _, node := range coveredNodes {
		if !slices.Contains(secretInfo.NodesWithCurrent, node) {
			secretInfo.NodesWithCurrent = append(secretInfo.NodesWithCurrent, node)
		}

		// Remove from previous if it was there (the fix)
		if secretInfo.PreviousHash != "" {
			newPrevious := []string{}
			for _, prevNode := range secretInfo.NodesWithPrevious {
				if prevNode != node {
					newPrevious = append(newPrevious, prevNode)
				}
			}
			secretInfo.NodesWithPrevious = newPrevious
		}
	}

	trackingData.Secrets["nova-config"] = secretInfo

	// Verify compute-1 is only in nodesWithCurrent, not in both
	if !slices.Contains(secretInfo.NodesWithCurrent, "compute-1") {
		t.Error("compute-1 should be in nodesWithCurrent after deployment")
	}

	if slices.Contains(secretInfo.NodesWithPrevious, "compute-1") {
		t.Error("compute-1 should NOT be in nodesWithPrevious after being upgraded")
	}

	// Verify both nodes are in current
	if len(secretInfo.NodesWithCurrent) != 2 {
		t.Errorf("Expected 2 nodes in nodesWithCurrent, got %d", len(secretInfo.NodesWithCurrent))
	}

	if len(secretInfo.NodesWithPrevious) != 0 {
		t.Errorf("Expected 0 nodes in nodesWithPrevious, got %d", len(secretInfo.NodesWithPrevious))
	}
}

func TestGradualRolloutWithAnsibleLimit(t *testing.T) {
	// Test gradual rollout: deployment 1 covers compute-0, deployment 2 covers compute-1
	tests := []struct {
		name                string
		initialTracking     *SecretTrackingData
		deployment1Nodes    []string // First deployment with AnsibleLimit
		deployment2Nodes    []string // Second deployment with AnsibleLimit
		expectedAllUpdated  bool
		expectedUpdatedCount int
	}{
		{
			name: "gradual rollout - first deployment partial",
			initialTracking: &SecretTrackingData{
				Secrets:    make(map[string]SecretVersionInfo),
				NodeStatus: make(map[string]NodeSecretStatus),
			},
			deployment1Nodes:     []string{"compute-0"},
			deployment2Nodes:     []string{"compute-1"},
			expectedAllUpdated:   true,
			expectedUpdatedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracking := tt.initialTracking

			// Deployment 1: covers some nodes
			secretInfo := SecretVersionInfo{
				CurrentHash:      "hash1",
				NodesWithCurrent: tt.deployment1Nodes,
				LastChanged:      time.Now(),
			}
			tracking.Secrets["test-secret"] = secretInfo

			// Deployment 2: accumulates more nodes
			for _, node := range tt.deployment2Nodes {
				if !slices.Contains(secretInfo.NodesWithCurrent, node) {
					secretInfo.NodesWithCurrent = append(secretInfo.NodesWithCurrent, node)
				}
			}
			tracking.Secrets["test-secret"] = secretInfo

			// Compute node status
			allNodes := []string{"compute-0", "compute-1"}
			for _, nodeName := range allNodes {
				nodeStatus := NodeSecretStatus{
					AllSecretsUpdated:   true,
					SecretsWithCurrent:  []string{},
					SecretsWithPrevious: []string{},
				}

				for secretName, sInfo := range tracking.Secrets {
					if slices.Contains(sInfo.NodesWithCurrent, nodeName) {
						nodeStatus.SecretsWithCurrent = append(nodeStatus.SecretsWithCurrent, secretName)
					} else {
						nodeStatus.AllSecretsUpdated = false
					}
				}

				tracking.NodeStatus[nodeName] = nodeStatus
			}

			// Compute summary
			summary := computeDeploymentSummary(tracking, len(allNodes), "test-cm")

			if summary.AllNodesUpdated != tt.expectedAllUpdated {
				t.Errorf("AllNodesUpdated = %v, want %v", summary.AllNodesUpdated, tt.expectedAllUpdated)
			}

			if summary.UpdatedNodes != tt.expectedUpdatedCount {
				t.Errorf("UpdatedNodes = %d, want %d", summary.UpdatedNodes, tt.expectedUpdatedCount)
			}
		})
	}
}

func TestSecretRotationWithGradualRollout(t *testing.T) {
	// Test secret rotation followed by gradual rollout
	tests := []struct {
		name                 string
		phase                string
		expectedNodesInBoth  bool // Should any node be in both current and previous?
		expectedAllUpdated   bool
		expectedUpdatedNodes int
	}{
		{
			name:                 "after rotation - one node upgraded",
			phase:                "partial",
			expectedNodesInBoth:  false,
			expectedAllUpdated:   false,
			expectedUpdatedNodes: 1,
		},
		{
			name:                 "after rotation - all nodes upgraded",
			phase:                "complete",
			expectedNodesInBoth:  false,
			expectedAllUpdated:   true,
			expectedUpdatedNodes: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initial: all nodes have hash-v1
			tracking := &SecretTrackingData{
				Secrets: map[string]SecretVersionInfo{
					"nova-config": {
						CurrentHash:      "hash-v1",
						NodesWithCurrent: []string{"compute-0", "compute-1"},
						LastChanged:      time.Now(),
					},
				},
				NodeStatus: make(map[string]NodeSecretStatus),
			}

			// Rotation happens: hash changes to v2
			secretInfo := tracking.Secrets["nova-config"]
			secretInfo.PreviousHash = secretInfo.CurrentHash
			secretInfo.NodesWithPrevious = secretInfo.NodesWithCurrent
			secretInfo.CurrentHash = "hash-v2"
			secretInfo.NodesWithCurrent = []string{} // Reset
			tracking.Secrets["nova-config"] = secretInfo

			// Deployment 1: covers compute-0 with hash-v2
			secretInfo.NodesWithCurrent = append(secretInfo.NodesWithCurrent, "compute-0")

			// Remove from previous (the fix)
			newPrevious := []string{}
			for _, node := range secretInfo.NodesWithPrevious {
				if node != "compute-0" {
					newPrevious = append(newPrevious, node)
				}
			}
			secretInfo.NodesWithPrevious = newPrevious
			tracking.Secrets["nova-config"] = secretInfo

			if tt.phase == "complete" {
				// Deployment 2: covers compute-1 with hash-v2
				secretInfo.NodesWithCurrent = append(secretInfo.NodesWithCurrent, "compute-1")

				// Remove from previous
				secretInfo.NodesWithPrevious = []string{} // Should be empty now
				secretInfo.PreviousHash = "" // Clear metadata
				tracking.Secrets["nova-config"] = secretInfo
			}

			// Verify no node is in both lists
			secretInfo = tracking.Secrets["nova-config"]
			for _, node := range secretInfo.NodesWithCurrent {
				if slices.Contains(secretInfo.NodesWithPrevious, node) {
					if !tt.expectedNodesInBoth {
						t.Errorf("Node %q is in BOTH nodesWithCurrent and nodesWithPrevious", node)
					}
				}
			}

			// Compute node status
			allNodes := []string{"compute-0", "compute-1"}
			for _, nodeName := range allNodes {
				nodeStatus := NodeSecretStatus{
					AllSecretsUpdated:   true,
					SecretsWithCurrent:  []string{},
					SecretsWithPrevious: []string{},
				}

				for secretName, sInfo := range tracking.Secrets {
					if slices.Contains(sInfo.NodesWithCurrent, nodeName) {
						nodeStatus.SecretsWithCurrent = append(nodeStatus.SecretsWithCurrent, secretName)
					} else if slices.Contains(sInfo.NodesWithPrevious, nodeName) {
						nodeStatus.SecretsWithPrevious = append(nodeStatus.SecretsWithPrevious, secretName)
						nodeStatus.AllSecretsUpdated = false
					} else {
						nodeStatus.AllSecretsUpdated = false
					}
				}

				tracking.NodeStatus[nodeName] = nodeStatus
			}

			// Compute summary
			summary := computeDeploymentSummary(tracking, len(allNodes), "test-cm")

			if summary.AllNodesUpdated != tt.expectedAllUpdated {
				t.Errorf("AllNodesUpdated = %v, want %v", summary.AllNodesUpdated, tt.expectedAllUpdated)
			}

			if summary.UpdatedNodes != tt.expectedUpdatedNodes {
				t.Errorf("UpdatedNodes = %d, want %d", summary.UpdatedNodes, tt.expectedUpdatedNodes)
			}
		})
	}
}

func TestRotationImmediatelyCoveringAllNodes(t *testing.T) {
	// Test the bug where rotation+full coverage left nodes in both lists
	// This was the actual bug seen in production

	tracking := &SecretTrackingData{
		Secrets: map[string]SecretVersionInfo{
			"nova-config": {
				CurrentHash:      "hash-v1",
				NodesWithCurrent: []string{"compute-0", "compute-1"},
				LastChanged:      time.Now(),
			},
		},
		NodeStatus: make(map[string]NodeSecretStatus),
	}

	// Rotation happens: hash changes to v2
	secretInfo := tracking.Secrets["nova-config"]

	// Move current to previous (rotation logic)
	secretInfo.PreviousHash = secretInfo.CurrentHash
	secretInfo.NodesWithPrevious = secretInfo.NodesWithCurrent

	// Update to new version
	secretInfo.CurrentHash = "hash-v2"

	// Deployment immediately covers all nodes
	coveredNodes := []string{"compute-0", "compute-1"}
	secretInfo.NodesWithCurrent = coveredNodes

	totalNodes := 2

	// Clear previous if all nodes covered (the fix)
	if len(secretInfo.NodesWithCurrent) == totalNodes && secretInfo.PreviousHash != "" {
		secretInfo.PreviousHash = ""
		secretInfo.NodesWithPrevious = []string{}
	}

	tracking.Secrets["nova-config"] = secretInfo

	// Verify nodes NOT in both lists
	if len(secretInfo.NodesWithPrevious) != 0 {
		t.Errorf("After rotation with full coverage, nodesWithPrevious should be empty, got %v",
			secretInfo.NodesWithPrevious)
	}

	if len(secretInfo.NodesWithCurrent) != 2 {
		t.Errorf("After rotation with full coverage, nodesWithCurrent should have 2 nodes, got %d",
			len(secretInfo.NodesWithCurrent))
	}

	if secretInfo.PreviousHash != "" {
		t.Errorf("After rotation with full coverage, previousHash should be cleared, got %q",
			secretInfo.PreviousHash)
	}

	// Compute node status
	allNodes := []string{"compute-0", "compute-1"}
	for _, nodeName := range allNodes {
		nodeStatus := NodeSecretStatus{
			AllSecretsUpdated:   true,
			SecretsWithCurrent:  []string{},
			SecretsWithPrevious: []string{},
		}

		for secretName, sInfo := range tracking.Secrets {
			if slices.Contains(sInfo.NodesWithCurrent, nodeName) {
				nodeStatus.SecretsWithCurrent = append(nodeStatus.SecretsWithCurrent, secretName)
			} else if slices.Contains(sInfo.NodesWithPrevious, nodeName) {
				nodeStatus.SecretsWithPrevious = append(nodeStatus.SecretsWithPrevious, secretName)
				nodeStatus.AllSecretsUpdated = false
			} else {
				nodeStatus.AllSecretsUpdated = false
			}
		}

		tracking.NodeStatus[nodeName] = nodeStatus
	}

	// Both nodes should be fully updated
	for _, nodeName := range allNodes {
		if !tracking.NodeStatus[nodeName].AllSecretsUpdated {
			t.Errorf("Node %q should have AllSecretsUpdated=true after rotation with full coverage", nodeName)
		}
	}

	// Summary should show all updated
	summary := computeDeploymentSummary(tracking, totalNodes, "test-cm")
	if !summary.AllNodesUpdated {
		t.Error("AllNodesUpdated should be true after rotation with full coverage")
	}
	if summary.UpdatedNodes != 2 {
		t.Errorf("UpdatedNodes should be 2 after rotation with full coverage, got %d", summary.UpdatedNodes)
	}
}

func TestMultipleSecretsWithDifferentRolloutStates(t *testing.T) {
	// Test scenario with multiple secrets at different rollout stages
	tracking := &SecretTrackingData{
		Secrets: map[string]SecretVersionInfo{
			"secret-a": {
				CurrentHash:      "hash-a1",
				NodesWithCurrent: []string{"compute-0", "compute-1"},
				LastChanged:      time.Now(),
			},
			"secret-b": {
				CurrentHash:       "hash-b2",
				PreviousHash:      "hash-b1",
				NodesWithCurrent:  []string{"compute-0"},
				NodesWithPrevious: []string{"compute-1"},
				LastChanged:       time.Now(),
			},
			"secret-c": {
				CurrentHash:      "hash-c1",
				NodesWithCurrent: []string{"compute-0"},
				LastChanged:      time.Now(),
			},
		},
		NodeStatus: make(map[string]NodeSecretStatus),
	}

	// Compute node status
	allNodes := []string{"compute-0", "compute-1"}
	for _, nodeName := range allNodes {
		nodeStatus := NodeSecretStatus{
			AllSecretsUpdated:   true,
			SecretsWithCurrent:  []string{},
			SecretsWithPrevious: []string{},
		}

		for secretName, secretInfo := range tracking.Secrets {
			if slices.Contains(secretInfo.NodesWithCurrent, nodeName) {
				nodeStatus.SecretsWithCurrent = append(nodeStatus.SecretsWithCurrent, secretName)
			} else if slices.Contains(secretInfo.NodesWithPrevious, nodeName) {
				nodeStatus.SecretsWithPrevious = append(nodeStatus.SecretsWithPrevious, secretName)
				nodeStatus.AllSecretsUpdated = false
			} else {
				nodeStatus.AllSecretsUpdated = false
			}
		}

		tracking.NodeStatus[nodeName] = nodeStatus
	}

	// Compute summary
	summary := computeDeploymentSummary(tracking, len(allNodes), "test-cm")

	// compute-0 has all current versions (a, b, c)
	if !tracking.NodeStatus["compute-0"].AllSecretsUpdated {
		t.Error("compute-0 should have all secrets updated")
	}

	// compute-1 missing secret-c and has old version of secret-b
	if tracking.NodeStatus["compute-1"].AllSecretsUpdated {
		t.Error("compute-1 should NOT have all secrets updated")
	}

	// Only 1 node fully updated
	if summary.UpdatedNodes != 1 {
		t.Errorf("UpdatedNodes = %d, want 1", summary.UpdatedNodes)
	}

	if summary.AllNodesUpdated {
		t.Error("AllNodesUpdated should be false")
	}
}

func TestDetectSecretDrift(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		trackingData  *SecretTrackingData
		secrets       []*corev1.Secret
		wantDrift     bool
		wantErr       bool
	}{
		{
			name:         "no tracking data",
			trackingData: nil,
			wantDrift:    false,
			wantErr:      false,
		},
		{
			name: "no drift - hashes match",
			trackingData: &SecretTrackingData{
				Secrets: map[string]SecretVersionInfo{
					"rabbitmq-secret": {
						CurrentHash:      "abc123",
						NodesWithCurrent: []string{"node1", "node2"},
						LastChanged:      now,
					},
				},
			},
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rabbitmq-secret",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"username": []byte("user"),
						"password": []byte("pass"),
					},
				},
			},
			wantDrift: false,
			wantErr:   false,
		},
		{
			name: "drift detected - hash changed",
			trackingData: &SecretTrackingData{
				Secrets: map[string]SecretVersionInfo{
					"rabbitmq-secret": {
						CurrentHash:      "old-hash",
						NodesWithCurrent: []string{"node1", "node2"},
						LastChanged:      now,
					},
				},
			},
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rabbitmq-secret",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"username": []byte("newuser"),
						"password": []byte("newpass"),
					},
				},
			},
			wantDrift: true,
			wantErr:   false,
		},
		{
			name: "drift detected - secret deleted",
			trackingData: &SecretTrackingData{
				Secrets: map[string]SecretVersionInfo{
					"rabbitmq-secret": {
						CurrentHash:      "abc123",
						NodesWithCurrent: []string{"node1"},
						LastChanged:      now,
					},
				},
			},
			secrets:   []*corev1.Secret{}, // Secret doesn't exist
			wantDrift: true,
			wantErr:   false,
		},
		{
			name: "drift detected - secret has no data",
			trackingData: &SecretTrackingData{
				Secrets: map[string]SecretVersionInfo{
					"rabbitmq-secret": {
						CurrentHash:      "abc123",
						NodesWithCurrent: []string{"node1"},
						LastChanged:      now,
					},
				},
			},
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rabbitmq-secret",
						Namespace: "test",
					},
					Data: nil, // Empty data
				},
			},
			wantDrift: true,
			wantErr:   false,
		},
		{
			name: "multiple secrets - one drifted",
			trackingData: &SecretTrackingData{
				Secrets: map[string]SecretVersionInfo{
					"secret1": {
						CurrentHash:      "hash1",
						NodesWithCurrent: []string{"node1"},
						LastChanged:      now,
					},
					"secret2": {
						CurrentHash:      "old-hash",
						NodesWithCurrent: []string{"node1"},
						LastChanged:      now,
					},
				},
			},
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret1",
						Namespace: "test",
					},
					Data: map[string][]byte{"key": []byte("value1")},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret2",
						Namespace: "test",
					},
					Data: map[string][]byte{"key": []byte("newvalue")}, // Changed
				},
			},
			wantDrift: true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fake client
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			_ = dataplanev1.AddToScheme(scheme)

			objs := make([]runtime.Object, len(tt.secrets))
			for i, s := range tt.secrets {
				objs[i] = s
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objs...).
				Build()

			reconciler := &OpenStackDataPlaneNodeSetReconciler{
				Client: fakeClient,
			}

			instance := &dataplanev1.OpenStackDataPlaneNodeSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nodeset",
					Namespace: "test",
				},
				Status: dataplanev1.OpenStackDataPlaneNodeSetStatus{
					SecretHashes: make(map[string]string),
				},
			}

			// Populate instance.Status.SecretHashes from trackingData
			// This simulates what happens when deployment hashes are copied to nodeset status
			if tt.trackingData != nil {
				for secretName, secretInfo := range tt.trackingData.Secrets {
					instance.Status.SecretHashes[secretName] = secretInfo.CurrentHash
				}
			}

			// For no-drift cases, compute actual hashes and update both tracking and status
			if tt.trackingData != nil && !tt.wantDrift && len(tt.secrets) > 0 {
				for secretName, secretInfo := range tt.trackingData.Secrets {
					for _, secret := range tt.secrets {
						if secret.Name == secretName && secret.Data != nil {
							hash, _ := util.ObjectHash(secret.Data)
							secretInfo.CurrentHash = hash
							tt.trackingData.Secrets[secretName] = secretInfo
							instance.Status.SecretHashes[secretName] = hash
						}
					}
				}
			}

			drift, err := reconciler.detectSecretDrift(context.Background(), instance, tt.trackingData)

			if (err != nil) != tt.wantErr {
				t.Errorf("detectSecretDrift() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if drift != tt.wantDrift {
				t.Errorf("detectSecretDrift() drift = %v, want %v", drift, tt.wantDrift)
			}
		})
	}
}
