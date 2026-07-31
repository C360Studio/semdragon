package api

// =============================================================================
// UNIT TESTS — handleCancelQuest
// =============================================================================
// These tests cover the HTTP handler that cancels an in-progress quest.
// cancelActiveLoop and cancelDAGSubQuests are both guarded by nil componentDeps
// checks, so they are no-ops in the unit test environment (no live NATS needed).
//
// Run with: go test ./service/api/ -run TestCancel -v
// =============================================================================

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
)

// =============================================================================
// handleCancelQuest
// =============================================================================

func TestHandleCancelQuest_NotFound(t *testing.T) {
	g := &mockGraph{
		getQuestFn: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
			return nil, errors.New("key not found")
		},
	}
	svc := newTestService(g, &mockWorld{})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /quests/{id}/cancel", svc.handleCancelQuest)

	req := httptest.NewRequest(http.MethodPost, "/quests/missing/cancel",
		bytes.NewReader([]byte(`{}`)))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
}

func TestHandleCancelQuest_InvalidID(t *testing.T) {
	g := &mockGraph{}
	svc := newTestService(g, &mockWorld{})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /quests/{id}/cancel", svc.handleCancelQuest)

	// Dots in the path ID are rejected by isValidPathID.
	req := httptest.NewRequest(http.MethodPost, "/quests/test.dev.game.board1.quest.abc/cancel",
		bytes.NewReader([]byte(`{}`)))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

func TestHandleCancelQuest_NotInProgress(t *testing.T) {
	tests := []struct {
		name   string
		status domain.QuestStatus
	}{
		{name: "posted quest", status: domain.QuestPosted},
		{name: "claimed quest", status: domain.QuestClaimed},
		{name: "in_review quest", status: domain.QuestInReview},
		{name: "completed quest", status: domain.QuestCompleted},
		{name: "failed quest", status: domain.QuestFailed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := sampleQuest()
			q.Status = tc.status

			g := &mockGraph{
				getQuestFn: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
					es := makeQuestEntityState(q)
					return &es, nil
				},
			}
			svc := newTestService(g, &mockWorld{})

			mux := http.NewServeMux()
			mux.HandleFunc("POST /quests/{id}/cancel", svc.handleCancelQuest)

			req := httptest.NewRequest(http.MethodPost, "/quests/q1/cancel",
				bytes.NewReader([]byte(`{}`)))
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusConflict {
				t.Errorf("%s: status = %d, want 409 Conflict\nbody: %s",
					tc.name, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandleCancelQuest_DisabledMutationSurface(t *testing.T) {
	q := sampleQuest()
	q.Status = domain.QuestInProgress

	g := &mockGraph{
		getQuestFn: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
			es := makeQuestEntityState(q)
			return &es, nil
		},
	}
	svc := newTestService(g, &mockWorld{})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /quests/{id}/cancel", svc.handleCancelQuest)

	req := httptest.NewRequest(http.MethodPost, "/quests/q1/cancel",
		bytes.NewReader([]byte(`{}`)))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500\nbody: %s", rr.Code, rr.Body.String())
	}
}
