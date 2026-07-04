# Research 04 — `pkg/lifecycle`, rule-engine lifecycle actions, `processor/gated-dag`, and the lifecycle-gateway

**Source:** `github.com/c360studio/semstreams` @ `v1.0.0-beta.132-2-g4ab25867`
(local checkout `/Users/coby/Code/c360/semstreams`). All signatures below are
copied verbatim from that tree; no code was modified.

**Governing ADRs:** ADR-047 (harness substrate, *superseded* on the bucket
question), **ADR-049 (the live design — schema-over-ENTITY_STATES)**, ADR-046
(gated-DAG dispatch component), ADR-064 (prepare-as-a-unit).

**What semdragons replaces with this cluster**
- Hand-rolled quest state machine (`processor/questboard`) → a declared
  `lifecycle.Workflow` (phases, transition table, phase predicate, atomic
  transition-coupled writes, audit history, operator gateway).
- Single-goroutine DAG loop (`processor/questdagexec`) → the `gated-dag`
  executor component (dependency ordering, claim-before-dispatch, restart
  recovery, stall detection, per-unit reset/retry).

The single most important architectural fact: **the harness owns no storage.**
Every phase/audit/field write is emitted as triples into `ENTITY_STATES` through
graph-ingest's `update_with_triples` / `create_with_triples` mutation subjects,
with CAS via `ExpectedRevision`. That means quest state is natively graph-visible
(GraphQL, rule conditions, inference all see `quest.status`) and the audit trail
IS the KV revision history — there is no private quest bucket to reconcile.

---

## 1. `pkg/lifecycle` Workflow declaration recipe

A workflow is declared by constructing a `lifecycle.Workflow` value and passing
it to `Manager.Register`. The state struct is a plain Go struct implementing the
4-method `Participant` interface, annotated with `lifecycle:` field tags.

### 1a. The `Workflow` declaration struct (`pkg/lifecycle/workflow.go`)

```go
type Workflow struct {
    Name            string          // workflow type id; must == Participant.Workflow(). REQUIRED
    EntityIDPattern string          // 6-part glob, e.g. "*.*.game.board1.quest.*". REQUIRED, validated ==6 segments
    Phases          []string        // informational; the authoritative graph is Transitions
    Transitions     Transitions     // the phase graph. REQUIRED, structurally validated
    PhasePredicate  string          // triple predicate carrying current phase, e.g. "quest.status". REQUIRED
    Schema          reflect.Type    // reflect.TypeOf(QuestState{}) — projection allocates via reflect.New. REQUIRED

    OperatorWritablePredicates []string    // default-deny allowlist for operator/rule patches
    AuditPredicates            AuditSpec    // framework-stamped source/at/from/note predicates
    ChildWorkflows             []ChildSpec  // owned child workflow links (parent→child triples)
    ReferencePredicates        []ReferenceSpec // non-owning cross-entity links
}

type AuditSpec struct {                 // stamped atomically on every Transition
    Source string // e.g. "quest.last_transition_source"  → "rule"|"operator"|"component"|"framework"
    At     string // e.g. "quest.last_transition_at"      → RFC3339Nano
    From   string // e.g. "quest.last_transition_from"     → previous phase
    Note   string // e.g. "quest.last_transition_note"     → free-text reason
}

type ChildSpec     struct { Workflow, LinkPredicate string }  // cardinality-many
type ReferenceSpec struct { Predicate, TargetPattern string }
```

### 1b. The transition table (`pkg/lifecycle/transitions.go`)

```go
type Transitions map[string][]string      // from-phase → allowed to-phases; terminal = empty slice

func (t Transitions) Validate() error                       // empty table / undeclared out-edge / dup out-edge → ErrInvalidTransitionsTable
func (t Transitions) IsTerminal(phase string) bool          // no out-edges (or undeclared) ⇒ terminal
func (t Transitions) IsValidTransition(from, to string) bool
func (t Transitions) Phases() []string
func (t Transitions) TerminalPhases() []string
```
`Validate` does **not** check reachability or forbid cycles (cycles are legal —
e.g. `in_review → in_progress → in_review`). Every out-edge target must appear as
a key, so terminal phases must be declared with `{}`.

### 1c. The `Participant` interface + field tags (`participant.go`, `tags.go`)

```go
type Participant interface {
    EntityID() string        // 6-part federated ID
    Workflow() string        // must match Workflow.Name
    Phase() string           // must be a key in Transitions
    IsTerminal() bool         // by convention: Transitions.IsTerminal(Phase())
    ParentEntityID() string   // "" for roots
}
```

Field-tag grammar (`lifecycle:"..."`, comma-separated), parsed at Register:
| tag token | meaning |
|---|---|
| `id` | marks EntityID field (exactly one; implicitly readonly) |
| `phase,predicate=quest.status` | marks phase field; **predicate is REQUIRED on the phase field** |
| `operator_writable,predicate=quest.foo` | opt-in to operator/rule patches; **predicate REQUIRED** |
| `reference,predicate=quest.assigned_agent` | projects the predicate's Object as a scalar entity-ID; readonly, cannot be operator_writable |
| `readonly,predicate=quest.last_transition_at` | projected but never patchable (audit fields) |
| `indexable` | reserved for v2 secondary index; v1 ignores |

Contradictions fail loudly at Register (`id`+`operator_writable`, `reference`+
`operator_writable`, `operator_writable` without predicate, `json:"-"` +
`operator_writable`, duplicate JSON name, duplicate predicate). A many-cardinality
predicate (child-link/reference) colliding with a single-valued predicate
(phase/audit/scalar) also fails — because `Transition` writes `RemoveTriples` for
every delta predicate and would otherwise delete the many-valued triples
(`validateDisjointness`, gh#234).

### 1d. Registration (`manager.go`)

```go
func NewManager(client *natsclient.Client, logger *slog.Logger) *Manager
func (m *Manager) Register(workflow Workflow) error   // ErrWorkflowAlreadyRegistered on dup Name
```
`Register` validates the declaration, reflects the Schema into projection
metadata (cached for the Manager's lifetime), and (if `AttachOwnership` was
called) registers an ADR-056 owner claim. **Ownership is a no-op unless
`AttachOwnership` is wired — for semdragons it can be ignored initially.**

### 1e. Minimal real declared-workflow example

The gated-DAG executor registers its own `FanOut` workflow — the tightest real
example in the tree (`processor/gated-dag/participant.go`):

```go
type FanOut struct {
    EntityIDField string `json:"entity_id" lifecycle:"id"`
    PhaseField    string `json:"phase" lifecycle:"phase,predicate=gateddag.fanout.phase"`
}
func (f *FanOut) EntityID() string       { return f.EntityIDField }
func (f *FanOut) Workflow() string       { return "gateddag-fanout" }
func (f *FanOut) Phase() string          { return f.PhaseField }
func (f *FanOut) IsTerminal() bool       { return Transitions.IsTerminal(f.PhaseField) }
func (f *FanOut) ParentEntityID() string { return "" }

var Transitions = lifecycle.Transitions{
    "dispatching": {"completed", "failed"},
    "completed":   {},
    "failed":      {},
}

func WorkflowDeclaration() lifecycle.Workflow {
    return lifecycle.Workflow{
        Name:            "gateddag-fanout",
        EntityIDPattern: "*.*.gateddag.fanout.instance.*",
        Phases:          []string{"dispatching", "completed", "failed"},
        Transitions:     Transitions,
        PhasePredicate:  "gateddag.fanout.phase",
        Schema:          reflect.TypeOf(FanOut{}),
    }
}
```

The richer test fixture (`manager_test.go` `fixtureWorkflow`) shows the full
audit + operator-writable shape a quest workflow will use:

```go
Workflow{
    Name:            "fixture",
    EntityIDPattern: "*.*.lifecycle.gcs.mission.*",
    Phases:          []string{"planning", "flying", "completed", "aborted", "failed"},
    Transitions: Transitions{
        "planning": {"flying", "aborted"}, "flying": {"completed", "aborted"},
        "completed": {}, "aborted": {}, "failed": {},
    },
    PhasePredicate: "mission.phase",
    Schema:         reflect.TypeOf(fixtureMission{}),
    OperatorWritablePredicates: []string{"mission.owner_org_id", "mission.note"},
    AuditPredicates: AuditSpec{
        Source: "mission.last_transition_source", At: "mission.last_transition_at",
        From: "mission.last_transition_from", Note: "mission.last_transition_note",
    },
}
// fixtureMission fields:
//   ID     string    `json:"entity_id"    lifecycle:"id"`
//   PhaseF string    `json:"phase"        lifecycle:"phase,predicate=mission.phase"`
//   OwnerOrgID string `json:"owner_org_id,omitempty" lifecycle:"operator_writable,predicate=mission.owner_org_id"`
//   Note   string    `json:"note,omitempty"         lifecycle:"operator_writable,predicate=mission.note"`
//   DroneID string   `json:"drone_id,omitempty"     lifecycle:"reference,predicate=mission.assigned_drone"`
//   LastAt time.Time `json:"last_at,omitempty"      lifecycle:"readonly,predicate=mission.last_transition_at"`
```

**Semdragons mapping:** `Name:"quest"`, `EntityIDPattern:"*.*.game.board1.quest.*"`,
`PhasePredicate:"quest.status"`, phases
`posted→claimed→in_progress→in_review→completed|failed|escalated|pending_triage`
(note per CLAUDE.md the predicate index uses `quest.status.<phase>`; `quest.status`
is the single-valued phase triple the harness owns). `claimed_by`, `assigned_party`
etc. become `operator_writable` or `reference` projection fields.

---

## 2. Transition API — atomic coupled state + audit history

### 2a. State-change operations (`manager.go`)

```go
func (m *Manager) Create(ctx, initial Participant) error                          // "add lifecycle dimension"
func (m *Manager) Transition(ctx, workflow, entityID, newPhase string,
                             source TransitionSource, note string) error
func (m *Manager) TransitionWith(ctx, workflow, entityID, newPhase string,
                             source TransitionSource, note string,
                             mutator func(Participant) error) error               // couple field writes to the phase change
func (m *Manager) UpdateFromOperator(ctx, workflow, entityID string, patch map[string]any) error
func (m *Manager) Complete(ctx, workflow, entityID string) error                  // → first terminal reachable from current phase
func (m *Manager) Fail(ctx, workflow, entityID, reason string) error              // → the phase literally named "failed"; reason REQUIRED

// reads
func (m *Manager) Get(ctx, workflow, entityID) (Participant, error)
func (m *Manager) GetWithRevision(ctx, workflow, entityID) (Participant, uint64, error)
func (m *Manager) GetRaw(ctx, entityID) (*graph.EntityState, error)               // bypass projection (debug)
func (m *Manager) List(ctx, workflow, ListOptions) ([]Participant, error)
func (m *Manager) Watch(ctx, workflow) (<-chan Participant, error)                // bootstrap snapshot then live; caller MUST cancel ctx
func (m *Manager) History(ctx, workflow, entityID) ([]TransitionEvent, error)
func (m *Manager) Children(ctx, parentEntityID, ChildOptions) ([]ChildResult, error)
func (m *Manager) References(ctx, entityID) ([]ReferenceStub, error)
func (m *Manager) LookupByEntityID(ctx, entityID) (Participant, error)            // resolves workflow by EntityIDPattern
func (m *Manager) GetWorkflowDefinition(workflow) (WorkflowDef, error)
func (m *Manager) ListWorkflows() []WorkflowDef

type TransitionSource string // "rule" | "operator" | "component" | "framework"
```

### 2b. How the atomic write actually works (the load-bearing detail)

`Transition`/`TransitionWith` (`manager.go:578-704`) run a CAS loop, bound to
`const updateRetries = 5`:

1. `getEntity` reads the entity + current KV revision.
2. Extract current phase from `PhasePredicate`; reject if
   `ErrEntityNotLifecycleManaged` (no phase triple), `ErrTerminalPhase`, or
   `!IsValidTransition(current, newPhase)` → `ErrInvalidTransition`.
3. Build the delta: the new phase triple **+** the four audit triples
   (source/at/from/note) **+**, if `TransitionWith`, the mutator's changed
   projection fields (diffed against current triples, zero-value-missing skipped).
4. Emit **one** `graph.UpdateEntityWithTriplesRequest` with
   `AddTriples=delta`, `RemoveTriples=<every predicate in delta>`,
   `ExpectedRevision=currentRev`. The remove-then-add makes every write
   **replace, not append** — critical because the rule engine's `GetFieldValue`
   reads *first*-match, so an accumulated phase list `[posted, claimed]` would
   make phase guards read the stale first value and never re-fire
   (`TestManager_TransitionReplacesPhaseTripleNotAppend`).
5. On `errs.ErrRevisionMismatch` (a concurrent writer moved the revision), loop
   re-reads and re-validates; after 5 tries → `ErrUpdateRetriesExhausted`.

This is the atomicity guarantee semdragons needs: **phase + coupled fields land
as one graph mutation with optimistic concurrency**, so two agents racing to
`claim` the same quest cannot both win — the loser gets a revision mismatch and
re-validates against the now-`claimed` state.

`TransitionWith` example (from the rule executor, `actions_lifecycle.go:83`):
```go
mgr.TransitionWith(ctx, "quest", entityID, "in_progress",
    lifecycle.TransitionSourceRule, reason,
    func(p lifecycle.Participant) error { return applyLifecycleSet(p, resolved) })
```

### 2c. Audit history (`manager_query.go:189`, `participant.go:111`)

```go
type TransitionEvent struct {
    From      string           `json:"from"`   // "" for the create event
    To        string           `json:"to"`
    At        time.Time        `json:"at"`     // KV revision Created() time — authoritative across restarts
    Triggered TransitionSource `json:"triggered"`
    Note      string           `json:"note,omitempty"`
}
```
`History` replays `bucket.History(entityID)` over `ENTITY_STATES`, decodes each
revision's `EntityState`, and emits an event whenever the `PhasePredicate` value
changed. `Triggered`/`Note`/`From` are recovered from the audit triples stamped
at that revision — so source attribution is real (the "always-`framework`" bug
ADR-049 closed). If no `AuditPredicates` are declared, `From` is reconstructed
from the prior revision and `Triggered` defaults to `framework`. **Consequence
for semdragons: declare `AuditPredicates` to get a truthful quest audit trail
for free — no separate history store.**

---

## 3. Rule-engine lifecycle actions

Three additive rule action types bridge rules → `lifecycle.Manager`, wired via
`ActionExecutor.SetLifecycleManager` / `Processor.lifecycleManager` at component
init. Nil manager ⇒ loud authoring error, never a silent no-op.

### 3a. JSON schema of the actions (fields on `rule.Action`, `actions.go` + `actions_lifecycle.go`)

```jsonc
// lifecycle_transition
{
  "type": "lifecycle_transition",
  "workflow": "quest",          // optional; omitted ⇒ Manager.LookupByEntityID resolves by EntityIDPattern
  "phase": "in_progress",       // required; substitution-resolved; must be a declared transition target
  "reason": "claimed by agent", // optional; becomes TransitionSource note (audit)
  "set": {                       // optional; applied atomically inside the same TransitionWith mutator
    "retry_count": {"op": "increment"},        // typed op
    "claimed_by":  "$entity.triple.claimant"    // literal / substitution-resolved
  }
}

// lifecycle_complete  → Manager.Complete (deterministic first-reachable terminal)
{ "type": "lifecycle_complete", "workflow": "quest" }

// lifecycle_fail      → Manager.Fail (transition to the "failed" phase; reason REQUIRED)
{ "type": "lifecycle_fail", "workflow": "quest", "reason": "boss battle defeat: $entity.triple.verdict" }
```

`set` value grammar (`applyLifecycleSet`): a bare literal (string/number/bool),
`{"op":"set","value":v}`, `{"op":"increment"}` (numeric +1), `{"op":"decrement"}`
(numeric −1, uint-clamped at 0). Every `set` key is pre-validated with
`Manager.AssertRuleWritable` (default-deny): typo'd fields, `id`, `phase`, and any
non-`operator_writable` field are rejected before the write — **so a rule can
never exceed operator authority, and the retry counter must be an
`operator_writable` projection field.** String values (in `phase`, `reason`, and
`set`) are substitution-resolved and loud-fail if a `$entity|related|state|
message.*` token survives (a mismatched trigger context).

### 3b. Conditions (`expression/types.go`, `rule_factory.go`)

A rule fires on a `ConditionExpression` list joined by `logic:"and"|"or"`:
```go
type ConditionExpression struct {
    Field    string      `json:"field"`     // triple predicate, e.g. "quest.command" or "mission.command"
    Operator string      `json:"operator"`  // see below
    Value    interface{} `json:"value"`
    Required bool        `json:"required"`
    From     interface{} `json:"from,omitempty"` // for the "transition" operator
}
```
Wired operators (`evaluator.go:23-51`, confirmed registered):
`eq, neq, lt, lte, gt, gte, in, nin, between, contains, starts_with, ends_with,
regex_match, length_eq, length_gt, length_lt, array_contains`. Conditions also see
substitution state fields: `$state.iteration`, `$state.max_iterations`,
`$state.last_transition`, `$prev.<field>`, and
`$entity.lifecycle.phase|terminal|workflow` (populated by
`PopulateLifecycleStateFields`).

### 3c. Loading + hot reload (`rule_loader.go`, `kv_config_integration.go`)

- **Files:** `config.rules_files` = paths to JSON (single rule *or* array);
  `config.inline_rules` for inline `Definition`s. Loaded at `Processor.Start`.
- **KV + hot reload:** on Start the component seeds file rules into the
  `semstreams_config` KV bucket under keys `rules.<id>` **idempotently** (`Create`,
  so operator edits already in KV are preserved), then opens a `Watch("rules.*")`.
  Watcher events are debounced 250 ms and trigger a **full-replace reconcile**
  (`reconcileFromKV` → `ValidateConfigUpdate` → `ApplyConfigUpdate`; rules absent
  from KV are removed). Agent tools (`create_rule`/`update_rule`/`delete_rule`)
  writing `rules.*` are picked up by the same watcher. **Semdragons can ship quest
  rules as JSON files and still let the DM edit them live via KV.**

### 3d. Real working example (`configs/rules/lifecycle/01-mission-launch.json`)

```json
{
  "id": "lifecycle_mission_launch_on_command",
  "type": "expression",
  "enabled": true,
  "conditions": [{ "field": "mission.command", "operator": "eq", "value": "launch" }],
  "logic": "and",
  "on_enter": [
    { "type": "lifecycle_transition", "workflow": "mission", "phase": "flying",
      "reason": "command=launch from UDP" }
  ]
}
```
Action lists are transition-driven: `on_enter` (false→true), `on_exit`
(true→false), `while_true` (every update while matching), `on_recovery`
(re-fire on restart if still matching). Cron rules use `type:"cron"` + `schedule`
+ `actions`.

### 3e. Attempt-conditional routing (retry-vs-triage) — **expressible in rules, no escape hatch needed**

The question: can "retry up to N, then route to `pending_triage`" be expressed in
a rule condition? **Yes**, because `gt/gte/lt/lte` are wired numeric operators and
`lifecycle_transition.set` can atomically increment a counter that lives as a
triple. Pattern:

1. Quest schema carries `RetryCount int json:"retry_count" lifecycle:"operator_writable,predicate=quest.retry_count"`.
2. On failure, the fail→requeue rule increments it in the same atomic transition:
   ```json
   { "type": "lifecycle_transition", "workflow": "quest", "phase": "posted",
     "set": { "retry_count": {"op": "increment"} } }
   ```
   guarded by `{"field": "quest.retry_count", "operator": "lt", "value": 3}`.
3. The triage rule fires on the complement:
   ```json
   { "conditions": [{"field":"quest.retry_count","operator":"gte","value":3}],
     "on_enter": [{ "type":"lifecycle_transition","workflow":"quest","phase":"pending_triage" }] }
   ```

Two subtleties to design around:
- The counter must be an `operator_writable` projected scalar (AssertRuleWritable
  is default-deny), and the `set` increment runs inside the *same* atomic phase
  write — so count and phase can never drift.
- The rule engine also has a *built-in* per-`(rule,entity)` budget
  (`Definition.MaxIterations` + `$state.iteration`/`$state.max_iterations`
  substitution). That is lighter but couples the budget to a rule's match count
  (reset on rule replace / re-dispatch), **not** to the quest's own attempt
  history — prefer the triple-counter approach for a durable, graph-visible quest
  retry budget.
- Escape hatch if ever needed: a component can call `Manager.TransitionWith`
  directly with arbitrary Go logic. But for retry-vs-triage the rule path suffices.

---

## 4. `gated-dag` contract (`processor/gated-dag/*`, pure brain in `pkg/gateddag`)

The executor is a **domain-agnostic component**. semdragons keeps its quest DAG
domain layer; it feeds the executor a resolved `depends_on` edge set on per-unit
graph entities and wires a handler on the dispatch subject. The pure decision
brain (`pkg/gateddag`) derives each unit's status from marker membership only —
there is no stored/mutated status field to race the markers.

### 4a. Config (`processor/gated-dag/config.go`, `schema.go`)

```go
type Config struct {
    UnitEntityPrefix string // REQUIRED: graph.query.prefix scope = one fan-out's unit set
    DispatchSubject  string // REQUIRED: where a dispatchable unit's reference is published
    FanOutWorkflow   string // default "gateddag-fanout" (self-registered); or point at your own registered workflow

    CompletedPredicate string // default "gateddag.completed"
    FailedPredicate    string // default "gateddag.failed"
    DirtiedPredicate   string // default "gateddag.dirtied"   (reset marker)
    DependsOnPredicate string // default "gateddag.depends_on" (Object = prerequisite unit ID)
    ClaimPredicate     string // default "gateddag.claim"      (durable in-flight record)

    Workers          int    // default 4  (bounded dispatch concurrency)
    QueueSize        int    // default 256
    BackstopInterval string // default "30s" (correctness floor re-eval tick)
    QueryTimeout     string // default "30s"
    MaxUnits         int    // default 1000 (whole-set read cap; truncation warns)
    FailurePolicy    string // "continue_others" (default) | "stop_on_first_failure"
    FanOutInstanceID string // optional; framework owns the FanOut instance lifecycle when set
    StallSubject     string // optional; edge-triggered StallEvent on 0→non-zero stall
}
```
Marker/edge predicates must be pairwise distinct (collision silently mis-reads a
marker class). Predicates are **free-form** — graph-ingest only validates the
indexing-profile predicate, so no vocabulary registration is required for the DAG
markers.

### 4b. Consumer setup checklist (from `doc.go`, ADR-046 Phase 2.1)

1. *(Optional)* Set `FanOutInstanceID` (a 6-part `*.*.gateddag.fanout.instance.*`
   ID) → executor creates the instance in `dispatching` on Start and
   auto-transitions it to `completed` when every unit is Done. **FanOut *failure*
   is always consumer-driven (`Manager.Fail`)** — a stall may be recoverable, so
   the framework never auto-fails.
2. Seed each unit as a graph entity under `UnitEntityPrefix`.
3. Write `depends_on` edges: one triple per edge, predicate `DependsOnPredicate`,
   `Object` = the prerequisite unit's entity ID, on the dependent.
4. On work completion/failure write the `CompletedPredicate`/`FailedPredicate`
   marker on the unit; to re-run, follow the **reset contract**: clear terminal +
   claim markers and set the `DirtiedPredicate` marker.

How units + edges are actually seeded (`fullstack_integration_test.go:298`):
```go
// unit "b" depends on "a": one depends_on triple, Object = "a"
gi.CreateEntity(ctx, &graph.EntityState{ ID: b, Version: 1, Triples: []message.Triple{
    {Subject: b, Predicate: "gateddag.depends_on", Object: a, Confidence: 1.0},
}})
// completion marker (presence-based; Object value irrelevant for terminal markers)
AddTripleRequest{Triple: {Subject: unitID, Predicate: "gateddag.completed", Object: true}}
```

### 4c. Dispatch-subject handler contract (`publisher.go`, `payload.go`)

When a unit becomes dispatchable the executor publishes a **reference, never
content** to `DispatchSubject`, wrapped in a `BaseMessage`:
```go
type DispatchMessage struct {
    UnitEntityID   string `json:"unit_entity_id"`   // the 6-part unit ID to turn into work
    FanOutWorkflow string `json:"fan_out_workflow,omitempty"`
}
// message.Type{Domain:"gateddag", Category:"dispatch", Version:"v1"}
```
**The consumer's handler is expected to:** decode the envelope, read the unit's
details from the graph by `UnitEntityID`, perform the work (for semdragons: spawn
the sub-quest's agentic loop / publish_agent), and on completion write the
`Completed` (or `Failed`) marker on that unit. Writing the marker is what advances
the DAG — the executor re-reads authoritatively and releases dependents. There is
no ack/return-value contract; the marker write IS the completion signal.

### 4d. Claim-before-dispatch & exactly-once (`executor.go`, `claim.go`)

`claimThenDispatch` (bounded worker): **commit the durable claim marker BEFORE
publishing** (invariant #2 — a crash between publish and claim would re-derive the
unit Ready on restart and double-run). The claim is an atomic replace-by-predicate
write (`RemoveTriples`+`AddTriples` of `ClaimPredicate`) with
`ExpectedRevision=0` (last-write-wins, *not* CAS). Mutual exclusion therefore does
**not** come from the claim write — it comes from **single-flight** (one
`reEvaluate` pass at a time per instance via `evalMu`) + **one component instance
per fan-out**. A CAS-UPGRADE POINT is documented in `claim.go` if cross-writer
exclusion is ever needed. An in-memory `inflight` set closes the
submit→claim-commit window (the async unit-completion watch can nudge a re-eval
sub-millisecond, before the durable claim lands).

### 4e. The pure brain (`pkg/gateddag/gateddag.go`)

```go
type MarkerSet struct { Completed, Failed, Dirtied map[string]bool }
func DeriveStatus(id string, m MarkerSet) Status         // Dirtied > Failed > Completed > Ready
func Evaluate(unitIDs, dependsOn, m) []Decision          // Dispatchable ⇔ Ready AND every prereq derives Done
func SelectDispatchable(unitIDs, dependsOn, m) []string
func Stalled(unitIDs, dependsOn, m) []string             // held-Ready units, nothing dispatchable, nothing in-flight
```
A unit is dispatchable iff its derived status is `Ready` **and every** entry in
`dependsOn[unit]` derives `Done` (all-prereqs closure, never "any"). A `Dirtied`
(reset) unit derives `Ready` over any stale terminal marker → clean re-dispatch.

### 4f. Re-eval, restart recovery, stall detection (`executor.go`)

- **Re-eval drivers:** (1) the lifecycle `Watch` over `FanOutWorkflow` — its
  bootstrap replay on subscribe **is the restart-recovery substrate** (invariant
  #3, must NOT be the `pkg/dispatch` completion-watcher, which suppresses replay);
  (2) a raw KV watch over `UnitEntityPrefix.>` that nudges the moment a
  marker lands (low-latency completions); (3) the periodic `BackstopInterval`
  tick (the correctness floor for missed watch events). All three coalesce into a
  single-flight `trigger` channel; each pass reads the authoritative whole set via
  `graph.ingest.query.prefix` and re-derives — **never cache-first**.
- **Restart recovery:** on boot the initial pass + the Watch bootstrap replay
  reconcile the DAG from KV truth; claimed-but-unmarked units are visible via the
  claim markers.
- **Stall detection:** `Stalled()` (a `depends_on` cycle, or everything blocked
  behind a failure) surfaces as the `gated_dag_stalled_units` gauge + a WARN log
  every pass, and — if `StallSubject` is set — an edge-triggered `StallEvent`
  (0→non-zero, backstop-tick-gated so transient mid-seeding stalls don't alert).
  **Stall is never silent idle** — this is the property `questdagexec`'s node
  exhaustion / escalation logic maps onto.
- **Referential-stub safety (gh#429):** an entity referenced before it is born is
  materialized as a stub (`core.identity.stub`); `extractGraph` **excludes stubs**
  so a not-yet-born unit isn't derived as dependency-free-and-dispatchable.

### 4g. Where per-unit state lives in the graph (for re-deriving `quest.dag.*`)

Everything is triples on the per-unit entity under `UnitEntityPrefix`:
`gateddag.depends_on` (edges), `gateddag.completed|failed|dirtied` (markers),
`gateddag.claim` (in-flight). The FanOut instance entity carries only
`gateddag.fanout.phase`. **semdragons re-derives its `quest.dag.*` projections by
reading these unit entities** — the executor never hides state in a private bucket,
so `quest.dag.node_states` / `node_assignees` / `node_retries` become graph reads.
Per ADR-064, a per-unit resource-prep step (e.g. a hydrated sub-quest workspace)
is modeled as its **own gated-DAG unit** `P_X` (`X depends_on P_X`,
`P_X depends_on {prereqs}`) rather than a pre-dispatch hook — failure then becomes
a visible `Failed` marker instead of a silent wedge.

---

## 5. lifecycle-gateway REST surface (`gateway/lifecycle-gateway/*`)

A framework component that exposes `Manager` read + operator-mutation over HTTP +
WebSocket. It reads `Dependencies.LifecycleManager` (the *same* Manager instance
the rule processor and gated-dag use) and mounts under a configurable
`PathPrefix` (default `/workflows`). Routes (`handlers.go`, `doc.go`, `openapi.go`):

| Method | Path | Handler → Manager call |
|---|---|---|
| GET | `{prefix}` | list workflow types + per-phase instance counts (`ListWorkflows` + `List`) |
| GET | `{prefix}/{type}` | list instances; query params `phase`, `active=true`, `limit`, `offset`, `match.<field>=<v>` (`List`) |
| GET | `{prefix}/{type}?stream=true` | **WebSocket** live updates (`Watch`); requires `enable_websocket=true` |
| GET | `{prefix}/{type}/{id}` | full instance state (`Get`) |
| GET | `{prefix}/{type}/{id}/history` | phase-transition history (`History`) |
| GET | `{prefix}/{type}/{id}/children` | child instances (`Children`) |
| POST | `{prefix}/{type}/{id}/state` | operator patch `{ "<field>": <v> }` (`UpdateFromOperator`; validates `operator_writable`) |
| POST | `{prefix}/{type}/{id}/transition` | `{ "phase": "...", "note": "..." }` (`Transition` with `TransitionSourceOperator`; validates the table) |

Uniform error envelope `{"error":"..."}`. Status mapping (`errorToStatus`):
`ErrWorkflowNotRegistered`/`ErrEntityNotFound` → 404;
`ErrFieldNotOperatorWritable`/`ErrInvalidTransition`/`ErrTerminalPhase` → 400;
`ErrUpdateRetriesExhausted` → 409 (write race); everything else → 500 (message
redacted, logged server-side). **This is the DM's operator surface** — inspect any
quest, replay its audit trail, force a transition, patch operator-writable fields,
and subscribe to a live quest-board feed, all without semdragons writing bespoke
endpoints.

---

## 6. Open risks / expressiveness gaps for the quest use case

1. **Per-transition latency + N reads.** Every transition is a NATS round-trip
   through graph-ingest (~5-10 ms vs a direct KV write), and `List`/`Children`
   are O(bucket-size) with per-child `Get` (N+1). ADR-049 accepts this for
   "handfuls per hour per entity" workflows. A busy quest board with boid churn +
   many concurrent claims could feel it; the v2 secondary index
   (`lifecycle:"indexable"`) is deferred until a bottleneck is demonstrated.
   *Mitigation:* keep quest dashboards polling sparingly; drive live UI off `Watch`
   / the message-logger SSE, not `List` refreshes.

2. **`Manager.Fail` hard-codes the phase name `"failed"`.** semdragons' lifecycle
   has both `failed` and `escalated`/`pending_triage` terminals. `Fail` only routes
   to a phase literally named `failed`; any other error terminal needs an explicit
   `Transition`. Fine, but the rule pack must not assume `lifecycle_fail` reaches
   `escalated`.

3. **`Complete` is ambiguous with multiple reachable terminals.** From a phase
   with several terminal out-edges, `Complete` picks the first deterministically and
   only WARN-logs. For quests where `in_review` can go to `completed`/`failed`/
   `escalated`, use explicit `lifecycle_transition`/`lifecycle_complete` targeting a
   named phase, not bare `Complete`.

4. **Retry counter must be `operator_writable`, and that also exposes it to the DM
   patch endpoint.** The default-deny model means the atomic `set:increment` retry
   counter is, by construction, also operator-patchable via
   `POST .../state`. Acceptable (arguably useful — DM can reset a retry budget) but
   worth a conscious decision; there is no "rule-writable but not operator-writable"
   tier.

5. **gated-dag mutual exclusion rests on single-instance + single-flight, not a
   lock.** The claim marker is last-write-wins. If semdragons ever runs two
   gated-dag executors over the same `UnitEntityPrefix` (e.g. two board replicas),
   units can double-dispatch. The framework's stated contract is **exactly one
   component instance per fan-out**; enforce that in deployment, or take the
   documented CAS-UPGRADE path.

6. **The consumer owns reliable completion-marker writes.** The full-stack test
   explicitly notes production has **no** auto-recovery for a
   claimed-but-never-marked unit (gh#373 follow-up) — a lost `Completed` marker
   strands the unit until reset. semdragons' dispatch handler must write terminal
   markers at-least-once (retry with a bounded budget) or supply its own
   orphaned-node watchdog. This is the same class as the existing
   "semstreams has no orphan-tool-call watchdog" note in the semdragons memory.

7. **DAG failure verdict is consumer-driven.** The executor auto-*completes* a
   FanOut but never auto-*fails* it (a stall may be recoverable). semdragons must
   decide when a stalled/blocked quest DAG becomes an `escalated` parent quest and
   call `Manager.Fail`/`Transition` itself — the `Stalled()` event/gauge is the
   signal, not an action. This maps directly onto `questdagexec`'s existing
   "escalate the parent quest on node exhaustion" responsibility.

8. **Schema evolution is forward-additive only.** Removed schema fields leave
   orphan triples in `ENTITY_STATES`; there is no migration primitive. Plan quest
   schema fields to be additive, or script triple cleanup on removal.
