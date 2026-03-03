package autonomy

import (
	"time"

	"github.com/c360studio/semdragons/processor/boidengine"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

// =============================================================================
// HEARTBEAT MANAGEMENT - Per-agent timer tracking
// =============================================================================
// Each tracked agent gets a timer that fires at a cadence determined by its
// current status. Timer callbacks trigger autonomy evaluation in their own
// goroutine. The trackers map is guarded by trackersMu; the lock is never
// held across I/O operations.
// =============================================================================

// agentTracker holds per-agent heartbeat state.
type agentTracker struct {
	agent       *agentprogression.Agent
	idleSince   time.Time                   // Process-local, not persisted
	heartbeat   *time.Timer                 // Per-agent timer
	interval    time.Duration               // Current heartbeat interval
	suggestions []boidengine.SuggestedClaim // Ranked boid suggestions, best first
}

// resetHeartbeatForAgent creates or updates the heartbeat timer for an agent.
// Caller must NOT hold trackersMu.
func (c *Component) resetHeartbeatForAgent(instance string, agent *agentprogression.Agent) {
	interval := c.config.IntervalForStatus(agent.Status)

	c.trackersMu.Lock()
	defer c.trackersMu.Unlock()

	tracker, exists := c.trackers[instance]
	if !exists {
		tracker = &agentTracker{}
		c.trackers[instance] = tracker
	}

	// Stop existing timer if running
	if tracker.heartbeat != nil {
		tracker.heartbeat.Stop()
	}

	tracker.agent = agent
	tracker.interval = interval

	// Track when agent became idle (process-local)
	if agent.Status == domain.AgentIdle && tracker.idleSince.IsZero() {
		tracker.idleSince = time.Now()
	} else if agent.Status != domain.AgentIdle {
		tracker.idleSince = time.Time{}
	}

	// Clear suggestion cache when agent is not idle
	if agent.Status != domain.AgentIdle {
		tracker.suggestions = nil
	}

	// Retired agents get no heartbeat
	if interval == 0 {
		tracker.heartbeat = nil
		return
	}

	// Use initial delay for new trackers, otherwise use the interval
	delay := interval
	if !exists {
		delay = time.Duration(c.config.InitialDelayMs) * time.Millisecond
	}

	tracker.heartbeat = time.AfterFunc(delay, func() {
		c.onHeartbeatFired(instance)
	})
}

// onHeartbeatFired is the timer callback. Runs in its own goroutine.
func (c *Component) onHeartbeatFired(instance string) {
	if !c.running.Load() {
		return
	}

	// When paused, skip evaluation but still fall through to re-arm the timer.
	if c.pauseChecker == nil || !c.pauseChecker.Paused() {
		c.evaluateAutonomy(instance)
	}

	// Re-arm the timer under a single Lock to avoid a TOCTOU race: using
	// separate RLock/RUnlock then Lock/Unlock would allow resetHeartbeatForAgent
	// to replace the timer between the two acquisitions, making our interval
	// value stale and the Reset target wrong.
	if !c.running.Load() {
		return
	}
	c.trackersMu.Lock()
	tracker, exists := c.trackers[instance]
	if exists && tracker.interval > 0 && tracker.heartbeat != nil {
		tracker.heartbeat.Reset(tracker.interval)
	}
	c.trackersMu.Unlock()
}

// resetHeartbeatInterval resets the backoff on a tracker (e.g., when new boid
// suggestion arrives). Caller must hold trackersMu.
func (c *Component) resetHeartbeatInterval(instance string) {
	tracker, exists := c.trackers[instance]
	if !exists {
		return
	}

	baseInterval := c.config.IntervalForStatus(domain.AgentIdle)
	if tracker.interval > baseInterval {
		tracker.interval = baseInterval
		if tracker.heartbeat != nil {
			// Stop before Reset: per Go docs, Reset must only be called on a
			// stopped or expired timer. This path is triggered by an incoming
			// boid suggestion, not from the timer's own callback, so the timer
			// may still be running.
			tracker.heartbeat.Stop()
			tracker.heartbeat.Reset(baseInterval)
		}
	}
}

// backoffHeartbeat increases the heartbeat interval for idle agents using the
// configured backoff factor, capped at MaxIntervalMs. Only affects idle agents.
// Caller must hold trackersMu.
func (c *Component) backoffHeartbeat(instance string) {
	tracker, exists := c.trackers[instance]
	if !exists || tracker.agent == nil {
		return
	}

	// Only backoff idle agents
	if tracker.agent.Status != domain.AgentIdle {
		return
	}

	maxInterval := time.Duration(c.config.MaxIntervalMs) * time.Millisecond
	newInterval := time.Duration(float64(tracker.interval) * c.config.BackoffFactor)
	if newInterval > maxInterval {
		newInterval = maxInterval
	}
	tracker.interval = newInterval
}

// cancelHeartbeat stops the timer and removes the tracker for an agent.
// Caller must NOT hold trackersMu.
func (c *Component) cancelHeartbeat(instance string) {
	c.trackersMu.Lock()
	defer c.trackersMu.Unlock()

	if tracker, exists := c.trackers[instance]; exists {
		if tracker.heartbeat != nil {
			tracker.heartbeat.Stop()
		}
		delete(c.trackers, instance)
	}
}

// cancelAllHeartbeats stops all timers and clears the tracker map.
// Called during Stop().
func (c *Component) cancelAllHeartbeats() {
	c.trackersMu.Lock()
	defer c.trackersMu.Unlock()

	for instance, tracker := range c.trackers {
		if tracker.heartbeat != nil {
			tracker.heartbeat.Stop()
		}
		delete(c.trackers, instance)
	}
}
