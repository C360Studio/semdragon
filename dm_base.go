package semdragons

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
)

// =============================================================================
// BASE DUNGEON MASTER - Shared infrastructure for all DM modes
// =============================================================================
// BaseDungeonMaster provides the core functionality shared across all DM modes:
// - Session management
// - Component orchestration (board, boids, evaluator, progression)
// - Event watching
// - World state aggregation
//
// Specific DM modes (Manual, Supervised, Assisted, FullAuto) embed this and
// add their own decision-making logic.
// =============================================================================

// BaseDungeonMaster provides shared infrastructure for all DM implementations.
type BaseDungeonMaster struct {
	board       *NATSQuestBoard
	boids       *DefaultBoidEngine
	evaluator   BattleEvaluator
	progression *ProgressionManager
	storage     *Storage
	events      *EventPublisher
	client      *natsclient.Client
	config      *BoardConfig
	logger      *slog.Logger

	// Session management
	sessions   map[string]*Session
	sessionsMu sync.RWMutex
}

// BaseDMConfig holds configuration for creating a BaseDungeonMaster.
type BaseDMConfig struct {
	Client      *natsclient.Client
	Board       *NATSQuestBoard
	Boids       *DefaultBoidEngine
	Evaluator   BattleEvaluator
	Progression *ProgressionManager
	Logger      *slog.Logger
}

// NewBaseDungeonMaster creates a new BaseDungeonMaster.
// Panics if required config fields (Board, Client) are nil.
func NewBaseDungeonMaster(cfg BaseDMConfig) *BaseDungeonMaster {
	if cfg.Board == nil {
		panic("BaseDMConfig.Board is required")
	}
	if cfg.Client == nil {
		panic("BaseDMConfig.Client is required")
	}

	dm := &BaseDungeonMaster{
		client:      cfg.Client,
		board:       cfg.Board,
		boids:       cfg.Boids,
		evaluator:   cfg.Evaluator,
		progression: cfg.Progression,
		storage:     cfg.Board.Storage(),
		events:      NewEventPublisher(cfg.Client),
		config:      cfg.Board.Config(),
		logger:      cfg.Logger,
		sessions:    make(map[string]*Session),
	}

	// Set defaults
	if dm.boids == nil {
		dm.boids = NewDefaultBoidEngine()
	}
	if dm.evaluator == nil {
		dm.evaluator = NewDefaultBattleEvaluator()
	}
	if dm.logger == nil {
		dm.logger = slog.Default()
	}

	return dm
}

// Board returns the underlying quest board.
func (dm *BaseDungeonMaster) Board() *NATSQuestBoard {
	return dm.board
}

// Boids returns the boid engine.
func (dm *BaseDungeonMaster) Boids() *DefaultBoidEngine {
	return dm.boids
}

// Evaluator returns the battle evaluator.
func (dm *BaseDungeonMaster) Evaluator() BattleEvaluator {
	return dm.evaluator
}

// Progression returns the progression manager.
func (dm *BaseDungeonMaster) Progression() *ProgressionManager {
	return dm.progression
}

// Storage returns the underlying storage.
func (dm *BaseDungeonMaster) Storage() *Storage {
	return dm.storage
}

// =============================================================================
// SESSION MANAGEMENT
// =============================================================================

// SessionKey returns the KV key for a session.
func (dm *BaseDungeonMaster) SessionKey(instance string) string {
	return fmt.Sprintf("session.%s", instance)
}

// StartSession begins a new DM session.
func (dm *BaseDungeonMaster) StartSession(ctx context.Context, config SessionConfig) (*Session, error) {
	instance := GenerateInstance()
	sessionID := dm.config.EntityID("session", instance)

	session := &Session{
		ID:     sessionID,
		Config: config,
		Active: true,
	}

	// Store session in KV
	if err := dm.putSession(ctx, instance, session); err != nil {
		return nil, fmt.Errorf("failed to store session: %w", err)
	}

	// Track in memory
	dm.sessionsMu.Lock()
	dm.sessions[sessionID] = session
	dm.sessionsMu.Unlock()

	// Emit session start event
	dm.events.PublishSessionStart(ctx, SessionStartPayload{
		SessionID: sessionID,
		Config:    config,
		StartedAt: time.Now(),
	})

	dm.logger.Info("session started",
		"session_id", sessionID,
		"mode", config.Mode,
		"name", config.Name,
	)

	return session, nil
}

// EndSession wraps up a DM session and returns summary statistics.
func (dm *BaseDungeonMaster) EndSession(ctx context.Context, sessionID string) (*SessionSummary, error) {
	dm.sessionsMu.Lock()
	session, ok := dm.sessions[sessionID]
	if !ok {
		dm.sessionsMu.Unlock()
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	session.Active = false
	delete(dm.sessions, sessionID)
	dm.sessionsMu.Unlock()

	// Compute summary
	summary := dm.computeSessionSummary(ctx, sessionID)

	// Update session in KV
	instance := ExtractInstance(sessionID)
	if err := dm.putSession(ctx, instance, session); err != nil {
		dm.logger.Warn("failed to update session state", "session_id", sessionID, "error", err)
	}

	// Emit session end event
	dm.events.PublishSessionEnd(ctx, SessionEndPayload{
		SessionID: sessionID,
		Summary:   *summary,
		EndedAt:   time.Now(),
	})

	dm.logger.Info("session ended",
		"session_id", sessionID,
		"quests_completed", summary.QuestsCompleted,
		"quests_failed", summary.QuestsFailed,
		"total_xp", summary.TotalXPAwarded,
	)

	return summary, nil
}

// GetSession retrieves a session by ID.
func (dm *BaseDungeonMaster) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	dm.sessionsMu.RLock()
	session, ok := dm.sessions[sessionID]
	dm.sessionsMu.RUnlock()

	if ok {
		return session, nil
	}

	// Try loading from storage
	instance := ExtractInstance(sessionID)
	return dm.getSession(ctx, instance)
}

// ListActiveSessions returns all active sessions.
func (dm *BaseDungeonMaster) ListActiveSessions() []*Session {
	dm.sessionsMu.RLock()
	defer dm.sessionsMu.RUnlock()

	sessions := make([]*Session, 0, len(dm.sessions))
	for _, session := range dm.sessions {
		if session.Active {
			sessions = append(sessions, session)
		}
	}
	return sessions
}

// =============================================================================
// SESSION STORAGE
// =============================================================================

func (dm *BaseDungeonMaster) putSession(ctx context.Context, instance string, session *Session) error {
	key := dm.SessionKey(instance)
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	_, err = dm.storage.KV().Put(ctx, key, data)
	return err
}

func (dm *BaseDungeonMaster) getSession(ctx context.Context, instance string) (*Session, error) {
	key := dm.SessionKey(instance)
	entry, err := dm.storage.KV().Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	var session Session
	if err := json.Unmarshal(entry.Value, &session); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &session, nil
}

// =============================================================================
// SESSION SUMMARY
// =============================================================================

func (dm *BaseDungeonMaster) computeSessionSummary(ctx context.Context, sessionID string) *SessionSummary {
	summary := &SessionSummary{
		SessionID: sessionID,
	}

	// Get board stats
	stats, err := dm.board.BoardStats(ctx)
	if err == nil {
		summary.QuestsCompleted = stats.TotalCompleted
		summary.QuestsFailed = stats.TotalFailed
		summary.QuestsEscalated = stats.TotalEscalated
	}

	// Count active agents
	agents, err := dm.loadAllAgents(ctx)
	if err == nil {
		for _, agent := range agents {
			if agent.Status != AgentRetired {
				summary.AgentsActive++
			}
			summary.TotalXPAwarded += agent.Stats.TotalXPEarned
		}
	}

	// TODO: Track level ups/downs and deaths during session
	// This would require session-scoped event tracking

	return summary
}

// =============================================================================
// EVENT WATCHING
// =============================================================================

// WatchEvents subscribes to the game event stream.
func (dm *BaseDungeonMaster) WatchEvents(ctx context.Context, filter EventFilter) (<-chan GameEvent, error) {
	// Create a buffered channel for events
	events := make(chan GameEvent, 100)

	var wg sync.WaitGroup

	// Subscribe to quest lifecycle events if no filter or quest types requested
	if len(filter.Types) == 0 || containsQuestEventTypes(filter.Types) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dm.subscribeQuestEvents(ctx, events, filter)
		}()
	}

	// Subscribe to agent events if no filter or agent types requested
	if len(filter.Types) == 0 || containsAgentEventTypes(filter.Types) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dm.subscribeAgentEvents(ctx, events, filter)
		}()
	}

	// Subscribe to battle events if no filter or battle types requested
	if len(filter.Types) == 0 || containsBattleEventTypes(filter.Types) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dm.subscribeBattleEvents(ctx, events, filter)
		}()
	}

	// Close events channel after all subscriptions complete
	go func() {
		<-ctx.Done()
		wg.Wait()
		close(events)
	}()

	return events, nil
}

func containsQuestEventTypes(types []GameEventType) bool {
	for _, t := range types {
		switch t {
		case EventQuestPosted, EventQuestClaimed, EventQuestStarted,
			EventQuestCompleted, EventQuestFailed, EventQuestEscalated:
			return true
		}
	}
	return false
}

func containsAgentEventTypes(types []GameEventType) bool {
	for _, t := range types {
		switch t {
		case EventAgentRecruited, EventAgentLevelUp, EventAgentLevelDown,
			EventAgentDeath, EventAgentPermadeath, EventAgentRevived:
			return true
		}
	}
	return false
}

func containsBattleEventTypes(types []GameEventType) bool {
	for _, t := range types {
		switch t {
		case EventBattleStarted, EventBattleVictory, EventBattleDefeat:
			return true
		}
	}
	return false
}

func (dm *BaseDungeonMaster) subscribeQuestEvents(ctx context.Context, events chan<- GameEvent, filter EventFilter) {
	dm.logger.Debug("subscribing to quest events", "filter", filter)

	const questLifecyclePrefix = "quest.lifecycle."
	subject := questLifecyclePrefix + ">"

	sub, err := dm.board.client.Subscribe(ctx, subject, func(_ context.Context, msg *nats.Msg) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Determine event type from subject suffix
		eventType := EventQuestPosted
		suffix := strings.TrimPrefix(msg.Subject, questLifecyclePrefix)
		if suffix != msg.Subject { // prefix was present
			switch suffix {
			case "claimed":
				eventType = EventQuestClaimed
			case "started":
				eventType = EventQuestStarted
			case "completed":
				eventType = EventQuestCompleted
			case "failed":
				eventType = EventQuestFailed
			case "escalated":
				eventType = EventQuestEscalated
			}
		}

		event := GameEvent{
			Type:      eventType,
			Timestamp: time.Now().UnixMilli(),
			Data:      msg.Data,
		}

		select {
		case events <- event:
		case <-ctx.Done():
			return
		}
	})
	if err != nil {
		dm.logger.Error("failed to subscribe to quest events", "error", err)
		return
	}

	// Unsubscribe when context is cancelled
	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
	}()
}

func (dm *BaseDungeonMaster) subscribeAgentEvents(ctx context.Context, events chan<- GameEvent, filter EventFilter) {
	dm.logger.Debug("subscribing to agent events", "filter", filter)

	const agentProgressionPrefix = "agent.progression."
	subject := agentProgressionPrefix + ">"

	sub, err := dm.board.client.Subscribe(ctx, subject, func(_ context.Context, msg *nats.Msg) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Determine event type from subject suffix
		eventType := EventAgentLevelUp
		suffix := strings.TrimPrefix(msg.Subject, agentProgressionPrefix)
		if suffix != msg.Subject { // prefix was present
			switch suffix {
			case "levelup":
				eventType = EventAgentLevelUp
			case "leveldown":
				eventType = EventAgentLevelDown
			case "death":
				eventType = EventAgentDeath
			}
		}

		event := GameEvent{
			Type:      eventType,
			Timestamp: time.Now().UnixMilli(),
			Data:      msg.Data,
		}

		select {
		case events <- event:
		case <-ctx.Done():
			return
		}
	})
	if err != nil {
		dm.logger.Error("failed to subscribe to agent events", "error", err)
		return
	}

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
	}()
}

func (dm *BaseDungeonMaster) subscribeBattleEvents(ctx context.Context, events chan<- GameEvent, filter EventFilter) {
	dm.logger.Debug("subscribing to battle events", "filter", filter)

	const battleReviewPrefix = "battle.review."
	subject := battleReviewPrefix + ">"

	sub, err := dm.board.client.Subscribe(ctx, subject, func(_ context.Context, msg *nats.Msg) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Determine event type from subject suffix
		eventType := EventBattleStarted
		suffix := strings.TrimPrefix(msg.Subject, battleReviewPrefix)
		if suffix != msg.Subject { // prefix was present
			switch suffix {
			case "started":
				eventType = EventBattleStarted
			case "victory":
				eventType = EventBattleVictory
			case "defeat":
				eventType = EventBattleDefeat
			}
		}

		event := GameEvent{
			Type:      eventType,
			Timestamp: time.Now().UnixMilli(),
			Data:      msg.Data,
		}

		select {
		case events <- event:
		case <-ctx.Done():
			return
		}
	})
	if err != nil {
		dm.logger.Error("failed to subscribe to battle events", "error", err)
		return
	}

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
	}()
}

// =============================================================================
// SESSION EVENTS
// =============================================================================

// SessionStartPayload contains data for dm.session.start events.
type SessionStartPayload struct {
	SessionID string        `json:"session_id"`
	Config    SessionConfig `json:"config"`
	StartedAt time.Time     `json:"started_at"`
}

// Validate checks that required fields are present.
func (p *SessionStartPayload) Validate() error {
	if p.SessionID == "" {
		return fmt.Errorf("session_id required")
	}
	if p.StartedAt.IsZero() {
		return fmt.Errorf("started_at required")
	}
	return nil
}

// SessionEndPayload contains data for dm.session.end events.
type SessionEndPayload struct {
	SessionID string         `json:"session_id"`
	Summary   SessionSummary `json:"summary"`
	EndedAt   time.Time      `json:"ended_at"`
}

// Validate checks that required fields are present.
func (p *SessionEndPayload) Validate() error {
	if p.SessionID == "" {
		return fmt.Errorf("session_id required")
	}
	if p.EndedAt.IsZero() {
		return fmt.Errorf("ended_at required")
	}
	return nil
}

// Typed subjects for session events.
var (
	// SubjectSessionStart is the typed subject for dm.session.start events.
	SubjectSessionStart = natsclient.NewSubject[SessionStartPayload](PredicateSessionStart)
	// SubjectSessionEnd is the typed subject for dm.session.end events.
	SubjectSessionEnd = natsclient.NewSubject[SessionEndPayload](PredicateSessionEnd)
)

// PublishSessionStart publishes a dm.session.start event.
func (ep *EventPublisher) PublishSessionStart(ctx context.Context, payload SessionStartPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectSessionStart.Publish(ctx, ep.client, payload)
}

// PublishSessionEnd publishes a dm.session.end event.
func (ep *EventPublisher) PublishSessionEnd(ctx context.Context, payload SessionEndPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectSessionEnd.Publish(ctx, ep.client, payload)
}
