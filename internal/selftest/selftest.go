package selftest

import (
	"fmt"
	"os/exec"

	"github.com/smeltery/bellows/internal/config"
	"github.com/smeltery/bellows/internal/plan"
	"github.com/smeltery/bellows/internal/workflow"
)

type Check struct {
	Level      string `json:"level"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

func Run(repoRoot, version string) []Check {
	checks := []Check{
		ok("version", fmt.Sprintf("bellows %s is installed", version)),
		dependency("act", "bellows run requires act"),
		docker(),
	}
	cfg, err := config.Load(repoRoot)
	if err != nil {
		return append(checks, warn("config", fmt.Sprintf("config did not load: %v", err), "run bellows config validate"))
	}
	checks = append(checks, ok("config", "config loaded"))
	workflows, err := workflow.Discover(repoRoot)
	if err != nil {
		return append(checks, warn("workflow_discovery", fmt.Sprintf("workflows did not load: %v", err), "check .github/workflows YAML"))
	}
	if len(workflows) == 0 {
		checks = append(checks, warn("workflow_discovery", "no workflows discovered", "add .github/workflows/*.yml or pass --workflow to bellows run"))
	} else {
		checks = append(checks, ok("workflow_discovery", fmt.Sprintf("discovered %d workflow(s)", len(workflows))))
	}
	executionPlan := plan.Build(cfg, workflows, plan.Options{WorkflowRefs: cfg.DefaultWorkflows})
	checks = append(checks, ok("plan", fmt.Sprintf("resolved %d workflow(s)", len(executionPlan.Workflows))))
	return checks
}

func dependency(name, suggestion string) Check {
	if _, err := exec.LookPath(name); err != nil {
		return warn(name+"_missing", fmt.Sprintf("%s was not found in PATH", name), suggestion)
	}
	return ok(name+"_available", fmt.Sprintf("%s is available", name))
}

func docker() Check {
	if _, err := exec.LookPath("docker"); err != nil {
		return warn("docker_missing", "docker was not found in PATH", "install Docker or a compatible Docker CLI")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		return warn("docker_unreachable", "Docker is installed but not reachable", "start Docker and rerun bellows self-test")
	}
	return ok("docker_reachable", "Docker is reachable")
}

func ok(code, message string) Check {
	return Check{Level: "ok", Code: code, Message: message}
}

func warn(code, message, suggestion string) Check {
	return Check{Level: "warn", Code: code, Message: message, Suggestion: suggestion}
}
