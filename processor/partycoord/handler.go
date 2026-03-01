package partycoord

import (
	"context"
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/pkg/errs"
)

// =============================================================================
// PARTY HANDLERS
// =============================================================================

// FormParty creates a new party for a quest.
func (c *Component) FormParty(ctx context.Context, questID domain.QuestID, leadID domain.AgentID) (*Party, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	partyID := domain.PartyID(c.boardConfig.EntityID("party", c.generateID()))
	now := time.Now()

	party := &Party{
		ID:            partyID,
		Name:          "", // Will be set by lead or auto-generated
		Status:        domain.PartyForming,
		QuestID:       questID,
		Lead:          leadID,
		Members:       []PartyMember{{AgentID: leadID, Role: domain.RoleLead, JoinedAt: now}},
		SubQuestMap:   make(map[domain.QuestID]domain.AgentID),
		SharedContext: []ContextItem{},
		SubResults:    make(map[domain.QuestID]any),
		FormedAt:      now,
	}

	// Store party
	c.activeParties.Store(partyID, party)

	// Emit to graph
	if err := c.graph.EmitEntity(ctx, party, "party.formed"); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "PartyCoord", "FormParty", "emit party entity")
	}

	// Publish party formed event
	if err := SubjectPartyFormed.Publish(ctx, c.deps.NATSClient, PartyFormedPayload{
		Party:    *party,
		FormedAt: now,
	}); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "PartyCoord", "FormParty", "publish party formed")
	}

	c.partiesFormed.Add(1)
	c.lastActivity.Store(now)

	c.logger.Info("party formed",
		"party_id", partyID,
		"quest_id", questID,
		"lead_id", leadID)

	return party, nil
}

// JoinParty adds a member to a party.
func (c *Component) JoinParty(ctx context.Context, partyID domain.PartyID, agentID domain.AgentID, role domain.PartyRole) error {
	val, ok := c.activeParties.Load(partyID)
	if !ok {
		return errors.New("party not found")
	}

	party := val.(*Party)
	now := time.Now()

	// Add member
	member := PartyMember{
		AgentID:  agentID,
		Role:     role,
		JoinedAt: now,
	}
	party.Members = append(party.Members, member)

	// Emit updated party state
	if err := c.graph.EmitEntity(ctx, party, "party.joined"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "PartyCoord", "JoinParty", "emit party entity")
	}

	// Publish join event
	if err := SubjectPartyJoined.Publish(ctx, c.deps.NATSClient, PartyJoinedPayload{
		PartyID:  partyID,
		AgentID:  agentID,
		Role:     role,
		JoinedAt: now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "PartyCoord", "JoinParty", "publish party joined")
	}

	c.lastActivity.Store(now)

	c.logger.Info("member joined party",
		"party_id", partyID,
		"agent_id", agentID,
		"role", role)

	return nil
}

// DisbandParty disbands a party.
func (c *Component) DisbandParty(ctx context.Context, partyID domain.PartyID, reason string) error {
	val, ok := c.activeParties.Load(partyID)
	if !ok {
		return errors.New("party not found")
	}

	party := val.(*Party)
	now := time.Now()

	party.Status = domain.PartyDisbanded
	party.DisbandedAt = &now

	// Remove from active parties
	c.activeParties.Delete(partyID)

	// Emit final state
	if err := c.graph.EmitEntity(ctx, party, "party.disbanded"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "PartyCoord", "DisbandParty", "emit party entity")
	}

	// Publish disband event
	if err := SubjectPartyDisbanded.Publish(ctx, c.deps.NATSClient, PartyDisbandedPayload{
		PartyID:     partyID,
		QuestID:     party.QuestID,
		Reason:      reason,
		DisbandedAt: now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "PartyCoord", "DisbandParty", "publish party disbanded")
	}

	c.partiesDisbanded.Add(1)
	c.lastActivity.Store(now)

	c.logger.Info("party disbanded",
		"party_id", partyID,
		"reason", reason)

	return nil
}

// GetParty returns a party by ID.
func (c *Component) GetParty(partyID domain.PartyID) (*Party, bool) {
	val, ok := c.activeParties.Load(partyID)
	if !ok {
		return nil, false
	}
	return val.(*Party), true
}

// ListActiveParties returns all active parties.
func (c *Component) ListActiveParties() []*Party {
	var parties []*Party
	c.activeParties.Range(func(_, value any) bool {
		if p, ok := value.(*Party); ok {
			parties = append(parties, p)
		}
		return true
	})
	return parties
}

// =============================================================================
// COORDINATION HANDLERS
// =============================================================================

// DecomposeQuest records quest decomposition by the lead.
func (c *Component) DecomposeQuest(ctx context.Context, partyID domain.PartyID, subQuests []domain.QuestID, strategy string) error {
	val, ok := c.activeParties.Load(partyID)
	if !ok {
		return errors.New("party not found")
	}

	party := val.(*Party)
	now := time.Now()

	party.Strategy = strategy

	// Publish decomposition event
	if err := SubjectPartyQuestDecomposed.Publish(ctx, c.deps.NATSClient, PartyQuestDecomposedPayload{
		PartyID:     partyID,
		LeadID:      party.Lead,
		ParentQuest: party.QuestID,
		SubQuests:   subQuests,
		Strategy:    strategy,
		Timestamp:   now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "PartyCoord", "DecomposeQuest", "publish quest decomposed")
	}

	c.lastActivity.Store(now)

	return nil
}

// AssignTask assigns a sub-quest to a party member.
func (c *Component) AssignTask(ctx context.Context, partyID domain.PartyID, subQuestID domain.QuestID, assignedTo domain.AgentID, rationale string) error {
	val, ok := c.activeParties.Load(partyID)
	if !ok {
		return errors.New("party not found")
	}

	party := val.(*Party)
	now := time.Now()

	// Record assignment
	party.SubQuestMap[subQuestID] = assignedTo

	// Publish assignment event
	if err := SubjectPartyTaskAssigned.Publish(ctx, c.deps.NATSClient, PartyTaskAssignedPayload{
		PartyID:    partyID,
		LeadID:     party.Lead,
		AssignedTo: assignedTo,
		SubQuestID: subQuestID,
		Rationale:  rationale,
		Timestamp:  now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "PartyCoord", "AssignTask", "publish task assigned")
	}

	c.lastActivity.Store(now)

	return nil
}

// SubmitResult records a member's sub-quest result.
func (c *Component) SubmitResult(ctx context.Context, partyID domain.PartyID, memberID domain.AgentID, subQuestID domain.QuestID, result any) error {
	val, ok := c.activeParties.Load(partyID)
	if !ok {
		return errors.New("party not found")
	}

	party := val.(*Party)
	now := time.Now()

	// Record result
	party.SubResults[subQuestID] = result

	// Publish result submission event
	if err := SubjectPartyResultSubmitted.Publish(ctx, c.deps.NATSClient, PartyResultSubmittedPayload{
		PartyID:    partyID,
		MemberID:   memberID,
		SubQuestID: subQuestID,
		Result:     result,
		Timestamp:  now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "PartyCoord", "SubmitResult", "publish result submitted")
	}

	c.lastActivity.Store(now)

	return nil
}

// StartRollup begins the rollup process.
func (c *Component) StartRollup(ctx context.Context, partyID domain.PartyID) error {
	val, ok := c.activeParties.Load(partyID)
	if !ok {
		return errors.New("party not found")
	}

	party := val.(*Party)
	now := time.Now()

	// Publish rollup started event
	if err := SubjectPartyRollupStarted.Publish(ctx, c.deps.NATSClient, PartyRollupStartedPayload{
		PartyID:         partyID,
		LeadID:          party.Lead,
		ParentQuestID:   party.QuestID,
		SubResultsCount: len(party.SubResults),
		Timestamp:       now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "PartyCoord", "StartRollup", "publish rollup started")
	}

	c.lastActivity.Store(now)

	return nil
}

// CompleteRollup completes the rollup process with the combined result.
func (c *Component) CompleteRollup(ctx context.Context, partyID domain.PartyID, rollupResult any) error {
	val, ok := c.activeParties.Load(partyID)
	if !ok {
		return errors.New("party not found")
	}

	party := val.(*Party)
	now := time.Now()

	party.RollupResult = rollupResult

	// Publish rollup completed event
	if err := SubjectPartyRollupCompleted.Publish(ctx, c.deps.NATSClient, PartyRollupCompletedPayload{
		PartyID:       partyID,
		LeadID:        party.Lead,
		ParentQuestID: party.QuestID,
		RollupResult:  rollupResult,
		Timestamp:     now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "PartyCoord", "CompleteRollup", "publish rollup completed")
	}

	c.rollupsCompleted.Add(1)
	c.lastActivity.Store(now)

	return nil
}

// =============================================================================
// HELPERS
// =============================================================================

// generateID generates a unique ID for a party.
func (c *Component) generateID() string {
	return time.Now().Format("20060102-150405-") + randomSuffix()
}

// randomSuffix generates a random alphanumeric suffix.
func randomSuffix() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}
