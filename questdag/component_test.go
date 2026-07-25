package questdag

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
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
