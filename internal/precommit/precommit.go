package precommit

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Status struct {
	Path        string `json:"path"`
	Exists      bool   `json:"exists"`
	Installed   bool   `json:"installed"`
	BellowsOnly bool   `json:"bellows_only"`
	Stage       string `json:"stage,omitempty"`
}

func Hook(stage string) string {
	if stage == "" {
		stage = "pre-push"
	}
	return fmt.Sprintf(`repos:
  - repo: local
    hooks:
      - id: bellows
        name: Run GitHub Actions locally with Bellows
        entry: bellows run --precommit
        language: system
        pass_filenames: false
        stages: [%s]
`, stage)
}

func Write(repoRoot string, force bool) (string, error) {
	return WriteStage(repoRoot, "pre-push", force)
}

func WriteStage(repoRoot, stage string, force bool) (string, error) {
	path := filepath.Join(repoRoot, ".pre-commit-config.yaml")
	existing, err := os.ReadFile(path)
	if err == nil {
		if strings.Contains(string(existing), "id: bellows") && !force {
			return path, nil
		}
		if len(existing) > 0 && !force {
			return "", fmt.Errorf("%s already exists; add the bellows hook manually or rerun with --force-pre-commit", path)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.WriteFile(path, []byte(Hook(stage)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func Remove(repoRoot string) (string, bool, error) {
	path := filepath.Join(repoRoot, ".pre-commit-config.yaml")
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return path, false, nil
	}
	if err != nil {
		return "", false, err
	}
	text := string(existing)
	if !strings.Contains(text, "id: bellows") {
		return path, false, nil
	}
	if strings.Count(text, "- id:") != 1 || !strings.Contains(text, "entry: bellows run --precommit") {
		return path, false, nil
	}
	if err := os.Remove(path); err != nil {
		return "", false, err
	}
	return path, true, nil
}

func Inspect(repoRoot string) (Status, error) {
	path := filepath.Join(repoRoot, ".pre-commit-config.yaml")
	status := Status{Path: path}
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	text := string(existing)
	status.Exists = true
	status.Installed = strings.Contains(text, "id: bellows") && strings.Contains(text, "entry: bellows run --precommit")
	status.BellowsOnly = status.Installed && strings.Count(text, "- id:") == 1
	status.Stage = stage(text)
	return status, nil
}

func stage(text string) string {
	const prefix = "stages: ["
	start := strings.Index(text, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	end := strings.Index(text[start:], "]")
	if end < 0 {
		return ""
	}
	value := strings.TrimSpace(text[start : start+end])
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return strings.Trim(value, `"'`)
}
