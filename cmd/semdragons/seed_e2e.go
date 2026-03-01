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
)

// maybeSeedE2E checks SEED_E2E and, when "true", writes a diverse set of
// agents and guilds directly into the ENTITY_STATES KV bucket so that the
// UI can display them immediately on startup.
//
// We write directly via GraphClient.EmitEntity (the same path the questboard
// processor uses) rather than going through the seeding component's pub/sub
// path, because other processors that would consume those events may not be
// started yet and the KV writes must be synchronous before the UI connects.
func maybeSeedE2E(ctx context.Context, cfg *config.Config, natsClient *natsclient.Client) error {
	if os.Getenv("SEED_E2E") != "true" {
		return nil
	}

	slog.Info("SEED_E2E=true — seeding E2E test data")

	boardCfg, err := extractBoardConfig(cfg)
	if err != nil {
		return fmt.Errorf("seed e2e: resolve board config: %w", err)
	}

	slog.Info("Seeding into board",
		"org", boardCfg.Org,
		"platform", boardCfg.Platform,
		"board", boardCfg.Board,
		"bucket", boardCfg.BucketName())

	graph := semdragons.NewGraphClient(natsClient, boardCfg)

	// Seed guilds first — agents will reference their IDs.
	dataWranglerID, codeSmithsID, err := seedGuilds(ctx, graph, boardCfg)
	if err != nil {
		return fmt.Errorf("seed e2e: seed guilds: %w", err)
	}

	if err := seedAgents(ctx, graph, boardCfg, dataWranglerID, codeSmithsID); err != nil {
		return fmt.Errorf("seed e2e: seed agents: %w", err)
	}

	if err := seedQuests(ctx, graph, boardCfg); err != nil {
		return fmt.Errorf("seed e2e: seed quests: %w", err)
	}

	if err := seedStore(ctx, graph); err != nil {
		return fmt.Errorf("seed e2e: seed store: %w", err)
	}

	slog.Info("E2E seed data written successfully")
	return nil
}

// extractBoardConfig reads org + platform from the platform config and board
// from the "game" service config, matching what extractPlatformMeta does.
func extractBoardConfig(cfg *config.Config) (*semdragons.BoardConfig, error) {
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

	return &semdragons.BoardConfig{
		Org:      cfg.Platform.Org,
		Platform: platformID,
		Board:    board,
	}, nil
}

// seedGuilds creates the two E2E guilds and returns their full entity IDs.
func seedGuilds(ctx context.Context, graph *semdragons.GraphClient, boardCfg *semdragons.BoardConfig) (dataWranglerID, codeSmithsID semdragons.GuildID, err error) {
	now := time.Now()

	specs := []struct {
		name        string
		description string
		culture     string
		motto       string
		questTypes  []string
		minLevel    int
		idPtr       *semdragons.GuildID
	}{
		{
			name:        "Data Wranglers",
			description: "Specialists in analysis, data transformation, and research tasks",
			culture:     "We turn raw data into actionable insight",
			motto:       "In data we trust",
			questTypes:  []string{"analysis", "data_transformation", "research", "summarization"},
			minLevel:    1,
			idPtr:       &dataWranglerID,
		},
		{
			name:        "Code Smiths",
			description: "Elite developers focused on code generation and code review",
			culture:     "Ship quality code every time",
			motto:       "Forged in pull requests",
			questTypes:  []string{"code_generation", "code_review"},
			minLevel:    3,
			idPtr:       &codeSmithsID,
		},
	}

	for _, spec := range specs {
		instance := domain.GenerateInstance()
		guildEntityID := boardCfg.GuildEntityID(instance)

		guild := &semdragons.Guild{
			ID:          semdragons.GuildID(guildEntityID),
			Name:        spec.name,
			Description: spec.description,
			Status:      semdragons.GuildActive,
			MaxMembers:  50,
			MinLevel:    spec.minLevel,
			Founded:     now,
			FoundedBy:   semdragons.AgentID("system"),
			Culture:     spec.culture,
			Motto:       spec.motto,
			Reputation:  0.8,
			SuccessRate: 0.85,
			QuestTypes:  spec.questTypes,
			CreatedAt:   now,
		}

		if err := graph.EmitEntity(ctx, guild, "guild.seeded"); err != nil {
			return "", "", fmt.Errorf("create guild %q: %w", spec.name, err)
		}

		*spec.idPtr = guild.ID

		slog.Info("Seeded guild",
			"id", guild.ID,
			"name", guild.Name)
	}

	return dataWranglerID, codeSmithsID, nil
}

// agentSpec describes one agent (or a batch sharing the same profile) to seed.
type agentSpec struct {
	namePattern string           // e.g. "apprentice-{n}" or a fixed name
	level       int              // base level assigned
	skills      []domain.SkillTag
	guildID     semdragons.GuildID
	isNPC       bool
	count       int // how many to create (1 = single named agent)
}

// seedAgents creates all E2E agents at their target levels.
func seedAgents(
	ctx context.Context,
	graph *semdragons.GraphClient,
	boardCfg *semdragons.BoardConfig,
	dataWranglerID, codeSmithsID semdragons.GuildID,
) error {
	specs := []agentSpec{
		// 3 apprentices (level 1-5)
		{
			namePattern: "apprentice-{n}",
			level:       2,
			skills:      []domain.SkillTag{domain.SkillSummarization},
			guildID:     dataWranglerID,
			count:       2,
		},
		{
			namePattern: "rookie-coder",
			level:       4,
			skills:      []domain.SkillTag{domain.SkillCodeGen},
			guildID:     codeSmithsID,
			count:       1,
		},

		// 2 journeymen (level 6-10)
		{
			namePattern: "analyst-{n}",
			level:       8,
			skills:      []domain.SkillTag{domain.SkillAnalysis, domain.SkillResearch},
			guildID:     dataWranglerID,
			count:       1,
		},
		{
			namePattern: "coder-journeyman",
			level:       9,
			skills:      []domain.SkillTag{domain.SkillCodeGen, domain.SkillCodeReview},
			guildID:     codeSmithsID,
			count:       1,
		},

		// 2 experts (level 11-15)
		{
			namePattern: "data-expert",
			level:       13,
			skills:      []domain.SkillTag{domain.SkillAnalysis, domain.SkillDataTransform, domain.SkillPlanning},
			guildID:     dataWranglerID,
			count:       1,
		},
		{
			namePattern: "senior-dev",
			level:       14,
			skills:      []domain.SkillTag{domain.SkillCodeGen, domain.SkillCodeReview, domain.SkillPlanning},
			guildID:     codeSmithsID,
			count:       1,
		},

		// 1 master (level 16-18)
		{
			namePattern: "archmage",
			level:       17,
			skills:      []domain.SkillTag{domain.SkillAnalysis, domain.SkillPlanning, domain.SkillResearch, domain.SkillTraining},
			guildID:     dataWranglerID,
			count:       1,
		},

		// 1 grandmaster (level 19-20)
		{
			namePattern: "grandmaster",
			level:       20,
			skills:      []domain.SkillTag{domain.SkillCodeGen, domain.SkillCodeReview, domain.SkillAnalysis, domain.SkillPlanning},
			guildID:     codeSmithsID,
			count:       1,
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
	boardCfg *semdragons.BoardConfig,
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
	skillProfs := make(map[semdragons.SkillTag]semdragons.SkillProficiency, len(spec.skills))
	for _, skill := range spec.skills {
		skillProfs[skill] = semdragons.SkillProficiency{
			Level:      profLevel,
			Progress:   50,
			TotalXP:    currentXP / int64(len(spec.skills)+1),
			QuestsUsed: spec.level * 3,
		}
	}

	guilds := []semdragons.GuildID{}
	if spec.guildID != "" {
		guilds = append(guilds, spec.guildID)
	}

	agent := &semdragons.Agent{
		ID:                 semdragons.AgentID(agentEntityID),
		Name:               name,
		DisplayName:        name,
		Status:             semdragons.AgentIdle,
		Level:              spec.level,
		XP:                 currentXP,
		XPToLevel:          xpToNext,
		Tier:               tier,
		IsNPC:              spec.isNPC,
		Guilds:             guilds,
		SkillProficiencies: skillProfs,
		Stats: semdragons.AgentStats{
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

// seedQuests creates a few quests in various states for E2E lifecycle testing.
func seedQuests(ctx context.Context, graph *semdragons.GraphClient, boardCfg *semdragons.BoardConfig) error {
	now := time.Now()

	quests := []*semdragons.Quest{
		{
			ID:          semdragons.QuestID(boardCfg.QuestEntityID("e2e-easy")),
			Title:       "E2E Easy Quest",
			Description: "A simple quest for lifecycle testing",
			Status:      semdragons.QuestPosted,
			Difficulty:  semdragons.DifficultyEasy,
			BaseXP:      100,
			MaxAttempts: 3,
			PostedAt:    now,
		},
		{
			ID:          semdragons.QuestID(boardCfg.QuestEntityID("e2e-review")),
			Title:       "E2E Review Quest",
			Description: "A quest that requires boss battle review",
			Status:      semdragons.QuestPosted,
			Difficulty:  semdragons.DifficultyModerate,
			BaseXP:      200,
			MaxAttempts: 3,
			PostedAt:    now,
			Constraints: semdragons.QuestConstraints{
				RequireReview: true,
				ReviewLevel:   semdragons.ReviewStandard,
			},
		},
		{
			ID:          semdragons.QuestID(boardCfg.QuestEntityID("e2e-hard")),
			Title:       "E2E Hard Quest",
			Description: "A hard quest requiring expert tier",
			Status:      semdragons.QuestPosted,
			Difficulty:  semdragons.DifficultyHard,
			BaseXP:      500,
			MaxAttempts: 2,
			PostedAt:    now,
			RequiredSkills: []semdragons.SkillTag{
				semdragons.SkillCodeGen,
			},
		},
	}

	for _, quest := range quests {
		if err := graph.EmitEntity(ctx, quest, "quest.seeded"); err != nil {
			return fmt.Errorf("create quest %q: %w", quest.Title, err)
		}

		slog.Info("Seeded quest",
			"id", quest.ID,
			"title", quest.Title,
			"difficulty", quest.Difficulty,
			"require_review", quest.Constraints.RequireReview)
	}

	return nil
}

// xpAtMidLevel computes XP at the midpoint of the given level.
// Uses the same exponential curve as the XP engine: 100 * 1.5^(level-1).
func xpAtMidLevel(level int) int64 {
	if level <= 1 {
		return 0
	}
	// XP needed to reach this level (sum of all prior level thresholds).
	var total int64
	for l := 2; l <= level; l++ {
		total += xpForLevel(l)
	}
	// Sit at ~50% through the current level.
	return total + xpForLevel(level+1)/2
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
