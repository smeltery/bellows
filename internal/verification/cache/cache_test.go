package cache

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	v "github.com/smeltery/bellows/internal/verification"
)

func identity() v.Identity {
	value := strings.Repeat("a", 64)
	return v.Identity{Repository: "owner/project", Head: strings.Repeat("1", 40), Base: strings.Repeat("2", 40), Tree: strings.Repeat("3", 40), Mode: v.ModeMerge, Executor: v.ExecutorFlox, Platform: "linux/amd64", Fingerprints: v.Fingerprints{Workflow: value, Config: value, Policy: value, Dependencies: value, Toolchain: value, Environment: value}}
}
func root(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
func open(t *testing.T, path string, id v.Identity, cold bool) *Lease {
	t.Helper()
	lease, err := Open(path, id, cold)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lease.Close(); err != nil {
			t.Error(err)
		}
	})
	return lease
}
func TestScopeAndColdBypass(t *testing.T) {
	path := root(t)
	id := identity()
	first := open(t, path, id, false)
	if first.State() != "cold" {
		t.Fatal(first.State())
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	id.Head = strings.Repeat("4", 40)
	again := open(t, path, id, false)
	if again.path != first.path || again.State() != "reuse" {
		t.Fatal("commit change invalidated downloads")
	}
	cold := open(t, path, id, true)
	if !cold.temporary || cold.path == again.path {
		t.Fatal("cold reused store")
	}
	for _, change := range []func(*v.Identity){func(i *v.Identity) { i.Repository = "other/project" }, func(i *v.Identity) { i.Platform = "darwin/arm64" }, func(i *v.Identity) { i.Fingerprints.Toolchain = strings.Repeat("b", 64) }, func(i *v.Identity) { i.Fingerprints.Dependencies = strings.Repeat("c", 64) }} {
		changed := id
		change(&changed)
		other := open(t, path, changed, false)
		if other.path == again.path {
			t.Fatal("incompatible scope reused")
		}
	}
	if len(again.Env()) != 2 || !strings.HasPrefix(again.Env()[0], "npm_config_cache=") {
		t.Fatal(again.Env())
	}
}
func TestConcurrentBusyFallbackAndActivePruning(t *testing.T) {
	path := root(t)
	active := open(t, path, identity(), false)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lease, err := Open(path, identity(), false)
			if err != nil {
				t.Error(err)
				return
			}
			if !lease.temporary || lease.State() != "busy" {
				t.Error("shared active entry")
			}
			if err = lease.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := Prune(path, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(active.path); err != nil {
		t.Fatal("pruned active lease")
	}
	if err := active.Close(); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(path, "unrelated")
	if err := os.WriteFile(unrelated, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Prune(path, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(active.path); !os.IsNotExist(err) {
		t.Fatal("inactive entry retained")
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatal("unowned file pruned")
	}
}
func TestRetentionAndCorruption(t *testing.T) {
	path := root(t)
	id := identity()
	old := open(t, path, id, false)
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(old.path, past, past); err != nil {
		t.Fatal(err)
	}
	id.Platform = "darwin/arm64"
	recent := open(t, path, id, false)
	if err := recent.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Prune(path, 1, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old.path); !os.IsNotExist(err) {
		t.Fatal("old entry retained")
	}
	outside := root(t)
	keep := filepath.Join(outside, "keep")
	if err := os.WriteFile(keep, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(recent.path, "npm")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(recent.path, "npm")); err != nil {
		t.Fatal(err)
	}
	rebuilt := open(t, path, id, false)
	if rebuilt.State() != "corrupt" || !rebuilt.temporary {
		t.Fatal("corrupt cache reused")
	}
	if err := Prune(path, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("followed symlink during prune")
	}
}
func TestRejectInvalidAndBindDaemon(t *testing.T) {
	path := root(t)
	id := identity()
	id.Fingerprints.Environment = ""
	if _, err := Open(path, id, false); err == nil {
		t.Fatal("partial identity accepted")
	}
	id = identity()
	id.Executor = v.ExecutorAct
	if _, err := Open(path, id, false); err == nil {
		t.Fatal("container cache claimed without mounts")
	}
	link := filepath.Join(root(t), "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(link, identity(), false); err == nil {
		t.Fatal("symlink root accepted")
	}
	if err := BindDaemon(path, "daemon", "owner/project"); err != nil {
		t.Fatal(err)
	}
	if err := BindDaemon(path, "daemon", "owner/project"); err != nil {
		t.Fatal(err)
	}
	if err := BindDaemon(path, "daemon", "other/project"); err == nil {
		t.Fatal("cross-repository daemon reuse")
	}
	if err := os.Mkdir(filepath.Join(path, ".lock"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := Prune(path, 1, time.Hour); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	fallback := open(t, path, identity(), false)
	if fallback.State() != "busy" {
		t.Fatal(fallback.State())
	}
}
