package controllers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/florian/vinculum/apps/orchestrator/api/v1alpha1"
	"github.com/florian/vinculum/apps/orchestrator/internal/config"
	"github.com/florian/vinculum/apps/orchestrator/internal/forgejo"
	requirementdoc "github.com/florian/vinculum/apps/orchestrator/internal/requirements"
)

func TestRequirementReconcilerSyncsRepositoryBackedRequirementAndCreatesTask(t *testing.T) {
	requirementContent := "---\ntitle: User authentication\nslug: user-auth\nstatus: TODO\ndepends_on:\n  - foundation-api.md\nbranch: req/user-auth\n---\n\n# Context\n\nImplement login.\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/forgejo_admin/demo-repo/contents/requirements/user-auth.md":
			_, _ = fmt.Fprint(w, `{"type":"file","sha":"abc123","encoding":"base64","content":"LS0tCnRpdGxlOiBVc2VyIGF1dGhlbnRpY2F0aW9uCnNsdWc6IHVzZXItYXV0aApzdGF0dXM6IFRPRE8KZGVwZW5kc19vbjoKICAtIGZvdW5kYXRpb24tYXBpLm1kCmJyYW5jaDogcmVxL3VzZXItYXV0aAotLS0KCiMgQ29udGV4dAoKSW1wbGVtZW50IGxvZ2luLgo="}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.Requirement{}, &v1alpha1.Task{}).
		WithObjects(
			&v1alpha1.Repository{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Repository"},
				ObjectMeta: metav1.ObjectMeta{Name: "demo-repo", Namespace: "vinculum-system"},
				Spec:       v1alpha1.RepositorySpec{Owner: "forgejo_admin", Name: "demo-repo", DefaultBranch: "main", RequirementsPath: "requirements", RequirementBranchPrefix: "req/"},
			},
			&v1alpha1.Requirement{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Requirement"},
				ObjectMeta: metav1.ObjectMeta{Name: "user-auth", Namespace: "vinculum-system"},
				Spec:       v1alpha1.RequirementSpec{RepositoryRef: "demo-repo", FilePath: "requirements/user-auth.md"},
			},
			&v1alpha1.Requirement{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Requirement"},
				ObjectMeta: metav1.ObjectMeta{Name: "foundation-api", Namespace: "vinculum-system"},
				Spec:       v1alpha1.RequirementSpec{RepositoryRef: "demo-repo", FilePath: "requirements/foundation-api.md"},
				Status:     v1alpha1.RequirementStatus{Phase: "Completed"},
			},
		).Build()

	reconciler := &RequirementReconciler{
		Client: client,
		Scheme: scheme,
		Forgejo: forgejo.NewClient(config.Config{
			ForgejoBaseURL:   server.URL,
			ForgejoAdminUser: "forgejo_admin",
			ForgejoAdminPass: "forgejo_admin",
		}),
	}
	request := ctrl.Request{NamespacedName: ctrlclient.ObjectKey{Name: "user-auth", Namespace: "vinculum-system"}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var requirement v1alpha1.Requirement
	if err := client.Get(context.Background(), ctrlclient.ObjectKey{Name: "user-auth", Namespace: "vinculum-system"}, &requirement); err != nil {
		t.Fatalf("get requirement: %v", err)
	}
	if requirement.Status.ObservedTitle != "User authentication" {
		t.Fatalf("expected observed title, got %q", requirement.Status.ObservedTitle)
	}
	if requirement.Status.ObservedBranch != "req/user-auth" {
		t.Fatalf("expected observed branch req/user-auth, got %q", requirement.Status.ObservedBranch)
	}
	if len(requirement.Status.ObservedDependsOn) != 1 || requirement.Status.ObservedDependsOn[0] != "requirements/foundation-api.md" {
		t.Fatalf("unexpected observed dependencies %#v", requirement.Status.ObservedDependsOn)
	}

	var task v1alpha1.Task
	if err := client.Get(context.Background(), ctrlclient.ObjectKey{Name: "user-auth-coder", Namespace: "vinculum-system"}, &task); err != nil {
		t.Fatalf("get created task: %v", err)
	}
	if task.Spec.RequirementFilePath != "requirements/user-auth.md" {
		t.Fatalf("unexpected requirement file path %q", task.Spec.RequirementFilePath)
	}
	if task.Spec.WorkingBranch != "req/user-auth" {
		t.Fatalf("unexpected task branch %q", task.Spec.WorkingBranch)
	}
	if task.Spec.StartupContractVersion != "v1" {
		t.Fatalf("unexpected startup contract version %q", task.Spec.StartupContractVersion)
	}
	parsed, err := requirementdoc.ParseDocument("requirements/user-auth.md", []byte(requirementContent))
	if err != nil {
		t.Fatalf("parse requirement content: %v", err)
	}
	if task.Spec.Prompt == "" || task.Spec.Prompt != buildTaskPrompt(requirement, parsed) {
		t.Fatalf("expected prompt to be derived from requirement file")
	}
}
