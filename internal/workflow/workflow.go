package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

type Workflow struct {
	Path string
	Name string
	Jobs map[string]Job
}

type Job struct {
	RunsOn []string
	Needs  []string
	Steps  []Step
}

type Step struct {
	Name string
	Uses string
	Run  string
	If   string
}

type rawWorkflow struct {
	Name string            `yaml:"name"`
	Jobs map[string]rawJob `yaml:"jobs"`
}

type rawJob struct {
	RunsOn yaml.Node      `yaml:"runs-on"`
	Needs  yaml.Node      `yaml:"needs"`
	Steps  []rawStep      `yaml:"steps"`
	Uses   string         `yaml:"uses"`
	With   map[string]any `yaml:"with"`
}

type rawStep struct {
	Name string `yaml:"name"`
	Uses string `yaml:"uses"`
	Run  string `yaml:"run"`
	If   string `yaml:"if"`
}

func Discover(repoRoot string) ([]Workflow, error) {
	pattern := filepath.Join(repoRoot, ".github", "workflows", "*.yml")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	more, err := filepath.Glob(filepath.Join(repoRoot, ".github", "workflows", "*.yaml"))
	if err != nil {
		return nil, err
	}
	paths = append(paths, more...)
	sort.Strings(paths)
	workflows := make([]Workflow, 0, len(paths))
	for _, path := range paths {
		wf, err := Load(path)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err == nil {
			wf.Path = rel
		}
		workflows = append(workflows, wf)
	}
	return workflows, nil
}

func Load(path string) (Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Workflow{}, fmt.Errorf("reading workflow %s: %w", path, err)
	}
	var raw rawWorkflow
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Workflow{}, fmt.Errorf("parsing workflow %s: %w", path, err)
	}
	wf := Workflow{
		Path: path,
		Name: raw.Name,
		Jobs: map[string]Job{},
	}
	for id, rawJob := range raw.Jobs {
		steps := make([]Step, 0, len(rawJob.Steps))
		for _, step := range rawJob.Steps {
			steps = append(steps, Step(step))
		}
		wf.Jobs[id] = Job{
			RunsOn: nodeStrings(rawJob.RunsOn),
			Needs:  nodeStrings(rawJob.Needs),
			Steps:  steps,
		}
	}
	return wf, nil
}

func Resolve(repoRoot, ref string) string {
	if ref == "" {
		return ref
	}
	if filepath.IsAbs(ref) {
		return ref
	}
	if filepath.Dir(ref) == "." {
		return filepath.Join(repoRoot, ".github", "workflows", ref)
	}
	return filepath.Join(repoRoot, ref)
}

func Labels(workflows []Workflow) []string {
	seen := map[string]bool{}
	for _, wf := range workflows {
		for _, job := range wf.Jobs {
			for _, label := range job.RunsOn {
				seen[label] = true
			}
		}
	}
	labels := make([]string, 0, len(seen))
	for label := range seen {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return labels
}

func JobIDs(wf Workflow) []string {
	ids := make([]string, 0, len(wf.Jobs))
	for id := range wf.Jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func nodeStrings(node yaml.Node) []string {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Value == "" {
			return nil
		}
		return []string{node.Value}
	case yaml.SequenceNode:
		values := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			if item.Value != "" {
				values = append(values, item.Value)
			}
		}
		return values
	default:
		return nil
	}
}
