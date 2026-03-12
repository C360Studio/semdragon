package guildformation

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/partycoord"
	"github.com/c360studio/semstreams/component"
)

// =============================================================================
// COMPONENT - GuildFormation as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Manages guild creation, membership, and promotions.
// State is maintained in-memory using sync.Map projections.
// =============================================================================

// Component implements the GuildFormation processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// Guild state - in-memory projection
	guilds sync.Map // map[domain.GuildID]*domain.Guild

	// Per-guild mutexes for protecting guild struct mutations.
	// Same pattern as dagMutexes in questdagexec: LoadOrStore for safe concurrent creation.
	guildMutexes sync.Map // map[domain.GuildID]*sync.Mutex

	// Agent to guild mapping - in-memory projection (each agent belongs to at most one guild)
	agentGuilds sync.Map // map[domain.AgentID]domain.GuildID

	// Timeout loop for pending guild dissolution
	timeoutDoneCh chan struct{}

	// sharedWins is bootstrapped at Start() from existing KV state, then kept
	// current by incremental KV watchers on quest and peer review entity types.
	sharedWins *sharedWinsCache

	// KV watchers for incremental shared wins updates
	questWatch  jetstream.KeyWatcher
	reviewWatch jetstream.KeyWatcher
	watchDoneCh chan struct{}

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}
	stopOnce sync.Once

	// Metrics
	guildsCreated   atomic.Uint64
	membersJoined   atomic.Uint64
	promotionsCount atomic.Uint64
	errorsCount     atomic.Int64
	lastActivity    atomic.Value // time.Time
	startTime       time.Time
}

// ensure Component implements the required interfaces.
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// =============================================================================
// DISCOVERABLE INTERFACE
// =============================================================================

// Meta returns basic component information.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        ComponentName,
		Type:        "processor",
		Description: "Guild formation and membership management",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
// Entity-centric: watches ENTITY_STATES KV for agent level/XP changes that trigger clustering.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "guild-events",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Guild lifecycle and membership events",
			Config: &component.NATSPort{
				Subject: domain.PredicateGuildCreated,
			},
		},
	}
}

// ConfigSchema returns the configuration schema for this component.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"org": {
				Type:        "string",
				Description: "Organization namespace",
				Default:     "default",
				Category:    "basic",
			},
			"platform": {
				Type:        "string",
				Description: "Platform/environment name",
				Default:     "local",
				Category:    "basic",
			},
			"board": {
				Type:        "string",
				Description: "Quest board name",
				Default:     "main",
				Category:    "basic",
			},
			"min_members_for_formation": {
				Type:        "int",
				Description: "Minimum members to form a guild",
				Default:     3,
				Category:    "guild",
			},
			"max_guild_size": {
				Type:        "int",
				Description: "Maximum members per guild",
				Default:     20,
				Category:    "guild",
			},
			"enable_quorum_formation": {
				Type:        "bool",
				Description: "When true, new guilds start as pending and require founding quorum",
				Default:     false,
				Category:    "guild",
			},
			"min_founding_members": {
				Type:        "int",
				Description: "Number of members (including founder) required to activate a pending guild",
				Default:     3,
				Category:    "guild",
			},
			"formation_timeout_sec": {
				Type:        "int",
				Description: "Seconds before a pending guild dissolves if quorum is not met",
				Default:     300,
				Category:    "guild",
			},
		},
		Required: []string{"org", "platform", "board"},
	}
}

// Health returns current health status.
func (c *Component) Health() component.HealthStatus {
	status := component.HealthStatus{
		Healthy:    c.running.Load(),
		LastCheck:  time.Now(),
		ErrorCount: int(c.errorsCount.Load()),
		Uptime:     time.Since(c.startTime),
	}

	if c.running.Load() {
		status.Status = "running"
	} else {
		status.Status = "stopped"
	}

	if c.errorsCount.Load() > 0 {
		status.LastError = "errors encountered during guild operations"
	}

	return status
}

// DataFlow returns current data flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	metrics := component.FlowMetrics{
		MessagesPerSecond: 0,
		BytesPerSecond:    0,
		ErrorRate:         0,
	}

	if lastTime, ok := c.lastActivity.Load().(time.Time); ok {
		metrics.LastActivity = lastTime
	}

	operations := c.guildsCreated.Load() + c.membersJoined.Load() + c.promotionsCount.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(operations) / uptime
	}

	if operations > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(operations)
	}

	return metrics
}

// =============================================================================
// LIFECYCLE INTERFACE
// =============================================================================

// Initialize performs one-time setup. No I/O operations here.
func (c *Component) Initialize() error {
	if c.config == nil {
		return errors.New("config not set")
	}

	if c.deps.NATSClient == nil {
		return errors.New("NATS client required")
	}

	c.boardConfig = c.config.ToBoardConfig()
	c.sharedWins = newSharedWinsCache()
	c.stopChan = make(chan struct{})

	return nil
}

// Start begins component operation with the given context.
func (c *Component) Start(ctx context.Context) error {
	// Check for cancellation before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Create graph client for entity state reads and watches
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)

	// Bootstrap shared-wins cache from completed quest and peer review history.
	// Errors are non-fatal — guild scoring degrades gracefully with an empty cache.
	c.bootstrapSharedWins(ctx)

	// Start incremental KV watchers for shared wins cache updates.
	// These keep the cache current as quests complete and peer reviews are submitted.
	// Failure is non-fatal — the bootstrap data is still available.
	c.startCohesionWatchers(ctx)

	// Start timeout loop for pending guild dissolution
	if c.config.EnableQuorumFormation {
		c.timeoutDoneCh = make(chan struct{})
		go c.runFormationTimeoutLoop()
	}

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("guildformation component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"quorum_formation", c.config.EnableQuorumFormation)

	return nil
}

// Stop gracefully shuts down the component.
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// Signal stop using sync.Once to prevent double-close panic
	c.stopOnce.Do(func() {
		close(c.stopChan)
	})

	// Stop KV watchers
	if c.questWatch != nil {
		c.questWatch.Stop()
	}
	if c.reviewWatch != nil {
		c.reviewWatch.Stop()
	}

	// Wait for cohesion watcher goroutine to exit
	if c.watchDoneCh != nil {
		select {
		case <-c.watchDoneCh:
		case <-time.After(timeout):
			c.logger.Warn("guildformation stop timed out waiting for cohesion watchers")
		}
	}

	// Wait for timeout loop to exit
	if c.timeoutDoneCh != nil {
		select {
		case <-c.timeoutDoneCh:
		case <-time.After(timeout):
			c.logger.Warn("guildformation stop timed out waiting for timeout loop")
		}
	}

	c.running.Store(false)
	c.logger.Info("guildformation component stopped")

	return nil
}

// BoardConfig returns the board configuration.
func (c *Component) BoardConfig() *domain.BoardConfig {
	return c.boardConfig
}

// guildMutex returns or creates the per-guild mutex for the given guild ID.
// Uses LoadOrStore for safe concurrent creation — same pattern as dagMutexes in questdagexec.
func (c *Component) guildMutex(id domain.GuildID) *sync.Mutex {
	val, _ := c.guildMutexes.LoadOrStore(id, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// =============================================================================
// SHARED WINS CACHE — Bootstrap, Incremental Watchers, Public Accessors
// =============================================================================

// bootstrapSharedWins scans completed quests and peer reviews from KV to seed the
// sharedWinsCache. It runs once at Start() time. Errors are logged and skipped so
// that a NATS hiccup at startup does not prevent the component from running.
// After bootstrap, incremental KV watchers (startCohesionWatchers) keep the cache
// current as new quests complete and peer reviews are submitted.
func (c *Component) bootstrapSharedWins(ctx context.Context) {
	questCount := 0
	partyCount := 0
	reviewCount := 0

	// --- Phase 1: completed party quests → shared wins -----------------------
	quests, err := c.graph.ListQuestsByPrefix(ctx, 0)
	if err != nil {
		c.logger.Warn("bootstrapSharedWins: failed to list quests", "err", err)
	} else {
		for i := range quests {
			q := domain.QuestFromEntityState(&quests[i])
			if q == nil || q.Status != domain.QuestCompleted || q.PartyID == nil {
				continue
			}

			partyEntity, err := c.graph.GetParty(ctx, *q.PartyID)
			if err != nil {
				c.logger.Debug("bootstrapSharedWins: get party failed",
					"party_id", *q.PartyID, "quest_id", q.ID, "err", err)
				continue
			}

			party := partycoord.PartyFromEntityState(partyEntity)
			if party == nil || len(party.Members) < 2 {
				continue
			}

			memberIDs := make([]domain.AgentID, 0, len(party.Members)+1)
			// Include the lead if they are not already in the Members slice.
			leadIncluded := false
			for _, m := range party.Members {
				memberIDs = append(memberIDs, m.AgentID)
				if m.AgentID == party.Lead {
					leadIncluded = true
				}
			}
			if party.Lead != "" && !leadIncluded {
				memberIDs = append(memberIDs, party.Lead)
			}

			c.sharedWins.RecordPartyWin(memberIDs)
			partyCount++
			questCount++
		}
	}

	// --- Phase 2: completed peer reviews → pairwise peer scores --------------
	reviews, err := c.graph.ListPeerReviewsByPrefix(ctx, 0)
	if err != nil {
		c.logger.Warn("bootstrapSharedWins: failed to list peer reviews", "err", err)
	} else {
		for i := range reviews {
			pr := domain.PeerReviewFromEntityState(&reviews[i])
			if pr == nil || pr.Status != domain.PeerReviewCompleted {
				continue
			}
			if pr.LeaderAvgRating > 0 {
				c.sharedWins.RecordPeerReview(pr.LeaderID, pr.MemberID, pr.LeaderAvgRating)
			}
			if pr.MemberAvgRating > 0 {
				c.sharedWins.RecordPeerReview(pr.MemberID, pr.LeaderID, pr.MemberAvgRating)
			}
			reviewCount++
		}
	}

	c.logger.Info("bootstrapSharedWins complete",
		"quests_scanned", questCount,
		"parties_recorded", partyCount,
		"reviews_recorded", reviewCount)
}

// startCohesionWatchers starts KV watchers for quest and peer review entities to
// incrementally update the shared wins cache. Both watchers feed a single goroutine
// so that cache updates are serialized.
func (c *Component) startCohesionWatchers(ctx context.Context) {
	questWatcher, err := c.graph.WatchEntityType(ctx, domain.EntityTypeQuest)
	if err != nil {
		c.logger.Warn("failed to start quest cohesion watcher", "error", err)
	} else {
		c.questWatch = questWatcher
	}

	reviewWatcher, err := c.graph.WatchEntityType(ctx, domain.EntityTypePeerReview)
	if err != nil {
		c.logger.Warn("failed to start peer review cohesion watcher", "error", err)
	} else {
		c.reviewWatch = reviewWatcher
	}

	// Only start the goroutine if at least one watcher is active.
	if c.questWatch != nil || c.reviewWatch != nil {
		c.watchDoneCh = make(chan struct{})
		go c.processCohesionUpdates(ctx)
	}
}

// processCohesionUpdates handles KV watch updates for quest and peer review
// entities, incrementally updating the shared wins cache. Historical entries
// replayed before the nil sentinel are skipped — bootstrapSharedWins already
// processed those. Only live updates after the sentinel are applied.
func (c *Component) processCohesionUpdates(ctx context.Context) {
	defer close(c.watchDoneCh)

	// Set up channels, handling nil watchers gracefully.
	var questUpdates, reviewUpdates <-chan jetstream.KeyValueEntry
	if c.questWatch != nil {
		questUpdates = c.questWatch.Updates()
	}
	if c.reviewWatch != nil {
		reviewUpdates = c.reviewWatch.Updates()
	}

	// Skip replay entries — bootstrap already handled historical data.
	questLive := c.questWatch == nil
	reviewLive := c.reviewWatch == nil

	for {
		select {
		case <-c.stopChan:
			return

		case <-ctx.Done():
			return

		case entry, ok := <-questUpdates:
			if !ok {
				questUpdates = nil
				continue
			}
			if entry == nil {
				questLive = true // nil sentinel: replay complete
				continue
			}
			if !questLive {
				continue // skip replay entries
			}
			c.handleQuestCohesionUpdate(ctx, entry)

		case entry, ok := <-reviewUpdates:
			if !ok {
				reviewUpdates = nil
				continue
			}
			if entry == nil {
				reviewLive = true // nil sentinel: replay complete
				continue
			}
			if !reviewLive {
				continue // skip replay entries
			}
			c.handleReviewCohesionUpdate(entry)
		}
	}
}

// handleQuestCohesionUpdate checks if a quest entity transitioned to completed
// with a party, and if so records shared wins for all party members.
func (c *Component) handleQuestCohesionUpdate(ctx context.Context, entry jetstream.KeyValueEntry) {
	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		return
	}

	q := domain.QuestFromEntityState(entityState)
	if q == nil || q.Status != domain.QuestCompleted || q.PartyID == nil {
		return
	}

	partyEntity, err := c.graph.GetParty(ctx, *q.PartyID)
	if err != nil {
		c.logger.Debug("cohesion watcher: get party failed",
			"party_id", *q.PartyID, "quest_id", q.ID, "err", err)
		return
	}

	party := partycoord.PartyFromEntityState(partyEntity)
	if party == nil || len(party.Members) < 2 {
		return
	}

	memberIDs := make([]domain.AgentID, 0, len(party.Members)+1)
	leadIncluded := false
	for _, m := range party.Members {
		memberIDs = append(memberIDs, m.AgentID)
		if m.AgentID == party.Lead {
			leadIncluded = true
		}
	}
	if party.Lead != "" && !leadIncluded {
		memberIDs = append(memberIDs, party.Lead)
	}

	c.sharedWins.RecordPartyWin(memberIDs)

	c.logger.Debug("cohesion watcher: recorded party win",
		"quest_id", q.ID, "party_id", *q.PartyID, "members", len(memberIDs))
}

// handleReviewCohesionUpdate checks if a peer review entity transitioned to
// completed and records pairwise ratings in the shared wins cache.
func (c *Component) handleReviewCohesionUpdate(entry jetstream.KeyValueEntry) {
	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		return
	}

	pr := domain.PeerReviewFromEntityState(entityState)
	if pr == nil || pr.Status != domain.PeerReviewCompleted {
		return
	}

	if pr.LeaderAvgRating > 0 {
		c.sharedWins.RecordPeerReview(pr.LeaderID, pr.MemberID, pr.LeaderAvgRating)
	}
	if pr.MemberAvgRating > 0 {
		c.sharedWins.RecordPeerReview(pr.MemberID, pr.LeaderID, pr.MemberAvgRating)
	}

	c.logger.Debug("cohesion watcher: recorded peer review",
		"review_id", pr.ID, "leader", pr.LeaderID, "member", pr.MemberID)
}

// SharedWins returns how many quests agents a and b completed together in the same party.
// Returns 0 if the cache is not initialized or no shared wins have been recorded.
func (c *Component) SharedWins(a, b domain.AgentID) int {
	if c.sharedWins == nil {
		return 0
	}
	return c.sharedWins.SharedWins(a, b)
}

// PairwisePeerScore returns the normalized (0–1) pairwise peer review score between
// agents a and b. Returns (0, false) if the cache is not initialized or no reviews
// have been recorded for the pair.
func (c *Component) PairwisePeerScore(a, b domain.AgentID) (float64, bool) {
	if c.sharedWins == nil {
		return 0, false
	}
	return c.sharedWins.PairwisePeerScore(a, b)
}
