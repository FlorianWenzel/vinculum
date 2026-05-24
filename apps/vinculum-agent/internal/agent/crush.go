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
	"github.com/florian/vinculum/apps/vinculum-agent/internal/git"
	"github.com/florian/vinculum/apps/vinculum-agent/internal/tasks"
)

const tailBytes = 16 * 1024

// Executor runs the configured LLM driver for each task and mutates
// State with output + artifacts. The runtime-agnostic logic (git
// pre/post, instruction symlinking, artifact persistence) lives here;
// the driver handles the CLI invocation.
type Executor struct {
	cfg    config.Config
	logger logger
	drv    Driver
}

type logger interface {
	Printf(format string, v ...any)
}

// NewExecutor returns an Executor wired to CrushDriver. Preserved for
// backward compatibility with callers (and tests) that pre-date the
// Driver abstraction. New code should call NewExecutorWithDriver.
func NewExecutor(cfg config.Config, l logger) *Executor {
	return NewExecutorWithDriver(cfg, l, NewCrushDriver(cfg, l))
}

// NewExecutorWithDriver returns an Executor that delegates the
// per-task CLI invocation to the supplied Driver.
func NewExecutorWithDriver(cfg config.Config, l logger, drv Driver) *Executor {
	if drv == nil {
		drv = NewCrushDriver(cfg, l)
	}
	return &Executor{cfg: cfg, logger: l, drv: drv}
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

	// If the Task carries a git workflow, redirect crush's working directory
	// to the cloned repo and run the pre-crush branch checkout before
	// running crush.
	runDir := workdir
	gitSpec := state.Payload.Spec.Git
	if gitSpec != nil {
		repoPath := strings.TrimSpace(os.Getenv("REPO_PATH"))
		if repoPath == "" {
			return errors.New("Task.spec.git requires Agent.spec.repo (REPO_PATH env not set)")
		}
		if err := e.gitPreCrush(ctx, repoPath, gitSpec, state.Payload.Name); err != nil {
			return fmt.Errorf("git pre-crush: %w", err)
		}
		runDir = repoPath
	}

	wantContinue := !state.Payload.Spec.Fresh
	// A per-Task model override on a continued session would feed the new
	// model the prior model's session state — usually garbage. Force a
	// fresh run.
	if wantContinue && strings.TrimSpace(state.Payload.Spec.Model) != "" {
		if e.logger != nil {
			e.logger.Printf("task overrides model %q; forcing --fresh to avoid mixing session state", state.Payload.Spec.Model)
		}
		wantContinue = false
	}

	e.ensureInstructions(runDir)
	model := strings.TrimSpace(state.Payload.Spec.Model)
	if model == "" {
		model = e.cfg.CrushModel
	}
	req := RunRequest{
		Workdir:      runDir,
		Prompt:       prompt,
		Model:        model,
		WithContinue: wantContinue,
		Env:          state.Payload.Spec.Env,
		LogSink:      state.Logs,
	}
	res := e.drv.Run(ctx, req)

	// First-ever task on a fresh agent: the driver may refuse to
	// continue a session that doesn't exist yet. Retry once fresh.
	if wantContinue && res.ExitCode != 0 && e.drv.ContinueRecoverable(res.Stderr) {
		if e.logger != nil {
			e.logger.Printf("%s: no prior session, retrying without --continue", e.drv.Name())
		}
		req.WithContinue = false
		res = e.drv.Run(ctx, req)
	}

	state.StdoutTail = tail(res.Stdout, tailBytes)
	state.StderrTail = tail(res.Stderr, tailBytes)
	state.ExitCode = res.ExitCode
	if res.Err != nil && res.ExitCode == -1 {
		return res.Err
	}
	if res.ExitCode != 0 {
		state.Reason = "NonZeroExit"
		state.Message = fmt.Sprintf("%s exited %d", e.drv.Name(), res.ExitCode)
		return fmt.Errorf("%s exited %d", e.drv.Name(), res.ExitCode)
	}

	if gitSpec != nil {
		if err := e.gitPostCrush(ctx, state, gitSpec); err != nil {
			return fmt.Errorf("git post-crush: %w", err)
		}
	}

	uris, err := e.persistArtifacts(ctx, state, workdir)
	if err != nil {
		return fmt.Errorf("persist artifacts: %w", err)
	}
	state.Artifacts = append(state.Artifacts, uris...)
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
		// The agent pod can only mount what the operator declared at Pod
		// creation time, and the workspace PVC is the one mount we know is
		// always present. Treat `pvc` as "copy results to a subpath inside
		// the workspace PVC so downstream consumers can mount it
		// read-only and inspect outputs."
		if sink.PVC == nil || sink.PVC.SubPath == "" {
			return nil, errors.New("pvc artifacts require subPath (relative to /workspace)")
		}
		workspaceRoot := strings.TrimSpace(os.Getenv("WORKSPACE_ROOT"))
		if workspaceRoot == "" {
			workspaceRoot = "/workspace"
		}
		dest := filepath.Join(workspaceRoot, strings.TrimPrefix(sink.PVC.SubPath, "/"))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(dest), err)
		}
		// Use `cp -a` to preserve mode + symlinks. The agent image has
		// coreutils.
		cmd := exec.CommandContext(ctx, "cp", "-a", source+"/.", dest)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("cp to %s: %s: %w", dest, strings.TrimSpace(string(out)), err)
		}
		claim := sink.PVC.ClaimName
		if claim == "" {
			// SubPath without claimName implicitly targets the agent's own
			// workspace PVC. Reflect that in the returned URI.
			claim = "agent-" + state.Payload.Spec.AgentRef + "-workspace"
		}
		return []string{"pvc://" + claim + "/" + strings.TrimPrefix(sink.PVC.SubPath, "/")}, nil
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

func (e *Executor) ensureInstructions(workdir string) {
	src := strings.TrimSpace(e.cfg.InstructionFile)
	if src == "" {
		return
	}
	if _, err := os.Stat(src); err != nil {
		return
	}
	base := filepath.Base(src)
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "AGENTS.md"
	}
	dst := filepath.Join(workdir, base)
	if info, err := os.Lstat(dst); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			// leave user-authored file in place
			return
		}
		if target, err := os.Readlink(dst); err == nil && target == src {
			return
		}
		_ = os.Remove(dst)
	}
	if err := os.Symlink(src, dst); err != nil {
		e.logger.Printf("instruction symlink %s -> %s: %v", dst, src, err)
	}
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

// gitPreCrush runs the pre-task git steps: fetch origin and check out the
// head branch (creating it if needed) off the base branch.
func (e *Executor) gitPreCrush(ctx context.Context, repoPath string, spec *tasks.TaskGit, taskName string) error {
	base := strings.TrimSpace(spec.BaseBranch)
	if base == "" {
		base = strings.TrimSpace(os.Getenv("REPO_BRANCH"))
	}
	if base == "" {
		return errors.New("baseBranch is required (Task.spec.git.baseBranch or Agent.spec.repo.branch)")
	}
	head := strings.TrimSpace(spec.HeadBranch)
	if head == "" {
		head = "vinculum/task-" + taskName
	}
	repo := git.Open(repoPath)
	if err := repo.Verify(ctx); err != nil {
		return fmt.Errorf("repo at %s not a git work tree: %w", repoPath, err)
	}
	// Keep vinculum's runtime droppings out of the commit. The instruction
	// file (AGENTS.md by default) gets symlinked into the workdir before
	// each crush run; without an exclude entry it lands in `git status` and
	// gets staged by AddAll.
	excludes := []string{".vnclm-scratch/"}
	if name := filepath.Base(strings.TrimSpace(e.cfg.InstructionFile)); name != "" && name != "." && name != string(filepath.Separator) {
		excludes = append(excludes, name)
	}
	if err := repo.AppendInfoExclude(ctx, excludes...); err != nil {
		if e.logger != nil {
			e.logger.Printf("git info/exclude: %v (continuing)", err)
		}
	}
	if err := repo.Fetch(ctx); err != nil {
		// Fetch can fail on network blips or in tests with a local-only
		// remote — log it but don't abort. Checkout still needs the local
		// base branch to exist.
		if e.logger != nil {
			e.logger.Printf("git fetch: %v (continuing)", err)
		}
	}
	if err := repo.CheckoutNewBranch(ctx, head, base); err != nil {
		return fmt.Errorf("checkout %s from %s: %w", head, base, err)
	}
	if e.logger != nil {
		e.logger.Printf("git: checked out %s from %s", head, base)
	}
	return nil
}

// gitPostCrush stages, commits, pushes, and (optionally) opens a PR after a
// successful crush run. If the working tree is clean, the Task is marked
// Succeeded with reason=NoChanges and the rest is skipped.
func (e *Executor) gitPostCrush(ctx context.Context, state *tasks.State, spec *tasks.TaskGit) error {
	repoPath := strings.TrimSpace(os.Getenv("REPO_PATH"))
	repo := git.Open(repoPath)

	dirty, err := repo.HasChanges(ctx)
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if !dirty {
		state.Reason = "NoChanges"
		state.Message = "crush made no changes to the working tree"
		if e.logger != nil {
			e.logger.Printf("git: no changes after crush, skipping commit + push")
		}
		return nil
	}

	if err := repo.AddAll(ctx); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	msg := strings.TrimSpace(spec.CommitMessage)
	if msg == "" {
		msg = "vinculum: " + state.Payload.Name
	}
	if err := repo.Commit(ctx, msg, false); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	head := strings.TrimSpace(spec.HeadBranch)
	if head == "" {
		head = "vinculum/task-" + state.Payload.Name
	}
	// force-with-lease lets a re-run with the same headBranch overwrite the
	// prior commit safely (refuses if a third party pushed since). Without
	// this, re-runs fail with non-fast-forward.
	if err := repo.Push(ctx, head, true, true); err != nil {
		return fmt.Errorf("git push: %w", err)
	}
	sha, _ := repo.HeadSHA(ctx)
	if e.logger != nil {
		e.logger.Printf("git: pushed %s @ %s", head, sha)
	}

	// PR creation is optional and GitHub-only for v0.3. Empty PRTitle or
	// SkipPR short-circuits.
	if spec.SkipPR || strings.TrimSpace(spec.PRTitle) == "" {
		return nil
	}
	remoteURL, err := repo.RemoteURL(ctx)
	if err != nil {
		return fmt.Errorf("git remote: %w", err)
	}
	owner, name, err := git.ParseGitHubRepo(remoteURL)
	if err != nil {
		if e.logger != nil {
			e.logger.Printf("git: %v — skipping PR creation (non-GitHub remote)", err)
		}
		return nil
	}
	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GIT_TOKEN"))
	}
	if token == "" {
		return errors.New("PRTitle set but no GitHub token available (set gitCredentials.tokenSecretRef with key GITHUB_TOKEN or token)")
	}
	base := strings.TrimSpace(spec.BaseBranch)
	if base == "" {
		base = strings.TrimSpace(os.Getenv("REPO_BRANCH"))
	}
	body := spec.PRBody
	if body == "" {
		body = state.StdoutTail
	}
	pr, err := git.NewGitHubClient(token).CreatePR(ctx, owner, name, git.GitHubPRRequest{
		Title: spec.PRTitle, Head: head, Base: base, Body: body,
	})
	if err != nil {
		return fmt.Errorf("create PR: %w", err)
	}
	state.Artifacts = append(state.Artifacts, pr.HTMLURL)
	if e.logger != nil {
		e.logger.Printf("git: opened PR #%d at %s", pr.Number, pr.HTMLURL)
	}
	return nil
}
