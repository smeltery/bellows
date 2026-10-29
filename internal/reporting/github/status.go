// Package github publishes advisory commit statuses, never verification evidence.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/smeltery/bellows/internal/verification"
)

type State string

const (
	Pending State = "pending"
	Success State = "success"
	Failure State = "failure"
	Error   State = "error"
)

type Client struct {
	Token  string
	HTTP   *http.Client
	APIURL string
}

type Update struct {
	Identity  verification.Identity
	State     State
	TargetURL string
}

// Guard must check current identity and the orchestration run generation. Callers
// serialize publication; GitHub has no compare-and-swap for statuses, so this
// cannot atomically order independent publishers or prevent a remote ref race.
type Guard func(context.Context, verification.Identity) error

var repository = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
var objectID = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
var runPath = regexp.MustCompile(`^/actions/runs/[0-9]+$`)

// Publish appends a status. Duplicate calls and ambiguous network retries can
// append identical records; success here means API acceptance only. Neither a
// receipt nor this advisory status proves that hostile code actually ran.
func (c Client) Publish(ctx context.Context, update Update, guard Guard) error {
	if strings.TrimSpace(c.Token) == "" || strings.ContainsAny(c.Token, "\r\n") || guard == nil {
		return errors.New("reporting requires a token and freshness guard")
	}
	if !repository.MatchString(update.Identity.Repository) || !objectID.MatchString(update.Identity.Head) {
		return errors.New("reporting requires an explicit repository and full commit ID")
	}
	descriptions := map[State]string{Pending: "Advisory local verification pending", Success: "Advisory local verification passed", Failure: "Advisory local verification failed", Error: "Advisory local verification unavailable"}
	description, ok := descriptions[update.State]
	if !ok {
		return errors.New("invalid advisory status state")
	}
	if update.TargetURL != "" {
		link, err := url.Parse(update.TargetURL)
		if err != nil || link.Scheme != "https" || link.Host != "github.com" || link.User != nil || link.RawQuery != "" || link.ForceQuery || link.Fragment != "" || link.RawPath != "" || !strings.HasPrefix(link.Path, "/"+update.Identity.Repository+"/") || !runPath.MatchString(strings.TrimPrefix(link.Path, "/"+update.Identity.Repository)) {
			return errors.New("status link must identify this repository's GitHub Actions run")
		}
	}
	base := c.APIURL
	if base == "" {
		base = "https://api.github.com"
	}
	endpoint, err := url.Parse(base)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || endpoint.RawPath != "" {
		return errors.New("reporting API must be an explicit HTTPS endpoint")
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/repos/" + update.Identity.Repository + "/statuses/" + update.Identity.Head
	body, err := json.Marshal(struct {
		State       State  `json:"state"`
		Context     string `json:"context"`
		Description string `json:"description"`
		TargetURL   string `json:"target_url,omitempty"`
	}{update.State, "bellows/verify", description, update.TargetURL})
	if err != nil {
		return errors.New("encode advisory status")
	}
	client := http.Client{}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := guard(ctx, update.Identity); err != nil {
			return errors.New("status publication rejected by freshness guard")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
		if err != nil {
			return errors.New("construct advisory status request")
		}
		request.Header.Set("Authorization", "Bearer "+c.Token)
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
		response, sendErr := client.Do(request)
		status := 0
		retryAfter := ""
		if response != nil {
			status = response.StatusCode
			retryAfter = response.Header.Get("Retry-After")
			_ = response.Body.Close()
		}
		if sendErr == nil && status == http.StatusCreated {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// Rate limits need caller scheduling: do not ignore Retry-After or spin.
		if sendErr == nil && (status < 500 || status > 599 || retryAfter != "") {
			return fmt.Errorf("GitHub rejected advisory status (HTTP %d)", status)
		}
		if attempt == 2 {
			return errors.New("advisory status upload failed after bounded retries")
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return errors.New("advisory status upload failed")
}
