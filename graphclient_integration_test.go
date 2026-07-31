//go:build integration

package semdragons_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/graph/readiness"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
)

func TestGraphClientReadyWaitsForGraphIngestCatalogOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)
	client := semdragons.NewGraphClient(tc.Client, nil)

	err := client.Ready(ctx)
	if err == nil {
		t.Fatal("GraphClient.Ready before graph-ingest = nil, want classified not-ready")
	}
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) {
		t.Fatalf("GraphClient.Ready error type = %T, want *errs.ClassifiedError: %v", err, err)
	}
	if classified.Class != errs.ErrorTransient {
		t.Errorf("GraphClient.Ready error class = %v, want transient", classified.Class)
	}
	if classified.Code != graph.ErrorCodeIndexNotReady {
		t.Errorf("GraphClient.Ready error code = %q, want %q",
			classified.Code, graph.ErrorCodeIndexNotReady)
	}
	if !strings.Contains(err.Error(), "graph-ingest") {
		t.Errorf("GraphClient.Ready error does not name graph-ingest owner: %v", err)
	}
	if _, bucketErr := tc.Client.GetKeyValueBucket(ctx, graph.BucketEntityStates); !errors.Is(bucketErr, jetstream.ErrBucketNotFound) {
		t.Fatalf("ENTITY_STATES after reader readiness probe: err=%v, want bucket absent", bucketErr)
	}

	ingestConfig := graphingest.DefaultConfig()
	ingestConfig.EnforceOwnerLease = true
	startGraphClientComponent(t, ctx, mustGraphClientJSON(t, ingestConfig), func(raw json.RawMessage) (component.Discoverable, error) {
		return graphingest.CreateGraphIngest(raw, component.Dependencies{NATSClient: tc.Client})
	})
	waitForGraphClientProducer(t, ctx, tc.Client, readiness.KeyGraphIngest)

	if err := client.Ready(ctx); err != nil {
		t.Fatalf("same GraphClient did not become ready after graph-ingest ownership startup: %v", err)
	}
	if _, err := tc.Client.GetKeyValueBucket(ctx, graph.BucketEntityStates); err != nil {
		t.Fatalf("graph-ingest did not provision catalog ENTITY_STATES: %v", err)
	}
}

func startGraphClientComponent(
	t *testing.T,
	ctx context.Context,
	raw json.RawMessage,
	create func(json.RawMessage) (component.Discoverable, error),
) {
	t.Helper()
	discoverable, err := create(raw)
	if err != nil {
		t.Fatalf("create graph-ingest: %v", err)
	}
	runtimeComponent, ok := discoverable.(component.LifecycleComponent)
	if !ok {
		t.Fatalf("graph-ingest %T is not a LifecycleComponent", discoverable)
	}
	if err := runtimeComponent.Initialize(); err != nil {
		t.Fatalf("initialize graph-ingest: %v", err)
	}
	if err := runtimeComponent.Start(ctx); err != nil {
		t.Fatalf("start graph-ingest: %v", err)
	}
	t.Cleanup(func() {
		if err := runtimeComponent.Stop(5 * time.Second); err != nil {
			t.Errorf("stop graph-ingest: %v", err)
		}
	})
}

func waitForGraphClientProducer(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	key string,
) {
	t.Helper()
	watcher := readiness.NewWatcher(client, key)
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("start readiness watcher for %s: %v", key, err)
	}
	t.Cleanup(watcher.Stop)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		reading := watcher.Read()
		if reading.Known && reading.Fresh && reading.Status.Ready &&
			reading.Status.BootstrapComplete && reading.Status.State == graph.IndexStateReady {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("producer %s did not become known/fresh/ready: reading=%+v: %v",
				key, reading, ctx.Err())
		case <-ticker.C:
		}
	}
}

func mustGraphClientJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal graph-ingest config: %v", err)
	}
	return raw
}
