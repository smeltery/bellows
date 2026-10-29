package cmd

import (
	"context"
	v "github.com/smeltery/bellows/internal/verification"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReportingFlagOptIn(t *testing.T) {
	t.Setenv("BELLOWS_REPORT_TOKEN", "fixture-secret")
	for _, opts := range []reportingFlags{{trusted: true}, {pull: 1}, {watchDuration: time.Second}, {enabled: true, trusted: true, pull: 1, watchDuration: -time.Second}, {enabled: true, trusted: true, pull: 1, watchDuration: 2 * time.Hour}, {enabled: true, pull: 1}, {enabled: true, trusted: true}} {
		if _, err := opts.create("example/repo", func(context.Context) (v.Identity, error) { return v.Identity{}, nil }); err == nil {
			t.Fatalf("accepted %+v", opts)
		}
	}
	if got, err := (&reportingFlags{}).create("example/repo", func(context.Context) (v.Identity, error) { return v.Identity{}, nil }); err != nil || got != nil {
		t.Fatal("local-only default attempted reporting")
	}
	if _, err := (&reportingFlags{enabled: true, trusted: true, pull: 1}).create("example/repo", func(context.Context) (v.Identity, error) { return v.Identity{}, nil }); err != nil {
		t.Fatal(err)
	}
}

func TestReportingResolverTracksCommittedPolicyChanges(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		runGit(t, root, append([]string{"-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid"}, args...)...)
	}
	git("init", "-b", "main")
	path := filepath.Join(root, ".bellows.yml")
	if err := os.WriteFile(path, []byte("old policy"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "base")
	git("checkout", "-b", "feature")
	git("commit", "--allow-empty", "-m", "head")
	resolve := currentWithRefs(root, "main", "feature", func(context.Context) (v.Identity, error) { return v.Identity{Repository: "example/repo"}, nil })
	before, err := resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("new policy"), 0600); err != nil {
		t.Fatal(err)
	}
	dirty, err := resolve(context.Background())
	if err != nil || dirty != before {
		t.Fatal("uncommitted original edits changed pinned inputs", err)
	}
	git("add", ".")
	git("commit", "-m", "change policy")
	after, err := resolve(context.Background())
	if err != nil || after.Head == before.Head || after.Base != before.Base {
		t.Fatal("committed policy movement went undetected", err)
	}
}
