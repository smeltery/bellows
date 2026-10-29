// Package policy gates advisory publication by trusted orchestration, not by
// cryptographic proof or isolation from hostile code running as the same user.
package policy

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"

	v "github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/verification/receipt"
)

type Options struct {
	OptIn      bool
	Repository string
}
type Decision struct {
	Allowed bool
	Reason  string
}
type Resolver func(context.Context) (v.Identity, error)
type Session struct {
	mu      sync.Mutex
	options Options
	latest  *Run
}

// Run is a process-local capability; only trusted orchestration may begin or
// complete it. There is intentionally no deserializer for this capability.
type Run struct {
	owner             *Session
	initial, terminal []byte
}

func New(options Options) *Session { return &Session{options: options} }

func (s *Session) Begin(r v.Receipt) (*Run, error) {
	data, err := receipt.Encode(r)
	if err != nil {
		return nil, err
	}
	if !s.options.OptIn || s.options.Repository == "" || r.Identity.Repository != s.options.Repository {
		return nil, errors.New("trusted workstation reporting requires explicit repository opt-in")
	}
	if r.CompletedAt != nil || len(r.Results) != 0 {
		return nil, errors.New("begin requires a new incomplete run")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run := &Run{owner: s, initial: data}
	s.latest = run
	return run, nil
}

// Complete seals observed execution results once. It must never be called with
// an imported receipt instead of results from the current trusted orchestrator.
func (r *Run) Complete(completed v.Receipt) error {
	if r == nil || r.owner == nil {
		return errors.New("unknown run")
	}
	data, err := receipt.Encode(completed)
	if err != nil {
		return err
	}
	if completed.CompletedAt == nil {
		return receipt.ErrIncomplete
	}
	s := r.owner
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latest != r || r.terminal != nil {
		return errors.New("run is superseded or already complete")
	}
	initial, err := receipt.Decode(r.initial)
	if err != nil {
		return err
	}
	metadata := completed
	metadata.CompletedAt = nil
	metadata.Results = []v.Result{}
	metadata.Input = initial.Input
	normalized, err := receipt.Encode(metadata)
	if err != nil {
		return err
	}
	if !bytes.Equal(normalized, r.initial) {
		return errors.New("run identity, plan or lifecycle changed")
	}
	r.terminal = data
	return nil
}

// Receipt returns a copy; modifying it cannot change the sealed run.
func (r *Run) Receipt() (v.Receipt, error) {
	if r == nil || r.owner == nil {
		return v.Receipt{}, errors.New("unknown run")
	}
	r.owner.mu.Lock()
	defer r.owner.mu.Unlock()
	data := r.initial
	if r.terminal != nil {
		data = r.terminal
	}
	return receipt.Decode(data)
}

// Authorize rereads current inputs on every call, including transport retries.
// The resolver is trusted application code, never supplied by repository content.
func (s *Session) Authorize(ctx context.Context, run *Run, presented v.Receipt, desired v.State, current Resolver) Decision {
	deny := func(reason string) Decision { return Decision{Reason: reason} }
	if !s.options.OptIn {
		return deny("opt_in_required")
	}
	if run == nil || run.owner != s {
		return deny("unknown_run")
	}
	data, err := receipt.Encode(presented)
	if err != nil {
		return deny("invalid_receipt")
	}
	s.mu.Lock()
	if s.latest != run {
		s.mu.Unlock()
		return deny("superseded_run")
	}
	sealed := run.initial
	completed := run.terminal != nil
	if completed {
		sealed = run.terminal
	}
	matches := bytes.Equal(data, sealed)
	s.mu.Unlock()
	if !matches {
		return deny("edited_receipt")
	}
	if presented.Identity.Repository != s.options.Repository {
		return deny("wrong_repository")
	}
	if current == nil {
		return deny("current_identity_unavailable")
	}
	identity, err := current(ctx)
	if err != nil || ctx.Err() != nil {
		return deny("current_identity_unavailable")
	}
	if identity != presented.Identity {
		return deny("stale_identity")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latest != run || completed != (run.terminal != nil) {
		return deny("lifecycle_changed")
	}
	if desired == v.Pending {
		if completed {
			return deny("already_complete")
		}
	} else {
		if !completed {
			return deny("incomplete_run")
		}
		outcome := v.Evaluate(presented.Input, presented.Checks, presented.Results)
		switch desired {
		case v.Passed:
			if receipt.Match(presented, identity) != nil {
				return deny("unsuccessful_run")
			}
		case v.Failed:
			if outcome != v.Failed {
				return deny("outcome_mismatch")
			}
		case v.Error:
			if outcome == v.Passed || outcome == v.Failed || outcome == v.Pending {
				return deny("outcome_mismatch")
			}
		default:
			return deny("unsupported_status")
		}
	}
	return Decision{Allowed: true, Reason: "trusted_workstation_advisory"}
}

// CheckExecutorEnv rejects publisher credential overrides before execution.
// Callers must also build the process base environment from an explicit allowlist.
func CheckExecutorEnv(env []string) error {
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(name) {
		case "GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN":
			return errors.New("publisher credentials are forbidden in executor environment")
		}
	}
	return nil
}
