package precommit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const Hook = `repos:
  - repo: local
    hooks:
      - id: bellows
        name: Run GitHub Actions locally with Bellows
        entry: bellows run --precommit
        language: system
        pass_filenames: false
        stages: [pre-push]
`

func Write(repoRoot string, force bool) (string, error) {
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
	if err := os.WriteFile(path, []byte(Hook), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
