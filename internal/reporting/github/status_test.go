package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smeltery/bellows/internal/verification"
)

func sample() Update {
	return Update{Identity: verification.Identity{Repository: "example/private", Head: strings.Repeat("a", 40)}, State: Success, TargetURL: "https://github.com/example/private/actions/runs/123"}
}

func allow(context.Context, verification.Identity) error { return nil }

func fixture(t *testing.T, handler http.HandlerFunc) Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return Client{Token: "synthetic-secret", APIURL: server.URL, HTTP: server.Client()}
}

func TestPublishPrivateRepositoryAndDuplicates(t *testing.T) {
	calls := 0
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/repos/example/private/statuses/"+strings.Repeat("a", 40) || r.Header.Get("Authorization") != "Bearer synthetic-secret" {
			t.Errorf("unexpected request method/path/authorization")
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["context"] != "bellows/verify" || body["state"] != "success" || body["description"] != "Advisory local verification passed" || len(body) != 4 {
			t.Errorf("unexpected payload: %v", body)
		}
		w.WriteHeader(http.StatusCreated)
	})
	for range 2 {
		if err := client.Publish(context.Background(), sample(), allow); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatal("duplicate calls must retain append-only API semantics")
	}
}

func TestHTTPFailuresAndBoundedRetries(t *testing.T) {
	for _, status := range []int{200, 401, 403, 404, 422, 429, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			client := fixture(t, func(w http.ResponseWriter, _ *http.Request) {
				calls++
				if status == 429 {
					w.Header().Set("Retry-After", "60")
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte("synthetic-secret raw-log"))
			})
			err := client.Publish(context.Background(), sample(), allow)
			if err == nil || strings.Contains(err.Error(), "synthetic-secret") || strings.Contains(err.Error(), "raw-log") {
				t.Fatalf("unsafe or absent error: %v", err)
			}
			want := 1
			if status >= 500 {
				want = 3
			}
			if calls != want {
				t.Fatalf("calls=%d want=%d", calls, want)
			}
		})
	}
}

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNetworkRetriesAndGuardRejectsOutOfOrderCompletion(t *testing.T) {
	for _, reject := range []bool{false, true} {
		calls, guards := 0, 0
		client := Client{Token: "synthetic", HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("synthetic-secret network detail")
		})}}
		err := client.Publish(context.Background(), sample(), func(_ context.Context, identity verification.Identity) error {
			guards++
			if identity != sample().Identity {
				t.Fatal("identity changed")
			}
			if reject && guards == 2 {
				return errors.New("stale generation synthetic-secret")
			}
			return nil
		})
		if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatalf("error=%v", err)
		}
		if (reject && (calls != 1 || guards != 2)) || (!reject && (calls != 3 || guards != 3)) {
			t.Fatalf("calls=%d guards=%d", calls, guards)
		}
	}
}

func TestCancelDuringRetryDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := fixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		time.AfterFunc(20*time.Millisecond, cancel)
	})
	started := time.Now()
	if err := client.Publish(ctx, sample(), allow); !errors.Is(err, context.Canceled) || time.Since(started) > time.Second {
		t.Fatalf("cancellation=%v duration=%v", err, time.Since(started))
	}
}

func TestRetryAfterDefersToCaller(t *testing.T) {
	calls := 0
	client := fixture(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	if err := client.Publish(context.Background(), sample(), allow); err == nil || calls != 1 {
		t.Fatalf("ignored retry-after: calls=%d err=%v", calls, err)
	}
}

func TestRejectInvalidTargetsBeforeNetwork(t *testing.T) {
	for _, change := range []func(*Client, *Update){
		func(c *Client, _ *Update) { c.Token = "" },
		func(c *Client, _ *Update) { c.APIURL = "http://example.invalid" },
		func(c *Client, _ *Update) { c.APIURL = "https://user:secret@example.invalid" },
		func(_ *Client, u *Update) { u.Identity.Repository = "../private" },
		func(_ *Client, u *Update) { u.Identity.Head = "main" },
		func(_ *Client, u *Update) { u.State = "secret-log" },
		func(_ *Client, u *Update) { u.TargetURL = "https://evil.invalid/logs" },
		func(_ *Client, u *Update) { u.TargetURL = "https://github.com/other/private/actions/runs/123" },
		func(_ *Client, u *Update) { u.TargetURL += "?token=secret" },
		func(_ *Client, u *Update) { u.TargetURL += "#secret" },
	} {
		client := Client{Token: "synthetic", HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid request reached network")
			return nil, nil
		})}}
		update := sample()
		change(&client, &update)
		if err := client.Publish(context.Background(), update, allow); err == nil {
			t.Fatal("accepted invalid request")
		}
	}
	if err := (Client{Token: "synthetic"}).Publish(context.Background(), sample(), nil); err == nil {
		t.Fatal("accepted missing guard")
	}
}

func TestRedirectNeverDisclosesCredential(t *testing.T) {
	var received atomic.Int32
	destination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer destination.Close()
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	})
	client.HTTP.CheckRedirect = func(*http.Request, []*http.Request) error { t.Fatal("caller redirect hook used"); return nil }
	if err := client.Publish(context.Background(), sample(), allow); err == nil || received.Load() != 0 {
		t.Fatalf("redirect followed: %v", err)
	}
}
