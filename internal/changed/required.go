package changed

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/smeltery/bellows/internal/config"
	"github.com/smeltery/bellows/internal/verification"
)

type Target struct {
	Check    verification.Check `json:"check"`
	Workflow string             `json:"workflow"`
	Job      string             `json:"job"`
	Matrix   map[string]string  `json:"matrix"`
	Skipped  bool               `json:"skipped"`
}

type RequiredOptions struct {
	Rules       []config.PathRule
	SharedPaths []string
}

type RequiredSelection struct {
	Targets []Target            `json:"targets"`
	Reasons map[string][]string `json:"reasons"`
}

var ErrNoRequiredChecks = errors.New("no required checks selected; verification is unsupported")

// SelectRequired is fail-closed merge selection; legacy Select retains its pre-push semantics.
func SelectRequired(changes Changes, targets []Target, opts RequiredOptions) (RequiredSelection, error) {
	out := RequiredSelection{Targets: []Target{}, Reasons: map[string][]string{}}
	if changes.Head == "" || changes.Base == "" || changes.MergeBase == "" {
		return out, fmt.Errorf("required selection needs resolved committed changes")
	}
	ordered, err := targetOrder(targets)
	if err != nil {
		return out, err
	}
	patterns := slices.Clone(opts.SharedPaths)
	for _, rule := range opts.Rules {
		patterns = append(patterns, rule.Pattern)
	}
	for _, pattern := range patterns {
		for _, segment := range strings.Split(filepath.ToSlash(pattern), "/") {
			if _, err := filepath.Match(segment, ""); err != nil {
				return out, fmt.Errorf("invalid path pattern %q: %w", pattern, err)
			}
		}
	}
	byRule := make([][]Target, len(opts.Rules))
	for i, rule := range opts.Rules {
		if rule.Pattern == "" || len(rule.Workflows)+len(rule.Jobs) == 0 {
			return out, fmt.Errorf("path rule needs a pattern and check selectors")
		}
		for _, target := range ordered {
			if workflowMatches(target.Workflow, rule.Workflows) && (len(rule.Jobs) == 0 || slices.Contains(rule.Jobs, target.Job)) {
				byRule[i] = append(byRule[i], target)
			}
		}
		for _, ref := range rule.Workflows {
			if !slices.ContainsFunc(byRule[i], func(t Target) bool { return workflowMatches(t.Workflow, []string{ref}) }) {
				return out, fmt.Errorf("rule %q selects unknown workflow %q", rule.Pattern, ref)
			}
		}
		for _, job := range rule.Jobs {
			if !slices.ContainsFunc(byRule[i], func(t Target) bool { return t.Job == job }) {
				return out, fmt.Errorf("rule %q selects unknown job %q", rule.Pattern, job)
			}
		}
	}
	add := func(selected []Target, reason string) {
		for _, target := range selected {
			id := target.Check.ID
			if !slices.Contains(out.Reasons[id], reason) {
				out.Reasons[id] = append(out.Reasons[id], reason)
			}
		}
	}
	shared := append([]string{".github/**", ".flox/**", ".bellows.yml", ".pre-commit-config.yaml", "lefthook.yml", "**/Makefile", "**/Dockerfile*", "**/go.mod", "**/go.sum", "**/package.json", "**/*lock*", "**/*.config.*", ".nvmrc", ".tool-versions", ".semgrep-version"}, opts.SharedPaths...)
	if len(changes.Files) == 0 {
		add(ordered, "no changed paths: full verification")
	}
	for _, path := range changes.Files {
		if slices.ContainsFunc(shared, func(pattern string) bool { return match(pattern, path) }) {
			add(ordered, "shared input: "+path)
			continue
		}
		matched := false
		for i, rule := range opts.Rules {
			if match(rule.Pattern, path) {
				matched = true
				add(byRule[i], "path "+path+" matches "+rule.Pattern)
			}
		}
		if !matched {
			add(ordered, "unmatched path: "+path)
		}
	}
	for i := len(ordered) - 1; i >= 0; i-- {
		check := ordered[i].Check
		if len(out.Reasons[check.ID]) != 0 {
			for _, need := range check.Needs {
				out.Reasons[need] = append(out.Reasons[need], "prerequisite of "+check.ID)
			}
		}
	}
	required := false
	for _, target := range ordered {
		if reasons := out.Reasons[target.Check.ID]; len(reasons) != 0 {
			sort.Strings(reasons)
			out.Targets = append(out.Targets, target)
			required = required || target.Check.Required
		}
	}
	if !required {
		return out, ErrNoRequiredChecks
	}
	return out, nil
}

func targetOrder(targets []Target) ([]Target, error) {
	byID := map[string]Target{}
	for _, target := range targets {
		if _, duplicate := byID[target.Check.ID]; duplicate || target.Check.ID == "" {
			return nil, fmt.Errorf("duplicate or empty check ID %q", target.Check.ID)
		}
		byID[target.Check.ID] = target
	}
	var ordered []Target
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		target, exists := byID[id]
		if !exists || state[id] == 1 {
			return fmt.Errorf("missing or cyclic prerequisite %q", id)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		needs := slices.Clone(target.Check.Needs)
		sort.Strings(needs)
		for i, need := range needs {
			if i > 0 && needs[i-1] == need {
				return fmt.Errorf("duplicate prerequisite %q", need)
			}
			if err := visit(need); err != nil {
				return err
			}
		}
		state[id] = 2
		target.Check.Needs = needs
		ordered = append(ordered, target)
		return nil
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func workflowMatches(path string, refs []string) bool {
	return len(refs) == 0 || slices.Contains(refs, path) || slices.Contains(refs, filepath.Base(path))
}
