package operator

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	operatorv1beta1 "github.com/openstack-k8s-operators/openstack-operator/apis/operator/v1beta1"
	"github.com/openstack-k8s-operators/openstack-operator/pkg/operator"
	"github.com/openstack-k8s-operators/openstack-operator/pkg/operator/bindata"
)

type MessagingTopologyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=rabbitmq.com,resources=rabbitmqclusters,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

func (r *MessagingTopologyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Get CA certificates from RabbitMQ clusters (gracefully handle missing CRDs)
	caCerts := []string{}
	rabbitClusters := &uns.UnstructuredList{}
	rabbitClusters.SetGroupVersionKind(schema.GroupVersionKind{Group: "rabbitmq.com", Version: "v1beta1", Kind: "RabbitmqClusterList"})

	if err := r.Client.List(ctx, rabbitClusters); err == nil {
		for _, cluster := range rabbitClusters.Items {
			if tls, found, _ := uns.NestedMap(cluster.Object, "spec", "tls"); found {
				if caSecretName, found, _ := uns.NestedString(tls, "caSecretName"); found && caSecretName != "" {
					caSecret := &corev1.Secret{}
					if r.Client.Get(ctx, client.ObjectKey{Name: caSecretName, Namespace: cluster.GetNamespace()}, caSecret) == nil {
						if caCert, exists := caSecret.Data["ca.crt"]; exists {
							caCerts = append(caCerts, string(caCert))
						}
					}
				}
			}
		}
	}
	// If RabbitMQ CRDs don't exist, caCerts will be empty (which is fine)

	bundleData := strings.Join(caCerts, "\n")

	// Update CA bundle secret and reconcile messaging-topology-operator deployment for each OpenStack instance
	openstackList := &operatorv1beta1.OpenStackList{}
	r.Client.List(ctx, openstackList)

	for _, openstack := range openstackList.Items {
		// Always create CA bundle secret (even if empty) so deployment can mount it
		caSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "messaging-topology-ca-bundle", Namespace: openstack.Namespace},
			Data:       map[string][]byte{"messaging-topology-ca-bundle.crt": []byte(bundleData)},
		}
		if err := ctrl.SetControllerReference(&openstack, caSecret, r.Scheme); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set controller reference for CA bundle secret: %w", err)
		}
		if err := r.Client.Create(ctx, caSecret); apierrors.IsAlreadyExists(err) {
			if err := r.Client.Update(ctx, caSecret); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update CA bundle secret: %w", err)
			}
		}

		// Fix orphaned webhook cert secret by adding owner reference
		webhookSecret := &corev1.Secret{}
		if err := r.Client.Get(ctx, client.ObjectKey{Name: "messaging-topology-operator-webhook-server-cert", Namespace: openstack.Namespace}, webhookSecret); err == nil {
			if err := ctrl.SetControllerReference(&openstack, webhookSecret, r.Scheme); err == nil {
				r.Client.Update(ctx, webhookSecret)
			}
		}

		// Reconcile messaging-topology-operator deployment with CA bundle hash
		if err := r.reconcileMessagingTopologyDeployment(ctx, &openstack, bundleData); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reconcile messaging-topology-operator deployment: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *MessagingTopologyReconciler) getOperatorImage(operatorName string) string {
	// Use the same pattern as main controller: RELATED_IMAGE_*_OPERATOR_MANAGER_IMAGE_URL
	envVar := "RELATED_IMAGE_" + strings.ToUpper(strings.ReplaceAll(operatorName, "-", "_")) + "_OPERATOR_MANAGER_IMAGE_URL"
	if img := os.Getenv(envVar); img != "" {
		return img
	}
	// Fallback to a reasonable default
	return "quay.io/openstack-k8s-operators/rabbitmq-messaging-topology-operator:latest"
}

func (r *MessagingTopologyReconciler) getKubeRbacProxyImage() string {
	// Get kube-rbac-proxy image from environment variable
	if img := os.Getenv("KUBE_RBAC_PROXY"); img != "" {
		return img
	}
	// Fallback to a reasonable default
	return "gcr.io/kubebuilder/kube-rbac-proxy:v0.15.0"
}

func (r *MessagingTopologyReconciler) reconcileMessagingTopologyDeployment(ctx context.Context, instance *operatorv1beta1.OpenStack, bundleData string) error {
	// Initialize with full defaults matching main controller pattern
	defaultEnv := []corev1.EnvVar{
		{
			Name:  "LEASE_DURATION",
			Value: "15",
		},
		{
			Name:  "RENEW_DEADLINE",
			Value: "10",
		},
		{
			Name:  "RETRY_PERIOD",
			Value: "2",
		},
	}

	kubeRbacProxyContainer := operator.Container{
		Image: r.getKubeRbacProxyImage(),
		Resources: operator.Resource{
			Limits: &operator.ResourceList{
				CPU:    operatorv1beta1.DefaultRbacProxyCPULimit.String(),
				Memory: operatorv1beta1.DefaultRbacProxyMemoryLimit.String(),
			},
			Requests: &operator.ResourceList{
				CPU:    operatorv1beta1.DefaultRbacProxyCPURequests.String(),
				Memory: operatorv1beta1.DefaultRbacProxyMemoryRequests.String(),
			},
		},
	}

	// Get messaging-topology-operator configuration with full defaults
	messagingTopologyOperator := operator.Operator{
		Name:      operatorv1beta1.MessagingTopologyOperatorName,
		Namespace: instance.Namespace,
		Deployment: operator.Deployment{
			Replicas: ptr.To(operatorv1beta1.ReplicasEnabled),
			Manager: operator.Container{
				Image: r.getOperatorImage(operatorv1beta1.MessagingTopologyOperatorName),
				Env:   defaultEnv,
				Resources: operator.Resource{
					Limits: &operator.ResourceList{
						CPU:    operatorv1beta1.DefaultManagerCPULimit.String(),
						Memory: operatorv1beta1.DefaultManagerMemoryLimit.String(),
					},
					Requests: &operator.ResourceList{
						CPU:    operatorv1beta1.DefaultManagerCPURequests.String(),
						Memory: operatorv1beta1.DefaultManagerMemoryRequests.String(),
					},
				},
			},
			KubeRbacProxy: kubeRbacProxyContainer,
		},
	}

	// Apply user overrides if any
	if opOvr := operator.HasOverrides(instance.Spec.OperatorOverrides, operatorv1beta1.MessagingTopologyOperatorName); opOvr != nil {
		operator.SetOverrides(*opOvr, &messagingTopologyOperator)
	}

	// Skip if disabled
	if *messagingTopologyOperator.Deployment.Replicas == 0 {
		return nil
	}

	// Add CA bundle hash to trigger deployment updates when secret changes
	bundleHash := fmt.Sprintf("%x", sha256.Sum256([]byte(bundleData)))
	messagingTopologyOperator.Deployment.Manager.Env = append(messagingTopologyOperator.Deployment.Manager.Env,
		corev1.EnvVar{Name: "CA_BUNDLE_HASH", Value: bundleHash})

	// Prepare template data
	data := bindata.MakeRenderData()
	data.Data["MessagingTopologyOperator"] = messagingTopologyOperator
	data.Data["OperatorNamespace"] = instance.Namespace

	// Render and apply the deployment
	return r.renderAndApply(ctx, instance, data, "messaging-topology")
}

func (r *MessagingTopologyReconciler) renderAndApply(ctx context.Context, instance *operatorv1beta1.OpenStack, data bindata.RenderData, templateName string) error {
	// Use same path logic as main controller
	bindir := "/bindata" // Default from openstack controller
	if envVar := os.Getenv("BASE_BINDATA"); envVar != "" {
		bindir = envVar
	}

	// Render only the specific messaging-topology.yaml template
	templatePath := filepath.Join(bindir, "operator", "messaging-topology.yaml")
	manifests, err := bindata.RenderTemplate(templatePath, &data)
	if err != nil {
		return fmt.Errorf("failed to render messaging-topology template: %w", err)
	}

	for _, obj := range manifests {
		if obj == nil {
			continue
		}

		obj.SetNamespace(instance.Namespace)
		if err := ctrl.SetControllerReference(instance, obj, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}

		if err := r.Client.Create(ctx, obj); apierrors.IsAlreadyExists(err) {
			if err := r.Client.Update(ctx, obj); err != nil {
				return fmt.Errorf("failed to update %s: %w", obj.GetName(), err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to create %s: %w", obj.GetName(), err)
		}
	}

	return nil
}

func (r *MessagingTopologyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1beta1.OpenStack{}).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
			// Only watch messaging-topology-ca-bundle secrets
			if obj.GetName() != "messaging-topology-ca-bundle" {
				return nil
			}

			// Find the OpenStack instance in this namespace
			list := &operatorv1beta1.OpenStackList{}
			mgr.GetClient().List(ctx, list, &client.ListOptions{Namespace: obj.GetNamespace()})
			reqs := make([]ctrl.Request, len(list.Items))
			for i, os := range list.Items {
				reqs[i] = ctrl.Request{NamespacedName: client.ObjectKey{Name: os.Name, Namespace: os.Namespace}}
			}
			return reqs
		}))

	// Try to watch RabbitmqCluster, but fail gracefully if CRDs don't exist
	rabbitmq := &uns.Unstructured{}
	rabbitmq.SetGroupVersionKind(schema.GroupVersionKind{Group: "rabbitmq.com", Version: "v1beta1", Kind: "RabbitmqCluster"})

	err := builder.Watches(rabbitmq, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		// Trigger for any OpenStack resource when RabbitMQ changes
		list := &operatorv1beta1.OpenStackList{}
		mgr.GetClient().List(ctx, list)
		reqs := make([]ctrl.Request, len(list.Items))
		for i, os := range list.Items {
			reqs[i] = ctrl.Request{NamespacedName: client.ObjectKey{Name: os.Name, Namespace: os.Namespace}}
		}
		return reqs
	})).Complete(r)

	// If RabbitmqCluster CRD doesn't exist, just proceed without the watch
	if err != nil && strings.Contains(err.Error(), "no matches for kind \"RabbitmqCluster\"") {
		return builder.Complete(r)
	}

	return err
}
