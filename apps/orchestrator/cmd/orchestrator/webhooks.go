package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/florian/vinculum/apps/orchestrator/api/v1alpha1"
)

type forgejoWebhookPayload struct {
	Action      string `json:"action"`
	PullRequest struct {
		Number  int64  `json:"number"`
		HTMLURL string `json:"html_url"`
		Merged  bool   `json:"merged"`
		Head    struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	} `json:"pull_request"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

func forgejoWebhookHandler(k8s ctrlclient.Client, namespace string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var payload forgejoWebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		log.FromContext(r.Context()).Info("received Forgejo webhook", "action", payload.Action, "repository", payload.Repository.Name, "pullRequest", payload.PullRequest.Number)
		if payload.PullRequest.Number == 0 || payload.Repository.Name == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"updated": 0, "ignored": true})
			return
		}
		updated, err := applyForgejoPullRequestWebhook(r.Context(), k8s, namespace, payload)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"updated": updated})
	}
}

func applyForgejoPullRequestWebhook(ctx context.Context, k8s ctrlclient.Client, namespace string, payload forgejoWebhookPayload) (int, error) {
	listOpts := []ctrlclient.ListOption{}
	if namespace != "" {
		listOpts = append(listOpts, ctrlclient.InNamespace(namespace))
	}
	var tasks v1alpha1.TaskList
	if err := k8s.List(ctx, &tasks, listOpts...); err != nil {
		return 0, err
	}
	updated := 0
	for i := range tasks.Items {
		task := &tasks.Items[i]
		if !matchesWebhookTask(task, payload) {
			continue
		}
		phase := webhookTaskPhase(payload)
		if phase == "" || task.Status.Phase == phase {
			continue
		}
		patched := task.DeepCopyObject().(*v1alpha1.Task)
		patched.Status.Phase = phase
		patched.Status.PullRequestURL = payload.PullRequest.HTMLURL
		patched.Status.PullRequestNumber = payload.PullRequest.Number
		patched.Status.Branch = firstNonEmpty(payload.PullRequest.Head.Ref, patched.Status.Branch, patched.Spec.WorkingBranch)
		if phase == "Merged" {
			now := metav1.Now()
			patched.Status.FinishedAt = &now
		}
		if err := k8s.Status().Update(ctx, patched); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}

func matchesWebhookTask(task *v1alpha1.Task, payload forgejoWebhookPayload) bool {
	if task.Status.PullRequestNumber != 0 && task.Status.PullRequestNumber == payload.PullRequest.Number {
		return true
	}
	if payload.Repository.Name != "" && task.Spec.RepositoryRef != payload.Repository.Name {
		return false
	}
	if payload.PullRequest.Head.Ref != "" && task.Spec.WorkingBranch != "" {
		return strings.EqualFold(task.Spec.WorkingBranch, payload.PullRequest.Head.Ref)
	}
	return false
}

func webhookTaskPhase(payload forgejoWebhookPayload) string {
	switch payload.Action {
	case "opened", "reopened", "synchronize":
		return "InReview"
	case "closed":
		if payload.PullRequest.Merged {
			return "Merged"
		}
		return "Approved"
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
