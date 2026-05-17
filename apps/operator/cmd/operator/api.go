package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/florian/vinculum/apps/operator/api/v1alpha1"
	appconfig "github.com/florian/vinculum/apps/operator/internal/config"
	vmetrics "github.com/florian/vinculum/apps/operator/internal/metrics"
	"github.com/florian/vinculum/apps/operator/internal/webhooks"
)

// fromAgentHeader is the HTTP header in-cluster orchestrator agents add to
// POST /api/tasks so the operator can count dispatches without parsing the
// pod's identity off the ServiceAccount token.
const fromAgentHeader = "X-Vinculum-From-Agent"

func newAPIMux(k8s client.Client, namespace string, cfg appconfig.Config) *http.ServeMux {
	mux := http.NewServeMux()

	webhooks.New(k8s, namespace).MountOn(mux)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		jsonOK(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/overview", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		listOpts := scopedList(namespace)
		var agents v1alpha1.AgentList
		var taskList v1alpha1.TaskList
		var schedules v1alpha1.AgentScheduleList
		_ = k8s.List(ctx, &agents, listOpts...)
		_ = k8s.List(ctx, &taskList, listOpts...)
		_ = k8s.List(ctx, &schedules, listOpts...)
		jsonOK(w, map[string]any{
			"agents":    agents.Items,
			"tasks":     taskList.Items,
			"schedules": schedules.Items,
		})
	})

	mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		switch r.Method {
		case http.MethodGet:
			var list v1alpha1.AgentList
			if err := k8s.List(ctx, &list, scopedList(namespace)...); err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, list.Items)
		case http.MethodPost:
			var spec struct {
				Name              string                     `json:"name"`
				Image             string                     `json:"image"`
				Model             string                     `json:"model"`
				Concurrency       int32                      `json:"concurrency"`
				Instructions      string                     `json:"instructions"`
				ProviderSecretRef string                     `json:"providerSecretRef"`
				MCPServers        []v1alpha1.InlineMCPServer `json:"mcpServers"`
				MCPServerRefs     []string                   `json:"mcpServerRefs"`
				Env               map[string]string          `json:"env"`
				Enabled           *bool                      `json:"enabled"`
				WorkspaceSize     string                     `json:"workspaceSize"`
				Orchestrator      bool                       `json:"orchestrator"`
				Peer              *bool                      `json:"peer"`
				Repo              *v1alpha1.AgentRepo        `json:"repo"`
				GitCredentials    *v1alpha1.GitCredentials   `json:"gitCredentials"`
				AllowedTools      []string                   `json:"allowedTools"`
				DisabledTools     []string                   `json:"disabledTools"`
			}
			if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			if spec.Name == "" {
				jsonError(w, http.StatusBadRequest, "name is required")
				return
			}
			ns := defaultNamespace(namespace)
			image := spec.Image
			if image == "" {
				image = cfg.AgentDefaultImage
			}
			enabled := true
			if spec.Enabled != nil {
				enabled = *spec.Enabled
			}
			obj := &v1alpha1.Agent{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Agent"},
				ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: ns},
				Spec: v1alpha1.AgentSpec{
					Image:                image,
					Concurrency:          spec.Concurrency,
					Model:                spec.Model,
					InstructionInline:    inlineInstructionFrom(spec.Instructions),
					InstructionMountPath: "/etc/vinculum",
					ProviderSecretRef:    secretRefFrom(spec.ProviderSecretRef),
					MCPServers:           spec.MCPServers,
					MCPServerRefs:        spec.MCPServerRefs,
					Env:                  spec.Env,
					Enabled:              enabled,
					WorkspaceSize:        spec.WorkspaceSize,
					Orchestrator:         spec.Orchestrator,
					Peer:                 spec.Peer,
					Repo:                 spec.Repo,
					GitCredentials:       spec.GitCredentials,
					AllowedTools:         spec.AllowedTools,
					DisabledTools:        spec.DisabledTools,
				},
			}
			if err := k8s.Create(ctx, obj); err != nil {
				jsonError(w, http.StatusConflict, err.Error())
				return
			}
			jsonOK(w, obj)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/agents/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		name := strings.TrimPrefix(r.URL.Path, "/api/agents/")
		if name == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ns := defaultNamespace(namespace)
		switch r.Method {
		case http.MethodGet:
			var obj v1alpha1.Agent
			if err := k8s.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, &obj); err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			jsonOK(w, obj)
		case http.MethodDelete:
			obj := &v1alpha1.Agent{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
			if err := k8s.Delete(ctx, obj); err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		switch r.Method {
		case http.MethodGet:
			var list v1alpha1.TaskList
			if err := k8s.List(ctx, &list, scopedList(namespace)...); err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, list.Items)
		case http.MethodPost:
			body := struct {
				Name string             `json:"name"`
				Spec v1alpha1.TaskSpec  `json:"spec"`
			}{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			if body.Name == "" || body.Spec.AgentRef == "" || body.Spec.Prompt == "" {
				jsonError(w, http.StatusBadRequest, "name, spec.agentRef, spec.prompt are required")
				return
			}
			ns := defaultNamespace(namespace)
			if err := validateAgentExists(ctx, k8s, ns, body.Spec.AgentRef); err != nil {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			obj := &v1alpha1.Task{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Task"},
				ObjectMeta: metav1.ObjectMeta{Name: body.Name, Namespace: ns},
				Spec:       body.Spec,
			}
			if err := k8s.Create(ctx, obj); err != nil {
				jsonError(w, http.StatusConflict, err.Error())
				return
			}
			if from := strings.TrimSpace(r.Header.Get(fromAgentHeader)); from != "" {
				vmetrics.OrchestratorDispatchesTotal.WithLabelValues(from, body.Spec.AgentRef).Inc()
			}
			jsonOK(w, obj)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/tasks/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		name := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
		if name == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ns := defaultNamespace(namespace)
		switch r.Method {
		case http.MethodGet:
			var obj v1alpha1.Task
			if err := k8s.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, &obj); err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			jsonOK(w, obj)
		case http.MethodDelete:
			obj := &v1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
			if err := k8s.Delete(ctx, obj); err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/messages", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		switch r.Method {
		case http.MethodGet:
			var list v1alpha1.MessageList
			if err := k8s.List(ctx, &list, scopedList(namespace)...); err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
			items := list.Items
			if to := strings.TrimSpace(r.URL.Query().Get("to")); to != "" {
				filtered := items[:0]
				for _, m := range items {
					if m.Spec.To == to {
						filtered = append(filtered, m)
					}
				}
				items = filtered
			}
			if from := strings.TrimSpace(r.URL.Query().Get("from")); from != "" {
				filtered := items[:0]
				for _, m := range items {
					if m.Spec.From == from {
						filtered = append(filtered, m)
					}
				}
				items = filtered
			}
			jsonOK(w, items)
		case http.MethodPost:
			body := struct {
				Name string               `json:"name"`
				Spec v1alpha1.MessageSpec `json:"spec"`
			}{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			from := strings.TrimSpace(r.Header.Get(fromAgentHeader))
			if from == "" {
				jsonError(w, http.StatusBadRequest, fromAgentHeader+" header is required (messages are agent-to-agent)")
				return
			}
			if body.Spec.To == "" || strings.TrimSpace(body.Spec.Body) == "" {
				jsonError(w, http.StatusBadRequest, "spec.to and spec.body are required")
				return
			}
			if body.Spec.To == from {
				jsonError(w, http.StatusBadRequest, "refusing self-addressed message")
				return
			}
			ns := defaultNamespace(namespace)
			receiver, err := getPeerAgent(ctx, k8s, ns, body.Spec.To)
			if err != nil {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			if !receiver.PeerEnabled() {
				jsonError(w, http.StatusBadRequest, fmt.Sprintf("peer messaging disabled on receiver %q", body.Spec.To))
				return
			}
			name := strings.TrimSpace(body.Name)
			if name == "" {
				name = fmt.Sprintf("msg-%s-%s-%d", from, body.Spec.To, time.Now().UnixNano())
			}
			spec := body.Spec
			spec.From = from
			obj := &v1alpha1.Message{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Message"},
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       spec,
			}
			if err := k8s.Create(ctx, obj); err != nil {
				jsonError(w, http.StatusConflict, err.Error())
				return
			}
			vmetrics.MessagesTotal.WithLabelValues(from, body.Spec.To, "Pending").Inc()
			jsonOK(w, obj)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/messages/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		name := strings.TrimPrefix(r.URL.Path, "/api/messages/")
		if name == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ns := defaultNamespace(namespace)
		switch r.Method {
		case http.MethodGet:
			var obj v1alpha1.Message
			if err := k8s.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, &obj); err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			jsonOK(w, obj)
		case http.MethodDelete:
			obj := &v1alpha1.Message{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
			if err := k8s.Delete(ctx, obj); err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/schedules", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		switch r.Method {
		case http.MethodGet:
			var list v1alpha1.AgentScheduleList
			if err := k8s.List(ctx, &list, scopedList(namespace)...); err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, list.Items)
		case http.MethodPost:
			body := struct {
				Name string                     `json:"name"`
				Spec v1alpha1.AgentScheduleSpec `json:"spec"`
			}{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			if body.Name == "" || body.Spec.Schedule == "" || body.Spec.AgentRef == "" {
				jsonError(w, http.StatusBadRequest, "name, spec.schedule, spec.agentRef are required")
				return
			}
			ns := defaultNamespace(namespace)
			if err := validateAgentExists(ctx, k8s, ns, body.Spec.AgentRef); err != nil {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			obj := &v1alpha1.AgentSchedule{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "AgentSchedule"},
				ObjectMeta: metav1.ObjectMeta{Name: body.Name, Namespace: ns},
				Spec:       body.Spec,
			}
			if err := k8s.Create(ctx, obj); err != nil {
				jsonError(w, http.StatusConflict, err.Error())
				return
			}
			jsonOK(w, obj)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/schedules/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		name := strings.TrimPrefix(r.URL.Path, "/api/schedules/")
		if name == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ns := defaultNamespace(namespace)
		switch r.Method {
		case http.MethodGet:
			var obj v1alpha1.AgentSchedule
			if err := k8s.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, &obj); err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			jsonOK(w, obj)
		case http.MethodDelete:
			obj := &v1alpha1.AgentSchedule{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
			if err := k8s.Delete(ctx, obj); err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/webhooktriggers", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		switch r.Method {
		case http.MethodGet:
			var list v1alpha1.WebhookTriggerList
			if err := k8s.List(ctx, &list, scopedList(namespace)...); err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, list.Items)
		case http.MethodPost:
			body := struct {
				Name string                     `json:"name"`
				Spec v1alpha1.WebhookTriggerSpec `json:"spec"`
			}{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			if body.Name == "" || body.Spec.AgentRef == "" || body.Spec.SecretRef.Name == "" || len(body.Spec.Events) == 0 {
				jsonError(w, http.StatusBadRequest, "name, spec.agentRef, spec.secretRef.name, spec.events are required")
				return
			}
			ns := defaultNamespace(namespace)
			obj := &v1alpha1.WebhookTrigger{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "WebhookTrigger"},
				ObjectMeta: metav1.ObjectMeta{Name: body.Name, Namespace: ns},
				Spec:       body.Spec,
			}
			if err := k8s.Create(ctx, obj); err != nil {
				jsonError(w, http.StatusConflict, err.Error())
				return
			}
			jsonOK(w, obj)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/webhooktriggers/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		name := strings.TrimPrefix(r.URL.Path, "/api/webhooktriggers/")
		if name == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ns := defaultNamespace(namespace)
		switch r.Method {
		case http.MethodGet:
			var obj v1alpha1.WebhookTrigger
			if err := k8s.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, &obj); err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			jsonOK(w, obj)
		case http.MethodDelete:
			obj := &v1alpha1.WebhookTrigger{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
			if err := k8s.Delete(ctx, obj); err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/mcps", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		switch r.Method {
		case http.MethodGet:
			var list v1alpha1.MCPServerList
			if err := k8s.List(ctx, &list, scopedList(namespace)...); err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, list.Items)
		case http.MethodPost:
			body := struct {
				Name string                 `json:"name"`
				Spec v1alpha1.MCPServerSpec `json:"spec"`
			}{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			if body.Name == "" {
				jsonError(w, http.StatusBadRequest, "name is required")
				return
			}
			if (body.Spec.URL == "") == (body.Spec.Command == "") {
				jsonError(w, http.StatusBadRequest, "exactly one of spec.url or spec.command must be set")
				return
			}
			ns := defaultNamespace(namespace)
			obj := &v1alpha1.MCPServer{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "MCPServer"},
				ObjectMeta: metav1.ObjectMeta{Name: body.Name, Namespace: ns},
				Spec:       body.Spec,
			}
			if err := k8s.Create(ctx, obj); err != nil {
				jsonError(w, http.StatusConflict, err.Error())
				return
			}
			jsonOK(w, obj)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/mcps/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		name := strings.TrimPrefix(r.URL.Path, "/api/mcps/")
		if name == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ns := defaultNamespace(namespace)
		switch r.Method {
		case http.MethodGet:
			var obj v1alpha1.MCPServer
			if err := k8s.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, &obj); err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			jsonOK(w, obj)
		case http.MethodDelete:
			obj := &v1alpha1.MCPServer{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
			if err := k8s.Delete(ctx, obj); err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	return mux
}

func scopedList(namespace string) []client.ListOption {
	if namespace == "" {
		return nil
	}
	return []client.ListOption{client.InNamespace(namespace)}
}

func defaultNamespace(namespace string) string {
	if namespace != "" {
		return namespace
	}
	return "default"
}

// getPeerAgent fetches an Agent and returns a clear error when missing /
// disabled / not ready. Used by POST /api/messages to validate the
// receiver before creating a Message CRD.
func getPeerAgent(ctx context.Context, k8s client.Client, namespace, name string) (*v1alpha1.Agent, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("spec.to is required")
	}
	var agent v1alpha1.Agent
	if err := k8s.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &agent); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("agent %q not found", name)
		}
		return nil, err
	}
	if !agent.Spec.Enabled {
		return nil, fmt.Errorf("agent %q is disabled", name)
	}
	if !agent.Status.Ready {
		return nil, fmt.Errorf("agent %q is not ready", name)
	}
	return &agent, nil
}

func validateAgentExists(ctx context.Context, k8s client.Client, namespace, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("agentRef is required")
	}
	var agent v1alpha1.Agent
	if err := k8s.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &agent); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("agentRef %q not found", name)
		}
		return err
	}
	return nil
}

func inlineInstructionFrom(content string) *v1alpha1.InlineFile {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return &v1alpha1.InlineFile{FileName: "AGENTS.md", Content: content}
}

func secretRefFrom(name string) *v1alpha1.SecretRef {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	return &v1alpha1.SecretRef{Name: strings.TrimSpace(name)}
}

func jsonOK(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
