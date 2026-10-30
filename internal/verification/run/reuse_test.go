package run

import (
	"context"
	"errors"
	"testing"

	v "github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/verification/worktree"
)

type emptyReuse struct{}

func (emptyReuse) Lookup(CachePlan, string) (v.Result, bool, error) {
	return v.Result{}, false, errors.New("unavailable")
}
func (emptyReuse) Save(Outcome) error { return nil }

func TestSuccessSealLifecycle(t *testing.T) {
	for _, kind := range []string{"success", "cleanup", "optional failure", "changed current", "edited plan", "executor provenance", "cleanup drift"} {
		t.Run(kind, func(t *testing.T) {
			opts, _ := fixture(t)
			build := opts.Build
			opts.Reuse = emptyReuse{}
			opts.Build = func(ctx context.Context, w *worktree.Run) (Plan, error) {
				p, err := build(ctx, w)
				p.Identity.Executor = v.ExecutorFlox
				p.Cacheable = map[string]bool{"test": true}
				identity := p.Identity
				p.Current = func(context.Context) (v.Identity, error) { return identity, nil }
				original := p.Executor
				p.Executor = fakeExecutor(func(ctx context.Context, req v.Request) (v.Result, error) {
					result, err := original.Run(ctx, req)
					switch kind {
					case "changed current":
						identity.Fingerprints.Policy = "changed"
					case "edited plan":
						p.Cacheable["extra"] = true
					case "executor provenance":
						result.Reuse = &v.Reuse{}
					case "optional failure":
						code := 1
						result.State = v.Failed
						result.ExitCode = &code
					}
					return result, err
				})
				if kind == "optional failure" {
					p.Targets[0].Check.Required = false
				}
				if kind == "cleanup drift" {
					p.Close = func() error { identity.Fingerprints.Toolchain = "changed"; return nil }
				}
				if kind == "cleanup" {
					p.Close = func() error { return errors.New("cleanup") }
				}
				return p, err
			}
			out, err := Execute(context.Background(), opts)
			plan, data, ids, sealErr := out.CacheEvidence()
			if kind != "success" {
				if sealErr == nil {
					t.Fatal("unsuccessful outcome sealed")
				}
				return
			}
			if err != nil || sealErr != nil || len(ids) != 1 {
				t.Fatalf("%v %v %v", err, sealErr, ids)
			}
			plan.Cacheable["test"] = false
			data[0] = '!'
			ids[0] = "edited"
			again, _, againIDs, err := out.CacheEvidence()
			if err != nil || !again.Cacheable["test"] || againIDs[0] != "test" {
				t.Fatal("evidence aliases seal")
			}
			imported := Outcome{Receipt: out.Receipt, State: out.State, Path: out.Path}
			if _, _, _, err := imported.CacheEvidence(); err == nil {
				t.Fatal("imported outcome trusted")
			}
			out.Receipt.RunID = "edited"
			if _, _, _, err := out.CacheEvidence(); err == nil {
				t.Fatal("edited outcome trusted")
			}
		})
	}
}
