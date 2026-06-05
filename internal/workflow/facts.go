package workflow

import "strings"

type ActionUse struct {
	Job  string
	Step Step
}

func (wf Workflow) ActionUses() []ActionUse {
	var uses []ActionUse
	for id, job := range wf.Jobs {
		for _, step := range job.Steps {
			if step.Uses != "" {
				uses = append(uses, ActionUse{Job: id, Step: step})
			}
		}
	}
	return uses
}

func (job Job) UsesOIDC() bool {
	return job.Permissions["id-token"] == "write"
}

func (job Job) UsesDeploymentEnvironment() bool {
	return len(job.Environment) > 0
}

func (job Job) MatrixRunnerLabels() []string {
	var labels []string
	for _, label := range job.RunsOn {
		if !isExpression(label) {
			continue
		}
		for key, values := range job.Matrix {
			if strings.Contains(label, "matrix."+key) {
				labels = append(labels, values...)
			}
		}
	}
	return labels
}

func (step Step) UsesAction(name string) bool {
	return strings.Contains(strings.ToLower(step.Uses), strings.ToLower(name))
}

func (step Step) IsGuardedForLocal() bool {
	condition := strings.ToLower(step.If)
	return strings.Contains(condition, "env.act") || strings.Contains(condition, "env.bellows")
}
