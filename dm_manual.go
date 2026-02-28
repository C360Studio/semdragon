package semdragons

import (
	"context"
	"fmt"
	"time"
)

// =============================================================================
// MANUAL DUNGEON MASTER - Human makes all decisions
// =============================================================================
// ManualDungeonMaster implements the DungeonMaster interface where a human
// makes all critical decisions. The DM provides tools and suggestions
// (e.g., boids rankings), but all actions require explicit human approval.
//
// This is the simplest DM mode and serves as the foundation for more
// automated modes (Supervised, Assisted, FullAuto).
// =============================================================================

// ManualDungeonMaster implements DungeonMaster with human-in-the-loop decisions.
type ManualDungeonMaster struct {
	*BaseDungeonMaster
	approvalRouter ApprovalRouter
	partyEngine    *PartyFormationEngine
}

// ManualDMConfig holds configuration for creating a ManualDungeonMaster.
type ManualDMConfig struct {
	BaseDMConfig
	ApprovalRouter ApprovalRouter
}

// NewManualDungeonMaster creates a new manual DM.
func NewManualDungeonMaster(cfg ManualDMConfig) *ManualDungeonMaster {
	base := NewBaseDungeonMaster(cfg.BaseDMConfig)

	dm := &ManualDungeonMaster{
		BaseDungeonMaster: base,
		approvalRouter:    cfg.ApprovalRouter,
		partyEngine:       NewPartyFormationEngine(base.boids, base.storage),
	}

	// Use mock router if none provided
	if dm.approvalRouter == nil {
		mock := NewMockApprovalRouter()
		mock.SetAutoApprove(true)
		dm.approvalRouter = mock
	}

	return dm
}

// Compile-time interface compliance check
var _ DungeonMaster = (*ManualDungeonMaster)(nil)

// getActiveSessionID returns the current session ID.
// Returns empty string if no active sessions exist (logs warning).
func (dm *ManualDungeonMaster) getActiveSessionID() string {
	sessions := dm.ListActiveSessions()
	if len(sessions) == 0 {
		dm.logger.Warn("no active session for approval request")
		return ""
	}
	if len(sessions) > 1 {
		dm.logger.Warn("multiple active sessions, using first",
			"count", len(sessions),
			"using", sessions[0].ID)
	}
	return sessions[0].ID
}

// =============================================================================
// QUEST MANAGEMENT
// =============================================================================

// CreateQuest crafts a quest from a high-level objective.
// In manual mode, all parameters are decided by the human.
func (dm *ManualDungeonMaster) CreateQuest(ctx context.Context, objective string, hints QuestHints) (*Quest, error) {
	sessionID := dm.getActiveSessionID()

	// Create approval request with suggestions
	suggestion := dm.suggestQuestParameters(objective, hints)
	req := NewQuestCreateApproval(sessionID, objective, hints, suggestion)

	// Request approval from human
	resp, err := dm.approvalRouter.RequestApproval(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("approval request failed: %w", err)
	}

	if !resp.Approved {
		return nil, fmt.Errorf("quest creation rejected: %s", resp.Reason)
	}

	// Build quest from response (using suggestion or overrides)
	quest := dm.buildQuestFromApproval(objective, suggestion, resp)

	// Post to board
	return dm.board.PostQuest(ctx, quest)
}

// ReviewQuestDecomposition approves or modifies a party lead's sub-quest breakdown.
func (dm *ManualDungeonMaster) ReviewQuestDecomposition(ctx context.Context, parentID QuestID, subQuests []Quest) ([]Quest, error) {
	sessionID := dm.getActiveSessionID()

	// Load parent quest for context
	parent, err := dm.board.GetQuest(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("load parent quest: %w", err)
	}

	// Create approval request
	req := ApprovalRequest{
		ID:        GenerateInstance(),
		SessionID: sessionID,
		Type:      ApprovalQuestDecomposition,
		Title:     "Review Quest Decomposition",
		Details:   fmt.Sprintf("Parent: %s, Sub-quests: %d", parent.Title, len(subQuests)),
		Payload: map[string]any{
			"parent":     *parent,
			"sub_quests": subQuests,
		},
		Options: []ApprovalOption{
			{ID: "approve", Label: "Approve", IsDefault: true},
			{ID: "modify", Label: "Modify"},
			{ID: "reject", Label: "Reject"},
		},
		CreatedAt: time.Now(),
	}

	resp, err := dm.approvalRouter.RequestApproval(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("approval request failed: %w", err)
	}

	if !resp.Approved {
		return nil, fmt.Errorf("decomposition rejected: %s", resp.Reason)
	}

	// Return approved sub-quests (potentially modified via overrides)
	return subQuests, nil
}

// =============================================================================
// AGENT MANAGEMENT
// =============================================================================

// RecruitAgent brings a new agent into the world at level 1.
func (dm *ManualDungeonMaster) RecruitAgent(ctx context.Context, config AgentConfig) (*Agent, error) {
	sessionID := dm.getActiveSessionID()

	// Create approval request
	req := NewAgentRecruitApproval(sessionID, config)

	resp, err := dm.approvalRouter.RequestApproval(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("approval request failed: %w", err)
	}

	if !resp.Approved || resp.SelectedID == "deny" {
		return nil, fmt.Errorf("agent recruitment rejected: %s", resp.Reason)
	}

	// Create new agent at level 1
	instance := GenerateInstance()
	agentID := AgentID(dm.config.AgentEntityID(instance))

	// Calculate XP to next level (default 100 for level 1 if no progression manager)
	xpToLevel := int64(100)
	if dm.progression != nil && dm.progression.xpEngine != nil {
		xpToLevel = dm.progression.xpEngine.XPToNextLevel(1)
	}

	agent := &Agent{
		ID:        agentID,
		Name:      config.Provider + "/" + config.Model,
		Status:    AgentIdle,
		Level:     1,
		XP:        0,
		XPToLevel: xpToLevel,
		Tier:      TierApprentice,
		Config:    config,
		Stats:     AgentStats{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Store agent
	if err := dm.storage.PutAgent(ctx, instance, agent); err != nil {
		return nil, fmt.Errorf("store agent: %w", err)
	}

	dm.logger.Info("agent recruited",
		"agent_id", agentID,
		"model", config.Model,
		"provider", config.Provider,
	)

	return agent, nil
}

// RetireAgent permanently removes an agent.
func (dm *ManualDungeonMaster) RetireAgent(ctx context.Context, agentID AgentID, reason string) error {
	sessionID := dm.getActiveSessionID()

	// Load agent for approval context
	instance := ExtractInstance(string(agentID))
	agent, err := dm.storage.GetAgent(ctx, instance)
	if err != nil {
		return fmt.Errorf("load agent: %w", err)
	}

	// Create approval request
	req := ApprovalRequest{
		ID:        GenerateInstance(),
		SessionID: sessionID,
		Type:      ApprovalAgentRetire,
		Title:     "Retire Agent",
		Details:   fmt.Sprintf("Agent: %s, Level: %d, Reason: %s", agent.Name, agent.Level, reason),
		Payload:   agent,
		Options: []ApprovalOption{
			{ID: "approve", Label: "Retire", Description: "Permanently retire this agent"},
			{ID: "deny", Label: "Cancel"},
		},
		CreatedAt: time.Now(),
	}

	resp, err := dm.approvalRouter.RequestApproval(ctx, req)
	if err != nil {
		return fmt.Errorf("approval request failed: %w", err)
	}

	if !resp.Approved || resp.SelectedID == "deny" {
		return fmt.Errorf("agent retirement cancelled")
	}

	// Retire the agent
	return dm.storage.UpdateAgent(ctx, instance, func(a *Agent) error {
		a.Status = AgentRetired
		a.UpdatedAt = time.Now()
		return nil
	})
}

// EvaluateAgent runs an ad-hoc assessment of an agent's performance.
func (dm *ManualDungeonMaster) EvaluateAgent(ctx context.Context, agentID AgentID) (*AgentEvaluation, error) {
	instance := ExtractInstance(string(agentID))
	agent, err := dm.storage.GetAgent(ctx, instance)
	if err != nil {
		return nil, fmt.Errorf("load agent: %w", err)
	}

	// Compute evaluation based on stats
	eval := &AgentEvaluation{
		AgentID:          agentID,
		CurrentLevel:     agent.Level,
		RecommendedLevel: agent.Level,
	}

	// Analyze strengths and weaknesses
	if agent.Stats.AvgQualityScore >= 0.8 {
		eval.Strengths = append(eval.Strengths, "High quality output")
	}
	if agent.Stats.AvgEfficiency >= 0.8 {
		eval.Strengths = append(eval.Strengths, "Efficient execution")
	}
	if agent.Stats.BossesDefeated > agent.Stats.BossesFailed*2 {
		eval.Strengths = append(eval.Strengths, "Strong review performance")
	}

	if agent.Stats.AvgQualityScore < 0.5 {
		eval.Weaknesses = append(eval.Weaknesses, "Quality needs improvement")
	}
	if agent.Stats.QuestsFailed > agent.Stats.QuestsCompleted {
		eval.Weaknesses = append(eval.Weaknesses, "High failure rate")
	}

	// Recommendation based on performance
	totalBattles := agent.Stats.BossesDefeated + agent.Stats.BossesFailed
	if totalBattles > 0 {
		successRate := float64(agent.Stats.BossesDefeated) / float64(totalBattles)
		if successRate >= 0.8 && agent.Stats.QuestsCompleted >= 5 {
			eval.Recommendation = "promote"
			eval.RecommendedLevel = agent.Level + 1
		} else if successRate < 0.4 && totalBattles >= 3 {
			eval.Recommendation = "demote"
			eval.RecommendedLevel = max(1, agent.Level-1)
		} else {
			eval.Recommendation = "maintain"
		}
	} else {
		eval.Recommendation = "maintain"
	}

	return eval, nil
}

// =============================================================================
// PARTY MANAGEMENT
// =============================================================================

// FormParty assembles a party for a quest.
func (dm *ManualDungeonMaster) FormParty(ctx context.Context, questID QuestID, strategy PartyStrategy) (*Party, error) {
	sessionID := dm.getActiveSessionID()

	// Load quest
	quest, err := dm.board.GetQuest(ctx, questID)
	if err != nil {
		return nil, fmt.Errorf("load quest: %w", err)
	}

	// Get available agents
	agents, err := dm.GetIdleAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("load agents: %w", err)
	}

	// Compute boids suggestions
	suggestions := dm.partyEngine.RankAgentsForQuest(agents, quest)

	// Create approval request with boids suggestions
	req := NewPartyFormationApproval(sessionID, *quest, suggestions)

	resp, err := dm.approvalRouter.RequestApproval(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("approval request failed: %w", err)
	}

	if !resp.Approved {
		return nil, fmt.Errorf("party formation rejected: %s", resp.Reason)
	}

	// Form party based on approval (use selected agents or defaults)
	party, err := dm.partyEngine.FormParty(ctx, quest, strategy, agents)
	if err != nil {
		return nil, fmt.Errorf("form party: %w", err)
	}

	// Store party
	partyInstance := ExtractInstance(string(party.ID))
	if err := dm.storage.PutParty(ctx, partyInstance, party); err != nil {
		return nil, fmt.Errorf("store party: %w", err)
	}

	dm.logger.Info("party formed",
		"party_id", party.ID,
		"quest_id", questID,
		"lead", party.Lead,
		"members", len(party.Members),
	)

	return party, nil
}

// =============================================================================
// INTERVENTION
// =============================================================================

// Intervene allows the DM to step into any ongoing quest.
func (dm *ManualDungeonMaster) Intervene(ctx context.Context, questID QuestID, action Intervention) error {
	sessionID := dm.getActiveSessionID()

	// Load quest
	quest, err := dm.board.GetQuest(ctx, questID)
	if err != nil {
		return fmt.Errorf("load quest: %w", err)
	}

	// Create approval request
	req := NewInterventionApproval(sessionID, *quest, &action)

	resp, err := dm.approvalRouter.RequestApproval(ctx, req)
	if err != nil {
		return fmt.Errorf("approval request failed: %w", err)
	}

	if !resp.Approved {
		return fmt.Errorf("intervention cancelled")
	}

	// Execute intervention based on selected type
	interventionType := InterventionType(resp.SelectedID)
	if interventionType == "" {
		interventionType = action.Type
	}

	switch interventionType {
	case InterventionAssist:
		// Record assist hint (stored in quest or separate log)
		dm.logger.Info("intervention: assist", "quest_id", questID, "reason", action.Reason)

	case InterventionRedirect:
		// Could modify quest parameters or reassign
		dm.logger.Info("intervention: redirect", "quest_id", questID, "reason", action.Reason)

	case InterventionTakeover:
		// DM completes the quest directly
		dm.logger.Info("intervention: takeover", "quest_id", questID, "reason", action.Reason)
		// Would typically involve DM providing the output

	case InterventionAbort:
		// Cancel the quest
		return dm.board.EscalateQuest(ctx, questID, "DM intervention: "+action.Reason)

	case InterventionAugment:
		// Add resources to the quest
		dm.logger.Info("intervention: augment", "quest_id", questID, "reason", action.Reason)
	}

	return nil
}

// HandleEscalation deals with escalated quests.
func (dm *ManualDungeonMaster) HandleEscalation(ctx context.Context, questID QuestID) (*EscalationResult, error) {
	sessionID := dm.getActiveSessionID()

	// Load quest
	quest, err := dm.board.GetQuest(ctx, questID)
	if err != nil {
		return nil, fmt.Errorf("load quest: %w", err)
	}

	// Create approval request
	req := NewEscalationApproval(sessionID, *quest, nil)

	resp, err := dm.approvalRouter.RequestApproval(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("approval request failed: %w", err)
	}

	if !resp.Approved {
		return nil, fmt.Errorf("escalation decision cancelled")
	}

	result := &EscalationResult{
		QuestID: questID,
	}

	switch resp.SelectedID {
	case "reassign":
		// Re-post the quest for another agent/party
		result.Resolution = "reassigned"
		// Would need to reset quest status and re-post

	case "decompose":
		// Break into sub-quests (would need human to provide breakdown)
		result.Resolution = "decomposed"

	case "dm_complete":
		// DM handles it directly
		result.Resolution = "completed_by_dm"
		result.DMCompleted = true

	case "cancel":
		// Cancel the quest
		questInstance := ExtractInstance(string(questID))
		if err := dm.storage.UpdateQuest(ctx, questInstance, func(q *Quest) error {
			q.Status = QuestCancelled
			return nil
		}); err != nil {
			return nil, fmt.Errorf("cancel quest: %w", err)
		}
		result.Resolution = "cancelled"
	}

	dm.logger.Info("escalation handled",
		"quest_id", questID,
		"resolution", result.Resolution,
	)

	return result, nil
}

// HandleBossBattle runs or delegates a boss battle for a completed quest.
func (dm *ManualDungeonMaster) HandleBossBattle(ctx context.Context, questID QuestID, submission any) (*BossBattle, error) {
	sessionID := dm.getActiveSessionID()

	// Verify quest exists
	_, err := dm.board.GetQuest(ctx, questID)
	if err != nil {
		return nil, fmt.Errorf("load quest: %w", err)
	}

	// Submit result to board (triggers automatic evaluation)
	battle, err := dm.board.SubmitResult(ctx, questID, submission)
	if err != nil {
		return nil, fmt.Errorf("submit result: %w", err)
	}

	// If battle has a verdict (from automated evaluation), route to human for confirmation
	if battle != nil && battle.Verdict != nil {
		req := NewBattleVerdictApproval(sessionID, *battle, *battle.Verdict)

		resp, err := dm.approvalRouter.RequestApproval(ctx, req)
		if err != nil {
			return battle, fmt.Errorf("approval request failed: %w", err)
		}

		// Handle override if selected
		if resp.SelectedID == "override_pass" && !battle.Verdict.Passed {
			battle.Verdict.Passed = true
			battle.Verdict.Feedback = "DM override: passed"
			battle.Status = BattleVictory
			if err := dm.board.CompleteQuest(ctx, questID, *battle.Verdict); err != nil {
				return battle, fmt.Errorf("complete quest after override: %w", err)
			}
		} else if resp.SelectedID == "override_fail" && battle.Verdict.Passed {
			battle.Verdict.Passed = false
			battle.Verdict.Feedback = "DM override: failed"
			battle.Status = BattleDefeat
			if err := dm.board.FailQuest(ctx, questID, "DM override: failed"); err != nil {
				return battle, fmt.Errorf("fail quest after override: %w", err)
			}
		}
	}

	return battle, nil
}

// =============================================================================
// HELPER METHODS
// =============================================================================

func (dm *ManualDungeonMaster) suggestQuestParameters(_ string, hints QuestHints) *QuestDecision {
	decision := &QuestDecision{
		Difficulty:  DifficultyModerate,
		ReviewLevel: ReviewStandard,
		Reasoning:   "Default suggestion based on hints",
	}

	if hints.SuggestedDifficulty != nil {
		decision.Difficulty = *hints.SuggestedDifficulty
	}
	decision.BaseXP = DefaultXPForDifficulty(decision.Difficulty)

	if len(hints.SuggestedSkills) > 0 {
		decision.RequiredSkills = hints.SuggestedSkills
	}

	if hints.PreferGuild != nil {
		decision.GuildPriority = hints.PreferGuild
	}

	if hints.RequireHumanReview {
		decision.ReviewLevel = ReviewHuman
	}

	return decision
}

func (dm *ManualDungeonMaster) buildQuestFromApproval(objective string, suggestion *QuestDecision, resp *ApprovalResponse) Quest {
	// Start with suggestion
	builder := NewQuest(objective).
		Description(objective).
		Difficulty(suggestion.Difficulty).
		XP(suggestion.BaseXP).
		ReviewAs(suggestion.ReviewLevel)

	if len(suggestion.RequiredSkills) > 0 {
		builder.RequireSkills(suggestion.RequiredSkills...)
	}

	if suggestion.GuildPriority != nil {
		builder.GuildPriority(*suggestion.GuildPriority)
	}

	if suggestion.PartyRequired {
		builder.RequireParty(suggestion.MinPartySize)
	}

	// Apply any overrides from approval response
	if resp.Overrides != nil {
		if diff, ok := resp.Overrides["difficulty"].(int); ok {
			builder.Difficulty(QuestDifficulty(diff))
		}
		if xp, ok := resp.Overrides["base_xp"].(int64); ok {
			builder.XP(xp)
		}
	}

	return builder.Build()
}
