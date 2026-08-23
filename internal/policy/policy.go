package policy

import (
	"fmt"

	"github.com/smeltery/bellows/internal/config"
	"github.com/smeltery/bellows/internal/doctor"
	"github.com/smeltery/bellows/internal/workflow"
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
	waivers := waiverSet(cfg.Policy.Waivers)
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
					violation := Violation{
						Level:      cloudJobLevel,
						Code:       "cloud_job_not_skipped",
						Workflow:   wf.Path,
						Job:        job,
						Message:    fmt.Sprintf("cloud-like job %s is not skipped locally", job),
						Suggestion: "add this job to skip.jobs",
					}
					if !waived(waivers, violation) {
						out = append(out, violation)
					}
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
			violation := Violation{
				Level:      policyLevel(finding),
				Code:       finding.Code,
				Workflow:   finding.Workflow,
				Job:        finding.Job,
				Message:    finding.Message,
				Suggestion: finding.Suggestion,
			}
			if !waived(waivers, violation) {
				out = append(out, violation)
			}
		}
	}
	return out
}

func waiverSet(waivers []config.PolicyWaiver) map[string]bool {
	out := map[string]bool{}
	for _, waiver := range waivers {
		out[waiver.Code+"\x00"+waiver.Job] = true
		out[waiver.Code+"\x00"] = true
	}
	return out
}

func waived(waivers map[string]bool, violation Violation) bool {
	return waivers[violation.Code+"\x00"+violation.Job] || waivers[violation.Code+"\x00"]
}

func policyLevel(finding doctor.Finding) string {
	if finding.Code == "unmapped_runner" {
		return "error"
	}
	return "warn"
}
