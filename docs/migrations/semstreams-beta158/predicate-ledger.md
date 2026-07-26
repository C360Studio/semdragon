# SemStreams beta.158 predicate rename ledger

Status: architect-reviewed target contract for semdragon issue #3. This ledger is
the implementation source of truth. It is not an alias table: each approved
scalar mapping writes and reads only the canonical identity. Scanner-hidden
indexed-family compatibility is called out explicitly where it remains.

## Audit coverage

The pinned declared-root command is:

```sh
go run github.com/c360studio/semstreams/cmd/predicate-audit@v1.0.0-beta.158 \
  ./domain ./questdag ./questlifecycle ./config
```

It reports exactly 63 invalid candidates from 336 extracted:

- `domain`: 47 findings.
- `questdag`: 10 findings.
- `questlifecycle`: one finding.
- `config`: five findings.

SemStreams #667 prevents one safe repository-wide invocation. It does not
justify an exemption. The processor corpus is audited one package at a time:

```sh
for root in processor/*; do
  test -d "$root" || continue
  go run github.com/c360studio/semstreams/cmd/predicate-audit@v1.0.0-beta.158 "$root"
done
```

That bounded sweep reports exactly 107 invalid candidates from 193 extracted.
The package totals are:

- agentprogression 22/32, agentstore 23/30, autonomy 19/21.
- bossbattle 8/19 before issue #26's retreat deletion, dmsession 2/4,
  executor 9/35.
- guildformation 2/5, partycoord 14/27, promptmanager 2/5.
- questboard 2/2 and seeding 4/13.
- All other processor package roots report zero invalid candidates.

The service command reports one finding from one extracted candidate:

```sh
go run github.com/c360studio/semstreams/cmd/predicate-audit@v1.0.0-beta.158 ./service
```

Bounded runs over `cmd`, `componentregistry`, `semsource`, `domains`,
`graphclient.go`, `config.go`, `llm_utils.go`, `trajectory.go`, and `vocab.go`
report zero invalid candidates. UI consumers are inventoried separately below
because the predicate auditor does not scan TypeScript/Svelte as graph
producers.

## Declared-root ledger

Each brace expression denotes every listed concrete predicate. Duplicate
declaration/registration findings use the same canonical identity.

### Quest and lifecycle

- `quest.command` becomes `quest.lifecycle.command`.
- `quest.claim.requested_by` becomes `quest.assignment.requested-by`.
- `quest.party.min_size` becomes `quest.party.min-size`.
- `quest.review.needs_review` becomes `quest.review.needs-review`.
- `quest.lifecycle.{posted_at,claimed_at,started_at,completed_at}` becomes
  `quest.lifecycle.{posted-at,claimed-at,started-at,completed-at}`.
- `quest.lifecycle.pending_triage` becomes `quest.lifecycle.pending-triage`.
- `quest.classification.{red_team_target,red_team_status,red_team_quest_id}`
  becomes
  `quest.classification.{red-team-target,red-team-status,red-team-quest-id}`.
- `quest.execution.loop_id` becomes `quest.execution.loop-id`.
- `quest.verdict.xp_awarded` becomes `quest.verdict.xp-awarded`.
- `quest.lifecycle.last_transition_{source,at,from,note}` becomes
  `quest.transition.{source,at,from,note}`. This is a namespace change as well
  as a lexical rename; no `quest.lifecycle.last-transition-*` alias survives.
- `quest.duration` becomes `quest.metrics.duration`.
- `quest.metrics.{turns_used,tokens_prompt,tokens_completion}` becomes
  `quest.metrics.{turns-used,tokens-prompt,tokens-completion}`.
- `quest.context.token_count` becomes `quest.context.token-count`.
- `quest.dag.{execution_id,node_quest_ids,node_states,node_assignees}` becomes
  `quest.dag.{execution-id,node-quest-ids,node-states,node-assignees}`.
- `quest.dag.{completed_nodes,failed_nodes,node_retries,node_id}` becomes
  `quest.dag.{completed-nodes,failed-nodes,node-retries,node-id}`.

Value/cardinality is unchanged for these lexical renames. Relationship values
for assignment, red-team targets, parent/dependency, and produced entities are
typed `@id`. Lifecycle is the sole owner of `quest.verdict.xp-awarded`.

### Guild and peer review

- `guild.config.{max_members,min_level}` becomes
  `guild.config.{max-members,min-level}`.
- `guild.stats.{quests_handled,quests_failed,success_rate}` becomes
  `guild.stats.{quests-handled,quests-failed,success-rate}`.
- `guild.lifecycle.created_at` becomes `guild.lifecycle.created-at`.
- `guild.routing.{quest_type,preferred_client}` becomes
  `guild.routing.{quest-type,preferred-client}`.
- `review.config.solo_task` becomes `review.config.solo-task`.
- `review.lifecycle.{created_at,completed_at}` becomes
  `review.lifecycle.{created-at,completed-at}`.
- `review.leader.submitted_at` becomes `review.leader.submitted-at`.
- `review.member.submitted_at` becomes `review.member.submitted-at`.
- `review.result.{leader_avg,member_avg}` becomes
  `review.result.{leader-avg,member-avg}`.

Guild application dynamic predicates are replaced by first-class entities
defined below. Review assignment values are `@id`; review scores and timestamps
remain scalars.

### Gated-DAG and questdag

The effective beta.158 framework defaults are approved:

- `gateddag.claim` becomes `gateddag.unit.claim`.
- `gateddag.completed` becomes `gateddag.unit.completed`.
- `gateddag.failed` becomes `gateddag.unit.failed`.
- `gateddag.dirtied` becomes `gateddag.unit.dirtied`.
- `gateddag.depends_on` becomes `gateddag.unit.depends-on`.

Product provenance becomes:

- `dagunit.node_id` becomes `questdag.unit.node-id`.
- `dagunit.parent_quest` becomes `questdag.unit.parent-quest` with `@id`.
- `dagunit.execution_id` becomes `questdag.unit.execution-id`.
- `dagunit.sub_quest` becomes `questdag.unit.sub-quest` with `@id`.
- `questdag.unit.plan-digest` is a new create-only scalar on the activation
  latch. It binds one execution ID to one canonical DAG manifest and makes a
  changed manifest under the same idempotency key a collision, not a replay.

`gateddag.unit.depends-on` is an ordinary questdag-owned predicate on the
dagunit subject. The previously proposed two-phase
create-then-add-dependencies protocol is rejected: the beta.158 executor watches
the unit prefix continuously and derives a real dependency-free phase-one unit
as Ready, so it can dispatch before the topology is complete.

Issue #3 uses this temporary product admission bridge while remaining pinned to
beta.135:

1. Canonicalize the validated DAG and calculate an order-independent plan
   digest.
2. Derive validator-safe stable unit IDs from board identity, execution ID, and
   node ID. Derive one stable activation-latch ID from board identity,
   execution ID, and the literal role `activation`. Execution ID is the replay
   idempotency key.
3. Atomically create the real activation unit with its provenance,
   `questdag.unit.plan-digest`, and a typed
   `gateddag.unit.depends-on` self-edge. Graph-ingest does not create a
   referential stub for a self-target, and the self-edge keeps the real latch
   non-dispatchable.
4. Create work units in prerequisite-first order. Each unit is born through one
   create-with-triples mutation containing all provenance, every declared
   dependency, and an additional typed dependency on the activation latch.
   Every referenced prerequisite therefore already exists; no birth stub or
   dependency-update phase is allowed.
5. Authoritatively reconcile the complete manifest before activation. Every
   expected unit must be real and must have the exact execution, node,
   provenance, datatype, and complete dependency set. A changed digest or
   immutable triple under the same stable ID is a collision. An activated plan
   with a missing or mismatched unit is graph corruption and is never silently
   repaired.
6. Release the plan through the private `questdag` completion seam. That seam
   owns `gateddag.unit.completed`, uses the classified idempotent
   update-with-triples lane with token
   `questdag-completion#<incarnation>`, and atomically replaces the latch's
   completion marker. It does not use append-style `triple.add`.
7. While pinned to beta.135, confirm every completion attempt with an
   authoritative entity read. Beta.135 cannot validate its decoded success
   echo, so that echo is never authoritative. Issue #4 may trust a
   non-degraded echo only after beta.158 validation. A lost mutation reply
   converges when read-back proves the marker; an absent marker remains a
   classified transient failure.

The executor owns only `gateddag.unit.claim`. SemStreams explicitly assigns
completed, failed, and reset markers to the consumer. Semdragon consequently
owns its narrow completion/failure/reset seams; it does not introduce a
general-purpose gated-DAG library. The activation bridge is retired when
[SemStreams #670](https://github.com/C360Studio/semstreams/issues/670) supplies
framework-owned staged fan-out admission with one plan activation commit.

Stable replay may observe work units that are already claimed or terminal.
Reconciliation compares their immutable seed contract and does not erase
framework or completion state. The final unit corpus contains no
`graph.StubMessageType` entities.

Issue #3 temporarily sources the five framework strings from one compatibility
definition because beta.135 does not export beta.158 defaults. Issue #4 deletes
that definition and consumes beta.158 defaults. No configuration override or
runtime alias survives.

## Processor and service ledger

### agentprogression: historical 22/32; issue #44 target 0/27 visible

The baseline and incremental measurements are distinct:

- The historical beta.158 baseline is 22 invalid candidates from 32
  extracted.
- Issue #30 quarantined the component and deleted the unproduced
  `AgentXPPayload`. Its five invalid occurrences were
  `agent.progression.xp.delta`, `agent.progression.xp.current`,
  `agent.progression.xp.quest`, `agent.progression.xp.awarded`, and
  `agent.progression.xp.penalty`. The measured post-quarantine baseline is 17
  invalid candidates from 26 extracted.
- Issue #44 canonicalizes the retained dormant Agent projection and replaces
  dynamic skill predicate names with one bounded repeated value. Its pinned
  beta.158 target is zero invalid candidates from 27 scanner-visible
  candidates.

Issue #44 applies exactly these 17 retained scalar canonicalizations:

- `agent.identity.display_name` becomes `agent.identity.display-name`.
- `agent.progression.xp.current` becomes
  `agent.progression.xp-current`.
- `agent.progression.xp.to_level` becomes
  `agent.progression.xp-to-level`.
- `agent.progression.death_count` becomes `agent.progression.death-count`.
- `agent.stats.{quests_completed,quests_failed,bosses_defeated,total_xp_earned}`
  becomes
  `agent.stats.{quests-completed,quests-failed,bosses-defeated,total-xp-earned}`.
- `agent.lifecycle.{created_at,updated_at}` becomes
  `agent.lifecycle.{created-at,updated-at}`.
- `agent.status.cooldown_until` becomes `agent.status.cooldown-until`.
- `agent.inventory.total_spent` becomes `agent.inventory.total-spent`.
- `agent.reputation.{peer_avg,peer_q1_avg,peer_q2_avg,peer_q3_avg,peer_count}`
  becomes
  `agent.reputation.{peer-avg,peer-q1-avg,peer-q2-avg,peer-q3-avg,peer-count}`.

The dynamic `agent.skill.{tag}.{level,total_xp}` family became repeated
`agent.skill.proficiency` map values with `tag`, `level`, and `total-xp`
fields. Runtime skill tags remain data and never become predicate segments.
This is a dormant downstream stopgap, not a framework-ready SemStreams
primitive; SemStreams #683 blocks reactivation. Reconstruction, UI consumers,
and generated schema use only these canonical names; issue #44 adds no
compatibility alias or dual reader.

Guild membership, current quest, and current party remain typed `@id`
relationships through `agent.membership.guild`, `agent.assignment.quest`, and
`agent.membership.party`. The 17 canonical scalars and
`agent.skill.proficiency` map values are not relationships. The removed
payload's historical `agent.progression.xp.quest` does not return.

Zero scanner-visible findings is not beta.158 readiness. The auditor does not
extract the dynamic tool, consumable, and active-effect predicate families or
prove their full-object writers ownership-safe. Those families still migrate
to first-class inventory-entry and active-effect child entities through issue
#4 and the owner-safe framework mutation path tracked by SemStreams #313.
Issue #44 performs no child-entity migration, owner bind, heartbeat, safe
writer migration, dependency bump, or component reactivation.

### agentstore: historical 23/30; issue #46 target 0/18 visible

The baseline and incremental measurements are distinct:

- The historical beta.158 baseline is 23 invalid candidates from 30
  extracted.
- Issue #32 deleted the repository-unconsumed raw event plane.
  `payloads.go` contributed nine invalid candidates from twelve extracted:
  `agent.purchase.{item_id,xp_spent}`,
  `agent.progression.xp.current`,
  `agent.consumable.{effect_type,expired_reason}`, and
  `agent.inventory.{change_type,item_id,quantity_change,total_spent}`. The
  measured post-quarantine baseline is 14 invalid from 18 extracted, all in
  retained `store.go`.
- Issue #46 canonicalizes those 14 scanner-visible retained mappings. Its
  pinned beta.158 target is zero invalid candidates from 18 extracted.

Issue #46 applies exactly these 14 canonicalizations:

- `agent.inventory.total_spent` becomes `agent.inventory.total-spent`.
- `agent.inventory.tools_count` becomes `agent.inventory.tools-count`.
- `agent.inventory.consumables_count` becomes
  `agent.inventory.consumables-count`.
- `store.item.purchase_type` becomes `store.item.purchase-type`.
- `store.item.xp_cost` becomes `store.item.xp-cost`.
- `store.item.min_tier` becomes `store.item.min-tier`.
- `store.item.in_stock` becomes `store.item.in-stock`.
- `store.item.rental_uses` becomes `store.item.rental-uses`.
- `store.item.min_level` becomes `store.item.min-level`.
- `store.item.guild_discount` becomes `store.item.guild-discount`.
- `store.item.tool_id` becomes `store.item.tool-id`.
- `store.item.effect_type` becomes `store.item.effect-type`.
- `store.item.effect_magnitude` becomes `store.item.effect-magnitude`.
- `store.item.effect_duration` becomes `store.item.effect-duration`.

Producer and reconstruction use only these canonical names. Issue #46 adds no
compatibility alias, dual reader, or dual writer. The REST JSON contract is
unchanged, and dormant graph store items remain outside the generated primary
entity schema, so no UI or generated-schema migration is required.

StoreItem identity is fail-closed. A configured catalog item uses
`BoardConfig.StoreItemEntityID`; a nil receiver, missing BoardConfig, or
missing item ID returns the empty sentinel and emits no triples. The removed
`store.item.<id>` fallback is not a compatibility identity. The pinned
entity-ID audit target is zero findings from 23 candidates, with the empty
return classified exactly as `intentional-sentinel`.

`agent.inventory.tool` remains a repeated scalar catalog alias with empty
datatype, not an `@id` relationship. Inventory/effect state still migrates to
the approved first-class child model below. Issue #46 adds no inventory-entry
or active-effect entity, owner bind, heartbeat, owner-token writer,
dependency/configuration change, component reactivation, local validation
framework, or substitute for SemStreams #313. The historical and
post-quarantine audit baselines remain unchanged.

The deleted raw payload predicates are neither compatibility aliases nor
append evidence. A later product need for purchase or consumption receipts
requires a new explicit projection and consumer contract; it does not
resurrect the removed DTO Graphables. The retained canonical
`agent.inventory.{total-spent,tools-count,consumables-count}` predicates remain
the future agentstore-owned aggregate target until the first-class cutover.

### autonomy: 19 findings

The auditor extracts 21 stored-predicate candidates from the seven autonomy
payload Graphables. `agent.autonomy.status` and `agent.autonomy.action` are
already canonical; the other 19 candidates require these renames:

- `agent.autonomy.{interval_ms,idle_duration_ms,has_suggestion,backoff_ms}` becomes
  `agent.autonomy.{interval-ms,idle-duration-ms,has-suggestion,backoff-ms}`.
- `agent.autonomy.{claim_quest,claim_score,claim_rank}` becomes
  `agent.autonomy.{claim-quest,claim-score,claim-rank}`.
- `agent.autonomy.{shop_item,shop_cost,shop_budget,shop_strategic}` becomes
  `agent.autonomy.{shop-item,shop-cost,shop-budget,shop-strategic}`.
- `agent.autonomy.{guild_join,guild_score,guild_choices}` becomes
  `agent.autonomy.{guild-join,guild-score,guild-choices}`.
- `agent.autonomy.{use_consumable,use_status,use_quest}` becomes
  `agent.autonomy.{use-consumable,use-status,use-quest}`.
- `agent.autonomy.{guild_create,guild_create_candidates}` becomes
  `agent.autonomy.{guild-create,guild-create-candidates}`.

Issue #14 declares all 21 canonical names in the product vocabulary, including
the two names that did not need a lexical change. `claim-quest`, `guild-join`,
and `use-quest` are the only `@id` values. Each is written on the payload's
agent subject, so none is a `ForeignEdgeClaim`.

`shop-item` and `use-consumable` remain scalar catalog aliases.
`guild-choices` is the number of choices evaluated, and
`guild-create-candidates` is the candidate count; both remain scalar bounded
observability values. A future product change may convert an item alias with
`BoardConfig.StoreItemEntityID`, but issue #14 does not change the current
value contract.

The production autonomy component publishes these payloads as raw JSON through
typed NATS subjects. The binary does not register autonomy payload factories
or route them through `graph.ingest.entity`, so the Graphable projection is not
currently persisted by graph-ingest. Issue #14 changes no event subjects and
adds no projection binding. Issue #4 must either wire the real framework
`BaseMessage`/payload-registry/graph-ingest route or delete the seven latent
Graphable surfaces and their graph-only tests.

### bossbattle: seven scanner-visible findings canonicalized

- `battle.lifecycle.{started_at,completed_at}` becomes
  `battle.lifecycle.{started-at,completed-at}`.
- `battle.execution.loop_id` becomes `battle.execution.loop-id`.
- `battle.verdict.{quality_score,xp_awarded,xp_penalty,level_change}` becomes
  `battle.verdict.{quality-score,xp-awarded,xp-penalty,level-change}`.

The original audit reported `8/19`. Its eighth finding was `battle.outcome`,
which existed only on the definition-only `BattleRetreatPayload` deleted by
issue #26. The current authoritative `BossBattle` baseline is therefore
`7/17`. Issue #48 reports `0/17` for this scalar-only packet; it does not
rewrite the historical measurement.

Issue #26 deletes a separate phantom event surface. Production has no publisher
for `battle.review.started`, `battle.review.verdict`,
`battle.review.victory`, `battle.review.defeat`, or
`battle.review.retreat`. Their five vocabulary constants and registrations,
raw bossbattle event/verdict ports, questboard output port, dmsession routing,
and battle game-event types are deleted rather than renamed or assigned an
owner.

The definition-only retreat surface is deleted with that facade:
`BattleRetreatPayload`, its graph methods and tests, the retreat status, and
the UI filter, style, fixture, OpenAPI value, and generated TypeScript value.
No retreat alias or compatibility reader survives. The authoritative
`BossBattle` model, evaluator, REST/dmworldstate/UI readers, real battle-state
KV port, and typed quest/agent relationships remain dormant for Phase 2.

Issue #48 cuts the authoritative producer, scalar Go reconstruction, UI graph
transformer, and generated graph schema to the seven canonical predicates in
one gate. It deletes the undocumented reconstruction alias
`battle.verdict.score`. It preserves `battle.assignment.quest` and
`battle.assignment.agent` as typed `@id` relationships. REST and OpenAPI
snake_case JSON fields remain unchanged, and bossbattle remains disabled in
every shipped profile.

The predicate scanner does not expose the blocking indexed-family defect.
`BossBattle.Triples` constructs criteria, judge, and result predicates at
runtime:

- `battle.criteria.<index>.<field>`;
- `battle.judge.<index>.<field>`; and
- `battle.result.<index>.<field>`.

Go reconstruction also still accepts legacy unindexed `battle.judge.id` in
`parseIndexedJudge`. Issue #48 does not present that indexed-family
compatibility path as part of the seven scalar aliases: it remains with the
scanner-hidden judge blocker and must retire under SemStreams #683.

These families cannot be made safe through wildcard owner claims or
occurrence-level audit exceptions. They remain unchanged and blocking after
the scalar packet. SemStreams beta.158 documents complex values as flattened
triples and does not provide a public repeated-structured-value contract, so
repeated downstream map objects are not an approved replacement.
[SemStreams #683](https://github.com/C360Studio/semstreams/issues/683) owns the
framework decision and tagged primitive. Semdragon does not substitute JSON
strings, a local codec, or first-class battle child entities.

Reactivation remains blocked until bossbattle binds and heartbeats one
authoritative projection contract, all cross-owner effects use their approved
owner APIs, fresh-state tests pass, and the mutation path consumes the
upstream helper tracked by
[SemStreams #313](https://github.com/C360Studio/semstreams/issues/313).
Semdragon does not copy a generic owner-token mutator locally.

### executor: nine findings across three boundaries

The pinned beta.158 audit extracts 35 candidates from `processor/executor` and
reports nine findings.

The three live execution lifecycle payloads account for 19 candidates and four
findings:

- `execution.config.{max_turns,max_tokens}` becomes
  `execution.config.{max-turns,max-tokens}`.
- `execution.metrics.tool_calls` becomes `execution.metrics.tool-calls`.

Both executor and questbridge publish started, completed, and failed payloads
as raw typed-subject JSON. Their Graphable methods emit 15 unique canonical
predicates, all declared in the product vocabulary:

- `execution.lifecycle.started`, `execution.agent.id`, `execution.loop.id`,
  `execution.config.max-turns`, `execution.config.max-tokens`, and
  `execution.tools.available`;
- `execution.lifecycle.completed`, `execution.status.final`,
  `execution.metrics.turns`, `execution.metrics.tool-calls`,
  `execution.tokens.prompt`, `execution.tokens.completion`, and
  `execution.duration.ms`; and
- `execution.lifecycle.failed` and `execution.failure.error`.

`execution.agent.id` is the only `@id`; it is a same-subject relationship on
the quest entity.

The tool-call and tool-result Graphables account for seven candidates and two
findings, `execution.tool.call_id` and `execution.tool.duration_ms`. They have
no production publisher or consumer. Issue #16 deletes both payloads, subjects,
vocabulary metadata, and relationship fixtures instead of renaming or binding
them. Issue #7 owns any future framework tool telemetry.

The real web-fetch writer accounts for nine candidates and three findings:

- `source.doc.{mime_type,fetched_by,quest_id}` becomes
  `source.doc.{mime-type,fetched-by,quest-id}`.

`source.doc.fetched-by` and `source.doc.quest-id` are same-subject `@id`
relationships on the web document. They join `execution.agent.id` as exactly
the three typed relationships remaining in the package. The other values,
including loop IDs, URLs, MIME type, status, errors, counts, tokens, and
durations, are scalar.

Issue #16 reuses SemStreams `vocabulary.DCTermsTitle`. Because beta.158 lacks
the standard `dc.terms.modified` constant and registration, one isolated
compatibility declaration may remain linked to
[SemStreams #673](https://github.com/C360Studio/semstreams/issues/673). It is
not product-owned generic vocabulary and is deleted when Semdragon adopts the
first tagged SemStreams release containing the upstream primitive.

No lifecycle Graphable is currently registered or routed through graph-ingest.
Issue #16 adds no binding. Issue #4 owns the route-or-delete decision and must
bind the actual publisher or publishers, not infer one owner from the payload
package. The web writer remains a direct full-object graph mutation; a later
web-document packet owns its replace-owned migration.

### dmsession: delete the definition-only graph surface

The baseline audit recorded two findings from four candidates:
`dm.session.quests_completed` and `dm.session.quests_failed`. Issue #20 does
not rename them. `SessionManager.StartSession` and `SessionManager.EndSession`
contain raw typed-subject publisher methods, and production methods in
`processor/dmsession/component.go` call them internally. No shipped
configuration activates the factory-registered component, no production code
outside `processor/dmsession` calls its component entrypoints, and no
payload-registry factory or route to `graph.ingest.entity` exists.

The raw event DTO and graph projection are separate contracts. Issue #20
retains the `SessionStartPayload` and `SessionEndPayload` DTOs, their
`dm.session.start` and `dm.session.end` typed subjects, validation, JSON fields,
and internal publisher methods. It deletes their graph-only `EntityID`,
`Triples`, and `Schema` methods, `Graphable` assertions, graph-only tests, and
all four latent graph predicates:

- `dm.session.mode`;
- `dm.session.name`;
- `dm.session.quests_completed`; and
- `dm.session.quests_failed`.

The historical findings remain recorded in `baseline-audit.md`; the target
corpus contains no dmsession event graph predicates. The numeric
`quests_completed` and `quests_failed` fields in `domain.SessionSummary`,
OpenAPI, and generated UI types remain unchanged because they are JSON
properties, not graph vocabulary or compatibility aliases.

If authoritative session graph state is approved later, it introduces
canonical `dm.session.quests-completed` and
`dm.session.quests-failed` predicates only through a real SemStreams lifecycle
or owned-state writer. Issue #4 owns that design together with removal of the
raw `Session` JSON write into the shared board entity-state bucket. Issue #20
does not invent a projection owner, binding, heartbeat, or replacement writer.

### guildformation: delete the five latent event projections

The baseline audit recorded two findings from five candidates:
`guild.event.created_by` and `guild.lifecycle.disbanded_at`. Issue #22 does not
rename them. Guildformation is enabled in all three shipped configurations,
and its create, join, leave, promote, and disband operations publish five raw
DTOs through typed NATS subjects. Those raw event contracts are active.

The same DTOs are not graph publishers. The binary has no guild event
payload-registry factory, `BaseMessage` wrapper, route through
`graph.ingest.entity`, projection bind, heartbeat, or graph reader. Issue #22
therefore deletes all five `Graphable` assertions, their fifteen `EntityID`,
`Triples`, and `Schema` methods, their graph-only tests, and their relationship
fixtures.

The deleted graph shapes include:

- `GuildCreatedPayload`, which repeats `domain.Guild.Triples` and adds
  `guild.event.created_by`;
- `GuildJoinedPayload`, which emits `guild.membership.member` and dynamic
  `guild.member.<agent-id>.rank`;
- `GuildLeftPayload`, which emits `guild.membership.left`;
- `GuildPromotedPayload`, which emits dynamic
  `guild.member.<agent-id>.rank`; and
- `GuildDisbandedPayload`, which emits `guild.status.state` and
  `guild.lifecycle.disbanded_at`.

`guild.event.created_by` duplicates the authoritative typed
`guild.founding.agent` relationship. `guild.lifecycle.disbanded_at` has no
field in `domain.Guild`, reconstruction, API, or UI state. Neither finding
receives a canonical replacement. The historical findings remain unchanged in
`baseline-audit.md`; the target event DTO corpus contains no guild graph
predicates.

The raw subjects `guild.management.created`, `guild.membership.joined`,
`guild.membership.left`, `guild.membership.promoted`, and
`guild.lifecycle.disbanded` remain unchanged. Their DTO fields, including
`founder_id`, `agent_id`, `timestamp`, and rank/reason/count values, remain JSON
wire data rather than graph vocabulary.

Authoritative `domain.Guild` graph relationships remain in scope:
`guild.founding.agent`, `guild.membership.agent`, and the current application
applicant/reviewer relationships stay typed `@id`. Issue #4 or a dedicated
guild-state packet replaces dynamic member/application facts with first-class
children and migrates the multiwriter full-object state path. SemStreams
already supplies the primitives needed to delete the event projections, so
issue #22 creates no upstream framework ask.

### seeding: delete the unactivated component

The historical baseline remains four findings from thirteen predicate
occurrences:

- `seeding.mode`;
- `seeding.stats.agents_created`;
- `seeding.stats.guilds_created`; and
- `guild.specialization`.

Issue #24 does not rename these predicates. All five `processor/seeding` files
were introduced together as an event-driven replacement, but no consumer was
ever added. The component is registered but absent from all three shipped
configurations. Its declared `seeding.request` input port has no subscription,
and no code outside the package calls `Seed`.

The four `StartedPayload`, `CompletedPayload`, `AgentSeededPayload`, and
`GuildSeededPayload` families are therefore definition-only runtime-dead
surfaces. No payload-registry factory, `BaseMessage`, graph-ingest route,
projection contract, owner bind, heartbeat, graph reader, API reader, or UI
reader exists. Issue #24 deletes the entire package, its four typed subjects,
DTOs, `Validate` methods, publisher calls, JSON fields, four `Graphable`
assertions, twelve graph-only methods, and all thirteen predicate occurrences.
There is no raw DTO compatibility contract and no canonical alias.

The package import, both registration callbacks, both component-name list
entries, seven otherwise-unused seeding vocabulary constants and
registrations, and all four root relationship-fixture rows are also deleted.
The `AgentSeededPayload` fixture's `agent.guild.member` edge does not replace
the authoritative Agent relationship `agent.membership.guild`.

Startup fixture seeding remains separate. `cmd/semdragons/seed_e2e.go` is
invoked for `SEED_AGENTS`, `SEED_E2E`, and `SEED_PROS` and creates
authoritative `agentprogression.Agent` state. Issue #4 or a dedicated
startup-birth packet moves its direct full-Agent `GraphClient.EmitEntity`
write through SemStreams graph-ingest `create_with_triples` and the
agentprogression owner boundary. Create-only Agent birth facts remain outside
mutable claims; initial mutable status, progression, statistics, and skills
cross the real owner API.

SemStreams already provides classified request/reply,
`create_with_triples`, lifecycle, rules, bounded dispatch, projection
contracts, and ownership. Deletion creates no upstream framework ask. The
parked provider-arena work now defines a future post-Phase-1 product
composition over those primitives rather than preserving the retired
processor.

### promptmanager: delete the definition-only projection

The baseline audit recorded two findings from five candidates:
`prompt.assembly.agent_id` and `prompt.assembly.fragment_count`. Issue #12 does
not rename them. Source and history review proves that
`PromptAssembledPayload` and `SubjectPromptAssembled` have no production
publisher, no payload-registry registration, and no lifecycle component. Their
only downstream use is the graph relationship fixture and vocabulary metadata.

Issue #12 therefore deletes the dead `Graphable`, typed subject, vocabulary
entry, and relationship fixture. It must not create a promptmanager component,
projection contract, owner bind, or heartbeat. The historical finding remains
recorded in `baseline-audit.md`; the target corpus contains no prompt-assembly
graph predicates.

`TestPromptAssemblyHasNoGraphProjectionSurface` in
`prompt_projection_inventory_contract_test.go` stale-checks the relevant
production source inventory and rejects the deleted surface and external
promptmanager registration hooks. Its rules are identifier-specific; they do
not prohibit SemStreams projection contracts, ownership services, heartbeats,
or lifecycle timers for real publishers.

If an actual publisher is added later, that publishing component owns the
declaration and lifecycle binding. Its contract uses entity pattern
`*.*.game.*.quest.*` and one append-evidence group containing
`prompt.assembly.completed`, `prompt.assembly.agent-id`,
`prompt.assembly.domain`, `prompt.assembly.tier`,
`prompt.assembly.provider`, and `prompt.assembly.fragment-count`. The typed
agent relationship is ordinary same-subject data because the triple subject is
the quest returned by `EntityID()`; it does not require a
`ForeignEdgeClaim`.

### partycoord: historical 14 findings, four scalars canonicalized

The historical beta.158 baseline remains 14 invalid candidates from 27
extracted. Issue #28 does not rewrite that baseline. It deletes ten findings:

- nine event-projection occurrences:
  `party.lifecycle.disbanded_at`,
  `party.coordination.decomposed_by`,
  `party.coordination.parent_quest`,
  `party.coordination.sub_quest_count`, `party.strategy`,
  `party.coordination.sub_quest`, `party.rollup.started_at`,
  `party.rollup.sub_results_count`, and `party.rollup.completed_at`; and
- the authoritative Party shadow occurrence of `party.strategy`.

The event findings disappear with all eight raw party DTO/typed-subject/
publisher/Graphable families. The same deletion removes thirteen event-only
vocabulary entries, three false event ports, two phantom DM event values, and
their graph relationship fixtures. No subscriber, payload-factory route, or
durable graph-ingest route consumed that plane, so issue #28 adds no
replacement event plane.

The package also deletes the test-only `DecomposeQuest`, `SubmitResult`,
`StartRollup`, and `CompleteRollup` methods and the `Strategy`,
`SharedContext`/`ContextItem`, `SubResults`, and `RollupResult` shadows. DAG
node output, review, retry, and rollup remain questdag/quest-lifecycle state.

The current pinned audit extracts eight candidates and reports exactly four
invalid scalars:

- `party.quest` becomes `party.assignment.quest` with the entity-reference
  datatype;
- `party.lead` becomes `party.membership.lead` with the entity-reference
  datatype;
- `party.lifecycle.formed_at` becomes `party.lifecycle.formed-at` as an
  RFC3339 scalar literal; and
- `party.lifecycle.disbanded_at` becomes `party.lifecycle.disbanded-at` as an
  RFC3339 scalar literal.

Issue #50 reports `0/8` for those scanner-visible predicates. The Party
producer, Go reconstruction, UI graph transformer, and generated schema move
together to the four canonical names. The four legacy scalar readers are
deleted. `party.status.state` is unchanged. REST and OpenAPI retain `lead`,
`quest_id`, and the remaining snake_case Party JSON fields.

The exact canonical `party.assignment.quest` edge overlaps lexically with the
temporary `party.assignment.<full-quest-id>` prefix. Go and UI parsers exclude
the exact predicate before parsing dynamic assignments. Regression
coverage containing that exact edge plus one six-part dynamic assignment
produces one real `SubQuestMap` entry and no synthetic `"quest"` key.

Scanner-hidden `party.member.<full-agent-id>.role` and
`party.assignment.<full-quest-id>` remain noncanonical blockers. Their
existing Go and UI compatibility parsers remain unchanged for current
questdagexec/questdag callers; they are not canonical framework contracts.
Issue #4 replaces them with first-class party-membership and party-assignment
entities after
[SemStreams #313](https://github.com/C360Studio/semstreams/issues/313)
provides the reusable owner-safe create/replace client. If #313 lands
replace-only, issue #4 opens a linked owned-create upstream issue rather than
copying a local mutator.

Issue #50 records a fresh-state reset and reseed as an operational cutover
requirement; this dormant slice does not execute a runtime reset or restart
test. It adds no compatibility alias, dual representation, in-place state
rewrite, wildcard owner claim, auditor exemption, child entity, projection
owner, bind, heartbeat, local mutator, dependency or configuration change, or
component reactivation.

### Auditor false positives proved non-graph

The two questboard occurrences and one service occurrence of `quest.failed`
are passed as `eventType` to `GraphClient.EmitEntityUpdate`. They populate
`EntityState.MessageType.Category`, not `Triple.Predicate`. They remain
classified audit findings until the scanner supports that data flow, but they
do not receive a stored-predicate rename.
[SemStreams #671](https://github.com/C360Studio/semstreams/issues/671) tracks
the required occurrence-exact, stale-checked classifications. The
classification must be attached to those three expressions only, never to a
file, value, package, or directory. This upstream tooling gap does not block
the completed questdag core wave, but task 3.1 remains open until the final
predicate audit has exact evidence.

## Replacement contracts for dynamic predicate families

First-class entity models below remain approved targets. Repeated structured
values are not framework-ready until SemStreams publishes a tagged contract.

### Repeated structured values blocked on SemStreams #683

Issue #44 wrote each `agent.skill.{tag}.{level,total_xp}` pair as a repeated
`agent.skill.proficiency` map containing `tag`, `level`, and `total-xp`. That
map-valued representation is a dormant downstream stopgap, not an approved
SemStreams primitive or a pattern for other projections. Agent reactivation
and framework-readiness remain blocked on
[SemStreams #683](https://github.com/C360Studio/semstreams/issues/683).

The scanner-hidden battle criteria, judge, and result families remain indexed
until #683 defines a typed repeated structured representation or another
framework-owned model. Issue #48 does not replace them with map objects, JSON
strings, a local codec, or first-class battle child entities.

### Approved first-class entities

Every pattern has exactly six positions:

- Inventory entry:
  `*.*.game.*.inventory-entry.*`.
  Predicates:
  `inventory.assignment.agent`, `inventory.assignment.item`,
  `inventory.state.purchase-type`, `inventory.state.xp-spent`,
  `inventory.state.quantity`, `inventory.state.uses-remaining`, and
  `inventory.lifecycle.purchased-at`. `inventory.state.quantity` is the
  consumable quantity/count; count is data, never a dynamic predicate suffix.
- Active effect:
  `*.*.game.*.active-effect.*`.
  Predicates:
  `effect.assignment.agent`, `effect.assignment.consumable`,
  `effect.assignment.quest`, `effect.state.type`,
  `effect.state.magnitude`, `effect.state.remaining`, and
  `effect.lifecycle.activated-at`.
- Guild membership:
  `*.*.game.*.guild-membership.*`.
  Predicates:
  `membership.assignment.guild`, `membership.assignment.agent`,
  `membership.state.rank`, `membership.metrics.contribution`, and
  `membership.lifecycle.joined-at`.
- Guild application:
  `*.*.game.*.guild-application.*`.
  Predicates:
  `application.assignment.guild`, `application.assignment.applicant`,
  `application.assignment.reviewer`, `application.state.status`,
  `application.spec.level`, `application.spec.tier`,
  `application.spec.reason`, `application.spec.message`,
  `application.spec.skills`, `application.lifecycle.created-at`, and
  `application.lifecycle.reviewed-at`. `application.spec.skills` is repeated.
- Party membership:
  `*.*.game.*.party-membership.*`.
  Predicates:
  `membership.assignment.party`, `membership.assignment.agent`,
  `membership.state.role`, and `membership.lifecycle.joined-at`.
- Party assignment:
  `*.*.game.*.party-assignment.*`.
  Predicates:
  `assignment.relationship.party`, `assignment.relationship.quest`,
  `assignment.relationship.agent`, `assignment.state.rationale`, and
  `assignment.lifecycle.assigned-at`.
- Party submission:
  `*.*.game.*.party-submission.*`.
  Predicates:
  `submission.relationship.party`, `submission.relationship.quest`,
  `submission.relationship.member`, `submission.result.output`,
  `submission.result.quality`, and `submission.lifecycle.submitted-at`.

Every `assignment` or `relationship` object above is typed `@id`, except that
store-item/consumable values are typed only after the catalog-to-entity
conversion below.

## Catalog aliases and entity references

Local catalog keys such as `retry_token`, `cooldown_skip`, `xp_boost`,
`quality_shield`, and `insight_scroll` are scalar aliases. A bare alias is not
an entity ID and must not carry datatype `@id`.

For `inventory.assignment.item`, `effect.assignment.consumable`, or an
autonomy shop-item relationship, the producer must call
`BoardConfig.StoreItemEntityID(alias)` and emit the returned canonical
six-position StoreItem ID. Only that converted value carries
`internal/semcompat.EntityReferenceDatatype` in issue #3 (and
`message.EntityReferenceDatatype` after issue #4). If the producer lacks
`BoardConfig` or cannot prove the conversion, the value and predicate remain
scalar.

Required table-driven evidence covers every configured catalog alias. It
asserts that conversion yields
`org.platform.game.board.storeitem.<alias>`, that the beta.158 validator accepts
the result, that the emitted object equals that ID with datatype `@id`, and
that alias extraction/lookup round-trips to the same catalog entry. A negative
case asserts that emitting the bare alias with datatype `@id` is rejected.

## UI consumer mapping

`ui/src/lib/services/entity-transformer.ts` is the direct graph-to-UI consumer.
`entity-transformer.test.ts` has both behavioral tests and per-entity
`HANDLED`/`IGNORED` schema-contract inventories. Issue #3 must update all three:
producer, transformer lookup, and test expectation.

The exact behavioral cases are `transforms all agent fields correctly`,
`transforms all quest fields correctly`, `transforms all guild fields
correctly`, `transforms battle fields correctly`, `transforms party fields
correctly`, `extracts skill proficiencies via regex`, `extracts guild members
from membership triples`, and `extracts party members and sub-quest assignments
from triples`. The generated-contract cases are `every <entity> predicate is
accounted for`, `no stale <entity> entries in HANDLED`, and
`no stale <entity> entries in IGNORED`.

- Agent fields:
  `display_name`, `xp`, `xp_to_level`, `death_count`, four stats fields,
  `created_at`, `updated_at`, and `skill_proficiencies`.
- Quest fields:
  `min_party_size`, lifecycle timestamps, `loop_id`,
  `context_token_count`, verdict `xp_awarded`, red-team IDs,
  `turns_used`, token metrics, and `duration`.
- Guild fields:
  `max_members`, `min_level`, stats, `created_at`, `quest_types`,
  member rank/contribution, and preferred client.
- Battle fields:
  lifecycle timestamps, loop ID, verdict numeric fields, criteria, judges,
  and results.
- Party fields:
  `quest_id`, `lead`, `formed_at`, `disbanded_at`, member roles, and
  temporary `sub_quest_map`.

There is no current `quest.dag.*` transformer mapping. DAG status is therefore
not protected by the schema-contract test; issue #3 must add explicit DAG
projection tests and retain the existing party-DAG e2e assertion.

## Relationship coverage expansion

The current domain contract test covers Quest, Guild, and PeerReview. It must
also enumerate:

- Agent:
  guild, current quest, current party, inventory-entry/item, and effect/quest.
- Party:
  parent quest, lead, members, and the temporary assignment relationship.
  The future contract separately covers party membership plus
  questdag-owned assignment and node submission/output relationships.
- BossBattle:
  assignment quest and agent. Judge IDs inside bounded objects are not graph
  edges.
- DAG:
  `gateddag.unit.depends-on`, `questdag.unit.parent-quest`, and
  `questdag.unit.sub-quest`.
- Executor web fetch:
  `source.doc.fetched-by` and `source.doc.quest-id`.
- Event payload projections:
  agent XP quest, autonomy quest/item/guild relations, store item relations,
  and review assignment.
- Authoritative Guild:
  `guild.founding.agent`, `guild.membership.agent`, and the current dynamic
  application applicant/reviewer relationships. Issue #4 replaces membership
  and application facts with first-class child entities.
- Additional payload projections:
  `execution.agent.id` and store purchase/consumable item references.

The deleted prompt-assembly, guild-event, party-event, and four seeding DTO
fixtures are not relationship coverage. If a real publisher is introduced
later, its typed relationship must be added back to this inventory with
evidence from that publisher.

Issue #3 uses one centralized compatibility marker,
`internal/semcompat.EntityReferenceDatatype = "@id"`, across all packages.
Issue #4 replaces every use with `message.EntityReferenceDatatype` and deletes
the compatibility definition immediately after adopting beta.158.
