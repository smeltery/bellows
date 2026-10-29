package receipt

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	v "github.com/smeltery/bellows/internal/verification"
)

var (
	ErrStale        = errors.New("receipt identity is stale")
	ErrIncomplete   = errors.New("receipt is incomplete")
	ErrUnsuccessful = errors.New("receipt does not report successful ready inputs")
	objectID        = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	digest          = regexp.MustCompile(`^[0-9a-f]{64}$`)
	repository      = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

func Validate(r v.Receipt) error {
	id, f := r.Identity, r.Identity.Fingerprints
	required := func(value string) bool { return r.Input == v.Ready || value != "" }
	if r.Version != v.ReceiptVersion || r.RunID == "" ||
		(required(id.Repository) && !repository.MatchString(id.Repository)) ||
		(r.Input == v.Ready && id.Platform == "") ||
		(required(id.Mode) && id.Mode != v.ModeHead && id.Mode != v.ModeMerge) ||
		(required(id.Executor) && id.Executor != v.ExecutorAct && id.Executor != v.ExecutorFlox) {
		return fmt.Errorf("invalid receipt version or identity")
	}
	for _, value := range []string{id.Head, id.Base, id.Tree} {
		if !objectID.MatchString(value) && required(value) {
			return fmt.Errorf("invalid Git object identity")
		}
	}
	for _, value := range []string{f.Workflow, f.Config, f.Policy, f.Dependencies, f.Toolchain, f.Environment} {
		if !digest.MatchString(value) && required(value) {
			return fmt.Errorf("invalid input fingerprint")
		}
	}
	switch r.Input {
	case v.Ready, v.Dirty, v.Conflict, v.Stale, v.Invalid:
	default:
		return fmt.Errorf("invalid input state")
	}
	if r.StartedAt.IsZero() || r.StartedAt.Location() != time.UTC || r.Checks == nil || r.Results == nil {
		return fmt.Errorf("invalid start time or null check/result arrays")
	}
	if r.CompletedAt != nil && (r.CompletedAt.Location() != time.UTC || r.CompletedAt.Before(r.StartedAt)) {
		return fmt.Errorf("invalid completion time")
	}
	checks := make(map[string]v.Check, len(r.Checks))
	for _, check := range r.Checks {
		if _, exists := checks[check.ID]; exists || check.ID == "" || check.Needs == nil {
			return fmt.Errorf("invalid or duplicate check")
		}
		checks[check.ID] = check
	}
	visited := map[string]uint8{}
	var visit func(string) error
	visit = func(id string) error {
		check, exists := checks[id]
		if !exists || visited[id] == 1 {
			return fmt.Errorf("unknown or cyclic prerequisite")
		}
		if visited[id] == 2 {
			return nil
		}
		visited[id] = 1
		seen := map[string]bool{}
		for _, dependency := range check.Needs {
			if seen[dependency] {
				return fmt.Errorf("duplicate prerequisite")
			}
			seen[dependency] = true
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visited[id] = 2
		return nil
	}
	for id := range checks {
		if err := visit(id); err != nil {
			return err
		}
	}
	results := make(map[string]v.Result, len(r.Results))
	for _, result := range r.Results {
		_, planned := checks[result.ID]
		_, duplicate := results[result.ID]
		if !planned || duplicate || result.DurationMS < 0 || (result.ExitCode != nil && *result.ExitCode < 0) {
			return fmt.Errorf("invalid or duplicate result")
		}
		switch result.State {
		case v.Passed:
			if result.ExitCode == nil || *result.ExitCode != 0 {
				return fmt.Errorf("passed result requires exit code zero")
			}
		case v.Failed:
			if result.ExitCode == nil || *result.ExitCode == 0 {
				return fmt.Errorf("failed result requires nonzero exit code")
			}
		case v.Pending, v.Skipped, v.Unsupported:
			if result.ExitCode != nil {
				return fmt.Errorf("unexecuted result cannot have an exit code")
			}
		case v.Cancelled, v.Error:
		default:
			return fmt.Errorf("invalid result state")
		}
		if r.CompletedAt != nil && (result.State == v.Pending || result.DurationMS > r.CompletedAt.Sub(r.StartedAt).Milliseconds()) {
			return fmt.Errorf("result contradicts receipt completion")
		}
		results[result.ID] = result
	}
	if r.CompletedAt != nil && len(results) != len(checks) {
		return fmt.Errorf("completed receipt is missing results")
	}
	for _, result := range results {
		if result.State == v.Passed {
			for _, dependency := range checks[result.ID].Needs {
				if results[dependency].State != v.Passed {
					return fmt.Errorf("passed result has an unsuccessful prerequisite")
				}
			}
		}
	}
	return nil
}

// Match checks freshness and successful completion, never trusted provenance.
func Match(r v.Receipt, current v.Identity) error {
	if err := Validate(r); err != nil {
		return err
	}
	if r.Identity != current || r.Input == v.Stale {
		return ErrStale
	}
	if r.CompletedAt == nil {
		return ErrIncomplete
	}
	if r.Input != v.Ready || v.Evaluate(r.Input, r.Checks, r.Results) != v.Passed {
		return ErrUnsuccessful
	}
	return nil
}
