package dm_session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
)

// =============================================================================
// SESSION MANAGER - Core session management logic
// =============================================================================
// SessionManager provides session lifecycle management for DM implementations.
// This handles session creation, storage, event watching, and world state.
// =============================================================================

// SessionManager handles DM session lifecycle.
type SessionManager struct {
	client      *natsclient.Client
	boardConfig *semdragons.BoardConfig
	graph       *semdragons.GraphClient
	logger      *slog.Logger

	// Session state
	sessions   map[string]*domain.Session
	sessionsMu sync.RWMutex
}

// NewSessionManager creates a new session manager.
func NewSessionManager(client *natsclient.Client, boardConfig *semdragons.BoardConfig, logger *slog.Logger) *SessionManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &SessionManager{
		client:      client,
		boardConfig: boardConfig,
		graph:       semdragons.NewGraphClient(client, boardConfig),
		logger:      logger,
		sessions:    make(map[string]*domain.Session),
	}
}

// =============================================================================
// SESSION LIFECYCLE
// =============================================================================

// SessionKey returns the KV key for a session.
func (sm *SessionManager) SessionKey(instance string) string {
	return fmt.Sprintf("session.%s", instance)
}

// StartSession begins a new DM session.
func (sm *SessionManager) StartSession(ctx context.Context, config domain.SessionConfig) (*domain.Session, error) {
	instance := semdragons.GenerateInstance()
	sessionID := sm.boardConfig.EntityID("session", instance)

	session := &domain.Session{
		ID:     sessionID,
		Config: config,
		Active: true,
	}

	// Store session in KV
	if err := sm.putSession(ctx, instance, session); err != nil {
		return nil, fmt.Errorf("failed to store session: %w", err)
	}

	// Track in memory
	sm.sessionsMu.Lock()
	sm.sessions[sessionID] = session
	sm.sessionsMu.Unlock()

	// Emit session start event
	if err := SubjectSessionStart.Publish(ctx, sm.client, SessionStartPayload{
		SessionID: sessionID,
		Config:    config,
		StartedAt: time.Now(),
	}); err != nil {
		sm.logger.Warn("failed to publish session start event", "error", err)
	}

	sm.logger.Info("session started",
		"session_id", sessionID,
		"mode", config.Mode,
		"name", config.Name,
	)

	return session, nil
}

// EndSession wraps up a DM session and returns summary statistics.
func (sm *SessionManager) EndSession(ctx context.Context, sessionID string) (*domain.SessionSummary, error) {
	sm.sessionsMu.Lock()
	session, ok := sm.sessions[sessionID]
	if !ok {
		sm.sessionsMu.Unlock()
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	session.Active = false
	delete(sm.sessions, sessionID)
	sm.sessionsMu.Unlock()

	// Compute summary
	summary := sm.computeSessionSummary(ctx, sessionID)

	// Update session in KV
	instance := semdragons.ExtractInstance(sessionID)
	if err := sm.putSession(ctx, instance, session); err != nil {
		sm.logger.Warn("failed to update session state", "session_id", sessionID, "error", err)
	}

	// Emit session end event
	if err := SubjectSessionEnd.Publish(ctx, sm.client, SessionEndPayload{
		SessionID: sessionID,
		Summary:   *summary,
		EndedAt:   time.Now(),
	}); err != nil {
		sm.logger.Warn("failed to publish session end event", "error", err)
	}

	sm.logger.Info("session ended",
		"session_id", sessionID,
		"quests_completed", summary.QuestsCompleted,
		"quests_failed", summary.QuestsFailed,
		"total_xp", summary.TotalXPAwarded,
	)

	return summary, nil
}

// GetSession retrieves a session by ID.
func (sm *SessionManager) GetSession(ctx context.Context, sessionID string) (*domain.Session, error) {
	sm.sessionsMu.RLock()
	session, ok := sm.sessions[sessionID]
	sm.sessionsMu.RUnlock()

	if ok {
		return session, nil
	}

	// Try loading from storage
	instance := semdragons.ExtractInstance(sessionID)
	return sm.getSession(ctx, instance)
}

// ListActiveSessions returns all active sessions.
func (sm *SessionManager) ListActiveSessions() []*domain.Session {
	sm.sessionsMu.RLock()
	defer sm.sessionsMu.RUnlock()

	sessions := make([]*domain.Session, 0, len(sm.sessions))
	for _, session := range sm.sessions {
		if session.Active {
			sessions = append(sessions, session)
		}
	}
	return sessions
}

// =============================================================================
// SESSION STORAGE
// =============================================================================

func (sm *SessionManager) putSession(ctx context.Context, instance string, session *domain.Session) error {
	key := sm.SessionKey(instance)
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	bucket, err := sm.client.GetKeyValueBucket(ctx, sm.boardConfig.BucketName())
	if err != nil {
		return fmt.Errorf("get bucket: %w", err)
	}
	_, err = bucket.Put(ctx, key, data)
	return err
}

func (sm *SessionManager) getSession(ctx context.Context, instance string) (*domain.Session, error) {
	key := sm.SessionKey(instance)
	bucket, err := sm.client.GetKeyValueBucket(ctx, sm.boardConfig.BucketName())
	if err != nil {
		return nil, fmt.Errorf("get bucket: %w", err)
	}
	entry, err := bucket.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	var session domain.Session
	if err := json.Unmarshal(entry.Value(), &session); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &session, nil
}

// =============================================================================
// SESSION SUMMARY
// =============================================================================

func (sm *SessionManager) computeSessionSummary(ctx context.Context, sessionID string) *domain.SessionSummary {
	summary := &domain.SessionSummary{
		SessionID: sessionID,
	}

	// Load agents and compute stats
	agents, err := sm.loadAllAgents(ctx)
	if err == nil {
		for _, agent := range agents {
			if agent.Status != semdragons.AgentRetired {
				summary.AgentsActive++
			}
			summary.TotalXPAwarded += agent.Stats.TotalXPEarned
		}
	}

	return summary
}

// loadAllAgents retrieves all agents from the graph system.
func (sm *SessionManager) loadAllAgents(ctx context.Context) ([]*semdragons.Agent, error) {
	entities, err := sm.graph.ListAgentsByPrefix(ctx, 1000)
	if err != nil {
		return nil, err
	}

	agents := make([]*semdragons.Agent, 0, len(entities))
	for _, entity := range entities {
		agent := semdragons.AgentFromEntityState(&entity)
		if agent != nil {
			agents = append(agents, agent)
		}
	}
	return agents, nil
}

// =============================================================================
// EVENT WATCHING
// =============================================================================

// WatchEvents subscribes to the game event stream.
func (sm *SessionManager) WatchEvents(ctx context.Context, filter domain.EventFilter) (<-chan domain.GameEvent, error) {
	events := make(chan domain.GameEvent, 100)

	var wg sync.WaitGroup

	// Subscribe to quest lifecycle events
	if len(filter.Types) == 0 || containsQuestEventTypes(filter.Types) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sm.subscribeQuestEvents(ctx, events)
		}()
	}

	// Subscribe to agent events
	if len(filter.Types) == 0 || containsAgentEventTypes(filter.Types) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sm.subscribeAgentEvents(ctx, events)
		}()
	}

	// Subscribe to battle events
	if len(filter.Types) == 0 || containsBattleEventTypes(filter.Types) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sm.subscribeBattleEvents(ctx, events)
		}()
	}

	go func() {
		<-ctx.Done()
		wg.Wait()
		close(events)
	}()

	return events, nil
}

func containsQuestEventTypes(types []domain.GameEventType) bool {
	for _, t := range types {
		switch t {
		case domain.EventQuestPosted, domain.EventQuestClaimed, domain.EventQuestStarted,
			domain.EventQuestCompleted, domain.EventQuestFailed, domain.EventQuestEscalated:
			return true
		}
	}
	return false
}

func containsAgentEventTypes(types []domain.GameEventType) bool {
	for _, t := range types {
		switch t {
		case domain.EventAgentRecruited, domain.EventAgentLevelUp, domain.EventAgentLevelDown,
			domain.EventAgentDeath, domain.EventAgentPermadeath, domain.EventAgentRevived:
			return true
		}
	}
	return false
}

func containsBattleEventTypes(types []domain.GameEventType) bool {
	for _, t := range types {
		switch t {
		case domain.EventBattleStarted, domain.EventBattleVictory, domain.EventBattleDefeat:
			return true
		}
	}
	return false
}

func (sm *SessionManager) subscribeQuestEvents(ctx context.Context, events chan<- domain.GameEvent) {
	sm.logger.Debug("subscribing to quest events")

	const questLifecyclePrefix = "quest.lifecycle."
	subject := questLifecyclePrefix + ">"

	sub, err := sm.client.Subscribe(ctx, subject, func(_ context.Context, msg *nats.Msg) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		eventType := domain.EventQuestPosted
		suffix := strings.TrimPrefix(msg.Subject, questLifecyclePrefix)
		if suffix != msg.Subject {
			switch suffix {
			case "claimed":
				eventType = domain.EventQuestClaimed
			case "started":
				eventType = domain.EventQuestStarted
			case "completed":
				eventType = domain.EventQuestCompleted
			case "failed":
				eventType = domain.EventQuestFailed
			case "escalated":
				eventType = domain.EventQuestEscalated
			}
		}

		event := domain.GameEvent{
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
		sm.logger.Error("failed to subscribe to quest events", "error", err)
		return
	}

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
	}()
}

func (sm *SessionManager) subscribeAgentEvents(ctx context.Context, events chan<- domain.GameEvent) {
	sm.logger.Debug("subscribing to agent events")

	const agentProgressionPrefix = "agent.progression."
	subject := agentProgressionPrefix + ">"

	sub, err := sm.client.Subscribe(ctx, subject, func(_ context.Context, msg *nats.Msg) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		eventType := domain.EventAgentLevelUp
		suffix := strings.TrimPrefix(msg.Subject, agentProgressionPrefix)
		if suffix != msg.Subject {
			switch suffix {
			case "levelup":
				eventType = domain.EventAgentLevelUp
			case "leveldown":
				eventType = domain.EventAgentLevelDown
			case "death":
				eventType = domain.EventAgentDeath
			}
		}

		event := domain.GameEvent{
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
		sm.logger.Error("failed to subscribe to agent events", "error", err)
		return
	}

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
	}()
}

func (sm *SessionManager) subscribeBattleEvents(ctx context.Context, events chan<- domain.GameEvent) {
	sm.logger.Debug("subscribing to battle events")

	const battleReviewPrefix = "battle.review."
	subject := battleReviewPrefix + ">"

	sub, err := sm.client.Subscribe(ctx, subject, func(_ context.Context, msg *nats.Msg) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		eventType := domain.EventBattleStarted
		suffix := strings.TrimPrefix(msg.Subject, battleReviewPrefix)
		if suffix != msg.Subject {
			switch suffix {
			case "started":
				eventType = domain.EventBattleStarted
			case "victory":
				eventType = domain.EventBattleVictory
			case "defeat":
				eventType = domain.EventBattleDefeat
			}
		}

		event := domain.GameEvent{
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
		sm.logger.Error("failed to subscribe to battle events", "error", err)
		return
	}

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
	}()
}

// Graph returns the graph client for entity operations.
func (sm *SessionManager) Graph() *semdragons.GraphClient {
	return sm.graph
}
