package policy

import (
	"testing"

	"github.com/smeltery/bellows/internal/config"
	"github.com/smeltery/bellows/internal/workflow"
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

func TestCheckCloudJobsPolicyLevel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Policy.CloudJobs = "warn"
	violations := Check(cfg, []workflow.Workflow{{
		Path: ".github/workflows/ci.yml",
		Jobs: map[string]workflow.Job{
			"deploy": {RunsOn: []string{"ubuntu-latest"}},
		},
	}})
	if len(violations) == 0 || violations[0].Level != "warn" {
		t.Fatalf("violations = %#v", violations)
	}
	cfg.Policy.CloudJobs = "off"
	violations = Check(cfg, []workflow.Workflow{{
		Path: ".github/workflows/ci.yml",
		Jobs: map[string]workflow.Job{
			"deploy": {RunsOn: []string{"ubuntu-latest"}},
		},
	}})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v", violations)
	}
}
