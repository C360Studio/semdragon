# ADR-007: Scenario-Driven Quest Specs and Decomposability Classification

## Status: Accepted

## Context

Quest creation today produces thin briefs: a title, a freeform description, flat
acceptance criteria strings, and optional hints. When a party lead receives this brief
and calls `decompose_quest`, they invent the decomposition from scratch — the brief
gives them almost no structured material to work with. The result is unpredictable
decompositions whose quality depends entirely on the LLM's ability to extract structure
from a paragraph of prose.

Meanwhile, **semspec** (a sibling product in the c360 ecosystem) follows a richer
creation model: plan → requirements → scenarios → tasks. Scenarios are concrete,
testable outcomes that sit between high-level requirements and low-level tasks. An
agent coordinator breaks scenarios down into tasks for its team. This intermediate layer
gives the coordinator structured material to work with instead of inventing from
nothing.

### Empirical grounding: agentic scaling research

Google Research's "Towards a Science of Scaling Agent Systems" (arxiv 2512.08296)
provides empirical findings that directly inform this design:

1. **Parallelizable tasks benefit from multi-agent coordination** — centralized
   coordination improved performance by 80.9% over a single agent on tasks with
   independent subtasks.

2. **Sequential tasks degrade with multi-agent** — every multi-agent variant tested
   degraded performance by 39–70% on tasks where steps depend on previous steps. The
   communication overhead fragments the reasoning process, leaving insufficient
   cognitive budget for the actual task.

3. **Error amplification** — independent agents can amplify errors up to ~17× when
   mistakes propagate unchecked, while centralized coordination (party lead review)
   limits propagation to ~4.4×.

4. **Coordination costs scale super-linearly with tool density** — the more tool-heavy
   the task, the worse multi-agent coordination performs.

5. **Higher-capability models benefit disproportionately** — top-quartile models achieve
   23% higher performance than predicted by linear scaling alone. For sequential
   high-stakes quests, routing to a higher-tier model matters more than adding agents.

**Key insight**: party vs solo is not about difficulty — it is about decomposability.
A Hard quest that is inherently sequential should be a solo high-tier agent. A Moderate
quest with four independent components is a great party quest.

### What exists today

- `QuestBrief` has `Title`, `Description`, `Difficulty`, `Skills`, `Acceptance`
  (flat `[]string`), `DependsOn`, `Hints`.
- `QuestHints.PartyRequired` defaults to false; the DM must explicitly set it.
- Party lead prompt says "call `decompose_quest` first" but gives no structured input
  to decompose from.
- No mechanism exists to classify whether a quest benefits from multi-agent
  coordination before a party is formed.
- The DM chat quest mode prompt teaches `quest_brief` and `quest_chain` JSON formats
  with flat acceptance criteria.

## Decision

Replace flat acceptance criteria with **structured scenarios** and add **goal** and
**requirements** fields to quest briefs. Scenarios carry optional `depends_on`
references to other scenarios, which serve as the signal for a deterministic
decomposability classification: independent scenarios → party quest, sequential chain →
solo quest routed to the highest-tier model available.

The DM chat quest mode prompt teaches the LLM to think in terms of
goal → requirements → scenarios and to declare scenario dependencies. The system
applies the routing heuristic deterministically after the DM produces the spec.

## Design

### Quest Spec Structure

```go
// QuestBrief is the JSON-friendly input for creating a quest.
// The DM chat produces this structure in quest mode.
type QuestBrief struct {
    Title        string           `json:"title"`
    Goal         string           `json:"goal"`
    Requirements []string         `json:"requirements,omitempty"`
    Scenarios    []QuestScenario  `json:"scenarios"`
    Difficulty   *QuestDifficulty `json:"difficulty,omitempty"`
    Skills       []SkillTag       `json:"skills,omitempty"`
    Hints        *QuestHints      `json:"hints,omitempty"`
    DependsOn    []QuestID        `json:"depends_on,omitempty"`
}

// QuestScenario is a concrete, testable outcome within a quest spec.
// Scenarios are the structured material the party lead uses when
// decomposing into sub-quests, or the acceptance criteria a solo
// agent works against.
type QuestScenario struct {
    Name        string   `json:"name"`
    Description string   `json:"description"`
    Skills      []string `json:"skills,omitempty"`
    DependsOn   []string `json:"depends_on,omitempty"` // References other scenario names
}
```

**Field semantics:**

| Field | Purpose | Example |
|-------|---------|---------|
| `title` | Short quest name | "Build user notification service" |
| `goal` | The "why" — desired outcome | "Users receive real-time notifications across email and in-app channels" |
| `requirements` | Concrete constraints | "Delivery within 5 seconds", "Per-event preferences" |
| `scenarios` | Testable outcomes | See examples below |
| `skills` | Aggregate skills (union of scenario skills) | ["code_generation", "analysis"] |

This is a breaking change to `QuestBrief`. The old `Description` and `Acceptance`
fields are removed, replaced by `Goal`, `Requirements`, and `Scenarios`. The project
is greenfield — there are no external API consumers to migrate.

### Decomposability Classification

The routing heuristic runs after the DM produces the spec and before the quest is
posted. It is deterministic — no LLM call required.

```go
// DecomposabilityClass determines how a quest should be staffed.
type DecomposabilityClass string

const (
    // DecomposableParallel means scenarios are independent — party quest.
    DecomposableParallel DecomposabilityClass = "parallel"
    // DecomposableSequential means scenarios form a dependency chain — solo quest.
    DecomposableSequential DecomposabilityClass = "sequential"
    // DecomposableMixed means some scenarios are independent, some sequential.
    DecomposableMixed DecomposabilityClass = "mixed"
    // DecomposableTrivial means single scenario or trivial difficulty — solo quest.
    DecomposableTrivial DecomposabilityClass = "trivial"
)

func ClassifyDecomposability(brief *QuestBrief) DecomposabilityClass {
    if len(brief.Scenarios) <= 1 {
        return DecomposableTrivial
    }

    hasDeps := false
    allDependent := true
    for _, s := range brief.Scenarios {
        if len(s.DependsOn) > 0 {
            hasDeps = true
        } else {
            allDependent = false
        }
    }

    if !hasDeps {
        return DecomposableParallel
    }
    // If only the first scenario has no deps and all others form a chain
    if allDependent && countRoots(brief.Scenarios) == 1 {
        return DecomposableSequential
    }
    return DecomposableMixed
}
```

**Routing rules:**

| Class | Staffing | Rationale |
|-------|----------|-----------|
| `parallel` | Party quest, party lead decomposes | Independent scenarios → 80.9% improvement from coordination |
| `sequential` | Solo, route to highest-tier model | Sequential chain → 39-70% degradation from multi-agent |
| `mixed` | Party quest, smaller party size | Some parallelism, lead groups sequential chains per agent |
| `trivial` | Solo | Single scenario or trivial difficulty, no coordination needed |

The classification is stored on the quest entity as `quest.routing.decomposability` and
`quest.routing.class` predicates so it is visible in the graph and trajectory.

**DM override:** `QuestHints` retains `PartyRequired` as a manual override. If the DM
or user explicitly sets `party_required: true` on a sequential quest, the system
honours it with a warning log. The heuristic is a default, not a gate.

### Scenario Dependency Validation

Scenario `depends_on` references are validated at creation time:

```go
func ValidateScenarioDependencies(scenarios []QuestScenario) error {
    names := make(map[string]bool, len(scenarios))
    for _, s := range scenarios {
        if s.Name == "" {
            return fmt.Errorf("scenario name is required")
        }
        if names[s.Name] {
            return fmt.Errorf("duplicate scenario name: %s", s.Name)
        }
        names[s.Name] = true
    }
    for _, s := range scenarios {
        for _, dep := range s.DependsOn {
            if !names[dep] {
                return fmt.Errorf("scenario %q depends on unknown scenario %q", s.Name, dep)
            }
            if dep == s.Name {
                return fmt.Errorf("scenario %q cannot depend on itself", s.Name)
            }
        }
    }
    // Cycle detection (same Kahn's algorithm as QuestChainBrief)
    return detectScenarioCycles(scenarios)
}
```

### Quest Entity Storage

Scenarios are stored as triples on the quest entity, alongside the existing fields:

```
quest.spec.goal           = "Users receive real-time notifications..."
quest.spec.requirements   = ["Delivery within 5 seconds", ...]  (JSON array)
quest.spec.scenarios      = [{"name":"...", "description":"...", ...}]  (JSON array)
quest.routing.class       = "parallel"
quest.routing.decomposability = "parallel"
```

The `quest.spec.scenarios` triple stores the full `[]QuestScenario` as JSON. This
keeps the schema flat (one triple vs N triples per scenario) while remaining
queryable via the graph.

### DM Quest Mode Prompt Changes

The quest mode system prompt teaches goal → requirements → scenarios thinking:

```
## Quest Spec Format

A quest spec has four parts:

1. **Goal** — The desired outcome. Why does this work matter? One or two sentences.
2. **Requirements** — Concrete constraints that must be satisfied. Short, testable statements.
3. **Scenarios** — Testable outcomes that prove the requirements are met. Each scenario
   has a name, description, and optional skills. If a scenario depends on another
   scenario completing first, add a depends_on reference.
4. **Skills** — Aggregate skill tags needed across all scenarios.

### Scenario Dependencies Matter

Scenarios that are independent of each other signal a PARTY quest — multiple agents
can work on them in parallel. Scenarios that depend on each other signal a SOLO quest —
one high-capability agent should handle the sequential chain.

Ask yourself: can these scenarios be worked on simultaneously by different agents, or
does each one need the previous one's output? Declare depends_on honestly — it drives
the routing decision.
```

The tagged JSON format evolves to:

```json
{
  "title": "Build user notification service",
  "goal": "Users receive real-time notifications across email and in-app channels",
  "requirements": [
    "Support email and in-app channels",
    "Per-event notification preferences",
    "Delivery within 5 seconds"
  ],
  "scenarios": [
    {
      "name": "In-app notification delivery",
      "description": "Workspace event fires with in-app enabled, notification appears in feed within 5s",
      "skills": ["code_generation"]
    },
    {
      "name": "Email with preference check",
      "description": "System checks user prefs, sends email only if enabled for that event type",
      "skills": ["code_generation", "data_transformation"]
    },
    {
      "name": "Preference configuration API",
      "description": "User can GET/PUT notification preferences per event type with validation",
      "skills": ["code_generation", "analysis"]
    }
  ],
  "difficulty": 3,
  "skills": ["code_generation", "data_transformation", "analysis"]
}
```

Three independent scenarios, no `depends_on` → classified as `parallel` → party quest.

Versus a sequential quest:

```json
{
  "title": "Migrate legacy auth to OAuth2",
  "goal": "Replace custom token auth with OAuth2 PKCE flow without breaking sessions",
  "requirements": [
    "Existing sessions valid during migration",
    "New auth uses OAuth2 PKCE",
    "Rollback plan available"
  ],
  "scenarios": [
    {
      "name": "OAuth2 provider integration",
      "description": "Configure OAuth2 provider, implement PKCE flow, verify token exchange"
    },
    {
      "name": "Session migration bridge",
      "description": "Adapter validates both old tokens and new OAuth2 tokens during transition",
      "depends_on": ["OAuth2 provider integration"]
    },
    {
      "name": "Cutover and rollback",
      "description": "Switch endpoints to OAuth2-only with feature flag, verify rollback restores old auth",
      "depends_on": ["Session migration bridge"]
    }
  ],
  "difficulty": 4
}
```

Linear dependency chain → classified as `sequential` → solo quest routed to
highest-tier model.

### Party Lead Prompt Changes

When a party lead receives a quest with structured scenarios, the decomposition prompt
evolves from "figure out how to decompose this" to "map these scenarios to sub-quests":

```
You are a PARTY LEAD coordinating a team of agents.

This quest has structured scenarios. Use them as your decomposition material:

{{ range .Scenarios }}
- **{{ .Name }}**: {{ .Description }}
  {{- if .Skills }} (skills: {{ .Skills }}){{ end }}
  {{- if .DependsOn }} (depends on: {{ .DependsOn }}){{ end }}
{{ end }}

YOUR WORKFLOW:
1. FIRST ACTION: Call decompose_quest to create sub-quests from these scenarios.
   - You may map scenarios 1:1 to sub-quests, group simple ones, or split complex ones.
   - Each sub-quest must be self-contained and independently executable.
   - Honour scenario dependencies as sub-quest dependencies.
   - Do NOT add a "combine" or "synthesize" step — that is YOUR responsibility.
2. REVIEW PHASE: You will review each completed sub-quest via review_sub_quest.
3. SYNTHESIS: After all sub-quests are accepted, YOU synthesize the final deliverable.
```

The scenarios give the lead concrete material instead of requiring invention from prose.
The lead retains discretion to regroup — scenarios are input, not a rigid contract.

### Solo Agent Prompt Changes

For sequential or trivial quests, the solo agent receives scenarios as structured
acceptance criteria instead of flat strings:

```
## Acceptance Scenarios

Complete each scenario in order. Each scenario's output may be needed by the next.

{{ range .Scenarios }}
### {{ .Name }}
{{ .Description }}
{{- if .DependsOn }}
Depends on: {{ .DependsOn }}
{{ end }}
{{ end }}
```

This gives solo agents clear structure without the overhead of party coordination.

### Quest Chain Evolution

`QuestChainBrief` entries gain the same fields. Each entry in a chain is itself a
quest spec with goal, requirements, and scenarios:

```go
type QuestChainEntry struct {
    Title        string           `json:"title"`
    Goal         string           `json:"goal,omitempty"`
    Requirements []string         `json:"requirements,omitempty"`
    Scenarios    []QuestScenario  `json:"scenarios,omitempty"`
    Difficulty   *QuestDifficulty `json:"difficulty,omitempty"`
    Skills       []SkillTag       `json:"skills,omitempty"`
    DependsOn    []int            `json:"depends_on,omitempty"`
    Hints        *QuestHints      `json:"hints,omitempty"`
}
```

Each chain entry is independently classified for decomposability. A chain might contain
a mix of party and solo quests.

### Model Routing for Sequential Quests

The decomposability classification feeds into model selection. When a quest is
classified as `sequential`, the system should route to the highest-capability model
available for that quest's required capabilities, because the research shows
accelerating returns from model intelligence on sequential reasoning tasks.

This is implemented as a capability modifier in the model registry resolution:

```go
capability := "quest-execution"
if quest.DecomposabilityClass == DecomposableSequential {
    capability = "quest-execution-sequential"
}
endpointName := models.Resolve(capability)
```

The `quest-execution-sequential` capability can be configured to prefer Opus over
Sonnet in the model registry, while standard `quest-execution` uses the default tier.
This is a configuration concern, not a code change — the model registry already
supports per-capability endpoint resolution.

## Component Change Summary

| Component | Change |
|-----------|--------|
| `domain/dm.go` | Replace `QuestBrief` fields: remove `Description`/`Acceptance`, add `Goal`, `Requirements`, `Scenarios []QuestScenario`; add `QuestScenario` type |
| `domain/dm.go` | Add `DecomposabilityClass` type and constants; add `ClassifyDecomposability()` function |
| `domain/dm.go` | Add `ValidateScenarioDependencies()` with cycle detection |
| `domain/types.go` | Add `DecomposabilityClass` field to `Quest` struct |
| `service/api/handlers.go` | In `handleCreateQuest` / `handlePostQuestChain`: use `Goal`/`Scenarios` directly; call `ClassifyDecomposability`; set `PartyRequired` from classification |
| `service/api/dm_prompt.go` | Rewrite quest mode instructions: goal → requirements → scenarios format, dependency guidance, updated JSON examples |
| `service/api/dm_prompt.go` | Update `writeAvailableOptions` with scenario guidance |
| `processor/promptmanager/fragments.go` | Update party lead directive to inject structured scenarios; add solo agent scenario directive |
| `processor/questbridge/handler.go` | Pass scenarios to prompt assembly; use `DecomposabilityClass` for model capability routing |
| `graphable.go` | Emit `quest.spec.goal`, `quest.spec.requirements`, `quest.spec.scenarios`, `quest.routing.class` triples |
| `reconstruction.go` | Reconstruct scenarios from triples in `QuestFromEntityState` |
| `config/semdragons.json` | Add `quest-execution-sequential` capability to model registry |

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| DM LLM produces unstructured text instead of scenarios | DM prompt strongly guides toward scenario format; quest mode retry logic re-prompts if no structured output extracted |
| DM misidentifies dependency structure (says independent when sequential) | Classification is a default, not a gate; boss battle review can catch quality issues from wrong staffing; humans can override via `party_required` hint |
| Scenario `depends_on` by name is fragile (typos) | Validated at creation time with clear error messages; names are short identifiers, not prose |
| More complex quest creation slows down DM chat | Scenarios replace acceptance criteria — they are not additive complexity. The DM already produces descriptions and acceptance; this structures what was freeform |
| Solo sequential quests with high difficulty may still fail | Route to highest-tier model; research shows disproportionate returns from model capability on sequential tasks; boss battle review catches failures |
| Party lead ignores scenarios and invents from scratch | Scenarios are injected into the prompt as structured input; lead retains discretion but has concrete material to start from |
| Breaking change to QuestBrief API | Greenfield project — no external consumers. Internal references (mock LLM, E2E tests) updated in the same pass |

## Alternatives Considered

**1. Full Gherkin-style scenarios (Given/When/Then).**
Rejected. Too structured for the DM LLM to produce reliably in chat. The lightweight
format (name + description + skills + depends_on) captures the essential information
without requiring formal syntax. Can be revisited if scenario quality proves
insufficient.

**2. DM classifies decomposability via LLM reasoning.**
Rejected. The classification is derivable from scenario dependencies — a graph property,
not a judgment call. Deterministic classification from the dependency structure is more
reliable and cheaper than an additional LLM call. The DM's job is to identify
dependencies honestly; the system handles routing.

**3. Always default to party quest, let the lead decide to go solo.**
Rejected by research. Sequential tasks degrade 39-70% with multi-agent coordination.
Defaulting to party and relying on the lead to recognize "I should do this alone" adds
a coordination step that the research shows actively hurts performance. The system
should make the right call before a party is formed.

**4. Separate "plan" chat mode for structured specs.**
Considered. ADR-001 planned a `plan` mode. However, quest specs with scenarios are a
natural evolution of the existing `quest` mode, not a separate workflow. Adding a third
mode fragments the UX. The quest mode prompt can teach scenario thinking directly.

**5. Require the user to classify parallel vs sequential.**
Rejected. Users should describe what they want; the system should figure out how to
staff it. Scenario dependencies are the natural expression of sequentiality — the user
declares them as part of describing the work, not as a separate routing decision.

## Relationship to Other ADRs

- **ADR-001** (DM Chat Routing) — quest mode prompt evolves to teach scenario-driven
  specs. The planned `plan` mode is subsumed by structured quest specs.
- **ADR-002** (Party Quest DAG) — party lead decomposition now receives structured
  scenarios as input material instead of freeform descriptions.
- **ADR-003** (QuestDAGExec Refactor) — no changes to the DAG execution engine; it
  receives sub-quests as before, but the sub-quests are better structured because the
  lead had scenarios to work from.

## Implementation Phases

1. **Types and validation** — `QuestScenario`, `DecomposabilityClass`,
   `ClassifyDecomposability()`, `ValidateScenarioDependencies()`. Remove old
   `Description`/`Acceptance` fields from `QuestBrief`, replace with `Goal`,
   `Requirements`, `Scenarios`. Update `QuestChainEntry` to match.
2. **DM prompt** — rewrite quest mode instructions with goal → requirements → scenarios
   format, dependency guidance, parallel vs sequential examples.
3. **Quest creation API** — apply classification, set `PartyRequired` from heuristic,
   store scenarios and routing class on quest entity.
4. **Prompt fragments** — update party lead directive with scenario injection; add solo
   agent scenario directive.
5. **Model routing** — add `quest-execution-sequential` capability, wire
   decomposability class into questbridge model resolution.

## Further Reading

- [02-DESIGN.md](../02-DESIGN.md) — Trust tiers and quest lifecycle
- [03-QUESTS.md](../03-QUESTS.md) — Quest creation, difficulty, XP, boss battles
- [adr/001-dm-chat-routing.md](001-dm-chat-routing.md) — DM chat modes and quest creation
- [adr/002-party-quest-dag-execution.md](002-party-quest-dag-execution.md) — Party lead decomposition
- Google Research, "Towards a Science of Scaling Agent Systems" (arxiv 2512.08296)
