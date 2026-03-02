package promptmanager

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/types"
)

// =============================================================================
// PROMPT ASSEMBLED PAYLOAD
// =============================================================================

// Ensure Graphable implementation.
var _ graph.Graphable = (*PromptAssembledPayload)(nil)

// SubjectPromptAssembled is the typed subject for prompt assembly events.
var SubjectPromptAssembled = natsclient.NewSubject[PromptAssembledPayload](domain.PredicatePromptAssembled)

// PromptAssembledPayload contains data for prompt.assembly.completed events.
type PromptAssembledPayload struct {
	QuestID       domain.QuestID `json:"quest_id"`
	AgentID       domain.AgentID `json:"agent_id"`
	DomainID      string         `json:"domain_id"`
	TierApplied   string         `json:"tier_applied"`
	Provider      string         `json:"provider"`
	FragmentsUsed []string       `json:"fragments_used"`
	FragmentCount int            `json:"fragment_count"`
	Timestamp     time.Time      `json:"timestamp"`
}

// EntityID returns the entity ID for this event.
func (p *PromptAssembledPayload) EntityID() string { return string(p.QuestID) }

// Triples returns semantic triples for this event.
func (p *PromptAssembledPayload) Triples() []message.Triple {
	source := "promptmanager"
	entityID := string(p.QuestID)

	return []message.Triple{
		{Subject: entityID, Predicate: domain.PredicatePromptAssembled, Object: true, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "prompt.assembly.agent_id", Object: string(p.AgentID), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "prompt.assembly.domain", Object: p.DomainID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "prompt.assembly.tier", Object: p.TierApplied, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "prompt.assembly.provider", Object: p.Provider, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "prompt.assembly.fragment_count", Object: p.FragmentCount, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *PromptAssembledPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "prompt.assembly.completed", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *PromptAssembledPayload) Validate() error {
	if p.QuestID == "" {
		return errors.New("quest_id required")
	}
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}
