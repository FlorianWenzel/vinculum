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

type InlineProviderAuth struct {
	FileKey string `json:"fileKey,omitempty"`
	Content string `json:"content,omitempty"`
}

type ForgejoUserSpec struct {
	AutoProvision bool   `json:"autoProvision,omitempty"`
	Email         string `json:"email,omitempty"`
	DisplayName   string `json:"displayName,omitempty"`
	Admin         bool   `json:"admin,omitempty"`
}

type DroneSpec struct {
	Role                 string              `json:"role"`
	ForgejoUsername      string              `json:"forgejoUsername"`
	Forgejo              ForgejoUserSpec     `json:"forgejo,omitempty"`
	Image                string              `json:"image"`
	Concurrency          int32               `json:"concurrency"`
	Model                string              `json:"model,omitempty"`
	OpenCodeAgent        string              `json:"opencodeAgent,omitempty"`
	InstructionConfigMap string              `json:"instructionConfigMap,omitempty"`
	InstructionInline    *InlineFile         `json:"instructionInline,omitempty"`
	InstructionMountPath string              `json:"instructionMountPath,omitempty"`
	SSHKeySecretRef      *SecretRef          `json:"sshKeySecretRef,omitempty"`
	ProviderSecretRef    *SecretRef          `json:"providerSecretRef,omitempty"`
	ProviderAuthFileKey  string              `json:"providerAuthFileKey,omitempty"`
	ProviderAuthInline   *InlineProviderAuth `json:"providerAuthInline,omitempty"`
	Env                  map[string]string   `json:"env,omitempty"`
	Enabled              bool                `json:"enabled"`
}

type DroneStatus struct {
	Phase                  string             `json:"phase,omitempty"`
	ActiveTasks            int32              `json:"activeTasks,omitempty"`
	Conditions             []metav1.Condition `json:"conditions,omitempty"`
	LastSeen               *metav1.Time       `json:"lastSeen,omitempty"`
	Assigned               []string           `json:"assignedTaskRuns,omitempty"`
	ForgejoUserID          int64              `json:"forgejoUserID,omitempty"`
	ForgejoReady           bool               `json:"forgejoReady,omitempty"`
	SSHSecretName          string             `json:"sshSecretName,omitempty"`
	SSHPublicKey           string             `json:"sshPublicKey,omitempty"`
	SSHKeyFingerprint      string             `json:"sshKeyFingerprint,omitempty"`
	ForgejoTokenSecretName string             `json:"forgejoTokenSecretName,omitempty"`
}

type Drone struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DroneSpec   `json:"spec,omitempty"`
	Status DroneStatus `json:"status,omitempty"`
}

type DroneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Drone `json:"items"`
}

func (in *Drone) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(Drone)
	in.DeepCopyInto(out)
	return out
}

func (in *DroneList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(DroneList)
	in.DeepCopyInto(out)
	return out
}

func (in *Drone) DeepCopyInto(out *Drone) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *DroneSpec) DeepCopyInto(out *DroneSpec) {
	*out = *in
	if in.SSHKeySecretRef != nil {
		out.SSHKeySecretRef = &SecretRef{Name: in.SSHKeySecretRef.Name}
	}
	if in.ProviderSecretRef != nil {
		out.ProviderSecretRef = &SecretRef{Name: in.ProviderSecretRef.Name}
	}
	if in.InstructionInline != nil {
		out.InstructionInline = &InlineFile{FileName: in.InstructionInline.FileName, Content: in.InstructionInline.Content}
	}
	if in.ProviderAuthInline != nil {
		out.ProviderAuthInline = &InlineProviderAuth{FileKey: in.ProviderAuthInline.FileKey, Content: in.ProviderAuthInline.Content}
	}
	if in.Env != nil {
		out.Env = map[string]string{}
		for k, v := range in.Env {
			out.Env[k] = v
		}
	}
}

func (in *DroneStatus) DeepCopyInto(out *DroneStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
	if in.LastSeen != nil {
		t := *in.LastSeen
		out.LastSeen = &t
	}
	if in.Assigned != nil {
		out.Assigned = append([]string(nil), in.Assigned...)
	}
}

func (in *DroneList) DeepCopyInto(out *DroneList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]Drone, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
