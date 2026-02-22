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
	"encoding/json"
	"fmt"
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
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
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

			// Update secret deployment tracking BEFORE copying hashes
			// Track secrets when:
			// 1. Config or secrets changed (normal case)
			// 2. Status is empty (first-time tracking)
			if configHashChanged || secretsChanged || instance.Status.SecretDeployment == nil {
				if err := r.updateSecretDeploymentTracking(ctx, helper, instance, deployment); err != nil {
					helper.GetLogger().Error(err, "Failed to update secret deployment tracking")
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

// SecretTrackingData represents the structure stored in the secret tracking ConfigMap
type SecretTrackingData struct {
	Secrets    map[string]SecretVersionInfo `json:"secrets"`
	NodeStatus map[string]NodeSecretStatus  `json:"nodeStatus"`
}

// SecretVersionInfo tracks a secret's current and previous versions across nodes
type SecretVersionInfo struct {
	CurrentHash       string    `json:"currentHash"`
	PreviousHash      string    `json:"previousHash,omitempty"`
	NodesWithCurrent  []string  `json:"nodesWithCurrent"`
	NodesWithPrevious []string  `json:"nodesWithPrevious,omitempty"`
	LastChanged       time.Time `json:"lastChanged"`
}

// NodeSecretStatus tracks which secrets a node has (current version)
type NodeSecretStatus struct {
	AllSecretsUpdated   bool     `json:"allSecretsUpdated"`
	SecretsWithCurrent  []string `json:"secretsWithCurrent"`
	SecretsWithPrevious []string `json:"secretsWithPrevious,omitempty"`
}

// getSecretTrackingConfigMapName returns the name of the tracking ConfigMap for a nodeset
func getSecretTrackingConfigMapName(nodesetName string) string {
	return fmt.Sprintf("%s-secret-tracking", nodesetName)
}

// getSecretTrackingData reads and parses the secret tracking data from the ConfigMap
func (r *OpenStackDataPlaneNodeSetReconciler) getSecretTrackingData(
	ctx context.Context,
	helper *helper.Helper,
	instance *dataplanev1.OpenStackDataPlaneNodeSet,
) (*SecretTrackingData, error) {
	configMapName := getSecretTrackingConfigMapName(instance.Name)

	cm := &corev1.ConfigMap{}
	err := helper.GetClient().Get(ctx, types.NamespacedName{
		Name:      configMapName,
		Namespace: instance.Namespace,
	}, cm)

	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// ConfigMap doesn't exist yet, return empty structure
			return &SecretTrackingData{
				Secrets:    make(map[string]SecretVersionInfo),
				NodeStatus: make(map[string]NodeSecretStatus),
			}, nil
		}
		return nil, err
	}

	// Parse JSON from tracking.json key
	trackingJSON, exists := cm.Data["tracking.json"]
	if !exists || trackingJSON == "" {
		// Empty ConfigMap, return empty structure
		return &SecretTrackingData{
			Secrets:    make(map[string]SecretVersionInfo),
			NodeStatus: make(map[string]NodeSecretStatus),
		}, nil
	}

	var data SecretTrackingData
	if err := json.Unmarshal([]byte(trackingJSON), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tracking data: %w", err)
	}

	// Ensure maps are initialized
	if data.Secrets == nil {
		data.Secrets = make(map[string]SecretVersionInfo)
	}
	if data.NodeStatus == nil {
		data.NodeStatus = make(map[string]NodeSecretStatus)
	}

	return &data, nil
}

// saveSecretTrackingData marshals and saves the tracking data to the ConfigMap
func (r *OpenStackDataPlaneNodeSetReconciler) saveSecretTrackingData(
	ctx context.Context,
	helper *helper.Helper,
	instance *dataplanev1.OpenStackDataPlaneNodeSet,
	data *SecretTrackingData,
) error {
	configMapName := getSecretTrackingConfigMapName(instance.Name)

	// Marshal to JSON
	trackingJSON, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tracking data: %w", err)
	}

	customData := map[string]string{
		"tracking.json": string(trackingJSON),
	}

	cms := []util.Template{
		{
			Name:         configMapName,
			Namespace:    instance.Namespace,
			InstanceType: instance.Kind,
			CustomData:   customData,
		},
	}

	return configmap.EnsureConfigMaps(ctx, helper, instance, cms, nil)
}

// computeDeploymentSummary calculates summary status from detailed tracking data
func computeDeploymentSummary(
	data *SecretTrackingData,
	totalNodes int,
	configMapName string,
) *dataplanev1.SecretDeploymentStatus {
	updatedNodes := 0
	for _, nodeStatus := range data.NodeStatus {
		if nodeStatus.AllSecretsUpdated {
			updatedNodes++
		}
	}

	now := v1.Now()
	return &dataplanev1.SecretDeploymentStatus{
		AllNodesUpdated: updatedNodes == totalNodes && totalNodes > 0,
		TotalNodes:      totalNodes,
		UpdatedNodes:    updatedNodes,
		ConfigMapName:   configMapName,
		LastUpdateTime:  &now,
	}
}

// updateSecretDeploymentTracking updates the NodeSet status with information about which
// nodes have been updated with secrets from a deployment.
//
// This tracks ALL secrets in deployment.Status.SecretHashes, storing detailed per-secret
// tracking in a ConfigMap and summary metrics in the CR status.
func (r *OpenStackDataPlaneNodeSetReconciler) updateSecretDeploymentTracking(
	ctx context.Context,
	helper *helper.Helper,
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

	// Get all nodes in this nodeset
	allNodes := getAllNodeNames(instance)
	totalNodes := len(allNodes)

	// Determine which nodes were covered by this deployment (handle AnsibleLimit)
	coveredNodes := getNodesCoveredByDeployment(deployment, instance)

	Log.Info("Updating secret deployment tracking",
		"deployment", deployment.Name,
		"deploymentReady", isDeploymentReady,
		"totalNodes", totalNodes,
		"coveredNodes", len(coveredNodes),
		"secretsInDeployment", len(deployment.Status.SecretHashes))

	// Load existing tracking data
	trackingData, err := r.getSecretTrackingData(ctx, helper, instance)
	if err != nil {
		Log.Error(err, "Failed to load secret tracking data")
		return err
	}

	// Process each secret in the deployment
	for secretName, secretHash := range deployment.Status.SecretHashes {
		secretInfo, exists := trackingData.Secrets[secretName]

		if !exists {
			// First time seeing this secret
			nodesWithCurrent := []string{}
			if isDeploymentReady {
				nodesWithCurrent = coveredNodes
			}

			secretInfo = SecretVersionInfo{
				CurrentHash:      secretHash,
				NodesWithCurrent: nodesWithCurrent,
				LastChanged:      time.Now(),
			}

			Log.Info("New secret detected",
				"secret", secretName,
				"hash", secretHash,
				"deploymentReady", isDeploymentReady,
				"nodesWithCurrent", len(nodesWithCurrent))

		} else if secretInfo.CurrentHash != secretHash {
			// SECRET ROTATION: Hash changed for existing secret
			// Preserve old version as previous

			Log.Info("Secret rotation detected",
				"secret", secretName,
				"oldHash", secretInfo.CurrentHash,
				"newHash", secretHash,
				"deploymentReady", isDeploymentReady)

			// Move current to previous
			secretInfo.PreviousHash = secretInfo.CurrentHash
			secretInfo.NodesWithPrevious = secretInfo.NodesWithCurrent

			// Update to new version
			secretInfo.CurrentHash = secretHash
			secretInfo.LastChanged = time.Now()

			// Reset node tracking for new version
			if isDeploymentReady {
				secretInfo.NodesWithCurrent = coveredNodes
			} else {
				secretInfo.NodesWithCurrent = []string{}
			}

		} else {
			// SAME version - accumulate nodes across deployments
			if isDeploymentReady {
				// Add newly covered nodes
				for _, node := range coveredNodes {
					if !slices.Contains(secretInfo.NodesWithCurrent, node) {
						secretInfo.NodesWithCurrent = append(secretInfo.NodesWithCurrent, node)
					}
				}

				// Clear previous version if all nodes now have current version
				if len(secretInfo.NodesWithCurrent) == totalNodes && secretInfo.PreviousHash != "" {
					Log.Info("All nodes updated with new secret version, clearing previous",
						"secret", secretName,
						"previousHash", secretInfo.PreviousHash)
					secretInfo.PreviousHash = ""
					secretInfo.NodesWithPrevious = []string{}
				}
			}
		}

		trackingData.Secrets[secretName] = secretInfo
	}

	// Update per-node status
	for _, nodeName := range allNodes {
		nodeStatus := NodeSecretStatus{
			AllSecretsUpdated:   true,
			SecretsWithCurrent:  []string{},
			SecretsWithPrevious: []string{},
		}

		// Check which secrets this node has
		for secretName, secretInfo := range trackingData.Secrets {
			if slices.Contains(secretInfo.NodesWithCurrent, nodeName) {
				nodeStatus.SecretsWithCurrent = append(nodeStatus.SecretsWithCurrent, secretName)
			} else if slices.Contains(secretInfo.NodesWithPrevious, nodeName) {
				nodeStatus.SecretsWithPrevious = append(nodeStatus.SecretsWithPrevious, secretName)
				nodeStatus.AllSecretsUpdated = false
			} else {
				// Node doesn't have this secret at all
				nodeStatus.AllSecretsUpdated = false
			}
		}

		trackingData.NodeStatus[nodeName] = nodeStatus
	}

	// Save tracking data to ConfigMap
	if err := r.saveSecretTrackingData(ctx, helper, instance, trackingData); err != nil {
		Log.Error(err, "Failed to save secret tracking data")
		return err
	}

	// Calculate and update summary in CR status
	configMapName := getSecretTrackingConfigMapName(instance.Name)
	instance.Status.SecretDeployment = computeDeploymentSummary(trackingData, totalNodes, configMapName)

	Log.Info("Secret deployment tracking updated",
		"totalNodes", instance.Status.SecretDeployment.TotalNodes,
		"updatedNodes", instance.Status.SecretDeployment.UpdatedNodes,
		"allNodesUpdated", instance.Status.SecretDeployment.AllNodesUpdated)

	return nil
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
