package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	FileName = ".bellows.yml"
	Version  = 1
)

type Config struct {
	Version          int             `yaml:"version"`
	DefaultWorkflows []string        `yaml:"default_workflows,omitempty"`
	Event            string          `yaml:"event,omitempty"`
	Docker           DockerConfig    `yaml:"docker,omitempty"`
	Runner           RunnerConfig    `yaml:"runner,omitempty"`
	Secrets          SecretsConfig   `yaml:"secrets,omitempty"`
	Skip             SkipConfig      `yaml:"skip,omitempty"`
	Precommit        PrecommitConfig `yaml:"precommit,omitempty"`
}

type DockerConfig struct {
	Platform string `yaml:"platform,omitempty"`
	Pull     string `yaml:"pull,omitempty"`
}

type RunnerConfig struct {
	DefaultImage string            `yaml:"default_image,omitempty"`
	Map          map[string]string `yaml:"map,omitempty"`
}

type SecretsConfig struct {
	Mode  string   `yaml:"mode,omitempty"`
	Allow []string `yaml:"allow,omitempty"`
}

type SkipConfig struct {
	Jobs          []string `yaml:"jobs,omitempty"`
	StepsMatching []string `yaml:"steps_matching,omitempty"`
}

type PrecommitConfig struct {
	Stage          string `yaml:"stage,omitempty"`
	Mode           string `yaml:"mode,omitempty"`
	TimeoutSeconds int    `yaml:"timeout_seconds,omitempty"`
}

func DefaultConfig() *Config {
	return &Config{
		Version: Version,
		Event:   "pull_request",
		Docker: DockerConfig{
			Pull: "missing",
		},
		Runner: RunnerConfig{
			Map: map[string]string{
				"ubuntu-latest": "catthehacker/ubuntu:act-22.04",
				"ubuntu-22.04":  "catthehacker/ubuntu:act-22.04",
				"ubuntu-24.04":  "catthehacker/ubuntu:act-24.04",
			},
		},
		Secrets: SecretsConfig{
			Mode: "none",
		},
		Precommit: PrecommitConfig{
			Stage:          "pre-push",
			Mode:           "full",
			TimeoutSeconds: 900,
		},
	}
}

func Load(repoRoot string) (*Config, error) {
	return LoadFrom(filepath.Join(repoRoot, FileName))
}

func LoadFrom(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func SaveTo(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

func (c *Config) ApplyDefaults() {
	defaults := DefaultConfig()
	if c.Version == 0 {
		c.Version = defaults.Version
	}
	if c.Event == "" {
		c.Event = defaults.Event
	}
	if c.Docker.Pull == "" {
		c.Docker.Pull = defaults.Docker.Pull
	}
	if c.Runner.Map == nil {
		c.Runner.Map = map[string]string{}
	}
	for label, image := range defaults.Runner.Map {
		if _, ok := c.Runner.Map[label]; !ok {
			c.Runner.Map[label] = image
		}
	}
	if c.Secrets.Mode == "" {
		c.Secrets.Mode = defaults.Secrets.Mode
	}
	if c.Precommit.Stage == "" {
		c.Precommit.Stage = defaults.Precommit.Stage
	}
	if c.Precommit.Mode == "" {
		c.Precommit.Mode = defaults.Precommit.Mode
	}
	if c.Precommit.TimeoutSeconds == 0 {
		c.Precommit.TimeoutSeconds = defaults.Precommit.TimeoutSeconds
	}
}

func (c *Config) Validate() error {
	if c.Version != Version {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	switch c.Event {
	case "pull_request", "push", "workflow_dispatch":
	default:
		return fmt.Errorf("unsupported event %q", c.Event)
	}
	switch c.Docker.Pull {
	case "missing", "always", "never":
	default:
		return fmt.Errorf("unsupported docker.pull %q", c.Docker.Pull)
	}
	switch c.Secrets.Mode {
	case "none", "env":
	default:
		return fmt.Errorf("unsupported secrets.mode %q", c.Secrets.Mode)
	}
	return nil
}

func Minimal(workflows []string, runnerLabels []string) *Config {
	cfg := DefaultConfig()
	cfg.DefaultWorkflows = workflows
	cfg.Runner.Map = map[string]string{}
	for _, label := range runnerLabels {
		if image, ok := DefaultRunnerImage(label); ok {
			cfg.Runner.Map[label] = image
		}
	}
	if len(cfg.Runner.Map) == 0 {
		cfg.Runner.Map["ubuntu-latest"] = "catthehacker/ubuntu:act-22.04"
	}
	return cfg
}

func DefaultRunnerImage(label string) (string, bool) {
	switch label {
	case "ubuntu-latest", "ubuntu-22.04",
		"blacksmith-2vcpu-ubuntu-2204", "blacksmith-4vcpu-ubuntu-2204", "blacksmith-8vcpu-ubuntu-2204":
		return "catthehacker/ubuntu:act-22.04", true
	case "ubuntu-24.04",
		"blacksmith-2vcpu-ubuntu-2404", "blacksmith-4vcpu-ubuntu-2404", "blacksmith-8vcpu-ubuntu-2404":
		return "catthehacker/ubuntu:act-24.04", true
	default:
		return "", false
	}
}
