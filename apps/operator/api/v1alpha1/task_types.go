package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TaskWorkspaceMode controls how the Task sees the Agent's shared workspace.
//
//	shared    (default): run in /workspace, file edits persist across Tasks.
//	ephemeral           : run in /workspace/task-<id>, cleaned up after.
type TaskWorkspace struct {
	Mode string `json:"mode,omitempty"`
}

// TaskSpec is the unit of work submitted to an Agent pod.
type TaskSpec struct {
	AgentRef       string            `json:"agentRef"`
	Prompt         string            `json:"prompt"`
	Fresh          bool              `json:"fresh,omitempty"`
	Workspace      *TaskWorkspace    `json:"workspace,omitempty"`
	Artifacts      *ArtifactSink     `json:"artifacts,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutSeconds int32             `json:"timeoutSeconds,omitempty"`
}

// TaskTemplate is TaskSpec without agentRef (derived from schedule owner).
type TaskTemplate struct {
	Prompt         string            `json:"prompt"`
	Fresh          bool              `json:"fresh,omitempty"`
	Workspace      *TaskWorkspace    `json:"workspace,omitempty"`
	Artifacts      *ArtifactSink     `json:"artifacts,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutSeconds int32             `json:"timeoutSeconds,omitempty"`
}

type TaskStatus struct {
	Phase              string             `json:"phase,omitempty"`
	Reason             string             `json:"reason,omitempty"`
	Message            string             `json:"message,omitempty"`
	PodName            string             `json:"podName,omitempty"`
	SessionID          string             `json:"sessionID,omitempty"`
	StartTime          *metav1.Time       `json:"startTime,omitempty"`
	CompletionTime     *metav1.Time       `json:"completionTime,omitempty"`
	ExitCode           int32              `json:"exitCode,omitempty"`
	StdoutTail         string             `json:"stdoutTail,omitempty"`
	StderrTail         string             `json:"stderrTail,omitempty"`
	ArtifactURLs       []string           `json:"artifactURLs,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
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
	if in.Workspace != nil {
		cp := *in.Workspace
		out.Workspace = &cp
	}
	if in.Artifacts != nil {
		cp := ArtifactSink{}
		in.Artifacts.DeepCopyInto(&cp)
		out.Artifacts = &cp
	}
	if in.Env != nil {
		out.Env = map[string]string{}
		for k, v := range in.Env {
			out.Env[k] = v
		}
	}
}

func (in *TaskTemplate) DeepCopyInto(out *TaskTemplate) {
	*out = *in
	if in.Workspace != nil {
		cp := *in.Workspace
		out.Workspace = &cp
	}
	if in.Artifacts != nil {
		cp := ArtifactSink{}
		in.Artifacts.DeepCopyInto(&cp)
		out.Artifacts = &cp
	}
	if in.Env != nil {
		out.Env = map[string]string{}
		for k, v := range in.Env {
			out.Env[k] = v
		}
	}
}

// ToSpec builds a TaskSpec from a TaskTemplate with the given agentRef.
func (in *TaskTemplate) ToSpec(agentRef string) TaskSpec {
	spec := TaskSpec{
		AgentRef:       agentRef,
		Prompt:         in.Prompt,
		Fresh:          in.Fresh,
		TimeoutSeconds: in.TimeoutSeconds,
	}
	if in.Workspace != nil {
		cp := *in.Workspace
		spec.Workspace = &cp
	}
	if in.Artifacts != nil {
		cp := ArtifactSink{}
		in.Artifacts.DeepCopyInto(&cp)
		spec.Artifacts = &cp
	}
	if in.Env != nil {
		spec.Env = map[string]string{}
		for k, v := range in.Env {
			spec.Env[k] = v
		}
	}
	return spec
}

func (in *TaskStatus) DeepCopyInto(out *TaskStatus) {
	*out = *in
	if in.StartTime != nil {
		t := *in.StartTime
		out.StartTime = &t
	}
	if in.CompletionTime != nil {
		t := *in.CompletionTime
		out.CompletionTime = &t
	}
	if in.ArtifactURLs != nil {
		out.ArtifactURLs = append([]string(nil), in.ArtifactURLs...)
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

// Task phases.
const (
	TaskPhasePending     = "Pending"
	TaskPhaseDispatching = "Dispatching"
	TaskPhaseRunning     = "Running"
	TaskPhaseSucceeded   = "Succeeded"
	TaskPhaseFailed      = "Failed"
	TaskPhaseTimedOut    = "TimedOut"
)

func IsTaskTerminal(phase string) bool {
	switch phase {
	case TaskPhaseSucceeded, TaskPhaseFailed, TaskPhaseTimedOut:
		return true
	}
	return false
}
