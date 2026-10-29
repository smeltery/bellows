package doctor

import (
	"strings"
	"testing"

	"github.com/smeltery/bellows/internal/config"
	"github.com/smeltery/bellows/internal/workflow"
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
				RunsOn:      []string{"blacksmith-4vcpu-ubuntu-2204"},
				Permissions: map[string]string{"id-token": "write"},
				Environment: []string{"production"},
				Steps: []workflow.Step{
					{Uses: "aws-actions/configure-aws-credentials@v4"},
					{Uses: "actions/upload-artifact@v4"},
					{Uses: "actions/cache@v4"},
				},
			},
		},
	}

	findings := Analyze(config.DefaultConfig(), []workflow.Workflow{wf})
	assertFinding(t, findings, "ok", "Blacksmith runner blacksmith-4vcpu-ubuntu-2204 maps to ghcr.io/smeltery/kiln:ubuntu-22.04")
	assertFinding(t, findings, "warn", "local OIDC is not emulated")
	assertFinding(t, findings, "warn", "uses deployment environment production")
	assertFinding(t, findings, "warn", "uploads artifacts")
	assertFinding(t, findings, "warn", "uses actions/cache")
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

func TestAnalyzeWarnsForDynamicRunnerExpressions(t *testing.T) {
	wf := workflow.Workflow{
		Path: ".github/workflows/ci.yml",
		Jobs: map[string]workflow.Job{
			"test": {RunsOn: []string{"${{ matrix.os }}"}},
		},
	}

	findings := Analyze(config.DefaultConfig(), []workflow.Workflow{wf})
	assertFinding(t, findings, "warn", "dynamic runner expression")
	for _, finding := range findings {
		if strings.Contains(finding.Message, "unmapped runner") {
			t.Fatalf("dynamic runner should not be reported as unmapped: %#v", findings)
		}
	}
}

func TestAnalyzeChecksMatrixRunnerValues(t *testing.T) {
	wf := workflow.Workflow{
		Path: ".github/workflows/ci.yml",
		Jobs: map[string]workflow.Job{
			"test": {
				RunsOn: []string{"${{ matrix.os }}"},
				Matrix: map[string][]string{"os": []string{"ubuntu-latest", "macos-latest"}},
			},
		},
	}

	findings := Analyze(config.DefaultConfig(), []workflow.Workflow{wf})
	assertFinding(t, findings, "warn", "dynamic runner expression")
	assertFinding(t, findings, "ok", "runner ubuntu-latest maps")
	assertFinding(t, findings, "ok", "macOS runner compatibility mapping macos-latest maps")
}

func TestAnalyzeReportsMacOSCompatibilityMapping(t *testing.T) {
	wf := workflow.Workflow{
		Path: ".github/workflows/ci.yml",
		Jobs: map[string]workflow.Job{
			"test": {RunsOn: []string{"macos-latest"}},
		},
	}

	findings := Analyze(config.DefaultConfig(), []workflow.Workflow{wf})
	assertFinding(t, findings, "ok", "macOS runner compatibility mapping macos-latest maps to ghcr.io/smeltery/kiln:ubuntu-22.04")
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
