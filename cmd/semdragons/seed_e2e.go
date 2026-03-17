package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/agentstore"
)

// maybeSeedE2E checks SEED_E2E and SEED_AGENTS to determine what data to
// write into the ENTITY_STATES KV bucket on startup.
//
//   - SEED_E2E=true  → Full E2E dataset: agents, quests, store (guilds form organically)
//   - SEED_AGENTS=true → Agents only (no quests or store) — good for first-time use
//
// We write directly via GraphClient.EmitEntity (the same path the questboard
// processor uses) rather than going through the seeding component's pub/sub
// path, because other processors that would consume those events may not be
// started yet and the KV writes must be synchronous before the UI connects.
func maybeSeedE2E(ctx context.Context, cfg *config.Config, natsClient *natsclient.Client) error {
	fullSeed := os.Getenv("SEED_E2E") == "true"
	agentsOnly := os.Getenv("SEED_AGENTS") == "true"

	if !fullSeed && !agentsOnly {
		return nil
	}

	boardCfg, err := extractBoardConfig(cfg)
	if err != nil {
		return fmt.Errorf("seed: resolve board config: %w", err)
	}

	slog.Info("Seeding into board",
		"mode", seedMode(fullSeed),
		"org", boardCfg.Org,
		"platform", boardCfg.Platform,
		"board", boardCfg.Board,
		"bucket", boardCfg.BucketName())

	graph := semdragons.NewGraphClient(natsClient, boardCfg)

	// Both modes seed agents. Full seed also seeds the store catalog.
	// Quests are never seeded — users create them via DM chat or API.
	if err := seedAgents(ctx, graph, boardCfg); err != nil {
		return fmt.Errorf("seed: agents: %w", err)
	}

	if fullSeed {
		if err := seedStore(ctx, graph); err != nil {
			return fmt.Errorf("seed: store: %w", err)
		}
	}

	slog.Info("Seed data written successfully", "mode", seedMode(fullSeed))
	return nil
}

func seedMode(full bool) string {
	if full {
		return "e2e"
	}
	return "agents-only"
}

// extractBoardConfig reads org + platform from the platform config and board
// from the "game" service config, matching what extractPlatformMeta does.
func extractBoardConfig(cfg *config.Config) (*domain.BoardConfig, error) {
	platformID := cfg.Platform.InstanceID
	if platformID == "" {
		platformID = cfg.Platform.ID
	}

	board := "board1" // default matches config/semdragons.json

	if gameSvc, ok := cfg.Services["game"]; ok && len(gameSvc.Config) > 0 {
		var gameCfg struct {
			Board string `json:"board"`
		}
		if err := json.Unmarshal(gameSvc.Config, &gameCfg); err == nil && gameCfg.Board != "" {
			board = gameCfg.Board
		}
	}

	return &domain.BoardConfig{
		Org:      cfg.Platform.Org,
		Platform: platformID,
		Board:    board,
	}, nil
}

// agentSpec describes one agent (or a batch sharing the same profile) to seed.
type agentSpec struct {
	namePattern string // e.g. "apprentice-{n}" or a fixed name
	level       int    // base level assigned
	skills      []domain.SkillTag
	archetype   domain.AgentArchetype
	isNPC       bool
	count       int // how many to create (1 = single named agent)
}

// seedAgents creates all E2E agents at their target levels.
// Agents start unguilded — guilds form organically via boid engine
// suggestions when agents build peer cohesion (shared wins + reviews).
func seedAgents(
	ctx context.Context,
	graph *semdragons.GraphClient,
	boardCfg *domain.BoardConfig,
) error {
	specs := []agentSpec{
		// 1 grandmaster (level 19-20) — board anchor, Strategist
		{
			namePattern: "archon",
			level:       20,
			skills:      []domain.SkillTag{domain.SkillPlanning, domain.SkillAnalysis},
			archetype:   domain.ArchetypeStrategist,
			count:       1,
		},

		// 2 masters (level 16-18) — party leads across guilds
		{
			namePattern: "forge-master",
			level:       17,
			skills:      []domain.SkillTag{domain.SkillCodeGen, domain.SkillCodeReview},
			archetype:   domain.ArchetypeEngineer,
			count:       1,
		},
		{
			namePattern: "lorekeeper",
			level:       16,
			skills:      []domain.SkillTag{domain.SkillResearch, domain.SkillAnalysis},
			archetype:   domain.ArchetypeScholar,
			count:       1,
		},

		// 3 experts (level 11-15) — guild founders; guildformation auto-clusters from Expert+ unguilded agents
		{
			namePattern: "iron-coder",
			level:       14,
			skills:      []domain.SkillTag{domain.SkillCodeGen},
			archetype:   domain.ArchetypeEngineer,
			count:       1,
		},
		{
			namePattern: "data-sage",
			level:       13,
			skills:      []domain.SkillTag{domain.SkillDataTransform, domain.SkillAnalysis},
			archetype:   domain.ArchetypeEngineer,
			count:       1,
		},
		{
			namePattern: "herald",
			level:       12,
			skills:      []domain.SkillTag{domain.SkillSummarization, domain.SkillCustomerComms},
			archetype:   domain.ArchetypeScribe,
			count:       1,
		},

		// 5 journeymen (level 6-10) — mid-tier workers with complementary coverage
		{
			namePattern: "blade-runner",
			level:       9,
			skills:      []domain.SkillTag{domain.SkillCodeGen},
			archetype:   domain.ArchetypeEngineer,
			count:       1,
		},
		{
			namePattern: "circuit-breaker",
			level:       8,
			skills:      []domain.SkillTag{domain.SkillCodeGen, domain.SkillDataTransform},
			archetype:   domain.ArchetypeEngineer,
			count:       1,
		},
		{
			namePattern: "deep-diver",
			level:       8,
			skills:      []domain.SkillTag{domain.SkillResearch},
			archetype:   domain.ArchetypeScholar,
			count:       1,
		},
		{
			namePattern: "wire-tap",
			level:       7,
			skills:      []domain.SkillTag{domain.SkillAnalysis},
			archetype:   domain.ArchetypeScholar,
			count:       1,
		},
		{
			namePattern: "scroll-keeper",
			level:       7,
			skills:      []domain.SkillTag{domain.SkillSummarization},
			archetype:   domain.ArchetypeScribe,
			count:       1,
		},

		// 9 apprentices (level 1-5) — bottom-heavy pyramid; earn their way up through quest work
		{
			namePattern: "flint",
			level:       4,
			skills:      []domain.SkillTag{domain.SkillPlanning},
			archetype:   domain.ArchetypeStrategist,
			count:       1,
		},
		{
			namePattern: "spark-{n}",
			level:       3,
			skills:      []domain.SkillTag{domain.SkillCodeGen},
			archetype:   domain.ArchetypeEngineer,
			count:       3,
		},
		{
			namePattern: "ember-{n}",
			level:       2,
			skills:      []domain.SkillTag{domain.SkillResearch},
			archetype:   domain.ArchetypeScholar,
			count:       3,
		},
		{
			namePattern: "echo-{n}",
			level:       2,
			skills:      []domain.SkillTag{domain.SkillSummarization},
			archetype:   domain.ArchetypeScribe,
			count:       2,
		},
	}

	for _, spec := range specs {
		for i := 0; i < spec.count; i++ {
			name := resolveAgentName(spec.namePattern, i+1)
			if err := seedOneAgent(ctx, graph, boardCfg, name, spec, i); err != nil {
				return fmt.Errorf("agent %q: %w", name, err)
			}
		}
	}

	return nil
}

// resolveAgentName replaces {n} placeholder or returns the pattern unchanged
// when count == 1.
func resolveAgentName(pattern string, n int) string {
	for i, ch := range pattern {
		if ch == '{' && i+3 <= len(pattern) && pattern[i:i+3] == "{n}" {
			return pattern[:i] + fmt.Sprintf("%d", n) + pattern[i+3:]
		}
	}
	return pattern
}

// seedOneAgent writes a single agent entity to the KV bucket.
func seedOneAgent(
	ctx context.Context,
	graph *semdragons.GraphClient,
	boardCfg *domain.BoardConfig,
	name string,
	spec agentSpec,
	_ int, // index reserved for future use (e.g., trait variation)
) error {
	now := time.Now()
	instance := domain.GenerateInstance()
	agentEntityID := boardCfg.AgentEntityID(instance)

	tier := domain.TierFromLevel(spec.level)

	// Compute XP at the midpoint of the current level so the agent isn't
	// sitting at exactly zero XP within their level.
	currentXP := xpAtMidLevel(spec.level)
	xpToNext := xpForLevel(spec.level + 1)

	// Build skill proficiencies — proficiency level roughly tracks tier.
	profLevel := proficiencyForTier(tier)
	skillProfs := make(map[domain.SkillTag]domain.SkillProficiency, len(spec.skills))
	for _, skill := range spec.skills {
		skillProfs[skill] = domain.SkillProficiency{
			Level:      profLevel,
			Progress:   50,
			TotalXP:    currentXP / int64(len(spec.skills)+1),
			QuestsUsed: spec.level * 3,
		}
	}

	// Derive archetype from spec or fall back to the agent's primary skill.
	archetype := spec.archetype
	if archetype == "" && len(spec.skills) > 0 {
		archetype = domain.ArchetypeForSkill(spec.skills[0])
	}

	agent := &agentprogression.Agent{
		ID:                 domain.AgentID(agentEntityID),
		Name:               name,
		DisplayName:        name,
		Status:             domain.AgentIdle,
		Level:              spec.level,
		XP:                 currentXP,
		XPToLevel:          xpToNext,
		Tier:               tier,
		Archetype:          archetype,
		IsNPC:              spec.isNPC,
		Guild:              "",
		SkillProficiencies: skillProfs,
		Stats: agentprogression.AgentStats{
			QuestsCompleted: spec.level * 5,
			QuestsFailed:    spec.level,
			TotalXPEarned:   currentXP * 3,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := graph.EmitEntity(ctx, agent, "agent.seeded"); err != nil {
		return err
	}

	slog.Info("Seeded agent",
		"id", agent.ID,
		"name", agent.Name,
		"level", agent.Level,
		"tier", tier.String())

	return nil
}

// xpAtMidLevel returns XP at ~50% progress through the current level.
// The agent's XP field represents progress within the current level
// (0 to xpForLevel(level+1)), not cumulative XP across all levels.
func xpAtMidLevel(level int) int64 {
	if level <= 1 {
		return 0
	}
	return xpForLevel(level+1) / 2
}

// xpForLevel returns XP required to advance from level to level+1.
// Matches the DefaultXPEngine formula: 100 * 1.5^(level-1).
func xpForLevel(level int) int64 {
	return int64(100 * math.Pow(1.5, float64(level-1)))
}

// seedStore writes the default store catalog to KV for UI visibility.
// The agentstore component also loads DefaultCatalog() into its in-memory sync.Map,
// but writing to KV lets the world state API and dashboard display store items.
func seedStore(ctx context.Context, graph *semdragons.GraphClient) error {
	catalog := agentstore.DefaultCatalog()

	for _, item := range catalog {
		if err := graph.PutEntityState(ctx, &item, "store.item.seeded"); err != nil {
			return fmt.Errorf("seed store item %q: %w", item.Name, err)
		}

		slog.Info("Seeded store item",
			"id", item.ID,
			"name", item.Name,
			"type", item.ItemType,
			"xp_cost", item.XPCost,
			"guild_discount", item.GuildDiscount)
	}

	return nil
}

// proficiencyForTier maps a trust tier to a reasonable starting proficiency.
func proficiencyForTier(tier domain.TrustTier) domain.ProficiencyLevel {
	switch tier {
	case domain.TierApprentice:
		return domain.ProficiencyNovice
	case domain.TierJourneyman:
		return domain.ProficiencyApprentice
	case domain.TierExpert:
		return domain.ProficiencyJourneyman
	case domain.TierMaster:
		return domain.ProficiencyExpert
	default: // Grandmaster
		return domain.ProficiencyMaster
	}
}
