package session

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/smeltery/bellows/internal/reporting/freshness"
	"github.com/smeltery/bellows/internal/reporting/github"
	v "github.com/smeltery/bellows/internal/verification"
)

type advisoryAPI struct {
	inputs  func() (string, string)
	updates []github.Update
}

func (a *advisoryAPI) PullIdentity(context.Context, string, int) (string, string, error) {
	head, base := a.inputs()
	return head, base, nil
}
func (a *advisoryAPI) Publish(ctx context.Context, update github.Update, guard github.Guard) error {
	if err := guard(ctx, update.Identity); err != nil {
		return err
	}
	a.updates = append(a.updates, update)
	return nil
}

func TestCompletedRunMovementOnlyInvalidatesItsOriginalHead(t *testing.T) {
	for _, change := range []string{"head", "base", "local head", "config", "policy", "toolchain"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			git := func(args ...string) string {
				cmd := exec.Command("git", append([]string{"-C", root, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid"}, args...)...)
				for _, entry := range os.Environ() {
					if !strings.HasPrefix(entry, "GIT_") {
						cmd.Env = append(cmd.Env, entry)
					}
				}
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %s %v", args, out, err)
				}
				return strings.TrimSpace(string(out))
			}
			git("init", "-b", "base")
			git("commit", "--allow-empty", "-m", "base")
			git("checkout", "-b", "feature")
			git("commit", "--allow-empty", "-m", "head")
			initial := evidence()
			initial.Identity.Head, initial.Identity.Base = git("rev-parse", "refs/heads/feature"), git("rev-parse", "refs/heads/base")
			current := initial.Identity
			api := &advisoryAPI{inputs: func() (string, string) { return git("rev-parse", "refs/heads/feature"), git("rev-parse", "refs/heads/base") }}
			reporter, err := New(api, "example/repo", 1, true, func(context.Context) (v.Identity, error) { return current, nil })
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reporter.Recheck(context.Background()); err == nil {
				t.Fatal("imported/unstarted run accepted")
			}
			if err := reporter.Start(context.Background(), initial); err != nil {
				t.Fatal(err)
			}
			completed := initial
			end := time.Now().UTC()
			completed.CompletedAt = &end
			zero := 0
			completed.Results = []v.Result{{ID: "test", State: v.Passed, ExitCode: &zero}}
			if err := reporter.Finish(context.Background(), completed); err != nil {
				t.Fatal(err)
			}
			if observation, err := reporter.Recheck(context.Background()); err != nil || observation.State != "current" || len(api.updates) != 2 {
				t.Fatalf("%+v %v", observation, err)
			}
			switch change {
			case "head":
				git("commit", "--allow-empty", "-m", "advanced")
			case "base":
				git("update-ref", "refs/heads/base", initial.Identity.Head)
			case "local head":
				current.Head = initial.Identity.Base
			case "config":
				current.Fingerprints.Config = strings.Repeat("e", 64)
			case "policy":
				current.Fingerprints.Policy = strings.Repeat("e", 64)
			case "toolchain":
				current.Fingerprints.Toolchain = strings.Repeat("e", 64)
			}
			observation, err := reporter.Recheck(context.Background())
			if !errors.Is(err, freshness.ErrStale) || observation.State != "stale" || len(api.updates) != 3 {
				t.Fatalf("%+v %v updates=%v", observation, err, api.updates)
			}
			last := api.updates[2]
			if last.State != github.Error || last.Identity.Head != initial.Identity.Head {
				t.Fatal("invalidation wrote another head or successful state")
			}
			sealed, err := reporter.run.Receipt()
			if err != nil || sealed.Identity != initial.Identity || sealed.Results[0].State != v.Passed {
				t.Fatal("historical evidence was rewritten")
			}
		})
	}
}
