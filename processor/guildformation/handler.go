package guildformation

import (
	"context"
	"errors"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// =============================================================================
// KV WATCH HANDLER - Entity-centric agent state monitoring
// =============================================================================
// Replaces the NATS Subscribe to agent XP predicates. Instead of receiving
// pre-built XP event payloads, this processor watches agent entity state in KV
// directly. "Agent progressed" is a fact about the world — it belongs in KV
// Watch, not a NATS subscription.
//
// The watch detects level changes in agents and triggers auto-formation
// clustering when enough agents have progressed to a new tier.
// =============================================================================

// processAgentWatchUpdates handles agent entity state changes from KV.
// Detects level/tier transitions that should trigger guild auto-formation.
func (c *Component) processAgentWatchUpdates() {
	defer close(c.watchDoneCh)

	for {
		select {
		case <-c.stopChan:
			return
		case entry, ok := <-c.agentWatch.Updates():
			if !ok {
				return
			}
			if entry == nil {
				continue // Initial sync complete
			}
			c.handleAgentStateChange(entry)
		}
	}
}

// handleAgentStateChange processes an agent entity state change from KV.
// Detects when an agent levels up and evaluates auto-formation clustering.
func (c *Component) handleAgentStateChange(entry jetstream.KeyValueEntry) {
	if !c.running.Load() {
		return
	}

	key := entry.Key()
	instance := semdragons.ExtractInstance(key)
	if instance == "" || instance == key {
		// Key did not contain a dot separator — not a valid entity ID.
		c.logger.Warn("agent watch entry has unexpected key format", "key", key)
		return
	}

	if entry.Operation() == jetstream.KeyValueDelete {
		c.agentsMu.Lock()
		delete(c.agents, instance)
		c.agentsMu.Unlock()
		c.logger.Debug("agent removed from guildformation cache", "instance", instance)
		return
	}

	// Decode entity state and reconstruct the Agent from its triples.
	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		c.logger.Warn("failed to decode agent entity state", "instance", instance, "error", err)
		return
	}
	agent := semdragons.AgentFromEntityState(entityState)
	if agent == nil {
		c.logger.Warn("failed to reconstruct agent from entity state", "instance", instance)
		return
	}

	// Diff against cached state to detect level changes.
	c.agentsMu.Lock()
	prev, hadPrev := c.agents[instance]
	c.agents[instance] = agent
	c.agentsMu.Unlock()

	if !hadPrev {
		// First time seeing this agent — just cache it, no clustering trigger.
		return
	}

	// React to tier promotions: re-evaluate guild clustering for this agent.
	if prev.Tier != agent.Tier || prev.Level != agent.Level {
		c.logger.Debug("agent progressed, evaluating auto-formation",
			"instance", instance,
			"old_level", prev.Level,
			"new_level", agent.Level,
			"old_tier", prev.Tier,
			"new_tier", agent.Tier)
		c.evaluateAutoFormation(agent)
	}
}

// evaluateAutoFormation checks whether a recently-progressed agent should be
// placed into a guild. It clusters agents by skill specialization when
// MinMembersForFormation unguilded agents share a skill and the auto-formation
// feature is enabled.
func (c *Component) evaluateAutoFormation(agent *semdragons.Agent) {
	if !c.config.EnableAutoFormation {
		return
	}

	// Collect all unguilded agents who share a skill with the progressed agent.
	agentSkills := agent.SkillProficiencies
	if len(agentSkills) == 0 {
		return
	}

	// Check each skill this agent has for potential guild clustering.
	for skill := range agentSkills {
		c.evaluateSkillCluster(agent, domain.SkillTag(skill))
	}
}

// evaluateSkillCluster collects all unguilded agents proficient in the given
// skill. If enough agents meet the threshold, it auto-forms a guild.
func (c *Component) evaluateSkillCluster(trigger *semdragons.Agent, skill domain.SkillTag) {
	// Collect unguilded agents with this skill from the cache.
	c.agentsMu.RLock()
	var candidates []domain.AgentID
	for _, a := range c.agents {
		if _, hasSkill := a.SkillProficiencies[semdragons.SkillTag(skill)]; !hasSkill {
			continue
		}
		// Only include unguilded agents (no existing guild membership in the cache).
		if guilds := c.GetAgentGuilds(a.ID); len(guilds) == 0 {
			candidates = append(candidates, a.ID)
		}
	}
	c.agentsMu.RUnlock()

	if len(candidates) < c.config.MinMembersForFormation {
		return
	}

	// Auto-form a guild with the trigger agent as founder.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	guildName := "Guild of " + string(skill)
	guild, err := c.CreateGuild(ctx, guildName, []domain.SkillTag{skill}, trigger.ID)
	if err != nil {
		c.logger.Error("auto-formation guild creation failed",
			"skill", skill,
			"founder", trigger.ID,
			"error", err)
		c.errorsCount.Add(1)
		return
	}

	c.logger.Info("auto-formed guild from skill cluster",
		"guild_id", guild.ID,
		"skill", skill,
		"candidates", len(candidates))

	// Add remaining candidates (skip the founder who was added by CreateGuild).
	for _, candidateID := range candidates {
		if candidateID == trigger.ID {
			continue
		}
		if err := c.JoinGuild(ctx, guild.ID, candidateID); err != nil {
			c.logger.Warn("failed to add candidate to auto-formed guild",
				"guild_id", guild.ID,
				"agent_id", candidateID,
				"error", err)
		}
	}
}

// =============================================================================
// GUILD HANDLERS
// =============================================================================

// CreateGuild creates a new guild.
func (c *Component) CreateGuild(ctx context.Context, name string, specializations []domain.SkillTag, founderID domain.AgentID) (*Guild, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	now := time.Now()
	instance := domain.GenerateInstance()
	guildID := domain.GuildID(c.boardConfig.GuildEntityID(instance))

	guild := &Guild{
		ID:              guildID,
		Name:            name,
		Status:          domain.GuildActive,
		Specializations: specializations,
		Guildmaster:     founderID,
		Members: []GuildMember{
			{
				AgentID:  founderID,
				Rank:     domain.GuildRankMaster,
				JoinedAt: now,
			},
		},
		FoundedAt: now,
	}

	// Store guild
	c.guilds.Store(guildID, guild)

	// Update agent guild mapping
	c.addAgentGuild(founderID, guildID)

	// Publish guild created event
	if err := SubjectGuildCreated.Publish(ctx, c.deps.NATSClient, GuildCreatedPayload{
		Guild:     *guild,
		FounderID: founderID,
		Timestamp: now,
	}); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "GuildFormation", "CreateGuild", "publish guild created")
	}

	c.guildsCreated.Add(1)
	c.lastActivity.Store(now)

	c.logger.Info("guild created",
		"guild_id", guildID,
		"guild_name", name,
		"founder", founderID)

	return guild, nil
}

// GetGuild returns a guild by ID.
func (c *Component) GetGuild(guildID domain.GuildID) (*Guild, bool) {
	val, ok := c.guilds.Load(guildID)
	if !ok {
		return nil, false
	}
	return val.(*Guild), true
}

// JoinGuild adds an agent to a guild.
func (c *Component) JoinGuild(ctx context.Context, guildID domain.GuildID, agentID domain.AgentID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	val, ok := c.guilds.Load(guildID)
	if !ok {
		return errors.New("guild not found")
	}
	guild := val.(*Guild)

	// Check if already a member
	if guild.IsMember(agentID) {
		return errors.New("already a member")
	}

	// Check max size
	if c.config.MaxGuildSize > 0 && guild.MemberCount() >= c.config.MaxGuildSize {
		return errors.New("guild is full")
	}

	now := time.Now()

	// Add member
	guild.Members = append(guild.Members, GuildMember{
		AgentID:  agentID,
		Rank:     domain.GuildRankInitiate,
		JoinedAt: now,
	})

	// Update agent guild mapping
	c.addAgentGuild(agentID, guildID)

	// Publish join event
	if err := SubjectGuildJoined.Publish(ctx, c.deps.NATSClient, GuildJoinedPayload{
		GuildID:   guildID,
		GuildName: guild.Name,
		AgentID:   agentID,
		Rank:      domain.GuildRankInitiate,
		Timestamp: now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "GuildFormation", "JoinGuild", "publish guild joined")
	}

	c.membersJoined.Add(1)
	c.lastActivity.Store(now)

	c.logger.Info("agent joined guild",
		"guild_id", guildID,
		"agent_id", agentID)

	return nil
}

// LeaveGuild removes an agent from a guild.
func (c *Component) LeaveGuild(ctx context.Context, guildID domain.GuildID, agentID domain.AgentID, reason string) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	val, ok := c.guilds.Load(guildID)
	if !ok {
		return errors.New("guild not found")
	}
	guild := val.(*Guild)

	// Check if a member
	if !guild.IsMember(agentID) {
		return errors.New("not a member")
	}

	// Cannot leave if guildmaster (must transfer first)
	if guild.Guildmaster == agentID {
		return errors.New("guildmaster must transfer leadership before leaving")
	}

	// Remove member
	newMembers := make([]GuildMember, 0, len(guild.Members)-1)
	for _, m := range guild.Members {
		if m.AgentID != agentID {
			newMembers = append(newMembers, m)
		}
	}
	guild.Members = newMembers

	// Remove from officers if applicable
	newOfficers := make([]domain.AgentID, 0)
	for _, officer := range guild.Officers {
		if officer != agentID {
			newOfficers = append(newOfficers, officer)
		}
	}
	guild.Officers = newOfficers

	// Update agent guild mapping
	c.removeAgentGuild(agentID, guildID)

	now := time.Now()

	// Publish leave event
	if err := SubjectGuildLeft.Publish(ctx, c.deps.NATSClient, GuildLeftPayload{
		GuildID:   guildID,
		GuildName: guild.Name,
		AgentID:   agentID,
		Reason:    reason,
		Timestamp: now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "GuildFormation", "LeaveGuild", "publish guild left")
	}

	c.lastActivity.Store(now)

	c.logger.Info("agent left guild",
		"guild_id", guildID,
		"agent_id", agentID,
		"reason", reason)

	return nil
}

// PromoteMember promotes a guild member to a higher rank.
func (c *Component) PromoteMember(ctx context.Context, guildID domain.GuildID, agentID domain.AgentID, newRank domain.GuildRank) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	val, ok := c.guilds.Load(guildID)
	if !ok {
		return errors.New("guild not found")
	}
	guild := val.(*Guild)

	member := guild.GetMember(agentID)
	if member == nil {
		return errors.New("not a member")
	}

	oldRank := member.Rank
	member.Rank = newRank

	// Add to officers if promoted to officer rank
	if newRank == domain.GuildRankOfficer && !guild.IsOfficer(agentID) {
		guild.Officers = append(guild.Officers, agentID)
	}

	now := time.Now()

	// Publish promotion event
	if err := SubjectGuildPromoted.Publish(ctx, c.deps.NATSClient, GuildPromotedPayload{
		GuildID:   guildID,
		GuildName: guild.Name,
		AgentID:   agentID,
		OldRank:   oldRank,
		NewRank:   newRank,
		Timestamp: now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "GuildFormation", "PromoteMember", "publish promotion")
	}

	c.promotionsCount.Add(1)
	c.lastActivity.Store(now)

	c.logger.Info("member promoted",
		"guild_id", guildID,
		"agent_id", agentID,
		"old_rank", oldRank,
		"new_rank", newRank)

	return nil
}

// DisbandGuild disbands a guild.
func (c *Component) DisbandGuild(ctx context.Context, guildID domain.GuildID, reason string) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	val, ok := c.guilds.Load(guildID)
	if !ok {
		return errors.New("guild not found")
	}
	guild := val.(*Guild)

	now := time.Now()
	guild.Status = domain.GuildInactive
	guild.DisbandedAt = &now

	// Remove all agent guild mappings
	for _, member := range guild.Members {
		c.removeAgentGuild(member.AgentID, guildID)
	}

	// Publish disband event
	if err := SubjectGuildDisbanded.Publish(ctx, c.deps.NATSClient, GuildDisbandedPayload{
		GuildID:          guildID,
		GuildName:        guild.Name,
		Reason:           reason,
		FinalMemberCount: len(guild.Members),
		Timestamp:        now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "GuildFormation", "DisbandGuild", "publish guild disbanded")
	}

	c.lastActivity.Store(now)

	c.logger.Info("guild disbanded",
		"guild_id", guildID,
		"reason", reason,
		"final_members", len(guild.Members))

	return nil
}

// GetAgentGuilds returns all guilds an agent belongs to.
func (c *Component) GetAgentGuilds(agentID domain.AgentID) []domain.GuildID {
	val, ok := c.agentGuilds.Load(agentID)
	if !ok {
		return nil
	}
	return val.([]domain.GuildID)
}

// ListGuilds returns all active guilds.
func (c *Component) ListGuilds() []*Guild {
	var guilds []*Guild
	c.guilds.Range(func(_, value any) bool {
		guild := value.(*Guild)
		if guild.Status == domain.GuildActive {
			guilds = append(guilds, guild)
		}
		return true
	})
	return guilds
}

// =============================================================================
// INTERNAL HELPERS
// =============================================================================

// addAgentGuild adds a guild to an agent's guild list.
func (c *Component) addAgentGuild(agentID domain.AgentID, guildID domain.GuildID) {
	val, ok := c.agentGuilds.Load(agentID)
	var guilds []domain.GuildID
	if ok {
		guilds = val.([]domain.GuildID)
	}
	guilds = append(guilds, guildID)
	c.agentGuilds.Store(agentID, guilds)
}

// removeAgentGuild removes a guild from an agent's guild list.
func (c *Component) removeAgentGuild(agentID domain.AgentID, guildID domain.GuildID) {
	val, ok := c.agentGuilds.Load(agentID)
	if !ok {
		return
	}
	guilds := val.([]domain.GuildID)
	newGuilds := make([]domain.GuildID, 0, len(guilds)-1)
	for _, g := range guilds {
		if g != guildID {
			newGuilds = append(newGuilds, g)
		}
	}
	if len(newGuilds) > 0 {
		c.agentGuilds.Store(agentID, newGuilds)
	} else {
		c.agentGuilds.Delete(agentID)
	}
}
