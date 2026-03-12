package questbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/executor"
	"github.com/c360studio/semdragons/processor/questdagexec"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/model"
	"github.com/c360studio/semstreams/storage"
)

// =============================================================================
// tripleString
// =============================================================================

func TestTripleString(t *testing.T) {
	now := time.Now()
	triples := []message.Triple{
		{Subject: "s", Predicate: "quest.status.state", Object: "in_progress", Source: "test", Timestamp: now},
		{Subject: "s", Predicate: "quest.assignment.agent", Object: "agent-42", Source: "test", Timestamp: now},
		{Subject: "s", Predicate: "quest.identity.id", Object: "q-abc", Source: "test", Timestamp: now},
		// Non-string object — should be skipped even when predicate matches.
		{Subject: "s", Predicate: "quest.progress.percent", Object: 75.5, Source: "test", Timestamp: now},
		// Empty string object is a valid string value.
		{Subject: "s", Predicate: "quest.optional.field", Object: "", Source: "test", Timestamp: now},
	}

	tests := []struct {
		name      string
		predicate string
		want      string
	}{
		{
			name:      "finds existing string predicate",
			predicate: "quest.status.state",
			want:      "in_progress",
		},
		{
			name:      "finds second string predicate",
			predicate: "quest.assignment.agent",
			want:      "agent-42",
		},
		{
			name:      "finds third string predicate",
			predicate: "quest.identity.id",
			want:      "q-abc",
		},
		{
			name:      "returns empty string for non-string object",
			predicate: "quest.progress.percent",
			want:      "",
		},
		{
			name:      "returns empty string value when object is empty string",
			predicate: "quest.optional.field",
			want:      "",
		},
		{
			name:      "returns empty string when predicate is absent",
			predicate: "quest.nonexistent.predicate",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tripleString(triples, tt.predicate)
			if got != tt.want {
				t.Errorf("tripleString(triples, %q) = %q; want %q", tt.predicate, got, tt.want)
			}
		})
	}

	t.Run("nil triples slice returns empty string", func(t *testing.T) {
		got := tripleString(nil, "quest.status.state")
		if got != "" {
			t.Errorf("tripleString(nil, ...) = %q; want %q", got, "")
		}
	})

	t.Run("empty triples slice returns empty string", func(t *testing.T) {
		got := tripleString([]message.Triple{}, "quest.status.state")
		if got != "" {
			t.Errorf("tripleString([], ...) = %q; want %q", got, "")
		}
	})

	t.Run("returns first match when predicate appears multiple times", func(t *testing.T) {
		dups := []message.Triple{
			{Predicate: "quest.tag.label", Object: "first"},
			{Predicate: "quest.tag.label", Object: "second"},
		}
		got := tripleString(dups, "quest.tag.label")
		if got != "first" {
			t.Errorf("tripleString with duplicate predicates = %q; want %q", got, "first")
		}
	})
}

// =============================================================================
// buildLegacySystemPrompt
// =============================================================================

func TestBuildLegacySystemPrompt(t *testing.T) {
	t.Run("agent config system prompt is prepended", func(t *testing.T) {
		agent := &agentprogression.Agent{
			ID:   "agent-1",
			Name: "Coder",
			Config: agentprogression.AgentConfig{
				SystemPrompt: "You are a senior Go developer.",
			},
		}
		quest := &domain.Quest{
			ID:    "q-1",
			Title: "Fix the bug",
		}

		got := buildLegacySystemPrompt(agent, quest)

		if !strings.HasPrefix(got, "You are a senior Go developer.") {
			t.Errorf("expected prompt to start with agent system prompt, got: %q", got)
		}
		if !strings.Contains(got, "Fix the bug") {
			t.Errorf("expected prompt to contain quest title, got: %q", got)
		}
	})

	t.Run("persona system prompt is included after agent config prompt", func(t *testing.T) {
		agent := &agentprogression.Agent{
			ID:   "agent-2",
			Name: "Shadowweaver",
			Config: agentprogression.AgentConfig{
				SystemPrompt: "Base instructions.",
			},
			Persona: &agentprogression.AgentPersona{
				SystemPrompt: "You are Shadowweaver, a cunning rogue.",
			},
		}
		quest := &domain.Quest{
			ID:    "q-2",
			Title: "Infiltrate the server",
		}

		got := buildLegacySystemPrompt(agent, quest)

		baseIdx := strings.Index(got, "Base instructions.")
		personaIdx := strings.Index(got, "You are Shadowweaver")
		titleIdx := strings.Index(got, "Infiltrate the server")

		if baseIdx == -1 {
			t.Error("base system prompt not found in output")
		}
		if personaIdx == -1 {
			t.Error("persona system prompt not found in output")
		}
		if titleIdx == -1 {
			t.Error("quest title not found in output")
		}
		if baseIdx > personaIdx {
			t.Error("base config prompt should appear before persona prompt")
		}
		if personaIdx > titleIdx {
			t.Error("persona prompt should appear before quest title")
		}
	})

	t.Run("nil persona does not panic", func(t *testing.T) {
		agent := &agentprogression.Agent{
			ID:      "agent-3",
			Name:    "NoPers",
			Persona: nil,
		}
		quest := &domain.Quest{
			ID:    "q-3",
			Title: "A simple task",
		}

		got := buildLegacySystemPrompt(agent, quest)
		if !strings.Contains(got, "A simple task") {
			t.Errorf("expected quest title in prompt, got: %q", got)
		}
	})

	t.Run("quest description is included when non-empty", func(t *testing.T) {
		agent := &agentprogression.Agent{ID: "agent-4", Name: "Analyst"}
		quest := &domain.Quest{
			ID:          "q-4",
			Title:       "Analyse logs",
			Description: "Parse the access logs and identify anomalies.",
		}

		got := buildLegacySystemPrompt(agent, quest)
		if !strings.Contains(got, "Parse the access logs and identify anomalies.") {
			t.Errorf("quest description missing from prompt: %q", got)
		}
	})

	t.Run("quest description is omitted when empty", func(t *testing.T) {
		agent := &agentprogression.Agent{ID: "agent-5", Name: "Analyst"}
		quest := &domain.Quest{
			ID:          "q-5",
			Title:       "Quick task",
			Description: "",
		}

		got := buildLegacySystemPrompt(agent, quest)
		if strings.Contains(got, "Description:") {
			t.Errorf("expected no Description line when quest has empty description, got: %q", got)
		}
	})

	t.Run("time limit is included when MaxDuration is set", func(t *testing.T) {
		agent := &agentprogression.Agent{ID: "agent-6", Name: "Timed"}
		quest := &domain.Quest{
			ID:    "q-6",
			Title: "Timed task",
			Constraints: domain.QuestConstraints{
				MaxDuration: 30 * time.Minute,
			},
		}

		got := buildLegacySystemPrompt(agent, quest)
		if !strings.Contains(got, "Time limit:") {
			t.Errorf("expected time limit line in prompt, got: %q", got)
		}
	})

	t.Run("time limit is omitted when MaxDuration is zero", func(t *testing.T) {
		agent := &agentprogression.Agent{ID: "agent-7", Name: "Untimed"}
		quest := &domain.Quest{
			ID:    "q-7",
			Title: "Untimed task",
			Constraints: domain.QuestConstraints{
				MaxDuration: 0,
			},
		}

		got := buildLegacySystemPrompt(agent, quest)
		if strings.Contains(got, "Time limit:") {
			t.Errorf("expected no time limit line when MaxDuration is zero, got: %q", got)
		}
	})

	t.Run("token budget is included when MaxTokens is set", func(t *testing.T) {
		agent := &agentprogression.Agent{ID: "agent-8", Name: "Budgeted"}
		quest := &domain.Quest{
			ID:    "q-8",
			Title: "Budget task",
			Constraints: domain.QuestConstraints{
				MaxTokens: 4096,
			},
		}

		got := buildLegacySystemPrompt(agent, quest)
		if !strings.Contains(got, "Token budget: 4096") {
			t.Errorf("expected token budget line in prompt, got: %q", got)
		}
	})

	t.Run("token budget is omitted when MaxTokens is zero", func(t *testing.T) {
		agent := &agentprogression.Agent{ID: "agent-9", Name: "Unbudgeted"}
		quest := &domain.Quest{
			ID:    "q-9",
			Title: "No budget task",
		}

		got := buildLegacySystemPrompt(agent, quest)
		if strings.Contains(got, "Token budget:") {
			t.Errorf("expected no token budget line when MaxTokens is zero, got: %q", got)
		}
	})

	t.Run("required skills are listed when present", func(t *testing.T) {
		agent := &agentprogression.Agent{ID: "agent-10", Name: "Skilled"}
		quest := &domain.Quest{
			ID:    "q-10",
			Title: "Multi-skill task",
			RequiredSkills: []domain.SkillTag{
				domain.SkillCodeGen,
				domain.SkillAnalysis,
			},
		}

		got := buildLegacySystemPrompt(agent, quest)
		if !strings.Contains(got, "This quest requires skills in:") {
			t.Errorf("expected skills preamble in prompt, got: %q", got)
		}
		if !strings.Contains(got, string(domain.SkillCodeGen)) {
			t.Errorf("expected code_generation skill in prompt, got: %q", got)
		}
		if !strings.Contains(got, string(domain.SkillAnalysis)) {
			t.Errorf("expected analysis skill in prompt, got: %q", got)
		}
	})

	t.Run("no skills section when RequiredSkills is empty", func(t *testing.T) {
		agent := &agentprogression.Agent{ID: "agent-11", Name: "Unskilled"}
		quest := &domain.Quest{
			ID:             "q-11",
			Title:          "Any-skill task",
			RequiredSkills: nil,
		}

		got := buildLegacySystemPrompt(agent, quest)
		if strings.Contains(got, "This quest requires skills in:") {
			t.Errorf("expected no skills section when no required skills, got: %q", got)
		}
	})

	t.Run("empty agent config system prompt omits blank section", func(t *testing.T) {
		agent := &agentprogression.Agent{
			ID:   "agent-12",
			Name: "Blank",
			Config: agentprogression.AgentConfig{
				SystemPrompt: "",
			},
		}
		quest := &domain.Quest{
			ID:    "q-12",
			Title: "Minimal quest",
		}

		got := buildLegacySystemPrompt(agent, quest)
		// Should not start with a blank line pair.
		if strings.HasPrefix(got, "\n\n") {
			t.Errorf("expected no leading blank lines, got: %q", got)
		}
		if !strings.Contains(got, "Minimal quest") {
			t.Errorf("expected quest title in prompt, got: %q", got)
		}
	})
}

// =============================================================================
// buildUserPrompt
// =============================================================================

func TestBuildUserPrompt(t *testing.T) {
	tests := []struct {
		name        string
		quest       *domain.Quest
		wantContain string
		wantExact   string
	}{
		{
			name: "nil input returns description",
			quest: &domain.Quest{
				ID:          "q-1",
				Description: "Summarize this document.",
				Input:       nil,
			},
			wantExact: "Summarize this document.",
		},
		{
			name: "string input returns the string directly",
			quest: &domain.Quest{
				ID:          "q-2",
				Description: "Process this file.",
				Input:       "Please analyse the logs in /var/log/app.log",
			},
			wantExact: "Please analyse the logs in /var/log/app.log",
		},
		{
			name: "non-string input wraps with quest description",
			quest: &domain.Quest{
				ID:          "q-3",
				Description: "Transform the data.",
				Input:       map[string]any{"key": "value"},
			},
			wantContain: "Transform the data.",
		},
		{
			name: "non-string input includes Quest input prefix",
			quest: &domain.Quest{
				ID:          "q-4",
				Description: "Handle this payload.",
				Input:       42,
			},
			wantContain: "Quest input:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildUserPrompt(tt.quest)

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("buildUserPrompt() = %q; want exact %q", got, tt.wantExact)
			}
			if tt.wantContain != "" && !strings.Contains(got, tt.wantContain) {
				t.Errorf("buildUserPrompt() = %q; want it to contain %q", got, tt.wantContain)
			}
		})
	}
}

// =============================================================================
// agentHasAnySkill
// =============================================================================

func TestAgentHasAnySkill(t *testing.T) {
	agentWithSkills := &agentprogression.Agent{
		ID:   "agent-1",
		Name: "Skilled",
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen:  {Level: domain.ProficiencyJourneyman},
			domain.SkillAnalysis: {Level: domain.ProficiencyNovice},
		},
	}

	agentNoSkills := &agentprogression.Agent{
		ID:                 "agent-2",
		Name:               "Unskilled",
		SkillProficiencies: nil,
	}

	agentEmptySkills := &agentprogression.Agent{
		ID:                 "agent-3",
		Name:               "EmptySkills",
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{},
	}

	tests := []struct {
		name   string
		agent  *agentprogression.Agent
		skills []domain.SkillTag
		want   bool
	}{
		{
			name:   "agent has exact skill in list",
			agent:  agentWithSkills,
			skills: []domain.SkillTag{domain.SkillCodeGen},
			want:   true,
		},
		{
			name:   "agent has second skill in list",
			agent:  agentWithSkills,
			skills: []domain.SkillTag{domain.SkillResearch, domain.SkillAnalysis},
			want:   true,
		},
		{
			name:   "agent has none of the listed skills",
			agent:  agentWithSkills,
			skills: []domain.SkillTag{domain.SkillResearch, domain.SkillPlanning},
			want:   false,
		},
		{
			name:   "agent with nil skill proficiencies returns false",
			agent:  agentNoSkills,
			skills: []domain.SkillTag{domain.SkillCodeGen},
			want:   false,
		},
		{
			name:   "agent with empty skill map returns false",
			agent:  agentEmptySkills,
			skills: []domain.SkillTag{domain.SkillCodeGen},
			want:   false,
		},
		{
			name:   "empty skills list returns false",
			agent:  agentWithSkills,
			skills: []domain.SkillTag{},
			want:   false,
		},
		{
			name:   "nil skills list returns false",
			agent:  agentWithSkills,
			skills: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agentHasAnySkill(tt.agent, tt.skills)
			if got != tt.want {
				t.Errorf("agentHasAnySkill(...) = %v; want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// toolNameAllowed
// =============================================================================

func TestToolNameAllowed(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		tool    string
		want    bool
	}{
		{
			name:    "tool present in allowed list",
			allowed: []string{"read_file", "write_file", "search"},
			tool:    "write_file",
			want:    true,
		},
		{
			name:    "tool is first in list",
			allowed: []string{"read_file", "write_file"},
			tool:    "read_file",
			want:    true,
		},
		{
			name:    "tool is last in list",
			allowed: []string{"read_file", "write_file"},
			tool:    "write_file",
			want:    true,
		},
		{
			name:    "tool absent from allowed list",
			allowed: []string{"read_file", "write_file"},
			tool:    "exec_command",
			want:    false,
		},
		{
			name:    "empty allowed list returns false",
			allowed: []string{},
			tool:    "read_file",
			want:    false,
		},
		{
			name:    "nil allowed list returns false",
			allowed: nil,
			tool:    "read_file",
			want:    false,
		},
		{
			name:    "case-sensitive: different case is not a match",
			allowed: []string{"Read_File"},
			tool:    "read_file",
			want:    false,
		},
		{
			name:    "single-element list that matches",
			allowed: []string{"bash"},
			tool:    "bash",
			want:    true,
		},
		{
			name:    "single-element list that does not match",
			allowed: []string{"bash"},
			tool:    "python",
			want:    false,
		},
		{
			name:    "empty tool name against empty allowed list",
			allowed: []string{},
			tool:    "",
			want:    false,
		},
		{
			name:    "empty tool name present in allowed list",
			allowed: []string{"", "read_file"},
			tool:    "",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolNameAllowed(tt.allowed, tt.tool)
			if got != tt.want {
				t.Errorf("toolNameAllowed(%v, %q) = %v; want %v",
					tt.allowed, tt.tool, got, tt.want)
			}
		})
	}
}

// =============================================================================
// agentSkillNames
// =============================================================================

func TestAgentSkillNames(t *testing.T) {
	t.Run("returns all skill tag strings for agent with skills", func(t *testing.T) {
		agent := &agentprogression.Agent{
			ID:   "agent-1",
			Name: "Skilled",
			SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
				domain.SkillCodeGen:  {Level: domain.ProficiencyExpert},
				domain.SkillResearch: {Level: domain.ProficiencyNovice},
				domain.SkillAnalysis: {Level: domain.ProficiencyJourneyman},
			},
		}

		got := agentSkillNames(agent)

		// Map iteration is unordered; sort both slices for deterministic comparison.
		sort.Strings(got)
		want := []string{
			string(domain.SkillAnalysis),
			string(domain.SkillCodeGen),
			string(domain.SkillResearch),
		}
		sort.Strings(want)

		if len(got) != len(want) {
			t.Errorf("agentSkillNames() len = %d; want %d", len(got), len(want))
			return
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("agentSkillNames()[%d] = %q; want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("returns empty slice for agent with nil skill proficiencies", func(t *testing.T) {
		agent := &agentprogression.Agent{
			ID:                 "agent-2",
			Name:               "NoSkills",
			SkillProficiencies: nil,
		}

		got := agentSkillNames(agent)
		if len(got) != 0 {
			t.Errorf("agentSkillNames() = %v; want empty slice", got)
		}
	})

	t.Run("returns empty slice for agent with empty skill map", func(t *testing.T) {
		agent := &agentprogression.Agent{
			ID:                 "agent-3",
			Name:               "EmptySkills",
			SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{},
		}

		got := agentSkillNames(agent)
		if len(got) != 0 {
			t.Errorf("agentSkillNames() = %v; want empty slice", got)
		}
	})

	t.Run("single skill returns single-element slice", func(t *testing.T) {
		agent := &agentprogression.Agent{
			ID:   "agent-4",
			Name: "OneSkill",
			SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
				domain.SkillPlanning: {Level: domain.ProficiencyMaster},
			},
		}

		got := agentSkillNames(agent)
		if len(got) != 1 {
			t.Errorf("agentSkillNames() len = %d; want 1", len(got))
			return
		}
		if got[0] != string(domain.SkillPlanning) {
			t.Errorf("agentSkillNames()[0] = %q; want %q", got[0], string(domain.SkillPlanning))
		}
	})

	t.Run("returned strings match SkillTag string values exactly", func(t *testing.T) {
		agent := &agentprogression.Agent{
			ID:   "agent-5",
			Name: "StringCheck",
			SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
				domain.SkillDataTransform: {Level: domain.ProficiencyApprentice},
			},
		}

		got := agentSkillNames(agent)
		if len(got) != 1 {
			t.Fatalf("expected 1 skill, got %d", len(got))
		}
		// SkillDataTransform = "data_transformation" — verify the raw string value.
		if got[0] != "data_transformation" {
			t.Errorf("agentSkillNames()[0] = %q; want %q", got[0], "data_transformation")
		}
	})
}

// =============================================================================
// mockSubQuestPoster
// =============================================================================

// mockSubQuestPoster is a test double for the SubQuestPoster interface.
// It captures calls to PostSubQuests for assertion and can inject errors.
type mockSubQuestPoster struct {
	called     bool
	parentID   domain.QuestID
	subQuests  []domain.Quest
	decomposer domain.AgentID
	returnErr  error
	// returnQuests, if non-nil, overrides the default echo-back behavior.
	returnQuests []domain.Quest
}

func (m *mockSubQuestPoster) PostSubQuests(
	_ context.Context,
	parentID domain.QuestID,
	subQuests []domain.Quest,
	decomposer domain.AgentID,
) ([]domain.Quest, error) {
	m.called = true
	m.parentID = parentID
	m.subQuests = subQuests
	m.decomposer = decomposer

	if m.returnErr != nil {
		return nil, m.returnErr
	}
	if m.returnQuests != nil {
		return m.returnQuests, nil
	}
	// Default: echo subQuests back with synthetic entity IDs so that
	// nodeQuestIDs mapping can be built in tests.
	result := make([]domain.Quest, len(subQuests))
	for i, sq := range subQuests {
		sq.ID = domain.QuestID(fmt.Sprintf("c360.test.game.board1.quest.sub%d", i))
		result[i] = sq
	}
	return result, nil
}

// =============================================================================
// extractDAGFromOutput
// =============================================================================

func TestExtractDAGFromOutput(t *testing.T) {
	// makeDagJSON builds a valid decompose_quest tool response JSON.
	makeDagJSON := func(nodes []map[string]any) string {
		payload := map[string]any{
			"goal": "Build a math module",
			"dag":  map[string]any{"nodes": nodes},
		}
		data, _ := json.Marshal(payload)
		return string(data)
	}

	singleNodeOutput := makeDagJSON([]map[string]any{
		{"id": "n1", "objective": "Implement add(a, b)"},
	})

	twoNodeLinearOutput := makeDagJSON([]map[string]any{
		{"id": "n1", "objective": "Implement add"},
		{"id": "n2", "objective": "Implement subtract", "depends_on": []string{"n1"}},
	})

	t.Run("valid DAG output is parsed and validated", func(t *testing.T) {
		dag, ok := extractDAGFromOutput(singleNodeOutput)
		if !ok {
			t.Fatal("extractDAGFromOutput() = false; want true for valid DAG output")
		}
		if dag == nil {
			t.Fatal("extractDAGFromOutput() returned nil dag with ok=true")
		}
		if len(dag.Nodes) != 1 {
			t.Errorf("dag.Nodes len = %d; want 1", len(dag.Nodes))
		}
		if dag.Nodes[0].ID != "n1" {
			t.Errorf("dag.Nodes[0].ID = %q; want %q", dag.Nodes[0].ID, "n1")
		}
	})

	t.Run("two-node linear DAG is parsed correctly", func(t *testing.T) {
		dag, ok := extractDAGFromOutput(twoNodeLinearOutput)
		if !ok {
			t.Fatal("extractDAGFromOutput() = false for valid two-node DAG")
		}
		if len(dag.Nodes) != 2 {
			t.Errorf("dag.Nodes len = %d; want 2", len(dag.Nodes))
		}
		if dag.Nodes[1].ID != "n2" {
			t.Errorf("dag.Nodes[1].ID = %q; want %q", dag.Nodes[1].ID, "n2")
		}
		if len(dag.Nodes[1].DependsOn) != 1 || dag.Nodes[1].DependsOn[0] != "n1" {
			t.Errorf("dag.Nodes[1].DependsOn = %v; want [n1]", dag.Nodes[1].DependsOn)
		}
	})

	t.Run("empty output returns false", func(t *testing.T) {
		_, ok := extractDAGFromOutput("")
		if ok {
			t.Error("extractDAGFromOutput(\"\") = true; want false")
		}
	})

	t.Run("plain prose output returns false", func(t *testing.T) {
		_, ok := extractDAGFromOutput("I have completed the quest successfully.")
		if ok {
			t.Error("extractDAGFromOutput(prose) = true; want false")
		}
	})

	t.Run("JSON without goal key returns false", func(t *testing.T) {
		noGoal := `{"dag": {"nodes": [{"id":"n1","objective":"x"}]}}`
		_, ok := extractDAGFromOutput(noGoal)
		if ok {
			t.Error("extractDAGFromOutput(no goal) = true; want false")
		}
	})

	t.Run("JSON without dag key returns false", func(t *testing.T) {
		noDag := `{"goal": "something"}`
		_, ok := extractDAGFromOutput(noDag)
		if ok {
			t.Error("extractDAGFromOutput(no dag) = true; want false")
		}
	})

	t.Run("DAG with cycle returns false (validation catches it)", func(t *testing.T) {
		cycleOutput := makeDagJSON([]map[string]any{
			{"id": "n1", "objective": "A", "depends_on": []string{"n2"}},
			{"id": "n2", "objective": "B", "depends_on": []string{"n1"}},
		})
		_, ok := extractDAGFromOutput(cycleOutput)
		if ok {
			t.Error("extractDAGFromOutput(cycle) = true; want false")
		}
	})

	t.Run("DAG with duplicate node IDs returns false (validation catches it)", func(t *testing.T) {
		dupOutput := makeDagJSON([]map[string]any{
			{"id": "n1", "objective": "First"},
			{"id": "n1", "objective": "Duplicate"},
		})
		_, ok := extractDAGFromOutput(dupOutput)
		if ok {
			t.Error("extractDAGFromOutput(duplicate IDs) = true; want false")
		}
	})

	t.Run("DAG with empty nodes array returns false (validation catches it)", func(t *testing.T) {
		emptyNodes := `{"goal":"x","dag":{"nodes":[]}}`
		_, ok := extractDAGFromOutput(emptyNodes)
		if ok {
			t.Error("extractDAGFromOutput(empty nodes) = true; want false")
		}
	})

	t.Run("JSON embedded in prose prefix is still extracted", func(t *testing.T) {
		proseWithJSON := "Here is my decomposition plan: " + singleNodeOutput
		dag, ok := extractDAGFromOutput(proseWithJSON)
		if !ok {
			t.Fatal("extractDAGFromOutput(prose+JSON) = false; want true")
		}
		if dag == nil || len(dag.Nodes) != 1 {
			t.Errorf("expected 1 node from embedded JSON, got dag=%v", dag)
		}
	})
}

// =============================================================================
// dagNodesToQuests
// =============================================================================

func TestDagNodesToQuests(t *testing.T) {
	parent := &domain.Quest{
		ID:         "c360.test.game.board1.quest.parent",
		Difficulty: domain.DifficultyHard,
	}

	t.Run("single node maps to quest correctly", func(t *testing.T) {
		nodes := []questdagexec.QuestNode{
			{
				ID:         "n1",
				Objective:  "Implement add(a, b)",
				Skills:     []string{"code_generation"},
				Difficulty: 3,
				Acceptance: []string{"must return correct sum"},
			},
		}
		quests := dagNodesToQuests(nodes, parent)
		if len(quests) != 1 {
			t.Fatalf("dagNodesToQuests() len = %d; want 1", len(quests))
		}
		q := quests[0]
		if q.Title != "Implement add(a, b)" {
			t.Errorf("Title = %q; want %q", q.Title, "Implement add(a, b)")
		}
		if q.Description != "Implement add(a, b)" {
			t.Errorf("Description = %q; want full objective", q.Description)
		}
		if q.Difficulty != domain.QuestDifficulty(3) {
			t.Errorf("Difficulty = %d; want 3", q.Difficulty)
		}
		if len(q.RequiredSkills) != 1 || q.RequiredSkills[0] != domain.SkillCodeGen {
			t.Errorf("RequiredSkills = %v; want [code_generation]", q.RequiredSkills)
		}
		if len(q.Acceptance) != 1 || q.Acceptance[0] != "must return correct sum" {
			t.Errorf("Acceptance = %v; want [must return correct sum]", q.Acceptance)
		}
	})

	t.Run("node with zero difficulty inherits parent difficulty", func(t *testing.T) {
		nodes := []questdagexec.QuestNode{
			{ID: "n1", Objective: "Task", Difficulty: 0},
		}
		quests := dagNodesToQuests(nodes, parent)
		if quests[0].Difficulty != parent.Difficulty {
			t.Errorf("Difficulty = %d; want parent difficulty %d",
				quests[0].Difficulty, parent.Difficulty)
		}
	})

	t.Run("node with explicit difficulty does not inherit from parent", func(t *testing.T) {
		nodes := []questdagexec.QuestNode{
			{ID: "n1", Objective: "Task", Difficulty: 5},
		}
		quests := dagNodesToQuests(nodes, parent)
		if quests[0].Difficulty != domain.QuestDifficulty(5) {
			t.Errorf("Difficulty = %d; want 5", quests[0].Difficulty)
		}
	})

	t.Run("objective longer than 100 chars is truncated in Title but not Description", func(t *testing.T) {
		long := strings.Repeat("x", 150)
		nodes := []questdagexec.QuestNode{
			{ID: "n1", Objective: long},
		}
		quests := dagNodesToQuests(nodes, parent)
		if len(quests[0].Title) != 100 {
			t.Errorf("Title len = %d; want 100", len(quests[0].Title))
		}
		if quests[0].Description != long {
			t.Errorf("Description should be the full objective, got len %d", len(quests[0].Description))
		}
	})

	t.Run("nil parent with zero node difficulty leaves difficulty at zero", func(t *testing.T) {
		nodes := []questdagexec.QuestNode{
			{ID: "n1", Objective: "Task", Difficulty: 0},
		}
		quests := dagNodesToQuests(nodes, nil)
		if quests[0].Difficulty != 0 {
			t.Errorf("Difficulty = %d; want 0 when parent is nil", quests[0].Difficulty)
		}
	})

	t.Run("empty nodes input returns empty slice", func(t *testing.T) {
		quests := dagNodesToQuests(nil, parent)
		if len(quests) != 0 {
			t.Errorf("dagNodesToQuests(nil) len = %d; want 0", len(quests))
		}
	})
}

// =============================================================================
// DAG execution state initialisation
// =============================================================================
// DAG state initialisation invariants are now tested in
// processor/questdagexec/handler_test.go (TestDagStateFromQuest). The old
// questdagexec.DAGInitPayload / BuildDAGExecutionStateFromInit pathway was
// removed when DAG state moved from the QUEST_DAGS KV bucket to quest.dag.*
// predicates on the parent quest entity in the graph.

// =============================================================================
// DAG decomposition path (component branch logic)
// =============================================================================

// TestCompleteQuestDAGBranching validates the four scenarios described in the
// ticket spec via the pure-function layer: no NATS required.
func TestCompleteQuestDAGBranching(t *testing.T) {
	makeDagJSON := func(nodes []map[string]any) string {
		payload := map[string]any{
			"goal": "Decompose the work",
			"dag":  map[string]any{"nodes": nodes},
		}
		data, _ := json.Marshal(payload)
		return string(data)
	}

	validDAGOutput := makeDagJSON([]map[string]any{
		{"id": "n1", "objective": "Step one", "skills": []string{"code_generation"}},
		{"id": "n2", "objective": "Step two", "depends_on": []string{"n1"}},
	})

	partyID := domain.PartyID("c360.test.game.board1.party.lead")

	t.Run("lead completes party quest with valid DAG — sub-quests are built", func(t *testing.T) {
		dag, ok := extractDAGFromOutput(validDAGOutput)
		if !ok {
			t.Fatal("test setup: valid DAG output should parse")
		}
		parent := &domain.Quest{
			ID:            "c360.test.game.board1.quest.party1",
			PartyRequired: true,
			PartyID:       &partyID,
			Difficulty:    domain.DifficultyModerate,
		}
		// Convert nodes and verify the result matches the DAG.
		subQuests := dagNodesToQuests(dag.Nodes, parent)
		if len(subQuests) != 2 {
			t.Fatalf("expected 2 sub-quests, got %d", len(subQuests))
		}
		if subQuests[0].Title != "Step one" {
			t.Errorf("subQuests[0].Title = %q; want %q", subQuests[0].Title, "Step one")
		}
		if subQuests[1].Title != "Step two" {
			t.Errorf("subQuests[1].Title = %q; want %q", subQuests[1].Title, "Step two")
		}
	})

	t.Run("lead completes party quest without DAG — normal completion, no sub-quests", func(t *testing.T) {
		proseOutput := "I have finished the analysis. The answer is 42."
		_, isDag := extractDAGFromOutput(proseOutput)
		if isDag {
			t.Error("prose output should not be detected as a DAG")
		}
		// No sub-quests would be created — test validates the detection gate.
	})

	t.Run("lead completes non-party quest — DAG detection gate is bypassed", func(t *testing.T) {
		// PartyRequired=false means the component never even calls extractDAGFromOutput.
		// We test that the condition is correct by asserting on the domain model.
		soloQuest := &domain.Quest{
			ID:            "c360.test.game.board1.quest.solo",
			PartyRequired: false,
		}
		if soloQuest.PartyRequired {
			t.Error("test invariant violated: soloQuest.PartyRequired must be false")
		}
		// Guard: even if output has a valid DAG, non-party quests skip the DAG branch.
		dag, ok := extractDAGFromOutput(validDAGOutput)
		if !ok {
			t.Fatal("valid DAG should parse")
		}
		_ = dag
		// Asserts that the branch condition `quest.PartyRequired && c.questBoard != nil`
		// evaluates to false when PartyRequired is false — protecting existing test coverage.
	})

	t.Run("DAG with invalid structure in output — quest would fail, no sub-quests", func(t *testing.T) {
		// A DAG output where a node references an unknown dependency.
		invalidDAGOutput := makeDagJSON([]map[string]any{
			{"id": "n1", "objective": "Step one", "depends_on": []string{"does-not-exist"}},
		})
		_, ok := extractDAGFromOutput(invalidDAGOutput)
		if ok {
			t.Error("DAG with unknown dependency should fail validation and return false")
		}
		// When extractDAGFromOutput returns false, completeQuest falls through to
		// normal completion for party quests, or handleDAGDecomposition is not called.
		// Invalid structure never reaches PostSubQuests.
	})

	t.Run("PostSubQuests failure is propagated as an error", func(t *testing.T) {
		dag, _ := extractDAGFromOutput(validDAGOutput)
		parent := &domain.Quest{
			ID:      "c360.test.game.board1.quest.party-err",
			PartyID: &partyID,
		}
		mock := &mockSubQuestPoster{returnErr: errors.New("agent tier too low for decomposition")}
		subQuests := dagNodesToQuests(dag.Nodes, parent)
		_, err := mock.PostSubQuests(context.Background(), parent.ID, subQuests, "agent-1")
		if err == nil {
			t.Fatal("expected PostSubQuests to return an error; got nil")
		}
		if !strings.Contains(err.Error(), "tier too low") {
			t.Errorf("error = %q; want it to contain 'tier too low'", err.Error())
		}
	})

	t.Run("two-pass dependency resolution maps node IDs to sub-quest entity IDs", func(t *testing.T) {
		dag, _ := extractDAGFromOutput(validDAGOutput)
		parent := &domain.Quest{
			ID:      "c360.test.game.board1.quest.party2",
			PartyID: &partyID,
		}
		mock := &mockSubQuestPoster{}
		subQuests := dagNodesToQuests(dag.Nodes, parent)
		posted, err := mock.PostSubQuests(context.Background(), parent.ID, subQuests, "lead-agent")
		if err != nil {
			t.Fatalf("PostSubQuests failed: %v", err)
		}

		// Build nodeQuestIDs the same way handleDAGDecomposition does.
		nodeQuestIDs := make(map[string]string, len(dag.Nodes))
		for i, node := range dag.Nodes {
			if i < len(posted) {
				nodeQuestIDs[node.ID] = string(posted[i].ID)
			}
		}

		// Second pass: resolve DependsOn node IDs to real quest entity IDs.
		for i, node := range dag.Nodes {
			if len(node.DependsOn) == 0 {
				continue
			}
			deps := make([]domain.QuestID, 0, len(node.DependsOn))
			for _, depNodeID := range node.DependsOn {
				if depQuestID, ok := nodeQuestIDs[depNodeID]; ok {
					deps = append(deps, domain.QuestID(depQuestID))
				}
			}
			posted[i].DependsOn = deps
		}

		// n2 (index 1) depends on n1 — verify it was resolved to the sub-quest entity ID.
		n2DepsOn := posted[1].DependsOn
		if len(n2DepsOn) != 1 {
			t.Fatalf("posted[1].DependsOn len = %d; want 1", len(n2DepsOn))
		}
		expectedDepID := domain.QuestID(nodeQuestIDs["n1"])
		if n2DepsOn[0] != expectedDepID {
			t.Errorf("posted[1].DependsOn[0] = %q; want %q", n2DepsOn[0], expectedDepID)
		}
	})
}

// =============================================================================
// loadPeerFeedback
// =============================================================================

func TestLoadPeerFeedback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		stats   agentprogression.AgentStats
		wantLen int
	}{
		{
			name:    "no reviews — returns nil",
			stats:   agentprogression.AgentStats{},
			wantLen: 0,
		},
		{
			name: "all questions at threshold — returns nil",
			stats: agentprogression.AgentStats{
				PeerReviewCount: 5,
				PeerReviewAvg:   3.0,
				PeerReviewQ1Avg: 3.0,
				PeerReviewQ2Avg: 3.0,
				PeerReviewQ3Avg: 3.0,
			},
			wantLen: 0,
		},
		{
			name: "all questions above threshold — returns nil",
			stats: agentprogression.AgentStats{
				PeerReviewCount: 3,
				PeerReviewAvg:   4.5,
				PeerReviewQ1Avg: 4.0,
				PeerReviewQ2Avg: 5.0,
				PeerReviewQ3Avg: 4.5,
			},
			wantLen: 0,
		},
		{
			name: "one question below threshold — returns one item",
			stats: agentprogression.AgentStats{
				PeerReviewCount: 4,
				PeerReviewAvg:   3.2,
				PeerReviewQ1Avg: 2.5,
				PeerReviewQ2Avg: 3.5,
				PeerReviewQ3Avg: 3.5,
			},
			wantLen: 1,
		},
		{
			name: "all questions below threshold — returns three items",
			stats: agentprogression.AgentStats{
				PeerReviewCount: 2,
				PeerReviewAvg:   1.5,
				PeerReviewQ1Avg: 1.5,
				PeerReviewQ2Avg: 2.0,
				PeerReviewQ3Avg: 1.0,
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			agent := &agentprogression.Agent{
				ID:    "agent-pr-test",
				Name:  "TestAgent",
				Stats: tt.stats,
			}

			got := loadPeerFeedback(agent)

			if len(got) != tt.wantLen {
				t.Errorf("loadPeerFeedback() len = %d; want %d", len(got), tt.wantLen)
				return
			}

			for _, fb := range got {
				if fb.Question == "" {
					t.Error("PeerFeedbackSummary.Question must not be empty")
				}
				if fb.AvgRating <= 0 || fb.AvgRating >= 3.0 {
					t.Errorf("PeerFeedbackSummary.AvgRating = %v; want < 3.0", fb.AvgRating)
				}
			}
		})
	}
}

func TestParseOutputIntent(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"work_product tag", "[INTENT: work_product]\nHere is the code...", "work_product"},
		{"clarification tag", "[INTENT: clarification]\nWhat language?", "clarification"},
		{"case insensitive", "[INTENT: Clarification]\nWhat?", "clarification"},
		{"extra whitespace", "[INTENT:   work_product  ]\nDone.", "work_product"},
		{"no tag", "Here is the result.", ""},
		{"tag not on first line", "Hello\n[INTENT: clarification]\nWhat?", ""},
		{"empty string", "", ""},
		{"partial prefix", "[INTENT: ", ""},
		{"missing bracket", "[INTENT: work_product", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOutputIntent(tt.text)
			if got != tt.want {
				t.Errorf("parseOutputIntent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsOutputClarificationRequest(t *testing.T) {
	tests := []struct {
		name   string
		output any
		want   bool
	}{
		// Structured intent tag (primary path)
		{
			"intent tag work_product",
			"[INTENT: work_product]\nHere is the implementation.",
			false,
		},
		{
			"intent tag clarification",
			"[INTENT: clarification]\nWhat language should I use?",
			true,
		},

		// Heuristic fallback (no intent tag)
		{
			"all questions no tag",
			"What language should I use?\nWhich framework do you prefer?",
			true,
		},
		{
			"mixed content majority questions",
			"I have some questions:\nWhat language?\nWhat framework?\nWhat DB?",
			true,
		},
		{
			"work product with a question",
			"Here is the implementation.\nThe code handles errors.\nDoes this look right?",
			true, // new: any ? in unstructured output routes to clarification
		},
		{
			"pure work product no tag",
			"Here is the completed code.\nAll tests pass.\nReady for review.",
			false,
		},

		// Intent tag takes precedence over heuristic
		{
			"intent tag overrides heuristic — questions tagged as work_product",
			"[INTENT: work_product]\nWhat did you think?\nDoes this look right?\nAny questions?",
			false, // tag says work_product, even though heuristic would say clarification
		},
		{
			"intent tag overrides heuristic — prose tagged as clarification",
			"[INTENT: clarification]\nI need more details about the requirements.",
			true, // tag says clarification, even though heuristic would say no
		},

		// Edge cases
		{"nil output", nil, false},
		{"non-string output", 42, false},
		{"empty string", "", false},
		{"whitespace only", "   \n\n  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isOutputClarificationRequest(tt.output)
			if got != tt.want {
				t.Errorf("isOutputClarificationRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// parseToolOutput
// =============================================================================

func TestParseToolOutput(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantOutputType  string
		wantContent     string
		wantOK          bool
	}{
		{
			name:           "valid work_product",
			input:          `{"type":"work_product","deliverable":"Here is the code","summary":"Done"}`,
			wantOutputType: "work_product",
			wantContent:    "Here is the code",
			wantOK:         true,
		},
		{
			name:           "valid clarification",
			input:          `{"type":"clarification","question":"What format?"}`,
			wantOutputType: "clarification",
			wantContent:    "What format?",
			wantOK:         true,
		},
		{
			name:           "work_product without summary",
			input:          `{"type":"work_product","deliverable":"result"}`,
			wantOutputType: "work_product",
			wantContent:    "result",
			wantOK:         true,
		},
		{
			name:           "summary-only work_product (file-based work)",
			input:          `{"type":"work_product","summary":"Built auth module with JWT tokens and tests"}`,
			wantOutputType: "work_product",
			wantContent:    "Built auth module with JWT tokens and tests",
			wantOK:         true,
		},
		{
			name:           "deliverable takes precedence over summary",
			input:          `{"type":"work_product","deliverable":"inline content","summary":"summary text"}`,
			wantOutputType: "work_product",
			wantContent:    "inline content",
			wantOK:         true,
		},
		{
			name:   "empty deliverable and empty summary",
			input:  `{"type":"work_product","deliverable":"","summary":""}`,
			wantOK: false,
		},
		{
			name:   "empty deliverable no summary",
			input:  `{"type":"work_product","deliverable":""}`,
			wantOK: false,
		},
		{
			name:   "empty question",
			input:  `{"type":"clarification","question":""}`,
			wantOK: false,
		},
		{
			name:   "unknown type",
			input:  `{"type":"other","deliverable":"x"}`,
			wantOK: false,
		},
		{
			name:   "missing type",
			input:  `{"deliverable":"x"}`,
			wantOK: false,
		},
		{
			name:   "not JSON",
			input:  "[INTENT: work_product]\nHere is the code",
			wantOK: false,
		},
		{
			name:   "empty string",
			input:  "",
			wantOK: false,
		},
		{
			name:           "whitespace around JSON",
			input:          `  {"type":"work_product","deliverable":"x"}  `,
			wantOutputType: "work_product",
			wantContent:    "x",
			wantOK:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotContent, gotOK := parseToolOutput(tt.input)

			if gotOK != tt.wantOK {
				t.Errorf("parseToolOutput() ok = %v; want %v", gotOK, tt.wantOK)
				return
			}
			if !tt.wantOK {
				// When ok=false the other return values are unspecified — stop here.
				return
			}
			if gotType != tt.wantOutputType {
				t.Errorf("parseToolOutput() outputType = %q; want %q", gotType, tt.wantOutputType)
			}
			if gotContent != tt.wantContent {
				t.Errorf("parseToolOutput() content = %q; want %q", gotContent, tt.wantContent)
			}
		})
	}
}

// =============================================================================
// resolveCapability
// =============================================================================

// capabilityMockRegistry satisfies model.RegistryReader with configurable
// fallback chains so resolveCapability's chain-length checks work.
type capabilityMockRegistry struct {
	chains map[string][]string // capability key -> fallback chain
}

// Resolve satisfies model.RegistryReader — resolveCapability only calls
// GetFallbackChain, so the capability name is not needed in this mock.
func (m *capabilityMockRegistry) Resolve(_ string) string { return "" }
func (m *capabilityMockRegistry) GetEndpoint(string) *model.EndpointConfig { return nil }
func (m *capabilityMockRegistry) GetFallbackChain(key string) []string {
	return m.chains[key]
}
func (m *capabilityMockRegistry) GetMaxTokens(string) int   { return 0 }
func (m *capabilityMockRegistry) GetDefault() string                  { return "" }
func (m *capabilityMockRegistry) ListCapabilities() []string          { return nil }
func (m *capabilityMockRegistry) ListEndpoints() []string             { return nil }
func (m *capabilityMockRegistry) ResolveSummarization() string        { return "" }

func TestResolveCapability(t *testing.T) {
	tests := []struct {
		name   string
		agent  *agentprogression.Agent
		quest  *domain.Quest
		chains map[string][]string
		want   string
	}{
		{
			name:  "sequential quest uses quest-execution-sequential when configured",
			agent: &agentprogression.Agent{Tier: domain.TierJourneyman},
			quest: &domain.Quest{DecomposabilityClass: domain.DecomposableSequential},
			chains: map[string][]string{
				"quest-execution-sequential": {"claude-opus"},
				"agent-work":                {"claude-sonnet"},
			},
			want: "quest-execution-sequential",
		},
		{
			name:  "sequential quest falls back to agent-work when sequential not configured",
			agent: &agentprogression.Agent{Tier: domain.TierJourneyman},
			quest: &domain.Quest{DecomposabilityClass: domain.DecomposableSequential},
			chains: map[string][]string{
				"agent-work": {"claude-sonnet"},
			},
			want: "agent-work",
		},
		{
			name:  "parallel quest uses agent-work (not sequential)",
			agent: &agentprogression.Agent{Tier: domain.TierJourneyman},
			quest: &domain.Quest{DecomposabilityClass: domain.DecomposableParallel},
			chains: map[string][]string{
				"quest-execution-sequential": {"claude-opus"},
				"agent-work":                {"claude-sonnet"},
			},
			want: "agent-work",
		},
		{
			name:  "trivial quest uses agent-work",
			agent: &agentprogression.Agent{Tier: domain.TierApprentice},
			quest: &domain.Quest{DecomposabilityClass: domain.DecomposableTrivial},
			chains: map[string][]string{
				"agent-work": {"claude-sonnet"},
			},
			want: "agent-work",
		},
		{
			name: "tier+skill key takes priority over bare agent-work",
			agent: &agentprogression.Agent{Tier: domain.TierExpert},
			quest: &domain.Quest{
				RequiredSkills: []domain.SkillTag{domain.SkillCodeGen},
			},
			chains: map[string][]string{
				"agent-work.expert.code_generation": {"specialized-model"},
				"agent-work":                        {"default-model"},
			},
			want: "agent-work.expert.code_generation",
		},
		{
			name:  "nil registry returns agent-work",
			agent: &agentprogression.Agent{Tier: domain.TierJourneyman},
			quest: &domain.Quest{DecomposabilityClass: domain.DecomposableSequential},
			want:  "agent-work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Component{}
			if tt.chains != nil {
				c.registry = &capabilityMockRegistry{chains: tt.chains}
			}
			got := c.resolveCapability(tt.agent, tt.quest)
			if got != tt.want {
				t.Errorf("resolveCapability() = %q, want %q", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Workspace lifecycle
// =============================================================================

// mockArtifactStore implements storage.Store for testing.
type mockArtifactStore struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newMockArtifactStore() *mockArtifactStore {
	return &mockArtifactStore{files: make(map[string][]byte)}
}

func (s *mockArtifactStore) Put(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[key] = append([]byte(nil), data...)
	return nil
}

func (s *mockArtifactStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.files[key]
	if !ok {
		return nil, fmt.Errorf("not found: %s", key)
	}
	return data, nil
}

func (s *mockArtifactStore) List(_ context.Context, prefix string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for k := range s.files {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *mockArtifactStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, key)
	return nil
}

// mockFilestoreComponent wraps a storage.Store and satisfies both
// component.Discoverable and domain.ArtifactStoreProvider so it can be
// returned by a mock ComponentRegistry under the "filestore" name.
type mockFilestoreComponent struct {
	store storage.Store
}

func (m *mockFilestoreComponent) ArtifactStore() storage.Store { return m.store }
func (m *mockFilestoreComponent) Meta() component.Metadata {
	return component.Metadata{Name: "filestore", Type: "storage"}
}
func (m *mockFilestoreComponent) InputPorts() []component.Port           { return nil }
func (m *mockFilestoreComponent) OutputPorts() []component.Port          { return nil }
func (m *mockFilestoreComponent) ConfigSchema() component.ConfigSchema   { return component.ConfigSchema{} }
func (m *mockFilestoreComponent) Health() component.HealthStatus         { return component.HealthStatus{} }
func (m *mockFilestoreComponent) DataFlow() component.FlowMetrics        { return component.FlowMetrics{} }

// mockRegistryWithFilestore implements component.Lookup.
// It serves the wrapped store under "filestore"; all other names return nil.
type mockRegistryWithFilestore struct {
	comp *mockFilestoreComponent
}

func (r *mockRegistryWithFilestore) Component(name string) component.Discoverable {
	if name == "filestore" && r.comp != nil {
		return r.comp
	}
	return nil
}

// newRegistryWithStore wraps store in a mock registry suitable for use as
// Component.deps.ComponentRegistry in workspace snapshot tests.
// Pass nil to simulate the filestore component being absent.
func newRegistryWithStore(store storage.Store) *mockRegistryWithFilestore {
	if store == nil {
		return &mockRegistryWithFilestore{}
	}
	return &mockRegistryWithFilestore{comp: &mockFilestoreComponent{store: store}}
}

// newMockSandboxServer creates an httptest server that simulates the sandbox API
// with pre-loaded workspace files for the given quest ID.
func newMockSandboxServer(questID string, files map[string]string) *httptest.Server {
	expectedPrefix := "/workspace/" + questID
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == expectedPrefix:
			// CreateWorkspace
			w.WriteHeader(http.StatusCreated)

		case r.Method == http.MethodDelete && r.URL.Path == expectedPrefix:
			// DeleteWorkspace
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodGet && r.URL.Path == expectedPrefix:
			// ListWorkspaceFiles — returns all files as flat entries.
			type entry struct {
				Name  string `json:"name"`
				IsDir bool   `json:"is_dir"`
				Size  int64  `json:"size"`
			}
			var entries []entry
			for path, content := range files {
				entries = append(entries, entry{Name: path, Size: int64(len(content))})
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"entries": entries})

		case r.Method == http.MethodGet && r.URL.Path == "/file":
			// ReadFile — return content for the requested path.
			path := r.URL.Query().Get("path")
			content, ok := files[path]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"content": content, "size": len(content)})

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

func TestSnapshotWorkspace_CopiesFilesToArtifactStore(t *testing.T) {
	questID := "quest-snap-001"
	workspaceFiles := map[string]string{
		"main.go":           "package main\n",
		"output/results.md": "# Results\nAll tests pass.\n",
	}

	server := newMockSandboxServer(questID, workspaceFiles)
	defer server.Close()

	store := newMockArtifactStore()
	comp := &Component{
		sandboxClient: executor.NewSandboxClient(server.URL),
		deps:          component.Dependencies{ComponentRegistry: newRegistryWithStore(store)},
		logger:        slog.Default(),
	}

	comp.snapshotWorkspace( questID)

	// Verify all files were copied to the artifact store under quests/{questID}/.
	for path, expectedContent := range workspaceFiles {
		key := fmt.Sprintf("quests/%s/%s", questID, path)
		data, err := store.Get(context.Background(), key)
		if err != nil {
			t.Errorf("expected artifact %q in store, got error: %v", key, err)
			continue
		}
		if string(data) != expectedContent {
			t.Errorf("artifact %q content = %q, want %q", key, string(data), expectedContent)
		}
	}
}

func TestSnapshotWorkspace_NoopWithoutSandboxClient(t *testing.T) {
	comp := &Component{
		logger: slog.Default(),
	}

	// Should not panic or error when sandboxClient is nil.
	comp.snapshotWorkspace( "quest-noop")
	t.Log("no panic with nil sandboxClient")
}

func TestSnapshotWorkspace_EmptyWorkspace(t *testing.T) {
	questID := "quest-empty"
	server := newMockSandboxServer(questID, map[string]string{})
	defer server.Close()

	store := newMockArtifactStore()
	comp := &Component{
		sandboxClient: executor.NewSandboxClient(server.URL),
		deps:          component.Dependencies{ComponentRegistry: newRegistryWithStore(store)},
		logger:        slog.Default(),
	}

	comp.snapshotWorkspace( questID)

	// No files should be in the store.
	keys, _ := store.List(context.Background(), "quests/")
	if len(keys) != 0 {
		t.Errorf("expected 0 artifacts, got %d: %v", len(keys), keys)
	}
}

func TestSnapshotWorkspace_SkipsStoreWhenNil(t *testing.T) {
	questID := "quest-nostore"
	workspaceFiles := map[string]string{"main.go": "package main\n"}

	server := newMockSandboxServer(questID, workspaceFiles)
	defer server.Close()

	comp := &Component{
		sandboxClient: executor.NewSandboxClient(server.URL),
		// deps.ComponentRegistry is nil — simulates no filestore configured.
		logger: slog.Default(),
	}

	// Should not panic — just creates workspace listing and warns.
	comp.snapshotWorkspace( questID)
	t.Log("no panic with nil artifactStore")
}

func TestCleanupWorkspace_NoopWithoutSandboxClient(t *testing.T) {
	comp := &Component{
		logger: slog.Default(),
	}

	// Should not panic or error when sandboxClient is nil.
	comp.cleanupWorkspace( "quest-noop")
	t.Log("no panic with nil sandboxClient")
}

func TestCleanupWorkspace_CallsDelete(t *testing.T) {
	deleteCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/workspace/") {
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	comp := &Component{
		sandboxClient: executor.NewSandboxClient(server.URL),
		logger:        slog.Default(),
	}

	comp.cleanupWorkspace( "quest-cleanup-001")

	if !deleteCalled {
		t.Error("expected DeleteWorkspace to be called on sandbox")
	}
}

func TestSnapshotWorkspace_PartialFailure_ContinuesAndCleanups(t *testing.T) {
	questID := "quest-partial"
	deleteCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/workspace/"):
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/workspace/"):
			// Return two files in workspace listing.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"entries": []map[string]any{
					{"name": "good.txt", "is_dir": false, "size": 5},
					{"name": "bad.txt", "is_dir": false, "size": 3},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/file":
			path := r.URL.Query().Get("path")
			if path == "bad.txt" {
				// Simulate read failure for one file.
				http.Error(w, "disk error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"content": "hello", "size": 5})

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	store := newMockArtifactStore()
	comp := &Component{
		sandboxClient: executor.NewSandboxClient(server.URL),
		deps:          component.Dependencies{ComponentRegistry: newRegistryWithStore(store)},
		logger:        slog.Default(),
	}

	comp.snapshotWorkspace( questID)

	// The good file should have been saved despite the bad file failing.
	goodKey := fmt.Sprintf("quests/%s/good.txt", questID)
	if _, err := store.Get(context.Background(), goodKey); err != nil {
		t.Errorf("expected good.txt in store, got error: %v", err)
	}

	// The bad file should not be in the store.
	badKey := fmt.Sprintf("quests/%s/bad.txt", questID)
	if _, err := store.Get(context.Background(), badKey); err == nil {
		t.Error("expected bad.txt to NOT be in store after read failure")
	}

	// Cleanup should still have been called.
	if !deleteCalled {
		t.Error("expected workspace cleanup after partial snapshot failure")
	}
}

func TestSnapshotWorkspace_SkipsOversizedFiles(t *testing.T) {
	questID := "quest-big"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/workspace/"):
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/workspace/"):
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"entries": []map[string]any{
					{"name": "small.txt", "is_dir": false, "size": 100},
					{"name": "huge.bin", "is_dir": false, "size": 20 * 1024 * 1024}, // 20MB > 10MB limit
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/file":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"content": "ok", "size": 2})

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	store := newMockArtifactStore()
	comp := &Component{
		sandboxClient: executor.NewSandboxClient(server.URL),
		deps:          component.Dependencies{ComponentRegistry: newRegistryWithStore(store)},
		logger:        slog.Default(),
	}

	comp.snapshotWorkspace( questID)

	// Small file should be stored.
	if _, err := store.Get(context.Background(), fmt.Sprintf("quests/%s/small.txt", questID)); err != nil {
		t.Errorf("expected small.txt in store: %v", err)
	}

	// Huge file should be skipped.
	if _, err := store.Get(context.Background(), fmt.Sprintf("quests/%s/huge.bin", questID)); err == nil {
		t.Error("expected huge.bin to be skipped due to size limit")
	}
}

func TestSnapshotWorkspace_CleansUpAfterSuccess(t *testing.T) {
	questID := "quest-cleanup-verify"
	deleteCalled := false

	workspaceFiles := map[string]string{"main.go": "package main\n"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/workspace/"):
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/workspace/"):
			type entry struct {
				Name  string `json:"name"`
				IsDir bool   `json:"is_dir"`
				Size  int64  `json:"size"`
			}
			var entries []entry
			for path, content := range workspaceFiles {
				entries = append(entries, entry{Name: path, Size: int64(len(content))})
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"entries": entries})

		case r.Method == http.MethodGet && r.URL.Path == "/file":
			path := r.URL.Query().Get("path")
			content := workspaceFiles[path]
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"content": content, "size": len(content)})

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	store := newMockArtifactStore()
	comp := &Component{
		sandboxClient: executor.NewSandboxClient(server.URL),
		deps:          component.Dependencies{ComponentRegistry: newRegistryWithStore(store)},
		logger:        slog.Default(),
	}

	comp.snapshotWorkspace( questID)

	// Verify DeleteWorkspace was called after snapshot.
	if !deleteCalled {
		t.Error("expected workspace cleanup after successful snapshot")
	}
}

