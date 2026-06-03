package policy

import (
	"testing"

	"github.com/dotbrains/bellows/internal/config"
	"github.com/dotbrains/bellows/internal/workflow"
)

func TestCheckRequiresCloudJobsToBeSkipped(t *testing.T) {
	violations := Check(config.DefaultConfig(), []workflow.Workflow{{
		Path: ".github/workflows/ci.yml",
		Jobs: map[string]workflow.Job{
			"deploy": {RunsOn: []string{"ubuntu-latest"}},
		},
	}})
	if len(violations) == 0 || violations[0].Code != "cloud_job_not_skipped" {
		t.Fatalf("violations = %#v", violations)
	}
}

func TestCheckPassesSkippedCloudJobs(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Skip.Jobs = []string{"deploy"}
	violations := Check(cfg, []workflow.Workflow{{
		Path: ".github/workflows/ci.yml",
		Jobs: map[string]workflow.Job{
			"deploy": {RunsOn: []string{"ubuntu-latest"}},
		},
	}})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v", violations)
	}
}
