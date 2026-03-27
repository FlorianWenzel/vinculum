package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/florian/vinculum/apps/orchestrator/api/v1alpha1"
	"github.com/florian/vinculum/apps/orchestrator/internal/forgejo"
)

type ReviewReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Forgejo *forgejo.Client
}

type reviewJobResult struct {
	Verdict string `json:"verdict"`
	Summary string `json:"summary"`
}

func (r *ReviewReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Review{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

func (r *ReviewReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.FromContext(ctx).Info("reconciling review", "name", req.Name, "namespace", req.Namespace)
	var review v1alpha1.Review
	if err := r.Get(ctx, req.NamespacedName, &review); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if review.Spec.TaskRef == "" {
		return ctrl.Result{}, nil
	}
	var task v1alpha1.Task
	if err := r.Get(ctx, types.NamespacedName{Name: review.Spec.TaskRef, Namespace: review.Namespace}, &task); err != nil {
		return ctrl.Result{RequeueAfter: 3}, nil
	}
	if review.Spec.Automated && review.Spec.Verdict == "" {
		if review.Spec.ReviewerDroneRef == "" {
			reviewerDrone, err := r.selectReviewerDrone(ctx, review.Namespace, review.Spec.RepositoryRef)
			if err != nil {
				review.Status.Phase = "Blocked"
				review.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionFalse, Reason: "ReviewerUnavailable", LastTransitionTime: metav1.Now(), Message: err.Error()}}
				_ = r.Status().Update(ctx, &review)
				return ctrl.Result{RequeueAfter: 5}, nil
			}
			updated := review.DeepCopyObject().(*v1alpha1.Review)
			updated.Spec.ReviewerDroneRef = reviewerDrone.Name
			if err := r.Update(ctx, updated); err != nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		reviewerDrone, err := r.getReviewerDrone(ctx, review.Namespace, review.Spec.ReviewerDroneRef)
		if err != nil {
			return ctrl.Result{RequeueAfter: 5}, nil
		}
		jobName := review.Name + "-runner"
		var job batchv1.Job
		err = r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: review.Namespace}, &job)
		if apierrors.IsNotFound(err) {
			job = r.buildReviewJob(review, task, *reviewerDrone, jobName)
			if err := controllerutil.SetControllerReference(&review, &job, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.Create(ctx, &job); err != nil {
				return ctrl.Result{}, err
			}
			reviewUpdated := review.DeepCopyObject().(*v1alpha1.Review)
			reviewUpdated.Status.Phase = "Running"
			reviewUpdated.Status.JobName = jobName
			reviewUpdated.Status.ReviewerDrone = reviewerDrone.Name
			reviewUpdated.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Running", LastTransitionTime: metav1.Now(), Message: "Automated review job is running."}}
			if err := r.Status().Update(ctx, reviewUpdated); err != nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 3}, nil
		}
		if err != nil {
			return ctrl.Result{}, err
		}
		if job.Status.Succeeded > 0 {
			result, err := r.readReviewJobResult(ctx, review.Namespace, job.Name)
			if err != nil {
				return ctrl.Result{RequeueAfter: 3}, nil
			}
			updated := review.DeepCopyObject().(*v1alpha1.Review)
			updated.Spec.Verdict = valueOr(result.Verdict, "Approve")
			updated.Spec.Summary = valueOr(result.Summary, reviewerOutcomeSummary(updated.Spec.Verdict, updated.Spec.ReviewerDroneRef))
			if err := r.Update(ctx, updated); err != nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		if job.Status.Failed > 0 {
			verdict, summary := r.reviewFailureOutcome(ctx, review.Namespace, job.Name, review.Spec.ReviewerDroneRef)
			updated := review.DeepCopyObject().(*v1alpha1.Review)
			updated.Spec.Verdict = verdict
			updated.Spec.Summary = summary
			if err := r.Update(ctx, updated); err != nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{RequeueAfter: 3}, nil
	}

	taskUpdated := task.DeepCopyObject().(*v1alpha1.Task)
	switch review.Spec.Verdict {
	case "Approve":
		if taskUpdated.Status.Phase != "Merged" {
			if merged, err := r.mergeTaskPullRequest(ctx, taskUpdated); err == nil && merged {
				taskUpdated.Status.Phase = "Merged"
			} else {
				taskUpdated.Status.Phase = "Approved"
			}
		}
	case "ChangesRequested":
		if taskUpdated.Status.Phase != "Merged" {
			taskUpdated.Status.Phase = "ChangesRequested"
		}
	case "Blocked":
		if taskUpdated.Status.Phase != "Merged" {
			taskUpdated.Status.Phase = "Blocked"
		}
	default:
		if taskUpdated.Status.Phase != "Merged" {
			taskUpdated.Status.Phase = "InReview"
		}
	}
	if review.Spec.Summary != "" {
		taskUpdated.Status.Summary = review.Spec.Summary
	}
	if err := r.Status().Update(ctx, taskUpdated); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}

	reviewUpdated := review.DeepCopyObject().(*v1alpha1.Review)
	reviewUpdated.Status.Phase = review.Spec.Verdict
	if reviewUpdated.Status.Phase == "" {
		reviewUpdated.Status.Phase = "Draft"
	}
	if reviewUpdated.Status.JobName == "" && review.Spec.Automated {
		reviewUpdated.Status.JobName = review.Name + "-runner"
	}
	if reviewUpdated.Status.ReviewerDrone == "" {
		reviewUpdated.Status.ReviewerDrone = review.Spec.ReviewerDroneRef
	}
	reviewUpdated.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Reconciled", LastTransitionTime: metav1.Now(), Message: "Review processed."}}
	if err := r.Status().Update(ctx, reviewUpdated); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ReviewReconciler) selectReviewerDrone(ctx context.Context, namespace, repoRef string) (*v1alpha1.Drone, error) {
	var drones v1alpha1.DroneList
	if err := r.List(ctx, &drones, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	eligible := make([]v1alpha1.Drone, 0)
	for _, item := range drones.Items {
		if !item.Spec.Enabled || item.Spec.Role != "reviewer" {
			continue
		}
		if item.Status.ForgejoReady == false || sshSecretName(&item) == "" {
			continue
		}
		ready, err := reviewRepositoryAccessReady(ctx, r.Client, namespace, item.Name, repoRef)
		if err != nil || !ready {
			continue
		}
		eligible = append(eligible, item)
	}
	if len(eligible) == 0 {
		return nil, fmt.Errorf("no reviewer drone with repository access is available")
	}
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].Status.ActiveTasks < eligible[j].Status.ActiveTasks
	})
	return &eligible[0], nil
}

func (r *ReviewReconciler) getReviewerDrone(ctx context.Context, namespace, name string) (*v1alpha1.Drone, error) {
	var drone v1alpha1.Drone
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &drone); err != nil {
		return nil, err
	}
	return &drone, nil
}

func reviewRepositoryAccessReady(ctx context.Context, c client.Client, namespace, droneRef, repoRef string) (bool, error) {
	var accessList v1alpha1.DroneRepositoryAccessList
	if err := c.List(ctx, &accessList, client.InNamespace(namespace)); err != nil {
		return false, err
	}
	for _, item := range accessList.Items {
		if item.Spec.DroneRef == droneRef && item.Spec.RepositoryRef == repoRef && item.Status.Phase == "Ready" {
			return true, nil
		}
	}
	return false, nil
}

func (r *ReviewReconciler) buildReviewJob(review v1alpha1.Review, task v1alpha1.Task, drone v1alpha1.Drone, name string) batchv1.Job {
	backoff := int32(0)
	script := []string{
		"set -eu",
		"git clone \"$REVIEW_REPO_URL\" /workspace/repo",
		"cd /workspace/repo",
		"git fetch origin \"$REVIEW_BASE_BRANCH\" \"$REVIEW_WORKING_BRANCH\"",
		"git checkout -B \"$REVIEW_WORKING_BRANCH\" \"origin/$REVIEW_WORKING_BRANCH\"",
		"rm -f /workspace/repo/.vinculum-review-result.json",
		"/usr/local/bin/vinculum-agent",
		"node -e 'const fs=require(\"fs\"); const p=\"/workspace/repo/.vinculum-review-result.json\"; if(!fs.existsSync(p)){console.error(\"missing review result file\"); process.exit(20)} const data=JSON.parse(fs.readFileSync(p,\"utf8\")); if(!data.summary){data.summary=\"Reviewer did not provide a summary.\"} fs.writeFileSync(\"/dev/termination-log\", JSON.stringify(data)); if(data.verdict===\"Approve\") process.exit(0); if(data.verdict===\"ChangesRequested\") process.exit(10); process.exit(20);'",
	}
	volumes := []corev1.Volume{{Name: "workspace", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
	mounts := []corev1.VolumeMount{{Name: "workspace", MountPath: "/workspace"}}
	initContainers := []corev1.Container{}
	if instructionConfigMapName(&drone) != "" {
		volumes = append(volumes, corev1.Volume{Name: "instructions", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: instructionConfigMapName(&drone)}}}})
		mounts = append(mounts, corev1.VolumeMount{Name: "instructions", MountPath: valueOr(drone.Spec.InstructionMountPath, "/instructions"), ReadOnly: true})
	}
	if sshSecretName(&drone) != "" {
		volumes = append(volumes,
			corev1.Volume{Name: "ssh-source", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: sshSecretName(&drone)}}},
			corev1.Volume{Name: "ssh-home", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		)
		initContainers = append(initContainers, corev1.Container{
			Name:            "prepare-ssh",
			Image:           drone.Spec.Image,
			ImagePullPolicy: corev1.PullAlways,
			Command:         []string{"/bin/sh", "-c"},
			Args:            []string{"cp /ssh-source/id_ed25519 /ssh-home/id_ed25519 && cp /ssh-source/id_ed25519.pub /ssh-home/id_ed25519.pub && chown 10001:10001 /ssh-home/id_ed25519 /ssh-home/id_ed25519.pub && chmod 600 /ssh-home/id_ed25519 && chmod 644 /ssh-home/id_ed25519.pub"},
			VolumeMounts:    []corev1.VolumeMount{{Name: "ssh-source", MountPath: "/ssh-source", ReadOnly: true}, {Name: "ssh-home", MountPath: "/ssh-home"}},
		})
		mounts = append(mounts, corev1.VolumeMount{Name: "ssh-home", MountPath: "/home/agent/.ssh"})
	}
	if providerSecretName(&drone) != "" {
		volumes = append(volumes, corev1.Volume{Name: "provider-auth", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: providerSecretName(&drone)}}})
		mounts = append(mounts, corev1.VolumeMount{Name: "provider-auth", MountPath: "/home/agent/.local/share/opencode/auth.json", SubPath: providerAuthFileKey(&drone), ReadOnly: true})
	}
	env := []corev1.EnvVar{
		{Name: "SERVER_ADDR", Value: ":8090"},
		{Name: "WORK_DIR", Value: "/workspace"},
		{Name: "INSTRUCTIONS_DIR", Value: valueOr(drone.Spec.InstructionMountPath, "/instructions")},
		{Name: "OPENCODE_MODEL", Value: drone.Spec.Model},
		{Name: "OPENCODE_AGENT", Value: drone.Spec.OpenCodeAgent},
		{Name: "AUTO_RUN_PROMPT", Value: buildReviewPrompt(review, task)},
		{Name: "AUTO_RUN_DIR", Value: "/workspace/repo"},
		{Name: "AUTO_RUN_WITH_INSTRUCTIONS", Value: "true"},
		{Name: "REVIEW_REPO_URL", Value: task.Spec.RepoURL},
		{Name: "REVIEW_BASE_BRANCH", Value: valueOr(task.Spec.BaseBranch, "main")},
		{Name: "REVIEW_WORKING_BRANCH", Value: valueOr(task.Spec.WorkingBranch, task.Name)},
		{Name: "REVIEW_REQUIREMENT_FILE_PATH", Value: review.Spec.RequirementFilePath},
		{Name: "GIT_SSH_COMMAND", Value: "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i /home/agent/.ssh/id_ed25519"},
	}
	for k, v := range drone.Spec.Env {
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}
	return batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: review.Namespace, Labels: map[string]string{"vinculum.dev/review": review.Name, "vinculum.dev/drone": drone.Name, "vinculum.dev/job-kind": "review"}},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"vinculum.dev/review": review.Name, "vinculum.dev/drone": drone.Name, "vinculum.dev/job-kind": "review"}},
				Spec: corev1.PodSpec{
					RestartPolicy:  corev1.RestartPolicyNever,
					InitContainers: initContainers,
					Containers: []corev1.Container{{
						Name:            "reviewer",
						Image:           drone.Spec.Image,
						ImagePullPolicy: corev1.PullAlways,
						Command:         []string{"/bin/sh", "-c"},
						Args:            []string{strings.Join(script, " && ")},
						Env:             env,
						VolumeMounts:    mounts,
					}},
					Volumes: volumes,
				},
			},
		},
	}
}

func buildReviewPrompt(review v1alpha1.Review, task v1alpha1.Task) string {
	parts := []string{
		"You are reviewing a pull request for correctness and feature behavior.",
		"The repository is already checked out on the pull request branch in /workspace/repo.",
		"Use the requirement file in the repository as the source of truth and compare the implementation against it.",
		"Compare the branch against the base branch, inspect the changed files, and validate the behavior end to end.",
		"Run all relevant checks you can discover. If this looks like a web application, run the app and exercise the changed flows with Cypress or an equivalent browser test tool when available. If Cypress is unavailable, use the best available test mechanism and say so in the summary.",
		"Focus on whether the change is correct, complete, and safe to merge.",
		"When finished, write JSON to /workspace/repo/.vinculum-review-result.json with exactly this shape: {\"verdict\":\"Approve|ChangesRequested|Blocked\",\"summary\":\"short review summary\"}.",
		"Choose Approve only if the change is ready to merge. Choose ChangesRequested for fixable problems. Choose Blocked if you cannot complete the review or validation.",
		"Task prompt:\n" + task.Spec.Prompt,
	}
	if review.Spec.PullRequestURL != "" {
		parts = append(parts, "Pull request URL: "+review.Spec.PullRequestURL)
	}
	if review.Spec.RequirementFilePath != "" {
		parts = append(parts, "Requirement file: /workspace/repo/"+strings.TrimPrefix(review.Spec.RequirementFilePath, "/"))
	}
	return strings.Join(parts, "\n\n")
}

func (r *ReviewReconciler) mergeTaskPullRequest(ctx context.Context, task *v1alpha1.Task) (bool, error) {
	if r.Forgejo == nil || task.Spec.RepositoryRef == "" || task.Status.PullRequestNumber == 0 {
		return false, nil
	}
	var repo v1alpha1.Repository
	if err := r.Get(ctx, types.NamespacedName{Name: task.Spec.RepositoryRef, Namespace: task.Namespace}, &repo); err != nil {
		return false, err
	}
	pr, err := r.Forgejo.GetPullRequest(ctx, repo.Spec.Owner, repo.Spec.Name, task.Status.PullRequestNumber)
	if err != nil {
		return false, err
	}
	if pr.Merged {
		return true, nil
	}
	if _, err := r.Forgejo.MergePullRequest(ctx, repo.Spec.Owner, repo.Spec.Name, task.Status.PullRequestNumber); err != nil {
		return false, err
	}
	return true, nil
}

func (r *ReviewReconciler) readReviewJobResult(ctx context.Context, namespace, jobName string) (reviewJobResult, error) {
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels{"job-name": jobName}); err != nil {
		return reviewJobResult{}, err
	}
	for _, pod := range pods.Items {
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name != "reviewer" || status.State.Terminated == nil {
				continue
			}
			var result reviewJobResult
			if err := json.Unmarshal([]byte(status.State.Terminated.Message), &result); err == nil {
				return result, nil
			}
		}
	}
	return reviewJobResult{}, fmt.Errorf("review result not available yet")
}

func (r *ReviewReconciler) reviewFailureOutcome(ctx context.Context, namespace, jobName, reviewerDrone string) (string, string) {
	if result, err := r.readReviewJobResult(ctx, namespace, jobName); err == nil {
		return valueOr(result.Verdict, "Blocked"), valueOr(result.Summary, reviewerOutcomeSummary(valueOr(result.Verdict, "Blocked"), reviewerDrone))
	}
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels{"job-name": jobName}); err != nil {
		return "Blocked", reviewerOutcomeSummary("Blocked", reviewerDrone)
	}
	for _, pod := range pods.Items {
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name != "reviewer" || status.State.Terminated == nil {
				continue
			}
			switch status.State.Terminated.ExitCode {
			case 10:
				return "ChangesRequested", reviewerOutcomeSummary("ChangesRequested", reviewerDrone)
			case 20:
				return "Blocked", reviewerOutcomeSummary("Blocked", reviewerDrone)
			}
		}
	}
	return "Blocked", reviewerOutcomeSummary("Blocked", reviewerDrone)
}

func reviewerOutcomeSummary(verdict, reviewerDrone string) string {
	prefix := "Automated review"
	if reviewerDrone != "" {
		prefix = "Reviewer drone " + reviewerDrone
	}
	switch verdict {
	case "Approve":
		return prefix + " approved the task after inspecting the branch changes."
	case "ChangesRequested":
		return prefix + " requested changes after finding unresolved TODO or FIXME markers in changed files."
	default:
		return prefix + " blocked the task because it could not validate the change set."
	}
}
