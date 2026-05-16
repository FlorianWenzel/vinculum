package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// WebhookTriggerSpec declares an event-driven Task creator. When a webhook
// arrives at the operator's webhook endpoint, the operator looks up all
// active triggers, picks the ones whose source/events/filter match, and
// stamps a Task per match from spec.taskTemplate.
type WebhookTriggerSpec struct {
	// Source is the webhook provider. Only "github" is supported in v1.
	Source string `json:"source"`
	// Events lists the webhook event types this trigger reacts to.
	// For github: "push", "pull_request", "pull_request.opened",
	// "pull_request.synchronize", "issue_comment", "check_run.completed".
	// Subtype after a dot is matched against the payload's "action" field.
	Events []string `json:"events"`
	// Filter restricts which payloads match. All set fields must match.
	// Empty fields mean "match anything".
	Filter WebhookFilter `json:"filter,omitempty"`
	// SecretRef names a Secret holding key "secret" — the HMAC-SHA256
	// shared secret GitHub signs requests with. Required.
	SecretRef SecretRef `json:"secretRef"`
	// TaskTemplate is the prototype for the Task that gets stamped on each
	// matched event. The template's `agentRef` is required; other fields
	// are passed through. Template vars `${event.repo}`, `${event.ref}`,
	// `${event.sha}`, `${event.pr.number}`, `${event.pr.title}` are
	// substituted in `prompt`, `commitMessage`, `prTitle`, `prBody`,
	// `headBranch`, `baseBranch` before the Task is created.
	TaskTemplate TaskTemplate `json:"taskTemplate"`
	// AgentRef is the Agent every stamped Task targets.
	AgentRef string `json:"agentRef"`
	// Suspend stops the trigger from firing without deleting it.
	Suspend bool `json:"suspend,omitempty"`
}

// WebhookFilter narrows which payloads a trigger reacts to. All set fields
// must match (AND). Globs are not supported in v1 — exact-match strings.
type WebhookFilter struct {
	// Repo is the owner/repo full name from the webhook payload (e.g.
	// "acme/api"). Empty means any.
	Repo string `json:"repo,omitempty"`
	// Branch matches against `refs/heads/<branch>` (for push) or
	// `pull_request.base.ref` (for pull_request). Empty means any.
	Branch string `json:"branch,omitempty"`
}

// WebhookTriggerStatus tracks recent invocations.
type WebhookTriggerStatus struct {
	// LastFired is the time the trigger last produced a Task.
	LastFired *metav1.Time `json:"lastFired,omitempty"`
	// FireCount is the lifetime number of Tasks this trigger has stamped.
	FireCount int64 `json:"fireCount,omitempty"`
	// LastReason / LastMessage carry the most recent rejection reason
	// (e.g. "BadSignature", "AgentNotFound") for debugging.
	LastReason  string             `json:"lastReason,omitempty"`
	LastMessage string             `json:"lastMessage,omitempty"`
	Conditions  []metav1.Condition `json:"conditions,omitempty"`
}

type WebhookTrigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebhookTriggerSpec   `json:"spec,omitempty"`
	Status WebhookTriggerStatus `json:"status,omitempty"`
}

type WebhookTriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebhookTrigger `json:"items"`
}

func (in *WebhookTrigger) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(WebhookTrigger)
	in.DeepCopyInto(out)
	return out
}

func (in *WebhookTriggerList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(WebhookTriggerList)
	in.DeepCopyInto(out)
	return out
}

func (in *WebhookTrigger) DeepCopyInto(out *WebhookTrigger) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *WebhookTriggerSpec) DeepCopyInto(out *WebhookTriggerSpec) {
	*out = *in
	if in.Events != nil {
		out.Events = append([]string(nil), in.Events...)
	}
	out.Filter = in.Filter
	out.SecretRef = SecretRef{Name: in.SecretRef.Name}
	in.TaskTemplate.DeepCopyInto(&out.TaskTemplate)
}

func (in *WebhookTriggerStatus) DeepCopyInto(out *WebhookTriggerStatus) {
	*out = *in
	if in.LastFired != nil {
		t := *in.LastFired
		out.LastFired = &t
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *WebhookTriggerList) DeepCopyInto(out *WebhookTriggerList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]WebhookTrigger, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
