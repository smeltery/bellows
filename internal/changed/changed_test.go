package changed

import (
	"reflect"
	"testing"

	"github.com/smeltery/bellows/internal/config"
)

func TestSelectMatchesPathRules(t *testing.T) {
	got := Select([]string{"docs/commands.md", "internal/plan/plan.go"}, []config.PathRule{
		{Pattern: "docs/*", Workflows: []string{"docs.yml"}},
		{Pattern: "internal/*/*", Workflows: []string{"ci.yml"}, Jobs: []string{"test"}},
	}, []string{"ci.yml"})

	if !reflect.DeepEqual(got.Workflows, []string{"ci.yml", "docs.yml"}) {
		t.Fatalf("Workflows = %#v", got.Workflows)
	}
	if !reflect.DeepEqual(got.Jobs, []string{"test"}) {
		t.Fatalf("Jobs = %#v", got.Jobs)
	}
}

func TestSelectUsesDefaultsForJobOnlyRules(t *testing.T) {
	got := Select([]string{"go.mod"}, []config.PathRule{
		{Pattern: "go.mod", Jobs: []string{"test"}},
	}, []string{"ci.yml"})

	if !reflect.DeepEqual(got.Workflows, []string{"ci.yml"}) {
		t.Fatalf("Workflows = %#v", got.Workflows)
	}
}

func TestSelectSupportsDoubleStarPathRules(t *testing.T) {
	got := Select([]string{"services/api/internal/server/main.go"}, []config.PathRule{
		{Pattern: "services/api/**", Jobs: []string{"api"}},
		{Pattern: "**/*.go", Jobs: []string{"go"}},
		{Pattern: "services/**/main.go", Jobs: []string{"main"}},
	}, []string{"ci.yml"})

	if !reflect.DeepEqual(got.Workflows, []string{"ci.yml"}) {
		t.Fatalf("Workflows = %#v", got.Workflows)
	}
	if !reflect.DeepEqual(got.Jobs, []string{"api", "go", "main"}) {
		t.Fatalf("Jobs = %#v", got.Jobs)
	}
}

func TestSelectSupportsNestedPackageJSON(t *testing.T) {
	got := Select([]string{"apps/web/package.json"}, []config.PathRule{
		{Pattern: "apps/**/package.json", Jobs: []string{"web"}},
	}, []string{"ci.yml"})

	if !reflect.DeepEqual(got.Jobs, []string{"web"}) {
		t.Fatalf("Jobs = %#v", got.Jobs)
	}
}
