package changed

import (
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dotbrains/bellows/internal/config"
)

type Selection struct {
	Files     []string
	Workflows []string
	Jobs      []string
}

func GitFiles(repoRoot string) ([]string, error) {
	out, err := exec.Command("git", "-C", repoRoot, "diff", "--name-only", "--cached", "HEAD").Output()
	if err == nil {
		files := splitLines(string(out))
		if len(files) > 0 {
			return files, nil
		}
	}
	out, err = exec.Command("git", "-C", repoRoot, "diff", "--name-only", "HEAD").Output()
	if err == nil {
		files := splitLines(string(out))
		if len(files) > 0 {
			return files, nil
		}
	}
	out, err = exec.Command("git", "-C", repoRoot, "ls-files", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil, err
	}
	return splitLines(string(out)), nil
}

func Select(files []string, rules []config.PathRule, defaults []string) Selection {
	selection := Selection{Files: append([]string(nil), files...)}
	workflowSet := map[string]bool{}
	jobSet := map[string]bool{}
	for _, file := range files {
		for _, rule := range rules {
			if !match(rule.Pattern, file) {
				continue
			}
			for _, wf := range rule.Workflows {
				workflowSet[wf] = true
			}
			for _, job := range rule.Jobs {
				jobSet[job] = true
			}
		}
	}
	for wf := range workflowSet {
		selection.Workflows = append(selection.Workflows, wf)
	}
	for job := range jobSet {
		selection.Jobs = append(selection.Jobs, job)
	}
	sort.Strings(selection.Workflows)
	sort.Strings(selection.Jobs)
	if len(selection.Workflows) == 0 && len(selection.Jobs) > 0 {
		selection.Workflows = append([]string(nil), defaults...)
	}
	return selection
}

func match(pattern, file string) bool {
	pattern = filepath.ToSlash(pattern)
	file = filepath.ToSlash(file)
	if strings.Contains(pattern, "**") {
		return matchSegments(strings.Split(pattern, "/"), strings.Split(file, "/"))
	}
	ok, err := filepath.Match(pattern, file)
	return err == nil && ok
}

func matchSegments(pattern, file []string) bool {
	if len(pattern) == 0 {
		return len(file) == 0
	}
	if pattern[0] == "**" {
		if matchSegments(pattern[1:], file) {
			return true
		}
		return len(file) > 0 && matchSegments(pattern, file[1:])
	}
	if len(file) == 0 {
		return false
	}
	ok, err := filepath.Match(pattern[0], file[0])
	if err != nil || !ok {
		return false
	}
	return matchSegments(pattern[1:], file[1:])
}

func splitLines(out string) []string {
	var files []string
	start := 0
	for i, r := range out {
		if r == '\n' {
			if i > start {
				files = append(files, out[start:i])
			}
			start = i + 1
		}
	}
	if start < len(out) {
		files = append(files, out[start:])
	}
	return files
}
