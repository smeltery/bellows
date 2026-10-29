package freshness

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smeltery/bellows/internal/reporting/github"
)

func TestCapabilityNeverClaimsMergeAuthorization(t *testing.T) {
	for status, want := range map[int]string{200: "readable", 403: "access_or_plan_restricted", 404: "absent_or_not_visible", 429: "unavailable"} {
		t.Run(want, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || !strings.Contains(r.URL.EscapedPath(), "feature%2Ftopic") || r.Header.Get("Authorization") != "Bearer synthetic-secret" {
					t.Error("unexpected capability lookup")
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"enforce_admins":true,"secret":"response-must-not-be-returned"}`))
			}))
			defer server.Close()
			got, err := Probe(context.Background(), github.Client{APIURL: server.URL, HTTP: server.Client(), Token: "synthetic-secret"}, "example/repo", "feature/topic")
			if err != nil || got.Mode != "advisory" || got.MergeAuthorized || got.Protection.State != want || got.Rules.State != want || calls != 2 {
				t.Fatalf("%+v %v calls=%d", got, err, calls)
			}
		})
	}
}

func TestCapabilityRejectsRedirectsAndInvalidInputs(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://invalid.example/credentials", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	api := github.Client{APIURL: server.URL, HTTP: server.Client(), Token: "synthetic"}
	got, err := Probe(context.Background(), api, "example/repo", "main")
	if err != nil || got.Protection.HTTP != 307 || got.MergeAuthorized {
		t.Fatalf("%+v %v", got, err)
	}
	for _, repo := range []string{"", "../repo", "owner/repo/extra"} {
		if _, err := Probe(context.Background(), api, repo, "main"); err == nil {
			t.Fatal("invalid repository accepted")
		}
	}
	api.APIURL = "http://invalid.example"
	if _, err := Probe(context.Background(), api, "example/repo", "main"); err == nil {
		t.Fatal("insecure API accepted")
	}
}
