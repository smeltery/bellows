// Package session connects trusted local lifecycle evidence to advisory publication.
package session

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/smeltery/bellows/internal/reporting/freshness"
	"github.com/smeltery/bellows/internal/reporting/github"
	"github.com/smeltery/bellows/internal/reporting/policy"
	v "github.com/smeltery/bellows/internal/verification"
)

type API interface {
	Publish(context.Context, github.Update, github.Guard) error
	PullIdentity(context.Context, string, int) (string, string, error)
}
type Reporter struct {
	mu         sync.Mutex
	api        API
	current    policy.Resolver
	policy     *policy.Session
	run        *policy.Run
	repository string
	pull       int
}

func New(api API, repository string, pull int, optIn bool, current policy.Resolver) (*Reporter, error) {
	if current == nil || api == nil || repository == "" || pull < 1 || !optIn {
		return nil, errors.New("advisory reporting requires explicit trusted workstation opt-in and a pull request")
	}
	return &Reporter{api: api, current: current, repository: repository, pull: pull, policy: policy.New(policy.Options{OptIn: optIn, Repository: repository})}, nil
}
func (r *Reporter) Start(ctx context.Context, initial v.Receipt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.run != nil {
		return errors.New("reporting run already started")
	}
	run, err := r.policy.Begin(initial)
	if err != nil {
		return err
	}
	r.run = run
	return r.publish(ctx, initial, v.Pending, github.Pending)
}
func (r *Reporter) Finish(ctx context.Context, completed v.Receipt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.run == nil {
		return errors.New("reporting run never started")
	}
	if err := r.run.Complete(completed); err != nil {
		return err
	}
	state := v.Evaluate(completed.Input, completed.Checks, completed.Results)
	status := github.Error
	desired := v.Error
	switch state {
	case v.Passed:
		status, desired = github.Success, v.Passed
	case v.Failed:
		status, desired = github.Failure, v.Failed
	}
	return r.publish(ctx, completed, desired, status)
}
func (r *Reporter) publish(ctx context.Context, evidence v.Receipt, desired v.State, status github.State) error {
	guard := func(ctx context.Context, _ v.Identity) error {
		decision := r.policy.Authorize(ctx, r.run, evidence, desired, r.resolve(evidence.Identity))
		if !decision.Allowed {
			return fmt.Errorf("advisory publication denied: %s", decision.Reason)
		}
		return nil
	}
	return r.api.Publish(ctx, github.Update{Identity: evidence.Identity, State: status}, guard)
}

func (r *Reporter) resolve(expected v.Identity) policy.Resolver {
	return func(ctx context.Context) (v.Identity, error) {
		current, err := r.current(ctx)
		if err != nil {
			return v.Identity{}, err
		}
		head, base, err := r.api.PullIdentity(ctx, r.repository, r.pull)
		if err != nil {
			return v.Identity{}, err
		}
		if current.Head == expected.Head {
			current.Head = head
		}
		if current.Base == expected.Base {
			current.Base = base
		}
		return current, nil
	}
}

// Recheck can only downgrade the completed run owned by this reporter.
func (r *Reporter) Recheck(ctx context.Context) (freshness.Observation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	unknown := freshness.Observation{State: "unknown", Changed: []string{}}
	if r.run == nil {
		return unknown, errors.New("reporting run never started")
	}
	owned := r.run
	evidence, err := owned.Receipt()
	if err != nil || evidence.CompletedAt == nil {
		return unknown, errors.New("reporting run is not complete")
	}
	current := r.resolve(evidence.Identity)
	observation := freshness.Check(ctx, evidence.Identity, current)
	if observation.State == "unknown" {
		return observation, freshness.ErrUnknown
	}
	if observation.State == "current" {
		return observation, nil
	}
	err = freshness.Invalidate(ctx, r.api, evidence.Identity, current, func() bool { return r.run == owned })
	return observation, errors.Join(freshness.ErrStale, err)
}
