package compat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompatibilityDocsAreGeneratedFromCode(t *testing.T) {
	want := Markdown()
	got, err := os.ReadFile(filepath.Join("..", "..", "docs", "compatibility.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("docs/compatibility.md is out of date\n got:\n%s\nwant:\n%s", string(got), want)
	}
}
