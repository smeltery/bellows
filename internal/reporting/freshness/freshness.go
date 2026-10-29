// Package freshness observes advisory evidence; it never authorizes a merge.
package freshness

import (
	"context"
	"errors"
	"time"

	"github.com/smeltery/bellows/internal/reporting/github"
	"github.com/smeltery/bellows/internal/reporting/policy"
	v "github.com/smeltery/bellows/internal/verification"
)

type Observation struct {
	State   string   `json:"state"`
	Changed []string `json:"changed"`
}

var ErrStale = errors.New("advisory verification identity is stale")
var ErrUnknown = errors.New("current advisory verification identity is unavailable")

func Check(ctx context.Context, expected v.Identity, current policy.Resolver) Observation {
	out := Observation{State: "unknown", Changed: []string{}}
	if current == nil || ctx.Err() != nil {
		return out
	}
	now, err := current(ctx)
	if err != nil || ctx.Err() != nil {
		return out
	}
	for _, field := range []struct{ name, before, after string }{
		{"repository", expected.Repository, now.Repository},
		{"head", expected.Head, now.Head},
		{"base", expected.Base, now.Base},
		{"tree", expected.Tree, now.Tree},
		{"mode", expected.Mode, now.Mode},
		{"executor", expected.Executor, now.Executor},
		{"platform", expected.Platform, now.Platform},
		{"workflow", expected.Fingerprints.Workflow, now.Fingerprints.Workflow},
		{"config", expected.Fingerprints.Config, now.Fingerprints.Config},
		{"policy", expected.Fingerprints.Policy, now.Fingerprints.Policy},
		{"dependencies", expected.Fingerprints.Dependencies, now.Fingerprints.Dependencies},
		{"toolchain", expected.Fingerprints.Toolchain, now.Fingerprints.Toolchain},
		{"environment", expected.Fingerprints.Environment, now.Fingerprints.Environment},
	} {
		if field.before != field.after {
			out.Changed = append(out.Changed, field.name)
		}
	}
	out.State = "current"
	if len(out.Changed) != 0 {
		out.State = "stale"
	}
	return out
}

type Publisher interface {
	Publish(context.Context, github.Update, github.Guard) error
}

// Invalidate only downgrades the original run's head. Ownership and freshness
// are checked on every attempt; GitHub still cannot atomically order publishers.
func Invalidate(ctx context.Context, api Publisher, expected v.Identity, current policy.Resolver, owned func() bool) error {
	if api == nil || owned == nil {
		return errors.New("advisory invalidation requires an owned reporting run")
	}
	guard := func(ctx context.Context, target v.Identity) error {
		if target != expected || !owned() {
			return errors.New("advisory run is superseded")
		}
		observation := Check(ctx, expected, current)
		if observation.State != "stale" || !owned() {
			return errors.New("advisory invalidation requires freshly observed stale inputs")
		}
		return nil
	}
	return api.Publish(ctx, github.Update{Identity: expected, State: github.Error}, guard)
}

// Watch accepts caller-owned ticks and a bounded context; it does not start a daemon.
func Watch(ctx context.Context, ticks <-chan time.Time, check func(context.Context) (Observation, error)) error {
	if check == nil {
		return ErrUnknown
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		observation, err := check(ctx)
		if ctx.Err() != nil && !errors.Is(err, ErrStale) && observation.State != "stale" {
			return ctx.Err()
		}
		if err != nil {
			return err
		}
		if observation.State == "stale" {
			return ErrStale
		}
		if observation.State != "current" {
			return ErrUnknown
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, open := <-ticks:
			if !open {
				return errors.New("advisory watch timer closed")
			}
		}
	}
}
