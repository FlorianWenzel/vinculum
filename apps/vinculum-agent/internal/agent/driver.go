package agent

import (
	"context"
)

// Driver abstracts the LLM-runner CLI the Executor invokes per task. The
// runtime-agnostic logic in Executor (git pre/post, artifact persistence,
// instruction symlinking) is the same regardless of driver; only the
// command that actually talks to the model differs.
//
// Two drivers exist today: CrushDriver (charmbracelet/crush) and
// ClaudeDriver (Anthropic claude-code). Adding another runtime is a new
// type implementing this interface plus a wiring entry in
// cmd/vinculum-agent/main.go.
type Driver interface {
	// Name is the human-readable runtime name. Used for log lines and
	// for the "<name> exited %d" Task.Status.Message.
	Name() string

	// Run invokes the underlying CLI synchronously and returns its
	// stdout, stderr, exit code, and (only on process-spawn failure)
	// an error. A non-zero exitCode with err==nil means the CLI ran
	// but returned an error code — the caller decides what that means.
	//
	// req.WithContinue is advisory: a driver may ignore it when its
	// CLI has no notion of session continuation. CrushDriver maps it
	// to --continue; ClaudeDriver currently ignores it (every claude
	// --print invocation is a fresh session).
	Run(ctx context.Context, req RunRequest) RunResult

	// ContinueRecoverable reports whether a non-zero exit looks like
	// "you asked to continue but there's no prior session." The
	// Executor uses this to retry once with WithContinue=false on the
	// agent's first-ever Task. Returns false for any driver whose CLI
	// doesn't surface this failure mode.
	ContinueRecoverable(stderr string) bool
}

// RunRequest is everything a Driver needs to execute one task turn.
type RunRequest struct {
	// Workdir is the directory the CLI runs in. Already exists.
	Workdir string
	// Prompt is the user message verbatim. Drivers are responsible
	// for passing it on the appropriate channel (argv for crush,
	// stdin for claude --print, etc).
	Prompt string
	// Model, when non-empty, overrides whatever default the agent
	// config has baked in.
	Model string
	// WithContinue requests session continuation. Drivers that don't
	// support this concept ignore it.
	WithContinue bool
	// Env is overlayed onto the process environment. Already-set
	// keys take precedence so a Task can override a pod env.
	Env map[string]string
	// LogSink, when non-nil, receives a copy of stdout+stderr in
	// real time (in addition to being captured into the buffers
	// returned in RunResult). Drivers should tee, not redirect.
	LogSink interface{ Write(p []byte) (int, error) }
}

// RunResult is the return value of Driver.Run.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	// Err is set only when the CLI failed to start or was killed by
	// signal. A normal non-zero exit reports ExitCode != 0 and
	// Err == nil so callers can distinguish "process ran and failed"
	// from "process didn't run".
	Err error
}
