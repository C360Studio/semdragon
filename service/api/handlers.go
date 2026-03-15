package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/retry"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/bossbattle"
	"github.com/c360studio/semdragons/processor/partycoord"
)

const maxRequestBodySize = 1 << 20 // 1 MB

// errStoreUnavailable is returned inside retry closures when the agent_store
// component is mid-restart (nil). This lets retry.Quick re-attempt rather
// than panicking on a nil pointer dereference.
var errStoreUnavailable = errors.New("store component unavailable")

// isBucketNotFound returns true if the error indicates the KV bucket doesn't exist yet.
// This is normal before components have started and created the entity states bucket.
// Uses errors.Is first for proper sentinel matching, with string fallback for wrapped errors.
func isBucketNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		return true
	}
	return strings.Contains(err.Error(), "bucket not found")
}

// isKeyNotFound returns true if the error indicates a KV key does not exist.
// Uses errors.Is first for proper sentinel matching, with string fallback for wrapped errors.
func isKeyNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return true
	}
	return strings.Contains(err.Error(), "key not found")
}

// isValidPathID returns true if the id is safe to use as a KV lookup key component.
// Empty strings, dots, and slashes are rejected: dots collide with the dotted entity-ID
// notation used in NATS KV keys, and slashes would escape the URL path segment.
func isValidPathID(id string) bool {
	return id != "" && !strings.Contains(id, ".") && !strings.Contains(id, "/")
}

// isValidSessionID checks that the session ID is a reasonable hex string.
// Session IDs are generated from TraceIDs (32 hex chars), so we accept
// hex characters only, with a generous length limit.
func isValidSessionID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// =============================================================================
// WORLD STATE
// =============================================================================

func (s *Service) handleWorldState(w http.ResponseWriter, r *http.Request) {
	state, err := s.world.WorldState(r.Context())
	if err != nil {
		s.writeError(w, "failed to load world state", http.StatusInternalServerError)
		s.logger.Error("Failed to load world state", "error", err)
		return
	}

	// Merge token budget stats into WorldStats.
	if s.tokenLedger != nil {
		ts := s.tokenLedger.Stats()
		state.Stats.TokensUsedHourly = ts.HourlyUsage.TotalTokens
		state.Stats.TokensLimitHourly = ts.HourlyLimit
		state.Stats.TokenBudgetPct = ts.BudgetPct
		state.Stats.TokenBreaker = ts.Breaker
		state.Stats.CostUsedHourlyUSD = ts.HourlyCostUSD
		state.Stats.CostTotalUSD = ts.TotalCostUSD
	}

	s.writeJSON(w, state)
}

// =============================================================================
// QUESTS
// =============================================================================

func (s *Service) handleListQuests(w http.ResponseWriter, r *http.Request) {
	entities, err := s.graph.ListQuestsByPrefix(r.Context(), s.config.MaxEntities)
	if err != nil {
		if isBucketNotFound(err) {
			s.writeJSON(w, []domain.Quest{})
			return
		}
		s.writeError(w, "failed to list quests", http.StatusInternalServerError)
		s.logger.Error("Failed to list quests", "error", err)
		return
	}

	// Apply optional filters
	statusFilter := r.URL.Query().Get("status")
	difficultyFilter := r.URL.Query().Get("difficulty")
	guildFilter := r.URL.Query().Get("guild_id")

	var quests []domain.Quest
	for _, entity := range entities {
		quest := domain.QuestFromEntityState(&entity)
		if quest == nil {
			continue
		}

		if statusFilter != "" && string(quest.Status) != statusFilter {
			continue
		}
		if difficultyFilter != "" {
			d, err := strconv.Atoi(difficultyFilter)
			if err == nil && int(quest.Difficulty) != d {
				continue
			}
		}
		if guildFilter != "" && (quest.GuildPriority == nil || string(*quest.GuildPriority) != guildFilter) {
			continue
		}

		quests = append(quests, *quest)
	}

	if quests == nil {
		quests = []domain.Quest{}
	}
	s.writeJSON(w, quests)
}

func (s *Service) handleGetQuest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	entity, err := s.graph.GetQuest(r.Context(), domain.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		s.logger.Error("Failed to get quest", "id", id, "error", err)
		return
	}

	quest := domain.QuestFromEntityState(entity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	s.writeJSON(w, quest)
}

func (s *Service) handleCreateQuest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req struct {
		Objective string `json:"objective"`
		Hints     *struct {
			SuggestedDifficulty *int     `json:"suggested_difficulty,omitempty"`
			SuggestedSkills     []string `json:"suggested_skills,omitempty"`
			PreferGuild         *string  `json:"prefer_guild,omitempty"`
			RequireHumanReview  bool     `json:"require_human_review"`
			ReviewLevel         *int     `json:"review_level,omitempty"`
			Budget              float64  `json:"budget"`
			Deadline            string   `json:"deadline,omitempty"`
			PartyRequired       bool     `json:"party_required"`
			MinPartySize        *int     `json:"min_party_size,omitempty"`
		} `json:"hints,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Objective == "" {
		s.writeError(w, "objective is required", http.StatusBadRequest)
		return
	}

	// Build quest
	instance := domain.GenerateShortInstance()
	questID := s.graph.Config().QuestEntityID(instance)

	quest := &domain.Quest{
		ID:          domain.QuestID(questID),
		Name:        truncateName(req.Objective, 25),
		Title:       req.Objective,
		Description: req.Objective,
		Goal:        req.Objective,
		Status:      domain.QuestPosted,
		Difficulty:  domain.DifficultyModerate,
		BaseXP:      100,
		MaxAttempts:          3,
		DecomposabilityClass: domain.DecomposableTrivial,
	}

	if req.Hints != nil {
		if req.Hints.SuggestedDifficulty != nil {
			d := *req.Hints.SuggestedDifficulty
			// DifficultyTrivial=0 through DifficultyLegendary=5 (iota-based constants)
			if d < int(domain.DifficultyTrivial) || d > int(domain.DifficultyLegendary) {
				s.writeError(w, "difficulty must be between 0 and 5", http.StatusBadRequest)
				return
			}
			quest.Difficulty = domain.QuestDifficulty(d)
		}
		for _, skill := range req.Hints.SuggestedSkills {
			quest.RequiredSkills = append(quest.RequiredSkills, domain.SkillTag(skill))
		}
		if req.Hints.PreferGuild != nil {
			gid := domain.GuildID(*req.Hints.PreferGuild)
			quest.GuildPriority = &gid
		}
		if req.Hints.RequireHumanReview {
			quest.Constraints.RequireReview = true
			if req.Hints.ReviewLevel != nil {
				quest.Constraints.ReviewLevel = domain.ReviewLevel(*req.Hints.ReviewLevel)
			} else {
				quest.Constraints.ReviewLevel = domain.ReviewStandard
			}
		}
		if req.Hints.PartyRequired {
			quest.PartyRequired = true
			quest.MinPartySize = 2 // default
			if req.Hints.MinPartySize != nil && *req.Hints.MinPartySize >= 2 && *req.Hints.MinPartySize <= 5 {
				quest.MinPartySize = *req.Hints.MinPartySize
			}
		}
	}

	s.upgradeToPartyIfNeeded(r.Context(), quest)

	if err := s.graph.EmitEntity(r.Context(), quest, "quest.posted"); err != nil {
		s.writeError(w, "failed to create quest", http.StatusInternalServerError)
		s.logger.Error("Failed to create quest", "error", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(quest)
}

func (s *Service) handlePostQuestChain(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var chain domain.QuestChainBrief
	if err := json.NewDecoder(r.Body).Decode(&chain); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := domain.ValidateQuestChainBrief(&chain); err != nil {
		s.writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	now := time.Now()

	// First pass: post each quest (no DependsOn yet — we need real IDs first)
	posted := make([]domain.Quest, 0, len(chain.Quests))
	for _, entry := range chain.Quests {
		instance := domain.GenerateShortInstance()
		questID := s.graph.Config().QuestEntityID(instance)

		difficulty := domain.DifficultyModerate
		if entry.Difficulty != nil {
			difficulty = *entry.Difficulty
		}

		name := entry.Name
		if name == "" {
			name = truncateName(entry.Title, 25)
		}

		quest := domain.Quest{
			ID:          domain.QuestID(questID),
			Name:        name,
			Title:       entry.Title,
			Description: entry.Goal,
			Status:      domain.QuestPosted,
			Difficulty:  difficulty,
			BaseXP:      domain.DefaultXPForDifficulty(difficulty),
			MinTier:     domain.TierFromDifficulty(difficulty),
			MaxAttempts: 3,
			PostedAt:    now,
			Acceptance:  entry.Requirements,
		}

		quest.RequiredSkills = append(quest.RequiredSkills, entry.Skills...)

		// Populate structured spec fields from brief.
		quest.Goal = entry.Goal
		quest.Requirements = entry.Requirements
		quest.Scenarios = entry.Scenarios

		// Classify decomposability from scenarios.
		brief := &domain.QuestBrief{
			Title:     entry.Title,
			Goal:      entry.Goal,
			Scenarios: entry.Scenarios,
		}
		quest.DecomposabilityClass = domain.ClassifyDecomposability(brief)

		if entry.Hints != nil {
			if entry.Hints.RequireHumanReview {
				quest.Constraints.RequireReview = true
				quest.Constraints.ReviewLevel = domain.ReviewStandard
			}
			if entry.Hints.ReviewLevel != nil {
				quest.Constraints.ReviewLevel = *entry.Hints.ReviewLevel
			}
			if entry.Hints.PreferGuild != nil {
				quest.GuildPriority = entry.Hints.PreferGuild
			}
			if entry.Hints.SuggestedDifficulty != nil {
				quest.Difficulty = *entry.Hints.SuggestedDifficulty
				quest.BaseXP = domain.DefaultXPForDifficulty(quest.Difficulty)
				quest.MinTier = domain.TierFromDifficulty(quest.Difficulty)
			}
			for _, skill := range entry.Hints.SuggestedSkills {
				quest.RequiredSkills = append(quest.RequiredSkills, skill)
			}
			if entry.Hints.PartyRequired {
				quest.PartyRequired = true
				quest.MinPartySize = 2 // default
				if entry.Hints.MinPartySize != nil && *entry.Hints.MinPartySize >= 2 && *entry.Hints.MinPartySize <= 5 {
					quest.MinPartySize = *entry.Hints.MinPartySize
				}
			}
		}

		// Set PartyRequired from classification if not explicitly set via hints.
		if entry.Hints == nil || !entry.Hints.PartyRequired {
			switch quest.DecomposabilityClass {
			case domain.DecomposableParallel, domain.DecomposableMixed:
				quest.PartyRequired = true
				if quest.MinPartySize < 2 {
					quest.MinPartySize = 2
				}
			}
		}

		s.upgradeToPartyIfNeeded(ctx, &quest)

		if err := s.graph.EmitEntity(ctx, &quest, "quest.posted"); err != nil {
			s.writeError(w, "failed to create quest", http.StatusInternalServerError)
			s.logger.Error("Failed to create chain quest", "title", entry.Title, "error", err)
			return
		}

		posted = append(posted, quest)
	}

	// Second pass: resolve index-based DependsOn to real QuestIDs
	for i, entry := range chain.Quests {
		if len(entry.DependsOn) == 0 {
			continue
		}

		deps := make([]domain.QuestID, 0, len(entry.DependsOn))
		for _, idx := range entry.DependsOn {
			deps = append(deps, posted[idx].ID)
		}
		posted[i].DependsOn = deps

		if err := s.graph.EmitEntityUpdate(ctx, &posted[i], "quest.dependencies.set"); err != nil {
			s.writeError(w, "failed to set quest dependencies", http.StatusInternalServerError)
			s.logger.Error("Failed to set chain dependencies", "quest_id", posted[i].ID, "error", err)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(posted)
}

// upgradeToPartyIfNeeded checks the agent roster against the quest's required
// skills. If no single agent can solo all skills (AND logic), upgrades the quest
// to PartyRequired with MinPartySize from greedy set-cover analysis.
// Best-effort: errors are logged, not propagated.
func (s *Service) upgradeToPartyIfNeeded(ctx context.Context, quest *domain.Quest) {
	if quest.PartyRequired || len(quest.RequiredSkills) == 0 {
		return
	}

	entities, err := s.graph.ListAgentsByPrefix(ctx, s.config.MaxEntities)
	if err != nil {
		s.logger.Warn("skill coverage check skipped: failed to load agents", "error", err)
		return
	}

	roster := make([]domain.AgentSkillSet, 0, len(entities))
	for i := range entities {
		agent := agentprogression.AgentFromEntityState(&entities[i])
		if agent == nil {
			continue
		}
		skills := make(map[domain.SkillTag]struct{}, len(agent.SkillProficiencies))
		for tag := range agent.SkillProficiencies {
			skills[tag] = struct{}{}
		}
		roster = append(roster, domain.AgentSkillSet{Skills: skills})
	}

	result := domain.ClassifySkillCoverage(quest.RequiredSkills, roster)
	if result.CanSolo {
		return
	}

	quest.PartyRequired = true
	if result.MinAgents > 0 {
		quest.MinPartySize = max(2, result.MinAgents)
	} else {
		quest.MinPartySize = 2
	}

	s.logger.Info("auto-upgraded quest to party: no agent covers all required skills",
		"quest_title", quest.Title,
		"required_skills", quest.RequiredSkills,
		"min_agents", result.MinAgents,
		"uncovered_skills", result.UncoveredSkills,
	)
}

// =============================================================================
// QUEST LIFECYCLE
// =============================================================================

func (s *Service) handleClaimQuest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid quest ID", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.AgentID == "" {
		s.writeError(w, "agent_id is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Load quest
	questEntity, err := s.graph.GetQuest(ctx, domain.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != domain.QuestPosted {
		s.writeError(w, "quest is not available for claiming (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	// Load agent
	agentEntity, err := s.graph.GetAgent(ctx, domain.AgentID(req.AgentID))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			s.writeError(w, "agent not found", http.StatusNotFound)
			return
		}
		s.writeError(w, "failed to retrieve agent", http.StatusInternalServerError)
		return
	}
	agent := agentprogression.AgentFromEntityState(agentEntity)
	if agent == nil {
		s.writeError(w, "agent not found", http.StatusNotFound)
		return
	}

	// Validate agent state
	if agent.Status != domain.AgentIdle {
		s.writeError(w, "agent is not idle (status: "+string(agent.Status)+")", http.StatusConflict)
		return
	}

	// Validate tier
	minTier := quest.MinTier
	if minTier == 0 {
		minTier = domain.TierFromDifficulty(quest.Difficulty)
	}
	if agent.Tier < minTier {
		s.writeError(w, "agent tier too low for this quest", http.StatusForbidden)
		return
	}

	// Validate skills
	for _, skill := range quest.RequiredSkills {
		if !agent.HasSkill(skill) {
			s.writeError(w, "agent lacks required skill: "+string(skill), http.StatusForbidden)
			return
		}
	}

	// Claim quest
	now := time.Now()
	agentID := agent.ID
	quest.Status = domain.QuestClaimed
	quest.ClaimedBy = &agentID
	quest.ClaimedAt = &now

	if err := s.graph.EmitEntityUpdate(ctx, quest, "quest.claimed"); err != nil {
		s.writeError(w, "failed to claim quest", http.StatusInternalServerError)
		s.logger.Error("Failed to claim quest", "error", err)
		return
	}

	// Update agent status
	questID := quest.ID
	agent.Status = domain.AgentOnQuest
	agent.CurrentQuest = &questID
	agent.UpdatedAt = now

	if err := s.graph.EmitEntityUpdate(ctx, agent, "agent.quest_claimed"); err != nil {
		s.logger.Error("Failed to update agent status after claim", "error", err)
	}

	s.writeJSON(w, quest)
}

func (s *Service) handleStartQuest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid quest ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	questEntity, err := s.graph.GetQuest(ctx, domain.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != domain.QuestClaimed {
		s.writeError(w, "quest must be claimed before starting (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	now := time.Now()
	quest.Status = domain.QuestInProgress
	quest.StartedAt = &now

	if err := s.graph.EmitEntityUpdate(ctx, quest, "quest.started"); err != nil {
		s.writeError(w, "failed to start quest", http.StatusInternalServerError)
		s.logger.Error("Failed to start quest", "error", err)
		return
	}

	s.writeJSON(w, quest)
}

func (s *Service) handleSubmitResult(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid quest ID", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req struct {
		Output string `json:"output"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	questEntity, err := s.graph.GetQuest(ctx, domain.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != domain.QuestInProgress {
		s.writeError(w, "quest must be in progress to submit (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	quest.Output = req.Output

	if quest.Constraints.RequireReview {
		quest.Status = domain.QuestInReview
	} else {
		now := time.Now()
		quest.Status = domain.QuestCompleted
		quest.CompletedAt = &now
	}

	if err := s.graph.EmitEntityUpdate(ctx, quest, "quest.submitted"); err != nil {
		s.writeError(w, "failed to submit quest result", http.StatusInternalServerError)
		s.logger.Error("Failed to submit quest result", "error", err)
		return
	}

	// If quest completed without review, release the agent
	if quest.Status == domain.QuestCompleted && quest.ClaimedBy != nil {
		s.releaseAgent(ctx, *quest.ClaimedBy)
	}

	s.writeJSON(w, quest)
}

func (s *Service) handleCompleteQuest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid quest ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	questEntity, err := s.graph.GetQuest(ctx, domain.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != domain.QuestInReview && quest.Status != domain.QuestInProgress {
		s.writeError(w, "quest must be in_review or in_progress to complete (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	now := time.Now()
	quest.Status = domain.QuestCompleted
	quest.CompletedAt = &now

	if err := s.graph.EmitEntityUpdate(ctx, quest, "quest.completed"); err != nil {
		s.writeError(w, "failed to complete quest", http.StatusInternalServerError)
		s.logger.Error("Failed to complete quest", "error", err)
		return
	}

	if quest.ClaimedBy != nil {
		s.releaseAgent(ctx, *quest.ClaimedBy)
	}

	s.writeJSON(w, quest)
}

func (s *Service) handleFailQuest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid quest ID", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	questEntity, err := s.graph.GetQuest(ctx, domain.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != domain.QuestInProgress && quest.Status != domain.QuestInReview {
		s.writeError(w, "quest must be in_progress or in_review to fail (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	// Release agent before reposting/failing
	if quest.ClaimedBy != nil {
		s.releaseAgent(ctx, *quest.ClaimedBy)
	}

	quest.Attempts++

	if quest.MaxAttempts > 0 && quest.Attempts >= quest.MaxAttempts {
		quest.Status = domain.QuestFailed
	} else {
		// Repost: reset assignment fields for another attempt
		quest.Status = domain.QuestPosted
		quest.ClaimedBy = nil
		quest.ClaimedAt = nil
		quest.StartedAt = nil
		quest.Output = nil
	}

	eventType := "quest.failed"
	if quest.Status == domain.QuestPosted {
		eventType = "quest.reposted"
	}

	if err := s.graph.EmitEntityUpdate(ctx, quest, eventType); err != nil {
		s.writeError(w, "failed to fail quest", http.StatusInternalServerError)
		s.logger.Error("Failed to fail quest", "error", err)
		return
	}

	s.writeJSON(w, quest)
}

func (s *Service) handleAbandonQuest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid quest ID", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req struct {
		Reason string `json:"reason"`
	}
	// Body is optional for abandon
	_ = json.NewDecoder(r.Body).Decode(&req)

	ctx := r.Context()
	questEntity, err := s.graph.GetQuest(ctx, domain.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != domain.QuestClaimed && quest.Status != domain.QuestInProgress {
		s.writeError(w, "quest must be claimed or in_progress to abandon (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	// Release agent
	if quest.ClaimedBy != nil {
		s.releaseAgent(ctx, *quest.ClaimedBy)
	}

	// Return quest to board
	quest.Status = domain.QuestPosted
	quest.ClaimedBy = nil
	quest.ClaimedAt = nil
	quest.StartedAt = nil
	quest.Output = nil

	if err := s.graph.EmitEntityUpdate(ctx, quest, "quest.abandoned"); err != nil {
		s.writeError(w, "failed to abandon quest", http.StatusInternalServerError)
		s.logger.Error("Failed to abandon quest", "error", err)
		return
	}

	s.writeJSON(w, quest)
}

// handleCancelQuest cancels an in-progress quest by sending a cancel signal
// to the active agentic loop and transitioning the quest to failed.
func (s *Service) handleCancelQuest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid quest ID", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Reason == "" {
		req.Reason = "Cancelled by admin"
	}

	ctx := r.Context()
	questEntity, err := s.graph.GetQuest(ctx, domain.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != domain.QuestInProgress {
		s.writeError(w, "quest must be in_progress to cancel (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	// Send cancel signal to the active agentic loop if we can find it.
	s.cancelActiveLoop(ctx, id)

	// If this is a DAG parent quest, cancel all sub-quest loops via questdagexec.
	s.cancelDAGSubQuests(ctx, id)

	// Release agent.
	if quest.ClaimedBy != nil {
		s.releaseAgent(ctx, *quest.ClaimedBy)
	}

	// Transition quest to failed.
	quest.Status = domain.QuestFailed
	quest.FailureReason = req.Reason

	if err := s.graph.EmitEntityUpdate(ctx, quest, "quest.cancelled"); err != nil {
		s.writeError(w, "failed to cancel quest", http.StatusInternalServerError)
		s.logger.Error("Failed to cancel quest", "error", err)
		return
	}

	s.writeJSON(w, quest)
}

// cancelActiveLoop sends a cancel signal to the agentic loop running for a quest.
func (s *Service) cancelActiveLoop(ctx context.Context, questID string) {
	if s.componentDeps == nil || s.componentDeps.ComponentRegistry == nil {
		return
	}

	comp := s.componentDeps.ComponentRegistry.Component("questbridge")
	if comp == nil {
		return
	}

	type activeLoopFinder interface {
		FindActiveLoop(questEntityKey string) (string, bool)
	}

	finder, ok := comp.(activeLoopFinder)
	if !ok {
		return
	}

	// Try the raw ID first, then the full entity key.
	loopID, found := finder.FindActiveLoop(questID)
	if !found {
		entityKey := s.boardConfig.QuestEntityID(questID)
		loopID, found = finder.FindActiveLoop(entityKey)
	}
	if !found {
		s.logger.Debug("no active loop found for quest", "quest_id", questID)
		return
	}

	s.sendCancelSignal(ctx, loopID)
}

// sendCancelSignal publishes a cancel UserSignal to the AGENT stream.
func (s *Service) sendCancelSignal(ctx context.Context, loopID string) {
	js, err := s.nats.JetStream()
	if err != nil {
		s.logger.Warn("sendCancelSignal: failed to get JetStream", "error", err)
		return
	}

	signal := &agentic.UserSignal{
		SignalID:    "cancel-api-" + loopID,
		Type:        agentic.SignalCancel,
		LoopID:      loopID,
		UserID:      "admin",
		ChannelType: "api",
		ChannelID:   "game-api",
		Timestamp:   time.Now(),
	}

	baseMsg := message.NewBaseMessage(signal.Schema(), signal, "game-api")
	data, marshalErr := json.Marshal(baseMsg)
	if marshalErr != nil {
		s.logger.Warn("sendCancelSignal: failed to marshal signal", "error", marshalErr)
		return
	}

	subject := fmt.Sprintf("agent.signal.%s", loopID)
	if _, pubErr := js.Publish(ctx, subject, data); pubErr != nil {
		s.logger.Warn("sendCancelSignal: failed to publish cancel signal",
			"loop_id", loopID, "error", pubErr)
	} else {
		s.logger.Info("sent cancel signal via API", "loop_id", loopID)
	}
}

// cancelDAGSubQuests delegates DAG cleanup to the questdagexec component when
// the quest being cancelled is a DAG parent. The questdagexec component handles
// sub-quest loop cancellation, parent escalation, and party disbanding.
func (s *Service) cancelDAGSubQuests(ctx context.Context, questID string) {
	if s.componentDeps == nil || s.componentDeps.ComponentRegistry == nil {
		return
	}

	comp := s.componentDeps.ComponentRegistry.Component("questdagexec")
	if comp == nil {
		return
	}

	type dagCanceller interface {
		CancelDAGForQuest(ctx context.Context, parentQuestEntityKey string)
	}

	if canceller, ok := comp.(dagCanceller); ok {
		// Use the full entity key since dagCache is keyed by entity key.
		entityKey := s.boardConfig.QuestEntityID(questID)
		canceller.CancelDAGForQuest(ctx, entityKey)
	}
}

// releaseAgent sets an agent back to idle and clears their current quest.
func (s *Service) releaseAgent(ctx context.Context, agentID domain.AgentID) {
	agentEntity, err := s.graph.GetAgent(ctx, agentID)
	if err != nil {
		s.logger.Error("Failed to load agent for release", "agent_id", agentID, "error", err)
		return
	}
	agent := agentprogression.AgentFromEntityState(agentEntity)
	if agent == nil {
		return
	}

	agent.Status = domain.AgentIdle
	agent.CurrentQuest = nil
	agent.UpdatedAt = time.Now()

	if err := s.graph.EmitEntityUpdate(ctx, agent, "agent.released"); err != nil {
		s.logger.Error("Failed to release agent", "agent_id", agentID, "error", err)
	}
}

// =============================================================================
// AGENTS
// =============================================================================

func (s *Service) handleListAgents(w http.ResponseWriter, r *http.Request) {
	entities, err := s.graph.ListAgentsByPrefix(r.Context(), s.config.MaxEntities)
	if err != nil {
		if isBucketNotFound(err) {
			s.writeJSON(w, []agentprogression.Agent{})
			return
		}
		s.writeError(w, "failed to list agents", http.StatusInternalServerError)
		s.logger.Error("Failed to list agents", "error", err)
		return
	}

	agents := make([]agentprogression.Agent, 0, len(entities))
	for _, entity := range entities {
		agent := agentprogression.AgentFromEntityState(&entity)
		if agent != nil {
			agents = append(agents, *agent)
		}
	}

	s.writeJSON(w, agents)
}

func (s *Service) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	entity, err := s.graph.GetAgent(r.Context(), domain.AgentID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve agent", http.StatusInternalServerError)
		s.logger.Error("Failed to get agent", "id", id, "error", err)
		return
	}

	agent := agentprogression.AgentFromEntityState(entity)
	if agent == nil {
		http.NotFound(w, r)
		return
	}

	s.writeJSON(w, agent)
}

func (s *Service) handleRecruitAgent(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req struct {
		Name        string   `json:"name"`
		DisplayName string   `json:"display_name,omitempty"`
		Skills      []string `json:"skills,omitempty"`
		IsNPC       bool     `json:"is_npc"`
		Level       *int     `json:"level,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		s.writeError(w, "name is required", http.StatusBadRequest)
		return
	}

	level := 1
	if req.Level != nil && *req.Level >= 1 && *req.Level <= 20 {
		level = *req.Level
	}

	instance := domain.GenerateShortInstance()
	agentID := s.graph.Config().AgentEntityID(instance)

	// Compute XP at ~50% progress through the current level so agents
	// recruited above level 1 have usable XP (matches seed_e2e formula).
	xpToNext := int64(100 * math.Pow(1.5, float64(level)))
	var startXP int64
	if level > 1 {
		startXP = xpToNext / 2
	}

	agent := &agentprogression.Agent{
		ID:                 domain.AgentID(agentID),
		Name:               req.Name,
		DisplayName:        req.DisplayName,
		Status:             domain.AgentIdle,
		Level:              level,
		XP:                 startXP,
		XPToLevel:          xpToNext,
		Tier:               domain.TierFromLevel(level),
		IsNPC:              req.IsNPC,
		SkillProficiencies: make(map[domain.SkillTag]domain.SkillProficiency),
	}

	for _, skill := range req.Skills {
		agent.SkillProficiencies[domain.SkillTag(skill)] = domain.SkillProficiency{
			Level: 1,
		}
	}

	if err := s.graph.EmitEntity(r.Context(), agent, "agent.recruited"); err != nil {
		s.writeError(w, "failed to recruit agent", http.StatusInternalServerError)
		s.logger.Error("Failed to recruit agent", "error", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(agent)
}

func (s *Service) handleRetireAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	entity, err := s.graph.GetAgent(r.Context(), domain.AgentID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve agent", http.StatusInternalServerError)
		s.logger.Error("Failed to get agent for retire", "id", id, "error", err)
		return
	}

	agent := agentprogression.AgentFromEntityState(entity)
	if agent == nil {
		http.NotFound(w, r)
		return
	}

	agent.Status = domain.AgentRetired

	if err := s.graph.EmitEntityUpdate(r.Context(), agent, "agent.retired"); err != nil {
		s.writeError(w, "failed to retire agent", http.StatusInternalServerError)
		s.logger.Error("Failed to retire agent", "error", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// BATTLES
// =============================================================================

func (s *Service) handleListBattles(w http.ResponseWriter, r *http.Request) {
	entities, err := s.graph.ListEntitiesByType(r.Context(), domain.EntityTypeBattle, s.config.MaxEntities)
	if err != nil {
		if isBucketNotFound(err) {
			s.writeJSON(w, []bossbattle.BossBattle{})
			return
		}
		s.writeError(w, "failed to list battles", http.StatusInternalServerError)
		s.logger.Error("Failed to list battles", "error", err)
		return
	}

	battles := make([]bossbattle.BossBattle, 0, len(entities))
	for _, entity := range entities {
		battle := bossbattle.BattleFromEntityState(&entity)
		if battle != nil {
			battles = append(battles, *battle)
		}
	}

	s.writeJSON(w, battles)
}

func (s *Service) handleGetBattle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	entity, err := s.graph.GetBattle(r.Context(), domain.BattleID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve battle", http.StatusInternalServerError)
		s.logger.Error("Failed to get battle", "id", id, "error", err)
		return
	}

	battle := bossbattle.BattleFromEntityState(entity)
	if battle == nil {
		http.NotFound(w, r)
		return
	}

	s.writeJSON(w, battle)
}

// =============================================================================
// PARTIES
// =============================================================================

func (s *Service) handleListParties(w http.ResponseWriter, r *http.Request) {
	entities, err := s.graph.ListEntitiesByType(r.Context(), domain.EntityTypeParty, s.config.MaxEntities)
	if err != nil {
		if isBucketNotFound(err) {
			s.writeJSON(w, []partycoord.Party{})
			return
		}
		s.writeError(w, "failed to list parties", http.StatusInternalServerError)
		s.logger.Error("Failed to list parties", "error", err)
		return
	}

	parties := make([]partycoord.Party, 0, len(entities))
	for _, entity := range entities {
		party := partycoord.PartyFromEntityState(&entity)
		if party != nil {
			parties = append(parties, *party)
		}
	}

	s.writeJSON(w, parties)
}

func (s *Service) handleGetParty(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	entity, err := s.graph.GetParty(r.Context(), domain.PartyID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve party", http.StatusInternalServerError)
		s.logger.Error("Failed to get party", "id", id, "error", err)
		return
	}

	party := partycoord.PartyFromEntityState(entity)
	if party == nil {
		http.NotFound(w, r)
		return
	}

	s.writeJSON(w, party)
}

// =============================================================================
// GUILDS
// =============================================================================

func (s *Service) handleListGuilds(w http.ResponseWriter, r *http.Request) {
	entities, err := s.graph.ListEntitiesByType(r.Context(), domain.EntityTypeGuild, s.config.MaxEntities)
	if err != nil {
		if isBucketNotFound(err) {
			s.writeJSON(w, []domain.Guild{})
			return
		}
		s.writeError(w, "failed to list guilds", http.StatusInternalServerError)
		s.logger.Error("Failed to list guilds", "error", err)
		return
	}

	guilds := make([]domain.Guild, 0, len(entities))
	for _, entity := range entities {
		guild := domain.GuildFromEntityState(&entity)
		if guild != nil {
			guilds = append(guilds, *guild)
		}
	}

	s.writeJSON(w, guilds)
}

func (s *Service) handleGetGuild(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	entity, err := s.graph.GetGuild(r.Context(), domain.GuildID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve guild", http.StatusInternalServerError)
		s.logger.Error("Failed to get guild", "id", id, "error", err)
		return
	}

	guild := domain.GuildFromEntityState(entity)
	if guild == nil {
		http.NotFound(w, r)
		return
	}

	s.writeJSON(w, guild)
}

// =============================================================================
// TRAJECTORIES
// =============================================================================

func (s *Service) handleGetTrajectory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		s.writeError(w, "invalid trajectory ID", http.StatusBadRequest)
		return
	}
	// Trajectory IDs (loopIDs) may contain dots, so use hex validation OR path validation.
	if !isValidSessionID(id) && !isValidPathID(id) {
		s.writeError(w, "invalid trajectory ID", http.StatusBadRequest)
		return
	}

	if s.trajectories == nil {
		s.writeError(w, "trajectory service unavailable", http.StatusServiceUnavailable)
		return
	}

	data, err := s.trajectories.GetTrajectory(r.Context(), id)
	if err != nil {
		if isBucketNotFound(err) {
			s.writeError(w, "trajectory service not deployed", http.StatusServiceUnavailable)
			return
		}
		if isKeyNotFound(err) {
			s.writeError(w, "trajectory not found", http.StatusNotFound)
			return
		}
		s.writeError(w, "failed to retrieve trajectory", http.StatusInternalServerError)
		s.logger.Error("Failed to get trajectory", "id", id, "error", err)
		return
	}

	// Default: strip verbose fields (messages, tool_calls) to keep payloads small.
	// Use ?detail=full to include everything for debugging.
	detail := r.URL.Query().Get("detail")
	if detail != "full" {
		data = trimTrajectoryDetail(data)
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// trimTrajectoryDetail strips Messages and ToolCalls from trajectory steps
// to keep the default response lightweight for the browser. Returns the
// original data unchanged if unmarshalling fails.
func trimTrajectoryDetail(data []byte) []byte {
	var traj map[string]any
	if err := json.Unmarshal(data, &traj); err != nil {
		return data
	}
	steps, ok := traj["steps"].([]any)
	if !ok || len(steps) == 0 {
		return data // Nothing to trim — preserve original byte ordering
	}
	modified := false
	for _, step := range steps {
		s, ok := step.(map[string]any)
		if !ok {
			continue
		}
		if _, has := s["messages"]; has {
			delete(s, "messages")
			modified = true
		}
		if _, has := s["tool_calls"]; has {
			delete(s, "tool_calls")
			modified = true
		}
	}
	if !modified {
		return data // Nothing was actually trimmed — preserve original bytes
	}
	trimmed, err := json.Marshal(traj)
	if err != nil {
		return data
	}
	return trimmed
}

// =============================================================================
// DUNGEON MASTER
// =============================================================================

func (s *Service) handleDMChat(w http.ResponseWriter, r *http.Request) {
	// Extend the write deadline for LLM calls, which can take 60-120s.
	// The default server WriteTimeout (10s) is too short for LLM inference.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Now().Add(llmHTTPTimeout + 15*time.Second)); err != nil {
		s.logger.Warn("Failed to extend write deadline for DM chat", "error", err)
	}

	if s.models == nil {
		s.writeError(w, "model registry not configured", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req struct {
		Message   string              `json:"message"`
		Mode      string              `json:"mode,omitempty"`
		Context   []dmChatContextItem `json:"context,omitempty"`
		History   []ChatMessage       `json:"history,omitempty"`
		SessionID string              `json:"session_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		s.writeError(w, "message is required", http.StatusBadRequest)
		return
	}

	if req.SessionID != "" && !isValidSessionID(req.SessionID) {
		s.writeError(w, "invalid session_id", http.StatusBadRequest)
		return
	}

	// Parse and validate chat mode (default: converse)
	chatMode := domain.ChatModeConverse
	if req.Mode != "" {
		chatMode = domain.ChatMode(req.Mode)
		if !domain.ValidChatMode(chatMode) {
			s.writeError(w, "invalid mode: must be converse or quest", http.StatusBadRequest)
			return
		}
	}

	// Route LLM capability by mode
	capability := "dm-chat"
	if chatMode == domain.ChatModeQuest {
		capability = "quest-design"
	}

	endpointName := s.models.Resolve(capability)
	endpoint := s.models.GetEndpoint(endpointName)
	if endpoint == nil {
		s.writeError(w, "no LLM endpoint available for "+capability, http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()

	// --- Trace context for audit trail ---
	// Each DM chat session gets a root trace; each turn is a child span.
	// The trace propagates to graph operations so quests created from
	// chat inherit the conversation's trace for end-to-end auditing.
	sessionID, turnSpan := s.getOrCreateChatTrace(req.SessionID)
	ctx = natsclient.ContextWithTrace(ctx, turnSpan)

	// Build session recap for multi-turn continuity
	var sessionRecap string
	if sessionID != "" && s.dmSessions != nil {
		session, err := s.dmSessions.GetSession(ctx, sessionID)
		if err == nil && session != nil && len(session.Turns) > 0 {
			sessionRecap = buildSessionRecap(session)
		}
	}

	// Build system prompt with world state, context, mode, and session recap
	systemPrompt := s.buildDMSystemPrompt(ctx, chatMode, req.Context, sessionRecap)

	// Build conversation: history + new user message
	messages := make([]ChatMessage, 0, len(req.History)+1)
	messages = append(messages, req.History...)
	messages = append(messages, ChatMessage{Role: "user", Content: req.Message})

	// Circuit breaker: reject if token budget exceeded.
	if s.tokenLedger != nil {
		if err := s.tokenLedger.Check(); err != nil {
			s.writeError(w, "token budget exceeded", http.StatusTooManyRequests)
			return
		}
	}

	// --- Tool-enabled path: use agenticmodel.Client for multi-turn tool loop ---
	var llmResponse string
	var toolsUsed []string
	var totalPromptTokens, totalCompletionTokens int

	if s.dmTools != nil {
		// Extend write deadline for tool loop (multiple LLM round-trips).
		if err := rc.SetWriteDeadline(time.Now().Add(dmToolLoopTimeout)); err != nil {
			s.logger.Warn("Failed to extend write deadline for DM tool loop", "error", err)
		}

		llmClient, clientErr := s.newDMClient(endpoint)
		if clientErr != nil {
			s.logger.Error("Failed to create DM LLM client", "error", clientErr)
			s.writeError(w, "LLM client initialization failed", http.StatusBadGateway)
			return
		}

		toolDefs := s.dmToolDefs()
		agenticMessages := dmConvertMessages(systemPrompt, messages)

		for iteration := range maxDMToolIterations {
			// Check context cancellation (client disconnect).
			if ctx.Err() != nil {
				s.logger.Warn("DM tool loop canceled (client disconnected)",
					"iteration", iteration, "endpoint", endpointName)
				s.writeError(w, "request canceled", http.StatusBadGateway)
				return
			}

			// Token budget check before each iteration.
			if s.tokenLedger != nil {
				if err := s.tokenLedger.Check(); err != nil {
					s.logger.Warn("DM tool loop stopped by token budget", "iteration", iteration)
					break
				}
			}

			agenticReq := agentic.AgentRequest{
				RequestID: fmt.Sprintf("dm-%s-%d", sessionID, iteration),
				Role:      agentic.RoleGeneral,
				Model:     endpoint.Model,
				Messages:  agenticMessages,
				Tools:     toolDefs,
				ToolChoice: &agentic.ToolChoice{Mode: "auto"},
			}

			resp, llmErr := llmClient.ChatCompletion(ctx, agenticReq)
			if llmErr != nil {
				if ctx.Err() != nil {
					s.logger.Warn("DM tool loop LLM call canceled", "error", llmErr, "iteration", iteration)
				} else {
					s.logger.Error("DM tool loop LLM call failed", "error", llmErr, "iteration", iteration)
				}
				s.writeError(w, "LLM request failed", http.StatusBadGateway)
				return
			}

			totalPromptTokens += resp.TokenUsage.PromptTokens
			totalCompletionTokens += resp.TokenUsage.CompletionTokens

			if resp.Status != agentic.StatusToolCall || len(resp.Message.ToolCalls) == 0 {
				// LLM responded with text — we're done.
				llmResponse = resp.Message.Content
				break
			}

			// Append assistant message with tool calls to conversation.
			agenticMessages = append(agenticMessages, resp.Message)

			// Execute each tool call and append results.
			for _, tc := range resp.Message.ToolCalls {
				toolsUsed = append(toolsUsed, tc.Name)
				result, toolErr := s.executeDMTool(ctx, tc)
				if toolErr != nil {
					result = fmt.Sprintf("Tool error: %s", toolErr.Error())
				}

				agenticMessages = append(agenticMessages, agentic.ChatMessage{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
					Name:       tc.Name,
				})
			}

			// Nudge the LLM to wrap up near the iteration limit.
			if iteration == maxDMToolIterations-2 {
				agenticMessages = append(agenticMessages, agentic.ChatMessage{
					Role:    "user",
					Content: "You have 1 tool call remaining. Please provide your final answer now.",
				})
			}
		}

		// If we exhausted iterations without a final text response, use the last content.
		if llmResponse == "" {
			llmResponse = "(DM tool loop completed without a final response)"
		}
	} else {
		// --- Legacy path: single callLLM without tools ---
		llmResult, err := callLLM(ctx, endpoint, systemPrompt, messages)
		if err != nil {
			if ctx.Err() != nil {
				s.logger.Warn("DM chat LLM call canceled (client disconnected)", "error", err,
					"endpoint", endpointName, "trace_id", turnSpan.TraceID, "span_id", turnSpan.SpanID)
			} else {
				s.logger.Error("DM chat LLM call failed", "error", err, "endpoint", endpointName,
					"trace_id", turnSpan.TraceID, "span_id", turnSpan.SpanID)
			}
			s.writeError(w, "LLM request failed", http.StatusBadGateway)
			return
		}
		llmResponse = llmResult.Content
		totalPromptTokens = llmResult.PromptTokens
		totalCompletionTokens = llmResult.CompletionTokens
	}

	// Record token usage.
	if s.tokenLedger != nil {
		source := "dm_chat"
		if len(toolsUsed) > 0 {
			source = "dm_chat_tool"
		}
		s.tokenLedger.Record(ctx, totalPromptTokens, totalCompletionTokens, source, endpointName)
	}

	traceInfo := semdragons.TraceInfoFromTraceContext(turnSpan)
	chatResp := DMChatResponse{
		Message:   llmResponse,
		Mode:      string(chatMode),
		ToolsUsed: toolsUsed,
		SessionID: sessionID,
		TraceInfo: TraceInfoResponse(traceInfo),
	}

	// Extract quest briefs/chains when mode is quest, with retry on failure
	if chatMode == domain.ChatModeQuest {
		brief, chain := extractQuestOutput(llmResponse)
		chatResp.QuestBrief = brief
		chatResp.QuestChain = chain

		// Retry once if quest mode produced no structured output.
		// Append the LLM's response + a nudge message to the conversation
		// so it sees its own output and gets a second chance.
		const maxQuestRetries = 1
		if brief == nil && chain == nil {
			for attempt := 0; attempt < maxQuestRetries; attempt++ {
				s.logger.Warn("Quest mode produced no structured output, retrying",
					"attempt", attempt+1, "endpoint", endpointName)

				retryMessages := make([]ChatMessage, len(messages)+2)
				copy(retryMessages, messages)
				retryMessages[len(messages)] = ChatMessage{Role: "assistant", Content: llmResponse}
				retryMessages[len(messages)+1] = ChatMessage{
					Role:    "user",
					Content: "Your response is missing the required JSON block. You MUST include a ```json:quest_brief code block containing {\"title\", \"goal\", \"requirements\", \"scenarios\", \"difficulty\", \"skills\"}. For multiple quests use ```json:quest_chain instead. Output the JSON now.",
				}

				retryResult, retryErr := callLLM(ctx, endpoint, systemPrompt, retryMessages)
				if retryErr != nil {
					s.logger.Warn("Quest retry LLM call failed", "error", retryErr, "attempt", attempt+1)
					break
				}

				// Record retry token usage.
				if s.tokenLedger != nil {
					s.tokenLedger.Record(ctx, retryResult.PromptTokens, retryResult.CompletionTokens, "dm_chat", endpointName)
				}

				brief, chain = extractQuestOutput(retryResult.Content)
				if brief != nil || chain != nil {
					chatResp.Message = llmResponse + "\n\n" + retryResult.Content
					chatResp.QuestBrief = brief
					chatResp.QuestChain = chain
					llmResponse = chatResp.Message
					break
				}

				s.logger.Warn("Quest retry still produced no structured output",
					"attempt", attempt+1, "endpoint", endpointName)
				llmResponse = llmResponse + "\n\n" + retryResult.Content
				chatResp.Message = llmResponse
			}
		}
	}

	// Best-effort: persist turn to KV. Don't fail the response on KV error.
	if s.dmSessions != nil {
		turn := DMChatTurn{
			UserMessage: req.Message,
			DMResponse:  llmResponse,
			Timestamp:   time.Now(),
			TraceID:     turnSpan.TraceID,
			SpanID:      turnSpan.SpanID,
			ToolsUsed:   toolsUsed,
		}
		if kvErr := s.dmSessions.appendTurn(ctx, sessionID, turn); kvErr != nil {
			s.logger.Warn("Failed to persist DM chat turn", "session_id", sessionID, "error", kvErr)
		}
	}

	s.writeJSON(w, chatResp)
}

// getOrCreateChatTrace returns a session ID and a child span for this turn.
// If sessionID is empty or unknown, a new root trace is created.
func (s *Service) getOrCreateChatTrace(sessionID string) (string, *natsclient.TraceContext) {
	// Try to find an existing session trace
	if sessionID != "" {
		s.chatTracesMu.RLock()
		rootTrace := s.chatTraces[sessionID]
		s.chatTracesMu.RUnlock()

		if rootTrace != nil {
			return sessionID, rootTrace.NewSpan()
		}
	}

	// New session: create root trace as a session-level anchor.
	// The root span is never emitted directly — it serves as
	// the parent for all turn spans in this session.
	rootTrace := natsclient.NewTraceContext()
	sessionID = rootTrace.TraceID

	s.chatTracesMu.Lock()
	if len(s.chatTraces) >= maxChatSessions {
		evictCount := maxChatSessions / 2
		for _, old := range s.chatTracesOrder[:evictCount] {
			delete(s.chatTraces, old)
		}
		s.chatTracesOrder = s.chatTracesOrder[evictCount:]
		s.logger.Warn("DM chat session trace cache evicted oldest entries",
			"evicted", evictCount, "remaining", len(s.chatTraces))
	}
	s.chatTraces[sessionID] = rootTrace
	s.chatTracesOrder = append(s.chatTracesOrder, sessionID)
	s.chatTracesMu.Unlock()

	return sessionID, rootTrace.NewSpan()
}

// extractQuestOutput attempts to parse a quest_brief or quest_chain from an LLM response.
// It tries tagged JSON blocks first, then falls back to generic JSON extraction.
// Returns (brief, chain) — at most one will be non-nil.
func extractQuestOutput(llmResponse string) (*domain.QuestBrief, *domain.QuestChainBrief) {
	// Try tagged JSON blocks first
	if extracted := extractTaggedJSON(llmResponse, "quest_chain"); extracted != "" {
		var chain domain.QuestChainBrief
		if err := json.Unmarshal([]byte(extracted), &chain); err == nil {
			if domain.ValidateQuestChainBrief(&chain) == nil {
				return nil, &chain
			}
		}
	}
	if extracted := extractTaggedJSON(llmResponse, "quest_brief"); extracted != "" {
		var brief domain.QuestBrief
		if err := json.Unmarshal([]byte(extracted), &brief); err == nil {
			if domain.ValidateQuestBrief(&brief) == nil {
				return &brief, nil
			}
		}
	}

	// Fall back to generic JSON extraction
	extracted := semdragons.ExtractJSONFromLLMResponse(llmResponse)
	if extracted == "" || extracted == llmResponse {
		return nil, nil
	}
	// Try as chain first (has "quests" array), then as brief
	var chain domain.QuestChainBrief
	if err := json.Unmarshal([]byte(extracted), &chain); err == nil && len(chain.Quests) > 0 {
		if domain.ValidateQuestChainBrief(&chain) == nil {
			return nil, &chain
		}
	}
	var brief domain.QuestBrief
	if err := json.Unmarshal([]byte(extracted), &brief); err == nil {
		if domain.ValidateQuestBrief(&brief) == nil {
			return &brief, nil
		}
	}
	return nil, nil
}

// extractTaggedJSON extracts JSON from a tagged code block like ```json:quest_brief
func extractTaggedJSON(text, tag string) string {
	marker := "```json:" + tag
	start := strings.Index(text, marker)
	if start == -1 {
		return ""
	}
	start += len(marker)
	// Skip to next line
	if nl := strings.Index(text[start:], "\n"); nl != -1 {
		start += nl + 1
	}
	end := strings.Index(text[start:], "```")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(text[start : start+end])
}

// interventionRequest is the JSON body for the DM intervention endpoint.
type interventionRequest struct {
	Clarification string `json:"clarification"`
	Action        string `json:"action"` // "repost" (default) or "clarify"
}

func (s *Service) handleDMIntervene(w http.ResponseWriter, r *http.Request) {
	questID := r.PathValue("questId")
	if !isValidPathID(questID) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Parse request body
	var req interventionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodySize)).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Default action is repost — resume the quest with the same agent.
	// "clarify" requires clarification text; "repost" allows optional guidance.
	if req.Action == "" {
		req.Action = "repost"
	}
	switch req.Action {
	case "repost":
		// Clarification is optional — if provided, injected as DM guidance.
	case "clarify":
		if req.Clarification == "" {
			s.writeError(w, "clarification is required for clarify action", http.StatusBadRequest)
			return
		}
	default:
		s.writeError(w, "unsupported action (repost, clarify)", http.StatusBadRequest)
		return
	}

	// Load quest
	questEntity, err := s.graph.GetQuest(ctx, domain.QuestID(questID))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to load quest", http.StatusInternalServerError)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	// Only escalated quests can be intervened on
	if quest.Status != domain.QuestEscalated {
		s.writeError(w, "quest must be escalated to intervene (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	// Extract the agent's question from the quest output (stored as FailureReason
	// when the quest was escalated by questbridge).
	question := extractClarificationQuestion(quest.Output)
	if question == "" {
		question = quest.FailureReason
	}

	// Only record a clarification exchange when the DM provided guidance text.
	// Plain reposts (no clarification) skip this to keep the exchange log clean.
	if req.Clarification != "" {
		var exchanges []domain.ClarificationExchange
		if quest.DMClarifications != nil {
			raw, _ := json.Marshal(quest.DMClarifications)
			json.Unmarshal(raw, &exchanges) //nolint:errcheck // best-effort; nil slice is fine
		}
		exchanges = append(exchanges, domain.ClarificationExchange{
			Question: question,
			Answer:   req.Clarification,
			AskedAt:  time.Now(),
		})
		quest.DMClarifications = exchanges
	}

	// Return quest to in_progress with the same agent — this is a communication
	// loop, not a retry. The agent keeps the quest and questbridge re-dispatches
	// with the updated clarification exchanges in the assembled prompt.
	quest.Status = domain.QuestInProgress
	quest.Escalated = false
	quest.Output = nil
	quest.FailureReason = ""
	quest.FailureType = ""
	// Do NOT increment Attempts — clarification is not a retry.
	// Do NOT clear ClaimedBy/ClaimedAt/StartedAt — agent stays assigned.

	if err := s.graph.EmitEntityUpdate(ctx, quest, "quest.started"); err != nil {
		s.writeError(w, "failed to resume quest", http.StatusInternalServerError)
		s.logger.Error("Failed to resume quest after DM intervention", "error", err)
		return
	}

	s.writeJSON(w, quest)
}

// TriageDecision is the request body for POST /dm/triage/{questId}.
type TriageDecision struct {
	Path           string   `json:"path"`                      // salvage, tpk, escalate, terminal
	Analysis       string   `json:"analysis"`                  // DM's failure analysis
	SalvagedOutput any      `json:"salvaged_output,omitempty"` // Curated partial work (salvage path)
	AntiPatterns   []string `json:"anti_patterns,omitempty"`   // What NOT to do (tpk path)
}

func (s *Service) handleDMTriage(w http.ResponseWriter, r *http.Request) {
	questID := r.PathValue("questId")
	if !isValidPathID(questID) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	var req TriageDecision
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodySize)).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	path := domain.RecoveryPath(req.Path)
	switch path {
	case domain.RecoverySalvage, domain.RecoveryTPK, domain.RecoveryEscalate, domain.RecoveryTerminal:
	default:
		s.writeError(w, "invalid recovery path: must be salvage, tpk, escalate, or terminal", http.StatusBadRequest)
		return
	}

	if req.Analysis == "" {
		s.writeError(w, "analysis is required", http.StatusBadRequest)
		return
	}

	// Load quest
	questEntity, err := s.graph.GetQuest(ctx, domain.QuestID(questID))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to load quest", http.StatusInternalServerError)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != domain.QuestPendingTriage {
		s.writeError(w, "quest must be in pending_triage (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	quest.RecoveryPath = path
	quest.FailureAnalysis = req.Analysis

	var predicate string

	switch path {
	case domain.RecoverySalvage:
		quest.SalvagedOutput = req.SalvagedOutput
		quest.MaxAttempts++
		quest.Status = domain.QuestPosted
		predicate = domain.PredicateQuestTriaged

	case domain.RecoveryTPK:
		quest.AntiPatterns = req.AntiPatterns
		quest.Output = nil
		quest.MaxAttempts++
		quest.Status = domain.QuestPosted
		predicate = domain.PredicateQuestTriaged

	case domain.RecoveryEscalate:
		quest.Status = domain.QuestEscalated
		quest.Escalated = true
		predicate = domain.PredicateQuestEscalated

	case domain.RecoveryTerminal:
		quest.Status = domain.QuestFailed
		predicate = "quest.failed"
	}

	if err := s.graph.EmitEntityUpdate(ctx, quest, predicate); err != nil {
		s.writeError(w, "failed to apply triage", http.StatusInternalServerError)
		s.logger.Error("Failed to apply DM triage", "error", err, "quest_id", questID, "path", path)
		return
	}

	s.logger.Info("DM triage applied", "quest_id", questID, "path", path, "new_status", quest.Status)
	s.writeJSON(w, quest)
}

// extractClarificationQuestion extracts the agent's question text from the
// quest output, stripping any structured [INTENT: clarification] header.
func extractClarificationQuestion(output any) string {
	text, ok := output.(string)
	if !ok {
		return ""
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return trimmed
	}
	// Strip [INTENT: ...] header line if present.
	if strings.HasPrefix(trimmed, "[INTENT:") {
		if idx := strings.Index(trimmed, "\n"); idx >= 0 {
			return strings.TrimSpace(trimmed[idx+1:])
		}
	}
	return trimmed
}

func (s *Service) handleGetDMSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidSessionID(id) {
		s.writeError(w, "invalid session ID", http.StatusBadRequest)
		return
	}

	if s.dmSessionReader == nil {
		s.writeError(w, "session store unavailable", http.StatusServiceUnavailable)
		return
	}

	session, err := s.dmSessionReader.GetSession(r.Context(), id)
	if err != nil {
		s.writeError(w, "failed to retrieve session", http.StatusInternalServerError)
		s.logger.Error("Failed to get DM session", "id", id, "error", err)
		return
	}
	if session == nil {
		s.writeError(w, "session not found", http.StatusNotFound)
		return
	}

	s.writeJSON(w, session)
}

// =============================================================================
// STORE
// =============================================================================

func (s *Service) handleListStore(w http.ResponseWriter, r *http.Request) {
	store := s.getStore()
	if store == nil {
		s.writeError(w, "store service unavailable", http.StatusServiceUnavailable)
		return
	}

	agentIDParam := r.URL.Query().Get("agent_id")
	if agentIDParam != "" {
		// Look up agent to get tier for filtering
		agentEntity, err := s.graph.GetAgent(r.Context(), domain.AgentID(agentIDParam))
		if err != nil {
			if isBucketNotFound(err) || isKeyNotFound(err) {
				s.writeError(w, "agent not found", http.StatusNotFound)
				return
			}
			s.writeError(w, "failed to retrieve agent", http.StatusInternalServerError)
			s.logger.Error("Failed to retrieve agent for store listing", "agent_id", agentIDParam, "error", err)
			return
		}
		agent := agentprogression.AgentFromEntityState(agentEntity)
		if agent == nil {
			s.writeError(w, "agent not found", http.StatusNotFound)
			return
		}
		s.writeJSON(w, store.ListItems(agent.Tier))
		return
	}

	s.writeJSON(w, store.Catalog())
}

func (s *Service) handleGetStoreItem(w http.ResponseWriter, r *http.Request) {
	store := s.getStore()
	if store == nil {
		s.writeError(w, "store service unavailable", http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid item ID", http.StatusBadRequest)
		return
	}

	item, ok := store.GetItem(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	s.writeJSON(w, item)
}

func (s *Service) handlePurchase(w http.ResponseWriter, r *http.Request) {
	if s.getStore() == nil {
		s.writeError(w, "store service unavailable", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req struct {
		AgentID string `json:"agent_id"`
		ItemID  string `json:"item_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.AgentID == "" || req.ItemID == "" {
		s.writeError(w, "agent_id and item_id are required", http.StatusBadRequest)
		return
	}
	if !isValidPathID(req.ItemID) {
		s.writeError(w, "invalid item_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Normalize agent ID to instance portion — graph.GetAgent handles both
	// full entity IDs and instance-only, but store methods use the ID as a
	// sync.Map key so it must match what path-based handlers pass (instance only).
	agentID := domain.AgentID(domain.ExtractInstance(req.AgentID))

	// Resolve agent state from graph
	agentEntity, err := s.graph.GetAgent(ctx, agentID)
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			s.writeError(w, "agent not found", http.StatusNotFound)
			return
		}
		s.writeError(w, "failed to retrieve agent", http.StatusInternalServerError)
		s.logger.Error("Failed to retrieve agent for purchase", "agent_id", req.AgentID, "error", err)
		return
	}
	agent := agentprogression.AgentFromEntityState(agentEntity)
	if agent == nil {
		s.writeError(w, "agent not found", http.StatusNotFound)
		return
	}

	// Check tier gate before purchasing
	store := s.getStore()
	if store == nil {
		s.writeError(w, "store service unavailable", http.StatusServiceUnavailable)
		return
	}
	item, itemOK := store.GetItem(req.ItemID)
	if !itemOK {
		s.writeError(w, "item not found", http.StatusNotFound)
		return
	}
	if agent.Tier < item.MinTier {
		s.writeError(w, "agent tier too low for this item", http.StatusForbidden)
		return
	}

	// Retry purchase — the agent_store component may be mid-restart if a
	// config KV update triggered a component restart cycle.
	owned, purchaseErr := retry.DoWithResult(ctx, retry.Quick(), func() (*agentstore.OwnedItem, error) {
		st := s.getStore()
		if st == nil {
			return nil, errStoreUnavailable
		}
		return st.Purchase(ctx, agentID, req.ItemID, agent.XP, agent.Level, agent.Guild)
	})
	if purchaseErr != nil {
		s.logger.Warn("Purchase failed", "agent_id", agentID, "item_id", req.ItemID, "error", purchaseErr)
		s.writeJSON(w, map[string]any{
			"success": false,
			"error":   purchaseErr.Error(),
		})
		return
	}

	store = s.getStore()
	var inv *agentstore.AgentInventory
	if store != nil {
		inv = store.GetInventory(agentID)
	}

	s.writeJSON(w, map[string]any{
		"success":      true,
		"item":         item,
		"xp_spent":     owned.XPSpent,
		"xp_remaining": agent.XP - owned.XPSpent,
		"inventory":    inv,
	})
}

func (s *Service) handleGetInventory(w http.ResponseWriter, r *http.Request) {
	store := s.getStore()
	if store == nil {
		s.writeError(w, "store service unavailable", http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	inv := store.GetInventory(domain.AgentID(id))
	s.writeJSON(w, inv)
}

func (s *Service) handleUseConsumable(w http.ResponseWriter, r *http.Request) {
	if s.getStore() == nil {
		s.writeError(w, "store service unavailable", http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req struct {
		ConsumableID string `json:"consumable_id"`
		QuestID      string `json:"quest_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ConsumableID == "" {
		s.writeError(w, "consumable_id is required", http.StatusBadRequest)
		return
	}

	agentID := domain.AgentID(id)

	var questIDPtr *domain.QuestID
	if req.QuestID != "" {
		qid := domain.QuestID(req.QuestID)
		questIDPtr = &qid
	}

	// Retry — agent_store may be mid-restart from a config KV update.
	err := retry.Do(r.Context(), retry.Quick(), func() error {
		st := s.getStore()
		if st == nil {
			return errStoreUnavailable
		}
		return st.UseConsumable(r.Context(), agentID, req.ConsumableID, questIDPtr)
	})
	if err != nil {
		s.writeJSON(w, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	store := s.getStore()
	var remaining int
	var effects []agentstore.ActiveEffect
	if store != nil {
		inv := store.GetInventory(agentID)
		remaining = inv.ConsumableCount(req.ConsumableID)
		effects = store.GetActiveEffects(agentID)
	}

	s.writeJSON(w, map[string]any{
		"success":        true,
		"remaining":      remaining,
		"active_effects": effects,
	})
}

func (s *Service) handleGetEffects(w http.ResponseWriter, r *http.Request) {
	store := s.getStore()
	if store == nil {
		s.writeError(w, "store service unavailable", http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	effects := store.GetActiveEffects(domain.AgentID(id))
	if effects == nil {
		effects = make([]agentstore.ActiveEffect, 0)
	}
	s.writeJSON(w, effects)
}

// =============================================================================
// PEER REVIEWS
// =============================================================================

func (s *Service) handleCreateReview(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req struct {
		QuestID    string  `json:"quest_id"`
		PartyID    *string `json:"party_id,omitempty"`
		LeaderID   string  `json:"leader_id"`
		MemberID   string  `json:"member_id"`
		IsSoloTask bool    `json:"is_solo_task"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.QuestID == "" {
		s.writeError(w, "quest_id is required", http.StatusBadRequest)
		return
	}
	if req.LeaderID == "" {
		s.writeError(w, "leader_id is required", http.StatusBadRequest)
		return
	}
	if req.MemberID == "" {
		s.writeError(w, "member_id is required", http.StatusBadRequest)
		return
	}
	if req.LeaderID == req.MemberID && !req.IsSoloTask {
		s.writeError(w, "leader_id and member_id must be different for non-solo reviews", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	instance := domain.GenerateShortInstance()
	reviewID := s.graph.Config().PeerReviewEntityID(instance)
	now := time.Now()

	review := &domain.PeerReview{
		ID:         domain.PeerReviewID(reviewID),
		Status:     domain.PeerReviewPending,
		QuestID:    domain.QuestID(req.QuestID),
		LeaderID:   domain.AgentID(req.LeaderID),
		MemberID:   domain.AgentID(req.MemberID),
		IsSoloTask: req.IsSoloTask,
		CreatedAt:  now,
	}
	if req.PartyID != nil {
		pid := domain.PartyID(*req.PartyID)
		review.PartyID = &pid
	}

	if err := s.graph.EmitEntity(ctx, review, "review.lifecycle.pending"); err != nil {
		s.writeError(w, "failed to create review", http.StatusInternalServerError)
		s.logger.Error("Failed to create peer review", "error", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(review)
}

func (s *Service) handleSubmitReview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid review ID", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req struct {
		ReviewerID string `json:"reviewer_id"`
		Ratings    struct {
			Q1 int `json:"q1"`
			Q2 int `json:"q2"`
			Q3 int `json:"q3"`
		} `json:"ratings"`
		Explanation string `json:"explanation,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.ReviewerID == "" {
		s.writeError(w, "reviewer_id is required", http.StatusBadRequest)
		return
	}

	ratings := domain.ReviewRatings{Q1: req.Ratings.Q1, Q2: req.Ratings.Q2, Q3: req.Ratings.Q3}
	if err := ratings.Validate(req.Explanation); err != nil {
		s.writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Load existing review
	entity, err := s.graph.GetPeerReview(ctx, domain.PeerReviewID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve review", http.StatusInternalServerError)
		s.logger.Error("Failed to get peer review", "id", id, "error", err)
		return
	}

	review := domain.PeerReviewFromEntityState(entity)
	if review == nil {
		http.NotFound(w, r)
		return
	}

	if review.Status == domain.PeerReviewCompleted {
		s.writeError(w, "review is already completed", http.StatusConflict)
		return
	}

	reviewerID := domain.AgentID(req.ReviewerID)
	now := time.Now()

	switch reviewerID {
	case review.LeaderID:
		if review.LeaderReview != nil {
			s.writeError(w, "leader has already submitted a review", http.StatusConflict)
			return
		}
		review.LeaderReview = &domain.ReviewSubmission{
			ReviewerID:  reviewerID,
			RevieweeID:  review.MemberID,
			Direction:   domain.ReviewDirectionLeaderToMember,
			Ratings:     ratings,
			Explanation: req.Explanation,
			SubmittedAt: now,
		}
	case review.MemberID:
		if review.IsSoloTask {
			s.writeError(w, "solo tasks do not accept member reviews", http.StatusBadRequest)
			return
		}
		if review.MemberReview != nil {
			s.writeError(w, "member has already submitted a review", http.StatusConflict)
			return
		}
		review.MemberReview = &domain.ReviewSubmission{
			ReviewerID:  reviewerID,
			RevieweeID:  review.LeaderID,
			Direction:   domain.ReviewDirectionMemberToLeader,
			Ratings:     ratings,
			Explanation: req.Explanation,
			SubmittedAt: now,
		}
	default:
		s.writeError(w, "reviewer is not a participant in this review", http.StatusForbidden)
		return
	}

	// Determine completion
	completed := false
	if review.IsSoloTask {
		// Solo tasks complete when leader (DM) submits
		completed = review.LeaderReview != nil
	} else {
		completed = review.LeaderReview != nil && review.MemberReview != nil
	}

	eventType := "review.lifecycle.submitted"
	if completed {
		review.Status = domain.PeerReviewCompleted
		review.CompletedAt = &now
		eventType = "review.lifecycle.completed"

		// Compute averages: LeaderAvgRating = member's rating of leader,
		// MemberAvgRating = leader's rating of member
		if review.LeaderReview != nil {
			review.MemberAvgRating = review.LeaderReview.Ratings.Average()
		}
		if review.MemberReview != nil {
			review.LeaderAvgRating = review.MemberReview.Ratings.Average()
		}
	} else {
		review.Status = domain.PeerReviewPartial
	}

	if err := s.graph.EmitEntityUpdate(ctx, review, eventType); err != nil {
		s.writeError(w, "failed to submit review", http.StatusInternalServerError)
		s.logger.Error("Failed to submit peer review", "id", id, "error", err)
		return
	}

	// Blind enforcement: mask the other party's submission until completed
	resp := blindMaskReview(review, reviewerID)
	s.writeJSON(w, resp)
}

// stripPartialSubmissions redacts both submissions from non-completed reviews
// returned by unauthenticated GET/LIST endpoints to enforce blind review.
func stripPartialSubmissions(review *domain.PeerReview) domain.PeerReview {
	if review.Status == domain.PeerReviewCompleted {
		return *review
	}
	masked := *review
	masked.LeaderReview = nil
	masked.MemberReview = nil
	masked.LeaderAvgRating = 0
	masked.MemberAvgRating = 0
	return masked
}

// blindMaskReview returns a copy of the review with the other party's submission
// masked out if the review is not yet completed. This enforces blind review —
// neither party can see the other's ratings until both have submitted.
func blindMaskReview(review *domain.PeerReview, viewerID domain.AgentID) *domain.PeerReview {
	if review.Status == domain.PeerReviewCompleted {
		return review
	}
	// Create shallow copy to avoid mutating original
	masked := *review
	switch viewerID {
	case review.LeaderID:
		masked.MemberReview = nil
		masked.LeaderAvgRating = 0
	case review.MemberID:
		masked.LeaderReview = nil
		masked.MemberAvgRating = 0
	default:
		// Non-participant: mask both
		masked.LeaderReview = nil
		masked.MemberReview = nil
		masked.LeaderAvgRating = 0
		masked.MemberAvgRating = 0
	}
	return &masked
}

func (s *Service) handleGetReview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid review ID", http.StatusBadRequest)
		return
	}

	entity, err := s.graph.GetPeerReview(r.Context(), domain.PeerReviewID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve review", http.StatusInternalServerError)
		s.logger.Error("Failed to get peer review", "id", id, "error", err)
		return
	}

	review := domain.PeerReviewFromEntityState(entity)
	if review == nil {
		http.NotFound(w, r)
		return
	}

	// Strip partial submissions from unauthenticated GET to enforce blind review.
	masked := stripPartialSubmissions(review)
	s.writeJSON(w, masked)
}

func (s *Service) handleListReviews(w http.ResponseWriter, r *http.Request) {
	entities, err := s.graph.ListPeerReviewsByPrefix(r.Context(), s.config.MaxEntities)
	if err != nil {
		if isBucketNotFound(err) {
			s.writeJSON(w, []domain.PeerReview{})
			return
		}
		s.writeError(w, "failed to list reviews", http.StatusInternalServerError)
		s.logger.Error("Failed to list peer reviews", "error", err)
		return
	}

	statusFilter := r.URL.Query().Get("status")
	questFilter := r.URL.Query().Get("quest_id")

	var reviews []domain.PeerReview
	for _, entity := range entities {
		review := domain.PeerReviewFromEntityState(&entity)
		if review == nil {
			continue
		}
		if statusFilter != "" && string(review.Status) != statusFilter {
			continue
		}
		if questFilter != "" && string(review.QuestID) != questFilter {
			continue
		}
		reviews = append(reviews, stripPartialSubmissions(review))
	}

	if reviews == nil {
		reviews = []domain.PeerReview{}
	}
	s.writeJSON(w, reviews)
}

func (s *Service) handleListAgentReviews(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid agent ID", http.StatusBadRequest)
		return
	}

	// Build a match function that checks both instance ID and full entity ID,
	// since reviews may store either form depending on the creation path.
	instance := domain.ExtractInstance(id)
	shortID := domain.AgentID(instance)
	fullID := shortID
	if s.boardConfig != nil {
		fullID = domain.AgentID(s.boardConfig.AgentEntityID(instance))
	}
	matchesAgent := func(reviewAgentID domain.AgentID) bool {
		return reviewAgentID == shortID || reviewAgentID == fullID
	}

	entities, err := s.graph.ListPeerReviewsByPrefix(r.Context(), s.config.MaxEntities)
	if err != nil {
		if isBucketNotFound(err) {
			s.writeJSON(w, []domain.PeerReview{})
			return
		}
		s.writeError(w, "failed to list reviews", http.StatusInternalServerError)
		s.logger.Error("Failed to list agent reviews", "agent_id", id, "error", err)
		return
	}

	var reviews []domain.PeerReview
	for _, entity := range entities {
		review := domain.PeerReviewFromEntityState(&entity)
		if review == nil {
			continue
		}
		if !matchesAgent(review.LeaderID) && !matchesAgent(review.MemberID) {
			continue
		}
		reviews = append(reviews, stripPartialSubmissions(review))
	}

	if reviews == nil {
		reviews = []domain.PeerReview{}
	}
	s.writeJSON(w, reviews)
}

// =============================================================================
// BOARD CONTROL (PLAY/PAUSE)
// =============================================================================

func (s *Service) handleBoardStatus(w http.ResponseWriter, _ *http.Request) {
	if s.board == nil {
		// No controller: report as running (not paused).
		s.writeJSON(w, map[string]any{
			"paused":    false,
			"paused_at": nil,
			"paused_by": nil,
		})
		return
	}
	s.writeJSON(w, s.board.State())
}

func (s *Service) handleBoardPause(w http.ResponseWriter, r *http.Request) {
	if s.board == nil {
		s.writeError(w, "board controller unavailable", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Actor string `json:"actor,omitempty"`
	}
	// Body is optional — actor field is nice-to-have.
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	st, err := s.board.Pause(r.Context(), req.Actor)
	if err != nil {
		s.writeError(w, "failed to pause board", http.StatusInternalServerError)
		s.logger.Error("Failed to pause board", "error", err)
		return
	}

	s.logger.Info("Board paused", "actor", req.Actor)
	s.writeJSON(w, st)
}

func (s *Service) handleBoardResume(w http.ResponseWriter, r *http.Request) {
	if s.board == nil {
		s.writeError(w, "board controller unavailable", http.StatusServiceUnavailable)
		return
	}

	st, err := s.board.Resume(r.Context())
	if err != nil {
		s.writeError(w, "failed to resume board", http.StatusInternalServerError)
		s.logger.Error("Failed to resume board", "error", err)
		return
	}

	s.logger.Info("Board resumed")
	s.writeJSON(w, st)
}

// =============================================================================
// TOKEN BUDGET
// =============================================================================

func (s *Service) handleTokenStats(w http.ResponseWriter, _ *http.Request) {
	if s.tokenLedger == nil {
		s.writeError(w, "token ledger not initialized", http.StatusServiceUnavailable)
		return
	}
	s.writeJSON(w, s.tokenLedger.Stats())
}

func (s *Service) handleSetTokenBudget(w http.ResponseWriter, r *http.Request) {
	if s.tokenLedger == nil {
		s.writeError(w, "token ledger not initialized", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req SetTokenBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.tokenLedger.SetBudget(r.Context(), req.GlobalHourlyLimit); err != nil {
		s.writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.writeJSON(w, s.tokenLedger.Stats())
}

// =============================================================================
// MODEL REGISTRY
// =============================================================================

// handleGetModels returns model registry state. With ?resolve=capability, returns
// the resolution result for a single capability. Without it, returns a full summary
// of all endpoints and capabilities.
func (s *Service) handleGetModels(w http.ResponseWriter, r *http.Request) {
	if s.models == nil {
		s.writeError(w, "model registry unavailable", http.StatusServiceUnavailable)
		return
	}

	// Single capability resolution
	if capability := r.URL.Query().Get("resolve"); capability != "" {
		name := s.models.Resolve(capability)
		ep := s.models.GetEndpoint(name)

		resp := ModelResolveResponse{
			Capability:   capability,
			EndpointName: name,
		}
		if ep != nil {
			resp.Model = ep.Model
			resp.Provider = ep.Provider
		}
		resp.FallbackChain = s.models.GetFallbackChain(capability)

		s.writeJSON(w, resp)
		return
	}

	// Full registry summary
	endpointNames := s.models.ListEndpoints()
	endpoints := make([]ModelEndpointSummary, 0, len(endpointNames))
	for _, name := range endpointNames {
		ep := s.models.GetEndpoint(name)
		if ep == nil {
			continue
		}
		endpoints = append(endpoints, ModelEndpointSummary{
			Name:            name,
			Provider:        ep.Provider,
			Model:           ep.Model,
			MaxTokens:       ep.MaxTokens,
			SupportsTools:   ep.SupportsTools,
			ReasoningEffort: ep.ReasoningEffort,
		})
	}

	s.writeJSON(w, ModelRegistrySummary{
		Endpoints:    endpoints,
		Capabilities: s.models.ListCapabilities(),
	})
}

// =============================================================================
// HELPERS
// =============================================================================

func (s *Service) writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("Failed to encode JSON response", "error", err)
	}
}

func (s *Service) writeError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// truncateName shortens a string to max characters, preferring word boundaries.
func truncateName(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if i := strings.LastIndex(s[:max], " "); i > max/2 {
		return s[:i] + "..."
	}
	return s[:max-1] + "..."
}
