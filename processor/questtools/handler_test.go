package questtools

import (
	"io"
	"log/slog"
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/agentic"
)

// newTestComponent constructs a minimal *Component suitable for unit-testing
// methods that do not touch NATS. Only the logger and config fields are set.
func newTestComponent(cfg Config) *Component {
	return &Component{
		config: &cfg,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// =============================================================================
// DefaultConfig
// =============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Org != "default" {
		t.Errorf("Org = %q; want %q", cfg.Org, "default")
	}
	if cfg.Platform != "local" {
		t.Errorf("Platform = %q; want %q", cfg.Platform, "local")
	}
	if cfg.Board != "main" {
		t.Errorf("Board = %q; want %q", cfg.Board, "main")
	}
	if cfg.StreamName != "AGENT" {
		t.Errorf("StreamName = %q; want %q", cfg.StreamName, "AGENT")
	}
	if cfg.QuestLoopsBucket != "QUEST_LOOPS" {
		t.Errorf("QuestLoopsBucket = %q; want %q", cfg.QuestLoopsBucket, "QUEST_LOOPS")
	}
	if cfg.Timeout != "60s" {
		t.Errorf("Timeout = %q; want %q", cfg.Timeout, "60s")
	}
	if !cfg.EnableBuiltins {
		t.Error("EnableBuiltins = false; want true")
	}
	if cfg.SandboxDir != "" {
		t.Errorf("SandboxDir = %q; want empty string", cfg.SandboxDir)
	}
}

// =============================================================================
// buildContextFromMetadata — nil and minimal cases
// =============================================================================

func TestBuildContextFromMetadata_NilMetadata(t *testing.T) {
	c := newTestComponent(DefaultConfig())

	call := &agentic.ToolCall{
		ID:       "call-1",
		Name:     "read_file",
		Metadata: nil,
	}

	agent, quest := c.buildContextFromMetadata(call)

	if agent.Tier != domain.TierApprentice {
		t.Errorf("Tier = %v; want TierApprentice (%v)", agent.Tier, domain.TierApprentice)
	}
	if agent.ID != "" {
		t.Errorf("agent.ID = %q; want empty", agent.ID)
	}
	if quest.ID != "" {
		t.Errorf("quest.ID = %q; want empty", quest.ID)
	}
	if len(agent.SkillProficiencies) != 0 {
		t.Errorf("SkillProficiencies len = %d; want 0", len(agent.SkillProficiencies))
	}
}

// =============================================================================
// buildContextFromMetadata — agent_id extraction
// =============================================================================

func TestBuildContextFromMetadata_AgentID(t *testing.T) {
	c := newTestComponent(DefaultConfig())

	call := &agentic.ToolCall{
		ID:   "call-2",
		Name: "read_file",
		Metadata: map[string]any{
			"agent_id": "dragon-7",
		},
	}

	agent, _ := c.buildContextFromMetadata(call)

	if agent.ID != domain.AgentID("dragon-7") {
		t.Errorf("agent.ID = %q; want %q", agent.ID, "dragon-7")
	}
	// Default tier must still be Apprentice when trust_tier is absent.
	if agent.Tier != domain.TierApprentice {
		t.Errorf("Tier = %v; want TierApprentice", agent.Tier)
	}
}

// =============================================================================
// buildContextFromMetadata — trust_tier as float64 (JSON number default)
// =============================================================================

func TestBuildContextFromMetadata_TrustTierFloat64(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		wantTier domain.TrustTier
	}{
		{
			name:     "TierApprentice (0) accepted",
			value:    float64(domain.TierApprentice),
			wantTier: domain.TierApprentice,
		},
		{
			name:     "TierJourneyman (1) accepted",
			value:    float64(domain.TierJourneyman),
			wantTier: domain.TierJourneyman,
		},
		{
			name:     "TierExpert (2) accepted",
			value:    float64(domain.TierExpert),
			wantTier: domain.TierExpert,
		},
		{
			name:     "TierMaster (3) accepted",
			value:    float64(domain.TierMaster),
			wantTier: domain.TierMaster,
		},
		{
			name:     "TierGrandmaster (4) accepted",
			value:    float64(domain.TierGrandmaster),
			wantTier: domain.TierGrandmaster,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestComponent(DefaultConfig())
			call := &agentic.ToolCall{
				ID:   "call-f64",
				Name: "read_file",
				Metadata: map[string]any{
					"trust_tier": tt.value,
				},
			}

			agent, _ := c.buildContextFromMetadata(call)

			if agent.Tier != tt.wantTier {
				t.Errorf("Tier = %v; want %v", agent.Tier, tt.wantTier)
			}
		})
	}
}

// =============================================================================
// buildContextFromMetadata — trust_tier as int
// =============================================================================

func TestBuildContextFromMetadata_TrustTierInt(t *testing.T) {
	tests := []struct {
		name     string
		value    int
		wantTier domain.TrustTier
	}{
		{
			name:     "TierApprentice (int 0) accepted",
			value:    int(domain.TierApprentice),
			wantTier: domain.TierApprentice,
		},
		{
			name:     "TierExpert (int 2) accepted",
			value:    int(domain.TierExpert),
			wantTier: domain.TierExpert,
		},
		{
			name:     "TierGrandmaster (int 4) accepted",
			value:    int(domain.TierGrandmaster),
			wantTier: domain.TierGrandmaster,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestComponent(DefaultConfig())
			call := &agentic.ToolCall{
				ID:   "call-int",
				Name: "read_file",
				Metadata: map[string]any{
					"trust_tier": tt.value,
				},
			}

			agent, _ := c.buildContextFromMetadata(call)

			if agent.Tier != tt.wantTier {
				t.Errorf("Tier = %v; want %v", agent.Tier, tt.wantTier)
			}
		})
	}
}

// =============================================================================
// buildContextFromMetadata — trust_tier out of bounds → fallback to Apprentice
// =============================================================================

func TestBuildContextFromMetadata_TrustTierOutOfBounds(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "negative float64", value: float64(-1)},
		{name: "too large float64", value: float64(99)},
		{name: "negative int", value: int(-5)},
		{name: "too large int", value: int(100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestComponent(DefaultConfig())
			call := &agentic.ToolCall{
				ID:   "call-oob",
				Name: "read_file",
				Metadata: map[string]any{
					"trust_tier": tt.value,
				},
			}

			agent, _ := c.buildContextFromMetadata(call)

			if agent.Tier != domain.TierApprentice {
				t.Errorf("Tier = %v; want TierApprentice (fallback for out-of-bounds %v)", agent.Tier, tt.value)
			}
		})
	}
}

// =============================================================================
// buildContextFromMetadata — trust_tier wrong Go type is ignored
// =============================================================================

func TestBuildContextFromMetadata_TrustTierWrongType(t *testing.T) {
	// A string value is not handled by any case in the type switch, so the tier
	// should remain at the default TierApprentice.
	c := newTestComponent(DefaultConfig())
	call := &agentic.ToolCall{
		ID:   "call-type",
		Name: "read_file",
		Metadata: map[string]any{
			"trust_tier": "expert",
		},
	}

	agent, _ := c.buildContextFromMetadata(call)

	if agent.Tier != domain.TierApprentice {
		t.Errorf("Tier = %v; want TierApprentice for unhandled type", agent.Tier)
	}
}

// =============================================================================
// buildContextFromMetadata — skills extraction
// =============================================================================

func TestBuildContextFromMetadata_Skills(t *testing.T) {
	t.Run("valid skill strings are mapped at level 1", func(t *testing.T) {
		c := newTestComponent(DefaultConfig())
		call := &agentic.ToolCall{
			ID:   "call-skills",
			Name: "read_file",
			Metadata: map[string]any{
				"skills": []any{"code_generation", "analysis"},
			},
		}

		agent, _ := c.buildContextFromMetadata(call)

		if len(agent.SkillProficiencies) != 2 {
			t.Fatalf("SkillProficiencies len = %d; want 2", len(agent.SkillProficiencies))
		}

		cg, ok := agent.SkillProficiencies[domain.SkillCodeGen]
		if !ok {
			t.Error("SkillCodeGen missing from proficiencies")
		} else if cg.Level != 1 {
			t.Errorf("SkillCodeGen.Level = %d; want 1", cg.Level)
		}

		an, ok := agent.SkillProficiencies[domain.SkillAnalysis]
		if !ok {
			t.Error("SkillAnalysis missing from proficiencies")
		} else if an.Level != 1 {
			t.Errorf("SkillAnalysis.Level = %d; want 1", an.Level)
		}
	})

	t.Run("non-string elements in skills slice are skipped", func(t *testing.T) {
		c := newTestComponent(DefaultConfig())
		call := &agentic.ToolCall{
			ID:   "call-mixed",
			Name: "read_file",
			Metadata: map[string]any{
				"skills": []any{"research", 42, true, "planning"},
			},
		}

		agent, _ := c.buildContextFromMetadata(call)

		// Only the two string entries should have been added.
		if len(agent.SkillProficiencies) != 2 {
			t.Errorf("SkillProficiencies len = %d; want 2 (non-strings skipped)", len(agent.SkillProficiencies))
		}
	})

	t.Run("empty skills slice results in empty proficiency map", func(t *testing.T) {
		c := newTestComponent(DefaultConfig())
		call := &agentic.ToolCall{
			ID:   "call-empty-skills",
			Name: "read_file",
			Metadata: map[string]any{
				"skills": []any{},
			},
		}

		agent, _ := c.buildContextFromMetadata(call)

		if len(agent.SkillProficiencies) != 0 {
			t.Errorf("SkillProficiencies len = %d; want 0", len(agent.SkillProficiencies))
		}
	})

	t.Run("skills key absent leaves nil proficiency map", func(t *testing.T) {
		c := newTestComponent(DefaultConfig())
		call := &agentic.ToolCall{
			ID:       "call-noskills",
			Name:     "read_file",
			Metadata: map[string]any{},
		}

		agent, _ := c.buildContextFromMetadata(call)

		if agent.SkillProficiencies != nil {
			t.Errorf("SkillProficiencies = %v; want nil when key absent", agent.SkillProficiencies)
		}
	})
}

// =============================================================================
// buildContextFromMetadata — quest_id extraction
// =============================================================================

func TestBuildContextFromMetadata_QuestID(t *testing.T) {
	c := newTestComponent(DefaultConfig())
	call := &agentic.ToolCall{
		ID:   "call-qid",
		Name: "read_file",
		Metadata: map[string]any{
			"quest_id": "q-abc123",
		},
	}

	_, quest := c.buildContextFromMetadata(call)

	if quest.ID != domain.QuestID("q-abc123") {
		t.Errorf("quest.ID = %q; want %q", quest.ID, "q-abc123")
	}
}

func TestBuildContextFromMetadata_QuestIDWrongType(t *testing.T) {
	// Non-string quest_id should be silently ignored (type assertion fails).
	c := newTestComponent(DefaultConfig())
	call := &agentic.ToolCall{
		ID:   "call-qid-bad",
		Name: "read_file",
		Metadata: map[string]any{
			"quest_id": 9999,
		},
	}

	_, quest := c.buildContextFromMetadata(call)

	if quest.ID != "" {
		t.Errorf("quest.ID = %q; want empty for non-string value", quest.ID)
	}
}

// =============================================================================
// buildContextFromMetadata — sandbox_dir injection and path escape checks
// =============================================================================

func TestBuildContextFromMetadata_SandboxDir(t *testing.T) {
	t.Run("sandbox_dir from metadata injected into arguments when no component sandbox", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.SandboxDir = "" // component sandbox is not set
		c := newTestComponent(cfg)

		call := &agentic.ToolCall{
			ID:   "call-sandbox-free",
			Name: "read_file",
			Metadata: map[string]any{
				"sandbox_dir": "/tmp/work",
			},
		}

		c.buildContextFromMetadata(call)

		if call.Arguments == nil {
			t.Fatal("call.Arguments is nil; want map with _sandbox_dir")
		}
		got, ok := call.Arguments["_sandbox_dir"]
		if !ok {
			t.Error("_sandbox_dir key missing from call.Arguments")
		} else if got != "/tmp/work" {
			t.Errorf("_sandbox_dir = %v; want %q", got, "/tmp/work")
		}
	})

	t.Run("sandbox_dir from metadata that is a valid sub-path of component sandbox is accepted", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.SandboxDir = "/tmp/sandbox"
		c := newTestComponent(cfg)

		call := &agentic.ToolCall{
			ID:   "call-sandbox-narrow",
			Name: "read_file",
			Metadata: map[string]any{
				"sandbox_dir": "/tmp/sandbox/project",
			},
		}

		c.buildContextFromMetadata(call)

		got, ok := call.Arguments["_sandbox_dir"]
		if !ok {
			t.Fatal("_sandbox_dir key missing from call.Arguments")
		}
		if got != "/tmp/sandbox/project" {
			t.Errorf("_sandbox_dir = %v; want %q", got, "/tmp/sandbox/project")
		}
	})

	t.Run("sandbox_dir that escapes component sandbox is rejected", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.SandboxDir = "/tmp/sandbox"
		c := newTestComponent(cfg)

		call := &agentic.ToolCall{
			ID:   "call-sandbox-escape",
			Name: "read_file",
			Metadata: map[string]any{
				"sandbox_dir": "/etc/passwd",
			},
		}

		c.buildContextFromMetadata(call)

		// The override was rejected: _sandbox_dir should fall back to the
		// component-level sandbox directory, not the attempted escape path.
		got, ok := call.Arguments["_sandbox_dir"]
		if !ok {
			t.Fatal("_sandbox_dir key missing from call.Arguments")
		}
		if got == "/etc/passwd" {
			t.Error("_sandbox_dir was set to escape path; expected component sandbox fallback")
		}
		if got != "/tmp/sandbox" {
			t.Errorf("_sandbox_dir = %v; want component sandbox %q after rejection", got, "/tmp/sandbox")
		}
	})

	t.Run("path traversal via .. is rejected when component sandbox is set", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.SandboxDir = "/tmp/sandbox"
		c := newTestComponent(cfg)

		call := &agentic.ToolCall{
			ID:   "call-sandbox-traversal",
			Name: "read_file",
			Metadata: map[string]any{
				"sandbox_dir": "/tmp/sandbox/../secret",
			},
		}

		c.buildContextFromMetadata(call)

		got, ok := call.Arguments["_sandbox_dir"]
		if !ok {
			t.Fatal("_sandbox_dir key missing from call.Arguments")
		}
		if got == "/tmp/sandbox/../secret" {
			t.Error("_sandbox_dir was set to traversal path; expected component sandbox fallback")
		}
		if got != "/tmp/sandbox" {
			t.Errorf("_sandbox_dir = %v; want component sandbox %q after traversal rejection", got, "/tmp/sandbox")
		}
	})

	t.Run("no sandbox at all: _sandbox_dir not injected into arguments", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.SandboxDir = ""
		c := newTestComponent(cfg)

		call := &agentic.ToolCall{
			ID:   "call-no-sandbox",
			Name: "read_file",
			// No sandbox_dir in metadata, no component sandbox.
		}

		c.buildContextFromMetadata(call)

		if call.Arguments != nil {
			if _, ok := call.Arguments["_sandbox_dir"]; ok {
				t.Error("_sandbox_dir unexpectedly present when no sandbox is configured")
			}
		}
	})

	t.Run("empty sandbox_dir in metadata is ignored when component sandbox is not set", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.SandboxDir = ""
		c := newTestComponent(cfg)

		call := &agentic.ToolCall{
			ID:   "call-empty-sandbox",
			Name: "read_file",
			Metadata: map[string]any{
				"sandbox_dir": "",
			},
		}

		c.buildContextFromMetadata(call)

		// sandboxDir stays "" so _sandbox_dir should not be injected.
		if call.Arguments != nil {
			if v, ok := call.Arguments["_sandbox_dir"]; ok {
				t.Errorf("_sandbox_dir = %v; want absent for empty override with no component sandbox", v)
			}
		}
	})

	t.Run("component-level sandbox injected when metadata has no sandbox_dir", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.SandboxDir = "/var/agent-work"
		c := newTestComponent(cfg)

		call := &agentic.ToolCall{
			ID:       "call-comp-sandbox",
			Name:     "read_file",
			Metadata: map[string]any{},
		}

		c.buildContextFromMetadata(call)

		got, ok := call.Arguments["_sandbox_dir"]
		if !ok {
			t.Fatal("_sandbox_dir key missing from call.Arguments")
		}
		if got != "/var/agent-work" {
			t.Errorf("_sandbox_dir = %v; want %q", got, "/var/agent-work")
		}
	})
}

// =============================================================================
// buildContextFromMetadata — combined metadata fields
// =============================================================================

func TestBuildContextFromMetadata_Combined(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SandboxDir = "/tmp/sandbox"
	c := newTestComponent(cfg)

	call := &agentic.ToolCall{
		ID:   "call-combined",
		Name: "write_file",
		Metadata: map[string]any{
			"agent_id":    "agent-007",
			"trust_tier":  float64(domain.TierExpert),
			"skills":      []any{"code_generation", "planning"},
			"quest_id":    "q-epic-001",
			"sandbox_dir": "/tmp/sandbox/mission",
		},
	}

	agent, quest := c.buildContextFromMetadata(call)

	if agent.ID != domain.AgentID("agent-007") {
		t.Errorf("agent.ID = %q; want %q", agent.ID, "agent-007")
	}
	if agent.Tier != domain.TierExpert {
		t.Errorf("Tier = %v; want TierExpert", agent.Tier)
	}
	if len(agent.SkillProficiencies) != 2 {
		t.Errorf("SkillProficiencies len = %d; want 2", len(agent.SkillProficiencies))
	}
	if _, ok := agent.SkillProficiencies[domain.SkillCodeGen]; !ok {
		t.Error("SkillCodeGen missing")
	}
	if _, ok := agent.SkillProficiencies[domain.SkillPlanning]; !ok {
		t.Error("SkillPlanning missing")
	}
	if quest.ID != domain.QuestID("q-epic-001") {
		t.Errorf("quest.ID = %q; want %q", quest.ID, "q-epic-001")
	}
	sandboxVal, ok := call.Arguments["_sandbox_dir"]
	if !ok {
		t.Fatal("_sandbox_dir missing from arguments")
	}
	if sandboxVal != "/tmp/sandbox/mission" {
		t.Errorf("_sandbox_dir = %v; want %q", sandboxVal, "/tmp/sandbox/mission")
	}
}

// =============================================================================
// buildContextFromMetadata — existing arguments are preserved
// =============================================================================

func TestBuildContextFromMetadata_PreservesExistingArguments(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SandboxDir = "/tmp/work"
	c := newTestComponent(cfg)

	call := &agentic.ToolCall{
		ID:   "call-args",
		Name: "read_file",
		Arguments: map[string]any{
			"path": "/tmp/work/data.txt",
		},
		Metadata: map[string]any{},
	}

	c.buildContextFromMetadata(call)

	if call.Arguments["path"] != "/tmp/work/data.txt" {
		t.Errorf("path argument was overwritten; got %v", call.Arguments["path"])
	}
	if call.Arguments["_sandbox_dir"] != "/tmp/work" {
		t.Errorf("_sandbox_dir = %v; want %q", call.Arguments["_sandbox_dir"], "/tmp/work")
	}
}
