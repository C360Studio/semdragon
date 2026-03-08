//go:build integration

package agentsheet_test

import (
	"context"
	"testing"
	"time"

	semgraph "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/service/agentsheet"
)

func setupNameGenTestBoard(t *testing.T) (*semdragons.GraphClient, func()) {
	t.Helper()
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithKVBuckets(semgraph.BucketEntityStates),
	)
	config := domain.BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "namegen",
	}

	gc := semdragons.NewGraphClient(tc.Client, &config)

	// Ensure board-specific KV bucket exists (mirrors main.go startup)
	if err := gc.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	return gc, func() {
		tc.Client.Close(context.Background())
	}
}

func TestNameGenerator_GenerateName(t *testing.T) {
	ctx := context.Background()

	graph, cleanup := setupNameGenTestBoard(t)
	defer cleanup()

	gen := agentsheet.NewNameGenerator(graph)

	agent := &agentprogression.Agent{
		ID:   "test-agent",
		Name: "test-agent",
		Tier: domain.TierApprentice,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen: {Level: domain.ProficiencyNovice},
		},
	}

	name, err := gen.GenerateName(ctx, agent)
	if err != nil {
		t.Fatalf("GenerateName failed: %v", err)
	}

	if name == "" {
		t.Error("expected non-empty name")
	}

	t.Logf("Generated name: %s", name)
}

func TestNameGenerator_SuggestNames(t *testing.T) {
	ctx := context.Background()

	graph, cleanup := setupNameGenTestBoard(t)
	defer cleanup()

	gen := agentsheet.NewNameGenerator(graph)

	agent := &agentprogression.Agent{
		ID:   "test-agent",
		Name: "test-agent",
		Tier: domain.TierExpert,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillAnalysis: {Level: domain.ProficiencyJourneyman},
			domain.SkillResearch: {Level: domain.ProficiencyNovice},
		},
	}

	suggestions, err := gen.SuggestNames(ctx, agent, 5)
	if err != nil {
		t.Fatalf("SuggestNames failed: %v", err)
	}

	if len(suggestions) < 3 {
		t.Errorf("expected at least 3 suggestions, got %d", len(suggestions))
	}

	// Verify uniqueness within suggestions
	seen := make(map[string]bool)
	for _, name := range suggestions {
		if seen[name] {
			t.Errorf("duplicate suggestion: %s", name)
		}
		seen[name] = true
		t.Logf("Suggested name: %s", name)
	}
}

func TestNameGenerator_SetDisplayName(t *testing.T) {
	ctx := context.Background()

	graph, cleanup := setupNameGenTestBoard(t)
	defer cleanup()

	gen := agentsheet.NewNameGenerator(graph)

	t.Run("valid name", func(t *testing.T) {
		agent := &agentprogression.Agent{ID: "agent-1", Name: "agent-1"}

		err := gen.SetDisplayName(ctx, agent, "Shadowblade")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if agent.DisplayName != "Shadowblade" {
			t.Errorf("expected DisplayName 'Shadowblade', got '%s'", agent.DisplayName)
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		agent := &agentprogression.Agent{ID: "agent-2", Name: "agent-2"}

		err := gen.SetDisplayName(ctx, agent, "")
		if err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("too long name rejected", func(t *testing.T) {
		agent := &agentprogression.Agent{ID: "agent-3", Name: "agent-3"}

		longName := "ThisNameIsWayTooLongToBeAcceptableForACharacterName"
		err := gen.SetDisplayName(ctx, agent, longName)
		if err == nil {
			t.Error("expected error for too long name")
		}
	})
}

func TestNameGenerator_Uniqueness(t *testing.T) {
	ctx := context.Background()

	graph, cleanup := setupNameGenTestBoard(t)
	defer cleanup()

	gen := agentsheet.NewNameGenerator(graph)

	// Create first agent with a display name
	instance1 := domain.GenerateInstance()
	agent1 := &agentprogression.Agent{
		ID:          domain.AgentID(graph.Config().AgentEntityID(instance1)),
		Name:        "agent-1",
		DisplayName: "Codewarden",
		Status:      domain.AgentIdle,
		Level:       1,
		Tier:        domain.TierApprentice,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := graph.PutEntityState(ctx, agent1, "agent.identity.created"); err != nil {
		t.Fatalf("failed to save agent1: %v", err)
	}

	// Try to give another agent the same name
	agent2 := &agentprogression.Agent{ID: "agent-2", Name: "agent-2"}

	err := gen.SetDisplayName(ctx, agent2, "Codewarden")
	if err == nil {
		t.Error("expected error for duplicate name")
	}

	// Different case should also be rejected
	err = gen.SetDisplayName(ctx, agent2, "codewarden")
	if err == nil {
		t.Error("expected error for case-insensitive duplicate")
	}

	// Different name should succeed
	err = gen.SetDisplayName(ctx, agent2, "Ironkeeper")
	if err != nil {
		t.Errorf("unexpected error for unique name: %v", err)
	}
}

func TestNameGenerator_SkillBasedPrefixes(t *testing.T) {
	ctx := context.Background()

	graph, cleanup := setupNameGenTestBoard(t)
	defer cleanup()

	gen := agentsheet.NewNameGenerator(graph)

	// Test that different skills produce different name styles
	codeAgent := &agentprogression.Agent{
		ID:   "code-agent",
		Tier: domain.TierJourneyman,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen:    {Level: domain.ProficiencyNovice},
			domain.SkillCodeReview: {Level: domain.ProficiencyNovice},
		},
	}

	researchAgent := &agentprogression.Agent{
		ID:   "research-agent",
		Tier: domain.TierJourneyman,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillResearch: {Level: domain.ProficiencyNovice},
			domain.SkillAnalysis: {Level: domain.ProficiencyNovice},
		},
	}

	// Generate multiple names to see the variety
	for i := 0; i < 5; i++ {
		codeName, _ := gen.GenerateName(ctx, codeAgent)
		researchName, _ := gen.GenerateName(ctx, researchAgent)
		t.Logf("Code agent: %s | Research agent: %s", codeName, researchName)
	}
}
