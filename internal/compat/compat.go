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

func Report(cfg *config.Config, workflows []workflow.Workflow) []Item {
	var items []Item
	for _, finding := range doctor.Analyze(cfg, workflows) {
		item, ok := fromFinding(finding)
		if ok {
			items = append(items, item)
		}
	}
	for _, wf := range workflows {
		for id, job := range wf.Jobs {
			for _, step := range job.Steps {
				uses := strings.ToLower(step.Uses)
				switch {
				case strings.Contains(uses, "docker/login-action"):
					items = append(items, partial(wf.Path, id, "registry-login", "container registry login requires local credentials", "skip publish jobs or provide local registry credentials"))
				case strings.Contains(uses, "github-script"):
					items = append(items, partial(wf.Path, id, "github-api", "GitHub API calls may need a local token", "guard the step for local runs or provide a scoped token"))
				case strings.Contains(uses, "softprops/action-gh-release") || strings.Contains(uses, "goreleaser/goreleaser-action"):
					items = append(items, unsupported(wf.Path, id, "release-publish", "release publishing should stay in cloud CI", "add this job to skip.jobs"))
				}
			}
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
