package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dotbrains/bellows/internal/act"
	"github.com/dotbrains/bellows/internal/config"
	"github.com/dotbrains/bellows/internal/doctor"
	"github.com/dotbrains/bellows/internal/precommit"
	"github.com/dotbrains/bellows/internal/workflow"
	"github.com/spf13/cobra"
)

type cliOptions struct {
	repoRoot string
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
			cfg := config.Minimal(defaults, workflow.Labels(workflows))
			path := filepath.Join(root, config.FileName)
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
	return cmd
}

func newRunCmd(opts *cliOptions) *cobra.Command {
	var workflowRef string
	var job string
	var event string
	var precommitMode bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run configured GitHub Actions workflows locally",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot(opts.repoRoot)
			if err != nil {
				return err
			}
			if err := requireDependency("act"); err != nil {
				return err
			}
			if err := requireDocker(); err != nil {
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
						return fmt.Errorf("Bellows failed while running %s job %s\nRerun: bellows run --workflow %s --job %s", display, job, display, job)
					}
					return fmt.Errorf("Bellows failed while running %s\nRerun: bellows run --workflow %s", display, display)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&workflowRef, "workflow", "w", "", "workflow file name or path")
	cmd.Flags().StringVarP(&job, "job", "j", "", "job id to run")
	cmd.Flags().StringVar(&event, "event", "", "event name to pass to act")
	cmd.Flags().BoolVar(&precommitMode, "precommit", false, "run with pre-commit friendly output")
	return cmd
}

func newDoctorCmd(opts *cliOptions) *cobra.Command {
	return &cobra.Command{
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
			findings := doctor.Analyze(cfg, workflows)
			findings = append(dependencyFindings(), findings...)
			var hasError bool
			for _, finding := range findings {
				cmd.Printf("[%s] %s\n", finding.Level, finding.Message)
				if finding.Level == "error" {
					hasError = true
				}
			}
			if hasError {
				return errors.New("doctor found local compatibility issues")
			}
			return nil
		},
	}
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
	return &cobra.Command{
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
			cmd.Printf("event: %s\n", cfg.Event)
			cmd.Printf("default workflows: %s\n", strings.Join(cfg.DefaultWorkflows, ", "))
			for _, wf := range workflows {
				cmd.Printf("%s: %s\n", wf.Path, strings.Join(workflow.JobIDs(wf), ", "))
			}
			return nil
		},
	}
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
		findings = append(findings, doctor.Finding{Level: "warn", Message: "act was not found in PATH; bellows run requires act"})
	} else {
		findings = append(findings, doctor.Finding{Level: "ok", Message: "act is available"})
	}
	if _, err := exec.LookPath("docker"); err != nil {
		findings = append(findings, doctor.Finding{Level: "warn", Message: "docker was not found in PATH; bellows run requires Docker"})
		return findings
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		findings = append(findings, doctor.Finding{Level: "warn", Message: "Docker is installed but not reachable"})
		return findings
	}
	findings = append(findings, doctor.Finding{Level: "ok", Message: "Docker is reachable"})
	return findings
}

func requireDocker() error {
	if err := requireDependency("docker"); err != nil {
		return err
	}
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "docker", "info")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Docker is not reachable. Start Docker and rerun bellows")
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
