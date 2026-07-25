# Provider Arena Instrument

## Why

The 2026-07 design audit's central finding: every semdragon evaluation loop is
LLM-judging-LLM with **zero executed ground truth**. XP, leveling, and boss
battles all rest on soft signals, so the game currently measures who *looks*
competent — vanity, not outcomes. The accepted fix is the **two-loop
principle**: soft signals (LLM/peer review) may steer routing and prompts only;
only hard signals (executed verification in hermetic environments) may move
trust or gate capability.

That principle unlocks something the audit did not name: once you can bind each
adventurer to a specific model provider *and* you have a hard signal, semdragon
becomes a **measurement instrument** for provider behavior under identical
incentives. The research object is not a win-rate leaderboard — it is the
**divergence** between how good a provider's work *looks* (soft) and how good it
*is* (hard). A provider that scores high on review but fails executed
verification is reward-hacking the soft signal, and that gap is measurable,
per-provider, under controlled conditions. This is a novel, defensible research
contribution — provider-level Goodhart susceptibility under executed ground
truth — and a far better answer to "what is this game for" than cosmetics.

**Why now:** the model registry already abstracts capability→provider with
preferred/fallback chains; the agent entity already carries `Provider`/`Model`
fields; and questbridge partially honors a per-agent provider override
(`handler.go:2179`). Smart model routing needs *earned* provider×task outcomes
under ground truth rather than a published benchmark.

The former `processor/seeding` package is not an arena foundation. It was never
activated by shipped configuration, subscribed to its declared request port,
called outside its package, or connected to consumers. Issue #24 retires it.
This change therefore creates a new post-Phase-1 product arena by composing
SemStreams primitives instead of preserving or extending that dead seam.

## What Changes

- **Per-agent provider binding becomes first-class.** A provider/model binding
  is declared through authoritative Agent birth and mutation contracts, then
  honored consistently as the driver of model-endpoint resolution. Capability
  keys remain the fallback. Startup fixtures may request bindings through that
  boundary; they do not own a second Agent representation.

- **A new product arena runs a comparison program.** It fans **one** fixed
  quest specification with identical tools, prompt, and tier out to N
  provider-bound agents × R repetitions, isolating the confounds of emergent
  free play. R>1 remains a knob because sampling variance is real.

- **Attempts are captured as provider-tagged graph entities.** Each attempt
  records provider, the **soft** signal (peer/LLM review score, normalized), the
  **hard** signal (executed verification pass/fail via a *pluggable*
  `VerificationResult` adapter), and cost/latency/tokens. The hard-signal source
  starts as the weakest available machine-checkable check (e.g. deterministic
  acceptance/tests in the sandbox) and strengthens as the dev-substrate hermetic
  verification matures — the instrument is **not gated** on that landing.

- **A readout** exposes the (quest, provider) cells for analysis, surfacing the
  divergence `mean(soft) − mean(hard)`, the soft↔hard correlation per provider,
  and same-model self-review inflation (A rating A vs A rating B).

**Explicitly out of scope** (each a potential follow-on change): smart routing /
bandit over provider×quest-type; live provider leaderboards and dashboards;
statistical-significance testing; and the mixed-provider *party* peer-review
collusion study. This change delivers the instrument that makes those worth
doing.

## Capabilities

### New Capabilities

- `provider-binding`: A model provider/model is a first-class, declarable
  property of an authoritative Agent, set through its birth/ownership boundary
  and honored as the primary driver of model-endpoint resolution, with
  capability-key resolution as the fallback.
- `evaluation-arena`: A controlled comparison harness that fans one fixed quest
  across N provider-bound agents × R repetitions and captures each attempt as a
  provider-tagged graph entity carrying soft signal, hard signal (via a pluggable
  verification adapter), and cost/latency, with a readout over the (quest,
  provider) cells.

### Modified Capabilities

<!-- None. The model registry and arena areas this touches have no seeded specs
     yet (specs/ is empty); per the lazy-spec convention they are introduced as
     the new capabilities above rather than modifications. -->

## Impact

- **Code**
  - New product arena component: model each arena run as a SemStreams lifecycle
    `Participant` managed by the shared `Manager`; use rules for triggers and
    phase transitions.
  - Fan out bounded N×R work with rule `for_each` when orchestration owns the
    collection, or `pkg/dispatch.BoundedDispatcher` when the component owns
    internal parallel work.
  - Create first-class arena-run and arena-attempt graph entities through
    graph-ingest create-with-triples. Bind exact projection contracts and use
    owner-classified mutation for mutable result predicates.
  - Extend authoritative Agent birth/agentprogression APIs for provider/model
    binding; do not recreate a seeding-only Agent model.
  - `processor/questbridge`: make the per-agent provider binding the consistent
    driver of model-endpoint resolution (today the override at `handler.go:2179`
    is partial; capability keys `agent-work.{tier}.{skill}` stay as fallback).
  - New `VerificationResult` adapter interface (pluggable hard-signal source) and
    a seed implementation over the simplest available machine-checkable check.
  - New `arena.run` / `arena.attempt` graph entities + `arena.*` predicates
    (graph-primary: outcomes are queryable/renderable entities, not a private
    store), and a readout/query surface over them.
- **Config**
  - `model_registry`: real multi-provider endpoints (anthropic/openai/gemini/…).
  - New arena config: quest specification, providers, repetitions, and bounded
    concurrency. Startup fixture bindings use the authoritative Agent birth
    request.
- **Dependencies**
  - Issue #4 or its dedicated Agent-birth packet must land first so the arena
    does not write full Agent snapshots or borrow another owner's token.
  - Pluggable hard-signal adapter — starts weak, strengthens with the
    dev-substrate hermetic verification (per the product boundary, semdragon
    *consumes* verification; it must not re-grow a private sandbox). Not gated on
    the substrate.
  - Real-LLM arena runs cost money (N providers × R repetitions × quests) — the
    active-monitoring protocol applies.
- **Framework boundary**
  - The arena must not add a private lifecycle engine, private goroutine pool,
    direct shared-KV state, or event-`Graphable` state projection.
  - Lifecycle Manager/Participant, rules, bounded `for_each` or dispatcher,
    first-class entities, graph-ingest create-with-triples, and classified
    projection ownership are required composition points.
- **Coordination & sequencing** (assessed against the in-progress
  `rebuild-quest-lifecycle-on-semstreams-rules` change; this change must not
  touch it)
  - **No spec-level conflict.** rebuild-quest-lifecycle is silent on model
    resolution/provider and lists "model registry changes" as an explicit
    non-goal (its design.md §Non-Goals). provider-binding + registry work modify
    nothing that change's specs own.
  - **Mechanical merge point only:** both edit questbridge `handler.go`, but
    different functions — rebuild-quest-lifecycle the lifecycle (submit/fail)
    seam, this the model-resolution seam. Sequence to avoid a merge collision;
    no design conflict.
  - **This change is DOWNSTREAM of issue #4 and Phase 1.** Issue #4 establishes
    owner-safe Agent birth and the beta.158 primitives used above. The arena's
    *soft* signal is a real
    review score, but rebuild-quest-lifecycle (Phase 1) disables the mechanics
    that produce it — its quest-lifecycle spec stubs review as
    `in_review → completed` with a "review-skipped-mechanics-disabled" verdict
    (no score), and disables bossbattle. So the arena needs **either** Phase 2
    review re-enabled **or** the **party-lead review path**, which produces a
    3-question average score and *survives* mechanics-disable (it is exactly what
    rebuild-quest-lifecycle Stage B2 task 3.3 ports into `questdag`). Candidate
    soft-signal source to settle in design. The *hard* signal is independent
    (pluggable verification adapter). Start this change only after issue #4,
    Phase 1, and the selected review/hard-signal dependencies land.
