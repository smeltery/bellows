package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/smeltery/bellows/internal/changed"
	"github.com/smeltery/bellows/internal/config"
	v "github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/verification/flox"
	verificationrun "github.com/smeltery/bellows/internal/verification/run"
	"github.com/smeltery/bellows/internal/verification/worktree"
)

func buildNativeVerificationPlan(ctx context.Context, name string, prepared *worktree.Run) (verificationrun.Plan, error) {
	out := verificationrun.Plan{Identity: prepared.Identity, Env: []string{}}
	cfg, err := config.Load(prepared.Path)
	if err != nil {
		return out, err
	}
	if cfg.Secrets.Mode != "none" {
		return out, fmt.Errorf("native verification requires secrets.mode: none")
	}
	checks, argv, err := cfg.NativeChecks(name)
	if err != nil {
		return out, err
	}
	for _, check := range checks {
		out.Targets = append(out.Targets, changed.Target{Check: check})
	}
	out.Argv = argv
	out.Executor = flox.Executor{}
	out.Identity.Executor = v.ExecutorFlox
	out.Identity.Platform = runtime.GOOS + "/" + runtime.GOARCH
	canonical, err := json.Marshal(struct {
		Name   string
		Config *config.Config
	}{name, cfg})
	if err != nil {
		return out, err
	}
	digest := func(data string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(data))) }
	out.Identity.Fingerprints = v.Fingerprints{Workflow: digest(prepared.Identity.Tree), Dependencies: digest(prepared.Identity.Tree), Config: digest(string(canonical)), Policy: digest(string(canonical)), Environment: digest("[]")}
	out.Identity.Fingerprints.Toolchain, err = (flox.Executor{}).Toolchain(ctx, prepared.Path)
	return out, err
}
