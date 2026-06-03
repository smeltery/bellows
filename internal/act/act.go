package act

import (
	"fmt"
	"os"
	"sort"

	"github.com/dotbrains/bellows/internal/config"
)

type Options struct {
	Event    string
	Workflow string
	Job      string
	Config   *config.Config
}

func Args(opts Options) ([]string, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("missing config")
	}
	event := opts.Event
	if event == "" {
		event = opts.Config.Event
	}
	args := []string{event}
	if opts.Workflow != "" {
		args = append(args, "--workflows", opts.Workflow)
	}
	if opts.Job != "" {
		args = append(args, "--job", opts.Job)
	}
	for _, platform := range platforms(opts.Config.Runner.Map) {
		args = append(args, "--platform", platform)
	}
	if opts.Config.Docker.Platform != "" {
		args = append(args, "--container-architecture", opts.Config.Docker.Platform)
	}
	switch opts.Config.Docker.Pull {
	case "always":
		args = append(args, "--pull")
	case "never":
		args = append(args, "--action-offline-mode")
	}
	for _, name := range opts.Config.Secrets.Allow {
		if opts.Config.Secrets.Mode == "env" && os.Getenv(name) != "" {
			args = append(args, "--secret", name)
		}
	}
	for _, job := range opts.Config.Skip.Jobs {
		args = append(args, "--skip-job", job)
	}
	return args, nil
}

func platforms(mapping map[string]string) []string {
	labels := make([]string, 0, len(mapping))
	for label := range mapping {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		if mapping[label] != "" {
			out = append(out, fmt.Sprintf("%s=%s", label, mapping[label]))
		}
	}
	return out
}
