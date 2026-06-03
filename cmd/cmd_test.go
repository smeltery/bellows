package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteVersion(t *testing.T) {
	orig := os.Args
	t.Cleanup(func() { os.Args = orig })
	os.Args = []string{"bellows", "--version"}

	if err := Execute("0.0.1-test"); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
}

func TestNewRootCmdRegistersMVPCommands(t *testing.T) {
	root := newRootCmd("0.1.0")
	if root.Use != "bellows" {
		t.Errorf("Use = %q", root.Use)
	}

	cmds := make(map[string]bool)
	for _, c := range root.Commands() {
		cmds[c.Name()] = true
	}
	for _, want := range []string{"init", "run", "doctor", "cache", "explain"} {
		if !cmds[want] {
			t.Errorf("missing subcommand %q", want)
		}
	}
}

func TestInitDiscoversWorkflowsAndWritesConfigs(t *testing.T) {
	repo := t.TempDir()
	writeWorkflowBody(t, repo, "ci.yml", `name: CI
jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, blacksmith-4vcpu-ubuntu-2204]
    runs-on: ${{ matrix.os }}
    steps:
      - run: go test ./...
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: ./deploy.sh
`)

	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "init"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	configYAML := readFile(t, filepath.Join(repo, ".bellows.yml"))
	for _, want := range []string{
		"default_workflows:",
		"- ci.yml",
		"blacksmith-4vcpu-ubuntu-2204: catthehacker/ubuntu:act-22.04",
		"ubuntu-latest: catthehacker/ubuntu:act-22.04",
		"- deploy",
	} {
		if !strings.Contains(configYAML, want) {
			t.Errorf(".bellows.yml missing %q:\n%s", want, configYAML)
		}
	}

	hook := readFile(t, filepath.Join(repo, ".pre-commit-config.yaml"))
	if !strings.Contains(hook, "entry: bellows run --precommit") || !strings.Contains(hook, "stages: [pre-push]") {
		t.Errorf("pre-commit hook should run bellows at pre-push:\n%s", hook)
	}
	if !strings.Contains(buf.String(), "wrote .bellows.yml") {
		t.Errorf("expected init output to mention .bellows.yml, got %q", buf.String())
	}
}

func TestInitRefusesExistingConfig(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "ubuntu-latest")
	if err := os.WriteFile(filepath.Join(repo, ".bellows.yml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--repo", repo, "init"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error when .bellows.yml exists")
	}
}

func TestDoctorCommandReportsBlacksmithMapping(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "blacksmith-4vcpu-ubuntu-2204")

	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "doctor"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Blacksmith runner blacksmith-4vcpu-ubuntu-2204 maps to catthehacker/ubuntu:act-22.04") {
		t.Errorf("doctor output missing Blacksmith mapping:\n%s", buf.String())
	}
}

func TestDoctorCommandSupportsJSON(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "blacksmith-4vcpu-ubuntu-2204")

	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "doctor", "--format", "json"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var findings []struct {
		Level   string `json:"level"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(buf.Bytes(), &findings); err != nil {
		t.Fatalf("invalid doctor JSON: %v\n%s", err, buf.String())
	}
	for _, finding := range findings {
		if finding.Code == "blacksmith_runner_mapped" && strings.Contains(finding.Message, "blacksmith-4vcpu-ubuntu-2204") {
			return
		}
	}
	t.Fatalf("missing Blacksmith JSON finding: %#v", findings)
}

func writeWorkflow(t *testing.T, repo, name, runner string) {
	t.Helper()
	body := "name: CI\njobs:\n  test:\n    runs-on: " + runner + "\n    steps:\n      - run: go test ./...\n"
	writeWorkflowBody(t, repo, name, body)
}

func writeWorkflowBody(t *testing.T, repo, name, body string) {
	t.Helper()
	dir := filepath.Join(repo, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
