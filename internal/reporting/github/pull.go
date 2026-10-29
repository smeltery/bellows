package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PullIdentity reads live same-repository PR inputs; closed and fork PRs are unsupported.
func (c Client) PullIdentity(ctx context.Context, repo string, number int) (head, base string, err error) {
	if !repository.MatchString(repo) || number < 1 || strings.TrimSpace(c.Token) == "" || strings.ContainsAny(c.Token, "\r\n") {
		return "", "", errors.New("explicit repository, pull request and reporting token required")
	}
	endpoint := c.APIURL
	if endpoint == "" {
		endpoint = "https://api.github.com"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawPath != "" {
		return "", "", errors.New("invalid reporting API endpoint")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + fmt.Sprintf("/repos/%s/pulls/%d", repo, number)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", "", errors.New("construct pull request lookup")
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	client := http.Client{}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return "", "", errors.New("pull request lookup failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("pull request lookup rejected (HTTP %d)", response.StatusCode)
	}
	var data struct {
		State      string
		Head, Base struct {
			SHA  string
			Repo struct {
				FullName string `json:"full_name"`
			}
		}
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&data); err != nil {
		return "", "", errors.New("invalid pull request response")
	}
	if data.State != "open" || data.Head.Repo.FullName != repo || data.Base.Repo.FullName != repo || !objectID.MatchString(data.Head.SHA) || !objectID.MatchString(data.Base.SHA) {
		return "", "", errors.New("reporting requires an open same-repository pull request with resolved inputs")
	}
	return data.Head.SHA, data.Base.SHA, nil
}
