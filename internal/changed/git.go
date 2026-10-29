package changed

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

type Scope string

const (
	Staged      Scope = "staged"
	WorkingTree Scope = "working-tree"
	Outgoing    Scope = "outgoing"
	BaseHead    Scope = "base-head"
)

type Options struct {
	Scope Scope
	Base  string
	Head  string
}

type Changes struct {
	Files     []string
	Base      string
	Head      string
	MergeBase string
}

// GitFiles preserves the local-change API, including all three change categories.
func GitFiles(repoRoot string) ([]string, error) {
	changes, err := Discover(repoRoot, Options{Scope: WorkingTree})
	return changes.Files, err
}

// Discover compares committed scopes from the merge base to the pinned head.
// WorkingTree includes index, unstaged and untracked paths; local scopes have no commit identity.
func Discover(repoRoot string, opts Options) (Changes, error) {
	var changes Changes
	var commands [][]string
	switch opts.Scope {
	case Staged, WorkingTree:
		if opts.Base != "" || opts.Head != "" {
			return changes, fmt.Errorf("%s scope does not accept base or head refs", opts.Scope)
		}
		commands = append(commands, []string{"diff", "--cached", "--name-only", "--no-renames", "-z", "--"})
		if opts.Scope == WorkingTree {
			commands = append(commands, []string{"diff", "--name-only", "--no-renames", "-z", "--"}, []string{"ls-files", "--others", "--exclude-standard", "-z"})
		}
	case Outgoing, BaseHead:
		if opts.Head == "" {
			opts.Head = "HEAD"
		}
		if opts.Base == "" && opts.Scope == Outgoing {
			opts.Base = "@{upstream}"
		}
		if opts.Base == "" {
			return changes, fmt.Errorf("base-head scope requires a base ref")
		}
		shallow, err := gitOutput(repoRoot, "rev-parse", "--is-shallow-repository")
		if err != nil {
			return changes, err
		}
		if strings.TrimSpace(shallow) != "false" {
			return changes, fmt.Errorf("committed change discovery requires complete history; fetch --unshallow before retrying")
		}
		for _, ref := range []struct {
			name   string
			target *string
		}{{opts.Base, &changes.Base}, {opts.Head, &changes.Head}} {
			out, err := gitOutput(repoRoot, "rev-parse", "--verify", "--end-of-options", ref.name+"^{commit}")
			if err != nil {
				return Changes{}, fmt.Errorf("resolve ref %q: %w", ref.name, err)
			}
			*ref.target = strings.TrimSpace(out)
		}
		out, err := gitOutput(repoRoot, "merge-base", "--all", changes.Base, changes.Head)
		if err != nil {
			return Changes{}, fmt.Errorf("resolve merge base: %w", err)
		}
		bases := strings.Fields(out)
		if len(bases) != 1 {
			return Changes{}, fmt.Errorf("expected one merge base, found %d", len(bases))
		}
		changes.MergeBase = bases[0]
		commands = append(commands, []string{"diff", "--name-only", "--no-renames", "-z", changes.MergeBase, changes.Head, "--"})
	default:
		return changes, fmt.Errorf("unknown change scope %q", opts.Scope)
	}
	paths := map[string]bool{}
	for _, args := range commands {
		out, err := gitOutput(repoRoot, args...)
		if err != nil {
			return Changes{}, err
		}
		for _, path := range strings.Split(out, "\x00") {
			if path != "" {
				paths[path] = true
			}
		}
	}
	for path := range paths {
		changes.Files = append(changes.Files, path)
	}
	sort.Strings(changes.Files)
	return changes, nil
}

func gitOutput(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		if failure, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(failure.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return string(out), nil
}
