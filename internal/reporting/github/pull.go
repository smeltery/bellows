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
	rootPath := strings.TrimRight(parsed.Path, "/") + "/repos/" + repo
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client := http.Client{}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	lookup := func(path string, target any) error {
		endpoint := *parsed
		endpoint.Path = rootPath + path
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return errors.New("construct reporting identity lookup")
		}
		request.Header.Set("Authorization", "Bearer "+c.Token)
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
		response, err := client.Do(request)
		if err != nil {
			return errors.New("reporting identity lookup failed")
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("reporting identity lookup rejected (HTTP %d)", response.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		if err != nil || len(body) > 1<<20 || json.Unmarshal(body, target) != nil {
			return errors.New("invalid reporting identity response")
		}
		return nil
	}
	var data struct {
		State      string
		Head, Base struct {
			SHA, Ref string
			Repo     struct {
				FullName string `json:"full_name"`
			}
		}
	}
	if err = lookup(fmt.Sprintf("/pulls/%d", number), &data); err != nil {
		return "", "", err
	}
	if data.State != "open" || data.Head.Repo.FullName != repo || data.Base.Repo.FullName != repo || !objectID.MatchString(data.Head.SHA) || !objectID.MatchString(data.Base.SHA) {
		return "", "", errors.New("reporting requires an open same-repository pull request with resolved inputs")
	}
	resolve := func(branch string) (string, error) {
		if !validBranch(branch) {
			return "", errors.New("invalid pull request branch")
		}
		var ref struct {
			Ref    string
			Object struct{ Type, SHA string }
		}
		if err := lookup("/git/ref/heads/"+branch, &ref); err != nil {
			return "", err
		}
		if ref.Ref != "refs/heads/"+branch || ref.Object.Type != "commit" || !objectID.MatchString(ref.Object.SHA) {
			return "", errors.New("branch lookup did not resolve the exact commit reference")
		}
		return ref.Object.SHA, nil
	}
	// PR snapshot SHAs can lag branch movement; resolve both actual branch refs.
	if head, err = resolve(data.Head.Ref); err != nil {
		return "", "", err
	}
	if base, err = resolve(data.Base.Ref); err != nil {
		return "", "", err
	}
	return head, base, nil
}

func validBranch(branch string) bool {
	if branch == "" || branch == "@" || strings.Contains(branch, "..") || strings.Contains(branch, "@{") || strings.HasSuffix(branch, ".") {
		return false
	}
	for _, c := range branch {
		if c <= ' ' || c == 127 || strings.ContainsRune("~^:?*[\\", c) {
			return false
		}
	}
	for _, part := range strings.Split(branch, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}
