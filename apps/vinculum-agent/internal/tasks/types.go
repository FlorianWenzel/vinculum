package tasks

import "time"

// DispatchPayload is the body operator POSTs to /task.
type DispatchPayload struct {
	TaskID    string   `json:"taskId"`
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Spec      TaskSpec `json:"spec"`
	// Kind tags the work item's source resource. Empty or "task" routes
	// status patches to the Task CRD; "message" tells the runner not to
	// patch (Message lifecycle is owned by the operator's reconciler).
	Kind string `json:"kind,omitempty"`
}

const (
	KindTask    = "task"
	KindMessage = "message"
)

type TaskSpec struct {
	AgentRef       string            `json:"agentRef"`
	Prompt         string            `json:"prompt"`
	Fresh          bool              `json:"fresh,omitempty"`
	Workspace      *TaskWorkspace    `json:"workspace,omitempty"`
	Artifacts      *ArtifactSink     `json:"artifacts,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutSeconds int32             `json:"timeoutSeconds,omitempty"`
	Model          string            `json:"model,omitempty"`
	Git            *TaskGit          `json:"git,omitempty"`
}

// TaskGit mirrors the operator's TaskGit shape. See
// apps/operator/api/v1alpha1/task_types.go for field docs.
type TaskGit struct {
	BaseBranch    string `json:"baseBranch,omitempty"`
	HeadBranch    string `json:"headBranch,omitempty"`
	CommitMessage string `json:"commitMessage,omitempty"`
	PRTitle       string `json:"prTitle,omitempty"`
	PRBody        string `json:"prBody,omitempty"`
	SkipPR        bool   `json:"skipPR,omitempty"`
}

type TaskWorkspace struct {
	Mode string `json:"mode,omitempty"`
}

type ArtifactSink struct {
	Type      string       `json:"type,omitempty"`
	SourceDir string       `json:"sourceDir,omitempty"`
	S3        *S3Sink      `json:"s3,omitempty"`
	PVC       *PVCSink     `json:"pvc,omitempty"`
	Webhook   *WebhookSink `json:"webhook,omitempty"`
}

type S3Sink struct {
	Bucket   string `json:"bucket"`
	Prefix   string `json:"prefix,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Region   string `json:"region,omitempty"`
}

type PVCSink struct {
	ClaimName string `json:"claimName"`
	SubPath   string `json:"subPath,omitempty"`
}

type WebhookSink struct {
	URL string `json:"url"`
}

// State is the in-memory mirror of a Task while the pod owns it.
type State struct {
	Payload    DispatchPayload
	Phase      string
	EnqueuedAt time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
	ExitCode   int
	StdoutTail string
	StderrTail string
	Artifacts  []string
	Reason     string
	Message    string
	Logs       *LogBuffer
	LogFile    string
}

const (
	PhaseQueued    = "Queued"
	PhaseRunning   = "Running"
	PhaseSucceeded = "Succeeded"
	PhaseFailed    = "Failed"
	PhaseTimedOut  = "TimedOut"
)
