package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/smeltery/bellows/internal/changed"
	"github.com/smeltery/bellows/internal/config"
	"github.com/smeltery/bellows/internal/plan"
	"github.com/smeltery/bellows/internal/policy"
	"github.com/smeltery/bellows/internal/runner"
	v "github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/verification/actexec"
	"github.com/smeltery/bellows/internal/verification/resultcache"
	verificationrun "github.com/smeltery/bellows/internal/verification/run"
	"github.com/smeltery/bellows/internal/verification/worktree"
	"github.com/smeltery/bellows/internal/workflow"
	"github.com/spf13/cobra"
)

func newVerifyCmd(opts *cliOptions) *cobra.Command {
	var repository, base, head, mode, daemon, format, artifacts, leases, profile string
	var downloadCache, coldCache, reuseResults, forceRerun bool
	cmd := &cobra.Command{Use: "verify", Short: "Verify committed changes locally and save an advisory receipt", Args: cobra.NoArgs}
	report := addReportingFlags(cmd)
	cmd.AddCommand(newCapabilityCmd())
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_][A-Za-z0-9_.-]*$`).MatchString(repository) || base == "" || (profile == "" && !verificationDaemon(daemon)) || (profile != "" && cmd.Flags().Changed("daemon")) {
			return fmt.Errorf("verify requires --repository owner/name, --base and either --profile or --daemon with a dedicated Unix socket or loopback TCP endpoint")
		}
		if (format != "text" && format != "json") || (mode != v.ModeMerge && mode != v.ModeHead) {
			return fmt.Errorf("verify requires --format text|json and --mode merge|head")
		}
		if profile == "" && (downloadCache || coldCache) {
			return fmt.Errorf("download-cache flags require --profile; cold Docker runs require a new daemon")
		}
		if reuseResults && profile == "" {
			return fmt.Errorf("--reuse-results requires --profile")
		}
		if forceRerun && !reuseResults {
			return fmt.Errorf("--force-rerun requires --reuse-results")
		}
		if report.enabled && mode != v.ModeMerge {
			return fmt.Errorf("advisory reporting requires merge verification mode")
		}
		root, err := repoRoot(opts.repoRoot)
		if err != nil {
			return err
		}
		cache, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		directories := map[string]*string{"receipts": &artifacts}
		if profile == "" {
			directories["leases"] = &leases
		}
		for name, value := range directories {
			if *value == "" {
				*value = filepath.Join(cache, "bellows", name)
			}
			if err := os.MkdirAll(*value, 0700); err != nil {
				return err
			}
			info, err := os.Lstat(*value)
			if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
				return fmt.Errorf("%s directory must be private and not a symlink", name)
			}
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		var reuse verificationrun.Reuse
		if reuseResults {
			store, cacheErr := resultcache.Open(filepath.Join(cache, "bellows", "results"), root)
			if cacheErr != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: result cache unavailable; executing checks")
			} else {
				reuse = store
			}
		}
		var current func(context.Context) (v.Identity, error)
		reporter, err := report.create(repository, func(ctx context.Context) (v.Identity, error) {
			if current == nil {
				return v.Identity{}, fmt.Errorf("current execution identity unavailable")
			}
			return current(ctx)
		})
		if err != nil {
			return err
		}
		var start func(context.Context, v.Receipt) error
		if reporter != nil {
			start = reporter.Start
		}
		out, runErr := verificationrun.Execute(ctx, verificationrun.Options{
			Worktree: worktree.Options{Root: root, Repository: repository, Base: base, Head: head, Mode: mode}, Artifacts: artifacts,
			Stdout: cmd.ErrOrStderr(), Stderr: cmd.ErrOrStderr(), Start: start, Reuse: reuse, ForceRerun: forceRerun,
			Build: func(ctx context.Context, prepared *worktree.Run) (verificationrun.Plan, error) {
				var built verificationrun.Plan
				var err error
				if profile != "" {
					built, err = buildNativeVerificationPlan(ctx, profile, prepared)
				} else {
					built, err = buildVerificationPlan(ctx, root, daemon, leases, prepared)
				}
				if err == nil {
					built, err = applyVerificationCache(prepared.Path, filepath.Join(cache, "bellows", "downloads"), downloadCache, coldCache, built)
				}
				current = currentWithRefs(root, base, head, built.Current)
				return built, err
			},
		})
		if reporter != nil {
			if out.Path == "" {
				runErr = errors.Join(runErr, fmt.Errorf("cannot publish an unpersisted verification outcome"))
			} else {
				reportCtx, stop := context.WithTimeout(context.Background(), 20*time.Second)
				reportErr := reporter.Finish(reportCtx, out.Receipt)
				stop()
				runErr = errors.Join(runErr, reportErr)
				if reportErr == nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "updated advisory bellows/verify for %s %s\n", repository, out.Receipt.Identity.Head)
					if watchErr := report.watch(ctx, reporter); watchErr != nil {
						out.State = v.Error
						runErr = errors.Join(runErr, watchErr)
					}
				}
			}
		}
		if format == "json" {
			err = json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		} else {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "verification: %s\nreceipt: %s\n", out.State, out.Path)
		}
		if runErr == nil && out.State != v.Passed {
			runErr = fmt.Errorf("verification did not pass: %s", out.State)
		}
		return errors.Join(runErr, err)
	}
	cmd.Flags().StringVar(&repository, "repository", "", "explicit GitHub owner/name identity")
	cmd.Flags().StringVar(&base, "base", "", "target branch or commit")
	cmd.Flags().StringVar(&head, "head", "HEAD", "source branch or commit")
	cmd.Flags().StringVar(&mode, "mode", v.ModeMerge, "input mode: merge or head")
	cmd.Flags().BoolVar(&reuseResults, "reuse-results", false, "reuse eligible successful native checks from the private local result store")
	cmd.Flags().BoolVar(&forceRerun, "force-rerun", false, "execute every check despite --reuse-results and refresh eligible results")
	cmd.Flags().BoolVar(&downloadCache, "download-cache", false, "reuse repository-scoped npm downloads for native profiles")
	cmd.Flags().BoolVar(&coldCache, "cold-cache", false, "use a disposable empty npm download store for native profiles")
	cmd.Flags().StringVar(&profile, "profile", "", "explicit native Flox verification profile (without --daemon)")
	cmd.Flags().StringVar(&daemon, "daemon", "", "operator-owned dedicated Linux Docker socket URL")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	cmd.Flags().StringVar(&artifacts, "artifacts", "", "private receipt directory (default: user cache/bellows/receipts)")
	cmd.Flags().StringVar(&leases, "leases", "", "shared private daemon lease directory (default: user cache/bellows/leases)")
	return cmd
}

func verificationDaemon(value string) bool {
	endpoint, err := url.Parse(value)
	if err != nil || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || endpoint.RawPath != "" {
		return false
	}
	if endpoint.Scheme == "unix" {
		return endpoint.Host == "" && filepath.IsAbs(endpoint.Path) && endpoint.Path != "/"
	}
	port, err := strconv.Atoi(endpoint.Port())
	return endpoint.Scheme == "tcp" && endpoint.Path == "" && (endpoint.Hostname() == "127.0.0.1" || endpoint.Hostname() == "::1") && err == nil && port > 0 && port <= 65535
}

func buildVerificationPlan(ctx context.Context, root, daemon, leases string, prepared *worktree.Run) (verificationrun.Plan, error) {
	out := verificationrun.Plan{Identity: prepared.Identity, Env: []string{}}
	cfg, err := config.Load(prepared.Path)
	if err != nil {
		return out, err
	}
	if cfg.Secrets.Mode != "none" || len(cfg.DefaultWorkflows) == 0 {
		return out, fmt.Errorf("verify requires default_workflows and secrets.mode: none")
	}
	var workflows []workflow.Workflow
	for _, ref := range cfg.DefaultWorkflows {
		path, err := filepath.EvalSymlinks(workflow.Resolve(prepared.Path, ref))
		if err != nil {
			return out, err
		}
		relative, err := filepath.Rel(prepared.Path, path)
		if err != nil || !strings.HasPrefix(filepath.ToSlash(relative), ".github/workflows/") {
			return out, fmt.Errorf("verification workflow must be inside .github/workflows")
		}
		wf, err := workflow.Load(path)
		if err != nil {
			return out, err
		}
		wf.Path = filepath.ToSlash(relative)
		workflows = append(workflows, wf)
	}
	for _, job := range workflow.CloudLikeJobIDs(workflows) {
		if !slices.Contains(cfg.Skip.Jobs, job) {
			return out, fmt.Errorf("cloud-like job %s must be skipped for verification", job)
		}
	}
	for _, violation := range policy.Check(cfg, workflows) {
		if violation.Level == "error" {
			return out, fmt.Errorf("verification policy: %s", violation.Message)
		}
	}
	targets, err := plan.VerificationTargets(workflows, cfg.Skip.Jobs)
	if err != nil {
		return out, err
	}
	changes, err := changed.Discover(root, changed.Options{Scope: changed.BaseHead, Base: prepared.Identity.Base, Head: prepared.Identity.Head})
	if err != nil {
		return out, err
	}
	selected, err := changed.SelectRequired(changes, targets, changed.RequiredOptions{Rules: cfg.Precommit.Paths})
	if err != nil {
		return out, err
	}
	out.Targets = selected.Targets
	mappings := map[string]string{}
	for _, label := range workflow.Labels(workflows) {
		if image, ok := runner.Resolve(label, cfg.Runner); ok {
			mappings[label] = image
		}
	}
	cfg.Runner.Map = mappings
	if cfg.Docker.Platform == "" {
		cfg.Docker.Platform = "linux/" + runtime.GOARCH
	}
	out.Identity.Executor, out.Identity.Platform = v.ExecutorAct, cfg.Docker.Platform
	canonical, err := json.Marshal(cfg)
	if err != nil {
		return out, err
	}
	digest := func(data string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(data))) }
	out.Identity.Fingerprints = v.Fingerprints{Workflow: digest(prepared.Identity.Tree), Dependencies: digest(prepared.Identity.Tree), Config: digest(string(canonical)), Policy: digest(string(canonical)), Environment: digest("[]")}
	executor, err := actexec.New(ctx, cfg, daemon, leases)
	if err != nil {
		return out, err
	}
	out.Executor, out.Close = executor, executor.Close
	var images []string
	for _, wf := range workflows {
		for _, job := range wf.Jobs {
			images = append(images, job.Images...)
		}
	}
	out.Identity.Fingerprints.Toolchain, err = executor.Toolchain(ctx, images)
	identity := out.Identity
	out.Current = func(ctx context.Context) (v.Identity, error) {
		fresh := identity
		var err error
		fresh.Fingerprints.Toolchain, err = executor.Toolchain(ctx, images)
		return fresh, err
	}
	return out, err
}
