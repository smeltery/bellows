package selftest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunReportsConfigAndWorkflowDiscovery(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".bellows.yml"), []byte("version: 1\ndefault_workflows: [ci.yml]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte("name: CI\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: go test ./...\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checks := Run(repo, "test")
	if !hasCheck(checks, "config") || !hasCheck(checks, "workflow_discovery") || !hasCheck(checks, "plan") {
		t.Fatalf("checks missing expected repository diagnostics: %#v", checks)
	}
}

func hasCheck(checks []Check, code string) bool {
	for _, check := range checks {
		if check.Code == code {
			return true
		}
	}
	return false
}
