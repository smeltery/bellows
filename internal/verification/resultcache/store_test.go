package resultcache

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smeltery/bellows/internal/changed"
	v "github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/verification/run"
	"github.com/smeltery/bellows/internal/verification/worktree"
)

type executor func(context.Context, v.Request) (v.Result, error)

func (f executor) Run(ctx context.Context, r v.Request) (v.Result, error) { return f(ctx, r) }
func fixture(t *testing.T) (run.Options, *Store, *int) {
	t.Helper()
	repo := t.TempDir()
	git := func(args ...string) {
		command := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
		command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
		if data, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", data, err)
		}
	}
	git("init", "-b", "main")
	git("commit", "--allow-empty", "-m", "fixture")
	root := filepath.Join(t.TempDir(), "results")
	store, err := Open(root, repo)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	opts := run.Options{Worktree: worktree.Options{Root: repo, Repository: "example/repo", Base: "main", Head: "main", Mode: v.ModeMerge}, Artifacts: t.TempDir()}
	if err := os.Chmod(opts.Artifacts, 0700); err != nil {
		t.Fatal(err)
	}
	opts.Build = func(_ context.Context, w *worktree.Run) (run.Plan, error) {
		id := w.Identity
		id.Executor = v.ExecutorFlox
		id.Platform = "linux/amd64"
		hash := strings.Repeat("a", 64)
		id.Fingerprints = v.Fingerprints{Workflow: hash, Config: hash, Policy: hash, Dependencies: hash, Toolchain: hash, Environment: hash}
		return run.Plan{Identity: id, Current: func(context.Context) (v.Identity, error) { return id, nil }, Targets: []changed.Target{{Check: v.Check{ID: "pure", Required: true, Needs: []string{}}}}, Argv: map[string][]string{"pure": {"check"}}, Env: []string{"MODE=one", "MODE=two"}, Cacheable: map[string]bool{"pure": true}, Executor: executor(func(_ context.Context, r v.Request) (v.Result, error) {
			calls++
			zero := 0
			return v.Result{ID: r.Check.ID, State: v.Passed, ExitCode: &zero}, nil
		})}, nil
	}
	return opts, store, &calls
}
func success(t *testing.T, opts run.Options) run.Outcome {
	t.Helper()
	out, err := run.Execute(context.Background(), opts)
	if err != nil || out.State != v.Passed {
		t.Fatalf("execute: %v state=%s", err, out.State)
	}
	return out
}
func descriptor(t *testing.T, out run.Outcome) run.CachePlan {
	t.Helper()
	plan, _, _, err := out.CacheEvidence()
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
func TestAttestedReuseAndForceRerun(t *testing.T) {
	opts, store, calls := fixture(t)
	opts.Reuse = store
	first := success(t, opts)
	second := success(t, opts)
	if *calls != 1 || first.Receipt.RunID == second.Receipt.RunID || second.Receipt.Results[0].Reuse == nil || second.Receipt.Results[0].DurationMS != 0 {
		t.Fatalf("calls%d first%+v second%+v", *calls, first, second)
	}
	original := *second.Receipt.Results[0].Reuse
	third := success(t, opts)
	if *third.Receipt.Results[0].Reuse != original {
		t.Fatal("reuse slid origin expiry")
	}
	opts.ForceRerun = true
	fourth := success(t, opts)
	if *calls != 2 || fourth.Receipt.Results[0].Reuse != nil {
		t.Fatal("force rerun did not execute")
	}
	if got, hit, err := store.Lookup(descriptor(t, first), "pure"); err != nil || !hit || *got.Reuse != original {
		t.Fatal("immutable entry overwritten", err)
	}
}
func TestRejectImportedEditedAndUnsuccessfulOutcomes(t *testing.T) {
	opts, store, _ := fixture(t)
	out := success(t, opts)
	encoded, _ := json.Marshal(out)
	var imported run.Outcome
	if err := json.Unmarshal(encoded, &imported); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(imported); err == nil {
		t.Fatal("imported outcome trusted")
	}
	out.Receipt.Identity.Head = strings.Repeat("f", 40)
	if err := store.Save(out); err == nil {
		t.Fatal("edited outcome trusted")
	}
	build := opts.Build
	for _, kind := range []string{"failed", "cleanup", "storage"} {
		t.Run(kind, func(t *testing.T) {
			opts.Build = func(ctx context.Context, w *worktree.Run) (run.Plan, error) {
				p, err := build(ctx, w)
				if kind == "failed" {
					p.Executor = executor(func(_ context.Context, r v.Request) (v.Result, error) {
						one := 1
						return v.Result{ID: r.Check.ID, State: v.Failed, ExitCode: &one}, nil
					})
				}
				if kind == "cleanup" {
					p.Close = func() error { return errors.New("cleanup") }
				}
				return p, err
			}
			current := opts
			if kind == "storage" {
				current.Artifacts = filepath.Join(t.TempDir(), "missing")
			}
			failed, _ := run.Execute(context.Background(), current)
			if err := store.Save(failed); err == nil {
				t.Fatal("unsuccessful outcome trusted")
			}
		})
	}
}
func TestAllInputsInvalidate(t *testing.T) {
	opts, store, _ := fixture(t)
	out := success(t, opts)
	if err := store.Save(out); err != nil {
		t.Fatal(err)
	}
	changes := map[string]func(*run.CachePlan){
		"head":          func(p *run.CachePlan) { p.Identity.Head = strings.Repeat("b", 40) },
		"base":          func(p *run.CachePlan) { p.Identity.Base = strings.Repeat("b", 40) },
		"tree":          func(p *run.CachePlan) { p.Identity.Tree = strings.Repeat("b", 40) },
		"repository":    func(p *run.CachePlan) { p.Identity.Repository = "other/repo" },
		"platform":      func(p *run.CachePlan) { p.Identity.Platform = "darwin/arm64" },
		"executor":      func(p *run.CachePlan) { p.Identity.Executor = v.ExecutorAct },
		"mode":          func(p *run.CachePlan) { p.Identity.Mode = v.ModeHead },
		"workflow":      func(p *run.CachePlan) { p.Identity.Fingerprints.Workflow = strings.Repeat("b", 64) },
		"config":        func(p *run.CachePlan) { p.Identity.Fingerprints.Config = strings.Repeat("b", 64) },
		"policy":        func(p *run.CachePlan) { p.Identity.Fingerprints.Policy = strings.Repeat("b", 64) },
		"dependencies":  func(p *run.CachePlan) { p.Identity.Fingerprints.Dependencies = strings.Repeat("b", 64) },
		"toolchain":     func(p *run.CachePlan) { p.Identity.Fingerprints.Toolchain = strings.Repeat("b", 64) },
		"environment":   func(p *run.CachePlan) { p.Identity.Fingerprints.Environment = strings.Repeat("b", 64) },
		"env order":     func(p *run.CachePlan) { p.Env[0], p.Env[1] = p.Env[1], p.Env[0] },
		"argv":          func(p *run.CachePlan) { p.Argv["pure"] = []string{"other"} },
		"matrix":        func(p *run.CachePlan) { p.Targets[0].Matrix = map[string]string{"os": "linux"} },
		"job":           func(p *run.CachePlan) { p.Targets[0].Job = "other" },
		"workflow path": func(p *run.CachePlan) { p.Targets[0].Workflow = "other.yml" },
		"required":      func(p *run.CachePlan) { p.Targets[0].Check.Required = false },
		"needs":         func(p *run.CachePlan) { p.Targets[0].Check.Needs = []string{"missing"} },
		"skipped":       func(p *run.CachePlan) { p.Targets[0].Skipped = true },
		"cacheable":     func(p *run.CachePlan) { p.Cacheable["pure"] = false },
		"entire plan": func(p *run.CachePlan) {
			p.Targets = append(p.Targets, changed.Target{Check: v.Check{ID: "new", Needs: []string{}}})
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			plan := descriptor(t, out)
			change(&plan)
			if _, hit, err := store.Lookup(plan, "pure"); err != nil || hit {
				t.Fatalf("unsafe hit %v", err)
			}
		})
	}
	store.producer = strings.Repeat("0", 64)
	if _, hit, err := store.Lookup(descriptor(t, out), "pure"); err != nil || hit {
		t.Fatal("producer upgrade reused evidence", err)
	}
}

func TestExpiryCorruptionAndPrivatePaths(t *testing.T) {
	opts, store, _ := fixture(t)
	out := success(t, opts)
	plan := descriptor(t, out)
	if err := store.Save(out); err != nil {
		t.Fatal(err)
	}
	key, _ := store.planKey(plan, "pure")
	path := filepath.Join(store.path, key+".json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{[]byte(`{}`), append(append([]byte{}, original...), []byte(`{}`)...), []byte(strings.Repeat("x", maxEntry+1))} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, hit, err := store.Lookup(plan, "pure"); err != nil || hit {
			t.Fatal("corrupt hit", err)
		}
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return out.Receipt.CompletedAt.Add(TTL) }
	if _, hit, err := store.Lookup(plan, "pure"); err != nil || hit {
		t.Fatal("expired hit", err)
	}
	store.now = time.Now
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(external, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, path); err != nil {
		t.Fatal(err)
	}
	if _, hit, err := store.Lookup(plan, "pure"); err != nil || hit {
		t.Fatal("symlink hit", err)
	}
	if _, err := Open(filepath.Join(opts.Worktree.Root, "cache"), opts.Worktree.Root); err == nil {
		t.Fatal("in-repository cache accepted")
	}
	if _, err := os.Stat(filepath.Join(opts.Worktree.Root, "cache")); !os.IsNotExist(err) {
		t.Fatal("rejected cache mutated checkout")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(store.path, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(alias, opts.Worktree.Root); err == nil {
		t.Fatal("symlink root accepted")
	}
	if err := os.Chmod(filepath.Join(store.path, ".key"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(store.path, opts.Worktree.Root); err == nil {
		t.Fatal("public key accepted")
	}
}

func TestConcurrentWritersAndBoundedOwnedRetention(t *testing.T) {
	opts, store, _ := fixture(t)
	out := success(t, opts)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store.Save(out); err != nil && !errors.Is(err, ErrBusy) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if _, hit, err := store.Lookup(descriptor(t, out), "pure"); err != nil || !hit {
		t.Fatal("no valid concurrent result", err)
	}
	root, close, err := store.access()
	if err != nil {
		t.Fatal(err)
	}
	sentinel := []byte("unowned")
	if err := root.WriteFile("keep.txt", sentinel, 0600); err != nil {
		t.Fatal(err)
	}
	for i := range maxEntries + 5 {
		key := fmt.Sprintf("%064x", i)
		origin := v.Reuse{RunID: "fixture", ReceiptID: strings.Repeat("a", 64), CompletedAt: time.Now().UTC().Add(-time.Hour), ExpiresAt: time.Now().UTC().Add(-time.Hour).Add(TTL)}
		origin.ExpiresAt = origin.CompletedAt.Add(TTL)
		payload, _ := json.Marshal(entry{1, key, store.producer, "pure", origin})
		encoded := sealTest(store, payload)
		if err := atomicCreate(root, key+".json", encoded); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.prune(root, 1024); err != nil {
		t.Fatal(err)
	}
	if err := close(); err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(store.path)
	count := 0
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".json") {
			count++
		}
	}
	if count >= maxEntries {
		t.Fatalf("retained%d", count)
	}
	if data, err := os.ReadFile(filepath.Join(store.path, "keep.txt")); err != nil || string(data) != string(sentinel) {
		t.Fatal("unowned file removed")
	}
}

func sealTest(store *Store, payload []byte) []byte {
	mac := hmac.New(sha256.New, store.key)
	_, _ = mac.Write(payload)
	data, _ := json.Marshal(envelope{payload, hex.EncodeToString(mac.Sum(nil))})
	return data
}

func TestAuthenticatedEntryCannotBeRenamedToDifferentPlan(t *testing.T) {
	opts, store, _ := fixture(t)
	out := success(t, opts)
	if err := store.Save(out); err != nil {
		t.Fatal(err)
	}
	original := descriptor(t, out)
	oldKey, _ := store.planKey(original, "pure")
	altered := descriptor(t, out)
	altered.Argv["pure"] = []string{"different", "assertion"}
	newKey, _ := store.planKey(altered, "pure")
	data, err := os.ReadFile(filepath.Join(store.path, oldKey+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.path, newKey+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, hit, err := store.Lookup(altered, "pure"); err != nil || hit {
		t.Fatal("authenticated payload rebound to another plan", err)
	}
}

func TestMixedRunNeverResignsReusedChecks(t *testing.T) {
	opts, store, _ := fixture(t)
	build := opts.Build
	opts.Build = func(ctx context.Context, w *worktree.Run) (run.Plan, error) {
		plan, err := build(ctx, w)
		plan.Targets = append(plan.Targets, changed.Target{Check: v.Check{ID: "second", Required: true, Needs: []string{"pure"}}})
		plan.Argv["second"] = []string{"second"}
		plan.Cacheable["second"] = true
		return plan, err
	}
	opts.Reuse = store
	first := success(t, opts)
	plan := descriptor(t, first)
	firstKey, _ := store.planKey(plan, "pure")
	secondKey, _ := store.planKey(plan, "second")
	path := filepath.Join(store.path, firstKey+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(store.path, secondKey+".json")); err != nil {
		t.Fatal(err)
	}
	mixed := success(t, opts)
	_, _, executed, err := mixed.CacheEvidence()
	if err != nil || len(executed) != 1 || executed[0] != "second" {
		t.Fatalf("executed%v %v", executed, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("mixed run resigned reused check", err)
	}
	if mixed.Receipt.Results[0].Reuse == nil || mixed.Receipt.Results[1].Reuse != nil {
		t.Fatal("mixed provenance incorrect")
	}
}

func TestCorruptSlotsAreBoundedAndRepairAfterExecution(t *testing.T) {
	opts, store, calls := fixture(t)
	opts.Reuse = store
	first := success(t, opts)
	plan := descriptor(t, first)
	key, _ := store.planKey(plan, "pure")
	path := filepath.Join(store.path, key+".json")
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := range maxEntries + 5 {
		if err := os.WriteFile(filepath.Join(store.path, fmt.Sprintf("%064x.json", i)), []byte("corrupt"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	repaired := success(t, opts)
	if *calls != 2 || repaired.Receipt.Results[0].Reuse != nil {
		t.Fatal("corruption did not execute fresh check")
	}
	if _, hit, err := store.Lookup(plan, "pure"); err != nil || !hit {
		t.Fatal("fresh attested execution did not repair slot", err)
	}
	files, err := os.ReadDir(store.path)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".json") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("corrupt slots retained %d", count)
	}
}

func TestDirectoryReplacementCannotRedirectWrites(t *testing.T) {
	opts, store, _ := fixture(t)
	out := success(t, opts)
	original := store.path + "-original"
	if err := os.Rename(store.path, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.path, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(store.path, "keep")
	if err := os.WriteFile(sentinel, []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(out); err == nil {
		t.Fatal("replaced directory accepted")
	}
	files, err := os.ReadDir(store.path)
	if err != nil || len(files) != 1 || files[0].Name() != "keep" {
		t.Fatal("replacement directory modified", err)
	}
}

func TestStoredEntriesContainOnlyOriginMetadata(t *testing.T) {
	opts, store, _ := fixture(t)
	build := opts.Build
	opts.Build = func(ctx context.Context, w *worktree.Run) (run.Plan, error) {
		plan, err := build(ctx, w)
		plan.Env = append(plan.Env, "PRIVATE_INPUT=fixture-sensitive-value")
		plan.Argv["pure"] = []string{"private-command-path"}
		return plan, err
	}
	out := success(t, opts)
	if err := store.Save(out); err != nil {
		t.Fatal(err)
	}
	key, _ := store.planKey(descriptor(t, out), "pure")
	data, err := os.ReadFile(filepath.Join(store.path, key+".json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"fixture-sensitive-value", "private-command-path", opts.Worktree.Root, "example/repo"} {
		if strings.Contains(string(data), private) {
			t.Fatal("private execution inputs persisted")
		}
	}
}
