package guildformation

import (
	"context"
	"errors"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// Sentinel errors for guild operations.
var (
	ErrAlreadyMember        = errors.New("already a member")
	ErrGuildFull            = errors.New("guild is full")
	ErrGuildNotPending      = errors.New("guild is not in pending state")
	ErrNotFounder           = errors.New("only the founder can review applications")
	ErrDuplicateApplication = errors.New("application already exists")
	ErrApplicationNotFound  = errors.New("application not found")
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
	instance := domain.ExtractInstance(key)
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
	agent := agentprogression.AgentFromEntityState(entityState)
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
		// New agent appeared. Check if this triggers auto-formation:
		// either this agent is Expert+ (potential founder) or a cached
		// Expert+ unguilded agent now has enough candidates.
		if c.config.EnableAutoFormation {
			if agent.Tier >= domain.TierExpert {
				c.evaluateAutoFormation(agent)
			} else {
				c.evaluateAutoFormationForCachedFounders()
			}
		}
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

// evaluateAutoFormationForCachedFounders scans the agent cache for any Expert+
// unguilded agent and re-evaluates auto-formation. Called when a new non-Expert
// agent appears, since it may be the Nth candidate that tips the threshold.
func (c *Component) evaluateAutoFormationForCachedFounders() {
	c.agentsMu.RLock()
	var founders []*agentprogression.Agent
	for _, a := range c.agents {
		if a.Tier >= domain.TierExpert {
			if guilds := c.GetAgentGuilds(a.ID); len(guilds) == 0 {
				founders = append(founders, a)
			}
		}
	}
	c.agentsMu.RUnlock()

	for _, founder := range founders {
		c.evaluateAutoFormation(founder)
	}
}

// evaluateAutoFormation checks whether enough unguilded agents exist to form a
// guild using the social model. Instead of clustering by shared skill, this
// seeds diverse guilds led by an Expert+ founder.
func (c *Component) evaluateAutoFormation(trigger *agentprogression.Agent) {
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
	var candidates []*agentprogression.Agent
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
		"status", guild.Status,
		"candidates", len(selected))

	// In quorum mode, candidates must apply via autonomy — don't auto-add them.
	if c.config.EnableQuorumFormation {
		return
	}

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
func selectDiverseCandidates(candidates []*agentprogression.Agent, count int, founderID domain.AgentID) []*agentprogression.Agent {
	selected := make([]*agentprogression.Agent, 0, count)
	seenSkills := make(map[domain.SkillTag]bool)

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
func generateGuildName(founder *agentprogression.Agent) string {
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
func (c *Component) CreateGuild(ctx context.Context, params CreateGuildParams) (*domain.Guild, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	now := time.Now()
	instance := domain.GenerateInstance()
	guildID := domain.GuildID(c.boardConfig.GuildEntityID(instance))

	status := domain.GuildActive
	var quorumSize int
	var deadline *time.Time
	eventPredicate := "guild.created"

	if c.config.EnableQuorumFormation {
		status = domain.GuildPending
		quorumSize = c.config.MinFoundingMembers
		t := now.Add(time.Duration(c.config.FormationTimeoutSec) * time.Second)
		deadline = &t
		eventPredicate = domain.PredicateGuildPending
	}

	guild := &domain.Guild{
		ID:                domain.GuildID(guildID),
		Name:              params.Name,
		Description:       "",
		Status:            status,
		Culture:           params.Culture,
		Motto:             params.Motto,
		MinLevel:          params.MinLevel,
		MaxMembers:        c.config.MaxGuildSize,
		FoundedBy:         domain.AgentID(params.FounderID),
		Founded:           now,
		QuorumSize:        quorumSize,
		FormationDeadline: deadline,
		Members: []domain.GuildMember{
			{
				AgentID:  domain.AgentID(params.FounderID),
				Rank:     domain.GuildRankMaster,
				JoinedAt: now,
			},
		},
		Reputation:  0.5, // Start neutral
		SuccessRate: 0.0,
		CreatedAt:   now,
	}

	// Store guild in memory — no mutex needed for a new guild since no other
	// goroutine can reference this ID yet.
	c.guilds.Store(guildID, guild)

	// Update agent guild mapping
	c.addAgentGuild(params.FounderID, guildID)

	// Persist to KV via graph client
	if err := c.graph.EmitEntity(ctx, guild, eventPredicate); err != nil {
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

	// Return a copy so the caller cannot mutate internal state.
	cp := *guild
	cp.Members = append([]domain.GuildMember(nil), guild.Members...)
	return &cp, nil
}

// GetGuild returns a copy of a guild by ID, safe for concurrent use.
func (c *Component) GetGuild(guildID domain.GuildID) (*domain.Guild, bool) {
	val, ok := c.guilds.Load(guildID)
	if !ok {
		return nil, false
	}
	mu := c.guildMutex(guildID)
	mu.Lock()
	original := val.(*domain.Guild)
	cp := *original
	cp.Members = append([]domain.GuildMember(nil), original.Members...)
	cp.Applications = append([]domain.GuildApplication(nil), original.Applications...)
	cp.QuestTypes = append([]string(nil), original.QuestTypes...)
	mu.Unlock()
	return &cp, true
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

	mu := c.guildMutex(guildID)
	mu.Lock()
	guild := val.(*domain.Guild)

	// Check if already a member
	if isMember(guild, agentID) {
		mu.Unlock()
		return ErrAlreadyMember
	}

	// Check max size
	if c.config.MaxGuildSize > 0 && len(guild.Members) >= c.config.MaxGuildSize {
		mu.Unlock()
		return ErrGuildFull
	}

	now := time.Now()

	// Add member
	guild.Members = append(guild.Members, domain.GuildMember{
		AgentID:  agentID,
		Rank:     domain.GuildRankInitiate,
		JoinedAt: now,
	})
	mu.Unlock()

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

	// Update the agent entity so its Guilds field reflects the new membership.
	c.updateAgentGuilds(ctx, agentID, guildID, true)

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

	mu := c.guildMutex(guildID)
	mu.Lock()
	guild := val.(*domain.Guild)

	// Check if a member
	if !isMember(guild, agentID) {
		mu.Unlock()
		return errors.New("not a member")
	}

	// Cannot leave if founder/guildmaster (must transfer first)
	if guild.FoundedBy == agentID {
		mu.Unlock()
		return errors.New("guildmaster must transfer leadership before leaving")
	}

	// Remove member
	newMembers := make([]domain.GuildMember, 0, len(guild.Members)-1)
	for _, m := range guild.Members {
		if m.AgentID != agentID {
			newMembers = append(newMembers, m)
		}
	}
	guild.Members = newMembers
	mu.Unlock()

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

	// Update the agent entity to remove the guild from its Guilds field.
	c.updateAgentGuilds(ctx, agentID, guildID, false)

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

	mu := c.guildMutex(guildID)
	mu.Lock()
	guild := val.(*domain.Guild)

	member := getMember(guild, agentID)
	if member == nil {
		mu.Unlock()
		return errors.New("not a member")
	}

	oldRank := member.Rank
	member.Rank = newRank
	mu.Unlock()

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

	mu := c.guildMutex(guildID)
	mu.Lock()
	guild := val.(*domain.Guild)

	now := time.Now()
	guild.Status = domain.GuildInactive

	// Snapshot members for cleanup outside the lock.
	members := append([]domain.GuildMember(nil), guild.Members...)
	mu.Unlock()

	// Remove all agent guild mappings
	for _, member := range members {
		c.removeAgentGuild(member.AgentID, guildID)
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
		FinalMemberCount: len(members),
		Timestamp:        now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "GuildFormation", "DisbandGuild", "publish guild disbanded")
	}

	c.lastActivity.Store(now)

	c.logger.Info("guild disbanded",
		"guild_id", guildID,
		"reason", reason,
		"final_members", len(members))

	return nil
}

// updateAgentGuilds loads the agent entity and adds or removes a guild ID
// from its Guilds field, then persists the update. Best-effort — guild entity
// is the source of truth; this keeps the agent entity in sync for UI display.
func (c *Component) updateAgentGuilds(ctx context.Context, agentID domain.AgentID, guildID domain.GuildID, add bool) {
	agentEntity, err := c.graph.GetAgent(ctx, agentID)
	if err != nil {
		c.logger.Debug("updateAgentGuilds: failed to load agent", "agent_id", agentID, "error", err)
		return
	}
	agent := agentprogression.AgentFromEntityState(agentEntity)
	if agent == nil {
		return
	}

	if add {
		// Avoid duplicates
		for _, g := range agent.Guilds {
			if g == guildID {
				return
			}
		}
		agent.Guilds = append(agent.Guilds, guildID)
	} else {
		filtered := make([]domain.GuildID, 0, len(agent.Guilds))
		for _, g := range agent.Guilds {
			if g != guildID {
				filtered = append(filtered, g)
			}
		}
		agent.Guilds = filtered
	}

	if err := c.graph.EmitEntityUpdate(ctx, agent, "agent.membership.updated"); err != nil {
		c.logger.Debug("updateAgentGuilds: failed to persist", "agent_id", agentID, "error", err)
	}
}

// GetAgentGuilds returns a copy of the guild IDs an agent belongs to.
func (c *Component) GetAgentGuilds(agentID domain.AgentID) []domain.GuildID {
	c.agentGuildsMu.Lock()
	val, ok := c.agentGuilds.Load(agentID)
	if !ok {
		c.agentGuildsMu.Unlock()
		return nil
	}
	guilds := val.([]domain.GuildID)
	cp := append([]domain.GuildID(nil), guilds...)
	c.agentGuildsMu.Unlock()
	return cp
}

// SubmitApplication submits an agent's application to join a pending guild.
func (c *Component) SubmitApplication(ctx context.Context, guildID domain.GuildID, agent *agentprogression.Agent, message string) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	val, ok := c.guilds.Load(guildID)
	if !ok {
		return errors.New("guild not found")
	}

	mu := c.guildMutex(guildID)
	mu.Lock()
	guild := val.(*domain.Guild)

	if guild.Status != domain.GuildPending {
		mu.Unlock()
		return ErrGuildNotPending
	}

	if isMember(guild, agent.ID) {
		mu.Unlock()
		return ErrAlreadyMember
	}

	// Check for duplicate pending application
	for _, app := range guild.Applications {
		if app.ApplicantID == agent.ID && app.Status == domain.ApplicationPending {
			mu.Unlock()
			return ErrDuplicateApplication
		}
	}

	now := time.Now()
	appID := domain.GenerateInstance()

	// Collect agent skills
	var skills []domain.SkillTag
	for skill := range agent.SkillProficiencies {
		skills = append(skills, skill)
	}

	app := domain.GuildApplication{
		ID:          appID,
		GuildID:     guildID,
		ApplicantID: agent.ID,
		Status:      domain.ApplicationPending,
		Message:     message,
		Skills:      skills,
		Level:       agent.Level,
		Tier:        agent.Tier,
		AppliedAt:   now,
	}

	guild.Applications = append(guild.Applications, app)
	mu.Unlock()

	// Persist to KV
	if err := c.graph.EmitEntity(ctx, guild, domain.PredicateGuildApplicationSubmitted); err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to persist guild application", "guild_id", guildID, "error", err)
	}

	c.lastActivity.Store(now)
	c.logger.Info("application submitted to guild",
		"guild_id", guildID,
		"applicant", agent.ID,
		"app_id", appID)

	return nil
}

// ReviewApplication accepts or rejects a pending application. Only the founder can review.
func (c *Component) ReviewApplication(ctx context.Context, guildID domain.GuildID, applicationID string, founderID domain.AgentID, accepted bool, reason string) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	val, ok := c.guilds.Load(guildID)
	if !ok {
		return errors.New("guild not found")
	}

	mu := c.guildMutex(guildID)
	mu.Lock()
	guild := val.(*domain.Guild)

	if guild.Status != domain.GuildPending {
		mu.Unlock()
		return ErrGuildNotPending
	}

	if guild.FoundedBy != founderID {
		mu.Unlock()
		return ErrNotFounder
	}

	// Find the application
	var app *domain.GuildApplication
	for i := range guild.Applications {
		if guild.Applications[i].ID == applicationID {
			app = &guild.Applications[i]
			break
		}
	}
	if app == nil {
		mu.Unlock()
		return ErrApplicationNotFound
	}
	if app.Status != domain.ApplicationPending {
		mu.Unlock()
		return errors.New("application already reviewed")
	}

	now := time.Now()
	app.ReviewedBy = &founderID
	app.Reason = reason
	app.ReviewedAt = &now

	var applicantID domain.AgentID
	predicate := domain.PredicateGuildApplicationRejected
	if accepted {
		app.Status = domain.ApplicationAccepted
		predicate = domain.PredicateGuildApplicationAccepted
		applicantID = app.ApplicantID

		// Add applicant as guild member
		guild.Members = append(guild.Members, domain.GuildMember{
			AgentID:  applicantID,
			Rank:     domain.GuildRankInitiate,
			JoinedAt: now,
		})
	} else {
		app.Status = domain.ApplicationRejected
	}
	mu.Unlock()

	// Agent guild mapping and metrics outside lock
	if accepted {
		c.addAgentGuild(applicantID, guildID)
		c.membersJoined.Add(1)
	}

	// Persist to KV
	if err := c.graph.EmitEntity(ctx, guild, predicate); err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to persist application review", "guild_id", guildID, "error", err)
	}

	c.lastActivity.Store(now)

	verb := "rejected"
	if accepted {
		verb = "accepted"
	}
	c.logger.Info("application reviewed",
		"guild_id", guildID,
		"app_id", applicationID,
		"applicant", applicantID,
		"decision", verb,
		"reason", reason)

	// Check if quorum is reached after acceptance
	if accepted {
		c.checkQuorumLocked(ctx, guildID, guild)
	}

	return nil
}

// checkQuorumLocked transitions a pending guild to active if the quorum is met.
// Acquires the per-guild mutex internally.
func (c *Component) checkQuorumLocked(ctx context.Context, guildID domain.GuildID, guild *domain.Guild) {
	mu := c.guildMutex(guildID)
	mu.Lock()
	if guild.Status != domain.GuildPending {
		mu.Unlock()
		return
	}
	if len(guild.Members) < guild.QuorumSize {
		mu.Unlock()
		return
	}

	guild.Status = domain.GuildActive
	guild.FormationDeadline = nil
	mu.Unlock()

	if err := c.graph.EmitEntity(ctx, guild, domain.PredicateGuildActivated); err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to persist guild activation", "guild_id", guild.ID, "error", err)
	}

	c.logger.Info("guild reached quorum and activated",
		"guild_id", guild.ID,
		"guild_name", guild.Name,
		"members", len(guild.Members),
		"quorum", guild.QuorumSize)
}

// dissolveGuild dissolves a pending guild that failed to reach quorum.
func (c *Component) dissolveGuild(ctx context.Context, guild *domain.Guild, reason string) {
	mu := c.guildMutex(guild.ID)
	mu.Lock()

	guild.Status = domain.GuildInactive

	// Reject all remaining pending applications
	now := time.Now()
	for i := range guild.Applications {
		if guild.Applications[i].Status == domain.ApplicationPending {
			guild.Applications[i].Status = domain.ApplicationRejected
			guild.Applications[i].Reason = reason
			guild.Applications[i].ReviewedAt = &now
		}
	}

	// Snapshot members for cleanup outside the lock.
	members := append([]domain.GuildMember(nil), guild.Members...)
	mu.Unlock()

	// Remove all agent guild mappings
	for _, member := range members {
		c.removeAgentGuild(member.AgentID, guild.ID)
	}

	if err := c.graph.EmitEntity(ctx, guild, domain.PredicateGuildDissolved); err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to persist guild dissolution", "guild_id", guild.ID, "error", err)
	}

	c.lastActivity.Store(now)
	c.logger.Info("pending guild dissolved",
		"guild_id", guild.ID,
		"guild_name", guild.Name,
		"reason", reason)
}

// runFormationTimeoutLoop periodically checks pending guilds and dissolves
// any that have passed their formation deadline.
func (c *Component) runFormationTimeoutLoop() {
	defer close(c.timeoutDoneCh)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.checkFormationTimeouts()
		}
	}
}

// checkFormationTimeouts scans for pending guilds past their deadline.
func (c *Component) checkFormationTimeouts() {
	now := time.Now()

	// Collect expired guilds first, then dissolve outside the Range to avoid
	// holding sync.Map's internal lock during dissolveGuild's I/O.
	var expired []*domain.Guild
	c.guilds.Range(func(key, value any) bool {
		guildID := key.(domain.GuildID)
		mu := c.guildMutex(guildID)
		mu.Lock()
		guild := value.(*domain.Guild)
		isExpired := guild.Status == domain.GuildPending &&
			guild.FormationDeadline != nil &&
			now.After(*guild.FormationDeadline)
		mu.Unlock()
		if isExpired {
			expired = append(expired, guild)
		}
		return true
	})

	for _, guild := range expired {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		c.dissolveGuild(ctx, guild, "formation timeout: quorum not met")
		cancel()
	}
}

// HasPendingGuilds returns true if at least one pending guild exists.
// More efficient than ListPendingGuilds for guard checks.
func (c *Component) HasPendingGuilds() bool {
	found := false
	c.guilds.Range(func(key, value any) bool {
		guildID := key.(domain.GuildID)
		mu := c.guildMutex(guildID)
		mu.Lock()
		isPending := value.(*domain.Guild).Status == domain.GuildPending
		mu.Unlock()
		if isPending {
			found = true
			return false // stop iteration
		}
		return true
	})
	return found
}

// ListPendingGuilds returns all pending guilds as shallow copies.
func (c *Component) ListPendingGuilds() []*domain.Guild {
	var guilds []*domain.Guild
	c.guilds.Range(func(key, value any) bool {
		guildID := key.(domain.GuildID)
		mu := c.guildMutex(guildID)
		mu.Lock()
		original := value.(*domain.Guild)
		if original.Status == domain.GuildPending {
			cp := *original
			cp.Members = append([]domain.GuildMember(nil), original.Members...)
			cp.Applications = append([]domain.GuildApplication(nil), original.Applications...)
			cp.QuestTypes = append([]string(nil), original.QuestTypes...)
			guilds = append(guilds, &cp)
		}
		mu.Unlock()
		return true
	})
	return guilds
}

// ListGuilds returns all active guilds as shallow copies with independent
// Members and QuestTypes slices, safe for concurrent read without locks.
func (c *Component) ListGuilds() []*domain.Guild {
	var guilds []*domain.Guild
	c.guilds.Range(func(key, value any) bool {
		guildID := key.(domain.GuildID)
		mu := c.guildMutex(guildID)
		mu.Lock()
		original := value.(*domain.Guild)
		if original.Status == domain.GuildActive {
			cp := *original
			cp.Members = append([]domain.GuildMember(nil), original.Members...)
			cp.QuestTypes = append([]string(nil), original.QuestTypes...)
			guilds = append(guilds, &cp)
		}
		mu.Unlock()
		return true
	})
	return guilds
}

// =============================================================================
// INTERNAL HELPERS
// =============================================================================

// isMember checks if an agent is a member of a guild.
func isMember(guild *domain.Guild, agentID domain.AgentID) bool {
	return getMember(guild, agentID) != nil
}

// getMember returns a pointer to a member in the guild's Members slice, or nil.
func getMember(guild *domain.Guild, agentID domain.AgentID) *domain.GuildMember {
	for i := range guild.Members {
		if guild.Members[i].AgentID == agentID {
			return &guild.Members[i]
		}
	}
	return nil
}

// addAgentGuild adds a guild to an agent's guild list.
// Uses agentGuildsMu to prevent TOCTOU races on Load-then-Store.
func (c *Component) addAgentGuild(agentID domain.AgentID, guildID domain.GuildID) {
	c.agentGuildsMu.Lock()
	defer c.agentGuildsMu.Unlock()

	val, ok := c.agentGuilds.Load(agentID)
	var guilds []domain.GuildID
	if ok {
		guilds = val.([]domain.GuildID)
	}
	guilds = append(guilds, guildID)
	c.agentGuilds.Store(agentID, guilds)
}

// removeAgentGuild removes a guild from an agent's guild list.
// Uses agentGuildsMu to prevent TOCTOU races on Load-then-Store.
func (c *Component) removeAgentGuild(agentID domain.AgentID, guildID domain.GuildID) {
	c.agentGuildsMu.Lock()
	defer c.agentGuildsMu.Unlock()

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
