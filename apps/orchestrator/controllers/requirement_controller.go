package controllers

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/florian/vinculum/apps/orchestrator/api/v1alpha1"
	"github.com/florian/vinculum/apps/orchestrator/internal/forgejo"
	requirementdoc "github.com/florian/vinculum/apps/orchestrator/internal/requirements"
)

type RequirementReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Forgejo *forgejo.Client
}

func (r *RequirementReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.FromContext(ctx).Info("reconciling requirement", "name", req.Name, "namespace", req.Namespace)
	var requirement v1alpha1.Requirement
	if err := r.Get(ctx, req.NamespacedName, &requirement); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	parsed, repo, revision, checksum, syncErr := r.loadRequirementDocument(ctx, &requirement)
	if syncErr != nil {
		updated := requirement.DeepCopyObject().(*v1alpha1.Requirement)
		updated.Status.Phase = "SyncError"
		updated.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionFalse, Reason: "SyncError", LastTransitionTime: metav1.Now(), Message: syncErr.Error()}}
		if err := r.Status().Update(ctx, updated); err != nil && !apierrors.IsConflict(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10}, nil
	}

	var tasks v1alpha1.TaskList
	_ = r.List(ctx, &tasks, client.InNamespace(requirement.Namespace))
	taskRefs := make([]string, 0)
	allMerged := true
	anyInProgress := false
	for _, task := range tasks.Items {
		if task.Spec.RequirementRef != requirement.Name {
			continue
		}
		taskRefs = append(taskRefs, task.Name)
		switch task.Status.Phase {
		case "Merged", "Succeeded":
		case "", "Planned", "Assigned", "Coding", "Running", "InReview", "Approved", "WaitingOnDependencies", "ChangesRequested", "Blocked", "Testing":
			allMerged = false
			anyInProgress = true
		default:
			allMerged = false
		}
	}
	sort.Strings(taskRefs)

	readyForTask, dependencyTaskRefs, dependencySummary := r.resolveRequirementDependencies(ctx, &requirement, repo, parsed)
	shouldPlan := requirementNeedsTask(parsed.Status)
	if shouldPlan && len(taskRefs) == 0 && readyForTask {
		taskName := requirement.Name + "-coder"
		task := &v1alpha1.Task{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Task"},
			ObjectMeta: metav1.ObjectMeta{Name: taskName, Namespace: requirement.Namespace},
			Spec: v1alpha1.TaskSpec{
				RequirementRef:         requirement.Name,
				RequirementFilePath:    requirement.Spec.FilePath,
				RepositoryRef:          requirement.Spec.RepositoryRef,
				BaseBranch:             repositoryBaseBranch(repo),
				WorkingBranch:          parsed.Branch,
				WorkspacePath:          "/workspace/repo",
				StartupContractVersion: "v1",
				DependsOn:              dependencyTaskRefs,
				Role:                   "coder",
				Prompt:                 buildTaskPrompt(requirement, parsed),
				Instructions:           true,
			},
		}
		if err := r.Create(ctx, task); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, err
		}
		taskRefs = append(taskRefs, taskName)
		allMerged = false
		anyInProgress = true
	}

	updated := requirement.DeepCopyObject().(*v1alpha1.Requirement)
	updated.Status.ObservedRevision = revision
	updated.Status.ContentChecksum = checksum
	updated.Status.ObservedTitle = parsed.Title
	updated.Status.ObservedSlug = parsed.Slug
	updated.Status.ObservedStatus = parsed.Status
	updated.Status.ObservedBranch = parsed.Branch
	updated.Status.ObservedDependsOn = normalizeDependencies(repo, parsed.DependsOn)
	updated.Status.TaskRefs = taskRefs
	for _, task := range tasks.Items {
		if task.Spec.RequirementRef != requirement.Name {
			continue
		}
		if task.Status.PullRequestURL != "" {
			updated.Status.PullRequestURL = task.Status.PullRequestURL
			updated.Status.PullRequestNumber = task.Status.PullRequestNumber
			break
		}
	}
	switch {
	case !readyForTask && shouldPlan:
		updated.Status.Phase = "WaitingOnDependencies"
	case allMerged && len(taskRefs) > 0:
		updated.Status.Phase = "Completed"
	case anyInProgress:
		updated.Status.Phase = "InProgress"
	case parsed.Status == "DONE":
		updated.Status.Phase = "Done"
	case parsed.Status == "IN_REVIEW":
		updated.Status.Phase = "InReview"
	case shouldPlan:
		updated.Status.Phase = "Ready"
	default:
		updated.Status.Phase = strings.Title(strings.ToLower(parsed.Status))
	}
	message := "Requirement metadata was synced from the repository file."
	if dependencySummary != "" && !readyForTask {
		message = dependencySummary
	}
	updated.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Reconciled", LastTransitionTime: metav1.Now(), Message: message}}
	if err := r.Status().Update(ctx, updated); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *RequirementReconciler) loadRequirementDocument(ctx context.Context, requirement *v1alpha1.Requirement) (requirementdoc.Document, *v1alpha1.Repository, string, string, error) {
	var repo v1alpha1.Repository
	if err := r.Get(ctx, types.NamespacedName{Name: requirement.Spec.RepositoryRef, Namespace: requirement.Namespace}, &repo); err != nil {
		return requirementdoc.Document{}, nil, "", "", err
	}
	if r.Forgejo == nil {
		return requirementdoc.Document{}, nil, "", "", fmt.Errorf("forgejo client is not configured")
	}
	content, sha, err := r.Forgejo.GetFile(ctx, repo.Spec.Owner, repo.Spec.Name, requirement.Spec.FilePath, repositoryBaseBranch(&repo))
	if err != nil {
		return requirementdoc.Document{}, &repo, "", "", err
	}
	parsed, err := requirementdoc.ParseDocument(requirement.Spec.FilePath, content)
	if err != nil {
		return requirementdoc.Document{}, &repo, sha, requirementdoc.Checksum(content), err
	}
	if strings.TrimSpace(parsed.Branch) == "" {
		parsed.Branch = repositoryBranchPrefix(&repo) + parsed.Slug
	}
	return parsed, &repo, sha, requirementdoc.Checksum(content), nil
}

func (r *RequirementReconciler) resolveRequirementDependencies(ctx context.Context, requirement *v1alpha1.Requirement, repo *v1alpha1.Repository, parsed requirementdoc.Document) (bool, []string, string) {
	dependsOn := normalizeDependencies(repo, parsed.DependsOn)
	if len(dependsOn) == 0 {
		return true, nil, ""
	}
	var requirements v1alpha1.RequirementList
	if err := r.List(ctx, &requirements, client.InNamespace(requirement.Namespace)); err != nil {
		return false, nil, err.Error()
	}
	deps := make([]string, 0)
	missing := make([]string, 0)
	waiting := make([]string, 0)
	for _, depPath := range dependsOn {
		dep := requirementByFilePath(requirements.Items, requirement.Spec.RepositoryRef, depPath)
		if dep == nil {
			missing = append(missing, depPath)
			continue
		}
		if dep.Status.Phase == "Completed" || dep.Status.Phase == "Done" {
			continue
		}
		if len(dep.Status.TaskRefs) == 0 {
			waiting = append(waiting, depPath)
			continue
		}
		deps = append(deps, dep.Status.TaskRefs...)
	}
	sort.Strings(deps)
	if len(missing) > 0 {
		return false, deps, "Requirement is waiting for dependency CRs: " + strings.Join(missing, ", ")
	}
	if len(waiting) > 0 {
		return false, deps, "Requirement is waiting on dependencies: " + strings.Join(waiting, ", ")
	}
	return true, deps, ""
}

func buildTaskPrompt(requirement v1alpha1.Requirement, parsed requirementdoc.Document) string {
	parts := []string{
		"Requirement source of truth: /workspace/repo/" + filepath.ToSlash(requirement.Spec.FilePath),
		"Requirement title: " + parsed.Title,
		"Requirement status: " + parsed.Status,
		"Requirement branch: " + parsed.Branch,
		"The repository is already checked out in /workspace/repo on the correct requirement branch.",
		"Use the requirement file as the authoritative specification. Implement the requirement, run relevant verification where practical, and leave the repository in a good state for commit and push.",
	}
	if len(parsed.DependsOn) > 0 {
		parts = append(parts, "Declared dependencies:\n- "+strings.Join(parsed.DependsOn, "\n- "))
	}
	if strings.TrimSpace(parsed.Body) != "" {
		parts = append(parts, "Requirement body:\n"+parsed.Body)
	}
	return strings.Join(parts, "\n\n")
}

func requirementByFilePath(requirements []v1alpha1.Requirement, repositoryRef, filePath string) *v1alpha1.Requirement {
	for i := range requirements {
		if requirements[i].Spec.RepositoryRef == repositoryRef && filepath.ToSlash(requirements[i].Spec.FilePath) == filepath.ToSlash(filePath) {
			return &requirements[i]
		}
	}
	return nil
}

func normalizeDependencies(repo *v1alpha1.Repository, dependsOn []string) []string {
	if repo == nil {
		return append([]string(nil), dependsOn...)
	}
	requirementsPath := repositoryRequirementsPath(repo)
	normalized := make([]string, 0, len(dependsOn))
	for _, item := range dependsOn {
		value := requirementdoc.NormalizeDependencyPath(requirementsPath, item)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func repositoryRequirementsPath(repo *v1alpha1.Repository) string {
	if repo == nil || strings.TrimSpace(repo.Spec.RequirementsPath) == "" {
		return "requirements"
	}
	return filepath.ToSlash(strings.Trim(strings.TrimSpace(repo.Spec.RequirementsPath), "/"))
}

func repositoryBaseBranch(repo *v1alpha1.Repository) string {
	if repo != nil && strings.TrimSpace(repo.Spec.DefaultBaseBranch) != "" {
		return strings.TrimSpace(repo.Spec.DefaultBaseBranch)
	}
	if repo != nil && strings.TrimSpace(repo.Spec.DefaultBranch) != "" {
		return strings.TrimSpace(repo.Spec.DefaultBranch)
	}
	return "main"
}

func repositoryBranchPrefix(repo *v1alpha1.Repository) string {
	if repo != nil && strings.TrimSpace(repo.Spec.RequirementBranchPrefix) != "" {
		return strings.TrimSpace(repo.Spec.RequirementBranchPrefix)
	}
	return "req/"
}

func requirementNeedsTask(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "TODO", "IN_PROGRESS":
		return true
	default:
		return false
	}
}
