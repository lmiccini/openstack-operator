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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
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
			name:            "empty limit covers all nodes",
			ansibleLimit:    "",
			nodesetNodes:    []string{"node-0", "node-1", "node-2"},
			expectedCovered: []string{"node-0", "node-1", "node-2"},
		},
		{
			name:            "wildcard covers all nodes",
			ansibleLimit:    "*",
			nodesetNodes:    []string{"node-0", "node-1"},
			expectedCovered: []string{"node-0", "node-1"},
		},
		{
			name:            "exact match",
			ansibleLimit:    "node-1",
			nodesetNodes:    []string{"node-0", "node-1", "node-2"},
			expectedCovered: []string{"node-1"},
		},
		{
			name:            "comma-separated list",
			ansibleLimit:    "node-0,node-2",
			nodesetNodes:    []string{"node-0", "node-1", "node-2"},
			expectedCovered: []string{"node-0", "node-2"},
		},
		{
			name:            "prefix wildcard",
			ansibleLimit:    "compute-*",
			nodesetNodes:    []string{"compute-0", "compute-1", "control-0"},
			expectedCovered: []string{"compute-0", "compute-1"},
		},
		{
			name:            "non-matching limit",
			ansibleLimit:    "missing-node",
			nodesetNodes:    []string{"node-0", "node-1"},
			expectedCovered: []string{},
		},
		{
			name:            "nil deployment",
			ansibleLimit:    "",
			nodesetNodes:    nil,
			expectedCovered: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := make(map[string]dataplanev1.NodeSection)
			for _, n := range tt.nodesetNodes {
				nodes[n] = dataplanev1.NodeSection{}
			}

			var deploy *dataplanev1.OpenStackDataPlaneDeployment
			var nodeset *dataplanev1.OpenStackDataPlaneNodeSet

			if tt.nodesetNodes == nil {
				deploy = nil
				nodeset = nil
			} else {
				deploy = &dataplanev1.OpenStackDataPlaneDeployment{
					Spec: dataplanev1.OpenStackDataPlaneDeploymentSpec{
						AnsibleLimit: tt.ansibleLimit,
					},
				}
				nodeset = &dataplanev1.OpenStackDataPlaneNodeSet{
					Spec: dataplanev1.OpenStackDataPlaneNodeSetSpec{
						Nodes: nodes,
					},
				}
			}

			covered := getNodesCoveredByDeployment(deploy, nodeset)

			if len(tt.expectedCovered) == 0 && len(covered) == 0 {
				return
			}

			for _, expected := range tt.expectedCovered {
				if !slices.Contains(covered, expected) {
					t.Errorf("expected node %q to be covered, but it was not. Got: %v", expected, covered)
				}
			}
			if len(covered) != len(tt.expectedCovered) {
				t.Errorf("expected %d covered nodes, got %d: %v", len(tt.expectedCovered), len(covered), covered)
			}
		})
	}
}

func TestGetAllNodeNames(t *testing.T) {
	tests := []struct {
		name     string
		nodeset  *dataplanev1.OpenStackDataPlaneNodeSet
		expected int
	}{
		{
			name:     "nil nodeset",
			nodeset:  nil,
			expected: 0,
		},
		{
			name: "three nodes",
			nodeset: &dataplanev1.OpenStackDataPlaneNodeSet{
				Spec: dataplanev1.OpenStackDataPlaneNodeSetSpec{
					Nodes: map[string]dataplanev1.NodeSection{
						"node-0": {}, "node-1": {}, "node-2": {},
					},
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getAllNodeNames(tt.nodeset)
			if len(result) != tt.expected {
				t.Errorf("expected %d nodes, got %d: %v", tt.expected, len(result), result)
			}
		})
	}
}

func TestComputeCompositeSecretHash(t *testing.T) {
	tests := []struct {
		name   string
		hashes map[string]string
		empty  bool
	}{
		{
			name:   "nil map returns empty",
			hashes: nil,
			empty:  true,
		},
		{
			name:   "empty map returns empty",
			hashes: map[string]string{},
			empty:  true,
		},
		{
			name:   "single secret",
			hashes: map[string]string{"secret-a": "hash1"},
			empty:  false,
		},
		{
			name:   "multiple secrets",
			hashes: map[string]string{"secret-a": "hash1", "secret-b": "hash2"},
			empty:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeCompositeSecretHash(tt.hashes)
			if tt.empty && result != "" {
				t.Errorf("expected empty hash, got %q", result)
			}
			if !tt.empty && result == "" {
				t.Errorf("expected non-empty hash, got empty")
			}
		})
	}
}

func TestCompositeHashDeterminism(t *testing.T) {
	hashes := map[string]string{
		"secret-b": "hash2",
		"secret-a": "hash1",
		"secret-c": "hash3",
	}

	first := computeCompositeSecretHash(hashes)
	for i := 0; i < 10; i++ {
		result := computeCompositeSecretHash(hashes)
		if result != first {
			t.Errorf("non-deterministic hash: got %q, expected %q (iteration %d)", result, first, i)
		}
	}
}

func TestCompositeHashSensitivity(t *testing.T) {
	hash1 := computeCompositeSecretHash(map[string]string{"s1": "a", "s2": "b"})
	hash2 := computeCompositeSecretHash(map[string]string{"s1": "a", "s2": "c"})
	hash3 := computeCompositeSecretHash(map[string]string{"s1": "a", "s3": "b"})

	if hash1 == hash2 {
		t.Error("hash should differ when a value changes")
	}
	if hash1 == hash3 {
		t.Error("hash should differ when a key changes")
	}
}

func TestComputeClusterSecretHash(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = dataplanev1.AddToScheme(scheme)

	sec1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "secret-a", Namespace: "test-ns"},
		Data:       map[string][]byte{"key": []byte("value-a")},
	}
	sec2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "secret-b", Namespace: "test-ns"},
		Data:       map[string][]byte{"key": []byte("value-b")},
	}

	h1, _ := secret.Hash(sec1)
	h2, _ := secret.Hash(sec2)

	expectedComposite := computeCompositeSecretHash(map[string]string{
		"secret-a": h1,
		"secret-b": h2,
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sec1, sec2).
		Build()

	r := &OpenStackDataPlaneNodeSetReconciler{Client: fakeClient}

	result, err := r.computeClusterSecretHash(context.Background(), "test-ns", []string{"secret-a", "secret-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != expectedComposite {
		t.Errorf("expected %q, got %q", expectedComposite, result)
	}
}

func TestComputeClusterSecretHashMissingSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = dataplanev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &OpenStackDataPlaneNodeSetReconciler{Client: fakeClient}

	result, err := r.computeClusterSecretHash(context.Background(), "test-ns", []string{"deleted-secret"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Deleted secret gets empty hash, composite should still be non-empty
	if result == "" {
		t.Error("expected non-empty composite hash even with deleted secret")
	}
	// Should differ from hash of the secret when it existed
	if result == computeCompositeSecretHash(map[string]string{"deleted-secret": "old-hash"}) {
		t.Error("hash should change when secret is deleted")
	}
}

func TestComputeClusterSecretHashEmpty(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = dataplanev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &OpenStackDataPlaneNodeSetReconciler{Client: fakeClient}

	result, err := r.computeClusterSecretHash(context.Background(), "test-ns", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty hash for no secrets, got %q", result)
	}
}

func TestDeployedNodeTrackingRoundTrip(t *testing.T) {
	original := &DeployedNodeTracking{
		DeployedSecretHash: "abc123",
		DeployedNodes:      []string{"node-0", "node-1", "node-2"},
	}

	data, err := serializeTracking(original)
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	restored, err := deserializeTracking(data)
	if err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	if restored.DeployedSecretHash != original.DeployedSecretHash {
		t.Errorf("hash mismatch: %q vs %q", restored.DeployedSecretHash, original.DeployedSecretHash)
	}
	if len(restored.DeployedNodes) != len(original.DeployedNodes) {
		t.Errorf("node count mismatch: %d vs %d", len(restored.DeployedNodes), len(original.DeployedNodes))
	}
}

func serializeTracking(data *DeployedNodeTracking) (string, error) {
	b, err := json.Marshal(data)
	return string(b), err
}

func deserializeTracking(data string) (*DeployedNodeTracking, error) {
	var tracking DeployedNodeTracking
	if err := json.Unmarshal([]byte(data), &tracking); err != nil {
		return nil, err
	}
	return &tracking, nil
}
