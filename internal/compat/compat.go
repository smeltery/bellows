package compat

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dotbrains/bellows/internal/config"
	"github.com/dotbrains/bellows/internal/doctor"
	"github.com/dotbrains/bellows/internal/workflow"
)

type Item struct {
	Status     string `json:"status"`
	Workflow   string `json:"workflow,omitempty"`
	Job        string `json:"job,omitempty"`
	Feature    string `json:"feature"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type Feature struct {
	Status      string
	Feature     string
	Description string
	Guidance    string
}

func Features() []Feature {
	return []Feature{
		{"supported", "runner", "Mapped GitHub, macOS fallback, and Blacksmith runner labels", "Review `.bellows.yml` runner mappings before relying on local parity."},
		{"partial", "dynamic-runner", "Matrix-driven `runs-on` expressions", "Map every possible matrix runner value."},
		{"partial", "artifact-upload", "`actions/upload-artifact` during local runs", "Guard upload steps with `if: ${{ env.ACT != 'true' }}` when needed."},
		{"partial", "actions-cache", "`actions/cache` local behavior", "Treat local cache hits as best effort and keep builds deterministic."},
		{"partial", "registry-login", "Container registry login actions", "Provide local credentials or skip publish jobs."},
		{"partial", "github-api", "GitHub API calls from workflow steps", "Provide a scoped token or guard the step for local runs."},
		{"unsupported", "runner", "Unmapped runner labels", "Add `runner.map` entries before running locally."},
		{"unsupported", "oidc", "GitHub OIDC token minting", "Keep OIDC-backed cloud credential flows in cloud CI or skip locally."},
		{"unsupported", "environment-protection", "GitHub deployment environments and approvals", "Keep protected deployments in cloud CI."},
		{"unsupported", "cloud-credentials", "Cloud credential setup actions such as AWS OIDC", "Skip these jobs locally unless a local credential path is explicit."},
		{"unsupported", "release-publish", "Release publishing actions", "Add release jobs to `skip.jobs`."},
	}
}

func Markdown() string {
	var b strings.Builder
	b.WriteString("# Compatibility\n\n")
	b.WriteString("This table is generated from Bellows compatibility classifications.\n\n")
	b.WriteString("| Status | Feature | Local Behavior | Guidance |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, feature := range Features() {
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s |\n", feature.Status, feature.Feature, feature.Description, feature.Guidance)
	}
	return b.String()
}

func Report(cfg *config.Config, workflows []workflow.Workflow) []Item {
	var items []Item
	for _, finding := range doctor.Analyze(cfg, workflows) {
		item, ok := fromFinding(finding)
		if ok {
			items = append(items, item)
		}
	}
	for _, wf := range workflows {
		for _, use := range wf.ActionUses() {
			rule, ok := MatchAction(use.Step.Uses)
			if !ok {
				continue
			}
			items = append(items, Item{Status: rule.Status, Workflow: wf.Path, Job: use.Job, Feature: rule.Feature, Message: rule.Message, Suggestion: rule.Suggestion})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if statusOrder(items[i].Status) != statusOrder(items[j].Status) {
			return statusOrder(items[i].Status) < statusOrder(items[j].Status)
		}
		if items[i].Workflow != items[j].Workflow {
			return items[i].Workflow < items[j].Workflow
		}
		if items[i].Job != items[j].Job {
			return items[i].Job < items[j].Job
		}
		return items[i].Feature < items[j].Feature
	})
	return items
}

func fromFinding(finding doctor.Finding) (Item, bool) {
	switch finding.Code {
	case "unmapped_runner":
		return unsupported(finding.Workflow, finding.Job, "runner", finding.Message, finding.Suggestion), true
	case "dynamic_runner":
		return partial(finding.Workflow, finding.Job, "dynamic-runner", finding.Message, finding.Suggestion), true
	case "oidc_permission":
		return unsupported(finding.Workflow, finding.Job, "oidc", finding.Message, finding.Suggestion), true
	case "deployment_environment":
		return unsupported(finding.Workflow, finding.Job, "environment-protection", finding.Message, finding.Suggestion), true
	case "aws_credentials":
		return unsupported(finding.Workflow, finding.Job, "cloud-credentials", finding.Message, finding.Suggestion), true
	case "upload_artifact":
		return partial(finding.Workflow, finding.Job, "artifact-upload", finding.Message, finding.Suggestion), true
	case "actions_cache":
		return partial(finding.Workflow, finding.Job, "actions-cache", finding.Message, finding.Suggestion), true
	case "runner_mapped", "blacksmith_runner_mapped", "macos_runner_mapped":
		return Item{Status: "supported", Workflow: finding.Workflow, Job: finding.Job, Feature: "runner", Message: finding.Message}, true
	default:
		return Item{}, false
	}
}

func unsupported(workflowPath, job, feature, message, suggestion string) Item {
	return Item{Status: "unsupported", Workflow: workflowPath, Job: job, Feature: feature, Message: message, Suggestion: suggestion}
}

func partial(workflowPath, job, feature, message, suggestion string) Item {
	return Item{Status: "partial", Workflow: workflowPath, Job: job, Feature: feature, Message: message, Suggestion: suggestion}
}

func statusOrder(status string) int {
	switch status {
	case "unsupported":
		return 0
	case "partial":
		return 1
	case "supported":
		return 2
	default:
		return 3
	}
}

func Summary(items []Item) string {
	counts := map[string]int{}
	for _, item := range items {
		counts[item.Status]++
	}
	return fmt.Sprintf("%d supported, %d partial, %d unsupported", counts["supported"], counts["partial"], counts["unsupported"])
}
