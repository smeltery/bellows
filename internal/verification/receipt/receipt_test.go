package receipt

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	v "github.com/smeltery/bellows/internal/verification"
)

func fixture(t *testing.T) v.Receipt {
	t.Helper()
	data, err := os.ReadFile("../testdata/passed.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestStrictDecode(t *testing.T) {
	data, err := Encode(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"unknown":        strings.Replace(string(data), `"version":1`, `"extra":true,"version":1`, 1),
		"duplicate":      strings.Replace(string(data), `"version":1`, `"version":1,"version":1`, 1),
		"missing":        strings.Replace(string(data), `"version":1,`, ``, 1),
		"null required":  strings.Replace(string(data), `"required":true`, `"required":null`, 1),
		"unknown nested": strings.Replace(string(data), `"required":true`, `"required":true,"secret":"x"`, 1),
		"null needs":     strings.Replace(string(data), `"needs":[]`, `"needs":null`, 1),
		"offset":         strings.ReplaceAll(string(data), `00Z`, `00+00:00`),
		"trailing":       string(data) + ` {}`,
		"version":        strings.Replace(string(data), `"version":1`, `"version":2`, 1),
		"invalid UTF8":   string(append(data, 0xff)),
		"oversized":      strings.Repeat(" ", MaxBytes+1),
		"null":           `null`, "truncated": string(data[:len(data)-1]),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(data)); err == nil {
				t.Fatal("accepted malformed receipt")
			}
		})
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Encode(decoded)
	if err != nil || !bytes.Equal(data, again) {
		t.Fatalf("unstable encoding: %v", err)
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]func(*v.Receipt){
		"object":                        func(r *v.Receipt) { r.Identity.Tree = "bad" },
		"digest":                        func(r *v.Receipt) { r.Identity.Fingerprints.Toolchain = "bad" },
		"duplicate check":               func(r *v.Receipt) { r.Checks = append(r.Checks, r.Checks[0]) },
		"unknown optional prerequisite": func(r *v.Receipt) { r.Checks[0].Required = false; r.Checks[0].Needs = []string{"unknown"} },
		"cycle":                         func(r *v.Receipt) { r.Checks[0].Needs = []string{r.Checks[0].ID} },
		"duplicate prerequisite": func(r *v.Receipt) {
			r.Checks = append(r.Checks, v.Check{ID: "other", Needs: []string{r.Checks[0].ID, r.Checks[0].ID}})
		},
		"duplicate result":          func(r *v.Receipt) { r.Results = append(r.Results, r.Results[0]) },
		"missing result":            func(r *v.Receipt) { r.Results = []v.Result{} },
		"unknown result":            func(r *v.Receipt) { r.Results[0].ID = "unknown" },
		"pending completed":         func(r *v.Receipt) { r.Results[0].State = v.Pending; r.Results[0].ExitCode = nil },
		"passed exit":               func(r *v.Receipt) { r.Results[0].ExitCode = nil },
		"failed zero":               func(r *v.Receipt) { r.Results[0].State = v.Failed },
		"negative duration":         func(r *v.Receipt) { r.Results[0].DurationMS = -1 },
		"duration outside lifetime": func(r *v.Receipt) { r.Results[0].DurationMS = 100000 },
		"completion before start":   func(r *v.Receipt) { end := r.StartedAt.Add(-time.Second); r.CompletedAt = &end },
		"local time":                func(r *v.Receipt) { r.StartedAt = r.StartedAt.In(time.FixedZone("local", 3600)) },
		"unknown state":             func(r *v.Receipt) { r.Results[0].State = "mystery" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := fixture(t)
			mutate(&r)
			if Validate(r) == nil {
				t.Fatal("accepted invalid receipt")
			}
		})
	}
	r := fixture(t)
	r.Input = v.Conflict
	r.Identity.Tree = ""
	r.Identity.Fingerprints = v.Fingerprints{}
	if err := Validate(r); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(Match(r, r.Identity), ErrUnsuccessful) {
		t.Fatal("conflict eligible")
	}
	r.Input = v.Ready
	if Validate(r) == nil {
		t.Fatal("ready allowed unresolved identity")
	}
}

func TestMatch(t *testing.T) {
	r := fixture(t)
	if err := Match(r, r.Identity); err != nil {
		t.Fatal(err)
	}
	changes := []func(*v.Identity){func(i *v.Identity) { i.Head = strings.Repeat("a", 40) }, func(i *v.Identity) { i.Base = strings.Repeat("a", 40) }, func(i *v.Identity) { i.Tree = strings.Repeat("a", 40) }, func(i *v.Identity) { i.Platform = "other" }, func(i *v.Identity) { i.Executor = v.ExecutorFlox }, func(i *v.Identity) { i.Mode = v.ModeHead }, func(i *v.Identity) { i.Repository = "other/project" }, func(i *v.Identity) { i.Fingerprints.Config = strings.Repeat("b", 64) }, func(i *v.Identity) { i.Fingerprints.Toolchain = strings.Repeat("b", 64) }, func(i *v.Identity) { i.Fingerprints.Workflow = strings.Repeat("b", 64) }, func(i *v.Identity) { i.Fingerprints.Policy = strings.Repeat("b", 64) }, func(i *v.Identity) { i.Fingerprints.Dependencies = strings.Repeat("b", 64) }, func(i *v.Identity) { i.Fingerprints.Environment = strings.Repeat("b", 64) }}
	for _, change := range changes {
		current := r.Identity
		change(&current)
		if !errors.Is(Match(r, current), ErrStale) {
			t.Fatal("accepted stale identity")
		}
	}
	for _, state := range []v.State{v.Failed, v.Skipped, v.Cancelled, v.Unsupported, v.Error} {
		r := fixture(t)
		r.Results[0].State = state
		r.Results[0].ExitCode = nil
		if state == v.Failed {
			code := 1
			r.Results[0].ExitCode = &code
		}
		if !errors.Is(Match(r, r.Identity), ErrUnsuccessful) {
			t.Fatalf("accepted %s", state)
		}
	}
	r.CompletedAt = nil
	if !errors.Is(Match(r, r.Identity), ErrIncomplete) {
		t.Fatal("accepted incomplete receipt")
	}
}

func TestAtomicPrivateStorage(t *testing.T) {
	worktree := t.TempDir()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	r := fixture(t)
	r.RunID = "../../escape"
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := Save(worktree, directory, r); results <- err }()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, os.ErrExist) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("writes succeeded: %d", successes)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("leftover artifacts: %v %v", entries, err)
	}
	path := filepath.Join(directory, entries[0].Name())
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %v %v", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err != nil {
		t.Fatal(err)
	}
	r.Identity.Repository = "changed/project"
	if _, err := Save(worktree, directory, r); !errors.Is(err, os.ErrExist) {
		t.Fatalf("overwrite: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("existing evidence changed")
	}
	inside := filepath.Join(worktree, "artifacts")
	if err := os.Mkdir(inside, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(worktree, inside, r); err == nil {
		t.Fatal("stored in tracked tree")
	}
	if _, err := Save(worktree, worktree, r); err == nil {
		t.Fatal("stored inside worktree")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(directory, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(worktree, link, r); err == nil {
		t.Fatal("accepted symlink")
	}
	if err := os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(worktree, directory, r); err == nil {
		t.Fatal("accepted public directory")
	}
	r.CompletedAt = nil
	if _, err := Save(worktree, t.TempDir(), r); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("incomplete: %v", err)
	}
}

func TestCanonicalOrderAndPrerequisites(t *testing.T) {
	r := fixture(t)
	root := r.Checks[0].ID
	r.Checks = append(r.Checks, v.Check{ID: "z", Needs: []string{}}, v.Check{ID: "a", Required: true, Needs: []string{"z", root}})
	zero := 0
	r.Results = append(r.Results, v.Result{ID: "z", State: v.Passed, ExitCode: &zero}, v.Result{ID: "a", State: v.Passed, ExitCode: &zero})
	data, err := Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	if r.Checks[2].Needs[0] != "z" {
		t.Fatal("encoding mutated prerequisites")
	}
	r.Checks[0], r.Checks[2] = r.Checks[2], r.Checks[0]
	r.Results[0], r.Results[2] = r.Results[2], r.Results[0]
	again, err := Encode(r)
	if err != nil || !bytes.Equal(data, again) {
		t.Fatalf("noncanonical ordering: %v", err)
	}
	r.Results[1].State = v.Skipped
	r.Results[1].ExitCode = nil
	if Validate(r) == nil {
		t.Fatal("accepted passed dependent of skipped prerequisite")
	}
}

func TestPersistFailedAndUnresolved(t *testing.T) {
	for _, input := range []v.InputState{v.Ready, v.Conflict} {
		r := fixture(t)
		r.Input = input
		r.Results[0].State = v.Failed
		code := 1
		r.Results[0].ExitCode = &code
		if input == v.Conflict {
			r.Identity.Tree = ""
			r.Identity.Fingerprints = v.Fingerprints{}
			r.Checks = []v.Check{}
			r.Results = []v.Result{}
		}
		directory := t.TempDir()
		if err := os.Chmod(directory, 0700); err != nil {
			t.Fatal(err)
		}
		path, err := Save(t.TempDir(), directory, r)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		if !errors.Is(Match(decoded, decoded.Identity), ErrUnsuccessful) {
			t.Fatal("failed receipt reusable")
		}
	}
}

func TestDirectorySwapCannotRedirectWrites(t *testing.T) {
	parent := t.TempDir()
	directory := filepath.Join(parent, "private")
	moved := filepath.Join(parent, "moved")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Chmod(outside, 0700); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if os.Rename(directory, moved) != nil {
				continue
			}
			if os.Symlink(outside, directory) == nil {
				_ = os.Remove(directory)
			}
			_ = os.Rename(moved, directory)
		}
	}()
	r := fixture(t)
	worktree := t.TempDir()
	for i := range 100 {
		r.RunID = fmt.Sprintf("swap-%d", i)
		_, _ = Save(worktree, directory, r)
	}
	close(stop)
	<-done
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("directory replacement redirected %d writes", len(entries))
	}
}

func TestReadersOnlySeeCompleteReceipts(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	worktree := t.TempDir()
	r := fixture(t)
	done := make(chan struct{})
	readErrors := make(chan error, 1)
	go func() {
		defer close(readErrors)
		for {
			select {
			case <-done:
				return
			default:
			}
			files, err := filepath.Glob(filepath.Join(directory, "*.json"))
			if err != nil {
				readErrors <- err
				return
			}
			for _, path := range files {
				data, err := os.ReadFile(path)
				if err == nil {
					_, err = Decode(data)
				}
				if err != nil {
					readErrors <- err
					return
				}
			}
		}
	}()
	for i := range 30 {
		r.RunID = fmt.Sprintf("publication-%d", i)
		if _, err := Save(worktree, directory, r); err != nil {
			close(done)
			t.Fatal(err)
		}
	}
	close(done)
	if err := <-readErrors; err != nil {
		t.Fatal(err)
	}
}

func TestUnresolvedIdentityRoundTrip(t *testing.T) {
	for _, input := range []v.InputState{v.Dirty, v.Conflict, v.Stale, v.Invalid} {
		r := fixture(t)
		r.Input = input
		r.Identity = v.Identity{}
		r.Checks = []v.Check{}
		r.Results = []v.Result{}
		data, err := Encode(r)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := Decode(data)
		if err != nil || decoded.Identity != r.Identity {
			t.Fatalf("unresolved roundtrip: %v", err)
		}
		for _, mutate := range []func(*v.Identity){func(i *v.Identity) { i.Repository = "https://example.invalid/repo" }, func(i *v.Identity) { i.Mode = "bad" }, func(i *v.Identity) { i.Executor = "bad" }} {
			bad := r
			mutate(&bad.Identity)
			if Validate(bad) == nil {
				t.Fatal("accepted malformed resolved field")
			}
		}
		r.Input = v.Ready
		if Validate(r) == nil {
			t.Fatal("ready accepted unresolved identity")
		}
	}
}
