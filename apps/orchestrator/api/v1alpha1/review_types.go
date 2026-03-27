package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type ReviewFinding struct {
	Category string `json:"category,omitempty"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

type ReviewSpec struct {
	RequirementRef      string          `json:"requirementRef,omitempty"`
	RequirementFilePath string          `json:"requirementFilePath,omitempty"`
	TaskRef             string          `json:"taskRef"`
	RepositoryRef       string          `json:"repositoryRef,omitempty"`
	ReviewerDroneRef    string          `json:"reviewerDroneRef,omitempty"`
	PullRequestURL      string          `json:"pullRequestUrl,omitempty"`
	Automated           bool            `json:"automated,omitempty"`
	Verdict             string          `json:"verdict,omitempty"`
	Summary             string          `json:"summary,omitempty"`
	Findings            []ReviewFinding `json:"findings,omitempty"`
}

type ReviewStatus struct {
	Phase         string             `json:"phase,omitempty"`
	JobName       string             `json:"jobName,omitempty"`
	ReviewerDrone string             `json:"reviewerDrone,omitempty"`
	Conditions    []metav1.Condition `json:"conditions,omitempty"`
}

type Review struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReviewSpec   `json:"spec,omitempty"`
	Status ReviewStatus `json:"status,omitempty"`
}

type ReviewList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Review `json:"items"`
}

func (in *Review) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(Review)
	in.DeepCopyInto(out)
	return out
}

func (in *ReviewList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(ReviewList)
	in.DeepCopyInto(out)
	return out
}

func (in *Review) DeepCopyInto(out *Review) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *ReviewSpec) DeepCopyInto(out *ReviewSpec) {
	*out = *in
	if in.Findings != nil {
		out.Findings = append([]ReviewFinding(nil), in.Findings...)
	}
}

func (in *ReviewStatus) DeepCopyInto(out *ReviewStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *ReviewList) DeepCopyInto(out *ReviewList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]Review, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
