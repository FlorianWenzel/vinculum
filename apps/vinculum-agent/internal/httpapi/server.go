package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/florian/vinculum/apps/vinculum-agent/internal/config"
	"github.com/florian/vinculum/apps/vinculum-agent/internal/tasks"
)

type Server struct {
	cfg    config.Config
	runner *tasks.Runner
}

func New(cfg config.Config, runner *tasks.Runner) *Server {
	return &Server{cfg: cfg, runner: runner}
}

func (s *Server) Mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/info", s.info)
	mux.HandleFunc("/task", s.postTask)
	mux.HandleFunc("/task/", s.taskByName)
	mux.HandleFunc("/tasks", s.listTasks)
	mux.HandleFunc("/tasks/", s.taskByName)
	mux.HandleFunc("/message", s.postMessage)
	return mux
}

// inboundMessage is the payload the operator's MessageReconciler POSTs to
// each agent pod's /message endpoint. The body is wrapped in a stable
// [peer-message ...] marker before being fed into the crush session, so
// per-agent AGENTS.md instructions can teach the LLM to recognize peer
// chatter vs. a fresh human Task.
type inboundMessage struct {
	MessageID string `json:"messageId"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	From      string `json:"from"`
	Body      string `json:"body"`
	InReplyTo string `json:"inReplyTo,omitempty"`
}

func (s *Server) postMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in inboundMessage
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Body) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and body are required"})
		return
	}
	payload := tasks.DispatchPayload{
		TaskID:    in.MessageID,
		Name:      in.Name,
		Namespace: in.Namespace,
		Kind:      tasks.KindMessage,
		Spec: tasks.TaskSpec{
			AgentRef: s.cfg.AgentName,
			Prompt:   wrapPeerMessage(in),
		},
	}
	if _, err := s.runner.Enqueue(payload); err != nil {
		if errors.Is(err, tasks.ErrAlreadyExists) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued", "message": in.Name})
}

func wrapPeerMessage(in inboundMessage) string {
	header := fmt.Sprintf("[peer-message from=%s name=%s", in.From, in.Name)
	if in.InReplyTo != "" {
		header += " inReplyTo=" + in.InReplyTo
	}
	header += "]"
	return header + "\n" + in.Body + "\n[/peer-message]"
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) info(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"agent":     s.cfg.AgentName,
		"namespace": s.cfg.Namespace,
		"current":   s.runner.Current(),
		"model":     s.cfg.CrushModel,
	})
}

func (s *Server) postTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var payload tasks.DispatchPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Spec.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and spec.prompt are required"})
		return
	}
	if _, err := s.runner.Enqueue(payload); err != nil {
		if errors.Is(err, tasks.ErrAlreadyExists) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued", "task": payload.Name})
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	states := s.runner.List()
	out := make([]map[string]any, 0, len(states))
	for _, st := range states {
		out = append(out, stateView(st))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) taskByName(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/task/")
	path = strings.TrimPrefix(path, "/tasks/")
	if path == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if idx := strings.Index(path, "/"); idx >= 0 {
		name, sub := path[:idx], path[idx+1:]
		if sub == "logs" && r.Method == http.MethodGet {
			s.taskLogs(w, r, name)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := path
	switch r.Method {
	case http.MethodGet:
		st, ok := s.runner.Get(name)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, stateView(st))
	case http.MethodDelete:
		if s.runner.Cancel(name) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) taskLogs(w http.ResponseWriter, r *http.Request, name string) {
	st, ok := s.runner.Get(name)
	follow := r.URL.Query().Get("follow") == "1" || r.URL.Query().Get("follow") == "true"

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	var source io.Reader
	switch {
	case ok && st.Logs != nil:
		source = st.Logs.Reader(follow)
	case ok && st.LogFile != "":
		f, err := os.Open(st.LogFile)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		defer f.Close()
		source = f
	default:
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := source.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func stateView(st *tasks.State) map[string]any {
	m := map[string]any{
		"name":       st.Payload.Name,
		"namespace":  st.Payload.Namespace,
		"phase":      st.Phase,
		"enqueuedAt": st.EnqueuedAt,
		"exitCode":   st.ExitCode,
	}
	if st.StartedAt != nil {
		m["startedAt"] = *st.StartedAt
	}
	if st.FinishedAt != nil {
		m["finishedAt"] = *st.FinishedAt
	}
	if st.Reason != "" {
		m["reason"] = st.Reason
	}
	if st.Message != "" {
		m["message"] = st.Message
	}
	if len(st.Artifacts) > 0 {
		m["artifactURLs"] = st.Artifacts
	}
	return m
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
