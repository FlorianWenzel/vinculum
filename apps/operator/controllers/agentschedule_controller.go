package controllers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1alpha1 "github.com/florian/vinculum/apps/operator/api/v1alpha1"
)

type AgentScheduleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Now    func() time.Time
}

func (r *AgentScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AgentSchedule{}).
		Owns(&v1alpha1.Task{}).
		Complete(r)
}

func (r *AgentScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	now := time.Now()
	if r.Now != nil {
		now = r.Now()
	}

	var schedule v1alpha1.AgentSchedule
	if err := r.Get(ctx, req.NamespacedName, &schedule); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if schedule.Spec.Suspend {
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	if strings.TrimSpace(schedule.Spec.Schedule) == "" {
		return ctrl.Result{}, nil
	}
	if strings.TrimSpace(schedule.Spec.AgentRef) == "" {
		return ctrl.Result{}, fmt.Errorf("spec.agentRef is required")
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(schedule.Spec.Schedule)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid schedule %q: %w", schedule.Spec.Schedule, err)
	}

	loc := time.Local
	if schedule.Spec.Timezone != "" {
		if l, lerr := time.LoadLocation(schedule.Spec.Timezone); lerr == nil {
			loc = l
		}
	}

	earliest := schedule.CreationTimestamp.Time
	if schedule.Status.LastScheduleTime != nil && schedule.Status.LastScheduleTime.After(earliest) {
		earliest = schedule.Status.LastScheduleTime.Time
	}
	missed := nextRunTimes(sched, earliest.In(loc), now.In(loc))
	if len(missed) == 0 {
		next := sched.Next(now.In(loc))
		wait := time.Until(next)
		if wait < time.Second {
			wait = time.Second
		}
		return ctrl.Result{RequeueAfter: wait}, nil
	}

	if err := r.cleanupHistory(ctx, &schedule); err != nil {
		return ctrl.Result{}, err
	}

	if !r.canStartConcurrent(ctx, &schedule) {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	target := missed[len(missed)-1]
	taskName := fmt.Sprintf("%s-%d", schedule.Name, target.Unix())
	desired := &v1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      taskName,
			Namespace: schedule.Namespace,
			Labels: map[string]string{
				"vinculum.dev/schedule": schedule.Name,
				"vinculum.dev/agent":    schedule.Spec.AgentRef,
			},
		},
		Spec: schedule.Spec.TaskTemplate.ToSpec(schedule.Spec.AgentRef),
	}
	if err := controllerutil.SetControllerReference(&schedule, desired, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Create(ctx, desired); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, err
	}

	scheduledAt := metav1.NewTime(target)
	schedule.Status.LastScheduleTime = &scheduledAt
	if err := r.Status().Update(ctx, &schedule); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}
	next := sched.Next(now.In(loc))
	return ctrl.Result{RequeueAfter: time.Until(next)}, nil
}

func nextRunTimes(s cron.Schedule, earliest, now time.Time) []time.Time {
	const maxMissed = 100
	var out []time.Time
	t := s.Next(earliest)
	for !t.After(now) && len(out) < maxMissed {
		out = append(out, t)
		t = s.Next(t)
	}
	return out
}

func (r *AgentScheduleReconciler) canStartConcurrent(ctx context.Context, sched *v1alpha1.AgentSchedule) bool {
	if sched.Spec.ConcurrencyPolicy == "" || sched.Spec.ConcurrencyPolicy == v1alpha1.AllowConcurrent {
		return true
	}
	var tasks v1alpha1.TaskList
	if err := r.List(ctx, &tasks, client.InNamespace(sched.Namespace), client.MatchingLabels{"vinculum.dev/schedule": sched.Name}); err != nil {
		return true
	}
	active := 0
	for _, task := range tasks.Items {
		if !v1alpha1.IsTaskTerminal(task.Status.Phase) {
			active++
		}
	}
	if active == 0 {
		return true
	}
	if sched.Spec.ConcurrencyPolicy == v1alpha1.ReplaceConcurrent {
		for i := range tasks.Items {
			t := tasks.Items[i]
			if !v1alpha1.IsTaskTerminal(t.Status.Phase) {
				_ = r.Delete(ctx, &t)
			}
		}
		return true
	}
	return false
}

func (r *AgentScheduleReconciler) cleanupHistory(ctx context.Context, sched *v1alpha1.AgentSchedule) error {
	limit := int(sched.Spec.HistoryLimit)
	if limit <= 0 {
		return nil
	}
	var tasks v1alpha1.TaskList
	if err := r.List(ctx, &tasks, client.InNamespace(sched.Namespace), client.MatchingLabels{"vinculum.dev/schedule": sched.Name}); err != nil {
		return err
	}
	finished := make([]v1alpha1.Task, 0, len(tasks.Items))
	for _, task := range tasks.Items {
		if v1alpha1.IsTaskTerminal(task.Status.Phase) {
			finished = append(finished, task)
		}
	}
	if len(finished) <= limit {
		return nil
	}
	sort.Slice(finished, func(i, j int) bool {
		return taskFinishedAt(finished[i]).Before(taskFinishedAt(finished[j]))
	})
	excess := len(finished) - limit
	for i := 0; i < excess; i++ {
		if err := r.Delete(ctx, &finished[i]); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func taskFinishedAt(task v1alpha1.Task) time.Time {
	if task.Status.CompletionTime != nil {
		return task.Status.CompletionTime.Time
	}
	return task.CreationTimestamp.Time
}
