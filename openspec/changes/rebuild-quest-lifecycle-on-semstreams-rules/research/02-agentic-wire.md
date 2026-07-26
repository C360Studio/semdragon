# Research 02 — Agentic Wire Contract / agentrun / Payload Registry (beta.21 → beta.132)

**Cluster:** agentic-loop wire contract, `agentrun` substrate, payload registry.
**Scope:** `processor/questbridge`, `processor/questtools` (and `processor/questdagexec` where it shares the wire).
**Method:** read authoritative beta.132 source at `/Users/coby/Code/c360/semstreams` (checked out `v1.0.0-beta.132-2-g4ab25867`); diffed `git v1.0.0-beta.21 .. v1.0.0-beta.132`; cross-checked every `agentic.*` symbol semdragons references.

## TL;DR (headline for task 1.6 and the change owner)

- **The agentic wire contract is backward-compatible.** Every change to `TaskMessage`, `ToolCall`, `ToolResult`, `ToolDefinition`, and the four loop events between beta.21 and beta.132 is **purely additive** (`omitempty` fields). No field was removed or renamed; no NATS subject changed; the AGENT stream is unchanged. semdragons' current construction and consumption compile and interoperate as-is.
- **The payload-registry surface is unchanged.** semdragons already adopted the beta.18 DI pattern (`payloadregistry.New()` + `payloadbuiltins.Register(reg)` + `message.NewDecoder(deps.PayloadRegistry)`). Nothing to change here.
- **ADR-053 `agentrun` shipped** in beta.132 (the ADR header still reads "Proposed" — stale; the code landed 2026-06-14). It is an **opt-in library**, wired only in `cmd/semstreams` and `cmd/e2e-semstreams`, requiring a `lifecycle.Manager`. semdragons has **zero** references to it.
- **Recommendation for 1.6: KEEP QUEST_LOOPS.** `agentrun` does not subsume it — they solve different problems (run-tree lineage/phase vs. quest↔loop↔agent↔sandbox correlation). Adopting `agentrun` is a *new feature* (first-class run lifecycle for party-DAG decompositions), not a migration forced by this upgrade. ADR-053 itself lists "legacy migration of … semdragon" under **Deferred (non-goals)**.

---

## 1. What ADR-053 `agentrun` introduces — and QUEST_LOOPS vs agentrun

### 1.1 What it is (plain terms)

A coordinator loop that spawns research/architect/builder child loops forms a **run**: a tree of nested agentic loops. Before ADR-053 the framework modeled the loop *tree* (`agent.loop.parent` triples) but had **no first-class entity for the run itself**; every consumer re-derived run identity by walking ancestry. `agentrun` adds that missing primitive.

Source: `agentic/agentrun/agentrun.go` (exported surface):

```go
type AgentRun struct {                 // a lifecycle.Participant, workflow="agent-run"
    EntityIDField      string          // FULL 6-part: org.platform.agent.chain.execution.<rootLoopID>
    PhaseField         string          // dispatched → executing ⇄ awaiting_approval → terminal
    ParentRunEntityID  string
}
func (r *AgentRun) EntityID() string        { return r.EntityIDField }
func (r *AgentRun) Workflow() string        { return WorkflowName } // "agent-run"
func (r *AgentRun) RunID() (string, bool)   { ... }                 // bare run loop-id, derived
func Register(mgr *lifecycle.Manager) error                        // registers the workflow
func Mint(ctx, mgr MintableManager, org, platform, rootLoopID string) (*AgentRun, error)
func ResolveRun(ctx, mgr, reader LoopTripleReader, org, platform, loopID string) (*AgentRun, error)
type MilestoneSubscriber struct { ... }   // subscribes agent.complete.*/agent.failed.*, demuxes, resolves run
```

Key properties:
- The run entity lives in `ENTITY_STATES` (low write-rate: phase transitions + audit). **Loops stay in `AGENT_LOOPS`** — unchanged (ADR-049 exception).
- Run identity now propagates on the wire as a typed **`RunID`** (bare run loop-id) plus **`RunEntityID`** (6-part chain entity), set at spawn and stamped as an `agent.run` triple. It is threaded onto `TaskMessage`, all four loop events, and `ToolCall.Metadata` (keys `agent.run_id` / `agent.run_entity_id`).
- It reuses the ADR-047 Lifecycle harness wholesale:
  `Manager.Create/Get/Transition/Complete/List/Watch/History/Children`,
  operator gateway (mounted by Semdragon at
  `GET /lifecycle-gateway/workflows`), restart recovery, and rule integration
  (`RunScope: "new"|"inherit"|"none"` on `rule.Action`).

It is **opt-in**: `cmd/semstreams/main.go:215` and `cmd/e2e-semstreams/main.go:182` call `agentrun.Register(svcDeps.LifecycleManager)` + `agentrun.NewMilestoneSubscriber(...)`. A binary that does not wire the lifecycle harness gets none of it, and every wire field stays empty (back-compat).

### 1.2 What semdragons' QUEST_LOOPS is

`processor/questbridge/component.go:70-78` — a domain correlation map persisted in the `QUEST_LOOPS` KV bucket for crash recovery:

```go
type QuestLoopMapping struct {
    LoopID     string           // agentic loop id
    QuestID    domain.QuestID
    AgentID    domain.AgentID
    SandboxDir string
    TrustTier  domain.TrustTier
    StartedAt  time.Time
    LoopType   string           // "execution" | "review" | "clarify" | "explore"
}
```

Keyed by **questID** for execution loops and by **loopID** for review/clarify/explore loops (component.go:65-69). Purpose (from usage): correlate `agent.complete.*` / `agent.failed.*` events back to a quest+agent, recover in-flight loops after a crash, find the active loop for the cancel API (`FindActiveLoop`, component.go:423), and drive sandbox cleanup.

### 1.3 Does agentrun subsume QUEST_LOOPS?

**No.** They are orthogonal:

| Dimension | `agentrun.AgentRun` (framework) | `QUEST_LOOPS` (semdragons) |
|---|---|---|
| Keyed by | 6-part chain-execution entity ID (run root loop) | questID / loopID |
| Carries | Phase, ParentRunEntityID, milestone triples | AgentID, TrustTier, SandboxDir, LoopType, StartedAt, QuestID↔LoopID |
| Answers | "which *run* (loop tree) does this loop belong to; what phase is it in" | "which quest/agent/sandbox is this loop; is it recoverable; is it cancellable" |
| Granularity | one per coordinator **run** (tree) | one per **loop** (execution/review/clarify/explore) |
| Storage | `ENTITY_STATES` via Lifecycle Manager (CAS) | dedicated `QUEST_LOOPS` bucket (History:5) |
| Requires | `lifecycle.Manager` wired + rule-engine `RunScope` | nothing beyond NATS KV |

`AgentRun` carries **none** of the fields QUEST_LOOPS exists to hold (agent id, trust tier, sandbox dir, loop type, per-loop quest reverse-lookup). It cannot replace the bucket.

There is exactly one place where the *concepts* overlap: semdragons' **party DAG** (`questdagexec`) — a parent quest decomposed into sub-quest loops — is precisely a "coordinator spawns child loops" run. If semdragons ever wants run-level lifecycle + milestone aggregation + operator visibility for party quests, `agentrun` is the natural primitive. But that is **net-new capability**, not a QUEST_LOOPS migration.

### 1.4 Recommendation (feeds task 1.6)

**Keep QUEST_LOOPS. Do not adopt `agentrun` as part of this upgrade.**

Reasoning:
1. **No subsumption.** QUEST_LOOPS is a per-loop domain correlation map; `AgentRun` is a per-run phase entity. Different keys, different payloads, different cardinality.
2. **Zero pressure from the upgrade.** The wire contract is additive-only; QUEST_LOOPS works unchanged against beta.132. Nothing forces a change.
3. **agentrun is opt-in and heavy.** Adopting it requires wiring the ADR-047 `lifecycle.Manager` into `cmd/semdragons` and expressing spawns through rule-engine `RunScope` — a substantial architectural addition, not a dependency bump.
4. **The framework agrees.** ADR-053 §Deferred (non-goals) explicitly lists "legacy migration of semspec/**semdragon**/semsage (prior art)" as out of scope.

**Optional future path (separate change, not this one):** if party-DAG runs should become first-class, semdragons can *additionally* (a) populate `TaskMessage.RunID` when questbridge/questdagexec spawn sub-quest loops, and (b) run an `agentrun.MilestoneSubscriber` for the party-run entity — while keeping QUEST_LOOPS for per-loop execution bookkeeping. The two coexist cleanly; `RunID` is `omitempty` and ignored when empty.

---

## 2. TaskMessage / ToolCall / ToolResult contract changes (old vs new)

All three types moved **additively**. Below, the new fields are the entire delta; every pre-existing field kept the same name, type, and JSON tag.

### 2.1 `TaskMessage` (`agentic/user_types.go:262`)

New fields added since beta.21 (all `omitempty`, all optional):

```go
// RunID is the 6-part-derived run anchor: the run loop-id this loop belongs to.
// Empty for loops not in a run. Inherited at spawn (ADR-053 D7).
RunID string `json:"run_id,omitempty"`

// InReplyTo is the bare loop-id this task is a reply to (gh#256).
InReplyTo string `json:"in_reply_to,omitempty"`

// ResponseFormat constrains the model's output to JSON / JSON-schema (ADR-034).
ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
```

Unchanged fields questbridge already populates (`processor/questbridge/handler.go:315-338`): `TaskID`, `LoopID`, `Role`, `Model`, `Prompt`, `Context *types.ConstructedContext`, `Tools []ToolDefinition`, `ToolChoice *ToolChoice`, `Metadata map[string]any`. **`TaskMessage` has no `MaxIterations` field** in either version — the loop ceiling is set by `agentic-loop.max_iterations` config, matching the existing semdragons note.

- **What questbridge MUST populate:** nothing new is required. Its current construction is complete and valid (`TaskMessage.Validate()` requires only `TaskID`, `Role`, `Model`, `Prompt`).
- **What questbridge MAY populate (optional, future):** `RunID` (only if adopting agentrun — see §1.4); `ResponseFormat` (if a quest wants structured output).

### 2.2 `ToolCall` (`agentic/tools.go:73`) — **unchanged**

```go
type ToolCall struct {
    ID         string         `json:"id"`
    Name       string         `json:"name"`
    Arguments  map[string]any `json:"arguments,omitempty"`
    Metadata   map[string]any `json:"metadata,omitempty"`  // domain context, propagated from task
    LoopID     string         `json:"loop_id,omitempty"`
    TraceID    string         `json:"trace_id,omitempty"`
    ApprovedBy string         `json:"approved_by,omitempty"`
}
```

No field delta versus beta.21. questtools consumes it exactly as today (`processor/questtools/handler.go:69` — `baseMsg.Payload().(*agentic.ToolCall)`; metadata reconstruction at handler.go:239-250 keys off `agent_id`/`trust_tier`/`skills`/`quest_id`/`sandbox_dir` — all still present).

Note the framework now *stamps additional metadata keys* on `ToolCall.Metadata` when a loop is in a run: `MetadataKeyRunID = "agent.run_id"` (tools.go:177) and `MetadataKeyRunEntityID = "agent.run_entity_id"` (tools.go:192). These are additive map entries — questtools' `buildContextFromMetadata` ignores unknown keys, so no impact.

### 2.3 `ToolResult` (`agentic/tools.go:366`)

One new field added since beta.21 (`omitempty`, optional):

```go
// ResultHint recommends a follow-up action when the call worked but the
// agent should refine (structured sibling of ErrorKind).
ResultHint ToolResultHint `json:"result_hint,omitempty"`
```

Everything questtools sets is unchanged: `CallID`, `Name`, `Content`, `Error`, `ErrorKind`, `Metadata`, `LoopID`, `TraceID`, `StopLoop`. `ToolResult.Validate()` still requires only `CallID`.

- **What questtools MUST emit:** unchanged. Current `publishResult` / `publishResultDetached` (handler.go:204-237) remain correct.
- **What questtools MAY emit (optional):** `ResultHint` to nudge the model toward a refine-and-retry on a successful-but-underspecified call (e.g., `graph_search` returned too many hits). Not required.

### 2.4 `ToolDefinition` (`agentic/tools.go:22`)

Two new fields since beta.21 (`omitempty`): `Strict bool` (OpenAI strict-mode tool calling) and `Paginated bool` (declares continuation-paging support). Both optional; `toolsForQuest` (`questbridge/handler.go:2509`) needs no change unless semdragons opts into strict schemas.

### 2.5 Loop events (`agentic/events.go`) — additive

`LoopCreatedEvent`, `LoopCompletedEvent`, `LoopFailedEvent`, `LoopCancelledEvent` each gained (ADR-053 D8), all `omitempty`:

```go
RunID       string `json:"run_id,omitempty"`        // bare run loop-id
RunEntityID string `json:"run_entity_id,omitempty"` // 6-part chain execution entity id
```

`LoopFailedEvent` / `LoopCancelledEvent` also carry `ParentLoopID` for ancestry walks. questbridge decodes `*LoopCompletedEvent` / `*LoopFailedEvent` / `*LoopCancelledEvent` (handler.go:522, 606, and the cancelled fallback at 525) — all still resolve via the payload registry; the new fields are simply available and currently unread.

### 2.6 BaseMessage wrapping — unchanged

Both sides still wrap payloads in a `message.BaseMessage`:
- Publish: `message.NewBaseMessage(msgType Type, payload Payload, source string, opts ...Option)` (`message/base_message.go:121`) — signature unchanged. (Note: `WithFederation`'s param type changed `config.PlatformConfig → platform.Config`, but semdragons uses **no** options and does not call `WithFederation`.)
- Decode: `message.NewDecoder(reg *payloadregistry.Registry)` (`message/decoder.go:33`) → `decoder.Decode(data) (*BaseMessage, error)` → `baseMsg.Payload()`. Unchanged.

---

## 3. Payload registry changes — none required

The DI pattern semdragons already runs is exactly the beta.132 shape. Confirmed no diff in `payloadregistry/` or `payloadbuiltins/` between beta.21 and beta.132.

Registration API (`payloadregistry/registry.go`):

```go
func New() *Registry
func (pr *Registry) Register(registration *Registration) error
type Registration struct {
    Factory     Factory        // func() any  — required
    Builder     Builder        // optional; JSON fallback used when nil
    Domain, Category, Version, Description string
    Example     map[string]any
}
```

Aggregators still present and unchanged:
- `payloadbuiltins.Register(reg *payloadregistry.Registry) error` (`payloadbuiltins/register.go:40`) — registers `message`, `oms`, `agentic`, `research`, `agentic-dispatch`, `gated-dag`, `github-webhook`, `objectstore`, `governance`.
- `agentic.RegisterPayloads(reg)` (`agentic/payload_registry.go:19`) registers `TaskMessage`, `ToolCall`, `ToolResult`, `AgentResponse`, `LoopCreated/Completed/Failed/Cancelled`, `ContextEvent`, approval events — i.e. **every payload questbridge/questtools decode.**
- Test helpers unchanged: `payloadbuiltins.NewTestRegistry(tb)` (testing.go:20), `payloadbuiltins.NewTestDecoder(tb)` (testing.go:35).
- `component.Dependencies.PayloadRegistry *payloadregistry.Registry` (`component/dependencies.go:74`) — same field the semdragons `register.go` files read (`deps.PayloadRegistry`).

semdragons' `buildPayloadRegistry()` (`cmd/semdragons/main.go:391-400`) and per-component `message.NewDecoder(deps.PayloadRegistry)` calls need **no change**.

**Caveat (not a change, a watch-item):** there is no new agentrun *payload* type — the run entity is a Lifecycle `Participant` in `ENTITY_STATES`, not a bus payload — so no additional registration is needed even if semdragons later adopts agentrun.

---

## 4. Subject / topic changes — none

The AGENT-stream subject map is identical to beta.21 (`agentic/doc.go:261-270`, `processor/agentic-loop/config.go:382-483`, `agentic/agentrun/agentrun.go:655` `AgentStreamName = "AGENT"`):

| Direction | Subject | semdragons side |
|---|---|---|
| questbridge → loop | `agent.task.<loopToken>` | publish, handler.go:362 (loop consumes `agent.task.*`) |
| loop → questtools | `tool.execute.<name>` | consume `tool.execute.*`, questtools handler.go:35 |
| questtools → loop | `tool.result.<callID>` | publish, questtools handler.go:207 (loop consumes `tool.result.>`) |
| loop → questbridge | `agent.complete.<loopID>` | consume `agent.complete.*`, handler.go:447 |
| loop → questbridge | `agent.failed.<loopID>` | consume `agent.failed.*`, handler.go:447 |

Completion subjects are built via `component.ResolveSubject(..., "agent.complete", loopID)` (`agentic-loop/handlers.go:1740`) → `agent.complete.<loopID>` (single token). semdragons' loop IDs are `quest-<dot-free questID>-<nuid>` (handler.go:314) — dot-free, so a single NATS token that matches the `agent.complete.*` / `agent.failed.*` single-`*` filters. **No subject rework needed.** The AGENT stream still spans `agent.>` + `tool.>`.

---

## 5. Impact list for semdragons (concrete file:line)

**Net effect: no breaking impact on the agentic wire surface. All sites below compile and interoperate unchanged against beta.132.** Listed for the change owner to verify and to flag the optional-adoption sites.

### questbridge
- `processor/questbridge/handler.go:315-338` — `TaskMessage` construction. **OK as-is.** Optional: set `RunID` (only if adopting agentrun) / `ResponseFormat`.
- `processor/questbridge/handler.go:351` — `message.NewBaseMessage(taskMsg.Schema(), &taskMsg, "questbridge")`. **Unchanged API.**
- `processor/questbridge/handler.go:362-363` — publish `agent.task.<id>` via `PublishToStream`. **Unchanged.**
- `processor/questbridge/handler.go:371-391` + `component.go:99, 357-366` — QUEST_LOOPS bucket + `QuestLoopMapping`. **Keep (see §1.4).**
- `processor/questbridge/handler.go:445-449` — `jetstream.ConsumerConfig{FilterSubjects: ...}`. This is the **nats.go** `jetstream` type (not `natsclient.StreamConsumerConfig`), so it is independent of the semstreams bump. **Unchanged.**
- `processor/questbridge/handler.go:515-525, 599-606` — decode `LoopCompletedEvent` / `LoopFailedEvent` / `LoopCancelledEvent` via `baseMsg.Payload()`. **Unchanged**; new `RunID`/`RunEntityID` fields available but unread.
- `processor/questbridge/component.go:88-91` — comment ("beta.18 retired the payloadregistry singleton"). **Cosmetic:** refresh wording when bumping.
- `processor/questbridge/register.go:53, 90` — `message.NewDecoder(deps.PayloadRegistry)`. **Unchanged.**

### questtools
- `processor/questtools/handler.go:32-40` — `natsclient.StreamConsumerConfig{... FilterSubject: "tool.execute.*"}`. **Unchanged** (singular `FilterSubject` field still present, beta.132 `natsclient/stream.go:27`).
- `processor/questtools/handler.go:52, 69` — `decoder.Decode` → `baseMsg.Payload().(*agentic.ToolCall)`. **Unchanged.**
- `processor/questtools/handler.go:148, 174-188` — `toolRegistry.Execute` → set `LoopID`/`TraceID` → `publishResultDetached`. **Unchanged.** Optional: set `result.ResultHint`.
- `processor/questtools/handler.go:204-237` — `publishResult` / `publishResultDetached` (`tool.result.<callID>`, detached 5s ctx — the wedge-prevention contract). **Unchanged.** Preserve the detached-ctx behavior.
- `processor/questtools/explore.go:112, 140, 178` — explore child-loop `TaskMessage` publish + `jetstream.ConsumerConfig{FilterSubjects: ...}` (again nats.go type). **Unchanged.**
- `processor/questtools/component.go:33-34` — comment ("beta.18 retired the payloadregistry"). **Cosmetic** refresh.
- `processor/questtools/register.go:47, 84` — `message.NewDecoder(deps.PayloadRegistry)`. **Unchanged.**

### questdagexec (shares the wire)
- `processor/questdagexec/events.go:257` — `jetstream.ConsumerConfig{FilterSubjects: ["agent.complete.*","agent.failed.*"]}` (nats.go type). **Unchanged.**
- `processor/questdagexec/component.go:92` — payloadregistry comment. **Cosmetic.**

### binary / tests
- `cmd/semdragons/main.go:391-400` — `buildPayloadRegistry()` (`payloadregistry.New()` + `payloadbuiltins.Register` + `semsource.RegisterPayloads`). **Unchanged.**
- `processor/{questbridge,questtools,questdagexec}/*_test.go` — `payloadbuiltins.NewTestRegistry(t)` / `NewTestDecoder(t)`. **Unchanged.**

### Out of scope (other research clusters)
Non-agentic breaking changes across the ~111 intermediate tags (e.g., `component.Dependencies`, `service.Manager`, `GraphClient`, `natsclient` beyond the wire methods checked here, `model.RegistryReader`) are owned by sibling tasks. Within **this** cluster, the only genuinely breaking framework signature change touching an imported package is `message.WithFederation`/`WithFederationAndTime` (param `config.PlatformConfig → platform.Config`) — and semdragons does not call either.

---

## Appendix — verification commands run

- `git -C semstreams describe --tags` → `v1.0.0-beta.132-2-g4ab25867`
- `git -C semstreams diff v1.0.0-beta.21 v1.0.0-beta.132 -- agentic/user_types.go agentic/tools.go` → additive only
- Enumerated all 25 `agentic.*` symbols semdragons references; **0 missing** in beta.132.
- `git diff v1.0.0-beta.21 v1.0.0-beta.132 -- payloadregistry/ payloadbuiltins/ pkg/types/context.go natsclient/stream.go` → no signature/field deltas on the surfaces semdragons uses.
- Confirmed migration docs only run through `migration-beta68-to-beta70.md`; there is **no** framework migration doc for beta.70 → beta.132, so this file is the authoritative record for the agentic-wire delta.
