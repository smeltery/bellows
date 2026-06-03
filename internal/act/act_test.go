package act

import (
	"reflect"
	"testing"

	"github.com/dotbrains/bellows/internal/config"
)

func TestArgsBuildsActCommand(t *testing.T) {
	t.Setenv("NPM_TOKEN", "token")
	cfg := config.DefaultConfig()
	cfg.DefaultWorkflows = []string{"ci.yml"}
	cfg.Runner.Map["blacksmith-4vcpu-ubuntu-2204"] = "catthehacker/ubuntu:act-22.04"
	cfg.Docker.Platform = "linux/amd64"
	cfg.Secrets.Mode = "env"
	cfg.Secrets.Allow = []string{"NPM_TOKEN", "MISSING_TOKEN"}
	cfg.Skip.Jobs = []string{"deploy"}

	got, err := Args(Options{
		Workflow: ".github/workflows/ci.yml",
		Job:      "test",
		Config:   cfg,
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"pull_request",
		"--workflows", ".github/workflows/ci.yml",
		"--job", "test",
		"--platform", "blacksmith-4vcpu-ubuntu-2204=catthehacker/ubuntu:act-22.04",
		"--platform", "ubuntu-22.04=catthehacker/ubuntu:act-22.04",
		"--platform", "ubuntu-24.04=catthehacker/ubuntu:act-24.04",
		"--platform", "ubuntu-latest=catthehacker/ubuntu:act-22.04",
		"--container-architecture", "linux/amd64",
		"--secret", "NPM_TOKEN",
		"--skip-job", "deploy",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Args mismatch\n got: %#v\nwant: %#v", got, want)
	}
}
