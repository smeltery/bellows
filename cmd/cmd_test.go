package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotbrains/bellows/internal/plan"
	"github.com/dotbrains/bellows/internal/precommit"
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
	for _, want := range []string{"init", "run", "doctor", "cache", "config", "policy", "compat", "hooks", "migrate", "explain"} {
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

func TestInitDryRunPrintsConfigWithoutWriting(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "blacksmith-16vcpu-ubuntu-2404")

	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "init", "--dry-run"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "blacksmith-16vcpu-ubuntu-2404: catthehacker/ubuntu:act-24.04") {
		t.Fatalf("dry-run output missing Blacksmith mapping:\n%s", buf.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".bellows.yml")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote .bellows.yml, stat err=%v", err)
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

func TestDoctorStrictFailsOnWarnings(t *testing.T) {
	repo := t.TempDir()
	writeWorkflowBody(t, repo, "ci.yml", `name: CI
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/upload-artifact@v5
`)

	root := newRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--repo", repo, "doctor", "--strict"})

	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "warnings") {
		t.Fatalf("expected strict warning failure, got %v", err)
	}
}

func TestDoctorFixPersistsSafeConfigChanges(t *testing.T) {
	repo := t.TempDir()
	writeWorkflowBody(t, repo, "ci.yml", `name: CI
jobs:
  test:
    runs-on: blacksmith-16vcpu-ubuntu-2404
    steps:
      - run: go test ./...
  publish:
    runs-on: ubuntu-latest
    steps:
      - run: ./publish.sh
`)
	if err := os.WriteFile(filepath.Join(repo, ".bellows.yml"), []byte("# keep me\nversion: 1\nrunner:\n  map: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "doctor", "--fix"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	configYAML := readFile(t, filepath.Join(repo, ".bellows.yml"))
	for _, want := range []string{
		"# keep me",
		"blacksmith-16vcpu-ubuntu-2404: catthehacker/ubuntu:act-24.04",
		"- publish",
	} {
		if !strings.Contains(configYAML, want) {
			t.Fatalf("fixed config missing %q:\n%s", want, configYAML)
		}
	}
}

func TestHooksInstallAndUninstall(t *testing.T) {
	repo := t.TempDir()
	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "hooks", "install", "--stage", "pre-commit"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	hook := readFile(t, filepath.Join(repo, ".pre-commit-config.yaml"))
	if !strings.Contains(hook, "stages: [pre-commit]") {
		t.Fatalf("hook stage mismatch:\n%s", hook)
	}

	root = newRootCmd("test")
	buf.Reset()
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "hooks", "uninstall"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".pre-commit-config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("hook was not removed, stat err=%v", err)
	}
}

func TestHooksStatusJSON(t *testing.T) {
	repo := t.TempDir()
	if _, err := precommit.WriteStage(repo, "pre-push", false); err != nil {
		t.Fatal(err)
	}
	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "hooks", "status", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var status struct {
		Installed   bool   `json:"installed"`
		BellowsOnly bool   `json:"bellows_only"`
		Stage       string `json:"stage"`
	}
	if err := json.Unmarshal(buf.Bytes(), &status); err != nil {
		t.Fatalf("invalid status JSON: %v\n%s", err, buf.String())
	}
	if !status.Installed || !status.BellowsOnly || status.Stage != "pre-push" {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestDoctorFixDryRunDoesNotWriteConfig(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "blacksmith-16vcpu-ubuntu-2404")
	original := "version: 1\nrunner:\n  map: {}\n"
	if err := os.WriteFile(filepath.Join(repo, ".bellows.yml"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "doctor", "--fix", "--fix-dry-run"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "[fix] mapped runner blacksmith-16vcpu-ubuntu-2404") {
		t.Fatalf("dry-run output missing fix preview:\n%s", buf.String())
	}
	if got := readFile(t, filepath.Join(repo, ".bellows.yml")); got != original {
		t.Fatalf("fix dry-run changed config:\n%s", got)
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

func TestDoctorCommandSupportsGitHubAnnotations(t *testing.T) {
	repo := t.TempDir()
	writeWorkflowBody(t, repo, "ci.yml", `name: CI
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/upload-artifact@v5
`)

	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "doctor", "--format", "github"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "::warning file=.github/workflows/ci.yml,title=upload_artifact::") {
		t.Fatalf("missing GitHub annotation:\n%s", buf.String())
	}
}

func TestRunListPrintsExecutionPlan(t *testing.T) {
	repo := t.TempDir()
	writeWorkflowBody(t, repo, "ci.yml", `name: CI
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: ./deploy.sh
`)
	if err := os.WriteFile(filepath.Join(repo, ".bellows.yml"), []byte("version: 1\ndefault_workflows:\n  - ci.yml\nskip:\n  jobs:\n    - deploy\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "run", "--list"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "[run] test") || !strings.Contains(out, "[skip] deploy") {
		t.Fatalf("run list output missing jobs:\n%s", out)
	}
}

func TestRunListEstimatePrintsSummary(t *testing.T) {
	repo := t.TempDir()
	writeWorkflowBody(t, repo, "ci.yml", `name: CI
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: ./deploy.sh
`)
	if err := os.WriteFile(filepath.Join(repo, ".bellows.yml"), []byte("version: 1\ndefault_workflows:\n  - ci.yml\nskip:\n  jobs:\n    - deploy\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "run", "--list", "--estimate"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "estimate: 1 runnable jobs, 1 skipped jobs, 1 avoided cloud jobs") {
		t.Fatalf("missing estimate:\n%s", buf.String())
	}
}

func TestRunPrecommitChangedFilesListsOnlyMatchedJob(t *testing.T) {
	repo := t.TempDir()
	writeWorkflowBody(t, repo, "ci.yml", `name: CI
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
  lint:
    runs-on: ubuntu-latest
    steps:
      - run: gofmt -w .
`)
	configYAML := `version: 1
default_workflows:
  - ci.yml
precommit:
  mode: changed-files
  paths:
    - pattern: "*.go"
      workflows: [ci.yml]
      jobs: [test]
`
	if err := os.WriteFile(filepath.Join(repo, ".bellows.yml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init")
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "run", "--precommit", "--list"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "[run] test") || strings.Contains(out, "[run] lint") {
		t.Fatalf("changed-files plan mismatch:\n%s", out)
	}
}

func TestExplainSupportsJSON(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "ubuntu-latest")
	if err := os.WriteFile(filepath.Join(repo, ".bellows.yml"), []byte("version: 1\ndefault_workflows:\n  - ci.yml\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "explain", "--format", "json"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Event     string `json:"event"`
		Workflows []struct {
			Path string `json:"path"`
			Jobs []struct {
				ID string `json:"id"`
			} `json:"jobs"`
		} `json:"workflows"`
	}
	if err := json.Unmarshal(buf.Bytes(), &plan); err != nil {
		t.Fatalf("invalid explain JSON: %v\n%s", err, buf.String())
	}
	if plan.Event != "pull_request" || len(plan.Workflows) != 1 || plan.Workflows[0].Jobs[0].ID != "test" {
		t.Fatalf("unexpected explain plan: %#v", plan)
	}
}

func TestExplainChangedFiles(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "ubuntu-latest")
	if err := os.WriteFile(filepath.Join(repo, ".bellows.yml"), []byte(`version: 1
default_workflows:
  - ci.yml
precommit:
  mode: changed-files
  paths:
    - pattern: "*.go"
      jobs: [test]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init")
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "explain", "changed-files"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "main.go") || !strings.Contains(buf.String(), "jobs: test") {
		t.Fatalf("changed file explanation mismatch:\n%s", buf.String())
	}
}

func TestConfigCommands(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".bellows.yml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "config", "validate"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "config ok") {
		t.Fatalf("unexpected validate output: %s", buf.String())
	}

	root = newRootCmd("test")
	buf.Reset()
	root.SetOut(buf)
	root.SetArgs([]string{"config", "print-schema", "doctor"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Bellows doctor findings") {
		t.Fatalf("unexpected schema output: %s", buf.String())
	}
}

func TestPolicyCommandReportsViolations(t *testing.T) {
	repo := t.TempDir()
	writeWorkflowBody(t, repo, "ci.yml", `name: CI
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: ./deploy.sh
`)

	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "policy", "--format", "github"})

	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "policy violations") {
		t.Fatalf("expected policy failure, got %v", err)
	}
	if !strings.Contains(buf.String(), "::error file=.github/workflows/ci.yml,title=cloud_job_not_skipped::") {
		t.Fatalf("missing policy annotation:\n%s", buf.String())
	}
}

func TestCompatCommandReportsUnsupportedFeatures(t *testing.T) {
	repo := t.TempDir()
	writeWorkflowBody(t, repo, "release.yml", `name: Release
jobs:
  release:
    runs-on: ubuntu-latest
    permissions:
      id-token: write
    steps:
      - uses: goreleaser/goreleaser-action@v6
`)
	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "compat"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "unsupported") || !strings.Contains(buf.String(), "release-publish") {
		t.Fatalf("compat output mismatch:\n%s", buf.String())
	}
}

func TestMigrateGitHubActionsPrintsRecommendedConfig(t *testing.T) {
	repo := t.TempDir()
	writeWorkflowBody(t, repo, "ci.yml", `name: CI
jobs:
  test:
    runs-on: blacksmith-4vcpu-ubuntu-2404
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
	root.SetArgs([]string{"--repo", repo, "migrate", "github-actions"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"migration plan", "blacksmith-4vcpu-ubuntu-2404", "- deploy", "bellows hooks install --stage pre-push"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("migration output missing %q:\n%s", want, buf.String())
		}
	}
}

func TestSampleRepoFixtureExplainsMigrationPlan(t *testing.T) {
	repo := filepath.Join("testdata", "sample-repo")

	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--repo", repo, "explain", "--format", "json"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var executionPlan plan.Plan
	if err := json.Unmarshal(buf.Bytes(), &executionPlan); err != nil {
		t.Fatalf("invalid fixture plan JSON: %v\n%s", err, buf.String())
	}
	if len(executionPlan.Workflows) != 1 {
		t.Fatalf("workflow count = %d, want 1", len(executionPlan.Workflows))
	}
	var sawBlacksmith bool
	var sawSkippedPublish bool
	for _, job := range executionPlan.Workflows[0].Jobs {
		if job.ID == "test" && strings.Join(job.RunnerImages, ",") == "ubuntu-latest=catthehacker/ubuntu:act-22.04,blacksmith-4vcpu-ubuntu-2404=catthehacker/ubuntu:act-24.04" {
			sawBlacksmith = true
		}
		if job.ID == "publish" && job.Skipped {
			sawSkippedPublish = true
		}
	}
	if !sawBlacksmith || !sawSkippedPublish {
		t.Fatalf("fixture plan missing expected migration details: %#v", executionPlan)
	}
}

func TestExamplesSmoke(t *testing.T) {
	root := filepath.Join("..", "examples")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		repo := filepath.Join(root, entry.Name())
		t.Run(entry.Name(), func(t *testing.T) {
			for _, args := range [][]string{
				{"config", "validate"},
				{"policy"},
				{"run", "--list"},
			} {
				cmd := newRootCmd("test")
				cmd.SetOut(&bytes.Buffer{})
				cmd.SetArgs(append([]string{"--repo", repo}, args...))
				if err := cmd.Execute(); err != nil {
					t.Fatalf("bellows %s failed: %v", strings.Join(args, " "), err)
				}
			}
		})
	}
}

func TestCompletionCommandGeneratesZsh(t *testing.T) {
	root := newRootCmd("test")
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"completion", "zsh"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "#compdef bellows") {
		t.Fatalf("unexpected completion output:\n%s", buf.String())
	}
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

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
