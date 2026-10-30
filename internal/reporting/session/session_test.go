package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smeltery/bellows/internal/reporting/github"
	v "github.com/smeltery/bellows/internal/verification"
)

func evidence() v.Receipt {
	hash := strings.Repeat("a", 64)
	return v.Receipt{Version: 1, RunID: "fixture", StartedAt: time.Now().UTC(), Input: v.Ready, Identity: v.Identity{Repository: "example/repo", Head: strings.Repeat("b", 40), Base: strings.Repeat("c", 40), Tree: strings.Repeat("d", 40), Mode: v.ModeMerge, Executor: v.ExecutorFlox, Platform: "linux/amd64", Fingerprints: v.Fingerprints{Workflow: hash, Config: hash, Policy: hash, Dependencies: hash, Toolchain: hash, Environment: hash}}, Checks: []v.Check{{ID: "test", Required: true, Needs: []string{}}}, Results: []v.Result{}}
}
func TestAdvisoryLifecycleAgainstHTTP(t *testing.T) {
	for _, scenario := range []string{"passed", "failed", "cancelled", "error", "head moved", "base moved", "toolchain moved", "api error", "duplicate", "reused passed", "reused head moved", "reused base moved", "reused toolchain moved"} {
		t.Run(scenario, func(t *testing.T) {
			kind := strings.TrimPrefix(scenario, "reused ")
			initial := evidence()
			if kind != scenario {
				initial.Version = v.ReceiptVersion
			}
			head, base := initial.Identity.Head, initial.Identity.Base
			current := initial.Identity
			var statuses []string
			lookupCount := 0
			reject := false
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer private-token" {
					t.Error("missing publisher credential")
				}
				if strings.Contains(r.URL.Path, "/git/ref/heads/") {
					branch := strings.TrimPrefix(r.URL.Path, "/repos/example/repo/git/ref/heads/")
					sha := head
					if branch == "main" {
						sha = base
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"ref": "refs/heads/" + branch, "object": map[string]string{"type": "commit", "sha": sha}})
					return
				}
				if r.Method == http.MethodGet {
					lookupCount++
					_ = json.NewEncoder(w).Encode(map[string]any{"state": "open", "head": map[string]any{"ref": "feature", "sha": initial.Identity.Head, "repo": map[string]string{"full_name": "example/repo"}}, "base": map[string]any{"ref": "main", "sha": initial.Identity.Base, "repo": map[string]string{"full_name": "example/repo"}}})
					return
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["context"] != "bellows/verify" {
					t.Error("incorrect context")
				}
				if reject {
					w.WriteHeader(500)
					return
				}
				statuses = append(statuses, body["state"])
				w.WriteHeader(201)
			}))
			defer server.Close()
			reporter, err := New(github.Client{Token: "private-token", APIURL: server.URL, HTTP: server.Client()}, "example/repo", 1, true, func(context.Context) (v.Identity, error) { return current, nil })
			if err != nil {
				t.Fatal(err)
			}
			if err = reporter.Start(context.Background(), initial); err != nil {
				t.Fatal(err)
			}
			final := initial
			finished := time.Now().UTC()
			final.CompletedAt = &finished
			code := 0
			result := v.Result{ID: "test", State: v.Passed, ExitCode: &code}
			if kind != scenario {
				result.Reuse = &v.Reuse{ReceiptID: strings.Repeat("a", 64), RunID: "prior-run", CompletedAt: initial.StartedAt.Add(-time.Minute), ExpiresAt: initial.StartedAt.Add(time.Hour)}
			}
			want := "success"
			switch kind {
			case "failed":
				code = 1
				result.State = v.Failed
				want = "failure"
			case "cancelled":
				result.State = v.Cancelled
				result.ExitCode = nil
				want = "error"
			case "error":
				result.State = v.Error
				result.ExitCode = nil
				want = "error"
			case "head moved":
				head = strings.Repeat("e", 40)
			case "base moved":
				base = strings.Repeat("e", 40)
			case "toolchain moved":
				current.Fingerprints.Toolchain = strings.Repeat("e", 64)
			case "api error":
				reject = true
			}
			final.Results = []v.Result{result}
			err = reporter.Finish(context.Background(), final)
			denied := kind == "head moved" || kind == "base moved" || kind == "toolchain moved" || kind == "api error"
			if (err != nil) != denied {
				t.Fatalf("finish: %v", err)
			}
			if denied {
				if len(statuses) != 1 {
					t.Fatalf("stale/failed upload published: %v", statuses)
				}
			} else if len(statuses) != 2 || statuses[1] != want {
				t.Fatalf("statuses=%v", statuses)
			}
			if lookupCount < 2 && kind != "toolchain moved" {
				t.Fatal("terminal status did not reread PR")
			}
			if kind == "duplicate" {
				if err := reporter.Finish(context.Background(), final); err == nil || len(statuses) != 2 {
					t.Fatal("duplicate completion accepted")
				}
			}
		})
	}
}
func TestReportingRequiresOptIn(t *testing.T) {
	if _, err := New(nil, "example/repo", 1, false, nil); err == nil {
		t.Fatal("missing explicit reporting accepted")
	}
}
