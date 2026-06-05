package releasecheck

import (
	"os/exec"
	"testing"
)

func TestCheckChangelogAllowsFeatureCommit(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test")
	git(t, repo, "commit", "--allow-empty", "-m", "feat: initial")
	git(t, repo, "tag", "--no-sign", "v0.1.0")
	git(t, repo, "commit", "--allow-empty", "-m", "feat: next")

	if err := (Runner{Root: repo}).checkChangelog(); err != nil {
		t.Fatalf("expected changelog to pass: %v", err)
	}
}

func TestCheckChangelogRejectsDocsOnlyCommit(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test")
	git(t, repo, "commit", "--allow-empty", "-m", "feat: initial")
	git(t, repo, "tag", "--no-sign", "v0.1.0")
	git(t, repo, "commit", "--allow-empty", "-m", "docs: update")

	if err := (Runner{Root: repo}).checkChangelog(); err == nil {
		t.Fatal("expected docs-only changelog to fail")
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
