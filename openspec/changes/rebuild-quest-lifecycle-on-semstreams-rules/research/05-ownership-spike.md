# Ownership spike — beta.132 quest-entity write/read + predicate ownership map

Task 1.3 deliverable. Two parts: (A) empirical confirmation that a quest entity writes
and reads correctly under beta.132's graph/ownership layer with the *existing*
architecture, and (B) the `quest.*` predicate ownership partition for Stage B (which
predicates the lifecycle owner claims vs questbridge vs questdag), so rules and
questbridge don't clobber each other.

## A. Empirical result

`go test -tags integration -count=1 ./processor/questboard/...` → **PASS (14.0s)** under
semstreams beta.132.

This exercises the contested write path end to end against a real testcontainers NATS:
- `TestComponent_QuestLifecycle` — full `posted → claimed → in_progress → submitted →
  completed`, each a read-modify-write of the whole quest struct via
  `GraphClient.EmitEntity` (raw KV `Put` to `ENTITY_STATES`).
- `TestComponent_ClaimQuest` / `TestClaimSetsAgentOnQuest` — CAS claim via
  `EmitEntityCAS` (KV `Update` with revision).
- `TestClaimQuest_BlockedByUnmetDependency` / `..._AllowedAfterDependencyCompleted` —
  dependency-gated claim.

**Conclusion:** beta.132's ownership layer (ADR-056) is *observe-only* on this tag —
`OwnerToken` is stamped but not compared at write time — and semdragons writes full
objects directly to KV, bypassing graph-ingest's mutation API entirely. So the existing
write/read path is unaffected by the bump. No ownership registration is required to keep
the current architecture green (confirms Stage A gate for the graph cluster).

## B. The clobbering problem (why Stage B needs a partition)

`BucketName()` = `"ENTITY_STATES"` (`domain/config.go:161`) — the same bucket
graph-ingest owns. Five components read-modify-write the **whole** quest struct and `Put`
it: questboard, questbridge, questdagexec, bossbattle, service/api. Today they serialize
on KV revision (CAS) but each write carries *every* `quest.*` predicate, so a stale writer
silently reverts another owner's predicate group (last-writer-wins on non-CAS `Put`).

When Stage B introduces the lifecycle Manager, the Manager writes the phase predicate via
graph-ingest triple mutation with `ExpectedRevision` CAS (ADR-049), while questbridge still
full-object `Put`s — they would fight over `quest.status.state`. The fix is a disjoint
ownership partition: each owner claims a predicate group, and questbridge stops full-object
`Put` in favor of `update_with_triples` scoped to its own group (ADR-055 update lane is
replace-by-(subject,predicate), so it preserves predicates it doesn't touch).

## C. Predicate ownership partition (grounded in the real `quest.*` namespace)

Groups are prefix-disjoint so ADR-056 registration (overlap-checked) accepts them.

### 1. Lifecycle-owned — claim `quest.status.*`, `quest.lifecycle.*`, `quest.attempts.*`, `quest.failure.*`, `quest.assignment.*`, `quest.verdict.*`, `quest.data.output`
Written **only** by the lifecycle Manager / rule actions, atomically with the phase
transition. Write mode `cas-transition` for the phase predicate; the rest are the
transition mutator's `set` fields.

| Predicate(s) | Role | Notes |
|--------------|------|-------|
| `quest.status.state` | **phase predicate** (D3) | `cas-transition`; unchanged phase strings for UI stability |
| `quest.lifecycle.status` | legacy mirror of phase | keep writing for UI/back-compat, or map; see risk R1 below |
| `quest.lifecycle.{posted,claimed,started,submitted,completed,failed,escalated,abandoned,decomposed,triaged,pending_triage}` | transition markers | audit predicates; set on the matching edge |
| `quest.lifecycle.{posted_at,claimed_at,started_at,completed_at}` | transition timestamps | set in the transition mutator |
| `quest.attempts.current`, `quest.attempts.max` | retry counter | `operator_writable`; `current` incremented in fail→requeue `lifecycle_transition.set`; drives rule branch `lt max`(retry) vs `gte max`(triage) — task 2.3 |
| `quest.failure.{reason,type,history,escalated}` | failure info | set on fail/escalate edge |
| `quest.assignment.agent`, `quest.assignment.party` | current assignee | set atomically with claim; cleared on abandon/fail-repost |
| `quest.verdict.{passed,score,feedback,xp_awarded}` | review verdict | in THIS change the stub rule sets `verdict: review-skipped-mechanics-disabled` on `in_review→completed`; `xp_awarded` stays 0 (mechanics off) |
| `quest.data.output` | work product | written by the submit/complete transition mutator (spec: "persisted as quest.data.output") |

### 2. questbridge-owned — claim `quest.execution.*`, `quest.context.*`, `quest.loop.*`, `quest.metrics.*`
Written by questbridge during dispatch/execution. Write mode `replace-owned` via
`update_with_triples`. Must NOT be included in any full-object `Put` of the quest.

| Predicate(s) | Role |
|--------------|------|
| `quest.execution.loop_id` | agentic loop correlation (mirrors QUEST_LOOPS) |
| `quest.loop.assigned` | loop assignment marker |
| `quest.context.{assembled,entities,repo,sources,token_count}` | assembled prompt context/accounting |
| `quest.metrics.{tokens_prompt,tokens_completion,turns_used,rollup}` | execution metrics |

### 3. questdag-owned — claim `quest.dag.*`
The DAG projection on the parent quest, re-derived from the gated-dag unit model in Stage
B2 (task 3.5). Owned by the questdag layer; write mode `replace-owned`. All
`quest.dag.{definition,execution_id,node_states,node_assignees,node_quest_ids,node_retries,completed_nodes,failed_nodes,clarifications,decomposed,...}`.

### 4. Creation-time immutable — no ownership claim needed (written once at post, then read-only)
`quest.identity.*`, `quest.spec.*`, `quest.difficulty.*`, `quest.skill.required`,
`quest.tier.minimum`, `quest.tool.required`, `quest.review.*`, `quest.xp.base`,
`quest.priority.guild`, `quest.party.*`, `quest.tag.*`, `quest.acceptance.*`,
`quest.classification.*` (red-team; mechanics-off this change), `quest.dependencies.*`,
`quest.dependency.quest`, `quest.parent.quest`, `quest.data.input`, `quest.routing.class`,
`quest.recovery.*`, `quest.dm.*`, `quest.progress.percent`.
(Triage fields `quest.recovery.*`/`quest.dm.*` are `operator_writable` so the DM/triage path
can amend them; the rest are set by the creating component and never mutated.)

*(`quest.nonexistent.predicate`, `quest.optional.field` are test-only fixtures in
`domain/graphable_test.go` — not part of the real namespace.)*

## D. Registration order (Stage B3 / task 4.1, 4.3)

Per ADR-056, `ownership.EnsureBuckets` (creates `OWNER_CLAIMS`) must run in
`cmd/semdragons/main.go` **before** components register their claims, then `projection.Bind`
per owner. Claims are `(pattern, predicate set, write mode, owner id)`; the three owned
groups above are registered by their respective components at Start. Double-claiming the
phase predicate fails registration by design — that is the guard that prevents the
clobbering.

## E. Risks for Stage B

- **R1 — dual status predicates.** Both `quest.status.state` and `quest.lifecycle.status`
  exist today. D3 names `quest.status.state` the phase predicate. `quest.lifecycle.status`
  must stay in sync (UI/back-compat). Options: (a) keep the lifecycle owner writing both in
  the transition mutator, or (b) drop `quest.lifecycle.status` and ship a UI mapping.
  Decide in task 2.1; prefer (a) to preserve UI-predicate stability (spec requirement).
- **R2 — `quest.data.output` boundary.** It is assembled by the executor/questbridge but
  semantically a lifecycle-transition output. Assigned to the lifecycle group (written by
  the submit/complete mutator) so review routing can gate on it; questbridge hands the value
  to the transition rather than `Put`-ing it directly.
- **R3 — observe-only enforcement.** Because beta.132 does not *reject* unauthorized writes
  yet, a missed migration of a full-object `Put` won't fail loudly — it will silently clobber.
  Mitigate by grepping for residual `EmitEntity`/`EmitEntityCAS` full-object writes of the
  quest after each component is ported (tasks 2.5, 3.6) and asserting predicate stability in
  the mock e2e (task 5.3).
