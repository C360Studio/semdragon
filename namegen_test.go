package semdragons

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

func setupNameGenTestBoard(t *testing.T) (*Storage, func()) {
	t.Helper()
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "namegen",
	}

	ctx := context.Background()
	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	return storage, func() {
		tc.Client.Close(context.Background())
	}
}

func TestNameGenerator_GenerateName(t *testing.T) {
	ctx := context.Background()

	storage, cleanup := setupNameGenTestBoard(t)
	defer cleanup()

	gen := NewNameGenerator(storage)

	agent := &Agent{
		ID:     "test-agent",
		Name:   "test-agent",
		Tier:   TierApprentice,
		Skills: []SkillTag{SkillCodeGen},
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

	storage, cleanup := setupNameGenTestBoard(t)
	defer cleanup()

	gen := NewNameGenerator(storage)

	agent := &Agent{
		ID:   "test-agent",
		Name: "test-agent",
		Tier: TierExpert,
		SkillProficiencies: map[SkillTag]SkillProficiency{
			SkillAnalysis: {Level: ProficiencyJourneyman},
			SkillResearch: {Level: ProficiencyNovice},
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

	storage, cleanup := setupNameGenTestBoard(t)
	defer cleanup()

	gen := NewNameGenerator(storage)

	t.Run("valid name", func(t *testing.T) {
		agent := &Agent{ID: "agent-1", Name: "agent-1"}

		err := gen.SetDisplayName(ctx, agent, "Shadowblade")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if agent.DisplayName != "Shadowblade" {
			t.Errorf("expected DisplayName 'Shadowblade', got '%s'", agent.DisplayName)
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		agent := &Agent{ID: "agent-2", Name: "agent-2"}

		err := gen.SetDisplayName(ctx, agent, "")
		if err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("too long name rejected", func(t *testing.T) {
		agent := &Agent{ID: "agent-3", Name: "agent-3"}

		longName := "ThisNameIsWayTooLongToBeAcceptableForACharacterName"
		err := gen.SetDisplayName(ctx, agent, longName)
		if err == nil {
			t.Error("expected error for too long name")
		}
	})
}

func TestNameGenerator_Uniqueness(t *testing.T) {
	ctx := context.Background()

	storage, cleanup := setupNameGenTestBoard(t)
	defer cleanup()

	gen := NewNameGenerator(storage)

	// Create first agent with a display name
	instance1 := GenerateInstance()
	agent1 := &Agent{
		ID:          AgentID(storage.Config().AgentEntityID(instance1)),
		Name:        "agent-1",
		DisplayName: "Codewarden",
		Status:      AgentIdle,
		Level:       1,
		Tier:        TierApprentice,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := storage.PutAgent(ctx, instance1, agent1); err != nil {
		t.Fatalf("failed to save agent1: %v", err)
	}

	// Try to give another agent the same name
	agent2 := &Agent{ID: "agent-2", Name: "agent-2"}

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

	storage, cleanup := setupNameGenTestBoard(t)
	defer cleanup()

	gen := NewNameGenerator(storage)

	// Test that different skills produce different name styles
	codeAgent := &Agent{
		ID:     "code-agent",
		Tier:   TierJourneyman,
		Skills: []SkillTag{SkillCodeGen, SkillCodeReview},
	}

	researchAgent := &Agent{
		ID:     "research-agent",
		Tier:   TierJourneyman,
		Skills: []SkillTag{SkillResearch, SkillAnalysis},
	}

	// Generate multiple names to see the variety
	for i := 0; i < 5; i++ {
		codeName, _ := gen.GenerateName(ctx, codeAgent)
		researchName, _ := gen.GenerateName(ctx, researchAgent)
		t.Logf("Code agent: %s | Research agent: %s", codeName, researchName)
	}
}
