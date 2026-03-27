package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type TaskSpec struct {
	RequirementRef         string   `json:"requirementRef,omitempty"`
	RequirementFilePath    string   `json:"requirementFilePath,omitempty"`
	DroneRef               string   `json:"droneRef,omitempty"`
	Role                   string   `json:"role,omitempty"`
	RepositoryRef          string   `json:"repositoryRef,omitempty"`
	RepoURL                string   `json:"repoUrl,omitempty"`
	BaseBranch             string   `json:"baseBranch,omitempty"`
	WorkingBranch          string   `json:"workingBranch,omitempty"`
	WorkspacePath          string   `json:"workspacePath,omitempty"`
	StartupContractVersion string   `json:"startupContractVersion,omitempty"`
	DependsOn              []string `json:"dependsOn,omitempty"`
	Prompt                 string   `json:"prompt"`
	Instructions           bool     `json:"instructions,omitempty"`
}

type TaskStatus struct {
	Phase               string             `json:"phase,omitempty"`
	AssignedDrone       string             `json:"assignedDrone,omitempty"`
	JobName             string             `json:"jobName,omitempty"`
	VerificationPhase   string             `json:"verificationPhase,omitempty"`
	VerificationJobName string             `json:"verificationJobName,omitempty"`
	VerificationDrone   string             `json:"verificationDrone,omitempty"`
	ReviewRef           string             `json:"reviewRef,omitempty"`
	PullRequestURL      string             `json:"pullRequestUrl,omitempty"`
	PullRequestNumber   int64              `json:"pullRequestNumber,omitempty"`
	Branch              string             `json:"branch,omitempty"`
	Summary             string             `json:"summary,omitempty"`
	StartedAt           *metav1.Time       `json:"startedAt,omitempty"`
	FinishedAt          *metav1.Time       `json:"finishedAt,omitempty"`
	Conditions          []metav1.Condition `json:"conditions,omitempty"`
}

type Task struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TaskSpec   `json:"spec,omitempty"`
	Status TaskStatus `json:"status,omitempty"`
}

type TaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Task `json:"items"`
}

type TaskRun = Task
type TaskRunList = TaskList
type TaskRunSpec = TaskSpec
type TaskRunStatus = TaskStatus

func (in *Task) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(Task)
	in.DeepCopyInto(out)
	return out
}

func (in *TaskList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(TaskList)
	in.DeepCopyInto(out)
	return out
}

func (in *Task) DeepCopyInto(out *Task) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *TaskSpec) DeepCopyInto(out *TaskSpec) {
	*out = *in
	if in.DependsOn != nil {
		out.DependsOn = append([]string(nil), in.DependsOn...)
	}
}

func (in *TaskStatus) DeepCopyInto(out *TaskStatus) {
	*out = *in
	if in.StartedAt != nil {
		t := *in.StartedAt
		out.StartedAt = &t
	}
	if in.FinishedAt != nil {
		t := *in.FinishedAt
		out.FinishedAt = &t
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *TaskList) DeepCopyInto(out *TaskList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]Task, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
