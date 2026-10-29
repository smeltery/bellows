package run

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/smeltery/bellows/internal/changed"
	v "github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/verification/receipt"
	"github.com/smeltery/bellows/internal/verification/worktree"
)

type fakeExecutor func(context.Context, v.Request) (v.Result, error)

func (f fakeExecutor) Run(ctx context.Context, r v.Request) (v.Result, error) { return f(ctx, r) }
func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git: %s %v", data, err)
	}
	return string(data)
}
func fixture(t *testing.T) (Options, string) {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	write := func(value string) {
		if err := os.WriteFile(filepath.Join(root, "file"), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		git(t, root, "add", ".")
		git(t, root, "commit", "-m", "fixture")
	}
	write("base")
	git(t, root, "checkout", "-b", "feature")
	write("feature")
	artifacts := t.TempDir()
	if err := os.Chmod(artifacts, 0700); err != nil {
		t.Fatal(err)
	}
	opts := Options{Worktree: worktree.Options{Root: root, Repository: "example/repo", Base: "main", Head: "feature", Mode: v.ModeMerge}, Artifacts: artifacts}
	opts.Build = func(_ context.Context, r *worktree.Run) (Plan, error) {
		identity := r.Identity
		identity.Executor = v.ExecutorAct
		identity.Platform = "linux/amd64"
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte("fixture")))
		identity.Fingerprints = v.Fingerprints{Workflow: digest, Config: digest, Policy: digest, Dependencies: digest, Toolchain: digest, Environment: digest}
		return Plan{Identity: identity, Targets: []changed.Target{{Check: v.Check{ID: "test", Required: true, Needs: []string{}}}}, Executor: fakeExecutor(func(_ context.Context, req v.Request) (v.Result, error) {
			content, err := os.ReadFile(filepath.Join(req.Worktree, "file"))
			if err != nil || string(content) != "feature" || req.Worktree == root {
				t.Fatalf("wrong checkout %s %q %v", req.Worktree, content, err)
			}
			zero := 0
			return v.Result{ID: req.Check.ID, State: v.Passed, ExitCode: &zero}, nil
		})}, nil
	}
	return opts, root
}
func TestExecutePersistsAndCleans(t *testing.T) {
	opts, root := fixture(t)
	before := git(t, root, "worktree", "list", "--porcelain")
	out, err := Execute(context.Background(), opts)
	if err != nil || out.State != v.Passed {
		t.Fatalf("%+v %v", out, err)
	}
	data, err := os.ReadFile(out.Path)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := receipt.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if err = receipt.Match(saved, out.Receipt.Identity); err != nil {
		t.Fatal(err)
	}
	if after := git(t, root, "worktree", "list", "--porcelain"); after != before {
		t.Fatal("worktree leaked")
	}
}
func TestFailuresNeverPass(t *testing.T) {
	for _, kind := range []string{"failed", "cancelled", "unsupported", "missing", "skipped", "stale", "cleanup", "dirty", "invalid result", "planner mutation", "storage"} {
		t.Run(kind, func(t *testing.T) {
			opts, root := fixture(t)
			build := opts.Build
			opts.Build = func(ctx context.Context, w *worktree.Run) (Plan, error) {
				p, err := build(ctx, w)
				p.Executor = fakeExecutor(func(_ context.Context, r v.Request) (v.Result, error) {
					code := 1
					switch kind {
					case "failed":
						return v.Result{ID: r.Check.ID, State: v.Failed, ExitCode: &code}, nil
					case "cancelled":
						return v.Result{ID: r.Check.ID, State: v.Cancelled}, nil
					case "unsupported":
						return v.Result{ID: r.Check.ID, State: v.Unsupported}, nil
					case "missing":
						return v.Result{}, nil
					case "dirty":
						if err := os.WriteFile(filepath.Join(r.Worktree, "file"), []byte("mutated"), 0600); err != nil {
							t.Fatal(err)
						}
					case "invalid result":
						return v.Result{ID: r.Check.ID, State: v.Passed}, nil
					case "stale":
						git(t, root, "update-ref", "refs/heads/feature", "main")
					}
					code = 0
					return v.Result{ID: r.Check.ID, State: v.Passed, ExitCode: &code}, nil
				})
				if kind == "planner mutation" {
					w.Identity.Head = "mutated"
					p.Identity = w.Identity
				}
				if kind == "skipped" {
					p.Targets[0].Skipped = true
				}
				if kind == "cleanup" {
					p.Close = func() error { return fmt.Errorf("cleanup failed") }
				}
				return p, err
			}
			if kind == "storage" {
				opts.Artifacts = filepath.Join(root, "missing")
			}
			out, _ := Execute(context.Background(), opts)
			if out.State == v.Passed {
				t.Fatalf("false pass: %+v", out)
			}
			if out.Path == "" && kind != "storage" {
				t.Fatal("missing failure receipt")
			}
		})
	}
}
func TestConflictReceipt(t *testing.T) {
	opts, root := fixture(t)
	git(t, root, "checkout", "main")
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("different"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "conflict")
	out, err := Execute(context.Background(), opts)
	if err == nil || out.Receipt.Input != v.Conflict || out.Path == "" {
		t.Fatalf("%+v %v", out, err)
	}
}
