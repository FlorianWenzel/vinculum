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

func TestReviewReconcilerAutoApprovesAutomatedReview(t *testing.T) {
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
		WithStatusSubresource(&v1alpha1.Task{}, &v1alpha1.Review{}, &batchv1.Job{}).
		WithObjects(
			&v1alpha1.Task{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Task"},
				ObjectMeta: metav1.ObjectMeta{Name: "demo-task", Namespace: "vinculum-system"},
				Spec:       v1alpha1.TaskSpec{RepositoryRef: "demo-repo", WorkingBranch: "demo-branch"},
				Status:     v1alpha1.TaskStatus{Phase: "InReview", VerificationPhase: "Passed", PullRequestURL: "http://localhost/pr/1"},
			},
			&v1alpha1.Review{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Review"},
				ObjectMeta: metav1.ObjectMeta{Name: "demo-review", Namespace: "vinculum-system"},
				Spec:       v1alpha1.ReviewSpec{TaskRef: "demo-task", RepositoryRef: "demo-repo", Automated: true, ReviewerDroneRef: "reviewer-1"},
				Status:     v1alpha1.ReviewStatus{JobName: "demo-review-runner", ReviewerDrone: "reviewer-1", Phase: "Running"},
			},
			&v1alpha1.Drone{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Drone"},
				ObjectMeta: metav1.ObjectMeta{Name: "reviewer-1", Namespace: "vinculum-system"},
				Spec:       v1alpha1.DroneSpec{Role: "reviewer", ForgejoUsername: "reviewer-1", Image: "ttl.sh/reviewer:1", Concurrency: 1, Enabled: true},
				Status:     v1alpha1.DroneStatus{ForgejoReady: true},
			},
			&batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "demo-review-runner", Namespace: "vinculum-system"},
				Status:     batchv1.JobStatus{Succeeded: 1},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "demo-review-runner-pod", Namespace: "vinculum-system", Labels: map[string]string{"job-name": "demo-review-runner"}},
				Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
					Name:  "reviewer",
					State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0, Message: `{"verdict":"Approve","summary":"Looks good."}`}},
				}}},
			},
		).Build()

	reconciler := &ReviewReconciler{Client: client, Scheme: scheme}
	request := ctrl.Request{NamespacedName: ctrlclient.ObjectKey{Name: "demo-review", Namespace: "vinculum-system"}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	var review v1alpha1.Review
	if err := client.Get(context.Background(), ctrlclient.ObjectKey{Name: "demo-review", Namespace: "vinculum-system"}, &review); err != nil {
		t.Fatalf("get review: %v", err)
	}
	if review.Spec.Verdict != "Approve" {
		t.Fatalf("expected automated verdict Approve, got %s", review.Spec.Verdict)
	}

	var task v1alpha1.Task
	if err := client.Get(context.Background(), ctrlclient.ObjectKey{Name: "demo-task", Namespace: "vinculum-system"}, &task); err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status.Phase != "Approved" {
		t.Fatalf("expected task phase Approved, got %s", task.Status.Phase)
	}
}
