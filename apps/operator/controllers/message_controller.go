package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/florian/vinculum/apps/operator/api/v1alpha1"
	vmetrics "github.com/florian/vinculum/apps/operator/internal/metrics"
)

const (
	messageDeliveredAnnotation = "vinculum.dev/delivered"
	messageAttemptsAnnotation  = "vinculum.dev/delivery-attempts"
	messageMaxDeliverAttempts  = 10
)

type MessageReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	HTTPClient      *http.Client
	metricsRecorded sync.Map
}

func (r *MessageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Message{}).
		Complete(r)
}

func (r *MessageReconciler) httpClient() *http.Client {
	if r.HTTPClient != nil {
		return r.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (r *MessageReconciler) recordTerminalMetric(m *v1alpha1.Message) {
	if !v1alpha1.IsMessageTerminal(m.Status.Phase) {
		return
	}
	uid := string(m.UID)
	if uid == "" {
		return
	}
	if _, seen := r.metricsRecorded.LoadOrStore(uid, true); seen {
		return
	}
	vmetrics.MessagesTotal.WithLabelValues(m.Spec.From, m.Spec.To, m.Status.Phase).Inc()
}

func (r *MessageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("message", req.NamespacedName.String())
	var msg v1alpha1.Message
	if err := r.Get(ctx, req.NamespacedName, &msg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if v1alpha1.IsMessageTerminal(msg.Status.Phase) {
		r.recordTerminalMetric(&msg)
		return ctrl.Result{}, nil
	}
	if msg.Annotations[messageDeliveredAnnotation] == "true" {
		return ctrl.Result{}, nil
	}

	if msg.Spec.To == "" || msg.Spec.Body == "" {
		return r.fail(ctx, &msg, "InvalidSpec", "spec.to and spec.body are required")
	}
	if msg.Spec.TimeoutSeconds > 0 && !msg.CreationTimestamp.IsZero() {
		if time.Since(msg.CreationTimestamp.Time) > time.Duration(msg.Spec.TimeoutSeconds)*time.Second {
			return r.timeout(ctx, &msg)
		}
	}

	var agent v1alpha1.Agent
	if err := r.Get(ctx, types.NamespacedName{Name: msg.Spec.To, Namespace: msg.Namespace}, &agent); err != nil {
		if apierrors.IsNotFound(err) {
			return r.fail(ctx, &msg, "AgentNotFound", fmt.Sprintf("receiver agent %q not found", msg.Spec.To))
		}
		return ctrl.Result{}, err
	}
	if !agent.Spec.Enabled {
		return r.fail(ctx, &msg, "AgentDisabled", fmt.Sprintf("receiver agent %q disabled", agent.Name))
	}
	if !agent.PeerEnabled() {
		return r.fail(ctx, &msg, "PeerDisabled", fmt.Sprintf("peer messaging disabled on %q", agent.Name))
	}
	if !agent.Status.Ready {
		if err := r.setPhase(ctx, &msg, v1alpha1.MessagePhasePending, "AgentNotReady", "waiting for receiver pod"); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if msg.Status.Phase == "" || msg.Status.Phase == v1alpha1.MessagePhasePending {
		msg.Status.Phase = v1alpha1.MessagePhaseDelivering
		msg.Status.ObservedGeneration = msg.Generation
		if err := r.Status().Update(ctx, &msg); err != nil && !apierrors.IsConflict(err) {
			return ctrl.Result{}, err
		}
	}

	url := fmt.Sprintf("http://agent-%s.%s.svc:%d/message", agent.Name, agent.Namespace, agentPort)
	payload := map[string]any{
		"messageId": string(msg.UID),
		"name":      msg.Name,
		"namespace": msg.Namespace,
		"from":      msg.Spec.From,
		"body":      msg.Spec.Body,
		"inReplyTo": msg.Spec.InReplyTo,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return r.fail(ctx, &msg, "EncodeFailed", err.Error())
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ctrl.Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := r.httpClient().Do(httpReq)
	if err != nil {
		return r.backoffDeliver(ctx, &msg, err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return r.backoffDeliver(ctx, &msg, fmt.Sprintf("agent returned %d", resp.StatusCode))
	}

	logger.V(1).Info("message delivered", "from", msg.Spec.From, "to", msg.Spec.To)
	now := metav1.Now()
	msg.Status.Phase = v1alpha1.MessagePhaseDelivered
	msg.Status.DeliveredAt = &now
	msg.Status.Reason = ""
	msg.Status.Message = ""
	if err := r.Status().Update(ctx, &msg); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}
	if msg.Annotations == nil {
		msg.Annotations = map[string]string{}
	}
	msg.Annotations[messageDeliveredAnnotation] = "true"
	delete(msg.Annotations, messageAttemptsAnnotation)
	if err := r.Update(ctx, &msg); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}

	// Thread back-link: append this Message's name to the parent's
	// status.replyMessages. Best-effort — failure does not undo delivery.
	if msg.Spec.InReplyTo != "" {
		if err := r.appendReply(ctx, msg.Namespace, msg.Spec.InReplyTo, msg.Name); err != nil {
			logger.V(1).Info("failed to back-link reply", "parent", msg.Spec.InReplyTo, "err", err.Error())
		}
	}
	r.recordTerminalMetric(&msg)
	return ctrl.Result{}, nil
}

func (r *MessageReconciler) appendReply(ctx context.Context, namespace, parentName, childName string) error {
	var parent v1alpha1.Message
	if err := r.Get(ctx, types.NamespacedName{Name: parentName, Namespace: namespace}, &parent); err != nil {
		return err
	}
	for _, n := range parent.Status.ReplyMessages {
		if n == childName {
			return nil
		}
	}
	parent.Status.ReplyMessages = append(parent.Status.ReplyMessages, childName)
	if err := r.Status().Update(ctx, &parent); err != nil && !apierrors.IsConflict(err) {
		return err
	}
	return nil
}

func (r *MessageReconciler) backoffDeliver(ctx context.Context, msg *v1alpha1.Message, errMsg string) (ctrl.Result, error) {
	attempts := 0
	if msg.Annotations != nil {
		if v, ok := msg.Annotations[messageAttemptsAnnotation]; ok {
			if parsed, err := strconv.Atoi(v); err == nil {
				attempts = parsed
			}
		}
	}
	attempts++
	if attempts >= messageMaxDeliverAttempts {
		return r.fail(ctx, msg, "DeliveryFailed", errMsg)
	}
	if msg.Annotations == nil {
		msg.Annotations = map[string]string{}
	}
	msg.Annotations[messageAttemptsAnnotation] = strconv.Itoa(attempts)
	if err := r.Update(ctx, msg); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}
	backoff := time.Duration(1<<attempts) * time.Second
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	return ctrl.Result{RequeueAfter: backoff}, nil
}

func (r *MessageReconciler) fail(ctx context.Context, msg *v1alpha1.Message, reason, message string) (ctrl.Result, error) {
	msg.Status.Phase = v1alpha1.MessagePhaseFailed
	msg.Status.Reason = reason
	msg.Status.Message = message
	if err := r.Status().Update(ctx, msg); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}
	r.recordTerminalMetric(msg)
	return ctrl.Result{}, nil
}

func (r *MessageReconciler) timeout(ctx context.Context, msg *v1alpha1.Message) (ctrl.Result, error) {
	msg.Status.Phase = v1alpha1.MessagePhaseTimedOut
	msg.Status.Reason = "Timeout"
	msg.Status.Message = "message not delivered within spec.timeoutSeconds"
	if err := r.Status().Update(ctx, msg); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}
	r.recordTerminalMetric(msg)
	return ctrl.Result{}, nil
}

func (r *MessageReconciler) setPhase(ctx context.Context, msg *v1alpha1.Message, phase, reason, message string) error {
	if msg.Status.Phase == phase && msg.Status.Reason == reason {
		return nil
	}
	msg.Status.Phase = phase
	msg.Status.Reason = reason
	msg.Status.Message = message
	msg.Status.ObservedGeneration = msg.Generation
	if err := r.Status().Update(ctx, msg); err != nil && !apierrors.IsConflict(err) {
		return err
	}
	return nil
}
