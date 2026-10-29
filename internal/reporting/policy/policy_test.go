package policy

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	v "github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/verification/receipt"
)

func completed(t *testing.T) v.Receipt {
	t.Helper()
	data, err := os.ReadFile("../../verification/testdata/passed.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := receipt.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func begun(t *testing.T) (*Session, *Run, v.Receipt) {
	t.Helper()
	r := completed(t)
	initial := r
	initial.CompletedAt = nil
	initial.Results = []v.Result{}
	session := New(Options{OptIn: true, Repository: r.Identity.Repository})
	run, err := session.Begin(initial)
	if err != nil {
		t.Fatal(err)
	}
	return session, run, r
}
func resolver(r v.Receipt) Resolver {
	return func(context.Context) (v.Identity, error) { return r.Identity, nil }
}
func decision(t *testing.T, s *Session, run *Run, r v.Receipt, state v.State, want string) {
	t.Helper()
	got := s.Authorize(context.Background(), run, r, state, resolver(r))
	if got.Reason != want {
		t.Fatalf("got %+v want %s", got, want)
	}
	if got.Allowed != (want == "trusted_workstation_advisory") {
		t.Fatalf("incorrect allowance %+v", got)
	}
}
func TestLifecycle(t *testing.T) {
	s, run, r := begun(t)
	initial, err := run.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	decision(t, s, run, initial, v.Pending, "trusted_workstation_advisory")
	decision(t, s, run, initial, v.Passed, "incomplete_run")
	if err := run.Complete(r); err != nil {
		t.Fatal(err)
	}
	decision(t, s, run, r, v.Passed, "trusted_workstation_advisory")
	decision(t, s, run, r, v.Pending, "already_complete")
	decision(t, s, run, r, v.Failed, "outcome_mismatch")
	decision(t, s, run, r, v.Error, "outcome_mismatch")
	if run.Complete(r) == nil {
		t.Fatal("completed twice")
	}
	copy, err := run.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	copy.Results[0].DurationMS = 0
	decision(t, s, run, copy, v.Passed, "edited_receipt")
	decision(t, s, run, r, v.Passed, "trusted_workstation_advisory")
	if _, err := s.Begin(initial); err != nil {
		t.Fatal(err)
	}
	decision(t, s, run, r, v.Passed, "superseded_run")
}
func TestImportedAndForeignEvidence(t *testing.T) {
	s, run, r := begun(t)
	// A syntactically valid imported success is not an owned execution capability.
	decision(t, s, nil, r, v.Passed, "unknown_run")
	decision(t, s, &Run{}, r, v.Passed, "unknown_run")
	other, foreign, _ := begun(t)
	if err := foreign.Complete(r); err != nil {
		t.Fatal(err)
	}
	decision(t, s, foreign, r, v.Passed, "unknown_run")
	decision(t, other, run, r, v.Passed, "unknown_run")
	initial, _ := run.Receipt()
	if _, err := New(Options{Repository: r.Identity.Repository}).Begin(initial); err == nil {
		t.Fatal("no opt-in")
	}
	if _, err := New(Options{OptIn: true, Repository: "other/repo"}).Begin(initial); err == nil {
		t.Fatal("wrong repository")
	}
	if err := run.Complete(r); err != nil {
		t.Fatal(err)
	}
	r.Identity.Repository = "other/repo"
	decision(t, s, run, r, v.Passed, "edited_receipt")
}
func TestChangedPlanAndIdentityCannotComplete(t *testing.T) {
	changes := map[string]func(*v.Receipt){
		"head":              func(r *v.Receipt) { r.Identity.Head = strings.Repeat("b", 40) },
		"base":              func(r *v.Receipt) { r.Identity.Base = strings.Repeat("b", 40) },
		"policy":            func(r *v.Receipt) { r.Identity.Fingerprints.Policy = strings.Repeat("b", 64) },
		"removed required":  func(r *v.Receipt) { r.Checks = []v.Check{}; r.Results = []v.Result{} },
		"optional required": func(r *v.Receipt) { r.Checks[0].Required = false },
		"missing result":    func(r *v.Receipt) { r.Results = []v.Result{} },
		"run ID":            func(r *v.Receipt) { r.RunID = "another" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			_, run, r := begun(t)
			change(&r)
			if run.Complete(r) == nil {
				t.Fatal("accepted changed execution")
			}
		})
	}
}
func TestFreshIdentityOnEveryAttempt(t *testing.T) {
	s, run, r := begun(t)
	if err := run.Complete(r); err != nil {
		t.Fatal(err)
	}
	changes := []func(*v.Identity){func(i *v.Identity) { i.Head = strings.Repeat("b", 40) }, func(i *v.Identity) { i.Base = strings.Repeat("b", 40) }, func(i *v.Identity) { i.Tree = strings.Repeat("b", 40) }, func(i *v.Identity) { i.Fingerprints.Policy = strings.Repeat("b", 64) }, func(i *v.Identity) { i.Fingerprints.Config = strings.Repeat("b", 64) }}
	for _, change := range changes {
		current := r.Identity
		change(&current)
		got := s.Authorize(context.Background(), run, r, v.Passed, func(context.Context) (v.Identity, error) { return current, nil })
		if got.Reason != "stale_identity" {
			t.Fatal(got)
		}
	}
	calls := 0
	current := func(context.Context) (v.Identity, error) { calls++; return r.Identity, nil }
	for range 2 {
		if !s.Authorize(context.Background(), run, r, v.Passed, current).Allowed {
			t.Fatal("fresh denied")
		}
	}
	if calls != 2 {
		t.Fatal("cached identity")
	}
	unavailable := func(context.Context) (v.Identity, error) {
		return v.Identity{}, errors.New("synthetic credential must not appear in reason")
	}
	got := s.Authorize(context.Background(), run, r, v.Passed, unavailable)
	if got.Reason != "current_identity_unavailable" {
		t.Fatal(got)
	}
	initial := r
	initial.CompletedAt = nil
	initial.Results = []v.Result{}
	got = s.Authorize(context.Background(), run, r, v.Passed, func(context.Context) (v.Identity, error) { _, err := s.Begin(initial); return r.Identity, err })
	if got.Reason != "lifecycle_changed" {
		t.Fatal(got)
	}
}
func TestTerminalFailuresAreReportable(t *testing.T) {
	for _, state := range []v.State{v.Failed, v.Error, v.Cancelled, v.Skipped, v.Unsupported} {
		s, run, r := begun(t)
		r.Results[0].State = state
		r.Results[0].ExitCode = nil
		if state == v.Failed {
			code := 1
			r.Results[0].ExitCode = &code
		}
		if err := run.Complete(r); err != nil {
			t.Fatal(err)
		}
		desired := v.Error
		if state == v.Failed {
			desired = v.Failed
		}
		decision(t, s, run, r, desired, "trusted_workstation_advisory")
		decision(t, s, run, r, v.Passed, "unsuccessful_run")
	}
}
func TestExecutorCredentialOverrideRejected(t *testing.T) {
	for _, name := range []string{"BELLOWS_REPORT_TOKEN", "GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
		executed := false
		env := []string{"PATH=/usr/bin", name + "=synthetic-secret"}
		if CheckExecutorEnv(env) == nil {
			executed = true
		}
		if executed {
			t.Fatalf("executor inherited %s", name)
		}
	}
	if err := CheckExecutorEnv([]string{"PATH=/usr/bin", "CI=true", "GITHUB_WORKSPACE=/tmp/example"}); err != nil {
		t.Fatal(err)
	}
}

func TestMalformedExecutorEnvironment(t *testing.T) {
	for _, entry := range []string{"TOKEN", "=value", "TOKEN=bad\x00value", "TO\x00KEN=value"} {
		if CheckExecutorEnv([]string{entry}) == nil {
			t.Fatal("accepted malformed environment")
		}
	}
	if err := CheckExecutorEnv([]string{"EMPTY="}); err != nil {
		t.Fatal(err)
	}
}
