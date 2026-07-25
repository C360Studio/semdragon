package executor

import (
	"reflect"
	"testing"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/internal/semcompat"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/types"
)

func TestExecutionLifecyclePayloadGraphContract(t *testing.T) {
	timestamp := time.Unix(1_700_000_000, 0).UTC()
	questID := domain.QuestID("c360.local.game.board.quest.quest-1")
	agentID := domain.AgentID("c360.local.game.board.agent.agent-1")

	tests := []struct {
		name       string
		entityID   string
		triples    []message.Triple
		wantValues map[string]any
	}{
		{
			name:     "started",
			entityID: string(questID),
			triples: (&ExecutionStartedPayload{
				QuestID: questID, AgentID: agentID, LoopID: "loop-1",
				MaxTurns: 8, MaxTokens: 4096, ToolCount: 3, Timestamp: timestamp,
			}).Triples(),
			wantValues: map[string]any{
				"execution.lifecycle.started": true,
				"execution.agent.id":          string(agentID),
				"execution.loop.id":           "loop-1",
				"execution.config.max-turns":  8,
				"execution.config.max-tokens": 4096,
				"execution.tools.available":   3,
			},
		},
		{
			name:     "completed",
			entityID: string(questID),
			triples: (&ExecutionCompletedPayload{
				QuestID: questID, AgentID: agentID, LoopID: "loop-1", Status: StatusComplete,
				TotalTurns: 4, TotalToolCalls: 2, PromptTokens: 120,
				CompletionTokens: 45, Duration: 1500 * time.Millisecond, Timestamp: timestamp,
			}).Triples(),
			wantValues: map[string]any{
				"execution.lifecycle.completed": true,
				"execution.status.final":        string(StatusComplete),
				"execution.metrics.turns":       4,
				"execution.metrics.tool-calls":  2,
				"execution.tokens.prompt":       120,
				"execution.tokens.completion":   45,
				"execution.duration.ms":         int64(1500),
			},
		},
		{
			name:     "failed",
			entityID: string(questID),
			triples: (&ExecutionFailedPayload{
				QuestID: questID, AgentID: agentID, LoopID: "loop-1", Status: StatusFailed,
				Error: "model unavailable", TurnsCompleted: 2, ToolCallsMade: 1,
				Duration: 750 * time.Millisecond, Timestamp: timestamp,
			}).Triples(),
			wantValues: map[string]any{
				"execution.lifecycle.failed":   true,
				"execution.status.final":       string(StatusFailed),
				"execution.failure.error":      "model unavailable",
				"execution.metrics.turns":      2,
				"execution.metrics.tool-calls": 1,
				"execution.duration.ms":        int64(750),
			},
		},
	}

	allPredicates := make(map[string]struct{})
	referenceCount := 0
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.triples) != len(tt.wantValues) {
				t.Fatalf("triple count = %d, want %d", len(tt.triples), len(tt.wantValues))
			}
			seen := make(map[string]int, len(tt.wantValues))
			for _, triple := range tt.triples {
				allPredicates[triple.Predicate] = struct{}{}
				seen[triple.Predicate]++
				if triple.Subject != tt.entityID {
					t.Errorf("%s subject = %q, want %q", triple.Predicate, triple.Subject, tt.entityID)
				}
				if !reflect.DeepEqual(triple.Object, tt.wantValues[triple.Predicate]) {
					t.Errorf("%s object = %#v, want %#v", triple.Predicate, triple.Object, tt.wantValues[triple.Predicate])
				}
				if triple.Source != "executor" || !triple.Timestamp.Equal(timestamp) || triple.Confidence != 1 {
					t.Errorf("%s provenance = source %q timestamp %v confidence %v", triple.Predicate, triple.Source, triple.Timestamp, triple.Confidence)
				}
				if triple.Datatype == semcompat.EntityReferenceDatatype {
					referenceCount++
					if triple.Predicate != "execution.agent.id" {
						t.Errorf("%s unexpectedly carries @id", triple.Predicate)
					}
				} else if triple.Datatype != "" {
					t.Errorf("%s datatype = %q, want scalar", triple.Predicate, triple.Datatype)
				}
			}
			for predicate := range tt.wantValues {
				if seen[predicate] != 1 {
					t.Errorf("%s cardinality = %d, want 1", predicate, seen[predicate])
				}
			}
		})
	}
	if len(allPredicates) != 15 {
		t.Errorf("unique lifecycle predicates = %d, want 15: %v", len(allPredicates), allPredicates)
	}
	if referenceCount != 1 {
		t.Errorf("lifecycle @id relationships = %d, want 1", referenceCount)
	}
}

func TestExecutionLifecyclePayloadSchemasAndValidationRemainStable(t *testing.T) {
	timestamp := time.Unix(1_700_000_000, 0).UTC()
	questID := domain.QuestID("c360.local.game.board.quest.quest-1")
	agentID := domain.AgentID("c360.local.game.board.agent.agent-1")

	tests := []struct {
		name    string
		payload interface {
			Schema() types.Type
			Validate() error
		}
		wantSchema types.Type
	}{
		{"started", &ExecutionStartedPayload{QuestID: questID, AgentID: agentID, Timestamp: timestamp}, types.Type{Domain: "semdragons", Category: "execution.started", Version: "v1"}},
		{"completed", &ExecutionCompletedPayload{QuestID: questID, AgentID: agentID, Timestamp: timestamp}, types.Type{Domain: "semdragons", Category: "execution.completed", Version: "v1"}},
		{"failed", &ExecutionFailedPayload{QuestID: questID, AgentID: agentID, Timestamp: timestamp}, types.Type{Domain: "semdragons", Category: "execution.failed", Version: "v1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.payload.Schema(); !got.Equal(tt.wantSchema) {
				t.Errorf("schema = %s, want %s", got, tt.wantSchema)
			}
			if err := tt.payload.Validate(); err != nil {
				t.Errorf("valid payload rejected: %v", err)
			}
		})
	}

	subjects := map[string]string{
		"started":   SubjectExecutionStarted.Pattern,
		"completed": SubjectExecutionCompleted.Pattern,
		"failed":    SubjectExecutionFailed.Pattern,
	}
	wantSubjects := map[string]string{
		"started":   domain.PredicateExecutionStarted,
		"completed": domain.PredicateExecutionCompleted,
		"failed":    domain.PredicateExecutionFailed,
	}
	if !reflect.DeepEqual(subjects, wantSubjects) {
		t.Errorf("lifecycle subjects = %#v, want %#v", subjects, wantSubjects)
	}
}
