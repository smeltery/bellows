package flox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/smeltery/bellows/internal/verification"
)

type Executor struct {
	Binary string
}

func (e Executor) binary() string {
	if e.Binary != "" {
		return e.Binary
	}
	return "flox"
}

func environment(explicit []string) ([]string, error) {
	values := map[string]string{}
	for _, key := range []string{"PATH", "HOME", "TMPDIR"} {
		if value := os.Getenv(key); value != "" {
			values[key] = value
		}
	}
	for _, entry := range explicit {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" || strings.ContainsRune(entry, '\x00') {
			return nil, fmt.Errorf("invalid verification environment entry")
		}
		switch key {
		case "GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "BELLOWS_REPORT_TOKEN":
			return nil, fmt.Errorf("reporting credential variable %s is not allowed in native checks", key)
		}
		values[key] = value
	}
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	sort.Strings(out)
	return out, nil
}

func locked(root string) ([]byte, error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("verification worktree must be absolute")
	}
	data, err := os.ReadFile(filepath.Join(root, ".flox", "env", "manifest.lock"))
	if err != nil {
		return nil, fmt.Errorf("read committed Flox lock: %w", err)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("flox lock is not valid JSON")
	}
	return data, nil
}

func (e Executor) Toolchain(ctx context.Context, root string) (string, error) {
	lock, err := locked(root)
	if err != nil {
		return "", err
	}
	return e.ToolchainForLock(ctx, lock)
}

// ToolchainForLock rereads the host executable using immutable committed lock data.
func (e Executor) ToolchainForLock(ctx context.Context, lock []byte) (string, error) {
	if !json.Valid(lock) {
		return "", fmt.Errorf("flox lock is not valid JSON")
	}
	var err error
	cmd := exec.CommandContext(ctx, e.binary(), "--version")
	cmd.Env, err = environment(nil)
	if err != nil {
		return "", err
	}
	boundProcess(cmd)
	version, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("inspect Flox version: %w", err)
	}
	if len(strings.TrimSpace(string(version))) == 0 {
		return "", fmt.Errorf("flox returned an empty version")
	}
	digest := sha256.New()
	digest.Write(lock)
	digest.Write([]byte{0})
	digest.Write(version)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func (e Executor) Run(ctx context.Context, request verification.Request) (result verification.Result, err error) {
	started := time.Now()
	result = verification.Result{ID: request.Check.ID, State: verification.Error}
	defer func() { result.DurationMS = time.Since(started).Milliseconds() }()
	if request.Check.ID == "" || request.RunID == "" || request.Identity.Executor != verification.ExecutorFlox || len(request.Argv) == 0 || request.Argv[0] == "" || request.Workflow != "" || request.Job != "" || len(request.Matrix) != 0 {
		return result, fmt.Errorf("native execution requires a run/check identity and argv, without workflow selectors")
	}
	if request.Identity.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		result.State = verification.Unsupported
		return result, fmt.Errorf("native platform does not match this host")
	}
	lock, err := locked(request.Worktree)
	if err != nil {
		return result, err
	}
	fingerprint, err := e.Toolchain(ctx, request.Worktree)
	if ctx.Err() != nil {
		result.State = verification.Cancelled
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if fingerprint != request.Identity.Fingerprints.Toolchain {
		return result, fmt.Errorf("flox toolchain identity changed before verification")
	}
	env, err := environment(request.Env)
	if err != nil {
		return result, err
	}
	cmd := exec.CommandContext(ctx, e.binary(), append([]string{"activate", "--"}, request.Argv...)...)
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = request.Worktree, env, request.Stdout, request.Stderr
	boundProcess(cmd)
	runErr := cmd.Run()
	if ctx.Err() != nil {
		result.State = verification.Cancelled
		return result, nil
	}
	current, err := locked(request.Worktree)
	if err != nil {
		return result, err
	}
	if string(current) != string(lock) {
		return result, fmt.Errorf("flox lock changed during verification; update and commit it before retrying")
	}
	if runErr != nil {
		var exit *exec.ExitError
		if !errors.As(runErr, &exit) {
			return result, fmt.Errorf("execute native check with Flox: %w", runErr)
		}
		result.State = verification.Failed
		if code := exit.ExitCode(); code >= 0 {
			result.ExitCode = &code
		} else {
			result.State = verification.Error
		}
		return result, nil
	}
	code := 0
	result.State, result.ExitCode = verification.Passed, &code
	return result, nil
}

func boundProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		} else {
			return err
		}
	}
	cmd.WaitDelay = time.Second
}
