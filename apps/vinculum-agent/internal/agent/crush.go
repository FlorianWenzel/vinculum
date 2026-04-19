package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/florian/vinculum/apps/vinculum-agent/internal/config"
	"github.com/florian/vinculum/apps/vinculum-agent/internal/tasks"
)

const tailBytes = 16 * 1024

// Executor runs crush for each task and mutates State with output + artifacts.
type Executor struct {
	cfg    config.Config
	logger logger
}

type logger interface {
	Printf(format string, v ...any)
}

func NewExecutor(cfg config.Config, l logger) *Executor {
	return &Executor{cfg: cfg, logger: l}
}

func (e *Executor) Execute(ctx context.Context, state *tasks.State, workdir string) error {
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return fmt.Errorf("mkdir workdir: %w", err)
	}
	ephemeral := state.Payload.Spec.Workspace != nil && state.Payload.Spec.Workspace.Mode == "ephemeral"
	if ephemeral {
		defer os.RemoveAll(workdir)
	}

	prompt := strings.TrimSpace(state.Payload.Spec.Prompt)
	if prompt == "" {
		return errors.New("prompt is required")
	}

	wantContinue := !state.Payload.Spec.Fresh
	stdout, stderr, exitCode, runErr := e.runCrush(ctx, state, workdir, prompt, wantContinue)

	// crush fails hard when --continue is requested but no prior session exists.
	// Fall back to a fresh run so the first Task on a new Agent doesn't error out.
	if wantContinue && exitCode != 0 && strings.Contains(stderr, "no sessions found to continue") {
		if e.logger != nil {
			e.logger.Printf("crush: no prior session, retrying without --continue")
		}
		stdout, stderr, exitCode, runErr = e.runCrush(ctx, state, workdir, prompt, false)
	}

	state.StdoutTail = tail(stdout, tailBytes)
	state.StderrTail = tail(stderr, tailBytes)
	state.ExitCode = exitCode
	if runErr != nil && exitCode == -1 {
		return runErr
	}
	if exitCode != 0 {
		state.Reason = "NonZeroExit"
		state.Message = fmt.Sprintf("crush exited %d", exitCode)
		return fmt.Errorf("crush exited %d", exitCode)
	}

	uris, err := e.persistArtifacts(ctx, state, workdir)
	if err != nil {
		return fmt.Errorf("persist artifacts: %w", err)
	}
	state.Artifacts = uris
	return nil
}

func (e *Executor) persistArtifacts(ctx context.Context, state *tasks.State, workdir string) ([]string, error) {
	sink := state.Payload.Spec.Artifacts
	if sink == nil || strings.EqualFold(sink.Type, "") || strings.EqualFold(sink.Type, "none") {
		return nil, nil
	}
	source := sink.SourceDir
	if source == "" {
		source = workdir
	}
	if !filepath.IsAbs(source) {
		source = filepath.Join(workdir, source)
	}
	switch strings.ToLower(sink.Type) {
	case "s3":
		if sink.S3 == nil || sink.S3.Bucket == "" {
			return nil, errors.New("s3 artifacts require bucket")
		}
		uri := "s3://" + sink.S3.Bucket
		if sink.S3.Prefix != "" {
			uri = uri + "/" + strings.TrimPrefix(sink.S3.Prefix, "/")
		}
		args := []string{"s3", "sync", source, uri}
		if sink.S3.Endpoint != "" {
			args = append([]string{"--endpoint-url", sink.S3.Endpoint}, args...)
		}
		if sink.S3.Region != "" {
			args = append([]string{"--region", sink.S3.Region}, args...)
		}
		cmd := exec.CommandContext(ctx, "aws", args...)
		cmd.Dir = source
		cmd.Env = os.Environ()
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("aws s3 sync: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return []string{uri}, nil
	case "pvc":
		return nil, nil
	case "webhook":
		if sink.Webhook == nil || sink.Webhook.URL == "" {
			return nil, errors.New("webhook artifacts require url")
		}
		payload := fmt.Sprintf(`{"agent":%q,"task":%q,"source":%q}`, state.Payload.Spec.AgentRef, state.Payload.Name, source)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, sink.Webhook.URL, bytes.NewBufferString(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		cl := &http.Client{Timeout: 30 * time.Second}
		resp, err := cl.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("webhook %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return []string{sink.Webhook.URL}, nil
	default:
		return nil, fmt.Errorf("unsupported artifacts type %q", sink.Type)
	}
}

func (e *Executor) runCrush(ctx context.Context, state *tasks.State, workdir, prompt string, withContinue bool) (string, string, int, error) {
	args := []string{"run"}
	if e.cfg.CrushQuiet {
		args = append(args, "--quiet")
	}
	if e.cfg.CrushDebug {
		args = append(args, "--debug")
	}
	if e.cfg.CrushModel != "" {
		args = append(args, "--model", e.cfg.CrushModel)
	}
	if withContinue {
		args = append(args, "--continue")
	}
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, "crush", args...)
	cmd.Dir = workdir
	cmd.Env = mergeEnv(os.Environ(), state.Payload.Spec.Env)

	var stdout, stderr bytes.Buffer
	if state.Logs != nil {
		cmd.Stdout = io.MultiWriter(&stdout, state.Logs)
		cmd.Stderr = io.MultiWriter(&stderr, state.Logs)
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
	return stdout.String(), stderr.String(), exitCode, err
}

func mergeEnv(base []string, overlay map[string]string) []string {
	if len(overlay) == 0 {
		return base
	}
	existing := map[string]int{}
	for i, kv := range base {
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			existing[kv[:eq]] = i
		}
	}
	for k, v := range overlay {
		kv := k + "=" + v
		if idx, ok := existing[k]; ok {
			base[idx] = kv
		} else {
			base = append(base, kv)
		}
	}
	return base
}

func tail(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[len(s)-limit:]
}
