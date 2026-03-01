package autonomy

import (
	"context"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// STUB ACTIONS - Wired in Phases 2-5
// =============================================================================
// Each factory returns an action with shouldExecute returning false.
// These are placeholders that will be implemented in later phases.
// =============================================================================

// action represents an autonomous action an agent can take.
type action struct {
	name          string
	shouldExecute func(*semdragons.Agent, *agentTracker) bool
	execute       func(context.Context, *semdragons.Agent, *agentTracker) error
}

// claimQuestAction returns a stub for claiming quests from the board.
// TODO: Implement in Phase 2 — use boid suggestion + questboard claim.
func claimQuestAction() action {
	return action{
		name:          "claim_quest",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}

// claimConcurrentAction returns a stub for claiming concurrent quests.
// TODO: Implement in Phase 3 — claim additional quests while on_quest.
func claimConcurrentAction() action {
	return action{
		name:          "claim_concurrent",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}

// shopAction returns a stub for browsing/purchasing from the store.
// TODO: Implement in Phase 4 — browse affordable items, decide to buy.
func shopAction() action {
	return action{
		name:          "shop",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}

// shopStrategicAction returns a stub for strategic purchases while on quest.
// TODO: Implement in Phase 4 — mid-quest tool/consumable acquisition.
func shopStrategicAction() action {
	return action{
		name:          "shop_strategic",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}

// useConsumableAction returns a stub for using consumables.
// TODO: Implement in Phase 4 — activate consumable at optimal moment.
func useConsumableAction() action {
	return action{
		name:          "use_consumable",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}

// useCooldownSkipAction returns a stub for using cooldown_skip consumable.
// TODO: Implement in Phase 4 — skip cooldown if agent owns the consumable.
func useCooldownSkipAction() action {
	return action{
		name:          "use_cooldown_skip",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}

// joinGuildAction returns a stub for joining a guild.
// TODO: Implement in Phase 5 — evaluate guild suggestions, auto-join.
func joinGuildAction() action {
	return action{
		name:          "join_guild",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}
