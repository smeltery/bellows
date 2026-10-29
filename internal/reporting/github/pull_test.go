package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPullLookupRejectsUnsafeResponses(t *testing.T) {
	for _, body := range []string{`{}`, `{"state":"closed"}`, `{"state":"open","head":{"sha":"bad"}}`, strings.Repeat("x", 1<<20)} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		_, _, err := (Client{Token: "private", APIURL: server.URL, HTTP: server.Client()}).PullIdentity(context.Background(), "example/repo", 1)
		server.Close()
		if err == nil {
			t.Fatal("unsafe response accepted")
		}
	}
}

func TestPullIdentityResolvesCurrentBranchRefs(t *testing.T) {
	old := strings.Repeat("a", 40)
	head, base := strings.Repeat("b", 40), strings.Repeat("c", 40)
	for _, branch := range []string{"release/main", "feature/#percent%"} {
		t.Run(branch, func(t *testing.T) {
			var paths []string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.Header.Get("Authorization") != "Bearer private" || r.URL.RawQuery != "" {
					t.Error("unsafe request")
				}
				if strings.Contains(r.URL.Path, "/pulls/") {
					fmt.Fprintf(w, `{"state":"open","head":{"ref":"topic/branch","sha":%q,"repo":{"full_name":"example/repo"}},"base":{"ref":%q,"sha":%q,"repo":{"full_name":"example/repo"}}}`, old, branch, old)
					return
				}
				name := strings.TrimPrefix(r.URL.Path, "/repos/example/repo/git/ref/heads/")
				sha := head
				if name == branch {
					sha = base
				}
				fmt.Fprintf(w, `{"ref":%q,"object":{"type":"commit","sha":%q}}`, "refs/heads/"+name, sha)
			}))
			defer server.Close()
			h, b, err := (Client{Token: "private", APIURL: server.URL, HTTP: server.Client()}).PullIdentity(context.Background(), "example/repo", 1)
			if err != nil || h != head || b != base || len(paths) != 3 || paths[2] != "/repos/example/repo/git/ref/heads/"+branch {
				t.Fatalf("%q %q %v paths=%v", h, b, err, paths)
			}
		})
	}
}

func TestPullIdentityRejectsUnresolvedRefs(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, scenario := range []string{"forbidden", "missing", "redirect", "wrong ref", "tag", "bad sha", "array", "trailing", "oversize", "fork", "invalid branch"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/pulls/") {
					repo, branch := "example/repo", "main"
					if scenario == "fork" {
						repo = "other/repo"
					}
					if scenario == "invalid branch" {
						branch = "../main"
					}
					fmt.Fprintf(w, `{"state":"open","head":{"ref":"feature","sha":%q,"repo":{"full_name":"example/repo"}},"base":{"ref":%q,"sha":%q,"repo":{"full_name":%q}}}`, sha, branch, sha, repo)
					return
				}
				name := strings.TrimPrefix(r.URL.Path, "/repos/example/repo/git/ref/heads/")
				if name == "feature" {
					fmt.Fprintf(w, `{"ref":"refs/heads/feature","object":{"type":"commit","sha":%q}}`, sha)
					return
				}
				ref, kind, id := "refs/heads/main", "commit", sha
				switch scenario {
				case "forbidden":
					w.WriteHeader(403)
					return
				case "missing":
					w.WriteHeader(404)
					return
				case "redirect":
					w.Header().Set("Location", "/unexpected")
					w.WriteHeader(302)
					return
				case "wrong ref":
					ref = "refs/heads/main-other"
				case "tag":
					kind = "tag"
				case "bad sha":
					id = "short"
				case "array":
					fmt.Fprint(w, `[]`)
					return
				case "oversize":
					fmt.Fprint(w, strings.Repeat(" ", 1<<20)+`{}`)
					return
				}
				fmt.Fprintf(w, `{"ref":%q,"object":{"type":%q,"sha":%q}}`, ref, kind, id)
				if scenario == "trailing" {
					fmt.Fprint(w, `{}`)
				}
			}))
			defer server.Close()
			h, b, err := (Client{Token: "private", APIURL: server.URL, HTTP: server.Client()}).PullIdentity(context.Background(), "example/repo", 1)
			if err == nil || h != "" || b != "" {
				t.Fatalf("accepted unresolved inputs %q %q %v", h, b, err)
			}
		})
	}
}

func TestInvalidBranchNames(t *testing.T) {
	for _, branch := range []string{"", "@", "a..b", "a@{b", "main.", "a b", "a\nb", "a\x7fb", "a~b", "a^b", "a:b", "a?b", "a*b", "a[b", "a\\b", "/main", "main/", "a//b", ".main", "a/.hidden", "main.lock"} {
		if validBranch(branch) {
			t.Errorf("accepted %q", branch)
		}
	}
}
