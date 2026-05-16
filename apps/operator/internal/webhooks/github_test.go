package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/florian/vinculum/apps/operator/api/v1alpha1"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func newTrigger(name, agent string, events []string, filter v1alpha1.WebhookFilter, template v1alpha1.TaskTemplate) *v1alpha1.WebhookTrigger {
	return &v1alpha1.WebhookTrigger{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vinculum-system"},
		Spec: v1alpha1.WebhookTriggerSpec{
			Source:       "github",
			Events:       events,
			Filter:       filter,
			SecretRef:    v1alpha1.SecretRef{Name: name + "-secret"},
			AgentRef:     agent,
			TaskTemplate: template,
		},
	}
}

func secret(name, value string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vinculum-system"},
		Data:       map[string][]byte{"secret": []byte(value)},
	}
}

func TestVerifyGitHubSignature(t *testing.T) {
	secret := []byte("topsecret")
	body := []byte(`{"x":1}`)
	good := sign(secret, body)
	if !verifyGitHubSignature(secret, body, good) {
		t.Error("good signature should verify")
	}
	if verifyGitHubSignature(secret, body, "sha256=00") {
		t.Error("wrong hex should not verify")
	}
	if verifyGitHubSignature(secret, body, "sha1=abcd") {
		t.Error("non-sha256 prefix should not verify")
	}
	if verifyGitHubSignature([]byte("other"), body, good) {
		t.Error("different secret should not verify")
	}
}

func TestEventMatches(t *testing.T) {
	if !eventMatches([]string{"push"}, "push", "") {
		t.Error("bare push should match")
	}
	if !eventMatches([]string{"pull_request.opened"}, "pull_request", "opened") {
		t.Error("type.action should match")
	}
	if eventMatches([]string{"pull_request.opened"}, "pull_request", "synchronize") {
		t.Error("action mismatch should not match")
	}
	if !eventMatches([]string{"pull_request"}, "pull_request", "opened") {
		t.Error("bare type should match any action")
	}
	if eventMatches([]string{"push"}, "pull_request", "opened") {
		t.Error("different type should not match")
	}
}

func TestFilterMatches_Branch(t *testing.T) {
	ev := &githubEvent{}
	ev.Ref = "refs/heads/main"
	ev.Repository.FullName = "acme/api"
	if !filterMatches(v1alpha1.WebhookFilter{Branch: "main"}, ev, "push") {
		t.Error("main push branch should match")
	}
	if filterMatches(v1alpha1.WebhookFilter{Branch: "release"}, ev, "push") {
		t.Error("non-matching branch should reject")
	}
}

func TestStampedTaskName(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if got := stampedTaskName("my-trig", "abcdef0123456789", now); !strings.HasPrefix(got, "my-trig-abcdef012345") {
		t.Errorf("name=%q", got)
	}
	if got := stampedTaskName("my-trig", "", now); !strings.HasPrefix(got, "my-trig-") {
		t.Errorf("name=%q (should fall back to timestamp)", got)
	}
}

// E2E-ish: drive the actual HTTP handler with a real GitHub-shaped payload.
func TestHandleGitHub_FullPath(t *testing.T) {
	scheme := newScheme(t)

	trigSecret := "mysecret"
	trig := newTrigger("acme-push", "coder",
		[]string{"push"},
		v1alpha1.WebhookFilter{Repo: "acme/api", Branch: "main"},
		v1alpha1.TaskTemplate{
			Prompt:         "ref ${event.ref} sha ${event.sha} repo ${event.repo}",
			Fresh:          true,
			TimeoutSeconds: 120,
		},
	)
	sec := secret("acme-push-secret", trigSecret)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(trig, sec).
		WithStatusSubresource(&v1alpha1.WebhookTrigger{}).
		Build()

	h := New(fakeClient, "vinculum-system")
	h.Now = func() time.Time { return time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC) }

	payload := map[string]any{
		"ref":   "refs/heads/main",
		"after": "abcdef1234567890",
		"repository": map[string]any{
			"full_name": "acme/api",
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "abc-123-def-456-deadbeef")
	req.Header.Set("X-Hub-Signature-256", sign([]byte(trigSecret), body))

	rec := httptest.NewRecorder()
	h.handleGitHub(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if got := resp["tasksCreated"]; got != float64(1) {
		t.Errorf("tasksCreated=%v want 1", got)
	}

	// Verify a Task with the substituted prompt was actually created.
	var tasks v1alpha1.TaskList
	if err := fakeClient.List(context.Background(), &tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks.Items) != 1 {
		t.Fatalf("want 1 task, got %d", len(tasks.Items))
	}
	tsk := tasks.Items[0]
	if tsk.Spec.AgentRef != "coder" {
		t.Errorf("agentRef=%q", tsk.Spec.AgentRef)
	}
	if !strings.Contains(tsk.Spec.Prompt, "ref refs/heads/main") || !strings.Contains(tsk.Spec.Prompt, "sha abcdef1234567890") || !strings.Contains(tsk.Spec.Prompt, "repo acme/api") {
		t.Errorf("template substitution failed: %q", tsk.Spec.Prompt)
	}
	if !strings.HasPrefix(tsk.Name, "acme-push-abc123def456") {
		t.Errorf("task name = %q (should embed delivery id)", tsk.Name)
	}
	// Verify trigger status was updated.
	var fetched v1alpha1.WebhookTrigger
	_ = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(trig), &fetched)
	if fetched.Status.FireCount != 1 {
		t.Errorf("FireCount=%d", fetched.Status.FireCount)
	}
	if fetched.Status.LastFired == nil {
		t.Error("LastFired should be set")
	}
}

func TestHandleGitHub_BadSignature(t *testing.T) {
	scheme := newScheme(t)
	trig := newTrigger("acme-push", "coder", []string{"push"}, v1alpha1.WebhookFilter{Repo: "acme/api"}, v1alpha1.TaskTemplate{Prompt: "x"})
	sec := secret("acme-push-secret", "rightsecret")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(trig, sec).WithStatusSubresource(&v1alpha1.WebhookTrigger{}).Build()
	h := New(c, "vinculum-system")

	payload := map[string]any{"ref": "refs/heads/main", "repository": map[string]any{"full_name": "acme/api"}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", sign([]byte("wrongsecret"), body))

	rec := httptest.NewRecorder()
	h.handleGitHub(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var tasks v1alpha1.TaskList
	_ = c.List(context.Background(), &tasks)
	if len(tasks.Items) != 0 {
		t.Errorf("bad signature created tasks: %d", len(tasks.Items))
	}
}

func TestHandleGitHub_NoMatchingTrigger(t *testing.T) {
	scheme := newScheme(t)
	// Trigger only listens for pull_request, but we send push.
	trig := newTrigger("acme-prs", "coder", []string{"pull_request"}, v1alpha1.WebhookFilter{}, v1alpha1.TaskTemplate{Prompt: "x"})
	sec := secret("acme-prs-secret", "s")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(trig, sec).WithStatusSubresource(&v1alpha1.WebhookTrigger{}).Build()
	h := New(c, "vinculum-system")

	body, _ := json.Marshal(map[string]any{"ref": "refs/heads/main"})
	req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", sign([]byte("s"), body))
	rec := httptest.NewRecorder()
	h.handleGitHub(rec, req)

	// Accepted but 0 tasks (the response says tasksCreated=0).
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var tasks v1alpha1.TaskList
	_ = c.List(context.Background(), &tasks)
	if len(tasks.Items) != 0 {
		t.Errorf("no-match created tasks: %d", len(tasks.Items))
	}
}

func TestHandleGitHub_Suspended(t *testing.T) {
	scheme := newScheme(t)
	trig := newTrigger("acme-push", "coder", []string{"push"}, v1alpha1.WebhookFilter{}, v1alpha1.TaskTemplate{Prompt: "x"})
	trig.Spec.Suspend = true
	sec := secret("acme-push-secret", "s")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(trig, sec).WithStatusSubresource(&v1alpha1.WebhookTrigger{}).Build()
	h := New(c, "vinculum-system")

	body, _ := json.Marshal(map[string]any{"ref": "refs/heads/main"})
	req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", sign([]byte("s"), body))
	rec := httptest.NewRecorder()
	h.handleGitHub(rec, req)

	var tasks v1alpha1.TaskList
	_ = c.List(context.Background(), &tasks)
	if len(tasks.Items) != 0 {
		t.Errorf("suspended trigger fired: %d tasks", len(tasks.Items))
	}
}

func TestSubstitute_LeavesUnknownVarsAlone(t *testing.T) {
	got := substitute("hello ${event.repo} and ${unknown}", map[string]string{"event.repo": "acme/api"})
	if got != "hello acme/api and ${unknown}" {
		t.Errorf("got %q", got)
	}
}