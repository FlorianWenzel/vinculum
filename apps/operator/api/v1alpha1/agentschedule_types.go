package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ConcurrencyPolicy mirrors Kubernetes CronJob semantics.
//   Allow:   concurrent runs allowed (default)
//   Forbid:  skip new run if previous still active
//   Replace: cancel active run, start new
type ConcurrencyPolicy string

const (
	AllowConcurrent   ConcurrencyPolicy = "Allow"
	ForbidConcurrent  ConcurrencyPolicy = "Forbid"
	ReplaceConcurrent ConcurrencyPolicy = "Replace"
)

type AgentScheduleSpec struct {
	AgentRef                string            `json:"agentRef"`
	Schedule                string            `json:"schedule"`
	Timezone                string            `json:"timezone,omitempty"`
	Suspend                 bool              `json:"suspend,omitempty"`
	ConcurrencyPolicy       ConcurrencyPolicy `json:"concurrencyPolicy,omitempty"`
	StartingDeadlineSeconds *int64            `json:"startingDeadlineSeconds,omitempty"`
	HistoryLimit            int32             `json:"historyLimit,omitempty"`
	TaskTemplate            TaskTemplate      `json:"taskTemplate"`
}

type AgentScheduleStatus struct {
	LastScheduleTime *metav1.Time       `json:"lastScheduleTime,omitempty"`
	Active           []string           `json:"active,omitempty"`
	Conditions       []metav1.Condition `json:"conditions,omitempty"`
}

type AgentSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentScheduleSpec   `json:"spec,omitempty"`
	Status AgentScheduleStatus `json:"status,omitempty"`
}

type AgentScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentSchedule `json:"items"`
}

func (in *AgentSchedule) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AgentSchedule)
	in.DeepCopyInto(out)
	return out
}

func (in *AgentScheduleList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AgentScheduleList)
	in.DeepCopyInto(out)
	return out
}

func (in *AgentSchedule) DeepCopyInto(out *AgentSchedule) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *AgentScheduleSpec) DeepCopyInto(out *AgentScheduleSpec) {
	*out = *in
	if in.StartingDeadlineSeconds != nil {
		v := *in.StartingDeadlineSeconds
		out.StartingDeadlineSeconds = &v
	}
	in.TaskTemplate.DeepCopyInto(&out.TaskTemplate)
}

func (in *AgentScheduleStatus) DeepCopyInto(out *AgentScheduleStatus) {
	*out = *in
	if in.LastScheduleTime != nil {
		t := *in.LastScheduleTime
		out.LastScheduleTime = &t
	}
	if in.Active != nil {
		out.Active = append([]string(nil), in.Active...)
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *AgentScheduleList) DeepCopyInto(out *AgentScheduleList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]AgentSchedule, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
