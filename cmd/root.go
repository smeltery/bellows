package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dotbrains/bellows/internal/act"
	"github.com/dotbrains/bellows/internal/config"
	"github.com/dotbrains/bellows/internal/doctor"
	"github.com/dotbrains/bellows/internal/precommit"
	"github.com/dotbrains/bellows/internal/runner"
	"github.com/dotbrains/bellows/internal/workflow"
	"github.com/spf13/cobra"
)

type cliOptions struct {
	repoRoot string
}

type executionPlan struct {
	Event            string         `json:"event"`
	DefaultWorkflows []string       `json:"default_workflows"`
	Workflows        []workflowPlan `json:"workflows"`
}

type workflowPlan struct {
	Path string    `json:"path"`
	Jobs []jobPlan `json:"jobs"`
}

type jobPlan struct {
	ID           string   `json:"id"`
	RunsOn       []string `json:"runs_on,omitempty"`
	RunnerImages []string `json:"runner_images,omitempty"`
	Skipped      bool     `json:"skipped"`
}

func newRootCmd(version string) *cobra.Command {
	opts := &cliOptions{}
	root := &cobra.Command{
		Use:   "bellows",
		Short: "Run GitHub Actions locally before you push",
		Long:  "Bellows runs existing GitHub Actions workflows locally with Docker before they reach cloud runners.",
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		Version:      version,
		SilenceUsage: true,
	}

	root.SetVersionTemplate(fmt.Sprintf("bellows version %s\n", version))
	root.PersistentFlags().StringVar(&opts.repoRoot, "repo", "", "repository root (defaults to current git repository)")

	root.AddCommand(newInitCmd(opts))
	root.AddCommand(newRunCmd(opts))
	root.AddCommand(newDoctorCmd(opts))
	root.AddCommand(newCacheCmd())
	root.AddCommand(newExplainCmd(opts))

	return root
}

func newInitCmd(opts *cliOptions) *cobra.Command {
	var force bool
	var installPreCommit bool
	var forcePreCommit bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create .bellows.yml for this repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot(opts.repoRoot)
			if err != nil {
				return err
			}
			workflows, err := workflow.Discover(root)
			if err != nil {
				return err
			}
			defaults := defaultWorkflowRefs(workflows)
			cfg := config.Minimal(defaults, workflow.Labels(workflows), workflow.CloudLikeJobIDs(workflows)...)
			path := filepath.Join(root, config.FileName)
			if dryRun {
				data, err := config.Marshal(cfg)
				if err != nil {
					return err
				}
				_, err = cmd.OutOrStdout().Write(data)
				return err
			}
			if !force {
				if _, err := os.Stat(path); err == nil {
					return fmt.Errorf("%s already exists (use --force to overwrite)", config.FileName)
				}
			}
			if err := config.SaveTo(cfg, path); err != nil {
				return err
			}
			cmd.Printf("wrote %s\n", rel(root, path))
			if installPreCommit {
				hookPath, err := precommit.Write(root, forcePreCommit)
				if err != nil {
					return err
				}
				cmd.Printf("wrote %s\n", rel(root, hookPath))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite .bellows.yml")
	cmd.Flags().BoolVar(&installPreCommit, "pre-commit", true, "write a pre-push pre-commit hook")
	cmd.Flags().BoolVar(&forcePreCommit, "force-pre-commit", false, "overwrite .pre-commit-config.yaml")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print generated .bellows.yml without writing files")
	return cmd
}

func newRunCmd(opts *cliOptions) *cobra.Command {
	var workflowRef string
	var job string
	var event string
	var precommitMode bool
	var listOnly bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run configured GitHub Actions workflows locally",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot(opts.repoRoot)
			if err != nil {
				return err
			}
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}
			refs := selectedWorkflows(workflowRef, cfg.DefaultWorkflows)
			if len(refs) == 0 {
				return fmt.Errorf("no workflow selected; run bellows init or pass --workflow")
			}
			if listOnly {
				workflows, err := workflow.Discover(root)
				if err != nil {
					return err
				}
				plan := buildExecutionPlan(cfg, workflows, refs, event, job)
				printExecutionPlan(cmd, plan)
				return nil
			}
			if err := requireDependency("act"); err != nil {
				return err
			}
			if err := requireDocker(); err != nil {
				return err
			}
			for _, ref := range refs {
				path := workflow.Resolve(root, ref)
				display := rel(root, path)
				args, err := act.Args(act.Options{
					Event:    event,
					Workflow: display,
					Job:      job,
					Config:   cfg,
				})
				if err != nil {
					return err
				}
				if precommitMode {
					cmd.Printf("bellows: running %s\n", display)
				}
				if err := runPassthrough(root, "act", args...); err != nil {
					if job != "" {
						return fmt.Errorf("bellows failed while running %s job %s\nrerun: bellows run --workflow %s --job %s", display, job, display, job)
					}
					return fmt.Errorf("bellows failed while running %s\nrerun: bellows run --workflow %s", display, display)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&workflowRef, "workflow", "w", "", "workflow file name or path")
	cmd.Flags().StringVarP(&job, "job", "j", "", "job id to run")
	cmd.Flags().StringVar(&event, "event", "", "event name to pass to act")
	cmd.Flags().BoolVar(&precommitMode, "precommit", false, "run with pre-commit friendly output")
	cmd.Flags().BoolVar(&listOnly, "list", false, "list runnable workflows and jobs without running act")
	return cmd
}

func newDoctorCmd(opts *cliOptions) *cobra.Command {
	var format string
	var strict bool
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Inspect local GitHub Actions compatibility",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot(opts.repoRoot)
			if err != nil {
				return err
			}
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}
			workflows, err := workflow.Discover(root)
			if err != nil {
				return err
			}
			if fix {
				fixes, err := applyDoctorFixes(root, cfg, workflows)
				if err != nil {
					return err
				}
				if format == "text" {
					for _, msg := range fixes {
						cmd.Printf("[fix] %s\n", msg)
					}
				}
				cfg, err = config.Load(root)
				if err != nil {
					return err
				}
			}
			findings := doctor.Analyze(cfg, workflows)
			findings = append(dependencyFindings(), findings...)
			if format == "json" {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(findings); err != nil {
					return err
				}
			} else if format != "text" {
				return fmt.Errorf("unsupported format %q", format)
			}
			var hasError bool
			var hasWarn bool
			for _, finding := range findings {
				if finding.Level == "error" {
					hasError = true
				}
				if finding.Level == "warn" {
					hasWarn = true
				}
			}
			if format == "text" {
				printGroupedFindings(cmd, findings)
			}
			if hasError {
				return errors.New("doctor found local compatibility issues")
			}
			if strict && hasWarn {
				return errors.New("doctor found local compatibility warnings")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	cmd.Flags().BoolVar(&strict, "strict", false, "fail when warnings are found")
	cmd.Flags().BoolVar(&fix, "fix", false, "apply safe .bellows.yml fixes before reporting diagnostics")
	return cmd
}

func newCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage Bellows local caches",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "prune",
		Short: "Prune Docker build cache",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireDependency("docker"); err != nil {
				return err
			}
			return runPassthrough("", "docker", "builder", "prune", "--force")
		},
	})
	return cmd
}

func newExplainCmd(opts *cliOptions) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "explain",
		Short: "Show what Bellows will run",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot(opts.repoRoot)
			if err != nil {
				return err
			}
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}
			workflows, err := workflow.Discover(root)
			if err != nil {
				return err
			}
			plan := buildExecutionPlan(cfg, workflows, cfg.DefaultWorkflows, "", "")
			if format == "json" {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(plan)
			}
			if format != "text" {
				return fmt.Errorf("unsupported format %q", format)
			}
			printExecutionPlan(cmd, plan)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	return cmd
}

func repoRoot(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	return os.Getwd()
}

func requireDependency(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s is required but was not found in PATH", name)
	}
	return nil
}

func dependencyFindings() []doctor.Finding {
	findings := make([]doctor.Finding, 0, 2)
	if _, err := exec.LookPath("act"); err != nil {
		findings = append(findings, doctor.NewFinding("warn", "act_missing", "act was not found in PATH; bellows run requires act"))
	} else {
		findings = append(findings, doctor.NewFinding("ok", "act_available", "act is available"))
	}
	if _, err := exec.LookPath("docker"); err != nil {
		findings = append(findings, doctor.NewFinding("warn", "docker_missing", "docker was not found in PATH; bellows run requires Docker"))
		return findings
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		findings = append(findings, doctor.NewFinding("warn", "docker_unreachable", "Docker is installed but not reachable"))
		return findings
	}
	findings = append(findings, doctor.NewFinding("ok", "docker_reachable", "Docker is reachable"))
	return findings
}

func applyDoctorFixes(root string, cfg *config.Config, workflows []workflow.Workflow) ([]string, error) {
	var messages []string
	for _, label := range workflow.Labels(workflows) {
		if cfg.Runner.Map == nil {
			cfg.Runner.Map = map[string]string{}
		}
		if cfg.Runner.Map[label] != "" {
			continue
		}
		image, ok := config.DefaultRunnerImage(label)
		if !ok {
			continue
		}
		cfg.Runner.Map[label] = image
		messages = append(messages, fmt.Sprintf("mapped runner %s to %s", label, image))
	}
	seenSkip := map[string]bool{}
	for _, job := range cfg.Skip.Jobs {
		seenSkip[job] = true
	}
	for _, job := range workflow.CloudLikeJobIDs(workflows) {
		if seenSkip[job] {
			continue
		}
		cfg.Skip.Jobs = append(cfg.Skip.Jobs, job)
		messages = append(messages, fmt.Sprintf("added %s to skip.jobs", job))
	}
	if len(messages) == 0 {
		return []string{"no safe fixes available"}, nil
	}
	return messages, config.SaveTo(cfg, filepath.Join(root, config.FileName))
}

func printGroupedFindings(cmd *cobra.Command, findings []doctor.Finding) {
	ordered := append([]doctor.Finding(nil), findings...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Workflow != ordered[j].Workflow {
			return ordered[i].Workflow < ordered[j].Workflow
		}
		if ordered[i].Job != ordered[j].Job {
			return ordered[i].Job < ordered[j].Job
		}
		return findingLevelOrder(ordered[i].Level) < findingLevelOrder(ordered[j].Level)
	})
	cmd.Println("dependencies")
	for _, finding := range ordered {
		if finding.Workflow == "" {
			printFinding(cmd, finding)
		}
	}
	currentWorkflow := ""
	currentJob := ""
	for _, finding := range ordered {
		if finding.Workflow == "" {
			continue
		}
		if finding.Workflow != currentWorkflow {
			currentWorkflow = finding.Workflow
			currentJob = ""
			cmd.Printf("\n%s\n", currentWorkflow)
		}
		if finding.Job != "" && finding.Job != currentJob {
			currentJob = finding.Job
			cmd.Printf("  job %s\n", currentJob)
		}
		printFinding(cmd, finding)
	}
}

func findingLevelOrder(level string) int {
	switch level {
	case "error":
		return 0
	case "warn":
		return 1
	case "ok":
		return 2
	case "info":
		return 3
	default:
		return 4
	}
}

func printFinding(cmd *cobra.Command, finding doctor.Finding) {
	prefix := "  "
	if finding.Job != "" {
		prefix = "    "
	}
	cmd.Printf("%s[%s] %s\n", prefix, finding.Level, finding.Message)
	if finding.Suggestion != "" {
		cmd.Printf("%s      suggestion: %s\n", prefix, finding.Suggestion)
	}
}

func buildExecutionPlan(cfg *config.Config, workflows []workflow.Workflow, refs []string, eventOverride, jobFilter string) executionPlan {
	event := eventOverride
	if event == "" {
		event = cfg.Event
	}
	plan := executionPlan{
		Event:            event,
		DefaultWorkflows: append([]string(nil), cfg.DefaultWorkflows...),
	}
	for _, wf := range workflows {
		if len(refs) > 0 && !workflowSelected(wf, refs) {
			continue
		}
		wp := workflowPlan{Path: wf.Path}
		for _, id := range workflow.JobIDs(wf) {
			if jobFilter != "" && id != jobFilter {
				continue
			}
			job := wf.Jobs[id]
			jp := jobPlan{
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
			wp.Jobs = append(wp.Jobs, jp)
		}
		plan.Workflows = append(plan.Workflows, wp)
	}
	return plan
}

func printExecutionPlan(cmd *cobra.Command, plan executionPlan) {
	cmd.Printf("event: %s\n", plan.Event)
	cmd.Printf("default workflows: %s\n", strings.Join(plan.DefaultWorkflows, ", "))
	for _, wf := range plan.Workflows {
		cmd.Printf("%s\n", wf.Path)
		for _, job := range wf.Jobs {
			status := "run"
			if job.Skipped {
				status = "skip"
			}
			cmd.Printf("  [%s] %s", status, job.ID)
			if len(job.RunnerImages) > 0 {
				cmd.Printf(" (%s)", strings.Join(job.RunnerImages, ", "))
			}
			cmd.Println()
		}
	}
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

func requireDocker() error {
	if err := requireDependency("docker"); err != nil {
		return err
	}
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "docker", "info")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker is not reachable; start Docker and rerun bellows")
	}
	return nil
}

func runPassthrough(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func selectedWorkflows(flag string, defaults []string) []string {
	if flag != "" {
		return []string{flag}
	}
	return defaults
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
	return []string{filepath.Base(workflows[0].Path)}
}

func rel(root, path string) string {
	if out, err := filepath.Rel(root, path); err == nil {
		return out
	}
	return path
}

// Execute runs the root command.
func Execute(version string) error {
	return newRootCmd(version).Execute()
}
