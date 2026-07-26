// Package httpauth owns Semdragons' product-shell HTTP authentication policy.
package httpauth

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// APIKeyEnv is the environment variable used by Semdragons HTTP surfaces.
const APIKeyEnv = "SEMDRAGONS_API_KEY"

// LoadAPIKey reads the configured API key. An empty key enables development
// passthrough, preserving the existing Semdragons policy.
func LoadAPIKey() string {
	return os.Getenv(APIKeyEnv)
}

// RequireAPIKey protects next with the Semdragons API-key policy.
func RequireAPIKey(apiKey string, next http.Handler) http.Handler {
	if apiKey == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-API-Key")
		if token == "" {
			if authorization := r.Header.Get("Authorization"); strings.HasPrefix(authorization, "Bearer ") {
				token = strings.TrimPrefix(authorization, "Bearer ")
			}
		}

		if token != apiKey {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireAPIKeyFunc is the HandlerFunc adapter used by the game service.
func RequireAPIKeyFunc(apiKey string, next http.HandlerFunc) http.HandlerFunc {
	return RequireAPIKey(apiKey, next).ServeHTTP
}

// ProtectNamespace returns middleware that authenticates only the exact path
// namespace and its descendants. Similar prefixes (for example, "workflowsevil")
// remain outside the protected namespace.
func ProtectNamespace(apiKey, namespace string) func(http.Handler) http.Handler {
	namespace = "/" + strings.Trim(namespace, "/")
	return func(next http.Handler) http.Handler {
		protected := RequireAPIKey(apiKey, next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == namespace || strings.HasPrefix(r.URL.Path, namespace+"/") {
				protected.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
