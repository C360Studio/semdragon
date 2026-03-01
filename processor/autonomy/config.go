package autonomy

import (
	"time"

	semdragons "github.com/c360studio/semdragons"
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
	}
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
func (c *Config) IntervalForStatus(status semdragons.AgentStatus) time.Duration {
	switch status {
	case semdragons.AgentIdle:
		return time.Duration(c.IdleIntervalMs) * time.Millisecond
	case semdragons.AgentOnQuest:
		return time.Duration(c.OnQuestIntervalMs) * time.Millisecond
	case semdragons.AgentInBattle:
		return time.Duration(c.InBattleIntervalMs) * time.Millisecond
	case semdragons.AgentCooldown:
		return time.Duration(c.CooldownIntervalMs) * time.Millisecond
	default:
		// Retired or unknown — no heartbeat
		return 0
	}
}
