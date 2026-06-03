package doctor

import (
	"strings"
	"testing"

	"github.com/dotbrains/bellows/internal/config"
	"github.com/dotbrains/bellows/internal/workflow"
)

func TestAnalyzeReportsNoWorkflows(t *testing.T) {
	findings := Analyze(config.DefaultConfig(), nil)
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if findings[0].Level != "error" || !strings.Contains(findings[0].Message, "no workflows") {
		t.Fatalf("finding = %#v", findings[0])
	}
}

func TestAnalyzeReportsBlacksmithMappingAndCloudWarnings(t *testing.T) {
	wf := workflow.Workflow{
		Path: ".github/workflows/ci.yml",
		Jobs: map[string]workflow.Job{
			"test": {
				RunsOn: []string{"blacksmith-4vcpu-ubuntu-2204"},
				Steps:  []workflow.Step{{Uses: "aws-actions/configure-aws-credentials@v4"}},
			},
		},
	}

	findings := Analyze(config.DefaultConfig(), []workflow.Workflow{wf})
	assertFinding(t, findings, "ok", "Blacksmith runner blacksmith-4vcpu-ubuntu-2204 maps to catthehacker/ubuntu:act-22.04")
	assertFinding(t, findings, "warn", "local OIDC is not emulated")
	assertFinding(t, findings, "info", "workflow .github/workflows/ci.yml has jobs: test")
}

func TestAnalyzeReportsUnmappedRunner(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Runner.Map = map[string]string{}
	wf := workflow.Workflow{
		Path: ".github/workflows/gpu.yml",
		Jobs: map[string]workflow.Job{
			"gpu": {RunsOn: []string{"blacksmith-gpu-ubuntu-2204"}},
		},
	}

	findings := Analyze(cfg, []workflow.Workflow{wf})
	assertFinding(t, findings, "error", "job gpu uses unmapped runner blacksmith-gpu-ubuntu-2204")
}

func assertFinding(t *testing.T, findings []Finding, level, message string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Level == level && strings.Contains(finding.Message, message) {
			return
		}
	}
	t.Fatalf("missing %s finding containing %q in %#v", level, message, findings)
}
