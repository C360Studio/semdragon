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
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/c360studio/semdragons/processor/boardcontrol"
	"github.com/c360studio/semdragons/processor/bossbattle"
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
	configFn             func() *domain.BoardConfig
	getQuestFn           func(ctx context.Context, id domain.QuestID) (*graph.EntityState, error)
	getAgentFn           func(ctx context.Context, id domain.AgentID) (*graph.EntityState, error)
	getBattleFn          func(ctx context.Context, id domain.BattleID) (*graph.EntityState, error)
	getPartyFn           func(ctx context.Context, id domain.PartyID) (*graph.EntityState, error)
	getPeerReviewFn      func(ctx context.Context, id domain.PeerReviewID) (*graph.EntityState, error)
	listQuestsFn         func(ctx context.Context, limit int) ([]graph.EntityState, error)
	listAgentsFn         func(ctx context.Context, limit int) ([]graph.EntityState, error)
	listPeerReviewsFn    func(ctx context.Context, limit int) ([]graph.EntityState, error)
	listEntitiesByTypeFn func(ctx context.Context, entityType string, limit int) ([]graph.EntityState, error)
	emitEntityFn         func(ctx context.Context, entity graph.Graphable, eventType string) error
	emitEntityUpdateFn   func(ctx context.Context, entity graph.Graphable, eventType string) error
}

func (m *mockGraph) Config() *domain.BoardConfig {
	if m.configFn != nil {
		return m.configFn()
	}
	return &domain.BoardConfig{Org: "test", Platform: "dev", Board: "board1"}
}

func (m *mockGraph) GetQuest(ctx context.Context, id domain.QuestID) (*graph.EntityState, error) {
	if m.getQuestFn != nil {
		return m.getQuestFn(ctx, id)
	}
	return nil, jetstream.ErrKeyNotFound
}

func (m *mockGraph) GetAgent(ctx context.Context, id domain.AgentID) (*graph.EntityState, error) {
	if m.getAgentFn != nil {
		return m.getAgentFn(ctx, id)
	}
	return nil, jetstream.ErrKeyNotFound
}

func (m *mockGraph) GetBattle(ctx context.Context, id domain.BattleID) (*graph.EntityState, error) {
	if m.getBattleFn != nil {
		return m.getBattleFn(ctx, id)
	}
	return nil, jetstream.ErrKeyNotFound
}

func (m *mockGraph) GetParty(ctx context.Context, id domain.PartyID) (*graph.EntityState, error) {
	if m.getPartyFn != nil {
		return m.getPartyFn(ctx, id)
	}
	return nil, jetstream.ErrKeyNotFound
}

func (m *mockGraph) GetPeerReview(ctx context.Context, id domain.PeerReviewID) (*graph.EntityState, error) {
	if m.getPeerReviewFn != nil {
		return m.getPeerReviewFn(ctx, id)
	}
	return nil, jetstream.ErrKeyNotFound
}

func (m *mockGraph) ListQuestsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	if m.listQuestsFn != nil {
		return m.listQuestsFn(ctx, limit)
	}
	return nil, nil
}

func (m *mockGraph) ListPeerReviewsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	if m.listPeerReviewsFn != nil {
		return m.listPeerReviewsFn(ctx, limit)
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

// mockStore implements StoreProvider with function fields.
type mockStore struct {
	listItemsFn        func(agentTier domain.TrustTier) []agentstore.StoreItem
	catalogFn          func() []agentstore.StoreItem
	getItemFn          func(itemID string) (*agentstore.StoreItem, bool)
	purchaseFn         func(ctx context.Context, agentID domain.AgentID, itemID string, currentXP int64, currentLevel int, agentGuilds []domain.GuildID) (*agentstore.OwnedItem, error)
	canAffordFn        func(itemID string, currentXP int64) (bool, int64)
	getInventoryFn     func(agentID domain.AgentID) *agentstore.AgentInventory
	useConsumableFn    func(ctx context.Context, agentID domain.AgentID, consumableID string, questID *domain.QuestID) error
	getActiveEffectsFn func(agentID domain.AgentID) []agentstore.ActiveEffect
}

func (m *mockStore) ListItems(agentTier domain.TrustTier) []agentstore.StoreItem {
	if m.listItemsFn != nil {
		return m.listItemsFn(agentTier)
	}
	return nil
}

func (m *mockStore) Catalog() []agentstore.StoreItem {
	if m.catalogFn != nil {
		return m.catalogFn()
	}
	return nil
}

func (m *mockStore) GetItem(itemID string) (*agentstore.StoreItem, bool) {
	if m.getItemFn != nil {
		return m.getItemFn(itemID)
	}
	return nil, false
}

func (m *mockStore) Purchase(ctx context.Context, agentID domain.AgentID, itemID string, currentXP int64, currentLevel int, agentGuilds []domain.GuildID) (*agentstore.OwnedItem, error) {
	if m.purchaseFn != nil {
		return m.purchaseFn(ctx, agentID, itemID, currentXP, currentLevel, agentGuilds)
	}
	return nil, errors.New("not implemented")
}

func (m *mockStore) CanAfford(itemID string, currentXP int64) (bool, int64) {
	if m.canAffordFn != nil {
		return m.canAffordFn(itemID, currentXP)
	}
	return false, 0
}

func (m *mockStore) GetInventory(agentID domain.AgentID) *agentstore.AgentInventory {
	if m.getInventoryFn != nil {
		return m.getInventoryFn(agentID)
	}
	return agentstore.NewAgentInventory(agentID)
}

func (m *mockStore) UseConsumable(ctx context.Context, agentID domain.AgentID, consumableID string, questID *domain.QuestID) error {
	if m.useConsumableFn != nil {
		return m.useConsumableFn(ctx, agentID, consumableID, questID)
	}
	return errors.New("not implemented")
}

func (m *mockStore) GetActiveEffects(agentID domain.AgentID) []agentstore.ActiveEffect {
	if m.getActiveEffectsFn != nil {
		return m.getActiveEffectsFn(agentID)
	}
	return nil
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

// newTestServiceWithStore creates a Service with graph, world, and store providers.
func newTestServiceWithStore(g GraphQuerier, w WorldStateProvider, s StoreProvider) *Service {
	return &Service{
		graph:  g,
		world:  w,
		store:  s,
		config: Config{Board: "board1", MaxEntities: 100},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// makeQuestEntityState builds an EntityState whose Triples will reconstruct
// to the supplied Quest via QuestFromEntityState.
func makeQuestEntityState(q *domain.Quest) graph.EntityState {
	return graph.EntityState{
		ID:      string(q.ID),
		Triples: q.Triples(),
	}
}

// makeAgentEntityState builds an EntityState whose Triples will reconstruct
// to the supplied Agent via AgentFromEntityState.
func makeAgentEntityState(a *agentprogression.Agent) graph.EntityState {
	return graph.EntityState{
		ID:      string(a.ID),
		Triples: a.Triples(),
	}
}

// makeBattleEntityState builds an EntityState whose Triples will reconstruct
// to the supplied BossBattle via BattleFromEntityState.
func makeBattleEntityState(b *bossbattle.BossBattle) graph.EntityState {
	return graph.EntityState{
		ID:      string(b.ID),
		Triples: b.Triples(),
	}
}

// sampleQuest returns a minimal Quest suitable for use in tests.
func sampleQuest() *domain.Quest {
	return &domain.Quest{
		ID:          domain.QuestID("test.dev.game.board1.quest.q1"),
		Title:       "Slay the Dragon",
		Description: "A very dangerous quest",
		Status:      domain.QuestPosted,
		Difficulty:  domain.DifficultyModerate,
		BaseXP:      100,
		MaxAttempts: 3,
	}
}

// sampleAgent returns a minimal Agent suitable for use in tests.
func sampleAgent() *agentprogression.Agent {
	return &agentprogression.Agent{
		ID:                 domain.AgentID("test.dev.game.board1.agent.a1"),
		Name:               "TestAgent",
		Status:             domain.AgentIdle,
		Level:              1,
		XP:                 0,
		XPToLevel:          100,
		Tier:               domain.TierApprentice,
		SkillProficiencies: make(map[domain.SkillTag]domain.SkillProficiency),
	}
}

// sampleBattle returns a minimal BossBattle suitable for use in tests.
func sampleBattle() *bossbattle.BossBattle {
	return &bossbattle.BossBattle{
		ID:      domain.BattleID("test.dev.game.board1.battle.b1"),
		QuestID: domain.QuestID("test.dev.game.board1.quest.q1"),
		AgentID: domain.AgentID("test.dev.game.board1.agent.a1"),
		Status:  domain.BattleActive,
		Level:   domain.ReviewStandard,
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
				var quests []domain.Quest
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
		getQuestFn func(context.Context, domain.QuestID) (*graph.EntityState, error)
		wantStatus int
	}{
		{
			name:       "success",
			pathID:     "q1",
			getQuestFn: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) { return &es, nil },
			wantStatus: http.StatusOK,
		},
		{
			name:   "key not found returns 404",
			pathID: "q1",
			getQuestFn: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "bucket not found returns 404",
			pathID: "q1",
			getQuestFn: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				return nil, jetstream.ErrBucketNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "invalid id (contains dot) returns 400",
			pathID: "c360.prod.game.board1.quest.abc",
			// getQuestFn will not be called
			getQuestFn: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				return nil, errors.New("should not be called")
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "server error returns 500",
			pathID: "q1",
			getQuestFn: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				return nil, errors.New("nats error")
			},
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
				var quest domain.Quest
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
				var agents []agentprogression.Agent
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
		getAgentFn func(context.Context, domain.AgentID) (*graph.EntityState, error)
		wantStatus int
	}{
		{
			name:       "success",
			pathID:     "a1",
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) { return &es, nil },
			wantStatus: http.StatusOK,
		},
		{
			name:   "key not found returns 404",
			pathID: "a1",
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "invalid id (contains dot) returns 400",
			pathID: "c360.prod.game.board1.agent.abc",
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				return nil, errors.New("should not be called")
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "server error returns 500",
			pathID: "a1",
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				return nil, errors.New("io error")
			},
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
				var agent agentprogression.Agent
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
			name: "success with one battle",
			listFn: func(_ context.Context, _ string, _ int) ([]graph.EntityState, error) {
				return []graph.EntityState{es}, nil
			},
			wantStatus: http.StatusOK,
			wantLen:    1,
		},
		{
			name: "bucket not found returns empty array",
			listFn: func(_ context.Context, _ string, _ int) ([]graph.EntityState, error) {
				return nil, jetstream.ErrBucketNotFound
			},
			wantStatus: http.StatusOK,
			wantLen:    0,
		},
		{
			name: "other error returns 500",
			listFn: func(_ context.Context, _ string, _ int) ([]graph.EntityState, error) {
				return nil, errors.New("timeout")
			},
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
				var battles []bossbattle.BossBattle
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
		name        string
		pathID      string
		getBattleFn func(context.Context, domain.BattleID) (*graph.EntityState, error)
		wantStatus  int
	}{
		{
			name:        "success",
			pathID:      "b1",
			getBattleFn: func(_ context.Context, _ domain.BattleID) (*graph.EntityState, error) { return &es, nil },
			wantStatus:  http.StatusOK,
		},
		{
			name:   "key not found returns 404",
			pathID: "b1",
			getBattleFn: func(_ context.Context, _ domain.BattleID) (*graph.EntityState, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "invalid id returns 400",
			pathID: "c360.prod.game.board1.battle.b1",
			getBattleFn: func(_ context.Context, _ domain.BattleID) (*graph.EntityState, error) {
				return nil, errors.New("should not be called")
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "server error returns 500",
			pathID: "b1",
			getBattleFn: func(_ context.Context, _ domain.BattleID) (*graph.EntityState, error) {
				return nil, errors.New("io error")
			},
			wantStatus: http.StatusInternalServerError,
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
	boardCfg := &domain.BoardConfig{Org: "test", Platform: "dev", Board: "board1"}

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
				var q domain.Quest
				decodeJSON(t, body, &q)
				if q.Title != "Defeat the lich king" {
					t.Errorf("title: got %q, want %q", q.Title, "Defeat the lich king")
				}
				if q.Status != domain.QuestPosted {
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
				var q domain.Quest
				decodeJSON(t, body, &q)
				if q.Difficulty != domain.DifficultyHard {
					t.Errorf("difficulty: got %d, want %d", q.Difficulty, domain.DifficultyHard)
				}
			},
		},
		{
			name:       "invalid JSON body returns 400",
			body:       "this is not json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "emit error returns 500",
			body: map[string]any{"objective": "Quest that fails to emit"},
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
				configFn:     func() *domain.BoardConfig { return boardCfg },
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
// CREATE QUEST WITH REVIEW HINTS TESTS
// =============================================================================

func TestHandleCreateQuest_ReviewHints(t *testing.T) {
	boardCfg := &domain.BoardConfig{Org: "test", Platform: "dev", Board: "board1"}

	tests := []struct {
		name      string
		body      map[string]any
		checkBody func(t *testing.T, body []byte)
	}{
		{
			name: "require_human_review sets constraints",
			body: map[string]any{
				"objective": "Review quest",
				"hints":     map[string]any{"require_human_review": true},
			},
			checkBody: func(t *testing.T, body []byte) {
				var q domain.Quest
				decodeJSON(t, body, &q)
				if !q.Constraints.RequireReview {
					t.Error("expected RequireReview to be true")
				}
				if q.Constraints.ReviewLevel != domain.ReviewStandard {
					t.Errorf("ReviewLevel: got %d, want %d (ReviewStandard)", q.Constraints.ReviewLevel, domain.ReviewStandard)
				}
			},
		},
		{
			name: "explicit review_level overrides default",
			body: map[string]any{
				"objective": "Strict review quest",
				"hints":     map[string]any{"require_human_review": true, "review_level": 2},
			},
			checkBody: func(t *testing.T, body []byte) {
				var q domain.Quest
				decodeJSON(t, body, &q)
				if q.Constraints.ReviewLevel != domain.ReviewStrict {
					t.Errorf("ReviewLevel: got %d, want %d (ReviewStrict)", q.Constraints.ReviewLevel, domain.ReviewStrict)
				}
			},
		},
		{
			name: "no review hint leaves defaults",
			body: map[string]any{
				"objective": "Normal quest",
			},
			checkBody: func(t *testing.T, body []byte) {
				var q domain.Quest
				decodeJSON(t, body, &q)
				if q.Constraints.RequireReview {
					t.Error("expected RequireReview to be false")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &mockGraph{
				configFn: func() *domain.BoardConfig { return boardCfg },
			}
			svc := newTestService(g, &mockWorld{})

			bodyBytes, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/quests", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			svc.handleCreateQuest(rr, req)

			if rr.Code != http.StatusCreated {
				t.Fatalf("status: got %d, want 201\nbody: %s", rr.Code, rr.Body.String())
			}
			tc.checkBody(t, rr.Body.Bytes())
		})
	}
}

// =============================================================================
// QUEST LIFECYCLE HANDLER TESTS
// =============================================================================

func TestHandleClaimQuest(t *testing.T) {
	postedQuest := sampleQuest()
	postedQuest.Difficulty = domain.DifficultyEasy // Apprentice can claim easy quests

	claimedQuest := sampleQuest()
	claimedQuest.Status = domain.QuestClaimed

	idleAgent := sampleAgent()
	busyAgent := sampleAgent()
	busyAgent.Status = domain.AgentOnQuest

	lowTierAgent := sampleAgent()
	lowTierAgent.Tier = domain.TierApprentice

	skilledAgent := sampleAgent()
	skilledAgent.Tier = domain.TierExpert
	skilledAgent.SkillProficiencies = map[domain.SkillTag]domain.SkillProficiency{
		domain.SkillCodeGen: {Level: 1},
	}

	tests := []struct {
		name       string
		pathID     string
		body       any
		getQuest   func(context.Context, domain.QuestID) (*graph.EntityState, error)
		getAgent   func(context.Context, domain.AgentID) (*graph.EntityState, error)
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:   "success claims posted quest",
			pathID: "q1",
			body:   map[string]any{"agent_id": "a1"},
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(postedQuest)
				return &es, nil
			},
			getAgent: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				es := makeAgentEntityState(idleAgent)
				return &es, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var q domain.Quest
				decodeJSON(t, body, &q)
				if q.Status != domain.QuestClaimed {
					t.Errorf("status: got %q, want claimed", q.Status)
				}
			},
		},
		{
			name:       "missing agent_id returns 400",
			pathID:     "q1",
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "quest not posted returns 409",
			pathID: "q1",
			body:   map[string]any{"agent_id": "a1"},
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(claimedQuest)
				return &es, nil
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:   "agent not idle returns 409",
			pathID: "q1",
			body:   map[string]any{"agent_id": "a1"},
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(postedQuest)
				return &es, nil
			},
			getAgent: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				es := makeAgentEntityState(busyAgent)
				return &es, nil
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:   "agent tier too low returns 403",
			pathID: "q1",
			body:   map[string]any{"agent_id": "a1"},
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				q := sampleQuest()
				q.Difficulty = domain.DifficultyHard // requires TierExpert
				es := makeQuestEntityState(q)
				return &es, nil
			},
			getAgent: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				es := makeAgentEntityState(lowTierAgent)
				return &es, nil
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:   "agent missing required skill returns 403",
			pathID: "q1",
			body:   map[string]any{"agent_id": "a1"},
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				q := sampleQuest()
				q.RequiredSkills = []domain.SkillTag{domain.SkillAnalysis}
				es := makeQuestEntityState(q)
				return &es, nil
			},
			getAgent: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				es := makeAgentEntityState(idleAgent)
				return &es, nil
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "quest not found returns 404",
			pathID:     "missing",
			body:       map[string]any{"agent_id": "a1"},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &mockGraph{
				getQuestFn: tc.getQuest,
				getAgentFn: tc.getAgent,
			}
			svc := newTestService(g, &mockWorld{})

			bodyBytes, _ := json.Marshal(tc.body)
			mux := http.NewServeMux()
			mux.HandleFunc("POST /quests/{id}/claim", svc.handleClaimQuest)

			req := httptest.NewRequest(http.MethodPost, "/quests/"+tc.pathID+"/claim", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

func TestHandleStartQuest(t *testing.T) {
	claimedQuest := sampleQuest()
	claimedQuest.Status = domain.QuestClaimed

	postedQuest := sampleQuest()

	tests := []struct {
		name       string
		pathID     string
		getQuest   func(context.Context, domain.QuestID) (*graph.EntityState, error)
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:   "success starts claimed quest",
			pathID: "q1",
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(claimedQuest)
				return &es, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var q domain.Quest
				decodeJSON(t, body, &q)
				if q.Status != domain.QuestInProgress {
					t.Errorf("status: got %q, want in_progress", q.Status)
				}
			},
		},
		{
			name:   "quest not claimed returns 409",
			pathID: "q1",
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(postedQuest)
				return &es, nil
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "quest not found returns 404",
			pathID:     "missing",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &mockGraph{getQuestFn: tc.getQuest}
			svc := newTestService(g, &mockWorld{})

			mux := http.NewServeMux()
			mux.HandleFunc("POST /quests/{id}/start", svc.handleStartQuest)

			req := httptest.NewRequest(http.MethodPost, "/quests/"+tc.pathID+"/start", nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

func TestHandleSubmitResult(t *testing.T) {
	inProgressQuest := sampleQuest()
	inProgressQuest.Status = domain.QuestInProgress

	reviewQuest := sampleQuest()
	reviewQuest.Status = domain.QuestInProgress
	reviewQuest.Constraints.RequireReview = true

	tests := []struct {
		name       string
		pathID     string
		body       any
		getQuest   func(context.Context, domain.QuestID) (*graph.EntityState, error)
		getAgent   func(context.Context, domain.AgentID) (*graph.EntityState, error)
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:   "submit without review completes quest",
			pathID: "q1",
			body:   map[string]any{"output": "result data"},
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(inProgressQuest)
				return &es, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var q domain.Quest
				decodeJSON(t, body, &q)
				if q.Status != domain.QuestCompleted {
					t.Errorf("status: got %q, want completed", q.Status)
				}
			},
		},
		{
			name:   "submit with review goes to in_review",
			pathID: "q1",
			body:   map[string]any{"output": "result data"},
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(reviewQuest)
				return &es, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var q domain.Quest
				decodeJSON(t, body, &q)
				if q.Status != domain.QuestInReview {
					t.Errorf("status: got %q, want in_review", q.Status)
				}
			},
		},
		{
			name:       "quest not found returns 404",
			pathID:     "missing",
			body:       map[string]any{"output": "data"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "quest not in_progress returns 409",
			pathID: "q1",
			body:   map[string]any{"output": "data"},
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(sampleQuest()) // status=posted
				return &es, nil
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &mockGraph{
				getQuestFn: tc.getQuest,
				getAgentFn: tc.getAgent,
			}
			svc := newTestService(g, &mockWorld{})

			bodyBytes, _ := json.Marshal(tc.body)
			mux := http.NewServeMux()
			mux.HandleFunc("POST /quests/{id}/submit", svc.handleSubmitResult)

			req := httptest.NewRequest(http.MethodPost, "/quests/"+tc.pathID+"/submit", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

func TestHandleFailQuest(t *testing.T) {
	inProgressQuest := sampleQuest()
	inProgressQuest.Status = domain.QuestInProgress
	inProgressQuest.MaxAttempts = 3
	inProgressQuest.Attempts = 0

	lastAttemptQuest := sampleQuest()
	lastAttemptQuest.Status = domain.QuestInProgress
	lastAttemptQuest.MaxAttempts = 3
	lastAttemptQuest.Attempts = 2

	tests := []struct {
		name       string
		pathID     string
		body       any
		getQuest   func(context.Context, domain.QuestID) (*graph.EntityState, error)
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:   "fail with retries reposts quest",
			pathID: "q1",
			body:   map[string]any{"reason": "timeout"},
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(inProgressQuest)
				return &es, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var q domain.Quest
				decodeJSON(t, body, &q)
				if q.Status != domain.QuestPosted {
					t.Errorf("status: got %q, want posted (repost)", q.Status)
				}
				if q.Attempts != 1 {
					t.Errorf("attempts: got %d, want 1", q.Attempts)
				}
			},
		},
		{
			name:   "fail on last attempt permanently fails",
			pathID: "q1",
			body:   map[string]any{"reason": "error"},
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(lastAttemptQuest)
				return &es, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var q domain.Quest
				decodeJSON(t, body, &q)
				if q.Status != domain.QuestFailed {
					t.Errorf("status: got %q, want failed", q.Status)
				}
				if q.Attempts != 3 {
					t.Errorf("attempts: got %d, want 3", q.Attempts)
				}
			},
		},
		{
			name:   "quest not in_progress returns 409",
			pathID: "q1",
			body:   map[string]any{"reason": "test"},
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(sampleQuest()) // posted
				return &es, nil
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &mockGraph{getQuestFn: tc.getQuest}
			svc := newTestService(g, &mockWorld{})

			bodyBytes, _ := json.Marshal(tc.body)
			mux := http.NewServeMux()
			mux.HandleFunc("POST /quests/{id}/fail", svc.handleFailQuest)

			req := httptest.NewRequest(http.MethodPost, "/quests/"+tc.pathID+"/fail", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

func TestHandleAbandonQuest(t *testing.T) {
	claimedQuest := sampleQuest()
	claimedQuest.Status = domain.QuestClaimed

	tests := []struct {
		name       string
		pathID     string
		getQuest   func(context.Context, domain.QuestID) (*graph.EntityState, error)
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:   "abandon returns quest to posted",
			pathID: "q1",
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(claimedQuest)
				return &es, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var q domain.Quest
				decodeJSON(t, body, &q)
				if q.Status != domain.QuestPosted {
					t.Errorf("status: got %q, want posted", q.Status)
				}
			},
		},
		{
			name:   "abandon posted quest returns 409",
			pathID: "q1",
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(sampleQuest())
				return &es, nil
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &mockGraph{getQuestFn: tc.getQuest}
			svc := newTestService(g, &mockWorld{})

			mux := http.NewServeMux()
			mux.HandleFunc("POST /quests/{id}/abandon", svc.handleAbandonQuest)

			req := httptest.NewRequest(http.MethodPost, "/quests/"+tc.pathID+"/abandon", bytes.NewReader([]byte(`{}`)))
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

func TestHandleCompleteQuest(t *testing.T) {
	inReviewQuest := sampleQuest()
	inReviewQuest.Status = domain.QuestInReview

	tests := []struct {
		name       string
		pathID     string
		getQuest   func(context.Context, domain.QuestID) (*graph.EntityState, error)
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:   "complete in_review quest",
			pathID: "q1",
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(inReviewQuest)
				return &es, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var q domain.Quest
				decodeJSON(t, body, &q)
				if q.Status != domain.QuestCompleted {
					t.Errorf("status: got %q, want completed", q.Status)
				}
			},
		},
		{
			name:   "complete posted quest returns 409",
			pathID: "q1",
			getQuest: func(_ context.Context, _ domain.QuestID) (*graph.EntityState, error) {
				es := makeQuestEntityState(sampleQuest())
				return &es, nil
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &mockGraph{getQuestFn: tc.getQuest}
			svc := newTestService(g, &mockWorld{})

			mux := http.NewServeMux()
			mux.HandleFunc("POST /quests/{id}/complete", svc.handleCompleteQuest)

			req := httptest.NewRequest(http.MethodPost, "/quests/"+tc.pathID+"/complete", nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

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
	boardCfg := &domain.BoardConfig{Org: "test", Platform: "dev", Board: "board1"}

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
				var a agentprogression.Agent
				decodeJSON(t, body, &a)
				if a.Name != "Gandalf" {
					t.Errorf("name: got %q, want %q", a.Name, "Gandalf")
				}
				if a.Status != domain.AgentIdle {
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
				var a agentprogression.Agent
				decodeJSON(t, body, &a)
				if _, ok := a.SkillProficiencies[domain.SkillCodeGen]; !ok {
					t.Error("expected code_generation skill proficiency in response")
				}
			},
		},
		{
			name: "emit error returns 500",
			body: map[string]any{"name": "Broken Agent"},
			emitEntityFn: func(_ context.Context, _ graph.Graphable, _ string) error {
				return errors.New("nats write failed")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &mockGraph{
				configFn:     func() *domain.BoardConfig { return boardCfg },
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
		getAgentFn         func(context.Context, domain.AgentID) (*graph.EntityState, error)
		emitEntityUpdateFn func(context.Context, graph.Graphable, string) error
		wantStatus         int
	}{
		{
			name:       "success returns 204",
			pathID:     "a1",
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) { return &es, nil },
			wantStatus: http.StatusNoContent,
		},
		{
			name:   "agent not found returns 404",
			pathID: "a1",
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "invalid id returns 400",
			pathID: "c360.prod.game.board1.agent.abc",
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				return nil, errors.New("should not be called")
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "get agent error returns 500",
			pathID: "a1",
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				return nil, errors.New("io error")
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "emit update error returns 500",
			pathID:     "a1",
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) { return &es, nil },
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

	// Trajectory lookup returns 503 when trajectories querier is nil
	t.Run("get trajectory returns 503 without querier", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /trajectories/{id}", svc.handleGetTrajectory)

		req := httptest.NewRequest(http.MethodGet, "/trajectories/traj1", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503 Service Unavailable, got %d", rr.Code)
		}
	})

	// DM chat returns 503 when model registry is nil (test service has no models)
	t.Run("dm chat returns 503 without models", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("POST /dm/chat", svc.handleDMChat)

		req := httptest.NewRequest(http.MethodPost, "/dm/chat", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503 Service Unavailable, got %d", rr.Code)
		}
	})
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
// STORE HANDLER TESTS
// =============================================================================

func sampleStoreItem() agentstore.StoreItem {
	return agentstore.StoreItem{
		ID:           "web_search",
		Name:         "Web Search",
		Description:  "Search the web",
		ItemType:     agentstore.ItemTypeTool,
		PurchaseType: agentstore.PurchasePermanent,
		XPCost:       50,
		MinTier:      domain.TierApprentice,
		InStock:      true,
	}
}

func TestHandleListStore(t *testing.T) {
	tool := sampleStoreItem()
	agent := sampleAgent()
	agent.XP = 500
	agent.Level = 5
	agent.Tier = domain.TierApprentice
	agentES := makeAgentEntityState(agent)

	tests := []struct {
		name       string
		query      string
		store      *mockStore
		getAgentFn func(context.Context, domain.AgentID) (*graph.EntityState, error)
		wantStatus int
		wantLen    int
	}{
		{
			name:  "catalog returns all items when no agent_id",
			query: "",
			store: &mockStore{
				catalogFn: func() []agentstore.StoreItem { return []agentstore.StoreItem{tool} },
			},
			wantStatus: http.StatusOK,
			wantLen:    1,
		},
		{
			name:  "filtered by agent tier when agent_id provided",
			query: "?agent_id=a1",
			store: &mockStore{
				listItemsFn: func(_ domain.TrustTier) []agentstore.StoreItem { return []agentstore.StoreItem{tool} },
			},
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				return &agentES, nil
			},
			wantStatus: http.StatusOK,
			wantLen:    1,
		},
		{
			name:  "agent not found returns 404",
			query: "?agent_id=missing",
			store: &mockStore{},
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mg := &mockGraph{getAgentFn: tc.getAgentFn}
			svc := newTestServiceWithStore(mg, &mockWorld{}, tc.store)

			req := httptest.NewRequest(http.MethodGet, "/store"+tc.query, nil)
			rr := httptest.NewRecorder()
			svc.handleListStore(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				var items []agentstore.StoreItem
				decodeJSON(t, rr.Body.Bytes(), &items)
				if len(items) != tc.wantLen {
					t.Errorf("items count: got %d, want %d", len(items), tc.wantLen)
				}
			}
		})
	}
}

func TestHandleGetStoreItem(t *testing.T) {
	tool := sampleStoreItem()

	tests := []struct {
		name       string
		pathID     string
		getItemFn  func(string) (*agentstore.StoreItem, bool)
		wantStatus int
	}{
		{
			name:   "found",
			pathID: "web_search",
			getItemFn: func(_ string) (*agentstore.StoreItem, bool) {
				return &tool, true
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "not found",
			pathID: "nonexistent",
			getItemFn: func(_ string) (*agentstore.StoreItem, bool) {
				return nil, false
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid id",
			pathID:     "bad.id",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{getItemFn: tc.getItemFn}
			svc := newTestServiceWithStore(&mockGraph{}, &mockWorld{}, store)

			mux := http.NewServeMux()
			mux.HandleFunc("GET /store/{id}", svc.handleGetStoreItem)

			req := httptest.NewRequest(http.MethodGet, "/store/"+tc.pathID, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				var item agentstore.StoreItem
				decodeJSON(t, rr.Body.Bytes(), &item)
				if item.ID != tool.ID {
					t.Errorf("item ID: got %q, want %q", item.ID, tool.ID)
				}
			}
		})
	}
}

func TestHandlePurchase(t *testing.T) {
	tool := sampleStoreItem()
	agent := sampleAgent()
	agent.XP = 200
	agent.Level = 5
	agent.Tier = domain.TierApprentice
	agentES := makeAgentEntityState(agent)

	tests := []struct {
		name       string
		body       map[string]string
		getAgentFn func(context.Context, domain.AgentID) (*graph.EntityState, error)
		getItemFn  func(string) (*agentstore.StoreItem, bool)
		purchaseFn func(context.Context, domain.AgentID, string, int64, int, []domain.GuildID) (*agentstore.OwnedItem, error)
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name: "successful purchase",
			body: map[string]string{"agent_id": "a1", "item_id": "web_search"},
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				return &agentES, nil
			},
			getItemFn: func(_ string) (*agentstore.StoreItem, bool) {
				return &tool, true
			},
			purchaseFn: func(_ context.Context, _ domain.AgentID, _ string, _ int64, _ int, _ []domain.GuildID) (*agentstore.OwnedItem, error) {
				return &agentstore.OwnedItem{ItemID: "web_search", ItemName: "Web Search", XPSpent: 50}, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				decodeJSON(t, body, &resp)
				if resp["success"] != true {
					t.Errorf("expected success=true, got %v", resp["success"])
				}
				if resp["xp_spent"].(float64) != 50 {
					t.Errorf("expected xp_spent=50, got %v", resp["xp_spent"])
				}
			},
		},
		{
			name: "insufficient XP",
			body: map[string]string{"agent_id": "a1", "item_id": "web_search"},
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				return &agentES, nil
			},
			getItemFn: func(_ string) (*agentstore.StoreItem, bool) {
				return &tool, true
			},
			purchaseFn: func(_ context.Context, _ domain.AgentID, _ string, _ int64, _ int, _ []domain.GuildID) (*agentstore.OwnedItem, error) {
				return nil, errors.New("insufficient XP")
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				decodeJSON(t, body, &resp)
				if resp["success"] != false {
					t.Errorf("expected success=false, got %v", resp["success"])
				}
			},
		},
		{
			name: "item not found",
			body: map[string]string{"agent_id": "a1", "item_id": "nonexistent"},
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				return &agentES, nil
			},
			getItemFn: func(_ string) (*agentstore.StoreItem, bool) {
				return nil, false
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "missing fields",
			body:       map[string]string{"agent_id": ""},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "agent not found",
			body: map[string]string{"agent_id": "missing", "item_id": "web_search"},
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid item_id format",
			body:       map[string]string{"agent_id": "a1", "item_id": "bad.id"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "tier gate blocks purchase",
			body: map[string]string{"agent_id": "a1", "item_id": "deploy_access"},
			getAgentFn: func(_ context.Context, _ domain.AgentID) (*graph.EntityState, error) {
				return &agentES, nil // agent is TierApprentice
			},
			getItemFn: func(_ string) (*agentstore.StoreItem, bool) {
				expertItem := agentstore.StoreItem{
					ID:      "deploy_access",
					MinTier: domain.TierExpert,
					InStock: true,
				}
				return &expertItem, true
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{
				getItemFn:  tc.getItemFn,
				purchaseFn: tc.purchaseFn,
				getInventoryFn: func(id domain.AgentID) *agentstore.AgentInventory {
					return agentstore.NewAgentInventory(id)
				},
			}
			mg := &mockGraph{getAgentFn: tc.getAgentFn}
			svc := newTestServiceWithStore(mg, &mockWorld{}, store)

			bodyJSON, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/store/purchase", bytes.NewReader(bodyJSON))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			svc.handlePurchase(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

func TestHandleGetInventory(t *testing.T) {
	tests := []struct {
		name       string
		pathID     string
		invFn      func(domain.AgentID) *agentstore.AgentInventory
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:   "returns inventory",
			pathID: "agent1",
			invFn: func(id domain.AgentID) *agentstore.AgentInventory {
				inv := agentstore.NewAgentInventory(id)
				inv.OwnedTools["web_search"] = agentstore.OwnedItem{ItemID: "web_search"}
				return inv
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var inv agentstore.AgentInventory
				decodeJSON(t, body, &inv)
				if _, ok := inv.OwnedTools["web_search"]; !ok {
					t.Error("expected web_search in owned tools")
				}
			},
		},
		{
			name:   "empty inventory for unknown agent",
			pathID: "unknown",
			invFn: func(id domain.AgentID) *agentstore.AgentInventory {
				return agentstore.NewAgentInventory(id)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid id",
			pathID:     "bad.id",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{getInventoryFn: tc.invFn}
			svc := newTestServiceWithStore(&mockGraph{}, &mockWorld{}, store)

			mux := http.NewServeMux()
			mux.HandleFunc("GET /agents/{id}/inventory", svc.handleGetInventory)

			req := httptest.NewRequest(http.MethodGet, "/agents/"+tc.pathID+"/inventory", nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

func TestHandleUseConsumable(t *testing.T) {
	tests := []struct {
		name       string
		pathID     string
		body       map[string]string
		useFn      func(context.Context, domain.AgentID, string, *domain.QuestID) error
		effectsFn  func(domain.AgentID) []agentstore.ActiveEffect
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:   "success",
			pathID: "agent1",
			body:   map[string]string{"consumable_id": "xp_boost"},
			useFn: func(_ context.Context, _ domain.AgentID, _ string, _ *domain.QuestID) error {
				return nil
			},
			effectsFn: func(_ domain.AgentID) []agentstore.ActiveEffect {
				return []agentstore.ActiveEffect{{ConsumableID: "xp_boost"}}
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				decodeJSON(t, body, &resp)
				if resp["success"] != true {
					t.Errorf("expected success=true, got %v", resp["success"])
				}
			},
		},
		{
			name:   "success with quest_id",
			pathID: "agent1",
			body:   map[string]string{"consumable_id": "xp_boost", "quest_id": "q1"},
			useFn: func(_ context.Context, _ domain.AgentID, _ string, questID *domain.QuestID) error {
				if questID == nil || string(*questID) != "q1" {
					return errors.New("expected quest_id to be q1")
				}
				return nil
			},
			effectsFn: func(_ domain.AgentID) []agentstore.ActiveEffect {
				return []agentstore.ActiveEffect{{ConsumableID: "xp_boost"}}
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				decodeJSON(t, body, &resp)
				if resp["success"] != true {
					t.Errorf("expected success=true, got %v", resp["success"])
				}
			},
		},
		{
			name:   "consumable not owned",
			pathID: "agent1",
			body:   map[string]string{"consumable_id": "xp_boost"},
			useFn: func(_ context.Context, _ domain.AgentID, _ string, _ *domain.QuestID) error {
				return errors.New("consumable not owned")
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				decodeJSON(t, body, &resp)
				if resp["success"] != false {
					t.Errorf("expected success=false, got %v", resp["success"])
				}
			},
		},
		{
			name:       "missing consumable_id",
			pathID:     "agent1",
			body:       map[string]string{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{
				useConsumableFn:    tc.useFn,
				getActiveEffectsFn: tc.effectsFn,
				getInventoryFn: func(id domain.AgentID) *agentstore.AgentInventory {
					return agentstore.NewAgentInventory(id)
				},
			}
			svc := newTestServiceWithStore(&mockGraph{}, &mockWorld{}, store)

			mux := http.NewServeMux()
			mux.HandleFunc("POST /agents/{id}/inventory/use", svc.handleUseConsumable)

			bodyJSON, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/agents/"+tc.pathID+"/inventory/use", bytes.NewReader(bodyJSON))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

func TestHandleGetEffects(t *testing.T) {
	tests := []struct {
		name       string
		pathID     string
		effectsFn  func(domain.AgentID) []agentstore.ActiveEffect
		wantStatus int
		wantLen    int
	}{
		{
			name:   "returns effects",
			pathID: "agent1",
			effectsFn: func(_ domain.AgentID) []agentstore.ActiveEffect {
				return []agentstore.ActiveEffect{{ConsumableID: "xp_boost"}}
			},
			wantStatus: http.StatusOK,
			wantLen:    1,
		},
		{
			name:   "empty effects returns empty array",
			pathID: "agent1",
			effectsFn: func(_ domain.AgentID) []agentstore.ActiveEffect {
				return nil
			},
			wantStatus: http.StatusOK,
			wantLen:    0,
		},
		{
			name:       "invalid id",
			pathID:     "bad.id",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{getActiveEffectsFn: tc.effectsFn}
			svc := newTestServiceWithStore(&mockGraph{}, &mockWorld{}, store)

			mux := http.NewServeMux()
			mux.HandleFunc("GET /agents/{id}/effects", svc.handleGetEffects)

			req := httptest.NewRequest(http.MethodGet, "/agents/"+tc.pathID+"/effects", nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				var effects []agentstore.ActiveEffect
				decodeJSON(t, rr.Body.Bytes(), &effects)
				if len(effects) != tc.wantLen {
					t.Errorf("effects count: got %d, want %d", len(effects), tc.wantLen)
				}
			}
		})
	}
}

func TestStoreHandlers_ComponentUnavailable(t *testing.T) {
	// When store is nil, all handlers should return 503.
	svc := newTestService(&mockGraph{}, &mockWorld{})

	handlers := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		body    string
	}{
		{"listStore", http.MethodGet, "/store", svc.handleListStore, ""},
		{"purchase", http.MethodPost, "/store/purchase", svc.handlePurchase, `{"agent_id":"a","item_id":"b"}`},
	}

	for _, h := range handlers {
		t.Run(h.name, func(t *testing.T) {
			var body *bytes.Reader
			if h.body != "" {
				body = bytes.NewReader([]byte(h.body))
			} else {
				body = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(h.method, h.path, body)
			rr := httptest.NewRecorder()
			h.handler(rr, req)

			if rr.Code != http.StatusServiceUnavailable {
				t.Errorf("%s: got %d, want %d\nbody: %s", h.name, rr.Code, http.StatusServiceUnavailable, rr.Body.String())
			}
		})
	}

	// Handlers with path values need a mux
	muxHandlers := []struct {
		name    string
		pattern string
		path    string
		method  string
		handler http.HandlerFunc
		body    string
	}{
		{"getStoreItem", "GET /store/{id}", "/store/web_search", http.MethodGet, svc.handleGetStoreItem, ""},
		{"getInventory", "GET /agents/{id}/inventory", "/agents/a1/inventory", http.MethodGet, svc.handleGetInventory, ""},
		{"useConsumable", "POST /agents/{id}/inventory/use", "/agents/a1/inventory/use", http.MethodPost, svc.handleUseConsumable, `{"consumable_id":"xp_boost"}`},
		{"getEffects", "GET /agents/{id}/effects", "/agents/a1/effects", http.MethodGet, svc.handleGetEffects, ""},
	}

	for _, h := range muxHandlers {
		t.Run(h.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc(h.pattern, h.handler)

			var body *bytes.Reader
			if h.body != "" {
				body = bytes.NewReader([]byte(h.body))
			} else {
				body = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(h.method, h.path, body)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusServiceUnavailable {
				t.Errorf("%s: got %d, want %d\nbody: %s", h.name, rr.Code, http.StatusServiceUnavailable, rr.Body.String())
			}
		})
	}
}

// =============================================================================
// TRAJECTORY HANDLER TESTS
// =============================================================================

// mockTrajectoryStore simulates the trajectory KV bucket for handler tests.
type mockTrajectoryStore struct {
	data   map[string][]byte
	getErr error // non-nil overrides all Get calls
}

func (m *mockTrajectoryStore) GetTrajectory(_ context.Context, id string) ([]byte, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	data, ok := m.data[id]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}
	return data, nil
}

func TestHandleGetTrajectory(t *testing.T) {
	sampleJSON := []byte(`{"loop_id":"abc123","steps":[],"duration":42}`)

	tests := []struct {
		name       string
		pathID     string
		natsNil    bool // simulate nats == nil
		data       map[string][]byte
		getErr     error
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:       "valid ID returns raw JSON",
			pathID:     "abc123",
			data:       map[string][]byte{"abc123": sampleJSON},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				if !bytes.Equal(body, sampleJSON) {
					t.Errorf("body mismatch: got %s, want %s", body, sampleJSON)
				}
			},
		},
		{
			name:       "missing trajectory returns 404",
			pathID:     "nonexistent",
			data:       map[string][]byte{},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "empty ID returns 404 from mux",
			pathID:     "",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "hex ID with dots works (loopIDs can contain dots)",
			pathID:     "abcdef0123456789",
			data:       map[string][]byte{"abcdef0123456789": sampleJSON},
			wantStatus: http.StatusOK,
		},
		{
			name:       "nil nats returns 503",
			pathID:     "abc123",
			natsNil:    true,
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "bucket not found returns 503",
			pathID:     "abc123",
			getErr:     jetstream.ErrBucketNotFound,
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "server error returns 500",
			pathID:     "abc123",
			getErr:     errors.New("nats connection lost"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(&mockGraph{}, &mockWorld{})

			if !tc.natsNil {
				svc.trajectories = &mockTrajectoryStore{data: tc.data, getErr: tc.getErr}
			}

			mux := http.NewServeMux()
			mux.HandleFunc("GET /trajectories/{id}", svc.handleGetTrajectory)

			req := httptest.NewRequest(http.MethodGet, "/trajectories/"+tc.pathID, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

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
// DM SESSION HANDLER TESTS
// =============================================================================

// mockDMSessionReader simulates the DM session KV store for handler tests.
type mockDMSessionReader struct {
	sessions map[string]*DMChatSession
	getErr   error
}

func (m *mockDMSessionReader) GetSession(_ context.Context, sessionID string) (*DMChatSession, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, nil
	}
	return session, nil
}

func TestHandleGetDMSession(t *testing.T) {
	sampleSession := &DMChatSession{
		SessionID: "abcdef0123456789abcdef0123456789",
		Turns: []DMChatTurn{
			{UserMessage: "hello", DMResponse: "greetings"},
		},
	}

	tests := []struct {
		name       string
		pathID     string
		sessions   map[string]*DMChatSession
		getErr     error
		nilStore   bool
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:   "valid session returns 200",
			pathID: "abcdef0123456789abcdef0123456789",
			sessions: map[string]*DMChatSession{
				"abcdef0123456789abcdef0123456789": sampleSession,
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var s DMChatSession
				decodeJSON(t, body, &s)
				if s.SessionID != sampleSession.SessionID {
					t.Errorf("session_id: got %q, want %q", s.SessionID, sampleSession.SessionID)
				}
				if len(s.Turns) != 1 {
					t.Errorf("turns count: got %d, want 1", len(s.Turns))
				}
			},
		},
		{
			name:       "missing session returns 404",
			pathID:     "abcdef0123456789abcdef0123456789",
			sessions:   map[string]*DMChatSession{},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid session ID returns 400",
			pathID:     "not-hex!",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty session ID returns 404 from mux",
			pathID:     "",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "nil store returns 503",
			pathID:     "abcdef0123456789abcdef0123456789",
			nilStore:   true,
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "store error returns 500",
			pathID:     "abcdef0123456789abcdef0123456789",
			sessions:   map[string]*DMChatSession{},
			getErr:     errors.New("nats error"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(&mockGraph{}, &mockWorld{})
			if !tc.nilStore {
				svc.dmSessionReader = &mockDMSessionReader{sessions: tc.sessions, getErr: tc.getErr}
			}

			mux := http.NewServeMux()
			mux.HandleFunc("GET /dm/sessions/{id}", svc.handleGetDMSession)

			req := httptest.NewRequest(http.MethodGet, "/dm/sessions/"+tc.pathID, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

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
// PEER REVIEW HANDLER TESTS
// =============================================================================

// makePeerReviewEntityState builds an EntityState whose Triples will reconstruct
// to the supplied PeerReview via PeerReviewFromEntityState.
func makePeerReviewEntityState(pr *domain.PeerReview) graph.EntityState {
	return graph.EntityState{
		ID:      string(pr.ID),
		Triples: pr.Triples(),
	}
}

// samplePeerReview returns a minimal PeerReview suitable for use in tests.
func samplePeerReview() *domain.PeerReview {
	return &domain.PeerReview{
		ID:        domain.PeerReviewID("test.dev.game.board1.peerreview.r1"),
		Status:    domain.PeerReviewPending,
		QuestID:   domain.QuestID("test.dev.game.board1.quest.q1"),
		LeaderID:  domain.AgentID("test.dev.game.board1.agent.leader1"),
		MemberID:  domain.AgentID("test.dev.game.board1.agent.member1"),
		CreatedAt: time.Now(),
	}
}

func TestHandleCreateReview(t *testing.T) {
	boardCfg := &domain.BoardConfig{Org: "test", Platform: "dev", Board: "board1"}

	tests := []struct {
		name         string
		body         any
		emitEntityFn func(context.Context, graph.Graphable, string) error
		wantStatus   int
		checkBody    func(t *testing.T, body []byte)
	}{
		{
			name: "success creates review with 201",
			body: map[string]any{
				"quest_id":  "q1",
				"leader_id": "leader1",
				"member_id": "member1",
			},
			wantStatus: http.StatusCreated,
			checkBody: func(t *testing.T, body []byte) {
				var review domain.PeerReview
				decodeJSON(t, body, &review)
				if review.Status != domain.PeerReviewPending {
					t.Errorf("status: got %q, want pending", review.Status)
				}
				if string(review.QuestID) != "q1" {
					t.Errorf("quest_id: got %q, want q1", review.QuestID)
				}
			},
		},
		{
			name: "solo task creates review with 201",
			body: map[string]any{
				"quest_id":     "q1",
				"leader_id":    "leader1",
				"member_id":    "member1",
				"is_solo_task": true,
			},
			wantStatus: http.StatusCreated,
			checkBody: func(t *testing.T, body []byte) {
				var review domain.PeerReview
				decodeJSON(t, body, &review)
				if !review.IsSoloTask {
					t.Error("expected is_solo_task=true")
				}
			},
		},
		{
			name:       "missing quest_id returns 400",
			body:       map[string]any{"leader_id": "l1", "member_id": "m1"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing leader_id returns 400",
			body:       map[string]any{"quest_id": "q1", "member_id": "m1"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing member_id returns 400",
			body:       map[string]any{"quest_id": "q1", "leader_id": "l1"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "self-review (leader==member, non-solo) returns 400",
			body: map[string]any{
				"quest_id":  "q1",
				"leader_id": "same-agent",
				"member_id": "same-agent",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "self-review allowed for solo task",
			body: map[string]any{
				"quest_id":     "q1",
				"leader_id":    "same-agent",
				"member_id":    "same-agent",
				"is_solo_task": true,
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid JSON returns 400",
			body:       "not json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "emit error returns 500",
			body: map[string]any{
				"quest_id":  "q1",
				"leader_id": "leader1",
				"member_id": "member1",
			},
			emitEntityFn: func(_ context.Context, _ graph.Graphable, _ string) error {
				return errors.New("nats write failed")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &mockGraph{
				configFn:     func() *domain.BoardConfig { return boardCfg },
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

			req := httptest.NewRequest(http.MethodPost, "/reviews", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			svc.handleCreateReview(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

func TestHandleSubmitReview(t *testing.T) {
	pendingReview := samplePeerReview()
	pendingES := makePeerReviewEntityState(pendingReview)

	// Partial review — leader has already submitted
	partialReview := samplePeerReview()
	partialReview.Status = domain.PeerReviewPartial
	partialReview.LeaderReview = &domain.ReviewSubmission{
		ReviewerID:  partialReview.LeaderID,
		RevieweeID:  partialReview.MemberID,
		Direction:   domain.ReviewDirectionLeaderToMember,
		Ratings:     domain.ReviewRatings{Q1: 4, Q2: 5, Q3: 4},
		SubmittedAt: time.Now(),
	}
	partialES := makePeerReviewEntityState(partialReview)

	// Solo pending review
	soloReview := samplePeerReview()
	soloReview.IsSoloTask = true
	soloES := makePeerReviewEntityState(soloReview)

	tests := []struct {
		name               string
		pathID             string
		body               any
		getPeerReviewFn    func(context.Context, domain.PeerReviewID) (*graph.EntityState, error)
		emitEntityUpdateFn func(context.Context, graph.Graphable, string) error
		wantStatus         int
		checkBody          func(t *testing.T, body []byte)
	}{
		{
			name:   "leader submits first — status partial",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": string(pendingReview.LeaderID),
				"ratings":     map[string]any{"q1": 4, "q2": 5, "q3": 3},
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &pendingES, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var review domain.PeerReview
				decodeJSON(t, body, &review)
				if review.Status != domain.PeerReviewPartial {
					t.Errorf("status: got %q, want partial", review.Status)
				}
				if review.LeaderReview == nil {
					t.Error("expected leader_review to be set")
				}
				// Blind enforcement: member review should be masked
				if review.MemberReview != nil {
					t.Error("expected member_review to be nil (blind enforcement)")
				}
			},
		},
		{
			name:   "member submits second — status completed",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": string(partialReview.MemberID),
				"ratings":     map[string]any{"q1": 3, "q2": 4, "q3": 5},
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &partialES, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var review domain.PeerReview
				decodeJSON(t, body, &review)
				if review.Status != domain.PeerReviewCompleted {
					t.Errorf("status: got %q, want completed", review.Status)
				}
				if review.LeaderReview == nil {
					t.Error("expected leader_review to be visible after completion")
				}
				if review.MemberReview == nil {
					t.Error("expected member_review to be visible after completion")
				}
				if review.CompletedAt == nil {
					t.Error("expected completed_at to be set")
				}
				// Check averages
				if review.MemberAvgRating == 0 {
					t.Error("expected member_avg_rating to be set")
				}
				if review.LeaderAvgRating == 0 {
					t.Error("expected leader_avg_rating to be set")
				}
			},
		},
		{
			name:   "solo task — leader submits and completes immediately",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": string(soloReview.LeaderID),
				"ratings":     map[string]any{"q1": 5, "q2": 5, "q3": 5},
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &soloES, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var review domain.PeerReview
				decodeJSON(t, body, &review)
				if review.Status != domain.PeerReviewCompleted {
					t.Errorf("status: got %q, want completed", review.Status)
				}
			},
		},
		{
			name:   "solo task — member cannot submit",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": string(soloReview.MemberID),
				"ratings":     map[string]any{"q1": 3, "q2": 3, "q3": 3},
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &soloES, nil
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "rating out of range (q1=0) returns 400",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": string(pendingReview.LeaderID),
				"ratings":     map[string]any{"q1": 0, "q2": 5, "q3": 5},
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &pendingES, nil
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "rating out of range (q3=6) returns 400",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": string(pendingReview.LeaderID),
				"ratings":     map[string]any{"q1": 3, "q2": 3, "q3": 6},
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &pendingES, nil
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "low avg without explanation returns 400",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": string(pendingReview.LeaderID),
				"ratings":     map[string]any{"q1": 1, "q2": 2, "q3": 1},
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &pendingES, nil
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "low avg with explanation returns 200",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": string(pendingReview.LeaderID),
				"ratings":     map[string]any{"q1": 1, "q2": 2, "q3": 1},
				"explanation": "Agent was unresponsive and missed deadlines",
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &pendingES, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "duplicate leader submission returns 409",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": string(partialReview.LeaderID),
				"ratings":     map[string]any{"q1": 3, "q2": 3, "q3": 3},
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &partialES, nil
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:   "unauthorized reviewer returns 403",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": "some-other-agent",
				"ratings":     map[string]any{"q1": 3, "q2": 3, "q3": 3},
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &pendingES, nil
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:   "missing reviewer_id returns 400",
			pathID: "r1",
			body:   map[string]any{"ratings": map[string]any{"q1": 3, "q2": 3, "q3": 3}},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &pendingES, nil
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "review not found returns 404",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": "any",
				"ratings":     map[string]any{"q1": 3, "q2": 3, "q3": 3},
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "invalid path ID returns 400",
			pathID: "bad.id",
			body: map[string]any{
				"reviewer_id": "any",
				"ratings":     map[string]any{"q1": 3, "q2": 3, "q3": 3},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "member submits first — status partial",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": string(pendingReview.MemberID),
				"ratings":     map[string]any{"q1": 5, "q2": 4, "q3": 3},
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &pendingES, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var review domain.PeerReview
				decodeJSON(t, body, &review)
				if review.Status != domain.PeerReviewPartial {
					t.Errorf("status: got %q, want partial", review.Status)
				}
				if review.MemberReview == nil {
					t.Error("expected member_review to be set")
				}
				// Blind: leader review masked
				if review.LeaderReview != nil {
					t.Error("expected leader_review to be nil (blind enforcement)")
				}
			},
		},
		{
			name:   "submit to completed review returns 409",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": string(partialReview.LeaderID),
				"ratings":     map[string]any{"q1": 5, "q2": 5, "q3": 5},
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				// Build a completed review
				completedReview := samplePeerReview()
				completedReview.Status = domain.PeerReviewCompleted
				completedReview.LeaderReview = &domain.ReviewSubmission{
					ReviewerID: completedReview.LeaderID,
					Ratings:    domain.ReviewRatings{Q1: 3, Q2: 3, Q3: 3},
				}
				completedReview.MemberReview = &domain.ReviewSubmission{
					ReviewerID: completedReview.MemberID,
					Ratings:    domain.ReviewRatings{Q1: 3, Q2: 3, Q3: 3},
				}
				es := makePeerReviewEntityState(completedReview)
				return &es, nil
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:   "emit update error returns 500",
			pathID: "r1",
			body: map[string]any{
				"reviewer_id": string(pendingReview.LeaderID),
				"ratings":     map[string]any{"q1": 4, "q2": 4, "q3": 4},
			},
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &pendingES, nil
			},
			emitEntityUpdateFn: func(_ context.Context, _ graph.Graphable, _ string) error {
				return errors.New("nats write failed")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &mockGraph{
				getPeerReviewFn:    tc.getPeerReviewFn,
				emitEntityUpdateFn: tc.emitEntityUpdateFn,
			}
			svc := newTestService(g, &mockWorld{})

			mux := http.NewServeMux()
			mux.HandleFunc("POST /reviews/{id}/submit", svc.handleSubmitReview)

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

			req := httptest.NewRequest(http.MethodPost, "/reviews/"+tc.pathID+"/submit", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

func TestHandleGetReview(t *testing.T) {
	review := samplePeerReview()
	es := makePeerReviewEntityState(review)

	// Partial review with leader submission — GET should strip it
	partialWithSubmission := samplePeerReview()
	partialWithSubmission.Status = domain.PeerReviewPartial
	partialWithSubmission.LeaderReview = &domain.ReviewSubmission{
		ReviewerID:  partialWithSubmission.LeaderID,
		RevieweeID:  partialWithSubmission.MemberID,
		Direction:   domain.ReviewDirectionLeaderToMember,
		Ratings:     domain.ReviewRatings{Q1: 4, Q2: 5, Q3: 3},
		SubmittedAt: time.Now(),
	}
	partialES := makePeerReviewEntityState(partialWithSubmission)

	tests := []struct {
		name            string
		pathID          string
		getPeerReviewFn func(context.Context, domain.PeerReviewID) (*graph.EntityState, error)
		wantStatus      int
		checkBody       func(t *testing.T, body []byte)
	}{
		{
			name:   "success returns 200",
			pathID: "r1",
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &es, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "partial review strips submissions (blind enforcement)",
			pathID: "r1",
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return &partialES, nil
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var review domain.PeerReview
				decodeJSON(t, body, &review)
				if review.LeaderReview != nil {
					t.Error("expected leader_review to be stripped from GET on partial review")
				}
				if review.MemberReview != nil {
					t.Error("expected member_review to be stripped from GET on partial review")
				}
			},
		},
		{
			name:   "key not found returns 404",
			pathID: "r1",
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "bucket not found returns 404",
			pathID: "r1",
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return nil, jetstream.ErrBucketNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "invalid id returns 400",
			pathID: "bad.id",
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return nil, errors.New("should not be called")
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "server error returns 500",
			pathID: "r1",
			getPeerReviewFn: func(_ context.Context, _ domain.PeerReviewID) (*graph.EntityState, error) {
				return nil, errors.New("nats error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(&mockGraph{getPeerReviewFn: tc.getPeerReviewFn}, &mockWorld{})

			mux := http.NewServeMux()
			mux.HandleFunc("GET /reviews/{id}", svc.handleGetReview)

			req := httptest.NewRequest(http.MethodGet, "/reviews/"+tc.pathID, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rr.Code, tc.wantStatus)
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

func TestHandleListReviews(t *testing.T) {
	pending := samplePeerReview()
	completed := samplePeerReview()
	completed.ID = domain.PeerReviewID("test.dev.game.board1.peerreview.r2")
	completed.Status = domain.PeerReviewCompleted
	completed.QuestID = domain.QuestID("test.dev.game.board1.quest.q2")

	entities := []graph.EntityState{
		makePeerReviewEntityState(pending),
		makePeerReviewEntityState(completed),
	}

	tests := []struct {
		name              string
		query             string
		listPeerReviewsFn func(context.Context, int) ([]graph.EntityState, error)
		wantStatus        int
		wantCount         int
	}{
		{
			name: "returns all reviews",
			listPeerReviewsFn: func(_ context.Context, _ int) ([]graph.EntityState, error) {
				return entities, nil
			},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:  "filter by status",
			query: "?status=completed",
			listPeerReviewsFn: func(_ context.Context, _ int) ([]graph.EntityState, error) {
				return entities, nil
			},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:  "filter by quest_id",
			query: "?quest_id=" + string(completed.QuestID),
			listPeerReviewsFn: func(_ context.Context, _ int) ([]graph.EntityState, error) {
				return entities, nil
			},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name: "empty result returns empty array",
			listPeerReviewsFn: func(_ context.Context, _ int) ([]graph.EntityState, error) {
				return nil, nil
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "bucket not found returns empty array",
			listPeerReviewsFn: func(_ context.Context, _ int) ([]graph.EntityState, error) {
				return nil, jetstream.ErrBucketNotFound
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "server error returns 500",
			listPeerReviewsFn: func(_ context.Context, _ int) ([]graph.EntityState, error) {
				return nil, errors.New("nats error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(&mockGraph{listPeerReviewsFn: tc.listPeerReviewsFn}, &mockWorld{})

			req := httptest.NewRequest(http.MethodGet, "/reviews"+tc.query, nil)
			rr := httptest.NewRecorder()
			svc.handleListReviews(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}

			if tc.wantStatus == http.StatusOK {
				var reviews []domain.PeerReview
				decodeJSON(t, rr.Body.Bytes(), &reviews)
				if len(reviews) != tc.wantCount {
					t.Errorf("count: got %d, want %d", len(reviews), tc.wantCount)
				}
			}
		})
	}
}

func TestHandleListAgentReviews(t *testing.T) {
	// Use short IDs that match what r.PathValue("id") returns,
	// since the handler constructs AgentID from the raw path value.
	leaderID := domain.AgentID("leader1")
	memberID := domain.AgentID("member1")
	otherID := domain.AgentID("other1")

	r1 := &domain.PeerReview{
		ID:        domain.PeerReviewID("test.dev.game.board1.peerreview.r1"),
		Status:    domain.PeerReviewPending,
		QuestID:   domain.QuestID("test.dev.game.board1.quest.q1"),
		LeaderID:  leaderID,
		MemberID:  memberID,
		CreatedAt: time.Now(),
	}
	r2 := &domain.PeerReview{
		ID:        domain.PeerReviewID("test.dev.game.board1.peerreview.r2"),
		Status:    domain.PeerReviewPending,
		QuestID:   domain.QuestID("test.dev.game.board1.quest.q2"),
		LeaderID:  otherID,
		MemberID:  otherID,
		CreatedAt: time.Now(),
	}

	entities := []graph.EntityState{
		makePeerReviewEntityState(r1),
		makePeerReviewEntityState(r2),
	}

	tests := []struct {
		name              string
		pathID            string
		listPeerReviewsFn func(context.Context, int) ([]graph.EntityState, error)
		wantStatus        int
		wantCount         int
	}{
		{
			name:   "returns reviews where agent is leader",
			pathID: "leader1",
			listPeerReviewsFn: func(_ context.Context, _ int) ([]graph.EntityState, error) {
				return entities, nil
			},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:   "returns reviews where agent is member",
			pathID: "member1",
			listPeerReviewsFn: func(_ context.Context, _ int) ([]graph.EntityState, error) {
				return entities, nil
			},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:   "agent with no reviews returns empty array",
			pathID: "nobody",
			listPeerReviewsFn: func(_ context.Context, _ int) ([]graph.EntityState, error) {
				return entities, nil
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name:       "invalid id returns 400",
			pathID:     "bad.id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "bucket not found returns empty array",
			pathID: "leader1",
			listPeerReviewsFn: func(_ context.Context, _ int) ([]graph.EntityState, error) {
				return nil, jetstream.ErrBucketNotFound
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(&mockGraph{listPeerReviewsFn: tc.listPeerReviewsFn}, &mockWorld{})

			mux := http.NewServeMux()
			mux.HandleFunc("GET /agents/{id}/reviews", svc.handleListAgentReviews)

			req := httptest.NewRequest(http.MethodGet, "/agents/"+tc.pathID+"/reviews", nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}

			if tc.wantStatus == http.StatusOK {
				var reviews []domain.PeerReview
				decodeJSON(t, rr.Body.Bytes(), &reviews)
				if len(reviews) != tc.wantCount {
					t.Errorf("count: got %d, want %d", len(reviews), tc.wantCount)
				}
			}
		})
	}
}

// =============================================================================
// COMPILE-TIME INTERFACE SATISFACTION CHECKS
// =============================================================================

// These blank-identifier assignments verify at compile time that the concrete
// types satisfy the interfaces, so a method signature drift is caught as a
// build error rather than a runtime panic.
var (
	_ GraphQuerier       = (*semdragons.GraphClient)(nil)
	_ GraphQuerier       = (*mockGraph)(nil)
	_ WorldStateProvider = (*mockWorld)(nil)
	_ StoreProvider      = (*agentstore.Component)(nil)
	_ StoreProvider      = (*mockStore)(nil)
	_ TrajectoryQuerier  = (*mockTrajectoryStore)(nil)
	_ DMSessionReader    = (*mockDMSessionReader)(nil)
	_ DMSessionReader    = (*dmSessionStore)(nil)
)

// messageTripleUsed ensures the graph/message import is referenced so the
// compiler doesn't complain. EntityState.Triples holds []message.Triple.
var _ = message.Triple{}

// =============================================================================
// BOARD CONTROL HANDLER TESTS
// =============================================================================

// mockKeyValue implements jetstream.KeyValue with function fields so individual
// test cases can supply exactly the behavior they need. All methods not set
// return sensible zero-value defaults so the mock satisfies the interface
// without requiring every test to stub every method.
type mockKeyValue struct {
	getFn    func(ctx context.Context, key string) (jetstream.KeyValueEntry, error)
	putFn    func(ctx context.Context, key string, value []byte) (uint64, error)
	watchFn  func(ctx context.Context, keys string, opts ...jetstream.WatchOpt) (jetstream.KeyWatcher, error)
	bucketFn func() string
}

func (m *mockKeyValue) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if m.getFn != nil {
		return m.getFn(ctx, key)
	}
	return nil, jetstream.ErrKeyNotFound
}

func (m *mockKeyValue) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	if m.putFn != nil {
		return m.putFn(ctx, key, value)
	}
	return 1, nil
}

func (m *mockKeyValue) Watch(ctx context.Context, keys string, opts ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	if m.watchFn != nil {
		return m.watchFn(ctx, keys, opts...)
	}
	return &mockKeyWatcher{updates: make(chan jetstream.KeyValueEntry)}, nil
}

// Remaining methods are required by the jetstream.KeyValue interface but are
// not exercised by boardcontrol.Controller in these unit tests.

func (m *mockKeyValue) GetRevision(_ context.Context, _ string, _ uint64) (jetstream.KeyValueEntry, error) {
	return nil, jetstream.ErrKeyNotFound
}

func (m *mockKeyValue) PutString(_ context.Context, _ string, _ string) (uint64, error) {
	return 0, nil
}

func (m *mockKeyValue) Create(_ context.Context, _ string, _ []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	return 0, nil
}

func (m *mockKeyValue) Update(_ context.Context, _ string, _ []byte, _ uint64) (uint64, error) {
	return 0, nil
}

func (m *mockKeyValue) Delete(_ context.Context, _ string, _ ...jetstream.KVDeleteOpt) error {
	return nil
}

func (m *mockKeyValue) Purge(_ context.Context, _ string, _ ...jetstream.KVDeleteOpt) error {
	return nil
}

func (m *mockKeyValue) WatchAll(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return &mockKeyWatcher{updates: make(chan jetstream.KeyValueEntry)}, nil
}

func (m *mockKeyValue) WatchFiltered(_ context.Context, _ []string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return &mockKeyWatcher{updates: make(chan jetstream.KeyValueEntry)}, nil
}

func (m *mockKeyValue) Keys(_ context.Context, _ ...jetstream.WatchOpt) ([]string, error) {
	return nil, nil
}

func (m *mockKeyValue) ListKeys(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	return nil, nil
}

func (m *mockKeyValue) ListKeysFiltered(_ context.Context, _ ...string) (jetstream.KeyLister, error) {
	return nil, nil
}

func (m *mockKeyValue) History(_ context.Context, _ string, _ ...jetstream.WatchOpt) ([]jetstream.KeyValueEntry, error) {
	return nil, nil
}

func (m *mockKeyValue) Bucket() string {
	if m.bucketFn != nil {
		return m.bucketFn()
	}
	return "BOARD_CONTROL"
}

func (m *mockKeyValue) PurgeDeletes(_ context.Context, _ ...jetstream.KVPurgeOpt) error {
	return nil
}

func (m *mockKeyValue) Status(_ context.Context) (jetstream.KeyValueStatus, error) {
	return nil, nil
}

// mockKeyWatcher implements jetstream.KeyWatcher for tests that do not need
// real KV watch behaviour (e.g. tests that skip Controller.Start).
type mockKeyWatcher struct {
	updates chan jetstream.KeyValueEntry
}

func (w *mockKeyWatcher) Updates() <-chan jetstream.KeyValueEntry {
	return w.updates
}

func (w *mockKeyWatcher) Stop() error {
	return nil
}

// newTestServiceWithBoard creates a Service whose board field is set to the
// supplied Controller. Use this for handler tests that exercise the
// board-available code paths.
func newTestServiceWithBoard(ctrl *boardcontrol.Controller) *Service {
	return &Service{
		graph:  &mockGraph{},
		world:  &mockWorld{},
		config: Config{Board: "board1", MaxEntities: 100},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		board:  ctrl,
	}
}

// newMockBucket returns a mockKeyValue where Put always succeeds and Get
// returns ErrKeyNotFound (board starts in the running/un-paused state).
func newMockBucket() *mockKeyValue {
	return &mockKeyValue{}
}

// newTestController creates a boardcontrol.Controller backed by the supplied
// mock bucket. It does NOT call Start so no watcher goroutine is launched,
// which keeps tests synchronous and avoids teardown complexity.
//
// Because Start is skipped, callers must drive state changes through the
// Controller's public methods (Pause, Resume) rather than relying on the
// watcher to propagate KV changes.
func newTestController(bucket jetstream.KeyValue) *boardcontrol.Controller {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// nats is nil here — callers must not exercise code paths that call
	// c.nats.Publish (i.e. Controller.Resume). Those paths are covered by
	// integration tests.
	return boardcontrol.NewController(bucket, nil, logger)
}

// TestHandleBoardStatus verifies the GET /board/status handler under
// different board availability scenarios.
func TestHandleBoardStatus(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *Service
		wantPaused bool
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name: "board_nil_returns_default_running_state",
			setup: func() *Service {
				// newTestService leaves board as nil.
				return newTestService(&mockGraph{}, &mockWorld{})
			},
			wantPaused: false,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				decodeJSON(t, body, &resp)
				paused, _ := resp["paused"].(bool)
				if paused {
					t.Error("expected paused=false when board is nil")
				}
				// paused_at and paused_by should be null/absent.
				if resp["paused_at"] != nil {
					t.Errorf("expected paused_at=null, got %v", resp["paused_at"])
				}
				if resp["paused_by"] != nil {
					t.Errorf("expected paused_by=null, got %v", resp["paused_by"])
				}
			},
		},
		{
			name: "board_paused_returns_paused_state",
			setup: func() *Service {
				ctrl := newTestController(newMockBucket())
				// Drive the controller into a paused state without going
				// through the network: Pause writes to the mock KV bucket
				// and updates the atomic fields immediately.
				_, err := ctrl.Pause(context.Background(), "dm")
				if err != nil {
					// If Pause fails the test cannot proceed; surface the error.
					t.Fatalf("ctrl.Pause: %v", err)
				}
				return newTestServiceWithBoard(ctrl)
			},
			wantPaused: true,
			checkBody: func(t *testing.T, body []byte) {
				var resp boardcontrol.BoardState
				decodeJSON(t, body, &resp)
				if !resp.Paused {
					t.Error("expected paused=true")
				}
				if resp.PausedAt == nil {
					t.Error("expected paused_at to be non-null after Pause")
				}
				if resp.PausedBy == nil || *resp.PausedBy != "dm" {
					t.Errorf("expected paused_by=dm, got %v", resp.PausedBy)
				}
			},
		},
		{
			name: "board_running_returns_running_state",
			setup: func() *Service {
				ctrl := newTestController(newMockBucket())
				// Controller starts in the running state (no Pause call).
				return newTestServiceWithBoard(ctrl)
			},
			wantPaused: false,
			checkBody: func(t *testing.T, body []byte) {
				var resp boardcontrol.BoardState
				decodeJSON(t, body, &resp)
				if resp.Paused {
					t.Error("expected paused=false for a freshly created controller")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := tc.setup()
			req := httptest.NewRequest(http.MethodGet, "/board/status", nil)
			rr := httptest.NewRecorder()
			svc.handleBoardStatus(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200\nbody: %s", rr.Code, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

// TestHandleBoardPause verifies the POST /board/pause handler under different
// board availability and request body scenarios.
func TestHandleBoardPause(t *testing.T) {
	tests := []struct {
		name       string
		board      *boardcontrol.Controller // nil means board unavailable
		body       any                      // request body; nil means no body
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:       "board_nil_returns_503",
			board:      nil,
			wantStatus: http.StatusServiceUnavailable,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]string
				decodeJSON(t, body, &resp)
				if resp["error"] == "" {
					t.Error("expected error field in 503 response")
				}
			},
		},
		{
			name:       "pause_with_actor_returns_paused_state",
			board:      newTestController(newMockBucket()),
			body:       map[string]any{"actor": "dm-user"},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp boardcontrol.BoardState
				decodeJSON(t, body, &resp)
				if !resp.Paused {
					t.Error("expected paused=true in response")
				}
				if resp.PausedAt == nil {
					t.Error("expected paused_at to be set")
				}
				if resp.PausedBy == nil || *resp.PausedBy != "dm-user" {
					t.Errorf("expected paused_by=dm-user, got %v", resp.PausedBy)
				}
			},
		},
		{
			name:       "pause_without_body_uses_empty_actor",
			board:      newTestController(newMockBucket()),
			body:       nil, // no body at all
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp boardcontrol.BoardState
				decodeJSON(t, body, &resp)
				if !resp.Paused {
					t.Error("expected paused=true even with no body")
				}
				// Empty actor string → PausedBy should remain nil.
				if resp.PausedBy != nil {
					t.Errorf("expected paused_by=null for empty actor, got %v", resp.PausedBy)
				}
			},
		},
		{
			name:       "pause_with_empty_json_body_uses_empty_actor",
			board:      newTestController(newMockBucket()),
			body:       map[string]any{}, // body present but actor omitted
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp boardcontrol.BoardState
				decodeJSON(t, body, &resp)
				if !resp.Paused {
					t.Error("expected paused=true with empty body")
				}
				if resp.PausedBy != nil {
					t.Errorf("expected paused_by=null, got %v", resp.PausedBy)
				}
			},
		},
		{
			name: "pause_returns_500_when_bucket_put_fails",
			board: newTestController(&mockKeyValue{
				putFn: func(_ context.Context, _ string, _ []byte) (uint64, error) {
					return 0, errors.New("nats: connection closed")
				},
			}),
			body:       map[string]any{"actor": "dm"},
			wantStatus: http.StatusInternalServerError,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]string
				decodeJSON(t, body, &resp)
				if resp["error"] == "" {
					t.Error("expected error field in 500 response")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				graph:  &mockGraph{},
				world:  &mockWorld{},
				config: Config{Board: "board1", MaxEntities: 100},
				logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
				board:  tc.board,
			}

			var reqBody io.Reader
			if tc.body != nil {
				bodyBytes, err := json.Marshal(tc.body)
				if err != nil {
					t.Fatalf("marshal request body: %v", err)
				}
				reqBody = bytes.NewReader(bodyBytes)
			}

			req := httptest.NewRequest(http.MethodPost, "/board/pause", reqBody)
			if tc.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			rr := httptest.NewRecorder()
			svc.handleBoardPause(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

// TestHandleBoardResume verifies the POST /board/resume handler under different
// board availability scenarios.
//
// Note: the board-available happy path (resume_returns_running_state) requires
// a real *natsclient.Client because boardcontrol.Controller.Resume calls
// c.nats.Publish after updating KV state. Since natsclient.Client is a
// concrete type (not an interface), it cannot be mocked without a live NATS
// connection. That path is covered by the integration test suite:
//
//	processor/boardcontrol/component_test.go (build tag: integration)
func TestHandleBoardResume(t *testing.T) {
	tests := []struct {
		name       string
		board      *boardcontrol.Controller
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:       "board_nil_returns_503",
			board:      nil,
			wantStatus: http.StatusServiceUnavailable,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]string
				decodeJSON(t, body, &resp)
				if resp["error"] == "" {
					t.Error("expected error field in 503 response")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				graph:  &mockGraph{},
				world:  &mockWorld{},
				config: Config{Board: "board1", MaxEntities: 100},
				logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
				board:  tc.board,
			}

			req := httptest.NewRequest(http.MethodPost, "/board/resume", nil)
			rr := httptest.NewRecorder()
			svc.handleBoardResume(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}
