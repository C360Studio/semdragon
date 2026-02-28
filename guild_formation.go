package semdragons

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// =============================================================================
// GUILD FORMATION - Social organization with natural diversity pressure
// =============================================================================
// Guilds are social organizations with mixed composition. Natural diversity
// pressure comes from quest requirements: quests need diverse skills, so
// homogeneous guilds fail quests and suffer reputation consequences.
//
// Formation is social:
//   - FoundGuild: Expert+ agent creates guild (costs XP)
//   - InviteToGuild: Officers+ invite agents
//   - ApplyToGuild: Agents apply, Officers+ approve
//   - LeaveGuild: Agents can leave (lose library access)
//
// Legacy cluster detection methods are retained for analysis/suggestions.
// =============================================================================

// GuildFormationEngine manages guild social formation and membership.
type GuildFormationEngine interface {
	// FoundGuild creates a new guild. Requires Expert tier (level 11+) and costs XP.
	FoundGuild(ctx context.Context, founderID AgentID, name, culture string) (*Guild, error)

	// InviteToGuild sends an invitation to an agent. Requires Officer+ rank.
	InviteToGuild(ctx context.Context, inviterID AgentID, guildID GuildID, inviteeID AgentID) error

	// ApplyToGuild submits an application to join a guild.
	ApplyToGuild(ctx context.Context, applicantID AgentID, guildID GuildID) error

	// ApproveApplication approves a pending application. Requires Officer+ rank.
	ApproveApplication(ctx context.Context, approverID AgentID, guildID GuildID, applicantID AgentID) error

	// LeaveGuild removes an agent from a guild.
	LeaveGuild(ctx context.Context, agentID AgentID, guildID GuildID) error

	// PromoteMember promotes a member to a higher rank. Requires appropriate rank.
	PromoteMember(ctx context.Context, promoterID AgentID, guildID GuildID, memberID AgentID, newRank GuildRank) error

	// DetectSkillClusters analyzes agents and suggests potential guild formations.
	// This is for analysis/suggestion purposes, not auto-formation.
	DetectSkillClusters(ctx context.Context, agents []*Agent) []GuildSuggestion

	// EvaluateGuildDiversity calculates how well a guild covers required skill combinations.
	EvaluateGuildDiversity(ctx context.Context, guildID GuildID) (*GuildDiversityReport, error)
}

// GuildFormationConfig holds tunable parameters for guild formation.
type GuildFormationConfig struct {
	// Founding requirements
	MinFounderLevel int   // Minimum level to found guild (default: 11, Expert tier)
	FoundingXPCost  int64 // XP cost to found guild (default: 500)

	// Membership constraints
	DefaultMaxMembers int // Default max members per guild (default: 20)

	// Legacy clustering parameters (for suggestions)
	MinClusterSize      int     // Minimum agents for a cluster (default: 3)
	MinClusterStrength  float64 // Minimum Jaccard similarity (default: 0.6)
	MinAgentLevel       int     // Minimum agent level for guild consideration (default: 3)
	RequireQualityScore float64 // Minimum avg quality score (default: 0.5)
}

// DefaultFormationConfig returns sensible defaults for guild formation.
func DefaultFormationConfig() GuildFormationConfig {
	return GuildFormationConfig{
		MinFounderLevel:     11, // Expert tier
		FoundingXPCost:      500,
		DefaultMaxMembers:   20,
		MinClusterSize:      3,
		MinClusterStrength:  0.6,
		MinAgentLevel:       3,
		RequireQualityScore: 0.5,
	}
}

// Validate checks that config values are sensible.
func (c GuildFormationConfig) Validate() error {
	if c.MinFounderLevel < 1 {
		return errors.New("MinFounderLevel must be at least 1")
	}
	if c.FoundingXPCost < 0 {
		return errors.New("FoundingXPCost cannot be negative")
	}
	if c.DefaultMaxMembers < 1 {
		return errors.New("DefaultMaxMembers must be at least 1")
	}
	if c.MinClusterSize < 1 {
		return errors.New("MinClusterSize must be at least 1")
	}
	if c.MinClusterStrength < 0 || c.MinClusterStrength > 1 {
		return errors.New("MinClusterStrength must be between 0 and 1")
	}
	if c.MinAgentLevel < 1 {
		return errors.New("MinAgentLevel must be at least 1")
	}
	if c.RequireQualityScore < 0 || c.RequireQualityScore > 1 {
		return errors.New("RequireQualityScore must be between 0 and 1")
	}
	return nil
}

// GuildDiversityReport shows how well a guild covers skill combinations.
type GuildDiversityReport struct {
	GuildID        GuildID    `json:"guild_id"`
	TotalMembers   int        `json:"total_members"`
	UniqueSkills   []SkillTag `json:"unique_skills"`
	SkillCoverage  float64    `json:"skill_coverage"`  // 0-1, unique skills / total known skills
	SkillGaps      []SkillTag `json:"skill_gaps"`      // Skills frequently required but not covered
	DiversityScore float64    `json:"diversity_score"` // 0-1, overall diversity measure
}

// GuildInvitation represents a pending guild invitation.
type GuildInvitation struct {
	GuildID   GuildID   `json:"guild_id"`
	InviterID AgentID   `json:"inviter_id"`
	InviteeID AgentID   `json:"invitee_id"`
	InvitedAt time.Time `json:"invited_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// GuildApplication represents a pending guild application.
type GuildApplication struct {
	GuildID     GuildID   `json:"guild_id"`
	ApplicantID AgentID   `json:"applicant_id"`
	AppliedAt   time.Time `json:"applied_at"`
	Message     string    `json:"message,omitempty"`
}

// DefaultGuildFormationEngine implements GuildFormationEngine.
type DefaultGuildFormationEngine struct {
	storage *Storage
	events  *EventPublisher
	config  GuildFormationConfig
	logger  *slog.Logger
}

// NewGuildFormationEngine creates a new formation engine with the given config.
func NewGuildFormationEngine(storage *Storage, events *EventPublisher, config GuildFormationConfig) *DefaultGuildFormationEngine {
	return &DefaultGuildFormationEngine{
		storage: storage,
		events:  events,
		config:  config,
		logger:  slog.Default(),
	}
}

// WithLogger sets a custom logger for the engine.
func (e *DefaultGuildFormationEngine) WithLogger(l *slog.Logger) *DefaultGuildFormationEngine {
	e.logger = l
	return e
}

// =============================================================================
// SOCIAL FORMATION METHODS
// =============================================================================

// FoundGuild creates a new guild. The founder becomes Guildmaster.
// Requirements:
//   - Founder must be Expert tier (level 11+)
//   - Costs 500 XP to found (investment, not free)
func (e *DefaultGuildFormationEngine) FoundGuild(ctx context.Context, founderID AgentID, name, culture string) (*Guild, error) {
	if e.storage == nil {
		return nil, errors.New("storage not configured")
	}

	// Load founder
	founder, err := e.storage.GetAgent(ctx, string(founderID))
	if err != nil {
		return nil, fmt.Errorf("load founder: %w", err)
	}

	// Check level requirement
	if founder.Level < e.config.MinFounderLevel {
		return nil, fmt.Errorf("must be level %d+ to found a guild (current: %d)", e.config.MinFounderLevel, founder.Level)
	}

	// Check XP cost
	if founder.XP < e.config.FoundingXPCost {
		return nil, fmt.Errorf("founding a guild costs %d XP (current: %d)", e.config.FoundingXPCost, founder.XP)
	}

	// Validate name
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("guild name required")
	}

	// Create guild
	now := time.Now()
	guildInstance := GenerateInstance()
	guild := &Guild{
		ID:         GuildID(guildInstance),
		Name:       name,
		Status:     GuildActive,
		Founded:    now,
		FoundedBy:  founderID,
		Culture:    culture,
		Reputation: 0.5, // Start neutral
		MaxMembers: e.config.DefaultMaxMembers,
		MinLevel:   1, // Founder can set this later
		Members: []GuildMember{
			{
				AgentID:      founderID,
				Rank:         GuildRankMaster,
				JoinedAt:     now,
				Contribution: 0,
			},
		},
		CreatedAt: now,
	}

	// Deduct XP from founder and add guild
	err = e.storage.UpdateAgent(ctx, string(founderID), func(a *Agent) error {
		a.XP -= e.config.FoundingXPCost
		a.Guilds = append(a.Guilds, guild.ID)
		a.UpdatedAt = now
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("deduct XP: %w", err)
	}

	// Store guild
	if err := e.storage.PutGuild(ctx, guildInstance, guild); err != nil {
		// Try to rollback XP deduction
		rollbackErr := e.storage.UpdateAgent(ctx, string(founderID), func(a *Agent) error {
			a.XP += e.config.FoundingXPCost
			// Remove guild from list
			for i, g := range a.Guilds {
				if g == guild.ID {
					a.Guilds = append(a.Guilds[:i], a.Guilds[i+1:]...)
					break
				}
			}
			return nil
		})
		if rollbackErr != nil {
			e.logger.Error("failed to rollback XP after guild creation failure",
				"founder", founderID,
				"error", rollbackErr)
		}
		return nil, fmt.Errorf("store guild: %w", err)
	}

	e.logger.Info("guild founded",
		"guild", guild.ID,
		"name", name,
		"founder", founderID)

	return guild, nil
}

// InviteToGuild sends an invitation to an agent. Requires Officer+ rank.
func (e *DefaultGuildFormationEngine) InviteToGuild(ctx context.Context, inviterID AgentID, guildID GuildID, inviteeID AgentID) error {
	if e.storage == nil {
		return errors.New("storage not configured")
	}

	guildInstance := string(guildID)
	guild, err := e.storage.GetGuild(ctx, guildInstance)
	if err != nil {
		return fmt.Errorf("load guild: %w", err)
	}

	// Check inviter is Officer+
	inviterRank := e.getMemberRank(inviterID, guild)
	if !canInvite(inviterRank) {
		return fmt.Errorf("must be Officer or higher to invite (current rank: %s)", inviterRank)
	}

	// Check invitee not already member
	if e.isGuildMember(inviteeID, guild) {
		return errors.New("agent is already a member")
	}

	// Check guild capacity
	if len(guild.Members) >= guild.MaxMembers {
		return errors.New("guild is at maximum capacity")
	}

	// Load invitee to verify they exist and meet level requirement
	invitee, err := e.storage.GetAgent(ctx, string(inviteeID))
	if err != nil {
		return fmt.Errorf("load invitee: %w", err)
	}

	if invitee.Level < guild.MinLevel {
		return fmt.Errorf("invitee must be level %d+ (current: %d)", guild.MinLevel, invitee.Level)
	}

	// For now, invitations are immediate (no pending state).
	// A full implementation would store GuildInvitation and require acceptance.
	return e.addMemberToGuild(ctx, guild, guildInstance, inviteeID, "invited by "+string(inviterID))
}

// ApplyToGuild submits an application to join a guild.
func (e *DefaultGuildFormationEngine) ApplyToGuild(ctx context.Context, applicantID AgentID, guildID GuildID) error {
	if e.storage == nil {
		return errors.New("storage not configured")
	}

	guildInstance := string(guildID)
	guild, err := e.storage.GetGuild(ctx, guildInstance)
	if err != nil {
		return fmt.Errorf("load guild: %w", err)
	}

	// Check not already member
	if e.isGuildMember(applicantID, guild) {
		return errors.New("already a member")
	}

	// Check guild capacity
	if len(guild.Members) >= guild.MaxMembers {
		return errors.New("guild is at maximum capacity")
	}

	// Load applicant
	applicant, err := e.storage.GetAgent(ctx, string(applicantID))
	if err != nil {
		return fmt.Errorf("load applicant: %w", err)
	}

	if applicant.Level < guild.MinLevel {
		return fmt.Errorf("must be level %d+ to apply (current: %d)", guild.MinLevel, applicant.Level)
	}

	// For now, applications are stored in a simple format.
	// A full implementation would have a separate applications KV bucket.
	e.logger.Info("guild application submitted",
		"guild", guildID,
		"applicant", applicantID)

	return nil
}

// ApproveApplication approves a pending application. Requires Officer+ rank.
func (e *DefaultGuildFormationEngine) ApproveApplication(ctx context.Context, approverID AgentID, guildID GuildID, applicantID AgentID) error {
	if e.storage == nil {
		return errors.New("storage not configured")
	}

	guildInstance := string(guildID)
	guild, err := e.storage.GetGuild(ctx, guildInstance)
	if err != nil {
		return fmt.Errorf("load guild: %w", err)
	}

	// Check approver is Officer+
	approverRank := e.getMemberRank(approverID, guild)
	if !canInvite(approverRank) {
		return fmt.Errorf("must be Officer or higher to approve (current rank: %s)", approverRank)
	}

	// Check applicant not already member
	if e.isGuildMember(applicantID, guild) {
		return errors.New("agent is already a member")
	}

	// Check guild capacity
	if len(guild.Members) >= guild.MaxMembers {
		return errors.New("guild is at maximum capacity")
	}

	return e.addMemberToGuild(ctx, guild, guildInstance, applicantID, "application approved by "+string(approverID))
}

// LeaveGuild removes an agent from a guild.
func (e *DefaultGuildFormationEngine) LeaveGuild(ctx context.Context, agentID AgentID, guildID GuildID) error {
	if e.storage == nil {
		return errors.New("storage not configured")
	}

	guildInstance := string(guildID)
	guild, err := e.storage.GetGuild(ctx, guildInstance)
	if err != nil {
		return fmt.Errorf("load guild: %w", err)
	}

	// Check is member
	if !e.isGuildMember(agentID, guild) {
		return errors.New("not a guild member")
	}

	// Check not the only Guildmaster
	memberRank := e.getMemberRank(agentID, guild)
	if memberRank == GuildRankMaster {
		guildmasterCount := 0
		for _, m := range guild.Members {
			if m.Rank == GuildRankMaster {
				guildmasterCount++
			}
		}
		if guildmasterCount <= 1 {
			return errors.New("cannot leave: you are the only Guildmaster; promote someone else first")
		}
	}

	// Remove from guild
	err = e.storage.UpdateGuild(ctx, guildInstance, func(g *Guild) error {
		for i, m := range g.Members {
			if m.AgentID == agentID {
				g.Members = append(g.Members[:i], g.Members[i+1:]...)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("remove from guild: %w", err)
	}

	// Remove guild from agent
	err = e.storage.UpdateAgent(ctx, string(agentID), func(a *Agent) error {
		for i, g := range a.Guilds {
			if g == guildID {
				a.Guilds = append(a.Guilds[:i], a.Guilds[i+1:]...)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		e.logger.Error("failed to remove guild from agent after leaving",
			"agent", agentID,
			"guild", guildID,
			"error", err)
	}

	e.logger.Info("agent left guild",
		"agent", agentID,
		"guild", guildID)

	return nil
}

// PromoteMember promotes a member to a higher rank.
// - Guildmaster can promote to any rank including Guildmaster
// - Officers can promote up to Veteran
func (e *DefaultGuildFormationEngine) PromoteMember(ctx context.Context, promoterID AgentID, guildID GuildID, memberID AgentID, newRank GuildRank) error {
	if e.storage == nil {
		return errors.New("storage not configured")
	}

	guildInstance := string(guildID)
	guild, err := e.storage.GetGuild(ctx, guildInstance)
	if err != nil {
		return fmt.Errorf("load guild: %w", err)
	}

	// Check promoter rank
	promoterRank := e.getMemberRank(promoterID, guild)
	if !canPromote(promoterRank, newRank) {
		return fmt.Errorf("insufficient rank to promote to %s (your rank: %s)", newRank, promoterRank)
	}

	// Check member exists
	if !e.isGuildMember(memberID, guild) {
		return errors.New("target is not a guild member")
	}

	// Update member rank
	return e.storage.UpdateGuild(ctx, guildInstance, func(g *Guild) error {
		for i := range g.Members {
			if g.Members[i].AgentID == memberID {
				g.Members[i].Rank = newRank
				return nil
			}
		}
		return errors.New("member not found")
	})
}

// EvaluateGuildDiversity calculates how well a guild covers skill combinations.
func (e *DefaultGuildFormationEngine) EvaluateGuildDiversity(ctx context.Context, guildID GuildID) (*GuildDiversityReport, error) {
	if e.storage == nil {
		return nil, errors.New("storage not configured")
	}

	guildInstance := string(guildID)
	guild, err := e.storage.GetGuild(ctx, guildInstance)
	if err != nil {
		return nil, fmt.Errorf("load guild: %w", err)
	}

	// Collect all unique skills from members
	skillSet := make(map[SkillTag]int)
	for _, member := range guild.Members {
		agent, err := e.storage.GetAgent(ctx, string(member.AgentID))
		if err != nil {
			continue
		}
		for _, skill := range agent.GetSkillTags() {
			skillSet[skill]++
		}
	}

	uniqueSkills := make([]SkillTag, 0, len(skillSet))
	for skill := range skillSet {
		uniqueSkills = append(uniqueSkills, skill)
	}

	// Calculate diversity score based on skill distribution
	// Higher score = more evenly distributed skills among members
	diversityScore := 0.0
	if len(guild.Members) > 0 && len(uniqueSkills) > 0 {
		// Entropy-like measure: how evenly are skills distributed?
		totalSkillOccurrences := 0
		for _, count := range skillSet {
			totalSkillOccurrences += count
		}
		for _, count := range skillSet {
			if totalSkillOccurrences > 0 {
				p := float64(count) / float64(totalSkillOccurrences)
				if p > 0 {
					diversityScore += p * (1 - p) // Variance contribution
				}
			}
		}
		// Normalize to 0-1
		maxVariance := float64(len(uniqueSkills)) * 0.25 // Max when p=0.5 for all
		if maxVariance > 0 {
			diversityScore = diversityScore / maxVariance
			if diversityScore > 1 {
				diversityScore = 1
			}
		}
	}

	return &GuildDiversityReport{
		GuildID:        guildID,
		TotalMembers:   len(guild.Members),
		UniqueSkills:   uniqueSkills,
		SkillCoverage:  float64(len(uniqueSkills)) / 10.0, // Assume 10 common skills
		DiversityScore: diversityScore,
	}, nil
}

// =============================================================================
// HELPER METHODS
// =============================================================================

// addMemberToGuild adds an agent as a new Initiate member.
func (e *DefaultGuildFormationEngine) addMemberToGuild(ctx context.Context, guild *Guild, guildInstance string, agentID AgentID, reason string) error {
	now := time.Now()

	// Add to guild
	err := e.storage.UpdateGuild(ctx, guildInstance, func(g *Guild) error {
		// Double-check not already member (race condition check)
		for _, m := range g.Members {
			if m.AgentID == agentID {
				return nil // Already member, no-op
			}
		}
		g.Members = append(g.Members, GuildMember{
			AgentID:      agentID,
			Rank:         GuildRankInitiate,
			JoinedAt:     now,
			Contribution: 0,
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("add to guild: %w", err)
	}

	// Add guild to agent
	err = e.storage.UpdateAgent(ctx, string(agentID), func(a *Agent) error {
		for _, g := range a.Guilds {
			if g == guild.ID {
				return nil // Already has guild
			}
		}
		a.Guilds = append(a.Guilds, guild.ID)
		return nil
	})
	if err != nil {
		// Rollback guild membership
		rollbackErr := e.storage.UpdateGuild(ctx, guildInstance, func(g *Guild) error {
			for i, m := range g.Members {
				if m.AgentID == agentID {
					g.Members = append(g.Members[:i], g.Members[i+1:]...)
					return nil
				}
			}
			return nil
		})
		if rollbackErr != nil {
			e.logger.Error("failed to rollback guild membership",
				"agent", agentID,
				"guild", guild.ID,
				"error", rollbackErr)
		}
		return fmt.Errorf("update agent: %w", err)
	}

	// Emit event
	if e.events != nil {
		payload := GuildAutoJoinedPayload{
			AgentID:  agentID,
			GuildID:  guild.ID,
			Rank:     GuildRankInitiate,
			JoinedAt: now,
			Reason:   reason,
		}
		if err := e.events.PublishGuildAutoJoined(ctx, payload); err != nil {
			e.logger.Debug("failed to publish guild joined event",
				"agent", agentID,
				"guild", guild.ID,
				"error", err)
		}
	}

	e.logger.Info("agent joined guild",
		"agent", agentID,
		"guild", guild.ID,
		"reason", reason)

	return nil
}

// getMemberRank returns the rank of a member, or empty string if not a member.
func (e *DefaultGuildFormationEngine) getMemberRank(agentID AgentID, guild *Guild) GuildRank {
	for _, m := range guild.Members {
		if m.AgentID == agentID {
			return m.Rank
		}
	}
	return ""
}

// isGuildMember checks if an agent is already a member of a guild.
func (e *DefaultGuildFormationEngine) isGuildMember(agentID AgentID, guild *Guild) bool {
	for _, member := range guild.Members {
		if member.AgentID == agentID {
			return true
		}
	}
	return false
}

// canInvite returns true if the rank can invite new members.
func canInvite(rank GuildRank) bool {
	return rank == GuildRankOfficer || rank == GuildRankMaster
}

// canPromote returns true if promoterRank can promote to targetRank.
func canPromote(promoterRank, targetRank GuildRank) bool {
	if promoterRank == GuildRankMaster {
		return true // Guildmaster can promote to any rank
	}
	if promoterRank == GuildRankOfficer {
		// Officers can promote up to Veteran
		return targetRank == GuildRankInitiate ||
			targetRank == GuildRankMember ||
			targetRank == GuildRankVeteran
	}
	return false
}

// =============================================================================
// LEGACY CLUSTERING (for suggestions/analysis, not auto-formation)
// =============================================================================

// DetectSkillClusters groups agents by primary skill and calculates cluster strength.
// This is for analysis and suggestions, not auto-formation.
func (e *DefaultGuildFormationEngine) DetectSkillClusters(ctx context.Context, agents []*Agent) []GuildSuggestion {
	// Filter eligible agents
	eligible := e.filterEligibleAgents(agents)
	if len(eligible) < e.config.MinClusterSize {
		return nil
	}

	// Group by primary skill (first declared skill in agent's skill list)
	skillGroups := e.groupByPrimarySkill(eligible)

	var suggestions []GuildSuggestion
	for skill, group := range skillGroups {
		// Check for context cancellation between skill groups
		select {
		case <-ctx.Done():
			e.logger.Debug("DetectSkillClusters cancelled", "processed_skills", len(suggestions))
			return suggestions
		default:
		}

		if len(group) < e.config.MinClusterSize {
			continue
		}

		// Calculate cluster strength via average pairwise Jaccard similarity
		strength := e.calculateClusterStrength(ctx, group)
		if strength < e.config.MinClusterStrength {
			continue
		}

		// Determine secondary skills shared by the cluster
		secondarySkills := e.findSecondarySkills(group, skill)

		// Find minimum level in the cluster
		minLevel := e.findMinLevel(group)

		// Generate suggestion
		agentIDs := make([]AgentID, len(group))
		for i, agent := range group {
			agentIDs[i] = agent.ID
		}

		suggestions = append(suggestions, GuildSuggestion{
			PrimarySkill:    skill,
			SecondarySkills: secondarySkills,
			AgentIDs:        agentIDs,
			ClusterStrength: strength,
			MinLevel:        minLevel,
			SuggestedName:   e.generateGuildName(skill),
		})
	}

	// Sort by cluster strength descending
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].ClusterStrength > suggestions[j].ClusterStrength
	})

	return suggestions
}

// filterEligibleAgents returns agents meeting minimum requirements.
func (e *DefaultGuildFormationEngine) filterEligibleAgents(agents []*Agent) []*Agent {
	var eligible []*Agent
	for _, agent := range agents {
		if agent.Level < e.config.MinAgentLevel {
			continue
		}
		if agent.Stats.AvgQualityScore < e.config.RequireQualityScore {
			continue
		}
		if len(agent.GetSkillTags()) == 0 {
			continue
		}
		if agent.Status == AgentRetired {
			continue
		}
		eligible = append(eligible, agent)
	}
	return eligible
}

// groupByPrimarySkill groups agents by their first declared skill.
func (e *DefaultGuildFormationEngine) groupByPrimarySkill(agents []*Agent) map[SkillTag][]*Agent {
	groups := make(map[SkillTag][]*Agent)
	for _, agent := range agents {
		skills := agent.GetSkillTags()
		if len(skills) == 0 {
			continue
		}
		primary := skills[0]
		groups[primary] = append(groups[primary], agent)
	}
	return groups
}

// calculateClusterStrength computes average pairwise Jaccard similarity.
func (e *DefaultGuildFormationEngine) calculateClusterStrength(ctx context.Context, agents []*Agent) float64 {
	if len(agents) < 2 {
		return 0
	}

	var total float64
	var pairs int

	for i, agentA := range agents {
		// Check for context cancellation periodically
		if i%10 == 0 {
			select {
			case <-ctx.Done():
				if pairs == 0 {
					return 0
				}
				return total / float64(pairs)
			default:
			}
		}

		for _, agentB := range agents[i+1:] {
			total += JaccardSimilarity(agentA.GetSkillTags(), agentB.GetSkillTags())
			pairs++
		}
	}

	if pairs == 0 {
		return 0
	}
	return total / float64(pairs)
}

// JaccardSimilarity computes the Jaccard index between two skill sets.
// Returns intersection / union, ranging from 0 to 1.
func JaccardSimilarity(a, b []SkillTag) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0 // Empty sets are identical
	}

	setA := make(map[SkillTag]struct{}, len(a))
	for _, skill := range a {
		setA[skill] = struct{}{}
	}

	setB := make(map[SkillTag]struct{}, len(b))
	for _, skill := range b {
		setB[skill] = struct{}{}
	}

	// Calculate intersection
	var intersection int
	for skill := range setA {
		if _, ok := setB[skill]; ok {
			intersection++
		}
	}

	// Calculate union
	union := make(map[SkillTag]struct{})
	for skill := range setA {
		union[skill] = struct{}{}
	}
	for skill := range setB {
		union[skill] = struct{}{}
	}

	if len(union) == 0 {
		return 0
	}

	return float64(intersection) / float64(len(union))
}

// findSecondarySkills identifies skills shared by most cluster members beyond primary.
func (e *DefaultGuildFormationEngine) findSecondarySkills(agents []*Agent, primary SkillTag) []SkillTag {
	skillCounts := make(map[SkillTag]int)
	threshold := len(agents) / 2

	for _, agent := range agents {
		for _, skill := range agent.GetSkillTags() {
			if skill != primary {
				skillCounts[skill]++
			}
		}
	}

	var secondary []SkillTag
	for skill, count := range skillCounts {
		if count >= threshold {
			secondary = append(secondary, skill)
		}
	}

	return secondary
}

// findMinLevel returns the minimum level among agents.
func (e *DefaultGuildFormationEngine) findMinLevel(agents []*Agent) int {
	if len(agents) == 0 {
		return 0
	}
	min := agents[0].Level
	for _, agent := range agents[1:] {
		if agent.Level < min {
			min = agent.Level
		}
	}
	return min
}

// generateGuildName creates a name from the primary skill.
func (e *DefaultGuildFormationEngine) generateGuildName(skill SkillTag) string {
	name := string(skill)
	name = strings.ReplaceAll(name, "_", " ")
	caser := cases.Title(language.English)
	name = caser.String(name)
	return name + " Guild"
}
