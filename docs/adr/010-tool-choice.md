# ADR-010: Tool Choice for Quest Execution

## Status: Accepted

## Context

LLM agents from different providers (Gemini, OpenAI, Anthropic) don't reliably call tools when instructed — especially for deterministic steps like party lead decomposition. Prior to this change, enforcement relied on prompt-level workarounds:

- `CategoryToolDirective` (priority 50) with numbered rules and imperative language
- `CategoryProviderHints` for Gemini/OpenAI to suppress text preamble before tool calls
- Text-only fallback with heuristic intent detection (`?` counting, `[INTENT:]` tags)

These are fragile and provider-dependent. All major providers now support API-level `tool_choice` (verified March 2026), and semstreams v1.0.0-alpha.39 shipped full ToolChoice threading (see semstreams ADR-023).

## Decision

Set `TaskMessage.ToolChoice` in questbridge based on quest context using a pure function `toolChoiceForQuest(quest, agent, tools)`.

### Tool Choice Map

| Quest Step | ToolChoice | Rationale |
|---|---|---|
| Party lead (decomposition) | `{Mode: "function", FunctionName: "decompose_quest"}` | Exactly one correct action |
| Single tool available | `{Mode: "function", FunctionName: that_tool}` | Only one option |
| Multiple tools (solo or party member) | `{Mode: "required"}` | Must use a tool, model picks which |
| No tools | `nil` (auto) | Nothing to force |

### Provider Translation (handled by semstreams agentic-model)

| Mode | OpenAI | Anthropic | Gemini (OAI-compat) |
|------|--------|-----------|---------------------|
| `auto` | `"auto"` | `{"type":"auto"}` | `"auto"` |
| `required` | `"required"` | `{"type":"any"}` | `"required"` (beta) |
| `function` | `{"type":"function","function":{"name":"X"}}` | `{"type":"tool","name":"X"}` | Unreliable |
| `none` | `"none"` | `{"type":"none"}` | `"none"` |

### Gemini Strategy

Force-specific (`function` mode) is unreliable on Gemini's OAI-compatible endpoint (ADR-009 row 22). The `CategoryProviderHints` prompt fragment is retained as belt-and-suspenders — it provides the specific tool name guidance that `required` alone cannot. When semstreams ships provider adapters (ADR-023 Phase 2), the Gemini adapter can rewrite `function` mode to `required` + prompt injection.

### Anthropic Caveat

`tool_choice: "any"` and force-specific modes are incompatible with extended thinking. If using Claude with thinking enabled, the adapter should downgrade to `auto` with a warning.

## Consequences

### Positive

- Deterministic quest steps (party lead decomposition) no longer rely on prompt engineering alone
- `tool_choice: required` for all tool-bearing quests eliminates the text-only fallback path for compliant providers
- Prompt fragments shift from desperate enforcement to workflow guidance
- `toolChoiceForQuest` is a pure function — reusable when DM eventually gets agentic tool access

### Negative

- Text-only fallback heuristics (`isOutputClarificationRequest`, `[INTENT:]` parsing) must be retained until provider validation confirms `required` works reliably across all providers
- Gemini force-specific still depends on prompt-level hints until adapter ships

### Future

Once `tool_choice: required` is validated across providers in E2E:
1. Remove text-only fallback heuristic code in questbridge completion handler
2. Simplify `CategoryProviderHints` — may become unnecessary for OpenAI/Anthropic
3. DM `/quest` command could route through agentic pipeline with tools (see project roadmap)
