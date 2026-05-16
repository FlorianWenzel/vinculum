package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florian/vinculum/apps/vinculum-agent/internal/config"
	"github.com/florian/vinculum/apps/vinculum-agent/internal/tasks"
)

type testLogger struct{ t *testing.T }

func (l *testLogger) Printf(format string, v ...any) { l.t.Logf(format, v...) }

// setupRepoForExecutor creates a bare "remote" and a working clone, sets
// the env the Executor expects (REPO_PATH, REPO_BRANCH, GIT_AUTHOR_*), and
// returns a cleanup func.
func setupRepoForExecutor(t *testing.T) (clonePath string, restore func()) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	root := t.TempDir()
	bare := filepath.Join(root, "bare.git")
	clone := filepath.Join(root, "clone")

	run := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"HOME="+root,
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.test",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.test",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
		}
	}
	run(root, "init", "--bare", "--initial-branch=main", bare)
	run(root, "clone", bare, clone)
	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(clone, "add", "README.md")
	run(clone, "commit", "-m", "initial")
	run(clone, "push", "origin", "main")

	// Set the env Executor.gitPreCrush / gitPostCrush expect to find.
	saved := map[string]string{}
	set := func(k, v string) {
		saved[k] = os.Getenv(k)
		t.Setenv(k, v)
	}
	set("REPO_PATH", clone)
	set("REPO_BRANCH", "main")
	set("HOME", root)
	set("GIT_AUTHOR_NAME", "Test")
	set("GIT_AUTHOR_EMAIL", "test@example.test")
	set("GIT_COMMITTER_NAME", "Test")
	set("GIT_COMMITTER_EMAIL", "test@example.test")
	restore = func() {
		// t.Setenv handles automatic restore — no-op for symmetry.
	}
	return clone, restore
}

func newState(name, head string) *tasks.State {
	return &tasks.State{
		Payload: tasks.DispatchPayload{
			Name: name,
			Spec: tasks.TaskSpec{
				AgentRef: "tester",
				Prompt:   "noop",
				Git:      &tasks.TaskGit{HeadBranch: head, BaseBranch: "main"},
			},
		},
	}
}

func TestGitPreCrush_CheckoutsHeadFromBase(t *testing.T) {
	clone, restore := setupRepoForExecutor(t)
	defer restore()
	exe := NewExecutor(config.Config{}, &testLogger{t})

	if err := exe.gitPreCrush(context.Background(), clone, &tasks.TaskGit{HeadBranch: "feat/x", BaseBranch: "main"}, "task-1"); err != nil {
		t.Fatal(err)
	}
	// Verify the branch is checked out.
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = clone
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "feat/x" {
		t.Errorf("HEAD = %q, want feat/x", strings.TrimSpace(string(out)))
	}
}

func TestGitPreCrush_DefaultHeadFromTaskName(t *testing.T) {
	clone, restore := setupRepoForExecutor(t)
	defer restore()
	exe := NewExecutor(config.Config{}, &testLogger{t})

	if err := exe.gitPreCrush(context.Background(), clone, &tasks.TaskGit{BaseBranch: "main"}, "haiku-123"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = clone
	out, _ := cmd.Output()
	if got := strings.TrimSpace(string(out)); got != "vinculum/task-haiku-123" {
		t.Errorf("HEAD = %q, want vinculum/task-haiku-123", got)
	}
}

func TestGitPreCrush_RequiresBaseBranch(t *testing.T) {
	clone, restore := setupRepoForExecutor(t)
	defer restore()
	// Unset REPO_BRANCH so the fallback is also absent.
	t.Setenv("REPO_BRANCH", "")
	exe := NewExecutor(config.Config{}, &testLogger{t})

	err := exe.gitPreCrush(context.Background(), clone, &tasks.TaskGit{HeadBranch: "feat/x"}, "t")
	if err == nil || !strings.Contains(err.Error(), "baseBranch") {
		t.Errorf("want baseBranch-required error, got %v", err)
	}
}

func TestGitPostCrush_NoChangesShortCircuits(t *testing.T) {
	clone, restore := setupRepoForExecutor(t)
	defer restore()
	exe := NewExecutor(config.Config{}, &testLogger{t})

	// Pre: create the head branch.
	if err := exe.gitPreCrush(context.Background(), clone, &tasks.TaskGit{HeadBranch: "feat/empty", BaseBranch: "main"}, "t"); err != nil {
		t.Fatal(err)
	}
	state := newState("t", "feat/empty")
	if err := exe.gitPostCrush(context.Background(), state, state.Payload.Spec.Git); err != nil {
		t.Fatalf("post-crush: %v", err)
	}
	if state.Reason != "NoChanges" {
		t.Errorf("reason = %q, want NoChanges", state.Reason)
	}
	if len(state.Artifacts) != 0 {
		t.Errorf("artifacts should be empty, got %v", state.Artifacts)
	}
}

func TestGitPostCrush_CommitsPushesWhenDirty(t *testing.T) {
	clone, restore := setupRepoForExecutor(t)
	defer restore()
	exe := NewExecutor(config.Config{}, &testLogger{t})

	spec := &tasks.TaskGit{HeadBranch: "feat/edit", BaseBranch: "main", CommitMessage: "test: a change"}
	if err := exe.gitPreCrush(context.Background(), clone, spec, "t"); err != nil {
		t.Fatal(err)
	}
	// Simulate crush touching a file.
	if err := os.WriteFile(filepath.Join(clone, "new.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := &tasks.State{Payload: tasks.DispatchPayload{Name: "t", Spec: tasks.TaskSpec{AgentRef: "tester", Git: spec}}, StdoutTail: "crush did stuff"}
	// SkipPR=true so we don't hit the GitHub API in this test.
	spec.SkipPR = true
	if err := exe.gitPostCrush(context.Background(), state, spec); err != nil {
		t.Fatalf("post-crush: %v", err)
	}
	if state.Reason == "NoChanges" {
		t.Errorf("should have committed, but state.Reason=NoChanges")
	}
	// Verify the branch landed on the remote (the bare repo we pushed to).
	cmd := exec.Command("git", "ls-remote", "--heads", "origin", "feat/edit")
	cmd.Dir = clone
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "refs/heads/feat/edit") {
		t.Errorf("push didn't land: %q", string(out))
	}
	// Verify commit message is what we asked for.
	cmd = exec.Command("git", "log", "-1", "--pretty=%s")
	cmd.Dir = clone
	out, _ = cmd.Output()
	if got := strings.TrimSpace(string(out)); got != "test: a change" {
		t.Errorf("commit subject = %q, want %q", got, "test: a change")
	}
}

func TestGitPostCrush_AGENTSMDExcluded(t *testing.T) {
	clone, restore := setupRepoForExecutor(t)
	defer restore()
	// Point InstructionFile at a real file inside the clone so
	// gitPreCrush adds its basename to .git/info/exclude.
	instr := filepath.Join(clone, "AGENTS.md")
	if err := os.WriteFile(instr, []byte("hi crush\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := NewExecutor(config.Config{InstructionFile: instr}, &testLogger{t})

	spec := &tasks.TaskGit{HeadBranch: "feat/exclude", BaseBranch: "main", SkipPR: true}
	if err := exe.gitPreCrush(context.Background(), clone, spec, "t"); err != nil {
		t.Fatal(err)
	}
	// Drop a real file plus the (already-present) AGENTS.md untracked file.
	if err := os.WriteFile(filepath.Join(clone, "real.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := &tasks.State{Payload: tasks.DispatchPayload{Name: "t", Spec: tasks.TaskSpec{AgentRef: "tester", Git: spec}}}
	if err := exe.gitPostCrush(context.Background(), state, spec); err != nil {
		t.Fatalf("post-crush: %v", err)
	}
	// Inspect the commit — must NOT contain AGENTS.md.
	cmd := exec.Command("git", "show", "--name-only", "--pretty=", "HEAD")
	cmd.Dir = clone
	out, _ := cmd.Output()
	files := strings.Fields(string(out))
	for _, f := range files {
		if f == "AGENTS.md" {
			t.Errorf("AGENTS.md leaked into commit: %v", files)
		}
	}
	if len(files) != 1 || files[0] != "real.txt" {
		t.Errorf("expected just real.txt, got %v", files)
	}
}