package runner

import (
	"testing"

	"github.com/dotbrains/bellows/internal/config"
)

func TestResolveUsesConfigDefaultAndBuiltins(t *testing.T) {
	cfg := config.RunnerConfig{
		DefaultImage: "local/default:latest",
		Map: map[string]string{
			"custom": "local/custom:latest",
		},
	}
	if got, ok := Resolve("custom", cfg); !ok || got != "local/custom:latest" {
		t.Fatalf("custom Resolve = %q, %v", got, ok)
	}
	if got, ok := Resolve("blacksmith-4vcpu-ubuntu-2204", cfg); !ok || got != "ghcr.io/dotbrains/kiln:ubuntu-22.04" {
		t.Fatalf("blacksmith Resolve = %q, %v", got, ok)
	}
	if got, ok := Resolve("unknown", cfg); !ok || got != "local/default:latest" {
		t.Fatalf("default Resolve = %q, %v", got, ok)
	}
}

func TestIsBlacksmith(t *testing.T) {
	if !IsBlacksmith("blacksmith-4vcpu-ubuntu-2204") {
		t.Fatal("expected blacksmith label")
	}
	if IsBlacksmith("ubuntu-latest") {
		t.Fatal("did not expect ubuntu-latest to be blacksmith")
	}
}
