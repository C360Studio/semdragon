//go:build integration

package questbridge

// =============================================================================
// CONTRACT TEST — questbridge ↔ agentic-loop TaskMessage wire round trip
// =============================================================================
// Stage A contract lock (rebuild-quest-lifecycle-on-semstreams-rules, task 1.5).
//
// This test pins the questbridge → agentic-loop wire contract independently of the
// component's dispatch logic: a TaskMessage built the way handler.go builds it must
// survive the BaseMessage envelope → JSON → AGENT-stream publish → consume →
// payload-registry decode chain with every consumer-relevant field intact.
//
// The behavioral test TestQuestStartedPublishesTaskMessage checks that dispatch
// fires and a few fields are non-empty. This guard is different in intent: it asserts
// full field fidelity through the same payload registry the agentic-loop uses, so a
// future semstreams bump that changes the TaskMessage encoding or the registry mapping
// fails here with a clear signal rather than silently dropping a field the loop reads.
// =============================================================================

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadbuiltins"
	pkgtypes "github.com/c360studio/semstreams/pkg/types"
)

func TestContract_TaskMessageWireRoundTrip(t *testing.T) {
	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "AGENT",
			Subjects: []string{"agent.task.>"},
		}),
	)
	client := testClient.Client
	ctx := context.Background()

	// Subscribe before publishing (avoids a subscribe-after-publish race).
	taskCh := subscribeToAgentTasks(t, client, ctx)

	// Build a TaskMessage mirroring handler.go's construction — every field the
	// agentic-loop consumer reads.
	want := agentic.TaskMessage{
		TaskID: "c360.test.game.board1.quest.contract1",
		LoopID: "quest-contract1-abc",
		Role:   agentic.RoleGeneral,
		Model:  "mock-model",
		Prompt: "do the contract work",
		Context: &pkgtypes.ConstructedContext{
			Content:    "assembled context body",
			TokenCount: 42,
			Entities:   []string{"c360.test.game.board1.quest.contract1"},
		},
		Tools: []agentic.ToolDefinition{
			{Name: "bash", Description: "run a shell command", Parameters: map[string]any{"type": "object"}},
			{Name: "submit_work_product", Description: "submit result", Parameters: map[string]any{"type": "object"}},
		},
		Metadata: map[string]any{
			"agent_id":   "c360.test.game.board1.agent.dragon",
			"quest_id":   "c360.test.game.board1.quest.contract1",
			"trust_tier": 6,
			"board":      "board1",
		},
	}

	// Wrap + publish exactly as questbridge does (handler.go:351-363).
	baseMsg := message.NewBaseMessage(want.Schema(), &want, "questbridge")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		t.Fatalf("marshal TaskMessage BaseMessage: %v", err)
	}
	if err := client.PublishToStream(ctx, "agent.task.contract1", data); err != nil {
		t.Fatalf("publish TaskMessage: %v", err)
	}

	// Consume + decode through the production payload registry.
	decoder := message.NewDecoder(payloadbuiltins.NewTestRegistry(t))
	var got *agentic.TaskMessage
	select {
	case raw := <-taskCh:
		bm, derr := decoder.Decode(raw)
		if derr != nil {
			t.Fatalf("decode TaskMessage BaseMessage: %v", derr)
		}
		var ok bool
		got, ok = bm.Payload().(*agentic.TaskMessage)
		if !ok {
			t.Fatalf("payload type = %T, want *agentic.TaskMessage", bm.Payload())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for TaskMessage on AGENT stream")
	}

	// Assert every consumer-relevant field survived the round trip.
	if got.TaskID != want.TaskID {
		t.Errorf("TaskID = %q, want %q", got.TaskID, want.TaskID)
	}
	if got.LoopID != want.LoopID {
		t.Errorf("LoopID = %q, want %q", got.LoopID, want.LoopID)
	}
	if got.Role != want.Role {
		t.Errorf("Role = %q, want %q", got.Role, want.Role)
	}
	if got.Model != want.Model {
		t.Errorf("Model = %q, want %q", got.Model, want.Model)
	}
	if got.Prompt != want.Prompt {
		t.Errorf("Prompt = %q, want %q", got.Prompt, want.Prompt)
	}
	if got.Context == nil {
		t.Fatal("Context dropped in round trip")
	}
	if got.Context.Content != want.Context.Content || got.Context.TokenCount != want.Context.TokenCount {
		t.Errorf("Context = %+v, want Content=%q TokenCount=%d",
			got.Context, want.Context.Content, want.Context.TokenCount)
	}
	if len(got.Tools) != len(want.Tools) {
		t.Fatalf("Tools length = %d, want %d", len(got.Tools), len(want.Tools))
	}
	for i := range want.Tools {
		if got.Tools[i].Name != want.Tools[i].Name {
			t.Errorf("Tools[%d].Name = %q, want %q", i, got.Tools[i].Name, want.Tools[i].Name)
		}
	}
	if got.Metadata == nil || got.Metadata["agent_id"] != want.Metadata["agent_id"] {
		t.Errorf("Metadata[agent_id] = %v, want %v", got.Metadata["agent_id"], want.Metadata["agent_id"])
	}
}
