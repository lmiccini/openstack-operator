package openstack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	certmgrv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/certmanager"
	"github.com/openstack-k8s-operators/lib-common/modules/common/clusterdns"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/object"
	"github.com/openstack-k8s-operators/lib-common/modules/common/rsh"
	dataplanev1 "github.com/openstack-k8s-operators/openstack-operator/apis/dataplane/v1beta1"

	// Cannot use the following import due to linting error:
	// Error: 	pkg/openstack/rabbitmq.go:10:2: use of internal package github.com/rabbitmq/cluster-operator/internal/status not allowed
	//rabbitstatus "github.com/rabbitmq/cluster-operator/internal/status"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1beta1 "github.com/openstack-k8s-operators/openstack-operator/apis/core/v1beta1"
	"k8s.io/utils/ptr"

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	rabbitmqv2 "github.com/rabbitmq/cluster-operator/v2/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

type mqStatus int

const (
	mqFailed   mqStatus = iota
	mqCreating mqStatus = iota
	mqReady    mqStatus = iota
)

func deleteUndefinedRabbitMQs(
	ctx context.Context,
	instance *corev1beta1.OpenStackControlPlane,
	helper *helper.Helper,
) (ctrl.Result, error) {

	log := GetLogger(ctx)
	// Fetch the list of RabbitMQ objects
	rabbitList := &rabbitmqv1.RabbitMqList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.GetNamespace()),
	}
	err := helper.GetClient().List(ctx, rabbitList, listOpts...)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("could not list rabbitmqs %w", err)
	}

	var delErrs []error
	for _, rabbitObj := range rabbitList.Items {
		// if it is not defined in the OpenStackControlPlane then delete it from k8s.
		if _, exists := (*instance.Spec.Rabbitmq.Templates)[rabbitObj.Name]; !exists {
			if object.CheckOwnerRefExist(instance.GetUID(), rabbitObj.OwnerReferences) {
				log.Info("Deleting Rabbitmq", "", rabbitObj.Name)

				certName := fmt.Sprintf("%s-svc", rabbitObj.Name)
				err = DeleteCertificate(ctx, helper, instance.Namespace, certName)
				if err != nil {
					delErrs = append(delErrs, fmt.Errorf("rabbit cert deletion for '%s' failed, because: %w", certName, err))
					continue
				}

				if _, err := EnsureDeleted(ctx, helper, &rabbitObj); err != nil {
					delErrs = append(delErrs, fmt.Errorf("rabbitmq deletion for '%s' failed, because: %w", rabbitObj.Name, err))
				}
			}
		}
	}

	if len(delErrs) > 0 {
		delErrs := errors.Join(delErrs...)
		return ctrl.Result{}, delErrs
	}

	return ctrl.Result{}, nil
}

// ReconcileRabbitMQs -
func ReconcileRabbitMQs(
	ctx context.Context,
	instance *corev1beta1.OpenStackControlPlane,
	version *corev1beta1.OpenStackVersion,
	helper *helper.Helper,
) (ctrl.Result, error) {
	var failures = []string{}
	var inprogress = []string{}
	var ctrlResult ctrl.Result
	var err error
	var status mqStatus

	if instance.Spec.Rabbitmq.Templates == nil {
		instance.Spec.Rabbitmq.Templates = ptr.To(map[string]rabbitmqv1.RabbitMqSpecCore{})
	}

	// Sort template names to ensure consistent ordering
	templateNames := make([]string, 0, len(*instance.Spec.Rabbitmq.Templates))
	for name := range *instance.Spec.Rabbitmq.Templates {
		templateNames = append(templateNames, name)
	}
	sort.Strings(templateNames)

	for _, name := range templateNames {
		spec := (*instance.Spec.Rabbitmq.Templates)[name]
		status, ctrlResult, err = reconcileRabbitMQ(ctx, instance, version, helper, name, spec)

		switch status {
		case mqFailed:
			failures = append(failures, fmt.Sprintf("%s(%v)", name, err.Error()))
		case mqCreating:
			inprogress = append(inprogress, name)
		case mqReady:
		default:
			return ctrl.Result{}, fmt.Errorf("invalid mqStatus from reconcileRabbitMQ: %d for RAbbitMQ %s", status, name)
		}
	}

	if len(failures) > 0 {
		errors := strings.Join(failures, ",")

		instance.Status.Conditions.Set(condition.FalseCondition(
			corev1beta1.OpenStackControlPlaneRabbitMQReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			corev1beta1.OpenStackControlPlaneRabbitMQReadyErrorMessage,
			errors))

		return ctrl.Result{}, fmt.Errorf("%s", errors)

	} else if len(inprogress) > 0 {
		instance.Status.Conditions.Set(condition.FalseCondition(
			corev1beta1.OpenStackControlPlaneRabbitMQReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			corev1beta1.OpenStackControlPlaneRabbitMQReadyRunningMessage))
	} else {
		instance.Status.Conditions.MarkTrue(
			corev1beta1.OpenStackControlPlaneRabbitMQReadyCondition,
			corev1beta1.OpenStackControlPlaneRabbitMQReadyMessage,
		)
	}

	_, errs := deleteUndefinedRabbitMQs(ctx, instance, helper)
	if errs != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			corev1beta1.OpenStackControlPlaneRabbitMQReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			corev1beta1.OpenStackControlPlaneRabbitMQReadyErrorMessage,
			errs))
		return ctrl.Result{}, errs
	}

	return ctrlResult, nil
}

func reconcileRabbitMQ(
	ctx context.Context,
	instance *corev1beta1.OpenStackControlPlane,
	version *corev1beta1.OpenStackVersion,
	helper *helper.Helper,
	name string,
	spec rabbitmqv1.RabbitMqSpecCore,
) (mqStatus, ctrl.Result, error) {
	rabbitmq := &rabbitmqv1.RabbitMq{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
		},
	}
	log := GetLogger(ctx)

	log.Info("Reconciling RabbitMQ", "RabbitMQ.Namespace", instance.Namespace, "RabbitMQ.Name", name)
	if !instance.Spec.Rabbitmq.Enabled {
		if _, err := EnsureDeleted(ctx, helper, rabbitmq); err != nil {
			return mqFailed, ctrl.Result{}, err
		}
		instance.Status.Conditions.Remove(corev1beta1.OpenStackControlPlaneRabbitMQReadyCondition)
		instance.Status.ContainerImages.RabbitmqImage = nil
		return mqReady, ctrl.Result{}, nil
	}

	clusterDomain := clusterdns.GetDNSClusterDomain()
	hostname := fmt.Sprintf("%s.%s.svc", name, instance.Namespace)
	hostnameHeadless := fmt.Sprintf("%s-nodes.%s.svc", name, instance.Namespace)
	hostnames := []string{
		hostname,
		fmt.Sprintf("%s.%s", hostname, clusterDomain),
		hostnameHeadless,
		fmt.Sprintf("%s.%s", hostnameHeadless, clusterDomain),
	}
	for i := 0; i < int(*spec.Replicas); i++ {
		hostnames = append(hostnames, fmt.Sprintf("%s-server-%d.%s-nodes.%s", name, i, name, instance.Namespace))
	}

	tlsCert := ""
	if instance.Spec.TLS.PodLevel.Enabled {
		certRequest := certmanager.CertificateRequest{
			IssuerName: instance.GetInternalIssuer(),
			CertName:   fmt.Sprintf("%s-svc", rabbitmq.Name),
			Hostnames:  hostnames,
			Subject: &certmgrv1.X509Subject{
				Organizations: []string{fmt.Sprintf("%s.%s", rabbitmq.Namespace, clusterDomain)},
			},
			Usages: []certmgrv1.KeyUsage{
				certmgrv1.UsageKeyEncipherment,
				certmgrv1.UsageDataEncipherment,
				certmgrv1.UsageDigitalSignature,
				certmgrv1.UsageServerAuth,
				certmgrv1.UsageClientAuth,
				certmgrv1.UsageContentCommitment,
			},
			Labels: map[string]string{serviceCertSelector: ""},
		}
		if instance.Spec.TLS.PodLevel.Internal.Cert.Duration != nil {
			certRequest.Duration = &instance.Spec.TLS.PodLevel.Internal.Cert.Duration.Duration
		}
		if instance.Spec.TLS.PodLevel.Internal.Cert.RenewBefore != nil {
			certRequest.RenewBefore = &instance.Spec.TLS.PodLevel.Internal.Cert.RenewBefore.Duration
		}
		certSecret, ctrlResult, err := certmanager.EnsureCert(
			ctx,
			helper,
			certRequest,
			nil)
		if err != nil {
			return mqFailed, ctrlResult, err
		} else if (ctrlResult != ctrl.Result{}) {
			return mqCreating, ctrlResult, nil
		}

		tlsCert = certSecret.Name
	}

	if spec.NodeSelector == nil {
		spec.NodeSelector = &instance.Spec.NodeSelector
	}

	// When there's no Topology referenced in the Service Template, inject the
	// top-level one
	if spec.TopologyRef == nil {
		spec.TopologyRef = instance.Spec.TopologyRef
	}

	// infra operator is now the controller
	err := removeRabbitmqClusterControllerReference(ctx, helper, instance, name)
	if err != nil {
		return mqFailed, ctrl.Result{}, err
	}
	// infra operator is now the controller
	err = removeConfigMapControllerReference(ctx, helper, instance, name)
	if err != nil {
		return mqFailed, ctrl.Result{}, err
	}

	op, err := controllerutil.CreateOrPatch(ctx, helper.GetClient(), rabbitmq, func() error {
		spec.DeepCopyInto(&rabbitmq.Spec.RabbitMqSpecCore)
		if rabbitmq.Spec.Persistence.StorageClassName == nil {
			log.Info(fmt.Sprintf("Setting StorageClassName: %s", instance.Spec.StorageClass))
			rabbitmq.Spec.Persistence.StorageClassName = &instance.Spec.StorageClass
		}
		if tlsCert != "" {

			rabbitmq.Spec.TLS.SecretName = tlsCert
			rabbitmq.Spec.TLS.CaSecretName = tlsCert
			rabbitmq.Spec.TLS.DisableNonTLSListeners = true
		}
		rabbitmq.Spec.ContainerImage = *version.Status.ContainerImages.RabbitmqImage
		err := controllerutil.SetControllerReference(helper.GetBeforeObject(), rabbitmq, helper.GetScheme())
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return mqFailed, ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		log.Info(fmt.Sprintf("RabbitMQ %s - %s", rabbitmq.Name, op))
	}

	if rabbitmq.Status.ObservedGeneration == rabbitmq.Generation && rabbitmq.IsReady() {
		instance.Status.ContainerImages.RabbitmqImage = version.Status.ContainerImages.RabbitmqImage
		return mqReady, ctrl.Result{}, nil
	}

	return mqCreating, ctrl.Result{}, nil
}

func removeRabbitmqClusterControllerReference(
	ctx context.Context,
	helper *helper.Helper,
	instance *corev1beta1.OpenStackControlPlane,
	name string,
) error {
	rabbitmqCluster := &rabbitmqv2.RabbitmqCluster{}
	namespacedName := types.NamespacedName{
		Name:      name,
		Namespace: instance.Namespace,
	}
	if err := helper.GetClient().Get(ctx, namespacedName, rabbitmqCluster); err != nil {
		if k8s_errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if metav1.IsControlledBy(rabbitmqCluster, instance) {
		_, err := controllerutil.CreateOrPatch(ctx, helper.GetClient(), rabbitmqCluster, func() error {
			return controllerutil.RemoveControllerReference(helper.GetBeforeObject(), rabbitmqCluster, helper.GetScheme())
		})
		return err
	}
	return nil
}

func removeConfigMapControllerReference(
	ctx context.Context,
	helper *helper.Helper,
	instance *corev1beta1.OpenStackControlPlane,
	name string,
) error {
	configMap := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{
		Name:      fmt.Sprintf("%s-config-data", name),
		Namespace: instance.Namespace,
	}
	if err := helper.GetClient().Get(ctx, namespacedName, configMap); err != nil {
		if k8s_errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if metav1.IsControlledBy(configMap, instance) {
		_, err := controllerutil.CreateOrPatch(ctx, helper.GetClient(), configMap, func() error {
			return controllerutil.RemoveControllerReference(helper.GetBeforeObject(), configMap, helper.GetScheme())
		})
		return err
	}
	return nil
}

// RabbitmqImageMatch - return true if the rabbitmq images match on the ControlPlane and Version, or if Rabbitmq is not enabled
func RabbitmqImageMatch(ctx context.Context, controlPlane *corev1beta1.OpenStackControlPlane, version *corev1beta1.OpenStackVersion) bool {
	log := GetLogger(ctx)
	if controlPlane.Spec.Rabbitmq.Enabled {
		if !stringPointersEqual(controlPlane.Status.ContainerImages.RabbitmqImage, version.Status.ContainerImages.RabbitmqImage) {
			log.Info("RabbitMQ image mismatch", "controlPlane.Status.ContainerImages.RabbitmqImage", controlPlane.Status.ContainerImages.RabbitmqImage, "version.Status.ContainerImages.RabbitmqImage", version.Status.ContainerImages.RabbitmqImage)
			return false
		}
	}

	return true
}

// getRabbitMQVersionFromPod executes rpm command in pod to get RabbitMQ version
func getRabbitMQVersionFromPod(ctx context.Context, helper *helper.Helper, podName, namespace string) (string, error) {
	log := GetLogger(ctx)

	log.Info("Getting RabbitMQ version", "pod", podName, "namespace", namespace)

	pod := types.NamespacedName{
		Name:      podName,
		Namespace: namespace,
	}

	// Execute rpm command to get RabbitMQ version
	cmd := []string{"/bin/sh", "-c", "rpm -qa | grep rabbitmq-server | head -1"}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Get config from controller runtime
	config := ctrl.GetConfigOrDie()

	err := rsh.ExecInPod(ctx, helper.GetKClient(), config, pod, "rabbitmq", cmd,
		func(stdoutBuf *bytes.Buffer, stderrBuf *bytes.Buffer) error {
			stdout = *stdoutBuf
			stderr = *stderrBuf
			return nil
		})

	if err != nil {
		return "", fmt.Errorf("failed to execute command in pod %s/%s: %w, stderr: %s", namespace, podName, err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return "", fmt.Errorf("no RabbitMQ server package found in pod %s/%s", namespace, podName)
	}

	return output, nil
}

// extractMajorVersion extracts major version from RabbitMQ RPM string
func extractMajorVersion(rpmVersion string) (int, error) {
	// Parse "rabbitmq-server-3.12.0-1.el9" -> major version 3
	parts := strings.Split(rpmVersion, "-")
	if len(parts) < 3 {
		return 0, fmt.Errorf("invalid RPM version format: %s", rpmVersion)
	}

	versionParts := strings.Split(parts[2], ".")
	if len(versionParts) < 1 {
		return 0, fmt.Errorf("invalid version format: %s", parts[2])
	}

	major, err := strconv.Atoi(versionParts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid major version: %s", versionParts[0])
	}

	return major, nil
}

// CheckRabbitMQMajorVersionChange checks if RabbitMQ requires major version update
func CheckRabbitMQMajorVersionChange(ctx context.Context, helper *helper.Helper, instance *corev1beta1.OpenStackControlPlane, version *corev1beta1.OpenStackVersion) (bool, error) {
	log := GetLogger(ctx)

	log.Info("Checking RabbitMQ major version change", "enabled", instance.Spec.Rabbitmq.Enabled)

	if !instance.Spec.Rabbitmq.Enabled {
		log.Info("RabbitMQ not enabled, skipping major version check")
		return false, nil
	}

	// Get first RabbitMQ template name
	var mqName string
	for name := range *instance.Spec.Rabbitmq.Templates {
		mqName = name
		break
	}
	if mqName == "" {
		log.Info("No RabbitMQ templates found, skipping major version check")
		return false, fmt.Errorf("no RabbitMQ templates found")
	}

	log.Info("Checking RabbitMQ major version change", "mqName", mqName)

	// Get current RabbitMQ pod
	rabbitPod := &corev1.Pod{}
	podName := fmt.Sprintf("%s-server-0", mqName)
	err := helper.GetClient().Get(ctx, types.NamespacedName{Name: podName, Namespace: instance.Namespace}, rabbitPod)
	if k8s_errors.IsNotFound(err) {
		log.Info("RabbitMQ pod not found, assuming fresh deployment", "podName", podName)
		return false, nil
	} else if err != nil {
		log.Error(err, "Failed to get RabbitMQ pod", "podName", podName)
		return false, err
	}

	log.Info("Found existing RabbitMQ pod, proceeding with version check", "podName", podName)

	// Create verification pod from new image
	verifyPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-version-check", mqName),
			Namespace: instance.Namespace,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "rabbitmq",
					Image:   *version.Status.ContainerImages.RabbitmqImage,
					Command: []string{"sleep", "3600"},
				},
			},
		},
	}

	// Create verification pod
	err = helper.GetClient().Create(ctx, verifyPod)
	if err != nil && !k8s_errors.IsAlreadyExists(err) {
		return false, err
	}

	// Wait for pod to be ready (simplified - real implementation should wait properly)

	// Get versions from both pods
	currentVersion, err := getRabbitMQVersionFromPod(ctx, helper, podName, instance.Namespace)
	if err != nil {
		// Clean up verification pod
		helper.GetClient().Delete(ctx, verifyPod)
		return false, err
	}

	newVersion, err := getRabbitMQVersionFromPod(ctx, helper, verifyPod.Name, instance.Namespace)
	if err != nil {
		// Clean up verification pod
		helper.GetClient().Delete(ctx, verifyPod)
		return false, err
	}

	// Clean up verification pod
	helper.GetClient().Delete(ctx, verifyPod)

	// Compare major versions
	currentMajor, err := extractMajorVersion(currentVersion)
	if err != nil {
		return false, err
	}

	newMajor, err := extractMajorVersion(newVersion)
	if err != nil {
		return false, err
	}

	log.Info("RabbitMQ version comparison", "current", currentMajor, "new", newMajor)

	return currentMajor != newMajor, nil
}

// DeleteRabbitMQClusterAndPVCs deletes RabbitMQ cluster and associated PVCs
func DeleteRabbitMQClusterAndPVCs(ctx context.Context, helper *helper.Helper, instance *corev1beta1.OpenStackControlPlane) error {
	log := GetLogger(ctx)

	// Delete all RabbitMQ resources
	for name := range *instance.Spec.Rabbitmq.Templates {
		// Delete RabbitMQ CR
		rabbitmq := &rabbitmqv1.RabbitMq{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: instance.Namespace,
			},
		}

		log.Info("Deleting RabbitMQ for major update", "name", name)
		if _, err := EnsureDeleted(ctx, helper, rabbitmq); err != nil {
			return err
		}

		// Delete PVCs
		pvcList := &corev1.PersistentVolumeClaimList{}
		listOpts := []client.ListOption{
			client.InNamespace(instance.Namespace),
			client.MatchingLabels(map[string]string{"app.kubernetes.io/name": name}),
		}

		err := helper.GetClient().List(ctx, pvcList, listOpts...)
		if err != nil {
			return err
		}

		for _, pvc := range pvcList.Items {
			log.Info("Deleting RabbitMQ PVC for major update", "pvc", pvc.Name)
			if err := helper.GetClient().Delete(ctx, &pvc); err != nil && !k8s_errors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

// CreateDataplaneDeploymentForNova creates a dataplane deployment with only nova service
func CreateDataplaneDeploymentForNova(ctx context.Context, helper *helper.Helper, instance *corev1beta1.OpenStackControlPlane, version *corev1beta1.OpenStackVersion) error {
	log := GetLogger(ctx)

	// Get all dataplane nodesets to include
	dataplaneNodesets, err := GetDataplaneNodesets(ctx, instance, helper)
	if err != nil {
		return err
	}

	if len(dataplaneNodesets.Items) == 0 {
		log.Info("No dataplane nodesets found, skipping dataplane deployment creation")
		return nil
	}

	nodeSetNames := make([]string, 0, len(dataplaneNodesets.Items))
	for _, nodeset := range dataplaneNodesets.Items {
		nodeSetNames = append(nodeSetNames, nodeset.Name)
	}

	deployment := &dataplanev1.OpenStackDataPlaneDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("rabbitmq-major-update-%s", version.Spec.TargetVersion),
			Namespace: instance.Namespace,
		},
		Spec: dataplanev1.OpenStackDataPlaneDeploymentSpec{
			NodeSets:         nodeSetNames,
			ServicesOverride: []string{"nova"},
			BackoffLimit:     ptr.To(int32(6)),
			PreserveJobs:     true,
		},
	}

	err = controllerutil.SetControllerReference(instance, deployment, helper.GetScheme())
	if err != nil {
		return err
	}

	log.Info("Creating dataplane deployment for RabbitMQ major update", "deployment", deployment.Name)
	err = helper.GetClient().Create(ctx, deployment)
	if err != nil && !k8s_errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

// CheckDataplaneDeploymentComplete checks if the nova dataplane deployment is complete
func CheckDataplaneDeploymentComplete(ctx context.Context, helper *helper.Helper, instance *corev1beta1.OpenStackControlPlane, version *corev1beta1.OpenStackVersion) (bool, error) {
	deploymentName := fmt.Sprintf("rabbitmq-major-update-%s", version.Spec.TargetVersion)

	deployment := &dataplanev1.OpenStackDataPlaneDeployment{}
	err := helper.GetClient().Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: instance.Namespace}, deployment)
	if k8s_errors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return deployment.IsReady(), nil
}
