package migrate

import (
	"fmt"
	"os"
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
	cfg.Precommit.Paths = pathRules(repoRoot, defaults, workflows)
	return Report{
		Config:       cfg,
		Workflows:    defaults,
		Runners:      runners,
		SkippedJobs:  skipped,
		NextCommands: []string{"bellows init --force", "bellows hooks install --stage pre-push", "bellows doctor", "bellows run --list --estimate"},
	}
}

func pathRules(repoRoot string, defaults []string, workflows []workflow.Workflow) []config.PathRule {
	if len(defaults) == 0 {
		return nil
	}
	jobs := jobIDs(workflows)
	testJob := firstMatching(jobs, "test", "build", "ci")
	lintJob := firstMatching(jobs, "lint")
	var rules []config.PathRule
	if exists(repoRoot, "go.mod") || hasWorkflowStep(workflows, "go test") {
		rules = append(rules, config.PathRule{Pattern: "*.go", Workflows: defaults, Jobs: compact(testJob)})
	}
	if exists(repoRoot, "package.json") || hasWorkflowStep(workflows, "npm ") || hasWorkflowStep(workflows, "pnpm ") || hasWorkflowStep(workflows, "bun ") {
		rules = append(rules,
			config.PathRule{Pattern: "*.js", Workflows: defaults, Jobs: compact(testJob)},
			config.PathRule{Pattern: "*.ts", Workflows: defaults, Jobs: compact(testJob)},
			config.PathRule{Pattern: "package*.json", Workflows: defaults, Jobs: compact(testJob)},
		)
	}
	if exists(repoRoot, "docs") {
		rules = append(rules, config.PathRule{Pattern: "docs/*", Workflows: defaults, Jobs: compact(lintJob)})
	}
	for _, dir := range []string{"apps", "services", "packages"} {
		if exists(repoRoot, dir) {
			rules = append(rules, config.PathRule{Pattern: dir + "/*", Workflows: defaults, Jobs: compact(testJob)})
		}
	}
	return rules
}

func jobIDs(workflows []workflow.Workflow) []string {
	seen := map[string]bool{}
	for _, wf := range workflows {
		for id := range wf.Jobs {
			seen[id] = true
		}
	}
	var ids []string
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func firstMatching(values []string, needles ...string) string {
	for _, needle := range needles {
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), needle) {
				return value
			}
		}
	}
	return ""
}

func compact(values ...string) []string {
	var out []string
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func exists(root, path string) bool {
	_, err := os.Stat(filepath.Join(root, path))
	return err == nil
}

func hasWorkflowStep(workflows []workflow.Workflow, needle string) bool {
	for _, wf := range workflows {
		for _, job := range wf.Jobs {
			for _, step := range job.Steps {
				if strings.Contains(step.Run, needle) {
					return true
				}
			}
		}
	}
	return false
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
