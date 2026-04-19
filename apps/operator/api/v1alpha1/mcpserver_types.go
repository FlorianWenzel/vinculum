package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// MCPServerSpec describes a reusable MCP server definition.
// Exactly one of URL (http transport) or Command (stdio transport) must be set.
type MCPServerSpec struct {
	URL       string            `json:"url,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	SecretRef *SecretRef        `json:"secretRef,omitempty"`
	Enabled   bool              `json:"enabled"`
	Timeout   int32             `json:"timeout,omitempty"`
}

type MCPServerStatus struct {
	Conditions    []metav1.Condition `json:"conditions,omitempty"`
	ReferencedBy  []string           `json:"referencedBy,omitempty"`
}

type MCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPServerSpec   `json:"spec,omitempty"`
	Status MCPServerStatus `json:"status,omitempty"`
}

type MCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPServer `json:"items"`
}

func (in *MCPServer) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(MCPServer)
	in.DeepCopyInto(out)
	return out
}

func (in *MCPServerList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(MCPServerList)
	in.DeepCopyInto(out)
	return out
}

func (in *MCPServer) DeepCopyInto(out *MCPServer) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *MCPServerSpec) DeepCopyInto(out *MCPServerSpec) {
	*out = *in
	if in.Args != nil {
		out.Args = append([]string(nil), in.Args...)
	}
	if in.Env != nil {
		out.Env = map[string]string{}
		for k, v := range in.Env {
			out.Env[k] = v
		}
	}
	if in.SecretRef != nil {
		out.SecretRef = &SecretRef{Name: in.SecretRef.Name}
	}
}

func (in *MCPServerStatus) DeepCopyInto(out *MCPServerStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
	if in.ReferencedBy != nil {
		out.ReferencedBy = append([]string(nil), in.ReferencedBy...)
	}
}

func (in *MCPServerList) DeepCopyInto(out *MCPServerList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]MCPServer, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
