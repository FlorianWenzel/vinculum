package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/florian/vinculum/apps/vinculum-agent/internal/config"
)

// ClaudeDriver invokes Anthropic's claude-code CLI in headless
// `--print` mode for each task. claude-code reads:
//   - AGENTS.md / CLAUDE.md from cwd hierarchy (instructions)
//   - .mcp.json from cwd (MCP server wiring)
//   - Credentials from $HOME/.claude/credentials.json (Max OAuth)
//     or $ANTHROPIC_API_KEY env (pay-per-token API)
//
// Every claude --print invocation is a fresh session — there's no
// equivalent of crush's --continue, so WithContinue is ignored and
// ContinueRecoverable always returns false.
type ClaudeDriver struct {
	cfg    config.Config
	logger logger
}

// NewClaudeDriver returns a Driver that runs `claude --print`.
func NewClaudeDriver(cfg config.Config, l logger) *ClaudeDriver {
	return &ClaudeDriver{cfg: cfg, logger: l}
}

// Name implements Driver.
func (d *ClaudeDriver) Name() string { return "claude-code" }

// ContinueRecoverable implements Driver. claude --print has no
// session-continue concept, so there's no first-run recovery needed.
func (d *ClaudeDriver) ContinueRecoverable(stderr string) bool { return false }

// buildClaudeArgs is the pure logic for assembling the claude CLI argv.
// Extracted so tests can assert on it without spawning a subprocess.
// mcpConfigPath is the value of $CLAUDE_MCP_CONFIG; empty means "no
// --mcp-config flag".
func buildClaudeArgs(model, mcpConfigPath string) []string {
	args := []string{
		"--print",
		"--dangerously-skip-permissions",
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	// CLAUDE_MCP_CONFIG is set by the operator when Agent.spec.runtime
	// is claude-code and any MCP servers are configured. Empty means
	// "no MCPs" — claude-code runs without any vinculum bridge, which
	// is fine for trivial standalone tasks but disables peer messaging
	// and orchestration. Most agent configs will have it set.
	if mcpConfigPath != "" {
		args = append(args, "--mcp-config", mcpConfigPath)
	}
	return args
}

// Run implements Driver.
func (d *ClaudeDriver) Run(ctx context.Context, req RunRequest) RunResult {
	args := buildClaudeArgs(req.Model, strings.TrimSpace(os.Getenv("CLAUDE_MCP_CONFIG")))

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = req.Workdir
	cmd.Env = mergeEnv(os.Environ(), req.Env)
	// Prompt goes on stdin so it doesn't appear in `ps` and so we
	// don't hit argv-length limits on long task prompts.
	cmd.Stdin = bytes.NewBufferString(req.Prompt)

	var stdout, stderr bytes.Buffer
	if req.LogSink != nil {
		cmd.Stdout = io.MultiWriter(&stdout, req.LogSink)
		cmd.Stderr = io.MultiWriter(&stderr, req.LogSink)
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Err:      err,
	}
}
