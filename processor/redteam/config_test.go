package redteam

import (
	"testing"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// DefaultConfig tests
// =============================================================================

func TestDefaultConfig_ReturnsValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultConfig().Validate() = %v, want nil", err)
	}
}

func TestDefaultConfig_Fields(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Org == "" {
		t.Error("Org should not be empty")
	}
	if cfg.Platform == "" {
		t.Error("Platform should not be empty")
	}
	if cfg.Board == "" {
		t.Error("Board should not be empty")
	}
	if cfg.ClaimTimeoutSec <= 0 {
		t.Error("ClaimTimeout should be positive")
	}
	if cfg.ExecutionTimeoutSec <= 0 {
		t.Error("ExecutionTimeout should be positive")
	}
	if cfg.ExecutionTimeoutSec <= cfg.ClaimTimeoutSec {
		t.Error("ExecutionTimeout must be greater than ClaimTimeout in default config")
	}
	if cfg.MinDifficulty != domain.DifficultyModerate {
		t.Errorf("MinDifficulty = %v, want %v", cfg.MinDifficulty, domain.DifficultyModerate)
	}
	if !cfg.PreferCrossGuild {
		t.Error("PreferCrossGuild should default to true")
	}
}

// =============================================================================
// Validate tests
// =============================================================================

func TestValidate_RequiresOrg(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Org = ""
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should return error when Org is empty")
	}
}

func TestValidate_RequiresPlatform(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Platform = ""
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should return error when Platform is empty")
	}
}

func TestValidate_RequiresBoard(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Board = ""
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should return error when Board is empty")
	}
}

func TestValidate_RequiresPositiveClaimTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ClaimTimeoutSec = 0
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should return error when ClaimTimeout is zero")
	}

	cfg.ClaimTimeoutSec = -1
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should return error when ClaimTimeout is negative")
	}
}

func TestValidate_RequiresPositiveExecutionTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ExecutionTimeoutSec = 0
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should return error when ExecutionTimeout is zero")
	}

	cfg.ExecutionTimeoutSec = -1
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should return error when ExecutionTimeout is negative")
	}
}

func TestValidate_ExecutionTimeoutMustExceedClaimTimeout(t *testing.T) {
	tests := []struct {
		name        string
		claimSec    int
		execSec     int
		wantErr     bool
	}{
		{"execution equals claim", 120, 120, true},
		{"execution less than claim", 300, 120, true},
		{"execution greater than claim", 120, 300, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.ClaimTimeoutSec = tt.claimSec
			cfg.ExecutionTimeoutSec = tt.execSec
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// ToBoardConfig tests
// =============================================================================

func TestToBoardConfig_MapsFields(t *testing.T) {
	cfg := Config{
		Org:      "myorg",
		Platform: "staging",
		Board:    "board2",
	}
	bc := cfg.ToBoardConfig()
	if bc.Org != "myorg" {
		t.Errorf("Org = %q, want %q", bc.Org, "myorg")
	}
	if bc.Platform != "staging" {
		t.Errorf("Platform = %q, want %q", bc.Platform, "staging")
	}
	if bc.Board != "board2" {
		t.Errorf("Board = %q, want %q", bc.Board, "board2")
	}
}
