// Package run composes local evidence; it never publishes or merges commits.
package run

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/smeltery/bellows/internal/changed"
	v "github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/verification/receipt"
	"github.com/smeltery/bellows/internal/verification/worktree"
)

type Plan struct {
	Identity v.Identity
	Targets  []changed.Target
	Executor v.Executor
	Argv     map[string][]string
	Env      []string
	Close    func() error
}
type Options struct {
	Worktree       worktree.Options
	Artifacts      string
	Build          func(context.Context, *worktree.Run) (Plan, error)
	Start          func(context.Context, v.Receipt) error
	Stdout, Stderr io.Writer
}
type Outcome struct {
	Receipt v.Receipt `json:"receipt"`
	Path    string    `json:"path"`
	State   v.State   `json:"state"`
}

func Execute(ctx context.Context, opts Options) (out Outcome, err error) {
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	out.Receipt = v.Receipt{Version: v.ReceiptVersion, RunID: rand.Text(), StartedAt: time.Now().UTC(), Input: v.Invalid, Checks: []v.Check{}, Results: []v.Result{}, Identity: v.Identity{Repository: opts.Worktree.Repository, Mode: opts.Worktree.Mode}}
	// Persist after cleanup so cleanup failures cannot leave a successful receipt.
	defer func() {
		seen := map[string]bool{}
		for _, result := range out.Receipt.Results {
			seen[result.ID] = true
		}
		for _, check := range out.Receipt.Checks {
			if !seen[check.ID] {
				out.Receipt.Results = append(out.Receipt.Results, v.Result{ID: check.ID, State: v.Skipped})
			}
		}
		finished := time.Now().UTC()
		out.Receipt.CompletedAt = &finished
		if validationErr := receipt.Validate(out.Receipt); validationErr != nil {
			err = errors.Join(err, validationErr)
			out.Receipt.Input = v.Invalid
			out.Receipt.Checks, out.Receipt.Results = []v.Check{}, []v.Result{}
			out.Receipt.Identity = v.Identity{}
		}
		out.State = v.Evaluate(out.Receipt.Input, out.Receipt.Checks, out.Receipt.Results)
		path, saveErr := receipt.Save(opts.Worktree.Root, opts.Artifacts, out.Receipt)
		out.Path = path
		if saveErr != nil {
			out.State = v.Error
		}
		err = errors.Join(err, saveErr)
	}()
	prepared, err := worktree.Prepare(ctx, opts.Worktree)
	if err != nil {
		var failure *worktree.PrepareError
		if errors.As(err, &failure) {
			out.Receipt.Input = failure.State
		}
		return out, err
	}
	defer func() {
		if cleanupErr := prepared.Close(); cleanupErr != nil {
			out.Receipt.Input = v.Invalid
			err = errors.Join(err, cleanupErr)
		}
	}()
	out.Receipt.Identity = prepared.Identity
	if opts.Build == nil {
		return out, fmt.Errorf("verification planner is required")
	}
	expected, checkout := prepared.Identity, prepared.Path
	plan, err := opts.Build(ctx, prepared)
	if plan.Close != nil {
		defer func() {
			if cleanupErr := plan.Close(); cleanupErr != nil {
				out.Receipt.Input = v.Invalid
				err = errors.Join(err, cleanupErr)
			}
		}()
	}
	if err != nil {
		return out, err
	}
	if prepared.Identity != expected || prepared.Path != checkout || plan.Identity.Repository != expected.Repository || plan.Identity.Head != expected.Head || plan.Identity.Base != expected.Base || plan.Identity.Tree != expected.Tree || plan.Identity.Mode != expected.Mode {
		return out, fmt.Errorf("planner changed pinned input identity")
	}
	out.Receipt.Identity = plan.Identity
	out.Receipt.Input = v.Ready
	if plan.Executor == nil {
		return out, fmt.Errorf("verification executor is required")
	}
	for _, target := range plan.Targets {
		out.Receipt.Checks = append(out.Receipt.Checks, target.Check)
	}
	if err := receipt.Validate(out.Receipt); err != nil {
		return out, err
	}
	if err := pristine(ctx, checkout, expected.Tree); err != nil {
		out.Receipt.Input = v.Dirty
		return out, err
	}
	if opts.Start != nil {
		data, _ := receipt.Encode(out.Receipt)
		initial, copyErr := receipt.Decode(data)
		if copyErr != nil {
			return out, copyErr
		}
		if err := opts.Start(ctx, initial); err != nil {
			return out, err
		}
	}
	results := map[string]v.State{}
	var executionErr error
	for _, target := range plan.Targets {
		result := v.Result{ID: target.Check.ID, State: v.Skipped}
		ready := !target.Skipped
		for _, need := range target.Check.Needs {
			ready = ready && results[need] == v.Passed
		}
		if ctx.Err() != nil {
			result.State = v.Cancelled
		} else if ready {
			result, err = plan.Executor.Run(ctx, v.Request{RunID: out.Receipt.RunID, Worktree: checkout, Identity: plan.Identity, Check: target.Check, Argv: plan.Argv[target.Check.ID], Workflow: target.Workflow, Job: target.Job, Matrix: target.Matrix, Env: plan.Env, Stdout: opts.Stdout, Stderr: opts.Stderr})
			if err != nil && result.State != v.Cancelled && result.State != v.Unsupported {
				result = v.Result{ID: target.Check.ID, State: v.Error}
			}
			if result.ID != target.Check.ID {
				result = v.Result{ID: target.Check.ID, State: v.Error}
				err = fmt.Errorf("executor returned a different check identity")
			}
		}
		out.Receipt.Results = append(out.Receipt.Results, result)
		results[target.Check.ID] = result.State
		if err != nil {
			executionErr = errors.Join(executionErr, fmt.Errorf("check %q: %w", target.Check.ID, err))
			err = nil
		}
	}
	inspectionCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pristine(inspectionCtx, checkout, expected.Tree); err != nil {
		out.Receipt.Input = v.Dirty
		return out, err
	}
	current, err := changed.Discover(opts.Worktree.Root, changed.Options{Scope: changed.BaseHead, Base: opts.Worktree.Base, Head: opts.Worktree.Head})
	if err != nil {
		out.Receipt.Input = v.Invalid
		return out, errors.Join(executionErr, err)
	}
	if current.Head != expected.Head || current.Base != expected.Base {
		out.Receipt.Input = v.Stale
		return out, fmt.Errorf("source head or base moved during verification")
	}
	return out, executionErr
}

func pristine(ctx context.Context, root, tree string) error {
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root, "-c", "core.fsmonitor=false"}, args...)...)
		for _, name := range []string{"PATH", "HOME", "TMPDIR"} {
			if value, ok := os.LookupEnv(name); ok {
				cmd.Env = append(cmd.Env, name+"="+value)
			}
		}
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	actual, err := git("rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	if actual != tree {
		return fmt.Errorf("prepared worktree tree changed")
	}
	status, err := git("status", "--porcelain", "-z", "--untracked-files=no", "--ignore-submodules=none")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("prepared tracked files changed")
	}
	return nil
}
