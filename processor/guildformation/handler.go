package guildformation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/pkg/errs"
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

	// Update the founder's agent entity so its Guild field reflects the new
	// membership. Without this, downstream watchers (boid engine, autonomy)
	// see the founder as unguilded and keep triggering guild creation.
	c.updateAgentGuild(ctx, params.FounderID, guildID, true)

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

	// Check if agent already belongs to a guild (single-guild constraint).
	if existing := c.GetAgentGuild(agentID); existing != "" {
		if existing == guildID {
			return ErrAlreadyMember
		}
		return fmt.Errorf("agent already belongs to guild %s", existing)
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

	// Update the agent entity so its Guild field reflects the new membership.
	c.updateAgentGuild(ctx, agentID, guildID, true)

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

	// Update the agent entity to clear its Guild field.
	c.updateAgentGuild(ctx, agentID, guildID, false)

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

// updateAgentGuild loads the agent entity and sets or clears the Guild field,
// then persists the update. Best-effort — guild entity is the source of truth;
// this keeps the agent entity in sync for UI display.
func (c *Component) updateAgentGuild(ctx context.Context, agentID domain.AgentID, guildID domain.GuildID, add bool) {
	agentEntity, err := c.graph.GetAgent(ctx, agentID)
	if err != nil {
		c.logger.Warn("updateAgentGuild: failed to load agent", "agent_id", agentID, "error", err)
		return
	}
	agent := agentprogression.AgentFromEntityState(agentEntity)
	if agent == nil {
		c.logger.Warn("updateAgentGuild: agent reconstruction returned nil", "agent_id", agentID)
		return
	}

	if add {
		agent.Guild = guildID
	} else {
		agent.Guild = ""
	}

	if err := c.graph.EmitEntityUpdate(ctx, agent, "agent.membership.updated"); err != nil {
		c.logger.Warn("updateAgentGuild: failed to persist", "agent_id", agentID, "error", err)
	} else {
		c.logger.Info("updateAgentGuild: agent guild updated",
			"agent_id", agentID, "guild_id", guildID, "add", add)
	}
}

// GetAgentGuild returns the guild ID the agent belongs to, or "" if unguilded.
func (c *Component) GetAgentGuild(agentID domain.AgentID) domain.GuildID {
	val, ok := c.agentGuilds.Load(agentID)
	if !ok {
		return ""
	}
	return val.(domain.GuildID)
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

// addAgentGuild records that the agent now belongs to the given guild.
func (c *Component) addAgentGuild(agentID domain.AgentID, guildID domain.GuildID) {
	c.agentGuilds.Store(agentID, guildID)
}

// removeAgentGuild clears the agent's guild membership.
func (c *Component) removeAgentGuild(agentID domain.AgentID, _ domain.GuildID) {
	// The guildID parameter is accepted for call-site compatibility but agents
	// have at most one guild, so we always clear the entire entry on removal.
	c.agentGuilds.Delete(agentID)
}
