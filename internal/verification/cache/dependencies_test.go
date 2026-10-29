package cache

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommittedDependencyFingerprint(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		base := []string{"-C", root, "-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + filepath.Join(root, ".no-hooks"), "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid"}
		cmd := exec.Command("git", append(base, args...)...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, output)
		}
	}
	write := func(name, data string) {
		t.Helper()
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	commit := func() { git("add", "."); git("commit", "--allow-empty", "-m", "fixture") }
	fingerprint := func() string {
		t.Helper()
		value, err := Dependencies(root)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	git("init", "-b", "main")
	write("package.json", `{"private":true}`)
	write("package-lock.json", `{"lockfileVersion":3}`)
	write(".npmrc", "//registry.example/:_authToken=synthetic-only\n")
	commit()
	first := fingerprint()
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "invalid"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	write("src/code.js", "changed source")
	commit()
	if fingerprint() != first {
		t.Fatal("source change invalidated downloads")
	}
	write("package-lock.json", `{"lockfileVersion":3,"changed":true}`)
	if fingerprint() != first {
		t.Fatal("uncommitted change included")
	}
	commit()
	second := fingerprint()
	if first == second {
		t.Fatal("lockfile change ignored")
	}
	write("packages/nested/package.json", `{"name":"workspace"}`)
	commit()
	third := fingerprint()
	if third == second {
		t.Fatal("workspace input ignored")
	}
	write("node_modules/ignored/package.json", `{"name":"installed"}`)
	commit()
	if fingerprint() != third {
		t.Fatal("installed tree included")
	}
	write(".npmrc", "//registry.example/:_authToken=different-synthetic\n")
	commit()
	if fingerprint() == third {
		t.Fatal("npm configuration ignored")
	}
	git("rm", ".npmrc")
	if err := os.Symlink("package.json", filepath.Join(root, ".npmrc")); err != nil {
		t.Fatal(err)
	}
	commit()
	if _, err := Dependencies(root); err == nil || strings.Contains(err.Error(), "synthetic") {
		t.Fatal("symlink or sensitive diagnostic", err)
	}
}
