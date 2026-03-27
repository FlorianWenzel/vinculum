package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type RepositoryAccess struct {
	DroneRef   string `json:"droneRef"`
	Permission string `json:"permission"`
}

type RepositorySpec struct {
	Owner                   string `json:"owner"`
	Name                    string `json:"name"`
	Description             string `json:"description,omitempty"`
	Private                 bool   `json:"private,omitempty"`
	AutoInit                bool   `json:"autoInit,omitempty"`
	DefaultBranch           string `json:"defaultBranch,omitempty"`
	RequirementsPath        string `json:"requirementsPath,omitempty"`
	DefaultBaseBranch       string `json:"defaultBaseBranch,omitempty"`
	RequirementBranchPrefix string `json:"requirementBranchPrefix,omitempty"`
	RequirementsSource      string `json:"requirementsSource,omitempty"`
}

type RepositoryStatus struct {
	Phase        string             `json:"phase,omitempty"`
	HTTPURL      string             `json:"httpUrl,omitempty"`
	SSHURL       string             `json:"sshUrl,omitempty"`
	WebhookURL   string             `json:"webhookUrl,omitempty"`
	WebhookReady bool               `json:"webhookReady,omitempty"`
	Conditions   []metav1.Condition `json:"conditions,omitempty"`
}

type Repository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RepositorySpec   `json:"spec,omitempty"`
	Status RepositoryStatus `json:"status,omitempty"`
}

type RepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Repository `json:"items"`
}

type ForgejoRepository = Repository
type ForgejoRepositoryList = RepositoryList
type ForgejoRepositorySpec = RepositorySpec
type ForgejoRepositoryStatus = RepositoryStatus

func (in *Repository) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(Repository)
	in.DeepCopyInto(out)
	return out
}

func (in *RepositoryList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(RepositoryList)
	in.DeepCopyInto(out)
	return out
}

func (in *Repository) DeepCopyInto(out *Repository) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *RepositorySpec) DeepCopyInto(out *RepositorySpec) {
	*out = *in
}

func (in *RepositoryStatus) DeepCopyInto(out *RepositoryStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *RepositoryList) DeepCopyInto(out *RepositoryList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]Repository, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
