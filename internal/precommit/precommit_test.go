package precommit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCreatesDefaultPrePushHook(t *testing.T) {
	repo := t.TempDir()

	path, err := Write(repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(repo, ".pre-commit-config.yaml") {
		t.Fatalf("path = %q", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{
		"id: bellows",
		"name: Run GitHub Actions locally with Bellows",
		"entry: bellows run --precommit",
		"language: system",
		"pass_filenames: false",
		"stages: [pre-push]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("hook missing %q:\n%s", want, out)
		}
	}
}

func TestWriteIsNoopWhenBellowsHookAlreadyExists(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, ".pre-commit-config.yaml")
	existing := "repos:\n  - repo: local\n    hooks:\n      - id: bellows\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Write(repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}
	if data := readHook(t, path); data != existing {
		t.Fatalf("existing hook changed:\n%s", data)
	}
}

func TestWriteRefusesUnrelatedExistingConfigWithoutForce(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, ".pre-commit-config.yaml")
	if err := os.WriteFile(path, []byte("repos: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Write(repo, false); err == nil {
		t.Fatal("expected error for unrelated existing config")
	}
}

func TestWriteForceOverwritesExistingConfig(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, ".pre-commit-config.yaml")
	if err := os.WriteFile(path, []byte("repos: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Write(repo, true); err != nil {
		t.Fatal(err)
	}
	if data := readHook(t, path); !strings.Contains(data, "entry: bellows run --precommit") {
		t.Fatalf("force did not write Bellows hook:\n%s", data)
	}
}

func TestRemoveRefusesMixedHookConfig(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, ".pre-commit-config.yaml")
	mixed := Hook("pre-push") + "      - id: other\n        name: Other\n        entry: other\n        language: system\n"
	if err := os.WriteFile(path, []byte(mixed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, removed, err := Remove(repo); err != nil || removed {
		t.Fatalf("Remove removed mixed config: removed=%v err=%v", removed, err)
	}
	if data := readHook(t, path); data != mixed {
		t.Fatalf("mixed hook changed:\n%s", data)
	}
}

func readHook(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
