package questdag

import (
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	gateddagexec "github.com/c360studio/semstreams/processor/gated-dag"

	"github.com/c360studio/semdragons/domain"
)

type stopFailingExecutor struct {
	component.LifecycleComponent
	err error
}

func (e *stopFailingExecutor) Stop(time.Duration) error { return e.err }

func TestStopReturnsExecutorFailure(t *testing.T) {
	stopErr := errors.New("executor stop failed")
	c := &Component{
		logger:   slog.Default(),
		executor: &stopFailingExecutor{err: stopErr},
	}
	c.running.Store(true)

	if err := c.Stop(time.Second); !errors.Is(err, stopErr) {
		t.Fatalf("Stop() error = %v, want wrapped executor error", err)
	}
	if c.running.Load() {
		t.Fatal("component remains running after executor stop failure")
	}
}

func TestGatedDAGConfigReliesOnFrameworkMarkerDefaults(t *testing.T) {
	cfg := DefaultConfig()
	component := &Component{
		config:      &cfg,
		boardConfig: &domain.BoardConfig{Org: cfg.Org, Platform: cfg.Platform, Board: cfg.Board},
	}

	data, err := json.Marshal(component.gatedDAGConfig())
	if err != nil {
		t.Fatalf("marshal executor config: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("decode executor config: %v", err)
	}
	for _, field := range []string{
		"completed_predicate",
		"failed_predicate",
		"dirtied_predicate",
		"depends_on_predicate",
		"claim_predicate",
	} {
		if _, exists := wire[field]; exists {
			t.Errorf("executor config overrides framework field %q", field)
		}
	}

	defaults := gateddagexec.DefaultConfig()
	if frameworkGatedDAGDefaults() != defaults {
		t.Fatalf("questdag marker defaults drifted from SemStreams: got %#v want %#v",
			frameworkGatedDAGDefaults(), defaults)
	}
}
