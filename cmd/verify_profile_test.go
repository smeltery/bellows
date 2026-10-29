package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	v "github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/verification/receipt"
	verificationrun "github.com/smeltery/bellows/internal/verification/run"
	"github.com/smeltery/bellows/internal/verification/worktree"
)

func TestNativeProfilesEndToEnd(t *testing.T) {
	root := t.TempDir()
	for _, file := range []struct{ source, destination string }{
		{"testdata/native/.bellows.yml", ".bellows.yml"}, {"testdata/native/check.sh", "check.sh"},
		{"../.flox/env.json", ".flox/env.json"}, {"../.flox/env/manifest.toml", ".flox/env/manifest.toml"}, {"../.flox/env/manifest.lock", ".flox/env/manifest.lock"},
	} {
		data, err := os.ReadFile(file.source)
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(root, file.destination)
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string {
		command := exec.Command("git", append([]string{"-C", root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, "GIT_") {
				command.Env = append(command.Env, value)
			}
		}
		data, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git: %v %s", err, data)
		}
		return strings.TrimSpace(string(data))
	}
	git("init", "-b", "main")
	git("add", ".")
	git("commit", "-m", "fixture")
	if os.Getenv("BELLOWS_NATIVE_FLOX_INTEGRATION") != "1" {
		bin := t.TempDir()
		script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo flox-fixture; exit; fi\n[ \"$1\" = activate ] && [ \"$2\" = -- ] || exit 93\nshift 2\nexec \"$@\"\n"
		if err := os.WriteFile(filepath.Join(bin, "flox"), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	t.Setenv("GH_TOKEN", "synthetic-publisher-secret")
	t.Setenv("BELLOWS_REPORT_TOKEN", "synthetic-publisher-secret")
	execute := func(profile string, want v.State) verificationrun.Outcome {
		t.Helper()
		command := newRootCmd("test")
		var stdout, stderr bytes.Buffer
		command.SetOut(&stdout)
		command.SetErr(&stderr)
		command.SetArgs([]string{"--repo", root, "verify", "--repository", "example/project", "--base", "main", "--profile", profile, "--format", "json", "--artifacts", privateVerificationDir(t)})
		err := command.Execute()
		if (err == nil) != (want == v.Passed) {
			t.Fatalf("profile %s error=%v stderr=%s stdout=%s", profile, err, &stderr, &stdout)
		}
		var out verificationrun.Outcome
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("json: %v %s", err, &stdout)
		}
		if out.State != want || out.Path == "" || out.Receipt.Identity.Executor != v.ExecutorFlox {
			t.Fatalf("outcome: %+v stderr=%s", out, &stderr)
		}
		data, err := os.ReadFile(out.Path)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := receipt.Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		if v.Evaluate(saved.Input, saved.Checks, saved.Results) != out.State {
			t.Fatal("canonical receipt changed aggregate state")
		}
		return out
	}
	first := execute("native", v.Passed)
	second := execute("native", v.Passed)
	if first.Receipt.Identity != second.Receipt.Identity {
		t.Fatal("identical profile inputs changed identity")
	}
	equivalent := execute("equivalent", v.Passed)
	if first.Receipt.Identity.Fingerprints.Config == equivalent.Receipt.Identity.Fingerprints.Config {
		t.Fatal("profile name missing from identity")
	}
	failed := execute("failure", v.Failed)
	states := map[string]v.State{}
	for _, result := range failed.Receipt.Results {
		states[result.ID] = result.State
	}
	if states["profile/failure/verify"] != v.Failed || states["profile/failure/test"] != v.Skipped {
		t.Fatalf("failure did not propagate: %v", states)
	}
	if git("status", "--porcelain") != "" {
		t.Fatal("original fixture checkout changed")
	}
}

func TestNativeProfileRejectsAmbiguousExecutor(t *testing.T) {
	command := newRootCmd("test")
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"verify", "--repository", "example/project", "--base", "main", "--profile", "native", "--daemon", "unix:///tmp/synthetic.sock"})
	if command.Execute() == nil {
		t.Fatal("accepted two executor selections")
	}
}

func TestNativeCurrentRereadsHostAfterWorktreeCleanup(t *testing.T) {
	root := verificationFixture(t, "version: 1\nprofiles: {native: {executor: flox, checks: {test: {argv: [echo]}}}}\n", "")
	if err := os.MkdirAll(filepath.Join(root, ".flox/env"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".flox/env/manifest.lock"), []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	binary := filepath.Join(bin, "flox")
	version := func(value string) {
		t.Helper()
		if err := os.WriteFile(binary, []byte("#!/bin/sh\necho "+value+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	version("flox-one")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	plan, err := buildNativeVerificationPlan(context.Background(), "native", &worktree.Run{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	current, err := plan.Current(context.Background())
	if err != nil || current != plan.Identity {
		t.Fatalf("immutable lock unavailable after cleanup: %v", err)
	}
	version("flox-two")
	current, err = plan.Current(context.Background())
	if err != nil || current.Fingerprints.Toolchain == plan.Identity.Fingerprints.Toolchain {
		t.Fatalf("host version change hidden: %v", err)
	}
	version("flox-one")
	t.Setenv("HOME", t.TempDir())
	current, err = plan.Current(context.Background())
	if err != nil || current.Fingerprints.Environment == plan.Identity.Fingerprints.Environment {
		t.Fatalf("host environment change hidden: %v", err)
	}
}
