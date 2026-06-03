package changed

import (
	"os/exec"
	"path/filepath"
	"sort"

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
			ok, err := filepath.Match(rule.Pattern, file)
			if err != nil || !ok {
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
