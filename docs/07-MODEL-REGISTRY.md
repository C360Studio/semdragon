# Model Registry

The model registry routes LLM calls to the correct provider endpoint. It decouples
processors from hard-coded model names by mapping **capability keys** to **endpoint
definitions**, with ordered preferred/fallback chains. Processors ask for a capability
(`agent-work`, `boss-battle`, etc.); the registry resolves it to a concrete endpoint and
hands back connection config.

## Contents

- [Configuration Format](#configuration-format)
- [Endpoints](#endpoints)
- [Capabilities](#capabilities)
- [Tier-Qualified Capabilities](#tier-qualified-capabilities)
- [Default Config (semdragons.json)](#default-config-semdragonsjson)
- [Production Config (models.json)](#production-config-modelsjson)
- [Provider Setup](#provider-setup)
- [How Processors Use the Registry](#how-processors-use-the-registry)
- [Environment Variables](#environment-variables)

---

## Configuration Format

The registry is a JSON object with three top-level keys:

```json
{
  "endpoints":    { ... },
  "capabilities": { ... },
  "defaults":     { ... }
}
```

The registry is loaded at startup via `semdragons.LoadModelRegistry(path)`. If the file
is missing, `DefaultModelRegistry()` is used automatically—Ollama on localhost with no
API keys required.

---

## Endpoints

Each key under `endpoints` is an arbitrary name used throughout the rest of the config.

| Field            | Type    | Required | Description                                              |
|------------------|---------|----------|----------------------------------------------------------|
| `provider`          | string  | yes      | `"anthropic"`, `"openai"`, `"gemini"`, or `"ollama"`     |
| `url`               | string  | no       | Base URL; defaults to provider standard if omitted       |
| `model`             | string  | yes      | Provider model identifier                                |
| `max_tokens`        | int     | yes      | Maximum tokens per request                               |
| `supports_tools`    | bool    | yes      | Whether this endpoint supports tool/function calling     |
| `tool_format`       | string  | no       | `"anthropic"` or `"openai"`; required when tools enabled |
| `api_key_env`       | string  | no       | Environment variable name holding the API key            |
| `reasoning_effort`  | string  | no       | Reasoning effort level: `"none"`, `"low"`, `"medium"`, or `"high"` |

**Example — three providers:**

```json
"endpoints": {
  "claude-4": {
    "provider": "anthropic",
    "model": "claude-sonnet-4-6",
    "max_tokens": 200000,
    "supports_tools": true,
    "tool_format": "anthropic",
    "api_key_env": "ANTHROPIC_API_KEY"
  },
  "gpt-4o": {
    "provider": "openai",
    "url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "max_tokens": 128000,
    "supports_tools": true,
    "tool_format": "openai",
    "api_key_env": "OPENAI_API_KEY"
  },
  "ollama-qwen": {
    "provider": "ollama",
    "url": "http://localhost:11434",
    "model": "qwen2.5-coder:7b",
    "max_tokens": 32768,
    "supports_tools": true,
    "tool_format": "openai"
  }
}
```

The `url` field is optional for Anthropic (the SDK handles the base URL). It is required
for OpenAI-compatible providers and Ollama.

### Reasoning Effort

The optional `reasoning_effort` field controls thinking budget for reasoning-capable models
(OpenAI o3/o4-mini, Gemini 2.5 Flash/Pro). It is forwarded as the `reasoning_effort`
parameter on OpenAI-compatible chat completion requests.

| Value    | Behavior                                                    |
|----------|-------------------------------------------------------------|
| `"none"` | Disable reasoning entirely                                  |
| `"low"`  | Minimal thinking — fast responses, lower cost               |
| `"medium"` | Balanced thinking — good for most tasks                   |
| `"high"` | Deep reasoning — thorough analysis, higher latency and cost |

Not all models support reasoning effort. Models that don't (e.g., Gemini 2.0 Flash Lite,
GPT-4o) silently ignore the field. The value is exposed in the `GET /game/models` API
response for dashboard display.

```json
"gemini-flash": {
  "provider": "openai",
  "url": "https://generativelanguage.googleapis.com/v1beta/openai/",
  "model": "gemini-2.5-flash-preview-04-17",
  "max_tokens": 65536,
  "supports_tools": true,
  "reasoning_effort": "low",
  "api_key_env": "GEMINI_API_KEY"
}
```

---

## Capabilities

Each key under `capabilities` is a logical task name. Processors resolve these to
endpoint names at runtime.

| Field            | Type       | Required | Description                                              |
|------------------|------------|----------|----------------------------------------------------------|
| `description`    | string     | no       | Human-readable description of the capability             |
| `preferred`      | []string   | yes      | Ordered list of endpoint names; first available is used  |
| `fallback`       | []string   | no       | Tried in order if all preferred endpoints fail           |
| `requires_tools` | bool       | no       | If `true`, only endpoints with `supports_tools` qualify  |

**Example:**

```json
"capabilities": {
  "agent-work": {
    "description": "Agent quest execution with tool calling",
    "preferred": ["claude-4", "gpt-4o"],
    "fallback": ["ollama-tools"],
    "requires_tools": true
  },
  "boss-battle": {
    "description": "Quest output evaluation by LLM judge",
    "preferred": ["claude-4"],
    "fallback": ["gpt-4o", "ollama"]
  }
}
```

Resolution walks `preferred` in order, then `fallback`. The first endpoint whose
`supports_tools` value satisfies `requires_tools` is selected.

---

## Tier-Qualified Capabilities

The registry supports **dotted capability keys** for fine-grained model selection based
on agent trust tier and quest skill. The resolution chain from most-specific to least:

```
agent-work.{tier}.{skill}   →   agent-work.{tier}   →   agent-work
```

This lets lower tiers use cheaper models while masters and grandmasters get frontier
models—without any code changes, purely through config.

**Production examples:**

```json
"agent-work.apprentice": {
  "description": "Apprentice tier: small/fast models",
  "preferred": ["haiku", "gpt-mini"],
  "fallback": ["ollama"],
  "requires_tools": true
},
"agent-work.expert": {
  "description": "Expert tier: full models",
  "preferred": ["claude-4", "gpt-4o"],
  "fallback": ["haiku"],
  "requires_tools": true
},
"agent-work.expert.summarization": {
  "description": "Expert summarization: cheap for simple work",
  "preferred": ["haiku", "gpt-mini"],
  "requires_tools": false
}
```

Trust tiers map to capability suffixes as follows:

| Trust Tier   | Levels | Capability Suffix        |
|--------------|--------|--------------------------|
| Apprentice   | 1–5    | `agent-work.apprentice`  |
| Journeyman   | 6–10   | `agent-work.journeyman`  |
| Expert       | 11–15  | `agent-work.expert`      |
| Master       | 16–18  | `agent-work.master`      |
| Grandmaster  | 19–20  | `agent-work.grandmaster` |

---

## Default Config (semdragons.json)

The default config ships with a single Ollama endpoint. No API keys are needed; Ollama
runs locally. This is the config used during `task test:integration` and local `task
build` runs.

```json
"model_registry": {
  "endpoints": {
    "ollama-qwen": {
      "provider": "ollama",
      "url": "http://localhost:11434",
      "model": "qwen2.5-coder:7b",
      "max_tokens": 32768,
      "supports_tools": true
    }
  },
  "capabilities": {
    "agent-work": {
      "description": "Default capability for agent quest execution",
      "preferred": ["ollama-qwen"],
      "requires_tools": true
    }
  },
  "defaults": {
    "model": "ollama-qwen",
    "capability": "agent-work"
  }
}
```

The `defaults.model` is the endpoint name used when capability resolution finds nothing.
The `defaults.capability` is the fallback capability key.

---

## Production Config (models.json)

`config/models.json` is a standalone registry file for production deployments. Load it
by passing `--model-config config/models.json` to the binary, or set the path in your
deployment config.

It defines four endpoints and multiple capability tiers:

| Endpoint      | Provider  | Model                         | Tools  |
|---------------|-----------|-------------------------------|--------|
| `claude-4`    | Anthropic | claude-sonnet-4-6    | yes    |
| `gpt-4o`      | OpenAI    | gpt-4o                        | yes    |
| `ollama`      | Ollama    | llama3.2                      | no     |
| `ollama-tools`| Ollama    | llama3.1                      | yes    |

Capabilities in `models.json`:

| Capability       | Preferred            | Fallback           | Notes                        |
|------------------|----------------------|--------------------|------------------------------|
| `agent-work`     | claude-4, gpt-4o     | ollama-tools       | Default agent execution      |
| `boss-battle`    | claude-4             | gpt-4o, ollama     | LLM-as-judge evaluation      |
| `quest-design`   | claude-4             | gpt-4o             | DM quest parameter decisions |
| `agent-eval`     | claude-4             | gpt-4o             | Agent performance assessment |

The production Go code (`ProductionModelRegistry()` in `config.go`) mirrors this JSON
exactly and includes the full tier-qualified capability hierarchy.

---

## Provider Setup

### Ollama (local, no API key)

1. Install Ollama from [ollama.com](https://ollama.com).
2. Pull the model you want to use:

   ```bash
   ollama pull qwen2.5-coder:7b    # default dev model (supports tools)
   ollama pull llama3.2             # lightweight, no tool support
   ollama pull llama3.1             # tool-capable via OpenAI format
   ```

3. Verify Ollama is running:

   ```bash
   curl http://localhost:11434/api/tags
   ```

Ollama uses `tool_format: "openai"` for tool calling. Ensure the model you pull
advertises tool support—not all Ollama models do.

### Anthropic

1. Obtain an API key from [console.anthropic.com](https://console.anthropic.com).
2. Export the key before starting the service:

   ```bash
   export ANTHROPIC_API_KEY="sk-ant-..."
   ```

3. Reference the key in your endpoint config:

   ```json
   "api_key_env": "ANTHROPIC_API_KEY"
   ```

No `url` field is required for Anthropic; the SDK uses `https://api.anthropic.com` by
default.

### OpenAI

1. Obtain an API key from [platform.openai.com](https://platform.openai.com).
2. Export the key before starting the service:

   ```bash
   export OPENAI_API_KEY="sk-..."
   ```

3. Reference the key and set the base URL in your endpoint config:

   ```json
   "provider": "openai",
   "url": "https://api.openai.com/v1",
   "api_key_env": "OPENAI_API_KEY"
   ```

### Custom / Self-Hosted (OpenAI-compatible)

Any service that exposes a `/chat/completions` endpoint works out of the box. Set
`provider: "openai"` and point `url` at your service:

```json
"my-llm": {
  "provider": "openai",
  "url": "https://my-llm.example.com/v1",
  "model": "my-model-name",
  "api_key_env": "MY_LLM_API_KEY",
  "max_tokens": 128000,
  "supports_tools": true
}
```

This works with vLLM, LM Studio, Azure OpenAI, OpenRouter, text-generation-inference,
and any other service that implements the OpenAI chat completions API. Semdragons appends
`/chat/completions` to the `url` automatically — provide the base URL only.

If your service doesn't require an API key, omit `api_key_env`.

The Gemini configs use this same approach — Google's Gemini API exposes an
OpenAI-compatible endpoint at `https://generativelanguage.googleapis.com/v1beta/openai`.

### Provider Routing

Under the hood, there are only two HTTP paths:

| Provider value | HTTP format | Auth header |
|----------------|-------------|-------------|
| `"anthropic"` | Anthropic Messages API (`/messages`) | `x-api-key` |
| Everything else (`"openai"`, `"gemini"`, `"ollama"`, etc.) | OpenAI chat completions (`/chat/completions`) | `Authorization: Bearer` |

If your service speaks OpenAI format, use `"openai"` as the provider regardless of who
built it.

---

## How Processors Use the Registry

Processors receive a `model.RegistryReader` from `component.Dependencies.ModelRegistry`
at startup. The interface provides three methods:

```go
type RegistryReader interface {
    Resolve(capability string) string         // capability key → endpoint name
    GetEndpoint(name string) *EndpointConfig  // endpoint name → connection config
    GetFallbackChain(key string) []string     // existence check + full chain
    GetDefault() string                       // default endpoint name
}
```

**Resolution flow in questbridge and executor:**

```
1. Determine agent trust tier and quest primary skill
2. Build capability key: "agent-work.{tier}.{skill}"
3. Call GetFallbackChain(key) — non-nil means the key exists in the registry
4. If missing, fall back to "agent-work.{tier}"
5. If still missing, fall back to "agent-work"
6. Call Resolve(capability) → endpoint name
7. Call GetEndpoint(endpointName) → *EndpointConfig
8. Pass EndpointConfig to agenticmodel.NewClient(endpoint) to get an LLM client
```

**Questbridge** writes the resolved endpoint name into the `TaskMessage.Model` field
before publishing to the AGENT stream. The `agentic-model` processor picks up that name
and opens the actual connection, so questbridge itself never holds an open LLM client.

**Bossbattle** resolves `boss-battle` capability when running LLM-judge evaluations.

**DM chat handler** (`service/api`) resolves `dm-chat` capability for the conversational
interface.

If `registry` is nil (not configured), questbridge falls back to passing the raw
capability string as the model key, and `agentic-model` uses its own defaults.

---

## Environment Variables

| Variable            | Used By    | Description                            |
|---------------------|------------|----------------------------------------|
| `ANTHROPIC_API_KEY` | Anthropic  | API key for all Anthropic endpoints    |
| `OPENAI_API_KEY`    | OpenAI     | API key for all OpenAI endpoints       |
| `GEMINI_API_KEY`    | Gemini     | API key for Google Gemini endpoints    |

Set these in your shell or deployment environment before starting the service. The
registry reads them at the time each LLM request is made, not at startup—so rotating
keys does not require a restart.

No environment variables are required for Ollama; it authenticates by network access
only.
