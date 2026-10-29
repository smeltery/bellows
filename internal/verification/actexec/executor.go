// Package actexec requires an operator-owned, dedicated Linux Docker daemon.
package actexec

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/smeltery/bellows/internal/act"
	"github.com/smeltery/bellows/internal/config"
	v "github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/workflow"
)

var ErrUnsupported = errors.New("dedicated Docker execution unavailable")
var pinnedAction = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+@[0-9a-f]{40}$`)

type Executor struct {
	cfg                    *config.Config
	endpoint, lease, cache string
	images                 map[string]string
	fingerprint            string
	mu                     sync.Mutex
	closed                 bool
}

func New(ctx context.Context, cfg *config.Config, endpoint, leaseDir string) (*Executor, error) {
	if cfg == nil || endpoint == "" || leaseDir == "" {
		return nil, fmt.Errorf("%w: config, dedicated endpoint and shared lease directory are required", ErrUnsupported)
	}
	copied := *cfg
	copied.Runner.Map = map[string]string{}
	for label, image := range cfg.Runner.Map {
		copied.Runner.Map[label] = image
	}
	e := &Executor{cfg: &copied, endpoint: endpoint}
	identity, err := e.docker(ctx, "info", "--format", "{{.ID}} {{.OSType}}")
	fields := strings.Fields(identity)
	if err != nil || len(fields) != 2 || fields[1] != "linux" {
		return nil, fmt.Errorf("%w: Docker Linux daemon preflight failed", ErrUnsupported)
	}
	info, err := os.Lstat(leaseDir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("lease directory must exist, be private and not a symlink")
	}
	e.lease = filepath.Join(leaseDir, fmt.Sprintf("%x.lease", sha256.Sum256([]byte(fields[0]))))
	file, err := os.OpenFile(e.lease, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("dedicated daemon is leased: %w", err)
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(e.lease)
		return nil, err
	}

	if err == nil {
		err = e.empty(ctx)
	}
	if err == nil {
		e.cache, err = os.MkdirTemp("", "bellows-act-")
	}
	if err != nil {
		_ = os.Remove(e.lease)
		return nil, err
	}
	return e, nil
}

func (e *Executor) docker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"--host", e.endpoint}, args...)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker preflight or lifecycle command failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (e *Executor) empty(ctx context.Context) error {
	for _, args := range [][]string{{"container", "ls", "--all", "--quiet"}, {"volume", "ls", "--quiet"}, {"network", "ls", "--filter", "type=custom", "--format", "{{.ID}}"}} {
		output, err := e.docker(ctx, args...)
		if err != nil {
			return err
		}
		if output != "" {
			return fmt.Errorf("dedicated daemon has %s resources; operator cleanup required", args[0])
		}
	}
	return nil
}

// Toolchain fingerprints pre-pulled immutable image IDs; it never pulls mutable tags.
func (e *Executor) Toolchain(ctx context.Context, images []string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return "", fmt.Errorf("executor is closed")
	}
	e.fingerprint = ""
	cmd := exec.CommandContext(ctx, "act", "--version")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + e.cache}
	version, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("act version unavailable: %w", err)
	}
	docker, err := e.docker(ctx, "version", "--format", "{{.Client.Version}} {{.Server.Version}}")
	if err != nil {
		return "", err
	}
	names := append([]string(nil), images...)
	for _, image := range e.cfg.Runner.Map {
		names = append(names, image)
	}
	sort.Strings(names)
	e.images = map[string]string{}
	material := string(version) + "\n" + docker
	for _, name := range names {
		id, err := e.docker(ctx, "image", "inspect", name, "--format", "{{.Id}}")
		if err != nil || id == "" {
			return "", fmt.Errorf("required image is not available locally")
		}
		e.images[name] = id
		material += "\n" + name + "=" + id
	}
	e.fingerprint = fmt.Sprintf("%x", sha256.Sum256([]byte(material)))
	return e.fingerprint, nil
}

func (e *Executor) Run(ctx context.Context, request v.Request) (result v.Result, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	result = v.Result{ID: request.Check.ID, State: v.Error}
	start := time.Now()
	defer func() { result.DurationMS = time.Since(start).Milliseconds() }()
	if e.closed || request.RunID == "" || request.Check.ID == "" || request.Workflow == "" || request.Job == "" || e.fingerprint == "" || request.Identity.Executor != v.ExecutorAct || request.Identity.Fingerprints.Toolchain != e.fingerprint {
		return result, fmt.Errorf("invalid act execution request")
	}
	for _, entry := range request.Env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name == "" || name == "PATH" || name == "HOME" || name == "GH_TOKEN" || name == "GITHUB_TOKEN" || name == "ENV" || name == "BASH_ENV" || name == "NODE_OPTIONS" || strings.HasPrefix(name, "DOCKER") || strings.HasPrefix(name, "ACT") || strings.HasPrefix(name, "GIT") || strings.HasPrefix(name, "XDG") || strings.HasPrefix(name, "LD_") || strings.HasPrefix(name, "DYLD_") {
			return result, fmt.Errorf("reserved execution environment variable")
		}
	}
	if len(e.images) == 0 {
		return result, fmt.Errorf("toolchain must be resolved before execution")
	}
	if err = e.imagesCurrent(ctx); err != nil {
		return result, err
	}
	if err = e.empty(ctx); err != nil {
		return result, err
	}
	if _, statErr := os.Lstat(filepath.Join(request.Worktree, ".actrc")); !errors.Is(statErr, os.ErrNotExist) {
		return result, fmt.Errorf("worktree .actrc is unsupported for controlled verification")
	}
	path := request.Workflow
	if !filepath.IsAbs(path) {
		path = filepath.Join(request.Worktree, path)
	}
	wf, loadErr := workflow.Load(path)
	if loadErr != nil {
		return result, loadErr
	}
	if !slices.Contains(wf.Events, e.cfg.Event) {
		result.State = v.Unsupported
		return result, fmt.Errorf("%w: workflow event does not match", ErrUnsupported)
	}
	job, exists := wf.Jobs[request.Job]
	if !exists || job.UnsupportedMatrix || job.If != "" || job.Uses != "" || len(job.Steps) == 0 || len(job.RunsOn) == 0 {
		result.State = v.Unsupported
		return result, fmt.Errorf("%w: absent, conditional, reusable or empty job", ErrUnsupported)
	}
	if slices.Contains(e.cfg.Skip.Jobs, request.Job) {
		result.State = v.Skipped
		return result, nil
	}
	if len(request.Matrix) != len(job.Matrix) {
		result.State = v.Unsupported
		return result, fmt.Errorf("%w: matrix selection must be exact", ErrUnsupported)
	}
	for axis, values := range job.Matrix {
		if !slices.Contains(values, request.Matrix[axis]) {
			result.State = v.Unsupported
			return result, fmt.Errorf("%w: matrix selection does not exist", ErrUnsupported)
		}
	}
	labels := job.RunsOn
	for _, label := range labels {
		if strings.Contains(label, "${{") {
			if len(job.MatrixRunnerLabels()) == 0 {
				result.State = v.Unsupported
				return result, fmt.Errorf("%w: dynamic runner", ErrUnsupported)
			}
			continue
		}
		if e.cfg.Runner.Map[label] == "" || e.cfg.Runner.Map[label] == "-" {
			result.State = v.Unsupported
			return result, fmt.Errorf("%w: unmapped runner", ErrUnsupported)
		}
	}
	for _, label := range job.MatrixRunnerLabels() {
		if e.cfg.Runner.Map[label] == "" || e.cfg.Runner.Map[label] == "-" {
			result.State = v.Unsupported
			return result, fmt.Errorf("%w: unmapped matrix runner", ErrUnsupported)
		}
	}
	for _, candidate := range wf.Jobs {
		for _, image := range candidate.Images {
			if _, ok := e.images[image]; !ok {
				result.State = v.Unsupported
				return result, fmt.Errorf("%w: workflow image not fingerprinted", ErrUnsupported)
			}
		}
		for _, step := range candidate.Steps {
			if step.Uses != "" && !pinnedAction.MatchString(step.Uses) {
				result.State = v.Unsupported
				return result, fmt.Errorf("%w: action must reference a full commit SHA", ErrUnsupported)
			}
		}
	}
	pinned := *e.cfg
	pinned.Runner.Map = map[string]string{}
	for label, image := range e.cfg.Runner.Map {
		pinned.Runner.Map[label] = e.images[image]
	}
	args, err := act.Args(act.Options{Workflow: request.Workflow, Job: request.Job, Config: &pinned})
	if err != nil {
		return result, err
	}
	keys := make([]string, 0, len(request.Matrix))
	for key := range request.Matrix {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := request.Matrix[key]
		if key == "" || strings.ContainsAny(key, ":\r\n") || value == "" || strings.Contains(value, "${{") {
			return result, fmt.Errorf("invalid static matrix selector")
		}
		args = append(args, "--matrix", key+":"+value)
	}
	for _, entry := range request.Env {
		name, _, _ := strings.Cut(entry, "=")
		flag := "--env"
		if slices.Contains(e.cfg.Secrets.Allow, name) {
			flag = "--secret"
		}
		args = append(args, flag, name)
	}
	args = append(args, "--rm", "--pull=false", "--no-cache-server", "--action-cache-path", e.cache, "--container-daemon-socket", "/var/run/docker.sock")
	for _, flag := range []string{"--env-file", "--secret-file", "--input-file", "--var-file"} {
		args = append(args, flag, os.DevNull)
	}
	cmd := exec.CommandContext(ctx, "act", args...)
	cmd.Dir = request.Worktree
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + e.cache}, request.Env...)
	cmd.Env = append(cmd.Env, "DOCKER_HOST="+e.endpoint, "DOCKER_CONTEXT=", "DOCKER_TLS_VERIFY=", "DOCKER_CERT_PATH=")
	cmd.Stdout, cmd.Stderr = request.Stdout, request.Stderr
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 10 * time.Second
	if _, err = e.docker(ctx, "volume", "create", "--label", "bellows.owner="+filepath.Base(e.lease), "act-toolcache"); err != nil {
		return result, err
	}
	runErr := cmd.Run()
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	owner, inspectErr := e.docker(cleanupCtx, "volume", "inspect", "act-toolcache", "--format", `{{index .Labels "bellows.owner"}}`)
	if inspectErr != nil || owner != filepath.Base(e.lease) {
		return result, fmt.Errorf("act toolcache ownership changed")
	}
	if _, err = e.docker(cleanupCtx, "volume", "rm", "act-toolcache"); err != nil {
		return result, err
	}
	if err = e.empty(cleanupCtx); err != nil {
		return result, err
	}
	if err = e.imagesCurrent(cleanupCtx); err != nil {
		return result, err
	}
	if ctx.Err() != nil {
		result.State = v.Cancelled
		return result, ctx.Err()
	}
	if runErr == nil {
		zero := 0
		result.State = v.Passed
		result.ExitCode = &zero
		return result, nil
	}
	var exit *exec.ExitError
	if errors.As(runErr, &exit) && exit.ExitCode() > 0 {
		code := exit.ExitCode()
		result.State = v.Failed
		result.ExitCode = &code
		return result, nil
	}
	return result, runErr
}

// Close never deletes Docker resources. Dirty daemons retain their lease.
func (e *Executor) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := e.empty(ctx); err != nil {
		return err
	}
	if err := os.RemoveAll(e.cache); err != nil {
		return err
	}
	if err := os.Remove(e.lease); err != nil {
		return err
	}
	e.closed = true
	return nil
}

func (e *Executor) imagesCurrent(ctx context.Context) error {
	for name, expected := range e.images {
		actual, err := e.docker(ctx, "image", "inspect", name, "--format", "{{.Id}}")
		if err != nil || actual != expected {
			return fmt.Errorf("docker image identity changed or disappeared")
		}
	}
	return nil
}
