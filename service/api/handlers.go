package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/nats-io/nats.go/jetstream"
)

const maxRequestBodySize = 1 << 20 // 1 MB

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
	s.writeJSON(w, state)
}

// =============================================================================
// QUESTS
// =============================================================================

func (s *Service) handleListQuests(w http.ResponseWriter, r *http.Request) {
	entities, err := s.graph.ListQuestsByPrefix(r.Context(), s.config.MaxEntities)
	if err != nil {
		if isBucketNotFound(err) {
			s.writeJSON(w, []semdragons.Quest{})
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

	var quests []semdragons.Quest
	for _, entity := range entities {
		quest := semdragons.QuestFromEntityState(&entity)
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
		quests = []semdragons.Quest{}
	}
	s.writeJSON(w, quests)
}

func (s *Service) handleGetQuest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	entity, err := s.graph.GetQuest(r.Context(), semdragons.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		s.logger.Error("Failed to get quest", "id", id, "error", err)
		return
	}

	quest := semdragons.QuestFromEntityState(entity)
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
	instance := semdragons.GenerateShortInstance()
	questID := s.graph.Config().QuestEntityID(instance)

	quest := &semdragons.Quest{
		ID:          semdragons.QuestID(questID),
		Title:       req.Objective,
		Description: req.Objective,
		Status:      semdragons.QuestPosted,
		Difficulty:  semdragons.DifficultyModerate,
		BaseXP:      100,
		MaxAttempts: 3,
	}

	if req.Hints != nil {
		if req.Hints.SuggestedDifficulty != nil {
			d := *req.Hints.SuggestedDifficulty
			// DifficultyTrivial=0 through DifficultyLegendary=5 (iota-based constants)
			if d < int(semdragons.DifficultyTrivial) || d > int(semdragons.DifficultyLegendary) {
				s.writeError(w, "difficulty must be between 0 and 5", http.StatusBadRequest)
				return
			}
			quest.Difficulty = semdragons.QuestDifficulty(d)
		}
		for _, skill := range req.Hints.SuggestedSkills {
			quest.RequiredSkills = append(quest.RequiredSkills, semdragons.SkillTag(skill))
		}
		if req.Hints.PreferGuild != nil {
			gid := semdragons.GuildID(*req.Hints.PreferGuild)
			quest.GuildPriority = &gid
		}
		if req.Hints.RequireHumanReview {
			quest.Constraints.RequireReview = true
			if req.Hints.ReviewLevel != nil {
				quest.Constraints.ReviewLevel = semdragons.ReviewLevel(*req.Hints.ReviewLevel)
			} else {
				quest.Constraints.ReviewLevel = semdragons.ReviewStandard
			}
		}
	}

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

	var chain semdragons.QuestChainBrief
	if err := json.NewDecoder(r.Body).Decode(&chain); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := semdragons.ValidateQuestChainBrief(&chain); err != nil {
		s.writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	now := time.Now()

	// First pass: post each quest (no DependsOn yet — we need real IDs first)
	posted := make([]semdragons.Quest, 0, len(chain.Quests))
	for _, entry := range chain.Quests {
		instance := semdragons.GenerateShortInstance()
		questID := s.graph.Config().QuestEntityID(instance)

		difficulty := semdragons.DifficultyModerate
		if entry.Difficulty != nil {
			difficulty = *entry.Difficulty
		}

		quest := semdragons.Quest{
			ID:          semdragons.QuestID(questID),
			Title:       entry.Title,
			Description: entry.Description,
			Status:      semdragons.QuestPosted,
			Difficulty:  difficulty,
			BaseXP:      semdragons.DefaultXPForDifficulty(difficulty),
			MinTier:     semdragons.TierFromDifficulty(difficulty),
			MaxAttempts: 3,
			PostedAt:    now,
			Acceptance:  entry.Acceptance,
		}

		quest.RequiredSkills = append(quest.RequiredSkills, entry.Skills...)

		if entry.Hints != nil {
			if entry.Hints.RequireHumanReview {
				quest.Constraints.RequireReview = true
				quest.Constraints.ReviewLevel = semdragons.ReviewStandard
			}
			if entry.Hints.PreferGuild != nil {
				quest.GuildPriority = entry.Hints.PreferGuild
			}
		}

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

		deps := make([]semdragons.QuestID, 0, len(entry.DependsOn))
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
	questEntity, err := s.graph.GetQuest(ctx, semdragons.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		return
	}
	quest := semdragons.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != semdragons.QuestPosted {
		s.writeError(w, "quest is not available for claiming (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	// Load agent
	agentEntity, err := s.graph.GetAgent(ctx, semdragons.AgentID(req.AgentID))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			s.writeError(w, "agent not found", http.StatusNotFound)
			return
		}
		s.writeError(w, "failed to retrieve agent", http.StatusInternalServerError)
		return
	}
	agent := semdragons.AgentFromEntityState(agentEntity)
	if agent == nil {
		s.writeError(w, "agent not found", http.StatusNotFound)
		return
	}

	// Validate agent state
	if agent.Status != semdragons.AgentIdle {
		s.writeError(w, "agent is not idle (status: "+string(agent.Status)+")", http.StatusConflict)
		return
	}

	// Validate tier
	minTier := quest.MinTier
	if minTier == 0 {
		minTier = semdragons.TierFromDifficulty(quest.Difficulty)
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
	quest.Status = semdragons.QuestClaimed
	quest.ClaimedBy = &agentID
	quest.ClaimedAt = &now

	if err := s.graph.EmitEntityUpdate(ctx, quest, "quest.claimed"); err != nil {
		s.writeError(w, "failed to claim quest", http.StatusInternalServerError)
		s.logger.Error("Failed to claim quest", "error", err)
		return
	}

	// Update agent status
	questID := quest.ID
	agent.Status = semdragons.AgentOnQuest
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
	questEntity, err := s.graph.GetQuest(ctx, semdragons.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		return
	}
	quest := semdragons.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != semdragons.QuestClaimed {
		s.writeError(w, "quest must be claimed before starting (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	now := time.Now()
	quest.Status = semdragons.QuestInProgress
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
	questEntity, err := s.graph.GetQuest(ctx, semdragons.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		return
	}
	quest := semdragons.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != semdragons.QuestInProgress {
		s.writeError(w, "quest must be in progress to submit (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	quest.Output = req.Output

	if quest.Constraints.RequireReview {
		quest.Status = semdragons.QuestInReview
	} else {
		now := time.Now()
		quest.Status = semdragons.QuestCompleted
		quest.CompletedAt = &now
	}

	if err := s.graph.EmitEntityUpdate(ctx, quest, "quest.submitted"); err != nil {
		s.writeError(w, "failed to submit quest result", http.StatusInternalServerError)
		s.logger.Error("Failed to submit quest result", "error", err)
		return
	}

	// If quest completed without review, release the agent
	if quest.Status == semdragons.QuestCompleted && quest.ClaimedBy != nil {
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
	questEntity, err := s.graph.GetQuest(ctx, semdragons.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		return
	}
	quest := semdragons.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != semdragons.QuestInReview && quest.Status != semdragons.QuestInProgress {
		s.writeError(w, "quest must be in_review or in_progress to complete (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	now := time.Now()
	quest.Status = semdragons.QuestCompleted
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
	questEntity, err := s.graph.GetQuest(ctx, semdragons.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		return
	}
	quest := semdragons.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != semdragons.QuestInProgress && quest.Status != semdragons.QuestInReview {
		s.writeError(w, "quest must be in_progress or in_review to fail (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	// Release agent before reposting/failing
	if quest.ClaimedBy != nil {
		s.releaseAgent(ctx, *quest.ClaimedBy)
	}

	quest.Attempts++

	if quest.MaxAttempts > 0 && quest.Attempts >= quest.MaxAttempts {
		quest.Status = semdragons.QuestFailed
	} else {
		// Repost: reset assignment fields for another attempt
		quest.Status = semdragons.QuestPosted
		quest.ClaimedBy = nil
		quest.ClaimedAt = nil
		quest.StartedAt = nil
		quest.Output = nil
	}

	eventType := "quest.failed"
	if quest.Status == semdragons.QuestPosted {
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
	questEntity, err := s.graph.GetQuest(ctx, semdragons.QuestID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve quest", http.StatusInternalServerError)
		return
	}
	quest := semdragons.QuestFromEntityState(questEntity)
	if quest == nil {
		http.NotFound(w, r)
		return
	}

	if quest.Status != semdragons.QuestClaimed && quest.Status != semdragons.QuestInProgress {
		s.writeError(w, "quest must be claimed or in_progress to abandon (status: "+string(quest.Status)+")", http.StatusConflict)
		return
	}

	// Release agent
	if quest.ClaimedBy != nil {
		s.releaseAgent(ctx, *quest.ClaimedBy)
	}

	// Return quest to board
	quest.Status = semdragons.QuestPosted
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

// releaseAgent sets an agent back to idle and clears their current quest.
func (s *Service) releaseAgent(ctx context.Context, agentID semdragons.AgentID) {
	agentEntity, err := s.graph.GetAgent(ctx, agentID)
	if err != nil {
		s.logger.Error("Failed to load agent for release", "agent_id", agentID, "error", err)
		return
	}
	agent := semdragons.AgentFromEntityState(agentEntity)
	if agent == nil {
		return
	}

	agent.Status = semdragons.AgentIdle
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
			s.writeJSON(w, []semdragons.Agent{})
			return
		}
		s.writeError(w, "failed to list agents", http.StatusInternalServerError)
		s.logger.Error("Failed to list agents", "error", err)
		return
	}

	agents := make([]semdragons.Agent, 0, len(entities))
	for _, entity := range entities {
		agent := semdragons.AgentFromEntityState(&entity)
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

	entity, err := s.graph.GetAgent(r.Context(), semdragons.AgentID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve agent", http.StatusInternalServerError)
		s.logger.Error("Failed to get agent", "id", id, "error", err)
		return
	}

	agent := semdragons.AgentFromEntityState(entity)
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
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		s.writeError(w, "name is required", http.StatusBadRequest)
		return
	}

	instance := semdragons.GenerateShortInstance()
	agentID := s.graph.Config().AgentEntityID(instance)

	agent := &semdragons.Agent{
		ID:                 semdragons.AgentID(agentID),
		Name:               req.Name,
		DisplayName:        req.DisplayName,
		Status:             semdragons.AgentIdle,
		Level:              1,
		XP:                 0,
		XPToLevel:          100,
		Tier:               semdragons.TierApprentice,
		IsNPC:              req.IsNPC,
		SkillProficiencies: make(map[semdragons.SkillTag]semdragons.SkillProficiency),
	}

	for _, skill := range req.Skills {
		agent.SkillProficiencies[semdragons.SkillTag(skill)] = semdragons.SkillProficiency{
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

	entity, err := s.graph.GetAgent(r.Context(), semdragons.AgentID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve agent", http.StatusInternalServerError)
		s.logger.Error("Failed to get agent for retire", "id", id, "error", err)
		return
	}

	agent := semdragons.AgentFromEntityState(entity)
	if agent == nil {
		http.NotFound(w, r)
		return
	}

	agent.Status = semdragons.AgentRetired

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
	entities, err := s.graph.ListEntitiesByType(r.Context(), semdragons.EntityTypeBattle, s.config.MaxEntities)
	if err != nil {
		if isBucketNotFound(err) {
			s.writeJSON(w, []semdragons.BossBattle{})
			return
		}
		s.writeError(w, "failed to list battles", http.StatusInternalServerError)
		s.logger.Error("Failed to list battles", "error", err)
		return
	}

	battles := make([]semdragons.BossBattle, 0, len(entities))
	for _, entity := range entities {
		battle := semdragons.BattleFromEntityState(&entity)
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

	entity, err := s.graph.GetBattle(r.Context(), semdragons.BattleID(id))
	if err != nil {
		if isBucketNotFound(err) || isKeyNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.writeError(w, "failed to retrieve battle", http.StatusInternalServerError)
		s.logger.Error("Failed to get battle", "id", id, "error", err)
		return
	}

	battle := semdragons.BattleFromEntityState(entity)
	if battle == nil {
		http.NotFound(w, r)
		return
	}

	s.writeJSON(w, battle)
}

// =============================================================================
// TRAJECTORIES
// =============================================================================

func (s *Service) handleGetTrajectory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}
	// TODO: Wire trajectory lookup from NATS KV when trajectory service is available
	s.writeError(w, "trajectory lookup not yet implemented", http.StatusNotImplemented)
}

// =============================================================================
// DUNGEON MASTER
// =============================================================================

func (s *Service) handleDMChat(w http.ResponseWriter, _ *http.Request) {
	s.writeError(w, "DM chat not yet implemented", http.StatusNotImplemented)
}

func (s *Service) handleDMIntervene(w http.ResponseWriter, r *http.Request) {
	questID := r.PathValue("questId")
	if !isValidPathID(questID) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}
	// TODO: Wire DM intervention when DM component is available
	s.writeError(w, "DM intervention not yet implemented", http.StatusNotImplemented)
}

// =============================================================================
// STORE
// =============================================================================

func (s *Service) handleListStore(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeError(w, "store service unavailable", http.StatusServiceUnavailable)
		return
	}

	agentIDParam := r.URL.Query().Get("agent_id")
	if agentIDParam != "" {
		// Look up agent to get tier for filtering
		agentEntity, err := s.graph.GetAgent(r.Context(), semdragons.AgentID(agentIDParam))
		if err != nil {
			if isBucketNotFound(err) || isKeyNotFound(err) {
				s.writeError(w, "agent not found", http.StatusNotFound)
				return
			}
			s.writeError(w, "failed to retrieve agent", http.StatusInternalServerError)
			s.logger.Error("Failed to retrieve agent for store listing", "agent_id", agentIDParam, "error", err)
			return
		}
		agent := semdragons.AgentFromEntityState(agentEntity)
		if agent == nil {
			s.writeError(w, "agent not found", http.StatusNotFound)
			return
		}
		s.writeJSON(w, s.store.ListItems(agent.Tier))
		return
	}

	s.writeJSON(w, s.store.Catalog())
}

func (s *Service) handleGetStoreItem(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeError(w, "store service unavailable", http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid item ID", http.StatusBadRequest)
		return
	}

	item, ok := s.store.GetItem(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	s.writeJSON(w, item)
}

func (s *Service) handlePurchase(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
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
	agentID := semdragons.AgentID(semdragons.ExtractInstance(req.AgentID))

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
	agent := semdragons.AgentFromEntityState(agentEntity)
	if agent == nil {
		s.writeError(w, "agent not found", http.StatusNotFound)
		return
	}

	// Check tier gate before purchasing
	item, itemOK := s.store.GetItem(req.ItemID)
	if !itemOK {
		s.writeError(w, "item not found", http.StatusNotFound)
		return
	}
	if agent.Tier < item.MinTier {
		s.writeError(w, "agent tier too low for this item", http.StatusForbidden)
		return
	}

	owned, purchaseErr := s.store.Purchase(ctx, agentID, req.ItemID, agent.XP, agent.Level, agent.Guilds)
	if purchaseErr != nil {
		s.logger.Warn("Purchase failed", "agent_id", agentID, "item_id", req.ItemID, "error", purchaseErr)
		s.writeJSON(w, map[string]any{
			"success": false,
			"error":   purchaseErr.Error(),
		})
		return
	}

	inv := s.store.GetInventory(agentID)

	s.writeJSON(w, map[string]any{
		"success":      true,
		"item":         item,
		"xp_spent":     owned.XPSpent,
		"xp_remaining": agent.XP - owned.XPSpent,
		"inventory":    inv,
	})
}

func (s *Service) handleGetInventory(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeError(w, "store service unavailable", http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	inv := s.store.GetInventory(semdragons.AgentID(id))
	s.writeJSON(w, inv)
}

func (s *Service) handleUseConsumable(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
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

	agentID := semdragons.AgentID(id)

	var questIDPtr *semdragons.QuestID
	if req.QuestID != "" {
		qid := semdragons.QuestID(req.QuestID)
		questIDPtr = &qid
	}

	if err := s.store.UseConsumable(r.Context(), agentID, req.ConsumableID, questIDPtr); err != nil {
		s.writeJSON(w, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	inv := s.store.GetInventory(agentID)
	remaining := inv.ConsumableCount(req.ConsumableID)
	effects := s.store.GetActiveEffects(agentID)

	s.writeJSON(w, map[string]any{
		"success":        true,
		"remaining":      remaining,
		"active_effects": effects,
	})
}

func (s *Service) handleGetEffects(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeError(w, "store service unavailable", http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid entity ID", http.StatusBadRequest)
		return
	}

	effects := s.store.GetActiveEffects(semdragons.AgentID(id))
	if effects == nil {
		effects = make([]agentstore.ActiveEffect, 0)
	}
	s.writeJSON(w, effects)
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
