package freshness

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/smeltery/bellows/internal/reporting/github"
	v "github.com/smeltery/bellows/internal/verification"
)

func TestCheckUsesAllIdentityFieldsWithoutExposingValues(t *testing.T) {
	expected := v.Identity{Repository: "example/repo", Head: "head", Base: "base"}
	for _, change := range []struct {
		name   string
		mutate func(*v.Identity)
	}{
		{"head", func(i *v.Identity) { i.Head = "secret-new-head" }},
		{"base", func(i *v.Identity) { i.Base = "secret-new-base" }},
		{"config", func(i *v.Identity) { i.Fingerprints.Config = "secret-config" }},
		{"policy", func(i *v.Identity) { i.Fingerprints.Policy = "secret-policy" }},
		{"toolchain", func(i *v.Identity) { i.Fingerprints.Toolchain = "secret-toolchain" }},
		{"environment", func(i *v.Identity) { i.Fingerprints.Environment = "secret-environment" }},
	} {
		now := expected
		change.mutate(&now)
		got := Check(context.Background(), expected, func(context.Context) (v.Identity, error) { return now, nil })
		if got.State != "stale" || !reflect.DeepEqual(got.Changed, []string{change.name}) {
			t.Fatalf("%+v", got)
		}
	}
	got := Check(context.Background(), expected, func(context.Context) (v.Identity, error) { return v.Identity{}, errors.New("secret network detail") })
	if got.State != "unknown" || len(got.Changed) != 0 {
		t.Fatalf("%+v", got)
	}
}

type publishing func(context.Context, github.Update, github.Guard) error

func (p publishing) Publish(ctx context.Context, u github.Update, g github.Guard) error {
	return p(ctx, u, g)
}

func TestInvalidationNeverPublishesSuccessOrWrongHeadAndRechecksOwnership(t *testing.T) {
	expected := v.Identity{Repository: "example/repo", Head: "old", Base: "base"}
	for _, scenario := range []string{"stale", "current", "unknown", "superseded", "retry", "wrong-head"} {
		t.Run(scenario, func(t *testing.T) {
			owned, lookups := true, 0
			current := func(context.Context) (v.Identity, error) {
				lookups++
				if scenario == "unknown" {
					return v.Identity{}, errors.New("unavailable")
				}
				now := expected
				if scenario != "current" {
					now.Base = "advanced"
				}
				if scenario == "superseded" || scenario == "retry" && lookups == 2 {
					owned = false
				}
				return now, nil
			}
			api := publishing(func(ctx context.Context, u github.Update, guard github.Guard) error {
				if u.State != github.Error || u.Identity != expected {
					t.Fatal("invalid invalidation target/state")
				}
				target := u.Identity
				if scenario == "wrong-head" {
					target.Head = "another"
				}
				if err := guard(ctx, target); err != nil {
					return err
				}
				if scenario == "retry" {
					return guard(ctx, target)
				}
				return nil
			})
			err := Invalidate(context.Background(), api, expected, current, func() bool { return owned })
			if (err == nil) != (scenario == "stale") {
				t.Fatalf("err=%v", err)
			}
			if scenario == "retry" && lookups != 2 {
				t.Fatal("retry reused observation")
			}
		})
	}
}

func TestWatchUsesExplicitTicksAndStopsOnStaleOrCancellation(t *testing.T) {
	ticks := make(chan time.Time, 1)
	ticks <- time.Time{}
	calls := 0
	err := Watch(context.Background(), ticks, func(context.Context) (Observation, error) {
		calls++
		if calls == 2 {
			return Observation{State: "stale"}, nil
		}
		return Observation{State: "current"}, nil
	})
	if !errors.Is(err, ErrStale) || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	err = Watch(ctx, nil, func(context.Context) (Observation, error) { cancel(); return Observation{State: "current"}, nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	err = Watch(ctx, nil, func(context.Context) (Observation, error) { cancel(); return Observation{State: "unknown"}, ErrUnknown })
	if !errors.Is(err, context.Canceled) {
		t.Fatal("expired lookup obscured watch cancellation", err)
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Time{})
	defer cancel()
	if err = Watch(ctx, nil, func(context.Context) (Observation, error) {
		t.Fatal("lookup after deadline")
		return Observation{}, nil
	}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
