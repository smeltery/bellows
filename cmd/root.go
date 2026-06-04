package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dotbrains/bellows/internal/act"
	"github.com/dotbrains/bellows/internal/changed"
	"github.com/dotbrains/bellows/internal/compat"
	"github.com/dotbrains/bellows/internal/config"
	"github.com/dotbrains/bellows/internal/doctor"
	"github.com/dotbrains/bellows/internal/migrate"
	"github.com/dotbrains/bellows/internal/plan"
	"github.com/dotbrains/bellows/internal/policy"
	"github.com/dotbrains/bellows/internal/precommit"
	"github.com/dotbrains/bellows/internal/schemas"
	"github.com/dotbrains/bellows/internal/secrets"
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
	root.AddCommand(newConfigCmd(opts))
	root.AddCommand(newPolicyCmd(opts))
	root.AddCommand(newCompatCmd(opts))
	root.AddCommand(newHooksCmd(opts))
	root.AddCommand(newMigrateCmd(opts))
	root.AddCommand(newSecretsCmd(opts))
	root.AddCommand(newReleaseCmd(opts))
	root.AddCommand(newExplainCmd(opts))
	root.AddCommand(newCompletionCmd(root))

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
	var estimate bool
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
			if precommitMode && cfg.Precommit.Mode == "changed-files" && workflowRef == "" {
				files, err := changed.GitFiles(root)
				if err != nil {
					return err
				}
				selection := changed.Select(files, cfg.Precommit.Paths, cfg.DefaultWorkflows)
				if len(selection.Workflows) == 0 && len(selection.Jobs) == 0 {
					cmd.Println("bellows: no changed files matched local workflow rules")
					return nil
				}
				if len(selection.Workflows) > 0 {
					refs = selection.Workflows
				}
				if job == "" && len(selection.Jobs) == 1 {
					job = selection.Jobs[0]
				}
			}
			if len(refs) == 0 {
				return fmt.Errorf("no workflow selected; run bellows init or pass --workflow")
			}
			if listOnly {
				workflows, err := workflow.Discover(root)
				if err != nil {
					return err
				}
				executionPlan := plan.Build(cfg, workflows, plan.Options{
					WorkflowRefs:  refs,
					EventOverride: event,
					JobFilter:     job,
				})
				printExecutionPlan(cmd, executionPlan, estimate)
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
	cmd.Flags().BoolVar(&estimate, "estimate", false, "include local and avoided cloud run estimates with --list")
	return cmd
}

func newDoctorCmd(opts *cliOptions) *cobra.Command {
	var format string
	var strict bool
	var fix bool
	var fixDryRun bool
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
				fixes, fixedCfg, err := doctorFixes(cfg, workflows)
				if err != nil {
					return err
				}
				if format == "text" {
					for _, msg := range fixes {
						cmd.Printf("[fix] %s\n", msg)
					}
				}
				cfg = fixedCfg
				if fixDryRun && len(fixes) > 0 && fixes[0] != "no safe fixes available" && format == "text" {
					diff, err := doctorFixDiff(root, cfg)
					if err != nil {
						return err
					}
					if diff != "" {
						cmd.Print(diff)
					}
				}
				if !fixDryRun && len(fixes) > 0 && fixes[0] != "no safe fixes available" {
					if err := config.PatchSafeFixes(filepath.Join(root, config.FileName), cfg); err != nil {
						return err
					}
					cfg, err = config.Load(root)
					if err != nil {
						return err
					}
				}
			}
			findings := doctor.Analyze(cfg, workflows)
			findings = append(dependencyFindings(), findings...)
			if format == "json" {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(findings); err != nil {
					return err
				}
			} else if format == "github" {
				printGitHubFindings(cmd.OutOrStdout(), findings)
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
	cmd.Flags().StringVar(&format, "format", "text", "output format: text, json, or github")
	cmd.Flags().BoolVar(&strict, "strict", false, "fail when warnings are found")
	cmd.Flags().BoolVar(&fix, "fix", false, "apply safe .bellows.yml fixes before reporting diagnostics")
	cmd.Flags().BoolVar(&fixDryRun, "fix-dry-run", false, "preview safe .bellows.yml fixes without writing files")
	return cmd
}

func doctorFixDiff(root string, cfg *config.Config) (string, error) {
	path := filepath.Join(root, config.FileName)
	original, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp("", "bellows-fix-*.yml")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(original); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := config.PatchSafeFixes(tmp.Name(), cfg); err != nil {
		return "", err
	}
	out, err := exec.Command("diff", "-u", "--label", config.FileName, "--label", config.FileName+" (fixed)", path, tmp.Name()).CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() > 1 {
			return "", fmt.Errorf("creating fix diff: %w", err)
		}
	}
	return string(out), nil
}

func newCompatCmd(opts *cliOptions) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "compat",
		Short: "Report local GitHub Actions compatibility",
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
			items := compat.Report(cfg, workflows)
			switch format {
			case "json":
				return json.NewEncoder(cmd.OutOrStdout()).Encode(items)
			case "text":
				cmd.Printf("compatibility: %s\n", compat.Summary(items))
				for _, item := range items {
					cmd.Printf("[%s] %s", item.Status, item.Feature)
					if item.Workflow != "" {
						cmd.Printf(" %s", item.Workflow)
					}
					if item.Job != "" {
						cmd.Printf(" job %s", item.Job)
					}
					cmd.Printf(": %s\n", item.Message)
					if item.Suggestion != "" {
						cmd.Printf("      suggestion: %s\n", item.Suggestion)
					}
				}
				return nil
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	return cmd
}

func newHooksCmd(opts *cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Install or remove Bellows pre-commit hooks",
	}
	var stage string
	var force bool
	install := &cobra.Command{
		Use:   "install",
		Short: "Install the Bellows pre-commit hook",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot(opts.repoRoot)
			if err != nil {
				return err
			}
			path, err := precommit.WriteStage(root, stage, force)
			if err != nil {
				return err
			}
			cmd.Printf("wrote %s\n", rel(root, path))
			return nil
		},
	}
	install.Flags().StringVar(&stage, "stage", "pre-push", "pre-commit stage")
	install.Flags().BoolVar(&force, "force", false, "overwrite .pre-commit-config.yaml")
	cmd.AddCommand(install)
	cmd.AddCommand(&cobra.Command{
		Use:   "uninstall",
		Short: "Remove the generated Bellows pre-commit hook",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot(opts.repoRoot)
			if err != nil {
				return err
			}
			path, removed, err := precommit.Remove(root)
			if err != nil {
				return err
			}
			if removed {
				cmd.Printf("removed %s\n", rel(root, path))
			} else {
				cmd.Printf("no generated Bellows hook found at %s\n", rel(root, path))
			}
			return nil
		},
	})
	var statusFormat string
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Inspect Bellows pre-commit hook status",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot(opts.repoRoot)
			if err != nil {
				return err
			}
			status, err := precommit.Inspect(root)
			if err != nil {
				return err
			}
			status.Path = rel(root, status.Path)
			switch statusFormat {
			case "json":
				return json.NewEncoder(cmd.OutOrStdout()).Encode(status)
			case "text":
				cmd.Printf("path: %s\n", status.Path)
				cmd.Printf("exists: %t\n", status.Exists)
				cmd.Printf("installed: %t\n", status.Installed)
				cmd.Printf("bellows_only: %t\n", status.BellowsOnly)
				if status.Stage != "" {
					cmd.Printf("stage: %s\n", status.Stage)
				}
				return nil
			default:
				return fmt.Errorf("unsupported format %q", statusFormat)
			}
		},
	}
	statusCmd.Flags().StringVar(&statusFormat, "format", "text", "output format: text or json")
	cmd.AddCommand(statusCmd)
	return cmd
}

func newMigrateCmd(opts *cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Generate migration guidance for existing CI setups",
	}
	var format string
	gha := &cobra.Command{
		Use:   "github-actions",
		Short: "Plan a migration from existing GitHub Actions workflows",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot(opts.repoRoot)
			if err != nil {
				return err
			}
			workflows, err := workflow.Discover(root)
			if err != nil {
				return err
			}
			report := migrate.GitHubActions(root, workflows)
			switch format {
			case "json":
				return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
			case "text":
				text, err := migrate.Text(report)
				if err != nil {
					return err
				}
				cmd.Print(text)
				return nil
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	gha.Flags().StringVar(&format, "format", "text", "output format: text or json")
	cmd.AddCommand(gha)
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

func newSecretsCmd(opts *cliOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "secrets", Short: "Inspect local secret usage"}
	var format string
	audit := &cobra.Command{
		Use:   "audit",
		Short: "Show workflow secret references and local allowlist status",
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
			refs := secrets.Audit(cfg, workflows)
			switch format {
			case "json":
				return json.NewEncoder(cmd.OutOrStdout()).Encode(refs)
			case "text":
				if len(refs) == 0 {
					cmd.Println("no workflow secrets referenced")
					return nil
				}
				for _, ref := range refs {
					status := "blocked"
					if ref.Allowed {
						status = "allowed"
					}
					cmd.Printf("[%s] %s job %s step %s uses %s\n", status, ref.Workflow, ref.Job, ref.Step, ref.Secret)
				}
				return nil
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	audit.Flags().StringVar(&format, "format", "text", "output format: text or json")
	cmd.AddCommand(audit)
	return cmd
}

func newReleaseCmd(opts *cliOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "release", Short: "Run release readiness checks"}
	cmd.AddCommand(&cobra.Command{
		Use:   "check",
		Short: "Run the local release readiness suite",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot(opts.repoRoot)
			if err != nil {
				return err
			}
			steps := [][]string{
				{"go", "test", "./..."},
				{"golangci-lint", "run"},
				{"actionlint"},
				{"goreleaser", "check"},
				{"go", "run", ".", "config", "validate"},
				{"go", "run", ".", "policy"},
				{"go", "run", ".", "compat"},
				{"go", "run", ".", "run", "--list", "--estimate"},
				{"goreleaser", "release", "--snapshot", "--clean", "--skip=publish"},
			}
			for _, step := range steps {
				cmd.Printf("$ %s\n", strings.Join(step, " "))
				if err := runPassthrough(root, step[0], step[1:]...); err != nil {
					return err
				}
			}
			archive := fmt.Sprintf("dist/bellows_%s_%s.tar.gz", goEnv("GOOS"), goEnv("GOARCH"))
			smoke := filepath.Join(root, "dist", "smoke")
			_ = os.RemoveAll(smoke)
			if err := os.MkdirAll(smoke, 0o755); err != nil {
				return err
			}
			if err := runPassthrough(root, "tar", "-xzf", archive, "-C", "dist/smoke"); err != nil {
				return err
			}
			if err := runPassthrough(root, filepath.Join("dist", "smoke", "bellows"), "--version"); err != nil {
				return err
			}
			smokeCompletion := exec.Command(filepath.Join("dist", "smoke", "bellows"), "completion", "bash")
			smokeCompletion.Dir = root
			smokeCompletion.Stdout = io.Discard
			smokeCompletion.Stderr = os.Stderr
			return smokeCompletion.Run()
		},
	})
	return cmd
}

func newConfigCmd(opts *cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Validate and inspect Bellows configuration",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Validate .bellows.yml",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot(opts.repoRoot)
			if err != nil {
				return err
			}
			if _, err := config.Load(root); err != nil {
				return err
			}
			cmd.Println("config ok")
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "print-schema [bellows|doctor|explain]",
		Short: "Print a Bellows JSON Schema",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := "bellows"
			if len(args) > 0 {
				name = args[0]
			}
			return printEmbeddedSchema(cmd.OutOrStdout(), name)
		},
	})
	return cmd
}

func newPolicyCmd(opts *cliOptions) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Validate Bellows repository policy",
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
			violations := policy.Check(cfg, workflows)
			switch format {
			case "json":
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(violations); err != nil {
					return err
				}
			case "github":
				printGitHubPolicy(cmd.OutOrStdout(), violations)
			case "text":
				for _, violation := range violations {
					cmd.Printf("[%s] %s\n", violation.Level, violation.Message)
					if violation.Suggestion != "" {
						cmd.Printf("      suggestion: %s\n", violation.Suggestion)
					}
				}
				if len(violations) == 0 {
					cmd.Println("policy ok")
				}
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
			if len(violations) > 0 {
				return errors.New("policy violations found")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "output format: text, json, or github")
	return cmd
}

func newCompletionCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Generate shell completion scripts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			default:
				return fmt.Errorf("unsupported shell %q", args[0])
			}
		},
	}
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
			executionPlan := plan.Build(cfg, workflows, plan.Options{WorkflowRefs: cfg.DefaultWorkflows})
			if format == "json" {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(executionPlan)
			}
			if format != "text" {
				return fmt.Errorf("unsupported format %q", format)
			}
			printExecutionPlan(cmd, executionPlan, false)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	var changedFormat string
	changedCmd := &cobra.Command{
		Use:   "changed-files",
		Short: "Explain changed-file precommit selection",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot(opts.repoRoot)
			if err != nil {
				return err
			}
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}
			files, err := changed.GitFiles(root)
			if err != nil {
				return err
			}
			selection := changed.Select(files, cfg.Precommit.Paths, cfg.DefaultWorkflows)
			if changedFormat == "json" {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(selection)
			}
			if changedFormat != "text" {
				return fmt.Errorf("unsupported format %q", changedFormat)
			}
			cmd.Printf("changed files: %d\n", len(selection.Files))
			for _, file := range selection.Files {
				cmd.Printf("  %s\n", file)
			}
			if len(selection.Workflows) == 0 && len(selection.Jobs) == 0 {
				cmd.Println("selection: no matching path rules")
				return nil
			}
			cmd.Printf("workflows: %s\n", strings.Join(selection.Workflows, ", "))
			if len(selection.Jobs) > 0 {
				cmd.Printf("jobs: %s\n", strings.Join(selection.Jobs, ", "))
			}
			return nil
		},
	}
	changedCmd.Flags().StringVar(&changedFormat, "format", "text", "output format: text or json")
	cmd.AddCommand(changedCmd)
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

func doctorFixes(cfg *config.Config, workflows []workflow.Workflow) ([]string, *config.Config, error) {
	fixed := *cfg
	fixed.Runner.Map = map[string]string{}
	for label, image := range cfg.Runner.Map {
		fixed.Runner.Map[label] = image
	}
	fixed.Skip.Jobs = append([]string(nil), cfg.Skip.Jobs...)
	var messages []string
	for _, label := range workflow.Labels(workflows) {
		if fixed.Runner.Map == nil {
			fixed.Runner.Map = map[string]string{}
		}
		if fixed.Runner.Map[label] != "" {
			continue
		}
		image, ok := config.DefaultRunnerImage(label)
		if !ok {
			continue
		}
		fixed.Runner.Map[label] = image
		messages = append(messages, fmt.Sprintf("mapped runner %s to %s", label, image))
	}
	seenSkip := map[string]bool{}
	for _, job := range fixed.Skip.Jobs {
		seenSkip[job] = true
	}
	for _, job := range workflow.CloudLikeJobIDs(workflows) {
		if seenSkip[job] {
			continue
		}
		fixed.Skip.Jobs = append(fixed.Skip.Jobs, job)
		messages = append(messages, fmt.Sprintf("added %s to skip.jobs", job))
	}
	if len(messages) == 0 {
		return []string{"no safe fixes available"}, &fixed, nil
	}
	return messages, &fixed, nil
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

func printExecutionPlan(cmd *cobra.Command, executionPlan plan.Plan, estimate bool) {
	cmd.Printf("event: %s\n", executionPlan.Event)
	cmd.Printf("default workflows: %s\n", strings.Join(executionPlan.DefaultWorkflows, ", "))
	for _, wf := range executionPlan.Workflows {
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
	if estimate {
		cmd.Printf("estimate: %d runnable jobs, %d skipped jobs, %d avoided cloud jobs\n",
			executionPlan.Estimate.RunnableJobs,
			executionPlan.Estimate.SkippedJobs,
			executionPlan.Estimate.EstimatedCloudSaves,
		)
	}
}

func printGitHubFindings(w io.Writer, findings []doctor.Finding) {
	for _, finding := range findings {
		if finding.Level != "warn" && finding.Level != "error" {
			continue
		}
		level := "warning"
		if finding.Level == "error" {
			level = "error"
		}
		fmt.Fprintf(w, "::%s file=%s,title=%s::%s\n", level, finding.Workflow, finding.Code, escapeAnnotation(finding.Message))
	}
}

func printGitHubPolicy(w io.Writer, violations []policy.Violation) {
	for _, violation := range violations {
		level := "warning"
		if violation.Level == "error" {
			level = "error"
		}
		fmt.Fprintf(w, "::%s file=%s,title=%s::%s\n", level, violation.Workflow, violation.Code, escapeAnnotation(violation.Message))
	}
}

func escapeAnnotation(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	value = strings.ReplaceAll(value, "\r", "%0D")
	value = strings.ReplaceAll(value, "\n", "%0A")
	return value
}

func printEmbeddedSchema(w io.Writer, name string) error {
	var data []byte
	switch name {
	case "bellows", "bellows.schema.json":
		data = schemas.Bellows
	case "doctor", "doctor.schema.json":
		data = schemas.Doctor
	case "explain", "explain.schema.json":
		data = schemas.Explain
	default:
		return fmt.Errorf("unknown schema %q", name)
	}
	_, err := w.Write(data)
	if err == nil && len(data) > 0 && data[len(data)-1] != '\n' {
		_, err = io.WriteString(w, "\n")
	}
	return err
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

func goEnv(name string) string {
	out, err := exec.Command("go", "env", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
