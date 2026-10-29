package cmd

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	v "github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/verification/cache"
	verificationrun "github.com/smeltery/bellows/internal/verification/run"
)

func applyVerificationCache(preparedPath, root string, enabled, cold bool, out verificationrun.Plan) (verificationrun.Plan, error) {
	enabled = enabled || cold
	if out.Identity.Executor == v.ExecutorAct {
		if enabled {
			return out, fmt.Errorf("download-cache flags require a native profile; cold Docker runs require a new daemon")
		}
		daemon, ok := out.Executor.(interface{ DaemonID() string })
		if !ok {
			return out, fmt.Errorf("docker daemon identity is unavailable")
		}
		return out, cache.BindDaemon(root, daemon.DaemonID(), out.Identity.Repository)
	}
	dependencies, err := cache.Dependencies(preparedPath)
	if err != nil {
		return out, err
	}
	normalize := func(id v.Identity) v.Identity {
		id.Fingerprints.Dependencies = dependencies
		if enabled {
			id.Fingerprints.Environment = fmt.Sprintf("%x", sha256.Sum256([]byte(id.Fingerprints.Environment+"\x00npm-downloads-v1")))
		}
		return id
	}
	out.Identity = normalize(out.Identity)
	if current := out.Current; current != nil {
		out.Current = func(ctx context.Context) (v.Identity, error) { id, err := current(ctx); return normalize(id), err }
	}
	if !enabled {
		return out, nil
	}
	if !cold {
		if err = cache.Prune(root, 32, 7*24*time.Hour); err != nil && !errors.Is(err, cache.ErrBusy) {
			return out, err
		}
	}
	lease, err := cache.Open(root, out.Identity, cold)
	if err != nil {
		return out, err
	}
	out.Env = append(out.Env, lease.Env()...)
	closeExecutor := out.Close
	out.Close = func() error {
		err := lease.Close()
		if closeExecutor != nil {
			err = errors.Join(err, closeExecutor())
		}
		return err
	}
	return out, nil
}
