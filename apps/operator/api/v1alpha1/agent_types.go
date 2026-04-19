package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type SecretRef struct {
	Name string `json:"name"`
}

type InlineFile struct {
	FileName string `json:"fileName,omitempty"`
	Content  string `json:"content,omitempty"`
}

type MCPServer struct {
	Name      string            `json:"name"`
	URL       string            `json:"url,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	SecretRef *SecretRef        `json:"secretRef,omitempty"`
	Enabled   bool              `json:"enabled,omitempty"`
}

type AgentSpec struct {
	Image                string      `json:"image"`
	Concurrency          int32       `json:"concurrency,omitempty"`
	Model                string      `json:"model,omitempty"`
	InstructionConfigMap string      `json:"instructionConfigMap,omitempty"`
	InstructionInline    *InlineFile `json:"instructionInline,omitempty"`
	InstructionMountPath string      `json:"instructionMountPath,omitempty"`
	// ProviderSecretRef names a Secret whose keys are injected as env vars
	// (envFrom) into the agent pod. Use standard crush env var names:
	// ANTHROPIC_API_KEY, OPENAI_API_KEY, AZURE_OPENAI_API_KEY, GEMINI_API_KEY, etc.
	ProviderSecretRef     *SecretRef        `json:"providerSecretRef,omitempty"`
	MCPServers            []MCPServer       `json:"mcpServers,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	Enabled               bool              `json:"enabled"`
	WorkspaceSize         string            `json:"workspaceSize,omitempty"`
	WorkspaceStorageClass *string           `json:"workspaceStorageClass,omitempty"`
}

type AgentStatus struct {
	Phase           string             `json:"phase,omitempty"`
	ActiveRuns      int32              `json:"activeRuns,omitempty"`
	Conditions      []metav1.Condition `json:"conditions,omitempty"`
	LastSeen        *metav1.Time       `json:"lastSeen,omitempty"`
	AssignedRuns    []string           `json:"assignedRuns,omitempty"`
	DeploymentName  string             `json:"deploymentName,omitempty"`
	ServiceName     string             `json:"serviceName,omitempty"`
	PVCName         string             `json:"pvcName,omitempty"`
	Ready           bool               `json:"ready,omitempty"`
	ReadyReplicas   int32              `json:"readyReplicas,omitempty"`
	CurrentTaskRef  string             `json:"currentTaskRef,omitempty"`
	SessionID       string             `json:"sessionID,omitempty"`
}

type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Agent `json:"items"`
}

func (in *Agent) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(Agent)
	in.DeepCopyInto(out)
	return out
}

func (in *AgentList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AgentList)
	in.DeepCopyInto(out)
	return out
}

func (in *Agent) DeepCopyInto(out *Agent) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *AgentSpec) DeepCopyInto(out *AgentSpec) {
	*out = *in
	if in.ProviderSecretRef != nil {
		out.ProviderSecretRef = &SecretRef{Name: in.ProviderSecretRef.Name}
	}
	if in.InstructionInline != nil {
		out.InstructionInline = &InlineFile{FileName: in.InstructionInline.FileName, Content: in.InstructionInline.Content}
	}
	if in.MCPServers != nil {
		out.MCPServers = make([]MCPServer, len(in.MCPServers))
		for i := range in.MCPServers {
			out.MCPServers[i] = in.MCPServers[i]
			if in.MCPServers[i].Args != nil {
				out.MCPServers[i].Args = append([]string(nil), in.MCPServers[i].Args...)
			}
			if in.MCPServers[i].Env != nil {
				out.MCPServers[i].Env = map[string]string{}
				for k, v := range in.MCPServers[i].Env {
					out.MCPServers[i].Env[k] = v
				}
			}
			if in.MCPServers[i].SecretRef != nil {
				out.MCPServers[i].SecretRef = &SecretRef{Name: in.MCPServers[i].SecretRef.Name}
			}
		}
	}
	if in.Env != nil {
		out.Env = map[string]string{}
		for k, v := range in.Env {
			out.Env[k] = v
		}
	}
	if in.WorkspaceStorageClass != nil {
		v := *in.WorkspaceStorageClass
		out.WorkspaceStorageClass = &v
	}
}

func (in *AgentStatus) DeepCopyInto(out *AgentStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
	if in.LastSeen != nil {
		t := *in.LastSeen
		out.LastSeen = &t
	}
	if in.AssignedRuns != nil {
		out.AssignedRuns = append([]string(nil), in.AssignedRuns...)
	}
}

func (in *AgentList) DeepCopyInto(out *AgentList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]Agent, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
