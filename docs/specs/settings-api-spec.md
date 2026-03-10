# Settings API Specification

**Target**: `service/api/` in semdragons
**Depends on**: semstreams `config.Manager`, `model.Registry`, `component.Registry`

## Overview

Three new endpoints that expose runtime config, validate system health, and allow mutation of mutable settings. The semdragons UI dashboard will consume these to render a Settings page with onboarding guidance.

---

## Endpoint 1: GET /api/game/settings

**Auth**: None required (read-only, no secrets exposed)

Returns the current runtime configuration. API keys are NEVER returned — only the env var name and whether it is set.

### Response (200)

```json
{
  "platform": {
    "org": "c360",
    "platform": "local-dev",
    "board": "board1",
    "environment": "development"
  },
  "nats": {
    "connected": true,
    "url": "nats://localhost:4222",
    "latency_ms": 1.2
  },
  "models": {
    "endpoints": [
      {
        "name": "ollama-coder",
        "provider": "ollama",
        "model": "qwen2.5-coder:7b",
        "url": "http://localhost:11434/v1",
        "max_tokens": 32768,
        "supports_tools": true,
        "tool_format": "",
        "api_key_env": "",
        "api_key_set": false,
        "stream": false,
        "reasoning_effort": "",
        "input_price_per_1m_tokens": 0,
        "output_price_per_1m_tokens": 0
      },
      {
        "name": "claude-4",
        "provider": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "url": "",
        "max_tokens": 200000,
        "supports_tools": true,
        "tool_format": "anthropic",
        "api_key_env": "ANTHROPIC_API_KEY",
        "api_key_set": true,
        "stream": true,
        "reasoning_effort": "",
        "input_price_per_1m_tokens": 3.0,
        "output_price_per_1m_tokens": 15.0
      }
    ],
    "capabilities": {
      "agent-work": {
        "description": "General agent task execution",
        "preferred": ["ollama-coder"],
        "fallback": ["ollama-qwen3"],
        "requires_tools": true
      },
      "boss-battle": {
        "description": "Review evaluation",
        "preferred": ["ollama-qwen3"],
        "fallback": [],
        "requires_tools": false
      }
    },
    "defaults": {
      "model": "ollama-coder",
      "capability": "agent-work"
    }
  },
  "components": [
    {
      "name": "questboard",
      "type": "processor",
      "enabled": true,
      "running": true,
      "healthy": true,
      "status": "running",
      "uptime_seconds": 3600,
      "error_count": 0,
      "last_error": ""
    },
    {
      "name": "bossbattle",
      "type": "processor",
      "enabled": true,
      "running": true,
      "healthy": false,
      "status": "degraded",
      "uptime_seconds": 3600,
      "error_count": 3,
      "last_error": "model endpoint unreachable"
    }
  ],
  "workspace": {
    "dir": "/workspace",
    "exists": false,
    "writable": false
  },
  "token_budget": {
    "global_hourly_limit": 1000000,
    "endpoint_pricing": {}
  }
}
```

### Implementation Notes

- **Platform**: Read from `s.boardConfig` and `s.config` (Org, Platform, Board) plus platform environment from `deps.Platform`
- **NATS**: `s.nats.Conn().IsConnected()`, `s.nats.Conn().RTT()` for latency
- **Model endpoints**: Iterate `s.models.ListEndpoints()`, call `s.models.GetEndpoint(name)` for each. For `api_key_set`, check `os.Getenv(ep.APIKeyEnv) != ""`
- **Capabilities**: Iterate `s.models.ListCapabilities()`, call `s.models.GetFallbackChain(cap)` for preferred+fallback
- **Components**: Iterate `s.componentDeps.ComponentRegistry.ListComponents()`, call `.Meta()` and `.Health()` on each `Discoverable`. Cross-reference with config's `Components` map for `enabled` status
- **Workspace**: `os.Stat(s.config.WorkspaceDir)` for exists, attempt `os.CreateTemp` + remove for writable check

---

## Endpoint 2: GET /api/game/settings/health

**Auth**: None required

Runs live validation checks with per-check timeouts (3 seconds each). Returns an overall health assessment plus an onboarding checklist for first-time users.

### Response (200)

```json
{
  "overall": "degraded",
  "checks": [
    {
      "name": "nats",
      "status": "ok",
      "message": "Connected, RTT 1.2ms"
    },
    {
      "name": "llm_endpoint:ollama-coder",
      "status": "ok",
      "message": "Endpoint configured (ollama, no API key needed)"
    },
    {
      "name": "llm_endpoint:claude-4",
      "status": "warning",
      "message": "ANTHROPIC_API_KEY is set but endpoint reachability not verified"
    },
    {
      "name": "workspace",
      "status": "error",
      "message": "/workspace does not exist"
    },
    {
      "name": "stream:AGENT",
      "status": "ok",
      "message": "Stream exists with 3 consumers"
    },
    {
      "name": "bucket:entity_state",
      "status": "ok",
      "message": "Bucket exists with 42 keys"
    }
  ],
  "checklist": [
    { "label": "NATS connected", "met": true },
    { "label": "LLM endpoint configured", "met": true },
    { "label": "API key set for LLM provider", "met": true, "help_text": "" },
    { "label": "Workspace directory exists and is writable", "met": false, "help_text": "Create the directory or update workspace_dir in config. For Docker: mounted automatically. For local dev: mkdir -p .workspace and set workspace_dir in config/semdragons.json services.game.config.workspace_dir" },
    { "label": "At least one agent recruited", "met": true },
    { "label": "At least one quest posted", "met": false, "help_text": "POST /api/game/quests with an objective to create your first quest" }
  ]
}
```

### Health Check Logic

| Check | Status: ok | Status: warning | Status: error |
|-------|-----------|-----------------|---------------|
| `nats` | Connected + RTT < 100ms | Connected + RTT > 100ms | Not connected |
| `llm_endpoint:{name}` | Ollama (no key needed) or key env set | Key env set but can't verify reachability | Key env required but empty |
| `workspace` | Dir exists + writable | Dir exists but not writable | Dir doesn't exist |
| `stream:AGENT` | Stream exists | — | Stream missing |
| `bucket:entity_state` | Bucket exists | — | Bucket missing |

**Overall**: `healthy` if all checks ok, `degraded` if any warning, `unhealthy` if any error.

### Onboarding Checklist Logic

- **NATS connected**: `s.nats.Conn().IsConnected()`
- **LLM endpoint configured**: `len(s.models.ListEndpoints()) > 0`
- **API key set**: At least one endpoint with non-empty `api_key_env` has `os.Getenv() != ""`, OR at least one ollama endpoint exists (no key needed)
- **Workspace writable**: `os.Stat` + `os.CreateTemp` test
- **Agent recruited**: `s.graph.ListAgentsByPrefix(ctx, 1)` returns non-empty
- **Quest posted**: `s.graph.ListQuestsByPrefix(ctx, 1)` returns non-empty

---

## Endpoint 3: POST /api/game/settings

**Auth**: Required (`requireAuth` — `X-API-Key` or `Authorization: Bearer`)

Mutates runtime-mutable settings. Returns the full updated `SettingsResponse` (same schema as GET).

### Request Body

All fields optional — only provided fields are applied.

```json
{
  "model_registry": {
    "endpoints": {
      "claude-4": {
        "provider": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "url": "",
        "max_tokens": 200000,
        "supports_tools": true,
        "tool_format": "anthropic",
        "api_key_env": "ANTHROPIC_API_KEY",
        "stream": true,
        "input_price_per_1m_tokens": 3.0,
        "output_price_per_1m_tokens": 15.0
      },
      "old-endpoint": {
        "remove": true
      }
    },
    "capabilities": {
      "agent-work": {
        "preferred": ["claude-4"],
        "fallback": ["ollama-coder"],
        "requires_tools": true
      }
    },
    "defaults": {
      "model": "claude-4",
      "capability": "agent-work"
    }
  },
  "token_budget": {
    "global_hourly_limit": 500000
  }
}
```

### Validation Rules (return 400 on failure)

1. Endpoint names must be non-empty strings
2. Provider must be one of: `anthropic`, `openai`, `ollama`, `openrouter`
3. `max_tokens` must be > 0
4. Cannot remove an endpoint referenced by any capability's `preferred` or `fallback` list
5. Capability `preferred` and `fallback` must reference endpoints that exist (including ones being added in the same request)
6. `token_budget.global_hourly_limit` must be >= 0 (0 = unlimited)
7. Full `model.Registry.Validate()` must pass after applying changes

### Error Response (400)

```json
{
  "error": "validation failed",
  "details": [
    "capability 'agent-work' references nonexistent endpoint 'deleted-model'",
    "endpoint 'bad' has invalid provider 'unknown'"
  ]
}
```

### Implementation Logic

```
1. Parse + validate request body
2. cfg := s.configManager.GetConfig().Get()
3. updated := cfg.Clone()
4. Apply endpoint adds/updates/removes to updated.ModelRegistry.Endpoints
5. Apply capability updates to updated.ModelRegistry.Capabilities
6. Apply defaults update to updated.ModelRegistry.Defaults
7. Run validation rules (above) + updated.ModelRegistry.Validate()
8. If invalid → return 400 with details
9. s.configManager.GetConfig().Update(updated)   // runtime in-memory
10. updated.SaveToFile(s.configPath)               // persist to disk (skip if configPath == "")
11. Refresh s.models to point to updated.ModelRegistry
12. Return assembled SettingsResponse (same as GET)
```

### What is NOT mutable via this endpoint

These require a server restart and are intentionally excluded:
- Platform identity (org, platform, environment)
- Board name
- NATS connection URLs
- JetStream stream/bucket definitions
- Component enable/disable (requires component manager lifecycle — future work)

---

## Required Changes

### New file: `service/api/settings.go`

All three handlers + request/response types.

### Modified: `service/api/service.go`

```go
// Add to Service struct:
configManager *config.Manager  // retained from deps.Manager for write path
configPath    string           // path to config JSON for disk persistence

// Add to api.Config:
ConfigPath string `json:"config_path,omitempty"`

// Wire in New():
configManager: deps.Manager,
configPath:    cfg.ConfigPath,

// Add to RegisterHTTPHandlers():
mux.HandleFunc("GET "+prefix+"settings", cors(s.handleGetSettings))
mux.HandleFunc("GET "+prefix+"settings/health", cors(s.handleSettingsHealth))
mux.HandleFunc("POST "+prefix+"settings", cors(requireAuth(apiKey, s.handleUpdateSettings)))
```

### Modified: `service/api/interfaces.go`

Optional — add `ComponentLister` interface if you want to abstract `ComponentRegistry` for unit testing:

```go
type ComponentLister interface {
    ListComponents() map[string]component.Discoverable
}
```

### Modified: `service/api/openapi.go`

Add `"Settings"` tag and path entries for all three endpoints. Register response/request types.

### Modified: `cmd/semdragons/main.go`

Pass `configPath` through to the game service config so the API can persist writes:

```go
// In the game service config setup, add:
"config_path": cliCfg.ConfigPath
```

### New file: `service/api/settings_test.go`

Unit tests with mocked `ModelResolver` and `ComponentLister`:
- GET returns correct structure, API keys never leaked
- Health checks return correct statuses for various scenarios
- POST validates and rejects bad input (missing endpoints, invalid providers)
- POST round-trip: update → GET → verify changes
- POST with empty configPath skips disk write

---

## Frontend Contract

The UI will consume these endpoints to render a Settings page. The frontend team will mock responses during development and integrate once the endpoints are live. Key UI behaviors:

- Settings page loads GET + health in parallel on mount
- Health auto-refreshes every 30 seconds
- Write operations send POST and refresh the full settings view on success
- API key status shown as green/red indicator dots (never shows the actual key)
- Immutable fields shown with lock icon + "requires restart" label
