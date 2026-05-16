// Package webhooks handles inbound webhook deliveries — currently
// GitHub — and stamps matching Tasks from WebhookTrigger templates.
//
// The handler intentionally lives outside the controller-runtime reconcile
// loop because webhooks are synchronous (return 2xx within seconds or
// GitHub considers the delivery failed). Reconcilers would add latency
// without giving us anything useful in return.
package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/florian/vinculum/apps/operator/api/v1alpha1"
)

// Handler exposes HTTP handlers that the operator's API mux mounts. It is
// a thin object: state lives entirely in the k8s API.
type Handler struct {
	K8s       client.Client
	Namespace string // operator's watch namespace; empty means cluster-wide
	Now       func() time.Time
}

// New returns a Handler. Tests inject a fake client + a fixed `now`.
func New(k8s client.Client, namespace string) *Handler {
	return &Handler{K8s: k8s, Namespace: namespace, Now: time.Now}
}

// MountOn registers /webhook/github on the given mux.
func (h *Handler) MountOn(mux *http.ServeMux) {
	mux.HandleFunc("/webhook/github", h.handleGitHub)
}

// githubEvent is the slice of a GitHub webhook payload we care about.
// Different event types use different sub-objects; we collect them all in
// one struct rather than have N typed parses.
type githubEvent struct {
	Action     string `json:"action"`
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	PullRequest struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Head   struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	} `json:"pull_request"`
}

func (h *Handler) handleGitHub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	eventType := strings.TrimSpace(r.Header.Get("X-GitHub-Event"))
	if eventType == "" {
		http.Error(w, "missing X-GitHub-Event", http.StatusBadRequest)
		return
	}
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	signature := r.Header.Get("X-Hub-Signature-256")

	var ev githubEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		http.Error(w, "parse payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	triggers, err := h.activeGitHubTriggers(ctx)
	if err != nil {
		http.Error(w, "list triggers: "+err.Error(), http.StatusInternalServerError)
		return
	}

	created := 0
	var firstFailure error
	for i := range triggers {
		t := &triggers[i]
		if t.Spec.Suspend {
			continue
		}
		if !eventMatches(t.Spec.Events, eventType, ev.Action) {
			continue
		}
		if !filterMatches(t.Spec.Filter, &ev, eventType) {
			continue
		}
		// Per-trigger signature check: each WebhookTrigger has its own
		// shared secret in its referenced Secret. We verify ALL matching
		// triggers' signatures rather than picking one — a misconfigured
		// trigger shouldn't get a free pass.
		secret, err := h.loadSecret(ctx, t.Namespace, t.Spec.SecretRef.Name)
		if err != nil {
			h.recordFailure(ctx, t, "SecretLoadFailed", err.Error())
			firstFailure = err
			continue
		}
		if !verifyGitHubSignature(secret, body, signature) {
			h.recordFailure(ctx, t, "BadSignature", "X-Hub-Signature-256 did not match")
			// We respond 401 to the sender because any matching trigger
			// failing verification means the delivery wasn't legitimate
			// for this trigger. GitHub will retry — fine.
			http.Error(w, "signature verification failed", http.StatusUnauthorized)
			return
		}
		// Stamp a Task.
		taskName := stampedTaskName(t.Name, deliveryID, h.Now())
		spec := buildTaskSpec(t, &ev)
		if err := h.createTask(ctx, t.Namespace, taskName, spec); err != nil {
			h.recordFailure(ctx, t, "TaskCreateFailed", err.Error())
			firstFailure = err
			continue
		}
		h.recordSuccess(ctx, t)
		created++
	}
	if created == 0 && firstFailure != nil {
		http.Error(w, firstFailure.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"event":         eventType,
		"action":        ev.Action,
		"tasksCreated":  created,
		"matchedAt":     h.Now().UTC().Format(time.RFC3339),
		"x-delivery-id": deliveryID,
	})
}

func (h *Handler) activeGitHubTriggers(ctx context.Context) ([]v1alpha1.WebhookTrigger, error) {
	var list v1alpha1.WebhookTriggerList
	opts := []client.ListOption{}
	if h.Namespace != "" {
		opts = append(opts, client.InNamespace(h.Namespace))
	}
	if err := h.K8s.List(ctx, &list, opts...); err != nil {
		return nil, err
	}
	out := make([]v1alpha1.WebhookTrigger, 0, len(list.Items))
	for _, t := range list.Items {
		if strings.EqualFold(t.Spec.Source, "github") {
			out = append(out, t)
		}
	}
	return out, nil
}

func (h *Handler) loadSecret(ctx context.Context, ns, name string) ([]byte, error) {
	var s corev1.Secret
	if err := h.K8s.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &s); err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", ns, name, err)
	}
	if v, ok := s.Data["secret"]; ok && len(v) > 0 {
		return v, nil
	}
	// Fallback for users who name the key after the provider convention.
	if v, ok := s.Data["GITHUB_WEBHOOK_SECRET"]; ok && len(v) > 0 {
		return v, nil
	}
	return nil, fmt.Errorf("secret %s/%s has no 'secret' (or 'GITHUB_WEBHOOK_SECRET') key", ns, name)
}

func (h *Handler) createTask(ctx context.Context, ns, name string, spec v1alpha1.TaskSpec) error {
	t := &v1alpha1.Task{
		TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Task"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       spec,
	}
	if err := h.K8s.Create(ctx, t); err != nil {
		// AlreadyExists on retry of the same delivery is fine — log and move on.
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	return nil
}

func (h *Handler) recordSuccess(ctx context.Context, t *v1alpha1.WebhookTrigger) {
	now := metav1.NewTime(h.Now())
	t.Status.LastFired = &now
	t.Status.FireCount++
	t.Status.LastReason = ""
	t.Status.LastMessage = ""
	_ = h.K8s.Status().Update(ctx, t)
}

func (h *Handler) recordFailure(ctx context.Context, t *v1alpha1.WebhookTrigger, reason, message string) {
	t.Status.LastReason = reason
	t.Status.LastMessage = message
	_ = h.K8s.Status().Update(ctx, t)
}

// verifyGitHubSignature checks the X-Hub-Signature-256 header against an
// HMAC-SHA256 of the raw request body keyed by the trigger's secret. The
// expected header value is "sha256=<hex>".
func verifyGitHubSignature(secret, body []byte, header string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	want, err := hex.DecodeString(header[len(prefix):])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), want)
}

// eventMatches returns true if any of the trigger's declared event types
// match the incoming (eventType, action). The match is exact for the bare
// type ("push" matches "push"), or type+action when the trigger specifies
// "pull_request.opened".
func eventMatches(triggerEvents []string, eventType, action string) bool {
	for _, want := range triggerEvents {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		if want == eventType {
			return true
		}
		if action != "" && want == eventType+"."+action {
			return true
		}
	}
	return false
}

func filterMatches(f v1alpha1.WebhookFilter, ev *githubEvent, eventType string) bool {
	if f.Repo != "" && !strings.EqualFold(f.Repo, ev.Repository.FullName) {
		return false
	}
	if f.Branch != "" {
		var branch string
		switch eventType {
		case "push":
			branch = strings.TrimPrefix(ev.Ref, "refs/heads/")
		case "pull_request":
			branch = ev.PullRequest.Base.Ref
		}
		if !strings.EqualFold(f.Branch, branch) {
			return false
		}
	}
	return true
}

// buildTaskSpec applies template-variable substitution to the trigger's
// taskTemplate fields, then turns the result into a TaskSpec targeting
// the trigger's agentRef.
func buildTaskSpec(t *v1alpha1.WebhookTrigger, ev *githubEvent) v1alpha1.TaskSpec {
	vars := map[string]string{
		"event.repo":     ev.Repository.FullName,
		"event.ref":      ev.Ref,
		"event.sha":      ev.After,
		"event.pr.number": fmt.Sprintf("%d", ev.PullRequest.Number),
		"event.pr.title":  ev.PullRequest.Title,
		"event.pr.head":   ev.PullRequest.Head.Ref,
		"event.pr.base":   ev.PullRequest.Base.Ref,
	}
	tpl := t.Spec.TaskTemplate
	tpl.Prompt = substitute(tpl.Prompt, vars)
	if tpl.Git != nil {
		gg := *tpl.Git
		gg.BaseBranch = substitute(gg.BaseBranch, vars)
		gg.HeadBranch = substitute(gg.HeadBranch, vars)
		gg.CommitMessage = substitute(gg.CommitMessage, vars)
		gg.PRTitle = substitute(gg.PRTitle, vars)
		gg.PRBody = substitute(gg.PRBody, vars)
		tpl.Git = &gg
	}
	return tpl.ToSpec(t.Spec.AgentRef)
}

// substitute replaces ${name} occurrences with vars[name]. Unknown vars
// are left intact so the user can spot misspellings in Task.spec.prompt.
func substitute(s string, vars map[string]string) string {
	if s == "" || !strings.Contains(s, "${") {
		return s
	}
	out := s
	for k, v := range vars {
		out = strings.ReplaceAll(out, "${"+k+"}", v)
	}
	return out
}

func stampedTaskName(trigger, deliveryID string, now time.Time) string {
	id := strings.ReplaceAll(deliveryID, "-", "")
	if id == "" {
		id = fmt.Sprintf("%d", now.UnixNano())
	}
	if len(id) > 12 {
		id = id[:12]
	}
	name := fmt.Sprintf("%s-%s", trigger, id)
	if len(name) > 253 {
		name = name[:253]
	}
	return strings.ToLower(name)
}

// Errs that callers might want to assert against.
var ErrUnauthorized = errors.New("webhook: signature verification failed")
