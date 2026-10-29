package flox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/smeltery/bellows/internal/verification"
)

func fixture(t *testing.T) (Executor, verification.Request) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "worktree with spaces")
	if err := os.MkdirAll(filepath.Join(root, ".flox", "env"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".flox", "env", "manifest.lock"), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "fake flox")
	script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo flox-test; exit; fi\n[ \"$1\" = activate ] && [ \"$2\" = -- ] || exit 93\nshift 2\nexec \"$@\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	e := Executor{Binary: binary}
	fingerprint, err := e.Toolchain(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	return e, verification.Request{RunID: "fixture-run", Worktree: root, Identity: verification.Identity{Executor: verification.ExecutorFlox, Platform: runtime.GOOS + "/" + runtime.GOARCH, Fingerprints: verification.Fingerprints{Toolchain: fingerprint}}, Check: verification.Check{ID: "test", Required: true}, Argv: []string{"/bin/sh", "-c", "exit 0"}}
}

func TestRunArgvEnvironmentAndWorktree(t *testing.T) {
	e, req := fixture(t)
	t.Setenv("BELLOWS_REPORT_TOKEN", "parent-report-secret")
	t.Setenv("GH_TOKEN", "parent-github-secret")
	req.Env = []string{"CHECK_INPUT=allowed"}
	req.Argv = []string{"/bin/sh", "-c", `printf '%s\n' "$PWD" "$CHECK_INPUT" "${GH_TOKEN-unset}" "${BELLOWS_REPORT_TOKEN-unset}" "$1"`, "fixture", "$(touch escaped)"}
	var output bytes.Buffer
	req.Stdout = &output
	result, err := e.Run(context.Background(), req)
	if err != nil || result.State != verification.Passed || result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	canonical, err := filepath.EvalSymlinks(req.Worktree)
	if err != nil {
		t.Fatal(err)
	}
	if want := canonical + "\nallowed\nunset\nunset\n$(touch escaped)\n"; output.String() != want {
		t.Fatalf("output=%q want=%q", output.String(), want)
	}
	if _, err := os.Stat(filepath.Join(req.Worktree, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("argv underwent shell expansion: %v", err)
	}
}

func TestRunFailureAndUnsupportedInputs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  func(*Executor, *verification.Request)
		state   verification.State
		wantErr bool
	}{
		{"exit", func(_ *Executor, r *verification.Request) { r.Argv = []string{"/bin/sh", "-c", "exit 17"} }, verification.Failed, false},
		{"stale toolchain", func(_ *Executor, r *verification.Request) { r.Identity.Fingerprints.Toolchain = "stale" }, verification.Error, true},
		{"signal", func(_ *Executor, r *verification.Request) { r.Argv = []string{"/bin/sh", "-c", "kill -TERM $$"} }, verification.Error, false},
		{"platform", func(_ *Executor, r *verification.Request) { r.Identity.Platform = "unsupported/host" }, verification.Unsupported, true},
		{"missing flox", func(e *Executor, _ *verification.Request) { e.Binary = "/missing/flox" }, verification.Error, true},
		{"workflow", func(_ *Executor, r *verification.Request) { r.Workflow = "ci.yml" }, verification.Error, true},
		{"matrix", func(_ *Executor, r *verification.Request) { r.Matrix = map[string]string{"os": "linux"} }, verification.Error, true},
		{"credentials", func(_ *Executor, r *verification.Request) { r.Env = []string{"GH_TOKEN=secret"} }, verification.Error, true},
		{"malformed env", func(_ *Executor, r *verification.Request) { r.Env = []string{"BAD"} }, verification.Error, true},
		{"unlocked", func(_ *Executor, r *verification.Request) {
			if err := os.Remove(filepath.Join(r.Worktree, ".flox/env/manifest.lock")); err != nil {
				t.Fatal(err)
			}
		}, verification.Error, true},
		{"changed lock", func(_ *Executor, r *verification.Request) {
			r.Argv = []string{"/bin/sh", "-c", `printf '{"version":2}' > .flox/env/manifest.lock`}
		}, verification.Error, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, r := fixture(t)
			tc.change(&e, &r)
			result, err := e.Run(context.Background(), r)
			if result.State != tc.state || (err != nil) != tc.wantErr {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if tc.name == "exit" && (result.ExitCode == nil || *result.ExitCode != 17) {
				t.Fatalf("exit=%v", result.ExitCode)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("error disclosed credential")
			}
		})
	}
}

func TestCancellationStopsDescendants(t *testing.T) {
	e, r := fixture(t)
	marker := filepath.Join(r.Worktree, "descendant-completed")
	r.Argv = []string{"/bin/sh", "-c", `(sleep 1; touch "$1") & wait`, "fixture", marker}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	result, err := e.Run(ctx, r)
	if err != nil || result.State != verification.Cancelled || result.ExitCode != nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant outlived cancellation: %v", err)
	}
}

func TestToolchainFingerprint(t *testing.T) {
	e, r := fixture(t)
	first, err := e.Toolchain(context.Background(), r.Worktree)
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.Toolchain(context.Background(), r.Worktree)
	if err != nil || first != again || len(first) != 64 {
		t.Fatalf("fingerprint unstable: %q %q %v", first, again, err)
	}
	if err := os.WriteFile(filepath.Join(r.Worktree, ".flox/env/manifest.lock"), []byte(`{"version":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := e.Toolchain(context.Background(), r.Worktree)
	if err != nil || first == changed {
		t.Fatalf("lock change not reflected: %v", err)
	}
}

func TestToolchainCancellation(t *testing.T) {
	e, r := fixture(t)
	if err := os.WriteFile(e.Binary, []byte("#!/bin/sh\nsleep 10 & wait\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := e.Toolchain(ctx, r.Worktree); err == nil {
		t.Fatal("cancelled version probe succeeded")
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("version probe did not stop promptly")
	}
}
