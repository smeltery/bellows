package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	v "github.com/smeltery/bellows/internal/verification"
	verificationrun "github.com/smeltery/bellows/internal/verification/run"
)

func TestNativeCacheFlagsEndToEnd(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	write := func(name, data string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(".bellows.yml", "version: 1\nprofiles:\n  native:\n    executor: flox\n    checks:\n      test:\n        argv: [/bin/sh, check.sh]\n")
	write(".flox/env/manifest.lock", "{}")
	write("package.json", `{"private":true}`)
	write("check.sh", "set -eu\ntest -d \"$npm_config_cache\"\ntest \"$npm_config_logs_max\" = 0\nprintf 'cache=%s\\n' \"$npm_config_cache\"\n")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid"}, args...)...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, output)
		}
	}
	git("init", "-b", "main")
	git("add", ".")
	git("commit", "-m", "fixture")
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "flox"), []byte("#!/bin/sh\nif test \"$1\" = --version; then echo fixture; exit; fi\nshift 2\nexec \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	execute := func(flag string) (verificationrun.Outcome, string) {
		t.Helper()
		command := newRootCmd("test")
		var stdout, stderr bytes.Buffer
		command.SetOut(&stdout)
		command.SetErr(&stderr)
		command.SetArgs([]string{"--repo", root, "verify", "--repository", "example/project", "--base", "main", "--profile", "native", "--format", "json", "--artifacts", privateVerificationDir(t), flag})
		if err := command.Execute(); err != nil {
			t.Fatalf("verify: %v\n%s\n%s", err, &stderr, &stdout)
		}
		var out verificationrun.Outcome
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		match := regexp.MustCompile(`cache=([^\r\n]+)`).FindStringSubmatch(stderr.String())
		if len(match) != 2 || out.State != v.Passed {
			t.Fatalf("%+v %s", out, &stderr)
		}
		if strings.Contains(stdout.String(), match[1]) {
			t.Fatal("random operational cache path entered receipt")
		}
		return out, match[1]
	}
	first, path := execute("--download-cache")
	second, reused := execute("--download-cache")
	cold, disposable := execute("--cold-cache")
	if path != reused || path == disposable || first.Receipt.Identity != second.Receipt.Identity || first.Receipt.Identity != cold.Receipt.Identity {
		t.Fatal("cache reuse or normalized identity changed")
	}
	if _, err := os.Stat(disposable); !os.IsNotExist(err) {
		t.Fatal("cold store retained")
	}
	write("source.txt", "changed source")
	git("add", ".")
	git("commit", "-m", "source change")
	changed, sameStore := execute("--download-cache")
	if sameStore != path || changed.Receipt.Identity.Tree == first.Receipt.Identity.Tree {
		t.Fatal("source change lost full identity or invalidated downloads")
	}
}

type cacheDaemon struct {
	v.Executor
	id string
}

func (d cacheDaemon) DaemonID() string { return d.id }
func TestDaemonCacheBindingAndColdRejection(t *testing.T) {
	root := privateVerificationDir(t)
	plan := verificationrun.Plan{Identity: v.Identity{Repository: "owner/project", Executor: v.ExecutorAct}, Executor: cacheDaemon{id: "daemon"}}
	if _, err := applyVerificationCache("", root, false, false, plan); err != nil {
		t.Fatal(err)
	}
	plan.Identity.Repository = "other/project"
	if _, err := applyVerificationCache("", root, false, false, plan); err == nil {
		t.Fatal("cross-repo Docker reuse")
	}
	if _, err := applyVerificationCache("", root, false, true, plan); err == nil {
		t.Fatal("claimed cold existing daemon")
	}
}
