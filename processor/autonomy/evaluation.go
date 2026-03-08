package autonomy

import (
	"context"
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

// =============================================================================
// AUTONOMY EVALUATION - Heartbeat-triggered decision loop
// =============================================================================
// On each heartbeat, the evaluator inspects the agent's current state and
// determines what (if any) action should be taken. In Phase 1 all action stubs
// return shouldExecute=false, so the only real work is cooldown expiry detection.
// =============================================================================

// evaluateAutonomy runs the autonomy loop for a single agent.
// Called from the heartbeat timer callback (runs in its own goroutine).
func (c *Component) evaluateAutonomy(instance string) {
	c.evaluationsRun.Add(1)
	c.lastActivity.Store(time.Now())

	// Read tracker under RLock, copy agent value (not pointer) to avoid a data
	// race: checkCooldownExpiry mutates the agent struct, so we must own our
	// copy. Also snapshot the tracker fields we need so we never pass the raw
	// tracker pointer outside the lock.
	c.trackersMu.RLock()
	tracker, exists := c.trackers[instance]
	if !exists {
		c.trackersMu.RUnlock()
		return
	}
	if tracker.agent == nil {
		c.trackersMu.RUnlock()
		return
	}
	agentCopy := *tracker.agent // value copy — safe to mutate in checkCooldownExpiry
	agent := &agentCopy
	agentStatus := agent.Status
	idleSince := tracker.idleSince
	hasSuggestion := len(tracker.suggestions) > 0
	currentInterval := tracker.interval
	// Build a stack-allocated tracker snapshot so action stubs never touch the
	// live tracker pointer outside the lock.
	trackerSnapshot := agentTracker{
		agent:       agent,
		idleSince:   tracker.idleSince,
		interval:    tracker.interval,
		suggestions: tracker.suggestions,
	}
	c.trackersMu.RUnlock()

	// Extend timeout when DM approval may block waiting for human response.
	// Each agent's heartbeat runs in its own goroutine, so blocking one
	// agent doesn't affect others.
	timeout := 10 * time.Second
	if c.config.DMMode == domain.DMSupervised || c.config.DMMode == domain.DMManual {
		timeout = c.config.ApprovalTimeout() + 10*time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	actionTaken := "none"

	// Check cooldown expiry first (before action evaluation)
	if agentStatus == domain.AgentCooldown {
		if c.checkCooldownExpiry(ctx, agent) {
			actionTaken = "cooldown_expired"
			c.cooldownsExpired.Add(1)
			c.emitEvaluated(ctx, agent, actionTaken, currentInterval)
			return
		}
	}

	// Get available actions for current state
	actions := c.actionsForState(agentStatus)

	// Evaluate each action in priority order. Pass &trackerSnapshot (not the
	// live tracker pointer) so stubs cannot race against concurrent writers.
	for _, act := range actions {
		if act.shouldExecute(agent, &trackerSnapshot) {
			if act.execute != nil {
				if err := act.execute(ctx, agent, &trackerSnapshot); err != nil {
					if errors.Is(err, errNoViableClaim) {
						// All suggestions exhausted — treat as "no action taken"
						// so the backoff path runs.
						break
					}
					c.errorsCount.Add(1)
					c.logger.Error("action execution failed",
						"instance", instance,
						"action", act.name,
						"error", err)
				} else {
					actionTaken = act.name
				}
			} else {
				actionTaken = act.name
			}
			break
		}
	}

	// If idle and no action taken, backoff and emit idle event
	if agentStatus == domain.AgentIdle && actionTaken == "none" {
		c.trackersMu.Lock()
		c.backoffHeartbeat(instance)
		var backoffMs int64
		if t, ok := c.trackers[instance]; ok {
			backoffMs = t.interval.Milliseconds()
		}
		c.trackersMu.Unlock()

		var idleDuration time.Duration
		if !idleSince.IsZero() {
			idleDuration = time.Since(idleSince)
		}

		if err := SubjectAutonomyIdle.Publish(ctx, c.deps.NATSClient, IdlePayload{
			AgentID:       agent.ID,
			IdleDuration:  idleDuration,
			HasSuggestion: hasSuggestion,
			BackoffMs:     backoffMs,
			Timestamp:     time.Now(),
		}); err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to publish idle event",
				"agent_id", agent.ID,
				"error", err)
		}
	}

	c.emitEvaluated(ctx, agent, actionTaken, currentInterval)
}

// emitEvaluated publishes an EvaluatedPayload event.
func (c *Component) emitEvaluated(ctx context.Context, agent *agentprogression.Agent, actionTaken string, interval time.Duration) {
	if err := SubjectAutonomyEvaluated.Publish(ctx, c.deps.NATSClient, EvaluatedPayload{
		AgentID:     agent.ID,
		AgentStatus: agent.Status,
		ActionTaken: actionTaken,
		Interval:    interval,
		Timestamp:   time.Now(),
	}); err != nil {
		c.errorsCount.Add(1)
	}
}

// checkCooldownExpiry checks if an agent's cooldown has expired and transitions
// them back to idle if so. Returns true if the cooldown was expired.
func (c *Component) checkCooldownExpiry(ctx context.Context, agent *agentprogression.Agent) bool {
	if agent.CooldownUntil == nil {
		return false
	}

	if !time.Now().After(*agent.CooldownUntil) {
		return false
	}

	// Cooldown has expired — transition to idle
	agent.Status = domain.AgentIdle
	agent.CooldownUntil = nil
	agent.UpdatedAt = time.Now()

	// Write back to KV; the KV watch will trigger heartbeat reset to idle cadence
	if err := c.graph.EmitEntityUpdate(ctx, agent, "agent.autonomy.cooldown_expired"); err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to transition agent from cooldown to idle",
			"agent_id", agent.ID,
			"error", err)
		return false
	}

	c.logger.Info("agent cooldown expired, transitioned to idle",
		"agent_id", agent.ID)
	return true
}

// actionsForState returns the available actions for a given agent status,
// ordered by priority. Each status gets a different set per the ADR action matrix.
func (c *Component) actionsForState(status domain.AgentStatus) []action {
	switch status {
	case domain.AgentIdle:
		return []action{
			c.reviewGuildApplicationsAction(), // Founders process applications before claiming quests
			c.claimQuestAction(),
			c.useConsumableAction(),
			c.shopAction(),
			c.applyToGuildAction(),
			c.joinGuildAction(),
			c.createGuildAction(),
		}
	case domain.AgentOnQuest:
		return []action{
			c.shopStrategicAction(),
			c.useConsumableAction(),
		}
	case domain.AgentInBattle:
		return []action{
			c.useConsumableAction(),
		}
	case domain.AgentCooldown:
		return []action{
			c.useCooldownSkipAction(),
			c.reviewGuildApplicationsAction(),
			c.shopAction(),
			c.applyToGuildAction(),
			c.joinGuildAction(),
			c.createGuildAction(),
		}
	default:
		// Retired or unknown
		return nil
	}
}
