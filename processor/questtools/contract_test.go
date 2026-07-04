//go:build integration

package questtools

// =============================================================================
// CONTRACT TEST — questtools ↔ tool.execute / tool.result wire round trip
// =============================================================================
// Stage A contract lock (rebuild-quest-lifecycle-on-semstreams-rules, task 1.5).
//
// The questtools handler (handler.go:52) decodes each tool.execute.* message as a
// BaseMessage whose payload MUST be *agentic.ToolCall, then publishes a
// BaseMessage-wrapped *agentic.ToolResult to tool.result.*. This test pins the
// INPUT side of that contract: a ToolCall published the way the agentic-loop
// publishes it must decode back to *agentic.ToolCall through the production payload
// registry with every field the handler reads (Name, Arguments, LoopID, TraceID,
// Metadata) intact.
//
// The OUTPUT side (ToolResult round trip, including CallID/LoopID/TraceID fidelity)
// is already covered end-to-end by pollForToolResult in the behavioral tests
// (TestToolExecutionSuccess, TestCorrelationIDPropagation). This guard fails loudly
// if a future semstreams bump changes the ToolCall encoding or registry mapping.
// =============================================================================

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadbuiltins"
	"github.com/nats-io/nats.go/jetstream"
)

func TestContract_ToolCallWireFidelity(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	want := agentic.ToolCall{
		ID:      "call-contract-001",
		Name:    "bash",
		LoopID:  "loop-contract-xyz",
		TraceID: "trace-contract-abc",
		Arguments: map[string]any{
			"command":      "echo contract",
			"_sandbox_dir": "/workspace/contract",
		},
		Metadata: map[string]any{
			"agent_id":   "contract-agent",
			"trust_tier": journeymanTier, // JSON number → float64 after decode
			"quest_id":   "quest-contract-001",
		},
	}

	// Publish the BaseMessage-wrapped ToolCall exactly as the agentic-loop does.
	publishToolCall(t, tc.Client, ctx, "tool.execute.bash", want)

	// Consume directly (not via the component) and decode through the payload registry.
	got := consumeToolCall(t, tc.Client, ctx, "tool.execute.bash", 10*time.Second)

	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Name != want.Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Name)
	}
	if got.LoopID != want.LoopID {
		t.Errorf("LoopID = %q, want %q", got.LoopID, want.LoopID)
	}
	if got.TraceID != want.TraceID {
		t.Errorf("TraceID = %q, want %q", got.TraceID, want.TraceID)
	}
	if got.Arguments["command"] != want.Arguments["command"] {
		t.Errorf("Arguments[command] = %v, want %v", got.Arguments["command"], want.Arguments["command"])
	}
	if got.Metadata["agent_id"] != want.Metadata["agent_id"] {
		t.Errorf("Metadata[agent_id] = %v, want %v", got.Metadata["agent_id"], want.Metadata["agent_id"])
	}
	// trust_tier round-trips as the same JSON number value (float64); the handler's
	// buildContextFromMetadata depends on this to reconstruct the trust tier.
	if got.Metadata["trust_tier"] != want.Metadata["trust_tier"] {
		t.Errorf("Metadata[trust_tier] = %v (%T), want %v (%T)",
			got.Metadata["trust_tier"], got.Metadata["trust_tier"],
			want.Metadata["trust_tier"], want.Metadata["trust_tier"])
	}
}

// consumeToolCall reads one BaseMessage-wrapped ToolCall from subject and decodes it
// via the production payload registry. Mirror of pollForToolResult for the input side.
func consumeToolCall(t *testing.T, client *natsclient.Client, ctx context.Context, subject string, timeout time.Duration) agentic.ToolCall {
	t.Helper()

	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream(): %v", err)
	}

	decoder := payloadbuiltins.NewTestDecoder(t)
	consumer, err := js.CreateOrUpdateConsumer(ctx, agentStreamName, jetstream.ConsumerConfig{
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverAllPolicy,
	})
	if err != nil {
		t.Fatalf("create consumer for %q: %v", subject, err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msgs, ferr := consumer.Fetch(1, jetstream.FetchMaxWait(200*time.Millisecond))
		if ferr != nil {
			continue
		}
		for msg := range msgs.Messages() {
			_ = msg.Ack()
			bm, derr := decoder.Decode(msg.Data())
			if derr != nil {
				t.Fatalf("decode ToolCall BaseMessage: %v", derr)
			}
			ptr, ok := bm.Payload().(*agentic.ToolCall)
			if !ok {
				t.Fatalf("payload type = %T, want *agentic.ToolCall", bm.Payload())
			}
			return *ptr
		}
	}

	t.Fatalf("timed out waiting for ToolCall on %q after %v", subject, timeout)
	return agentic.ToolCall{} // unreachable; satisfies compiler
}
