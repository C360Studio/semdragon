package questbridge

import (
	"sort"
	"strings"
	"testing"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/message"
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
// extractInstanceFromEntityKey
// =============================================================================

func TestExtractInstanceFromEntityKey(t *testing.T) {
	tests := []struct {
		name      string
		entityKey string
		want      string
	}{
		{
			name:      "standard six-part key returns last segment",
			entityKey: "c360.prod.game.board1.quest.abc123",
			want:      "abc123",
		},
		{
			name:      "six-part key with agent type",
			entityKey: "c360.dev.game.main.agent.dragon",
			want:      "dragon",
		},
		{
			name:      "six-part key with UUID-like instance",
			entityKey: "org.platform.game.board.quest.550e8400-e29b-41d4-a716-446655440000",
			want:      "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:      "single segment with no dots is returned as-is",
			entityKey: "justanid",
			want:      "justanid",
		},
		{
			name:      "two-part key returns last segment",
			entityKey: "quest.abc",
			want:      "abc",
		},
		{
			name:      "key ending in dot returns empty string after last dot",
			entityKey: "c360.prod.game.board.quest.",
			want:      "",
		},
		{
			name:      "empty string returns empty string",
			entityKey: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractInstanceFromEntityKey(tt.entityKey)
			if got != tt.want {
				t.Errorf("extractInstanceFromEntityKey(%q) = %q; want %q",
					tt.entityKey, got, tt.want)
			}
		})
	}
}

// =============================================================================
// buildLegacySystemPrompt
// =============================================================================

func TestBuildLegacySystemPrompt(t *testing.T) {
	t.Run("agent config system prompt is prepended", func(t *testing.T) {
		agent := &semdragons.Agent{
			ID:   "agent-1",
			Name: "Coder",
			Config: semdragons.AgentConfig{
				SystemPrompt: "You are a senior Go developer.",
			},
		}
		quest := &semdragons.Quest{
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
		agent := &semdragons.Agent{
			ID:   "agent-2",
			Name: "Shadowweaver",
			Config: semdragons.AgentConfig{
				SystemPrompt: "Base instructions.",
			},
			Persona: &semdragons.AgentPersona{
				SystemPrompt: "You are Shadowweaver, a cunning rogue.",
			},
		}
		quest := &semdragons.Quest{
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
		agent := &semdragons.Agent{
			ID:      "agent-3",
			Name:    "NoPers",
			Persona: nil,
		}
		quest := &semdragons.Quest{
			ID:    "q-3",
			Title: "A simple task",
		}

		got := buildLegacySystemPrompt(agent, quest)
		if !strings.Contains(got, "A simple task") {
			t.Errorf("expected quest title in prompt, got: %q", got)
		}
	})

	t.Run("quest description is included when non-empty", func(t *testing.T) {
		agent := &semdragons.Agent{ID: "agent-4", Name: "Analyst"}
		quest := &semdragons.Quest{
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
		agent := &semdragons.Agent{ID: "agent-5", Name: "Analyst"}
		quest := &semdragons.Quest{
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
		agent := &semdragons.Agent{ID: "agent-6", Name: "Timed"}
		quest := &semdragons.Quest{
			ID:    "q-6",
			Title: "Timed task",
			Constraints: semdragons.QuestConstraints{
				MaxDuration: 30 * time.Minute,
			},
		}

		got := buildLegacySystemPrompt(agent, quest)
		if !strings.Contains(got, "Time limit:") {
			t.Errorf("expected time limit line in prompt, got: %q", got)
		}
	})

	t.Run("time limit is omitted when MaxDuration is zero", func(t *testing.T) {
		agent := &semdragons.Agent{ID: "agent-7", Name: "Untimed"}
		quest := &semdragons.Quest{
			ID:    "q-7",
			Title: "Untimed task",
			Constraints: semdragons.QuestConstraints{
				MaxDuration: 0,
			},
		}

		got := buildLegacySystemPrompt(agent, quest)
		if strings.Contains(got, "Time limit:") {
			t.Errorf("expected no time limit line when MaxDuration is zero, got: %q", got)
		}
	})

	t.Run("token budget is included when MaxTokens is set", func(t *testing.T) {
		agent := &semdragons.Agent{ID: "agent-8", Name: "Budgeted"}
		quest := &semdragons.Quest{
			ID:    "q-8",
			Title: "Budget task",
			Constraints: semdragons.QuestConstraints{
				MaxTokens: 4096,
			},
		}

		got := buildLegacySystemPrompt(agent, quest)
		if !strings.Contains(got, "Token budget: 4096") {
			t.Errorf("expected token budget line in prompt, got: %q", got)
		}
	})

	t.Run("token budget is omitted when MaxTokens is zero", func(t *testing.T) {
		agent := &semdragons.Agent{ID: "agent-9", Name: "Unbudgeted"}
		quest := &semdragons.Quest{
			ID:    "q-9",
			Title: "No budget task",
		}

		got := buildLegacySystemPrompt(agent, quest)
		if strings.Contains(got, "Token budget:") {
			t.Errorf("expected no token budget line when MaxTokens is zero, got: %q", got)
		}
	})

	t.Run("required skills are listed when present", func(t *testing.T) {
		agent := &semdragons.Agent{ID: "agent-10", Name: "Skilled"}
		quest := &semdragons.Quest{
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
		agent := &semdragons.Agent{ID: "agent-11", Name: "Unskilled"}
		quest := &semdragons.Quest{
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
		agent := &semdragons.Agent{
			ID:   "agent-12",
			Name: "Blank",
			Config: semdragons.AgentConfig{
				SystemPrompt: "",
			},
		}
		quest := &semdragons.Quest{
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
		quest       *semdragons.Quest
		wantContain string
		wantExact   string
	}{
		{
			name: "nil input returns description",
			quest: &semdragons.Quest{
				ID:          "q-1",
				Description: "Summarize this document.",
				Input:       nil,
			},
			wantExact: "Summarize this document.",
		},
		{
			name: "string input returns the string directly",
			quest: &semdragons.Quest{
				ID:          "q-2",
				Description: "Process this file.",
				Input:       "Please analyse the logs in /var/log/app.log",
			},
			wantExact: "Please analyse the logs in /var/log/app.log",
		},
		{
			name: "non-string input wraps with quest description",
			quest: &semdragons.Quest{
				ID:          "q-3",
				Description: "Transform the data.",
				Input:       map[string]any{"key": "value"},
			},
			wantContain: "Transform the data.",
		},
		{
			name: "non-string input includes Quest input prefix",
			quest: &semdragons.Quest{
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
	agentWithSkills := &semdragons.Agent{
		ID:   "agent-1",
		Name: "Skilled",
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen:  {Level: domain.ProficiencyJourneyman},
			domain.SkillAnalysis: {Level: domain.ProficiencyNovice},
		},
	}

	agentNoSkills := &semdragons.Agent{
		ID:                 "agent-2",
		Name:               "Unskilled",
		SkillProficiencies: nil,
	}

	agentEmptySkills := &semdragons.Agent{
		ID:                 "agent-3",
		Name:               "EmptySkills",
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{},
	}

	tests := []struct {
		name   string
		agent  *semdragons.Agent
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
		agent := &semdragons.Agent{
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
		agent := &semdragons.Agent{
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
		agent := &semdragons.Agent{
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
		agent := &semdragons.Agent{
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
		agent := &semdragons.Agent{
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
