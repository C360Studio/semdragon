package autonomy

import (
	"time"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// CONFIGURATION
// =============================================================================

// Config holds all configuration for the autonomy processor.
type Config struct {
	// Board identity
	Org      string `json:"org"`
	Platform string `json:"platform"`
	Board    string `json:"board"`

	// Heartbeat timing (milliseconds)
	InitialDelayMs     int     `json:"initial_delay_ms"`      // Delay before first heartbeat
	IdleIntervalMs     int     `json:"idle_interval_ms"`      // Heartbeat interval when idle
	OnQuestIntervalMs  int     `json:"on_quest_interval_ms"`  // Heartbeat interval when on quest
	InBattleIntervalMs int     `json:"in_battle_interval_ms"` // Heartbeat interval when in battle
	CooldownIntervalMs int     `json:"cooldown_interval_ms"`  // Heartbeat interval when on cooldown
	MaxIntervalMs      int     `json:"max_interval_ms"`       // Maximum backoff interval
	BackoffFactor      float64 `json:"backoff_factor"`        // Multiplier for idle backoff

	// Shopping thresholds
	MinXPSurplusForShopping int64   `json:"min_xp_surplus_for_shopping"` // Min XP above XPToLevel to trigger idle shopping
	MaxShopSpendRatio       float64 `json:"max_shop_spend_ratio"`        // Fraction of surplus to spend when idle
	CooldownShopMinXP       int64   `json:"cooldown_shop_min_xp"`        // Min XP to shop during cooldown
	StrategicShopMaxCost    int64   `json:"strategic_shop_max_cost"`     // Max XP to spend on strategic mid-quest purchase

	// Consumable use thresholds
	CooldownSkipMinRemainingMs int `json:"cooldown_skip_min_remaining_ms"` // Min remaining cooldown (ms) to justify using skip

	// Guild joining thresholds
	MaxGuildsPerAgent int `json:"max_guilds_per_agent"` // Max guilds an agent can autonomously join
	GuildJoinMinLevel int `json:"guild_join_min_level"` // Min agent level to autonomously join guilds
	GuildSuggestionsN int `json:"guild_suggestions_n"`  // Number of guild choices to evaluate

	// Guild creation thresholds
	GuildCreateMinLevel   int `json:"guild_create_min_level"`   // Min agent level to propose a guild (Master tier)
	GuildCreateMinFellows int `json:"guild_create_min_fellows"` // Min fellowship candidates to propose
	GuildCreateMaxFounders int `json:"guild_create_max_founders"` // Max founding members to invite

	// DM approval routing
	DMMode            domain.DMMode `json:"dm_mode"`             // DM mode governing approval behavior
	SessionID         string        `json:"session_id"`          // DM session ID for approval requests
	ApprovalTimeoutMs int           `json:"approval_timeout_ms"` // Timeout for approval requests (ms); default 300000 (5 min)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                "default",
		Platform:           "local",
		Board:              "main",
		InitialDelayMs:     2000,
		IdleIntervalMs:     5000,
		OnQuestIntervalMs:  30000,
		InBattleIntervalMs: 60000,
		CooldownIntervalMs: 15000,
		MaxIntervalMs:      60000,
		BackoffFactor:      1.5,

		MinXPSurplusForShopping:    50,
		MaxShopSpendRatio:          0.5,
		CooldownShopMinXP:          25,
		StrategicShopMaxCost:       200,
		CooldownSkipMinRemainingMs: 30000,

		MaxGuildsPerAgent:     3,
		GuildJoinMinLevel:     3,
		GuildSuggestionsN:     5,
		GuildCreateMinLevel:    16, // Master tier
		GuildCreateMinFellows:  3,  // Matches MinMembersForFormation
		GuildCreateMaxFounders: 6,  // Initial founding group size

		DMMode:            domain.DMFullAuto,
		ApprovalTimeoutMs: 300000, // 5 minutes
	}
}

// CooldownSkipMinRemaining returns the minimum remaining cooldown duration
// that justifies spending a cooldown_skip consumable.
func (c *Config) CooldownSkipMinRemaining() time.Duration {
	return time.Duration(c.CooldownSkipMinRemainingMs) * time.Millisecond
}

// ApprovalTimeout returns the timeout for DM approval requests.
// Defaults to 5 minutes if not configured.
func (c *Config) ApprovalTimeout() time.Duration {
	if c.ApprovalTimeoutMs <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(c.ApprovalTimeoutMs) * time.Millisecond
}

// ToBoardConfig converts processor config to domain BoardConfig.
func (c *Config) ToBoardConfig() *domain.BoardConfig {
	return &domain.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}

// IntervalForStatus returns the heartbeat interval for a given agent status.
// Returns 0 for retired agents (no heartbeat needed).
func (c *Config) IntervalForStatus(status domain.AgentStatus) time.Duration {
	switch status {
	case domain.AgentIdle:
		return time.Duration(c.IdleIntervalMs) * time.Millisecond
	case domain.AgentOnQuest:
		return time.Duration(c.OnQuestIntervalMs) * time.Millisecond
	case domain.AgentInBattle:
		return time.Duration(c.InBattleIntervalMs) * time.Millisecond
	case domain.AgentCooldown:
		return time.Duration(c.CooldownIntervalMs) * time.Millisecond
	default:
		// Retired or unknown — no heartbeat
		return 0
	}
}
