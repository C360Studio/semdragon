package httpauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAPIKey(t *testing.T) {
	t.Parallel()

	const apiKey = "secret"
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	tests := []struct {
		name       string
		key        string
		headers    map[string]string
		wantStatus int
	}{
		{name: "dev passthrough", wantStatus: http.StatusNoContent},
		{name: "missing", key: apiKey, wantStatus: http.StatusUnauthorized},
		{name: "wrong", key: apiKey, headers: map[string]string{"X-API-Key": "wrong"}, wantStatus: http.StatusUnauthorized},
		{name: "api key", key: apiKey, headers: map[string]string{"X-API-Key": apiKey}, wantStatus: http.StatusNoContent},
		{name: "bearer", key: apiKey, headers: map[string]string{"Authorization": "Bearer " + apiKey}, wantStatus: http.StatusNoContent},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for name, value := range tt.headers {
				req.Header.Set(name, value)
			}
			rec := httptest.NewRecorder()

			RequireAPIKey(tt.key, next).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestLoadAPIKey(t *testing.T) {
	t.Setenv(APIKeyEnv, "from-environment")
	if got := LoadAPIKey(); got != "from-environment" {
		t.Fatalf("LoadAPIKey() = %q, want from-environment", got)
	}
}

func TestProtectNamespaceMatchesExactPathBoundary(t *testing.T) {
	t.Parallel()

	const (
		apiKey    = "secret"
		namespace = "/lifecycle-gateway/workflows"
	)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := ProtectNamespace(apiKey, namespace)(next)

	tests := []struct {
		path       string
		wantStatus int
	}{
		{path: namespace, wantStatus: http.StatusUnauthorized},
		{path: namespace + "/", wantStatus: http.StatusUnauthorized},
		{path: namespace + "/quest", wantStatus: http.StatusUnauthorized},
		{path: namespace + "evil", wantStatus: http.StatusNoContent},
		{path: "/game/quests", wantStatus: http.StatusNoContent},
		{path: "/openapi.json", wantStatus: http.StatusNoContent},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
