package workflow

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDiscoverParsesWorkflows(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := `name: CI
on: [pull_request]
jobs:
  test:
    runs-on: [ubuntu-latest, blacksmith-4vcpu-ubuntu-2204]
    needs: build
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Test
        run: go test ./...
`
	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	workflows, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(workflows) != 1 {
		t.Fatalf("workflow count = %d", len(workflows))
	}
	if workflows[0].Path != ".github/workflows/ci.yml" {
		t.Fatalf("Path = %q", workflows[0].Path)
	}
	job := workflows[0].Jobs["test"]
	if !reflect.DeepEqual(job.RunsOn, []string{"ubuntu-latest", "blacksmith-4vcpu-ubuntu-2204"}) {
		t.Fatalf("RunsOn = %#v", job.RunsOn)
	}
	if !reflect.DeepEqual(Labels(workflows), []string{"blacksmith-4vcpu-ubuntu-2204", "ubuntu-latest"}) {
		t.Fatalf("Labels = %#v", Labels(workflows))
	}
}

func TestDiscoverExpandsMatrixRunnerLabelsAndCloudJobs(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := `name: Release
jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
    runs-on: ${{ matrix.os }}
    permissions:
      id-token: write
    environment:
      name: production
    steps:
      - uses: actions/upload-artifact@v4
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: ./deploy.sh
`
	if err := os.WriteFile(filepath.Join(dir, "release.yml"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	workflows, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	labels := Labels(workflows)
	if !reflect.DeepEqual(labels, []string{"macos-latest", "ubuntu-latest"}) {
		t.Fatalf("Labels = %#v", labels)
	}
	if !reflect.DeepEqual(CloudLikeJobIDs(workflows), []string{"deploy"}) {
		t.Fatalf("CloudLikeJobIDs = %#v", CloudLikeJobIDs(workflows))
	}
	job := workflows[0].Jobs["test"]
	if !reflect.DeepEqual(job.Matrix["os"], []string{"ubuntu-latest", "macos-latest"}) {
		t.Fatalf("Matrix = %#v", job.Matrix)
	}
	if got := job.Permissions["id-token"]; got != "write" {
		t.Fatalf("Permissions id-token = %q", got)
	}
	if !reflect.DeepEqual(job.Environment, []string{"production"}) {
		t.Fatalf("Environment = %#v", job.Environment)
	}
}

func TestResolve(t *testing.T) {
	root := "/repo"
	tests := map[string]string{
		"ci.yml":                   "/repo/.github/workflows/ci.yml",
		".github/workflows/ci.yml": "/repo/.github/workflows/ci.yml",
	}
	for input, want := range tests {
		if got := Resolve(root, input); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLoadRealisticWorkflowFacts(t *testing.T) {
	wf, err := Load(filepath.Join("testdata", "realistic.yml"))
	if err != nil {
		t.Fatal(err)
	}
	testJob := wf.Jobs["test"]
	if got := strings.Join(testJob.MatrixRunnerLabels(), ","); got != "ubuntu-latest,blacksmith-4vcpu-ubuntu-2404" {
		t.Fatalf("matrix runner labels = %q", got)
	}
	deploy := wf.Jobs["deploy"]
	if !deploy.UsesOIDC() || !deploy.UsesDeploymentEnvironment() {
		t.Fatalf("deploy facts missing: %#v", deploy)
	}
	var sawCache, sawRelease bool
	for _, use := range wf.ActionUses() {
		if use.Step.UsesAction("actions/cache") {
			sawCache = true
		}
		if use.Step.UsesAction("goreleaser/goreleaser-action") {
			sawRelease = true
		}
	}
	if !sawCache || !sawRelease {
		t.Fatalf("action facts missing cache=%t release=%t", sawCache, sawRelease)
	}
}
