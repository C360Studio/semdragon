package questdag

import (
	"fmt"
	"time"

	"github.com/c360studio/semdragons/domain"
)

// ComponentName is the registry name of the questdag processor. It supersedes
// the questdagexec event loop; both may be registered during the migration, but
// only one is enabled in a given config.
const ComponentName = "questdag"

// Config configures the questdag processor: the board identity used to derive
// entity IDs, and the gated-DAG executor + durable dispatch consumer it owns.
type Config struct {
	// Board identity (6-part entity-ID prefix org.platform.game.board).
	Org      string `json:"org"`
	Platform string `json:"platform"`
	Board    string `json:"board"`

	// DispatchSubject is where the executor publishes a dispatchable unit
	// reference and where the dispatch consumer subscribes.
	DispatchSubject string `json:"dispatch_subject"`
	// DispatchStream is the JetStream stream backing durable dispatch (ADR-070).
	// The executor creates it (get-or-create); the consumer reads from it.
	DispatchStream string `json:"dispatch_stream"`
	// ConsumerName is this component's durable consumer name on DispatchStream.
	ConsumerName string `json:"consumer_name"`

	// Executor tuning (passed through to processor/gated-dag).
	BackstopInterval     string `json:"backstop_interval"`
	DispatchDedupeWindow string `json:"dispatch_dedupe_window"` // must be >= backstop
	StrandedAfter        string `json:"stranded_after"`         // stall threshold; "0" disables

	// Durable-consume tuning. AckWait bounds a dispatch's processing; Heartbeat
	// (< AckWait) holds a long claim-start past AckWait without redelivery.
	AckWaitSeconds   int `json:"ack_wait_seconds"`
	HeartbeatSeconds int `json:"heartbeat_seconds"`
}

// DefaultConfig returns the default questdag configuration.
func DefaultConfig() Config {
	return Config{
		Org:                  "c360",
		Platform:             "local",
		Board:                "board1",
		DispatchSubject:      "gateddag.dispatch.quest",
		DispatchStream:       "GATEDDAG_DISPATCH",
		ConsumerName:         "questdag-dispatch",
		BackstopInterval:     "30s",
		DispatchDedupeWindow: "30s",
		StrandedAfter:        "5m",
		AckWaitSeconds:       30,
		HeartbeatSeconds:     10,
	}
}

// Validate checks the configuration for internal consistency.
func (c *Config) Validate() error {
	if c.Org == "" || c.Platform == "" || c.Board == "" {
		return fmt.Errorf("questdag: org, platform, and board are required")
	}
	if c.DispatchSubject == "" {
		return fmt.Errorf("questdag: dispatch_subject is required")
	}
	if c.DispatchStream == "" {
		return fmt.Errorf("questdag: dispatch_stream is required")
	}
	if c.ConsumerName == "" {
		return fmt.Errorf("questdag: consumer_name is required")
	}
	if c.HeartbeatSeconds <= 0 || c.AckWaitSeconds <= 0 {
		return fmt.Errorf("questdag: ack_wait_seconds and heartbeat_seconds must be positive")
	}
	// The durable-consume primitive enforces heartbeat*2 <= ack_wait; fail early
	// here with a clearer message.
	if c.HeartbeatSeconds*2 > c.AckWaitSeconds {
		return fmt.Errorf("questdag: heartbeat_seconds (%d) must be <= ack_wait_seconds/2 (%d) so a heartbeat fires before AckWait expires",
			c.HeartbeatSeconds, c.AckWaitSeconds/2)
	}
	return nil
}

// BoardConfig builds the domain BoardConfig from the identity fields.
func (c *Config) BoardConfig() *domain.BoardConfig {
	return &domain.BoardConfig{Org: c.Org, Platform: c.Platform, Board: c.Board}
}

// AckWait returns the ack-wait as a duration.
func (c *Config) AckWait() time.Duration { return time.Duration(c.AckWaitSeconds) * time.Second }

// Heartbeat returns the heartbeat interval as a duration.
func (c *Config) Heartbeat() time.Duration { return time.Duration(c.HeartbeatSeconds) * time.Second }
