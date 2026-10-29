package precommit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLefthookCommandsPreserveFailures(t *testing.T) {
	for _, name := range []string{"lefthook.yml", "flox.yml"} {
		t.Run(name, func(t *testing.T) {
			var cfg struct {
				PrePush struct {
					Commands map[string]struct{ Run string }
				} `yaml:"pre-push"`
			}
			data, err := os.ReadFile(filepath.Join("testdata", "lefthook", name))
			if err != nil {
				t.Fatal(err)
			}
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(t.TempDir(), "repo with spaces")
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			for name, script := range map[string]string{
				"bellows": "#!/bin/sh\nprintf '%s\\n' \"$@\" > invocation\nexit \"$BELLOWS_TEST_EXIT\"\n",
				"flox":    "#!/bin/sh\n[ \"$1\" = activate ] && [ \"$2\" = -- ] || exit 92\nshift 2\nexec \"$@\"\n",
			} {
				if err := os.WriteFile(filepath.Join(root, name), []byte(script), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for _, status := range []string{"0", "37"} {
				cmd := exec.Command("sh", "-c", cfg.PrePush.Commands["bellows"].Run)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "BELLOWS_TEST_EXIT="+status)
				err := cmd.Run()
				if status == "0" && err != nil {
					t.Fatal(err)
				}
				if status != "0" && (err == nil || cmd.ProcessState.ExitCode() != 37) {
					t.Fatalf("failure not propagated: %v", err)
				}
				args, err := os.ReadFile(filepath.Join(root, "invocation"))
				if err != nil {
					t.Fatal(err)
				}
				if strings.TrimSpace(string(args)) != "run\n--precommit" {
					t.Fatalf("args: %q", args)
				}
			}
		})
	}
}
