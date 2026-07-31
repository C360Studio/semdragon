//go:build integration

package semdragons_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/gateway"
	lifecyclegateway "github.com/c360studio/semstreams/gateway/lifecycle-gateway"
	"github.com/c360studio/semstreams/graph/readiness"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	"github.com/c360studio/semstreams/pkg/ownership"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	rule "github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/types"
	"github.com/c360studio/semstreams/vocabulary"

	"github.com/c360studio/semdragons/componentregistry"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/internal/httpauth"
	"github.com/c360studio/semdragons/questlifecycle"
)

func TestLifecycleGatewayUsesSharedQuestManager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)

	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)
	manager := lifecycle.NewManager(testClient.Client, slog.Default())
	ownerRegistry, err := ownership.EnsureBuckets(
		ctx,
		testClient.Client,
		slog.Default(),
		vocabulary.InverseResolver,
	)
	if err != nil {
		t.Fatalf("ensure beta.159 ownership buckets: %v", err)
	}
	manager.AttachOwnership(ctx, ownerRegistry)
	t.Cleanup(func() {
		cancel()
		manager.WaitOwnership()
	})
	domain.RegisterVocabulary()
	if err := manager.Register(questlifecycle.WorkflowDeclaration()); err != nil {
		t.Fatalf("register quest workflow: %v", err)
	}

	ingestConfig := graphingest.DefaultConfig()
	ingestConfig.EnforceOwnerLease = true
	startPhase1Component(
		t,
		ctx,
		mustPhase1JSON(t, ingestConfig),
		func(raw json.RawMessage) (component.Discoverable, error) {
			return graphingest.CreateGraphIngest(raw, component.Dependencies{NATSClient: testClient.Client})
		},
	)
	waitForGraphClientProducer(t, ctx, testClient.Client, readiness.KeyGraphIngest)
	startPhase1Component(
		t,
		ctx,
		mustPhase1JSON(t, shippedQuestRuleConfig(t)),
		func(raw json.RawMessage) (component.Discoverable, error) {
			return rule.CreateRuleProcessor(raw, component.Dependencies{
				NATSClient:       testClient.Client,
				LifecycleManager: manager,
			})
		},
	)
	waitForGraphClientProducer(t, ctx, testClient.Client, readiness.KeyRule)

	const questID = "local.integration.game.board1.quest.gateway-contract"
	if err := manager.Create(ctx, &questlifecycle.QuestState{
		EntityIDField: questID,
		PhaseField:    questlifecycle.PhaseInProgress,
		MaxAttempts:   3,
	}); err != nil {
		t.Fatalf("create quest: %v", err)
	}

	registry := component.NewRegistry()
	if err := componentregistry.RegisterAll(registry); err != nil {
		t.Fatalf("register component factories: %v", err)
	}
	rawConfig, err := json.Marshal(lifecyclegateway.Config{
		PathPrefix:      "workflows",
		EnableWebSocket: boolPointer(false),
		MaxBodyBytes:    65536,
	})
	if err != nil {
		t.Fatalf("marshal lifecycle-gateway config: %v", err)
	}
	discoverable, err := registry.CreateComponent(
		lifecyclegateway.ComponentName,
		types.ComponentConfig{
			Type:    types.ComponentTypeGateway,
			Name:    lifecyclegateway.ComponentName,
			Enabled: true,
			Config:  rawConfig,
		},
		component.Dependencies{
			NATSClient:       testClient.Client,
			LifecycleManager: manager,
		},
	)
	if err != nil {
		t.Fatalf("create registered lifecycle gateway: %v", err)
	}
	runtimeComponent := discoverable.(component.LifecycleComponent)
	if err := runtimeComponent.Initialize(); err != nil {
		t.Fatalf("initialize lifecycle gateway: %v", err)
	}
	if err := runtimeComponent.Start(ctx); err != nil {
		t.Fatalf("start lifecycle gateway: %v", err)
	}
	t.Cleanup(func() {
		if err := runtimeComponent.Stop(5 * time.Second); err != nil {
			t.Errorf("stop lifecycle gateway: %v", err)
		}
	})

	mux := http.NewServeMux()
	discoverable.(gateway.HTTPHandler).RegisterHTTPHandlers("/lifecycle-gateway", mux)
	const apiKey = "gateway-secret"
	server := httptest.NewServer(httpauth.ProtectNamespace(
		apiKey,
		"/lifecycle-gateway/workflows",
	)(mux))
	t.Cleanup(server.Close)

	request := func(method, path string, body []byte, authenticated bool) (int, []byte) {
		t.Helper()
		req, requestErr := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(body))
		if requestErr != nil {
			t.Fatalf("create request: %v", requestErr)
		}
		if authenticated {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, requestErr := server.Client().Do(req)
		if requestErr != nil {
			t.Fatalf("%s %s: %v", method, path, requestErr)
		}
		defer resp.Body.Close()
		responseBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			t.Fatalf("read response: %v", readErr)
		}
		return resp.StatusCode, responseBody
	}

	status, _ := request(http.MethodGet, "/lifecycle-gateway/workflows", nil, false)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated workflow discovery status = %d, want %d", status, http.StatusUnauthorized)
	}

	status, body := request(http.MethodGet, "/lifecycle-gateway/workflows", nil, true)
	if status != http.StatusOK {
		t.Fatalf("workflow discovery status = %d, body=%s", status, body)
	}
	var workflows []struct {
		Workflow string `json:"workflow"`
	}
	if err := json.Unmarshal(body, &workflows); err != nil {
		t.Fatalf("decode workflows: %v", err)
	}
	if len(workflows) != 1 || workflows[0].Workflow != questlifecycle.WorkflowName {
		t.Fatalf("workflows = %#v, want only quest", workflows)
	}

	status, body = request(http.MethodGet, "/lifecycle-gateway/workflows/quest", nil, true)
	if status != http.StatusOK {
		t.Fatalf("list quest instances status = %d, body=%s", status, body)
	}
	var instances []questlifecycle.QuestState
	if err := json.Unmarshal(body, &instances); err != nil {
		t.Fatalf("decode quest instances: %v", err)
	}
	if len(instances) != 1 || instances[0].EntityIDField != questID {
		t.Fatalf("quest instances = %#v, want only %q", instances, questID)
	}

	streamPath := "/lifecycle-gateway/workflows/quest?stream=true"
	status, body = request(http.MethodGet, streamPath, nil, true)
	if status != http.StatusForbidden {
		t.Fatalf("disabled stream status = %d, want %d, body=%s", status, http.StatusForbidden, body)
	}

	instancePath := "/lifecycle-gateway/workflows/quest/" + questID
	status, body = request(http.MethodGet, instancePath+"/children", nil, true)
	if status != http.StatusOK || string(body) != "[]\n" {
		t.Fatalf("children status = %d, body=%s, want 200 []", status, body)
	}
	status, body = request(http.MethodGet, "/lifecycle-gateway/workflows/unknown", nil, true)
	if status != http.StatusNotFound {
		t.Fatalf("unknown workflow status = %d, want %d, body=%s", status, http.StatusNotFound, body)
	}
	status, body = request(
		http.MethodGet,
		"/lifecycle-gateway/workflows/quest/local.integration.game.board1.quest.missing",
		nil,
		true,
	)
	if status != http.StatusNotFound {
		t.Fatalf("unknown entity status = %d, want %d, body=%s", status, http.StatusNotFound, body)
	}

	statePath := instancePath + "/state"
	status, body = request(http.MethodPost, statePath, []byte(`{"max_attempts":4}`), false)
	if status != http.StatusUnauthorized {
		t.Fatalf(
			"unauthenticated state patch status = %d, want %d, body=%s",
			status,
			http.StatusUnauthorized,
			body,
		)
	}
	status, body = request(http.MethodPost, statePath, []byte(`{"max_attempts":4}`), true)
	if status != http.StatusOK {
		t.Fatalf("authenticated state patch status = %d, body=%s", status, body)
	}
	for _, protectedPatch := range []string{
		`{"phase":"failed"}`,
		`{"entity_id":"local.integration.game.board1.quest.rewritten"}`,
	} {
		status, body = request(http.MethodPost, statePath, []byte(protectedPatch), true)
		if status != http.StatusBadRequest {
			t.Fatalf(
				"protected state patch %s status = %d, want %d, body=%s",
				protectedPatch,
				status,
				http.StatusBadRequest,
				body,
			)
		}
	}
	// SemStreams #682 tracks the lifecycle-gateway implementation returning
	// 400 here while its OpenAPI contract documents 413 for an oversized body.
	status, body = request(http.MethodPost, statePath, bytes.Repeat([]byte(" "), 65537), true)
	if status != http.StatusBadRequest {
		t.Fatalf(
			"oversized state patch status = %d, want %d, body=%s",
			status,
			http.StatusBadRequest,
			body,
		)
	}

	transitionPath := "/lifecycle-gateway/workflows/quest/" + questID + "/transition"
	status, body = request(
		http.MethodPost,
		transitionPath,
		[]byte(`{"phase":"claimed","note":"invalid transition probe"}`),
		true,
	)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid transition status = %d, want %d, body=%s", status, http.StatusBadRequest, body)
	}
	status, body = request(
		http.MethodPost,
		transitionPath,
		[]byte(`{"phase":"in_review","note":"operator requested review"}`),
		false,
	)
	if status != http.StatusUnauthorized {
		t.Fatalf(
			"unauthenticated operator transition status = %d, want %d, body=%s",
			status,
			http.StatusUnauthorized,
			body,
		)
	}
	status, body = request(
		http.MethodPost,
		transitionPath,
		[]byte(`{"phase":"in_review","note":"operator requested review"}`),
		true,
	)
	if status != http.StatusOK {
		t.Fatalf("operator transition status = %d, body=%s", status, body)
	}

	var state questlifecycle.QuestState
	deadline := time.Now().Add(20 * time.Second)
	for {
		status, body = request(http.MethodGet, instancePath, nil, true)
		if status != http.StatusOK {
			t.Fatalf("inspect instance status = %d, body=%s", status, body)
		}
		if err := json.Unmarshal(body, &state); err != nil {
			t.Fatalf("decode quest state: %v", err)
		}
		if state.PhaseField == questlifecycle.PhaseCompleted {
			if state.Verdict != "review-skipped-mechanics-disabled" {
				t.Fatalf("observed completed state without atomic mechanics-stub verdict: %#v", state)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("quest did not complete through mechanics-stub rule; last state=%#v", state)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for rule completion: %v", ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	if state.MaxAttempts != 4 {
		t.Errorf("max_attempts = %d, want authenticated operator patch value 4", state.MaxAttempts)
	}

	status, body = request(http.MethodGet, instancePath+"/history", nil, true)
	if status != http.StatusOK {
		t.Fatalf("history status = %d, body=%s", status, body)
	}
	var history []lifecycle.TransitionEvent
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	// beta.159's framework-owned ENTITY_STATES catalog intentionally retains
	// one KV revision. SemStreams #681 tracks a framework history policy that
	// can support multi-transition reconstruction. The gateway remains truthful
	// today by returning the latest retained transition only; Semdragon must not
	// precreate the framework bucket with a divergent history value to expand it.
	if len(history) != 1 {
		t.Fatalf("history = %#v, want exactly the latest catalog-retained transition", history)
	}
	ruleEvent := history[0]
	if ruleEvent.From != questlifecycle.PhaseInReview ||
		ruleEvent.To != questlifecycle.PhaseCompleted ||
		ruleEvent.Triggered != lifecycle.TransitionSourceRule ||
		ruleEvent.Note != "review skipped (mechanics disabled)" {
		t.Errorf("rule history event = %#v", ruleEvent)
	}
}

func boolPointer(value bool) *bool {
	return &value
}
