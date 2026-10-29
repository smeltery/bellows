package plan

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/smeltery/bellows/internal/config"
	"github.com/smeltery/bellows/internal/runner"
	"github.com/smeltery/bellows/internal/workflow"
)

type Plan struct {
	Event            string         `json:"event"`
	DefaultWorkflows []string       `json:"default_workflows"`
	Workflows        []WorkflowPlan `json:"workflows"`
	Estimate         Estimate       `json:"estimate"`
}

type WorkflowPlan struct {
	Path string    `json:"path"`
	Jobs []JobPlan `json:"jobs"`
}

type JobPlan struct {
	ID           string   `json:"id"`
	RunsOn       []string `json:"runs_on,omitempty"`
	RunnerImages []string `json:"runner_images,omitempty"`
	Skipped      bool     `json:"skipped"`
}

type Estimate struct {
	RunnableJobs        int `json:"runnable_jobs"`
	SkippedJobs         int `json:"skipped_jobs"`
	EstimatedLocalRuns  int `json:"estimated_local_runs"`
	EstimatedCloudSaves int `json:"estimated_cloud_saves"`
}

type Options struct {
	WorkflowRefs  []string
	EventOverride string
	JobFilter     string
}

func Build(cfg *config.Config, workflows []workflow.Workflow, opts Options) Plan {
	event := opts.EventOverride
	if event == "" {
		event = cfg.Event
	}
	out := Plan{
		Event:            event,
		DefaultWorkflows: append([]string(nil), cfg.DefaultWorkflows...),
	}
	for _, wf := range workflows {
		if len(opts.WorkflowRefs) > 0 && !workflowSelected(wf, opts.WorkflowRefs) {
			continue
		}
		wp := WorkflowPlan{Path: wf.Path}
		for _, id := range workflow.JobIDs(wf) {
			if opts.JobFilter != "" && id != opts.JobFilter {
				continue
			}
			job := wf.Jobs[id]
			jp := JobPlan{
				ID:      id,
				RunsOn:  append([]string(nil), job.RunsOn...),
				Skipped: contains(cfg.Skip.Jobs, id),
			}
			for _, label := range job.RunsOn {
				for _, resolved := range resolvedRunnerLabels(label, job.Matrix) {
					if image, ok := runner.Resolve(resolved, cfg.Runner); ok {
						jp.RunnerImages = append(jp.RunnerImages, fmt.Sprintf("%s=%s", resolved, image))
					}
				}
			}
			if jp.Skipped {
				out.Estimate.SkippedJobs++
				out.Estimate.EstimatedCloudSaves++
			} else {
				out.Estimate.RunnableJobs++
				out.Estimate.EstimatedLocalRuns++
			}
			wp.Jobs = append(wp.Jobs, jp)
		}
		out.Workflows = append(out.Workflows, wp)
	}
	return out
}

func workflowSelected(wf workflow.Workflow, refs []string) bool {
	for _, ref := range refs {
		if ref == wf.Path || ref == filepath.Base(wf.Path) {
			return true
		}
	}
	return false
}

func resolvedRunnerLabels(label string, matrix map[string][]string) []string {
	if !strings.Contains(label, "${{") {
		return []string{label}
	}
	for key, values := range matrix {
		if strings.Contains(label, "matrix."+key) {
			return values
		}
	}
	return nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
