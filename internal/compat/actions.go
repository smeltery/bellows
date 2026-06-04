package compat

import "strings"

type ActionRule struct {
	Contains   string
	Status     string
	Feature    string
	Message    string
	Suggestion string
}

var ActionRules = []ActionRule{
	{"docker/login-action", "partial", "registry-login", "container registry login requires local credentials", "skip publish jobs or provide local registry credentials"},
	{"github-script", "partial", "github-api", "GitHub API calls may need a local token", "guard the step for local runs or provide a scoped token"},
	{"softprops/action-gh-release", "unsupported", "release-publish", "release publishing should stay in cloud CI", "add this job to skip.jobs"},
	{"goreleaser/goreleaser-action", "unsupported", "release-publish", "release publishing should stay in cloud CI", "add this job to skip.jobs"},
}

func MatchAction(uses string) (ActionRule, bool) {
	uses = strings.ToLower(uses)
	for _, rule := range ActionRules {
		if strings.Contains(uses, rule.Contains) {
			return rule, true
		}
	}
	return ActionRule{}, false
}
