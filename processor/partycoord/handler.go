package partycoord

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
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

// =============================================================================
// QUEST STATE MANAGEMENT (KV Watch — facts about the world)
// =============================================================================

// loadInitialQuestState loads all quests from KV into the local cache.
// This populates the cache before the watcher starts so no state is missed.
func (c *Component) loadInitialQuestState(ctx context.Context) error {
	questEntities, err := c.graph.ListQuestsByPrefix(ctx, 100)
	if err != nil {
		return err
	}

	c.questsMu.Lock()
	for _, entity := range questEntities {
		quest := domain.QuestFromEntityState(&entity)
		if quest != nil {
			instance := domain.ExtractInstance(string(quest.ID))
			c.quests[instance] = quest
		}
	}
	c.questsMu.Unlock()

	c.logger.Debug("loaded initial quest state", "quests", len(questEntities))
	return nil
}

// startQuestWatcher sets up a KV watch on quest entities and starts the
// background goroutine that processes updates.
func (c *Component) startQuestWatcher(ctx context.Context) error {
	kv, err := c.graph.KVBucket(ctx)
	if err != nil {
		return err
	}

	// Watch all quests on this board: org.platform.game.board.quest.>
	questPrefix := c.graph.Config().TypePrefix("quest") + ".>"
	watcher, err := kv.Watch(ctx, questPrefix)
	if err != nil {
		return err
	}
	c.questWatch = watcher

	go c.processQuestWatchUpdates()

	c.logger.Debug("started KV watcher for quest state")
	return nil
}

// stopQuestWatcher stops the KV watcher.
func (c *Component) stopQuestWatcher() {
	if c.questWatch != nil {
		c.questWatch.Stop()
	}
}

// processQuestWatchUpdates consumes KV watch updates for quest state changes.
// Runs in a dedicated goroutine; signals watchDoneCh when it exits.
func (c *Component) processQuestWatchUpdates() {
	defer close(c.watchDoneCh)

	for {
		select {
		case <-c.stopChan:
			return

		case entry, ok := <-c.questWatch.Updates():
			if !ok {
				return
			}
			if entry == nil {
				// nil entry signals initial sync complete — all existing entries delivered
				continue
			}
			c.handleQuestUpdate(entry)
		}
	}
}

// handleQuestUpdate processes a single quest state change from KV.
// Keys in the ENTITY_STATES bucket use the full 6-part entity ID format:
// org.platform.game.board.quest.instance
func (c *Component) handleQuestUpdate(entry jetstream.KeyValueEntry) {
	key := entry.Key()
	instance := domain.ExtractInstance(key)
	if instance == "" || instance == key {
		// Key did not contain a dot separator — not a valid entity ID.
		c.logger.Warn("quest watch entry has unexpected key format", "key", key)
		return
	}

	if entry.Operation() == jetstream.KeyValueDelete {
		c.questsMu.Lock()
		delete(c.quests, instance)
		c.questsMu.Unlock()
		c.logger.Debug("quest removed from cache", "instance", instance)
		return
	}

	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		c.logger.Warn("failed to decode quest entity state", "instance", instance, "error", err)
		return
	}

	quest := domain.QuestFromEntityState(entityState)
	if quest == nil {
		c.logger.Warn("failed to reconstruct quest from entity state", "instance", instance)
		return
	}

	c.questsMu.Lock()
	prev := c.quests[instance]
	c.quests[instance] = quest
	c.questsMu.Unlock()

	c.lastActivity.Store(time.Now())
	c.logger.Debug("quest cache updated", "instance", instance, "status", quest.Status)

	// React to quest state transitions when auto-formation is enabled.
	if c.config.AutoFormParties {
		// A quest moving into "claimed" status and requiring a party triggers formation.
		c.maybeFormParty(prev, quest)
		// A quest posted with party_required=true triggers the full orchestration:
		// find lead → form party → recruit → claim → start.
		c.maybeInitiatePartyQuest(prev, quest)
		// A quest completing may unblock dependent party quests whose initial
		// claim failed due to unmet dependencies. Re-check those now.
		c.retryBlockedPartyQuests(prev, quest)
	}
}

// maybeFormParty inspects the quest state transition and auto-forms a party
// when a party-required quest is claimed and no party has been assigned yet.
func (c *Component) maybeFormParty(prev, curr *domain.Quest) {
	if !curr.PartyRequired {
		return
	}

	// Only react to the transition into "claimed" status
	prevStatus := domain.QuestStatus("")
	if prev != nil {
		prevStatus = prev.Status
	}
	if prevStatus == curr.Status {
		return // No status change; nothing to do
	}
	if curr.Status != domain.QuestClaimed {
		return
	}
	if curr.ClaimedBy == nil {
		return
	}
	if curr.PartyID != nil {
		return // Party already assigned
	}

	ctx := context.Background()
	party, err := c.FormParty(ctx, curr.ID, *curr.ClaimedBy)
	if err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("auto-form party failed",
			"quest_id", curr.ID,
			"agent_id", *curr.ClaimedBy,
			"error", err)
		return
	}

	c.logger.Info("auto-formed party for party-required quest",
		"quest_id", curr.ID,
		"party_id", party.ID,
		"lead_id", *curr.ClaimedBy)
}

// maybeInitiatePartyQuest detects posted party-required quests and orchestrates
// the full formation flow: find a Master-tier lead, form party, recruit members,
// then claim and start the quest. This is the missing connector that bridges
// party-required quest posting to the existing party/DAG execution pipeline.
func (c *Component) maybeInitiatePartyQuest(prev, curr *domain.Quest) {
	if !curr.PartyRequired {
		return
	}

	// Only react to the transition into "posted" status
	prevStatus := domain.QuestStatus("")
	if prev != nil {
		prevStatus = prev.Status
	}
	if prevStatus == curr.Status {
		return
	}
	if curr.Status != domain.QuestPosted {
		return
	}

	ctx := context.Background()

	// Find all idle agents from the graph
	agents, err := c.findIdleAgents(ctx)
	if err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("party quest: failed to find idle agents",
			"quest_id", curr.ID, "error", err)
		return
	}

	// Select a Master-tier lead
	lead, err := selectPartyLead(agents)
	if err != nil {
		c.errorsCount.Add(1)
		c.logger.Warn("party quest: no eligible lead found",
			"quest_id", curr.ID, "idle_agents", len(agents), "error", err)
		return
	}

	// Form the party with the lead
	party, err := c.FormParty(ctx, curr.ID, lead.ID)
	if err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("party quest: failed to form party",
			"quest_id", curr.ID, "lead", lead.ID, "error", err)
		return
	}

	// Recruit members: add idle agents that have relevant skills
	recruited := c.recruitMembers(ctx, party.ID, lead.ID, agents, curr)

	c.logger.Info("party quest: formed party and recruited members",
		"quest_id", curr.ID,
		"party_id", party.ID,
		"lead", lead.ID,
		"members_recruited", recruited)

	// Resolve questboard lazily — it must be running before we can claim.
	qb := c.resolveQuestBoard()
	if qb == nil {
		c.logger.Warn("party quest: questboard unavailable, cannot claim quest",
			"quest_id", curr.ID)
		return
	}

	// Claim the quest for the party (questboard validates lead tier + party size)
	if err := qb.ClaimQuestForParty(ctx, curr.ID, party.ID); err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("party quest: failed to claim quest for party",
			"quest_id", curr.ID, "party_id", party.ID, "error", err)
		return
	}

	// Start the quest (triggers questbridge → decompose → questdagexec)
	if err := qb.StartQuest(ctx, curr.ID); err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("party quest: failed to start quest",
			"quest_id", curr.ID, "error", err)
		return
	}

	c.logger.Info("party quest: initiated full party quest flow",
		"quest_id", curr.ID,
		"party_id", party.ID,
		"lead", lead.ID,
		"total_members", recruited+1)
}

// retryBlockedPartyQuests is called when a quest transitions to "completed".
// It scans the quest cache for posted party quests that depend on the completed
// quest and retries the claim+start flow. This handles the case where a party
// formed but the initial claim failed due to unmet dependencies.
func (c *Component) retryBlockedPartyQuests(prev, completed *domain.Quest) {
	// Only react to transitions INTO completed status.
	if completed.Status != domain.QuestCompleted {
		return
	}
	prevStatus := domain.QuestStatus("")
	if prev != nil {
		prevStatus = prev.Status
	}
	if prevStatus == completed.Status {
		return
	}

	completedID := completed.ID

	// Scan quest cache for posted party quests that depend on this one.
	c.questsMu.RLock()
	var blocked []*domain.Quest
	for _, q := range c.quests {
		if q.Status != domain.QuestPosted || !q.PartyRequired {
			continue
		}
		for _, dep := range q.DependsOn {
			if string(dep) == string(completedID) {
				blocked = append(blocked, q)
				break
			}
		}
	}
	c.questsMu.RUnlock()

	if len(blocked) == 0 {
		return
	}

	ctx := context.Background()
	qb := c.resolveQuestBoard()
	if qb == nil {
		return
	}

	for _, quest := range blocked {
		// If a party already exists (formed earlier but claim failed),
		// retry the claim+start directly.
		if quest.PartyID != nil {
			c.logger.Info("retrying blocked party quest after dependency completed",
				"quest_id", quest.ID,
				"party_id", *quest.PartyID,
				"unblocked_by", completedID)

			if err := qb.ClaimQuestForParty(ctx, quest.ID, *quest.PartyID); err != nil {
				c.logger.Warn("retry: still cannot claim blocked party quest",
					"quest_id", quest.ID, "error", err)
				continue
			}
			if err := qb.StartQuest(ctx, quest.ID); err != nil {
				c.logger.Error("retry: failed to start unblocked party quest",
					"quest_id", quest.ID, "error", err)
				continue
			}
			c.logger.Info("retry: unblocked party quest started",
				"quest_id", quest.ID, "party_id", *quest.PartyID)
		} else {
			// No party yet — trigger the full formation flow.
			// Pass nil as prev to simulate a fresh "posted" transition.
			c.logger.Info("initiating party quest after dependency completed",
				"quest_id", quest.ID, "unblocked_by", completedID)
			c.maybeInitiatePartyQuest(nil, quest)
		}
	}
}

// findIdleAgents lists all agents from the graph and returns those that are idle.
func (c *Component) findIdleAgents(ctx context.Context) ([]agentprogression.Agent, error) {
	entities, err := c.graph.ListEntitiesByType(ctx, "agent", 1000)
	if err != nil {
		return nil, errs.Wrap(err, "PartyCoord", "findIdleAgents", "list agents")
	}

	var idle []agentprogression.Agent
	for _, entity := range entities {
		agent := agentprogression.AgentFromEntityState(&entity)
		if agent == nil {
			continue
		}
		if agent.Status == domain.AgentIdle && agent.CurrentQuest == nil {
			idle = append(idle, *agent)
		}
	}
	return idle, nil
}

// selectPartyLead finds the highest-level Master-tier agent from the list.
func selectPartyLead(agents []agentprogression.Agent) (*agentprogression.Agent, error) {
	var best *agentprogression.Agent
	for _, agent := range agents {
		perms := domain.TierPermissionsFor(domain.TierFromLevel(agent.Level))
		if !perms.CanLeadParty {
			continue
		}
		if best == nil || agent.Level > best.Level {
			agentCopy := agent
			best = &agentCopy
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no Master-tier agent available to lead party")
	}
	return best, nil
}

// recruitMembers adds eligible idle agents to the party as executors.
// Returns the number of members recruited (excluding the lead).
//
// The recruitment target is the larger of MinPartySize and the max parallel
// width of the scenario dependency graph, so the party has enough members to
// saturate concurrent sub-quest execution.
func (c *Component) recruitMembers(ctx context.Context, partyID domain.PartyID, leadID domain.AgentID, agents []agentprogression.Agent, quest *domain.Quest) int {
	recruited := 0
	// Use the widest parallel frontier of the scenario graph as the
	// recruitment target so the party can saturate DAG parallelism.
	minSize := max(quest.MinPartySize, 2) // at least lead + 1 member
	parallelWidth := domain.MaxParallelWidth(quest.Scenarios)
	targetSize := max(minSize, parallelWidth)

	c.logger.Debug("party recruitment target",
		"quest_id", quest.ID,
		"min_party_size", quest.MinPartySize,
		"parallel_width", parallelWidth,
		"target_size", targetSize,
		"available_agents", len(agents)-1) // minus lead

	for _, agent := range agents {
		if agent.ID == leadID {
			continue
		}
		// Already have enough members (lead counts as 1)
		if recruited+1 >= targetSize {
			break
		}

		if err := c.JoinParty(ctx, partyID, agent.ID, domain.RoleExecutor); err != nil {
			c.logger.Warn("party quest: failed to recruit member",
				"party_id", partyID, "agent_id", agent.ID, "error", err)
			continue
		}
		recruited++
	}
	return recruited
}
