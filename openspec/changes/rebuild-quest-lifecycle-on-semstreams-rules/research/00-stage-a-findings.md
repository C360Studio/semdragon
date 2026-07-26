# Stage A findings — beta.21 → beta.132 migration impact on semdragons

Task 1.1 deliverable. Consolidates four cluster deep-dives (files 01–04 in this dir)
into a single list of every upstream change that touches semdragons, with a verdict
per item. Source of truth for the migration; the four cluster files carry the quoted
signatures and file:line evidence.

## Headline

**The beta.21 → beta.132 bump is near-zero compile risk.** Across all four risk
clusters (graph/ownership, agentic wire, general framework surfaces, lifecycle/gated-dag),
every upstream change to a package semdragons imports is either additive (`omitempty`
fields, keyed-literal-safe struct growth) or lands in a package semdragons does not import.
The framework held backward compatibility on the consumed surface across all 111 releases.

The real work is **Stage B** (architectural swaps) and the **ownership/predicate
partitioning** (behavioral, not a compile blocker). Two *runtime* behaviors need
re-verification (see "Runtime re-verify" below).

## Items touching semdragons

### Compile-level (Stage A / task 1.2, 1.4)

| # | Upstream change | Package | Touches semdragons? | Action |
|---|-----------------|---------|---------------------|--------|
| 1 | Core data types `message.Triple` (299 uses), `graph.EntityState` (254), `graph.Graphable` (80) | message, graph | **No break** — unchanged/additive (`EntityState` gained optional `StorageRef`) | none; `marshalEntityState` still compiles |
| 2 | `graph.EmitEntity*` "hits" (69+26+...) | (semdragons' own `GraphClient`) | **N/A** — calls on a local var named `graph`, not the semstreams pkg | none |
| 3 | Agentic wire: `TaskMessage`, `ToolCall`, `ToolResult`, `ToolDefinition`, loop events | agentic | **No break** — all additive `omitempty` (`RunID`, `InReplyTo`, `ResponseFormat`, `ResultHint`, `Strict`, `Paginated`) | none; questbridge/questtools build unchanged |
| 4 | Payload registry (beta.18 DI pattern) | payloadregistry, payloadbuiltins | **No break** — identical | none |
| 5 | Component registry: `Factory`, `RegisterWithConfig`, `GenerateConfigSchema`, `Discoverable`, `LifecycleComponent` | component | **No break** — unchanged; `Dependencies` grew fields but all constructions use keyed literals | none |
| 6 | `service.Service` grew `Health()`/`RegisterMetrics()` | service | **No break** — `service/api.Service` embeds `*service.BaseService`, inherits them | none |
| 7 | Test harness `NewTestClient`/`WithKV`/`WithKVBuckets`/`TestStreamConfig` | natsclient | **No break** — already beta.132-shaped; semdragons already does `client := testClient.Client` | none |
| 8 | `errs.Wrap` (4-arg, 110 uses), `config.New*Manager`, `model.EndpointConfig/Registry`, `metric`, `vocabulary`, `pkg/types`, `pkg/context` | various | **No break** — signatures verified identical | none |
| 9 | `message.WithFederation` signature change (`config.PlatformConfig` → `platform.Config`) | message | **No** — semdragons never calls it | none |

### Runtime re-verify (Stage A gate / task 1.7 — behavioral, no compile signal)

| # | Upstream change | ADR | Risk to semdragons | Verify how |
|---|-----------------|-----|--------------------|------------|
| R1 | natsclient retired dual error channel → header-classified RPC error replies | ADR-060 (`672d3e5d`) | `service/api/dm_session.go:218` assumes plaintext error bytes | integration test / runtime check of DM session error path |
| R2 | `triple.add`/`add_batch` lost auto-vivify — triple to absent entity now rejected `ErrorCodeEntityNotFound` | ADR-055 (`270ed60e`) | **Low** — semdragons writes full-object `Put`, never incremental `triple.add`; entities are born whole | confirm no `triple.add` path; covered by integration tests |
| R3 | `vocabulary.Register` now amends instead of replaces on re-register | (`df2f19ff`) | **None** — strictly safer | no action |

### Ownership / architecture (Stage B — behavioral, feeds task 1.3 spike + 2.x)

| # | Finding | ADR | Feeds |
|---|---------|-----|-------|
| O1 | semdragons is a second, **unregistered full-object writer** to `ENTITY_STATES` (`domain/config.go:161` `BucketName()="ENTITY_STATES"`). Five components (questboard/questbridge/questdagexec/bossbattle/service-api) read-modify-write the whole quest struct and `Put`, clobbering each other's `quest.*` predicate groups. | ADR-056 | 1.3, 2.x |
| O2 | **Ownership unit is the predicate group**, not the entity. Claims `(pattern, predicate set, write mode, owner id)` live in `OWNER_CLAIMS`, overlap-checked at registration; runtime enforcement currently observe-only (`OwnerToken` stamped, not compared) → adoptable incrementally. | ADR-056 | 1.3 |
| O3 | **Task 1.3 resolution (proposed):** partition `quest.*` into disjoint owned groups — lifecycle owner gets `quest.status.state` (`cas-transition`) + `quest.lifecycle.*`; questbridge gets `quest.execution.*`+`quest.context.*` (`replace-owned`) and switches from full-object `Put` to `update_with_triples`. Double-claiming the phase predicate fails registration by design. | ADR-056/049 | 2.x |
| O4 | Lifecycle Manager writes via **graph-ingest triple mutations with `ExpectedRevision` CAS** (ADR-049), not raw KV `Put`. So adopting the Manager for phase writes (Stage B) requires questbridge to stop full-object `Put` on the phase predicate. | ADR-049 | 2.x |
| O5 | Boot ordering: `ownership.EnsureBuckets` must run in `cmd/semdragons/main.go` **before** component `Register`; then `projection.Bind` per owner. | ADR-056 | 4.x |
| O6 | ADR-055 update lane `AddTriples` is **replace-by-(subject,predicate)**, not append — the opposite of semdragons' full-object `Put`. Relevant when moving to `update_with_triples`. | ADR-055/049 | 2.x |

## Decisions locked by this research

- **Task 1.6 — QUEST_LOOPS vs agentrun: KEEP QUEST_LOOPS, do not adopt agentrun** in this
  change. They are different cardinality (per-loop correlation map carrying
  `AgentID`/`TrustTier`/`SandboxDir`/`LoopType` vs per-run Lifecycle Participant). The wire
  is additive-only so QUEST_LOOPS works unchanged; agentrun is opt-in/heavy (needs a
  `lifecycle.Manager` in `cmd/semdragons` + rule-engine `RunScope`); ADR-053 §Deferred lists
  "legacy migration of semdragon" as out of scope. Future coexistence path: set
  `TaskMessage.RunID` (omitempty) on sub-quest spawns + run an `agentrun.MilestoneSubscriber`
  — separate change. (Detail: `02-agentic-wire.md`.)

- **Task 2.3 — attempt-conditional retry-vs-triage routing IS rule-expressible** (no Manager
  escape hatch needed): store `retry_count` as an `operator_writable` triple, increment it in
  the fail→requeue `lifecycle_transition.set`, branch on `lt max` (retry) vs `gte max`
  (pending_triage) using the wired `gt/gte/lt/lte` rule operators. (Detail:
  `04-lifecycle-gateddag.md`.)

## Stage B primitives (reference for tasks 2.x/3.x)

- **Workflow declaration**: `lifecycle.Workflow{Name, EntityIDPattern, Phases,
  Transitions map[string][]string, PhasePredicate, Schema: reflect.TypeOf(QuestState{}),
  OperatorWritablePredicates, AuditPredicates}` + `Manager.Register(wf)`. `QuestState`
  implements `Participant` (EntityID/Workflow/Phase/IsTerminal/ParentEntityID) with
  `lifecycle:"phase,predicate=quest.status"` etc. struct tags. Terminal phase = empty
  transition slice. `gateddag.FanOut` is the tightest real example in-repo.
- **Transition API**: `TransitionWith(ctx, wf, id, newPhase, source, note, mutator)` —
  5-try CAS loop, writes phase+audit(+mutator fields) as one replace-not-append mutation.
  `History()` → `[]TransitionEvent` with source attribution.
- **Rule actions**: `lifecycle_transition` (phase + optional `set` ops), `lifecycle_complete`,
  `lifecycle_fail`; `set` keys default-deny via `AssertRuleWritable`. Rules load from JSON,
  seed into KV `rules.*`, hot-reload via debounced `Watch("rules.*")`.
- **gated-dag handler contract**: executor publishes reference-only
  `DispatchMessage{UnitEntityID, FanOutWorkflow}` (BaseMessage-wrapped) to `DispatchSubject`
  when a unit is dispatchable; consumer reads the unit from the graph, does the work, and
  **writes the `Completed`/`Failed` marker** (the marker write is the DAG-advancing signal —
  no ack/return contract). Claim marker committed BEFORE dispatch (exactly-once). Per-unit
  state (depends_on/markers/claim) lives as triples under `UnitEntityPrefix` — semdragons
  re-derives `quest.dag.*` by reading them.
- **Known gated-dag risks**: `Fail` hard-codes phase `"failed"` (escalated/pending_triage
  need explicit transitions); no orphan-node auto-recovery (gh#373) so the consumer must write
  completion markers at-least-once; retry counter is unavoidably DM-patchable; forward-additive
  schema only.
- **lifecycle-gateway** (operator surface, task 4.3): Semdragon mounts
  `GET/POST /lifecycle-gateway/workflows/{type}/{id}[/history|children|state|transition]`,
  backed by the shared `Manager`. WebSocket support exists upstream but is
  disabled in every shipped Semdragon profile.

## Follow-up housekeeping (not blocking)

- Root `CLAUDE.md` "Message Logger" example references `natsclient.WithMessageLogger`, which
  does not exist at beta.132 — documentation bug; refresh when convenient.
