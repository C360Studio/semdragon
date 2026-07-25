//go:build integration

package main

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

func TestSeedOneAgentWritesAuthoritativeStateWithoutLegacyEvents(t *testing.T) {
	testClient := natsclient.NewTestClient(
		t,
		natsclient.WithKV(),
		natsclient.WithFileStorage(),
		natsclient.WithKVBuckets(graph.BucketEntityStates),
	)
	client := testClient.Client
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	const barrierSubject = "seeding.__barrier"
	legacyEvents := make(chan string, 16)
	barrier := make(chan struct{}, 1)
	subscription, err := client.Subscribe(ctx, "seeding.>", func(_ context.Context, msg *nats.Msg) {
		if msg.Subject == barrierSubject {
			barrier <- struct{}{}
			return
		}
		legacyEvents <- msg.Subject
	})
	if err != nil {
		t.Fatalf("subscribe to legacy seeding wildcard: %v", err)
	}
	t.Cleanup(func() {
		if err := subscription.Unsubscribe(); err != nil {
			t.Errorf("unsubscribe from legacy seeding wildcard: %v", err)
		}
	})
	if _, err := client.RTT(); err != nil {
		t.Fatalf("flush legacy seeding subscription with RTT: %v", err)
	}

	boardConfig := &domain.BoardConfig{Org: "test", Platform: "integration", Board: "seed-one-agent"}
	graphClient := semdragons.NewGraphClient(client, boardConfig)
	if err := graphClient.EnsureBucket(ctx); err != nil {
		t.Fatalf("ensure entity-state bucket: %v", err)
	}
	watcher, err := graphClient.WatchEntityType(ctx, domain.EntityTypeAgent)
	if err != nil {
		t.Fatalf("watch authoritative Agent states: %v", err)
	}
	t.Cleanup(func() {
		if err := watcher.Stop(); err != nil {
			t.Errorf("stop Agent state watcher: %v", err)
		}
	})

	spec := agentSpec{
		namePattern: "contract-agent",
		level:       8,
		skills:      []domain.SkillTag{domain.SkillCodeGen, domain.SkillAnalysis},
		archetype:   domain.ArchetypeEngineer,
		count:       1,
	}
	if err := seedOneAgent(ctx, graphClient, boardConfig, "contract-agent", spec, 0); err != nil {
		t.Fatalf("seed one authoritative Agent: %v", err)
	}
	if err := client.Publish(ctx, barrierSubject, nil); err != nil {
		t.Fatalf("publish legacy wildcard callback barrier: %v", err)
	}
	select {
	case <-barrier:
	case <-ctx.Done():
		t.Fatalf("wait for legacy wildcard callback barrier: %v", ctx.Err())
	}
	select {
	case subject := <-legacyEvents:
		t.Fatalf("startup Agent birth emitted forbidden legacy event on %q", subject)
	default:
	}

	var state *graph.EntityState
	for state == nil {
		select {
		case entry := <-watcher.Updates():
			if entry == nil {
				continue
			}
			state, err = semdragons.DecodeEntityState(entry)
			if err != nil {
				t.Fatalf("decode authoritative Agent EntityState: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("wait for authoritative Agent KV birth: %v", ctx.Err())
		}
	}

	if state.MessageType.Category != "agent.seeded" {
		t.Errorf("Agent birth message category = %q, want agent.seeded", state.MessageType.Category)
	}
	agent := agentprogression.AgentFromEntityState(state)
	if agent == nil {
		t.Fatal("AgentFromEntityState returned nil")
	}
	if agent.EntityID() != state.ID {
		t.Errorf("decoded Agent EntityID = %q, want state ID %q", agent.EntityID(), state.ID)
	}
	if agent.Name != "contract-agent" || agent.Level != spec.level ||
		agent.Archetype != spec.archetype || agent.Status != domain.AgentIdle {
		t.Errorf("decoded authoritative Agent = %#v, want startup seed spec %#v", agent, spec)
	}

	gotFacts := comparableAgentFacts(t, state.Triples)
	wantFacts := comparableAgentFacts(t, agent.Triples())
	if !reflect.DeepEqual(gotFacts, wantFacts) {
		t.Errorf("persisted Agent facts = %#v, want authoritative Graphable facts %#v", gotFacts, wantFacts)
	}
}

type comparableAgentFact struct {
	Subject    string
	Object     any
	Source     string
	Confidence float64
	Datatype   string
}

func comparableAgentFacts(t *testing.T, triples []message.Triple) map[string]comparableAgentFact {
	t.Helper()
	facts := make(map[string]comparableAgentFact, len(triples))
	for _, triple := range triples {
		data, err := json.Marshal(triple.Object)
		if err != nil {
			t.Fatalf("marshal Agent fact %s: %v", triple.Predicate, err)
		}
		var object any
		if err := json.Unmarshal(data, &object); err != nil {
			t.Fatalf("normalize Agent fact %s: %v", triple.Predicate, err)
		}
		facts[triple.Predicate] = comparableAgentFact{
			Subject:    triple.Subject,
			Object:     object,
			Source:     triple.Source,
			Confidence: triple.Confidence,
			Datatype:   triple.Datatype,
		}
	}
	return facts
}
