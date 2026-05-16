// vnclm-mcp is a stdio MCP server that exposes the vinculum operator API
// to a running crush session, letting a "master" Agent dispatch Tasks to
// peer Agents and read their results.
//
// Expected env (injected by the operator when spec.orchestrator: true):
//
//	VINCULUM_OPERATOR_URL  e.g. http://vinculum-operator.vinculum-system.svc:8084
//	AGENT_NAME             the calling pod's agent name (used for self-dispatch guard)
//	AGENT_NAMESPACE        the calling pod's namespace (used to address peer agents)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/florian/vinculum/apps/vnclm-mcp/internal/mcp"
	"github.com/florian/vinculum/apps/vnclm-mcp/internal/opclient"
)

func main() {
	operatorURL := strings.TrimRight(os.Getenv("VINCULUM_OPERATOR_URL"), "/")
	if operatorURL == "" {
		fmt.Fprintln(os.Stderr, "vnclm-mcp: VINCULUM_OPERATOR_URL is required (enable spec.orchestrator on this Agent)")
		os.Exit(2)
	}
	selfName := os.Getenv("AGENT_NAME")
	namespace := os.Getenv("AGENT_NAMESPACE")

	op := opclient.New(operatorURL).WithFromAgent(selfName)
	srv := buildServer(op, selfName, namespace, &http.Client{Timeout: 30 * time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "vnclm-mcp: %v\n", err)
		os.Exit(1)
	}
}

// peerLogFetcher is the function used to fetch logs from a peer agent pod.
// Indirected for tests.
type peerLogFetcher func(ctx context.Context, agent, task string) (string, error)

func buildServer(op *opclient.Client, selfName, namespace string, httpc *http.Client) *mcp.Server {
	srv := mcp.NewServer("vinculum", "0.1.0")
	registerTools(srv, op, selfName, namespace, defaultPeerLogFetcher(httpc, namespace))
	return srv
}

func defaultPeerLogFetcher(httpc *http.Client, namespace string) peerLogFetcher {
	return func(ctx context.Context, agent, task string) (string, error) {
		// Peer agent pods expose their HTTP API on port 8090 via a Service
		// named "agent-<name>" in the same namespace.
		host := fmt.Sprintf("agent-%s", agent)
		if namespace != "" {
			host = fmt.Sprintf("agent-%s.%s.svc", agent, namespace)
		}
		url := fmt.Sprintf("http://%s:8090/task/%s/logs", host, task)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		resp, err := httpc.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("%d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return string(body), nil
	}
}

func registerTools(srv *mcp.Server, op *opclient.Client, selfName, namespace string, fetchLogs peerLogFetcher) {
	_ = namespace // reserved for future cross-ns features
	srv.RegisterTool(mcp.Tool{
		Name:        "list_agents",
		Description: "List Agents in this cluster. Returns name, model, phase, readiness, and whether each is itself in orchestrator mode.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(ctx context.Context, _ json.RawMessage) (*mcp.CallResult, error) {
		agents, err := op.ListAgents(ctx)
		if err != nil {
			return mcp.ErrorResult("list_agents: " + err.Error()), nil
		}
		return mcp.AsJSONText(agents), nil
	})

	srv.RegisterTool(mcp.Tool{
		Name:        "dispatch_task",
		Description: "Create a Task against a peer Agent. Returns immediately with the new Task name and phase; use wait_task or get_task to read results.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"agent", "prompt"},
			"properties": map[string]any{
				"agent":         map[string]any{"type": "string", "description": "Name of the target Agent (must not be self)."},
				"prompt":        map[string]any{"type": "string", "description": "Prompt to send to the peer agent."},
				"name":          map[string]any{"type": "string", "description": "Optional explicit Task name. Auto-generated if omitted."},
				"fresh":         map[string]any{"type": "boolean", "description": "If true, start the peer's crush session fresh (no history)."},
				"workspaceMode": map[string]any{"type": "string", "enum": []string{"shared", "ephemeral"}, "description": "Peer workspace mode."},
				"timeoutSeconds": map[string]any{"type": "integer", "description": "Per-Task timeout."},
			},
		},
	}, func(ctx context.Context, raw json.RawMessage) (*mcp.CallResult, error) {
		var args struct {
			Agent          string `json:"agent"`
			Prompt         string `json:"prompt"`
			Name           string `json:"name"`
			Fresh          bool   `json:"fresh"`
			WorkspaceMode  string `json:"workspaceMode"`
			TimeoutSeconds int32  `json:"timeoutSeconds"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
		}
		if args.Agent == "" || args.Prompt == "" {
			return mcp.ErrorResult("agent and prompt are required"), nil
		}
		if selfName != "" && args.Agent == selfName {
			return mcp.ErrorResult("refusing to dispatch task from agent to itself (self-recursion guard)"), nil
		}
		name := args.Name
		if name == "" {
			name = fmt.Sprintf("%s-mcp-%d", args.Agent, time.Now().UnixNano())
		}
		t, err := op.DispatchTask(ctx, opclient.DispatchTaskInput{
			Name:           name,
			AgentRef:       args.Agent,
			Prompt:         args.Prompt,
			Fresh:          args.Fresh,
			WorkspaceMode:  args.WorkspaceMode,
			TimeoutSeconds: args.TimeoutSeconds,
		})
		if err != nil {
			return mcp.ErrorResult("dispatch_task: " + err.Error()), nil
		}
		return mcp.AsJSONText(t), nil
	})

	srv.RegisterTool(mcp.Tool{
		Name:        "get_task",
		Description: "Read current Task status. Returns phase plus stdoutTail/stderrTail/exitCode for completed Tasks.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"name"},
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		},
	}, func(ctx context.Context, raw json.RawMessage) (*mcp.CallResult, error) {
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &args); err != nil || args.Name == "" {
			return mcp.ErrorResult("name is required"), nil
		}
		t, err := op.GetTask(ctx, args.Name)
		if err != nil {
			return mcp.ErrorResult("get_task: " + err.Error()), nil
		}
		return mcp.AsJSONText(t), nil
	})

	srv.RegisterTool(mcp.Tool{
		Name:        "wait_task",
		Description: "Block until a Task reaches a terminal phase (Succeeded/Failed/TimedOut) or timeoutSeconds elapses.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"name"},
			"properties": map[string]any{
				"name":           map[string]any{"type": "string"},
				"timeoutSeconds": map[string]any{"type": "integer", "description": "Max wait. Defaults to 900 (15 min)."},
				"pollSeconds":    map[string]any{"type": "integer", "description": "Poll interval. Defaults to 3s."},
			},
		},
	}, func(ctx context.Context, raw json.RawMessage) (*mcp.CallResult, error) {
		var args struct {
			Name           string `json:"name"`
			TimeoutSeconds int    `json:"timeoutSeconds"`
			PollSeconds    int    `json:"pollSeconds"`
		}
		if err := json.Unmarshal(raw, &args); err != nil || args.Name == "" {
			return mcp.ErrorResult("name is required"), nil
		}
		timeout := args.TimeoutSeconds
		if timeout <= 0 {
			timeout = 900
		}
		poll := args.PollSeconds
		if poll <= 0 {
			poll = 3
		}
		waitCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
		t, err := op.WaitTask(waitCtx, args.Name, time.Duration(poll)*time.Second)
		if err != nil {
			// Even on timeout we return the last known state, marked as error
			// so the model sees it didn't complete.
			body, _ := json.MarshalIndent(map[string]any{"task": t, "error": err.Error()}, "", "  ")
			return mcp.ErrorResult(string(body)), nil
		}
		return mcp.AsJSONText(t), nil
	})

	srv.RegisterTool(mcp.Tool{
		Name:        "get_task_logs",
		Description: "Stream the peer agent pod's recent log output for a Task (full crush stdout/stderr).",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"agent", "name"},
			"properties": map[string]any{
				"agent": map[string]any{"type": "string", "description": "Name of the Agent that owns the Task."},
				"name":  map[string]any{"type": "string", "description": "Task name."},
			},
		},
	}, func(ctx context.Context, raw json.RawMessage) (*mcp.CallResult, error) {
		var args struct {
			Agent string `json:"agent"`
			Name  string `json:"name"`
		}
		if err := json.Unmarshal(raw, &args); err != nil || args.Agent == "" || args.Name == "" {
			return mcp.ErrorResult("agent and name are required"), nil
		}
		logs, err := fetchLogs(ctx, args.Agent, args.Name)
		if err != nil {
			return mcp.ErrorResult("get_task_logs: " + err.Error()), nil
		}
		return mcp.TextResult(logs), nil
	})

	srv.RegisterTool(mcp.Tool{
		Name:        "cancel_task",
		Description: "Delete a Task. Cancels in-flight execution.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"name"},
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		},
	}, func(ctx context.Context, raw json.RawMessage) (*mcp.CallResult, error) {
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &args); err != nil || args.Name == "" {
			return mcp.ErrorResult("name is required"), nil
		}
		if err := op.DeleteTask(ctx, args.Name); err != nil {
			return mcp.ErrorResult("cancel_task: " + err.Error()), nil
		}
		return mcp.TextResult(`{"ok": true}`), nil
	})
}
