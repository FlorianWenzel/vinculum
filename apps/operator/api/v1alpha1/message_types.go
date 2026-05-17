package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// MessageSpec is an async peer-to-peer chat message between two Agents.
// Unlike a Task, a Message has no git workflow, no artifact sink, and no
// "result". A reply is a separate Message that sets InReplyTo to this
// Message's name — the conversation is the resource graph.
type MessageSpec struct {
	// To is the name of the receiving Agent. Required. Must reference an
	// Agent in the same namespace with PeerEnabled() == true.
	To string `json:"to"`
	// From is the name of the sending Agent. The operator overwrites this
	// from the X-Vinculum-From-Agent header on POST /api/messages, so the
	// caller can't forge another agent's identity over the in-cluster API.
	From string `json:"from,omitempty"`
	// Body is the message body. Wrapped into a [peer-message ...] block on
	// the receiver and fed into its crush session as a fresh prompt.
	Body string `json:"body"`
	// InReplyTo, when set, is the name of the Message this one replies to.
	// The Message controller appends this Message's name to the parent's
	// status.replyMessages so threads are discoverable both ways.
	InReplyTo string `json:"inReplyTo,omitempty"`
	// TimeoutSeconds, when > 0, marks the Message TimedOut if it has not
	// reached phase Delivered by then. There is no auto-wake of the sender;
	// the sender chases a non-response by sending a follow-up Message.
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
}

type MessageStatus struct {
	// Phase progresses Pending -> Delivering -> Delivered, or terminates
	// at Failed / TimedOut. Once Delivered, downstream processing is the
	// receiver's per-pod crush turn — observable via that Agent's pod logs.
	Phase              string             `json:"phase,omitempty"`
	Reason             string             `json:"reason,omitempty"`
	Message            string             `json:"message,omitempty"`
	DeliveredAt        *metav1.Time       `json:"deliveredAt,omitempty"`
	// ReplyMessages holds the names of Messages that set InReplyTo to this
	// one. Populated by the Message controller, best-effort.
	ReplyMessages      []string           `json:"replyMessages,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

type Message struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MessageSpec   `json:"spec,omitempty"`
	Status MessageStatus `json:"status,omitempty"`
}

type MessageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Message `json:"items"`
}

func (in *Message) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(Message)
	in.DeepCopyInto(out)
	return out
}

func (in *MessageList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(MessageList)
	in.DeepCopyInto(out)
	return out
}

func (in *Message) DeepCopyInto(out *Message) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

func (in *MessageStatus) DeepCopyInto(out *MessageStatus) {
	*out = *in
	if in.DeliveredAt != nil {
		t := *in.DeliveredAt
		out.DeliveredAt = &t
	}
	if in.ReplyMessages != nil {
		out.ReplyMessages = append([]string(nil), in.ReplyMessages...)
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *MessageList) DeepCopyInto(out *MessageList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]Message, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// Message phases.
const (
	MessagePhasePending    = "Pending"
	MessagePhaseDelivering = "Delivering"
	MessagePhaseDelivered  = "Delivered"
	MessagePhaseFailed     = "Failed"
	MessagePhaseTimedOut   = "TimedOut"
)

func IsMessageTerminal(phase string) bool {
	switch phase {
	case MessagePhaseDelivered, MessagePhaseFailed, MessagePhaseTimedOut:
		return true
	}
	return false
}
