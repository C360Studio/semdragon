# Fresh-volume replay runbook

## Purpose and status

This executable candidate procedure for [SemDragon issue #59] under [parent issue #4] was run against the
reviewed candidate below.

Execution status: failed.

Blocked by [SemStreams #712] and [SemStreams #713].

It tests whether the SemStreams cutover can rebuild the disposable SemDragon development graph and retain exact
semantic state after a seed-disabled restart. The failed result below is evidence, not acceptance.

Run the commands from the repository root in one Bash shell. Stop on the first unexpected value, nonzero exit,
GraphQL error, count mismatch, image mismatch, or parity difference. Do not substitute another commit, module,
Compose project, NATS context, server, volume, configuration, or backend image.

Reviewed commit: `a4912d725ccb722e38b87feecb31b7c56e328ffc`

Reviewed module: `github.com/c360studio/semstreams v1.0.0-beta.158.0.20260728003411-89617f32dc36`

Reviewed Compose project: `semdragon-cutover`

Reviewed Compose graph volume: `nats-data`

Reviewed engine volume: `semdragon-cutover_nats-data`

Development/test NATS URL: `nats://localhost:4222`

Development/test NATS context: `semdragon-dev`

Protected services: `local-semsource`, `nats-local-semsource`, `nats-semsource`, and `semsource`.

Protected volumes: `nats-local-semsource-data`, `nats-semsource-data`, `semembed-cache`, and `workspaces`.

The only data deletion in this procedure is the literal removal of `semdragon-cutover_nats-data`. Do not stop,
remove, rebuild, or modify a protected service. Do not remove, prune, recreate, or modify a protected volume.

## Observed candidate result

Candidate `a4912d725ccb722e38b87feecb31b7c56e328ffc` used immutable backend image
`sha256:7d8cd71a2eaf1b30e277f3114a0bd9301a526118c29a6c0c297a31ad172f2296`.

Both sides stabilized across three consecutive normalized `ENTITY_STATES` checksum checks. This temporary
quiescence boundary addresses the readiness gap tracked by [SemStreams #712]; it cannot hide the [SemStreams #713]
restart-idempotence failure.

The stable pre-restart digest was
`2033bbde523036af0cfbdc53fa22ba3f2a42973fe793304fcfddc15312ff345a`. The stable post-restart digest was
`bdb7d45d7837732fc3e563d0b874aeef1bc7ece4c57a2a66ca136c73faefc918`.

Exactly six model-registry hierarchy entities changed and 15 duplicate semantic triples were appended. The
domain, type, and system containers each gained three triples and moved from revision 6 to 9. The
`ollama-coder`, `ollama-qwen3`, and `semembed` endpoints each gained two triples and moved from revisions 4 to 6,
3 to 5, and 2 to 4, respectively. The duplicated predicates were `hierarchy.domain.contains`,
`hierarchy.system.contains`, `hierarchy.type.contains`, and `hierarchy.type.sibling`.

The stable digests differ, so the replay and operational acceptance gate remain failed.

## Preconditions

- Obtain an approved maintenance window with no other writer targeting this development graph.
- Use Docker, Docker Compose, NATS CLI 0.3.1, `curl`, `jq`, `git`, `grep`, `diff`, and Bash.
- Keep the checkout and locally built backend image unchanged throughout the procedure.
- Preserve the complete terminal transcript and every generated evidence file.
- Do not run an operational command until an operator reviews this procedure and its literal deletion target.

## 1. Verify reviewed source and module

The assertions bind the run to the exact reviewed source and SemStreams module. The listing records the selected
module in the retained transcript.

```sh
set -euo pipefail
test "$(git rev-parse HEAD)" = "a4912d725ccb722e38b87feecb31b7c56e328ffc"
go list -m github.com/c360studio/semstreams
test "$(go list -m github.com/c360studio/semstreams)" = "github.com/c360studio/semstreams v1.0.0-beta.158.0.20260728003411-89617f32dc36"
```

The results must confirm reviewed commit
`a4912d725ccb722e38b87feecb31b7c56e328ffc` and reviewed module
`github.com/c360studio/semstreams v1.0.0-beta.158.0.20260728003411-89617f32dc36`.
Any mismatch ends the run.

## 2. Render and review topology

The shipped development configurations are:

- `config/semdragons-e2e-tier1.json`
- `config/semdragons-semsource.json`
- `config/semdragons.json`

The candidate set is the sorted union of nonempty subjects on `kv`, `kv-write`, and `kv-watch` ports in those
files.

Candidate buckets (7): `ALIAS_INDEX`, `COMMUNITY_INDEX`, `EMBEDDINGS_CACHE`, `ENTITY_STATES`, `INCOMING_INDEX`, `OUTGOING_INDEX`, `PREDICATE_INDEX`.

Render the effective Compose topology and declared volume names. Retain both outputs. Confirm that `nats` mounts
`nats-data` at `/data`, the backend targets `nats://nats:4222`, and the project identity is exact.

```sh
docker compose --project-name semdragon-cutover -f docker/compose.yml config
docker compose --project-name semdragon-cutover -f docker/compose.yml config --volumes
test "$(docker compose --project-name semdragon-cutover -f docker/compose.yml config --format json | jq -r '.name')" = "semdragon-cutover"
```

## 3. Pin and verify the NATS context

The version assertion must run before every other NATS invocation. Save and select the literal context only after
the assertion passes, then verify its server.

```sh
test "$(nats --version)" = "0.3.1"
nats context save semdragon-dev --server nats://localhost:4222
nats context select semdragon-dev
test "$(nats context info semdragon-dev --json | jq -r '.server')" = "nats://localhost:4222"
```

## 4. Prepare immutable backend evidence

Prepare every evidence directory before writing evidence files. Record the one approved rendered backend image
name, prove that it occurs exactly once in rendered Compose images, and build it exactly once.

```sh
mkdir -p cutover-evidence before after
printf '%s\n' 'semdragon-cutover-backend' > cutover-evidence/backend-image.name
test "$(cat cutover-evidence/backend-image.name)" = "semdragon-cutover-backend"
test "$(docker compose --project-name semdragon-cutover -f docker/compose.yml config --images | grep -Fxc "$(cat cutover-evidence/backend-image.name)")" = "1"
docker compose --project-name semdragon-cutover -f docker/compose.yml build backend
BACKEND_IMAGE_ID="$(docker image inspect "semdragon-cutover-backend" --format '{{.Id}}')"
case "$BACKEND_IMAGE_ID" in sha256:*) ;; *) exit 1 ;; esac
```

Keep this shell open. `BACKEND_IMAGE_ID` is the immutable identity used to bracket both starts and verify both
running backend containers. Do not build or pull another backend image during the run.

## 5. Stop writers and approve the one deletion

The operator reviews and copies the literal target from the rendered topology. Any mismatch must abort before
deletion.

Stop only the backend and its graph NATS service. Immediately prove that neither service is running. Remove only
their stopped containers, without a volume-removal option, and prove their container IDs are absent.

```sh
docker compose --project-name semdragon-cutover -f docker/compose.yml stop backend nats
RUNNING_WRITER_SERVICES="$(docker compose --project-name semdragon-cutover -f docker/compose.yml ps --status running --services backend nats)"
test -z "$RUNNING_WRITER_SERVICES"
docker compose --project-name semdragon-cutover -f docker/compose.yml rm -f backend nats
BACKEND_NATS_CONTAINER_IDS="$(docker compose --project-name semdragon-cutover -f docker/compose.yml ps --all -q backend nats)"
test -z "$BACKEND_NATS_CONTAINER_IDS"
```

Prove that no container remains attached to the graph volume. Then verify its exact name and both Compose labels.

```sh
GRAPH_VOLUME_ATTACHMENTS="$(docker ps --all --quiet --filter volume=semdragon-cutover_nats-data)"
test -z "$GRAPH_VOLUME_ATTACHMENTS"
test "$(docker volume inspect semdragon-cutover_nats-data --format '{{.Name}}')" = "semdragon-cutover_nats-data"
test "$(docker volume inspect semdragon-cutover_nats-data --format '{{ index .Labels "com.docker.compose.project" }}')" = "semdragon-cutover"
test "$(docker volume inspect semdragon-cutover_nats-data --format '{{ index .Labels "com.docker.compose.volume" }}')" = "nats-data"
```

Before deletion, the operator must independently confirm:

- the reviewed project and both volume labels match;
- the stopped-service, empty-container-ID, and zero-attachment assertions passed;
- no protected service or protected volume is in the command; and
- the command contains one literal target with no wildcard, substitution, or variable.

Any mismatch, ambiguity, attachment, unexpected writer, or review failure must abort before deletion. After
approval, execute this command exactly once:

```sh
docker volume rm -- semdragon-cutover_nats-data
```

Do not substitute a broad Compose teardown, a Docker prune, or a NATS bucket or stream deletion.

## 6. Seed exactly once

Seed canonical state exactly once. Assert the immutable local image immediately before and after the start. The
start enables only `SEED_AGENTS`, includes both backend and NATS, and forbids builds and pulls.

```sh
test "$(docker image inspect "semdragon-cutover-backend" --format '{{.Id}}')" = "$BACKEND_IMAGE_ID"
SEED_E2E=false SEED_AGENTS=true SEED_PROS=false docker compose --project-name semdragon-cutover -f docker/compose.yml up -d --no-build --pull never backend nats
test "$(docker image inspect "semdragon-cutover-backend" --format '{{.Id}}')" = "$BACKEND_IMAGE_ID"
test "$(docker inspect "$(docker compose --project-name semdragon-cutover -f docker/compose.yml ps -q backend)" --format '{{.Image}}')" = "$BACKEND_IMAGE_ID"
```

Poll the exact nonempty GraphQL readiness condition:

```sh
until curl --fail --silent --show-error --request POST --header 'Content-Type: application/json' --data '{"query":"query CutoverReadiness { graphSummary { data { total_entities } } }"}' http://localhost:8081/graph-gateway/graphql | jq -e '(.errors == null) and (.data.graphSummary.data.total_entities > 0)' >/dev/null; do sleep 2; done
```

## 7. Capture complete baseline evidence

The entity workflow inventories every `ENTITY_STATES` key, fetches every raw value, validates the complete stored
shape, proves cardinality and ID uniqueness, normalizes only the allowlisted operational fields, and retains all
triples. `set -euo pipefail` makes any failed NATS, `jq`, `curl`, or count step abort the shell.

```sh
set -euo pipefail
nats --context semdragon-dev kv ls ENTITY_STATES | LC_ALL=C sort > before/entity-ids.txt
test -s before/entity-ids.txt
: > before/entities.raw.jsonl
while IFS= read -r entity_id; do nats --context semdragon-dev kv get ENTITY_STATES "$entity_id" --raw | jq -ce --arg entity_id "$entity_id" 'if (type == "object") and (.id | type == "string") and (.id == $entity_id) and (.message_type | type == "object") and (.message_type.domain | type == "string") and (.message_type.category | type == "string") and (.message_type.version | type == "string") and (.version | type == "number") and (.updated_at | type == "string") and (.triples | type == "array") and all(.triples[]; (type == "object") and (.subject | type == "string") and (.predicate | type == "string") and has("object") and (.source | type == "string") and (.timestamp | type == "string") and (.confidence | type == "number")) then . else error("invalid ENTITY_STATES value") end' >> before/entities.raw.jsonl; done < before/entity-ids.txt
test "$(wc -l < before/entity-ids.txt | tr -d ' ')" = "$(wc -l < before/entities.raw.jsonl | tr -d ' ')"
jq -sSe --argjson expected "$(wc -l < before/entity-ids.txt | tr -d ' ')" 'if (length == $expected) and (length == ([.[].id] | unique | length)) and all(.[]; (type == "object") and (.id | type == "string") and (.message_type | type == "object") and (.message_type.domain | type == "string") and (.message_type.category | type == "string") and (.message_type.version | type == "string") and (.version | type == "number") and (.updated_at | type == "string") and (.triples | type == "array") and all(.triples[]; (type == "object") and (.subject | type == "string") and (.predicate | type == "string") and has("object") and (.source | type == "string") and (.timestamp | type == "string") and (.confidence | type == "number"))) then map(.updated_at = "<normalized>" | .triples |= map(.timestamp = "<normalized>")) | sort_by(.id) else error("invalid ENTITY_STATES JSONL") end' before/entities.raw.jsonl > before/entities.normalized.json
test "$(wc -l < before/entity-ids.txt | tr -d ' ')" = "$(jq -r 'length' before/entities.normalized.json)"
```

Repeat the complete normalized entity capture until its SHA-256 checksum is identical for three consecutive
captures. Retain the final stable capture as the baseline. This [SemStreams #712] workaround establishes
quiescence; it does not weaken parity or the [SemStreams #713] correctness gate.

The relationship workflow reads IDs from a file-backed loop, validates every GraphQL envelope, proves one response
per entity ID, then merges, deduplicates, and sorts the complete relationship set.

```sh
: > before/relationship-responses.jsonl
while IFS= read -r entity_id; do curl --fail --silent --show-error --request POST --header 'Content-Type: application/json' --data "$(jq -nc --arg entityId "$entity_id" --arg query 'query RelationshipsEvidence($entityId: String!) { relationships(entityId: $entityId) { from to predicate } }' '{query:$query,variables:{entityId:$entityId}}')" http://localhost:8081/graph-gateway/graphql | jq -ce 'if ((.errors == null) and (.data.relationships | type == "array")) then . else error("invalid relationships GraphQL envelope") end' >> before/relationship-responses.jsonl; done < before/entity-ids.txt
test "$(wc -l < before/entity-ids.txt | tr -d ' ')" = "$(wc -l < before/relationship-responses.jsonl | tr -d ' ')"
jq -sSe 'if all(.[]; ((.errors == null) and (.data.relationships | type == "array"))) then [.[].data.relationships[]] | unique_by([.from,.predicate,.to]) | sort_by(.from,.predicate,.to) else error("invalid relationships GraphQL envelopes") end' before/relationship-responses.jsonl > before/relationships.normalized.json
```

Derive quest status and DAG evidence from the validated complete entity corpus. Both projections retain datatype.

```sh
jq -Se 'if ((type == "array") and all(.[]; ((type == "object") and (.triples | type == "array")))) then [.[] as $entity | $entity.triples[] | select(.predicate == "quest.status.state") | {id:$entity.id,predicate:.predicate,object:.object,datatype:.datatype}] | sort_by(.id,.predicate,.object,.datatype) else error("invalid normalized entity corpus") end' before/entities.normalized.json > before/quest-status.normalized.json
jq -Se 'if ((type == "array") and all(.[]; ((type == "object") and (.triples | type == "array")))) then [.[] as $entity | $entity.triples[] | select((.predicate | startswith("gateddag.unit.")) or (.predicate | startswith("questdag.unit."))) | {id:$entity.id,predicate:.predicate,object:.object,datatype:.datatype}] | sort_by(.id,.predicate,.object,.datatype) else error("invalid normalized entity corpus") end' before/entities.normalized.json > before/dag-projection.normalized.json
```

No intervening writes are permitted.

## 8. Restart without seeds and capture replay evidence

With the maintenance window still exclusive, restart the same reviewed binary. Assert the immutable local image
immediately before and after the restart, and verify the running container image. Every seed flag is false; builds
and pulls remain forbidden.

```sh
test "$(docker image inspect "semdragon-cutover-backend" --format '{{.Id}}')" = "$BACKEND_IMAGE_ID"
SEED_E2E=false SEED_AGENTS=false SEED_PROS=false docker compose --project-name semdragon-cutover -f docker/compose.yml up -d --force-recreate --no-build --pull never backend nats
test "$(docker image inspect "semdragon-cutover-backend" --format '{{.Id}}')" = "$BACKEND_IMAGE_ID"
test "$(docker inspect "$(docker compose --project-name semdragon-cutover -f docker/compose.yml ps -q backend)" --format '{{.Image}}')" = "$BACKEND_IMAGE_ID"
```

Poll the same exact nonempty readiness condition:

```sh
until curl --fail --silent --show-error --request POST --header 'Content-Type: application/json' --data '{"query":"query CutoverReadiness { graphSummary { data { total_entities } } }"}' http://localhost:8081/graph-gateway/graphql | jq -e '(.errors == null) and (.data.graphSummary.data.total_entities > 0)' >/dev/null; do sleep 2; done
```

Repeat the complete-state capture without any seed or other write.

```sh
set -euo pipefail
nats --context semdragon-dev kv ls ENTITY_STATES | LC_ALL=C sort > after/entity-ids.txt
test -s after/entity-ids.txt
: > after/entities.raw.jsonl
while IFS= read -r entity_id; do nats --context semdragon-dev kv get ENTITY_STATES "$entity_id" --raw | jq -ce --arg entity_id "$entity_id" 'if (type == "object") and (.id | type == "string") and (.id == $entity_id) and (.message_type | type == "object") and (.message_type.domain | type == "string") and (.message_type.category | type == "string") and (.message_type.version | type == "string") and (.version | type == "number") and (.updated_at | type == "string") and (.triples | type == "array") and all(.triples[]; (type == "object") and (.subject | type == "string") and (.predicate | type == "string") and has("object") and (.source | type == "string") and (.timestamp | type == "string") and (.confidence | type == "number")) then . else error("invalid ENTITY_STATES value") end' >> after/entities.raw.jsonl; done < after/entity-ids.txt
test "$(wc -l < after/entity-ids.txt | tr -d ' ')" = "$(wc -l < after/entities.raw.jsonl | tr -d ' ')"
jq -sSe --argjson expected "$(wc -l < after/entity-ids.txt | tr -d ' ')" 'if (length == $expected) and (length == ([.[].id] | unique | length)) and all(.[]; (type == "object") and (.id | type == "string") and (.message_type | type == "object") and (.message_type.domain | type == "string") and (.message_type.category | type == "string") and (.message_type.version | type == "string") and (.version | type == "number") and (.updated_at | type == "string") and (.triples | type == "array") and all(.triples[]; (type == "object") and (.subject | type == "string") and (.predicate | type == "string") and has("object") and (.source | type == "string") and (.timestamp | type == "string") and (.confidence | type == "number"))) then map(.updated_at = "<normalized>" | .triples |= map(.timestamp = "<normalized>")) | sort_by(.id) else error("invalid ENTITY_STATES JSONL") end' after/entities.raw.jsonl > after/entities.normalized.json
test "$(wc -l < after/entity-ids.txt | tr -d ' ')" = "$(jq -r 'length' after/entities.normalized.json)"
```

Repeat the post-restart normalized entity capture until its SHA-256 checksum is identical for three consecutive
captures. A stable mismatch is still a [SemStreams #713] replay failure and must keep the gate failed.

Repeat the file-backed, fail-closed relationship capture and count proof.

```sh
: > after/relationship-responses.jsonl
while IFS= read -r entity_id; do curl --fail --silent --show-error --request POST --header 'Content-Type: application/json' --data "$(jq -nc --arg entityId "$entity_id" --arg query 'query RelationshipsEvidence($entityId: String!) { relationships(entityId: $entityId) { from to predicate } }' '{query:$query,variables:{entityId:$entityId}}')" http://localhost:8081/graph-gateway/graphql | jq -ce 'if ((.errors == null) and (.data.relationships | type == "array")) then . else error("invalid relationships GraphQL envelope") end' >> after/relationship-responses.jsonl; done < after/entity-ids.txt
test "$(wc -l < after/entity-ids.txt | tr -d ' ')" = "$(wc -l < after/relationship-responses.jsonl | tr -d ' ')"
jq -sSe 'if all(.[]; ((.errors == null) and (.data.relationships | type == "array"))) then [.[].data.relationships[]] | unique_by([.from,.predicate,.to]) | sort_by(.from,.predicate,.to) else error("invalid relationships GraphQL envelopes") end' after/relationship-responses.jsonl > after/relationships.normalized.json
```

Repeat the datatype-preserving derived projections.

```sh
jq -Se 'if ((type == "array") and all(.[]; ((type == "object") and (.triples | type == "array")))) then [.[] as $entity | $entity.triples[] | select(.predicate == "quest.status.state") | {id:$entity.id,predicate:.predicate,object:.object,datatype:.datatype}] | sort_by(.id,.predicate,.object,.datatype) else error("invalid normalized entity corpus") end' after/entities.normalized.json > after/quest-status.normalized.json
jq -Se 'if ((type == "array") and all(.[]; ((type == "object") and (.triples | type == "array")))) then [.[] as $entity | $entity.triples[] | select((.predicate | startswith("gateddag.unit.")) or (.predicate | startswith("questdag.unit."))) | {id:$entity.id,predicate:.predicate,object:.object,datatype:.datatype}] | sort_by(.id,.predicate,.object,.datatype) else error("invalid normalized entity corpus") end' after/entities.normalized.json > after/dag-projection.normalized.json
```

## 9. Prove exact parity

Each comparison must exit zero with no output:

```sh
diff -u before/entities.normalized.json after/entities.normalized.json
diff -u before/relationships.normalized.json after/relationships.normalized.json
diff -u before/quest-status.normalized.json after/quest-status.normalized.json
diff -u before/dag-projection.normalized.json after/dag-projection.normalized.json
```

Normalization changes only top-level EntityState updated_at and each triple timestamp. Nested semantic object
timestamp, revision, and readiness keys are preserved. The complete entity corpus and derived captures preserve
entity IDs, predicates, object values, datatypes, cardinality, relationship targets, quest status, DAG dependencies,
and DAG outputs. A difference in any preserved field is a replay failure.

## 10. Retain evidence and report final state

Retain the evidence as one immutable cutover record:

- the approved window and operator identity;
- source, module, NATS CLI, context, topology, volume, and protected-resource checks;
- the rendered backend image name, immutable image ID, and pre-start and post-start assertions;
- the complete terminal transcript;
- every file under `cutover-evidence`, `before`, and `after`;
- all four parity command results; and
- backend and NATS logs covering the seed, restart, readiness checks, and captures.

Report the final environment state as pass only when both readiness checks succeeded, all cardinality checks
passed, both container image IDs matched, every diff exited zero, and no intervening write occurred. Record the
literal surviving project and graph volume, all seed flags false, protected resources unchanged, and any abort or
anomaly. An incomplete evidence record is not a successful replay.

[SemDragon issue #59]: https://github.com/C360Studio/semdragon/issues/59
[parent issue #4]: https://github.com/C360Studio/semdragon/issues/4
[SemStreams #712]: https://github.com/C360Studio/semstreams/issues/712
[SemStreams #713]: https://github.com/C360Studio/semstreams/issues/713
