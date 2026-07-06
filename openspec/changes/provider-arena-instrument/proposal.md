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

**Why now:** the seams already exist, so this is a small lift with outsized
payoff. The model registry already abstracts capability→provider with
preferred/fallback chains; the agent entity already carries `Provider`/`Model`
fields; questbridge already partially honors a per-agent provider override
(`handler.go:2179`); and the seeding processor already has a `ModeTrainingArena`
seam (today a simplified grinding harness). Smart model routing is the industry
obsession right now, but a good router needs *earned* data — provider×task
outcomes under ground truth. This instrument generates exactly that data, so the
router (a later change) is built on measured reality instead of a published
benchmark. Building the instrument first is the correct sequence.

## What Changes

- **Per-agent provider binding becomes first-class.** A provider/model binding
  is declarable at roster seeding, populated onto the agent entity, and honored
  consistently as the driver of model-endpoint resolution (capability keys
  remain the fallback). A roster or party can then be composed of different
  providers — "a Claude adventurer, a GPT adventurer, a Gemini adventurer."

- **The arena gains a comparison program.** The existing arena seam runs a
  *progression* program (grind agents to a level distribution). This adds a
  *comparison* program that fans **one** fixed quest spec (identical tools,
  prompt, tier) out to N provider-bound agents × R repetitions, isolating the
  confounds the emergent free-play game introduces. R>1 is a knob because
  sampling variance is real.

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
  property of an agent, populated at seeding and honored as the primary driver
  of model-endpoint resolution (with capability-key resolution as the fallback).
- `evaluation-arena`: A controlled comparison harness that fans one fixed quest
  across N provider-bound agents × R repetitions and captures each attempt as a
  provider-tagged graph entity carrying soft signal, hard signal (via a pluggable
  verification adapter), and cost/latency, with a readout over the (quest,
  provider) cells.

### Modified Capabilities

<!-- None. The model registry and seeding areas this touches have no seeded specs
     yet (specs/ is empty); per the lazy-spec convention they are introduced as
     the new capabilities above rather than modifications. -->

## Impact

- **Code**
  - `processor/seeding`: add a provider/model binding to `AgentSpec`; populate
    `Agent.Provider`/`Agent.Model` on seed; add the arena *comparison* program
    alongside the existing progression program.
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
  - seeding config: provider on `AgentSpec`; arena comparison-program config
    (quest spec, providers, repetitions).
- **Dependencies**
  - Pluggable hard-signal adapter — starts weak, strengthens with the
    dev-substrate hermetic verification (per the product boundary, semdragon
    *consumes* verification; it must not re-grow a private sandbox). Not gated on
    the substrate.
  - Real-LLM arena runs cost money (N providers × R repetitions × quests) — the
    active-monitoring protocol applies.
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
  - **This change is DOWNSTREAM of Phase 1.** The arena's *soft* signal is a real
    review score, but rebuild-quest-lifecycle (Phase 1) disables the mechanics
    that produce it — its quest-lifecycle spec stubs review as
    `in_review → completed` with a "review-skipped-mechanics-disabled" verdict
    (no score), and disables bossbattle. So the arena needs **either** Phase 2
    review re-enabled **or** the **party-lead review path**, which produces a
    3-question average score and *survives* mechanics-disable (it is exactly what
    rebuild-quest-lifecycle Stage B2 task 3.3 ports into `questdag`). Candidate
    soft-signal source to settle in design. The *hard* signal is independent
    (pluggable verification adapter). ⇒ start this change after Phase 1 lands.
