package partycoord

import (
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/message"
)

// =============================================================================
// PARTY - A temporary group formed to tackle a quest
// =============================================================================
// Party is the core entity owned by the partycoord processor.
// It implements graph.Graphable for persistence in the semstreams graph system.
// =============================================================================

// Party is a temporary group of agents formed to tackle a quest together.
// The party lead is responsible for decomposing the quest and rolling up results.
type Party struct {
	ID      domain.PartyID     `json:"id"`
	Name    string             `json:"name"` // Auto-generated or lead-chosen
	Status  domain.PartyStatus `json:"status"`
	QuestID domain.QuestID     `json:"quest_id"` // The quest this party was formed for

	// Composition
	Lead    domain.AgentID `json:"lead"`
	Members []PartyMember  `json:"members"`

	// Coordination
	Strategy      string                            `json:"strategy"`       // The lead's plan of attack
	SubQuestMap   map[domain.QuestID]domain.AgentID `json:"sub_quest_map"`  // Who's doing what
	SharedContext []ContextItem                     `json:"shared_context"` // Party-wide knowledge

	// Results
	SubResults   map[domain.QuestID]any `json:"sub_results,omitempty"`   // Collected sub-quest outputs
	RollupResult any                    `json:"rollup_result,omitempty"` // Lead's combined result

	FormedAt    time.Time  `json:"formed_at"`
	DisbandedAt *time.Time `json:"disbanded_at,omitempty"`
}

// PartyMember represents an agent's membership in a party.
type PartyMember struct {
	AgentID  domain.AgentID    `json:"agent_id"`
	Role     domain.PartyRole  `json:"role"`
	Skills   []domain.SkillTag `json:"skills"` // Why they were recruited
	JoinedAt time.Time         `json:"joined_at"`
}

// ContextItem represents a piece of shared knowledge in a party.
type ContextItem struct {
	Key     string         `json:"key"`
	Value   any            `json:"value"`
	AddedBy domain.AgentID `json:"added_by"`
	AddedAt time.Time      `json:"added_at"`
}

// =============================================================================
// GRAPHABLE IMPLEMENTATION
// =============================================================================

// EntityID returns the 6-part entity ID for this party.
func (p *Party) EntityID() string {
	return string(p.ID)
}

// Triples returns all semantic facts about this party.
func (p *Party) Triples() []message.Triple {
	now := time.Now()
	source := "partycoord"
	entityID := p.EntityID()

	triples := []message.Triple{
		// Identity
		{Subject: entityID, Predicate: "party.identity.name", Object: p.Name, Source: source, Timestamp: now, Confidence: 1.0},

		// Status
		{Subject: entityID, Predicate: "party.status.state", Object: string(p.Status), Source: source, Timestamp: now, Confidence: 1.0},

		// Quest relationship
		{Subject: entityID, Predicate: "party.quest", Object: string(p.QuestID), Source: source, Timestamp: now, Confidence: 1.0},

		// Lead
		{Subject: entityID, Predicate: "party.lead", Object: string(p.Lead), Source: source, Timestamp: now, Confidence: 1.0},

		// Membership count
		{Subject: entityID, Predicate: "party.membership.count", Object: len(p.Members), Source: source, Timestamp: now, Confidence: 1.0},

		// Lifecycle
		{Subject: entityID, Predicate: "party.lifecycle.formed_at", Object: p.FormedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
	}

	// Strategy if set
	if p.Strategy != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "party.strategy", Object: p.Strategy,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Member relationships
	for _, member := range p.Members {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "party.membership.member", Object: string(member.AgentID),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "party.member." + string(member.AgentID) + ".role", Object: string(member.Role),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Sub-quest assignments
	for questID, agentID := range p.SubQuestMap {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "party.assignment." + string(questID), Object: string(agentID),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Disbanded time if set
	if p.DisbandedAt != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "party.lifecycle.disbanded_at", Object: p.DisbandedAt.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Context items count
	triples = append(triples, message.Triple{
		Subject: entityID, Predicate: "party.context.count", Object: len(p.SharedContext),
		Source: source, Timestamp: now, Confidence: 1.0,
	})

	// Sub-results count
	triples = append(triples, message.Triple{
		Subject: entityID, Predicate: "party.results.count", Object: len(p.SubResults),
		Source: source, Timestamp: now, Confidence: 1.0,
	})

	return triples
}
