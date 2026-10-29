// Package cache reuses downloads, never installed dependencies or test results.
package cache

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	v "github.com/smeltery/bellows/internal/verification"
)

var ErrBusy = errors.New("cache metadata is busy")
var repository = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var digest = regexp.MustCompile(`^[0-9a-f]{64}$`)
var objectID = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

const marker = "bellows-npm-cache-v1\n"

type Lease struct {
	path, state       string
	temporary, closed bool
	mu                sync.Mutex
}

// Open falls back to a disposable store when a reusable entry is active or corrupt.
func Open(root string, id v.Identity, cold bool) (lease *Lease, err error) {
	if !repository.MatchString(id.Repository) || id.Platform == "" || !objectID.MatchString(id.Head) || !objectID.MatchString(id.Base) || !objectID.MatchString(id.Tree) || (id.Mode != v.ModeHead && id.Mode != v.ModeMerge) || (id.Executor != v.ExecutorFlox && id.Executor != v.ExecutorAct) {
		return nil, fmt.Errorf("invalid cache identity")
	}
	f := id.Fingerprints
	for _, value := range []string{f.Workflow, f.Config, f.Policy, f.Dependencies, f.Toolchain, f.Environment} {
		if !digest.MatchString(value) {
			return nil, fmt.Errorf("cache identity requires complete fingerprints")
		}
	}
	if id.Executor != v.ExecutorFlox {
		return nil, fmt.Errorf("npm download cache requires native execution; container mounts are unsupported")
	}
	if cold {
		return disposable("cold")
	}
	if err := privateDirectory(root); err != nil {
		return nil, err
	}
	if err := lock(root); err != nil {
		if errors.Is(err, ErrBusy) {
			return disposable("busy")
		}
		return nil, err
	}
	defer func() {
		unlock(root, &err)
		if err != nil && lease != nil {
			err = errors.Join(err, lease.Close())
			lease = nil
		}
	}()
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join([]string{id.Repository, id.Platform, f.Toolchain, f.Dependencies}, "\x00"))))
	path := filepath.Join(root, key)
	state := "reuse"
	if err := os.Mkdir(path, 0700); err == nil {
		state = "cold"
		if err = os.WriteFile(filepath.Join(path, ".owner"), []byte(marker), 0600); err != nil {
			return nil, err
		}
	} else if !os.IsExist(err) {
		return nil, err
	}
	if !owned(path) {
		return disposable("corrupt")
	}
	if err := os.Mkdir(filepath.Join(path, ".lease"), 0700); err != nil {
		if os.IsExist(err) {
			return disposable("busy")
		}
		return nil, err
	}
	if err := privateDirectory(filepath.Join(path, "npm")); err != nil {
		if cleanupErr := os.Remove(filepath.Join(path, ".lease")); cleanupErr != nil {
			return nil, cleanupErr
		}
		return disposable("corrupt")
	}
	return &Lease{path: path, state: state}, nil
}

func disposable(state string) (*Lease, error) {
	path, err := os.MkdirTemp("", "bellows-npm-")
	if err != nil {
		return nil, err
	}
	if err = os.Mkdir(filepath.Join(path, "npm"), 0700); err != nil {
		return nil, errors.Join(err, os.RemoveAll(path))
	}
	return &Lease{path: path, state: state, temporary: true}, nil
}

func (l *Lease) Env() []string {
	return []string{"npm_config_cache=" + filepath.Join(l.path, "npm"), "npm_config_logs_max=0"}
}

// State describes store availability, not a claim that npm found a cache hit.
func (l *Lease) State() string { return l.state }
func (l *Lease) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	if l.temporary {
		if err := os.RemoveAll(l.path); err != nil {
			return err
		}
	} else {
		now := time.Now()
		if err := os.Chtimes(l.path, now, now); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(l.path, ".lease")); err != nil {
			return err
		}
	}
	l.closed = true
	return nil
}

func privateDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("cache directory must be private and not a symlink")
	}
	return nil
}
func owned(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return false
	}
	owner, err := os.ReadFile(filepath.Join(path, ".owner"))
	return err == nil && string(owner) == marker
}
func lock(root string) error {
	if err := os.Mkdir(filepath.Join(root, ".lock"), 0700); err != nil {
		if os.IsExist(err) {
			return ErrBusy
		}
		return err
	}
	return nil
}

// Prune bounds inactive owned entries; active/stale leases are never broken.
func Prune(root string, maxEntries int, maxAge time.Duration) (err error) {
	if maxEntries < 0 || maxAge < 0 {
		return fmt.Errorf("invalid cache retention")
	}
	if err := privateDirectory(root); err != nil {
		return err
	}
	if err := lock(root); err != nil {
		return err
	}
	defer unlock(root, &err)
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var candidates []os.FileInfo
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if !digest.MatchString(entry.Name()) || !owned(path) {
			continue
		}
		if _, err := os.Lstat(filepath.Join(path, ".lease")); !os.IsNotExist(err) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		candidates = append(candidates, info)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ModTime().Before(candidates[j].ModTime()) })
	for i, entry := range candidates {
		if len(candidates)-i <= maxEntries && time.Since(entry.ModTime()) <= maxAge {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// BindDaemon prevents reusing a known daemon's BuildKit layers across repositories.
// It does not prove a daemon is fresh; cold Docker runs require a newly owned daemon.
func BindDaemon(root, daemonID, repo string) (err error) {
	if daemonID == "" || !repository.MatchString(repo) {
		return fmt.Errorf("invalid Docker cache binding")
	}
	if err := privateDirectory(root); err != nil {
		return err
	}
	if err := lock(root); err != nil {
		return err
	}
	defer unlock(root, &err)
	name := filepath.Join(root, fmt.Sprintf("%x.daemon", sha256.Sum256([]byte(daemonID))))
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte(repo)))
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		info, statErr := os.Lstat(name)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("invalid Docker cache binding file")
		}
		data, readErr := os.ReadFile(name)
		if readErr != nil {
			return readErr
		}
		if string(data) != expected {
			return fmt.Errorf("docker daemon is bound to another repository")
		}
		return nil
	}
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(expected)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func unlock(root string, err *error) {
	*err = errors.Join(*err, os.Remove(filepath.Join(root, ".lock")))
}
