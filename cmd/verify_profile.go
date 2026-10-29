package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	out.Identity.Fingerprints = v.Fingerprints{Workflow: digest(prepared.Identity.Tree), Dependencies: digest(prepared.Identity.Tree), Config: digest(string(canonical)), Policy: digest(string(canonical))}
	lock, err := os.ReadFile(filepath.Join(prepared.Path, ".flox/env/manifest.lock"))
	if err != nil {
		return out, err
	}
	identity := out.Identity
	out.Current = func(ctx context.Context) (v.Identity, error) {
		current := identity
		environment, _ := json.Marshal(map[string]string{"PATH": os.Getenv("PATH"), "HOME": os.Getenv("HOME"), "TMPDIR": os.Getenv("TMPDIR")})
		current.Fingerprints.Environment = digest(string(environment))
		var err error
		current.Fingerprints.Toolchain, err = (flox.Executor{}).ToolchainForLock(ctx, lock)
		return current, err
	}
	out.Identity, err = out.Current(ctx)
	return out, err
}
