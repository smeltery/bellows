package policy

import (
	"fmt"

	"github.com/dotbrains/bellows/internal/config"
	"github.com/dotbrains/bellows/internal/doctor"
	"github.com/dotbrains/bellows/internal/workflow"
)

type Violation struct {
	Level      string `json:"level"`
	Code       string `json:"code"`
	Workflow   string `json:"workflow,omitempty"`
	Job        string `json:"job,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

func Check(cfg *config.Config, workflows []workflow.Workflow) []Violation {
	var out []Violation
	skipped := map[string]bool{}
	for _, job := range cfg.Skip.Jobs {
		skipped[job] = true
	}
	cloudJobLevel := cfg.Policy.CloudJobs
	if cloudJobLevel == "" {
		cloudJobLevel = "error"
	}
	if cloudJobLevel != "off" {
		for _, wf := range workflows {
			for _, job := range workflow.CloudLikeJobIDs([]workflow.Workflow{wf}) {
				if !skipped[job] {
					out = append(out, Violation{
						Level:      cloudJobLevel,
						Code:       "cloud_job_not_skipped",
						Workflow:   wf.Path,
						Job:        job,
						Message:    fmt.Sprintf("cloud-like job %s is not skipped locally", job),
						Suggestion: "add this job to skip.jobs",
					})
				}
			}
		}
	}
	for _, finding := range doctor.Analyze(cfg, workflows) {
		if skipped[finding.Job] {
			continue
		}
		switch finding.Code {
		case "unmapped_runner", "upload_artifact", "aws_credentials", "oidc_permission", "deployment_environment":
			out = append(out, Violation{
				Level:      policyLevel(finding),
				Code:       finding.Code,
				Workflow:   finding.Workflow,
				Job:        finding.Job,
				Message:    finding.Message,
				Suggestion: finding.Suggestion,
			})
		}
	}
	return out
}

func policyLevel(finding doctor.Finding) string {
	if finding.Code == "unmapped_runner" {
		return "error"
	}
	return "warn"
}
