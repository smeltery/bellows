package cmd

import (
	"context"
	v "github.com/smeltery/bellows/internal/verification"
	"testing"
)

func TestReportingFlagOptIn(t *testing.T) {
	t.Setenv("BELLOWS_REPORT_TOKEN", "fixture-secret")
	for _, opts := range []reportingFlags{{trusted: true}, {pull: 1}, {enabled: true, pull: 1}, {enabled: true, trusted: true}} {
		if _, err := opts.create("example/repo", func(context.Context) (v.Identity, error) { return v.Identity{}, nil }); err == nil {
			t.Fatalf("accepted %+v", opts)
		}
	}
	if got, err := (&reportingFlags{}).create("example/repo", func(context.Context) (v.Identity, error) { return v.Identity{}, nil }); err != nil || got != nil {
		t.Fatal("local-only default attempted reporting")
	}
	if _, err := (&reportingFlags{enabled: true, trusted: true, pull: 1}).create("example/repo", func(context.Context) (v.Identity, error) { return v.Identity{}, nil }); err != nil {
		t.Fatal(err)
	}
}
