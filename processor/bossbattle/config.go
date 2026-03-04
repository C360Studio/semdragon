package bossbattle

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/promptmanager"
)

// Config holds the component configuration.
type Config struct {
	// BoardConfig contains org, platform, board for entity IDs and bucket naming.
	Org      string `json:"org" schema:"type:string,description:Organization namespace"`
	Platform string `json:"platform" schema:"type:string,description:Platform/environment name"`
	Board    string `json:"board" schema:"type:string,description:Quest board name"`

	// Battle settings
	DefaultTimeout     time.Duration `json:"default_timeout" schema:"type:duration,description:Default battle timeout"`
	MaxConcurrent      int           `json:"max_concurrent" schema:"type:int,description:Max concurrent battles"`
	AutoStartOnSubmit  bool          `json:"auto_start_on_submit" schema:"type:bool,description:Auto-start battles on submission"`
	RequireReviewLevel bool          `json:"require_review_level" schema:"type:bool,description:Only battle quests with review level set"`

	// Domain selects which DomainCatalog to inject (e.g. "software", "dnd", "research").
	Domain string `json:"domain,omitempty"`

	// DomainCatalog enables domain-aware review criteria when set.
	// Not serialized to JSON — resolved from Domain at construction time.
	DomainCatalog *promptmanager.DomainCatalog `json:"-"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                "default",
		Platform:           "local",
		Board:              "main",
		DefaultTimeout:     5 * time.Minute,
		MaxConcurrent:      10,
		AutoStartOnSubmit:  true,
		RequireReviewLevel: true,
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
	if c.DefaultTimeout <= 0 {
		return errors.New("default_timeout must be positive")
	}
	if c.MaxConcurrent < 1 {
		return errors.New("max_concurrent must be at least 1")
	}
	return nil
}
