package migrate

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dotbrains/bellows/internal/config"
	"github.com/dotbrains/bellows/internal/workflow"
)

type Report struct {
	Config       *config.Config `json:"config"`
	Workflows    []string       `json:"workflows"`
	Runners      []string       `json:"runners"`
	SkippedJobs  []string       `json:"skipped_jobs"`
	NextCommands []string       `json:"next_commands"`
}

func GitHubActions(repoRoot string, workflows []workflow.Workflow) Report {
	defaults := defaultWorkflowRefs(workflows)
	runners := workflow.Labels(workflows)
	skipped := workflow.CloudLikeJobIDs(workflows)
	cfg := config.Minimal(defaults, runners, skipped...)
	return Report{
		Config:       cfg,
		Workflows:    defaults,
		Runners:      runners,
		SkippedJobs:  skipped,
		NextCommands: []string{"bellows init --force", "bellows hooks install --stage pre-push", "bellows doctor", "bellows run --list --estimate"},
	}
}

func Text(report Report) (string, error) {
	data, err := config.Marshal(report.Config)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("migration plan\n")
	fmt.Fprintf(&b, "workflows: %s\n", strings.Join(report.Workflows, ", "))
	if len(report.Runners) > 0 {
		fmt.Fprintf(&b, "runners: %s\n", strings.Join(report.Runners, ", "))
	}
	if len(report.SkippedJobs) > 0 {
		fmt.Fprintf(&b, "skip locally: %s\n", strings.Join(report.SkippedJobs, ", "))
	}
	b.WriteString("\nrecommended .bellows.yml\n")
	b.Write(data)
	b.WriteString("\nnext commands\n")
	for _, command := range report.NextCommands {
		fmt.Fprintf(&b, "  %s\n", command)
	}
	return b.String(), nil
}

func defaultWorkflowRefs(workflows []workflow.Workflow) []string {
	if len(workflows) == 0 {
		return nil
	}
	for _, wf := range workflows {
		base := filepath.Base(wf.Path)
		if base == "ci.yml" || base == "ci.yaml" {
			return []string{base}
		}
	}
	refs := []string{filepath.Base(workflows[0].Path)}
	sort.Strings(refs)
	return refs
}
