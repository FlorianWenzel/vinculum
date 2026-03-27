package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	"github.com/florian/vinculum/apps/orchestrator/internal/sshkeys"
)

type DroneReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Forgejo *forgejo.Client
}

func (r *DroneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Drone{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

func (r *DroneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.FromContext(ctx).Info("reconciling drone", "name", req.Name, "namespace", req.Namespace)
	var drone v1alpha1.Drone
	if err := r.Get(ctx, req.NamespacedName, &drone); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.ensureInlineAssets(ctx, &drone); err != nil {
		return ctrl.Result{}, err
	}
	if drone.Spec.Forgejo.AutoProvision {
		if err := r.ensureForgejoIdentity(ctx, &drone); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	status := drone.DeepCopyObject().(*v1alpha1.Drone)
	status.Status.ActiveTasks = r.computeActiveTasks(ctx, &drone)
	status.Status.Assigned = append([]string(nil), drone.Status.Assigned...)
	status.Status.Phase = "Ready"
	if status.Status.ActiveTasks > 0 {
		status.Status.Phase = "Busy"
	}
	now := metav1.Now()
	status.Status.LastSeen = &now
	if err := r.Status().Update(ctx, status); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *DroneReconciler) computeActiveTasks(ctx context.Context, drone *v1alpha1.Drone) int32 {
	var tasks v1alpha1.TaskRunList
	if err := r.List(ctx, &tasks, client.InNamespace(drone.Namespace)); err != nil {
		return drone.Status.ActiveTasks
	}
	var count int32
	assigned := make([]string, 0)
	for _, task := range tasks.Items {
		if task.Status.AssignedDrone != drone.Name {
			continue
		}
		switch task.Status.Phase {
		case "Assigned", "Coding", "Running":
		default:
			continue
		}
		count++
		assigned = append(assigned, task.Name)
	}
	drone.Status.Assigned = assigned
	return count
}

func (r *DroneReconciler) ensureInlineAssets(ctx context.Context, drone *v1alpha1.Drone) error {
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
	return nil
}

func (r *DroneReconciler) ensureForgejoIdentity(ctx context.Context, drone *v1alpha1.Drone) error {
	email := strings.TrimSpace(drone.Spec.Forgejo.Email)
	if email == "" {
		email = fmt.Sprintf("%s@vinculum.local", drone.Spec.ForgejoUsername)
	}
	display := strings.TrimSpace(drone.Spec.Forgejo.DisplayName)
	if display == "" {
		display = drone.Name
	}
	user, err := r.Forgejo.EnsureUser(ctx, drone.Spec.ForgejoUsername, email, display, drone.Spec.Forgejo.Admin)
	if err != nil {
		return err
	}
	secretName := generatedSSHSecretName(drone)
	var secret corev1.Secret
	err = r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: drone.Namespace}, &secret)
	if apierrors.IsNotFound(err) {
		pair, err := sshkeys.Generate(drone.Spec.ForgejoUsername + "@vinculum")
		if err != nil {
			return err
		}
		secret = corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: drone.Namespace}}
		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, &secret, func() error {
			secret.Type = corev1.SecretTypeOpaque
			secret.StringData = map[string]string{
				"id_ed25519":     pair.PrivatePEM,
				"id_ed25519.pub": pair.PublicKey,
			}
			return controllerutil.SetControllerReference(drone, &secret, r.Scheme)
		})
		if err != nil {
			return err
		}
		if err := r.Forgejo.EnsurePublicKey(ctx, drone.Spec.ForgejoUsername, drone.Name+"-ssh", pair.PublicKey); err != nil {
			return err
		}
		status := drone.DeepCopyObject().(*v1alpha1.Drone)
		status.Status.ForgejoUserID = user.ID
		status.Status.ForgejoReady = true
		status.Status.SSHSecretName = secretName
		status.Status.SSHPublicKey = pair.PublicKey
		status.Status.SSHKeyFingerprint = pair.Fingerprint
		if err := r.ensureTokenSecret(ctx, drone); err != nil {
			return err
		}
		status.Status.ForgejoTokenSecretName = generatedTokenSecretName(drone)
		return r.Status().Update(ctx, status)
	}
	if err != nil {
		return err
	}
	status := drone.DeepCopyObject().(*v1alpha1.Drone)
	status.Status.ForgejoUserID = user.ID
	status.Status.ForgejoReady = true
	status.Status.SSHSecretName = secretName
	if err := r.ensureTokenSecret(ctx, drone); err != nil {
		return err
	}
	status.Status.ForgejoTokenSecretName = generatedTokenSecretName(drone)
	return r.Status().Update(ctx, status)
}

func (r *DroneReconciler) ensureTokenSecret(ctx context.Context, drone *v1alpha1.Drone) error {
	tokenSecretName := generatedTokenSecretName(drone)
	var tokenSecret corev1.Secret
	err := r.Get(ctx, types.NamespacedName{Name: tokenSecretName, Namespace: drone.Namespace}, &tokenSecret)
	if apierrors.IsNotFound(err) {
		token, err := r.Forgejo.CreateTokenAsUser(ctx, drone.Spec.ForgejoUsername, drone.Spec.ForgejoUsername+"-vinculum!", fmt.Sprintf("%s-%d", drone.Name, time.Now().Unix()))
		if err != nil {
			return err
		}
		tokenSecret = corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: tokenSecretName, Namespace: drone.Namespace}}
		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, &tokenSecret, func() error {
			tokenSecret.Type = corev1.SecretTypeOpaque
			tokenSecret.StringData = map[string]string{"token": token}
			return controllerutil.SetControllerReference(drone, &tokenSecret, r.Scheme)
		})
		return err
	}
	return err
}

func generatedSSHSecretName(drone *v1alpha1.Drone) string {
	return drone.Name + "-ssh"
}

func generatedTokenSecretName(drone *v1alpha1.Drone) string {
	return drone.Name + "-forgejo-token"
}
