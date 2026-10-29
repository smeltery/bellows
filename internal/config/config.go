package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	FileName = ".bellows.yml"
	Version  = 1
)

type Config struct {
	Version          int                      `yaml:"version"`
	DefaultWorkflows []string                 `yaml:"default_workflows,omitempty"`
	Event            string                   `yaml:"event,omitempty"`
	Docker           DockerConfig             `yaml:"docker,omitempty"`
	Runner           RunnerConfig             `yaml:"runner,omitempty"`
	Secrets          SecretsConfig            `yaml:"secrets,omitempty"`
	Skip             SkipConfig               `yaml:"skip,omitempty"`
	Policy           PolicyConfig             `yaml:"policy,omitempty"`
	Precommit        PrecommitConfig          `yaml:"precommit,omitempty"`
	Profiles         map[string]NativeProfile `yaml:"profiles,omitempty"`
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

type PolicyConfig struct {
	CloudJobs string         `yaml:"cloud_jobs,omitempty"`
	Waivers   []PolicyWaiver `yaml:"waivers,omitempty"`
}

type PolicyWaiver struct {
	Code   string `yaml:"code,omitempty"`
	Job    string `yaml:"job,omitempty"`
	Reason string `yaml:"reason,omitempty"`
}

type PrecommitConfig struct {
	Stage          string     `yaml:"stage,omitempty"`
	Mode           string     `yaml:"mode,omitempty"`
	TimeoutSeconds int        `yaml:"timeout_seconds,omitempty"`
	Paths          []PathRule `yaml:"paths,omitempty"`
}

type PathRule struct {
	Pattern   string   `yaml:"pattern,omitempty"`
	Workflows []string `yaml:"workflows,omitempty"`
	Jobs      []string `yaml:"jobs,omitempty"`
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
				"ubuntu-latest": "ghcr.io/smeltery/kiln:ubuntu-22.04",
				"ubuntu-22.04":  "ghcr.io/smeltery/kiln:ubuntu-22.04",
				"ubuntu-24.04":  "ghcr.io/smeltery/kiln:ubuntu-24.04",
				"macos-latest":  "ghcr.io/smeltery/kiln:ubuntu-22.04",
				"macos-14":      "ghcr.io/smeltery/kiln:ubuntu-22.04",
				"macos-15":      "ghcr.io/smeltery/kiln:ubuntu-22.04",
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
	if err := validateSchemaShape(data); err != nil {
		return nil, err
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := cfg.ValidatePaths(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validateSchemaShape(data []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	root := mappingRoot(&doc)
	if root == nil {
		return nil
	}
	allowed := map[string]map[string]bool{
		"":          keys("version", "default_workflows", "event", "docker", "runner", "secrets", "skip", "policy", "precommit", "profiles"),
		"docker":    keys("platform", "pull"),
		"runner":    keys("default_image", "map"),
		"secrets":   keys("mode", "allow"),
		"skip":      keys("jobs", "steps_matching"),
		"policy":    keys("cloud_jobs", "waivers"),
		"precommit": keys("stage", "mode", "timeout_seconds", "paths"),
	}
	if profiles := mappingValue(root, "profiles"); profiles != nil && profiles.Kind != yaml.MappingNode {
		return fmt.Errorf("profiles must be a mapping")
	}
	return validateKeys(root, "", allowed)
}

func validateKeys(node *yaml.Node, path string, allowed map[string]map[string]bool) error {
	keys, ok := allowed[path]
	if !ok || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if !keys[key] {
			if path == "" {
				return fmt.Errorf("unsupported config key %q", key)
			}
			return fmt.Errorf("unsupported config key %q under %s", key, path)
		}
		if err := validateKeys(node.Content[i+1], keyPath(path, key), allowed); err != nil {
			return err
		}
	}
	return nil
}

func keys(values ...string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}

func keyPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func SaveTo(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

func UpgradeFile(path string, dryRun bool) ([]byte, bool, error) {
	cfg, err := LoadFrom(path)
	if err != nil {
		return nil, false, err
	}
	if cfg.Version != Version {
		return nil, false, fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			data, err := Marshal(cfg)
			if err != nil {
				return nil, false, err
			}
			if !dryRun {
				if err := SaveTo(cfg, path); err != nil {
					return nil, false, err
				}
			}
			return data, true, nil
		}
		return nil, false, fmt.Errorf("reading config: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(current, &doc); err != nil {
		return nil, false, fmt.Errorf("parsing config %s: %w", path, err)
	}
	root := mappingRoot(&doc)
	if root == nil {
		data, err := Marshal(cfg)
		if err != nil {
			return nil, false, err
		}
		if !dryRun {
			if err := SaveTo(cfg, path); err != nil {
				return nil, false, err
			}
		}
		return data, true, nil
	}
	upsertScalarTagged(root, "version", fmt.Sprintf("%d", Version), "!!int")
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, false, fmt.Errorf("marshaling config: %w", err)
	}
	changed := string(out) != string(current)
	if changed && !dryRun {
		if err := os.WriteFile(path, out, 0o644); err != nil {
			return nil, false, fmt.Errorf("writing config: %w", err)
		}
	}
	return out, changed, nil
}

func PatchSafeFixes(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SaveTo(cfg, path)
		}
		return fmt.Errorf("reading config: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing config %s: %w", path, err)
	}
	root := mappingRoot(&doc)
	if root == nil {
		return SaveTo(cfg, path)
	}
	upsertScalarTagged(root, "version", fmt.Sprintf("%d", cfg.Version), "!!int")
	if len(cfg.Runner.Map) > 0 {
		runnerNode := ensureMapping(root, "runner")
		mapNode := ensureMapping(runnerNode, "map")
		labels := make([]string, 0, len(cfg.Runner.Map))
		for label := range cfg.Runner.Map {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		for _, label := range labels {
			upsertScalar(mapNode, label, cfg.Runner.Map[label])
		}
	}
	if len(cfg.Skip.Jobs) > 0 {
		skipNode := ensureMapping(root, "skip")
		jobsNode := ensureSequence(skipNode, "jobs")
		seen := map[string]bool{}
		for _, item := range jobsNode.Content {
			seen[item.Value] = true
		}
		jobs := append([]string(nil), cfg.Skip.Jobs...)
		sort.Strings(jobs)
		for _, job := range jobs {
			if seen[job] {
				continue
			}
			jobsNode.Content = append(jobsNode.Content, scalarNode(job))
		}
	}
	if len(cfg.Precommit.Paths) > 0 {
		precommitNode := ensureMapping(root, "precommit")
		pathsNode := ensureSequence(precommitNode, "paths")
		seen := map[string]bool{}
		for _, item := range pathsNode.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			if pattern := mappingValue(item, "pattern"); pattern != nil {
				seen[pattern.Value] = true
			}
		}
		for _, rule := range cfg.Precommit.Paths {
			if seen[rule.Pattern] {
				continue
			}
			pathsNode.Content = append(pathsNode.Content, pathRuleNode(rule))
			seen[rule.Pattern] = true
		}
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

func Marshal(cfg *Config) ([]byte, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshaling config: %w", err)
	}
	return data, nil
}

func mappingRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return nil
	}
	return doc
}

func ensureMapping(parent *yaml.Node, key string) *yaml.Node {
	if value := mappingValue(parent, key); value != nil {
		if value.Kind != yaml.MappingNode {
			value.Kind = yaml.MappingNode
			value.Tag = "!!map"
			value.Value = ""
			value.Content = nil
		}
		value.Style = 0
		return value
	}
	value := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	parent.Content = append(parent.Content, scalarNode(key), value)
	return value
}

func ensureSequence(parent *yaml.Node, key string) *yaml.Node {
	if value := mappingValue(parent, key); value != nil {
		if value.Kind != yaml.SequenceNode {
			value.Kind = yaml.SequenceNode
			value.Tag = "!!seq"
			value.Value = ""
			value.Content = nil
		}
		value.Style = 0
		return value
	}
	value := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	parent.Content = append(parent.Content, scalarNode(key), value)
	return value
}

func upsertScalar(parent *yaml.Node, key, value string) {
	upsertScalarTagged(parent, key, value, "!!str")
}

func upsertScalarTagged(parent *yaml.Node, key, value, tag string) {
	if node := mappingValue(parent, key); node != nil {
		node.Kind = yaml.ScalarNode
		node.Tag = tag
		node.Value = value
		return
	}
	parent.Content = append(parent.Content, scalarNode(key), &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value})
}

func mappingValue(parent *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			return parent.Content[i+1]
		}
	}
	return nil
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func pathRuleNode(rule PathRule) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	node.Content = append(node.Content, scalarNode("pattern"), scalarNode(rule.Pattern))
	if len(rule.Workflows) > 0 {
		node.Content = append(node.Content, scalarNode("workflows"), stringSequence(rule.Workflows))
	}
	if len(rule.Jobs) > 0 {
		node.Content = append(node.Content, scalarNode("jobs"), stringSequence(rule.Jobs))
	}
	return node
}

func stringSequence(values []string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		node.Content = append(node.Content, scalarNode(value))
	}
	return node
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
	switch c.Policy.CloudJobs {
	case "", "error", "warn", "off":
	default:
		return fmt.Errorf("unsupported policy.cloud_jobs %q", c.Policy.CloudJobs)
	}
	for i, waiver := range c.Policy.Waivers {
		if waiver.Code == "" {
			return fmt.Errorf("policy.waivers[%d].code is required", i)
		}
		if waiver.Reason == "" {
			return fmt.Errorf("policy.waivers[%d].reason is required", i)
		}
	}
	return c.ValidateProfiles()
}

func (c *Config) ValidatePaths() error {
	for i, rule := range c.Precommit.Paths {
		if rule.Pattern == "" {
			return fmt.Errorf("precommit.paths[%d].pattern is required", i)
		}
		if len(rule.Workflows) == 0 && len(rule.Jobs) == 0 {
			return fmt.Errorf("precommit.paths[%d] must select at least one workflow or job", i)
		}
	}
	return nil
}

func Minimal(workflows []string, runnerLabels []string, skippedJobs ...string) *Config {
	cfg := DefaultConfig()
	cfg.DefaultWorkflows = workflows
	cfg.Runner.Map = map[string]string{}
	for _, label := range runnerLabels {
		if image, ok := DefaultRunnerImage(label); ok {
			cfg.Runner.Map[label] = image
		}
	}
	if len(cfg.Runner.Map) == 0 {
		cfg.Runner.Map["ubuntu-latest"] = "ghcr.io/smeltery/kiln:ubuntu-22.04"
	}
	cfg.Skip.Jobs = append([]string(nil), skippedJobs...)
	return cfg
}

func DefaultRunnerImage(label string) (string, bool) {
	switch label {
	case "ubuntu-latest", "ubuntu-22.04", "macos-latest", "macos-14", "macos-15",
		"blacksmith-2vcpu-ubuntu-2204", "blacksmith-4vcpu-ubuntu-2204", "blacksmith-8vcpu-ubuntu-2204",
		"blacksmith-16vcpu-ubuntu-2204", "blacksmith-32vcpu-ubuntu-2204":
		return "ghcr.io/smeltery/kiln:ubuntu-22.04", true
	case "ubuntu-24.04",
		"blacksmith-2vcpu-ubuntu-2404", "blacksmith-4vcpu-ubuntu-2404", "blacksmith-8vcpu-ubuntu-2404",
		"blacksmith-16vcpu-ubuntu-2404", "blacksmith-32vcpu-ubuntu-2404":
		return "ghcr.io/smeltery/kiln:ubuntu-24.04", true
	default:
		return "", false
	}
}
