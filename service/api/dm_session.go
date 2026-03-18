package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// DM session KV bucket configuration.
const (
	dmSessionBucketName = "DM_SESSIONS"
	dmSessionKeyPrefix  = "dm.session."
	dmSessionHistory    = 5
	dmSessionTTL        = 7 * 24 * time.Hour

	// trajectoryQuerySubject is the NATS request/reply subject served by agentic-loop
	// for trajectory lookups. Replaces direct AGENT_TRAJECTORIES KV reads.
	trajectoryQuerySubject = "agentic.query.trajectory"

	// trajectoryQueryTimeout is the timeout for trajectory request/reply calls.
	trajectoryQueryTimeout = 5 * time.Second

	// appendTurnMaxRetries is the maximum number of CAS retry attempts in appendTurn.
	appendTurnMaxRetries = 3
)

// DMChatSession represents a persisted DM chat session.
type DMChatSession struct {
	SessionID string       `json:"session_id"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	Turns     []DMChatTurn `json:"turns"`
}

// DMChatTurn represents a single user→DM exchange in a chat session.
type DMChatTurn struct {
	UserMessage string    `json:"user_message"`
	DMResponse  string    `json:"dm_response"`
	Timestamp   time.Time `json:"timestamp"`
	TraceID     string    `json:"trace_id,omitempty"`
	SpanID      string    `json:"span_id,omitempty"`
	ToolsUsed   []string  `json:"tools_used,omitempty"`
}

// dmSessionStore wraps NATS KV for DM chat session persistence.
// Bucket creation is lazy — the store degrades gracefully if KV is unavailable.
// mu guards the cached bucket field so concurrent callers do not race on first access.
type dmSessionStore struct {
	nats   *natsclient.Client
	logger *slog.Logger
	mu     sync.Mutex
	bucket jetstream.KeyValue // cached after first successful access; guarded by mu
}

// ensureBucket creates the DM_SESSIONS bucket if it doesn't exist (idempotent).
// On failure the bucket field is left nil so the next call retries.
func (s *dmSessionStore) ensureBucket(ctx context.Context) (jetstream.KeyValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.bucket != nil {
		return s.bucket, nil
	}
	bucket, err := s.nats.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      dmSessionBucketName,
		Description: "DM chat session persistence",
		History:     dmSessionHistory,
		TTL:         dmSessionTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("ensure DM session bucket: %w", err)
	}
	s.bucket = bucket
	return bucket, nil
}

// getBucket returns the bucket without creating it. Returns nil, nil if missing.
// On failure the bucket field is left nil so the next call retries.
func (s *dmSessionStore) getBucket(ctx context.Context) (jetstream.KeyValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.bucket != nil {
		return s.bucket, nil
	}
	bucket, err := s.nats.GetKeyValueBucket(ctx, dmSessionBucketName)
	if err != nil {
		if isBucketNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get DM session bucket: %w", err)
	}
	s.bucket = bucket
	return bucket, nil
}

// sessionKey builds the KV key for a session ID.
func sessionKey(sessionID string) string {
	return dmSessionKeyPrefix + sessionID
}

// appendTurn adds a turn to an existing session or creates a new one.
// Uses compare-and-swap (Create for new keys, Update for existing keys) to avoid
// lost-update races when multiple callers append concurrently.
// Best-effort — callers should not fail their response on KV errors.
func (s *dmSessionStore) appendTurn(ctx context.Context, sessionID string, turn DMChatTurn) error {
	bucket, err := s.ensureBucket(ctx)
	if err != nil {
		return err
	}

	key := sessionKey(sessionID)

	for attempt := range appendTurnMaxRetries {
		now := time.Now()

		// Try to read the existing entry and capture its revision for CAS.
		entry, getErr := bucket.Get(ctx, key)
		if getErr != nil && !isKeyNotFound(getErr) {
			return fmt.Errorf("read session %s: %w", sessionID, getErr)
		}

		var session DMChatSession

		if getErr == nil && entry != nil {
			// Existing session — unmarshal current state.
			if jsonErr := json.Unmarshal(entry.Value(), &session); jsonErr != nil {
				s.logger.Warn("Corrupt DM session, starting fresh",
					"session_id", sessionID,
					"error", jsonErr,
					"attempt", attempt+1)
				session = DMChatSession{}
			}
		}

		if session.SessionID == "" {
			session.SessionID = sessionID
			session.CreatedAt = now
		}
		session.UpdatedAt = now
		session.Turns = append(session.Turns, turn)

		data, marshalErr := json.Marshal(session)
		if marshalErr != nil {
			return fmt.Errorf("marshal session %s: %w", sessionID, marshalErr)
		}

		if isKeyNotFound(getErr) {
			// Key doesn't exist yet — use Create for atomic first-write.
			_, casErr := bucket.Create(ctx, key, data)
			if casErr == nil {
				return nil
			}
			// Another writer created the key between our Get and Create; retry.
			s.logger.Debug("CAS create conflict, retrying",
				"session_id", sessionID,
				"attempt", attempt+1)
			continue
		}

		// Key exists — use Update with the revision we just read.
		_, casErr := bucket.Update(ctx, key, data, entry.Revision())
		if casErr == nil {
			return nil
		}
		// Another writer updated the key between our Get and Update; retry.
		s.logger.Debug("CAS update conflict, retrying",
			"session_id", sessionID,
			"attempt", attempt+1)
	}

	return fmt.Errorf("write session %s: exceeded %d CAS retries", sessionID, appendTurnMaxRetries)
}

// GetSession satisfies the DMSessionReader interface.
// Returns nil, nil if the session does not exist.
func (s *dmSessionStore) GetSession(ctx context.Context, sessionID string) (*DMChatSession, error) {
	bucket, err := s.getBucket(ctx)
	if err != nil {
		return nil, err
	}
	if bucket == nil {
		return nil, nil
	}

	entry, err := bucket.Get(ctx, sessionKey(sessionID))
	if err != nil {
		if isKeyNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session %s: %w", sessionID, err)
	}

	var session DMChatSession
	if err := json.Unmarshal(entry.Value(), &session); err != nil {
		return nil, fmt.Errorf("unmarshal session %s: %w", sessionID, err)
	}
	return &session, nil
}

// natsTrajectoryQuerier reads trajectory data via NATS request/reply to agentic-loop.
// The agentic-loop component serves trajectories from its in-memory cache (active
// and recently completed loops) on the "agentic.query.trajectory" subject.
type natsTrajectoryQuerier struct {
	nats *natsclient.Client
}

func (q *natsTrajectoryQuerier) GetTrajectory(ctx context.Context, id string) ([]byte, error) {
	req, err := json.Marshal(struct {
		LoopID string `json:"loopId"`
	}{LoopID: id})
	if err != nil {
		return nil, fmt.Errorf("marshal trajectory request: %w", err)
	}
	data, err := q.nats.Request(ctx, trajectoryQuerySubject, req, trajectoryQueryTimeout)
	if err != nil {
		return nil, err
	}
	return data, nil
}
