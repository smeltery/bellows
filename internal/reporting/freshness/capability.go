package freshness

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/smeltery/bellows/internal/reporting/github"
)

type Access struct {
	State string `json:"state"`
	HTTP  int    `json:"http"`
}
type Capability struct {
	Mode            string `json:"mode"`
	MergeAuthorized bool   `json:"merge_authorized"`
	Protection      Access `json:"protection"`
	Rules           Access `json:"rules"`
}

// Probe only establishes API readability. Even HTTP 200 does not prove strict
// required checks, administrator coverage, or the absence of UI/API bypasses.
func Probe(ctx context.Context, api github.Client, repository, branch string) (Capability, error) {
	out := Capability{Mode: "advisory", Protection: Access{State: "unknown"}, Rules: Access{State: "unknown"}}
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_][A-Za-z0-9_.-]*$`).MatchString(repository) || branch == "" || strings.Contains(branch, "..") || strings.ContainsAny(branch, "\r\n\x00") || strings.ContainsAny(api.Token, "\r\n") {
		return out, errors.New("explicit repository and branch required for capability lookup")
	}
	base := api.APIURL
	if base == "" {
		base = "https://api.github.com"
	}
	endpoint, err := url.Parse(base)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || endpoint.RawPath != "" {
		return out, errors.New("invalid capability API endpoint")
	}
	client := http.Client{}
	if api.HTTP != nil {
		client = *api.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for _, probe := range []struct {
		path   string
		result *Access
	}{
		{"branches/" + url.PathEscape(branch) + "/protection", &out.Protection},
		{"rules/branches/" + url.PathEscape(branch), &out.Rules},
	} {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/repos/"+repository+"/"+probe.path, nil)
		if err != nil {
			return out, errors.New("construct capability lookup")
		}
		if api.Token != "" {
			request.Header.Set("Authorization", "Bearer "+api.Token)
		}
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
		response, err := client.Do(request)
		if err != nil {
			return out, errors.New("capability lookup unavailable")
		}
		_ = response.Body.Close()
		probe.result.HTTP = response.StatusCode
		switch response.StatusCode {
		case http.StatusOK:
			probe.result.State = "readable"
		case http.StatusForbidden:
			probe.result.State = "access_or_plan_restricted"
		case http.StatusNotFound:
			probe.result.State = "absent_or_not_visible"
		default:
			probe.result.State = "unavailable"
		}
	}
	return out, nil
}
