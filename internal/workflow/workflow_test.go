package workflow

import (
	"os"
	"path/filepath"
	"reflect"
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
