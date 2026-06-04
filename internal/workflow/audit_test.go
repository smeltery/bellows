package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowsAvoidDeprecatedActionMajors(t *testing.T) {
	root := filepath.Join("..", "..", ".github", "workflows")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	blocked := []string{
		"actions/upload-artifact@v5",
		"golangci/golangci-lint-action@v8",
		"goreleaser/goreleaser-action@v6",
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range blocked {
			if strings.Contains(string(data), token) {
				t.Fatalf("%s uses deprecated action %s", path, token)
			}
		}
	}
}
