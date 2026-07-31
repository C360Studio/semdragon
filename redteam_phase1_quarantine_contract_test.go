package semdragons_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semstreams/component"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"
	semservice "github.com/c360studio/semstreams/service"

	"github.com/c360studio/semdragons/processor/redteam"
)

type redTeamProfileContract struct {
	Components map[string]struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Config  struct {
			Org                     string `json:"org"`
			Platform                string `json:"platform"`
			Board                   string `json:"board"`
			MinDifficulty           int    `json:"min_difficulty"`
			ClaimTimeoutSeconds     int    `json:"claim_timeout_seconds"`
			ExecutionTimeoutSeconds int    `json:"execution_timeout_seconds"`
			PreferCrossGuild        bool   `json:"prefer_cross_guild"`
		} `json:"config"`
	} `json:"components"`
}

func TestPhase1RedTeamDisabledAndCompleteInEveryShippedProfile(t *testing.T) {
	for _, path := range shippedRuntimeProfiles {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			var profile redTeamProfileContract
			if err := json.Unmarshal(data, &profile); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
			redTeamConfig, exists := profile.Components[redteam.ComponentName]
			if !exists {
				t.Fatal("components.redteam must remain registered but disabled for Phase 1")
			}
			if redTeamConfig.Type != "processor" {
				t.Errorf("components.redteam.type = %q, want processor", redTeamConfig.Type)
			}
			if redTeamConfig.Name != redteam.ComponentName {
				t.Errorf("components.redteam.name = %q, want %q", redTeamConfig.Name, redteam.ComponentName)
			}
			if redTeamConfig.Enabled {
				t.Error("components.redteam.enabled = true, want false")
			}
			cfg := redTeamConfig.Config
			if cfg.Org == "" || cfg.Platform == "" || cfg.Board == "" {
				t.Errorf("components.redteam identity config is incomplete: %+v", cfg)
			}
			if cfg.MinDifficulty < 0 {
				t.Errorf("components.redteam.min_difficulty = %d, want non-negative", cfg.MinDifficulty)
			}
			if cfg.ClaimTimeoutSeconds <= 0 {
				t.Errorf("components.redteam.claim_timeout_seconds = %d, want positive", cfg.ClaimTimeoutSeconds)
			}
			if cfg.ExecutionTimeoutSeconds <= cfg.ClaimTimeoutSeconds {
				t.Errorf(
					"components.redteam.execution_timeout_seconds = %d, want greater than claim timeout %d",
					cfg.ExecutionTimeoutSeconds,
					cfg.ClaimTimeoutSeconds,
				)
			}
			if !cfg.PreferCrossGuild {
				t.Error("components.redteam.prefer_cross_guild = false, want preserved true policy")
			}

			var runtimeProfile runtimeProfileContract
			if err := json.Unmarshal(data, &runtimeProfile); err != nil {
				t.Fatalf("decode %s runtime contract: %v", path, err)
			}
			for _, violation := range runtimeProfileViolations(runtimeProfile) {
				t.Error(violation)
			}
		})
	}
}

func TestPhase1ComponentManagerDoesNotCreateDisabledRedTeam(t *testing.T) {
	for _, path := range shippedRuntimeProfiles {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			var profile semconfig.Config
			if err := json.Unmarshal(data, &profile); err != nil {
				t.Fatalf("decode %s as semstreams config: %v", path, err)
			}
			redTeamConfig, exists := profile.Components[redteam.ComponentName]
			if !exists {
				t.Fatal("components.redteam is missing")
			}

			registry := component.NewRegistry()
			if err := redteam.Register(registry); err != nil {
				t.Fatalf("register dormant redteam factory: %v", err)
			}
			managerService, err := semservice.NewComponentManager(
				nil,
				&semservice.Dependencies{
					ComponentRegistry: registry,
					NATSClient:        &natsclient.Client{},
				},
			)
			if err != nil {
				t.Fatalf("construct component manager: %v", err)
			}
			manager, ok := managerService.(*semservice.ComponentManager)
			if !ok {
				t.Fatalf("component manager type = %T", managerService)
			}
			filtered := &semconfig.Config{
				Components: semconfig.ComponentConfigs{
					redteam.ComponentName: redTeamConfig,
				},
			}
			if err := manager.CreateComponentsFromConfig(context.Background(), filtered); err != nil {
				t.Fatalf("create components from disabled config: %v", err)
			}
			if components := manager.ListComponents(); len(components) != 0 {
				t.Errorf("disabled profile created %d component instances, want 0", len(components))
			}
			if instance := registry.Component(redteam.ComponentName); instance != nil {
				t.Errorf("disabled profile registered live redteam instance %T", instance)
			}
		})
	}
}

func TestPhase1RedTeamProductPolicyRemainsDormant(t *testing.T) {
	assertFileContains(t, "processor/redteam/config.go",
		"type Config struct",
		"func DefaultConfig() Config",
		"func (c *Config) Validate() error",
	)
	assertFileContains(t, "processor/redteam/handler.go",
		"func (c *Component) handleNormalQuestInReview(",
		"domain.QuestTypeRedTeam",
		"func (c *Component) handleRedTeamQuestFinished(",
	)
	assertFileContains(t, "processor/redteam/lessons.go",
		"func (c *Component) extractAndStoreLessons(",
		"func normalizeFindings(",
		"domain.LessonQuality",
	)
	assertFileContains(t, "domain/types.go",
		`QuestTypeRedTeam QuestType = "red_team_review"`,
	)
	assertFileContains(t, "domain/vocab.go",
		`PredicateRedTeamPosted = "redteam.lifecycle.posted"`,
		`PredicateRedTeamCompleted = "redteam.lifecycle.completed"`,
		`PredicateRedTeamSkipped = "redteam.lifecycle.skipped"`,
	)
}
