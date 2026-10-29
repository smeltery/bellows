package actexec

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smeltery/bellows/internal/config"
	v "github.com/smeltery/bellows/internal/verification"
)

// This opt-in test owns only its outer DinD containers; ordinary tests need no Docker.
func TestDedicatedDockerIntegration(t *testing.T) {
	if os.Getenv("BELLOWS_SERVICE_INTEGRATION") != "1" {
		t.Skip("requires opt-in Docker daemon with privileged DinD support")
	}
	for _, mode := range []string{"pass", "fail", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
			defer cancel()
			run := func(args ...string) string {
				t.Helper()
				output, err := exec.CommandContext(ctx, "docker", args...).Output()
				if err != nil {
					t.Fatalf("docker %v: %v\n%s", args, err, output)
				}
				return strings.TrimSpace(string(output))
			}
			name := "bellows-service-" + strings.ToLower(rand.Text())
			id := run("run", "--detach", "--privileged", "--name", name, "--label", "bellows.fixture="+name, "--env", "DOCKER_TLS_CERTDIR=", "--publish", "127.0.0.1::2375", "docker:28-dind")
			t.Cleanup(func() {
				output, err := exec.Command("docker", "rm", "--force", id).CombinedOutput()
				if err != nil {
					t.Errorf("owned daemon cleanup: %v %s", err, output)
				}
			})
			endpoint := "tcp://" + run("port", id, "2375/tcp")
			for {
				if err := exec.CommandContext(ctx, "docker", "--host", endpoint, "info").Run(); err == nil {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("dedicated daemon readiness timed out")
				case <-time.After(time.Second):
				}
			}
			for _, image := range []string{"docker:28-cli", "postgres:17-alpine"} {
				run("--host", endpoint, "pull", image)
			}
			cfg := config.DefaultConfig()
			cfg.Runner.Map = map[string]string{"ubuntu-latest": "docker:28-cli"}
			leases := t.TempDir()
			if err := os.Chmod(leases, 0700); err != nil {
				t.Fatal(err)
			}
			e, err := New(ctx, cfg, endpoint, leases)
			if err != nil {
				t.Fatal(err)
			}
			digest, err := e.Toolchain(ctx, []string{"docker:28-cli", "postgres:17-alpine"})
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			data, err := os.ReadFile("testdata/services.yml")
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(root, "services.yml"), data, 0600); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"init", "-b", "main"}, {"add", "."}, {"-c", "commit.gpgsign=false", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "fixture"}} {
				cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=" + filepath.Join(root, ".no-hooks")}, args...)...)
				cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
				cmd.Dir = root
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git: %v %s", err, output)
				}
			}
			var log bytes.Buffer
			req := v.Request{RunID: name, Worktree: root, Workflow: "./services.yml", Job: "service", Check: v.Check{ID: "service", Required: true}, Identity: v.Identity{Executor: v.ExecutorAct, Fingerprints: v.Fingerprints{Toolchain: digest}}, Env: []string{"BELLOWS_FIXTURE_MODE=" + mode}, Stdout: &log, Stderr: &log}
			runCtx := ctx
			var stop context.CancelFunc
			if mode == "cancel" {
				runCtx, stop = context.WithTimeout(ctx, 25*time.Second)
				defer stop()
			}
			result, runErr := e.Run(runCtx, req)
			expected := map[string]v.State{"pass": v.Passed, "fail": v.Failed, "cancel": v.Cancelled}[mode]
			if result.State != expected {
				t.Fatalf("result=%+v err=%v\n%s", result, runErr, &log)
			}
			if err = e.Close(); err != nil {
				t.Fatalf("cleanup: %v\n%s", err, &log)
			}
		})
	}
}
