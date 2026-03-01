package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	semdragons "github.com/c360studio/semdragons"
)

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
	entity, err := s.graph.GetQuest(r.Context(), semdragons.QuestID(id))
	if err != nil {
		http.NotFound(w, r)
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
	var req struct {
		Objective string `json:"objective"`
		Hints     *struct {
			SuggestedDifficulty *int      `json:"suggested_difficulty,omitempty"`
			SuggestedSkills     []string  `json:"suggested_skills,omitempty"`
			PreferGuild         *string   `json:"prefer_guild,omitempty"`
			RequireHumanReview  bool      `json:"require_human_review"`
			Budget              float64   `json:"budget"`
			Deadline            string    `json:"deadline,omitempty"`
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
			quest.Difficulty = semdragons.QuestDifficulty(*req.Hints.SuggestedDifficulty)
		}
		for _, s := range req.Hints.SuggestedSkills {
			quest.RequiredSkills = append(quest.RequiredSkills, semdragons.SkillTag(s))
		}
		if req.Hints.PreferGuild != nil {
			gid := semdragons.GuildID(*req.Hints.PreferGuild)
			quest.GuildPriority = &gid
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

// =============================================================================
// AGENTS
// =============================================================================

func (s *Service) handleListAgents(w http.ResponseWriter, r *http.Request) {
	entities, err := s.graph.ListAgentsByPrefix(r.Context(), s.config.MaxEntities)
	if err != nil {
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
	entity, err := s.graph.GetAgent(r.Context(), semdragons.AgentID(id))
	if err != nil {
		http.NotFound(w, r)
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

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	entity, err := s.graph.GetAgent(r.Context(), semdragons.AgentID(id))
	if err != nil {
		http.NotFound(w, r)
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
	entity, err := s.graph.GetBattle(r.Context(), semdragons.BattleID(id))
	if err != nil {
		http.NotFound(w, r)
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

func (s *Service) handleGetTrajectory(w http.ResponseWriter, _ *http.Request) {
	// TODO: Wire trajectory lookup from NATS KV when trajectory service is available
	s.writeError(w, "trajectory lookup not yet implemented", http.StatusNotImplemented)
}

// =============================================================================
// DUNGEON MASTER
// =============================================================================

func (s *Service) handleDMChat(w http.ResponseWriter, _ *http.Request) {
	s.writeError(w, "DM chat not yet implemented", http.StatusNotImplemented)
}

func (s *Service) handleDMIntervene(w http.ResponseWriter, _ *http.Request) {
	s.writeError(w, "DM intervention not yet implemented", http.StatusNotImplemented)
}

// =============================================================================
// STORE
// =============================================================================

func (s *Service) handleListStore(w http.ResponseWriter, _ *http.Request) {
	// Agent store items are managed by the agentstore processor component.
	// The API service doesn't hold a direct reference to the component instance.
	// For now, return an empty array; will be wired when component access is available.
	s.writeJSON(w, []any{})
}

func (s *Service) handleGetStoreItem(w http.ResponseWriter, _ *http.Request) {
	s.writeError(w, "store item lookup not yet available", http.StatusNotImplemented)
}

func (s *Service) handlePurchase(w http.ResponseWriter, _ *http.Request) {
	s.writeError(w, "store purchase not yet available", http.StatusNotImplemented)
}

func (s *Service) handleGetInventory(w http.ResponseWriter, _ *http.Request) {
	s.writeError(w, "inventory lookup not yet available", http.StatusNotImplemented)
}

func (s *Service) handleUseConsumable(w http.ResponseWriter, _ *http.Request) {
	s.writeError(w, "consumable use not yet available", http.StatusNotImplemented)
}

func (s *Service) handleGetEffects(w http.ResponseWriter, _ *http.Request) {
	s.writeError(w, "effects lookup not yet available", http.StatusNotImplemented)
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
