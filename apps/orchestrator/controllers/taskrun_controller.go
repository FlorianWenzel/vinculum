package controllers

import (
	"context"
	"fmt"
	"path/filepath"
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

type TaskRunReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Forgejo *forgejo.Client
}

func (r *TaskRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.TaskRun{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

func (r *TaskRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.FromContext(ctx).Info("reconciling taskrun", "name", req.Name, "namespace", req.Namespace)
	var task v1alpha1.TaskRun
	if err := r.Get(ctx, req.NamespacedName, &task); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if task.Status.Phase == "Merged" || task.Status.Phase == "Failed" || task.Status.Phase == "Approved" {
		if task.Status.Phase == "Approved" {
			if updated, err := r.syncPullRequestState(ctx, &task); err == nil && updated {
				return ctrl.Result{}, nil
			}
		}
		return ctrl.Result{}, nil
	}
	if ready, waitingOn, err := r.dependenciesReady(ctx, &task); err == nil && !ready {
		if task.Status.Phase != "WaitingOnDependencies" || task.Status.Summary != waitingOn {
			updated := task.DeepCopyObject().(*v1alpha1.TaskRun)
			updated.Status.Phase = "WaitingOnDependencies"
			updated.Status.Summary = waitingOn
			if statusErr := r.Status().Update(ctx, updated); statusErr != nil && !apierrors.IsConflict(statusErr) {
				return ctrl.Result{}, statusErr
			}
		}
		return ctrl.Result{RequeueAfter: 5}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}
	if task.Spec.DroneRef == "" && task.Spec.Role == "" {
		if task.Status.Phase == "" {
			task.Status.Phase = "Planned"
			if err := r.Status().Update(ctx, &task); err != nil && !apierrors.IsConflict(err) {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if task.Spec.RepositoryRef != "" && task.Spec.RepoURL == "" {
		var repo v1alpha1.ForgejoRepository
		if err := r.Get(ctx, types.NamespacedName{Name: task.Spec.RepositoryRef, Namespace: task.Namespace}, &repo); err != nil {
			return ctrl.Result{RequeueAfter: 5}, nil
		}
		repoURL := repo.Status.SSHURL
		if repoURL == "" {
			repoURL = fmt.Sprintf("ssh://git@vinculum-infra-forgejo-ssh.vinculum-system.svc.cluster.local:22/%s/%s.git", repo.Spec.Owner, repo.Spec.Name)
		}
		task.Spec.RepoURL = repoURL
		if err := r.Update(ctx, &task); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}
	if task.Spec.BaseBranch == "" || task.Spec.WorkingBranch == "" {
		updated := task.DeepCopyObject().(*v1alpha1.TaskRun)
		if updated.Spec.BaseBranch == "" {
			updated.Spec.BaseBranch = "main"
		}
		if updated.Spec.WorkingBranch == "" {
			updated.Spec.WorkingBranch = task.Name
		}
		if err := r.Update(ctx, updated); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if task.Spec.RepositoryRef != "" && task.Spec.DroneRef != "" {
		ready, err := r.repositoryAccessReady(ctx, task.Namespace, task.Spec.DroneRef, task.Spec.RepositoryRef)
		if err == nil && !ready {
			return ctrl.Result{RequeueAfter: 3}, nil
		}
	}

	drone, err := r.selectDrone(ctx, &task)
	if err != nil {
		if task.Status.Phase != "Planned" {
			updated := task.DeepCopyObject().(*v1alpha1.TaskRun)
			updated.Status.Phase = "Planned"
			updated.Status.Summary = err.Error()
			if statusErr := r.Status().Update(ctx, updated); statusErr != nil && !apierrors.IsConflict(statusErr) {
				return ctrl.Result{}, statusErr
			}
		}
		return ctrl.Result{RequeueAfter: 5}, nil
	}
	if task.Spec.RepositoryRef != "" {
		ready, accessErr := r.repositoryAccessReady(ctx, task.Namespace, drone.Name, task.Spec.RepositoryRef)
		if accessErr == nil && !ready {
			return ctrl.Result{RequeueAfter: 3}, nil
		}
	}

	if err := r.ensureDroneAssets(ctx, drone); err != nil {
		task.Status.Phase = "Blocked"
		_ = r.Status().Update(ctx, &task)
		return ctrl.Result{RequeueAfter: 5}, nil
	}
	if generatedSSHSecretName(drone) != "" && drone.Status.SSHSecretName == "" && drone.Spec.SSHKeySecretRef == nil {
		task.Status.Phase = "Blocked"
		_ = r.Status().Update(ctx, &task)
		return ctrl.Result{RequeueAfter: 5}, nil
	}

	if task.Status.AssignedDrone == "" {
		task.Status.AssignedDrone = drone.Name
		task.Status.Phase = "Assigned"
		now := metav1.Now()
		task.Status.StartedAt = &now
		if err := r.Status().Update(ctx, &task); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	jobName := fmt.Sprintf("%s-runner", task.Name)
	var job batchv1.Job
	err = r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: task.Namespace}, &job)
	if apierrors.IsNotFound(err) {
		job = r.buildJob(task, *drone, jobName)
		if err := controllerutil.SetControllerReference(&task, &job, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, &job); err != nil {
			return ctrl.Result{}, err
		}
		task.Status.JobName = jobName
		task.Status.Phase = "Coding"
		if err := r.Status().Update(ctx, &task); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		if err := r.updateDroneStatus(ctx, drone, task.Name, 1); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 3}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	if job.Status.Succeeded > 0 && task.Status.Phase != "InReview" && task.Status.Phase != "Merged" && task.Status.Phase != "Testing" {
		if err := r.ensurePullRequest(ctx, &task); err != nil {
			task.Status.Phase = "Blocked"
			task.Status.Summary = err.Error()
			_ = r.Status().Update(ctx, &task)
			return ctrl.Result{RequeueAfter: 5}, nil
		}
		verificationDrone, err := r.selectSupportDrone(ctx, task.Namespace, task.Spec.RepositoryRef, []string{"tester"}, drone)
		if err != nil {
			task.Status.Phase = "Blocked"
			task.Status.Summary = err.Error()
			_ = r.Status().Update(ctx, &task)
			return ctrl.Result{RequeueAfter: 5}, nil
		}
		verificationJobName := fmt.Sprintf("%s-verifier", task.Name)
		var verificationJob batchv1.Job
		err = r.Get(ctx, types.NamespacedName{Name: verificationJobName, Namespace: task.Namespace}, &verificationJob)
		if apierrors.IsNotFound(err) {
			verificationJob = r.buildVerificationJob(task, *verificationDrone, verificationJobName)
			if err := controllerutil.SetControllerReference(&task, &verificationJob, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.Create(ctx, &verificationJob); err != nil {
				return ctrl.Result{}, err
			}
			task.Status.Phase = "Testing"
			task.Status.VerificationPhase = "Running"
			task.Status.VerificationJobName = verificationJobName
			task.Status.VerificationDrone = verificationDrone.Name
			finished := metav1.Now()
			task.Status.FinishedAt = &finished
			if task.Status.Branch == "" {
				task.Status.Branch = valueOr(task.Spec.WorkingBranch, task.Name)
			}
			if err := r.Status().Update(ctx, &task); err != nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
			if err := r.updateDroneStatus(ctx, drone, task.Name, -1); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 3}, nil
		}
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	if task.Status.VerificationJobName != "" {
		var verificationJob batchv1.Job
		if err := r.Get(ctx, types.NamespacedName{Name: task.Status.VerificationJobName, Namespace: task.Namespace}, &verificationJob); err != nil {
			if apierrors.IsNotFound(err) {
				return ctrl.Result{RequeueAfter: 3}, nil
			}
			return ctrl.Result{}, err
		}
		if verificationJob.Status.Succeeded > 0 && task.Status.VerificationPhase != "Passed" && task.Status.Phase != "Approved" && task.Status.Phase != "Merged" {
			review, err := r.ensureAutomatedReview(ctx, &task)
			if err != nil {
				task.Status.Phase = "Blocked"
				task.Status.Summary = err.Error()
				_ = r.Status().Update(ctx, &task)
				return ctrl.Result{RequeueAfter: 5}, nil
			}
			task.Status.Phase = "InReview"
			task.Status.VerificationPhase = "Passed"
			task.Status.ReviewRef = review.Name
			task.Status.Summary = "Verification passed and automated review was requested."
			if err := r.Status().Update(ctx, &task); err != nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
		}
		if verificationJob.Status.Failed > 0 && task.Status.VerificationPhase != "Failed" && task.Status.Phase != "Failed" {
			task.Status.Phase = "Failed"
			task.Status.VerificationPhase = "Failed"
			task.Status.Summary = "Verification job failed."
			finished := metav1.Now()
			task.Status.FinishedAt = &finished
			if err := r.Status().Update(ctx, &task); err != nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
		}
	}
	if job.Status.Failed > 0 && task.Status.Phase != "Failed" {
		task.Status.Phase = "Failed"
		finished := metav1.Now()
		task.Status.FinishedAt = &finished
		if err := r.Status().Update(ctx, &task); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		if err := r.updateDroneStatus(ctx, drone, task.Name, -1); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: 5}, nil
}

func (r *TaskRunReconciler) selectDrone(ctx context.Context, task *v1alpha1.TaskRun) (*v1alpha1.Drone, error) {
	if task.Status.AssignedDrone != "" {
		var drone v1alpha1.Drone
		if err := r.Get(ctx, types.NamespacedName{Name: task.Status.AssignedDrone, Namespace: task.Namespace}, &drone); err != nil {
			return nil, err
		}
		return &drone, nil
	}
	var drones v1alpha1.DroneList
	if err := r.List(ctx, &drones, client.InNamespace(task.Namespace)); err != nil {
		return nil, err
	}
	eligible := make([]v1alpha1.Drone, 0)
	for _, d := range drones.Items {
		if !d.Spec.Enabled {
			continue
		}
		if task.Spec.DroneRef != "" && d.Name != task.Spec.DroneRef {
			continue
		}
		if task.Spec.Role != "" && d.Spec.Role != task.Spec.Role {
			continue
		}
		if d.Spec.Concurrency > 0 && d.Status.ActiveTasks >= d.Spec.Concurrency {
			continue
		}
		eligible = append(eligible, d)
	}
	if len(eligible) == 0 {
		return nil, fmt.Errorf("no available drone")
	}
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].Status.ActiveTasks < eligible[j].Status.ActiveTasks
	})
	return &eligible[0], nil
}

func (r *TaskRunReconciler) repositoryAccessReady(ctx context.Context, namespace, droneRef, repoRef string) (bool, error) {
	var accessList v1alpha1.DroneRepositoryAccessList
	if err := r.List(ctx, &accessList, client.InNamespace(namespace)); err != nil {
		return false, err
	}
	for _, item := range accessList.Items {
		if item.Spec.DroneRef == droneRef && item.Spec.RepositoryRef == repoRef && item.Status.Phase == "Ready" {
			return true, nil
		}
	}
	return false, nil
}

func (r *TaskRunReconciler) dependenciesReady(ctx context.Context, task *v1alpha1.TaskRun) (bool, string, error) {
	if len(task.Spec.DependsOn) == 0 {
		return true, "", nil
	}
	blockedBy := make([]string, 0)
	for _, dependency := range task.Spec.DependsOn {
		var depTask v1alpha1.Task
		if err := r.Get(ctx, types.NamespacedName{Name: dependency, Namespace: task.Namespace}, &depTask); err != nil {
			if apierrors.IsNotFound(err) {
				blockedBy = append(blockedBy, dependency+" (missing)")
				continue
			}
			return false, "", err
		}
		if depTask.Status.Phase != "Merged" {
			blockedBy = append(blockedBy, dependency+" ("+valueOr(depTask.Status.Phase, "pending")+")")
		}
	}
	if len(blockedBy) == 0 {
		return true, "", nil
	}
	return false, "Waiting on dependencies: " + strings.Join(blockedBy, ", "), nil
}

func (r *TaskRunReconciler) selectSupportDrone(ctx context.Context, namespace, repoRef string, roles []string, fallback *v1alpha1.Drone) (*v1alpha1.Drone, error) {
	var drones v1alpha1.DroneList
	if err := r.List(ctx, &drones, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	for _, role := range roles {
		eligible := make([]v1alpha1.Drone, 0)
		for _, item := range drones.Items {
			if !item.Spec.Enabled || item.Spec.Role != role {
				continue
			}
			ready, err := r.repositoryAccessReady(ctx, namespace, item.Name, repoRef)
			if err != nil || !ready {
				continue
			}
			if sshSecretName(&item) == "" {
				continue
			}
			eligible = append(eligible, item)
		}
		if len(eligible) == 0 {
			continue
		}
		sort.Slice(eligible, func(i, j int) bool {
			return eligible[i].Status.ActiveTasks < eligible[j].Status.ActiveTasks
		})
		return &eligible[0], nil
	}
	if fallback != nil {
		ready, err := r.repositoryAccessReady(ctx, namespace, fallback.Name, repoRef)
		if err == nil && ready {
			return fallback, nil
		}
	}
	return nil, fmt.Errorf("no support drone with repository access for roles %s", strings.Join(roles, ", "))
}

func (r *TaskRunReconciler) buildVerificationJob(task v1alpha1.TaskRun, drone v1alpha1.Drone, name string) batchv1.Job {
	backoff := int32(0)
	script := []string{
		"set -eu",
		"git clone \"$TASKRUN_REPO_URL\" /workspace/repo",
		"cd /workspace/repo",
		"git fetch origin \"$TASKRUN_BASE_BRANCH\" \"$TASKRUN_WORKING_BRANCH\"",
		"git checkout -B \"$TASKRUN_WORKING_BRANCH\" \"origin/$TASKRUN_WORKING_BRANCH\"",
		"changed=$(git diff --name-only \"origin/$TASKRUN_BASE_BRANCH...HEAD\")",
		"if [ -z \"$changed\" ]; then echo 'No changed files found for verification.' >&2; exit 1; fi",
		"printf 'Changed files:\n%s\n' \"$changed\"",
		"if [ -f package.json ] && command -v node >/dev/null 2>&1; then if node -e 'const fs=require(\"fs\"); const p=JSON.parse(fs.readFileSync(\"package.json\",\"utf8\")); process.exit(p.scripts && p.scripts.test ? 0 : 1)'; then npm test --if-present; else echo 'No npm test script detected; skipping npm tests.'; fi; if node -e 'const fs=require(\"fs\"); const p=JSON.parse(fs.readFileSync(\"package.json\",\"utf8\")); process.exit(p.scripts && p.scripts.lint ? 0 : 1)'; then npm run lint --if-present; else echo 'No npm lint script detected; skipping lint.'; fi; if node -e 'const fs=require(\"fs\"); const p=JSON.parse(fs.readFileSync(\"package.json\",\"utf8\")); process.exit(p.scripts && p.scripts.build ? 0 : 1)'; then npm run build --if-present; else echo 'No npm build script detected; skipping build.'; fi; fi",
		"if [ -x ./test.sh ]; then ./test.sh; elif [ -x ./scripts/test.sh ]; then ./scripts/test.sh; else echo 'No repository test shell script detected; skipping shell-script tests.'; fi",
		"if [ -f go.mod ]; then if command -v go >/dev/null 2>&1; then go test ./...; else echo 'Go module detected but go binary is unavailable in the verifier image; skipping go test.'; fi; fi",
		"if [ -f pyproject.toml ] || [ -f requirements.txt ]; then if command -v pytest >/dev/null 2>&1; then pytest; else echo 'Python project detected but pytest is unavailable in the verifier image; skipping pytest.'; fi; fi",
		"if [ -f Cargo.toml ]; then if command -v cargo >/dev/null 2>&1; then cargo test; else echo 'Rust project detected but cargo is unavailable in the verifier image; skipping cargo test.'; fi; fi",
		"echo 'Verification completed successfully.'",
	}
	volumes := []corev1.Volume{{Name: "workspace", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
	mounts := []corev1.VolumeMount{{Name: "workspace", MountPath: "/workspace"}}
	initContainers := []corev1.Container{}
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
	env := []corev1.EnvVar{
		{Name: "TASKRUN_REPO_URL", Value: task.Spec.RepoURL},
		{Name: "TASKRUN_BASE_BRANCH", Value: valueOr(task.Spec.BaseBranch, "main")},
		{Name: "TASKRUN_WORKING_BRANCH", Value: valueOr(task.Spec.WorkingBranch, task.Name)},
		{Name: "GIT_SSH_COMMAND", Value: "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i /home/agent/.ssh/id_ed25519"},
	}
	return batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: task.Namespace, Labels: map[string]string{"vinculum.dev/taskrun": task.Name, "vinculum.dev/drone": drone.Name, "vinculum.dev/job-kind": "verification"}},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"vinculum.dev/taskrun": task.Name, "vinculum.dev/drone": drone.Name, "vinculum.dev/job-kind": "verification"}},
				Spec: corev1.PodSpec{
					RestartPolicy:  corev1.RestartPolicyNever,
					InitContainers: initContainers,
					Containers: []corev1.Container{{
						Name:            "verifier",
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

func (r *TaskRunReconciler) buildJob(task v1alpha1.TaskRun, drone v1alpha1.Drone, name string) batchv1.Job {
	backoff := int32(0)
	cmd := []string{"/usr/local/bin/vinculum-agent"}
	initContainers := []corev1.Container{}
	env := []corev1.EnvVar{
		{Name: "SERVER_ADDR", Value: ":8090"},
		{Name: "WORK_DIR", Value: "/workspace"},
		{Name: "INSTRUCTIONS_DIR", Value: valueOr(drone.Spec.InstructionMountPath, "/instructions")},
		{Name: "OPENCODE_MODEL", Value: drone.Spec.Model},
		{Name: "OPENCODE_AGENT", Value: drone.Spec.OpenCodeAgent},
		{Name: "AUTO_RUN_PROMPT", Value: task.Spec.Prompt},
		{Name: "AUTO_RUN_DIR", Value: "/workspace/repo"},
		{Name: "AUTO_RUN_WITH_INSTRUCTIONS", Value: boolString(task.Spec.Instructions)},
		{Name: "TASKRUN_REPO_URL", Value: task.Spec.RepoURL},
		{Name: "TASKRUN_BASE_BRANCH", Value: valueOr(task.Spec.BaseBranch, "main")},
		{Name: "TASKRUN_WORKING_BRANCH", Value: valueOr(task.Spec.WorkingBranch, task.Name)},
		{Name: "TASKRUN_REQUIREMENT_FILE_PATH", Value: task.Spec.RequirementFilePath},
		{Name: "TASKRUN_WORKSPACE_PATH", Value: valueOr(task.Spec.WorkspacePath, "/workspace/repo")},
		{Name: "TASKRUN_STARTUP_CONTRACT_VERSION", Value: valueOr(task.Spec.StartupContractVersion, "v1")},
		{Name: "FORGEJO_USERNAME", Value: drone.Spec.ForgejoUsername},
		{Name: "GIT_AUTHOR_NAME", Value: valueOr(drone.Spec.Forgejo.DisplayName, drone.Spec.ForgejoUsername)},
		{Name: "GIT_AUTHOR_EMAIL", Value: valueOr(drone.Spec.Forgejo.Email, fmt.Sprintf("%s@vinculum.local", drone.Spec.ForgejoUsername))},
		{Name: "AUTO_COMMIT_MESSAGE", Value: taskCommitMessage(task)},
	}
	for k, v := range drone.Spec.Env {
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}
	if task.Spec.RepoURL != "" {
		requirementFile := task.Spec.RequirementFilePath
		if requirementFile != "" {
			requirementFile = "/workspace/repo/" + strings.TrimPrefix(filepath.ToSlash(requirementFile), "/")
		}
		env[5].Value = fmt.Sprintf("You are already in the correct repository checkout at %s on branch %s from base branch %s. Your role is %s. Requirement source of truth: %s. Use the requirement file as the authoritative specification, use tea for Forgejo operations when helpful, run relevant verification if available, and leave the repository in a good state for commit and push.\n\n%s", valueOr(task.Spec.WorkspacePath, "/workspace/repo"), valueOr(task.Spec.WorkingBranch, task.Name), valueOr(task.Spec.BaseBranch, "main"), valueOr(task.Spec.Role, "coder"), valueOr(requirementFile, "not provided"), task.Spec.Prompt)
	}
	volumes := []corev1.Volume{{Name: "workspace", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
	mounts := []corev1.VolumeMount{{Name: "workspace", MountPath: "/workspace"}}
	if instructionConfigMapName(&drone) != "" {
		volumes = append(volumes, corev1.Volume{Name: "instructions", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: instructionConfigMapName(&drone)}}}})
		mounts = append(mounts, corev1.VolumeMount{Name: "instructions", MountPath: valueOr(drone.Spec.InstructionMountPath, "/instructions"), ReadOnly: true})
	}
	sshSecretName := sshSecretName(&drone)
	if sshSecretName != "" {
		volumes = append(volumes,
			corev1.Volume{Name: "ssh-source", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: sshSecretName}}},
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
		env = append(env, corev1.EnvVar{Name: "GIT_SSH_COMMAND", Value: "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i /home/agent/.ssh/id_ed25519"})
	}
	if providerSecretName(&drone) != "" {
		volumes = append(volumes, corev1.Volume{Name: "provider-auth", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: providerSecretName(&drone)}}})
		mounts = append(mounts, corev1.VolumeMount{Name: "provider-auth", MountPath: "/home/agent/.local/share/opencode/auth.json", SubPath: providerAuthFileKey(&drone), ReadOnly: true})
	}
	if tokenSecretName(&drone) != "" {
		env = append(env, corev1.EnvVar{Name: "FORGEJO_TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: tokenSecretName(&drone)}, Key: "token"}}})
	}
	return batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: task.Namespace, Labels: map[string]string{"vinculum.dev/taskrun": task.Name, "vinculum.dev/drone": drone.Name}},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"vinculum.dev/taskrun": task.Name, "vinculum.dev/drone": drone.Name}},
				Spec: corev1.PodSpec{
					RestartPolicy:  corev1.RestartPolicyNever,
					InitContainers: initContainers,
					Containers: []corev1.Container{{
						Name:            "drone",
						Image:           drone.Spec.Image,
						ImagePullPolicy: corev1.PullAlways,
						Command:         cmd,
						Env:             env,
						VolumeMounts:    mounts,
					}},
					Volumes: volumes,
				},
			},
		},
	}
}

func (r *TaskRunReconciler) ensureDroneAssets(ctx context.Context, drone *v1alpha1.Drone) error {
	if drone.Spec.InstructionInline != nil {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: instructionConfigMapName(drone), Namespace: drone.Namespace}}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
			cm.Data = map[string]string{instructionFileName(drone): drone.Spec.InstructionInline.Content}
			return controllerutil.SetControllerReference(drone, cm, r.Scheme)
		})
		if err != nil {
			return err
		}
	}
	if drone.Spec.ProviderAuthInline != nil {
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: providerSecretName(drone), Namespace: drone.Namespace}}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
			secret.Type = corev1.SecretTypeOpaque
			secret.StringData = map[string]string{providerAuthFileKey(drone): drone.Spec.ProviderAuthInline.Content}
			return controllerutil.SetControllerReference(drone, secret, r.Scheme)
		})
		if err != nil {
			return err
		}
	}
	if name := instructionConfigMapName(drone); name != "" {
		var cm corev1.ConfigMap
		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: drone.Namespace}, &cm); err != nil {
			return err
		}
	}
	if name := providerSecretName(drone); name != "" {
		var secret corev1.Secret
		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: drone.Namespace}, &secret); err != nil {
			return err
		}
	}
	return nil
}

func (r *TaskRunReconciler) updateDroneStatus(ctx context.Context, drone *v1alpha1.Drone, taskName string, delta int32) error {
	current := drone.DeepCopyObject().(*v1alpha1.Drone)
	current.Status.ActiveTasks += delta
	if current.Status.ActiveTasks < 0 {
		current.Status.ActiveTasks = 0
	}
	now := metav1.Now()
	current.Status.LastSeen = &now
	current.Status.Phase = "Idle"
	if current.Status.ActiveTasks > 0 {
		current.Status.Phase = "Busy"
	}
	assigned := make([]string, 0, len(current.Status.Assigned))
	seen := false
	for _, item := range current.Status.Assigned {
		if item == taskName {
			seen = true
			if delta < 0 {
				continue
			}
		}
		assigned = append(assigned, item)
	}
	if delta > 0 && !seen {
		assigned = append(assigned, taskName)
	}
	current.Status.Assigned = assigned
	if err := r.Status().Update(ctx, current); err != nil {
		if apierrors.IsConflict(err) {
			return nil
		}
		return err
	}
	return nil
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func instructionConfigMapName(drone *v1alpha1.Drone) string {
	if drone.Spec.InstructionInline != nil {
		return drone.Name + "-instructions"
	}
	return drone.Spec.InstructionConfigMap
}

func instructionFileName(drone *v1alpha1.Drone) string {
	if drone.Spec.InstructionInline != nil && strings.TrimSpace(drone.Spec.InstructionInline.FileName) != "" {
		return drone.Spec.InstructionInline.FileName
	}
	return "AGENT.md"
}

func providerSecretName(drone *v1alpha1.Drone) string {
	if drone.Spec.ProviderAuthInline != nil {
		return drone.Name + "-provider-auth"
	}
	if drone.Spec.ProviderSecretRef != nil {
		return drone.Spec.ProviderSecretRef.Name
	}
	return ""
}

func providerAuthFileKey(drone *v1alpha1.Drone) string {
	if drone.Spec.ProviderAuthInline != nil && strings.TrimSpace(drone.Spec.ProviderAuthInline.FileKey) != "" {
		return drone.Spec.ProviderAuthInline.FileKey
	}
	if strings.TrimSpace(drone.Spec.ProviderAuthFileKey) != "" {
		return drone.Spec.ProviderAuthFileKey
	}
	return "auth.json"
}

func sshSecretName(drone *v1alpha1.Drone) string {
	if drone.Spec.SSHKeySecretRef != nil {
		return drone.Spec.SSHKeySecretRef.Name
	}
	if drone.Status.SSHSecretName != "" {
		return drone.Status.SSHSecretName
	}
	if drone.Spec.Forgejo.AutoProvision {
		return generatedSSHSecretName(drone)
	}
	return ""
}

func tokenSecretName(drone *v1alpha1.Drone) string {
	if drone.Status.ForgejoTokenSecretName != "" {
		return drone.Status.ForgejoTokenSecretName
	}
	if drone.Spec.Forgejo.AutoProvision {
		return generatedTokenSecretName(drone)
	}
	return ""
}

func int32ptr(v int32) *int32 { return &v }

func (r *TaskRunReconciler) ensureAutomatedReview(ctx context.Context, task *v1alpha1.TaskRun) (*v1alpha1.Review, error) {
	reviewName := task.Name + "-auto-review"
	var review v1alpha1.Review
	if err := r.Get(ctx, types.NamespacedName{Name: reviewName, Namespace: task.Namespace}, &review); err == nil {
		return &review, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}
	reviewerDrone, err := r.selectSupportDrone(ctx, task.Namespace, task.Spec.RepositoryRef, []string{"reviewer"}, nil)
	reviewerName := ""
	if err == nil && reviewerDrone != nil {
		reviewerName = reviewerDrone.Name
	}
	review = v1alpha1.Review{
		TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Review"},
		ObjectMeta: metav1.ObjectMeta{Name: reviewName, Namespace: task.Namespace},
		Spec: v1alpha1.ReviewSpec{
			RequirementRef:      task.Spec.RequirementRef,
			RequirementFilePath: task.Spec.RequirementFilePath,
			TaskRef:             task.Name,
			RepositoryRef:       task.Spec.RepositoryRef,
			ReviewerDroneRef:    reviewerName,
			PullRequestURL:      task.Status.PullRequestURL,
			Automated:           true,
			Summary:             automatedReviewSummary(reviewerName),
		},
	}
	if err := r.Create(ctx, &review); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if err := r.Get(ctx, types.NamespacedName{Name: reviewName, Namespace: task.Namespace}, &review); err != nil {
				return nil, err
			}
			return &review, nil
		}
		return nil, err
	}
	return &review, nil
}

func (r *TaskRunReconciler) ensurePullRequest(ctx context.Context, task *v1alpha1.TaskRun) error {
	if task.Spec.RepositoryRef == "" || r.Forgejo == nil {
		return nil
	}
	var repo v1alpha1.ForgejoRepository
	if err := r.Get(ctx, types.NamespacedName{Name: task.Spec.RepositoryRef, Namespace: task.Namespace}, &repo); err != nil {
		return err
	}
	pr, err := r.ensurePullRequestAsAssignedDrone(ctx, task, &repo)
	if err != nil {
		return fmt.Errorf("job succeeded but pull request creation failed: %w", err)
	}
	task.Status.PullRequestURL = pr.HTMLURL
	task.Status.PullRequestNumber = pr.Number
	task.Status.Branch = valueOr(task.Spec.WorkingBranch, task.Name)
	return nil
}

func (r *TaskRunReconciler) ensurePullRequestAsAssignedDrone(ctx context.Context, task *v1alpha1.TaskRun, repo *v1alpha1.ForgejoRepository) (forgejo.PullRequest, error) {
	assignedDrone := strings.TrimSpace(task.Status.AssignedDrone)
	if assignedDrone == "" {
		assignedDrone = strings.TrimSpace(task.Spec.DroneRef)
	}
	if assignedDrone == "" {
		return forgejo.PullRequest{}, fmt.Errorf("no assigned drone available for pull request creation")
	}
	var drone v1alpha1.Drone
	if err := r.Get(ctx, types.NamespacedName{Name: assignedDrone, Namespace: task.Namespace}, &drone); err != nil {
		return forgejo.PullRequest{}, err
	}
	secretName := strings.TrimSpace(drone.Status.ForgejoTokenSecretName)
	if secretName == "" {
		return forgejo.PullRequest{}, fmt.Errorf("drone %s has no Forgejo token secret", drone.Name)
	}
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: task.Namespace}, &secret); err != nil {
		return forgejo.PullRequest{}, err
	}
	token := strings.TrimSpace(string(secret.Data["token"]))
	if token == "" {
		return forgejo.PullRequest{}, fmt.Errorf("drone %s Forgejo token secret is empty", drone.Name)
	}
	return r.Forgejo.EnsurePullRequestWithToken(ctx, token, repo.Spec.Owner, repo.Spec.Name, valueOr(task.Spec.WorkingBranch, task.Name), valueOr(task.Spec.BaseBranch, "main"), taskPullRequestTitle(task), taskPullRequestBody(task))
}

func (r *TaskRunReconciler) syncPullRequestState(ctx context.Context, task *v1alpha1.TaskRun) (bool, error) {
	if r.Forgejo == nil || task.Spec.RepositoryRef == "" || task.Status.PullRequestNumber == 0 {
		return false, nil
	}
	var repo v1alpha1.ForgejoRepository
	if err := r.Get(ctx, types.NamespacedName{Name: task.Spec.RepositoryRef, Namespace: task.Namespace}, &repo); err != nil {
		return false, err
	}
	pr, err := r.Forgejo.GetPullRequest(ctx, repo.Spec.Owner, repo.Spec.Name, task.Status.PullRequestNumber)
	if err != nil {
		return false, err
	}
	updated := task.DeepCopyObject().(*v1alpha1.TaskRun)
	changed := false
	if pr.Merged && updated.Status.Phase != "Merged" {
		updated.Status.Phase = "Merged"
		now := metav1.Now()
		updated.Status.FinishedAt = &now
		changed = true
	}
	if pr.HTMLURL != "" && updated.Status.PullRequestURL != pr.HTMLURL {
		updated.Status.PullRequestURL = pr.HTMLURL
		changed = true
	}
	if !changed {
		return false, nil
	}
	if err := r.Status().Update(ctx, updated); err != nil {
		if apierrors.IsConflict(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func taskCommitMessage(task v1alpha1.TaskRun) string {
	if task.Spec.RequirementFilePath != "" {
		return fmt.Sprintf("Implement %s", filepath.Base(task.Spec.RequirementFilePath))
	}
	if task.Spec.RequirementRef != "" {
		return fmt.Sprintf("Implement %s", task.Spec.RequirementRef)
	}
	return fmt.Sprintf("Implement %s", task.Name)
}

func taskPullRequestTitle(task *v1alpha1.TaskRun) string {
	if task.Spec.RequirementFilePath != "" {
		return fmt.Sprintf("Implement %s", filepath.Base(task.Spec.RequirementFilePath))
	}
	if task.Spec.RequirementRef != "" {
		return fmt.Sprintf("Implement %s", task.Spec.RequirementRef)
	}
	return fmt.Sprintf("Implement %s", task.Name)
}

func taskPullRequestBody(task *v1alpha1.TaskRun) string {
	parts := []string{
		"## Summary",
		"- Automated Vinculum task execution",
		"- Task: `" + task.Name + "`",
	}
	if task.Spec.RequirementRef != "" {
		parts = append(parts, "- Requirement: `"+task.Spec.RequirementRef+"`")
	}
	if task.Spec.RequirementFilePath != "" {
		parts = append(parts, "- Requirement file: `"+task.Spec.RequirementFilePath+"`")
	}
	parts = append(parts, "", "## Prompt", task.Spec.Prompt)
	return strings.Join(parts, "\n")
}

func automatedReviewSummary(reviewerName string) string {
	if reviewerName != "" {
		return fmt.Sprintf("Automated review requested from reviewer drone %s after verification passed.", reviewerName)
	}
	return "Automated review requested after verification passed."
}
