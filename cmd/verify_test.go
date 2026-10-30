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
	verificationrun "github.com/smeltery/bellows/internal/verification/run"
	"github.com/smeltery/bellows/internal/verification/worktree"
)

func TestVerifyHelpAndValidation(t *testing.T) {
	for _, args := range [][]string{
		{"verify"},
		{"verify", "--repository", "../bad", "--base", "main", "--daemon", "unix:///tmp/socket"},
		{"verify", "--repository", "owner/repo", "--base", "main", "--daemon", "tcp://remote:2375"},
		{"verify", "--repository", "owner/repo", "--base", "main", "--daemon", "unix:///tmp/socket", "--format", "yaml"},
		{"verify", "--repository", "owner/repo", "--base", "main", "--daemon", "unix:///tmp/socket", "--mode", "working"},
	} {
		root := newRootCmd("test")
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	root := newRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"verify", "--help"})
	if err := root.Execute(); err != nil || !strings.Contains(out.String(), "--repository") || !strings.Contains(out.String(), "--daemon") {
		t.Fatalf("help: %s %v", &out, err)
	}
}

func TestVerificationDaemonEndpoints(t *testing.T) {
	for _, endpoint := range []string{"unix:///tmp/private.sock", "tcp://127.0.0.1:2375", "tcp://[::1]:2375"} {
		if !verificationDaemon(endpoint) {
			t.Errorf("rejected %s", endpoint)
		}
	}
	for _, endpoint := range []string{"", "unix://host/path", "unix:///", "tcp://remote:2375", "tcp://localhost:2375", "tcp://127.0.0.1", "tcp://127.0.0.1:0", "tcp://127.0.0.1:65536", "tcp://user:secret@127.0.0.1:2375", "tcp://127.0.0.1:2375?secret", "tcp://127.0.0.1:2375/path"} {
		if verificationDaemon(endpoint) {
			t.Errorf("accepted %s", endpoint)
		}
	}
}

func TestVerifyJSONFailureKeepsStdoutStructured(t *testing.T) {
	root := newRootCmd("test")
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--repo", t.TempDir(), "verify", "--repository", "owner/repo", "--base", "missing", "--daemon", "unix:///missing/socket", "--format", "json", "--artifacts", privateVerificationDir(t), "--leases", privateVerificationDir(t)})
	if err := root.Execute(); err == nil {
		t.Fatal("verification failure returned success")
	}
	var outcome verificationrun.Outcome
	if err := json.Unmarshal(stdout.Bytes(), &outcome); err != nil || outcome.State == v.Passed || outcome.Path == "" {
		t.Fatalf("outcome=%+v stdout=%s err=%v", outcome, &stdout, err)
	}
}

func verificationFixture(t *testing.T, cfg, body string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".github/workflows"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{".bellows.yml": cfg, ".github/workflows/ci.yml": body} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestVerificationPlannerRejectsUnsafeConfigBeforeDocker(t *testing.T) {
	for _, tc := range []struct{ cfg, workflow string }{
		{"version: 1\ndefault_workflows: [ci.yml]\nsecrets: {mode: env}\n", "jobs: {}"},
		{"version: 1\ndefault_workflows: [ci.yml]\npolicy: {cloud_jobs: off}\n", "jobs:\n  deploy:\n    runs-on: ubuntu-24.04\n    steps: [{run: deploy}]"},
		{"version: 1\ndefault_workflows: [../../outside.yml]\n", "jobs: {}"},
	} {
		root := verificationFixture(t, tc.cfg, tc.workflow)
		if _, err := buildVerificationPlan(context.Background(), root, "unix:///missing/socket", t.TempDir(), &worktree.Run{Path: root}); err == nil {
			t.Fatal("unsafe planner input accepted")
		}
	}
}

func TestVerificationPlannerPinsIdentityAndOnlyNeededImages(t *testing.T) {
	root := verificationFixture(t, "version: 1\ndefault_workflows: [ci.yml]\n", "on: pull_request\njobs:\n  test:\n    runs-on: ubuntu-24.04\n    steps: [{run: 'true'}]\n")
	git := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, "GIT_") {
				cmd.Env = append(cmd.Env, value)
			}
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-b", "main")
	git("add", ".")
	git("commit", "-m", "fixture")
	head, tree := git("rev-parse", "HEAD"), git("rev-parse", "HEAD^{tree}")
	bin := t.TempDir()
	scripts := map[string]string{
		"act":    "#!/bin/sh\nprintf 'act fixture\\n'\n",
		"docker": "#!/bin/sh\ncase \"$3\" in\ninfo) echo 'fixture-daemon linux';;\nversion) echo 'fixture-version';;\nimage) [ \"$5\" = ghcr.io/smeltery/kiln:ubuntu-24.04 ] || exit 93; echo sha256:fixture;;\nesac\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	prepared := &worktree.Run{Path: root, Identity: v.Identity{Repository: "owner/repo", Head: head, Base: head, Tree: tree, Mode: v.ModeMerge}}
	plan, err := buildVerificationPlan(context.Background(), root, "unix:///synthetic.sock", privateVerificationDir(t), prepared)
	if plan.Close != nil {
		defer func() {
			if err := plan.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	if err != nil || plan.Identity.Head != head || plan.Identity.Tree != tree || len(plan.Targets) != 1 || len(plan.Env) != 0 || len(plan.Identity.Fingerprints.Toolchain) != 64 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
}

func privateVerificationDir(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResultReuseFlagValidation(t *testing.T) {
	for _, tc := range []struct {
		flags   []string
		message string
	}{
		{[]string{"--daemon", "unix:///tmp/socket", "--reuse-results"}, "--reuse-results requires --profile"},
		{[]string{"--profile", "native", "--force-rerun"}, "--force-rerun requires --reuse-results"},
	} {
		command := newRootCmd("test")
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs(append([]string{"verify", "--repository", "example/project", "--base", "main"}, tc.flags...))
		if err := command.Execute(); err == nil || !strings.Contains(err.Error(), tc.message) {
			t.Fatalf("expected %q, got %v", tc.message, err)
		}
	}
	command := newVerifyCmd(&cliOptions{})
	for _, flag := range []string{"reuse-results", "force-rerun"} {
		if value, err := command.Flags().GetBool(flag); err != nil || value {
			t.Fatalf("%s must default off: %v", flag, err)
		}
	}
}
