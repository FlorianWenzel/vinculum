package opclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestListAgents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agents" || r.Method != http.MethodGet {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"metadata": map[string]any{"name": "locutus", "namespace": "vinculum"},
				"spec":     map[string]any{"model": "claude-opus-4-7", "orchestrator": true},
				"status":   map[string]any{"phase": "Ready", "ready": true},
			},
			{
				"metadata": map[string]any{"name": "drone-7", "namespace": "vinculum"},
				"spec":     map[string]any{"model": "claude-sonnet-4-6"},
				"status":   map[string]any{"phase": "Pending"},
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	agents, err := c.ListAgents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 2 {
		t.Fatalf("want 2 agents, got %d", len(agents))
	}
	if agents[0].Name != "locutus" || !agents[0].Ready || !agents[0].Orchestrator {
		t.Errorf("locutus parsing: %+v", agents[0])
	}
	if agents[1].Phase != "Pending" || agents[1].Ready {
		t.Errorf("drone-7 parsing: %+v", agents[1])
	}
}

func TestDispatchAndGetTask(t *testing.T) {
	var got struct {
		Name string `json:"name"`
		Spec struct {
			AgentRef string `json:"agentRef"`
			Prompt   string `json:"prompt"`
			Fresh    bool   `json:"fresh"`
		} `json:"spec"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/tasks":
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"metadata": map[string]any{"name": got.Name, "namespace": "vinculum"},
				"spec":     map[string]any{"agentRef": got.Spec.AgentRef},
				"status":   map[string]any{"phase": "Pending"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/tasks/t-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"metadata": map[string]any{"name": "t-1", "namespace": "vinculum"},
				"spec":     map[string]any{"agentRef": "drone-7"},
				"status":   map[string]any{"phase": "Succeeded", "exitCode": 0, "stdoutTail": "hello"},
			})
		default:
			http.Error(w, "nope", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := New(srv.URL)
	created, err := c.DispatchTask(context.Background(), DispatchTaskInput{
		Name: "t-1", AgentRef: "drone-7", Prompt: "go", Fresh: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "t-1" || created.AgentRef != "drone-7" {
		t.Errorf("dispatch result: %+v", created)
	}
	if got.Spec.AgentRef != "drone-7" || got.Spec.Prompt != "go" || !got.Spec.Fresh {
		t.Errorf("dispatch payload: %+v", got)
	}

	finished, err := c.GetTask(context.Background(), "t-1")
	if err != nil {
		t.Fatal(err)
	}
	if finished.Phase != "Succeeded" || finished.StdoutTail != "hello" {
		t.Errorf("get task: %+v", finished)
	}
}

func TestWaitTask_TerminalAfterPolls(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		phase := "Running"
		if n >= 3 {
			phase = "Succeeded"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]any{"name": "t", "namespace": "vinculum"},
			"status":   map[string]any{"phase": phase},
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	t0 := time.Now()
	got, err := c.WaitTask(ctx, "t", 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != "Succeeded" {
		t.Errorf("want Succeeded, got %s", got.Phase)
	}
	if atomic.LoadInt32(&calls) < 3 {
		t.Errorf("expected at least 3 polls, got %d", calls)
	}
	if time.Since(t0) > 1*time.Second {
		t.Errorf("wait took too long: %v", time.Since(t0))
	}
}

func TestWaitTask_ContextDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]any{"name": "t"},
			"status":   map[string]any{"phase": "Running"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := c.WaitTask(ctx, "t", 30*time.Millisecond)
	if err == nil {
		t.Fatal("expected deadline error")
	}
}

func TestIsTerminal(t *testing.T) {
	for _, c := range []struct {
		phase string
		want  bool
	}{
		{"Succeeded", true}, {"Failed", true}, {"TimedOut", true},
		{"Pending", false}, {"Running", false}, {"Dispatching", false}, {"", false},
	} {
		if got := IsTerminal(c.phase); got != c.want {
			t.Errorf("IsTerminal(%q)=%v, want %v", c.phase, got, c.want)
		}
	}
}
