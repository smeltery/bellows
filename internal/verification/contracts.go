package verification

import (
	"context"
	"io"
	"time"
)

const (
	ReceiptVersion = 1
	ModeMerge      = "merge"
	ModeHead       = "head"
	ExecutorAct    = "act"
	ExecutorFlox   = "flox"
)

type State string

const (
	Pending     State = "pending"
	Passed      State = "passed"
	Failed      State = "failed"
	Skipped     State = "skipped"
	Cancelled   State = "cancelled"
	Unsupported State = "unsupported"
	Error       State = "error"
)

type InputState string

const (
	Ready    InputState = "ready"
	Dirty    InputState = "dirty"
	Conflict InputState = "conflict"
	Stale    InputState = "stale"
	Invalid  InputState = "error"
)

type Fingerprints struct {
	Workflow     string `json:"workflow"`
	Config       string `json:"config"`
	Policy       string `json:"policy"`
	Dependencies string `json:"dependencies"`
	Toolchain    string `json:"toolchain"`
	Environment  string `json:"environment"`
}

type Identity struct {
	Repository   string       `json:"repository"`
	Head         string       `json:"head"`
	Base         string       `json:"base"`
	Tree         string       `json:"tree"`
	Mode         string       `json:"mode"`
	Executor     string       `json:"executor"`
	Platform     string       `json:"platform"`
	Fingerprints Fingerprints `json:"fingerprints"`
}

type Check struct {
	ID       string   `json:"id"`
	Required bool     `json:"required"`
	Needs    []string `json:"needs"`
}

type Result struct {
	ID         string `json:"id"`
	State      State  `json:"state"`
	ExitCode   *int   `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
}

type Receipt struct {
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Version     int        `json:"version"`
	RunID       string     `json:"run_id"`
	Identity    Identity   `json:"identity"`
	Input       InputState `json:"input"`
	Checks      []Check    `json:"checks"`
	Results     []Result   `json:"results"`
}

type Request struct {
	RunID    string
	Worktree string
	Identity Identity
	Check    Check
	Argv     []string
	Workflow string
	Job      string
	Matrix   map[string]string
	Env      []string
	Stdout   io.Writer
	Stderr   io.Writer
}

type Executor interface {
	Run(context.Context, Request) (Result, error)
}

// Evaluate aggregates a validated plan; it does not authenticate its producer.
func Evaluate(input InputState, checks []Check, results []Result) State {
	if input != Ready {
		return Error
	}
	planned := make(map[string]Check, len(checks))
	for _, check := range checks {
		if _, exists := planned[check.ID]; exists || check.ID == "" {
			return Error
		}
		planned[check.ID] = check
	}
	observed := make(map[string]Result, len(results))
	for _, result := range results {
		if _, exists := planned[result.ID]; !exists {
			return Error
		}
		if _, exists := observed[result.ID]; exists || result.DurationMS < 0 {
			return Error
		}
		switch result.State {
		case Pending, Failed, Skipped, Cancelled, Unsupported, Error:
		case Passed:
			if result.ExitCode == nil || *result.ExitCode != 0 {
				return Error
			}
		default:
			return Error
		}
		observed[result.ID] = result
	}
	required := false
	for _, check := range checks {
		result, exists := observed[check.ID]
		if result.State == Passed {
			for _, dependency := range check.Needs {
				if observed[dependency].State != Passed {
					return Error
				}
			}
		}
		if !check.Required {
			continue
		}
		required = true
		if !exists {
			return Error
		}
		if result.State != Passed {
			return result.State
		}
	}
	if !required {
		return Unsupported
	}
	return Passed
}
