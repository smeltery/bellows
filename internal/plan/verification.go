package plan

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/smeltery/bellows/internal/changed"
	"github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/workflow"
)

// VerificationTargets expands already-selected workflows; skips remain required, visible checks.
func VerificationTargets(workflows []workflow.Workflow, skippedJobs []string) ([]changed.Target, error) {
	var targets []changed.Target
	for _, wf := range workflows {
		jobs := map[string][]string{}
		start := len(targets)
		for _, id := range workflow.JobIDs(wf) {
			job := wf.Jobs[id]
			if job.UnsupportedMatrix {
				return nil, fmt.Errorf("%s job %s: matrix requires unsupported dynamic/include/exclude/object evaluation", wf.Path, id)
			}
			matrices, err := staticMatrices(job.Matrix)
			if err != nil {
				return nil, fmt.Errorf("%s job %s: %w", wf.Path, id, err)
			}
			for _, matrix := range matrices {
				checkID := filepath.ToSlash(wf.Path) + "/" + id
				if len(matrix) != 0 {
					values := url.Values{}
					for key, value := range matrix {
						values.Set(key, value)
					}
					checkID += "?" + values.Encode()
				}
				jobs[id] = append(jobs[id], checkID)
				targets = append(targets, changed.Target{Check: verification.Check{ID: checkID, Required: true, Needs: []string{}}, Workflow: wf.Path, Job: id, Matrix: matrix, Skipped: contains(skippedJobs, id)})
			}
		}
		for i := start; i < len(targets); i++ {
			for _, need := range wf.Jobs[targets[i].Job].Needs {
				if len(jobs[need]) == 0 {
					return nil, fmt.Errorf("%s job %s: missing prerequisite %s", wf.Path, targets[i].Job, need)
				}
				targets[i].Check.Needs = append(targets[i].Check.Needs, jobs[need]...)
			}
			sort.Strings(targets[i].Check.Needs)
		}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Check.ID < targets[j].Check.ID })
	return targets, nil
}

func staticMatrices(matrix map[string][]string) ([]map[string]string, error) {
	rows := []map[string]string{{}}
	keys := make([]string, 0, len(matrix))
	for key := range matrix {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values := matrix[key]
		if key == "" || key == "include" || key == "exclude" || len(values) == 0 || len(rows) > 256/len(values) {
			return nil, fmt.Errorf("matrix must have nonempty static axes and at most 256 combinations")
		}
		var next []map[string]string
		for _, row := range rows {
			for _, value := range values {
				if value == "" || strings.Contains(value, "${{") {
					return nil, fmt.Errorf("matrix values must be nonempty static scalars")
				}
				expanded := map[string]string{key: value}
				for name, existing := range row {
					expanded[name] = existing
				}
				next = append(next, expanded)
			}
		}
		rows = next
	}
	return rows, nil
}
