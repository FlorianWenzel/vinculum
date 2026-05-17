package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florian/vinculum/apps/vnclm-mcp/internal/mcp"
	"github.com/florian/vinculum/apps/vnclm-mcp/internal/opclient"
)

// stubOperator returns an httptest.Server that mimics the operator's API
// surface needed by the MCP server, plus a recorder of dispatch payloads.
type stubOperator struct {
	mu            sync.Mutex
	dispatched    []map[string]any
	tasks         map[string]map[string]any
	taskPhaseFunc func(name string, calls int) string
	taskCalls     map[string]int
}

func newStub() *stubOperator {
	return &stubOperator{
		tasks:     map[string]map[string]any{},
		taskCalls: map[string]int{},
	}
}

func (s *stubOperator) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/agents":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"metadata": map[string]any{"name": "drone-7", "namespace": "vinculum"},
					"spec":     map[string]any{"model": "claude-haiku-4-5"},
					"status":   map[string]any{"phase": "Ready", "ready": true},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/tasks":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			s.dispatched = append(s.dispatched, body)
			name, _ := body["name"].(string)
			spec, _ := body["spec"].(map[string]any)
			agent, _ := spec["agentRef"].(string)
			s.tasks[name] = map[string]any{
				"metadata": map[string]any{"name": name, "namespace": "vinculum"},
				"spec":     map[string]any{"agentRef": agent},
				"status":   map[string]any{"phase": "Pending"},
			}
			_ = json.NewEncoder(w).Encode(s.tasks[name])
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/tasks/"):
			name := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
			s.taskCalls[name]++
			obj, ok := s.tasks[name]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			if s.taskPhaseFunc != nil {
				status := obj["status"].(map[string]any)
				status["phase"] = s.taskPhaseFunc(name, s.taskCalls[name])
			}
			_ = json.NewEncoder(w).Encode(obj)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/tasks/"):
			name := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
			delete(s.tasks, name)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}
}

// driveServer wires the MCP server up to in/out pipes and runs it in a
// goroutine. It returns the writer (test → server), a line reader (server
// → test), and a cleanup func.
func driveServer(t *testing.T, op *opclient.Client, selfName string, fetchLogs peerLogFetcher) (io.Writer, *bufio.Scanner, func()) {
	t.Helper()
	return driveServerWith(t, op, selfName, true, true, fetchLogs)
}

// driveServerWith lets a test enable just the peer surface, just the
// orchestrator surface, or both — covering the gating that real pods do
// via VINCULUM_PEER / VINCULUM_ORCHESTRATOR env vars.
func driveServerWith(t *testing.T, op *opclient.Client, selfName string, peer, orchestrator bool, fetchLogs peerLogFetcher) (io.Writer, *bufio.Scanner, func()) {
	t.Helper()
	srv := mcp.NewServer("vinculum", "test")
	if orchestrator {
		registerOrchestratorTools(srv, op, selfName, "vinculum", fetchLogs)
	}
	if peer {
		registerPeerTools(srv, op, selfName)
	}

	clientToServer, serverIn := io.Pipe()
	serverOut, fromServer := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = srv.Serve(ctx, clientToServer, fromServer)
		close(done)
	}()

	scanner := bufio.NewScanner(serverOut)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	cleanup := func() {
		cancel()
		_ = serverIn.Close()
		_ = fromServer.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("server did not shut down")
		}
	}
	return serverIn, scanner, cleanup
}

// rpcCall writes a request and reads/parses the next response line.
func rpcCall(t *testing.T, w io.Writer, sc *bufio.Scanner, id int, method string, params any) map[string]any {
	t.Helper()
	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	b, _ := json.Marshal(req)
	if _, err := w.Write(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
	if !sc.Scan() {
		t.Fatalf("no response for %s: %v", method, sc.Err())
	}
	var resp map[string]any
	if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v: %s", err, sc.Text())
	}
	return resp
}

// extractText pulls the first text block out of a tools/call result.
func extractText(t *testing.T, resp map[string]any) (string, bool) {
	t.Helper()
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %+v", resp)
	}
	isErr, _ := result["isError"].(bool)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("empty content")
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	return text, isErr
}

func TestMCPHandshakeAndListAgents(t *testing.T) {
	stub := newStub()
	opSrv := httptest.NewServer(stub.handler(t))
	defer opSrv.Close()

	w, sc, cleanup := driveServer(t, opclient.New(opSrv.URL), "locutus", nil)
	defer cleanup()

	init := rpcCall(t, w, sc, 1, "initialize", map[string]any{"protocolVersion": "2024-11-05"})
	if init["error"] != nil {
		t.Fatalf("initialize error: %+v", init)
	}

	list := rpcCall(t, w, sc, 2, "tools/list", nil)
	res, _ := list["result"].(map[string]any)
	tools, _ := res["tools"].([]any)
	// 6 orchestrator tools + 3 peer tools = 9 when both are enabled.
	if len(tools) != 9 {
		t.Errorf("want 9 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tt := range tools {
		m := tt.(map[string]any)
		names[m["name"].(string)] = true
	}
	for _, want := range []string{
		"list_agents", "dispatch_task", "get_task", "wait_task", "get_task_logs", "cancel_task",
		"send_message", "list_peers", "get_message",
	} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}

	call := rpcCall(t, w, sc, 3, "tools/call", map[string]any{
		"name":      "list_agents",
		"arguments": map[string]any{},
	})
	text, isErr := extractText(t, call)
	if isErr {
		t.Fatalf("list_agents reported error: %s", text)
	}
	if !strings.Contains(text, "drone-7") {
		t.Errorf("list_agents output missing drone-7: %s", text)
	}
}

func TestDispatchTask_SelfRecursionGuard(t *testing.T) {
	stub := newStub()
	opSrv := httptest.NewServer(stub.handler(t))
	defer opSrv.Close()

	w, sc, cleanup := driveServer(t, opclient.New(opSrv.URL), "locutus", nil)
	defer cleanup()

	rpcCall(t, w, sc, 1, "initialize", map[string]any{})
	resp := rpcCall(t, w, sc, 2, "tools/call", map[string]any{
		"name":      "dispatch_task",
		"arguments": map[string]any{"agent": "locutus", "prompt": "loop"},
	})
	text, isErr := extractText(t, resp)
	if !isErr {
		t.Fatalf("expected error result, got success: %s", text)
	}
	if !strings.Contains(text, "self-recursion") {
		t.Errorf("error should mention self-recursion: %s", text)
	}
	if len(stub.dispatched) != 0 {
		t.Errorf("operator should not have received any dispatch, got %d", len(stub.dispatched))
	}
}

func TestDispatchAndWaitTask(t *testing.T) {
	stub := newStub()
	// First few polls: Running; afterwards: Succeeded.
	stub.taskPhaseFunc = func(_ string, calls int) string {
		if calls >= 2 {
			return "Succeeded"
		}
		return "Running"
	}
	opSrv := httptest.NewServer(stub.handler(t))
	defer opSrv.Close()

	w, sc, cleanup := driveServer(t, opclient.New(opSrv.URL), "locutus", nil)
	defer cleanup()

	rpcCall(t, w, sc, 1, "initialize", map[string]any{})

	dispatch := rpcCall(t, w, sc, 2, "tools/call", map[string]any{
		"name": "dispatch_task",
		"arguments": map[string]any{
			"agent":  "drone-7",
			"prompt": "compose a haiku",
			"name":   "task-haiku",
		},
	})
	dispatchText, isErr := extractText(t, dispatch)
	if isErr {
		t.Fatalf("dispatch errored: %s", dispatchText)
	}
	if !strings.Contains(dispatchText, "task-haiku") {
		t.Errorf("dispatch output missing task name: %s", dispatchText)
	}
	if len(stub.dispatched) != 1 {
		t.Fatalf("want 1 dispatched task, got %d", len(stub.dispatched))
	}
	spec := stub.dispatched[0]["spec"].(map[string]any)
	if spec["agentRef"] != "drone-7" || spec["prompt"] != "compose a haiku" {
		t.Errorf("dispatch payload mismatch: %+v", spec)
	}

	wait := rpcCall(t, w, sc, 3, "tools/call", map[string]any{
		"name": "wait_task",
		"arguments": map[string]any{
			"name":           "task-haiku",
			"pollSeconds":    1, // minimum; loop will tick once per second
			"timeoutSeconds": 30,
		},
	})
	waitText, isErr := extractText(t, wait)
	if isErr {
		t.Fatalf("wait_task errored: %s", waitText)
	}
	if !strings.Contains(waitText, "Succeeded") {
		t.Errorf("wait_task did not return Succeeded: %s", waitText)
	}
}

func TestGetTaskLogs(t *testing.T) {
	opSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not used in this test", http.StatusNotFound)
	}))
	defer opSrv.Close()

	fakeLogs := func(_ context.Context, agent, task string) (string, error) {
		return "[" + agent + "/" + task + "] log line one\nlog line two\n", nil
	}

	w, sc, cleanup := driveServer(t, opclient.New(opSrv.URL), "locutus", fakeLogs)
	defer cleanup()

	rpcCall(t, w, sc, 1, "initialize", map[string]any{})
	resp := rpcCall(t, w, sc, 2, "tools/call", map[string]any{
		"name":      "get_task_logs",
		"arguments": map[string]any{"agent": "drone-7", "name": "task-haiku"},
	})
	text, isErr := extractText(t, resp)
	if isErr {
		t.Fatalf("get_task_logs errored: %s", text)
	}
	if !strings.Contains(text, "log line one") || !strings.Contains(text, "drone-7/task-haiku") {
		t.Errorf("unexpected log output: %s", text)
	}
}

func TestSendMessage(t *testing.T) {
	var captured map[string]any
	var capturedHeader string
	opSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/messages":
			capturedHeader = r.Header.Get("X-Vinculum-From-Agent")
			_ = json.NewDecoder(r.Body).Decode(&captured)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"metadata": map[string]any{"name": "msg-1", "namespace": "vinculum"},
				"spec":     map[string]any{"to": "drone-7", "from": "locutus", "body": "hi"},
				"status":   map[string]any{"phase": "Pending"},
			})
		default:
			http.Error(w, "not used", http.StatusNotFound)
		}
	}))
	defer opSrv.Close()

	w, sc, cleanup := driveServer(t, opclient.New(opSrv.URL).WithFromAgent("locutus"), "locutus", nil)
	defer cleanup()

	rpcCall(t, w, sc, 1, "initialize", map[string]any{})
	resp := rpcCall(t, w, sc, 2, "tools/call", map[string]any{
		"name":      "send_message",
		"arguments": map[string]any{"to": "drone-7", "body": "hi", "name": "msg-1"},
	})
	text, isErr := extractText(t, resp)
	if isErr {
		t.Fatalf("send_message errored: %s", text)
	}
	if !strings.Contains(text, "drone-7") {
		t.Errorf("response missing recipient: %s", text)
	}
	if capturedHeader != "locutus" {
		t.Errorf("X-Vinculum-From-Agent not set: %q", capturedHeader)
	}
	spec, _ := captured["spec"].(map[string]any)
	if spec["to"] != "drone-7" || spec["body"] != "hi" {
		t.Errorf("payload mismatch: %+v", spec)
	}
}

func TestSendMessage_SelfGuard(t *testing.T) {
	opSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("operator should not be hit: %s %s", r.Method, r.URL.Path)
		http.Error(w, "should not be called", http.StatusBadRequest)
	}))
	defer opSrv.Close()

	w, sc, cleanup := driveServer(t, opclient.New(opSrv.URL), "locutus", nil)
	defer cleanup()

	rpcCall(t, w, sc, 1, "initialize", map[string]any{})
	resp := rpcCall(t, w, sc, 2, "tools/call", map[string]any{
		"name":      "send_message",
		"arguments": map[string]any{"to": "locutus", "body": "hello me"},
	})
	text, isErr := extractText(t, resp)
	if !isErr {
		t.Fatalf("expected error, got success: %s", text)
	}
	if !strings.Contains(text, "self") {
		t.Errorf("error should mention self: %s", text)
	}
}

func TestPeerOnlyGating(t *testing.T) {
	opSrv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer opSrv.Close()

	w, sc, cleanup := driveServerWith(t, opclient.New(opSrv.URL), "locutus", true, false, nil)
	defer cleanup()

	rpcCall(t, w, sc, 1, "initialize", map[string]any{})
	list := rpcCall(t, w, sc, 2, "tools/list", nil)
	res := list["result"].(map[string]any)
	tools := res["tools"].([]any)
	names := map[string]bool{}
	for _, tt := range tools {
		names[tt.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"send_message", "list_peers", "get_message"} {
		if !names[want] {
			t.Errorf("peer-only mode missing %q", want)
		}
	}
	for _, bad := range []string{"dispatch_task", "wait_task", "list_agents", "get_task", "get_task_logs", "cancel_task"} {
		if names[bad] {
			t.Errorf("peer-only mode should not expose %q", bad)
		}
	}
}

func TestOrchestratorOnlyGating(t *testing.T) {
	opSrv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer opSrv.Close()

	w, sc, cleanup := driveServerWith(t, opclient.New(opSrv.URL), "locutus", false, true, nil)
	defer cleanup()

	rpcCall(t, w, sc, 1, "initialize", map[string]any{})
	list := rpcCall(t, w, sc, 2, "tools/list", nil)
	res := list["result"].(map[string]any)
	tools := res["tools"].([]any)
	names := map[string]bool{}
	for _, tt := range tools {
		names[tt.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"dispatch_task", "wait_task", "list_agents", "get_task", "get_task_logs", "cancel_task"} {
		if !names[want] {
			t.Errorf("orchestrator-only mode missing %q", want)
		}
	}
	for _, bad := range []string{"send_message", "list_peers", "get_message"} {
		if names[bad] {
			t.Errorf("orchestrator-only mode should not expose %q", bad)
		}
	}
}

func TestCancelTask(t *testing.T) {
	stub := newStub()
	stub.tasks["task-x"] = map[string]any{
		"metadata": map[string]any{"name": "task-x"},
		"status":   map[string]any{"phase": "Running"},
	}
	opSrv := httptest.NewServer(stub.handler(t))
	defer opSrv.Close()

	w, sc, cleanup := driveServer(t, opclient.New(opSrv.URL), "locutus", nil)
	defer cleanup()

	rpcCall(t, w, sc, 1, "initialize", map[string]any{})
	resp := rpcCall(t, w, sc, 2, "tools/call", map[string]any{
		"name":      "cancel_task",
		"arguments": map[string]any{"name": "task-x"},
	})
	if _, isErr := extractText(t, resp); isErr {
		t.Fatal("cancel should succeed")
	}
	if _, ok := stub.tasks["task-x"]; ok {
		t.Error("task should have been deleted from stub")
	}
}
