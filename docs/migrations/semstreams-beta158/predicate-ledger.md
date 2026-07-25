# SemStreams beta.158 predicate rename ledger

Status: architect-reviewed target contract for semdragon issue #3. This ledger is
the implementation source of truth. It is not an alias table: issue #3 writes
only the canonical identity and deletes every legacy reader in the same change.

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
- bossbattle 8/19, dmsession 2/4, executor 9/35.
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

### agentprogression: 22 findings

- `agent.identity.display_name` becomes `agent.identity.display-name`.
- `agent.progression.xp.{current,to_level,delta,quest,awarded,penalty}` becomes
  `agent.progression.{xp-current,xp-to-level,xp-delta,xp-quest,xp-awarded,xp-penalty}`.
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

`agent.progression.xp-quest` is a quest relationship and is typed `@id`.

### agentstore: 23 findings

- `agent.purchase.{item_id,xp_spent}` becomes
  `agent.purchase.{item-id,xp-spent}`.
- `agent.consumable.{effect_type,expired_reason}` becomes
  `agent.consumable.{effect-type,expired-reason}`.
- `agent.inventory.{change_type,item_id,quantity_change,total_spent}` becomes
  `agent.inventory.{change-type,item-id,quantity-change,total-spent}`.
- `agent.inventory.{tools_count,consumables_count}` becomes
  `agent.inventory.{tools-count,consumables-count}`.
- `store.item.{purchase_type,xp_cost,min_tier,in_stock,rental_uses,min_level}`
  becomes
  `store.item.{purchase-type,xp-cost,min-tier,in-stock,rental-uses,min-level}`.
- `store.item.{guild_discount,tool_id,effect_type,effect_magnitude,effect_duration}`
  becomes
  `store.item.{guild-discount,tool-id,effect-type,effect-magnitude,effect-duration}`.
- The repeated `agent.progression.xp.current` occurrence becomes
  `agent.progression.xp-current`.

Inventory/effect state does not remain embedded on the agent; the approved
first-class model below supersedes dynamic inventory predicates.
`agentstore` must not project `agent.progression.xp-current`: the payload is
graph-bound today, but that triple crosses the agentprogression claim. The
store workflow invokes the agentprogression owner API for the XP change and
removes this predicate from its own `Triples`.

The remaining graph-bound store payload predicates are classified rather than
exempted. `agent.purchase.{item-id,xp-spent}`,
`agent.consumable.{effect-type,expired-reason}`, and
`agent.inventory.{change-type,item-id,quantity-change}` are append evidence.
`agent.inventory.{total-spent,tools-count,consumables-count}` are
agentstore-owned current aggregates. Their relationship-looking item values
remain scalar catalog IDs unless converted as specified under “Catalog aliases
and entity references.”

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

### bossbattle: eight findings

- `battle.lifecycle.{started_at,completed_at}` becomes
  `battle.lifecycle.{started-at,completed-at}`.
- `battle.execution.loop_id` becomes `battle.execution.loop-id`.
- `battle.verdict.{quality_score,xp_awarded,xp_penalty,level_change}` becomes
  `battle.verdict.{quality-score,xp-awarded,xp-penalty,level-change}`.
- `battle.outcome` becomes `battle.review.outcome`.

### dmsession, executor, guildformation, and seeding

- `dm.session.{quests_completed,quests_failed}` becomes
  `dm.session.{quests-completed,quests-failed}`.
- `execution.config.{max_turns,max_tokens}` becomes
  `execution.config.{max-turns,max-tokens}`.
- `execution.metrics.tool_calls` becomes `execution.metrics.tool-calls`.
- `execution.tool.{call_id,duration_ms}` becomes
  `execution.tool.{call-id,duration-ms}`.
- `source.doc.{mime_type,fetched_by,quest_id}` becomes
  `source.doc.{mime-type,fetched-by,quest-id}`.
- `guild.event.created_by` becomes `guild.event.created-by`.
- `guild.lifecycle.disbanded_at` becomes `guild.lifecycle.disbanded-at`.
- `seeding.mode` becomes `seeding.config.mode`.
- `seeding.stats.{agents_created,guilds_created}` becomes
  `seeding.stats.{agents-created,guilds-created}`.
- `guild.specialization` becomes `guild.identity.specialization`.

`source.doc.fetched-by` and `source.doc.quest-id` are `@id`.

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

### partycoord: 14 findings

- `party.quest` becomes `party.assignment.quest` with `@id`.
- `party.lead` becomes `party.membership.lead` with `@id`.
- `party.strategy` becomes `party.coordination.strategy`.
- `party.lifecycle.{formed_at,disbanded_at}` becomes
  `party.lifecycle.{formed-at,disbanded-at}`.
- `party.coordination.{decomposed_by,parent_quest,sub_quest_count,sub_quest}`
  becomes
  `party.coordination.{decomposed-by,parent-quest,sub-quest-count,sub-quest}`.
- `party.rollup.{started_at,sub_results_count,completed_at}` becomes
  `party.rollup.{started-at,sub-results-count,completed-at}`.

`decomposed-by`, `parent-quest`, and repeated `sub-quest` are `@id`.

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

## Approved replacement for dynamic predicate families

These models are architect-approved targets. Implementation evidence is pending
until issue #3 adds producers, readers, tests, and schema generation.

### Bounded values

- `agent.skill.{tag}.{level,total_xp}` becomes repeated
  `agent.skill.proficiency`. Each bounded object contains `tag`, `level`, and
  `total-xp`; no runtime tag appears in a predicate.
- Indexed battle criteria become repeated `battle.review.criteria` objects with
  `name`, `description`, `weight`, and `threshold`.
- Indexed judges become repeated `battle.review.judges` objects with `id` and
  `type`.
- Indexed results become repeated `battle.review.results` objects with
  `criterion-name`, `judge-id`, `score`, `passed`, and `reasoning`.

### First-class entities

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
  `quest_id`, `lead`, `strategy`, `formed_at`, member roles, and
  `sub_quest_map`.

There is no current `quest.dag.*` transformer mapping. DAG status is therefore
not protected by the schema-contract test; issue #3 must add explicit DAG
projection tests and retain the existing party-DAG e2e assertion.

## Relationship coverage expansion

The current domain contract test covers Quest, Guild, and PeerReview. It must
also enumerate:

- Agent:
  guild, current quest, current party, inventory-entry/item, and effect/quest.
- Party:
  parent quest, lead, members, decomposed-by, sub-quests, and first-class
  assignment/submission relationships.
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
  party decomposition/assignment/submission, guild application, and review
  assignment.
- Additional payload projections:
  `execution.agent.id`, `agent.guild.member`, `guild.event.created-by`,
  `guild.membership.member`, and store purchase/consumable item references.

The deleted prompt-assembly fixture is not relationship coverage. If a real
publisher is introduced later, its typed `prompt.assembly.agent-id` edge must
be added back to this inventory with evidence from that publisher.

Issue #3 uses one centralized compatibility marker,
`internal/semcompat.EntityReferenceDatatype = "@id"`, across all packages.
Issue #4 replaces every use with `message.EntityReferenceDatatype` and deletes
the compatibility definition immediately after adopting beta.158.
