package controllers

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/florian/vinculum/apps/orchestrator/api/v1alpha1"
	"github.com/florian/vinculum/apps/orchestrator/internal/forgejo"
)

type DroneRepositoryAccessReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Forgejo *forgejo.Client
}

func (r *DroneRepositoryAccessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.FromContext(ctx).Info("reconciling drone repository access", "name", req.Name, "namespace", req.Namespace)
	var access v1alpha1.DroneRepositoryAccess
	if err := r.Get(ctx, req.NamespacedName, &access); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var drone v1alpha1.Drone
	if err := r.Get(ctx, types.NamespacedName{Name: access.Spec.DroneRef, Namespace: access.Namespace}, &drone); err != nil {
		return ctrl.Result{RequeueAfter: 3}, nil
	}
	if !drone.Status.ForgejoReady {
		return ctrl.Result{RequeueAfter: 3}, nil
	}

	var repo v1alpha1.ForgejoRepository
	if err := r.Get(ctx, types.NamespacedName{Name: access.Spec.RepositoryRef, Namespace: access.Namespace}, &repo); err != nil {
		return ctrl.Result{RequeueAfter: 3}, nil
	}
	if repo.Status.Phase != "Ready" {
		return ctrl.Result{RequeueAfter: 3}, nil
	}

	if err := r.Forgejo.EnsureCollaborator(ctx, repo.Spec.Owner, repo.Spec.Name, drone.Spec.ForgejoUsername, access.Spec.Permission); err != nil {
		if err.Error() == "not found" {
			return ctrl.Result{RequeueAfter: 3}, nil
		}
		return ctrl.Result{}, err
	}

	status := access.DeepCopyObject().(*v1alpha1.DroneRepositoryAccess)
	status.Status.Phase = "Ready"
	status.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Reconciled", LastTransitionTime: metav1.Now(), Message: "Repository access granted."}}
	if err := r.Status().Update(ctx, status); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}
