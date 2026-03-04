// Command gen-entity-schema extracts predicates from Graphable.Triples()
// implementations and outputs a JSON schema for TypeScript contract validation.
//
// Usage:
//
//	go run ./cmd/gen-entity-schema > ui/src/lib/services/entity-schema.generated.json
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/bossbattle"
	"github.com/c360studio/semdragons/processor/partycoord"
	"github.com/c360studio/semstreams/message"
)

// Marker values for detecting dynamic predicate segments.
const (
	markerSkillA = "_marker_skill_a_"
	markerSkillB = "_marker_skill_b_"
	markerAgentA = "_marker_agent_a_"
	markerAgentB = "_marker_agent_b_"
	markerQuestA = "_marker_quest_a_"
	markerQuestB = "_marker_quest_b_"
)

// EntitySchema categorizes predicates for a single entity type.
type EntitySchema struct {
	Static     []string `json:"static"`
	Dynamic    []string `json:"dynamic"`
	Optional   []string `json:"optional"`
	MultiValue []string `json:"multi_value"`
}

// Schema is the top-level output structure.
type Schema struct {
	GeneratedAt string                  `json:"generated_at"`
	Entities    map[string]EntitySchema `json:"entities"`
}

func main() {
	schema := Schema{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Entities: map[string]EntitySchema{
			"agent":  extractAgent(),
			"quest":  extractQuest(),
			"battle": extractBattle(),
			"party":  extractParty(),
			"guild":  extractGuild(),
		},
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(schema); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode schema: %v\n", err)
		os.Exit(1)
	}
}

// predicateSet extracts the set of predicate strings from a slice of triples.
func predicateSet(triples []message.Triple) map[string]bool {
	set := make(map[string]bool, len(triples))
	for _, t := range triples {
		set[t.Predicate] = true
	}
	return set
}

// normalizePredicate replaces marker values with template placeholders.
// Skills use {tag}, entity references use {id}.
func normalizePredicate(pred string) string {
	for _, m := range []string{markerSkillA, markerSkillB} {
		if strings.Contains(pred, m) {
			return strings.Replace(pred, m, "{tag}", 1)
		}
	}
	for _, m := range []string{markerAgentA, markerAgentB, markerQuestA, markerQuestB} {
		if strings.Contains(pred, m) {
			return strings.Replace(pred, m, "{id}", 1)
		}
	}
	return pred
}

// classify compares minimal and full triple sets to categorize predicates.
func classify(minimal, full []message.Triple) EntitySchema {
	minSet := predicateSet(minimal)
	fullSet := predicateSet(full)

	// Count occurrences to detect multi-value predicates.
	fullCounts := make(map[string]int)
	for _, t := range full {
		fullCounts[t.Predicate]++
	}

	var static, dynamic, optional, multiValue []string

	// Collect all unique normalized predicates.
	seen := make(map[string]bool)
	for pred := range fullSet {
		norm := normalizePredicate(pred)
		if seen[norm] {
			continue
		}
		seen[norm] = true

		isDynamic := norm != pred // Contains a template placeholder.

		if isDynamic {
			dynamic = append(dynamic, norm)
		} else if minSet[pred] {
			// Present in both minimal and full → always emitted.
			if fullCounts[pred] > 1 {
				multiValue = append(multiValue, pred)
			} else {
				static = append(static, pred)
			}
		} else {
			// Only present in full → optional or multi-value from populated slices.
			if fullCounts[pred] > 1 {
				multiValue = append(multiValue, pred)
			} else {
				optional = append(optional, pred)
			}
		}
	}

	sort.Strings(static)
	sort.Strings(dynamic)
	sort.Strings(optional)
	sort.Strings(multiValue)

	// Ensure empty slices serialize as [] not null.
	if static == nil {
		static = []string{}
	}
	if dynamic == nil {
		dynamic = []string{}
	}
	if optional == nil {
		optional = []string{}
	}
	if multiValue == nil {
		multiValue = []string{}
	}

	return EntitySchema{
		Static:     static,
		Dynamic:    dynamic,
		Optional:   optional,
		MultiValue: multiValue,
	}
}

// =============================================================================
// Entity constructors: minimal (only required fields) and full (all fields set)
// =============================================================================

func extractAgent() EntitySchema {
	now := time.Now()
	cooldown := now.Add(time.Hour)
	questID := domain.QuestID("test-quest")
	partyID := domain.PartyID("test-party")

	minimal := &agentprogression.Agent{
		ID:                 "test-agent",
		Name:               "test",
		DisplayName:        "Test",
		Status:             domain.AgentIdle,
		Level:              1,
		XP:                 0,
		XPToLevel:          100,
		DeathCount:         0,
		Tier:               domain.TierApprentice,
		Guilds:             nil,
		SkillProficiencies: nil,
		Stats:              agentprogression.AgentStats{},
		IsNPC:              false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	full := &agentprogression.Agent{
		ID:          "test-agent",
		Name:        "test",
		DisplayName: "Test Full",
		Status:      domain.AgentOnQuest,
		Level:       10,
		XP:          5000,
		XPToLevel:   8000,
		DeathCount:  2,
		Tier:        domain.TierJourneyman,
		Guilds:      []domain.GuildID{"guild-1", "guild-2"},
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillTag(markerSkillA): {Level: 3, TotalXP: 5000},
			domain.SkillTag(markerSkillB): {Level: 2, TotalXP: 2000},
		},
		CurrentQuest:  &questID,
		CurrentParty:  &partyID,
		CooldownUntil: &cooldown,
		Stats: agentprogression.AgentStats{
			QuestsCompleted: 10,
			QuestsFailed:    2,
			BossesDefeated:  8,
			TotalXPEarned:   50000,
		},
		IsNPC:     true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	return classify(minimal.Triples(), full.Triples())
}

func extractQuest() EntitySchema {
	now := time.Now()
	claimedAt := now.Add(time.Minute)
	startedAt := now.Add(2 * time.Minute)
	completedAt := now.Add(time.Hour)
	agentID := domain.AgentID("test-agent")
	partyID := domain.PartyID("test-party")
	guildID := domain.GuildID("test-guild")
	parentID := domain.QuestID("parent-quest")

	minimal := &domain.Quest{
		ID:          "test-quest",
		Title:       "test",
		Description: "test",
		Status:      domain.QuestPosted,
		Difficulty:  domain.DifficultyEasy,
		MinTier:     domain.TierApprentice,
		BaseXP:      100,
		PostedAt:    now,
		Attempts:    0,
		MaxAttempts: 3,
		Constraints: domain.QuestConstraints{ReviewLevel: domain.ReviewStandard},
	}

	full := &domain.Quest{
		ID:             "test-quest",
		Title:          "Full Quest",
		Description:    "A fully-populated quest",
		Status:         domain.QuestInProgress,
		Difficulty:     domain.DifficultyEpic,
		RequiredSkills: []domain.SkillTag{domain.SkillAnalysis, domain.SkillCodeGen},
		RequiredTools:  []string{"tool-a", "tool-b"},
		MinTier:        domain.TierExpert,
		BaseXP:         500,
		ClaimedBy:      &agentID,
		PartyID:        &partyID,
		GuildPriority:  &guildID,
		ParentQuest:    &parentID,
		PostedAt:       now,
		ClaimedAt:      &claimedAt,
		StartedAt:      &startedAt,
		CompletedAt:    &completedAt,
		Attempts:       1,
		MaxAttempts:    5,
		TrajectoryID:   "traj-123",
		Constraints:    domain.QuestConstraints{ReviewLevel: domain.ReviewStrict},
	}

	return classify(minimal.Triples(), full.Triples())
}

func extractBattle() EntitySchema {
	now := time.Now()
	completedAt := now.Add(time.Hour)

	minimal := &bossbattle.BossBattle{
		ID:        "test-battle",
		QuestID:   "test-quest",
		AgentID:   "test-agent",
		Status:    domain.BattleActive,
		Level:     domain.ReviewStandard,
		StartedAt: now,
	}

	full := &bossbattle.BossBattle{
		ID:          "test-battle",
		QuestID:     "test-quest",
		AgentID:     "test-agent",
		Status:      domain.BattleVictory,
		Level:       domain.ReviewStrict,
		StartedAt:   now,
		CompletedAt: &completedAt,
		Verdict: &domain.BattleVerdict{
			Passed:       true,
			QualityScore: 0.95,
			XPAwarded:    500,
			Feedback:     "Great work",
		},
		Judges: []domain.Judge{
			{ID: "judge-1", Type: domain.JudgeLLM},
			{ID: "judge-2", Type: domain.JudgeAutomated},
		},
	}

	return classify(minimal.Triples(), full.Triples())
}

func extractParty() EntitySchema {
	now := time.Now()
	disbanded := now.Add(time.Hour)

	minimal := &partycoord.Party{
		ID:       "test-party",
		Name:     "Test Party",
		Status:   domain.PartyForming,
		QuestID:  "test-quest",
		Lead:     "test-agent",
		FormedAt: now,
	}

	full := &partycoord.Party{
		ID:       "test-party",
		Name:     "Full Party",
		Status:   domain.PartyActive,
		QuestID:  "test-quest",
		Lead:     "test-agent",
		Strategy: "balanced",
		Members: []partycoord.PartyMember{
			{AgentID: domain.AgentID(markerAgentA), Role: domain.RoleExecutor},
			{AgentID: domain.AgentID(markerAgentB), Role: domain.RoleReviewer},
		},
		SubQuestMap: map[domain.QuestID]domain.AgentID{
			domain.QuestID(markerQuestA): "agent-a",
			domain.QuestID(markerQuestB): "agent-b",
		},
		FormedAt:    now,
		DisbandedAt: &disbanded,
	}

	return classify(minimal.Triples(), full.Triples())
}

func extractGuild() EntitySchema {
	now := time.Now()

	minimal := &domain.Guild{
		ID:            "test-guild",
		Name:          "Test Guild",
		Description:   "test",
		Status:        domain.GuildActive,
		MaxMembers:    50,
		MinLevel:      1,
		Founded:       now,
		FoundedBy:     "founder",
		Reputation:    0.5,
		QuestsHandled: 10,
		QuestsFailed:  1,
		SuccessRate:   0.9,
		CreatedAt:     now,
	}

	full := &domain.Guild{
		ID:          "test-guild",
		Name:        "Full Guild",
		Description: "A fully-populated guild",
		Status:      domain.GuildActive,
		MaxMembers:  100,
		MinLevel:    5,
		Founded:     now,
		FoundedBy:   "founder",
		Culture:     "excellence",
		Motto:       "ship it",
		Members: []domain.GuildMember{
			{AgentID: domain.AgentID(markerAgentA), Rank: domain.GuildRankMember, Contribution: 100},
			{AgentID: domain.AgentID(markerAgentB), Rank: domain.GuildRankVeteran, Contribution: 500},
		},
		SharedTools:      []string{"tool-1", "tool-2"},
		QuestTypes:       []string{"analysis", "code_review"},
		PreferredClients: []string{"client-a", "client-b"},
		Reputation:       0.95,
		QuestsHandled:    150,
		QuestsFailed:     10,
		SuccessRate:      0.93,
		CreatedAt:        now,
	}

	return classify(minimal.Triples(), full.Triples())
}
