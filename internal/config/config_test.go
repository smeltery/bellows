package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Version != 1 {
		t.Errorf("Version = %d, want 1", cfg.Version)
	}
	if cfg.Event != "pull_request" {
		t.Errorf("Event = %q, want pull_request", cfg.Event)
	}
	if got := cfg.Runner.Map["ubuntu-latest"]; got != "ghcr.io/smeltery/kiln:ubuntu-22.04" {
		t.Errorf("ubuntu-latest map = %q", got)
	}
	if _, ok := cfg.Runner.Map["blacksmith-4vcpu-ubuntu-2204"]; ok {
		t.Error("default config should stay minimal until Blacksmith labels are detected")
	}
	if got := cfg.Runner.Map["macos-latest"]; got != "ghcr.io/smeltery/kiln:ubuntu-22.04" {
		t.Errorf("macos-latest map = %q", got)
	}
}

func TestLoad_NoFile(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Event != "pull_request" {
		t.Errorf("Event = %q, want pull_request", cfg.Event)
	}
}

func TestSaveToAndLoadFrom(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".bellows.yml")
	cfg := DefaultConfig()
	cfg.Event = "push"
	cfg.DefaultWorkflows = []string{"ci.yml", ".github/workflows/release.yml"}
	cfg.Docker.Platform = "linux/amd64"
	cfg.Runner.Map["blacksmith-4vcpu-ubuntu-2204"] = "ghcr.io/smeltery/kiln:ubuntu-22.04"
	cfg.Secrets.Mode = "env"
	cfg.Secrets.Allow = []string{"GITHUB_TOKEN", "NPM_TOKEN"}
	cfg.Skip.Jobs = []string{"deploy"}
	cfg.Skip.StepsMatching = []string{"Configure AWS credentials"}

	if err := SaveTo(cfg, path); err != nil {
		t.Fatalf("SaveTo error: %v", err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("SaveTo did not create file")
	}
	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom error: %v", err)
	}
	if !reflect.DeepEqual(loaded, cfg) {
		t.Errorf("loaded config mismatch\n got: %#v\nwant: %#v", loaded, cfg)
	}
}

func TestLoadFrom_PartialConfigKeepsDefaults(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".bellows.yml")
	if err := os.WriteFile(path, []byte("event: workflow_dispatch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom error: %v", err)
	}
	if cfg.Event != "workflow_dispatch" {
		t.Errorf("Event = %q, want workflow_dispatch", cfg.Event)
	}
	if cfg.Version != 1 {
		t.Errorf("Version = %d, want default 1", cfg.Version)
	}
	if got := cfg.Runner.Map["ubuntu-latest"]; got != "ghcr.io/smeltery/kiln:ubuntu-22.04" {
		t.Errorf("ubuntu-latest map = %q, want default image", got)
	}
}

func TestMinimalAddsDetectedRunnerMappings(t *testing.T) {
	cfg := Minimal([]string{"ci.yml"}, []string{"blacksmith-4vcpu-ubuntu-2204", "macos-latest"}, "deploy")
	if !reflect.DeepEqual(cfg.DefaultWorkflows, []string{"ci.yml"}) {
		t.Fatalf("DefaultWorkflows = %#v", cfg.DefaultWorkflows)
	}
	if got := cfg.Runner.Map["blacksmith-4vcpu-ubuntu-2204"]; got != "ghcr.io/smeltery/kiln:ubuntu-22.04" {
		t.Errorf("blacksmith map = %q", got)
	}
	if got := cfg.Runner.Map["macos-latest"]; got != "ghcr.io/smeltery/kiln:ubuntu-22.04" {
		t.Errorf("macos map = %q", got)
	}
	if !reflect.DeepEqual(cfg.Skip.Jobs, []string{"deploy"}) {
		t.Errorf("Skip.Jobs = %#v", cfg.Skip.Jobs)
	}
}

func TestSaveTo_WritesExpectedConfigYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".bellows.yml")
	cfg := Minimal([]string{"ci.yml"}, []string{"ubuntu-latest"})
	if err := SaveTo(cfg, path); err != nil {
		t.Fatalf("SaveTo error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{
		"version: 1",
		"default_workflows:",
		"- ci.yml",
		"event: pull_request",
		"runner:",
		"ubuntu-latest: ghcr.io/smeltery/kiln:ubuntu-22.04",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("saved YAML missing %q:\n%s", want, out)
		}
	}
}

func TestMinimalConfigGolden(t *testing.T) {
	cfg := Minimal([]string{"ci.yml"}, []string{"ubuntu-latest", "blacksmith-4vcpu-ubuntu-2404"}, "deploy")
	data, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "minimal.golden.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(want) {
		t.Fatalf("minimal config mismatch\n got:\n%s\nwant:\n%s", string(data), string(want))
	}
}

func TestPatchSafeFixesPreservesComments(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".bellows.yml")
	original := "# team note\nversion: 1\nrunner:\n  map: {}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Runner.Map["blacksmith-4vcpu-ubuntu-2404"] = "ghcr.io/smeltery/kiln:ubuntu-24.04"
	cfg.Skip.Jobs = []string{"deploy"}
	if err := PatchSafeFixes(path, cfg); err != nil {
		t.Fatal(err)
	}
	out := readConfig(t, path)
	for _, want := range []string{
		"# team note",
		"blacksmith-4vcpu-ubuntu-2404: ghcr.io/smeltery/kiln:ubuntu-24.04",
		"- deploy",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("patched config missing %q:\n%s", want, out)
		}
	}
}

func readConfig(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLoadFrom_InvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{invalid yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFrom(path); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadFromRejectsUnknownKeys(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".bellows.yml")
	if err := os.WriteFile(path, []byte("version: 1\nunknown: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFrom(path); err == nil || !strings.Contains(err.Error(), "unsupported config key") {
		t.Fatalf("expected unknown key error, got %v", err)
	}
}

func TestLoadFromRequiresPolicyWaiverReason(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".bellows.yml")
	if err := os.WriteFile(path, []byte("version: 1\npolicy:\n  waivers:\n    - code: upload_artifact\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFrom(path); err == nil || !strings.Contains(err.Error(), "reason is required") {
		t.Fatalf("expected waiver reason error, got %v", err)
	}
}
