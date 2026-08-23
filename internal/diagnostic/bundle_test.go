package diagnostic

import (
	"testing"

	"github.com/smeltery/bellows/internal/config"
	"github.com/smeltery/bellows/internal/precommit"
	"github.com/smeltery/bellows/internal/workflow"
)

func TestNewBundleSummarizesConfigAndWorkflows(t *testing.T) {
	cfg := config.Minimal([]string{"ci.yml"}, []string{"ubuntu-latest"})
	workflows := []workflow.Workflow{{
		Path: ".github/workflows/ci.yml",
		Name: "CI",
		Jobs: map[string]workflow.Job{"test": {RunsOn: []string{"ubuntu-latest"}}},
	}}
	bundle := NewBundle(cfg, workflows, precommit.Status{Path: ".pre-commit-config.yaml"})
	if bundle.Config["event"] != "pull_request" {
		t.Fatalf("unexpected config summary: %#v", bundle.Config)
	}
	if bundle.Workflows.Count != 1 || bundle.Workflows.Workflows[0].Jobs[0] != "test" {
		t.Fatalf("unexpected workflow summary: %#v", bundle.Workflows)
	}
	if len(bundle.Doctor) == 0 || len(bundle.Compat) == 0 {
		t.Fatalf("bundle missing diagnostics: %#v", bundle)
	}
}
