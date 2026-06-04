package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestJSONSchemasAreValidJSON(t *testing.T) {
	root := filepath.Join("..", "..", "docs", "schemas")
	for _, name := range []string{"bellows.schema.json", "doctor.schema.json", "explain.schema.json"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(data, &schema); err != nil {
			t.Fatalf("%s is invalid JSON: %v", name, err)
		}
		if schema["$schema"] == "" || schema["title"] == "" {
			t.Fatalf("%s missing schema metadata", name)
		}
	}
}

func TestEmbeddedSchemasMatchDocs(t *testing.T) {
	docsRoot := filepath.Join("..", "..", "docs", "schemas")
	internalRoot := filepath.Join("..", "schemas")
	for _, name := range []string{"bellows.schema.json", "doctor.schema.json", "explain.schema.json"} {
		docs, err := os.ReadFile(filepath.Join(docsRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		embedded, err := os.ReadFile(filepath.Join(internalRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(docs) != string(embedded) {
			t.Fatalf("%s differs between docs and internal schemas", name)
		}
	}
}
