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

package dataplane

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	infranetworkv1 "github.com/openstack-k8s-operators/infra-operator/apis/network/v1beta1"
	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/rolebinding"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/serviceaccount"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	baremetalv1 "github.com/openstack-k8s-operators/openstack-baremetal-operator/api/v1beta1"
	openstackv1 "github.com/openstack-k8s-operators/openstack-operator/api/core/v1beta1"
	dataplanev1 "github.com/openstack-k8s-operators/openstack-operator/api/dataplane/v1beta1"
	deployment "github.com/openstack-k8s-operators/openstack-operator/internal/dataplane"
	dataplaneutil "github.com/openstack-k8s-operators/openstack-operator/internal/dataplane/util"

	machineconfig "github.com/openshift/api/machineconfiguration/v1"
)

const (
	// AnsibleSSHPrivateKey ssh private key
	AnsibleSSHPrivateKey = "ssh-privatekey"
	// AnsibleSSHAuthorizedKeys authorized keys
	AnsibleSSHAuthorizedKeys = "authorized_keys"

	// RabbitMQ user secret prefix
	rabbitmqUserSecretPrefix = "rabbitmq-user-"
)

var (
	// Regex to extract username from RabbitMQ transport_url
	// Format: rabbit://USERNAME:PASSWORD@HOST:PORT/?params
	transportURLUsernameRegex = regexp.MustCompile(`rabbit://([^:]+):`)
)

// OpenStackDataPlaneNodeSetReconciler reconciles a OpenStackDataPlaneNodeSet object
type OpenStackDataPlaneNodeSetReconciler struct {
	client.Client
	Kclient    kubernetes.Interface
	Scheme     *runtime.Scheme
	Controller controller.Controller
	Cache      cache.Cache
	Watching   map[string]bool
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *OpenStackDataPlaneNodeSetReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("OpenStackDataPlaneNodeSet")
}

// +kubebuilder:rbac:groups=dataplane.openstack.org,resources=openstackdataplanenodesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dataplane.openstack.org,resources=openstackdataplanenodesets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dataplane.openstack.org,resources=openstackdataplanenodesets/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=dataplane.openstack.org,resources=openstackdataplaneservices,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=dataplane.openstack.org,resources=openstackdataplaneservices/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=baremetal.openstack.org,resources=openstackbaremetalsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=baremetal.openstack.org,resources=openstackbaremetalsets/status,verbs=get
// +kubebuilder:rbac:groups=baremetal.openstack.org,resources=openstackbaremetalsets/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=network.openstack.org,resources=ipsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=network.openstack.org,resources=ipsets/status,verbs=get
// +kubebuilder:rbac:groups=network.openstack.org,resources=ipsets/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=network.openstack.org,resources=netconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=network.openstack.org,resources=dnsmasqs,verbs=get;list;watch
// +kubebuilder:rbac:groups=network.openstack.org,resources=dnsmasqs/status,verbs=get
// +kubebuilder:rbac:groups=network.openstack.org,resources=dnsdata,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=network.openstack.org,resources=dnsdata/status,verbs=get
// +kubebuilder:rbac:groups=network.openstack.org,resources=dnsdata/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core.openstack.org,resources=openstackversions,verbs=get;list;watch

// RBAC for the ServiceAccount for the internal image registry
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get
// +kubebuilder:rbac:groups="",resources=projects,verbs=get
// +kubebuilder:rbac:groups="project.openshift.io",resources=projects,verbs=get
// +kubebuilder:rbac:groups="",resources=imagestreamimages,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=imagestreammappings,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=imagestreams,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=imagestreams/layers,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=imagestreamtags,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=imagetags,verbs=get;list;watch
// +kubebuilder:rbac:groups="image.openshift.io",resources=imagestreamimages,verbs=get;list;watch
// +kubebuilder:rbac:groups="image.openshift.io",resources=imagestreammappings,verbs=get;list;watch
// +kubebuilder:rbac:groups="image.openshift.io",resources=imagestreams,verbs=get;list;watch
// +kubebuilder:rbac:groups="image.openshift.io",resources=imagestreams/layers,verbs=get
// +kubebuilder:rbac:groups="image.openshift.io",resources=imagetags,verbs=get;list;watch
// +kubebuilder:rbac:groups="image.openshift.io",resources=imagestreamtags,verbs=get;list;watch

// RBAC for ImageContentSourcePolicy and MachineConfig
// +kubebuilder:rbac:groups="operator.openshift.io",resources=imagecontentsourcepolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups="config.openshift.io",resources=imagedigestmirrorsets,verbs=get;list;watch
// +kubebuilder:rbac:groups="config.openshift.io",resources=images,verbs=get;list;watch
// +kubebuilder:rbac:groups="machineconfiguration.openshift.io",resources=machineconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=rabbitmqusers,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the OpenStackDataPlaneNodeSet object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *OpenStackDataPlaneNodeSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling NodeSet")

	// Try to set up MachineConfig watch if not already done
	// This is done conditionally because MachineConfig CRD may not exist on all clusters
	r.ensureMachineConfigWatch(ctx)

	validate := validator.New()

	// Fetch the OpenStackDataPlaneNodeSet instance
	instance := &dataplanev1.OpenStackDataPlaneNodeSet{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	helper, _ := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		Log,
	)

	// initialize status if Conditions is nil, but do not reset if it already
	// exists
	isNewInstance := instance.Status.Conditions == nil
	if isNewInstance {
		instance.Status.Conditions = condition.Conditions{}
	}

	// Save a copy of the conditions so that we can restore the LastTransitionTime
	// when a condition's state doesn't change.
	savedConditions := instance.Status.Conditions.DeepCopy()

	// Reset all conditions to Unknown as the state is not yet known for
	// this reconcile loop.
	instance.InitConditions()
	// Set ObservedGeneration since we've reset conditions
	instance.Status.ObservedGeneration = instance.Generation

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() { // update the Ready condition based on the sub conditions
		// Don't update the status, if reconciler Panics
		if r := recover(); r != nil {
			Log.Info(fmt.Sprintf("panic during reconcile %v\n", r))
			panic(r)
		}
		if instance.Status.Conditions.AllSubConditionIsTrue() {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, dataplanev1.NodeSetReadyMessage)
		} else if instance.Status.Conditions.IsUnknown(condition.ReadyCondition) {
			// Recalculate ReadyCondition based on the state of the rest of the conditions
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		condition.RestoreLastTransitionTimes(
			&instance.Status.Conditions, savedConditions)

		err := helper.PatchInstance(ctx, instance)
		if err != nil {
			Log.Error(err, "Error updating instance status conditions")
			_err = err
			return
		}
	}()

	if instance.Status.ConfigMapHashes == nil {
		instance.Status.ConfigMapHashes = make(map[string]string)
	}
	if instance.Status.SecretHashes == nil {
		instance.Status.SecretHashes = make(map[string]string)
	}
	if instance.Status.ContainerImages == nil {
		instance.Status.ContainerImages = make(map[string]string)
	}

	instance.Status.Conditions.MarkFalse(dataplanev1.SetupReadyCondition, condition.RequestedReason, condition.SeverityInfo, condition.ReadyInitMessage)

	// Detect config changes and set Status ConfigHash
	configHash, err := r.GetSpecConfigHash(instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	if configHash != instance.Status.DeployedConfigHash {
		instance.Status.ConfigHash = configHash
	}

	// Ensure Services
	err = deployment.EnsureServices(ctx, helper, instance, validate)
	if err != nil {
		instance.Status.Conditions.MarkFalse(
			dataplanev1.SetupReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			dataplanev1.DataPlaneNodeSetErrorMessage,
			err.Error())
		return ctrl.Result{}, err
	}

	// Ensure IPSets Required for Nodes
	allIPSets, netServiceNetMap, isReady, err := deployment.EnsureIPSets(ctx, helper, instance)
	if err != nil || !isReady {
		return ctrl.Result{}, err
	}

	// Ensure DNSData Required for Nodes
	dnsDetails, err := deployment.EnsureDNSData(
		ctx, helper,
		instance, allIPSets)
	if err != nil || !dnsDetails.IsReady {
		return ctrl.Result{}, err
	}
	instance.Status.DNSClusterAddresses = dnsDetails.ClusterAddresses
	instance.Status.CtlplaneSearchDomain = dnsDetails.CtlplaneSearchDomain
	instance.Status.AllHostnames = dnsDetails.Hostnames
	instance.Status.AllIPs = dnsDetails.AllIPs

	ansibleSSHPrivateKeySecret := instance.Spec.NodeTemplate.AnsibleSSHPrivateKeySecret

	secretKeys := []string{}
	secretKeys = append(secretKeys, AnsibleSSHPrivateKey)
	if !instance.Spec.PreProvisioned {
		secretKeys = append(secretKeys, AnsibleSSHAuthorizedKeys)
	}
	_, result, err = secret.VerifySecret(
		ctx,
		types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      ansibleSSHPrivateKeySecret,
		},
		secretKeys,
		helper.GetClient(),
		time.Second*5,
	)
	if err != nil {
		instance.Status.Conditions.MarkFalse(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			"%s", err.Error())
		return result, err
	} else if (result != ctrl.Result{}) {
		// Since the the private key secret should have been manually created by the user when provided in the spec,
		// we treat this as a warning because it means that reconciliation will not be able to continue.
		instance.Status.Conditions.MarkFalse(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			dataplanev1.InputReadyWaitingMessage,
			"secret/"+ansibleSSHPrivateKeySecret)
		return result, nil
	}

	// all our input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	// Reconcile ServiceAccount
	nodeSetServiceAccount := serviceaccount.NewServiceAccount(
		&corev1.ServiceAccount{
			ObjectMeta: v1.ObjectMeta{
				Namespace: instance.Namespace,
				Name:      instance.Name,
			},
		},
		time.Duration(10),
	)
	saResult, err := nodeSetServiceAccount.CreateOrPatch(ctx, helper)
	if err != nil {
		instance.Status.Conditions.MarkFalse(
			condition.ServiceAccountReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceAccountReadyErrorMessage,
			err.Error())
		return saResult, err
	} else if (saResult != ctrl.Result{}) {
		instance.Status.Conditions.MarkFalse(
			condition.ServiceAccountReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.ServiceAccountCreatingMessage)
		return saResult, nil
	}

	regViewerRoleBinding := rolebinding.NewRoleBinding(
		&rbacv1.RoleBinding{
			ObjectMeta: v1.ObjectMeta{
				Namespace: instance.Namespace,
				Name:      instance.Name,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      instance.Name,
					Namespace: instance.Namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     "registry-viewer",
			},
		},
		time.Duration(10),
	)
	rbResult, err := regViewerRoleBinding.CreateOrPatch(ctx, helper)
	if err != nil {
		instance.Status.Conditions.MarkFalse(
			condition.ServiceAccountReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceAccountReadyErrorMessage,
			err.Error())
		return rbResult, err
	} else if (rbResult != ctrl.Result{}) {
		instance.Status.Conditions.MarkFalse(
			condition.ServiceAccountReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.ServiceAccountCreatingMessage)
		return rbResult, nil
	}

	instance.Status.Conditions.MarkTrue(
		condition.ServiceAccountReadyCondition,
		condition.ServiceAccountReadyMessage)

	version, err := dataplaneutil.GetVersion(ctx, helper, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	containerImages := dataplaneutil.GetContainerImages(version)
	var provResult deployment.ProvisionResult
	// Reconcile BaremetalSet if required
	if !instance.Spec.PreProvisioned {
		// Reset the NodeSetBareMetalProvisionReadyCondition to unknown
		instance.Status.Conditions.MarkUnknown(dataplanev1.NodeSetBareMetalProvisionReadyCondition,
			condition.InitReason, condition.InitReason)

		provResult, err = deployment.DeployBaremetalSet(ctx, helper, instance,
			allIPSets, dnsDetails.ServerAddresses, containerImages)
		if err != nil || !provResult.IsProvisioned {
			return ctrl.Result{}, err
		}
		instance.Status.BmhRefHash = provResult.BmhRefHash
	}

	isDeploymentReady, isDeploymentRunning, isDeploymentFailed, failedDeployment, err := checkDeployment(
		ctx, helper, instance, r)
	if !isDeploymentFailed && err != nil {
		instance.Status.Conditions.MarkFalse(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			condition.DeploymentReadyErrorMessage,
			err.Error())
		Log.Error(err, "Unable to get deployed OpenStackDataPlaneDeployments.")
		return ctrl.Result{}, err
	}

	if !isDeploymentRunning {
		// Generate NodeSet Inventory
		_, errInventory := deployment.GenerateNodeSetInventory(ctx, helper, instance,
			allIPSets, dnsDetails.ServerAddresses, containerImages, netServiceNetMap)
		if errInventory != nil {
			errorMsg := fmt.Sprintf("Unable to generate inventory for %s", instance.Name)
			util.LogErrorForObject(helper, errInventory, errorMsg, instance)
			instance.Status.Conditions.MarkFalse(
				dataplanev1.SetupReadyCondition,
				condition.ErrorReason,
				condition.SeverityError,
				dataplanev1.DataPlaneNodeSetErrorMessage,
				errorMsg)
			return ctrl.Result{}, errInventory
		}
	}
	// all setup tasks complete, mark SetupReadyCondition True
	instance.Status.Conditions.MarkTrue(dataplanev1.SetupReadyCondition, condition.ReadyMessage)

	// Set DeploymentReadyCondition to False if it was unknown.
	// Handles the case where the NodeSet is created, but not yet deployed.
	if instance.Status.Conditions.IsUnknown(condition.DeploymentReadyCondition) {
		Log.Info("Set NodeSet DeploymentReadyCondition false")
		instance.Status.Conditions.MarkFalse(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			dataplanev1.NodeSetDeploymentReadyWaitingMessage)
	}

	if isDeploymentReady {
		Log.Info("Set NodeSet DeploymentReadyCondition true")
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition,
			condition.DeploymentReadyMessage)
		instance.Status.DeployedBmhHash = instance.Status.BmhRefHash
	} else if isDeploymentRunning {
		Log.Info("Deployment still running...", "instance", instance)
		Log.Info("Set NodeSet DeploymentReadyCondition false")
		instance.Status.Conditions.MarkFalse(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage)
	} else if isDeploymentFailed {
		podsInterface := r.Kclient.CoreV1().Pods(instance.Namespace)
		podsList, _err := podsInterface.List(ctx, v1.ListOptions{
			LabelSelector: fmt.Sprintf("openstackdataplanedeployment=%s", failedDeployment),
			FieldSelector: "status.phase=Failed",
		})

		if _err != nil {
			Log.Error(err, "unable to retrieve list of pods for dataplane diagnostic")
		} else {
			for _, pod := range podsList.Items {
				Log.Info(fmt.Sprintf("openstackansibleee job %s failed due to %s with message: %s", pod.Name, pod.Status.Reason, pod.Status.Message))
			}
		}
		Log.Info("Set NodeSet DeploymentReadyCondition false")
		deployErrorMsg := ""
		if err != nil {
			deployErrorMsg = err.Error()
		}
		instance.Status.Conditions.MarkFalse(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			"%s", deployErrorMsg)
	}

	return ctrl.Result{}, err
}

func checkDeployment(ctx context.Context, helper *helper.Helper,
	instance *dataplanev1.OpenStackDataPlaneNodeSet,
	r *OpenStackDataPlaneNodeSetReconciler) (
	isNodeSetDeploymentReady bool, isNodeSetDeploymentRunning bool,
	isNodeSetDeploymentFailed bool, failedDeploymentName string, err error) {

	// Get all completed deployments
	deployments := &dataplanev1.OpenStackDataPlaneDeploymentList{}
	opts := []client.ListOption{
		client.InNamespace(instance.Namespace),
	}
	err = helper.GetClient().List(ctx, deployments, opts...)
	if err != nil {
		helper.GetLogger().Error(err, "Unable to retrieve OpenStackDataPlaneDeployment CRs %v")
		return isNodeSetDeploymentReady, isNodeSetDeploymentRunning, isNodeSetDeploymentFailed, failedDeploymentName, err
	}

	// Collect deployments that target this nodeset (excluding deleted ones)
	var relevantDeployments []*dataplanev1.OpenStackDataPlaneDeployment
	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		if !deployment.DeletionTimestamp.IsZero() {
			continue
		}
		if slices.Contains(deployment.Spec.NodeSets, instance.Name) {
			relevantDeployments = append(relevantDeployments, deployment)
		}
	}

	// Sort relevant deployments from oldest to newest, then take the last one
	var latestRelevantDeployment *dataplanev1.OpenStackDataPlaneDeployment
	if len(relevantDeployments) > 0 {
		slices.SortFunc(relevantDeployments, func(a, b *dataplanev1.OpenStackDataPlaneDeployment) int {
			aReady := a.Status.Conditions.Get(condition.DeploymentReadyCondition)
			bReady := b.Status.Conditions.Get(condition.DeploymentReadyCondition)
			if aReady != nil && bReady != nil {
				if aReady.LastTransitionTime.Before(&bReady.LastTransitionTime) {
					return -1
				}
			}
			return 1
		})
		latestRelevantDeployment = relevantDeployments[len(relevantDeployments)-1]
	}

	for _, deployment := range relevantDeployments {
		// Always add to DeploymentStatuses (for visibility)
		deploymentConditions := deployment.Status.NodeSetConditions[instance.Name]
		if instance.Status.DeploymentStatuses == nil {
			instance.Status.DeploymentStatuses = make(map[string]condition.Conditions)
		}
		instance.Status.DeploymentStatuses[deployment.Name] = deploymentConditions

		// Apply filtering for overall nodeset deployment state logic
		isLatestDeployment := latestRelevantDeployment != nil && deployment.Name == latestRelevantDeployment.Name
		deploymentCondition := deploymentConditions.Get(dataplanev1.NodeSetDeploymentReadyCondition)

		// Skip failed/error deployments that aren't the latest
		// All running and completed deployments are processed
		isCurrentDeploymentFailed := condition.IsError(deploymentCondition)
		if isCurrentDeploymentFailed && !isLatestDeployment {
			continue
		}

		isCurrentDeploymentRunning := deploymentConditions.IsFalse(dataplanev1.NodeSetDeploymentReadyCondition) && !isCurrentDeploymentFailed
		isCurrentDeploymentReady := deploymentConditions.IsTrue(dataplanev1.NodeSetDeploymentReadyCondition)

		// Reset the vars for every deployment that affects overall state
		isNodeSetDeploymentReady = false
		isNodeSetDeploymentRunning = false
		isNodeSetDeploymentFailed = false

		if isCurrentDeploymentFailed {
			err = fmt.Errorf("%s", deploymentCondition.Message)
			failedDeploymentName = deployment.Name
			isNodeSetDeploymentFailed = true
			break
		}
		if isCurrentDeploymentRunning {
			isNodeSetDeploymentRunning = true
		}

		if isCurrentDeploymentReady {
			// If the nodeset configHash does not match with what's in the deployment or
			// deployedBmhHash is different from current bmhRefHash.
			if (deployment.Status.NodeSetHashes[instance.Name] != instance.Status.ConfigHash) ||
				(!instance.Spec.PreProvisioned &&
					deployment.Status.BmhRefHashes[instance.Name] != instance.Status.BmhRefHash) {
				continue
			}

			hasAnsibleVarsFromChanged, err := checkAnsibleVarsFromChanged(ctx, helper, instance, deployment.Status.ConfigMapHashes, deployment.Status.SecretHashes)

			if err != nil {
				return isNodeSetDeploymentReady, isNodeSetDeploymentRunning, isNodeSetDeploymentFailed, failedDeploymentName, err
			}

			if hasAnsibleVarsFromChanged {
				continue
			}

			isNodeSetDeploymentReady = true

			// Track if this deployment is actually changing the nodeset config
			// IMPORTANT: Check BEFORE copying hashes, since multiple deployments process in the same reconcile
			newDeployedConfigHash, hasNodeSetHash := deployment.Status.NodeSetHashes[instance.Name]
			if !hasNodeSetHash {
				// Deployment doesn't have a hash for this nodeset, skip credential tracking
				helper.GetLogger().Info("Deployment missing NodeSetHash, skipping credential tracking",
					"deployment", deployment.Name,
					"nodeset", instance.Name)
				newDeployedConfigHash = ""
			}
			configHashChanged := instance.Status.DeployedConfigHash != newDeployedConfigHash

			// Check if any secrets changed (for credential rotation detection)
			// This must be done BEFORE copying the hashes below
			secretsChanged := false
			for k, newHash := range deployment.Status.SecretHashes {
				if oldHash, exists := instance.Status.SecretHashes[k]; !exists || oldHash != newHash {
					secretsChanged = true
					break
				}
			}

			// Update service credential tracking status BEFORE copying hashes
			// Track credentials when:
			// 1. Config or secrets changed (normal case)
			// 2. Status is empty (bootstrapping case - populate from existing ready deployments)
			shouldTrackCredentials := configHashChanged || secretsChanged
			if !shouldTrackCredentials && instance.Status.ServiceCredentialStatus == nil {
				// Bootstrap: populate status from existing ready deployment
				shouldTrackCredentials = true
				helper.GetLogger().Info("Bootstrapping credential tracking from existing deployment",
					"deployment", deployment.Name)
			}

			if shouldTrackCredentials {
				if err := r.updateServiceCredentialStatus(ctx, instance, deployment); err != nil {
					// This is a critical error - if we can't track credentials, RabbitMQ users
					// could be deleted prematurely. Return error to retry.
					helper.GetLogger().Error(err, "Failed to update service credential status")
					return false, false, false, "", err
				}
			}

			// Now copy the hashes to nodeset status
			for k, v := range deployment.Status.ConfigMapHashes {
				instance.Status.ConfigMapHashes[k] = v
			}
			for k, v := range deployment.Status.SecretHashes {
				instance.Status.SecretHashes[k] = v
			}
			for k, v := range deployment.Status.ContainerImages {
				instance.Status.ContainerImages[k] = v
			}
			instance.Status.DeployedConfigHash = newDeployedConfigHash

			// Get list of services by name, either from ServicesOverride or
			// the NodeSet.
			var services []string
			if len(deployment.Spec.ServicesOverride) != 0 {
				services = deployment.Spec.ServicesOverride
			} else {
				services = instance.Spec.Services
			}

			// For each service, check if EDPMServiceType is "update" or "update-services", and
			// if so, copy Deployment.Status.DeployedVersion to
			// NodeSet.Status.DeployedVersion
			for _, serviceName := range services {
				service := &dataplanev1.OpenStackDataPlaneService{}
				name := types.NamespacedName{
					Namespace: instance.Namespace,
					Name:      serviceName,
				}
				err := helper.GetClient().Get(ctx, name, service)
				if err != nil {
					helper.GetLogger().Error(err, "Unable to retrieve OpenStackDataPlaneService %v")
					return isNodeSetDeploymentReady, isNodeSetDeploymentRunning, isNodeSetDeploymentFailed, failedDeploymentName, err
				}

				if service.Spec.EDPMServiceType != "update" && service.Spec.EDPMServiceType != "update-services" {
					continue
				}

				// An "update" or "update-services" service Deployment has been completed, so
				// set the NodeSet's DeployedVersion to the Deployment's
				// DeployedVersion.
				instance.Status.DeployedVersion = deployment.Status.DeployedVersion
			}
		}
	}

	return isNodeSetDeploymentReady, isNodeSetDeploymentRunning, isNodeSetDeploymentFailed, failedDeploymentName, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *OpenStackDataPlaneNodeSetReconciler) SetupWithManager(
	ctx context.Context, mgr ctrl.Manager,
) error {
	// index for ConfigMaps listed on ansibleVarsFrom
	if err := mgr.GetFieldIndexer().IndexField(ctx,
		&dataplanev1.OpenStackDataPlaneNodeSet{}, "spec.ansibleVarsFrom.ansible.configMaps",
		func(rawObj client.Object) []string {
			nodeSet := rawObj.(*dataplanev1.OpenStackDataPlaneNodeSet)
			configMaps := make([]string, 0)

			appendConfigMaps := func(varsFrom []dataplanev1.DataSource) {
				for _, ref := range varsFrom {
					if ref.ConfigMapRef != nil {
						configMaps = append(configMaps, ref.ConfigMapRef.Name)
					}
				}
			}

			appendConfigMaps(nodeSet.Spec.NodeTemplate.Ansible.AnsibleVarsFrom)
			for _, node := range nodeSet.Spec.Nodes {
				appendConfigMaps(node.Ansible.AnsibleVarsFrom)
			}
			return configMaps
		}); err != nil {
		return err
	}

	// index for Secrets listed on ansibleVarsFrom
	if err := mgr.GetFieldIndexer().IndexField(ctx,
		&dataplanev1.OpenStackDataPlaneNodeSet{}, "spec.ansibleVarsFrom.ansible.secrets",
		func(rawObj client.Object) []string {
			nodeSet := rawObj.(*dataplanev1.OpenStackDataPlaneNodeSet)
			secrets := make([]string, 0, len(nodeSet.Spec.Nodes)+1)
			if nodeSet.Spec.NodeTemplate.AnsibleSSHPrivateKeySecret != "" {
				secrets = append(secrets, nodeSet.Spec.NodeTemplate.AnsibleSSHPrivateKeySecret)
			}

			appendSecrets := func(varsFrom []dataplanev1.DataSource) {
				for _, ref := range varsFrom {
					if ref.SecretRef != nil {
						secrets = append(secrets, ref.SecretRef.Name)
					}
				}
			}

			appendSecrets(nodeSet.Spec.NodeTemplate.Ansible.AnsibleVarsFrom)
			for _, node := range nodeSet.Spec.Nodes {
				appendSecrets(node.Ansible.AnsibleVarsFrom)
			}
			return secrets
		}); err != nil {
		return err
	}
	// Initialize the Watching map for conditional CRD watches
	r.Watching = make(map[string]bool)
	r.Cache = mgr.GetCache()

	// Build the controller without MachineConfig watch (added conditionally later)
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&dataplanev1.OpenStackDataPlaneNodeSet{},
			builder.WithPredicates(predicate.Or(
				predicate.GenerationChangedPredicate{},
				predicate.AnnotationChangedPredicate{},
				predicate.LabelChangedPredicate{}))).
		Owns(&batchv1.Job{}).
		Owns(&baremetalv1.OpenStackBaremetalSet{}).
		Owns(&infranetworkv1.IPSet{}).
		Owns(&infranetworkv1.DNSData{}).
		Owns(&corev1.Secret{}).
		Watches(&infranetworkv1.DNSMasq{},
			handler.EnqueueRequestsFromMapFunc(r.genericWatcherFn)).
		Watches(&dataplanev1.OpenStackDataPlaneDeployment{},
			handler.EnqueueRequestsFromMapFunc(r.deploymentWatcherFn)).
		Watches(&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.secretWatcherFn),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Watches(&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.secretWatcherFn),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Watches(&openstackv1.OpenStackVersion{},
			handler.EnqueueRequestsFromMapFunc(r.genericWatcherFn)).
		// NOTE: MachineConfig watch is added conditionally during reconciliation
		// to avoid failures when the MachineConfig CRD doesn't exist
		Build(r)

	if err != nil {
		return err
	}
	r.Controller = c
	return nil
}

// machineConfigWatcherFn - watches for changes to the registries MachineConfig resource and queues
// a reconcile of each NodeSet if the MachineConfig is changed.
func (r *OpenStackDataPlaneNodeSetReconciler) machineConfigWatcherFn(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	Log := r.GetLogger(ctx)
	nodeSets := &dataplanev1.OpenStackDataPlaneNodeSetList{}
	kind := strings.ToLower(obj.GetObjectKind().GroupVersionKind().Kind)
	const registryMachineConfigName string = "99-master-generated-registries"

	if obj.GetName() != registryMachineConfigName {
		return nil
	}

	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
	}
	if err := r.List(ctx, nodeSets, listOpts...); err != nil {
		Log.Error(err, "Unable to retrieve OpenStackDataPlaneNodeSetList")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(nodeSets.Items))
	for _, nodeSet := range nodeSets.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      nodeSet.Name,
			},
		})
		Log.Info(fmt.Sprintf("reconcile loop for openstackdataplanenodeset %s triggered by %s %s",
			nodeSet.Name, kind, obj.GetName()))
	}
	return requests
}

// machineConfigWatcherFnTyped - typed version of machineConfigWatcherFn for use with source.Kind
func (r *OpenStackDataPlaneNodeSetReconciler) machineConfigWatcherFnTyped(
	ctx context.Context, obj *machineconfig.MachineConfig,
) []reconcile.Request {
	return r.machineConfigWatcherFn(ctx, obj)
}

const machineConfigCRDName = "machineconfigs.machineconfiguration.openshift.io"

// ensureMachineConfigWatch attempts to set up a watch for MachineConfig resources.
// This is done conditionally because the MachineConfig CRD may not exist on all clusters
// (e.g., non-OpenShift Kubernetes clusters or clusters without the Machine Config Operator).
// Returns true if the CRD is available (watch was set up or already exists), false otherwise.
func (r *OpenStackDataPlaneNodeSetReconciler) ensureMachineConfigWatch(ctx context.Context) bool {
	Log := r.GetLogger(ctx)

	// Check if we're already watching
	if r.Watching[machineConfigCRDName] {
		return true
	}

	// Check if the MachineConfig CRD exists
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Kind:    "CustomResourceDefinition",
		Version: "v1",
	})

	err := r.Get(ctx, client.ObjectKey{Name: machineConfigCRDName}, crd)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			Log.Info("MachineConfig CRD not found, disconnected environment features disabled")
		} else {
			Log.Error(err, "Error checking for MachineConfig CRD")
		}
		return false
	}

	// CRD exists, set up the watch
	Log.Info("MachineConfig CRD found, enabling watch for disconnected environment support")
	err = r.Controller.Watch(
		source.Kind(
			r.Cache,
			&machineconfig.MachineConfig{},
			handler.TypedEnqueueRequestsFromMapFunc(r.machineConfigWatcherFnTyped),
			predicate.TypedResourceVersionChangedPredicate[*machineconfig.MachineConfig]{},
		),
	)
	if err != nil {
		Log.Error(err, "Failed to set up MachineConfig watch")
		return false
	}

	r.Watching[machineConfigCRDName] = true
	Log.Info("Successfully set up MachineConfig watch")
	return true
}

// IsMachineConfigAvailable returns true if the MachineConfig CRD is available and being watched
func (r *OpenStackDataPlaneNodeSetReconciler) IsMachineConfigAvailable() bool {
	return r.Watching[machineConfigCRDName]
}

func (r *OpenStackDataPlaneNodeSetReconciler) secretWatcherFn(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	Log := r.GetLogger(ctx)
	nodeSets := &dataplanev1.OpenStackDataPlaneNodeSetList{}
	kind := strings.ToLower(obj.GetObjectKind().GroupVersionKind().Kind)
	selector := "spec.ansibleVarsFrom.ansible.configMaps"
	if kind == "secret" {
		selector = "spec.ansibleVarsFrom.ansible.secrets"
	}

	listOpts := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(selector, obj.GetName()),
		Namespace:     obj.GetNamespace(),
	}

	if err := r.List(ctx, nodeSets, listOpts); err != nil {
		Log.Error(err, "Unable to retrieve OpenStackDataPlaneNodeSetList")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(nodeSets.Items))
	for _, nodeSet := range nodeSets.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      nodeSet.Name,
			},
		})
		Log.Info(fmt.Sprintf("reconcile loop for openstackdataplanenodeset %s triggered by %s %s",
			nodeSet.Name, kind, obj.GetName()))
	}
	return requests
}

func (r *OpenStackDataPlaneNodeSetReconciler) genericWatcherFn(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	Log := r.GetLogger(ctx)
	nodeSets := &dataplanev1.OpenStackDataPlaneNodeSetList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
	}
	if err := r.List(ctx, nodeSets, listOpts...); err != nil {
		Log.Error(err, "Unable to retrieve OpenStackDataPlaneNodeSetList")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(nodeSets.Items))
	for _, nodeSet := range nodeSets.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      nodeSet.Name,
			},
		})
		Log.Info(fmt.Sprintf("Reconciling NodeSet %s due to watcher on %s/%s", nodeSet.Name, obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName()))
	}
	return requests
}

func (r *OpenStackDataPlaneNodeSetReconciler) deploymentWatcherFn(
	ctx context.Context, //revive:disable-line
	obj client.Object,
) []reconcile.Request {
	Log := r.GetLogger(ctx)
	namespace := obj.GetNamespace()
	deployment := obj.(*dataplanev1.OpenStackDataPlaneDeployment)

	requests := make([]reconcile.Request, 0, len(deployment.Spec.NodeSets))
	for _, nodeSet := range deployment.Spec.NodeSets {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: namespace,
				Name:      nodeSet,
			},
		})
		Log.Info(fmt.Sprintf("Reconciling NodeSet %s due to watcher on %s/%s", nodeSet, obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName()))
	}
	return requests
}

// GetSpecConfigHash initialises a new struct with only the field we want to check for variances in.
// We then hash the contents of the new struct using md5 and return the hashed string.
func (r *OpenStackDataPlaneNodeSetReconciler) GetSpecConfigHash(instance *dataplanev1.OpenStackDataPlaneNodeSet) (string, error) {
	configHash, err := util.ObjectHash(&instance.Spec)
	if err != nil {
		return "", err
	}
	return configHash, nil
}

// checkAnsibleVarsFromChanged computes current hashes for ConfigMaps/Secrets
// referenced in AnsibleVarsFrom and compares them with deployed hashes.
// Returns true if any content has changed, false otherwise.
func checkAnsibleVarsFromChanged(
	ctx context.Context,
	helper *helper.Helper,
	instance *dataplanev1.OpenStackDataPlaneNodeSet,
	deployedConfigMapHashes map[string]string,
	deployedSecretHashes map[string]string,
) (bool, error) {
	currentConfigMapHashes := make(map[string]string)
	currentSecretHashes := make(map[string]string)

	namespace := instance.Namespace

	// Process NodeTemplate level AnsibleVarsFrom
	if err := deployment.ProcessAnsibleVarsFrom(ctx, helper, namespace, currentConfigMapHashes, currentSecretHashes, instance.Spec.NodeTemplate.Ansible.AnsibleVarsFrom); err != nil {
		return false, err
	}

	// Process individual Node level AnsibleVarsFrom
	for _, node := range instance.Spec.Nodes {
		if err := deployment.ProcessAnsibleVarsFrom(ctx, helper, namespace, currentConfigMapHashes, currentSecretHashes, node.Ansible.AnsibleVarsFrom); err != nil {
			return false, err
		}
	}

	// Compare current ConfigMap hashes with deployed hashes
	for name, currentHash := range currentConfigMapHashes {
		if deployedHash, exists := deployedConfigMapHashes[name]; !exists || deployedHash != currentHash {
			helper.GetLogger().Info("ConfigMap content changed", "configMap", name)
			return true, nil
		}
	}

	// Compare current Secret hashes with deployed hashes
	for name, currentHash := range currentSecretHashes {
		if deployedHash, exists := deployedSecretHashes[name]; !exists || deployedHash != currentHash {
			helper.GetLogger().Info("Secret content changed", "secret", name)
			return true, nil
		}
	}

	return false, nil
}

// updateServiceCredentialStatus updates the NodeSet status with information about which
// nodes have been updated with RabbitMQ credentials from a deployment.
//
// This status is read by infra-operator's RabbitMQUser controller to determine when
// old credentials can be safely deleted during rotation.
//
// BUG FIX: Tracks both current AND previous credential versions during rotation to prevent
// premature deletion. The previous version is preserved until all nodes have the new version.
func (r *OpenStackDataPlaneNodeSetReconciler) updateServiceCredentialStatus(
	ctx context.Context,
	instance *dataplanev1.OpenStackDataPlaneNodeSet,
	deployment *dataplanev1.OpenStackDataPlaneDeployment,
) error {
	Log := r.GetLogger(ctx)

	// Validate inputs
	if instance == nil || deployment == nil {
		return fmt.Errorf("instance and deployment must not be nil")
	}

	// Check if deployment is ready
	isDeploymentReady := deployment.Status.Conditions.IsTrue(condition.DeploymentReadyCondition)

	// Check if we need to bootstrap (status is nil)
	needsBootstrap := instance.Status.ServiceCredentialStatus == nil

	// Initialize map if needed
	if instance.Status.ServiceCredentialStatus == nil {
		instance.Status.ServiceCredentialStatus = make(map[string]dataplanev1.ServiceCredentialInfo)
	}

	// Get all nodes in this nodeset
	allNodes := getAllNodeNames(instance)
	totalNodes := len(allNodes)

	// Determine which nodes were covered by this deployment (handle AnsibleLimit)
	coveredNodes := getNodesCoveredByDeployment(deployment, instance)

	Log.Info("Updating RabbitMQ credential status",
		"deployment", deployment.Name,
		"deploymentReady", isDeploymentReady,
		"totalNodes", totalNodes,
		"coveredNodes", len(coveredNodes),
		"needsBootstrap", needsBootstrap)

	// Detect RabbitMQ secrets in this deployment by parsing transport_url from config secrets
	rabbitmqSecrets, err := r.detectRabbitMQSecretsInDeployment(ctx, deployment, instance.Namespace)
	if err != nil {
		Log.Error(err, "Failed to detect RabbitMQ secrets in deployment")
		return err
	}

	// Bootstrap fallback: If this is the first time tracking credentials (status is nil)
	// and we couldn't find any in the deployment (empty result), fall back to introspecting
	// RabbitMQUser CRs to discover what's currently deployed.
	//
	// This handles edge cases like:
	// - Tracking operator deployed after deployments already completed
	// - Old deployments that don't have config secrets with transport_url
	// - Deployments were deleted but nodes still have credentials
	//
	// After bootstrap, future deployments will be tracked normally via transport_url parsing.
	if needsBootstrap && len(rabbitmqSecrets) == 0 {
		Log.Info("Bootstrapping: No RabbitMQ secrets in deployment, introspecting RabbitMQUser CRs",
			"deployment", deployment.Name)

		bootstrapSecrets, err := r.bootstrapRabbitMQSecretsFromRabbitMQUsers(ctx, instance.Namespace)
		if err != nil {
			Log.Error(err, "Failed to bootstrap RabbitMQ secrets from RabbitMQUser CRs")
			return err
		}

		if len(bootstrapSecrets) > 0 {
			Log.Info("Bootstrap discovered RabbitMQ credentials from RabbitMQUser CRs",
				"count", len(bootstrapSecrets))
			rabbitmqSecrets = bootstrapSecrets
		}
	}

	// Track each RabbitMQ secret using TransportURL identifier as key (not secret name)
	// This enables automatic rotation detection: when a new user is created for the same
	// TransportURL, we move the old credential to Previous* fields
	for secretName, secretInfo := range rabbitmqSecrets {
		// Extract TransportURL identifier from secret name to use as tracking key
		// Example: rabbitmq-user-nova-cell1-transport-cell1user2-user → nova-cell1-transport
		transportURLID := extractTransportURLFromSecretName(secretName)
		if transportURLID == "" {
			Log.Error(fmt.Errorf("failed to extract TransportURL from secret name"), "",
				"secret", secretName)
			continue
		}

		// Use TransportURL as key for status map
		credInfo, exists := instance.Status.ServiceCredentialStatus[transportURLID]

		if !exists {
			// First time seeing this TransportURL
			updatedNodes := []string{}
			if isDeploymentReady {
				updatedNodes = coveredNodes
			}

			credInfo = dataplanev1.ServiceCredentialInfo{
				SecretName:      secretName,
				SecretHash:      secretInfo.secretHash,
				UpdatedNodes:    updatedNodes,
				TotalNodes:      totalNodes,
				AllNodesUpdated: isDeploymentReady && (len(updatedNodes) == totalNodes),
			}

			Log.Info("New RabbitMQ credential detected",
				"transportURL", transportURLID,
				"secret", secretName,
				"deploymentReady", isDeploymentReady,
				"updatedNodes", len(updatedNodes))

		} else if credInfo.SecretName != secretName {
			// CREDENTIAL ROTATION: Different secret (new user) for same TransportURL
			// BUG FIX: Save current as previous BEFORE updating to new version
			// This prevents old credentials from becoming invisible to RabbitMQ controller

			Log.Info("RabbitMQ user rotation detected",
				"transportURL", transportURLID,
				"oldSecret", credInfo.SecretName,
				"newSecret", secretName,
				"deploymentReady", isDeploymentReady)

			// Preserve old version as previous
			credInfo.PreviousSecretName = credInfo.SecretName
			credInfo.PreviousSecretHash = credInfo.SecretHash

			// Update to new version
			credInfo.SecretName = secretName
			credInfo.SecretHash = secretInfo.secretHash

			// Reset node tracking for new version
			// Only add nodes if deployment is ready
			if isDeploymentReady {
				credInfo.UpdatedNodes = coveredNodes
				credInfo.AllNodesUpdated = (len(coveredNodes) == totalNodes)
			} else {
				credInfo.UpdatedNodes = []string{}
				credInfo.AllNodesUpdated = false
			}

			credInfo.TotalNodes = totalNodes

		} else {
			// SAME version - accumulate nodes across deployments
			// Only update if deployment is ready
			if isDeploymentReady {
				// Add newly covered nodes
				for _, node := range coveredNodes {
					if !slices.Contains(credInfo.UpdatedNodes, node) {
						credInfo.UpdatedNodes = append(credInfo.UpdatedNodes, node)
					}
				}

				// Recalculate AllNodesUpdated
				credInfo.AllNodesUpdated = (len(credInfo.UpdatedNodes) == totalNodes)

				// Clear previous version if all nodes now have current version
				if credInfo.AllNodesUpdated && credInfo.PreviousSecretHash != "" {
					Log.Info("All nodes updated with new credential, clearing previous version tracking",
						"secret", secretName,
						"previousSecret", credInfo.PreviousSecretName)
					credInfo.PreviousSecretName = ""
					credInfo.PreviousSecretHash = ""
				}
			}

			// Update total nodes if nodeset was scaled
			if credInfo.TotalNodes != totalNodes {
				Log.Info("NodeSet size changed, updating credential tracking",
					"secret", secretName,
					"oldTotal", credInfo.TotalNodes,
					"newTotal", totalNodes,
					"updatedNodes", len(credInfo.UpdatedNodes))
				credInfo.TotalNodes = totalNodes
				credInfo.AllNodesUpdated = (len(credInfo.UpdatedNodes) == totalNodes)
			}
		}

		// Update timestamp if deployment is ready
		if isDeploymentReady {
			deploymentReadyCondition := deployment.Status.Conditions.Get(condition.DeploymentReadyCondition)
			if deploymentReadyCondition != nil {
				credInfo.LastUpdateTime = &deploymentReadyCondition.LastTransitionTime
			}
		}

		// Save back to status using TransportURL as key
		instance.Status.ServiceCredentialStatus[transportURLID] = credInfo

		Log.Info("Updated RabbitMQ credential status",
			"transportURL", transportURLID,
			"secret", secretName,
			"deploymentReady", isDeploymentReady,
			"updatedNodes", len(credInfo.UpdatedNodes),
			"totalNodes", credInfo.TotalNodes,
			"allNodesUpdated", credInfo.AllNodesUpdated,
			"hasPrevious", credInfo.PreviousSecretHash != "")
	}

	return nil
}

// RabbitMQSecretInfo holds information about a RabbitMQ credential secret
type RabbitMQSecretInfo struct {
	secretName string // RabbitMQUser secret name (e.g., rabbitmq-user-nova-cell1-transport-novacell1-11-user)
	secretHash string // hash of the secret
}

// detectRabbitMQSecretsInDeployment detects RabbitMQ credential secrets in a deployment
// by parsing transport_url from config secrets.
//
// Algorithm:
// 1. Scan all secrets in deployment.Status.SecretHashes
// 2. Look for transport_url fields in secret data
// 3. Extract username from transport_url using regex
// 4. Find corresponding rabbitmq-user-* secret by listing and matching username
// 5. Track those secrets
func (r *OpenStackDataPlaneNodeSetReconciler) detectRabbitMQSecretsInDeployment(
	ctx context.Context,
	deployment *dataplanev1.OpenStackDataPlaneDeployment,
	namespace string,
) (map[string]RabbitMQSecretInfo, error) {
	Log := r.GetLogger(ctx)
	secrets := make(map[string]RabbitMQSecretInfo)

	if deployment == nil {
		return secrets, nil
	}

	// Track usernames we've already found to avoid duplicates
	foundUsernames := make(map[string]bool)

	// Scan all secrets in the deployment for transport_url fields
	for secretName := range deployment.Status.SecretHashes {
		// Skip certificate secrets (optimization - they won't have transport_url)
		if strings.HasPrefix(secretName, "cert-") {
			continue
		}

		// Get the secret
		secret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: namespace,
		}, secret)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				Log.V(1).Info("Secret not found, skipping", "secret", secretName)
				continue
			}
			Log.Error(err, "Failed to get secret, skipping", "secret", secretName)
			continue
		}

		// Scan all fields in secret data for transport_url
		for key, value := range secret.Data {
			valueStr := string(value)

			// Look for transport_url in this field
			if !strings.Contains(valueStr, "transport_url=") {
				continue
			}

			// Extract username from transport_url using regex
			matches := transportURLUsernameRegex.FindStringSubmatch(valueStr)
			if len(matches) < 2 {
				Log.V(1).Info("Could not extract username from transport_url",
					"secret", secretName,
					"field", key)
				continue
			}

			username := matches[1]

			// Skip if we already found this username
			if foundUsernames[username] {
				continue
			}
			foundUsernames[username] = true

			Log.V(1).Info("Found RabbitMQ username in deployment",
				"username", username,
				"secret", secretName,
				"field", key)

			// Find the rabbitmq-user-* secret for this username
			rabbitMQUserSecret, err := r.findRabbitMQUserSecretByUsername(ctx, namespace, username)
			if err != nil {
				Log.Error(err, "Failed to find RabbitMQ user secret for username",
					"username", username)
				continue
			}
			if rabbitMQUserSecret == "" {
				Log.V(1).Info("No RabbitMQ user secret found for username",
					"username", username)
				continue
			}

			// Get the secret to compute hash
			userSecret := &corev1.Secret{}
			err = r.Get(ctx, types.NamespacedName{
				Name:      rabbitMQUserSecret,
				Namespace: namespace,
			}, userSecret)
			if err != nil {
				if k8s_errors.IsNotFound(err) {
					Log.Info("RabbitMQ user secret not found",
						"secret", rabbitMQUserSecret)
					continue
				}
				Log.Error(err, "Failed to get RabbitMQ user secret",
					"secret", rabbitMQUserSecret)
				continue
			}

			// Compute hash
			rabbitmqSecretHash, err := util.ObjectHash(userSecret.Data)
			if err != nil {
				Log.Error(err, "Failed to compute RabbitMQ secret hash",
					"secret", rabbitMQUserSecret)
				continue
			}

			secrets[rabbitMQUserSecret] = RabbitMQSecretInfo{
				secretName: rabbitMQUserSecret,
				secretHash: rabbitmqSecretHash,
			}

			Log.Info("Detected RabbitMQ credential in deployment",
				"username", username,
				"secret", rabbitMQUserSecret,
				"hash", rabbitmqSecretHash)
		}
	}

	return secrets, nil
}

// findRabbitMQUserSecretByUsername finds the rabbitmq-user-* secret name for a given username.
// It lists all secrets with the rabbitmq-user- prefix and checks their username field.
func (r *OpenStackDataPlaneNodeSetReconciler) findRabbitMQUserSecretByUsername(
	ctx context.Context,
	namespace string,
	username string,
) (string, error) {
	Log := r.GetLogger(ctx)

	// List all secrets in the namespace
	secretList := &corev1.SecretList{}
	if err := r.List(ctx, secretList, client.InNamespace(namespace)); err != nil {
		return "", fmt.Errorf("failed to list secrets: %w", err)
	}

	// Find secret with rabbitmq-user- prefix that has this username
	for i := range secretList.Items {
		secret := &secretList.Items[i]

		// Only check rabbitmq-user-* secrets
		if !strings.HasPrefix(secret.Name, rabbitmqUserSecretPrefix) {
			continue
		}

		// Check if this secret has the username we're looking for
		if secretUsername, ok := secret.Data["username"]; ok {
			if string(secretUsername) == username {
				Log.V(1).Info("Found matching RabbitMQ user secret",
					"username", username,
					"secret", secret.Name)
				return secret.Name, nil
			}
		}
	}

	return "", nil
}

// bootstrapRabbitMQSecretsFromRabbitMQUsers discovers RabbitMQ user secrets by introspecting
// RabbitMQUser CRs in the namespace. This is used ONLY during bootstrap (when serviceCredentialStatus
// is nil AND no secrets found in deployment) to discover what credentials are actually in use on
// the dataplane nodes.
//
// Why this is needed:
//   - Normal operation parses transport_url from config secrets in the deployment
//   - But if the deployment was deleted, or tracking was deployed after deployments completed,
//     we have no deployment to parse
//   - Edge case: operator upgraded, old deployments exist but don't have the tracking logic yet
//   - Bootstrap fills the gap by directly listing RabbitMQUser CRs to see what's currently deployed
//
// This is a one-time operation:
// - Only runs when status is nil (first time seeing this nodeset)
// - Only runs when deployment has no RabbitMQ secrets (can't parse from deployment)
// - After bootstrap, normal transport_url parsing takes over
// - Future deployments will be tracked via detectRabbitMQSecretsInDeployment()
func (r *OpenStackDataPlaneNodeSetReconciler) bootstrapRabbitMQSecretsFromRabbitMQUsers(
	ctx context.Context,
	namespace string,
) (map[string]RabbitMQSecretInfo, error) {
	Log := r.GetLogger(ctx)
	secrets := make(map[string]RabbitMQSecretInfo)

	Log.Info("Bootstrapping RabbitMQ credential tracking from RabbitMQUser CRs", "namespace", namespace)

	// List all RabbitMQUser CRs in the namespace
	rabbitMQUserList := &rabbitmqv1.RabbitMQUserList{}
	if err := r.List(ctx, rabbitMQUserList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list RabbitMQUsers for bootstrap: %w", err)
	}

	Log.Info("Found RabbitMQUsers during bootstrap", "count", len(rabbitMQUserList.Items))

	// Extract RabbitMQ user secrets from each RabbitMQUser
	for i := range rabbitMQUserList.Items {
		rabbitMQUser := &rabbitMQUserList.Items[i]

		// Get the RabbitMQ user secret name from the RabbitMQUser status
		secretName := rabbitMQUser.Status.SecretName
		if secretName == "" {
			Log.V(1).Info("RabbitMQUser has no secret name in status, skipping",
				"rabbitMQUser", rabbitMQUser.Name)
			continue
		}

		// Only track RabbitMQ user secrets (follow naming convention for safety)
		if !strings.HasPrefix(secretName, rabbitmqUserSecretPrefix) {
			Log.V(1).Info("Secret is not a RabbitMQ user secret, skipping",
				"rabbitMQUser", rabbitMQUser.Name,
				"secret", secretName)
			continue
		}

		// Skip if we already found this secret
		if _, exists := secrets[secretName]; exists {
			continue
		}

		// Get the secret to compute its hash
		secret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: namespace,
		}, secret)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				Log.Info("RabbitMQ secret referenced by RabbitMQUser not found, skipping",
					"rabbitMQUser", rabbitMQUser.Name,
					"secret", secretName)
				continue
			}
			Log.Error(err, "Failed to get RabbitMQ secret during bootstrap, skipping",
				"rabbitMQUser", rabbitMQUser.Name,
				"secret", secretName)
			continue
		}

		// Compute hash of the secret data
		rabbitmqSecretHash, err := util.ObjectHash(secret.Data)
		if err != nil {
			Log.Error(err, "Failed to compute RabbitMQ secret hash during bootstrap, skipping",
				"secret", secretName)
			continue
		}

		secrets[secretName] = RabbitMQSecretInfo{
			secretName: secretName,
			secretHash: rabbitmqSecretHash,
		}

		Log.Info("Discovered RabbitMQ credential during bootstrap",
			"rabbitMQUser", rabbitMQUser.Name,
			"secret", secretName,
			"hash", rabbitmqSecretHash)
	}

	return secrets, nil
}

// extractTransportURLFromSecretName extracts the TransportURL identifier from a
// rabbitmq-user-* secret name.
//
// Secret name pattern: rabbitmq-user-{transporturl}-{username}-user
// Example: rabbitmq-user-nova-cell1-transport-cell1user1-user
//
//	→ TransportURL: nova-cell1-transport
//
// This is used to detect rotation when a new user is created for the same TransportURL.
func extractTransportURLFromSecretName(secretName string) string {
	// Remove rabbitmq-user- prefix
	if !strings.HasPrefix(secretName, rabbitmqUserSecretPrefix) {
		return ""
	}
	remainder := strings.TrimPrefix(secretName, rabbitmqUserSecretPrefix)

	// Secret format: {transporturl}-{username}-user
	// We need to find where transporturl ends and username begins
	// The suffix is always "-user", so remove it first
	if !strings.HasSuffix(remainder, "-user") {
		return ""
	}
	withoutSuffix := strings.TrimSuffix(remainder, "-user")

	// Now we have: {transporturl}-{username}
	// Find the last occurrence of "-" to split transporturl from username
	lastDash := strings.LastIndex(withoutSuffix, "-")
	if lastDash == -1 {
		return ""
	}

	transportURL := withoutSuffix[:lastDash]
	return transportURL
}

// getNodesCoveredByDeployment determines which nodes were covered by a deployment
// based on the AnsibleLimit field
func getNodesCoveredByDeployment(
	deployment *dataplanev1.OpenStackDataPlaneDeployment,
	nodeset *dataplanev1.OpenStackDataPlaneNodeSet,
) []string {
	if deployment == nil || nodeset == nil {
		return []string{}
	}

	allNodes := getAllNodeNames(nodeset)

	// Check AnsibleLimit
	ansibleLimit := deployment.Spec.AnsibleLimit
	if ansibleLimit == "" || ansibleLimit == "*" {
		// All nodes covered
		return allNodes
	}

	// Parse AnsibleLimit (comma-separated list)
	limitParts := strings.Split(ansibleLimit, ",")

	coveredNodes := make([]string, 0, len(allNodes))
	for _, node := range allNodes {
		for _, part := range limitParts {
			part = strings.TrimSpace(part)

			// Exact match
			if part == node {
				coveredNodes = append(coveredNodes, node)
				break
			}

			// Wildcard matching
			if strings.HasSuffix(part, "*") {
				prefix := strings.TrimSuffix(part, "*")
				if strings.HasPrefix(node, prefix) {
					coveredNodes = append(coveredNodes, node)
					break
				}
			}
		}
	}

	return coveredNodes
}

// getAllNodeNames returns a list of all node names in the nodeset
func getAllNodeNames(nodeset *dataplanev1.OpenStackDataPlaneNodeSet) []string {
	if nodeset == nil {
		return []string{}
	}
	nodes := make([]string, 0, len(nodeset.Spec.Nodes))
	for nodeName := range nodeset.Spec.Nodes {
		nodes = append(nodes, nodeName)
	}
	return nodes
}
