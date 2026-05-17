// Package opclient is a thin JSON HTTP client for the vinculum operator's
// internal API, used by the vinculum MCP server running inside an
// orchestrator-mode Agent pod.
package opclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	base     string
	hc       *http.Client
	fromName string // pod's AGENT_NAME — emitted as X-Vinculum-From-Agent
}

func New(base string) *Client {
	return &Client{base: base, hc: &http.Client{Timeout: 30 * time.Second}}
}

// WithFromAgent tags subsequent POST /api/tasks calls with the orchestrator
// agent's name via the X-Vinculum-From-Agent header. The operator uses it
// to label the vinculum_orchestrator_dispatches_total metric.
func (c *Client) WithFromAgent(name string) *Client {
	cp := *c
	cp.fromName = strings.TrimSpace(name)
	return &cp
}

// Agent is a partial mirror of v1alpha1.Agent — only the fields the master
// agent typically wants. The full object is returned as JSON; this shape
// keeps tool output compact.
type Agent struct {
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	Model        string            `json:"model,omitempty"`
	Phase        string            `json:"phase,omitempty"`
	Ready        bool              `json:"ready,omitempty"`
	Orchestrator bool              `json:"orchestrator,omitempty"`
	Peer         bool              `json:"peer,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

// Task is a partial mirror of v1alpha1.Task.
type Task struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	AgentRef       string `json:"agentRef"`
	Phase          string `json:"phase,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Message        string `json:"message,omitempty"`
	ExitCode       int32  `json:"exitCode,omitempty"`
	StdoutTail     string `json:"stdoutTail,omitempty"`
	StderrTail     string `json:"stderrTail,omitempty"`
	StartTime      string `json:"startTime,omitempty"`
	CompletionTime string `json:"completionTime,omitempty"`
}

// rawAgent / rawTask mirror just enough of the operator's JSON to extract
// fields without pulling in apimachinery.
type rawAgent struct {
	Metadata struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		Model        string `json:"model"`
		Orchestrator bool   `json:"orchestrator"`
		// Peer is *bool in the CRD; absent or null means default-on. We
		// model it as a pointer so we can distinguish unset (→ true) from
		// explicit false.
		Peer *bool `json:"peer"`
	} `json:"spec"`
	Status struct {
		Phase string `json:"phase"`
		Ready bool   `json:"ready"`
	} `json:"status"`
}

func (r rawAgent) toAgent() Agent {
	peer := true
	if r.Spec.Peer != nil {
		peer = *r.Spec.Peer
	}
	return Agent{
		Name:         r.Metadata.Name,
		Namespace:    r.Metadata.Namespace,
		Model:        r.Spec.Model,
		Phase:        r.Status.Phase,
		Ready:        r.Status.Ready,
		Orchestrator: r.Spec.Orchestrator,
		Peer:         peer,
		Labels:       r.Metadata.Labels,
	}
}

// Message is a partial mirror of v1alpha1.Message returned to the MCP
// caller. The reply chain is discoverable via ReplyMessages plus a
// follow-up get_message call.
type Message struct {
	Name          string   `json:"name"`
	Namespace     string   `json:"namespace"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	Body          string   `json:"body"`
	InReplyTo     string   `json:"inReplyTo,omitempty"`
	Phase         string   `json:"phase,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	DeliveredAt   string   `json:"deliveredAt,omitempty"`
	ReplyMessages []string `json:"replyMessages,omitempty"`
}

type rawMessage struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		To             string `json:"to"`
		From           string `json:"from"`
		Body           string `json:"body"`
		InReplyTo      string `json:"inReplyTo"`
		TimeoutSeconds int32  `json:"timeoutSeconds"`
	} `json:"spec"`
	Status struct {
		Phase         string   `json:"phase"`
		Reason        string   `json:"reason"`
		Message       string   `json:"message"`
		DeliveredAt   string   `json:"deliveredAt"`
		ReplyMessages []string `json:"replyMessages"`
	} `json:"status"`
}

func (r rawMessage) toMessage() Message {
	return Message{
		Name:          r.Metadata.Name,
		Namespace:     r.Metadata.Namespace,
		From:          r.Spec.From,
		To:            r.Spec.To,
		Body:          r.Spec.Body,
		InReplyTo:     r.Spec.InReplyTo,
		Phase:         r.Status.Phase,
		Reason:        r.Status.Reason,
		DeliveredAt:   r.Status.DeliveredAt,
		ReplyMessages: r.Status.ReplyMessages,
	}
}

type rawTask struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		AgentRef string `json:"agentRef"`
	} `json:"spec"`
	Status struct {
		Phase          string `json:"phase"`
		Reason         string `json:"reason"`
		Message        string `json:"message"`
		ExitCode       int32  `json:"exitCode"`
		StdoutTail     string `json:"stdoutTail"`
		StderrTail     string `json:"stderrTail"`
		StartTime      string `json:"startTime"`
		CompletionTime string `json:"completionTime"`
	} `json:"status"`
}

func (r rawTask) toTask() Task {
	return Task{
		Name:           r.Metadata.Name,
		Namespace:      r.Metadata.Namespace,
		AgentRef:       r.Spec.AgentRef,
		Phase:          r.Status.Phase,
		Reason:         r.Status.Reason,
		Message:        r.Status.Message,
		ExitCode:       r.Status.ExitCode,
		StdoutTail:     r.Status.StdoutTail,
		StderrTail:     r.Status.StderrTail,
		StartTime:      r.Status.StartTime,
		CompletionTime: r.Status.CompletionTime,
	}
}

func (c *Client) ListAgents(ctx context.Context) ([]Agent, error) {
	var raw []rawAgent
	if err := c.get(ctx, "/api/agents", &raw); err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.toAgent())
	}
	return out, nil
}

func (c *Client) GetAgent(ctx context.Context, name string) (*Agent, error) {
	var r rawAgent
	if err := c.get(ctx, "/api/agents/"+name, &r); err != nil {
		return nil, err
	}
	a := r.toAgent()
	return &a, nil
}

// DispatchTaskInput is the create-task body. Only AgentRef and Prompt are
// required.
type DispatchTaskInput struct {
	Name           string
	AgentRef       string
	Prompt         string
	Fresh          bool
	WorkspaceMode  string
	TimeoutSeconds int32
	Env            map[string]string
}

func (c *Client) DispatchTask(ctx context.Context, in DispatchTaskInput) (*Task, error) {
	spec := map[string]any{
		"agentRef": in.AgentRef,
		"prompt":   in.Prompt,
	}
	if in.Fresh {
		spec["fresh"] = true
	}
	if in.WorkspaceMode != "" {
		spec["workspace"] = map[string]string{"mode": in.WorkspaceMode}
	}
	if in.TimeoutSeconds > 0 {
		spec["timeoutSeconds"] = in.TimeoutSeconds
	}
	if len(in.Env) > 0 {
		spec["env"] = in.Env
	}
	body := map[string]any{"name": in.Name, "spec": spec}
	var r rawTask
	if err := c.post(ctx, "/api/tasks", body, &r); err != nil {
		return nil, err
	}
	t := r.toTask()
	return &t, nil
}

func (c *Client) GetTask(ctx context.Context, name string) (*Task, error) {
	var r rawTask
	if err := c.get(ctx, "/api/tasks/"+name, &r); err != nil {
		return nil, err
	}
	t := r.toTask()
	return &t, nil
}

func (c *Client) DeleteTask(ctx context.Context, name string) error {
	return c.delete(ctx, "/api/tasks/"+name)
}

// SendMessageInput is the body for POST /api/messages.
type SendMessageInput struct {
	Name           string
	To             string
	Body           string
	InReplyTo      string
	TimeoutSeconds int32
}

func (c *Client) SendMessage(ctx context.Context, in SendMessageInput) (*Message, error) {
	spec := map[string]any{
		"to":   in.To,
		"body": in.Body,
	}
	if in.InReplyTo != "" {
		spec["inReplyTo"] = in.InReplyTo
	}
	if in.TimeoutSeconds > 0 {
		spec["timeoutSeconds"] = in.TimeoutSeconds
	}
	body := map[string]any{"spec": spec}
	if in.Name != "" {
		body["name"] = in.Name
	}
	var r rawMessage
	if err := c.post(ctx, "/api/messages", body, &r); err != nil {
		return nil, err
	}
	m := r.toMessage()
	return &m, nil
}

func (c *Client) GetMessage(ctx context.Context, name string) (*Message, error) {
	var r rawMessage
	if err := c.get(ctx, "/api/messages/"+name, &r); err != nil {
		return nil, err
	}
	m := r.toMessage()
	return &m, nil
}

// WaitTask polls GetTask until the phase is terminal (Succeeded/Failed/TimedOut)
// or the context's deadline fires.
func (c *Client) WaitTask(ctx context.Context, name string, pollEvery time.Duration) (*Task, error) {
	if pollEvery <= 0 {
		pollEvery = 3 * time.Second
	}
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		t, err := c.GetTask(ctx, name)
		if err != nil {
			return nil, err
		}
		if IsTerminal(t.Phase) {
			return t, nil
		}
		select {
		case <-ctx.Done():
			return t, ctx.Err()
		case <-tick.C:
		}
	}
}

func IsTerminal(phase string) bool {
	switch phase {
	case "Succeeded", "Failed", "TimedOut":
		return true
	}
	return false
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

func (c *Client) delete(ctx context.Context, path string) error {
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.fromName != "" && method == http.MethodPost {
		req.Header.Set("X-Vinculum-From-Agent", c.fromName)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		msg := bytes.TrimSpace(b)
		if len(msg) == 0 {
			return fmt.Errorf("%s: %s", resp.Status, path)
		}
		return fmt.Errorf("%d %s: %s", resp.StatusCode, path, string(msg))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
