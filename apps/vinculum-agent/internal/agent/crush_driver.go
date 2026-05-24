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

// CrushDriver invokes charmbracelet/crush for each task. It honors
// crush's --continue session-resume flag and recognizes the
// "no sessions found to continue" stderr signature for first-run
// fallback.
type CrushDriver struct {
	cfg    config.Config
	logger logger
}

// NewCrushDriver returns a Driver that runs `crush`.
func NewCrushDriver(cfg config.Config, l logger) *CrushDriver {
	return &CrushDriver{cfg: cfg, logger: l}
}

// Name implements Driver.
func (d *CrushDriver) Name() string { return "crush" }

// ContinueRecoverable implements Driver.
func (d *CrushDriver) ContinueRecoverable(stderr string) bool {
	return strings.Contains(stderr, "no sessions found to continue")
}

// Run implements Driver.
func (d *CrushDriver) Run(ctx context.Context, req RunRequest) RunResult {
	args := []string{"run"}
	if d.cfg.CrushQuiet {
		args = append(args, "--quiet")
	}
	if d.cfg.CrushDebug {
		args = append(args, "--debug")
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.WithContinue {
		args = append(args, "--continue")
	}
	args = append(args, req.Prompt)

	cmd := exec.CommandContext(ctx, "crush", args...)
	cmd.Dir = req.Workdir
	cmd.Env = mergeEnv(os.Environ(), req.Env)

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
