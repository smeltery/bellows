package secrets

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/smeltery/bellows/internal/config"
	"github.com/smeltery/bellows/internal/workflow"
)

var secretRef = regexp.MustCompile(`secrets\.([A-Za-z_][A-Za-z0-9_]*)`)

type Reference struct {
	Workflow string `json:"workflow"`
	Job      string `json:"job"`
	Step     string `json:"step,omitempty"`
	Secret   string `json:"secret"`
	Allowed  bool   `json:"allowed"`
}

func Audit(cfg *config.Config, workflows []workflow.Workflow) []Reference {
	allowed := map[string]bool{}
	for _, name := range cfg.Secrets.Allow {
		allowed[name] = true
	}
	seen := map[string]bool{}
	var refs []Reference
	for _, wf := range workflows {
		for jobID, job := range wf.Jobs {
			for i, step := range job.Steps {
				stepName := step.Name
				if stepName == "" {
					stepName = fmt.Sprintf("step %d", i+1)
				}
				for _, value := range stepValues(step) {
					for _, match := range secretRef.FindAllStringSubmatch(value, -1) {
						ref := Reference{Workflow: wf.Path, Job: jobID, Step: stepName, Secret: match[1], Allowed: allowed[match[1]]}
						key := ref.Workflow + "\x00" + ref.Job + "\x00" + ref.Step + "\x00" + ref.Secret
						if seen[key] {
							continue
						}
						seen[key] = true
						refs = append(refs, ref)
					}
				}
			}
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Workflow != refs[j].Workflow {
			return refs[i].Workflow < refs[j].Workflow
		}
		if refs[i].Job != refs[j].Job {
			return refs[i].Job < refs[j].Job
		}
		if refs[i].Step != refs[j].Step {
			return refs[i].Step < refs[j].Step
		}
		return refs[i].Secret < refs[j].Secret
	})
	return refs
}

func stepValues(step workflow.Step) []string {
	values := []string{step.If, step.Run, step.Uses}
	for _, value := range step.With {
		if s, ok := value.(string); ok {
			values = append(values, s)
		}
	}
	return values
}
