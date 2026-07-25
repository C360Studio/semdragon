package main

import (
	"context"
	"os"
	"testing"
)

func TestMaybeSeedE2EEnvUnsetIsNoOp(t *testing.T) {
	t.Setenv("SEED_PROS", "")
	t.Setenv("SEED_E2E", "")
	t.Setenv("SEED_AGENTS", "")

	if err := maybeSeedE2E(context.Background(), nil, nil); err != nil {
		t.Fatalf("maybeSeedE2E with all seed env switches unset: %v", err)
	}
}

func TestStartupSeedModeFromEnvironment(t *testing.T) {
	tests := []struct {
		name       string
		pros       string
		e2e        string
		agentsOnly string
		want       string
	}{
		{name: "unset"},
		{name: "agents only", agentsOnly: "true", want: "agents-only"},
		{name: "E2E", e2e: "true", want: "e2e"},
		{name: "pros", pros: "true", want: "pros"},
		{name: "pros precedence", pros: "true", e2e: "true", agentsOnly: "true", want: "pros"},
		{name: "E2E precedence", e2e: "true", agentsOnly: "true", want: "e2e"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SEED_PROS", tt.pros)
			t.Setenv("SEED_E2E", tt.e2e)
			t.Setenv("SEED_AGENTS", tt.agentsOnly)
			if got := startupSeedMode(os.Getenv); got != tt.want {
				t.Errorf("startupSeedMode() = %q, want %q", got, tt.want)
			}
		})
	}
}
