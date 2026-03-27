package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/florian/vinculum/apps/vinculum-agent/internal/config"
)

type Service struct {
	cfg    config.Config
	logger *log.Logger
	cmd    *exec.Cmd
	mu     sync.Mutex
}

type RunRequest struct {
	Prompt              string   `json:"prompt"`
	WorkingDirectory    string   `json:"workingDirectory,omitempty"`
	Model               string   `json:"model,omitempty"`
	Agent               string   `json:"agent,omitempty"`
	Session             string   `json:"session,omitempty"`
	Continue            bool     `json:"continue,omitempty"`
	Fork                bool     `json:"fork,omitempty"`
	IncludeInstructions bool     `json:"includeInstructions,omitempty"`
	InstructionFiles    []string `json:"instructionFiles,omitempty"`
}

type RunResult struct {
	Command   []string `json:"command"`
	Directory string   `json:"directory"`
	ExitCode  int      `json:"exitCode"`
	Stdout    string   `json:"stdout"`
	Stderr    string   `json:"stderr"`
}

type ExecRequest struct {
	Command          string   `json:"command"`
	Args             []string `json:"args,omitempty"`
	WorkingDirectory string   `json:"workingDirectory,omitempty"`
	TimeoutSeconds   int      `json:"timeoutSeconds,omitempty"`
}

type Info struct {
	WorkDir         string   `json:"workDir"`
	InstructionsDir string   `json:"instructionsDir"`
	OpenCodeURL     string   `json:"opencodeUrl"`
	Binaries        []string `json:"binaries"`
	InstructionDocs []string `json:"instructionDocs"`
}

type AutoRunResult struct {
	RunResult
	RepositoryDir string `json:"repositoryDir,omitempty"`
	BaseBranch    string `json:"baseBranch,omitempty"`
	WorkingBranch string `json:"workingBranch,omitempty"`
	CommitSHA     string `json:"commitSha,omitempty"`
	Pushed        bool   `json:"pushed,omitempty"`
}

func NewService(cfg config.Config, logger *log.Logger) *Service {
	return &Service{cfg: cfg, logger: logger}
}

func (s *Service) Start(ctx context.Context) error {
	if err := os.MkdirAll(s.cfg.WorkDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(s.cfg.InstructionsDir, 0o755); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "opencode", "serve", "--hostname", s.cfg.OpenCodeHost, "--port", s.cfg.OpenCodePort, "--log-level", s.cfg.OpenCodeLogLevel)
	cmd.Dir = s.cfg.WorkDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		return err
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	if err := s.waitForPort(20 * time.Second); err != nil {
		return err
	}

	s.logger.Printf("opencode server ready on %s", s.OpenCodeURL())
	return nil
}

func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}
	_ = s.cmd.Process.Signal(syscall.SIGTERM)
	_, _ = s.cmd.Process.Wait()
	s.cmd = nil
}

func (s *Service) OpenCodeURL() string {
	return fmt.Sprintf("http://%s:%s", s.cfg.OpenCodeHost, s.cfg.OpenCodePort)
}

func (s *Service) Info() Info {
	files, _ := s.listInstructionFiles()
	return Info{
		WorkDir:         s.cfg.WorkDir,
		InstructionsDir: s.cfg.InstructionsDir,
		OpenCodeURL:     s.OpenCodeURL(),
		Binaries:        []string{"opencode", "git", "ssh", "tea"},
		InstructionDocs: files,
	}
}

func (s *Service) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return RunResult{}, fmt.Errorf("prompt is required")
	}

	prompt := strings.TrimSpace(req.Prompt)
	if req.IncludeInstructions {
		instructions, err := s.buildInstructions(req.InstructionFiles)
		if err != nil {
			return RunResult{}, err
		}
		if instructions != "" {
			prompt = instructions + "\n\nUser task:\n" + prompt
		}
	}

	dir := s.resolveDir(req.WorkingDirectory)
	args := []string{"run", "--attach", s.OpenCodeURL(), "--dir", dir}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	} else if s.cfg.OpenCodeModel != "" {
		args = append(args, "--model", s.cfg.OpenCodeModel)
	}
	if req.Agent != "" {
		args = append(args, "--agent", req.Agent)
	} else if s.cfg.OpenCodeAgent != "" {
		args = append(args, "--agent", s.cfg.OpenCodeAgent)
	}
	if req.Session != "" {
		args = append(args, "--session", req.Session)
	}
	if req.Continue {
		args = append(args, "--continue")
	}
	if req.Fork {
		args = append(args, "--fork")
	}
	args = append(args, prompt)

	return s.runCommand(ctx, dir, "opencode", args...)
}

func (s *Service) AutoRun(ctx context.Context) (AutoRunResult, error) {
	result := AutoRunResult{
		BaseBranch:    valueOr(strings.TrimSpace(s.cfg.TaskBaseBranch), "main"),
		WorkingBranch: strings.TrimSpace(s.cfg.TaskWorkingBranch),
	}

	workDir := s.resolveDir(s.cfg.AutoRunDir)
	if strings.TrimSpace(s.cfg.TaskRepoURL) != "" {
		repoDir, err := s.prepareRepository(ctx, workDir, result.BaseBranch, result.WorkingBranch)
		if err != nil {
			return AutoRunResult{}, err
		}
		result.RepositoryDir = repoDir
		workDir = repoDir
	}

	runResult, err := s.Run(ctx, RunRequest{
		Prompt:              s.cfg.AutoRunPrompt,
		WorkingDirectory:    workDir,
		IncludeInstructions: s.cfg.AutoRunWithDocs,
	})
	if err != nil {
		return AutoRunResult{}, err
	}
	result.RunResult = runResult

	if result.RepositoryDir == "" {
		return result, nil
	}

	branch, sha, err := s.finalizeRepository(ctx, result.RepositoryDir, result.BaseBranch)
	if err != nil {
		return AutoRunResult{}, err
	}
	result.WorkingBranch = branch
	result.CommitSHA = sha
	result.Pushed = true
	return result, nil

}

func (s *Service) Exec(ctx context.Context, req ExecRequest) (RunResult, error) {
	if strings.TrimSpace(req.Command) == "" {
		return RunResult{}, fmt.Errorf("command is required")
	}
	if req.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
		defer cancel()
	}
	return s.runCommand(ctx, s.resolveDir(req.WorkingDirectory), req.Command, req.Args...)
}

func (s *Service) runCommand(ctx context.Context, dir, bin string, args ...string) (RunResult, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return RunResult{}, err
		}
	}
	return RunResult{Command: append([]string{bin}, args...), Directory: dir, ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

func (s *Service) resolveDir(path string) string {
	if strings.TrimSpace(path) == "" {
		return s.cfg.WorkDir
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.cfg.WorkDir, path)
}

func (s *Service) prepareRepository(ctx context.Context, dir, baseBranch, workingBranch string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(dir); err == nil {
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			return "", readErr
		}
		if len(entries) > 0 {
			return "", fmt.Errorf("repository directory %s is not empty", dir)
		}
	}
	if _, err := s.runCommandStrict(ctx, filepath.Dir(dir), "git", "clone", s.cfg.TaskRepoURL, filepath.Base(dir)); err != nil {
		return "", err
	}
	if _, err := s.runCommandStrict(ctx, dir, "git", "config", "user.name", s.cfg.GitAuthorName); err != nil {
		return "", err
	}
	if _, err := s.runCommandStrict(ctx, dir, "git", "config", "user.email", s.cfg.GitAuthorEmail); err != nil {
		return "", err
	}
	branch := strings.TrimSpace(workingBranch)
	if branch == "" {
		branch = baseBranch
	}
	checkoutArgs := []string{"checkout", "-B", branch}
	if baseBranch != "" {
		checkoutArgs = append(checkoutArgs, "origin/"+baseBranch)
	}
	if _, err := s.runCommandStrict(ctx, dir, "git", checkoutArgs...); err != nil {
		fallbackArgs := []string{"checkout", "-B", branch}
		if _, fallbackErr := s.runCommandStrict(ctx, dir, "git", fallbackArgs...); fallbackErr != nil {
			return "", err
		}
	}
	return dir, nil
}

func (s *Service) finalizeRepository(ctx context.Context, dir, baseBranch string) (string, string, error) {
	branch, err := s.runCommandStrict(ctx, dir, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", "", err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" || branch == "HEAD" {
		return "", "", fmt.Errorf("task left repository in detached HEAD state")
	}

	status, err := s.runCommandStrict(ctx, dir, "git", "status", "--porcelain")
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(status) != "" {
		if _, err := s.runCommandStrict(ctx, dir, "git", "add", "-A"); err != nil {
			return "", "", err
		}
		if _, err := s.runCommandStrict(ctx, dir, "git", "commit", "-m", s.cfg.AutoCommitMessage); err != nil {
			return "", "", err
		}
	}

	ahead, err := s.runCommandStrict(ctx, dir, "git", "rev-list", "--count", fmt.Sprintf("origin/%s..HEAD", baseBranch))
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(ahead) == "0" {
		return "", "", fmt.Errorf("task completed without repository changes to push")
	}
	if _, err := s.runCommandStrict(ctx, dir, "git", "push", "-u", "origin", branch); err != nil {
		return "", "", err
	}
	sha, err := s.runCommandStrict(ctx, dir, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", "", err
	}
	return branch, strings.TrimSpace(sha), nil
}

func (s *Service) runCommandStrict(ctx context.Context, dir, bin string, args ...string) (string, error) {
	result, err := s.runCommand(ctx, dir, bin, args...)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = strings.TrimSpace(result.Stdout)
		}
		if message == "" {
			message = fmt.Sprintf("%s %s exited with code %d", bin, strings.Join(args, " "), result.ExitCode)
		}
		return "", fmt.Errorf("%s", message)
	}
	return result.Stdout, nil
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func (s *Service) waitForPort(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	address := net.JoinHostPort(s.cfg.OpenCodeHost, s.cfg.OpenCodePort)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("opencode server did not become ready on %s", address)
}

func (s *Service) buildInstructions(selected []string) (string, error) {
	files := selected
	if len(files) == 0 {
		var err error
		files, err = s.listInstructionFiles()
		if err != nil {
			return "", err
		}
	}
	if len(files) == 0 {
		return "", nil
	}

	parts := make([]string, 0, len(files))
	for _, file := range files {
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(s.cfg.InstructionsDir, file)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		parts = append(parts, fmt.Sprintf("## %s\n%s", filepath.Base(path), strings.TrimSpace(string(content))))
	}
	return strings.Join(parts, "\n\n"), nil
}

func (s *Service) listInstructionFiles() ([]string, error) {
	entries, err := os.ReadDir(s.cfg.InstructionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	return files, nil
}
