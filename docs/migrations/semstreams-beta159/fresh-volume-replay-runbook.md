# Fresh-volume replay runbook

## Purpose and status

This executable candidate procedure for [SemDragon issue #59] under
[parent issue #4] gates the SemStreams beta.159 cutover.

Execution status: pending reviewed candidate identity and operational execution.

SemStreams #712 and #713 are resolved in beta.159. Their upstream evidence
unblocks this rerun but does not constitute Semdragon acceptance.

The executable portion tests whether the SemStreams cutover can boot and
restart the disposable SemDragon development graph from exact zero state
without an unowned seed, a spontaneous entity, or ownership/readiness drift.
It is a framework-only preflight, not the required nonempty semantic replay.

SemStreams beta.159 exposes lifecycle discovery, patch, and transition but no
public ownership-safe operation for creating the first lifecycle instance.
[SemStreams #814] tracks that missing primitive. Until it is available, this
runbook MUST NOT reactivate a product seed, call raw graph mutation, or report
append/revision replay acceptance. The earlier failed result below remains
historical evidence, not acceptance of the new candidate.

Run the commands from the repository root in one Bash shell. Stop on the first unexpected value, nonzero exit,
GraphQL error, count mismatch, image mismatch, or parity difference. Do not substitute another commit, module,
Compose project, NATS context, server, volume, configuration, or backend image.

Reviewed commit: set this to the accepted beta.159 candidate before execution.

Reviewed module: `github.com/c360studio/semstreams v1.0.0-beta.159`

Reviewed Compose project: `semdragon-cutover`

Reviewed Compose graph volume: `nats-data`

Reviewed engine volume: `semdragon-cutover_nats-data`

Development/test NATS URL: `nats://localhost:4222`

Development/test NATS context: `semdragon-dev`

Protected services: `local-semsource`, `nats-local-semsource`, `nats-semsource`, and `semsource`.

Protected volumes: `nats-local-semsource-data`, `nats-semsource-data`, `semembed-cache`, and `workspaces`.

The only data deletion in this procedure is the literal removal of `semdragon-cutover_nats-data`. Do not stop,
remove, rebuild, or modify a protected service. Do not remove, prune, recreate, or modify a protected volume.

## Historical beta.158 result

Candidate `a4912d725ccb722e38b87feecb31b7c56e328ffc` used immutable backend image
`sha256:7d8cd71a2eaf1b30e277f3114a0bd9301a526118c29a6c0c297a31ad172f2296`.

Both sides stabilized across three consecutive normalized `ENTITY_STATES`
checksum checks. That temporary quiescence workaround was not authoritative and
did not hide the restart-idempotence failure.

The stable pre-restart digest was
`2033bbde523036af0cfbdc53fa22ba3f2a42973fe793304fcfddc15312ff345a`. The stable post-restart digest was
`bdb7d45d7837732fc3e563d0b874aeef1bc7ece4c57a2a66ca136c73faefc918`.

Exactly six model-registry hierarchy entities changed and 15 duplicate semantic triples were appended. The
domain, type, and system containers each gained three triples and moved from revision 6 to 9. The
`ollama-coder`, `ollama-qwen3`, and `semembed` endpoints each gained two triples and moved from revisions 4 to 6,
3 to 5, and 2 to 4, respectively. The duplicated predicates were `hierarchy.domain.contains`,
`hierarchy.system.contains`, `hierarchy.type.contains`, and `hierarchy.type.sibling`.

The stable digests differ. This evidence explains why the beta.159 replay must
run from fresh state; it must not be relabeled as a pass.

## Preconditions

- Pass the production source gate proving no direct `ENTITY_STATES` creation,
  raw open, raw handle, `Put`, or `Update` remains.
- Pass boot composition with strict owner leases, graph-ingest as catalog owner,
  enabled rule/lifecycle ownership bound through SemStreams, and every unbound
  product writer disabled fail closed.
- Pass all three shipped-config validations, including stream bounds,
  graph-gateway readiness keys, and removed embedding cache fields.
- Confirm SemStreams #814 remains open or replace this zero-state preflight with
  a separately reviewed public lifecycle-create procedure after it closes.
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
test "$(git rev-parse HEAD)" = "REPLACE_WITH_REVIEWED_BETA159_CANDIDATE"
go list -m github.com/c360studio/semstreams
test "$(go list -m github.com/c360studio/semstreams)" = "github.com/c360studio/semstreams v1.0.0-beta.159"
```

Before approval, replace the source placeholder in the document and command
with the reviewed beta.159 Semdragon commit. The module must be exactly
`github.com/c360studio/semstreams v1.0.0-beta.159`. Any mismatch ends the run.

## 2. Render and review topology

The shipped development configurations are:

- `config/semdragons-e2e-tier1.json`
- `config/semdragons-semsource.json`
- `config/semdragons.json`

The reviewed bucket set comes from enabled component ownership and the
SemStreams beta.159 catalog, not only explicit port subjects. Retain the
generated inventory with the evidence and prove that `EMBEDDINGS_CACHE` is
absent. `ENTITY_STATES` must have the catalog definition owned by graph-ingest;
Semdragon must not pre-create or reconcile it.

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

## 6. Start the zero-state framework-only profile

Start the framework-only profile with all product seeding disabled. Product
owner adapters are not yet bound, so enabling any seed flag is a cutover
failure. Assert the immutable local image immediately before and after the
start. The command includes both backend and NATS and forbids builds and pulls.
It also sets one explicit development/test API key so lifecycle discovery can
prove the protected public gateway is running. Do not reuse this literal key
outside the disposable cutover project.

```sh
CUTOVER_API_KEY='semdragon-cutover-replay'
test -n "$CUTOVER_API_KEY"
test "$(docker image inspect "semdragon-cutover-backend" --format '{{.Id}}')" = "$BACKEND_IMAGE_ID"
SEMDRAGONS_API_KEY="$CUTOVER_API_KEY" SEED_E2E=false SEED_AGENTS=false SEED_PROS=false docker compose --project-name semdragon-cutover -f docker/compose.yml up -d --no-build --pull never backend nats
test "$(docker image inspect "semdragon-cutover-backend" --format '{{.Id}}')" = "$BACKEND_IMAGE_ID"
test "$(docker inspect "$(docker compose --project-name semdragon-cutover -f docker/compose.yml ps -q backend)" --format '{{.Image}}')" = "$BACKEND_IMAGE_ID"
```

Poll authoritative caught-up state for the exact producer set. The command
folds one response client-side; it does not consume an aggregate verdict:

```sh
until READINESS_JSON="$(curl --fail --silent --show-error http://localhost:8081/graph-gateway/readiness)" && jq -e '
  (.producers | type == "array") and
  (.producers | length == 3) and
  ([.producers[].key] | sort == ["graph-index", "graph-ingest", "rule"]) and
  all(.producers[];
    (.known == true) and
    (.fresh == true) and
    ((.state == "building") or (.state == "ready")) and
    (.bootstrap_complete == true) and
    (.lag == 0)
  )
' <<<"$READINESS_JSON" >/dev/null; do sleep 2; done
jq -S . <<<"$READINESS_JSON" > before/readiness.json
```

Prove the lifecycle namespace rejects an unauthenticated client, then discover
exactly the registered quest workflow with the reviewed bearer key. These are
read-only calls. Beta.159 has no create route, so do not invent one or attempt
state patch/transition against a nonexistent entity.

```sh
test "$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' http://localhost:8081/lifecycle-gateway/workflows)" = "401"
curl --fail --silent --show-error \
  --header "Authorization: Bearer $CUTOVER_API_KEY" \
  http://localhost:8081/lifecycle-gateway/workflows \
  > before/workflows.raw.json
jq -Se 'if ((type == "array") and (length == 1) and (.[0].workflow == "quest")) then sort_by(.workflow) else error("unexpected lifecycle workflow inventory") end' \
  before/workflows.raw.json > before/workflows.normalized.json
```

Then prove independently that the framework-only graph remains exactly empty
through the flat beta.159 GraphQL envelope. A positive count is a failure: with
all product writers and seeds disabled, it would be an unexplained write.

```sh
curl --fail --silent --show-error --request POST \
  --header 'Content-Type: application/json' \
  --data '{"query":"query CutoverGraphSummary { graphSummary { total_entities } }"}' \
  http://localhost:8081/graph-gateway/graphql > before/graph-summary.raw.json
jq -Se 'if ((.errors == null) and (.data.graphSummary.total_entities == 0)) then .data.graphSummary else error("framework-only graph is not empty") end' \
  before/graph-summary.raw.json > before/graph-summary.normalized.json
```

## 7. Capture the zero-state baseline

Confirm that the framework-owned catalog bucket's backing stream contains zero
messages. The exact-zero GraphQL assertion above and this direct JetStream
state assertion are independent. This avoids relying on key-list behavior for
an empty bucket under `set -euo pipefail`.

```sh
set -euo pipefail
nats --context semdragon-dev stream info KV_ENTITY_STATES --json > before/entity-states-stream.raw.json
jq -Se 'if (.state.messages == 0) then {messages:.state.messages} else error("ENTITY_STATES is not empty") end' \
  before/entity-states-stream.raw.json > before/entity-states-stream.normalized.json
jq -n '[]' > before/entities.normalized.json
jq -n '[]' > before/relationships.normalized.json
jq -n '[]' > before/quest-status.normalized.json
jq -n '[]' > before/dag-projection.normalized.json
```

The four empty arrays are explicit zero-state evidence, not observed semantic
entity, relationship, quest, or DAG replay. No intervening writes are permitted.

## 8. Restart the framework-only profile and recapture zero state

With the maintenance window still exclusive, restart the same reviewed binary. Assert the immutable local image
immediately before and after the restart, and verify the running container image. Every seed flag is false; builds
and pulls remain forbidden.

```sh
test "$(docker image inspect "semdragon-cutover-backend" --format '{{.Id}}')" = "$BACKEND_IMAGE_ID"
SEMDRAGONS_API_KEY="$CUTOVER_API_KEY" SEED_E2E=false SEED_AGENTS=false SEED_PROS=false docker compose --project-name semdragon-cutover -f docker/compose.yml up -d --force-recreate --no-build --pull never backend nats
test "$(docker image inspect "semdragon-cutover-backend" --format '{{.Id}}')" = "$BACKEND_IMAGE_ID"
test "$(docker inspect "$(docker compose --project-name semdragon-cutover -f docker/compose.yml ps -q backend)" --format '{{.Image}}')" = "$BACKEND_IMAGE_ID"
```

Poll the same exact three-producer caught-up condition:

```sh
until READINESS_JSON="$(curl --fail --silent --show-error http://localhost:8081/graph-gateway/readiness)" && jq -e '
  (.producers | type == "array") and
  (.producers | length == 3) and
  ([.producers[].key] | sort == ["graph-index", "graph-ingest", "rule"]) and
  all(.producers[];
    (.known == true) and
    (.fresh == true) and
    ((.state == "building") or (.state == "ready")) and
    (.bootstrap_complete == true) and
    (.lag == 0)
  )
' <<<"$READINESS_JSON" >/dev/null; do sleep 2; done
jq -S . <<<"$READINESS_JSON" > after/readiness.json
```

Repeat authenticated lifecycle discovery and prove the graph and catalog bucket
remain exactly empty:

```sh
test "$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' http://localhost:8081/lifecycle-gateway/workflows)" = "401"
curl --fail --silent --show-error \
  --header "Authorization: Bearer $CUTOVER_API_KEY" \
  http://localhost:8081/lifecycle-gateway/workflows \
  > after/workflows.raw.json
jq -Se 'if ((type == "array") and (length == 1) and (.[0].workflow == "quest")) then sort_by(.workflow) else error("unexpected lifecycle workflow inventory") end' \
  after/workflows.raw.json > after/workflows.normalized.json
curl --fail --silent --show-error --request POST \
  --header 'Content-Type: application/json' \
  --data '{"query":"query CutoverGraphSummary { graphSummary { total_entities } }"}' \
  http://localhost:8081/graph-gateway/graphql > after/graph-summary.raw.json
jq -Se 'if ((.errors == null) and (.data.graphSummary.total_entities == 0)) then .data.graphSummary else error("framework-only graph is not empty") end' \
  after/graph-summary.raw.json > after/graph-summary.normalized.json
nats --context semdragon-dev stream info KV_ENTITY_STATES --json > after/entity-states-stream.raw.json
jq -Se 'if (.state.messages == 0) then {messages:.state.messages} else error("ENTITY_STATES is not empty") end' \
  after/entity-states-stream.raw.json > after/entity-states-stream.normalized.json
jq -n '[]' > after/entities.normalized.json
jq -n '[]' > after/relationships.normalized.json
jq -n '[]' > after/quest-status.normalized.json
jq -n '[]' > after/dag-projection.normalized.json
```

## 9. Prove exact zero-state parity

Each comparison must exit zero with no output:

```sh
diff -u before/entities.normalized.json after/entities.normalized.json
diff -u before/relationships.normalized.json after/relationships.normalized.json
diff -u before/quest-status.normalized.json after/quest-status.normalized.json
diff -u before/dag-projection.normalized.json after/dag-projection.normalized.json
diff -u before/entity-states-stream.normalized.json after/entity-states-stream.normalized.json
diff -u before/graph-summary.normalized.json after/graph-summary.normalized.json
diff -u before/workflows.normalized.json after/workflows.normalized.json
```

These comparisons prove only that the framework-only composition remains empty,
keeps the same registered workflow, and introduces no spontaneous entity across
restart. They do not exercise append identity, entity revision stability, a
lifecycle transition, relationships, quest status, or DAG state.

## 10. Semantic replay remains blocked

Do not report the beta.159 cutover replay accepted from the zero-state result.
SemStreams #814 must first provide a reviewed public, ownership-safe lifecycle
creation operation. A follow-up runbook revision must then:

No legacy product seed or raw mutation is an acceptable workaround for the
missing framework primitive.

1. create one deterministic quest through that public API after exact producer
   readiness;
2. perform only documented lifecycle state or transition calls with the bearer
   key;
3. capture the resulting nonempty entity, relationship, quest-status, and DAG
   evidence without raw graph mutation;
4. restart the identical image with every seed flag false; and
5. require exact semantic parity, stable revisions, and no duplicate append
   identity.

## 11. Retain evidence and report final state

Retain the evidence as one immutable cutover record:

- the approved window and operator identity;
- source, module, NATS CLI, context, topology, volume, and protected-resource checks;
- the rendered backend image name, immutable image ID, and pre-start and post-start assertions;
- the complete terminal transcript;
- every file under `cutover-evidence`, `before`, and `after`;
- all seven zero-state parity command results; and
- backend and NATS logs covering the initial framework-only start, restart,
  readiness checks, and captures.

Report the final environment state only as `zero-state preflight passed` when
both readiness checks succeeded, both lifecycle authentication/discovery checks
passed, both exact-zero graph and bucket-stream assertions passed, both container image IDs
matched, every diff exited zero, and no intervening write occurred. Record the
literal surviving project and graph volume, all seed flags false, protected
resources unchanged, and any abort or anomaly.

Issue #59 and semantic replay remain incomplete pending SemStreams #814. An
incomplete evidence record is not even a successful zero-state preflight, and a
complete zero-state record is still not successful semantic replay.

[SemDragon issue #59]: https://github.com/C360Studio/semdragon/issues/59
[parent issue #4]: https://github.com/C360Studio/semdragon/issues/4
[SemStreams #712]: https://github.com/C360Studio/semstreams/issues/712
[SemStreams #713]: https://github.com/C360Studio/semstreams/issues/713
[SemStreams #814]: https://github.com/C360Studio/semstreams/issues/814
