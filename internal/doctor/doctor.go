package doctor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dotbrains/bellows/internal/config"
	"github.com/dotbrains/bellows/internal/runner"
	"github.com/dotbrains/bellows/internal/workflow"
)

type Finding struct {
	Level      string `json:"level"`
	Code       string `json:"code"`
	Workflow   string `json:"workflow,omitempty"`
	Job        string `json:"job,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

func NewFinding(level, code, message string) Finding {
	return Finding{Level: level, Code: code, Message: message}
}

func Analyze(cfg *config.Config, workflows []workflow.Workflow) []Finding {
	var findings []Finding
	if len(workflows) == 0 {
		return []Finding{NewFinding("error", "no_workflows", "no workflows found under .github/workflows")}
	}
	for _, wf := range workflows {
		findings = append(findings, Finding{
			Level:    "info",
			Code:     "workflow_jobs",
			Workflow: wf.Path,
			Message:  fmt.Sprintf("workflow %s has jobs: %s", wf.Path, strings.Join(workflow.JobIDs(wf), ", ")),
		})
		for id, job := range wf.Jobs {
			findings = append(findings, analyzeJob(cfg, wf.Path, id, job)...)
		}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		order := map[string]int{"error": 0, "warn": 1, "ok": 2, "info": 3}
		return order[findings[i].Level] < order[findings[j].Level]
	})
	return findings
}

func analyzeJob(cfg *config.Config, workflowPath, id string, job workflow.Job) []Finding {
	var findings []Finding
	for _, label := range job.RunsOn {
		if strings.Contains(label, "${{") {
			findings = append(findings, Finding{
				Level:      "warn",
				Code:       "dynamic_runner",
				Workflow:   workflowPath,
				Job:        id,
				Message:    fmt.Sprintf("job %s uses dynamic runner expression %s", id, label),
				Suggestion: "ensure every possible matrix runner value is mapped in .bellows.yml",
			})
			for _, matrixLabel := range matrixValuesForExpression(label, job.Matrix) {
				findings = append(findings, analyzeRunner(cfg, workflowPath, id, matrixLabel)...)
			}
			continue
		}
		findings = append(findings, analyzeRunner(cfg, workflowPath, id, label)...)
	}
	if job.Permissions["id-token"] == "write" {
		findings = append(findings, Finding{
			Level:      "warn",
			Code:       "oidc_permission",
			Workflow:   workflowPath,
			Job:        id,
			Message:    fmt.Sprintf("job %s requests OIDC id-token permissions; local OIDC is not emulated", id),
			Suggestion: "skip this job locally or provide a local credential path",
		})
	}
	if len(job.Environment) > 0 {
		findings = append(findings, Finding{
			Level:      "warn",
			Code:       "deployment_environment",
			Workflow:   workflowPath,
			Job:        id,
			Message:    fmt.Sprintf("job %s uses deployment environment %s", id, strings.Join(job.Environment, ", ")),
			Suggestion: "keep deployment environment checks in cloud CI or skip this job locally",
		})
	}
	for _, step := range job.Steps {
		findings = append(findings, analyzeStep(workflowPath, id, step)...)
	}
	return findings
}

func analyzeRunner(cfg *config.Config, workflowPath, id, label string) []Finding {
	image, ok := runner.Resolve(label, cfg.Runner)
	if ok {
		prefix := "runner"
		code := "runner_mapped"
		if runner.IsBlacksmith(label) {
			prefix = "Blacksmith runner"
			code = "blacksmith_runner_mapped"
		} else if strings.HasPrefix(label, "macos-") {
			prefix = "macOS runner compatibility mapping"
			code = "macos_runner_mapped"
		}
		return []Finding{{
			Level:    "ok",
			Code:     code,
			Workflow: workflowPath,
			Job:      id,
			Message:  fmt.Sprintf("%s %s maps to %s", prefix, label, image),
		}}
	}
	return []Finding{{
		Level:      "error",
		Code:       "unmapped_runner",
		Workflow:   workflowPath,
		Job:        id,
		Message:    fmt.Sprintf("job %s uses unmapped runner %s", id, label),
		Suggestion: fmt.Sprintf("add runner.map.%s to .bellows.yml", label),
	}}
}

func analyzeStep(workflowPath, id string, step workflow.Step) []Finding {
	uses := strings.ToLower(step.Uses)
	condition := strings.ToLower(step.If)
	switch {
	case strings.Contains(uses, "aws-actions/configure-aws-credentials"):
		return []Finding{{
			Level:      "warn",
			Code:       "aws_credentials",
			Workflow:   workflowPath,
			Job:        id,
			Message:    fmt.Sprintf("job %s uses AWS credentials; local OIDC is not emulated", id),
			Suggestion: "skip this job locally or configure explicit local AWS credentials",
		}}
	case strings.Contains(uses, "actions/upload-artifact") && !strings.Contains(condition, "env.act") && !strings.Contains(condition, "env.bellows"):
		return []Finding{{
			Level:      "warn",
			Code:       "upload_artifact",
			Workflow:   workflowPath,
			Job:        id,
			Message:    fmt.Sprintf("job %s uploads artifacts; act may not provide GitHub artifact runtime credentials", id),
			Suggestion: "guard this step with if: ${{ env.ACT != 'true' }}",
		}}
	case strings.Contains(uses, "actions/cache"):
		return []Finding{{
			Level:      "warn",
			Code:       "actions_cache",
			Workflow:   workflowPath,
			Job:        id,
			Message:    fmt.Sprintf("job %s uses actions/cache; local cache behavior may differ from GitHub Actions", id),
			Suggestion: "treat local cache behavior as best-effort",
		}}
	default:
		return nil
	}
}

func matrixValuesForExpression(expr string, matrix map[string][]string) []string {
	for key, values := range matrix {
		if strings.Contains(expr, "matrix."+key) {
			return values
		}
	}
	return nil
}
