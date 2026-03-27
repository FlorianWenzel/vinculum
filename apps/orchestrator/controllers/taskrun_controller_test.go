package controllers

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/florian/vinculum/apps/orchestrator/api/v1alpha1"
)

func TestDependenciesReady(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	reconciler := &TaskRunReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&v1alpha1.Task{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Task"},
			ObjectMeta: metav1.ObjectMeta{Name: "task-a", Namespace: "vinculum-system"},
			Status:     v1alpha1.TaskStatus{Phase: "Merged"},
		},
		&v1alpha1.Task{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Task"},
			ObjectMeta: metav1.ObjectMeta{Name: "task-b", Namespace: "vinculum-system"},
			Status:     v1alpha1.TaskStatus{Phase: "Approved"},
		},
	).Build()}

	ready, waitingOn, err := reconciler.dependenciesReady(context.Background(), &v1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task-c", Namespace: "vinculum-system"}, Spec: v1alpha1.TaskSpec{DependsOn: []string{"task-a", "task-b", "task-missing"}}})
	if err != nil {
		t.Fatalf("dependenciesReady: %v", err)
	}
	if ready {
		t.Fatalf("expected dependencies to be blocked")
	}
	if waitingOn == "" {
		t.Fatalf("expected waiting summary to be populated")
	}

	ready, waitingOn, err = reconciler.dependenciesReady(context.Background(), &v1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task-d", Namespace: "vinculum-system"}, Spec: v1alpha1.TaskSpec{DependsOn: []string{"task-a"}}})
	if err != nil {
		t.Fatalf("dependenciesReady merged: %v", err)
	}
	if !ready || waitingOn != "" {
		t.Fatalf("expected merged dependency to be ready, got ready=%t waiting=%q", ready, waitingOn)
	}
}

func TestTaskReconcilerAssignsDroneByRoleWithoutExplicitDroneRef(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.Task{}, &v1alpha1.Drone{}).
		WithObjects(
			&v1alpha1.Task{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Task"},
				ObjectMeta: metav1.ObjectMeta{Name: "demo-task", Namespace: "vinculum-system"},
				Spec: v1alpha1.TaskSpec{
					RepositoryRef: "demo-repo",
					RepoURL:       "ssh://git@example/demo-repo.git",
					Role:          "coder",
					Prompt:        "implement the change",
					BaseBranch:    "main",
					WorkingBranch: "req/demo",
				},
			},
			&v1alpha1.Drone{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Drone"},
				ObjectMeta: metav1.ObjectMeta{Name: "demo-drone", Namespace: "vinculum-system"},
				Spec:       v1alpha1.DroneSpec{Role: "coder", ForgejoUsername: "demo-bot", Image: "ttl.sh/vinculum-agent:12h", Concurrency: 1, Enabled: true},
				Status:     v1alpha1.DroneStatus{ForgejoReady: true, SSHSecretName: "demo-ssh"},
			},
			&v1alpha1.DroneRepositoryAccess{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "DroneRepositoryAccess"},
				ObjectMeta: metav1.ObjectMeta{Name: "demo-access", Namespace: "vinculum-system"},
				Spec:       v1alpha1.DroneRepositoryAccessSpec{DroneRef: "demo-drone", RepositoryRef: "demo-repo", Permission: "write"},
				Status:     v1alpha1.DroneRepositoryAccessStatus{Phase: "Ready"},
			},
		).Build()

	reconciler := &TaskRunReconciler{Client: client, Scheme: scheme}
	request := ctrl.Request{NamespacedName: ctrlclient.ObjectKey{Name: "demo-task", Namespace: "vinculum-system"}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var task v1alpha1.Task
	if err := client.Get(context.Background(), ctrlclient.ObjectKey{Name: "demo-task", Namespace: "vinculum-system"}, &task); err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status.AssignedDrone != "demo-drone" {
		t.Fatalf("expected assigned drone demo-drone, got %q", task.Status.AssignedDrone)
	}
	if task.Status.Phase != "Coding" {
		t.Fatalf("expected phase Coding, got %q", task.Status.Phase)
	}
}
