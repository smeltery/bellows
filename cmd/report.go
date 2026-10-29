package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/smeltery/bellows/internal/changed"
	"github.com/smeltery/bellows/internal/reporting/freshness"
	"github.com/smeltery/bellows/internal/reporting/github"
	"github.com/smeltery/bellows/internal/reporting/policy"
	"github.com/smeltery/bellows/internal/reporting/session"
	v "github.com/smeltery/bellows/internal/verification"
	"github.com/spf13/cobra"
	"os"
	"time"
)

type reportingFlags struct {
	enabled, trusted bool
	pull             int
	watchDuration    time.Duration
}

func currentWithRefs(root, base, head string, current policy.Resolver) policy.Resolver {
	return func(ctx context.Context) (v.Identity, error) {
		if current == nil {
			return v.Identity{}, fmt.Errorf("current toolchain identity unavailable")
		}
		identity, err := current(ctx)
		if err != nil {
			return v.Identity{}, err
		}
		refs, err := changed.Discover(root, changed.Options{Scope: changed.BaseHead, Base: base, Head: head})
		if err != nil {
			return v.Identity{}, err
		}
		identity.Head, identity.Base = refs.Head, refs.Base
		return identity, nil
	}
}

func addReportingFlags(cmd *cobra.Command) *reportingFlags {
	opts := &reportingFlags{}
	cmd.Flags().BoolVar(&opts.enabled, "report", false, "publish advisory bellows/verify status for this run")
	cmd.Flags().BoolVar(&opts.trusted, "trust-workstation", false, "explicitly trust this workstation and tested repository code")
	cmd.Flags().IntVar(&opts.pull, "pull-request", 0, "same-repository pull request number for live head/base validation")
	cmd.Flags().DurationVar(&opts.watchDuration, "watch-advisory", 0, "watch this reported run for stale inputs after completion (maximum 1h)")
	return opts
}
func (o *reportingFlags) create(repository string, current policy.Resolver) (*session.Reporter, error) {
	if !o.enabled {
		if o.trusted || o.pull != 0 || o.watchDuration != 0 {
			return nil, fmt.Errorf("reporting options require --report")
		}
		return nil, nil
	}
	if o.watchDuration < 0 || o.watchDuration > time.Hour {
		return nil, fmt.Errorf("advisory watch duration must be between zero and one hour")
	}
	token := os.Getenv("BELLOWS_REPORT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("--report requires BELLOWS_REPORT_TOKEN with commit status write and pull request read permissions")
	}
	return session.New(github.Client{Token: token}, repository, o.pull, o.trusted, current)
}

func (o *reportingFlags) watch(ctx context.Context, reporter *session.Reporter) error {
	if o.watchDuration == 0 {
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, o.watchDuration)
	defer cancel()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	err := freshness.Watch(bounded, ticker.C, reporter.Recheck)
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		return nil
	}
	return err
}

func newCapabilityCmd() *cobra.Command {
	var repository, branch, format string
	cmd := &cobra.Command{Use: "capabilities", Short: "Inspect advisory enforcement API availability without changing rules", Args: cobra.NoArgs}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if format != "json" && format != "text" {
			return fmt.Errorf("unsupported capability output format")
		}
		capability, probeErr := freshness.Probe(cmd.Context(), github.Client{Token: os.Getenv("BELLOWS_REPORT_TOKEN")}, repository, branch)
		var outputErr error
		if format == "json" {
			outputErr = json.NewEncoder(cmd.OutOrStdout()).Encode(capability)
		} else {
			_, outputErr = fmt.Fprintf(cmd.OutOrStdout(), "mode: advisory (no merge authorization)\nprotection API: %s\nbranch rules API: %s\n", capability.Protection.State, capability.Rules.State)
		}
		return errors.Join(probeErr, outputErr)
	}
	cmd.Flags().StringVar(&repository, "repository", "", "explicit GitHub owner/name")
	cmd.Flags().StringVar(&branch, "branch", "main", "target branch")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	return cmd
}
