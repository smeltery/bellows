package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/smeltery/bellows/internal/verification"
)

type Options struct {
	Root, Repository, Base, Head, Mode, TempDir string
}

type Run struct {
	Path                  string
	Identity              verification.Identity
	root, owned, checkout string
}

type PrepareError struct {
	State verification.InputState
	Err   error
}

func (e *PrepareError) Error() string { return string(e.State) + ": " + e.Err.Error() }
func (e *PrepareError) Unwrap() error { return e.Err }

func Prepare(ctx context.Context, opts Options) (_ *Run, err error) {
	if opts.Mode != verification.ModeMerge && opts.Mode != verification.ModeHead {
		return nil, &PrepareError{verification.Invalid, fmt.Errorf("unsupported mode %q", opts.Mode)}
	}
	if opts.Head == "" {
		opts.Head = "HEAD"
	}
	if opts.Base == "" {
		return nil, &PrepareError{verification.Invalid, errors.New("base ref is required")}
	}
	opts.Root, err = filepath.Abs(opts.Root)
	if err != nil {
		return nil, &PrepareError{verification.Invalid, err}
	}
	shallow, err := git(ctx, opts.Root, "", "rev-parse", "--is-shallow-repository")
	if err != nil {
		return nil, &PrepareError{verification.Invalid, err}
	}
	if shallow != "false" {
		return nil, &PrepareError{verification.Invalid, errors.New("complete history is required; fetch --unshallow")}
	}
	resolve := func(ref string) (string, error) {
		return git(ctx, opts.Root, "", "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	}
	head, err := resolve(opts.Head)
	if err != nil {
		return nil, &PrepareError{verification.Invalid, err}
	}
	base, err := resolve(opts.Base)
	if err != nil {
		return nil, &PrepareError{verification.Invalid, err}
	}
	tree, err := git(ctx, opts.Root, "", "rev-parse", head+"^{tree}")
	if err != nil {
		return nil, &PrepareError{verification.Invalid, err}
	}
	commit := head
	if opts.Mode == verification.ModeMerge {
		tree, err = git(ctx, opts.Root, "", "merge-tree", "--write-tree", "--no-messages", base, head)
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 1 {
				return nil, &PrepareError{verification.Conflict, err}
			}
			return nil, &PrepareError{verification.Invalid, err}
		}
		commit, err = git(ctx, opts.Root, "Bellows verification\n", "commit-tree", tree, "-p", base, "-p", head)
		if err != nil {
			return nil, &PrepareError{verification.Invalid, err}
		}
	}
	if opts.TempDir == "" {
		opts.TempDir = os.TempDir()
	}
	temp, err := filepath.Abs(opts.TempDir)
	if err != nil {
		return nil, &PrepareError{verification.Invalid, err}
	}
	temp, err = filepath.EvalSymlinks(temp)
	if err != nil {
		return nil, &PrepareError{verification.Invalid, err}
	}
	owned, err := os.MkdirTemp(temp, "bellows-verify-")
	if err != nil {
		return nil, &PrepareError{verification.Invalid, err}
	}
	run := &Run{root: opts.Root, owned: owned, checkout: filepath.Join(owned, "checkout")}
	run.Path = run.checkout
	run.Identity = verification.Identity{Repository: opts.Repository, Head: head, Base: base, Tree: tree, Mode: opts.Mode}
	defer func() {
		if err != nil {
			err = errors.Join(err, run.Close())
		}
	}()
	if _, err = git(ctx, opts.Root, "", "worktree", "add", "--detach", "--", run.checkout, commit); err != nil {
		return nil, &PrepareError{verification.Invalid, err}
	}
	for _, ref := range []struct{ name, expected string }{{opts.Head, head}, {opts.Base, base}} {
		actual, resolveErr := resolve(ref.name)
		if resolveErr != nil {
			return nil, &PrepareError{verification.Stale, resolveErr}
		}
		if actual != ref.expected {
			return nil, &PrepareError{verification.Stale, fmt.Errorf("ref %q moved during preparation", ref.name)}
		}
	}
	return run, nil
}

// Close uses private ownership paths even if a caller changes the exported Path.
func (r *Run) Close() error {
	if r.owned == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	list, err := git(ctx, r.root, "", "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return err
	}
	if strings.Contains(list, "worktree "+r.checkout+"\x00") {
		if _, err := git(ctx, r.root, "", "worktree", "remove", "--force", "--", r.checkout); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(r.owned); err != nil {
		return err
	}
	r.owned = ""
	return nil
}

func git(ctx context.Context, root, input string, args ...string) (string, error) {
	options := []string{"-C", root, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "core.sparseCheckout=false", "-c", "index.sparse=false"}
	cmd := exec.CommandContext(ctx, "git", append(options, args...)...)
	for _, name := range []string{"PATH", "HOME", "TMPDIR", "TMP", "TEMP", "LANG", "LC_ALL"} {
		if value, ok := os.LookupEnv(name); ok {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_AUTHOR_NAME=Bellows", "GIT_AUTHOR_EMAIL=bellows@example.invalid", "GIT_COMMITTER_NAME=Bellows", "GIT_COMMITTER_EMAIL=bellows@example.invalid")
	cmd.Stdin = strings.NewReader(input)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			err = errors.Join(ctx.Err(), err)
		}
		var failure *exec.ExitError
		if errors.As(err, &failure) {
			out = append(out, failure.Stderr...)
		}
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
