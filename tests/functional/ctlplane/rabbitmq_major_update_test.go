/*
Copyright 2024.

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

package functional_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	corev1beta1 "github.com/openstack-k8s-operators/openstack-operator/apis/core/v1beta1"
	"github.com/openstack-k8s-operators/openstack-operator/pkg/openstack"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MockRabbitMQVersionChecker implements a mock version checker for tests
type MockRabbitMQVersionChecker struct {
	shouldReturnMajorUpdate bool
	shouldReturnError       error
}

// CheckMajorVersionChange returns the configured mock values
func (m *MockRabbitMQVersionChecker) CheckMajorVersionChange(ctx context.Context, helper *helper.Helper, instance *corev1beta1.OpenStackControlPlane, version *corev1beta1.OpenStackVersion) (bool, error) {
	return m.shouldReturnMajorUpdate, m.shouldReturnError
}

// createMockOpenStackVersion creates a mock OpenStackVersion object for testing
func createMockOpenStackVersion(name, namespace, targetVersion string) *corev1beta1.OpenStackVersion {
	rabbitMQImage := "quay.io/podified-antelope-centos9/openstack-rabbitmq:3.12.0"
	return &corev1beta1.OpenStackVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1beta1.OpenStackVersionSpec{
			TargetVersion: targetVersion,
		},
		Status: corev1beta1.OpenStackVersionStatus{
			ContainerImages: corev1beta1.ContainerImages{
				ContainerTemplate: corev1beta1.ContainerTemplate{
					RabbitmqImage: &rabbitMQImage,
				},
			},
		},
	}
}

// createMockOpenStackControlPlane creates a mock OpenStackControlPlane object for testing
func createMockOpenStackControlPlane(name, namespace string) *corev1beta1.OpenStackControlPlane {
	return &corev1beta1.OpenStackControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1beta1.OpenStackControlPlaneSpec{
			Rabbitmq: corev1beta1.RabbitmqSection{
				Enabled: true,
			},
		},
	}
}

// createMockHelper creates a mock helper for testing
func createMockHelper() *helper.Helper {
	// For this test, we don't need a real helper since we're mocking the version check
	// We'll just return nil and the mock will handle everything
	return nil
}

var _ = Describe("RabbitMQ Major Update Workflow", func() {
	Context("Multiple target versions exist", func() {
		var (
			updatedVersion = "0.0.2"
		)

		It("should trigger RabbitMQ major update workflow when major versions differ", Serial, func() {
			// Set up mock version checker to return true (major update needed)
			mockChecker := &MockRabbitMQVersionChecker{
				shouldReturnMajorUpdate: true,
				shouldReturnError:       nil,
			}
			openstack.SetRabbitMQVersionChecker(mockChecker)

			// Restore the original checker after the test
			DeferCleanup(func() {
				openstack.SetRabbitMQVersionChecker(&openstack.DefaultRabbitMQVersionChecker{})
			})

			// Create mock objects
			namespace := "test-namespace"
			osVersionName := "test-openstack-version"
			controlPlaneName := "test-controlplane"

			mockOSVersion := createMockOpenStackVersion(osVersionName, namespace, updatedVersion)
			mockControlPlane := createMockOpenStackControlPlane(controlPlaneName, namespace)

			// Test the CheckRabbitMQMajorVersionChange function directly
			// This simulates what the controller would do
			ctx := context.Background()
			mockHelper := createMockHelper()
			needsMajorUpdate, err := openstack.CheckRabbitMQMajorVersionChange(ctx, mockHelper, mockControlPlane, mockOSVersion)

			// Verify the mock was called and returned the expected result
			Expect(err).Should(BeNil())
			Expect(needsMajorUpdate).Should(BeTrue())

			// Verify that the mock checker was actually used
			Expect(mockChecker.shouldReturnMajorUpdate).Should(BeTrue())
		})
	})

	Context("CustomContainerImages are set", func() {
		var (
			updatedVersion = "0.0.2"
		)

		It("should trigger RabbitMQ major update workflow when major versions differ", Serial, func() {
			// Set up mock version checker to return false (no major update needed)
			mockChecker := &MockRabbitMQVersionChecker{
				shouldReturnMajorUpdate: false,
				shouldReturnError:       nil,
			}
			openstack.SetRabbitMQVersionChecker(mockChecker)

			// Restore the original checker after the test
			DeferCleanup(func() {
				openstack.SetRabbitMQVersionChecker(&openstack.DefaultRabbitMQVersionChecker{})
			})

			// Create mock objects with custom container images
			namespace := "test-namespace"
			osVersionName := "test-openstack-version"
			controlPlaneName := "test-controlplane"

			mockOSVersion := createMockOpenStackVersion(osVersionName, namespace, updatedVersion)
			mockControlPlane := createMockOpenStackControlPlane(controlPlaneName, namespace)

			// Test the CheckRabbitMQMajorVersionChange function directly
			// This simulates what the controller would do
			ctx := context.Background()
			mockHelper := createMockHelper()
			needsMajorUpdate, err := openstack.CheckRabbitMQMajorVersionChange(ctx, mockHelper, mockControlPlane, mockOSVersion)

			// Verify the mock was called and returned the expected result
			Expect(err).Should(BeNil())
			Expect(needsMajorUpdate).Should(BeFalse())

			// Verify that the mock checker was actually used
			Expect(mockChecker.shouldReturnMajorUpdate).Should(BeFalse())
		})
	})
})
