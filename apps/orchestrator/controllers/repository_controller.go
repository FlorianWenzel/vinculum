package controllers

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/florian/vinculum/apps/orchestrator/api/v1alpha1"
	"github.com/florian/vinculum/apps/orchestrator/internal/forgejo"
)

type ForgejoRepositoryReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Forgejo    *forgejo.Client
	WebhookURL string
}

func (r *ForgejoRepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ForgejoRepository{}).
		Complete(r)
}

func (r *ForgejoRepositoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.FromContext(ctx).Info("reconciling forgejo repository", "name", req.Name, "namespace", req.Namespace)
	var repoCR v1alpha1.ForgejoRepository
	if err := r.Get(ctx, req.NamespacedName, &repoCR); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	repo, err := r.Forgejo.EnsureRepository(ctx, repoCR.Spec.Owner, repoCR.Spec.Name, repoCR.Spec.Description, valueOr(repoCR.Spec.DefaultBranch, "main"), repoCR.Spec.Private, repoCR.Spec.AutoInit)
	if err != nil {
		if err.Error() == "not found" {
			return ctrl.Result{RequeueAfter: 5}, nil
		}
		return ctrl.Result{}, err
	}
	webhookReady := false
	if r.WebhookURL != "" {
		if _, err := r.Forgejo.EnsureRepositoryWebhook(ctx, repoCR.Spec.Owner, repoCR.Spec.Name, r.WebhookURL); err != nil {
			return ctrl.Result{}, err
		}
		webhookReady = true
	}
	status := repoCR.DeepCopyObject().(*v1alpha1.ForgejoRepository)
	status.Status.Phase = "Ready"
	status.Status.HTTPURL = repo.CloneURL
	status.Status.SSHURL = r.Forgejo.SSHCloneURL(repoCR.Spec.Owner, repoCR.Spec.Name)
	status.Status.WebhookURL = r.WebhookURL
	status.Status.WebhookReady = webhookReady
	status.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Reconciled", LastTransitionTime: metav1.Now(), Message: "Repository and access are ready."}}
	if err := r.Status().Update(ctx, status); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}
