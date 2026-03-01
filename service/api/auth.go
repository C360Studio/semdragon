package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// loadAPIKey reads the SEMDRAGONS_API_KEY environment variable.
// An empty string means dev mode — auth is disabled.
func loadAPIKey() string {
	return os.Getenv("SEMDRAGONS_API_KEY")
}

// requireAuth wraps handler with API key authentication.
//
// When apiKey is empty the handler is returned unchanged — dev mode passthrough
// with zero overhead. When a key is configured, the request must supply it via
// the X-API-Key header or an Authorization: Bearer <token> header.
//
// CORS headers are applied by the outer cors() wrapper, so preflight OPTIONS
// requests never reach this middleware.
func requireAuth(apiKey string, next http.HandlerFunc) http.HandlerFunc {
	if apiKey == "" {
		// Dev mode: no auth configured, pass through directly.
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-API-Key")
		if token == "" {
			// Fall back to Authorization: Bearer <token>
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				token = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if token != apiKey {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}

		next(w, r)
	}
}
