package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/smeltery/bellows/internal/changed"
	v "github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/verification/receipt"
)

type CachePlan struct {
	Identity  v.Identity
	Targets   []changed.Target
	Argv      map[string][]string
	Env       []string
	Cacheable map[string]bool
}

type Reuse interface {
	Lookup(CachePlan, string) (v.Result, bool, error)
	Save(Outcome) error
}

type successSeal struct {
	plan     []byte
	receipt  []byte
	executed []string
	path     string
}

// CacheEvidence is available only for an unchanged successful in-process outcome.
// It is a trusted orchestration capability, not isolation from hostile same-user code.
func (out Outcome) CacheEvidence() (CachePlan, []byte, []string, error) {
	var plan CachePlan
	if out.seal == nil || out.State != v.Passed || out.Path != out.seal.path {
		return plan, nil, nil, errors.New("outcome has no successful execution seal")
	}
	data, err := receipt.Encode(out.Receipt)
	if err != nil || !bytes.Equal(data, out.seal.receipt) {
		return plan, nil, nil, errors.New("sealed outcome was edited")
	}
	if err := json.Unmarshal(out.seal.plan, &plan); err != nil {
		return plan, nil, nil, err
	}
	return plan, bytes.Clone(data), slices.Clone(out.seal.executed), nil
}

func (p Plan) cachePlan() CachePlan {
	return CachePlan{Identity: p.Identity, Targets: p.Targets, Argv: p.Argv, Env: p.Env, Cacheable: p.Cacheable}
}

func copyCachePlan(data []byte) (CachePlan, error) {
	var plan CachePlan
	err := json.Unmarshal(data, &plan)
	return plan, err
}

func currentIdentity(ctx context.Context, plan Plan) error {
	if plan.Current == nil {
		return errors.New("result reuse requires a current identity resolver")
	}
	current, err := plan.Current(ctx)
	if err != nil {
		return err
	}
	if current != plan.Identity {
		return errors.New("verification identity changed during result reuse")
	}
	return ctx.Err()
}

func cachedResult(result v.Result, expected string, current v.Receipt) error {
	if result.ID != expected || result.Reuse == nil || result.State != v.Passed ||
		result.Reuse.ExpiresAt.Before(time.Now().UTC()) {
		return errors.New("cache did not return a current successful result")
	}
	current.Results = append(slices.Clone(current.Results), result)
	if err := receipt.Validate(current); err != nil {
		return fmt.Errorf("invalid cache provenance: %w", err)
	}
	return nil
}

func sealOutcome(ctx context.Context, out *Outcome, plan Plan, descriptor []byte, executed []string) error {
	for _, result := range out.Receipt.Results {
		if result.State != v.Passed || result.ExitCode == nil || *result.ExitCode != 0 {
			return errors.New("all results must pass for cache admission")
		}
	}
	if err := currentIdentity(ctx, plan); err != nil {
		return err
	}
	actual, err := json.Marshal(plan.cachePlan())
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, descriptor) {
		return errors.New("cache plan changed")
	}
	data, err := receipt.Encode(out.Receipt)
	if err != nil {
		return err
	}
	out.seal = &successSeal{plan: bytes.Clone(descriptor), receipt: data, executed: slices.Clone(executed), path: out.Path}
	return nil
}
