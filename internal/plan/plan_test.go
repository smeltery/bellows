package plan

import (
	"reflect"
	"testing"

	"github.com/dotbrains/bellows/internal/config"
	"github.com/dotbrains/bellows/internal/workflow"
)

func TestBuildResolvesMatrixRunnersAndSkips(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DefaultWorkflows = []string{"ci.yml"}
	cfg.Skip.Jobs = []string{"deploy"}
	workflows := []workflow.Workflow{{
		Path: ".github/workflows/ci.yml",
		Jobs: map[string]workflow.Job{
			"test": {
				RunsOn: []string{"${{ matrix.os }}"},
				Matrix: map[string][]string{"os": []string{"ubuntu-latest", "macos-latest"}},
			},
			"deploy": {RunsOn: []string{"ubuntu-latest"}},
		},
	}}

	got := Build(cfg, workflows, Options{WorkflowRefs: []string{"ci.yml"}})
	if got.Event != "pull_request" {
		t.Fatalf("Event = %q", got.Event)
	}
	if len(got.Workflows) != 1 || len(got.Workflows[0].Jobs) != 2 {
		t.Fatalf("unexpected plan: %#v", got)
	}
	if !got.Workflows[0].Jobs[0].Skipped {
		t.Fatalf("deploy should be first sorted job and skipped: %#v", got.Workflows[0].Jobs)
	}
	wantImages := []string{
		"ubuntu-latest=catthehacker/ubuntu:act-22.04",
		"macos-latest=catthehacker/ubuntu:act-22.04",
	}
	if !reflect.DeepEqual(got.Workflows[0].Jobs[1].RunnerImages, wantImages) {
		t.Fatalf("RunnerImages = %#v, want %#v", got.Workflows[0].Jobs[1].RunnerImages, wantImages)
	}
}

func TestBuildFiltersJob(t *testing.T) {
	cfg := config.DefaultConfig()
	workflows := []workflow.Workflow{{
		Path: ".github/workflows/ci.yml",
		Jobs: map[string]workflow.Job{
			"build": {RunsOn: []string{"ubuntu-latest"}},
			"test":  {RunsOn: []string{"ubuntu-latest"}},
		},
	}}

	got := Build(cfg, workflows, Options{JobFilter: "test"})
	if len(got.Workflows) != 1 || len(got.Workflows[0].Jobs) != 1 {
		t.Fatalf("unexpected filtered plan: %#v", got)
	}
	if got.Workflows[0].Jobs[0].ID != "test" {
		t.Fatalf("filtered job = %q", got.Workflows[0].Jobs[0].ID)
	}
}
