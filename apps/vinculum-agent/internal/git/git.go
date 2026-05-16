// Package git is a thin exec.Cmd wrapper around the `git` CLI. We use the
// CLI rather than go-git so the same authentication path (GIT_SSH_COMMAND,
// GIT_ASKPASS, GIT_CONFIG_*) that the init container relies on continues
// to work uniformly at task time.
package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// indirected for clarity; not for swapping in tests.
var (
	osReadFile     = os.ReadFile
	osOpenAppend   = func(p string) (*os.File, error) { return os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) }
	filepathIsAbs  = filepath.IsAbs
)

// Repo represents a working tree on disk. All methods invoke `git` against
// the repo's directory.
type Repo struct {
	dir string
	// extraEnv is appended to os.Environ() for every git invocation. Tests
	// set this to override HOME so `git config --global` is sandboxed; in
	// production it's empty and the pod's env is used as-is.
	extraEnv []string
}

// Open returns a handle to an existing working tree. It does NOT validate
// that the directory actually contains a .git folder — call Verify if you
// need that.
func Open(dir string) *Repo { return &Repo{dir: dir} }

// WithEnv returns a copy of r with extra env vars appended to every git
// command. Useful for tests; in production prefer setting env on the
// containing process so init + main containers see the same values.
func (r *Repo) WithEnv(env ...string) *Repo {
	out := *r
	out.extraEnv = append(append([]string(nil), r.extraEnv...), env...)
	return &out
}

// Verify confirms the directory is inside a git work tree.
func (r *Repo) Verify(ctx context.Context) error {
	_, err := r.run(ctx, "rev-parse", "--is-inside-work-tree")
	return err
}

// Fetch updates origin's refs. Equivalent to `git fetch origin`.
func (r *Repo) Fetch(ctx context.Context) error {
	_, err := r.run(ctx, "fetch", "origin", "--prune")
	return err
}

// CheckoutNewBranch creates head from origin/base (or local base) and
// switches to it. Equivalent to `git checkout -B <head> origin/<base>` if
// origin/<base> exists, else `git checkout -B <head> <base>`.
func (r *Repo) CheckoutNewBranch(ctx context.Context, head, base string) error {
	if head == "" {
		return errors.New("head branch is required")
	}
	if base == "" {
		return errors.New("base branch is required")
	}
	if _, err := r.run(ctx, "rev-parse", "--verify", "origin/"+base); err == nil {
		_, err := r.run(ctx, "checkout", "-B", head, "origin/"+base)
		return err
	}
	_, err := r.run(ctx, "checkout", "-B", head, base)
	return err
}

// HasChanges returns true if the working tree has uncommitted modifications
// (tracked or untracked). Useful as the short-circuit check before commit.
func (r *Repo) HasChanges(ctx context.Context) (bool, error) {
	out, err := r.run(ctx, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// AddAll stages every change (new + modified + deleted).
func (r *Repo) AddAll(ctx context.Context) error {
	_, err := r.run(ctx, "add", "-A")
	return err
}

// Commit creates a single commit with the given message. The caller is
// responsible for staging changes first (use AddAll). Author/committer come
// from GIT_AUTHOR_* / GIT_COMMITTER_* env vars when set; otherwise from
// git config. Pass `allowEmpty=true` to permit an empty commit (rarely
// useful — prefer HasChanges instead).
func (r *Repo) Commit(ctx context.Context, message string, allowEmpty bool) error {
	if strings.TrimSpace(message) == "" {
		return errors.New("commit message is required")
	}
	args := []string{"commit", "-m", message}
	if allowEmpty {
		args = append(args, "--allow-empty")
	}
	_, err := r.run(ctx, args...)
	return err
}

// Push sends the named branch upstream to origin. SetUpstream=true mirrors
// the `--set-upstream` flag (needed on first push of a branch).
func (r *Repo) Push(ctx context.Context, branch string, setUpstream bool) error {
	if branch == "" {
		return errors.New("branch is required")
	}
	args := []string{"push"}
	if setUpstream {
		args = append(args, "--set-upstream")
	}
	args = append(args, "origin", branch)
	_, err := r.run(ctx, args...)
	return err
}

// HeadSHA returns the abbreviated SHA of the current HEAD. Used to surface
// "what got pushed" in Task.status.
func (r *Repo) HeadSHA(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// RemoteURL returns origin's URL. Used to derive owner/repo for PR creation.
func (r *Repo) RemoteURL(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// AppendInfoExclude appends the given paths to .git/info/exclude (the
// per-clone, never-committed ignore list). Idempotent: lines already in
// the file are not duplicated. Used to keep vinculum's own runtime
// droppings (AGENTS.md symlink, scratch files) out of `git status` and
// therefore out of the commit.
func (r *Repo) AppendInfoExclude(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	gitDirOut, err := r.run(ctx, "rev-parse", "--git-dir")
	if err != nil {
		return err
	}
	gitDir := strings.TrimSpace(gitDirOut)
	if !filepathIsAbs(gitDir) {
		gitDir = r.dir + string(os.PathSeparator) + gitDir
	}
	excludePath := gitDir + string(os.PathSeparator) + "info" + string(os.PathSeparator) + "exclude"
	existing, _ := osReadFile(excludePath)
	known := map[string]bool{}
	for _, line := range strings.Split(string(existing), "\n") {
		known[strings.TrimSpace(line)] = true
	}
	var add []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || known[p] {
			continue
		}
		add = append(add, p)
		known[p] = true
	}
	if len(add) == 0 {
		return nil
	}
	f, err := osOpenAppend(excludePath)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		add = append([]string{""}, add...)
	}
	_, err = f.WriteString(strings.Join(add, "\n") + "\n")
	return err
}

// run executes git with the given args inside r.dir and returns stdout as a
// string. On non-zero exit it returns an error whose Error() includes
// stderr so callers can surface useful diagnostics in Task.status.
func (r *Repo) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir
	if len(r.extraEnv) > 0 {
		cmd.Env = append(cmd.Environ(), r.extraEnv...)
	}
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		if stderr != "" {
			return string(out), fmt.Errorf("git %s: %s", strings.Join(args, " "), stderr)
		}
		return string(out), fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}
