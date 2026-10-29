package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smeltery/bellows/internal/verification"
)

func fixtureGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "user.name=Test", "-c", "user.email=test@example.invalid"}, args...)...)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func put(t *testing.T, root, name, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, root string) string {
	t.Helper()
	fixtureGit(t, root, "add", ".")
	fixtureGit(t, root, "commit", "-m", "fixture")
	return fixtureGit(t, root, "rev-parse", "HEAD")
}

func fixture(t *testing.T) (Options, string, string) {
	t.Helper()
	root := t.TempDir()
	fixtureGit(t, root, "init", "-b", "main")
	put(t, root, "common", "initial\n")
	commit(t, root)
	fixtureGit(t, root, "checkout", "-b", "feature")
	put(t, root, "feature", "head\n")
	head := commit(t, root)
	fixtureGit(t, root, "checkout", "main")
	put(t, root, "base", "base\n")
	base := commit(t, root)
	fixtureGit(t, root, "checkout", "feature")
	return Options{Root: root, Repository: "example/project", Head: "feature", Base: "main", Mode: verification.ModeMerge, TempDir: t.TempDir()}, head, base
}

func TestPrepareMaterializesFullTreeFromSparseCheckout(t *testing.T) {
	opts, _, _ := fixture(t)
	fixtureGit(t, opts.Root, "sparse-checkout", "set", "--no-cone", "common")
	before := fixtureGit(t, opts.Root, "sparse-checkout", "list")
	run, err := Prepare(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := run.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, name := range []string{"common", "feature", "base"} {
		if _, err := os.Stat(filepath.Join(run.Path, name)); err != nil {
			t.Fatalf("incomplete checkout: %s: %v", name, err)
		}
	}
	if after := fixtureGit(t, opts.Root, "sparse-checkout", "list"); after != before {
		t.Fatal("source sparse settings changed")
	}
	if _, err := os.Stat(filepath.Join(opts.Root, "feature")); !os.IsNotExist(err) {
		t.Fatal("source sparse checkout changed")
	}
}

func TestPrepareMergePreservesDirtyCheckoutAndCleansOnlyOwnedPaths(t *testing.T) {
	opts, head, base := fixture(t)
	put(t, opts.Root, "common", "staged\n")
	fixtureGit(t, opts.Root, "add", "common")
	put(t, opts.Root, "common", "unstaged\n")
	put(t, opts.Root, "untracked", "local\n")
	status := fixtureGit(t, opts.Root, "status", "--porcelain=v1", "-z")
	index := fixtureGit(t, opts.Root, "write-tree")
	run, err := Prepare(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := run.Close(); err != nil {
			t.Error(err)
		}
	})
	if run.Identity.Head != head || run.Identity.Base != base || run.Identity.Tree != fixtureGit(t, run.Path, "rev-parse", "HEAD^{tree}") {
		t.Fatalf("wrong identity: %+v", run.Identity)
	}
	for _, file := range []string{"feature", "base"} {
		if _, err := os.Stat(filepath.Join(run.Path, file)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(run.Path, "untracked")); !os.IsNotExist(err) {
		t.Fatalf("untracked file leaked: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(run.Path, "common")); err != nil || string(data) != "initial\n" {
		t.Fatalf("dirty source leaked: %s, %v", data, err)
	}
	if fixtureGit(t, opts.Root, "status", "--porcelain=v1", "-z") != status || fixtureGit(t, opts.Root, "write-tree") != index || fixtureGit(t, opts.Root, "rev-parse", "HEAD") != head {
		t.Fatal("original checkout changed")
	}
	checkout := run.Path
	foreign := t.TempDir()
	put(t, foreign, "keep", "safe")
	run.Path = foreign
	if err := run.Close(); err != nil {
		t.Fatal(err)
	}
	if err := run.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(checkout); !os.IsNotExist(err) {
		t.Fatalf("checkout retained: %v", err)
	}
	if _, err := os.Stat(filepath.Join(foreign, "keep")); err != nil {
		t.Fatal("foreign path removed")
	}
}

func TestPrepareHeadOnlyAndIdenticalRefs(t *testing.T) {
	opts, head, _ := fixture(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	opts.TempDir, err = filepath.Rel(cwd, opts.TempDir)
	if err != nil {
		t.Fatal(err)
	}
	opts.Mode = verification.ModeHead
	run, err := Prepare(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if run.Identity.Tree != fixtureGit(t, opts.Root, "rev-parse", head+"^{tree}") {
		t.Fatal("wrong head tree")
	}
	if _, err := os.Stat(filepath.Join(run.Path, "base")); !os.IsNotExist(err) {
		t.Fatal("head mode included base changes")
	}
	if err := run.Close(); err != nil {
		t.Fatal(err)
	}
	opts.Mode = verification.ModeMerge
	opts.Base = opts.Head
	run, err = Prepare(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := run.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareConflictAndInvalidInputs(t *testing.T) {
	opts, _, _ := fixture(t)
	put(t, opts.Root, "common", "feature conflict\n")
	commit(t, opts.Root)
	fixtureGit(t, opts.Root, "checkout", "main")
	put(t, opts.Root, "common", "base conflict\n")
	commit(t, opts.Root)
	before := fixtureGit(t, opts.Root, "worktree", "list", "--porcelain")
	run, err := Prepare(context.Background(), opts)
	var preparation *PrepareError
	if run != nil || !errors.As(err, &preparation) || preparation.State != verification.Conflict {
		t.Fatalf("want conflict: %v", err)
	}
	if fixtureGit(t, opts.Root, "worktree", "list", "--porcelain") != before {
		t.Fatal("conflict leaked worktree")
	}
	if entries, err := os.ReadDir(opts.TempDir); err != nil || len(entries) != 0 {
		t.Fatalf("conflict leaked temp files: %v", err)
	}
	for _, invalid := range []Options{{Root: opts.Root, Mode: "unknown"}, {Root: opts.Root, Mode: verification.ModeMerge}, {Root: opts.Root, Base: "missing", Mode: verification.ModeMerge}} {
		if _, err := Prepare(context.Background(), invalid); !errors.As(err, &preparation) || preparation.State != verification.Invalid {
			t.Fatalf("want invalid: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Prepare(ctx, opts); !errors.Is(err, context.Canceled) {
		t.Fatalf("want cancelled: %v", err)
	}
}

func TestConcurrentWorktreesAndInheritedGitEnvironment(t *testing.T) {
	opts, _, _ := fixture(t)
	foreign, _, _ := fixture(t)
	t.Setenv("GIT_DIR", filepath.Join(foreign.Root, ".git"))
	t.Setenv("GIT_WORK_TREE", foreign.Root)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(foreign.Root, ".git", "index"))
	hook := filepath.Join(opts.Root, ".git", "hooks", "post-checkout")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, opts.Root, "config", "commit.gpgsign", "true")
	runs := make([]*Run, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range runs {
		wg.Add(1)
		go func() { defer wg.Done(); runs[i], errs[i] = Prepare(context.Background(), opts) }()
	}
	wg.Wait()
	for i := range runs {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
	}
	if err := runs[0].Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(runs[1].Path); err != nil {
		t.Fatal("cleanup removed another run")
	}
	if err := runs[1].Close(); err != nil {
		t.Fatal(err)
	}
	if runs[0].Path == runs[1].Path {
		t.Fatal("concurrent runs shared paths")
	}
	if entries, err := os.ReadDir(opts.TempDir); err != nil || len(entries) != 0 {
		t.Fatalf("cleanup leaked paths: %v", err)
	}
}

func interceptAdd(t *testing.T, body string) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	script := "#!/bin/sh\ncase \" $* \" in *\" worktree add \"*)\n" + body + "\n;; esac\nexec " + shellQuote(real) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(directory, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func TestPrepareRejectsRefsThatMoveDuringCheckout(t *testing.T) {
	opts, _, base := fixture(t)
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	interceptAdd(t, shellQuote(real)+" -C "+shellQuote(opts.Root)+" update-ref refs/heads/feature "+shellQuote(base))
	run, err := Prepare(context.Background(), opts)
	var preparation *PrepareError
	if run != nil || !errors.As(err, &preparation) || preparation.State != verification.Stale {
		t.Fatalf("want stale ref: %v", err)
	}
	if entries, err := os.ReadDir(opts.TempDir); err != nil || len(entries) != 0 {
		t.Fatalf("stale run leaked paths: %v", err)
	}
	if strings.Contains(fixtureGit(t, opts.Root, "worktree", "list", "--porcelain"), "bellows-verify-") {
		t.Fatal("stale run retained registered worktree")
	}
}

func TestPrepareCancellationCleansOwnedResources(t *testing.T) {
	opts, _, _ := fixture(t)
	started := filepath.Join(t.TempDir(), "started")
	interceptAdd(t, "touch "+shellQuote(started)+"\nsleep 30")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := Prepare(ctx, opts); finished <- err }()
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		select {
		case err := <-finished:
			t.Fatalf("ended before cancellation: %v", err)
		case <-deadline:
			t.Fatal("checkout never started")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled checkout did not stop")
	}
	if entries, err := os.ReadDir(opts.TempDir); err != nil || len(entries) != 0 {
		t.Fatalf("cancelled run leaked paths: %v", err)
	}
}

func TestPreparationDoesNotInheritReportingCredentials(t *testing.T) {
	opts, _, _ := fixture(t)
	t.Setenv("GH_TOKEN", "synthetic-secret")
	t.Setenv("GITHUB_TOKEN", "synthetic-secret")
	interceptAdd(t, "if [ -n \"${GH_TOKEN}${GITHUB_TOKEN}\" ]; then echo credential-leak >&2; exit 99; fi")
	run, err := Prepare(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := run.Close(); err != nil {
		t.Fatal(err)
	}
}
