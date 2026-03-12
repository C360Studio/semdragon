# ADR-009: Provider Adapters in agentic-model

## Status: Proposed

## Context

The semstreams `agentic-model` component uses the OpenAI SDK as a universal client for all LLM providers (OpenAI, Gemini, Anthropic, Ollama). Each provider exposes an OpenAI-compatible `/v1/chat/completions` endpoint, but none are truly compatible — each has quirks that cause 400 errors, dropped tool calls, or silent data loss.

Today these quirks are handled as inline fixes scattered across `client.go`, `stream.go`, and `component.go`. They are hard to find, test in isolation, or extend. As we add providers (Anthropic native, Mistral, Groq, DeepSeek), the scattered approach will not scale.

### Known Provider Quirks (as of alpha.33)

| Quirk | Provider | Current Location | Impact |
|-------|----------|-----------------|--------|
| Empty content on assistant tool_call messages | Gemini | client.go:142-146 | 400 INVALID_ARGUMENT |
| Tool result `name` field required | Gemini | client.go:116-124 | 400 INVALID_ARGUMENT |
| Missing `index` on streaming tool call deltas | Gemini | stream.go:87-100 | Corrupted tool calls |
| `reasoning_content` rejected in requests | Gemini, others | client.go:103-109 | 400 INVALID_ARGUMENT |
| Empty contents after orphaned tool pair removal | Gemini | context_manager.go | 400 contents not specified |
| `reasoning_effort` parameter | OpenAI o1 only | client.go:164-166 | Ignored by others |
| Tool support gating | Per-endpoint | component.go:328-337 | Tools silently stripped |
| Tool choice forcing unreliable | Gemini Flash | (prompt workaround) | Model ignores tool_choice |

### Why Now

The empty-contents-after-repair bug (row 5) is currently blocking Gemini E2E tests. Fixing it as another inline patch adds to the mess. A provider adapter pattern gives each fix a home and makes the next provider easy to add.

## Decision

### Per-Provider Adapter Interface

Define a `ProviderAdapter` interface in `agentic-model` that normalizes requests and responses at the boundary:

```go
// ProviderAdapter normalizes request/response payloads for a specific
// LLM provider's OpenAI-compatible endpoint. Adapters handle quirks
// that would otherwise cause 400 errors or silent data corruption.
type ProviderAdapter interface {
    // Name returns the provider identifier (e.g., "gemini", "openai").
    Name() string

    // NormalizeRequest adjusts the ChatCompletionRequest before sending.
    // Called after the generic request is built, before the HTTP call.
    NormalizeRequest(req *openai.ChatCompletionRequest)

    // NormalizeMessages adjusts the message array before sending.
    // Called before NormalizeRequest for message-level fixes.
    NormalizeMessages(messages []openai.ChatCompletionMessage) []openai.ChatCompletionMessage

    // NormalizeStreamDelta adjusts a streaming tool call delta.
    // Returns the corrected tool call index.
    NormalizeStreamDelta(delta openai.ToolCall, lastIndex int) int

    // NormalizeResponse adjusts the ChatCompletionResponse after receiving.
    // Called before the response is passed to the agentic loop.
    NormalizeResponse(resp *openai.ChatCompletionResponse)
}
```

### Adapter Resolution

Adapters are selected by matching the endpoint's `provider` field (already present in model registry config). Falls back to a no-op `GenericAdapter` for unknown providers.

```json
{
  "endpoints": {
    "gemini-pro": {
      "provider": "gemini",
      "model": "gemini-2.5-pro",
      "base_url": "https://generativelanguage.googleapis.com/v1beta/openai"
    }
  }
}
```

```go
func AdapterFor(provider string) ProviderAdapter {
    switch provider {
    case "gemini":
        return &GeminiAdapter{}
    case "openai":
        return &OpenAIAdapter{}
    case "anthropic":
        return &AnthropicAdapter{}
    default:
        return &GenericAdapter{}
    }
}
```

### Concrete Adapters

**GeminiAdapter** consolidates all Gemini quirks:

| Method | Quirk | Current fix |
|--------|-------|-------------|
| `NormalizeMessages` | Set empty content to `" "` on tool_call messages | client.go:142-146 |
| `NormalizeMessages` | Ensure `name` on tool result messages | client.go:116-124 |
| `NormalizeMessages` | Strip `reasoning_content` | client.go:103-109 |
| `NormalizeMessages` | Ensure at least one user message exists | (new — fixes empty contents bug) |
| `NormalizeStreamDelta` | Infer index from ID presence | stream.go:87-100 |
| `NormalizeRequest` | Strip `reasoning_effort` (not supported) | (no-op today, silent) |

**OpenAIAdapter** handles OpenAI-specific features:

| Method | Feature |
|--------|---------|
| `NormalizeRequest` | Pass through `reasoning_effort` for o1 models |
| `NormalizeMessages` | Strip `reasoning_content` from outgoing messages |

**GenericAdapter** applies safe defaults:

| Method | Behavior |
|--------|----------|
| `NormalizeMessages` | Strip `reasoning_content` (universal safety) |
| All others | No-op |

### No New Infrastructure

Adapters are pure functions on request/response structs. No new services, no proxy layers, no runtime dependencies. They live in `agentic-model` alongside the client.

### Registration Point

The adapter is resolved once per cached client (clients are cached per endpoint in `component.go`). The adapter is stored on the `Client` struct and called at the appropriate points in `buildChatRequest` and `ChatCompletion`.

## Rationale

1. **Single responsibility**: Each adapter owns all quirks for one provider. When Gemini changes behavior, there's one file to update.
2. **Testable in isolation**: `TestGeminiAdapter_NormalizeMessages` tests quirk handling without HTTP or NATS.
3. **Discoverable**: New developers can read `gemini_adapter.go` to understand all Gemini quirks instead of grepping across files.
4. **Extensible**: Adding Anthropic native API or DeepSeek means writing one new adapter, not scattering fixes.
5. **No abstraction tax**: Adapters are called at well-defined points. The hot path (streaming tokens) is unaffected — only the delta index inference runs per-chunk.

## Implementation

### Phase 1: Extract (no behavior change)

1. Define `ProviderAdapter` interface
2. Move existing quirks from `client.go` and `stream.go` into `GeminiAdapter` and `OpenAIAdapter`
3. Add `GenericAdapter` as the default
4. Wire adapter resolution into `Client` construction
5. Replace inline quirk code with adapter calls
6. Existing tests continue to pass unchanged

### Phase 2: Fix empty contents bug

1. Add `GeminiAdapter.NormalizeMessages` rule: if no non-system messages remain after context trimming, inject a synthetic user message with the original task summary
2. This prevents the 400 that currently blocks Gemini E2E

### Phase 3: Provider-specific features

1. Tool choice forcing strategy per adapter (Gemini ignores `tool_choice`; adapter can rewrite to prompt-level instruction)
2. Schema normalization (Gemini strict mode, OpenAI strict mode differ)
3. Token counting differences (adapter provides token estimation hints)

### File Layout

```
processor/agentic-model/
  adapter.go              # Interface + AdapterFor() resolver
  adapter_generic.go      # GenericAdapter (safe defaults)
  adapter_gemini.go       # GeminiAdapter (all Gemini quirks)
  adapter_gemini_test.go
  adapter_openai.go       # OpenAIAdapter
  adapter_openai_test.go
  adapter_anthropic.go    # AnthropicAdapter (when needed)
```

## Consequences

### Positive

- Provider quirks are documented by code — each adapter file is a living compatibility reference
- New providers are additive (write adapter, register in `AdapterFor`)
- The empty-contents bug fix has a natural home in `GeminiAdapter.NormalizeMessages`
- E2E tests per provider validate the full adapter path

### Negative

- Slight indirection — debugging a Gemini 400 requires knowing to look at `adapter_gemini.go` instead of `client.go`
- Adapter interface may need expansion as new quirk categories emerge (streaming, embeddings, structured output)
- Risk of over-abstracting if adapters accumulate provider-specific config that should be in the model registry

### Mitigations

- Keep adapters as pure functions — no state, no config, no dependencies beyond the OpenAI SDK types
- If an adapter method grows beyond ~30 lines, it's a sign the quirk should be fixed upstream or reported as a provider bug
- Review adapter changes alongside E2E test results for that provider
