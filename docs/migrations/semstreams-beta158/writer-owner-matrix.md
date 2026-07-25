# SemStreams beta.158 writer-owner matrix

Status: architect-reviewed target for semdragon issue #3.

## Raw mutation inventory

GraphClient centralizes two product statements that mutate `ENTITY_STATES`
directly:

- `graphclient.go:123`: `KVStore.Put`, reached through `EmitEntity`,
  `EmitEntityUpdate`, and `PutEntityState`. This is last-writer-wins
  full-object replacement.
- `graphclient.go:153`: `KVStore.Update`, reached through `EmitEntityCAS`.
  This is CAS full-object replacement; CAS prevents a stale revision but not
  cross-owner predicate loss.

A third direct statement bypasses GraphClient:

- `processor/dmsession/session.go:183`: `KVStore.Put` serializes
  `domain.Session` and writes it under `session.<instance>` in
  `BoardConfig.BucketName()`. That method returns the shared `ENTITY_STATES`
  board bucket, not a separate dmsession bucket. The value is not
  `graph.EntityState`, and the key is not the canonical six-part session entity
  ID.

Non-test GraphClient callers are grouped below. Raw KV writes in boardcontrol,
dmapproval, tokenbudget, and API DM sessions target their separately classified
named buckets. That is a bucket-specific classification, not a package
exemption. Dmsession is explicitly excluded from that statement.

Framework mutation paths are:

- lifecycle Manager create/transition/operator update;
- graph-ingest triple mutation used by lifecycle command stamps;
- graph-ingest atomic create-with-triples used by questdag seeding;
- graph-ingest classified replace-by-predicate mutation used by the private
  questdag completion seam; and
- gated-DAG claim mutation.

## Owner-claim notation

- Patterns are canonical six-position globs:
  `org.platform.domain.system.type.instance`.
- Predicates are exact strings. No predicate prefix or wildcard is an owner
  claim.
- Tokens use SemStreams wire form `<owner>#<incarnation>`.
- Issue #3 defines registrations while pinned to beta.135. Issue #4 wires the
  beta.158 registry and stamps the tokens.
- `strict` foreign edges require the independently produced target to exist
  before the edge is written.
- Create-only predicates are birth facts, not `OwnerClaim.Predicates`.
- Append-evidence predicates use the framework append lane and have no
  replace-owned token.

## Quest entity claims

### Lifecycle

- Owner: `quest`; token `quest#<incarnation>`.
- Pattern: `*.*.game.*.quest.*`.
- Mode: `cas-transition`.
- Producer: `questlifecycle.Workflow` and lifecycle rule/operator actions.
- Exact predicates:
  `quest.status.state`, `quest.lifecycle.command`,
  `quest.assignment.requested-by`,
  `quest.attempts.current`, `quest.attempts.max`,
  `quest.assignment.agent`, `quest.assignment.party`,
  `quest.lifecycle.pending-triage`, `quest.lifecycle.claimed-at`,
  `quest.lifecycle.started-at`, `quest.lifecycle.completed-at`,
  `quest.failure.escalated`, `quest.failure.reason`,
  `quest.failure.type`, `quest.failure.history`,
  `quest.recovery.path`, `quest.recovery.analysis`,
  `quest.recovery.salvaged`, `quest.recovery.antipatterns`,
  `quest.verdict.passed`, `quest.verdict.score`,
  `quest.verdict.feedback`, `quest.verdict.xp-awarded`,
  `quest.transition.source`, `quest.transition.at`,
  `quest.transition.from`, and `quest.transition.note`.

Lifecycle is the sole owner of `quest.verdict.xp-awarded`. In Phase 2,
bossbattle supplies transition input through lifecycle; it does not write that
predicate directly.
The four transition predicates replace
`quest.lifecycle.last_transition_{source,at,from,note}`; the old namespace is
not registered as an alias.

### Quest creation

- Owner: `quest-create`; token `quest-create#<incarnation>`.
- Pattern: `*.*.game.*.quest.*`.
- Mode: create only; no replace-owned registration after birth.
- Producer: questboard/API creation.
- Exact predicates:
  `quest.identity.name`, `quest.identity.title`,
  `quest.identity.description`, `quest.difficulty.level`,
  `quest.classification.type`, `quest.tier.minimum`,
  `quest.party.required`, `quest.party.min-size`, `quest.xp.base`,
  `quest.skill.required`, `quest.tool.required`,
  `quest.acceptance.criterion`, `quest.data.input`,
  `quest.review.level`, `quest.review.needs-review`,
  `quest.lifecycle.posted-at`, `quest.parent.quest`,
  `quest.dependency.quest`, `quest.priority.guild`,
  `quest.classification.red-team-target`,
  `quest.classification.red-team-quest-id`,
  `quest.spec.goal`, `quest.spec.requirements`,
  `quest.spec.scenarios`, `quest.routing.class`, and
  `quest.context.repo`.

Red-team identity is create-only. Mutable red-team status is deliberately not
in this claim.

### Red-team status

- Owner: `redteam`; token `redteam#<incarnation>`.
- Pattern: `*.*.game.*.quest.*`.
- Mode: `replace-owned`.
- Producer: `processor/redteam`.
- Exact predicate: `quest.classification.red-team-status`.

### Questbridge

- Owner: `questbridge`; token `questbridge#<incarnation>`.
- Pattern: `*.*.game.*.quest.*`.
- Mode: `replace-owned`.
- Producer: `processor/questbridge`.
- Exact predicates:
  `quest.execution.loop-id`, `quest.context.token-count`,
  `quest.context.sources`, `quest.context.entities`,
  `quest.metrics.turns-used`, `quest.metrics.tokens-prompt`,
  `quest.metrics.tokens-completion`, `quest.metrics.duration`,
  `quest.data.output`, and `quest.dm.clarifications`.

Questbridge requests transitions through lifecycle. It does not claim
`quest.lifecycle.command`, phase, assignment, failure, or verdict.
Service/API clarification writes route through questbridge; they do not form a
second owner for `quest.dm.clarifications`.

### Quest DAG projection

- Owner: `questdag`; token `questdag#<incarnation>`.
- Pattern: `*.*.game.*.quest.*`.
- Mode: `replace-owned`.
- Producer: questdag projection controller.
- Exact predicates:
  `quest.dag.execution-id`, `quest.dag.definition`,
  `quest.dag.node-quest-ids`, `quest.dag.node-states`,
  `quest.dag.node-assignees`, `quest.dag.completed-nodes`,
  `quest.dag.failed-nodes`, `quest.dag.node-retries`,
  `quest.dag.node-id`, and `quest.dag.clarifications`.

### Quest artifacts

- Owner: `quest-artifacts`; token `quest-artifacts#<incarnation>`.
- Pattern: `*.*.game.*.quest.*`.
- Mode: `replace-owned`.
- Future producer: the reactivated bossbattle artifact controller.
- Exact mutable predicates:
  `quest.artifacts.merged` and `quest.artifacts.indexed`.
- Append evidence:
  `quest.relationship.produced`.

The artifact controller never replaces lifecycle, assignment, verdict, or
questbridge predicates. Produced-relationship entries are typed `@id` and are
append evidence so concurrent results cannot erase one another.

## DAG-unit claims

All rows use pattern `*.*.game.*.dagunit.*`. They intentionally overlap on the
entity pattern but not on predicates.

### Framework claimer

- Owner: gated-DAG framework; framework-minted token.
- Mode: `replace-owned`.
- Producer: beta.158 `processor/gated-dag`.
- Exact predicate: `gateddag.unit.claim`.

The framework owns only the claim marker.

### Questdag seeder

- Owner: `questdag`; token `questdag#<incarnation>`.
- Mode: create only; no replace-owned registration after birth.
- Producer: `questdag.SeedUnits`.
- Create-only facts, excluded from `OwnerClaim.Predicates`:
  `gateddag.unit.depends-on`, `questdag.unit.node-id`,
  `questdag.unit.parent-quest`, `questdag.unit.execution-id`,
  `questdag.unit.sub-quest`, and `questdag.unit.plan-digest`.

All six are ordinary predicates on the seeder-owned unit subject, not
`ForeignEdgeClaim`s; `questdag.unit.plan-digest` appears only on the activation
latch. Execution ID is the idempotency key. Validator-safe unit IDs are stable
functions of board, execution, and node identity, and the activation latch
stores the canonical plan digest so a different manifest under the same
execution ID is a collision.

The random-ID/two-phase protocol is rejected. The seeder atomically creates a
real deterministic activation unit with its provenance, plan digest, and typed
self-dependency. It then creates work units in prerequisite-first order. Each
work unit's single create-with-triples birth contains its complete immutable
provenance, every declared dependency, and the activation-latch dependency.
Every referenced target already exists, so the corpus contains no
`graph.StubMessageType` unit and no dependency-update phase.

Before release, the seeder authoritatively reconciles the complete manifest.
Matching stable entities are accepted on replay; a mismatched immutable fact or
digest fails as a collision. If the latch is already complete, a missing or
mismatched unit is graph corruption and is not repaired. The seeder does not
own any terminal or framework claim marker.

### Private completion seam

- Owner: `questdag-completion`; token
  `questdag-completion#<incarnation>`.
- Mode: `replace-owned`.
- Producer: private questdag completion writer used by work completion and
  activation-latch release.
- Exact predicate:
  `gateddag.unit.completed`.

This is the exclusive product writer for the completion predicate. It uses the
classified update-with-triples lane to atomically remove and replace the
single-valued marker with its owner token; production append-style
`triple.add` is prohibited. While issue #3 remains on beta.135, which lacks the
beta.158 decoded-entity validator, every mutation response is confirmed by
authoritative entity read-back. Issue #4 may trust a non-degraded echo only
after beta.158 validation. A degraded or lost reply also requires read-back;
marker presence converges to success, while marker absence preserves the
classified transient failure.

### Failure handler

- Owner: `questdag-failure`; token `questdag-failure#<incarnation>`.
- Mode: `replace-owned`.
- Producer: questdag dispatch/failure handling.
- Exact predicate:
  `gateddag.unit.failed`.

### Reset controller

- Owner: `questdag-reset`; token `questdag-reset#<incarnation>`.
- Mode: `replace-owned`.
- Producer: retry/reset controller.
- Exact predicate: `gateddag.unit.dirtied`.
- Dependency: SemStreams #382 supplies the supported mutator; issue #3 records
  the claim but does not copy an internal framework mutator.

## Agent and game entity claims

### Agent identity and progression

- Owner: `agentprogression`; token `agentprogression#<incarnation>`.
- Pattern: `*.*.game.*.agent.*`.
- Mode: `replace-owned`.
- Producer: `processor/agentprogression`.
- Exact mutable predicates:
  `agent.status.state`, `agent.status.cooldown-until`,
  `agent.progression.level`, `agent.progression.tier`,
  `agent.progression.xp-current`, `agent.progression.xp-to-level`,
  `agent.progression.death-count`, `agent.stats.quests-completed`,
  `agent.stats.quests-failed`, `agent.stats.bosses-defeated`,
  `agent.stats.total-xp-earned`, `agent.lifecycle.updated-at`,
  `agent.reputation.peer-avg`, `agent.reputation.peer-q1-avg`,
  `agent.reputation.peer-q2-avg`, `agent.reputation.peer-q3-avg`,
  `agent.reputation.peer-count`, and `agent.skill.proficiency`.
- Create-only facts, excluded from `OwnerClaim.Predicates`:
  `agent.identity.name`, `agent.identity.display-name`,
  `agent.identity.archetype`, `agent.npc.flag`,
  and `agent.lifecycle.created-at`.
- Append evidence:
  `agent.progression.xp-delta`, `agent.progression.xp-quest`,
  `agent.progression.xp-awarded`, and `agent.progression.xp-penalty`.

Guild/party membership and quest assignment move to the first-class membership
and assignment entities. Agentprogression does not claim their legacy
agent-entity projection predicates.

### Autonomy observation events

The autonomy component publishes seven live raw typed-subject event payloads.
Those publishes are not SemStreams `BaseMessage` graph-ingest writes: the
binary has no autonomy payload-registry factories and no route from the event
subjects to `graph.ingest.entity`. Issue #14 canonicalizes their declared
Graphable corpus while pinned to beta.135, but does not register an owner or
claim graph persistence.

The future graph contract contains append evidence, not owned current state:
`agent.autonomy.status`, `agent.autonomy.action`,
`agent.autonomy.interval-ms`, `agent.autonomy.idle-duration-ms`,
`agent.autonomy.has-suggestion`, `agent.autonomy.backoff-ms`,
`agent.autonomy.claim-quest`, `agent.autonomy.claim-score`,
`agent.autonomy.claim-rank`, `agent.autonomy.shop-item`,
`agent.autonomy.shop-cost`, `agent.autonomy.shop-budget`,
`agent.autonomy.shop-strategic`, `agent.autonomy.guild-join`,
`agent.autonomy.guild-score`, `agent.autonomy.guild-choices`,
`agent.autonomy.use-consumable`, `agent.autonomy.use-status`,
`agent.autonomy.use-quest`, `agent.autonomy.guild-create`, and
`agent.autonomy.guild-create-candidates`.

Exactly `agent.autonomy.claim-quest`, `agent.autonomy.guild-join`, and
`agent.autonomy.use-quest` are typed `@id`. They are edges on the payload's
agent subject, not writes to a foreign subject, so no `ForeignEdgeClaim` is
declared. Bare catalog values in `agent.autonomy.shop-item` and
`agent.autonomy.use-consumable` are scalar. `agent.autonomy.guild-choices` and
`agent.autonomy.guild-create-candidates` are scalar counts.

If issue #4 wires a real framework graph route, it binds these seven contracts
under static owner `autonomy`, pattern `*.*.game.*.agent.*`:

- `semdragons.autonomy.evaluated.v1`;
- `semdragons.autonomy.idle.v1`;
- `semdragons.autonomy.claimintent.v1`;
- `semdragons.autonomy.shopintent.v1`;
- `semdragons.autonomy.guildintent.v1`;
- `semdragons.autonomy.useintent.v1`; and
- `semdragons.autonomy.guildcreateintent.v1`.

Each contract contains one `append-evidence` group for the predicates emitted
by that payload, no foreign edges, and no indexing profile. Issue #4 uses
`projection.BindAndHeartbeat` with one process-lifetime static heartbeater,
bound once at the composition root after ownership initialization and before
the autonomy component starts. Although append evidence is overlap-exempt, it
is represented by an `OwnerClaim`; beta.158 liveness compaction exempts only
foreign-edge-only registrations.

If issue #4 does not add the real `BaseMessage`, payload-registry, and
graph-ingest route, it deletes the seven Graphable projections rather than
binding a phantom graph writer.

### Agent store aggregates and evidence

- Owner: `agentstore`; token `agentstore#<incarnation>`.
- Pattern: `*.*.game.*.agent.*`.
- Mode: `replace-owned`.
- Exact mutable predicates:
  `agent.inventory.total-spent`, `agent.inventory.tools-count`, and
  `agent.inventory.consumables-count`.
- Append evidence:
  `agent.purchase.item-id`, `agent.purchase.xp-spent`,
  `agent.consumable.effect-type`, `agent.consumable.expired-reason`,
  `agent.inventory.change-type`, `agent.inventory.item-id`, and
  `agent.inventory.quantity-change`.

The agentstore XP payload is graph-bound, not exempt. Its
`agent.progression.xp-current` projection is prohibited because
agentprogression owns that predicate. Agentstore requests the XP mutation
through the agentprogression owner API and removes the cross-owner triple.
Catalog item aliases in the evidence predicates above remain scalar.

### Store item

- Owner: `agentstore`; token `agentstore#<incarnation>`.
- Pattern: `*.*.game.*.storeitem.*`.
- Mode: `replace-owned`.
- Producer: `processor/agentstore`.
- Exact mutable predicate:
  `store.item.in-stock`.
- Create-only facts, excluded from `OwnerClaim.Predicates`:
  `store.item.name`, `store.item.description`, `store.item.type`,
  `store.item.purchase-type`, `store.item.xp-cost`,
  `store.item.min-tier`, `store.item.rental-uses`, `store.item.min-level`,
  `store.item.guild-discount`, `store.item.tool-id`,
  `store.item.effect-type`, `store.item.effect-magnitude`, and
  `store.item.effect-duration`.

The legacy three-part StoreItem entity-ID fallback is deleted. Catalog aliases
such as `quality_shield` remain scalar until converted by
`BoardConfig.StoreItemEntityID`.

### Guild target claims

The claims below are the approved target, not implemented ownership. Current
production has three full-object writers:

- guildformation calls `GraphClient.EmitEntity` for guild creation,
  membership, promotion, application review, activation, disband, and
  dissolution;
- agentprogression reconstructs and replaces the Guild for performance
  statistics and member contribution; and
- redteam reconstructs and replaces the Guild when appending lessons.

Guildformation also writes the Agent's embedded Guild field through
`EmitEntityUpdate`. None of these paths binds an owner claim, and any stale
snapshot can erase another writer's predicates.

Guild phase uses the SemStreams lifecycle substrate:

- Owner: Guild lifecycle Manager; framework-minted token.
- Pattern: `*.*.game.*.guild.*`.
- Mode: `cas-transition`.
- Producer: guildformation and rules request lifecycle transitions.
- Exact predicate: `guild.status.state`.

The remaining current Guild state uses the following approved target claim:

- Owner: `guildformation`; token `guildformation#<incarnation>`.
- Pattern: `*.*.game.*.guild.*`.
- Mode: `replace-owned`.
- Producer: guildformation; agentprogression and redteam submit inputs through
  the owner boundary rather than replacing Guild snapshots.
- Exact mutable predicates:
  `guild.quorum.size`, `guild.quorum.deadline`,
  `guild.stats.reputation`, `guild.stats.quests-handled`,
  `guild.stats.quests-failed`, `guild.stats.success-rate`,
  `guild.resource.tool`, `guild.routing.preferred-client`,
  `guild.routing.quest-type`, and `guild.knowledge.lessons`.
- Create-only facts, excluded from `OwnerClaim.Predicates`:
  `guild.identity.name`, `guild.identity.description`,
  `guild.identity.culture`, `guild.identity.motto`,
  `guild.identity.specialization`,
  `guild.config.max-members`, `guild.config.min-level`,
  `guild.founding.date`, `guild.founding.agent`, and
  `guild.lifecycle.created-at`.

The quorum predicates are exactly `guild.quorum.size` and
`guild.quorum.deadline`; `guild.config.quorum-size` is not a registered
predicate. `guild.lifecycle.disbanded-at` is not a target predicate: it exists
only in the deleted `GuildDisbandedPayload` graph projection and has no
authoritative Guild field or reader.

Dynamic `guild.member.<agent-id>.*` and
`guild.application.<application-id>.*` predicates do not enter either claim.
Issue #4 or a dedicated guild-state packet replaces them with the first-class
guild-membership and guild-application children declared below. The same
packet replaces the component-local pending-guild ticker with SemStreams rule
scheduling and lifecycle transitions. Ownership, classified mutation,
lifecycle, rules, and first-class children already cover this migration; issue
#22 needs no new upstream framework issue to delete the event Graphables.

### Party

- Owner: `partycoord`; token `partycoord#<incarnation>`.
- Pattern: `*.*.game.*.party.*`.
- Mode: `replace-owned`.
- Producer: `processor/partycoord`.
- Exact mutable predicates:
  `party.status.state`, `party.membership.count`,
  `party.lifecycle.disbanded-at`, `party.coordination.strategy`,
  `party.coordination.decomposed-by`,
  `party.coordination.parent-quest`,
  `party.coordination.sub-quest-count`,
  `party.coordination.sub-quest`, `party.context.count`,
  `party.results.count`, `party.disband.reason`,
  `party.rollup.started-at`, `party.rollup.sub-results-count`,
  `party.rollup.completed-at`, and `party.rollup.status`.
- Create-only facts, excluded from `OwnerClaim.Predicates`:
  `party.identity.name`, `party.assignment.quest`,
  `party.membership.lead`, and `party.lifecycle.formed-at`.

Dynamic members, assignments, and submissions move to their first-class
patterns below and are not included in the party claim.

### Boss battle

Current Phase-1 state: dormant. No shipped profile activates
`processor/bossbattle`, and bossbattle binds no projection owner or heartbeat.
Factory registration, readers, the evaluator, and the battle-state KV port are
retained code, not an active ownership claim. The current indexed
`BossBattle.Triples` writer is not approved for activation.

The approved Phase-2 target is:

- Owner: `bossbattle`; token `bossbattle#<incarnation>`.
- Pattern: `*.*.game.*.battle.*`.
- Mode: `replace-owned`.
- Producer: the reactivated `processor/bossbattle`.
- Exact mutable predicates:
  `battle.status.state`, `battle.review.outcome`,
  `battle.review.results`, `battle.lifecycle.completed-at`,
  `battle.verdict.passed`, `battle.verdict.quality-score`,
  `battle.verdict.xp-awarded`, `battle.verdict.xp-penalty`,
  `battle.verdict.level-change`, `battle.verdict.feedback`,
  `battle.results.count`, and `battle.results.passed`.
- Create-only facts, excluded from `OwnerClaim.Predicates`:
  `battle.identity.name`, `battle.assignment.quest`,
  `battle.assignment.agent`, `battle.review.level`,
  `battle.review.criteria`, `battle.review.judges`,
  `battle.lifecycle.started-at`, and `battle.execution.loop-id`.

The repeated criteria, judges, and results replace the scanner-hidden
`battle.criteria.<index>.*`, `battle.judge.<index>.*`, and
`battle.result.<index>.*` families on a fresh graph. No wildcard owner claim,
legacy reader, alias, dual write, or in-place rewrite is allowed.

Bossbattle owns only the future battle projection. Every foreign effect uses
the exact authoritative boundary:

- quest phase, assignment, attempts, failure, verdict, and completion use the
  quest lifecycle Manager or rule-command boundary;
- agent status and progression use the `agentprogression` owner API;
- peer-review birth and completion use the review create and lifecycle owners;
- artifact merge/index state uses the `quest-artifacts` owner;
- DAG failure, reset, and rollup use questdag and quest lifecycle; and
- agent-loop cancellation uses the typed framework `UserSignal`.

No bossbattle path may replace a Quest, Agent, PeerReview, or other
cross-owner full object. Reactivation is blocked until these handoffs, the
fresh-state projection tests, the owner bind/heartbeat, and the
[SemStreams #313](https://github.com/C360Studio/semstreams/issues/313)
mutation helper are complete.

## Peer-review ownership

### Review creation

- Owner: `review-create`; token `review-create#<incarnation>`.
- Pattern: `*.*.game.*.peerreview.*`.
- Mode: create only.
- Producers: questdag review creation and review API creation; reactivated
  bossbattle joins only in Phase 2.
- Exact predicates:
  `review.assignment.quest`, `review.assignment.leader`,
  `review.assignment.member`, `review.assignment.party`,
  `review.config.solo-task`, and `review.lifecycle.created-at`.

### Dedicated review workflow

- Owner: `review`; token `review#<incarnation>`.
- Pattern: `*.*.game.*.peerreview.*`.
- Mode: `cas-transition`.
- Producer: dedicated review lifecycle workflow.
- Exact predicates:
  `review.status.state`, `review.leader.q1`, `review.leader.q2`,
  `review.leader.q3`, `review.leader.explanation`,
  `review.leader.submitted-at`, `review.member.q1`,
  `review.member.q2`, `review.member.q3`,
  `review.member.explanation`, `review.member.submitted-at`,
  `review.result.leader-avg`, `review.result.member-avg`, and
  `review.lifecycle.completed-at`.

API and questdag submit review transitions through this workflow. Reactivated
bossbattle must do the same; none replaces the review entity.

## First-class child entity claims

All use `<owner>#<incarnation>`. Each registration contains only the exact
mutable set; the listed birth facts are excluded from
`OwnerClaim.Predicates`.

- `agentstore`, pattern `*.*.game.*.inventory-entry.*`.
  Mutable: `inventory.state.quantity` and
  `inventory.state.uses-remaining`.
  Create-only: `inventory.assignment.agent`,
  `inventory.assignment.item`, `inventory.state.purchase-type`,
  `inventory.state.xp-spent`, and `inventory.lifecycle.purchased-at`.
  `inventory.state.quantity` is the consumable quantity/count.
- `agentstore`, pattern `*.*.game.*.active-effect.*`.
  Mutable: `effect.state.remaining`.
  Create-only: `effect.assignment.agent`,
  `effect.assignment.consumable`, `effect.assignment.quest`,
  `effect.state.type`, `effect.state.magnitude`, and
  `effect.lifecycle.activated-at`.
- `guildformation`, pattern `*.*.game.*.guild-membership.*`.
  Mutable: `membership.state.rank` and
  `membership.metrics.contribution`.
  Create-only: `membership.assignment.guild`,
  `membership.assignment.agent`, and `membership.lifecycle.joined-at`.
- `guildformation`, pattern `*.*.game.*.guild-application.*`.
  Mutable: `application.assignment.reviewer`, `application.state.status`,
  and `application.lifecycle.reviewed-at`.
  Create-only: `application.assignment.guild`,
  `application.assignment.applicant`, `application.spec.level`,
  `application.spec.tier`, `application.spec.reason`,
  `application.spec.message`, repeated `application.spec.skills`, and
  `application.lifecycle.created-at`.
- `partycoord`, pattern `*.*.game.*.party-membership.*`.
  Mutable: `membership.state.role`.
  Create-only: `membership.assignment.party`,
  `membership.assignment.agent`, and `membership.lifecycle.joined-at`.
- `partycoord`, pattern `*.*.game.*.party-assignment.*`.
  Mutable: `assignment.state.rationale`.
  Create-only: `assignment.relationship.party`,
  `assignment.relationship.quest`, `assignment.relationship.agent`, and
  `assignment.lifecycle.assigned-at`.
- `partycoord`, pattern `*.*.game.*.party-submission.*`.
  Mutable: none after creation.
  Create-only: `submission.relationship.party`,
  `submission.relationship.quest`, `submission.relationship.member`,
  `submission.result.output`, `submission.result.quality`, and
  `submission.lifecycle.submitted-at`.

No parent-entity reconcile includes predicates from these child entities.
Bare store catalog aliases are scalar. `inventory.assignment.item` and
`effect.assignment.consumable` carry `@id` only after
`BoardConfig.StoreItemEntityID` conversion and the table-driven validator,
datatype, and round-trip evidence specified in the rename ledger.

## Executor lifecycle observations

Executor and questbridge both publish the three live execution lifecycle
payload families as raw typed-subject JSON. No payload factory or graph-ingest
route exists, so issue #16 declares no owner and adds no binding.

Their prospective graph shapes contain 15 unique predicates under pattern
`*.*.game.*.quest.*`. Each started, completed, or failed shape uses one
`append-evidence` group, no foreign edges, and no indexing profile.
`execution.agent.id` is the only typed relationship and is an edge on the
payload's quest subject.

Issue #4 either deletes these Graphables or wires a real framework graph route.
If executor and questbridge both become graph publishers, each binds only the
shapes it actually emits under its own owner ID. If one component projects the
shared event subjects, that projector is the owner. Every static owner uses
`projection.BindAndHeartbeat` at the composition root; the payload package does
not imply ownership.

The unproduced tool-call and tool-result Graphables are deleted in issue #16.
Issue #7 owns any later tool observation contract through SemStreams
agentic-tools and trajectories.

## Executor web-fetch target claim

Issue #16 canonicalizes this real writer's predicate names and relationships,
but does not yet activate the following owner claim or replace its full-object
write:

- Owner: `executor-web-fetch`; token
  `executor-web-fetch#<incarnation>`.
- Pattern: `*.*.web.agent.doc.*`.
- Mode: `replace-owned`.
- Producer: `processor/executor.persistWebContent`.
- Exact predicates:
  `source.doc.content`, `source.doc.summary`, `source.doc.url`,
  `source.doc.mime-type`, `source.doc.scope`,
  `source.doc.fetched-by`, `source.doc.quest-id`,
  `dc.terms.title`, and `dc.terms.modified`.

This resolves the previously ambiguous executor/semsource ownership: the
executor web-fetch producer owns this exact source-document pattern and set.
The pattern is disjoint from Semsource-owned document patterns even where
predicate names overlap. A later web-document packet binds the replace-owned
contract with `projection.BindAndHeartbeat` and moves `GraphClient.EmitEntity`
to the supported owner-classified mutation lane.

`source.doc.fetched-by` and `source.doc.quest-id` are typed same-subject
relationships and require no `ForeignEdgeClaim`. SemStreams supplies
`vocabulary.DCTermsTitle`. The temporary `dc.terms.modified` declaration is a
compatibility surface linked to SemStreams #673, not product-owned vocabulary,
and is deleted on adoption of a tagged upstream implementation.

## DM-session raw-state misuse

The dmsession event DTOs are not graph writers. Their internal typed-subject
publisher methods emit raw JSON, and production methods in
`processor/dmsession/component.go` call the manager methods internally. No
shipped configuration activates the factory-registered component, no
production code outside `processor/dmsession` calls its component entrypoints,
and no payload-registry factory, `BaseMessage` route, graph-ingest route,
projection contract, or graph reader exists. Issue #20 deletes their graph-only
methods, assertions, tests, and latent predicates while preserving the raw
event subjects and JSON contract.

`SessionManager.putSession` is a different boundary. It stores raw
`domain.Session` JSON under `session.<instance>` directly in the shared board
`ENTITY_STATES` bucket. This bypasses the canonical `graph.EntityState` value
shape, six-part entity key, graph-ingest mutation lane, and owner
classification. It is downstream misuse of framework-owned state, not an
event projection and not evidence for a dmsession owner claim.

Issue #20 does not invent a replacement writer. Issue #4 or a dedicated packet
must decide whether session state is still required, then use SemStreams
lifecycle and ownership primitives for canonical birth and classified
mutation. Any static owner binds and heartbeats at the real composition root.
If lifecycle Manager owns the workflow, it supplies that framework lifecycle.
No event-derived writer may coexist with the authoritative state writer.

## Graph-bound payload classification

Implementing `Graphable` or supplying triples to a graph-emitting client makes
a payload graph-bound. Message/event use does not make its predicates
non-graph. The production payload families are classified as follows:

The current prompt-assembly `Graphable` is not a production payload family.
Source and history review found no publisher, payload-registry registration, or
lifecycle component. Issue #12 deletes its type, typed subject, vocabulary
entry, and relationship fixture rather than assigning a phantom owner or
heartbeat. The historical predicate-audit findings remain in
`baseline-audit.md`.

The automated absence guard is
`TestPromptAssemblyHasNoGraphProjectionSurface` in
`prompt_projection_inventory_contract_test.go`. It stale-checks the relevant
production source inventory and forbids only the deleted prompt surface and
external promptmanager registration hooks. It does not ban general framework
projection contracts, ownership services, heartbeats, or lifecycle timers.

- Agentprogression current state uses its exact owner claim. XP delta, quest,
  award, and penalty observations use append evidence.
- Agentstore current inventory totals use its exact owner claim. Purchase,
  consumable, and inventory-change observations use append evidence.
  `agent.progression.xp-current` is removed from agentstore `Triples` and is
  sent through the agentprogression owner API.
- Autonomy action/decision predicates listed above use append evidence.
  Relationship datatypes still obey the catalog-alias conversion boundary.
- Questboard/questbridge/questdag, partycoord, and review payloads either call
  the owner workflow, create the classified first-class entity, or append
  their documented evidence. Dormant bossbattle binds no owner; Phase 2 must
  use the same classified lanes. Former cross-owner projections are removed,
  not silently exempted.
- Guildformation's five raw event DTOs remain active wire contracts, but their
  latent Graphables are deleted in issue #22. Guild events are not append
  evidence, and no event projection owner is registered.
- Executor execution observations use append evidence. The entire unactivated
  and unconsumed seeding processor is deleted in issue #24; its statistics are
  not append evidence, and no raw DTO or graph contract remains. The
  definition-only DM-session event Graphables and their counters are deleted;
  the separate raw session-state write is the downstream misuse described
  above. Web document state uses the executor web-fetch owner claim.
- `quest.failed` is proved non-graph only at the two questboard and one service
  expressions recorded in the rename ledger: there it is
  `EntityState.MessageType.Category`, never `Triple.Predicate`.

The guild event projections `guild.event.created_by`,
`guild.membership.member`, `guild.membership.left`, and dynamic event rank facts
are deleted in issue #22. Authoritative `guild.membership.agent` remains until
issue #4 creates first-class membership children. Legacy payload projections
for party membership/assignment/submission are removed when their
corresponding first-class entities are created. The seeding-event
`agent.guild.member` projection and all four seeding fixture rows are deleted
outright in issue #24. A producer may not keep both representations merely
because both are graph-bound.

### Startup/E2E Agent birth

The retained `cmd/semdragons/seed_e2e.go` path is not the deleted seeding
processor. It runs only for `SEED_AGENTS`, `SEED_E2E`, or `SEED_PROS` and
currently calls `GraphClient.EmitEntity` with a complete
`agentprogression.Agent` snapshot.

- Current scope: create only against an empty fresh corpus.
- Create-only facts:
  Agent identity, archetype, NPC flag, and creation time remain outside
  `OwnerClaim.Predicates`.
- Handoff:
  issue #4 or a dedicated startup-birth packet uses graph-ingest
  `create_with_triples` and the agentprogression owner API for initial mutable
  status, progression, statistics, and skills.
- Prohibition:
  startup code does not borrow or hand-compose the agentprogression owner token
  and does not write full Agent state directly into shared KV after adoption.

The future provider arena is a separate post-Phase-1 product composition using
SemStreams lifecycle Manager/Participant, rules, bounded fan-out or dispatcher,
first-class run/attempt entities, graph-ingest creation, and classified
projection ownership. The deleted processor exposes no upstream framework gap.

If prompt-assembly evidence is introduced later, the actual publishing
component binds the SemStreams contract under its own owner ID. All six
prompt-assembly predicates use append evidence on
`*.*.game.*.quest.*`. The typed `prompt.assembly.agent-id` relationship is
same-subject data because its triple subject equals the payload quest entity;
it is not a `ForeignEdgeClaim`. A triple's `Source` metadata does not establish
the contract owner.

## Foreign-edge claims

A SemStreams `ForeignEdgeClaim` is not every `@id` relationship. It applies
when a Graphable emits a triple whose subject differs from its own entity ID.
The source sweep finds exactly three such production projections, all in
`processor/partycoord/payloads.go`.

### Allowed during issue #3

- Owner: `partycoord`; token `partycoord#<incarnation>`.
- Producer: `semdragons.party.joined.v1`.
- Predicate: `agent.membership.party`.
- Target pattern: `*.*.game.*.agent.*`.
- Edge mode: `strict`; the agent has independent birth.

This projection is deleted when party membership becomes first-class. Until
then, agentprogression must not include `agent.membership.party` in a
replace-owned claim.

### Prohibited overlap

`semdragons.party.assigned.v1` currently emits
`quest.assignment.agent` and `quest.assignment.party` on a sub-quest subject.
Both exact predicates belong to the quest lifecycle `cas-transition` claim.
Registering partycoord foreign-edge claims would create an owner/foreign-edge
overlap, so issue #3 removes those two payload projections. Partycoord creates
the first-class party-assignment entity and requests the quest transition
through lifecycle.

All other `@id` predicates in this matrix have the owner entity as their
subject, so they are ordinary owned relationship triples, not foreign-edge
claims. Their target-birth rule is `strict`.

`gateddag.unit.depends-on` is one of those ordinary questdag-owned predicates.
The activation unit is atomically born with its self-edge, for which
graph-ingest creates no referential stub. Work units are born
prerequisite-first with their complete edge set and activation dependency, so
the target-birth rule remains strict. Every relationship uses
`internal/semcompat.EntityReferenceDatatype`.

## Legacy caller disposition

- questboard and status-mutating service API handlers: temporary; delete after
  lifecycle callers migrate.
- questbridge: keep only its exact owner-scoped mutations.
- questdagexec: temporary; delete after the private questdag completion,
  failure, and reset controllers own their exact marker sets.
- autonomy, bossbattle, redteam, guildformation, and partycoord: replace
  cross-entity full-object writes with owner APIs or first-class child creates.
- agentprogression and agentstore: replace full-object writes with exact
  owner-scoped mutation.
- startup/e2e seed: retain the empty-corpus create-only workflow, then migrate
  its mutable Agent birth state through the agentprogression owner boundary.

## Exhaustive coverage and registration gate

The gate builds a production-emission inventory from every `Graphable.Triples`
implementation and every direct graph-client triple construction. Its key is
the concrete `(subject pattern, predicate, producer expression)`; dynamic
predicate construction fails before classification.

Each inventory row must resolve to exactly one of:

1. an exact predicate in one registrable `OwnerClaim`;
2. a create-only birth fact, absent from every `OwnerClaim.Predicates`;
3. append evidence using the framework append mode;
4. one registrable `ForeignEdgeClaim`, with a demonstrably foreign subject; or
5. a proved non-graph expression-level annotation backed by a test of the
   actual destination field.

Zero matches and multiple matches both fail. The same test registers all
owner and foreign-edge claims against the beta.158 registry contract and fails
on pattern/predicate overlap, invalid six-position patterns, wildcard
predicates, unregistrable modes, or an owner token outside
`<owner>#<incarnation>`. It also asserts that the DAG seed,
store/guild/party/battle, and first-class-child create-only sets never enter
their mutable claims.

The three `quest.failed` expressions are the only initial non-graph rows.
Directory-, package-, and filename-wide exemptions are forbidden. This gate,
the pinned predicate audit, relationship coverage, and generated-schema/UI
contract tests must all pass before any writer starts.

The deterministic activation latch is a narrow issue #3 product bridge, not a
local gated-DAG framework. It is retired when
[SemStreams #670](https://github.com/C360Studio/semstreams/issues/670) supplies
framework-owned staged fan-out admission and the beta.158 adoption slice
consumes that primitive.

## Upstream framework boundaries

- [SemStreams #313](https://github.com/C360Studio/semstreams/issues/313) is the
  existing public ask for a reusable owned-write helper. The
  [Semdragon evidence comment](https://github.com/C360Studio/semstreams/issues/313#issuecomment-5078996034)
  records this repository's store, inventory, effect, guild, party, and DAG
  mutation needs. It does not block the completed questdag core wave because
  that wave retains only the narrow private completion adapter.
- [SemStreams #671](https://github.com/C360Studio/semstreams/issues/671) owns
  occurrence-exact, stale-checked predicate-audit classifications for the
  three non-predicate `quest.failed` expressions. It does not block the DAG
  wave; the broader zero-findings predicate-audit gate remains open.
- [SemStreams #670](https://github.com/C360Studio/semstreams/issues/670) owns
  generic staged fan-out admission and retires the activation latch.
- [semdragon #4](https://github.com/C360Studio/semdragon/issues/4) owns
  beta.158 adoption, decoded-entity validation, and registry wiring.

## Approved questdag core-wave evidence

The 2026-07-25 live verification closes only OpenSpec tasks 2.7, 2.8, and 3.7:

- `go test ./...` and `go test -race ./questdag` pass.
- Focused live graph-ingest/gated-DAG integration proves completion read-back,
  zero early dispatch at every create boundary, replay convergence, activated
  manifest corruption rejection, root-only activation release, and a final
  corpus without `graph.StubMessageType` units.
- `task ui:test` and `task schema:check` pass.
- `task contract:predicates-core` passes for `domain`, `questdag`,
  `questlifecycle`, and `config`.
- `openspec validate adopt-semstreams-prev1-contract --strict` passes.

This evidence does not close the processor/service predicate corpus,
repository-wide entity-ID and relationship gates, fresh-volume runbook, or
beta.158 adoption.
