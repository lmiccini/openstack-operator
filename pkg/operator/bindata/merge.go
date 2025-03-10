package bindata

import (
	"github.com/pkg/errors"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	deploymentRevisionAnnotation = "deployment.kubernetes.io/revision"
)

// MergeMetadataForUpdate merges the read-only fields of metadata.
// This is to be able to do a a meaningful comparison in apply,
// since objects created on runtime do not have these fields populated.
func MergeMetadataForUpdate(current, updated *uns.Unstructured) error {
	updated.SetCreationTimestamp(current.GetCreationTimestamp())
	updated.SetSelfLink(current.GetSelfLink())
	updated.SetGeneration(current.GetGeneration())
	updated.SetUID(current.GetUID())
	updated.SetResourceVersion(current.GetResourceVersion())

	mergeAnnotations(current, updated)
	mergeLabels(current, updated)

	return nil
}

// MergeObjectForUpdate prepares a "desired" object to be updated.
// Some objects, such as Deployments and Services require
// some semantic-aware updates
func MergeObjectForUpdate(current, updated *uns.Unstructured) error {
	if err := MergeWebhookConfigurationForUpdate(current, updated); err != nil {
		return err
	}

	if err := MergeDeploymentForUpdate(current, updated); err != nil {
		return err
	}

	if err := MergeServiceForUpdate(current, updated); err != nil {
		return err
	}

	if err := MergeServiceAccountForUpdate(current, updated); err != nil {
		return err
	}

	// For all object types, merge metadata.
	// Run this last, in case any of the more specific merge logic has
	// changed "updated"
	if err := MergeMetadataForUpdate(current, updated); err != nil {
		return err
	}

	return nil
}

// MergeWebhookConfigurationForUpdate merges certs and clientConfig from the current object
func MergeWebhookConfigurationForUpdate(current, updated *uns.Unstructured) error {
	gvk := updated.GroupVersionKind()

	if gvk.Group == "admissionregistration.k8s.io" && (gvk.Kind == "MutatingWebhookConfiguration" || gvk.Kind == "ValidatingWebhookConfiguration") {

		for i, webhook := range updated.Object["webhooks"].([]interface{}) {

			currentClientConfig := current.Object["webhooks"].([]interface{})[i].(map[string]interface{})["clientConfig"].(map[string]interface{})
			if currentClientConfig != nil {
				webhook.(map[string]interface{})["clientConfig"] = currentClientConfig
			}

		}
	}
	return nil
}

// MergeDeploymentForUpdate updates Deployment objects.
// We merge annotations, keeping ours except the Deployment Revision annotation.
func MergeDeploymentForUpdate(current, updated *uns.Unstructured) error {
	gvk := updated.GroupVersionKind()
	if gvk.Group == "apps" && gvk.Kind == "Deployment" {

		// Copy over the revision annotation from current up to updated
		// otherwise, updated would win, and this annotation is "special" and
		// needs to be preserved
		curAnnotations := current.GetAnnotations()
		updatedAnnotations := updated.GetAnnotations()
		if updatedAnnotations == nil {
			updatedAnnotations = map[string]string{}
		}

		anno, ok := curAnnotations[deploymentRevisionAnnotation]
		if ok {
			updatedAnnotations[deploymentRevisionAnnotation] = anno
		}

		updated.SetAnnotations(updatedAnnotations)
	}

	return nil
}

// MergeServiceForUpdate ensures the ClusterIP/IPFamily is never modified
func MergeServiceForUpdate(current, updated *uns.Unstructured) error {
	gvk := updated.GroupVersionKind()
	if gvk.Group == "" && gvk.Kind == "Service" {
		clusterIP, found, err := uns.NestedString(current.Object, "spec", "clusterIP")
		if err != nil {
			return err
		}
		if found {
			err = uns.SetNestedField(updated.Object, clusterIP, "spec", "clusterIP")
			if err != nil {
				return err
			}
		}

		ipFamily, found, err := uns.NestedString(current.Object, "spec", "ipFamily")
		if err != nil {
			return err
		}
		if found {
			err = uns.SetNestedField(updated.Object, ipFamily, "spec", "ipFamily")
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// MergeServiceAccountForUpdate copies secrets from current to updated.
// This is intended to preserve the auto-generated token.
// Right now, we just copy current to updated and don't support supplying
// any secrets ourselves.
func MergeServiceAccountForUpdate(current, updated *uns.Unstructured) error {
	gvk := updated.GroupVersionKind()
	if gvk.Group == "" && gvk.Kind == "ServiceAccount" {
		curSecrets, ok, err := uns.NestedSlice(current.Object, "secrets")
		if err != nil {
			return err
		}

		if ok {
			err := uns.SetNestedField(updated.Object, curSecrets, "secrets")
			if err != nil {
				return err
			}
		}

		curImagePullSecrets, ok, err := uns.NestedSlice(current.Object, "imagePullSecrets")
		if err != nil {
			return err
		}
		if ok {
			err := uns.SetNestedField(updated.Object, curImagePullSecrets, "imagePullSecrets")
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// mergeAnnotations copies over any annotations from current to updated,
// with updated winning if there's a conflict
func mergeAnnotations(current, updated *uns.Unstructured) {
	updatedAnnotations := updated.GetAnnotations()
	curAnnotations := current.GetAnnotations()

	if curAnnotations == nil {
		curAnnotations = map[string]string{}
	}

	for k, v := range updatedAnnotations {
		curAnnotations[k] = v
	}

	updated.SetAnnotations(curAnnotations)
}

// mergeLabels copies over any labels from current to updated,
// with updated winning if there's a conflict
func mergeLabels(current, updated *uns.Unstructured) {
	updatedLabels := updated.GetLabels()
	curLabels := current.GetLabels()

	if curLabels == nil {
		curLabels = map[string]string{}
	}

	for k, v := range updatedLabels {
		curLabels[k] = v
	}

	// add a label for openstack.openstack.org/crd to identify CRDs we install
	gvk := updated.GroupVersionKind()
	if gvk.Group == "apiextensions.k8s.io" && gvk.Kind == "CustomResourceDefinition" {
		curLabels["openstack.openstack.org/crd"] = ""
	}
	// Validating/Mutating webhooks aren't namespaced meaning we can't own them directly
	// via the initialization resource. This adds a custom label so that at least we
	// can identify them for cleanup via a finalizer
	if gvk.Group == "admissionregistration.k8s.io" && (gvk.Kind == "MutatingWebhookConfiguration" || gvk.Kind == "ValidatingWebhookConfiguration") {
		curLabels["openstack.openstack.org/managed"] = "true"
	}

	// remove the olm.managed label this allows cleanup of the operator objects
	// from legacy service operator deployments prior to FR2
	delete(curLabels, "olm.managed")

	updated.SetLabels(curLabels)
}

// IsObjectSupported rejects objects with configurations we don't support.
// This catches ServiceAccounts with secrets, which is valid but we don't
// support reconciling them.
func IsObjectSupported(obj *uns.Unstructured) error {
	gvk := obj.GroupVersionKind()

	// We cannot create ServiceAccounts with secrets because there's currently
	// no need and the merging logic is complex.
	// If you need this, please file an issue.
	if gvk.Group == "" && gvk.Kind == "ServiceAccount" {
		secrets, ok, err := uns.NestedSlice(obj.Object, "secrets")
		if err != nil {
			return err
		}

		if ok && len(secrets) > 0 {
			return errors.Errorf("cannot create ServiceAccount with secrets")
		}
	}

	return nil
}
