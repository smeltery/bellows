package diagnostic

import (
	"sort"

	"github.com/dotbrains/bellows/internal/compat"
	"github.com/dotbrains/bellows/internal/config"
	"github.com/dotbrains/bellows/internal/doctor"
	"github.com/dotbrains/bellows/internal/policy"
	"github.com/dotbrains/bellows/internal/precommit"
	"github.com/dotbrains/bellows/internal/secrets"
	"github.com/dotbrains/bellows/internal/workflow"
)

type Bundle struct {
	Config    map[string]any      `json:"config"`
	Workflows WorkflowSummaries   `json:"workflows"`
	Doctor    []doctor.Finding    `json:"doctor"`
	Compat    []compat.Item       `json:"compat"`
	Policy    []policy.Violation  `json:"policy"`
	Secrets   []secrets.Reference `json:"secrets"`
	Hooks     precommit.Status    `json:"hooks"`
}

type WorkflowSummaries struct {
	Count     int               `json:"count"`
	Workflows []WorkflowSummary `json:"workflows"`
}

type WorkflowSummary struct {
	Path string   `json:"path"`
	Name string   `json:"name,omitempty"`
	Jobs []string `json:"jobs"`
}

func NewBundle(cfg *config.Config, workflows []workflow.Workflow, hookStatus precommit.Status) Bundle {
	return Bundle{
		Config: ConfigSummary(cfg),
		Workflows: WorkflowSummaries{
			Count:     len(workflows),
			Workflows: SummarizeWorkflows(workflows),
		},
		Doctor:  doctor.Analyze(cfg, workflows),
		Compat:  compat.Report(cfg, workflows),
		Policy:  policy.Check(cfg, workflows),
		Secrets: secrets.Audit(cfg, workflows),
		Hooks:   hookStatus,
	}
}

func ConfigSummary(cfg *config.Config) map[string]any {
	runnerLabels := make([]string, 0, len(cfg.Runner.Map))
	for label := range cfg.Runner.Map {
		runnerLabels = append(runnerLabels, label)
	}
	sort.Strings(runnerLabels)
	return map[string]any{
		"version":           cfg.Version,
		"default_workflows": cfg.DefaultWorkflows,
		"event":             cfg.Event,
		"runner_labels":     runnerLabels,
		"secrets_mode":      cfg.Secrets.Mode,
		"secrets_allow":     cfg.Secrets.Allow,
		"skip_jobs":         cfg.Skip.Jobs,
		"policy_cloud_jobs": cfg.Policy.CloudJobs,
		"precommit_stage":   cfg.Precommit.Stage,
		"precommit_mode":    cfg.Precommit.Mode,
	}
}

func SummarizeWorkflows(workflows []workflow.Workflow) []WorkflowSummary {
	out := make([]WorkflowSummary, 0, len(workflows))
	for _, wf := range workflows {
		out = append(out, WorkflowSummary{Path: wf.Path, Name: wf.Name, Jobs: workflow.JobIDs(wf)})
	}
	return out
}
