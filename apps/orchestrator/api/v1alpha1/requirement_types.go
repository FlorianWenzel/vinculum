package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type RequirementSpec struct {
	RepositoryRef string `json:"repositoryRef"`
	FilePath      string `json:"filePath"`
}

type RequirementStatus struct {
	Phase             string             `json:"phase,omitempty"`
	ObservedRevision  string             `json:"observedRevision,omitempty"`
	ContentChecksum   string             `json:"contentChecksum,omitempty"`
	ObservedTitle     string             `json:"observedTitle,omitempty"`
	ObservedSlug      string             `json:"observedSlug,omitempty"`
	ObservedStatus    string             `json:"observedStatus,omitempty"`
	ObservedBranch    string             `json:"observedBranch,omitempty"`
	ObservedDependsOn []string           `json:"observedDependsOn,omitempty"`
	TaskRefs          []string           `json:"taskRefs,omitempty"`
	PullRequestURL    string             `json:"pullRequestUrl,omitempty"`
	PullRequestNumber int64              `json:"pullRequestNumber,omitempty"`
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
}

type Requirement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RequirementSpec   `json:"spec,omitempty"`
	Status RequirementStatus `json:"status,omitempty"`
}

type RequirementList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Requirement `json:"items"`
}

func (in *Requirement) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(Requirement)
	in.DeepCopyInto(out)
	return out
}

func (in *RequirementList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(RequirementList)
	in.DeepCopyInto(out)
	return out
}

func (in *Requirement) DeepCopyInto(out *Requirement) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *RequirementSpec) DeepCopyInto(out *RequirementSpec) {
	*out = *in
}

func (in *RequirementStatus) DeepCopyInto(out *RequirementStatus) {
	*out = *in
	if in.ObservedDependsOn != nil {
		out.ObservedDependsOn = append([]string(nil), in.ObservedDependsOn...)
	}
	if in.TaskRefs != nil {
		out.TaskRefs = append([]string(nil), in.TaskRefs...)
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *RequirementList) DeepCopyInto(out *RequirementList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]Requirement, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
