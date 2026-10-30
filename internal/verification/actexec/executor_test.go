package actexec

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smeltery/bellows/internal/config"
	v "github.com/smeltery/bellows/internal/verification"
)

func fake(t *testing.T) (*Executor, v.Request, string) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(data), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "image"), "sha256:original")
	write(filepath.Join(bin, "docker"), `#!/bin/sh
shift 2
case "$1 $2" in
 'info --format') echo 'daemon-id linux';;
 'version --format') echo '28.0 28.0';;
 'image inspect') cat '`+root+`/image';;
 'volume create') printf '%s' "${4#*=}" > '`+root+`/owner';;
 'volume inspect') cat '`+root+`/owner';;
 'volume ls') if test -f '`+root+`/owner'; then echo act-toolcache; fi;;
 'volume rm') rm '`+root+`/owner';;
 'container ls') if test -f '`+root+`/dirty'; then echo owned-container; fi;;
esac
`)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	write(filepath.Join(bin, "act"), `#!/bin/sh
if test "$1" = --version; then echo 'act version 0.2.88'; exit; fi
printf '%s\n' "$@" > '`+root+`/args'
while test "$#" -gt 0; do
 if test "$1" = --job; then job="$2"; fi
 if test "$1" = --env-file; then cat "$2" > '`+root+`/environment'; fi
 shift
done
case "$job" in
 fail) exit 7;;
 cancel) trap '' INT; exec `+quote(executable)+` '-test.run=^TestActCancellationProcess$' -- `+quote(filepath.Join(root, "cancel-ready"))+`;;
 foreign) echo foreign > '`+root+`/owner';;
 dirty) touch '`+root+`/dirty';;
esac
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cfg := config.DefaultConfig()
	cfg.Runner.Map = map[string]string{"ubuntu-latest": "runner:tag"}
	leases := filepath.Join(root, "leases")
	if err := os.Mkdir(leases, 0700); err != nil {
		t.Fatal(err)
	}
	e, err := New(context.Background(), cfg, "tcp://dedicated:2375", leases)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := e.Toolchain(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	workflow := "on: pull_request\njobs:\n"
	for _, name := range []string{"pass", "fail", "cancel", "dirty", "foreign"} {
		workflow += "  " + name + ":\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        os: [linux]\n        version: ['a:b']\n    steps:\n      - run: echo synthetic\n"
	}
	write(filepath.Join(root, "ci.yml"), workflow)
	req := v.Request{RunID: "run", Worktree: root, Workflow: "./ci.yml", Job: "pass", Matrix: map[string]string{"os": "linux", "version": "a:b"}, Check: v.Check{ID: "pass", Required: true}, Identity: v.Identity{Executor: v.ExecutorAct, Fingerprints: v.Fingerprints{Toolchain: digest}}}
	t.Cleanup(func() {
		_ = os.Remove(filepath.Join(root, "dirty"))
		_ = os.Remove(filepath.Join(root, "owner"))
		_ = e.Close()
	})
	return e, req, root
}
func TestDedicatedRunLifecycle(t *testing.T) {
	e, req, root := fake(t)
	digest, err := e.Toolchain(context.Background(), nil)
	if err != nil || digest != req.Identity.Fingerprints.Toolchain {
		t.Fatal("toolchain identity changed across reads", err)
	}
	if _, err := New(context.Background(), e.cfg, "tcp://alias:2375", filepath.Dir(e.lease)); err == nil {
		t.Fatal("daemon alias bypassed lease")
	}
	req.Matrix = map[string]string{"version": "a:b", "os": "linux"}
	req.Env = []string{"NORMAL=hello world", "SECRET_INPUT=hidden-value"}
	e.cfg.Secrets.Allow = []string{"SECRET_INPUT"}
	result, err := e.Run(context.Background(), req)
	if err != nil || result.State != v.Passed {
		t.Fatalf("%+v %v", result, err)
	}
	args, err := os.ReadFile(filepath.Join(root, "args"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--matrix\nos:linux\n--matrix\nversion:a:b") || !strings.Contains(string(args), "sha256:original") || !strings.Contains(string(args), "--pull=false") {
		t.Fatal(string(args))
	}
	environment, err := os.ReadFile(filepath.Join(root, "environment"))
	if err != nil || string(environment) != `{"NORMAL":"hello world"}` || strings.Contains(string(args), "hidden-value") {
		t.Fatal("environment or secret forwarding incorrect", string(environment), err)
	}
	if _, err := os.Stat(filepath.Join(e.cache, "environment.yml")); !os.IsNotExist(err) {
		t.Fatal("environment file retained")
	}
	req.Job = "fail"
	result, err = e.Run(context.Background(), req)
	if err != nil || result.State != v.Failed || *result.ExitCode != 7 {
		t.Fatalf("%+v %v", result, err)
	}
	req.Job = "cancel"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type outcome struct {
		result v.Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() { result, err := e.Run(ctx, req); done <- outcome{result, err} }()
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "cancel-ready")); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("act did not start")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case out := <-done:
		if !errors.Is(out.err, context.Canceled) || out.result.State != v.Cancelled {
			t.Fatalf("%+v %v", out.result, out.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("act did not cancel")
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.lease); !os.IsNotExist(err) {
		t.Fatal("lease not released")
	}
}
func TestRefuseUnsafeInputsAndRetainDirtyLease(t *testing.T) {
	e, req, root := fake(t)
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "PATH", "HOME", "DOCKER_HOST", "ACT_OPTIONS", "BASH_ENV"} {
		bad := req
		bad.Env = []string{name + "=secret"}
		if result, err := e.Run(context.Background(), bad); err == nil || result.State == v.Passed {
			t.Fatal(name)
		}
	}
	bad := req
	bad.Identity.Fingerprints.Toolchain = "changed"
	if _, err := e.Run(context.Background(), bad); err == nil {
		t.Fatal("stale toolchain accepted")
	}
	bad = req
	bad.Job = "missing"
	if result, err := e.Run(context.Background(), bad); !errors.Is(err, ErrUnsupported) || result.State != v.Unsupported {
		t.Fatalf("%+v %v", result, err)
	}
	if err := os.WriteFile(filepath.Join(root, "image"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Run(context.Background(), req); err == nil {
		t.Fatal("image mutation accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "image"), []byte("sha256:original"), 0600); err != nil {
		t.Fatal(err)
	}
	req.Job = "dirty"
	if _, err := e.Run(context.Background(), req); err == nil {
		t.Fatal("leftover accepted")
	}
	if err := e.Close(); err == nil {
		t.Fatal("dirty daemon lease released")
	}
	if _, err := os.Stat(e.lease); err != nil {
		t.Fatal(err)
	}
}
func TestMissingDedicatedDaemon(t *testing.T) {
	if _, err := New(context.Background(), config.DefaultConfig(), "", t.TempDir()); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestNeverRemoveForeignVolume(t *testing.T) {
	e, req, root := fake(t)
	req.Job = "foreign"
	if _, err := e.Run(context.Background(), req); err == nil {
		t.Fatal("foreign ownership accepted")
	}
	owner, err := os.ReadFile(filepath.Join(root, "owner"))
	if err != nil || strings.TrimSpace(string(owner)) != "foreign" {
		t.Fatal("foreign volume removed")
	}
	if err = e.Close(); err == nil {
		t.Fatal("dirty daemon lease released")
	}
}

func TestToolchainFreshnessAfterClose(t *testing.T) {
	e, req, root := fake(t)
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	digest, err := e.Toolchain(context.Background(), nil)
	if err != nil || digest != req.Identity.Fingerprints.Toolchain {
		t.Fatal("closed freshness read changed", err)
	}
	if err = os.WriteFile(filepath.Join(root, "image"), []byte("sha256:new"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := e.Toolchain(context.Background(), nil)
	if err != nil || changed == digest {
		t.Fatal("closed freshness read missed changed image", err)
	}
	if result, err := e.Run(context.Background(), req); err == nil || result.State == v.Passed {
		t.Fatal("closed executor ran")
	}
}
func TestUnsupportedServiceContext(t *testing.T) {
	e, req, root := fake(t)
	file, err := os.OpenFile(filepath.Join(root, "ci.yml"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString("# ${{ job.services.postgres.ports[5432] }}\n"); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if result, err := e.Run(context.Background(), req); !errors.Is(err, ErrUnsupported) || result.State != v.Unsupported {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestCancellationBeforeActStarts(t *testing.T) {
	e, req, _ := fake(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := e.Run(ctx, req)
	if result.State != v.Cancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("%+v %v", result, err)
	}
}

// A Go handler mirrors act: unlike a POSIX shell trap it re-enables inherited
// ignored SIGINT (as observed when tests run from Git hooks).
func TestActCancellationProcess(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	if !signal.Ignored(os.Interrupt) {
		t.Fatal("cancellation fixture must inherit ignored SIGINT")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := os.WriteFile(os.Args[len(os.Args)-1], nil, 0600); err != nil {
		t.Fatal(err)
	}
	<-ctx.Done()
	os.Exit(130)
}
