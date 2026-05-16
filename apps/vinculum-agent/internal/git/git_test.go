package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupRepo creates a bare "remote" and a working clone of it, returns the
// clone path. All git ops are isolated via HOME so global config can't
// interfere with the test.
func setupRepo(t *testing.T) (clonePath string, gitEnv []string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	root := t.TempDir()
	bare := filepath.Join(root, "bare.git")
	clone := filepath.Join(root, "clone")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}

	// Isolate git config + identity so tests don't depend on the host user's
	// .gitconfig (commit author, etc.).
	gitEnv = []string{
		"HOME=" + home,
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.test",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.test",
		"GIT_TERMINAL_PROMPT=0",
	}

	run := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), gitEnv...)
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
	return clone, gitEnv
}

func TestRepoLifecycle(t *testing.T) {
	clone, env := setupRepo(t)
	r := Open(clone).WithEnv(env...)
	ctx := context.Background()

	if err := r.Verify(ctx); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// New branch off main.
	if err := r.CheckoutNewBranch(ctx, "feat/x", "main"); err != nil {
		t.Fatalf("checkout: %v", err)
	}

	// Initially clean.
	dirty, err := r.HasChanges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Error("expected clean working tree after checkout")
	}

	// Write a file → dirty.
	if err := os.WriteFile(filepath.Join(clone, "feat.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err = r.HasChanges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Error("expected dirty after write")
	}

	if err := r.AddAll(ctx); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := r.Commit(ctx, "feat: add x", false); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Post-commit, clean again.
	dirty, _ = r.HasChanges(ctx)
	if dirty {
		t.Error("expected clean working tree after commit")
	}

	sha, err := r.HeadSHA(ctx)
	if err != nil || sha == "" {
		t.Fatalf("HeadSHA=%q err=%v", sha, err)
	}

	if err := r.Push(ctx, "feat/x", true, false); err != nil {
		t.Fatalf("push: %v", err)
	}

	// Verify the branch landed on the remote.
	cmd := exec.Command("git", "ls-remote", "--heads", "origin", "feat/x")
	cmd.Dir = clone
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ls-remote: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "refs/heads/feat/x") {
		t.Errorf("push didn't land: %q", string(out))
	}
}

func TestPush_ForceWithLease_OverwritesDivergedRemote(t *testing.T) {
	clone, env := setupRepo(t)
	r := Open(clone).WithEnv(env...)
	ctx := context.Background()

	// First push: a normal commit on a new branch.
	if err := r.CheckoutNewBranch(ctx, "feat/y", "main"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clone, "a.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.AddAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.Commit(ctx, "v1", false); err != nil {
		t.Fatal(err)
	}
	if err := r.Push(ctx, "feat/y", true, false); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// Simulate the runtime's "re-run from base" pattern: throw away local
	// state, re-checkout from base, make a different commit.
	if err := r.CheckoutNewBranch(ctx, "feat/y", "main"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clone, "a.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.AddAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.Commit(ctx, "v2", false); err != nil {
		t.Fatal(err)
	}

	// Plain push should be rejected (non-fast-forward).
	if err := r.Push(ctx, "feat/y", false, false); err == nil {
		t.Error("plain push to diverged remote should be rejected")
	}
	// force-with-lease should succeed.
	if err := r.Push(ctx, "feat/y", false, true); err != nil {
		t.Errorf("force-with-lease push: %v", err)
	}
}

func TestCheckoutNewBranch_FallbackWhenNoOriginRef(t *testing.T) {
	clone, env := setupRepo(t)
	r := Open(clone).WithEnv(env...)
	ctx := context.Background()
	// "feat/y" doesn't exist on origin, but `main` is available locally —
	// the fall-through path uses the local ref.
	if err := r.CheckoutNewBranch(ctx, "feat/y", "main"); err != nil {
		t.Fatalf("checkout: %v", err)
	}
}

func TestHasChanges_UntrackedFile(t *testing.T) {
	clone, env := setupRepo(t)
	r := Open(clone).WithEnv(env...)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(clone, "untracked"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err := r.HasChanges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Error("untracked file should count as a change")
	}
}

func TestCommit_RequiresMessage(t *testing.T) {
	clone, env := setupRepo(t)
	r := Open(clone).WithEnv(env...)
	if err := r.Commit(context.Background(), "  ", false); err == nil {
		t.Error("expected error on empty message")
	}
}

func TestRemoteURL(t *testing.T) {
	clone, env := setupRepo(t)
	r := Open(clone).WithEnv(env...)
	url, err := r.RemoteURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(url, "bare.git") {
		t.Errorf("remote url = %q, want suffix bare.git", url)
	}
}

func TestVerify_NotARepo(t *testing.T) {
	dir := t.TempDir()
	r := Open(dir).WithEnv("HOME=" + dir)
	if err := r.Verify(context.Background()); err == nil {
		t.Error("expected verify to fail on non-repo dir")
	}
}
