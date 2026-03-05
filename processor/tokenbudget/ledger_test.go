package tokenbudget

import (
	"context"
	"log/slog"
	"math"
	"os"
	"sync"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNewTokenLedger_DefaultLimit(t *testing.T) {
	l := NewTokenLedger(nil, testLogger())
	if l.hourlyLimit.Load() != DefaultHourlyLimit {
		t.Errorf("expected default limit %d, got %d", DefaultHourlyLimit, l.hourlyLimit.Load())
	}
}

func TestNewTokenLedger_CustomLimit(t *testing.T) {
	cfg := &BudgetConfig{GlobalHourlyLimit: 500_000}
	l := NewTokenLedger(cfg, testLogger())
	if l.hourlyLimit.Load() != 500_000 {
		t.Errorf("expected limit 500000, got %d", l.hourlyLimit.Load())
	}
}

func TestRecord_AccumulatesTotals(t *testing.T) {
	l := NewTokenLedger(nil, testLogger())
	ctx := context.Background()

	l.Record(ctx, 100, 200, "test", "")
	l.Record(ctx, 50, 75, "test", "")

	stats := l.Stats()
	if stats.HourlyUsage.PromptTokens != 150 {
		t.Errorf("hourly prompt = %d, want 150", stats.HourlyUsage.PromptTokens)
	}
	if stats.HourlyUsage.CompletionTokens != 275 {
		t.Errorf("hourly completion = %d, want 275", stats.HourlyUsage.CompletionTokens)
	}
	if stats.HourlyUsage.TotalTokens != 425 {
		t.Errorf("hourly total = %d, want 425", stats.HourlyUsage.TotalTokens)
	}
	if stats.TotalUsage.TotalTokens != 425 {
		t.Errorf("total = %d, want 425", stats.TotalUsage.TotalTokens)
	}
}

func TestCheck_UnderBudget(t *testing.T) {
	l := NewTokenLedger(&BudgetConfig{GlobalHourlyLimit: 1000}, testLogger())
	ctx := context.Background()

	l.Record(ctx, 100, 200, "test", "")

	if err := l.Check(); err != nil {
		t.Errorf("expected no error under budget, got: %v", err)
	}
}

func TestCheck_OverBudget(t *testing.T) {
	l := NewTokenLedger(&BudgetConfig{GlobalHourlyLimit: 100}, testLogger())
	ctx := context.Background()

	l.Record(ctx, 50, 60, "test", "") // 110 > 100

	if err := l.Check(); err == nil {
		t.Error("expected error when over budget")
	}
}

func TestCheck_Unlimited(t *testing.T) {
	l := NewTokenLedger(&BudgetConfig{GlobalHourlyLimit: 0}, testLogger())
	// Zero means unlimited — override the default
	l.hourlyLimit.Store(0)
	ctx := context.Background()

	l.Record(ctx, 999_999_999, 999_999_999, "test", "")

	if err := l.Check(); err != nil {
		t.Errorf("expected no error for unlimited, got: %v", err)
	}
}

func TestStats_BreakerStates(t *testing.T) {
	tests := []struct {
		name    string
		limit   int64
		prompt  int
		comp    int
		breaker string
	}{
		{"ok", 1000, 100, 100, BreakerOK},
		{"warning at 80%", 1000, 400, 400, BreakerWarning},
		{"tripped at 100%", 1000, 500, 500, BreakerTripped},
		{"tripped over 100%", 1000, 600, 600, BreakerTripped},
		{"unlimited", 0, 999999, 999999, BreakerOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewTokenLedger(&BudgetConfig{GlobalHourlyLimit: tt.limit}, testLogger())
			if tt.limit == 0 {
				l.hourlyLimit.Store(0)
			}
			ctx := context.Background()
			l.Record(ctx, tt.prompt, tt.comp, "test", "")

			stats := l.Stats()
			if stats.Breaker != tt.breaker {
				t.Errorf("breaker = %q, want %q (hourly=%d, limit=%d)",
					stats.Breaker, tt.breaker, stats.HourlyUsage.TotalTokens, stats.HourlyLimit)
			}
		})
	}
}

func TestStats_BudgetPct(t *testing.T) {
	l := NewTokenLedger(&BudgetConfig{GlobalHourlyLimit: 1000}, testLogger())
	ctx := context.Background()

	l.Record(ctx, 250, 250, "test", "") // 500/1000 = 0.5

	stats := l.Stats()
	if stats.BudgetPct < 0.49 || stats.BudgetPct > 0.51 {
		t.Errorf("budget_pct = %f, want ~0.5", stats.BudgetPct)
	}
}

func TestSetBudget_UpdatesLimit(t *testing.T) {
	l := NewTokenLedger(nil, testLogger())
	ctx := context.Background()

	if err := l.SetBudget(ctx, 2_000_000); err != nil {
		t.Fatalf("SetBudget failed: %v", err)
	}

	if l.hourlyLimit.Load() != 2_000_000 {
		t.Errorf("limit = %d, want 2000000", l.hourlyLimit.Load())
	}
}

func TestSetBudget_RejectsNegative(t *testing.T) {
	l := NewTokenLedger(nil, testLogger())
	ctx := context.Background()

	if err := l.SetBudget(ctx, -1); err == nil {
		t.Error("expected error for negative limit")
	}
}

func TestRollEpoch_ResetsHourlyCounters(t *testing.T) {
	l := NewTokenLedger(&BudgetConfig{GlobalHourlyLimit: 1000}, testLogger())
	ctx := context.Background()

	l.Record(ctx, 100, 200, "test", "")

	// Simulate an epoch change by setting the epoch to the past.
	l.hourlyEpoch.Store(l.hourlyEpoch.Load() - 1)

	stats := l.Stats()
	if stats.HourlyUsage.TotalTokens != 0 {
		t.Errorf("hourly total after epoch roll = %d, want 0", stats.HourlyUsage.TotalTokens)
	}
	// Total should still be accumulated.
	if stats.TotalUsage.TotalTokens != 300 {
		t.Errorf("total after epoch roll = %d, want 300", stats.TotalUsage.TotalTokens)
	}
}

type mockPauser struct {
	called bool
	actor  string
}

func (m *mockPauser) PauseBoard(_ context.Context, actor string) error {
	m.called = true
	m.actor = actor
	return nil
}

func TestRecord_AutoPausesOnTripped(t *testing.T) {
	l := NewTokenLedger(&BudgetConfig{GlobalHourlyLimit: 100}, testLogger())
	mp := &mockPauser{}
	l.SetBoardPauser(mp)
	ctx := context.Background()

	l.Record(ctx, 50, 60, "test", "") // 110 > 100

	if !mp.called {
		t.Error("expected auto-pause to be called when budget exceeded")
	}
	if mp.actor != "token_budget" {
		t.Errorf("pause actor = %q, want %q", mp.actor, "token_budget")
	}
}

func TestRecord_NoPauseWhenUnderBudget(t *testing.T) {
	l := NewTokenLedger(&BudgetConfig{GlobalHourlyLimit: 1000}, testLogger())
	mp := &mockPauser{}
	l.SetBoardPauser(mp)
	ctx := context.Background()

	l.Record(ctx, 10, 20, "test", "")

	if mp.called {
		t.Error("expected no auto-pause when under budget")
	}
}

func TestStart_NoKV(t *testing.T) {
	l := NewTokenLedger(nil, testLogger())
	if err := l.Start(context.Background()); err != nil {
		t.Errorf("Start without KV should succeed, got: %v", err)
	}
}

func TestRecord_ConcurrentSafety(t *testing.T) {
	l := NewTokenLedger(&BudgetConfig{GlobalHourlyLimit: 10_000_000}, testLogger())
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				l.Record(ctx, 10, 20, "test", "")
				_ = l.Check()
				_ = l.Stats()
			}
		}()
	}
	wg.Wait()

	stats := l.Stats()
	if stats.TotalUsage.PromptTokens != 100_000 {
		t.Errorf("prompt = %d, want 100000", stats.TotalUsage.PromptTokens)
	}
	if stats.TotalUsage.CompletionTokens != 200_000 {
		t.Errorf("completion = %d, want 200000", stats.TotalUsage.CompletionTokens)
	}
}

// =============================================================================
// COST TRACKING TESTS
// =============================================================================

func TestRecord_WithPricing_AccumulatesCost(t *testing.T) {
	cfg := &BudgetConfig{
		GlobalHourlyLimit: 10_000_000,
		EndpointPricing: map[string]EndpointCost{
			"gpt-4": {InputPer1M: 30.0, OutputPer1M: 60.0},
		},
	}
	l := NewTokenLedger(cfg, testLogger())
	ctx := context.Background()

	// 1000 prompt tokens @ $30/1M = $0.03
	// 500 completion tokens @ $60/1M = $0.03
	// Total = $0.06
	l.Record(ctx, 1000, 500, "test", "gpt-4")

	stats := l.Stats()
	if math.Abs(stats.HourlyCostUSD-0.06) > 0.001 {
		t.Errorf("hourly cost = %f, want ~0.06", stats.HourlyCostUSD)
	}
	if math.Abs(stats.TotalCostUSD-0.06) > 0.001 {
		t.Errorf("total cost = %f, want ~0.06", stats.TotalCostUSD)
	}
	if math.Abs(stats.HourlyUsage.EstimatedCostUSD-0.06) > 0.001 {
		t.Errorf("hourly usage estimated cost = %f, want ~0.06", stats.HourlyUsage.EstimatedCostUSD)
	}
}

func TestRecord_UnknownEndpoint_NoCost(t *testing.T) {
	cfg := &BudgetConfig{
		GlobalHourlyLimit: 10_000_000,
		EndpointPricing: map[string]EndpointCost{
			"gpt-4": {InputPer1M: 30.0, OutputPer1M: 60.0},
		},
	}
	l := NewTokenLedger(cfg, testLogger())
	ctx := context.Background()

	l.Record(ctx, 1000, 500, "test", "unknown-endpoint")

	stats := l.Stats()
	if stats.HourlyCostUSD != 0 {
		t.Errorf("hourly cost = %f, want 0 for unknown endpoint", stats.HourlyCostUSD)
	}
}

func TestRecord_EmptyEndpoint_NoCost(t *testing.T) {
	cfg := &BudgetConfig{
		GlobalHourlyLimit: 10_000_000,
		EndpointPricing: map[string]EndpointCost{
			"gpt-4": {InputPer1M: 30.0, OutputPer1M: 60.0},
		},
	}
	l := NewTokenLedger(cfg, testLogger())
	ctx := context.Background()

	l.Record(ctx, 1000, 500, "test", "")

	stats := l.Stats()
	if stats.HourlyCostUSD != 0 {
		t.Errorf("hourly cost = %f, want 0 for empty endpoint", stats.HourlyCostUSD)
	}
}

func TestRollEpoch_ResetsHourlyCost(t *testing.T) {
	cfg := &BudgetConfig{
		GlobalHourlyLimit: 10_000_000,
		EndpointPricing: map[string]EndpointCost{
			"gpt-4": {InputPer1M: 30.0, OutputPer1M: 60.0},
		},
	}
	l := NewTokenLedger(cfg, testLogger())
	ctx := context.Background()

	l.Record(ctx, 1000, 500, "test", "gpt-4")

	// Simulate an epoch change.
	l.hourlyEpoch.Store(l.hourlyEpoch.Load() - 1)

	stats := l.Stats()
	if stats.HourlyCostUSD != 0 {
		t.Errorf("hourly cost after epoch roll = %f, want 0", stats.HourlyCostUSD)
	}
	// Total cost should persist across epoch boundaries.
	if math.Abs(stats.TotalCostUSD-0.06) > 0.001 {
		t.Errorf("total cost after epoch roll = %f, want ~0.06", stats.TotalCostUSD)
	}
}

func TestStats_IncludesCostFields(t *testing.T) {
	l := NewTokenLedger(nil, testLogger())
	stats := l.Stats()

	// With no pricing configured, cost should be zero.
	if stats.HourlyCostUSD != 0 {
		t.Errorf("hourly cost = %f, want 0", stats.HourlyCostUSD)
	}
	if stats.TotalCostUSD != 0 {
		t.Errorf("total cost = %f, want 0", stats.TotalCostUSD)
	}
	if stats.HourlyUsage.EstimatedCostUSD != 0 {
		t.Errorf("hourly usage estimated cost = %f, want 0", stats.HourlyUsage.EstimatedCostUSD)
	}
	if stats.TotalUsage.EstimatedCostUSD != 0 {
		t.Errorf("total usage estimated cost = %f, want 0", stats.TotalUsage.EstimatedCostUSD)
	}
}
