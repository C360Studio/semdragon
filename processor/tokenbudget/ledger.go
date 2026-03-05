// Package tokenbudget provides a token usage ledger with circuit-breaker
// semantics. All three LLM call paths (quest execution, boss battle, DM chat)
// share a single TokenLedger instance to enforce a global hourly budget.
//
// Hot-path methods (Record, Check, Stats) use atomic counters with no locks
// or KV round-trips. KV persistence is async/best-effort for restart recovery.
package tokenbudget

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// BucketName is the NATS KV bucket for token budget state.
const BucketName = "TOKEN_BUDGET"

// KV key names.
const (
	keyBudgetConfig = "budget.config"
	keyUsageTotal   = "usage.total"
)

// Breaker state string constants.
const (
	BreakerOK      = "ok"
	BreakerWarning = "warning"
	BreakerTripped = "tripped"
)

// DefaultHourlyLimit is the safe default: 1M tokens/hour.
const DefaultHourlyLimit int64 = 1_000_000

// EndpointCost holds per-endpoint pricing in USD per 1M tokens.
type EndpointCost struct {
	InputPer1M  float64 `json:"input_per_1m_tokens"`
	OutputPer1M float64 `json:"output_per_1m_tokens"`
}

// BudgetConfig holds the configurable limits.
type BudgetConfig struct {
	GlobalHourlyLimit int64                   `json:"global_hourly_limit"`          // 0 = unlimited
	EndpointPricing   map[string]EndpointCost `json:"endpoint_pricing,omitempty"`
}

// TokenStats is the snapshot returned by Stats() and the /board/tokens endpoint.
type TokenStats struct {
	HourlyUsage   UsageSnapshot `json:"hourly_usage"`
	TotalUsage    UsageSnapshot `json:"total_usage"`
	HourlyLimit   int64         `json:"hourly_limit"`
	BudgetPct     float64       `json:"budget_pct"`
	Breaker       string        `json:"breaker"`
	HourlyEpoch   int64         `json:"hourly_epoch"`
	HourlyCostUSD float64       `json:"hourly_cost_usd"`
	TotalCostUSD  float64       `json:"total_cost_usd"`
}

// UsageSnapshot holds prompt/completion/total token counts.
type UsageSnapshot struct {
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

// BoardPauser pauses the game board. Implemented by a thin adapter around
// boardcontrol.Controller since its Pause returns a concrete type.
type BoardPauser interface {
	PauseBoard(ctx context.Context, actor string) error
}

// TokenLedger tracks token usage with atomic counters on the hot path.
type TokenLedger struct {
	// Hourly rolling window
	hourlyPrompt     atomic.Int64
	hourlyCompletion atomic.Int64
	hourlyEpoch      atomic.Int64 // Unix hour (time / 3600)

	// All-time counters
	totalPrompt     atomic.Int64
	totalCompletion atomic.Int64

	// Cost tracking in micro-dollars (1 USD = 1,000,000) for atomic int64
	hourlyCostMicro atomic.Int64
	totalCostMicro  atomic.Int64

	// Pricing lookup (immutable after init, no lock needed)
	pricing map[string]EndpointCost

	// Budget
	hourlyLimit atomic.Int64

	// Optional integrations
	pauser BoardPauser
	bucket jetstream.KeyValue
	logger *slog.Logger
}

// NewTokenLedger creates a ledger with the given budget config.
func NewTokenLedger(cfg *BudgetConfig, logger *slog.Logger) *TokenLedger {
	l := &TokenLedger{logger: logger}
	limit := DefaultHourlyLimit
	if cfg != nil && cfg.GlobalHourlyLimit > 0 {
		limit = cfg.GlobalHourlyLimit
	}
	if cfg != nil && len(cfg.EndpointPricing) > 0 {
		l.pricing = cfg.EndpointPricing
	}
	l.hourlyLimit.Store(limit)
	l.hourlyEpoch.Store(currentEpoch())
	return l
}

// SetBoardPauser injects the board pauser for auto-pause on budget exceeded.
func (l *TokenLedger) SetBoardPauser(p BoardPauser) {
	l.pauser = p
}

// SetBucket attaches a KV bucket for persistence. Call before Start().
func (l *TokenLedger) SetBucket(bucket jetstream.KeyValue) {
	l.bucket = bucket
}

// Start hydrates counters from KV on restart.
func (l *TokenLedger) Start(ctx context.Context) error {
	if l.bucket == nil {
		return nil
	}

	// Load budget config from KV (overrides constructor config).
	if entry, err := l.bucket.Get(ctx, keyBudgetConfig); err == nil {
		var cfg BudgetConfig
		if jsonErr := json.Unmarshal(entry.Value(), &cfg); jsonErr == nil && cfg.GlobalHourlyLimit > 0 {
			l.hourlyLimit.Store(cfg.GlobalHourlyLimit)
		}
	}

	// Load total usage from KV.
	if entry, err := l.bucket.Get(ctx, keyUsageTotal); err == nil {
		var snap UsageSnapshot
		if jsonErr := json.Unmarshal(entry.Value(), &snap); jsonErr == nil {
			l.totalPrompt.Store(snap.PromptTokens)
			l.totalCompletion.Store(snap.CompletionTokens)
			if snap.EstimatedCostUSD > 0 {
				l.totalCostMicro.Store(int64(snap.EstimatedCostUSD * 1_000_000))
			}
		}
	}

	l.logger.Info("token ledger started",
		"hourly_limit", l.hourlyLimit.Load(),
		"total_prompt", l.totalPrompt.Load(),
		"total_completion", l.totalCompletion.Load())

	return nil
}

// Record adds token usage to both hourly and total counters.
// If the hourly budget is exceeded after recording, auto-pauses the board.
// The endpoint parameter is used for cost estimation via the pricing map.
func (l *TokenLedger) Record(ctx context.Context, promptTokens, completionTokens int, source, endpoint string) {
	l.rollEpochIfNeeded()

	l.hourlyPrompt.Add(int64(promptTokens))
	l.hourlyCompletion.Add(int64(completionTokens))
	l.totalPrompt.Add(int64(promptTokens))
	l.totalCompletion.Add(int64(completionTokens))

	// Accumulate cost if pricing is configured for this endpoint.
	if pricing, ok := l.pricing[endpoint]; ok {
		costMicro := int64((float64(promptTokens)*pricing.InputPer1M +
			float64(completionTokens)*pricing.OutputPer1M) / 1_000_000 * 1_000_000)
		l.hourlyCostMicro.Add(costMicro)
		l.totalCostMicro.Add(costMicro)
	}

	l.logger.Debug("tokens recorded",
		"prompt", promptTokens,
		"completion", completionTokens,
		"source", source,
		"endpoint", endpoint,
		"hourly_total", l.hourlyPrompt.Load()+l.hourlyCompletion.Load())

	// Check if budget is now exceeded and auto-pause.
	if l.pauser != nil && l.breakerState() == BreakerTripped {
		l.logger.Warn("token budget exceeded, auto-pausing board",
			"hourly_total", l.hourlyPrompt.Load()+l.hourlyCompletion.Load(),
			"hourly_limit", l.hourlyLimit.Load(),
			"source", source)
		if err := l.pauser.PauseBoard(ctx, "token_budget"); err != nil {
			l.logger.Error("failed to auto-pause board", "error", err)
		}
	}

	// Async KV persist (best-effort).
	l.persistTotalAsync(ctx)
}

// Check returns an error if the hourly budget is exceeded.
func (l *TokenLedger) Check() error {
	l.rollEpochIfNeeded()

	limit := l.hourlyLimit.Load()
	if limit <= 0 {
		return nil // unlimited
	}

	hourlyTotal := l.hourlyPrompt.Load() + l.hourlyCompletion.Load()
	if hourlyTotal >= limit {
		return fmt.Errorf("token budget exceeded: %d/%d tokens used this hour", hourlyTotal, limit)
	}
	return nil
}

// Stats returns the current usage snapshot.
// Each atomic counter is loaded exactly once into a local variable so that
// derived fields (TotalTokens, BudgetPct) are computed from the same values
// that appear in the snapshot — preventing total_tokens != prompt + completion
// inconsistencies under concurrent updates.
func (l *TokenLedger) Stats() TokenStats {
	l.rollEpochIfNeeded()

	limit := l.hourlyLimit.Load()
	hp := l.hourlyPrompt.Load()
	hc := l.hourlyCompletion.Load()
	tp := l.totalPrompt.Load()
	tc := l.totalCompletion.Load()
	hourlyTotal := hp + hc

	var pct float64
	if limit > 0 {
		pct = float64(hourlyTotal) / float64(limit)
	}

	hourlyCostUSD := float64(l.hourlyCostMicro.Load()) / 1_000_000
	totalCostUSD := float64(l.totalCostMicro.Load()) / 1_000_000

	return TokenStats{
		HourlyUsage: UsageSnapshot{
			PromptTokens:     hp,
			CompletionTokens: hc,
			TotalTokens:      hourlyTotal,
			EstimatedCostUSD: hourlyCostUSD,
		},
		TotalUsage: UsageSnapshot{
			PromptTokens:     tp,
			CompletionTokens: tc,
			TotalTokens:      tp + tc,
			EstimatedCostUSD: totalCostUSD,
		},
		HourlyLimit:   limit,
		BudgetPct:     pct,
		Breaker:       l.breakerState(),
		HourlyEpoch:   l.hourlyEpoch.Load(),
		HourlyCostUSD: hourlyCostUSD,
		TotalCostUSD:  totalCostUSD,
	}
}

// SetBudget updates the hourly limit and persists to KV.
// KV persistence happens before the in-memory store so that a failed write
// never leaves the two sources out of sync.
func (l *TokenLedger) SetBudget(ctx context.Context, limit int64) error {
	if limit < 0 {
		return fmt.Errorf("hourly limit must be non-negative")
	}

	if l.bucket != nil {
		cfg := BudgetConfig{GlobalHourlyLimit: limit}
		data, err := json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshal budget config: %w", err)
		}
		if _, err := l.bucket.Put(ctx, keyBudgetConfig, data); err != nil {
			return fmt.Errorf("persist budget config: %w", err)
		}
	}

	l.hourlyLimit.Store(limit)
	l.logger.Info("token budget updated", "hourly_limit", limit)
	return nil
}

// breakerState computes the breaker status from current usage.
func (l *TokenLedger) breakerState() string {
	limit := l.hourlyLimit.Load()
	if limit <= 0 {
		return BreakerOK
	}

	hourlyTotal := l.hourlyPrompt.Load() + l.hourlyCompletion.Load()
	pct := float64(hourlyTotal) / float64(limit)

	if pct >= 1.0 {
		return BreakerTripped
	}
	if pct >= 0.8 {
		return BreakerWarning
	}
	return BreakerOK
}

// rollEpochIfNeeded resets hourly counters when the hour changes.
//
// Narrow race window: between the CAS on hourlyEpoch and the two Store calls
// below, a concurrent Record call may read the new epoch (CAS already applied)
// but still add tokens to the old counters before they are zeroed. Those tokens
// are silently dropped when Store(0) runs. This is intentional — budget
// enforcement can tolerate minor under-counting at epoch boundaries, and
// the alternative (a mutex here) would serialize every hot-path Record call.
func (l *TokenLedger) rollEpochIfNeeded() {
	now := currentEpoch()
	old := l.hourlyEpoch.Load()
	if now != old {
		if l.hourlyEpoch.CompareAndSwap(old, now) {
			l.hourlyPrompt.Store(0)
			l.hourlyCompletion.Store(0)
			l.hourlyCostMicro.Store(0)
		}
	}
}

// currentEpoch returns the current Unix hour (seconds / 3600).
func currentEpoch() int64 {
	return time.Now().Unix() / 3600
}

// persistTotalAsync saves total usage to KV in a goroutine. Best-effort only.
// Each atomic counter is read once before the goroutine launches so the
// snapshot is internally consistent (Fix #5 applied here too).
// context.Background() is used instead of the caller's ctx because the caller
// (e.g., an HTTP request) may be cancelled before the async write completes.
func (l *TokenLedger) persistTotalAsync(_ context.Context) {
	if l.bucket == nil {
		return
	}

	tp := l.totalPrompt.Load()
	tc := l.totalCompletion.Load()
	totalCostUSD := float64(l.totalCostMicro.Load()) / 1_000_000
	snap := UsageSnapshot{
		PromptTokens:     tp,
		CompletionTokens: tc,
		TotalTokens:      tp + tc,
		EstimatedCostUSD: totalCostUSD,
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}

	go func() {
		putCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, putErr := l.bucket.Put(putCtx, keyUsageTotal, data); putErr != nil {
			l.logger.Debug("failed to persist total usage to KV", "error", putErr)
		}
	}()
}
