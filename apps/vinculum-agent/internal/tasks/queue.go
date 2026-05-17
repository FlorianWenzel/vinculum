package tasks

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/florian/vinculum/apps/vinculum-agent/internal/config"
	"github.com/florian/vinculum/apps/vinculum-agent/internal/kube"
)

// Runner executes tasks serially out of an in-memory FIFO queue.
type Runner struct {
	cfg    config.Config
	logger *log.Logger
	kube   *kube.Client
	exec   Executor

	mu       sync.Mutex
	queue    []*State
	byName   map[string]*State
	current  *State
	cancel   context.CancelFunc
	notify   chan struct{}
}

// Executor abstracts the crush invocation so tests can substitute a fake.
type Executor interface {
	Execute(ctx context.Context, state *State, workdir string) error
}

func NewRunner(cfg config.Config, logger *log.Logger, k *kube.Client, exec Executor) *Runner {
	return &Runner{
		cfg:    cfg,
		logger: logger,
		kube:   k,
		exec:   exec,
		byName: map[string]*State{},
		notify: make(chan struct{}, 1),
	}
}

var (
	ErrAlreadyExists = errors.New("task already accepted")
)

// Enqueue accepts a new task. Returns ErrAlreadyExists on duplicate name.
func (r *Runner) Enqueue(payload DispatchPayload) (*State, error) {
	r.mu.Lock()
	if _, ok := r.byName[payload.Name]; ok {
		r.mu.Unlock()
		return nil, ErrAlreadyExists
	}
	state := &State{
		Payload:    payload,
		Phase:      PhaseQueued,
		EnqueuedAt: time.Now(),
	}
	r.byName[payload.Name] = state
	r.queue = append(r.queue, state)
	r.mu.Unlock()

	select {
	case r.notify <- struct{}{}:
	default:
	}
	return state, nil
}

func (r *Runner) Get(name string) (*State, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byName[name]
	return s, ok
}

func (r *Runner) List() []*State {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*State, 0, len(r.byName))
	for _, s := range r.byName {
		out = append(out, s)
	}
	return out
}

// Cancel removes a queued task or cancels the in-flight one.
func (r *Runner) Cancel(name string) bool {
	r.mu.Lock()
	if r.current != nil && r.current.Payload.Name == name && r.cancel != nil {
		r.cancel()
		r.mu.Unlock()
		return true
	}
	for i, s := range r.queue {
		if s.Payload.Name == name {
			r.queue = append(r.queue[:i], r.queue[i+1:]...)
			delete(r.byName, name)
			r.mu.Unlock()
			return true
		}
	}
	r.mu.Unlock()
	return false
}

// Current returns the in-flight task name, if any.
func (r *Runner) Current() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		return ""
	}
	return r.current.Payload.Name
}

// Run consumes the queue until ctx is done. On ctx cancel, in-flight task is
// marked Failed (AgentRestarted) and the operator re-dispatches queued tasks.
func (r *Runner) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			r.drainOnShutdown()
			return
		case <-r.notify:
		case <-time.After(2 * time.Second):
		}
		for {
			state := r.pop()
			if state == nil {
				break
			}
			r.execute(ctx, state)
			if ctx.Err() != nil {
				r.drainOnShutdown()
				return
			}
		}
	}
}

func (r *Runner) pop() *State {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queue) == 0 {
		return nil
	}
	state := r.queue[0]
	r.queue = r.queue[1:]
	r.current = state
	return state
}

func (r *Runner) execute(parent context.Context, state *State) {
	runCtx, cancel := context.WithCancel(parent)
	if t := state.Payload.Spec.TimeoutSeconds; t > 0 {
		runCtx, cancel = context.WithTimeout(parent, time.Duration(t)*time.Second)
	}
	r.mu.Lock()
	r.cancel = cancel
	r.mu.Unlock()

	start := time.Now()
	state.StartedAt = &start
	state.Phase = PhaseRunning

	logDir := filepath.Join(r.cfg.WorkspaceRoot, ".tasks", state.Payload.Name)
	_ = os.MkdirAll(logDir, 0o755)
	state.LogFile = filepath.Join(logDir, "log")
	state.Logs = NewLogBuffer(state.LogFile)

	r.patchStatus(state, map[string]any{
		"phase":     "Running",
		"startTime": start.UTC().Format(time.RFC3339),
		"podName":   podName(),
	})

	workdir := r.cfg.WorkspaceRoot
	if state.Payload.Spec.Workspace != nil && state.Payload.Spec.Workspace.Mode == "ephemeral" {
		workdir = fmt.Sprintf("%s/task-%s", r.cfg.WorkspaceRoot, state.Payload.Name)
	}

	err := r.exec.Execute(runCtx, state, workdir)
	end := time.Now()
	state.FinishedAt = &end
	cancel()
	if state.Logs != nil {
		state.Logs.Close()
	}

	r.mu.Lock()
	r.current = nil
	r.cancel = nil
	r.mu.Unlock()

	phase := PhaseSucceeded
	if parent.Err() != nil {
		phase = PhaseFailed
		state.Reason = "AgentRestarted"
		state.Message = "pod shutdown before task completion"
	} else if runCtx.Err() == context.DeadlineExceeded {
		phase = PhaseTimedOut
		state.Reason = "Timeout"
		state.Message = "task exceeded timeout"
	} else if err != nil {
		phase = PhaseFailed
		if state.Reason == "" {
			state.Reason = "RunFailed"
		}
		if state.Message == "" {
			state.Message = err.Error()
		}
	}
	state.Phase = phase
	status := map[string]any{
		"phase":          mapPhase(phase),
		"exitCode":       state.ExitCode,
		"stdoutTail":     state.StdoutTail,
		"stderrTail":     state.StderrTail,
		"completionTime": end.UTC().Format(time.RFC3339),
	}
	if len(state.Artifacts) > 0 {
		status["artifactURLs"] = state.Artifacts
	}
	if state.Reason != "" {
		status["reason"] = state.Reason
	}
	if state.Message != "" {
		status["message"] = state.Message
	}
	r.patchStatus(state, status)
}

func (r *Runner) drainOnShutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.queue {
		r.patchStatus(s, map[string]any{
			"phase":   "Pending",
			"reason":  "AgentRestarted",
			"message": "pod shutdown; awaiting redispatch",
		})
	}
	r.queue = nil
}

func (r *Runner) patchStatus(state *State, status map[string]any) {
	if r.kube == nil {
		return
	}
	// Messages don't carry the same status shape as Tasks (no stdoutTail,
	// no exit code on the CRD). Their lifecycle is owned by the operator's
	// MessageReconciler; the agent just runs the crush turn.
	if state.Payload.Kind == KindMessage {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.kube.PatchStatus(ctx, state.Payload.Name, status); err != nil {
		r.logger.Printf("patch status %s: %v", state.Payload.Name, err)
	}
}

func mapPhase(p string) string {
	switch p {
	case PhaseSucceeded:
		return "Succeeded"
	case PhaseFailed:
		return "Failed"
	case PhaseTimedOut:
		return "TimedOut"
	case PhaseRunning:
		return "Running"
	case PhaseQueued:
		return "Pending"
	}
	return p
}
