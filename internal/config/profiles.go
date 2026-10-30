package config

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	v "github.com/smeltery/bellows/internal/verification"
	"gopkg.in/yaml.v3"
)

type NativeProfile struct {
	Executor string                 `yaml:"executor"`
	Checks   map[string]NativeCheck `yaml:"checks"`
}
type NativeCheck struct {
	Cacheable bool     `yaml:"cacheable,omitempty"`
	Argv      []string `yaml:"argv"`
	Needs     []string `yaml:"needs,omitempty"`
}

var profileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// Profile strictness is opt-in; legacy configuration decoding remains unchanged.
func (p *NativeProfile) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("profile must be a mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		if key == "executor" {
			continue
		}
		if key != "checks" || value.Kind != yaml.MappingNode {
			return fmt.Errorf("unsupported profile field or invalid checks: %s", key)
		}
		for j := 1; j < len(value.Content); j += 2 {
			check := value.Content[j]
			if check.Kind != yaml.MappingNode {
				return fmt.Errorf("profile check must be a mapping")
			}
			for k := 0; k < len(check.Content); k += 2 {
				field, list := check.Content[k].Value, check.Content[k+1]
				if field == "cacheable" {
					if list.Kind != yaml.ScalarNode || list.Tag != "!!bool" {
						return fmt.Errorf("cacheable must be a boolean")
					}
					continue
				}
				if (field != "argv" && field != "needs") || list.Kind != yaml.SequenceNode {
					return fmt.Errorf("unsupported check field or invalid list: %s", field)
				}
				for _, item := range list.Content {
					if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
						return fmt.Errorf("%s entries must be strings", field)
					}
				}
			}
		}
	}
	type plain NativeProfile
	return node.Decode((*plain)(p))
}

func (c *Config) ValidateProfiles() error {
	for name, profile := range c.Profiles {
		if !profileName.MatchString(name) || profile.Executor != v.ExecutorFlox || len(profile.Checks) == 0 {
			return fmt.Errorf("profile %q requires a valid name, executor flox and checks", name)
		}
		visited := map[string]uint8{}
		var visit func(string) error
		visit = func(id string) error {
			check, exists := profile.Checks[id]
			if !exists || visited[id] == 1 {
				return fmt.Errorf("profile %s has unknown or cyclic prerequisite %s", name, id)
			}
			if visited[id] == 2 {
				return nil
			}
			visited[id] = 1
			if !profileName.MatchString(id) || len(check.Argv) == 0 || strings.TrimSpace(check.Argv[0]) == "" {
				return fmt.Errorf("profile %s check %s requires a valid ID and argv command", name, id)
			}
			for _, arg := range check.Argv {
				if strings.ContainsRune(arg, 0) {
					return fmt.Errorf("profile argv cannot contain NUL")
				}
			}
			seen := map[string]bool{}
			for _, need := range check.Needs {
				if seen[need] {
					return fmt.Errorf("duplicate profile prerequisite %s", need)
				}
				seen[need] = true
				if err := visit(need); err != nil {
					return err
				}
			}
			visited[id] = 2
			return nil
		}
		for id := range profile.Checks {
			if err := visit(id); err != nil {
				return err
			}
		}
	}
	return nil
}

// NativeChecks returns a deterministic, entirely required plan with copied argv.
func (c *Config) NativeChecks(name string) ([]v.Check, map[string][]string, error) {
	if err := c.ValidateProfiles(); err != nil {
		return nil, nil, err
	}
	profile, exists := c.Profiles[name]
	if !exists {
		return nil, nil, fmt.Errorf("unknown native profile %q", name)
	}
	prefix := "profile/" + name + "/"
	ids := make([]string, 0, len(profile.Checks))
	for id := range profile.Checks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	checks := make([]v.Check, 0, len(ids))
	argv := map[string][]string{}
	seen := map[string]bool{}
	var appendCheck func(string)
	appendCheck = func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		check := profile.Checks[id]
		dependencies := slices.Clone(check.Needs)
		sort.Strings(dependencies)
		needs := make([]string, 0, len(dependencies))
		for _, need := range dependencies {
			appendCheck(need)
			needs = append(needs, prefix+need)
		}
		checks = append(checks, v.Check{ID: prefix + id, Required: true, Needs: needs})
		argv[prefix+id] = slices.Clone(check.Argv)
	}
	for _, id := range ids {
		appendCheck(id)
	}
	return checks, argv, nil
}
