package client

import "time"

// Unstructured-ish mirrors of operator types. We only decode the fields we render.
type ObjectMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	CreationTimestamp time.Time         `json:"creationTimestamp,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
}

type MCPServer struct {
	Name    string            `json:"name"`
	URL     string            `json:"url,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Enabled bool              `json:"enabled"`
}

type InlineFile struct {
	FileName string `json:"fileName,omitempty"`
	Content  string `json:"content,omitempty"`
}

type AgentSpec struct {
	Image                string            `json:"image,omitempty"`
	Concurrency          int32             `json:"concurrency,omitempty"`
	Model                string            `json:"model,omitempty"`
	InstructionInline    *InlineFile       `json:"instructionInline,omitempty"`
	InstructionMountPath string            `json:"instructionMountPath,omitempty"`
	ProviderSecretRef    *SecretRef        `json:"providerSecretRef,omitempty"`
	MCPServers           []MCPServer       `json:"mcpServers,omitempty"`
	Env                  map[string]string `json:"env,omitempty"`
	Enabled              bool              `json:"enabled"`
	WorkspaceSize        string            `json:"workspaceSize,omitempty"`
}

type SecretRef struct {
	Name string `json:"name"`
}

type AgentStatus struct {
	Phase          string `json:"phase,omitempty"`
	Ready          bool   `json:"ready,omitempty"`
	DeploymentName string `json:"deploymentName,omitempty"`
	ServiceName    string `json:"serviceName,omitempty"`
	PVCName        string `json:"pvcName,omitempty"`
	CurrentTaskRef string `json:"currentTaskRef,omitempty"`
}

type Agent struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   ObjectMeta   `json:"metadata"`
	Spec       AgentSpec    `json:"spec"`
	Status     AgentStatus  `json:"status,omitempty"`
}

type TaskWorkspace struct {
	Mode string `json:"mode,omitempty"`
}

type TaskSpec struct {
	AgentRef       string            `json:"agentRef"`
	Prompt         string            `json:"prompt"`
	Fresh          bool              `json:"fresh,omitempty"`
	Workspace      *TaskWorkspace    `json:"workspace,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutSeconds int32             `json:"timeoutSeconds,omitempty"`
}

type TaskStatus struct {
	Phase          string    `json:"phase,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	Message        string    `json:"message,omitempty"`
	PodName        string    `json:"podName,omitempty"`
	StartTime      time.Time `json:"startTime,omitempty"`
	CompletionTime time.Time `json:"completionTime,omitempty"`
	ExitCode       int       `json:"exitCode,omitempty"`
	StdoutTail     string    `json:"stdoutTail,omitempty"`
	StderrTail     string    `json:"stderrTail,omitempty"`
}

type Task struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       TaskSpec   `json:"spec"`
	Status     TaskStatus `json:"status,omitempty"`
}

type TaskTemplate struct {
	Prompt         string            `json:"prompt"`
	Fresh          bool              `json:"fresh,omitempty"`
	Workspace      *TaskWorkspace    `json:"workspace,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutSeconds int32             `json:"timeoutSeconds,omitempty"`
}

type ScheduleSpec struct {
	AgentRef                string       `json:"agentRef"`
	Schedule                string       `json:"schedule"`
	Timezone                string       `json:"timezone,omitempty"`
	Suspend                 bool         `json:"suspend,omitempty"`
	ConcurrencyPolicy       string       `json:"concurrencyPolicy,omitempty"`
	StartingDeadlineSeconds int64        `json:"startingDeadlineSeconds,omitempty"`
	HistoryLimit            int32        `json:"historyLimit,omitempty"`
	TaskTemplate            TaskTemplate `json:"taskTemplate"`
}

type Schedule struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   ObjectMeta   `json:"metadata"`
	Spec       ScheduleSpec `json:"spec"`
}

// Provider is a vnclm-level abstraction on top of a labeled k8s Secret.
type Provider struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Type      string            `json:"type"`
	Keys      []string          `json:"keys"`
}
