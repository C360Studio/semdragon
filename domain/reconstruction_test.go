package domain

import (
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

func TestQuestRoundTrip_ReviewConstraints(t *testing.T) {
	tests := []struct {
		name          string
		requireReview bool
		reviewLevel   ReviewLevel
	}{
		{
			name:          "review required standard",
			requireReview: true,
			reviewLevel:   ReviewStandard,
		},
		{
			name:          "review not required",
			requireReview: false,
			reviewLevel:   ReviewAuto,
		},
		{
			name:          "review required strict",
			requireReview: true,
			reviewLevel:   ReviewStrict,
		},
		{
			name:          "human review",
			requireReview: true,
			reviewLevel:   ReviewHuman,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := &Quest{
				ID:          QuestID("test.dev.game.board1.quest.q1"),
				Title:       "Round Trip Test",
				Description: "Testing reconstruction",
				Status:      QuestPosted,
				Difficulty:  DifficultyModerate,
				BaseXP:      200,
				MaxAttempts: 3,
				PostedAt:    time.Now().Truncate(time.Second),
				Constraints: QuestConstraints{
					RequireReview: tt.requireReview,
					ReviewLevel:   tt.reviewLevel,
				},
			}

			// Serialize to triples and reconstruct
			entity := &graph.EntityState{
				ID:      string(original.ID),
				Triples: original.Triples(),
			}

			reconstructed := QuestFromEntityState(entity)

			if reconstructed.Constraints.RequireReview != tt.requireReview {
				t.Errorf("RequireReview = %v, want %v", reconstructed.Constraints.RequireReview, tt.requireReview)
			}
			if reconstructed.Constraints.ReviewLevel != tt.reviewLevel {
				t.Errorf("ReviewLevel = %v, want %v", reconstructed.Constraints.ReviewLevel, tt.reviewLevel)
			}
		})
	}
}

func TestQuestRoundTrip_FullFields(t *testing.T) {
	claimedAt := time.Now().Truncate(time.Second)
	agentID := AgentID("test.dev.game.board1.agent.a1")
	guildID := GuildID("test.dev.game.board1.guild.g1")

	original := &Quest{
		ID:             QuestID("test.dev.game.board1.quest.q1"),
		Title:          "Full Round Trip",
		Description:    "All fields set",
		Status:         QuestClaimed,
		Difficulty:     DifficultyHard,
		BaseXP:         500,
		MaxAttempts:    5,
		Attempts:       1,
		PostedAt:       time.Now().Truncate(time.Second),
		ClaimedAt:      &claimedAt,
		ClaimedBy:      &agentID,
		GuildPriority:  &guildID,
		RequiredSkills: []SkillTag{SkillCodeGen, SkillAnalysis},
		RequiredTools:  []string{"tool-a", "tool-b"},
		MinTier:        TierJourneyman,
		PartyRequired:  true,
		LoopID: "quest-test-loop-abc",
		Constraints: QuestConstraints{
			RequireReview: true,
			ReviewLevel:   ReviewStrict,
		},
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if r.Title != original.Title {
		t.Errorf("Title = %q, want %q", r.Title, original.Title)
	}
	if r.Status != original.Status {
		t.Errorf("Status = %q, want %q", r.Status, original.Status)
	}
	if r.Difficulty != original.Difficulty {
		t.Errorf("Difficulty = %v, want %v", r.Difficulty, original.Difficulty)
	}
	if r.BaseXP != original.BaseXP {
		t.Errorf("BaseXP = %v, want %v", r.BaseXP, original.BaseXP)
	}
	if r.Attempts != original.Attempts {
		t.Errorf("Attempts = %v, want %v", r.Attempts, original.Attempts)
	}
	if r.MaxAttempts != original.MaxAttempts {
		t.Errorf("MaxAttempts = %v, want %v", r.MaxAttempts, original.MaxAttempts)
	}
	if r.ClaimedBy == nil || *r.ClaimedBy != agentID {
		t.Errorf("ClaimedBy = %v, want %v", r.ClaimedBy, &agentID)
	}
	if r.GuildPriority == nil || *r.GuildPriority != guildID {
		t.Errorf("GuildPriority = %v, want %v", r.GuildPriority, &guildID)
	}
	if r.MinTier != original.MinTier {
		t.Errorf("MinTier = %v, want %v", r.MinTier, original.MinTier)
	}
	if r.PartyRequired != original.PartyRequired {
		t.Errorf("PartyRequired = %v, want %v", r.PartyRequired, original.PartyRequired)
	}
	if r.LoopID != original.LoopID {
		t.Errorf("LoopID = %q, want %q", r.LoopID, original.LoopID)
	}
	if r.Constraints.RequireReview != original.Constraints.RequireReview {
		t.Errorf("RequireReview = %v, want %v", r.Constraints.RequireReview, original.Constraints.RequireReview)
	}
	if r.Constraints.ReviewLevel != original.Constraints.ReviewLevel {
		t.Errorf("ReviewLevel = %v, want %v", r.Constraints.ReviewLevel, original.Constraints.ReviewLevel)
	}
	if len(r.RequiredSkills) != len(original.RequiredSkills) {
		t.Errorf("RequiredSkills len = %d, want %d", len(r.RequiredSkills), len(original.RequiredSkills))
	}
	if len(r.RequiredTools) != len(original.RequiredTools) {
		t.Errorf("RequiredTools len = %d, want %d", len(r.RequiredTools), len(original.RequiredTools))
	}
}

func TestQuestFromEntityState_NilReturnsNil(t *testing.T) {
	if got := QuestFromEntityState(nil); got != nil {
		t.Errorf("QuestFromEntityState(nil) = %v, want nil", got)
	}
}

func TestQuestRoundTrip_DependsOnAndAcceptance(t *testing.T) {
	dep1 := QuestID("test.dev.game.board1.quest.dep1")
	dep2 := QuestID("test.dev.game.board1.quest.dep2")

	original := &Quest{
		ID:          QuestID("test.dev.game.board1.quest.q1"),
		Title:       "Depends On Test",
		Description: "Testing DependsOn and Acceptance round-trip",
		Status:      QuestPosted,
		Difficulty:  DifficultyModerate,
		BaseXP:      200,
		MaxAttempts: 3,
		PostedAt:    time.Now().Truncate(time.Second),
		DependsOn:   []QuestID{dep1, dep2},
		Acceptance: []string{
			"All unit tests pass",
			"Code review approved",
			"Documentation updated",
		},
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if len(r.DependsOn) != 2 {
		t.Fatalf("DependsOn len = %d, want 2", len(r.DependsOn))
	}
	if r.DependsOn[0] != dep1 {
		t.Errorf("DependsOn[0] = %q, want %q", r.DependsOn[0], dep1)
	}
	if r.DependsOn[1] != dep2 {
		t.Errorf("DependsOn[1] = %q, want %q", r.DependsOn[1], dep2)
	}

	if len(r.Acceptance) != 3 {
		t.Fatalf("Acceptance len = %d, want 3", len(r.Acceptance))
	}
	if r.Acceptance[0] != "All unit tests pass" {
		t.Errorf("Acceptance[0] = %q, want %q", r.Acceptance[0], "All unit tests pass")
	}
	if r.Acceptance[1] != "Code review approved" {
		t.Errorf("Acceptance[1] = %q, want %q", r.Acceptance[1], "Code review approved")
	}
	if r.Acceptance[2] != "Documentation updated" {
		t.Errorf("Acceptance[2] = %q, want %q", r.Acceptance[2], "Documentation updated")
	}
}

func TestQuestRoundTrip_DAGFields(t *testing.T) {
	// Parent quest with DAG execution state
	nodeStates := map[string]string{"node-1": "completed", "node-2": "in_progress"}
	nodeQuestIDs := map[string]string{"node-1": "quest.sub1", "node-2": "quest.sub2"}
	nodeAssignees := map[string]string{"node-1": "agent.a1", "node-2": "agent.a2"}
	completedNodes := []string{"node-1"}
	failedNodes := []string(nil)
	nodeRetries := map[string]int{"node-1": 3, "node-2": 2}

	dagDef := map[string]any{
		"nodes": []any{
			map[string]any{"id": "node-1", "objective": "do thing 1"},
			map[string]any{"id": "node-2", "objective": "do thing 2", "depends_on": []any{"node-1"}},
		},
	}

	original := &Quest{
		ID:                QuestID("test.dev.game.board1.quest.parent"),
		Title:             "DAG Parent Quest",
		Status:            QuestInProgress,
		PostedAt:          time.Now().Truncate(time.Second),
		DAGExecutionID:    "dag-exec-abc",
		DAGDefinition:     dagDef,
		DAGNodeQuestIDs:   nodeQuestIDs,
		DAGNodeStates:     nodeStates,
		DAGNodeAssignees:  nodeAssignees,
		DAGCompletedNodes: completedNodes,
		DAGFailedNodes:    failedNodes,
		DAGNodeRetries:    nodeRetries,
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if r.DAGExecutionID != "dag-exec-abc" {
		t.Errorf("DAGExecutionID = %q, want %q", r.DAGExecutionID, "dag-exec-abc")
	}
	if r.DAGDefinition == nil {
		t.Fatal("DAGDefinition is nil, want non-nil")
	}
	if r.DAGNodeStates == nil {
		t.Fatal("DAGNodeStates is nil, want non-nil")
	}
	if r.DAGNodeQuestIDs == nil {
		t.Fatal("DAGNodeQuestIDs is nil, want non-nil")
	}
	if r.DAGNodeAssignees == nil {
		t.Fatal("DAGNodeAssignees is nil, want non-nil")
	}
	if r.DAGCompletedNodes == nil {
		t.Fatal("DAGCompletedNodes is nil, want non-nil")
	}
	if r.DAGNodeRetries == nil {
		t.Fatal("DAGNodeRetries is nil, want non-nil")
	}
}

func TestQuestRoundTrip_DAGSubQuestFields(t *testing.T) {
	clarifications := []map[string]any{
		{"question": "What format?", "answer": "JSON", "asked_at": "2024-01-01T00:00:00Z"},
	}

	original := &Quest{
		ID:                QuestID("test.dev.game.board1.quest.sub1"),
		Title:             "Sub Quest",
		Status:            QuestInProgress,
		PostedAt:          time.Now().Truncate(time.Second),
		DAGNodeID:         "node-1",
		DAGClarifications: clarifications,
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if r.DAGNodeID != "node-1" {
		t.Errorf("DAGNodeID = %q, want %q", r.DAGNodeID, "node-1")
	}
	if r.DAGClarifications == nil {
		t.Fatal("DAGClarifications is nil, want non-nil")
	}
}

func TestQuestRoundTrip_DAGFieldsEmpty(t *testing.T) {
	// Quest without DAG fields should have zero values
	original := &Quest{
		ID:       QuestID("test.dev.game.board1.quest.solo"),
		Title:    "Solo Quest",
		Status:   QuestPosted,
		PostedAt: time.Now().Truncate(time.Second),
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if r.DAGExecutionID != "" {
		t.Errorf("DAGExecutionID = %q, want empty", r.DAGExecutionID)
	}
	if r.DAGDefinition != nil {
		t.Errorf("DAGDefinition = %v, want nil", r.DAGDefinition)
	}
	if r.DAGNodeID != "" {
		t.Errorf("DAGNodeID = %q, want empty", r.DAGNodeID)
	}
}

func TestQuestRoundTrip_QuestSpecFields(t *testing.T) {
	original := &Quest{
		ID:          QuestID("test.dev.game.board1.quest.spec1"),
		Title:       "Quest Spec Round Trip",
		Description: "Testing Goal, Requirements, Scenarios, and DecomposabilityClass",
		Status:      QuestPosted,
		Difficulty:  DifficultyHard,
		BaseXP:      400,
		MaxAttempts: 3,
		PostedAt:    time.Now().Truncate(time.Second),

		Goal: "Build a distributed caching layer",
		Requirements: []string{
			"Must support TTL-based expiration",
			"Must handle 10k req/s",
			"Must be horizontally scalable",
		},
		Scenarios: []QuestScenario{
			{
				Name:        "cache-write",
				Description: "Write path with TTL support",
				Skills:      []string{"codegen", "systems"},
			},
			{
				Name:        "cache-read",
				Description: "Read path with cache-miss fallback",
				Skills:      []string{"codegen"},
				DependsOn:   []string{"cache-write"},
			},
			{
				Name:        "cache-eviction",
				Description: "TTL eviction loop",
				Skills:      []string{"codegen", "systems"},
				DependsOn:   []string{"cache-write"},
			},
		},
		DecomposabilityClass: DecomposableMixed,
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if r.Goal != original.Goal {
		t.Errorf("Goal = %q, want %q", r.Goal, original.Goal)
	}

	if len(r.Requirements) != len(original.Requirements) {
		t.Fatalf("Requirements len = %d, want %d", len(r.Requirements), len(original.Requirements))
	}
	for i, req := range original.Requirements {
		if r.Requirements[i] != req {
			t.Errorf("Requirements[%d] = %q, want %q", i, r.Requirements[i], req)
		}
	}

	if r.DecomposabilityClass != original.DecomposabilityClass {
		t.Errorf("DecomposabilityClass = %q, want %q", r.DecomposabilityClass, original.DecomposabilityClass)
	}

	if len(r.Scenarios) != len(original.Scenarios) {
		t.Fatalf("Scenarios len = %d, want %d", len(r.Scenarios), len(original.Scenarios))
	}

	orig0 := original.Scenarios[0]
	got0 := r.Scenarios[0]
	if got0.Name != orig0.Name {
		t.Errorf("Scenarios[0].Name = %q, want %q", got0.Name, orig0.Name)
	}
	if got0.Description != orig0.Description {
		t.Errorf("Scenarios[0].Description = %q, want %q", got0.Description, orig0.Description)
	}
	if len(got0.Skills) != len(orig0.Skills) {
		t.Fatalf("Scenarios[0].Skills len = %d, want %d", len(got0.Skills), len(orig0.Skills))
	}
	for i, sk := range orig0.Skills {
		if got0.Skills[i] != sk {
			t.Errorf("Scenarios[0].Skills[%d] = %q, want %q", i, got0.Skills[i], sk)
		}
	}
	if len(got0.DependsOn) != 0 {
		t.Errorf("Scenarios[0].DependsOn len = %d, want 0", len(got0.DependsOn))
	}

	orig1 := original.Scenarios[1]
	got1 := r.Scenarios[1]
	if got1.Name != orig1.Name {
		t.Errorf("Scenarios[1].Name = %q, want %q", got1.Name, orig1.Name)
	}
	if len(got1.DependsOn) != 1 || got1.DependsOn[0] != "cache-write" {
		t.Errorf("Scenarios[1].DependsOn = %v, want [cache-write]", got1.DependsOn)
	}

	orig2 := original.Scenarios[2]
	got2 := r.Scenarios[2]
	if got2.Name != orig2.Name {
		t.Errorf("Scenarios[2].Name = %q, want %q", got2.Name, orig2.Name)
	}
	if len(got2.DependsOn) != 1 || got2.DependsOn[0] != "cache-write" {
		t.Errorf("Scenarios[2].DependsOn = %v, want [cache-write]", got2.DependsOn)
	}
}

// TestQuestRoundTrip_ScenariosPostKVRoundTrip tests the asScenariosSlice helper's
// []any-of-map[string]any path, which is what comes back after JSON unmarshalling
// from NATS KV storage.
func TestQuestRoundTrip_ScenariosPostKVRoundTrip(t *testing.T) {
	// Simulate what NATS KV returns after JSON round-trip: []any of map[string]any
	rawScenarios := []any{
		map[string]any{
			"name":        "setup",
			"description": "Provision infrastructure",
			"skills":      []any{"devops", "systems"},
			"depends_on":  []any{},
		},
		map[string]any{
			"name":        "deploy",
			"description": "Deploy application artifacts",
			"skills":      []any{"codegen"},
			"depends_on":  []any{"setup"},
		},
		map[string]any{
			// No "skills" or "depends_on" keys at all — missing fields stay zero
			"name":        "verify",
			"description": "Run smoke tests",
		},
	}

	entity := &graph.EntityState{
		ID: "test.dev.game.board1.quest.kv1",
		Triples: []message.Triple{
			{Predicate: "quest.spec.goal", Object: "Ship the release"},
			{Predicate: "quest.spec.requirements", Object: []any{"Tested", "Documented"}},
			{Predicate: "quest.spec.scenarios", Object: rawScenarios},
			{Predicate: "quest.routing.class", Object: "sequential"},
			{Predicate: "quest.status.state", Object: "posted"},
			{Predicate: "quest.identity.title", Object: "KV Round-Trip Quest"},
			{Predicate: "quest.lifecycle.posted_at", Object: time.Now().Format(time.RFC3339)},
		},
	}

	r := QuestFromEntityState(entity)

	if r.Goal != "Ship the release" {
		t.Errorf("Goal = %q, want %q", r.Goal, "Ship the release")
	}

	if r.DecomposabilityClass != DecomposableSequential {
		t.Errorf("DecomposabilityClass = %q, want %q", r.DecomposabilityClass, DecomposableSequential)
	}

	if len(r.Requirements) != 2 {
		t.Fatalf("Requirements len = %d, want 2", len(r.Requirements))
	}
	if r.Requirements[0] != "Tested" {
		t.Errorf("Requirements[0] = %q, want %q", r.Requirements[0], "Tested")
	}
	if r.Requirements[1] != "Documented" {
		t.Errorf("Requirements[1] = %q, want %q", r.Requirements[1], "Documented")
	}

	if len(r.Scenarios) != 3 {
		t.Fatalf("Scenarios len = %d, want 3", len(r.Scenarios))
	}

	// Scenario 0: setup — has skills, empty depends_on slice
	s0 := r.Scenarios[0]
	if s0.Name != "setup" {
		t.Errorf("Scenarios[0].Name = %q, want %q", s0.Name, "setup")
	}
	if s0.Description != "Provision infrastructure" {
		t.Errorf("Scenarios[0].Description = %q, want %q", s0.Description, "Provision infrastructure")
	}
	if len(s0.Skills) != 2 {
		t.Fatalf("Scenarios[0].Skills len = %d, want 2", len(s0.Skills))
	}
	if s0.Skills[0] != "devops" || s0.Skills[1] != "systems" {
		t.Errorf("Scenarios[0].Skills = %v, want [devops systems]", s0.Skills)
	}
	// empty []any depends_on produces nil or empty after conversion — either is acceptable
	if len(s0.DependsOn) != 0 {
		t.Errorf("Scenarios[0].DependsOn = %v, want empty", s0.DependsOn)
	}

	// Scenario 1: deploy — depends on setup
	s1 := r.Scenarios[1]
	if s1.Name != "deploy" {
		t.Errorf("Scenarios[1].Name = %q, want %q", s1.Name, "deploy")
	}
	if len(s1.DependsOn) != 1 || s1.DependsOn[0] != "setup" {
		t.Errorf("Scenarios[1].DependsOn = %v, want [setup]", s1.DependsOn)
	}

	// Scenario 2: verify — missing skills and depends_on keys entirely
	s2 := r.Scenarios[2]
	if s2.Name != "verify" {
		t.Errorf("Scenarios[2].Name = %q, want %q", s2.Name, "verify")
	}
	if len(s2.Skills) != 0 {
		t.Errorf("Scenarios[2].Skills = %v, want empty", s2.Skills)
	}
	if len(s2.DependsOn) != 0 {
		t.Errorf("Scenarios[2].DependsOn = %v, want empty", s2.DependsOn)
	}
}

// TestQuestRoundTrip_QuestSpecFieldsEmpty verifies that a quest without spec
// fields round-trips cleanly with zero values (no phantom data).
func TestQuestRoundTrip_QuestSpecFieldsEmpty(t *testing.T) {
	original := &Quest{
		ID:          QuestID("test.dev.game.board1.quest.nospec"),
		Title:       "No Spec Quest",
		Status:      QuestPosted,
		BaseXP:      100,
		MaxAttempts: 3,
		PostedAt:    time.Now().Truncate(time.Second),
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if r.Goal != "" {
		t.Errorf("Goal = %q, want empty", r.Goal)
	}
	if len(r.Requirements) != 0 {
		t.Errorf("Requirements = %v, want empty", r.Requirements)
	}
	if r.Scenarios != nil {
		t.Errorf("Scenarios = %v, want nil", r.Scenarios)
	}
	if r.DecomposabilityClass != "" {
		t.Errorf("DecomposabilityClass = %q, want empty", r.DecomposabilityClass)
	}
}

func TestQuestRoundTrip_EmptyDependsOnAndAcceptance(t *testing.T) {
	original := &Quest{
		ID:          QuestID("test.dev.game.board1.quest.q1"),
		Title:       "No Deps Test",
		Status:      QuestPosted,
		Difficulty:  DifficultyEasy,
		BaseXP:      50,
		MaxAttempts: 3,
		PostedAt:    time.Now().Truncate(time.Second),
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if len(r.DependsOn) != 0 {
		t.Errorf("DependsOn len = %d, want 0", len(r.DependsOn))
	}
	if len(r.Acceptance) != 0 {
		t.Errorf("Acceptance len = %d, want 0", len(r.Acceptance))
	}
}

// TestQuestRoundTrip_FailureRecoveryFields verifies that all failure recovery
// fields — including a multi-record FailureHistory with sub-field fidelity —
// survive a full Triples() → QuestFromEntityState() round-trip.
func TestQuestRoundTrip_FailureRecoveryFields(t *testing.T) {
	ts1 := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	ts2 := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	agentID := AgentID("test.dev.game.board1.agent.a1")

	original := &Quest{
		ID:          QuestID("test.dev.game.board1.quest.recovery1"),
		Title:       "Recovery Round Trip",
		Status:      QuestFailed,
		PostedAt:    time.Now().Truncate(time.Second),
		MaxAttempts: 3,
		Attempts:    2,
		FailureHistory: []FailureRecord{
			{
				Attempt:       1,
				FailureType:   FailureTimeout,
				FailureReason: "Exceeded time limit waiting for external service",
				Output:        nil,
				AgentID:       agentID,
				LoopID:        "loop-abc-001",
				TriageVerdict: "",
				Timestamp:     ts1,
			},
			{
				Attempt:       2,
				FailureType:   FailureQuality,
				FailureReason: "Output did not meet acceptance criteria",
				Output:        "partial result",
				AgentID:       agentID,
				LoopID:        "loop-abc-002",
				TriageVerdict: "salvage",
				Timestamp:     ts2,
			},
		},
		RecoveryPath:    RecoverySalvage,
		FailureAnalysis: "Agent produced partial output but timed out on validation step",
		SalvagedOutput:  "Partial implementation of the caching layer",
		AntiPatterns:    []string{"Ignored timeout signals", "Did not validate input"},
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	// Recovery scalars
	if r.RecoveryPath != RecoverySalvage {
		t.Errorf("RecoveryPath = %q, want %q", r.RecoveryPath, RecoverySalvage)
	}
	if r.FailureAnalysis != original.FailureAnalysis {
		t.Errorf("FailureAnalysis = %q, want %q", r.FailureAnalysis, original.FailureAnalysis)
	}
	if r.SalvagedOutput != original.SalvagedOutput {
		t.Errorf("SalvagedOutput = %v, want %v", r.SalvagedOutput, original.SalvagedOutput)
	}

	// AntiPatterns slice
	if len(r.AntiPatterns) != 2 {
		t.Fatalf("AntiPatterns len = %d, want 2", len(r.AntiPatterns))
	}
	if r.AntiPatterns[0] != "Ignored timeout signals" {
		t.Errorf("AntiPatterns[0] = %q, want %q", r.AntiPatterns[0], "Ignored timeout signals")
	}
	if r.AntiPatterns[1] != "Did not validate input" {
		t.Errorf("AntiPatterns[1] = %q, want %q", r.AntiPatterns[1], "Did not validate input")
	}

	// FailureHistory length
	if len(r.FailureHistory) != 2 {
		t.Fatalf("FailureHistory len = %d, want 2", len(r.FailureHistory))
	}

	// Record 0: timeout, no triage verdict
	rec0 := r.FailureHistory[0]
	if rec0.Attempt != 1 {
		t.Errorf("FailureHistory[0].Attempt = %d, want 1", rec0.Attempt)
	}
	if rec0.FailureType != FailureTimeout {
		t.Errorf("FailureHistory[0].FailureType = %q, want %q", rec0.FailureType, FailureTimeout)
	}
	if rec0.FailureReason != "Exceeded time limit waiting for external service" {
		t.Errorf("FailureHistory[0].FailureReason = %q, want %q", rec0.FailureReason, "Exceeded time limit waiting for external service")
	}
	if rec0.AgentID != agentID {
		t.Errorf("FailureHistory[0].AgentID = %q, want %q", rec0.AgentID, agentID)
	}
	if rec0.LoopID != "loop-abc-001" {
		t.Errorf("FailureHistory[0].LoopID = %q, want %q", rec0.LoopID, "loop-abc-001")
	}
	if rec0.TriageVerdict != "" {
		t.Errorf("FailureHistory[0].TriageVerdict = %q, want empty", rec0.TriageVerdict)
	}
	if !rec0.Timestamp.Equal(ts1) {
		t.Errorf("FailureHistory[0].Timestamp = %v, want %v", rec0.Timestamp, ts1)
	}

	// Record 1: quality failure, with triage verdict
	rec1 := r.FailureHistory[1]
	if rec1.Attempt != 2 {
		t.Errorf("FailureHistory[1].Attempt = %d, want 2", rec1.Attempt)
	}
	if rec1.FailureType != FailureQuality {
		t.Errorf("FailureHistory[1].FailureType = %q, want %q", rec1.FailureType, FailureQuality)
	}
	if rec1.FailureReason != "Output did not meet acceptance criteria" {
		t.Errorf("FailureHistory[1].FailureReason = %q, want %q", rec1.FailureReason, "Output did not meet acceptance criteria")
	}
	if rec1.LoopID != "loop-abc-002" {
		t.Errorf("FailureHistory[1].LoopID = %q, want %q", rec1.LoopID, "loop-abc-002")
	}
	if rec1.TriageVerdict != "salvage" {
		t.Errorf("FailureHistory[1].TriageVerdict = %q, want %q", rec1.TriageVerdict, "salvage")
	}
	if !rec1.Timestamp.Equal(ts2) {
		t.Errorf("FailureHistory[1].Timestamp = %v, want %v", rec1.Timestamp, ts2)
	}
}

// TestQuestRoundTrip_FailureRecoveryEmpty verifies that a quest without
// recovery fields reconstructs with zero values — no phantom data.
func TestQuestRoundTrip_FailureRecoveryEmpty(t *testing.T) {
	original := &Quest{
		ID:          QuestID("test.dev.game.board1.quest.norecovery"),
		Title:       "No Recovery Quest",
		Status:      QuestPosted,
		BaseXP:      100,
		MaxAttempts: 3,
		PostedAt:    time.Now().Truncate(time.Second),
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if len(r.FailureHistory) != 0 {
		t.Errorf("FailureHistory = %v, want empty", r.FailureHistory)
	}
	if r.RecoveryPath != "" {
		t.Errorf("RecoveryPath = %q, want empty", r.RecoveryPath)
	}
	if r.FailureAnalysis != "" {
		t.Errorf("FailureAnalysis = %q, want empty", r.FailureAnalysis)
	}
	if r.SalvagedOutput != nil {
		t.Errorf("SalvagedOutput = %v, want nil", r.SalvagedOutput)
	}
	if len(r.AntiPatterns) != 0 {
		t.Errorf("AntiPatterns = %v, want empty", r.AntiPatterns)
	}
}

// TestQuestRoundTrip_ArtifactFields verifies that all artifact tracking fields
// survive a full Triples() → QuestFromEntityState() round-trip with complete
// fidelity, including multiple ProducedEntities entries.
func TestQuestRoundTrip_ArtifactFields(t *testing.T) {
	t.Run("all artifact fields populated", func(t *testing.T) {
		original := &Quest{
			ID:               QuestID("test.dev.game.board1.quest.art1"),
			Title:            "Artifact Round Trip",
			Status:           QuestCompleted,
			PostedAt:         time.Now().Truncate(time.Second),
			ArtifactsMerged:  "fed987cba654321",
			ArtifactsIndexed: true,
			ProducedEntities: []string{
				"c360.prod.src.repo1.file.main.go",
				"c360.prod.src.repo1.func.handleQuest",
			},
		}

		entity := &graph.EntityState{
			ID:      string(original.ID),
			Triples: original.Triples(),
		}

		r := QuestFromEntityState(entity)

		if r.ArtifactsMerged != original.ArtifactsMerged {
			t.Errorf("ArtifactsMerged = %q, want %q", r.ArtifactsMerged, original.ArtifactsMerged)
		}
		if !r.ArtifactsIndexed {
			t.Errorf("ArtifactsIndexed = false, want true")
		}
		if len(r.ProducedEntities) != 2 {
			t.Fatalf("ProducedEntities len = %d, want 2", len(r.ProducedEntities))
		}
		// ProducedEntities are appended in triple order; order is deterministic
		// because graphable.go iterates the slice directly.
		if r.ProducedEntities[0] != original.ProducedEntities[0] {
			t.Errorf("ProducedEntities[0] = %q, want %q", r.ProducedEntities[0], original.ProducedEntities[0])
		}
		if r.ProducedEntities[1] != original.ProducedEntities[1] {
			t.Errorf("ProducedEntities[1] = %q, want %q", r.ProducedEntities[1], original.ProducedEntities[1])
		}
	})

	t.Run("produced entities with multiple IDs all preserved", func(t *testing.T) {
		ids := []string{
			"c360.prod.src.repo1.file.alpha.go",
			"c360.prod.src.repo1.file.beta.go",
			"c360.prod.src.repo1.file.gamma.go",
		}

		original := &Quest{
			ID:               QuestID("test.dev.game.board1.quest.art2"),
			Title:            "Multi Produced Quest",
			Status:           QuestCompleted,
			PostedAt:         time.Now().Truncate(time.Second),
			ArtifactsMerged:  "fed987cba654321",
			ProducedEntities: ids,
		}

		entity := &graph.EntityState{
			ID:      string(original.ID),
			Triples: original.Triples(),
		}

		r := QuestFromEntityState(entity)

		if len(r.ProducedEntities) != len(ids) {
			t.Fatalf("ProducedEntities len = %d, want %d", len(r.ProducedEntities), len(ids))
		}
		for i, want := range ids {
			if r.ProducedEntities[i] != want {
				t.Errorf("ProducedEntities[%d] = %q, want %q", i, r.ProducedEntities[i], want)
			}
		}
	})

	t.Run("empty artifact fields produce no phantom data", func(t *testing.T) {
		original := &Quest{
			ID:          QuestID("test.dev.game.board1.quest.art3"),
			Title:       "No Artifact Quest",
			Status:      QuestPosted,
			PostedAt:    time.Now().Truncate(time.Second),
			MaxAttempts: 3,
		}

		entity := &graph.EntityState{
			ID:      string(original.ID),
			Triples: original.Triples(),
		}

		r := QuestFromEntityState(entity)

		if r.ArtifactsMerged != "" {
			t.Errorf("ArtifactsMerged = %q, want empty", r.ArtifactsMerged)
		}
		if r.ArtifactsIndexed {
			t.Errorf("ArtifactsIndexed = true, want false")
		}
		if len(r.ProducedEntities) != 0 {
			t.Errorf("ProducedEntities = %v, want empty", r.ProducedEntities)
		}

		// Verify none of the artifact predicates appear in the emitted triples.
		artifactPredicates := map[string]bool{
			PredicateQuestArtifactsMerged:  true,
			PredicateQuestArtifactsIndexed: true,
			PredicateQuestProduced:         true,
		}
		for _, triple := range entity.Triples {
			if artifactPredicates[triple.Predicate] {
				t.Errorf("unexpected artifact triple emitted: predicate %q", triple.Predicate)
			}
		}
	})
}

// TestQuestRoundTrip_RepoField verifies that the Repo field survives a full
// Triples() → QuestFromEntityState() round-trip.
func TestQuestRoundTrip_RepoField(t *testing.T) {
	t.Run("repo set", func(t *testing.T) {
		original := &Quest{
			ID:       QuestID("test.dev.game.board1.quest.repo1"),
			Title:    "Repo Round Trip",
			Status:   QuestPosted,
			PostedAt: time.Now().Truncate(time.Second),
			Repo:     "my-project-repo",
		}

		entity := &graph.EntityState{
			ID:      string(original.ID),
			Triples: original.Triples(),
		}

		r := QuestFromEntityState(entity)

		if r.Repo != original.Repo {
			t.Errorf("Repo = %q, want %q", r.Repo, original.Repo)
		}
	})

	t.Run("repo empty — no phantom data", func(t *testing.T) {
		original := &Quest{
			ID:       QuestID("test.dev.game.board1.quest.repo2"),
			Title:    "No Repo Quest",
			Status:   QuestPosted,
			PostedAt: time.Now().Truncate(time.Second),
		}

		entity := &graph.EntityState{
			ID:      string(original.ID),
			Triples: original.Triples(),
		}

		r := QuestFromEntityState(entity)

		if r.Repo != "" {
			t.Errorf("Repo = %q, want empty", r.Repo)
		}

		for _, triple := range entity.Triples {
			if triple.Predicate == PredicateQuestRepo {
				t.Errorf("unexpected repo triple emitted: predicate %q", triple.Predicate)
			}
		}
	})
}

// TestQuestRoundTrip_FailureHistoryPostKVRoundTrip simulates what NATS KV
// returns after a JSON round-trip: the failure history arrives as []any of
// map[string]any with string values for numeric and time fields. This
// exercises the asFailureRecordSlice conversion path directly.
func TestQuestRoundTrip_FailureHistoryPostKVRoundTrip(t *testing.T) {
	ts := time.Now().Truncate(time.Second)
	tsStr := ts.UTC().Format(time.RFC3339)

	// Simulate raw KV data: attempt is a float64 (JSON number), timestamps
	// are RFC3339 strings, all other fields are plain strings.
	rawHistory := []any{
		map[string]any{
			"attempt":        float64(1),
			"failure_type":   "timeout",
			"failure_reason": "Deadline exceeded",
			"agent_id":       "test.dev.game.board1.agent.a99",
			"loop_id":        "loop-kv-001",
			"triage_verdict": "",
			"timestamp":      tsStr,
		},
		map[string]any{
			"attempt":        float64(2),
			"failure_type":   "quality",
			"failure_reason": "Score below threshold",
			"output":         "draft output",
			"agent_id":       "test.dev.game.board1.agent.a99",
			"loop_id":        "loop-kv-002",
			"triage_verdict": "tpk",
			"timestamp":      tsStr,
		},
	}

	entity := &graph.EntityState{
		ID: "test.dev.game.board1.quest.kvrecovery",
		Triples: []message.Triple{
			{Predicate: "quest.identity.title", Object: "KV Recovery Quest"},
			{Predicate: "quest.status.state", Object: "failed"},
			{Predicate: "quest.lifecycle.posted_at", Object: ts.UTC().Format(time.RFC3339)},
			{Predicate: "quest.failure.history", Object: rawHistory},
			{Predicate: "quest.recovery.path", Object: "tpk"},
			{Predicate: "quest.recovery.analysis", Object: "Both attempts failed quality gate"},
			{Predicate: "quest.recovery.antipatterns", Object: []any{"Skipped validation", "No error handling"}},
		},
	}

	r := QuestFromEntityState(entity)

	// Scalars
	if r.RecoveryPath != RecoveryTPK {
		t.Errorf("RecoveryPath = %q, want %q", r.RecoveryPath, RecoveryTPK)
	}
	if r.FailureAnalysis != "Both attempts failed quality gate" {
		t.Errorf("FailureAnalysis = %q, want %q", r.FailureAnalysis, "Both attempts failed quality gate")
	}

	// AntiPatterns via AsStringSlice from []any
	if len(r.AntiPatterns) != 2 {
		t.Fatalf("AntiPatterns len = %d, want 2", len(r.AntiPatterns))
	}
	if r.AntiPatterns[0] != "Skipped validation" {
		t.Errorf("AntiPatterns[0] = %q, want %q", r.AntiPatterns[0], "Skipped validation")
	}
	if r.AntiPatterns[1] != "No error handling" {
		t.Errorf("AntiPatterns[1] = %q, want %q", r.AntiPatterns[1], "No error handling")
	}

	// FailureHistory converted from []any of map[string]any
	if len(r.FailureHistory) != 2 {
		t.Fatalf("FailureHistory len = %d, want 2", len(r.FailureHistory))
	}

	rec0 := r.FailureHistory[0]
	if rec0.Attempt != 1 {
		t.Errorf("FailureHistory[0].Attempt = %d, want 1", rec0.Attempt)
	}
	if rec0.FailureType != FailureTimeout {
		t.Errorf("FailureHistory[0].FailureType = %q, want %q", rec0.FailureType, FailureTimeout)
	}
	if rec0.FailureReason != "Deadline exceeded" {
		t.Errorf("FailureHistory[0].FailureReason = %q, want %q", rec0.FailureReason, "Deadline exceeded")
	}
	if rec0.AgentID != AgentID("test.dev.game.board1.agent.a99") {
		t.Errorf("FailureHistory[0].AgentID = %q, want %q", rec0.AgentID, "test.dev.game.board1.agent.a99")
	}
	if rec0.LoopID != "loop-kv-001" {
		t.Errorf("FailureHistory[0].LoopID = %q, want %q", rec0.LoopID, "loop-kv-001")
	}
	if rec0.TriageVerdict != "" {
		t.Errorf("FailureHistory[0].TriageVerdict = %q, want empty", rec0.TriageVerdict)
	}
	if !rec0.Timestamp.Equal(ts) {
		t.Errorf("FailureHistory[0].Timestamp = %v, want %v", rec0.Timestamp, ts)
	}

	rec1 := r.FailureHistory[1]
	if rec1.Attempt != 2 {
		t.Errorf("FailureHistory[1].Attempt = %d, want 2", rec1.Attempt)
	}
	if rec1.FailureType != FailureQuality {
		t.Errorf("FailureHistory[1].FailureType = %q, want %q", rec1.FailureType, FailureQuality)
	}
	if rec1.TriageVerdict != "tpk" {
		t.Errorf("FailureHistory[1].TriageVerdict = %q, want %q", rec1.TriageVerdict, "tpk")
	}
	if rec1.Output != "draft output" {
		t.Errorf("FailureHistory[1].Output = %v, want %q", rec1.Output, "draft output")
	}
	if rec1.LoopID != "loop-kv-002" {
		t.Errorf("FailureHistory[1].LoopID = %q, want %q", rec1.LoopID, "loop-kv-002")
	}
}
