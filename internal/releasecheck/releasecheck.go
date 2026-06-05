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
