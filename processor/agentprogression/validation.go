package agentprogression

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
)

// ValidateAgentCanClaim checks whether an agent is eligible to claim a quest.
// Returns nil if the agent can claim, or an error describing why not.
// Used by both the questboard processor and autonomy processor to share
// validation logic.
func ValidateAgentCanClaim(agent *Agent, quest *domain.Quest) error {
	switch agent.Status {
	case domain.AgentRetired:
		return errors.New("agent is retired")
	case domain.AgentInBattle:
		return errors.New("agent is in battle")
	case domain.AgentCooldown:
		if agent.CooldownUntil != nil && time.Now().Before(*agent.CooldownUntil) {
			return errors.New("agent on cooldown")
		}
		// Expired cooldown — allow (status corrected on claim)
	}

	if agent.CurrentQuest != nil {
		return errors.New("agent already on a quest")
	}

	if domain.TierFromLevel(agent.Level) < quest.MinTier {
		return errors.New("agent tier too low")
	}

	if quest.PartyRequired {
		return errors.New("quest requires party")
	}

	if len(quest.RequiredSkills) > 0 {
		hasSkill := false
		for _, required := range quest.RequiredSkills {
			if agent.HasSkill(required) {
				hasSkill = true
				break
			}
		}
		if !hasSkill {
			return errors.New("agent lacks required skills")
		}
	}

	return nil
}
