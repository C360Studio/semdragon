// Package boardcontrol provides a KV-backed play/pause controller for the
// game board. Components check PauseChecker.Paused() before starting new
// autonomous actions. The Controller watches a BOARD_CONTROL KV bucket so
// pause state persists across restarts and propagates across replicas.
package boardcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// BucketName is the NATS KV bucket used for board control state.
const BucketName = "BOARD_CONTROL"

// boardStateKey is the KV key for the pause flag.
const boardStateKey = "board.state"

// resumeSubject is the NATS pub/sub subject broadcast on unpause so
// components like QuestBridge can reconcile deferred work.
const resumeSubject = "board.control.resumed"

// PauseChecker is implemented by anything that can report whether the board
// is currently paused. Components hold a PauseChecker and skip new autonomous
// work when Paused() returns true. A nil PauseChecker means always-running.
type PauseChecker interface {
	Paused() bool
}

// BoardState is the JSON structure stored in the BOARD_CONTROL KV bucket.
type BoardState struct {
	Paused   bool    `json:"paused"`
	PausedAt *string `json:"paused_at"` // RFC 3339 or null
	PausedBy *string `json:"paused_by"` // identifier or null
}

// Controller watches the BOARD_CONTROL KV bucket and maintains an atomic
// pause flag that components read without locking.
type Controller struct {
	paused   atomic.Bool
	pausedAt atomic.Value // stores *string (RFC 3339 timestamp or nil)
	pausedBy atomic.Value // stores *string (actor identifier or nil)

	bucket jetstream.KeyValue
	nats   *natsclient.Client
	logger *slog.Logger

	stopChan chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewController creates a Controller that watches the given KV bucket for
// pause state changes. Call Start to begin watching and Stop to tear down.
func NewController(bucket jetstream.KeyValue, nats *natsclient.Client, logger *slog.Logger) *Controller {
	return &Controller{
		bucket:   bucket,
		nats:     nats,
		logger:   logger,
		stopChan: make(chan struct{}),
	}
}

// Paused returns true when the board is paused.
func (c *Controller) Paused() bool {
	return c.paused.Load()
}

// State returns the current board state snapshot.
func (c *Controller) State() BoardState {
	st := BoardState{Paused: c.paused.Load()}
	if v, ok := c.pausedAt.Load().(*string); ok {
		st.PausedAt = v
	}
	if v, ok := c.pausedBy.Load().(*string); ok {
		st.PausedBy = v
	}
	return st
}

// Pause sets the board to paused state, persisting to KV.
func (c *Controller) Pause(ctx context.Context, actor string) (BoardState, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	st := BoardState{
		Paused:   true,
		PausedAt: &now,
	}
	if actor != "" {
		st.PausedBy = &actor
	}

	data, err := json.Marshal(st)
	if err != nil {
		return BoardState{}, fmt.Errorf("marshal board state: %w", err)
	}

	if _, err := c.bucket.Put(ctx, boardStateKey, data); err != nil {
		return BoardState{}, fmt.Errorf("put board state: %w", err)
	}

	// Apply locally immediately (watcher will also fire).
	c.applyState(st)
	return st, nil
}

// Resume sets the board to running state, persisting to KV.
// Publishes a resume notification so components can reconcile deferred work.
func (c *Controller) Resume(ctx context.Context) (BoardState, error) {
	st := BoardState{Paused: false}

	data, err := json.Marshal(st)
	if err != nil {
		return BoardState{}, fmt.Errorf("marshal board state: %w", err)
	}

	if _, err := c.bucket.Put(ctx, boardStateKey, data); err != nil {
		return BoardState{}, fmt.Errorf("put board state: %w", err)
	}

	// Apply locally immediately.
	c.applyState(st)

	// Notify components of resume so they can reconcile.
	if err := c.nats.Publish(ctx, resumeSubject, nil); err != nil {
		c.logger.Warn("failed to publish resume notification", "error", err)
		// Non-fatal: state is already updated in KV.
	}

	return st, nil
}

// Start begins watching the KV bucket for state changes. Loads the current
// state synchronously before returning so callers can rely on Paused() after Start.
func (c *Controller) Start(ctx context.Context) error {
	// Load initial state.
	entry, err := c.bucket.Get(ctx, boardStateKey)
	if err != nil {
		// Key not found is fine — default is running (not paused).
		if !isKeyNotFound(err) {
			return fmt.Errorf("load initial board state: %w", err)
		}
	} else {
		var st BoardState
		if jsonErr := json.Unmarshal(entry.Value(), &st); jsonErr == nil {
			c.applyState(st)
		}
	}

	// Start KV watcher goroutine.
	watcher, err := c.bucket.Watch(ctx, boardStateKey)
	if err != nil {
		return fmt.Errorf("watch board state: %w", err)
	}

	c.wg.Add(1)
	go c.watchLoop(watcher)

	c.logger.Info("board control started", "paused", c.paused.Load())
	return nil
}

// Stop tears down the watcher goroutine.
func (c *Controller) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopChan)
	})
	c.wg.Wait()
}

// ResumeSubject returns the NATS subject used for resume notifications.
// Components subscribe to this to trigger reconciliation after unpause.
func ResumeSubject() string {
	return resumeSubject
}

// EnsureBucket creates the BOARD_CONTROL KV bucket if it doesn't exist.
func EnsureBucket(ctx context.Context, nats *natsclient.Client) (jetstream.KeyValue, error) {
	return nats.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      BucketName,
		Description: "Board play/pause control state",
		History:     5,
	})
}

// applyState updates the atomic fields from a BoardState.
func (c *Controller) applyState(st BoardState) {
	c.paused.Store(st.Paused)
	c.pausedAt.Store(st.PausedAt)
	c.pausedBy.Store(st.PausedBy)
}

// watchLoop processes KV watch updates until stopped.
func (c *Controller) watchLoop(watcher jetstream.KeyWatcher) {
	defer c.wg.Done()
	defer watcher.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case entry, ok := <-watcher.Updates():
			if !ok {
				return
			}
			// Nil sentinel marks end of historical replay.
			if entry == nil {
				continue
			}

			var st BoardState
			if err := json.Unmarshal(entry.Value(), &st); err != nil {
				c.logger.Warn("failed to decode board state", "error", err)
				continue
			}

			wasPaused := c.paused.Load()
			c.applyState(st)

			if wasPaused != st.Paused {
				if st.Paused {
					c.logger.Info("board paused")
				} else {
					c.logger.Info("board resumed")
				}
			}
		}
	}
}

// isKeyNotFound checks for NATS KV key-not-found errors.
// Uses errors.Is first for proper sentinel matching, with string fallback for wrapped errors.
func isKeyNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return true
	}
	return strings.Contains(err.Error(), "key not found")
}
