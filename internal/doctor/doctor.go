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
	Level   string
	Message string
}

func Analyze(cfg *config.Config, workflows []workflow.Workflow) []Finding {
	var findings []Finding
	if len(workflows) == 0 {
		findings = append(findings, Finding{"error", "no workflows found under .github/workflows"})
		return findings
	}
	for _, wf := range workflows {
		findings = append(findings, Finding{"info", fmt.Sprintf("workflow %s has jobs: %s", wf.Path, strings.Join(workflow.JobIDs(wf), ", "))})
		for id, job := range wf.Jobs {
			for _, label := range job.RunsOn {
				image, ok := runner.Resolve(label, cfg.Runner)
				if ok {
					prefix := "runner"
					if runner.IsBlacksmith(label) {
						prefix = "Blacksmith runner"
					}
					findings = append(findings, Finding{"ok", fmt.Sprintf("%s %s maps to %s", prefix, label, image)})
				} else {
					findings = append(findings, Finding{"error", fmt.Sprintf("job %s uses unmapped runner %s", id, label)})
				}
			}
			for _, step := range job.Steps {
				if strings.Contains(step.Uses, "aws-actions/configure-aws-credentials") {
					findings = append(findings, Finding{"warn", fmt.Sprintf("job %s uses AWS credentials; local OIDC is not emulated", id)})
				}
			}
		}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		order := map[string]int{"error": 0, "warn": 1, "ok": 2, "info": 3}
		return order[findings[i].Level] < order[findings[j].Level]
	})
	return findings
}
