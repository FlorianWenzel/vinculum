package main

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/florian/vinculum/apps/orchestrator/api/v1alpha1"
)

func TestApplyForgejoPullRequestWebhookMarksTaskMerged(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha1.Task{}).WithObjects(&v1alpha1.Task{
		TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Task"},
		ObjectMeta: metav1.ObjectMeta{Name: "demo-task", Namespace: "vinculum-system"},
		Spec:       v1alpha1.TaskSpec{RepositoryRef: "demo-repo", WorkingBranch: "demo-branch"},
		Status:     v1alpha1.TaskStatus{Phase: "Approved", PullRequestNumber: 7, PullRequestURL: "http://localhost/pr/7"},
	}).Build()

	payload := forgejoWebhookPayload{Action: "closed"}
	payload.Repository.Name = "demo-repo"
	payload.Repository.Owner.Login = "forgejo_admin"
	payload.PullRequest.Number = 7
	payload.PullRequest.Merged = true
	payload.PullRequest.HTMLURL = "http://localhost/pr/7"
	payload.PullRequest.Head.Ref = "demo-branch"

	updated, err := applyForgejoPullRequestWebhook(context.Background(), client, "vinculum-system", payload)
	if err != nil {
		t.Fatalf("apply webhook: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 updated task, got %d", updated)
	}

	var task v1alpha1.Task
	if err := client.Get(context.Background(), ctrlclient.ObjectKey{Name: "demo-task", Namespace: "vinculum-system"}, &task); err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status.Phase != "Merged" {
		t.Fatalf("expected phase Merged, got %s", task.Status.Phase)
	}
}
