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

// Sentinel errors for guild operations.
var (
	ErrAlreadyMember = errors.New("already a member")
	ErrGuildFull     = errors.New("guild is full")
)

// =============================================================================
// KV WATCH HANDLER - Entity-centric agent state monitoring
// =============================================================================
// Watches agent entity state in KV directly. "Agent progressed" is a fact about
// the world -- it belongs in KV Watch, not a NATS subscription.
//
// The watch detects level changes in agents and triggers social-model auto-formation
// when enough unguilded agents exist and a qualified founder is available.
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
// Detects when an agent levels up and evaluates auto-formation.
func (c *Component) handleAgentStateChange(entry jetstream.KeyValueEntry) {
	if !c.running.Load() {
		return
	}

	key := entry.Key()
	instance := semdragons.ExtractInstance(key)
	if instance == "" || instance == key {
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
		return
	}

	// React to tier promotions: re-evaluate guild formation for this agent.
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

// evaluateAutoFormation checks whether enough unguilded agents exist to form a
// guild using the social model. Instead of clustering by shared skill, this
// seeds diverse guilds led by an Expert+ founder.
func (c *Component) evaluateAutoFormation(trigger *semdragons.Agent) {
	if !c.config.EnableAutoFormation {
		return
	}

	// Gate: founder must be Expert+ tier (level 11+) -- founding is a leadership act.
	if trigger.Tier < domain.TierExpert {
		return
	}

	// Only unguilded agents can found a guild.
	if guilds := c.GetAgentGuilds(trigger.ID); len(guilds) > 0 {
		return
	}

	// Collect all unguilded agents from the cache.
	c.agentsMu.RLock()
	var candidates []*semdragons.Agent
	for _, a := range c.agents {
		if guilds := c.GetAgentGuilds(a.ID); len(guilds) == 0 {
			candidates = append(candidates, a)
		}
	}
	c.agentsMu.RUnlock()

	if len(candidates) < c.config.MinMembersForFormation {
		return
	}

	// Diversity: pick candidates with different primary skills to seed mixed composition.
	selected := selectDiverseCandidates(candidates, c.config.MinMembersForFormation, trigger.ID)
	if len(selected) < c.config.MinMembersForFormation {
		// Not enough diverse candidates; fall back to whatever we have.
		selected = candidates
		if len(selected) > c.config.MinMembersForFormation {
			selected = selected[:c.config.MinMembersForFormation]
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	guild, err := c.CreateGuild(ctx, CreateGuildParams{
		Name:      generateGuildName(trigger),
		Culture:   "Founded through demonstrated expertise",
		FounderID: trigger.ID,
		MinLevel:  1,
	})
	if err != nil {
		c.logger.Error("auto-formation guild creation failed",
			"founder", trigger.ID,
			"error", err)
		c.errorsCount.Add(1)
		return
	}

	c.logger.Info("auto-formed guild via social model",
		"guild_id", guild.ID,
		"guild_name", guild.Name,
		"candidates", len(selected))

	// Add remaining candidates (skip the founder who was added by CreateGuild).
	for _, candidate := range selected {
		if candidate.ID == trigger.ID {
			continue
		}
		if err := c.JoinGuild(ctx, guild.ID, candidate.ID); err != nil {
			c.logger.Warn("failed to add candidate to auto-formed guild",
				"guild_id", guild.ID,
				"agent_id", candidate.ID,
				"error", err)
		}
	}
}

// selectDiverseCandidates picks agents with different primary skills, always
// including the founder. Returns up to `count` agents.
func selectDiverseCandidates(candidates []*semdragons.Agent, count int, founderID domain.AgentID) []*semdragons.Agent {
	selected := make([]*semdragons.Agent, 0, count)
	seenSkills := make(map[semdragons.SkillTag]bool)

	// Always include the founder first.
	for _, c := range candidates {
		if c.ID == founderID {
			selected = append(selected, c)
			for skill := range c.SkillProficiencies {
				seenSkills[skill] = true
			}
			break
		}
	}

	// Add candidates whose primary skill is not yet represented.
	for _, c := range candidates {
		if len(selected) >= count {
			break
		}
		if c.ID == founderID {
			continue
		}
		hasNewSkill := false
		for skill := range c.SkillProficiencies {
			if !seenSkills[skill] {
				hasNewSkill = true
				break
			}
		}
		if hasNewSkill {
			selected = append(selected, c)
			for skill := range c.SkillProficiencies {
				seenSkills[skill] = true
			}
		}
	}

	return selected
}

// generateGuildName creates a guild name from the founder's identity.
func generateGuildName(founder *semdragons.Agent) string {
	name := founder.Name
	if founder.DisplayName != "" {
		name = founder.DisplayName
	}
	return name + "'s Guild"
}

// =============================================================================
// GUILD HANDLERS
// =============================================================================

// CreateGuildParams holds parameters for creating a new guild.
type CreateGuildParams struct {
	Name      string
	Culture   string
	Motto     string
	FounderID domain.AgentID
	MinLevel  int
}

// CreateGuild creates a new guild using the social-construct model.
func (c *Component) CreateGuild(ctx context.Context, params CreateGuildParams) (*semdragons.Guild, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	now := time.Now()
	instance := domain.GenerateInstance()
	guildID := domain.GuildID(c.boardConfig.GuildEntityID(instance))

	guild := &semdragons.Guild{
		ID:          semdragons.GuildID(guildID),
		Name:        params.Name,
		Description: "",
		Status:      domain.GuildActive,
		Culture:     params.Culture,
		Motto:       params.Motto,
		MinLevel:    params.MinLevel,
		MaxMembers:  c.config.MaxGuildSize,
		FoundedBy:   semdragons.AgentID(params.FounderID),
		Founded:     now,
		Members: []semdragons.GuildMember{
			{
				AgentID:  semdragons.AgentID(params.FounderID),
				Rank:     domain.GuildRankMaster,
				JoinedAt: now,
			},
		},
		Reputation:  0.5, // Start neutral
		SuccessRate: 0.0,
		CreatedAt:   now,
	}

	// Store guild in memory
	c.guilds.Store(guildID, guild)

	// Update agent guild mapping
	c.addAgentGuild(params.FounderID, guildID)

	// Persist to KV via graph client
	if err := c.graph.EmitEntity(ctx, guild, "guild.created"); err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to persist guild to KV", "guild_id", guildID, "error", err)
		// Continue -- in-memory state is valid, KV will catch up on next mutation.
	}

	// Publish guild created event
	if err := SubjectGuildCreated.Publish(ctx, c.deps.NATSClient, GuildCreatedPayload{
		Guild:     *guild,
		FounderID: params.FounderID,
		Timestamp: now,
	}); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "GuildFormation", "CreateGuild", "publish guild created")
	}

	c.guildsCreated.Add(1)
	c.lastActivity.Store(now)

	c.logger.Info("guild created",
		"guild_id", guildID,
		"guild_name", params.Name,
		"culture", params.Culture,
		"founder", params.FounderID)

	return guild, nil
}

// GetGuild returns a guild by ID.
func (c *Component) GetGuild(guildID domain.GuildID) (*semdragons.Guild, bool) {
	val, ok := c.guilds.Load(guildID)
	if !ok {
		return nil, false
	}
	return val.(*semdragons.Guild), true
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
	guild := val.(*semdragons.Guild)

	// Check if already a member
	if isMember(guild, semdragons.AgentID(agentID)) {
		return ErrAlreadyMember
	}

	// Check max size
	if c.config.MaxGuildSize > 0 && len(guild.Members) >= c.config.MaxGuildSize {
		return ErrGuildFull
	}

	now := time.Now()

	// Add member
	guild.Members = append(guild.Members, semdragons.GuildMember{
		AgentID:  semdragons.AgentID(agentID),
		Rank:     domain.GuildRankInitiate,
		JoinedAt: now,
	})

	// Update agent guild mapping
	c.addAgentGuild(agentID, guildID)

	// Persist updated guild to KV
	if err := c.graph.EmitEntity(ctx, guild, "guild.member.joined"); err != nil {
		c.logger.Error("failed to persist guild update to KV", "guild_id", guildID, "error", err)
	}

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
	guild := val.(*semdragons.Guild)

	// Check if a member
	if !isMember(guild, semdragons.AgentID(agentID)) {
		return errors.New("not a member")
	}

	// Cannot leave if founder/guildmaster (must transfer first)
	if guild.FoundedBy == semdragons.AgentID(agentID) {
		return errors.New("guildmaster must transfer leadership before leaving")
	}

	// Remove member
	newMembers := make([]semdragons.GuildMember, 0, len(guild.Members)-1)
	for _, m := range guild.Members {
		if m.AgentID != semdragons.AgentID(agentID) {
			newMembers = append(newMembers, m)
		}
	}
	guild.Members = newMembers

	// Update agent guild mapping
	c.removeAgentGuild(agentID, guildID)

	now := time.Now()

	// Persist updated guild to KV
	if err := c.graph.EmitEntity(ctx, guild, "guild.member.left"); err != nil {
		c.logger.Error("failed to persist guild update to KV", "guild_id", guildID, "error", err)
	}

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
	guild := val.(*semdragons.Guild)

	member := getMember(guild, semdragons.AgentID(agentID))
	if member == nil {
		return errors.New("not a member")
	}

	oldRank := member.Rank
	member.Rank = newRank

	now := time.Now()

	// Persist updated guild to KV
	if err := c.graph.EmitEntity(ctx, guild, "guild.member.promoted"); err != nil {
		c.logger.Error("failed to persist guild update to KV", "guild_id", guildID, "error", err)
	}

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
	guild := val.(*semdragons.Guild)

	now := time.Now()
	guild.Status = domain.GuildInactive

	// Remove all agent guild mappings
	for _, member := range guild.Members {
		c.removeAgentGuild(domain.AgentID(member.AgentID), guildID)
	}

	// Persist updated guild to KV
	if err := c.graph.EmitEntity(ctx, guild, "guild.disbanded"); err != nil {
		c.logger.Error("failed to persist guild disbandment to KV", "guild_id", guildID, "error", err)
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

// ListGuilds returns all active guilds as shallow copies with independent
// Members and QuestTypes slices, safe for concurrent read without locks.
func (c *Component) ListGuilds() []*semdragons.Guild {
	var guilds []*semdragons.Guild
	c.guilds.Range(func(_, value any) bool {
		original := value.(*semdragons.Guild)
		if original.Status == domain.GuildActive {
			cp := *original
			cp.Members = append([]semdragons.GuildMember(nil), original.Members...)
			cp.QuestTypes = append([]string(nil), original.QuestTypes...)
			guilds = append(guilds, &cp)
		}
		return true
	})
	return guilds
}

// =============================================================================
// INTERNAL HELPERS
// =============================================================================

// isMember checks if an agent is a member of a guild.
func isMember(guild *semdragons.Guild, agentID semdragons.AgentID) bool {
	return getMember(guild, agentID) != nil
}

// getMember returns a pointer to a member in the guild's Members slice, or nil.
func getMember(guild *semdragons.Guild, agentID semdragons.AgentID) *semdragons.GuildMember {
	for i := range guild.Members {
		if guild.Members[i].AgentID == agentID {
			return &guild.Members[i]
		}
	}
	return nil
}

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
