package semdragons_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/component"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"
	semservice "github.com/c360studio/semstreams/service"

	"github.com/c360studio/semdragons/processor/boidengine"
)

type boidPhase1Profile struct {
	Components map[string]struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Config  struct {
			RulesFiles            []string            `json:"rules_files"`
			EntityWatchBuckets    map[string][]string `json:"entity_watch_buckets"`
			BoidSuggestionsBucket string              `json:"boid_suggestions_bucket"`
		} `json:"config"`
	} `json:"components"`
	Streams map[string]struct {
		Subjects []string `json:"subjects"`
	} `json:"streams"`
}

func TestPhase1BoidEngineDisabledInEveryShippedProfile(t *testing.T) {
	for _, path := range shippedRuntimeProfiles {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			var profile boidPhase1Profile
			if err := json.Unmarshal(data, &profile); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}

			boids, exists := profile.Components[boidengine.ComponentName]
			if !exists {
				t.Fatal("components.boidengine must remain registered but disabled for Phase 1")
			}
			if boids.Type != "processor" {
				t.Errorf("components.boidengine.type = %q, want processor", boids.Type)
			}
			if boids.Name != boidengine.ComponentName {
				t.Errorf("components.boidengine.name = %q, want %q", boids.Name, boidengine.ComponentName)
			}
			if boids.Enabled {
				t.Error("components.boidengine.enabled = true, want false")
			}
			if boids.Config.BoidSuggestionsBucket != "BOID_SUGGESTIONS" {
				t.Errorf(
					"components.boidengine.config.boid_suggestions_bucket = %q, want BOID_SUGGESTIONS",
					boids.Config.BoidSuggestionsBucket,
				)
			}

			rules, exists := profile.Components["rule-processor"]
			if !exists {
				t.Fatal("components.rule-processor is missing")
			}
			if !slices.Equal(rules.Config.RulesFiles, completeQuestRulePack) {
				t.Error("rules_files does not equal the complete ordered quest pack")
			}
			if !slices.Equal(rules.Config.EntityWatchBuckets["ENTITY_STATES"], []string{"*.*.game.*.quest.*"}) {
				t.Error("entity_watch_buckets.ENTITY_STATES does not equal the quest entity pattern")
			}

			for streamName, stream := range profile.Streams {
				if slices.Contains(stream.Subjects, "agent.boid.>") {
					t.Errorf("streams.%s retains stale subject agent.boid.>", streamName)
				}
			}
		})
	}
}

func TestPhase1ComponentManagerDoesNotCreateDisabledBoidEngine(t *testing.T) {
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
			boidConfig, exists := profile.Components[boidengine.ComponentName]
			if !exists {
				t.Fatal("components.boidengine is missing")
			}

			registry := component.NewRegistry()
			if err := boidengine.Register(registry); err != nil {
				t.Fatalf("register dormant boidengine factory: %v", err)
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
					boidengine.ComponentName: boidConfig,
				},
			}
			if err := manager.CreateComponentsFromConfig(context.Background(), filtered); err != nil {
				t.Fatalf("create components from disabled config: %v", err)
			}
			if components := manager.ListComponents(); len(components) != 0 {
				t.Errorf("disabled profile created %d component instances, want 0", len(components))
			}
			if instance := registry.Component(boidengine.ComponentName); instance != nil {
				t.Errorf("disabled profile registered live boidengine instance %T", instance)
			}
		})
	}
}

func TestPhase1BoidPolicyAndLiveSuggestionContractsRemainDormant(t *testing.T) {
	assertFileContains(t, "processor/boidengine/boids.go",
		"type DefaultBoidEngine struct",
		"func NewDefaultBoidEngine() *DefaultBoidEngine",
		"func DefaultBoidRules() BoidRules",
	)
	assertFileContains(t, "processor/boidengine/component.go",
		`Name:        "boid-suggestions"`,
		`Subject: "boid.suggestions.*"`,
		`Name:        "boid-guild-suggestions"`,
		`Subject: "boid.guild.*"`,
		`Bucket:      c.config.BoidSuggestionsBucket`,
	)
	assertFileContains(t, "processor/boidengine/config.go",
		`BoidSuggestionsBucket: "BOID_SUGGESTIONS"`,
	)
	assertFileContains(t, "processor/autonomy/component.go",
		`Subscribe(ctx, "boid.suggestions.>"`,
		`Subscribe(ctx, "boid.guild.>"`,
	)
	assertFileContains(t, "processor/autonomy/actions.go",
		"func (c *Component) claimQuestAction() action",
		"func (c *Component) joinGuildAction() action",
		"func (c *Component) createGuildAction() action",
	)
	assertFileContains(t, "processor/dmpartyformation/component.go",
		`"github.com/c360studio/semdragons/processor/boidengine"`,
		"boidengine.NewDefaultBoidEngine()",
	)

	configData := make([]string, 0, len(shippedRuntimeProfiles))
	for _, path := range shippedRuntimeProfiles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		configData = append(configData, string(data))
	}
	if strings.Contains(strings.Join(configData, "\n"), `"agent.boid.>"`) {
		t.Error("shipped profiles retain stale agent.boid.> stream subject")
	}
}
