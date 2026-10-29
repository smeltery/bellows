package github

import (
	"context"
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
