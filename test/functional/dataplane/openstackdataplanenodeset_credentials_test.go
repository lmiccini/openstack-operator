package functional

import (
	"encoding/json"
	"os"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	dataplaneutil "github.com/openstack-k8s-operators/openstack-operator/internal/dataplane/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
)

type deployedNodeTracking struct {
	DeployedSecretHash string   `json:"deployedSecretHash"`
	DeployedNodes      []string `json:"deployedNodes"`
}

var _ = Describe("Dataplane NodeSet Credential Tracking Test", func() {
	var (
		dataplaneDeploymentName types.NamespacedName
		dataplaneNodeSetName    types.NamespacedName
		dataplaneSSHSecretName  types.NamespacedName
		dataplaneNetConfigName  types.NamespacedName
		dnsMasqName             types.NamespacedName
		dataplaneNodeName       types.NamespacedName
		caBundleSecretName      types.NamespacedName
		credentialSecretName    types.NamespacedName
		credentialServiceName   types.NamespacedName
		trackingConfigMapName   types.NamespacedName
	)

	BeforeEach(func() {
		dataplaneNodeSetName = types.NamespacedName{
			Name:      "edpm-compute-nodeset",
			Namespace: namespace,
		}
		dataplaneDeploymentName = types.NamespacedName{
			Name:      "edpm-deployment",
			Namespace: namespace,
		}
		dataplaneSSHSecretName = types.NamespacedName{
			Name:      "dataplane-ansible-ssh-private-key-secret",
			Namespace: namespace,
		}
		dataplaneNetConfigName = types.NamespacedName{
			Name:      "dataplane-netconfig",
			Namespace: namespace,
		}
		dnsMasqName = types.NamespacedName{
			Name:      "dnsmasq",
			Namespace: namespace,
		}
		dataplaneNodeName = types.NamespacedName{
			Name:      "edpm-compute-nodeset-node-1",
			Namespace: namespace,
		}
		caBundleSecretName = types.NamespacedName{
			Name:      "combined-ca-bundle",
			Namespace: namespace,
		}
		credentialSecretName = types.NamespacedName{
			Name:      "credential-test-secret",
			Namespace: namespace,
		}
		credentialServiceName = types.NamespacedName{
			Name:      "credential-service",
			Namespace: namespace,
		}
		trackingConfigMapName = types.NamespacedName{
			Name:      "edpm-compute-nodeset-secret-tracking",
			Namespace: namespace,
		}

		err := os.Setenv("OPERATOR_SERVICES", "../../../config/services")
		Expect(err).NotTo(HaveOccurred())
	})

	When("A deployment completes with a service that references a secret", func() {
		BeforeEach(func() {
			CreateSSHSecret(dataplaneSSHSecretName)
			CreateCABundleSecret(caBundleSecretName)
			DeferCleanup(th.DeleteInstance, th.CreateSecret(credentialSecretName, map[string][]byte{
				"credential-key": []byte("credential-value-v1"),
			}))

			DeferCleanup(th.DeleteInstance, CreateDataPlaneServiceFromSpec(
				credentialServiceName, map[string]interface{}{
					"dataSources": []map[string]interface{}{
						{
							"secretRef": map[string]interface{}{
								"name": credentialSecretName.Name,
							},
						},
					},
				}))
			DeferCleanup(th.DeleteService, credentialServiceName)

			DeferCleanup(th.DeleteInstance, CreateNetConfig(dataplaneNetConfigName, DefaultNetConfigSpec()))
			DeferCleanup(th.DeleteInstance, CreateDNSMasq(dnsMasqName, DefaultDNSMasqSpec()))
			SimulateDNSMasqComplete(dnsMasqName)

			DeferCleanup(th.DeleteInstance, CreateDataplaneNodeSet(
				dataplaneNodeSetName,
				CredentialTestNodeSetSpec(dataplaneNodeSetName.Name, credentialServiceName.Name)))
			SimulateIPSetComplete(dataplaneNodeName)
			SimulateDNSDataComplete(dataplaneNodeSetName)

			DeferCleanup(th.DeleteInstance, CreateDataplaneDeployment(
				dataplaneDeploymentName, map[string]interface{}{
					"nodeSets": []string{dataplaneNodeSetName.Name},
				}))

			nodeSet := GetDataplaneNodeSet(dataplaneNodeSetName)
			for _, serviceName := range nodeSet.Spec.Services {
				svcName := types.NamespacedName{
					Name:      serviceName,
					Namespace: namespace,
				}
				service := GetService(svcName)
				deployment := GetDataplaneDeployment(dataplaneDeploymentName)
				aeeName, _ := dataplaneutil.GetAnsibleExecutionNameAndLabels(
					service, deployment.GetName(), nodeSet.GetName())
				Eventually(func(g Gomega) {
					ansibleeeName := types.NamespacedName{
						Name:      aeeName,
						Namespace: namespace,
					}
					ansibleEE := GetAnsibleee(ansibleeeName)
					ansibleEE.Status.Succeeded = 1
					g.Expect(th.K8sClient.Status().Update(th.Ctx, ansibleEE)).To(Succeed())
				}, th.Timeout, th.Interval).Should(Succeed())
			}

			th.ExpectCondition(
				dataplaneDeploymentName,
				ConditionGetterFunc(DataplaneDeploymentConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("Should create the secret-tracking ConfigMap with valid tracking data", func() {
			cm := GetSecretTrackingConfigMap(trackingConfigMapName)
			Expect(cm.Data).To(HaveKey("tracking.json"))

			var td deployedNodeTracking
			err := json.Unmarshal([]byte(cm.Data["tracking.json"]), &td)
			Expect(err).NotTo(HaveOccurred())

			Expect(td.DeployedSecretHash).NotTo(BeEmpty())
			Expect(td.DeployedNodes).To(ContainElement(dataplaneNodeName.Name))
		})

		It("Should populate SecretDeploymentStatus in the NodeSet status", func() {
			Eventually(func(g Gomega) {
				nodeSet := GetDataplaneNodeSet(dataplaneNodeSetName)
				g.Expect(nodeSet.Status.SecretDeployment).NotTo(BeNil())
				g.Expect(nodeSet.Status.SecretDeployment.AllNodesUpdated).To(BeTrue())
				g.Expect(nodeSet.Status.SecretDeployment.TotalNodes).To(Equal(1))
				g.Expect(nodeSet.Status.SecretDeployment.UpdatedNodes).To(Equal(1))
				g.Expect(nodeSet.Status.SecretDeployment.DeployedSecretHash).NotTo(BeEmpty())
				g.Expect(nodeSet.Status.SecretDeployment.LastUpdateTime).NotTo(BeNil())
			}, th.Timeout, th.Interval).Should(Succeed())
		})
	})

	When("A secret changes after initial deployment (drift detection)", func() {
		BeforeEach(func() {
			CreateSSHSecret(dataplaneSSHSecretName)
			CreateCABundleSecret(caBundleSecretName)
			DeferCleanup(th.DeleteInstance, th.CreateSecret(credentialSecretName, map[string][]byte{
				"credential-key": []byte("credential-value-v1"),
			}))

			DeferCleanup(th.DeleteInstance, CreateDataPlaneServiceFromSpec(
				credentialServiceName, map[string]interface{}{
					"dataSources": []map[string]interface{}{
						{
							"secretRef": map[string]interface{}{
								"name": credentialSecretName.Name,
							},
						},
					},
				}))
			DeferCleanup(th.DeleteService, credentialServiceName)

			DeferCleanup(th.DeleteInstance, CreateNetConfig(dataplaneNetConfigName, DefaultNetConfigSpec()))
			DeferCleanup(th.DeleteInstance, CreateDNSMasq(dnsMasqName, DefaultDNSMasqSpec()))
			SimulateDNSMasqComplete(dnsMasqName)

			DeferCleanup(th.DeleteInstance, CreateDataplaneNodeSet(
				dataplaneNodeSetName,
				CredentialTestNodeSetSpec(dataplaneNodeSetName.Name, credentialServiceName.Name)))
			SimulateIPSetComplete(dataplaneNodeName)
			SimulateDNSDataComplete(dataplaneNodeSetName)

			DeferCleanup(th.DeleteInstance, CreateDataplaneDeployment(
				dataplaneDeploymentName, map[string]interface{}{
					"nodeSets": []string{dataplaneNodeSetName.Name},
				}))

			nodeSet := GetDataplaneNodeSet(dataplaneNodeSetName)
			for _, serviceName := range nodeSet.Spec.Services {
				svcName := types.NamespacedName{
					Name:      serviceName,
					Namespace: namespace,
				}
				service := GetService(svcName)
				deployment := GetDataplaneDeployment(dataplaneDeploymentName)
				aeeName, _ := dataplaneutil.GetAnsibleExecutionNameAndLabels(
					service, deployment.GetName(), nodeSet.GetName())
				Eventually(func(g Gomega) {
					ansibleeeName := types.NamespacedName{
						Name:      aeeName,
						Namespace: namespace,
					}
					ansibleEE := GetAnsibleee(ansibleeeName)
					ansibleEE.Status.Succeeded = 1
					g.Expect(th.K8sClient.Status().Update(th.Ctx, ansibleEE)).To(Succeed())
				}, th.Timeout, th.Interval).Should(Succeed())
			}

			th.ExpectCondition(
				dataplaneDeploymentName,
				ConditionGetterFunc(DataplaneDeploymentConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				ns := GetDataplaneNodeSet(dataplaneNodeSetName)
				g.Expect(ns.Status.SecretDeployment).NotTo(BeNil())
				g.Expect(ns.Status.SecretDeployment.AllNodesUpdated).To(BeTrue())
			}, th.Timeout, th.Interval).Should(Succeed())

			Eventually(func(g Gomega) {
				secret := th.GetSecret(credentialSecretName)
				secret.Data["credential-key"] = []byte("credential-value-v2-rotated")
				g.Expect(th.K8sClient.Update(th.Ctx, &secret)).To(Succeed())
			}, th.Timeout, th.Interval).Should(Succeed())
		})

		It("Should detect drift and set AllNodesUpdated to false", func() {
			Eventually(func(g Gomega) {
				nodeSet := GetDataplaneNodeSet(dataplaneNodeSetName)
				g.Expect(nodeSet.Status.SecretDeployment).NotTo(BeNil())
				g.Expect(nodeSet.Status.SecretDeployment.AllNodesUpdated).To(BeFalse())
				g.Expect(nodeSet.Status.SecretDeployment.UpdatedNodes).To(Equal(0))
			}, th.Timeout, th.Interval).Should(Succeed())
		})
	})

	When("A second deployment runs after secret rotation", func() {
		BeforeEach(func() {
			CreateSSHSecret(dataplaneSSHSecretName)
			CreateCABundleSecret(caBundleSecretName)
			DeferCleanup(th.DeleteInstance, th.CreateSecret(credentialSecretName, map[string][]byte{
				"credential-key": []byte("credential-value-v1"),
			}))

			DeferCleanup(th.DeleteInstance, CreateDataPlaneServiceFromSpec(
				credentialServiceName, map[string]interface{}{
					"dataSources": []map[string]interface{}{
						{
							"secretRef": map[string]interface{}{
								"name": credentialSecretName.Name,
							},
						},
					},
				}))
			DeferCleanup(th.DeleteService, credentialServiceName)

			DeferCleanup(th.DeleteInstance, CreateNetConfig(dataplaneNetConfigName, DefaultNetConfigSpec()))
			DeferCleanup(th.DeleteInstance, CreateDNSMasq(dnsMasqName, DefaultDNSMasqSpec()))
			SimulateDNSMasqComplete(dnsMasqName)

			DeferCleanup(th.DeleteInstance, CreateDataplaneNodeSet(
				dataplaneNodeSetName,
				CredentialTestNodeSetSpec(dataplaneNodeSetName.Name, credentialServiceName.Name)))
			SimulateIPSetComplete(dataplaneNodeName)
			SimulateDNSDataComplete(dataplaneNodeSetName)

			// First deployment
			DeferCleanup(th.DeleteInstance, CreateDataplaneDeployment(
				dataplaneDeploymentName, map[string]interface{}{
					"nodeSets": []string{dataplaneNodeSetName.Name},
				}))

			nodeSet := GetDataplaneNodeSet(dataplaneNodeSetName)
			for _, serviceName := range nodeSet.Spec.Services {
				svcName := types.NamespacedName{
					Name:      serviceName,
					Namespace: namespace,
				}
				service := GetService(svcName)
				deployment := GetDataplaneDeployment(dataplaneDeploymentName)
				aeeName, _ := dataplaneutil.GetAnsibleExecutionNameAndLabels(
					service, deployment.GetName(), nodeSet.GetName())
				Eventually(func(g Gomega) {
					ansibleeeName := types.NamespacedName{
						Name:      aeeName,
						Namespace: namespace,
					}
					ansibleEE := GetAnsibleee(ansibleeeName)
					ansibleEE.Status.Succeeded = 1
					g.Expect(th.K8sClient.Status().Update(th.Ctx, ansibleEE)).To(Succeed())
				}, th.Timeout, th.Interval).Should(Succeed())
			}

			th.ExpectCondition(
				dataplaneDeploymentName,
				ConditionGetterFunc(DataplaneDeploymentConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				ns := GetDataplaneNodeSet(dataplaneNodeSetName)
				g.Expect(ns.Status.SecretDeployment).NotTo(BeNil())
				g.Expect(ns.Status.SecretDeployment.AllNodesUpdated).To(BeTrue())
			}, th.Timeout, th.Interval).Should(Succeed())

			// Rotate the secret
			Eventually(func(g Gomega) {
				secret := th.GetSecret(credentialSecretName)
				secret.Data["credential-key"] = []byte("credential-value-v2-rotated")
				g.Expect(th.K8sClient.Update(th.Ctx, &secret)).To(Succeed())
			}, th.Timeout, th.Interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ns := GetDataplaneNodeSet(dataplaneNodeSetName)
				g.Expect(ns.Status.SecretDeployment).NotTo(BeNil())
				g.Expect(ns.Status.SecretDeployment.AllNodesUpdated).To(BeFalse())
			}, th.Timeout, th.Interval).Should(Succeed())

			// Second deployment to deploy the rotated secret
			secondDeploymentName := types.NamespacedName{
				Name:      "edpm-deployment-rotation",
				Namespace: namespace,
			}
			DeferCleanup(th.DeleteInstance, CreateDataplaneDeployment(
				secondDeploymentName, map[string]interface{}{
					"nodeSets": []string{dataplaneNodeSetName.Name},
				}))

			nodeSet = GetDataplaneNodeSet(dataplaneNodeSetName)
			for _, serviceName := range nodeSet.Spec.Services {
				svcName := types.NamespacedName{
					Name:      serviceName,
					Namespace: namespace,
				}
				service := GetService(svcName)
				deployment := GetDataplaneDeployment(secondDeploymentName)
				aeeName, _ := dataplaneutil.GetAnsibleExecutionNameAndLabels(
					service, deployment.GetName(), nodeSet.GetName())
				Eventually(func(g Gomega) {
					ansibleeeName := types.NamespacedName{
						Name:      aeeName,
						Namespace: namespace,
					}
					ansibleEE := GetAnsibleee(ansibleeeName)
					ansibleEE.Status.Succeeded = 1
					g.Expect(th.K8sClient.Status().Update(th.Ctx, ansibleEE)).To(Succeed())
				}, th.Timeout, th.Interval).Should(Succeed())
			}

			th.ExpectCondition(
				secondDeploymentName,
				ConditionGetterFunc(DataplaneDeploymentConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("Should update tracking to reflect the new secret version", func() {
			Eventually(func(g Gomega) {
				nodeSet := GetDataplaneNodeSet(dataplaneNodeSetName)
				g.Expect(nodeSet.Status.SecretDeployment).NotTo(BeNil())
				g.Expect(nodeSet.Status.SecretDeployment.AllNodesUpdated).To(BeTrue())
				g.Expect(nodeSet.Status.SecretDeployment.UpdatedNodes).To(Equal(1))
				g.Expect(nodeSet.Status.SecretDeployment.DeployedSecretHash).NotTo(BeEmpty())
			}, th.Timeout, th.Interval).Should(Succeed())

			cm := GetSecretTrackingConfigMap(trackingConfigMapName)
			var td deployedNodeTracking
			err := json.Unmarshal([]byte(cm.Data["tracking.json"]), &td)
			Expect(err).NotTo(HaveOccurred())

			Expect(td.DeployedSecretHash).NotTo(BeEmpty())
			Expect(td.DeployedNodes).To(ContainElement(dataplaneNodeName.Name))
		})
	})
})
