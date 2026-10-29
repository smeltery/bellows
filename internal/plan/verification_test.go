package plan

import (
	"net/url"
	"strings"
	"testing"

	"github.com/smeltery/bellows/internal/workflow"
)

func TestVerificationTargets(t *testing.T) {
	wf := workflow.Workflow{Path: ".github/workflows/ci.yml", Jobs: map[string]workflow.Job{
		"build": {Matrix: map[string][]string{"os": {"linux", "mac"}, "version": {"a&b=c", "d"}}},
		"test":  {Needs: []string{"build"}},
	}}
	targets, err := VerificationTargets([]workflow.Workflow{wf}, []string{"build"})
	if err != nil || len(targets) != 5 {
		t.Fatalf("%+v %v", targets, err)
	}
	for _, target := range targets {
		if target.Job == "test" {
			if len(target.Check.Needs) != 4 {
				t.Fatal(target)
			}
		} else {
			if !target.Skipped || !target.Check.Required {
				t.Fatal("skip lost")
			}
			values, err := url.ParseQuery(strings.SplitN(target.Check.ID, "?", 2)[1])
			if err != nil || values.Get("version") != target.Matrix["version"] {
				t.Fatal(target)
			}
		}
	}
}
func TestVerificationTargetsRejectUnsupported(t *testing.T) {
	for name, job := range map[string]workflow.Job{
		"dynamic": {UnsupportedMatrix: true}, "missing": {Needs: []string{"missing"}},
		"empty": {Matrix: map[string][]string{"os": {}}}, "expression": {Matrix: map[string][]string{"os": {"${{ inputs.os }}"}}},
		"cap":         {Matrix: map[string][]string{"os": make([]string, 257)}},
		"product cap": {Matrix: map[string][]string{"a": strings.Fields(strings.Repeat("x ", 17)), "b": strings.Fields(strings.Repeat("y ", 16))}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := VerificationTargets([]workflow.Workflow{{Path: "ci.yml", Jobs: map[string]workflow.Job{"test": job}}}, nil); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
