package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type DroneRepositoryAccessSpec struct {
	DroneRef      string `json:"droneRef"`
	RepositoryRef string `json:"repositoryRef"`
	Permission    string `json:"permission"`
}

type DroneRepositoryAccessStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type DroneRepositoryAccess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DroneRepositoryAccessSpec   `json:"spec,omitempty"`
	Status DroneRepositoryAccessStatus `json:"status,omitempty"`
}

type DroneRepositoryAccessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DroneRepositoryAccess `json:"items"`
}

func (in *DroneRepositoryAccess) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(DroneRepositoryAccess)
	in.DeepCopyInto(out)
	return out
}

func (in *DroneRepositoryAccessList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(DroneRepositoryAccessList)
	in.DeepCopyInto(out)
	return out
}

func (in *DroneRepositoryAccess) DeepCopyInto(out *DroneRepositoryAccess) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

func (in *DroneRepositoryAccessStatus) DeepCopyInto(out *DroneRepositoryAccessStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *DroneRepositoryAccessList) DeepCopyInto(out *DroneRepositoryAccessList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]DroneRepositoryAccess, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
