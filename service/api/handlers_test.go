package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/nats-io/nats.go/jetstream"
)

// =============================================================================
// MOCK IMPLEMENTATIONS
// =============================================================================

// mockGraph implements GraphQuerier with function fields so each test case can
// supply exactly the behavior it needs without coupling tests together.
type mockGraph struct {
	configFn             func() *semdragons.BoardConfig
	getQuestFn           func(ctx context.Context, id semdragons.QuestID) (*graph.EntityState, error)
	getAgentFn           func(ctx context.Context, id semdragons.AgentID) (*graph.EntityState, error)
	getBattleFn          func(ctx context.Context, id semdragons.BattleID) (*graph.EntityState, error)
	listQuestsFn         func(ctx context.Context, limit int) ([]graph.EntityState, error)
	listAgentsFn         func(ctx context.Context, limit int) ([]graph.EntityState, error)
	listEntitiesByTypeFn func(ctx context.Context, entityType string, limit int) ([]graph.EntityState, error)
	emitEntityFn         func(ctx context.Context, entity graph.Graphable, eventType string) error
	emitEntityUpdateFn   func(ctx context.Context, entity graph.Graphable, eventType string) error
}

func (m *mockGraph) Config() *semdragons.BoardConfig {
	if m.configFn != nil {
		return m.configFn()
	}
	return &semdragons.BoardConfig{Org: "test", Platform: "dev", Board: "board1"}
}

func (m *mockGraph) GetQuest(ctx context.Context, id semdragons.QuestID) (*graph.EntityState, error) {
	if m.getQuestFn != nil {
		return m.getQuestFn(ctx, id)
	}
	return nil, jetstream.ErrKeyNotFound
}

func (m *mockGraph) GetAgent(ctx context.Context, id semdragons.AgentID) (*graph.EntityState, error) {
	if m.getAgentFn != nil {
		return m.getAgentFn(ctx, id)
	}
	return nil, jetstream.ErrKeyNotFound
}

func (m *mockGraph) GetBattle(ctx context.Context, id semdragons.BattleID) (*graph.EntityState, error) {
	if m.getBattleFn != nil {
		return m.getBattleFn(ctx, id)
	}
	return nil, jetstream.ErrKeyNotFound
}

func (m *mockGraph) ListQuestsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	if m.listQuestsFn != nil {
		return m.listQuestsFn(ctx, limit)
	}
	return nil, nil
}

func (m *mockGraph) ListAgentsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	if m.listAgentsFn != nil {
		return m.listAgentsFn(ctx, limit)
	}
	return nil, nil
}

func (m *mockGraph) ListEntitiesByType(ctx context.Context, entityType string, limit int) ([]graph.EntityState, error) {
	if m.listEntitiesByTypeFn != nil {
		return m.listEntitiesByTypeFn(ctx, entityType, limit)
	}
	return nil, nil
}

func (m *mockGraph) EmitEntity(ctx context.Context, entity graph.Graphable, eventType string) error {
	if m.emitEntityFn != nil {
		return m.emitEntityFn(ctx, entity, eventType)
	}
	return nil
}

func (m *mockGraph) EmitEntityUpdate(ctx context.Context, entity graph.Graphable, eventType string) error {
	if m.emitEntityUpdateFn != nil {
		return m.emitEntityUpdateFn(ctx, entity, eventType)
	}
	return nil
}

// mockWorld implements WorldStateProvider.
type mockWorld struct {
	worldStateFn func(ctx context.Context) (*domain.WorldState, error)
}

func (m *mockWorld) WorldState(ctx context.Context) (*domain.WorldState, error) {
	if m.worldStateFn != nil {
		return m.worldStateFn(ctx)
	}
	return &domain.WorldState{
		Agents:  []any{},
		Quests:  []any{},
		Parties: []any{},
		Guilds:  []any{},
		Battles: []any{},
	}, nil
}

// =============================================================================
// TEST HELPER
// =============================================================================

// newTestService creates a Service wired with test doubles.
// The embedded *service.BaseService is left nil: none of the handlers call
// BaseService methods, so the nil embed is safe for unit tests.
func newTestService(g GraphQuerier, w WorldStateProvider) *Service {
	return &Service{
		graph:  g,
		world:  w,
		config: Config{Board: "board1", MaxEntities: 100},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// makeQuestEntityState builds an EntityState whose Triples will reconstruct
// to the supplied Quest via QuestFromEntityState.
func makeQuestEntityState(q *semdragons.Quest) graph.EntityState {
	return graph.EntityState{
		ID:      string(q.ID),
		Triples: q.Triples(),
	}
}

// makeAgentEntityState builds an EntityState whose Triples will reconstruct
// to the supplied Agent via AgentFromEntityState.
func makeAgentEntityState(a *semdragons.Agent) graph.EntityState {
	return graph.EntityState{
		ID:      string(a.ID),
		Triples: a.Triples(),
	}
}

// makeBattleEntityState builds an EntityState whose Triples will reconstruct
// to the supplied BossBattle via BattleFromEntityState.
func makeBattleEntityState(b *semdragons.BossBattle) graph.EntityState {
	return graph.EntityState{
		ID:      string(b.ID),
		Triples: b.Triples(),
	}
}

// sampleQuest returns a minimal Quest suitable for use in tests.
func sampleQuest() *semdragons.Quest {
	return &semdragons.Quest{
		ID:          semdragons.QuestID("test.dev.game.board1.quest.q1"),
		Title:       "Slay the Dragon",
		Description: "A very dangerous quest",
		Status:      semdragons.QuestPosted,
		Difficulty:  semdragons.DifficultyModerate,
		BaseXP:      100,
		MaxAttempts: 3,
	}
}

// sampleAgent returns a minimal Agent suitable for use in tests.
func sampleAgent() *semdragons.Agent {
	return &semdragons.Agent{
		ID:                 semdragons.AgentID("test.dev.game.board1.agent.a1"),
		Name:               "TestAgent",
		Status:             semdragons.AgentIdle,
		Level:              1,
		XP:                 0,
		XPToLevel:          100,
		Tier:               semdragons.TierApprentice,
		SkillProficiencies: make(map[semdragons.SkillTag]semdragons.SkillProficiency),
	}
}

// sampleBattle returns a minimal BossBattle suitable for use in tests.
func sampleBattle() *semdragons.BossBattle {
	return &semdragons.BossBattle{
		ID:      semdragons.BattleID("test.dev.game.board1.battle.b1"),
		QuestID: semdragons.QuestID("test.dev.game.board1.quest.q1"),
		AgentID: semdragons.AgentID("test.dev.game.board1.agent.a1"),
		Status:  semdragons.BattleActive,
		Level:   semdragons.ReviewStandard,
	}
}

// decodeJSON is a test helper that decodes the response body into dst.
func decodeJSON(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("decode JSON response: %v\nbody: %s", err, body)
	}
}

// =============================================================================
// HELPER FUNCTION TESTS
// =============================================================================

func TestIsValidPathID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "empty string", input: "", want: false},
		{name: "contains dot", input: "a.b", want: false},
		{name: "contains slash", input: "a/b", want: false},
		{name: "full entity id with dots", input: "c360.prod.game.board1.quest.abc", want: false},
		{name: "simple alphanumeric", input: "abc123", want: true},
		{name: "hex instance", input: "f3a2b1c0", want: true},
		{name: "single char", input: "x", want: true},
		{name: "leading slash", input: "/foo", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidPathID(tc.input); got != tc.want {
				t.Errorf("isValidPathID(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsBucketNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "jetstream sentinel", err: jetstream.ErrBucketNotFound, want: true},
		{name: "wrapped sentinel", err: errors.New("get entity states bucket: bucket not found"), want: true},
		{name: "string match", err: errors.New("bucket not found"), want: true},
		{name: "unrelated error", err: errors.New("connection refused"), want: false},
		{name: "key not found (different error)", err: jetstream.ErrKeyNotFound, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBucketNotFound(tc.err); got != tc.want {
				t.Errorf("isBucketNotFound(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsKeyNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "jetstream sentinel", err: jetstream.ErrKeyNotFound, want: true},
		{name: "wrapped sentinel", err: errors.New("get entity: key not found"), want: true},
		{name: "string match", err: errors.New("key not found"), want: true},
		{name: "unrelated error", err: errors.New("timeout"), want: false},
		{name: "bucket not found (different error)", err: jetstream.ErrBucketNotFound, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isKeyNotFound(tc.err); got != tc.want {
				t.Errorf("isKeyNotFound(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// =============================================================================
// AUTH MIDDLEWARE TESTS
// =============================================================================

// okHandler is a simple handler that records whether it was called.
func okHandler(called *bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	}
}

func TestRequireAuth_DevMode(t *testing.T) {
	// Empty key → dev mode: handler is returned unchanged, all requests pass.
	var called bool
	handler := requireAuth("", okHandler(&called))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	// No auth header at all — should still pass in dev mode.
	rr := httptest.NewRecorder()
	handler(rr, req)

	if !called {
		t.Error("expected handler to be called in dev mode, but it was not")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequireAuth_ValidXAPIKey(t *testing.T) {
	const apiKey = "secret-token"
	var called bool
	handler := requireAuth(apiKey, okHandler(&called))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-API-Key", apiKey)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if !called {
		t.Error("expected handler to be called with valid X-API-Key")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequireAuth_ValidBearer(t *testing.T) {
	const apiKey = "bearer-token-xyz"
	var called bool
	handler := requireAuth(apiKey, okHandler(&called))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if !called {
		t.Error("expected handler to be called with valid Bearer token")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequireAuth_InvalidKey(t *testing.T) {
	const apiKey = "correct-key"
	var called bool
	handler := requireAuth(apiKey, okHandler(&called))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rr := httptest.NewRecorder()
	handler(rr, req)

	if called {
		t.Error("expected handler NOT to be called with wrong key")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}

	var resp map[string]string
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if resp["error"] != "unauthorized" {
		t.Errorf("expected error=unauthorized, got %q", resp["error"])
	}
}

func TestRequireAuth_MissingKey(t *testing.T) {
	const apiKey = "correct-key"
	var called bool
	handler := requireAuth(apiKey, okHandler(&called))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	// No auth header.
	rr := httptest.NewRecorder()
	handler(rr, req)

	if called {
		t.Error("expected handler NOT to be called with no key")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// =============================================================================
// WORLD STATE HANDLER TESTS
// =============================================================================

func TestHandleWorldState(t *testing.T) {
	tests := []struct {
		name       string
		worldFn    func(context.Context) (*domain.WorldState, error)
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name: "success",
			worldFn: func(_ context.Context) (*domain.WorldState, error) {
				return &domain.WorldState{
					Agents:  []any{},
					Quests:  []any{},
					Parties: []any{},
					Guilds:  []any{},
					Battles: []any{},
					Stats:   domain.WorldStats{ActiveAgents: 3},
				}, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var state domain.WorldState
				decodeJSON(t, body, &state)
				if state.Stats.ActiveAgents != 3 {
					t.Errorf("expected 3 active agents, got %d", state.Stats.ActiveAgents)
				}
			},
		},
		{
			name: "world state error returns 500",
			worldFn: func(_ context.Context) (*domain.WorldState, error) {
				return nil, errors.New("nats connection lost")
			},
			wantStatus: http.StatusInternalServerError,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]string
				decodeJSON(t, body, &resp)
				if resp["error"] == "" {
					t.Error("expected error field in response")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(&mockGraph{}, &mockWorld{worldStateFn: tc.worldFn})
			req := httptest.NewRequest(http.MethodGet, "/world", nil)
			rr := httptest.NewRecorder()
			svc.handleWorldState(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rr.Code, tc.wantStatus)
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

// =============================================================================
// QUEST HANDLER TESTS
// =============================================================================

func TestHandleListQuests(t *testing.T) {
	q := sampleQuest()
	es := makeQuestEntityState(q)

	tests := []struct {
		name       string
		listFn     func(context.Context, int) ([]graph.EntityState, error)
		query      string
		wantStatus int
		wantLen    int
	}{
		{
			name:       "success with one quest",
			listFn:     func(_ context.Context, _ int) ([]graph.EntityState, error) { return []graph.EntityState{es}, nil },
			wantStatus: http.StatusOK,
			wantLen:    1,
		},
		{
			name:       "bucket not found returns empty array not error",
			listFn:     func(_ context.Context, _ int) ([]graph.EntityState, error) { return nil, jetstream.ErrBucketNotFound },
			wantStatus: http.StatusOK,
			wantLen:    0,
		},
		{
			name:       "other error returns 500",
			listFn:     func(_ context.Context, _ int) ([]graph.EntityState, error) { return nil, errors.New("timeout") },
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:   "status filter excludes non-matching quests",
			listFn: func(_ context.Context, _ int) ([]graph.EntityState, error) { return []graph.EntityState{es}, nil },
			// sampleQuest() has Status=posted; filter for claimed → 0 results
			query:      "?status=claimed",
			wantStatus: http.StatusOK,
			wantLen:    0,
		},
		{
			name:       "status filter includes matching quests",
			listFn:     func(_ context.Context, _ int) ([]graph.EntityState, error) { return []graph.EntityState{es}, nil },
			query:      "?status=posted",
			wantStatus: http.StatusOK,
			wantLen:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(&mockGraph{listQuestsFn: tc.listFn}, &mockWorld{})
			req := httptest.NewRequest(http.MethodGet, "/quests"+tc.query, nil)
			rr := httptest.NewRecorder()
			svc.handleListQuests(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rr.Code, tc.wantStatus)
			}

			if tc.wantStatus == http.StatusOK {
				var quests []semdragons.Quest
				decodeJSON(t, rr.Body.Bytes(), &quests)
				if len(quests) != tc.wantLen {
					t.Errorf("quest count: got %d, want %d", len(quests), tc.wantLen)
				}
			}
		})
	}
}

func TestHandleGetQuest(t *testing.T) {
	q := sampleQuest()
	es := makeQuestEntityState(q)

	tests := []struct {
		name       string
		pathID     string
		getQuestFn func(context.Context, semdragons.QuestID) (*graph.EntityState, error)
		wantStatus int
	}{
		{
			name:       "success",
			pathID:     "q1",
			getQuestFn: func(_ context.Context, _ semdragons.QuestID) (*graph.EntityState, error) { return &es, nil },
			wantStatus: http.StatusOK,
		},
		{
			name:       "key not found returns 404",
			pathID:     "q1",
			getQuestFn: func(_ context.Context, _ semdragons.QuestID) (*graph.EntityState, error) { return nil, jetstream.ErrKeyNotFound },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "bucket not found returns 404",
			pathID:     "q1",
			getQuestFn: func(_ context.Context, _ semdragons.QuestID) (*graph.EntityState, error) { return nil, jetstream.ErrBucketNotFound },
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "invalid id (contains dot) returns 400",
			pathID: "c360.prod.game.board1.quest.abc",
			// getQuestFn will not be called
			getQuestFn: func(_ context.Context, _ semdragons.QuestID) (*graph.EntityState, error) {
				return nil, errors.New("should not be called")
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "server error returns 500",
			pathID:     "q1",
			getQuestFn: func(_ context.Context, _ semdragons.QuestID) (*graph.EntityState, error) { return nil, errors.New("nats error") },
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(&mockGraph{getQuestFn: tc.getQuestFn}, &mockWorld{})

			// Use a real mux to handle path values via Go 1.22 routing.
			mux := http.NewServeMux()
			mux.HandleFunc("GET /quests/{id}", svc.handleGetQuest)

			req := httptest.NewRequest(http.MethodGet, "/quests/"+tc.pathID, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rr.Code, tc.wantStatus)
			}

			if tc.wantStatus == http.StatusOK {
				var quest semdragons.Quest
				decodeJSON(t, rr.Body.Bytes(), &quest)
				if string(quest.ID) != string(q.ID) {
					t.Errorf("quest ID: got %q, want %q", quest.ID, q.ID)
				}
			}
		})
	}
}

// =============================================================================
// AGENT HANDLER TESTS
// =============================================================================

func TestHandleListAgents(t *testing.T) {
	a := sampleAgent()
	es := makeAgentEntityState(a)

	tests := []struct {
		name       string
		listFn     func(context.Context, int) ([]graph.EntityState, error)
		wantStatus int
		wantLen    int
	}{
		{
			name:       "success with one agent",
			listFn:     func(_ context.Context, _ int) ([]graph.EntityState, error) { return []graph.EntityState{es}, nil },
			wantStatus: http.StatusOK,
			wantLen:    1,
		},
		{
			name:       "bucket not found returns empty array",
			listFn:     func(_ context.Context, _ int) ([]graph.EntityState, error) { return nil, jetstream.ErrBucketNotFound },
			wantStatus: http.StatusOK,
			wantLen:    0,
		},
		{
			name:       "other error returns 500",
			listFn:     func(_ context.Context, _ int) ([]graph.EntityState, error) { return nil, errors.New("timeout") },
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(&mockGraph{listAgentsFn: tc.listFn}, &mockWorld{})
			req := httptest.NewRequest(http.MethodGet, "/agents", nil)
			rr := httptest.NewRecorder()
			svc.handleListAgents(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rr.Code, tc.wantStatus)
			}

			if tc.wantStatus == http.StatusOK {
				var agents []semdragons.Agent
				decodeJSON(t, rr.Body.Bytes(), &agents)
				if len(agents) != tc.wantLen {
					t.Errorf("agent count: got %d, want %d", len(agents), tc.wantLen)
				}
			}
		})
	}
}

func TestHandleGetAgent(t *testing.T) {
	a := sampleAgent()
	es := makeAgentEntityState(a)

	tests := []struct {
		name       string
		pathID     string
		getAgentFn func(context.Context, semdragons.AgentID) (*graph.EntityState, error)
		wantStatus int
	}{
		{
			name:       "success",
			pathID:     "a1",
			getAgentFn: func(_ context.Context, _ semdragons.AgentID) (*graph.EntityState, error) { return &es, nil },
			wantStatus: http.StatusOK,
		},
		{
			name:       "key not found returns 404",
			pathID:     "a1",
			getAgentFn: func(_ context.Context, _ semdragons.AgentID) (*graph.EntityState, error) { return nil, jetstream.ErrKeyNotFound },
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "invalid id (contains dot) returns 400",
			pathID: "c360.prod.game.board1.agent.abc",
			getAgentFn: func(_ context.Context, _ semdragons.AgentID) (*graph.EntityState, error) {
				return nil, errors.New("should not be called")
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "server error returns 500",
			pathID:     "a1",
			getAgentFn: func(_ context.Context, _ semdragons.AgentID) (*graph.EntityState, error) { return nil, errors.New("io error") },
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(&mockGraph{getAgentFn: tc.getAgentFn}, &mockWorld{})

			mux := http.NewServeMux()
			mux.HandleFunc("GET /agents/{id}", svc.handleGetAgent)

			req := httptest.NewRequest(http.MethodGet, "/agents/"+tc.pathID, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rr.Code, tc.wantStatus)
			}

			if tc.wantStatus == http.StatusOK {
				var agent semdragons.Agent
				decodeJSON(t, rr.Body.Bytes(), &agent)
				if agent.Name != a.Name {
					t.Errorf("agent name: got %q, want %q", agent.Name, a.Name)
				}
			}
		})
	}
}

// =============================================================================
// BATTLE HANDLER TESTS
// =============================================================================

func TestHandleListBattles(t *testing.T) {
	b := sampleBattle()
	es := makeBattleEntityState(b)

	tests := []struct {
		name       string
		listFn     func(context.Context, string, int) ([]graph.EntityState, error)
		wantStatus int
		wantLen    int
	}{
		{
			name:       "success with one battle",
			listFn:     func(_ context.Context, _ string, _ int) ([]graph.EntityState, error) { return []graph.EntityState{es}, nil },
			wantStatus: http.StatusOK,
			wantLen:    1,
		},
		{
			name:       "bucket not found returns empty array",
			listFn:     func(_ context.Context, _ string, _ int) ([]graph.EntityState, error) { return nil, jetstream.ErrBucketNotFound },
			wantStatus: http.StatusOK,
			wantLen:    0,
		},
		{
			name:       "other error returns 500",
			listFn:     func(_ context.Context, _ string, _ int) ([]graph.EntityState, error) { return nil, errors.New("timeout") },
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(&mockGraph{listEntitiesByTypeFn: tc.listFn}, &mockWorld{})
			req := httptest.NewRequest(http.MethodGet, "/battles", nil)
			rr := httptest.NewRecorder()
			svc.handleListBattles(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rr.Code, tc.wantStatus)
			}

			if tc.wantStatus == http.StatusOK {
				var battles []semdragons.BossBattle
				decodeJSON(t, rr.Body.Bytes(), &battles)
				if len(battles) != tc.wantLen {
					t.Errorf("battle count: got %d, want %d", len(battles), tc.wantLen)
				}
			}
		})
	}
}

func TestHandleGetBattle(t *testing.T) {
	b := sampleBattle()
	es := makeBattleEntityState(b)

	tests := []struct {
		name         string
		pathID       string
		getBattleFn  func(context.Context, semdragons.BattleID) (*graph.EntityState, error)
		wantStatus   int
	}{
		{
			name:        "success",
			pathID:      "b1",
			getBattleFn: func(_ context.Context, _ semdragons.BattleID) (*graph.EntityState, error) { return &es, nil },
			wantStatus:  http.StatusOK,
		},
		{
			name:        "key not found returns 404",
			pathID:      "b1",
			getBattleFn: func(_ context.Context, _ semdragons.BattleID) (*graph.EntityState, error) { return nil, jetstream.ErrKeyNotFound },
			wantStatus:  http.StatusNotFound,
		},
		{
			name:   "invalid id returns 400",
			pathID: "c360.prod.game.board1.battle.b1",
			getBattleFn: func(_ context.Context, _ semdragons.BattleID) (*graph.EntityState, error) {
				return nil, errors.New("should not be called")
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "server error returns 500",
			pathID:      "b1",
			getBattleFn: func(_ context.Context, _ semdragons.BattleID) (*graph.EntityState, error) { return nil, errors.New("io error") },
			wantStatus:  http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(&mockGraph{getBattleFn: tc.getBattleFn}, &mockWorld{})

			mux := http.NewServeMux()
			mux.HandleFunc("GET /battles/{id}", svc.handleGetBattle)

			req := httptest.NewRequest(http.MethodGet, "/battles/"+tc.pathID, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rr.Code, tc.wantStatus)
			}
		})
	}
}

// =============================================================================
// CREATE QUEST HANDLER TESTS
// =============================================================================

func TestHandleCreateQuest(t *testing.T) {
	boardCfg := &semdragons.BoardConfig{Org: "test", Platform: "dev", Board: "board1"}

	tests := []struct {
		name         string
		body         any
		emitEntityFn func(context.Context, graph.Graphable, string) error
		wantStatus   int
		checkBody    func(t *testing.T, body []byte)
	}{
		{
			name:       "success creates quest with 201",
			body:       map[string]any{"objective": "Defeat the lich king"},
			wantStatus: http.StatusCreated,
			checkBody: func(t *testing.T, body []byte) {
				var q semdragons.Quest
				decodeJSON(t, body, &q)
				if q.Title != "Defeat the lich king" {
					t.Errorf("title: got %q, want %q", q.Title, "Defeat the lich king")
				}
				if q.Status != semdragons.QuestPosted {
					t.Errorf("status: got %q, want posted", q.Status)
				}
			},
		},
		{
			name:       "missing objective returns 400",
			body:       map[string]any{"hints": map[string]any{}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "difficulty out of range returns 400",
			body: map[string]any{
				"objective": "A quest",
				"hints":     map[string]any{"suggested_difficulty": 99},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "valid difficulty hint is applied",
			body: map[string]any{
				"objective": "Hard quest",
				"hints":     map[string]any{"suggested_difficulty": 3}, // DifficultyHard
			},
			wantStatus: http.StatusCreated,
			checkBody: func(t *testing.T, body []byte) {
				var q semdragons.Quest
				decodeJSON(t, body, &q)
				if q.Difficulty != semdragons.DifficultyHard {
					t.Errorf("difficulty: got %d, want %d", q.Difficulty, semdragons.DifficultyHard)
				}
			},
		},
		{
			name:       "invalid JSON body returns 400",
			body:       "this is not json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "emit error returns 500",
			body:  map[string]any{"objective": "Quest that fails to emit"},
			emitEntityFn: func(_ context.Context, _ graph.Graphable, _ string) error {
				return errors.New("nats write failed")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			emitFn := tc.emitEntityFn // may be nil → mockGraph default returns nil

			g := &mockGraph{
				configFn:     func() *semdragons.BoardConfig { return boardCfg },
				emitEntityFn: emitFn,
			}
			svc := newTestService(g, &mockWorld{})

			var bodyBytes []byte
			switch v := tc.body.(type) {
			case string:
				bodyBytes = []byte(v)
			default:
				var err error
				bodyBytes, err = json.Marshal(v)
				if err != nil {
					t.Fatalf("marshal request body: %v", err)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/quests", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			svc.handleCreateQuest(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

// =============================================================================
// RECRUIT AGENT HANDLER TESTS
// =============================================================================

func TestHandleRecruitAgent(t *testing.T) {
	boardCfg := &semdragons.BoardConfig{Org: "test", Platform: "dev", Board: "board1"}

	tests := []struct {
		name         string
		body         any
		emitEntityFn func(context.Context, graph.Graphable, string) error
		wantStatus   int
		checkBody    func(t *testing.T, body []byte)
	}{
		{
			name:       "success creates agent with 201",
			body:       map[string]any{"name": "Gandalf"},
			wantStatus: http.StatusCreated,
			checkBody: func(t *testing.T, body []byte) {
				var a semdragons.Agent
				decodeJSON(t, body, &a)
				if a.Name != "Gandalf" {
					t.Errorf("name: got %q, want %q", a.Name, "Gandalf")
				}
				if a.Status != semdragons.AgentIdle {
					t.Errorf("status: got %q, want idle", a.Status)
				}
				if a.Level != 1 {
					t.Errorf("level: got %d, want 1", a.Level)
				}
			},
		},
		{
			name:       "missing name returns 400",
			body:       map[string]any{"is_npc": true},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON returns 400",
			body:       "not json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "skills are wired into proficiencies",
			body: map[string]any{
				"name":   "Skilled Agent",
				"skills": []string{"code_generation"},
			},
			wantStatus: http.StatusCreated,
			checkBody: func(t *testing.T, body []byte) {
				var a semdragons.Agent
				decodeJSON(t, body, &a)
				if _, ok := a.SkillProficiencies[semdragons.SkillCodeGen]; !ok {
					t.Error("expected code_generation skill proficiency in response")
				}
			},
		},
		{
			name:  "emit error returns 500",
			body:  map[string]any{"name": "Broken Agent"},
			emitEntityFn: func(_ context.Context, _ graph.Graphable, _ string) error {
				return errors.New("nats write failed")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &mockGraph{
				configFn:     func() *semdragons.BoardConfig { return boardCfg },
				emitEntityFn: tc.emitEntityFn,
			}
			svc := newTestService(g, &mockWorld{})

			var bodyBytes []byte
			switch v := tc.body.(type) {
			case string:
				bodyBytes = []byte(v)
			default:
				var err error
				bodyBytes, err = json.Marshal(v)
				if err != nil {
					t.Fatalf("marshal request body: %v", err)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/agents", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			svc.handleRecruitAgent(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

// =============================================================================
// RETIRE AGENT HANDLER TESTS
// =============================================================================

func TestHandleRetireAgent(t *testing.T) {
	a := sampleAgent()
	es := makeAgentEntityState(a)

	tests := []struct {
		name               string
		pathID             string
		getAgentFn         func(context.Context, semdragons.AgentID) (*graph.EntityState, error)
		emitEntityUpdateFn func(context.Context, graph.Graphable, string) error
		wantStatus         int
	}{
		{
			name:       "success returns 204",
			pathID:     "a1",
			getAgentFn: func(_ context.Context, _ semdragons.AgentID) (*graph.EntityState, error) { return &es, nil },
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "agent not found returns 404",
			pathID:     "a1",
			getAgentFn: func(_ context.Context, _ semdragons.AgentID) (*graph.EntityState, error) { return nil, jetstream.ErrKeyNotFound },
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "invalid id returns 400",
			pathID: "c360.prod.game.board1.agent.abc",
			getAgentFn: func(_ context.Context, _ semdragons.AgentID) (*graph.EntityState, error) {
				return nil, errors.New("should not be called")
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "get agent error returns 500",
			pathID:     "a1",
			getAgentFn: func(_ context.Context, _ semdragons.AgentID) (*graph.EntityState, error) { return nil, errors.New("io error") },
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "emit update error returns 500",
			pathID:     "a1",
			getAgentFn: func(_ context.Context, _ semdragons.AgentID) (*graph.EntityState, error) { return &es, nil },
			emitEntityUpdateFn: func(_ context.Context, _ graph.Graphable, _ string) error {
				return errors.New("write failed")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &mockGraph{
				getAgentFn:         tc.getAgentFn,
				emitEntityUpdateFn: tc.emitEntityUpdateFn,
			}
			svc := newTestService(g, &mockWorld{})

			mux := http.NewServeMux()
			mux.HandleFunc("POST /agents/{id}/retire", svc.handleRetireAgent)

			req := httptest.NewRequest(http.MethodPost, "/agents/"+tc.pathID+"/retire", nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rr.Code, tc.wantStatus)
			}
		})
	}
}

// =============================================================================
// STUB HANDLER TESTS (501 Not Implemented)
// =============================================================================

func TestStubHandlers(t *testing.T) {
	svc := newTestService(&mockGraph{}, &mockWorld{})

	tests := []struct {
		name    string
		method  string
		pattern string
		path    string
		handler http.HandlerFunc
	}{
		{
			name:    "get trajectory returns 501",
			method:  http.MethodGet,
			pattern: "GET /trajectories/{id}",
			path:    "/trajectories/traj1",
			handler: svc.handleGetTrajectory,
		},
		{
			name:    "dm chat returns 501",
			method:  http.MethodPost,
			pattern: "POST /dm/chat",
			path:    "/dm/chat",
			handler: svc.handleDMChat,
		},
		{
			name:    "get store item returns 501",
			method:  http.MethodGet,
			pattern: "GET /store/{id}",
			path:    "/store/item1",
			handler: svc.handleGetStoreItem,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc(tc.pattern, tc.handler)

			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusNotImplemented {
				t.Errorf("expected 501 Not Implemented, got %d", rr.Code)
			}
		})
	}
}

// =============================================================================
// RESPONSE HELPER TESTS
// =============================================================================

func TestWriteJSON(t *testing.T) {
	svc := newTestService(&mockGraph{}, &mockWorld{})

	rr := httptest.NewRecorder()
	svc.writeJSON(rr, map[string]string{"key": "value"})

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}

	var got map[string]string
	decodeJSON(t, rr.Body.Bytes(), &got)
	if got["key"] != "value" {
		t.Errorf("body: got %q, want value", got["key"])
	}
}

func TestWriteError(t *testing.T) {
	svc := newTestService(&mockGraph{}, &mockWorld{})

	rr := httptest.NewRecorder()
	svc.writeError(rr, "something went wrong", http.StatusBadRequest)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}

	var got map[string]string
	decodeJSON(t, rr.Body.Bytes(), &got)
	if got["error"] != "something went wrong" {
		t.Errorf("error message: got %q, want 'something went wrong'", got["error"])
	}
}

// =============================================================================
// COMPILE-TIME INTERFACE SATISFACTION CHECKS
// =============================================================================

// These blank-identifier assignments verify at compile time that the concrete
// types satisfy the interfaces, so a method signature drift is caught as a
// build error rather than a runtime panic.
var (
	_ GraphQuerier      = (*semdragons.GraphClient)(nil)
	_ GraphQuerier      = (*mockGraph)(nil)
	_ WorldStateProvider = (*mockWorld)(nil)
)

// messageTripleUsed ensures the graph/message import is referenced so the
// compiler doesn't complain. EntityState.Triples holds []message.Triple.
var _ = message.Triple{}
