package changed

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/smeltery/bellows/internal/config"
	"github.com/smeltery/bellows/internal/verification"
)

func requiredTargets() []Target {
	return []Target{
		{Check: verification.Check{ID: "test", Required: true, Needs: []string{"build"}}, Workflow: "ci.yml", Job: "test"},
		{Check: verification.Check{ID: "build", Required: true, Needs: []string{"setup"}}, Workflow: "ci.yml", Job: "build"},
		{Check: verification.Check{ID: "setup", Required: true}, Workflow: "ci.yml", Job: "setup", Skipped: true},
		{Check: verification.Check{ID: "docs", Required: true}, Workflow: "docs.yml", Job: "docs"},
	}
}
func committed(paths ...string) Changes {
	return Changes{Head: "head", Base: "base", MergeBase: "base", Files: paths}
}
func TestRequiredSelection(t *testing.T) {
	opts := RequiredOptions{Rules: []config.PathRule{{Pattern: "src/**", Jobs: []string{"test"}}, {Pattern: "docs/**", Workflows: []string{"docs.yml"}}}, SharedPaths: []string{"shared/**"}}
	for _, tc := range []struct {
		name  string
		paths []string
		count int
	}{
		{"transitive", []string{"src/a.go"}, 3}, {"explicit docs", []string{"docs/guide.md"}, 1},
		{"unknown", []string{"new/file"}, 4}, {"empty", nil, 4}, {"workflow", []string{".github/workflows/ci.yml"}, 4},
		{"lock", []string{"src/bun.lock"}, 4}, {"toolchain", []string{"src/go.mod"}, 4}, {"shared", []string{"shared/input"}, 4},
		{"rename endpoints", []string{"docs/old.md", "new/file"}, 4}, {"deleted docs", []string{"docs/deleted.md"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SelectRequired(committed(tc.paths...), requiredTargets(), opts)
			if err != nil || len(got.Targets) != tc.count {
				t.Fatalf("%+v %v", got, err)
			}
			for _, target := range got.Targets {
				if len(got.Reasons[target.Check.ID]) == 0 {
					t.Fatal("missing reason")
				}
			}
			if tc.name == "transitive" && (!got.Targets[0].Skipped || !got.Targets[0].Check.Required) {
				t.Fatal("skipped prerequisite hidden")
			}
		})
	}
	a, _ := SelectRequired(committed("src/a", "docs/b", "unknown"), requiredTargets(), opts)
	targets := requiredTargets()
	targets[0], targets[3] = targets[3], targets[0]
	b, _ := SelectRequired(committed("unknown", "docs/b", "src/a"), targets, opts)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("nondeterministic: %+v %+v", a, b)
	}
}
func TestRequiredSelectionRejectsInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		changes Changes
		targets []Target
		opts    RequiredOptions
	}{
		{"unresolved", Changes{}, requiredTargets(), RequiredOptions{}},
		{"missing", committed(), requiredTargets()[:1], RequiredOptions{}},
		{"cycle", committed(), []Target{{Check: verification.Check{ID: "a", Needs: []string{"b"}}}, {Check: verification.Check{ID: "b", Needs: []string{"a"}}}}, RequiredOptions{}},
		{"duplicate", committed(), append(requiredTargets(), requiredTargets()[0]), RequiredOptions{}},
		{"unknown job", committed(), requiredTargets(), RequiredOptions{Rules: []config.PathRule{{Pattern: "**", Jobs: []string{"typo"}}}}},
		{"unknown workflow", committed(), requiredTargets(), RequiredOptions{Rules: []config.PathRule{{Pattern: "**", Workflows: []string{"typo"}}}}},
		{"invalid pattern", committed(), requiredTargets(), RequiredOptions{SharedPaths: []string{"["}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := SelectRequired(tc.changes, tc.targets, tc.opts); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	for _, targets := range [][]Target{nil, {{Check: verification.Check{ID: "optional"}}}} {
		if _, err := SelectRequired(committed(), targets, RequiredOptions{}); !errors.Is(err, ErrNoRequiredChecks) {
			t.Fatalf("%v", err)
		}
	}
}

func TestCommittedRenameAndDeletionSelection(t *testing.T) {
	root := repository(t)
	if err := os.Mkdir(filepath.Join(root, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	write(t, root, "docs/old.md", "old")
	write(t, root, "docs/deleted.md", "deleted")
	base := commit(t, root)
	git(t, root, "mv", "docs/old.md", "renamed.md")
	git(t, root, "rm", "docs/deleted.md")
	commit(t, root)
	changes, err := Discover(root, Options{Scope: BaseHead, Base: base})
	if err != nil {
		t.Fatal(err)
	}
	assertFiles(t, changes.Files, []string{"docs/deleted.md", "docs/old.md", "renamed.md"})
	got, err := SelectRequired(changes, requiredTargets(), RequiredOptions{Rules: []config.PathRule{{Pattern: "docs/**", Jobs: []string{"docs"}}}})
	if err != nil || len(got.Targets) != 4 {
		t.Fatalf("%+v %v", got, err)
	}
}
