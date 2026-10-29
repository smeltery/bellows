package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func profileConfig(t *testing.T, body string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte("version: 1\nprofiles:\n"+body), 0600); err != nil {
		t.Fatal(err)
	}
	return LoadFrom(path)
}
func TestNativeProfileValidation(t *testing.T) {
	for name, body := range map[string]string{
		"null profile":          "  bad: null",
		"null profiles":         "  null",
		"sequence profiles":     "  []",
		"unknown profile field": "  native: {executor: flox, shell: true, checks: {test: {argv: [echo]}}}",
		"unknown check field":   "  native: {executor: flox, checks: {test: {argv: [echo], run: echo}}}",
		"ambiguous executor":    "  native: {executor: act, checks: {test: {argv: [echo]}}}",
		"missing executor":      "  native: {checks: {test: {argv: [echo]}}}",
		"missing checks":        "  native: {executor: flox}",
		"empty checks":          "  native: {executor: flox, checks: {}}",
		"missing argv":          "  native: {executor: flox, checks: {test: {needs: []}}}",
		"empty argv":            "  native: {executor: flox, checks: {test: {argv: []}}}",
		"empty command":         "  native: {executor: flox, checks: {test: {argv: [' ']}}}",
		"numeric argv":          "  native: {executor: flox, checks: {test: {argv: [echo, 123]}}}",
		"null argv":             "  native: {executor: flox, checks: {test: {argv: null}}}",
		"unknown need":          "  native: {executor: flox, checks: {test: {argv: [echo], needs: [missing]}}}",
		"cycle":                 "  native: {executor: flox, checks: {a: {argv: [echo], needs: [b]}, b: {argv: [echo], needs: [a]}}}",
		"duplicate need":        "  native: {executor: flox, checks: {a: {argv: [echo]}, b: {argv: [echo], needs: [a,a]}}}",
		"duplicate check":       "  native: {executor: flox, checks: {test: {argv: [echo]}, test: {argv: [echo]}}}",
		"invalid name":          "  '../native': {executor: flox, checks: {test: {argv: [echo]}}}",
		"invalid check ID":      "  native: {executor: flox, checks: {'a/b': {argv: [echo]}}}",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := profileConfig(t, body); err == nil {
				t.Fatal("accepted invalid profile")
			}
		})
	}
}
func TestNativeCheckPlanCopiesAndPrefixes(t *testing.T) {
	cfg, err := profileConfig(t, "  native: {executor: flox, checks: {z: {argv: [echo, ''], needs: [a]}, a: {argv: [echo, '$(touch escaped)']}}}")
	if err != nil {
		t.Fatal(err)
	}
	checks, argv, err := cfg.NativeChecks("native")
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 2 || checks[0].ID != "profile/native/a" || !checks[0].Required || len(checks[0].Needs) != 0 || checks[1].Needs[0] != checks[0].ID {
		t.Fatalf("checks=%+v", checks)
	}
	argv[checks[0].ID][0] = "changed"
	if cfg.Profiles["native"].Checks["a"].Argv[0] != "echo" {
		t.Fatal("argv aliases config")
	}
	if _, _, err := cfg.NativeChecks("missing"); err == nil {
		t.Fatal("unknown selection accepted")
	}
	data, err := Marshal(cfg)
	if err != nil || !strings.Contains(string(data), "profiles:") {
		t.Fatalf("marshal: %v", err)
	}
}
