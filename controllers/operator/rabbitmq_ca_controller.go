package operator

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	operatorv1beta1 "github.com/openstack-k8s-operators/openstack-operator/apis/operator/v1beta1"
)

type RabbitMQCAReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=rabbitmq.com,resources=rabbitmqclusters,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update

func (r *RabbitMQCAReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	rabbitClusters := &uns.UnstructuredList{}
	rabbitClusters.SetGroupVersionKind(schema.GroupVersionKind{Group: "rabbitmq.com", Version: "v1beta1", Kind: "RabbitmqClusterList"})
	r.Client.List(ctx, rabbitClusters)

	caCerts := []string{}
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

	// Always create CA bundle in all OpenStack namespaces (empty if no certs)
	bundleData := strings.Join(caCerts, "\n")
	openstackList := &operatorv1beta1.OpenStackList{}
	r.Client.List(ctx, openstackList)
	for _, openstack := range openstackList.Items {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "messaging-topology-ca-bundle", Namespace: openstack.Namespace},
			Data:       map[string][]byte{"messaging-topology-ca-bundle.crt": []byte(bundleData)},
		}
		if err := r.Client.Create(ctx, secret); apierrors.IsAlreadyExists(err) {
			r.Client.Update(ctx, secret)
		}
	}
	return ctrl.Result{}, nil
}

func (r *RabbitMQCAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	rabbitmq := &uns.Unstructured{}
	rabbitmq.SetGroupVersionKind(schema.GroupVersionKind{Group: "rabbitmq.com", Version: "v1beta1", Kind: "RabbitmqCluster"})
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1beta1.OpenStack{}).
		Watches(rabbitmq, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
			// Trigger for any OpenStack resource when RabbitMQ changes
			list := &operatorv1beta1.OpenStackList{}
			mgr.GetClient().List(ctx, list)
			reqs := make([]ctrl.Request, len(list.Items))
			for i, os := range list.Items {
				reqs[i] = ctrl.Request{NamespacedName: client.ObjectKey{Name: os.Name, Namespace: os.Namespace}}
			}
			return reqs
		})).
		Complete(r)
}
