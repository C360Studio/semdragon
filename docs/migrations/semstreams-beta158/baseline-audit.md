# SemStreams beta.158 baseline audit

Status: reproducible issue #3 baseline captured on 2026-07-25. This is a
failing baseline, not a zero-audit result.

## Pinned predicate audit

The task contract pins beta.158:

```sh
task contract:predicates
```

It expands to:

```sh
go run github.com/c360studio/semstreams/cmd/predicate-audit@v1.0.0-beta.158 \
  ./domain ./questdag ./questlifecycle ./config
```

Observed result:

```text
predicate audit failed: 63 invalid or unclassified candidates (336 extracted)
```

The exact root split is domain 47, questdag 10, questlifecycle one, and config
five. Finding classes are:

- invalid lower-kebab segment characters;
- predicates with fewer or more than three segments;
- copied pre-beta.158 gated-DAG defaults; and
- lifecycle/rule predicate declarations using the same legacy identities.

The complete old-to-new family inventory is in `predicate-ledger.md`.

## Bounded processor and service audit

SemStreams #667 prevents one repository-wide scanner invocation. The corpus is
split into bounded roots; no package is exempt:

```sh
for root in processor/*; do
  test -d "$root" || continue
  go run github.com/c360studio/semstreams/cmd/predicate-audit@v1.0.0-beta.158 "$root"
done

go run github.com/c360studio/semstreams/cmd/predicate-audit@v1.0.0-beta.158 ./service
```

Observed processor result: 107 invalid candidates from 193 extracted.

```text
agentprogression 22/32
agentstore       23/30
autonomy         19/21
bossbattle        8/19
dmsession         2/4
executor          9/35
guildformation    2/5
partycoord        14/27
promptmanager      2/5
questboard         2/2
seeding            4/13
```

Every other processor package root reports zero invalid candidates. The service
root reports one invalid candidate from one extracted.

The questboard findings and service finding are the three
`predicate = "quest.failed"` assignments passed as `eventType` to
`GraphClient.EmitEntityUpdate`. `marshalEntityState` stores that argument in
`EntityState.MessageType.Category`; it never becomes a `Triple.Predicate`.
Those three expressions require narrow auditor classification. No file,
package, or value-wide exemption is valid.

Additional bounded runs over `cmd`, `componentregistry`, `semsource`,
`domains`, `graphclient.go`, `config.go`, `llm_utils.go`, `trajectory.go`, and
`vocab.go` report zero invalid candidates.

## Pinned entity-ID audit

The task contract command is:

```sh
task contract:entity-ids
```

It expands to:

```sh
go run github.com/c360studio/semstreams/cmd/entity-id-audit@v1.0.0-beta.158 \
  --include-untracked .
```

Observed result:

```text
entity ID audit failed: 5 invalid or unclassified candidates (14 extracted)
```

The exact five findings and approved replacements are:

```text
config/semdragons-e2e-tier1.json:23  local.semsource. -> local.semsource
config/semdragons-semsource.json:25  local.semsource. -> local.semsource
config/semdragons-semsource.json:33  osh.             -> osh
config/semdragons.json:23            local.semsource. -> local.semsource
ui/src/lib/services/api.test.ts:483  test.            -> test
```

All five fail with `entity_id_prefix_invalid:empty_segment`. Canonical query
prefixes contain one through six literal positions and no trailing dot.

`BoardConfig.EntityID` constructs:

```text
org.platform.game.board.type.instance
```

`ParseEntityID` checks exactly six positions. `IsValidEntityID` checks six
non-empty positions but does not yet enforce beta.158 grammar and bounds.
Issue #3 must use the pinned validator contract rather than treating the local
helper as sufficient.

## Entity patterns

Current canonical patterns are:

```text
*.*.game.*.quest.*
*.*.game.*.agent.*
*.*.game.*.party.*
*.*.game.*.guild.*
*.*.game.*.battle.*
*.*.game.*.storeitem.*
*.*.game.*.peerreview.*
*.*.game.*.dagunit.*
*.*.web.agent.doc.*
```

Issue #3 adds the first-class patterns recorded in the predicate ledger:
inventory entry, active effect, guild/party membership, guild application,
party assignment, and party submission. Each is six positions.

The legacy `StoreItem.EntityID` three-part fallback is a production finding,
not a compatibility feature. Successful Graphable tests using short
`guild-xyz`, bare session IDs, or other non-six-part IDs remain findings until
the payload is proven non-persisted or changed to a canonical ID.

Local store catalog keys (`retry_token`, `cooldown_skip`, `xp_boost`,
`quality_shield`, and `insight_scroll`) are scalar aliases, not entity IDs.
Shop, inventory, and effect objects become relationships only after
`BoardConfig.StoreItemEntityID` converts the alias to a validated
six-position StoreItem ID. Bare aliases remain scalar and carry no `@id`
datatype.

## Narrow negative fixtures

Only rejection fixtures for the validator under test may be classified:

- `cmd/sandbox/server_test.go` case `../evil` is a path-traversal negative, not
  a graph ID. Classify that literal/test case only.
- `service/api/handlers_test.go` input `a.b` is an instance-name validation
  negative, not an entity ID. Classify that literal/test case only.
- Malformed JSON and invalid enum fixtures do not enter graph-audit input and
  need no graph exemption.

The `ui/src/lib/services/api.test.ts` value `test.` is not an intentional
negative; the entity-ID auditor found it in a successful structured query
fixture. It changes to `test`.

Broad exclusions such as `*_test.go`, `processor/**`, `ui/**`, or a fixture
directory are forbidden.

## Relationship baseline

The relationship contract currently inventories 16 domain predicates across
Quest, Guild, and PeerReview. The implementation uses the literal beta.158 wire
value while pinned to beta.135.

Issue #3 centralizes that temporary value as:

```text
internal/semcompat.EntityReferenceDatatype = "@id"
```

All graph producers use that one definition. Issue #4 replaces it with
`message.EntityReferenceDatatype` and deletes the compatibility definition
immediately after the beta.158 dependency change.

Coverage is incomplete until tests enumerate:

- Agent guild/quest/party, inventory item, and effect quest edges.
- Party quest/lead/member/decomposition/assignment/submission edges.
- BossBattle quest and agent assignments.
- DAG dependency, parent quest, and sub-quest edges.
- Web-fetch agent and quest edges.
- Autonomy, progression, store, party, guild-application, and review payload
  relationship projections.

Relationship-looking collection metadata is not automatically an edge.
`quest.context.entities`, registry tool IDs, bounded judge objects, and
candidate-ID observability lists remain scalar unless their contract is
explicitly promoted.

`gateddag.unit.depends-on` is an ordinary questdag-owned predicate on the unit
subject, not a foreign-edge or birth-stub claim. The seeder creates all real
units first and then writes typed dependency edges between existing targets.
The store alias conversion receives table-driven validator, datatype, and
round-trip coverage before any shop/inventory/effect value is promoted to an
entity reference.

## Writer and generated-artifact baseline

Two direct product mutation statements exist:

- `KVStore.Put` in `graphclient.go:123`;
- `KVStore.Update` in `graphclient.go:153`.

Their full-object callers cross lifecycle, questbridge, questdag, agent, guild,
party, battle, review, and store ownership boundaries. Exact claims, tokens,
patterns, and foreign-edge modes are in `writer-owner-matrix.md`.

The replacement coverage gate inventories every production-emitted
`(subject pattern, predicate, producer expression)` and requires exactly one
classification: exact owner claim, create-only fact, append evidence, foreign
edge, or expression-level proved non-graph. It also registers all claims and
rejects overlaps, invalid patterns, wildcard predicates, and create-only facts
leaking into mutable claims. The three known `quest.failed` message-category
expressions are the only initial non-graph entries.

`go run ./cmd/gen-entity-schema` succeeds, but the checked-in
`ui/src/lib/services/entity-schema.generated.json` timestamp predates this
audit. Its consumer contract is
`ui/src/lib/services/entity-transformer.test.ts`, which accounts for generated
Agent, Quest, Guild, Battle, and Party predicates. Both generated schema and
transformer tests must change atomically with producers.

## Gate status

```text
Predicate audit:        FAIL  63/336 declared + 107/193 processor + 1/1 service
Entity-ID audit:        FAIL  5/14
Relationship coverage: FAIL  16 domain predicates; required families missing
Writer ownership:      FAIL  full-object writers cross exact owner claims
Schema freshness:      FAIL  checked-in schema predates current generator output
UI mapping:            FAIL  transformer and tests consume legacy identities
```

Issue #3 is complete only when the same pinned, bounded commands report zero,
relationship and ownership tests pass, generated artifacts are fresh, and UI
behavioral/schema-contract tests use only canonical predicates.
