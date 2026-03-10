package questdagexec

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
)

// Config holds the configuration for the questdagexec component.
type Config struct {
	// Board identity — org.platform.game.board forms the entity ID prefix.
	Org      string `json:"org"`
	Platform string `json:"platform"`
	Board    string `json:"board"`

	// DAGTimeout is the maximum wall-clock time allowed for a full DAG to
	// reach a terminal state (all nodes completed or at least one failed).
	DAGTimeout time.Duration `json:"dag_timeout"`

	// RecruitmentTimeout is the maximum time to wait for enough idle agents
	// to staff all DAG nodes during the initial recruitment pass.
	RecruitmentTimeout time.Duration `json:"recruitment_timeout"`

	// RecruitmentInterval is how often to retry recruitment when the initial
	// pass did not find enough idle agents.
	RecruitmentInterval time.Duration `json:"recruitment_interval"`

	// MaxRetriesPerNode is the number of times a node can be retried after
	// the lead rejects its output before the node transitions to NodeFailed.
	MaxRetriesPerNode int `json:"max_retries_per_node"`

	// StreamName is the AGENT JetStream stream used to publish lead review
	// TaskMessages.
	StreamName string `json:"stream_name"`

	// TriageEnabled activates DM triage at the DAG node exhaustion boundary.
	// When true, nodes that exhaust MaxRetriesPerNode transition to
	// pending_triage instead of immediately escalating the parent quest.
	// The questboard's triage watcher processes the triage decision and
	// either reposts (salvage/tpk) or fails/escalates the sub-quest.
	TriageEnabled bool `json:"triage_enabled"`

	// Domain selects a DomainCatalog for review prompt assembly (optional).
	Domain string `json:"domain,omitempty"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                 "default",
		Platform:            "local",
		Board:               "main",
		DAGTimeout:          30 * time.Minute,
		RecruitmentTimeout:  5 * time.Minute,
		RecruitmentInterval: 30 * time.Second,
		MaxRetriesPerNode:   2,
		StreamName:          "AGENT",
	}
}

// ToBoardConfig converts component config to semdragons BoardConfig.
func (c *Config) ToBoardConfig() *domain.BoardConfig {
	return &domain.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}

// Validate checks the configuration for required fields and valid values.
func (c *Config) Validate() error {
	if c.Org == "" {
		return errors.New("org is required")
	}
	if c.Platform == "" {
		return errors.New("platform is required")
	}
	if c.Board == "" {
		return errors.New("board is required")
	}
	if c.DAGTimeout <= 0 {
		return errors.New("dag_timeout must be positive")
	}
	if c.RecruitmentTimeout <= 0 {
		return errors.New("recruitment_timeout must be positive")
	}
	if c.RecruitmentInterval <= 0 {
		return errors.New("recruitment_interval must be positive")
	}
	if c.MaxRetriesPerNode < 0 {
		return errors.New("max_retries_per_node must be non-negative")
	}
	if c.StreamName == "" {
		return errors.New("stream_name is required")
	}
	return nil
}
