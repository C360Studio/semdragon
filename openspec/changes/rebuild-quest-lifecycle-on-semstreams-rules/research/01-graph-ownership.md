# Research 01 — Graph-write / ownership / authoritative-semantic-state (beta.21 → beta.132)

Cluster owner: graph-write / ownership / semantic-state semantics.
Scope: what ADR-054/055/056/049 changed in semstreams' graph layer, the exact
API surface semdragons touches, the `quest.*` ownership question (feeds task
1.3), and the concrete semdragons impact list.

Sources read (in `/Users/coby/Code/c360/semstreams`):
`docs/adr/049,054,055,056`, `graph/mutation_requests.go`, `graph/graphable.go`,
`graph/types.go`, `graph/constants.go`, `graph/stub.go`, `pkg/ownership/claim.go`,
`pkg/projection/contract.go`, `docs/operations/migration-*.md`.

---

## TL;DR — the single most important finding

**semdragons does not call the semstreams graph mutation API at all.** Its own
`GraphClient` (`/Users/coby/Code/c360/semdragon/graphclient.go`) writes entity
state by *raw KV `Put`/`Update`* straight into the `ENTITY_STATES` bucket
(`graphclient.go:123` `store.Put`, `:153` `store.Update`). It never sends
`graph.mutation.entity.create_with_triples` / `update_with_triples` /
`triple.add`. Therefore **the ADR-054/055/056 changes — which are almost
entirely changes to graph-ingest's mutation handlers and to new
`pkg/ownership` / `pkg/projection` packages — do NOT break semdragons'
compilation.** The three semstreams `graph`-package symbols semdragons actually
imports (`graph.EntityState`, `graph.Graphable`, `graph.BucketEntityStates`) are
all unchanged / additive in beta.132.

The changes are **semantic and architectural**, and they matter because:

1. `BoardConfig.BucketName()` returns the literal string `"ENTITY_STATES"`
   (`/Users/coby/Code/c360/semdragon/domain/config.go:161`) — the *same bucket*
   graph-ingest / graph-index / graph-query are wired to
   (`config/semdragons.json:212,227,267,294`). So semdragons is a **second,
   unregistered, full-object writer to `ENTITY_STATES`**, and the new ADRs are
   precisely a framework-wide crackdown on exactly this pattern (unregistered
   writers, envelope-less births, ownership-less predicate writes).
2. The rebuild ("…on-semstreams-rules") deliberately routes quest lifecycle
   *through* the framework (lifecycle `Manager`, rule-engine `lifecycle_*`
   actions, `gated-dag`). The moment quest writes go through graph-ingest
   instead of raw `Put`, semdragons inherits the full ADR-055 envelope rule +
   ADR-056 ownership contract + ADR-054 indexing profile.

So this cluster is **near-zero compile risk, high design risk.** Task 1.3's
ownership spike is the real work.

---

## 1. What changed (ADR-049 / 054 / 055 / 056 in plain terms)

These four ADRs form one arc: *`ENTITY_STATES` quietly became the framework's
authoritative state store, and the framework is now naming and enforcing the
contract for writing it.*

### ADR-049 — schema-as-discipline over `ENTITY_STATES` (beta.86)
The lifecycle harness stopped owning a private per-workflow KV bucket. A
`pkg/lifecycle.Manager` declares what a workflow *is* (phases, transition table,
operator-writable predicates, audit predicates) and emits state changes through
graph-ingest like everyone else. **graph-ingest is "the single writer to
`ENTITY_STATES`."** The engine primitive it added is **CAS-on-condition**: the
new `ExpectedRevision` field on `UpdateEntityWithTriplesRequest` — non-zero means
"apply iff the entity is still at revision N, else reject." This is how two
rules transitioning the same entity from the same phase don't both win.

### ADR-054 — indexing eligibility ("graph visibility is storage; indexing is policy")
Separates *being in the graph* (always, unconditional, queryable) from *being
embedded/clustered* (a budgeted retrieval policy). A new single-valued reserved
triple **`entity.indexing.profile`** ∈ `content | control | signal | trace`
declares a producer's retrieval intent. graph-ingest stamps it **at entity
birth** (create seam). Policy (a per-`(profile, entity_type)` matrix) decides
which substrates index it. Currently **lenient**: absent profile = legacy-embed,
so nothing goes dark yet; a future `strict_indexing_profile` flip (gated on a
cost-ledger) would drop `trace`/`signal` out of embedding. Declaration channels:
(a) a new optional `IndexingProfiler` payload interface, (b) an
`indexing_profile` field on the **create/update** mutation envelopes, (c) a
graph-ingest `message.Type` → profile fallback floor.

### ADR-055 — write-intent taxonomy ("the mutation API is not a producer API"), **BREAKING, implemented 2026-06-19**
graph-ingest's write surface is now a **four-lane taxonomy**:

| Lane | Ingress | Conflict semantics | Creates? | Envelope |
|---|---|---|---|---|
| Fact arrival | Graphable stream consumer | CAS-retry, async, **appends** | yes | carried (registered payload) |
| Entity create | `entity.create_with_triples` | atomic **create-or-fail** | yes | **required** |
| State transition | `update_with_triples` + `ExpectedRevision>0` | **CAS-on-condition, rejects, sync**, replace-by-predicate | no (must-exist) | inherited |
| Evidence append | `triple.add` / `triple.add_batch` | append multi-valued, **must-exist** | **no** | inherited |

The load-bearing break: **`triple.add` / `triple.add_batch` lost auto-vivify.**
A triple whose subject entity does not already exist is now **rejected** with
`graph.ErrorCodeEntityNotFound` (was: silently create an envelope-less
`EntityState{Version:0}`). The normative rule: **no write may create an entity
without a semantic envelope** (a registered payload type, or an explicit
`MessageType` domain/category/version, plus an `IndexingProfile`). Entities are
born only via *Fact arrival* or *Entity create* — both envelope-bearing.

### ADR-056 — authoritative semantic state = the **predicate-group ownership contract** (Accepted 2026-06-13)
Names the responsibility ADR-049 left implicit. The ownership unit is **not the
entity — it's the predicate GROUP.** One entity is a *hybrid*: several owners
each own a disjoint set of its predicates, plus un-owned append/evidence
predicates. A claim is a tuple:

> `(entity-ID pattern, predicate set, write mode, owner id)`

`write mode ∈ { replace-owned, cas-transition, append-evidence }`. Claims live
in a shared `OWNER_CLAIMS` KV epoch and are cross-process overlap-checked at
registration (two owners claiming the same cell → registration fails unless a
declared waiver). New substrate: `pkg/ownership` (the enforcement registry) +
`pkg/projection` (the declarative `Contract` that ownership claims **derive
from**). **Runtime enforcement on the current tag is OBSERVE-ONLY**: overlaps and
unclaimed foreign edges are metered/logged (`mutation_rejections_total`,
`foreign_edge_unclaimed_total`), *not* rejected; the `OwnerToken` write-lease is
stamped on requests for "observe-only transit" but not yet compared. So adopting
ownership is incremental — you can register claims and watch the metrics before
anything hard-fails.

---

## 2. API-level breaking changes (exact signatures)

### 2a. The graph-package surface semdragons compiles against — UNCHANGED

semdragons imports `semstreams/graph` in 25 files but uses only three
package-level symbols:

| Symbol | beta.132 status | Evidence |
|---|---|---|
| `graph.EntityState` (struct: `ID, Triples, StorageRef, MessageType, Version, UpdatedAt`) | **unchanged**; additive method `IsStub()` only | `graph/types.go:24-47`, `graph/stub.go:48` |
| `graph.Graphable` (interface: `EntityID() string` + `Triples() []message.Triple`) | **unchanged** — `IndexingProfiler` is a *separate optional* interface, not a new required method | `graph/graphable.go:54-60` |
| `graph.BucketEntityStates` (`= "ENTITY_STATES"`) | **unchanged** | `graph/constants.go:6` |

`message.Triple` (`Subject/Predicate/Object/Source/Timestamp/Confidence`) and
`message.Type` (`Domain/Category/Version`) are likewise stable. **Net: the
graph-write/ownership cluster contributes zero compile breaks to task 1.2.** Any
compile breakage in the bump comes from other clusters (payload registry /
BaseMessage / natsclient / agentic), not this one.

### 2b. `EmitEntity` / `EmitEntityUpdate` are semdragons' OWN methods, not semstreams'

Important disambiguation for the whole team: the ~69 `graph.EmitEntityUpdate`,
26 `graph.EmitEntity`, 42 `graph.GetQuest` "hits" are **method calls on a local
variable named `graph` of type `*semdragons.GraphClient`**, defined in
`/Users/coby/Code/c360/semdragon/graphclient.go`:

```go
// graphclient.go:112 — full-object last-writer-wins Put
func (gc *GraphClient) EmitEntity(ctx, entity graph.Graphable, eventType string) error {
    data, _ := marshalEntityState(entity, eventType) // whole EntityState, all Triples()
    store, _ := gc.ensureStore(ctx)
    _, err := store.Put(ctx, entity.EntityID(), data) // raw KV Put — bypasses graph-ingest
    ...
}
// graphclient.go:131 — EmitEntityUpdate is literally EmitEntity (alias)
// graphclient.go:142 — EmitEntityCAS uses store.Update(...revision) (CAS)
```

These do **not** change with the semstreams bump — they are semdragons code.
The `eventType` string (e.g. `"quest.claimed"`) becomes `MessageType.Category`
(`graphclient.go:83-87`), **not** a triple predicate; the predicates come from
each entity's `Triples()`.

### 2c. New request-struct fields the REBUILD will use (additive)

When Stage B routes quest writes through graph-ingest, these are the request
shapes (quoted from `graph/mutation_requests.go`, beta.132):

```go
// graph/mutation_requests.go:38 — Entity create lane (create-or-fail)
type CreateEntityWithTriplesRequest struct {
    Entity          *EntityState     `json:"entity"`
    Triples         []message.Triple `json:"triples"`
    IndexingProfile string           `json:"indexing_profile,omitempty"` // NEW (ADR-054): content|control|signal|trace
    OwnerToken      string           `json:"owner_token,omitempty"`      // NEW (ADR-056): "<owner>#<incarnation>"
    TraceID         string           `json:"trace_id,omitempty"`
    RequestID       string           `json:"request_id,omitempty"`
}

// graph/mutation_requests.go:89 — State transition / replace lane
type UpdateEntityWithTriplesRequest struct {
    Entity           *EntityState     `json:"entity"`
    AddTriples       []message.Triple `json:"add_triples,omitempty"`    // REPLACE-by-(subject,predicate), NOT append
    RemoveTriples    []string         `json:"remove_triples,omitempty"` // pure delete, runs before merge
    ExpectedRevision uint64           `json:"expected_revision,omitempty"` // ADR-049: non-zero = CAS-on-condition, rejects
    IndexingProfile  string           `json:"indexing_profile,omitempty"`  // NEW (ADR-054): explicit-override channel
    OwnerToken       string           `json:"owner_token,omitempty"`       // NEW (ADR-056)
    TraceID          string           `json:"trace_id,omitempty"`
    RequestID        string           `json:"request_id,omitempty"`
}
```

Two behaviors to internalize for the rebuild:
- **`AddTriples` on the update lane is REPLACE-by-(subject,predicate)** (upsert),
  not append. To add to a multi-valued predicate while preserving siblings you
  must use `triple.add` / `triple.add_batch` instead. This is the opposite of
  semdragons' current full-object `Put`.
- **Metadata is preserve-when-zero**: a bare `Entity{ID}` delta does not erase
  `MessageType`/`Version`/`StorageRef` (`mutation_requests.go:82-88`).

### 2d. New ownership / projection API (the spike target)

```go
// pkg/ownership/claim.go:17 — write modes
const ( ModeReplaceOwned="replace-owned"; ModeCASTransition="cas-transition"; ModeAppendEvidence="append-evidence" )

// pkg/ownership/claim.go:87 — a single owned predicate group
type OwnerClaim struct {
    Owner      string    // subject-safe id, e.g. "rule-pack.quest-lifecycle"
    Pattern    string    // 6-part entity-ID glob, e.g. "c360.*.game.*.quest.*"
    Predicates []string  // EXACT strings, no wildcards
    Mode       WriteMode
    Incarnation string   // per-process boot nonce (OwnerToken half)
}

// pkg/projection/contract.go:46 — claims DERIVE from a projection contract (ADR-056 Decision 6)
type Contract struct {
    Name, MessageType, EntityPattern, IndexingProfile string
    Groups       []PredicateGroup // owned/append groups by write mode
    ForeignEdges []ForeignEdge
}
// pkg/projection/contract.go:174 — bind a contract to an owner id, returns a lease token
func Bind(ctx, ownerReg *ownership.Registry, owner string, contracts ...Contract) (ownership.OwnerToken, error)
// bootstrap: ownership.EnsureBuckets(ctx, client, logger, resolver) (*Registry, error)  // pkg/ownership/bootstrap.go:47
```

---

## 3. Ownership question for `quest.*` predicates (feeds task 1.3)

### The problem, made concrete
The **same quest entity** is written today by at least five components, each via
full-object `EmitEntityUpdate` (last-writer-wins `Put`):

| Writer | file (call-site count) | `quest.*` predicate group it authors |
|---|---|---|
| questboard | `processor/questboard/handler.go` (20) | `quest.identity.*`, `quest.status.state` (phase), `quest.lifecycle.*`, `quest.assignment.*`, `quest.difficulty.*`, `quest.attempts.*`, `quest.dependencies.*` |
| questbridge | `processor/questbridge/handler.go` (15) | `quest.execution.loop_id`, `quest.context.*` (assembled/entities/repo/sources/token_count) |
| questdagexec | `processor/questdagexec/handler.go` (6) + `eventloop.go` (2) | `quest.dag.*` |
| bossbattle | `processor/bossbattle/handler.go` (10) | review verdict → `quest.status.state` = `in_review`, `quest.failure.*` |
| service/api | `service/api/handlers.go` (18) | operator status mutations across the above |
| redteam | `processor/redteam/handler.go` (3) | `quest.classification.red_team_*` |

Because each writer does a **read-modify-write of the whole quest struct then
`Put`s all of `Triples()`** (e.g. `questbridge/handler.go:181` `GetQuest` →
mutate → `:346` `EmitEntityUpdate(quest, "quest.context.assembled")`), a stale
writer **clobbers** predicates another writer just set. Today this is only
mitigated where `EmitEntityCAS` is used (contested claims, 8 sites). This is
exactly the ADR-056 multi-owner hazard — and worse than the ADRs' own examples,
because full-object `Put` overwrites *every* predicate group, not just the one
the writer cares about.

### How rules + questbridge avoid conflicting under the new model
The rebuild's D2 has rules own transitions and questbridge own context. Under
ADR-056 that maps cleanly to **disjoint owned predicate groups**:

1. **Partition `quest.*` into owner groups** (each a `projection.Contract` /
   `OwnerClaim`, pattern `c360.*.game.*.quest.*`, EXACT predicate lists):
   - **quest-lifecycle owner** (the rule pack + lifecycle `Workflow`):
     `quest.status.state` as the **phase predicate** — `Mode: cas-transition`
     (via `ExpectedRevision`, so two concurrent transitions can't both win);
     `quest.lifecycle.*` + `quest.attempts.*` audit as `replace-owned`.
   - **questbridge owner**: `quest.execution.*` + `quest.context.*` —
     `Mode: replace-owned`. Crucially, questbridge must switch from full-object
     `Put` to `update_with_triples` (replace-by-predicate), which **preserves
     foreign predicates** (the phase, the dag state) instead of clobbering them.
   - **questdag owner**: `quest.dag.*` — `replace-owned`.
   - **review/redteam owners**: their own verdict/classification groups.
2. **The phase predicate is single-owner.** D2 says rules own transitions, so
   only the lifecycle owner may write `quest.status.state`. bossbattle's current
   direct write of `in_review` (and API status mutations) must instead go
   through the same lifecycle transition seam (D6 makes `in_review→completed` a
   rule; task 4.4 deletes the API status handlers). If two owners both claim
   `quest.status.state`, `RegisterOwner` **fails at boot** — which is the
   feature: it forces the design to pick one writer.
3. **Registration is the arbiter, not runtime.** On the current tag runtime
   enforcement is observe-only, so overlaps surface as metrics/logs, not
   dropped writes — the spike can register claims and confirm zero overlap
   *before* trusting them.

**Concrete spike (task 1.3) recipe:**
- `ownership.EnsureBuckets(...)` at boot (before any registration), then `Bind`
  one `projection.Contract` per owner above; assert no overlap error.
- Write the quest phase via `UpdateEntityWithTriplesRequest{ExpectedRevision:N}`
  through graph-ingest; confirm CAS-reject on a stale revision.
- Write `quest.execution.loop_id` via a *separate* update from a *different*
  owner; confirm the phase predicate survives (replace-by-predicate preserves
  it) — the property full-object `Put` does not have.
- Decide `OwnerToken` sourcing: `"<owner>#<incarnation>"` where incarnation is
  `Registry.Incarnation()`; harmless to stamp now (observe-only transit).

### Open sub-questions for the spike to close
- Is `quest.status.state` best modeled as a lifecycle `Workflow` phase
  (`Manager.Transition` owns the CAS) or a rule-pack `cas-transition` claim?
  D4 leans Workflow; either registers one owner over the phase predicate.
- Do `quest.dag.*` predicates on the *parent* quest (written by questdag) and
  the lifecycle phase on the *same* parent entity register as two disjoint
  owners (yes — hybrid entity, the normal case) — confirm no accidental overlap
  with `quest.dependencies.*`.

---

## 4. Impact list for semdragons (concrete)

### A. Compilation (task 1.2) — this cluster: none
No graph-write/ownership symbol semdragons imports changed. `graph.EntityState`,
`graph.Graphable`, `graph.BucketEntityStates` all compile against beta.132
unchanged. **Do not expect this cluster to appear in the compile-fix pass.**

### B. Behavioral / runtime — present today, unchanged by the bump but now
"against the rules"
- **Second writer to `ENTITY_STATES`.** `domain/config.go:161`
  `BucketName()="ENTITY_STATES"` + raw `Put` in `graphclient.go:123` means
  semdragons violates the "single writer to `ENTITY_STATES`" invariant ADR-049
  and ADR-056 reinforce. Not a crash — but every semdragons entity is
  **envelope-partial** (`MessageType{Domain:"semdragons", Category:eventType,
  Version:"v1"}`, `graphclient.go:83`) and **profile-less** and
  **ownership-less**. Under ADR-055's rule these entities could never have been
  born through the mutation API; they exist only because raw `Put` sidesteps it.
- **Indexing (ADR-054): low risk today.** semdragons' config enables only
  `graph-ingest/index/query` (`config/semdragons.json:198-296`), *not*
  graph-embedding/graph-clustering, so the embedding storm / strict-flip
  darkening is not an active semdragons concern. If embedding is ever enabled,
  semdragons' many `Put`-written, profile-less entities would embed in lenient
  mode and go dark on a strict flip. Note also graph-ingest never stamps a
  profile on them (it never sees the raw `Put`).

### C. The rebuild (Stage B) — where the ADRs bite
- **`questboard` transition handlers → lifecycle `Workflow` + rules.** The
  bespoke `EmitEntityUpdate(quest, "quest.claimed"/...)` transitions
  (`processor/questboard/handler.go`, 20 sites) become `Manager.Transition`
  (CAS via `ExpectedRevision`) or rule `lifecycle_transition` actions. The phase
  predicate is the existing `quest.status.state` (design D3 calls it
  `quest.status`; the code string is `quest.status.state` — the `Workflow.
  PhasePredicate` must be exact).
- **questbridge full-object `Put` → replace-by-predicate.**
  `processor/questbridge/handler.go` read-modify-writes the entire quest and
  `Put`s it (`:181`→`:346`, `:729`→`:739`, `:760`→`:800/:814…`). Under the
  ownership model this must become a scoped `update_with_triples` writing only
  `quest.execution.*` / `quest.context.*`, or it will clobber the rule-owned
  phase. **This is the highest-value refactor** and the reason for the D2 split.
- **Direct `graph.EntityState{}` construction** at
  `graphclient.go:80`, `processor/questdagexec/handler.go:1600`,
  `processor/questdagexec/eventloop.go:176` — still compiles (struct unchanged),
  but the questdagexec event loop is being deleted (D5/task 3.6) in favor of
  `gated-dag`; those two sites go away.
- **bossbattle / service-api status writes** to `quest.status.state` must route
  through the single lifecycle owner (D6 stub rule for `in_review`; task 4.4
  deletes API status handlers) or they contend for the phase predicate group.
- **Register ownership at boot.** New wiring in `cmd/semdragons/main.go`:
  `ownership.EnsureBuckets` **before** component `Register` (bootstrap-ordering
  hazard — buckets must exist before any owner binds), then per-owner
  `projection.Bind`. Absent this, behavior is exactly pre-ADR-056 (graceful
  skip), so it can land incrementally.

### D. Non-issues to stop worrying about
- No new *required* interface method on `Graphable` (IndexingProfiler is
  optional). Existing `Triples()` payloads keep compiling.
- The `triple.add` must-exist flip cannot break semdragons today — semdragons
  never calls `triple.add`. It only becomes relevant if the rebuild chooses the
  evidence-append lane for any `quest.*` multi-valued predicate (e.g.
  `quest.failure.history`, `quest.acceptance.criterion`), in which case the
  entity must already exist (it will — born via the lifecycle create lane).

---

## Appendix — quest predicate namespace (for group partitioning)

Distinct 3-part `quest.*` predicates found in semdragons source (partial, the
groupable prefixes): `quest.identity.*` (id/name/title/description),
`quest.status.state`, `quest.lifecycle.*` (claimed/completed/failed/abandoned/
escalated + `*_at`, `decomposed`, `pending_triage`), `quest.assignment.*`
(agent/party), `quest.difficulty.level`, `quest.attempts.*` (current/max),
`quest.dependencies.*` + `quest.dependency.quest`, `quest.execution.loop_id`,
`quest.context.*` (assembled/entities/repo/sources/token_count),
`quest.dag.*` (definition/execution_id/node_*/completed_nodes/failed_nodes/
clarification*/state_updated/…), `quest.failure.*` (reason/type/history/
escalated), `quest.classification.*` (type/red_team_*),
`quest.artifacts.*` (indexed/merged), `quest.acceptance.criterion`,
`quest.data.*` (input/output/title), `quest.dm.clarifications*`.
