// Package session connects trusted local lifecycle evidence to advisory publication.
package session

import (
	"context"
	"errors"
	"fmt"
	"sync"

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
	policy     *policy.Session
	run        *policy.Run
	repository string
	pull       int
}

func New(api API, repository string, pull int, optIn bool) (*Reporter, error) {
	if api == nil || repository == "" || pull < 1 || !optIn {
		return nil, errors.New("advisory reporting requires explicit trusted workstation opt-in and a pull request")
	}
	return &Reporter{api: api, repository: repository, pull: pull, policy: policy.New(policy.Options{OptIn: optIn, Repository: repository})}, nil
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
		decision := r.policy.Authorize(ctx, r.run, evidence, desired, func(ctx context.Context) (v.Identity, error) {
			head, base, err := r.api.PullIdentity(ctx, r.repository, r.pull)
			if err != nil {
				return v.Identity{}, err
			}
			current := evidence.Identity
			current.Head, current.Base = head, base
			return current, nil
		})
		if !decision.Allowed {
			return fmt.Errorf("advisory publication denied: %s", decision.Reason)
		}
		return nil
	}
	return r.api.Publish(ctx, github.Update{Identity: evidence.Identity, State: status}, guard)
}
