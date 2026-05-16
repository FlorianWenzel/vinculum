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
	taskDispatchedAnnotation = "vinculum.dev/dispatched"
	taskAttemptsAnnotation   = "vinculum.dev/dispatch-attempts"
	taskMaxDispatchAttempts  = 10
)

type TaskReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	HTTPClient *http.Client
	// metricsRecorded tracks UIDs of Tasks whose terminal metrics have
	// already been emitted, so the per-reconcile observation doesn't
	// double-count on repeated reconciles. Operator restart resets the
	// map; that's acceptable (old terminal Tasks shouldn't keep emitting
	// new samples anyway).
	metricsRecorded sync.Map
}

// recordTerminalMetrics observes the Task's duration and increments the
// per-phase counter exactly once per Task lifetime (per operator process).
func (r *TaskReconciler) recordTerminalMetrics(task *v1alpha1.Task) {
	if !v1alpha1.IsTaskTerminal(task.Status.Phase) {
		return
	}
	uid := string(task.UID)
	if uid == "" {
		return
	}
	if _, seen := r.metricsRecorded.LoadOrStore(uid, true); seen {
		return
	}
	vmetrics.TasksTotal.WithLabelValues(task.Spec.AgentRef, task.Status.Phase).Inc()
	if task.Status.StartTime != nil && task.Status.CompletionTime != nil {
		d := task.Status.CompletionTime.Sub(task.Status.StartTime.Time).Seconds()
		if d >= 0 {
			vmetrics.TaskDurationSeconds.WithLabelValues(task.Spec.AgentRef, task.Status.Phase).Observe(d)
		}
	}
}

func (r *TaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Task{}).
		Complete(r)
}

func (r *TaskReconciler) httpClient() *http.Client {
	if r.HTTPClient != nil {
		return r.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (r *TaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("task", req.NamespacedName.String())
	var task v1alpha1.Task
	if err := r.Get(ctx, req.NamespacedName, &task); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if v1alpha1.IsTaskTerminal(task.Status.Phase) {
		r.recordTerminalMetrics(&task)
		return ctrl.Result{}, nil
	}
	if task.Annotations[taskDispatchedAnnotation] == "true" {
		return ctrl.Result{}, nil
	}

	if task.Spec.AgentRef == "" {
		return r.fail(ctx, &task, "InvalidSpec", "agentRef is required")
	}
	var agent v1alpha1.Agent
	if err := r.Get(ctx, types.NamespacedName{Name: task.Spec.AgentRef, Namespace: task.Namespace}, &agent); err != nil {
		if apierrors.IsNotFound(err) {
			return r.fail(ctx, &task, "AgentNotFound", fmt.Sprintf("agent %q not found", task.Spec.AgentRef))
		}
		return ctrl.Result{}, err
	}
	if !agent.Spec.Enabled {
		return r.fail(ctx, &task, "AgentDisabled", fmt.Sprintf("agent %q disabled", agent.Name))
	}
	if !agent.Status.Ready {
		reason := "AgentNotReady"
		message := "waiting for agent pod"
		// If the operator already diagnosed the agent's pending state (e.g.
		// the git-clone init container failed), surface that on the Task
		// instead of a generic message.
		if agent.Status.Reason != "" {
			reason = agent.Status.Reason
			if agent.Status.Message != "" {
				message = agent.Status.Message
			}
		}
		if err := r.setPhase(ctx, &task, v1alpha1.TaskPhasePending, reason, message); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if task.Status.Phase == "" || task.Status.Phase == v1alpha1.TaskPhasePending {
		task.Status.Phase = v1alpha1.TaskPhaseDispatching
		task.Status.ObservedGeneration = task.Generation
		if err := r.Status().Update(ctx, &task); err != nil && !apierrors.IsConflict(err) {
			return ctrl.Result{}, err
		}
	}

	url := fmt.Sprintf("http://agent-%s.%s.svc:%d/task", agent.Name, agent.Namespace, agentPort)
	payload := taskDispatchPayload(&task)
	body, err := json.Marshal(payload)
	if err != nil {
		return r.fail(ctx, &task, "EncodeFailed", err.Error())
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ctrl.Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := r.httpClient().Do(httpReq)
	if err != nil {
		return r.backoffDispatch(ctx, &task, err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return r.backoffDispatch(ctx, &task, fmt.Sprintf("agent returned %d", resp.StatusCode))
	}

	logger.V(1).Info("task dispatched", "agent", agent.Name)
	if task.Annotations == nil {
		task.Annotations = map[string]string{}
	}
	task.Annotations[taskDispatchedAnnotation] = "true"
	delete(task.Annotations, taskAttemptsAnnotation)
	return ctrl.Result{}, r.Update(ctx, &task)
}

func (r *TaskReconciler) backoffDispatch(ctx context.Context, task *v1alpha1.Task, msg string) (ctrl.Result, error) {
	attempts := 0
	if task.Annotations != nil {
		if v, ok := task.Annotations[taskAttemptsAnnotation]; ok {
			if parsed, err := strconv.Atoi(v); err == nil {
				attempts = parsed
			}
		}
	}
	attempts++
	if attempts >= taskMaxDispatchAttempts {
		return r.fail(ctx, task, "DispatchFailed", msg)
	}
	if task.Annotations == nil {
		task.Annotations = map[string]string{}
	}
	task.Annotations[taskAttemptsAnnotation] = strconv.Itoa(attempts)
	if err := r.Update(ctx, task); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}
	backoff := time.Duration(1<<attempts) * time.Second
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	return ctrl.Result{RequeueAfter: backoff}, nil
}

func taskDispatchPayload(task *v1alpha1.Task) map[string]any {
	return map[string]any{
		"taskId":    string(task.UID),
		"name":      task.Name,
		"namespace": task.Namespace,
		"spec":      task.Spec,
	}
}

func (r *TaskReconciler) fail(ctx context.Context, task *v1alpha1.Task, reason, message string) (ctrl.Result, error) {
	now := metav1.Now()
	task.Status.Phase = v1alpha1.TaskPhaseFailed
	task.Status.Reason = reason
	task.Status.Message = message
	if task.Status.CompletionTime == nil {
		task.Status.CompletionTime = &now
	}
	if err := r.Status().Update(ctx, task); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *TaskReconciler) setPhase(ctx context.Context, task *v1alpha1.Task, phase, reason, message string) error {
	if task.Status.Phase == phase && task.Status.Reason == reason {
		return nil
	}
	task.Status.Phase = phase
	task.Status.Reason = reason
	task.Status.Message = message
	task.Status.ObservedGeneration = task.Generation
	if err := r.Status().Update(ctx, task); err != nil && !apierrors.IsConflict(err) {
		return err
	}
	return nil
}
