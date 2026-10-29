package changed

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	options := []string{"-C", root, "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "core.hooksPath=" + filepath.Join(root, ".no-hooks"), "-c", "user.name=Test", "-c", "user.email=test@example.invalid"}
	cmd := exec.Command("git", append(options, args...)...)
	cmd.Env = gitEnvironment()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func repository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	return root
}

func write(t *testing.T, root, name, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, root string) string {
	t.Helper()
	git(t, root, "add", ".")
	git(t, root, "commit", "--allow-empty", "-m", "fixture")
	return git(t, root, "rev-parse", "HEAD")
}

func assertFiles(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
}

func TestGitFilesUnionsLocalChangesAndPreservesPaths(t *testing.T) {
	root := repository(t)
	for _, file := range []string{"both", "deleted", "old name", ".gitignore"} {
		write(t, root, file, "ignored\n")
	}
	commit(t, root)
	write(t, root, "both", "staged")
	git(t, root, "add", "both")
	write(t, root, "both", "unstaged")
	git(t, root, "mv", "old name", "new\nname")
	if err := os.Remove(filepath.Join(root, "deleted")); err != nil {
		t.Fatal(err)
	}
	write(t, root, "untracked\nfile", "new")
	write(t, root, "ignored", "ignored")

	staged, err := Discover(root, Options{Scope: Staged})
	if err != nil {
		t.Fatal(err)
	}
	assertFiles(t, staged.Files, []string{"both", "new\nname", "old name"})
	files, err := GitFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	assertFiles(t, files, []string{"both", "deleted", "new\nname", "old name", "untracked\nfile"})
}

func TestDiscoverInitialCommitAndUnbornHead(t *testing.T) {
	root := repository(t)
	write(t, root, "staged", "a")
	git(t, root, "add", "staged")
	write(t, root, "untracked", "b")
	got, err := Discover(root, Options{Scope: WorkingTree})
	if err != nil {
		t.Fatal(err)
	}
	assertFiles(t, got.Files, []string{"staged", "untracked"})
	if got.Head != "" || got.Base != "" || got.MergeBase != "" {
		t.Fatalf("local changes must not claim commit identity: %+v", got)
	}
	if _, err := Discover(root, Options{Scope: BaseHead, Base: "main"}); err == nil {
		t.Fatal("unborn commit comparison must fail")
	}
	commit(t, root)
	got, err = Discover(root, Options{Scope: BaseHead, Base: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	assertFiles(t, got.Files, nil)
}

func TestDiscoverCommittedChangesUsesMergeBaseAndPinnedIDs(t *testing.T) {
	root := repository(t)
	write(t, root, "original", "content\n")
	ancestor := commit(t, root)
	git(t, root, "checkout", "-b", "feature")
	git(t, root, "mv", "original", "renamed")
	write(t, root, "feature", "change")
	head := commit(t, root)
	git(t, root, "checkout", "main")
	write(t, root, "base-only", "change")
	base := commit(t, root)
	git(t, root, "checkout", "feature")
	git(t, root, "branch", "--set-upstream-to=main")
	for _, opts := range []Options{{Scope: BaseHead, Base: "main"}, {Scope: Outgoing}} {
		got, err := Discover(root, opts)
		if err != nil {
			t.Fatal(err)
		}
		assertFiles(t, got.Files, []string{"feature", "original", "renamed"})
		if got.Base != base || got.Head != head || got.MergeBase != ancestor {
			t.Fatalf("incorrect comparison identity: %+v", got)
		}
	}
	write(t, root, "dirty", "must not enter committed scope")
	got, err := Discover(root, Options{Scope: BaseHead, Base: base, Head: head})
	if err != nil {
		t.Fatal(err)
	}
	assertFiles(t, got.Files, []string{"feature", "original", "renamed"})
}

func TestDiscoverRefAndHistoryErrors(t *testing.T) {
	root := repository(t)
	commit(t, root)
	for _, opts := range []Options{
		{Scope: "unknown"}, {Scope: Staged, Base: "main"},
		{Scope: WorkingTree, Head: "HEAD"}, {Scope: BaseHead},
		{Scope: BaseHead, Base: "absent"},
		{Scope: BaseHead, Base: "main", Head: "absent"}, {Scope: Outgoing},
	} {
		if _, err := Discover(root, opts); err == nil {
			t.Errorf("expected explicit error for %+v", opts)
		}
	}
	git(t, root, "checkout", "--orphan", "unrelated")
	write(t, root, "unrelated", "distinct root commit")
	commit(t, root)
	if _, err := Discover(root, Options{Scope: BaseHead, Base: "main"}); err == nil || !strings.Contains(err.Error(), "merge base") {
		t.Fatalf("unrelated history: %v", err)
	}
	if _, err := GitFiles(t.TempDir()); err == nil {
		t.Fatal("invalid repository must not return empty success")
	}
}

func TestDiscoverRejectsShallowCommittedHistory(t *testing.T) {
	root := repository(t)
	commit(t, root)
	commit(t, root)
	clone := filepath.Join(t.TempDir(), "shallow")
	git(t, root, "clone", "--depth=1", "file://"+root, clone)
	if _, err := Discover(clone, Options{Scope: BaseHead, Base: "HEAD"}); err == nil || !strings.Contains(err.Error(), "unshallow") {
		t.Fatalf("shallow history: %v", err)
	}
}

func TestDiscoverIgnoresInheritedRepositoryEnvironment(t *testing.T) {
	root := repository(t)
	commit(t, root)
	write(t, root, "own", "change")
	foreign := repository(t)
	foreignHead := commit(t, foreign)
	write(t, foreign, "foreign", "must not be included")
	t.Setenv("GIT_DIR", filepath.Join(foreign, ".git"))
	t.Setenv("GIT_WORK_TREE", foreign)
	t.Setenv("GIT_COMMON_DIR", filepath.Join(foreign, ".git"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(foreign, ".git", "index"))
	got, err := GitFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	assertFiles(t, got, []string{"own"})
	if git(t, foreign, "rev-parse", "HEAD") != foreignHead {
		t.Fatal("foreign repository was modified")
	}
}

func TestDiscoverIncludesIgnoredSubmoduleChanges(t *testing.T) {
	dependency := repository(t)
	commit(t, dependency)
	root := repository(t)
	commit(t, root)
	git(t, root, "-c", "protocol.file.allow=always", "submodule", "add", dependency, "dependency")
	git(t, root, "config", "-f", ".gitmodules", "submodule.dependency.ignore", "all")
	base := commit(t, root)
	git(t, root, "config", "diff.ignoreSubmodules", "all")
	commit(t, filepath.Join(root, "dependency"))
	for _, scope := range []Scope{WorkingTree, Staged, BaseHead} {
		opts := Options{Scope: scope}
		if scope == Staged {
			git(t, root, "add", "dependency")
		}
		if scope == BaseHead {
			commit(t, root)
			opts.Base = base
		}
		got, err := Discover(root, opts)
		if err != nil {
			t.Fatal(err)
		}
		assertFiles(t, got.Files, []string{"dependency"})
	}
}
