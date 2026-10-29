package cmd

import (
	"fmt"
	"github.com/smeltery/bellows/internal/reporting/github"
	"github.com/smeltery/bellows/internal/reporting/session"
	"github.com/spf13/cobra"
	"os"
)

type reportingFlags struct {
	enabled, trusted bool
	pull             int
}

func addReportingFlags(cmd *cobra.Command) *reportingFlags {
	opts := &reportingFlags{}
	cmd.Flags().BoolVar(&opts.enabled, "report", false, "publish advisory bellows/verify status for this run")
	cmd.Flags().BoolVar(&opts.trusted, "trust-workstation", false, "explicitly trust this workstation and tested repository code")
	cmd.Flags().IntVar(&opts.pull, "pull-request", 0, "same-repository pull request number for live head/base validation")
	return opts
}
func (o *reportingFlags) create(repository string) (*session.Reporter, error) {
	if !o.enabled {
		if o.trusted || o.pull != 0 {
			return nil, fmt.Errorf("reporting options require --report")
		}
		return nil, nil
	}
	token := os.Getenv("BELLOWS_REPORT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("--report requires BELLOWS_REPORT_TOKEN with commit status write and pull request read permissions")
	}
	return session.New(github.Client{Token: token}, repository, o.pull, o.trusted)
}
