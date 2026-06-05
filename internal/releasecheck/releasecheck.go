package releasecheck

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Runner struct {
	Root   string
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
}

func (r Runner) Run() error {
	steps := [][]string{
		{"go", "test", "./..."},
		{"golangci-lint", "run"},
		{"actionlint"},
		{"goreleaser", "check"},
		{"go", "run", ".", "config", "validate"},
		{"go", "run", ".", "policy"},
		{"go", "run", ".", "compat"},
		{"go", "run", ".", "run", "--list", "--estimate"},
		{"goreleaser", "release", "--snapshot", "--clean", "--skip=publish"},
	}
	for _, step := range steps {
		if _, err := fmt.Fprintf(r.out(), "$ %s\n", strings.Join(step, " ")); err != nil {
			return err
		}
		if err := r.run(step[0], step[1:]...); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(r.out(), "$ git changelog check"); err != nil {
		return err
	}
	if err := r.checkChangelog(); err != nil {
		return err
	}
	archive := fmt.Sprintf("dist/bellows_%s_%s.tar.gz", goEnv("GOOS"), goEnv("GOARCH"))
	smoke := filepath.Join(r.Root, "dist", "smoke")
	_ = os.RemoveAll(smoke)
	if err := os.MkdirAll(smoke, 0o755); err != nil {
		return err
	}
	if err := r.run("tar", "-xzf", archive, "-C", "dist/smoke"); err != nil {
		return err
	}
	if err := r.run(filepath.Join("dist", "smoke", "bellows"), "--version"); err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join("dist", "smoke", "bellows"), "completion", "bash")
	cmd.Dir = r.Root
	cmd.Stdout = io.Discard
	cmd.Stderr = r.errOut()
	return cmd.Run()
}

func (r Runner) checkChangelog() error {
	previous, err := commandOutput(r.Root, "git", "describe", "--tags", "--abbrev=0", "HEAD^")
	if err != nil {
		return nil
	}
	previous = strings.TrimSpace(previous)
	out, err := commandOutput(r.Root, "git", "log", "--format=%s", previous+"..HEAD")
	if err != nil {
		return fmt.Errorf("checking changelog commits: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "docs:") || strings.HasPrefix(line, "chore:") {
			continue
		}
		return nil
	}
	return fmt.Errorf("release changelog would be empty; add a non-docs/non-chore commit before publishing")
}

func (r Runner) run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = r.Root
	cmd.Stdout = r.out()
	cmd.Stderr = r.errOut()
	cmd.Stdin = r.in()
	return cmd.Run()
}

func (r Runner) out() io.Writer {
	if r.Stdout != nil {
		return r.Stdout
	}
	return os.Stdout
}

func (r Runner) errOut() io.Writer {
	if r.Stderr != nil {
		return r.Stderr
	}
	return os.Stderr
}

func (r Runner) in() io.Reader {
	if r.Stdin != nil {
		return r.Stdin
	}
	return os.Stdin
}

func goEnv(name string) string {
	out, err := exec.Command("go", "env", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func commandOutput(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}
