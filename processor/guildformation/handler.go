package guildformation

import (
	"context"
	"errors"
	"time"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// GUILD OPERATIONS (delegated to engine)
// =============================================================================

// FoundGuild creates a new guild.
func (c *Component) FoundGuild(ctx context.Context, founderID semdragons.AgentID, name, culture string) (*semdragons.Guild, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	guild, err := c.engine.FoundGuild(ctx, founderID, name, culture)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, err
	}

	c.guildsCreated.Add(1)
	return guild, nil
}

// InviteToGuild sends an invitation to an agent.
func (c *Component) InviteToGuild(ctx context.Context, inviterID semdragons.AgentID, guildID semdragons.GuildID, inviteeID semdragons.AgentID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	if err := c.engine.InviteToGuild(ctx, inviterID, guildID, inviteeID); err != nil {
		c.errorsCount.Add(1)
		return err
	}

	c.membersAdded.Add(1)
	return nil
}

// LeaveGuild removes an agent from a guild.
func (c *Component) LeaveGuild(ctx context.Context, agentID semdragons.AgentID, guildID semdragons.GuildID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	if err := c.engine.LeaveGuild(ctx, agentID, guildID); err != nil {
		c.errorsCount.Add(1)
		return err
	}

	return nil
}

// PromoteMember promotes a guild member.
func (c *Component) PromoteMember(ctx context.Context, promoterID semdragons.AgentID, guildID semdragons.GuildID, memberID semdragons.AgentID, newRank semdragons.GuildRank) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	if err := c.engine.PromoteMember(ctx, promoterID, guildID, memberID, newRank); err != nil {
		c.errorsCount.Add(1)
		return err
	}

	return nil
}

// DetectSkillClusters suggests potential guild formations.
func (c *Component) DetectSkillClusters(ctx context.Context) ([]semdragons.GuildSuggestion, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	// Load all agents from graph
	agentEntities, err := c.graph.ListAgentsByPrefix(ctx, 100)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, err
	}

	// Reconstruct agents from entity states
	agents := make([]*semdragons.Agent, 0, len(agentEntities))
	for _, entity := range agentEntities {
		agent := semdragons.AgentFromEntityState(&entity)
		if agent != nil {
			agents = append(agents, agent)
		}
	}

	suggestions := c.engine.DetectSkillClusters(ctx, agents)
	return suggestions, nil
}

// EvaluateGuildDiversity calculates guild skill coverage.
func (c *Component) EvaluateGuildDiversity(ctx context.Context, guildID semdragons.GuildID) (*semdragons.GuildDiversityReport, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	report, err := c.engine.EvaluateGuildDiversity(ctx, guildID)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, err
	}

	return report, nil
}

// =============================================================================
// ACCESSORS
// =============================================================================

// Graph returns the underlying graph client for external access.
func (c *Component) Graph() *semdragons.GraphClient {
	return c.graph
}

// Engine returns the underlying formation engine.
func (c *Component) Engine() *semdragons.DefaultGuildFormationEngine {
	return c.engine
}

// Stats returns guild formation statistics.
func (c *Component) Stats() GuildStats {
	return GuildStats{
		GuildsCreated:   c.guildsCreated.Load(),
		MembersAdded:    c.membersAdded.Load(),
		SuggestionsEmit: c.suggestionsEmit.Load(),
		Errors:          c.errorsCount.Load(),
	}
}

// GuildStats holds guild formation statistics.
type GuildStats struct {
	GuildsCreated   uint64 `json:"guilds_created"`
	MembersAdded    uint64 `json:"members_added"`
	SuggestionsEmit uint64 `json:"suggestions_emit"`
	Errors          int64  `json:"errors"`
}
