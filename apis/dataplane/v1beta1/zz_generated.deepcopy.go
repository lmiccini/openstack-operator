//go:build !ignore_autogenerated

/*
Copyright 2022.

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

// Code generated by controller-gen. DO NOT EDIT.

package v1beta1

import (
	"encoding/json"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	networkv1beta1 "github.com/openstack-k8s-operators/infra-operator/apis/network/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AnsibleEESpec) DeepCopyInto(out *AnsibleEESpec) {
	*out = *in
	if in.ExtraMounts != nil {
		in, out := &in.ExtraMounts, &out.ExtraMounts
		*out = make([]storage.VolMounts, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Env != nil {
		in, out := &in.Env, &out.Env
		*out = make([]v1.EnvVar, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.ExtraVars != nil {
		in, out := &in.ExtraVars, &out.ExtraVars
		*out = make(map[string]json.RawMessage, len(*in))
		for key, val := range *in {
			var outVal []byte
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = make(json.RawMessage, len(*in))
				copy(*out, *in)
			}
			(*out)[key] = outVal
		}
	}
	if in.DNSConfig != nil {
		in, out := &in.DNSConfig, &out.DNSConfig
		*out = new(v1.PodDNSConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.NetworkAttachments != nil {
		in, out := &in.NetworkAttachments, &out.NetworkAttachments
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AnsibleEESpec.
func (in *AnsibleEESpec) DeepCopy() *AnsibleEESpec {
	if in == nil {
		return nil
	}
	out := new(AnsibleEESpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AnsibleOpts) DeepCopyInto(out *AnsibleOpts) {
	*out = *in
	if in.AnsibleVars != nil {
		in, out := &in.AnsibleVars, &out.AnsibleVars
		*out = make(map[string]json.RawMessage, len(*in))
		for key, val := range *in {
			var outVal []byte
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = make(json.RawMessage, len(*in))
				copy(*out, *in)
			}
			(*out)[key] = outVal
		}
	}
	if in.AnsibleVarsFrom != nil {
		in, out := &in.AnsibleVarsFrom, &out.AnsibleVarsFrom
		*out = make([]DataSource, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AnsibleOpts.
func (in *AnsibleOpts) DeepCopy() *AnsibleOpts {
	if in == nil {
		return nil
	}
	out := new(AnsibleOpts)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ConfigMapEnvSource) DeepCopyInto(out *ConfigMapEnvSource) {
	*out = *in
	out.LocalObjectReference = in.LocalObjectReference
	if in.Optional != nil {
		in, out := &in.Optional, &out.Optional
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ConfigMapEnvSource.
func (in *ConfigMapEnvSource) DeepCopy() *ConfigMapEnvSource {
	if in == nil {
		return nil
	}
	out := new(ConfigMapEnvSource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DataSource) DeepCopyInto(out *DataSource) {
	*out = *in
	if in.ConfigMapRef != nil {
		in, out := &in.ConfigMapRef, &out.ConfigMapRef
		*out = new(ConfigMapEnvSource)
		(*in).DeepCopyInto(*out)
	}
	if in.SecretRef != nil {
		in, out := &in.SecretRef, &out.SecretRef
		*out = new(SecretEnvSource)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DataSource.
func (in *DataSource) DeepCopy() *DataSource {
	if in == nil {
		return nil
	}
	out := new(DataSource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LocalObjectReference) DeepCopyInto(out *LocalObjectReference) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LocalObjectReference.
func (in *LocalObjectReference) DeepCopy() *LocalObjectReference {
	if in == nil {
		return nil
	}
	out := new(LocalObjectReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeSection) DeepCopyInto(out *NodeSection) {
	*out = *in
	if in.Networks != nil {
		in, out := &in.Networks, &out.Networks
		*out = make([]networkv1beta1.IPSetNetwork, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.BmhLabelSelector != nil {
		in, out := &in.BmhLabelSelector, &out.BmhLabelSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.UserData != nil {
		in, out := &in.UserData, &out.UserData
		*out = new(v1.SecretReference)
		**out = **in
	}
	if in.NetworkData != nil {
		in, out := &in.NetworkData, &out.NetworkData
		*out = new(v1.SecretReference)
		**out = **in
	}
	in.Ansible.DeepCopyInto(&out.Ansible)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeSection.
func (in *NodeSection) DeepCopy() *NodeSection {
	if in == nil {
		return nil
	}
	out := new(NodeSection)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeTemplate) DeepCopyInto(out *NodeTemplate) {
	*out = *in
	if in.ExtraMounts != nil {
		in, out := &in.ExtraMounts, &out.ExtraMounts
		*out = make([]storage.VolMounts, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Networks != nil {
		in, out := &in.Networks, &out.Networks
		*out = make([]networkv1beta1.IPSetNetwork, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.UserData != nil {
		in, out := &in.UserData, &out.UserData
		*out = new(v1.SecretReference)
		**out = **in
	}
	if in.NetworkData != nil {
		in, out := &in.NetworkData, &out.NetworkData
		*out = new(v1.SecretReference)
		**out = **in
	}
	in.Ansible.DeepCopyInto(&out.Ansible)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeTemplate.
func (in *NodeTemplate) DeepCopy() *NodeTemplate {
	if in == nil {
		return nil
	}
	out := new(NodeTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenStackDataPlaneDeployment) DeepCopyInto(out *OpenStackDataPlaneDeployment) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenStackDataPlaneDeployment.
func (in *OpenStackDataPlaneDeployment) DeepCopy() *OpenStackDataPlaneDeployment {
	if in == nil {
		return nil
	}
	out := new(OpenStackDataPlaneDeployment)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *OpenStackDataPlaneDeployment) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenStackDataPlaneDeploymentList) DeepCopyInto(out *OpenStackDataPlaneDeploymentList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]OpenStackDataPlaneDeployment, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenStackDataPlaneDeploymentList.
func (in *OpenStackDataPlaneDeploymentList) DeepCopy() *OpenStackDataPlaneDeploymentList {
	if in == nil {
		return nil
	}
	out := new(OpenStackDataPlaneDeploymentList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *OpenStackDataPlaneDeploymentList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenStackDataPlaneDeploymentSpec) DeepCopyInto(out *OpenStackDataPlaneDeploymentSpec) {
	*out = *in
	if in.NodeSets != nil {
		in, out := &in.NodeSets, &out.NodeSets
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.BackoffLimit != nil {
		in, out := &in.BackoffLimit, &out.BackoffLimit
		*out = new(int32)
		**out = **in
	}
	if in.AnsibleExtraVars != nil {
		in, out := &in.AnsibleExtraVars, &out.AnsibleExtraVars
		*out = make(map[string]json.RawMessage, len(*in))
		for key, val := range *in {
			var outVal []byte
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = make(json.RawMessage, len(*in))
				copy(*out, *in)
			}
			(*out)[key] = outVal
		}
	}
	if in.ServicesOverride != nil {
		in, out := &in.ServicesOverride, &out.ServicesOverride
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.AnsibleJobNodeSelector != nil {
		in, out := &in.AnsibleJobNodeSelector, &out.AnsibleJobNodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenStackDataPlaneDeploymentSpec.
func (in *OpenStackDataPlaneDeploymentSpec) DeepCopy() *OpenStackDataPlaneDeploymentSpec {
	if in == nil {
		return nil
	}
	out := new(OpenStackDataPlaneDeploymentSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenStackDataPlaneDeploymentStatus) DeepCopyInto(out *OpenStackDataPlaneDeploymentStatus) {
	*out = *in
	if in.NodeSetConditions != nil {
		in, out := &in.NodeSetConditions, &out.NodeSetConditions
		*out = make(map[string]condition.Conditions, len(*in))
		for key, val := range *in {
			var outVal []condition.Condition
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = make(condition.Conditions, len(*in))
				for i := range *in {
					(*in)[i].DeepCopyInto(&(*out)[i])
				}
			}
			(*out)[key] = outVal
		}
	}
	if in.AnsibleEEHashes != nil {
		in, out := &in.AnsibleEEHashes, &out.AnsibleEEHashes
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.ConfigMapHashes != nil {
		in, out := &in.ConfigMapHashes, &out.ConfigMapHashes
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.SecretHashes != nil {
		in, out := &in.SecretHashes, &out.SecretHashes
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.NodeSetHashes != nil {
		in, out := &in.NodeSetHashes, &out.NodeSetHashes
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.BmhRefHashes != nil {
		in, out := &in.BmhRefHashes, &out.BmhRefHashes
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.ContainerImages != nil {
		in, out := &in.ContainerImages, &out.ContainerImages
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(condition.Conditions, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenStackDataPlaneDeploymentStatus.
func (in *OpenStackDataPlaneDeploymentStatus) DeepCopy() *OpenStackDataPlaneDeploymentStatus {
	if in == nil {
		return nil
	}
	out := new(OpenStackDataPlaneDeploymentStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenStackDataPlaneNodeSet) DeepCopyInto(out *OpenStackDataPlaneNodeSet) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenStackDataPlaneNodeSet.
func (in *OpenStackDataPlaneNodeSet) DeepCopy() *OpenStackDataPlaneNodeSet {
	if in == nil {
		return nil
	}
	out := new(OpenStackDataPlaneNodeSet)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *OpenStackDataPlaneNodeSet) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenStackDataPlaneNodeSetList) DeepCopyInto(out *OpenStackDataPlaneNodeSetList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]OpenStackDataPlaneNodeSet, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenStackDataPlaneNodeSetList.
func (in *OpenStackDataPlaneNodeSetList) DeepCopy() *OpenStackDataPlaneNodeSetList {
	if in == nil {
		return nil
	}
	out := new(OpenStackDataPlaneNodeSetList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *OpenStackDataPlaneNodeSetList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenStackDataPlaneNodeSetSpec) DeepCopyInto(out *OpenStackDataPlaneNodeSetSpec) {
	*out = *in
	in.BaremetalSetTemplate.DeepCopyInto(&out.BaremetalSetTemplate)
	in.NodeTemplate.DeepCopyInto(&out.NodeTemplate)
	if in.Nodes != nil {
		in, out := &in.Nodes, &out.Nodes
		*out = make(map[string]NodeSection, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	if in.Env != nil {
		in, out := &in.Env, &out.Env
		*out = make([]v1.EnvVar, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.NetworkAttachments != nil {
		in, out := &in.NetworkAttachments, &out.NetworkAttachments
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Services != nil {
		in, out := &in.Services, &out.Services
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Tags != nil {
		in, out := &in.Tags, &out.Tags
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenStackDataPlaneNodeSetSpec.
func (in *OpenStackDataPlaneNodeSetSpec) DeepCopy() *OpenStackDataPlaneNodeSetSpec {
	if in == nil {
		return nil
	}
	out := new(OpenStackDataPlaneNodeSetSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenStackDataPlaneNodeSetStatus) DeepCopyInto(out *OpenStackDataPlaneNodeSetStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(condition.Conditions, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.DeploymentStatuses != nil {
		in, out := &in.DeploymentStatuses, &out.DeploymentStatuses
		*out = make(map[string]condition.Conditions, len(*in))
		for key, val := range *in {
			var outVal []condition.Condition
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = make(condition.Conditions, len(*in))
				for i := range *in {
					(*in)[i].DeepCopyInto(&(*out)[i])
				}
			}
			(*out)[key] = outVal
		}
	}
	if in.AllHostnames != nil {
		in, out := &in.AllHostnames, &out.AllHostnames
		*out = make(map[string]map[networkv1beta1.NetNameStr]string, len(*in))
		for key, val := range *in {
			var outVal map[networkv1beta1.NetNameStr]string
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = make(map[networkv1beta1.NetNameStr]string, len(*in))
				for key, val := range *in {
					(*out)[key] = val
				}
			}
			(*out)[key] = outVal
		}
	}
	if in.AllIPs != nil {
		in, out := &in.AllIPs, &out.AllIPs
		*out = make(map[string]map[networkv1beta1.NetNameStr]string, len(*in))
		for key, val := range *in {
			var outVal map[networkv1beta1.NetNameStr]string
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = make(map[networkv1beta1.NetNameStr]string, len(*in))
				for key, val := range *in {
					(*out)[key] = val
				}
			}
			(*out)[key] = outVal
		}
	}
	if in.ConfigMapHashes != nil {
		in, out := &in.ConfigMapHashes, &out.ConfigMapHashes
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.SecretHashes != nil {
		in, out := &in.SecretHashes, &out.SecretHashes
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.DNSClusterAddresses != nil {
		in, out := &in.DNSClusterAddresses, &out.DNSClusterAddresses
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ContainerImages != nil {
		in, out := &in.ContainerImages, &out.ContainerImages
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenStackDataPlaneNodeSetStatus.
func (in *OpenStackDataPlaneNodeSetStatus) DeepCopy() *OpenStackDataPlaneNodeSetStatus {
	if in == nil {
		return nil
	}
	out := new(OpenStackDataPlaneNodeSetStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenStackDataPlaneService) DeepCopyInto(out *OpenStackDataPlaneService) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenStackDataPlaneService.
func (in *OpenStackDataPlaneService) DeepCopy() *OpenStackDataPlaneService {
	if in == nil {
		return nil
	}
	out := new(OpenStackDataPlaneService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *OpenStackDataPlaneService) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenStackDataPlaneServiceList) DeepCopyInto(out *OpenStackDataPlaneServiceList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]OpenStackDataPlaneService, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenStackDataPlaneServiceList.
func (in *OpenStackDataPlaneServiceList) DeepCopy() *OpenStackDataPlaneServiceList {
	if in == nil {
		return nil
	}
	out := new(OpenStackDataPlaneServiceList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *OpenStackDataPlaneServiceList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenStackDataPlaneServiceSpec) DeepCopyInto(out *OpenStackDataPlaneServiceSpec) {
	*out = *in
	if in.DataSources != nil {
		in, out := &in.DataSources, &out.DataSources
		*out = make([]DataSource, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.TLSCerts != nil {
		in, out := &in.TLSCerts, &out.TLSCerts
		*out = make(map[string]OpenstackDataPlaneServiceCert, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	if in.ContainerImageFields != nil {
		in, out := &in.ContainerImageFields, &out.ContainerImageFields
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenStackDataPlaneServiceSpec.
func (in *OpenStackDataPlaneServiceSpec) DeepCopy() *OpenStackDataPlaneServiceSpec {
	if in == nil {
		return nil
	}
	out := new(OpenStackDataPlaneServiceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenStackDataPlaneServiceStatus) DeepCopyInto(out *OpenStackDataPlaneServiceStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(condition.Conditions, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenStackDataPlaneServiceStatus.
func (in *OpenStackDataPlaneServiceStatus) DeepCopy() *OpenStackDataPlaneServiceStatus {
	if in == nil {
		return nil
	}
	out := new(OpenStackDataPlaneServiceStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenstackDataPlaneServiceCert) DeepCopyInto(out *OpenstackDataPlaneServiceCert) {
	*out = *in
	if in.Contents != nil {
		in, out := &in.Contents, &out.Contents
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Networks != nil {
		in, out := &in.Networks, &out.Networks
		*out = make([]networkv1beta1.NetNameStr, len(*in))
		copy(*out, *in)
	}
	if in.KeyUsages != nil {
		in, out := &in.KeyUsages, &out.KeyUsages
		*out = make([]certmanagerv1.KeyUsage, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenstackDataPlaneServiceCert.
func (in *OpenstackDataPlaneServiceCert) DeepCopy() *OpenstackDataPlaneServiceCert {
	if in == nil {
		return nil
	}
	out := new(OpenstackDataPlaneServiceCert)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SecretEnvSource) DeepCopyInto(out *SecretEnvSource) {
	*out = *in
	out.LocalObjectReference = in.LocalObjectReference
	if in.Optional != nil {
		in, out := &in.Optional, &out.Optional
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SecretEnvSource.
func (in *SecretEnvSource) DeepCopy() *SecretEnvSource {
	if in == nil {
		return nil
	}
	out := new(SecretEnvSource)
	in.DeepCopyInto(out)
	return out
}
