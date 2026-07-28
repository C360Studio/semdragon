package semdragons_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	freshVolumeRunbookPath = "docs/migrations/semstreams-beta158/fresh-volume-replay-runbook.md"
	reviewedSemStreamsPin  = "github.com/c360studio/semstreams v1.0.0-beta.158.0.20260728003411-89617f32dc36"
	composeProject         = "semdragon-cutover"
	graphVolume            = "nats-data"
	engineGraphVolume      = composeProject + "_" + graphVolume
	developmentNATSURL     = "nats://localhost:4222"
	developmentNATSContext = "semdragon-dev"
	reviewedNATSCLIVersion = "0.3.1"
	reviewedBackendImage   = composeProject + "-backend"
	backendImageNamePath   = "cutover-evidence/backend-image.name"
)

var (
	shippedConfigPaths = []string{
		"config/semdragons-e2e-tier1.json",
		"config/semdragons-semsource.json",
		"config/semdragons.json",
	}
	protectedVolumes = []string{
		"nats-local-semsource-data",
		"nats-semsource-data",
	}
	protectedServices = []string{
		"local-semsource",
		"nats-local-semsource",
		"nats-semsource",
		"semsource",
	}
	backendSeedFlags = []string{
		"SEED_AGENTS",
		"SEED_E2E",
		"SEED_PROS",
	}
	fullCommitPattern = regexp.MustCompile(`(?m)Reviewed commit: ` + "`" + `([0-9a-f]{40})` + "`")
)

type cutoverTopology struct {
	compose          composeContract
	configBuckets    map[string][]string
	candidateBuckets []string
}

type composeContract struct {
	Services map[string]struct {
		Environment any      `yaml:"environment"`
		Volumes     []string `yaml:"volumes"`
	} `yaml:"services"`
	Volumes map[string]any `yaml:"volumes"`
}

type evidenceContract struct {
	label string
	file  string
}

func TestFreshVolumeReplayRunbookContract(t *testing.T) {
	topology := loadCutoverTopology(t)

	t.Run("shipped topology remains reviewed", func(t *testing.T) {
		assertRunbookViolations(t, validateShippedTopology(topology))
	})

	t.Run("runbook is complete and deletion-safe", func(t *testing.T) {
		runbook, err := os.ReadFile(freshVolumeRunbookPath)
		if err != nil {
			t.Errorf("read required runbook %s: %v", freshVolumeRunbookPath, err)
			return
		}
		assertRunbookViolations(t, validateFreshVolumeRunbook(string(runbook), topology))
	})

	t.Run("unsafe or incomplete mutations fail closed", func(t *testing.T) {
		baseline := validRunbookContractFixture(topology)
		assertRunbookViolations(t, validateFreshVolumeRunbook(baseline, topology))

		preservedFields := "entity IDs, predicates, object values, datatypes, cardinality, relationship targets, quest status, DAG dependencies, and DAG outputs"
		wrappedFields := strings.Replace(preservedFields, "quest status, ", "quest status,\n", 1)
		wrapped := strings.Replace(baseline, preservedFields, wrappedFields, 1)
		assertRunbookViolations(t, validateFreshVolumeRunbook(wrapped, topology))
		removedSemanticField := strings.Replace(wrapped, "datatypes, ", "", 1)
		if violations := validateFreshVolumeRunbook(removedSemanticField, topology); !containsRunbookViolation(
			violations,
			"preserved replay fields",
		) {
			t.Fatalf("wrapped prose with removed semantic field violations = %v", violations)
		}

		mutations := []struct {
			name     string
			mutate   func(string) string
			fragment string
		}{
			{
				name: "wildcard deletion target",
				mutate: func(runbook string) string {
					return strings.Replace(runbook, deletionCommand(), "docker volume rm -- docker_*", 1)
				},
				fragment: "wildcard or unresolved deletion target",
			},
			{
				name: "unresolved deletion target",
				mutate: func(runbook string) string {
					return strings.Replace(runbook, deletionCommand(), "docker volume rm -- ${GRAPH_VOLUME}", 1)
				},
				fragment: "wildcard or unresolved deletion target",
			},
			{
				name: "broad compose teardown",
				mutate: func(runbook string) string {
					return runbook + "\ndocker compose --project-name semdragon-cutover -f docker/compose.yml down -v\n"
				},
				fragment: "broad docker compose volume teardown",
			},
			{
				name: "unscoped volume prune",
				mutate: func(runbook string) string {
					return runbook + "\ndocker volume prune --force\n"
				},
				fragment: "unscoped docker volume prune",
			},
			{
				name: "system prune volumes",
				mutate: func(runbook string) string {
					return runbook + "\ndocker system prune --volumes --force\n"
				},
				fragment: "docker system prune with volumes",
			},
			{
				name: "NATS bucket deletion",
				mutate: func(runbook string) string {
					return runbook + "\nnats --context semdragon-dev kv rm ENTITY_STATES --force\n"
				},
				fragment: "NATS bucket or stream deletion",
			},
			{
				name: "NATS stream deletion",
				mutate: func(runbook string) string {
					return runbook + "\nnats --context semdragon-dev stream rm GRAPH --force\n"
				},
				fragment: "NATS bucket or stream deletion",
			},
			{
				name: "protected SemSource volume",
				mutate: func(runbook string) string {
					return strings.Replace(
						runbook,
						deletionCommand(),
						"docker volume rm -- semdragon-cutover_nats-semsource-data",
						1,
					)
				},
				fragment: "protected or out-of-scope volume",
			},
			{
				name: "unrelated volume",
				mutate: func(runbook string) string {
					return strings.Replace(runbook, deletionCommand(), "docker volume rm -- unrelated-data", 1)
				},
				fragment: "protected or out-of-scope volume",
			},
			{
				name: "restart reseeds",
				mutate: func(runbook string) string {
					return strings.Replace(
						runbook,
						"SEED_E2E=false SEED_AGENTS=false SEED_PROS=false docker compose",
						"SEED_E2E=false SEED_AGENTS=true SEED_PROS=false docker compose",
						1,
					)
				},
				fragment: "restart does not set every seed flag false",
			},
			{
				name: "duplicate seed-enabled start",
				mutate: func(runbook string) string {
					return runbook + "\n" + seedEnabledCommand() + "\n"
				},
				fragment: "seed-enabled start count",
			},
			{
				name: "duplicate restart",
				mutate: func(runbook string) string {
					return runbook + "\n" + restartWithoutSeedsCommand() + "\n"
				},
				fragment: "all-false restart count",
			},
			{
				name: "bypassed stopped-writer proof",
				mutate: func(runbook string) string {
					return strings.Replace(runbook, writerProofCommand(), `: "$RUNNING_WRITER_SERVICES"`, 1)
				},
				fragment: "empty stopped-writer assertion",
			},
			{
				name: "removed stopped-writer proof",
				mutate: func(runbook string) string {
					return strings.Replace(runbook, writerProofCommand()+"\n", "", 1)
				},
				fragment: "empty stopped-writer assertion",
			},
			{
				name: "removed backend stop",
				mutate: func(runbook string) string {
					return strings.Replace(runbook, backendStopCommand()+"\n", "", 1)
				},
				fragment: "backend stop count",
			},
			{
				name: "duplicated backend stop",
				mutate: func(runbook string) string {
					return runbook + "\n" + backendStopCommand() + "\n"
				},
				fragment: "backend stop count",
			},
			{
				name: "moved backend stop",
				mutate: func(runbook string) string {
					return swapRunbookLines(runbook, backendStopCommand(), writerQueryCommand())
				},
				fragment: "followed by fail-fast writer query",
			},
			{
				name: "GET readiness",
				mutate: func(runbook string) string {
					return strings.ReplaceAll(runbook, "--request POST", "--request GET")
				},
				fragment: "POST graphSummary readiness",
			},
			{
				name: "wrong readiness payload",
				mutate: func(runbook string) string {
					return strings.ReplaceAll(runbook, "graphSummary", "__typename")
				},
				fragment: "POST graphSummary readiness",
			},
			{
				name: "readiness ignores errors",
				mutate: func(runbook string) string {
					return strings.ReplaceAll(runbook, "(.errors == null)", "true")
				},
				fragment: "POST graphSummary readiness",
			},
			{
				name: "readiness accepts empty graph",
				mutate: func(runbook string) string {
					return strings.ReplaceAll(runbook, "total_entities > 0", "total_entities >= 0")
				},
				fragment: "POST graphSummary readiness",
			},
			{
				name: "readiness OR bypass",
				mutate: func(runbook string) string {
					return strings.ReplaceAll(runbook, readinessCommand(), readinessCommand()+" || true")
				},
				fragment: "POST graphSummary readiness",
			},
			{
				name: "readiness semicolon bypass",
				mutate: func(runbook string) string {
					return strings.ReplaceAll(runbook, readinessCommand(), readinessCommand()+"; true")
				},
				fragment: "POST graphSummary readiness",
			},
			{
				name: "readiness pipeline bypass",
				mutate: func(runbook string) string {
					return strings.ReplaceAll(runbook, readinessCommand(), readinessCommand()+" | cat")
				},
				fragment: "POST graphSummary readiness",
			},
			{
				name: "Compose project drift",
				mutate: func(runbook string) string {
					return strings.ReplaceAll(runbook, composeProject, "other-project")
				},
				fragment: "Compose project",
			},
			{
				name: "missing context creation",
				mutate: func(runbook string) string {
					return strings.Replace(runbook, natsContextSaveCommand()+"\n", "", 1)
				},
				fragment: "NATS context creation",
			},
			{
				name: "missing context selection",
				mutate: func(runbook string) string {
					return strings.Replace(runbook, natsContextSelectCommand()+"\n", "", 1)
				},
				fragment: "NATS context selection",
			},
			{
				name: "wrong context server",
				mutate: func(runbook string) string {
					return strings.ReplaceAll(runbook, developmentNATSURL, "nats://localhost:4333")
				},
				fragment: "NATS context server verification",
			},
			{
				name: "missing no-write interval",
				mutate: func(runbook string) string {
					return strings.Replace(runbook, "No intervening writes are permitted.", "", 1)
				},
				fragment: "no-intervening-write rule",
			},
			{
				name: "extra candidate bucket",
				mutate: func(runbook string) string {
					old := fmt.Sprintf("Candidate buckets (%d):", len(topology.candidateBuckets))
					replacement := fmt.Sprintf("Candidate buckets (%d): `EXTRA`,", len(topology.candidateBuckets)+1)
					return strings.Replace(runbook, old, replacement, 1)
				},
				fragment: "candidate bucket set",
			},
			{
				name: "phase order drift",
				mutate: func(runbook string) string {
					return swapRunbookLines(runbook, deletionCommand(), seedEnabledCommand())
				},
				fragment: "phase order",
			},
			{
				name: "capture does not write file",
				mutate: func(runbook string) string {
					return strings.Replace(
						runbook,
						"> before/entities.normalized.json",
						"before/entities.normalized.json",
						1,
					)
				},
				fragment: "reviewed evidence workflow command",
			},
			{
				name: "missing relationship parity",
				mutate: func(runbook string) string {
					return strings.ReplaceAll(runbook, "relationships.normalized.json", "edges.json")
				},
				fragment: "relationships normalized parity",
			},
			{
				name: "short commit",
				mutate: func(runbook string) string {
					match := fullCommitPattern.FindStringSubmatch(runbook)
					return strings.ReplaceAll(runbook, match[1], match[1][:12])
				},
				fragment: "exact reviewed commit",
			},
		}
		appendMutation := func(name string, mutate func(string) string, fragment string) {
			mutations = append(mutations, struct {
				name     string
				mutate   func(string) string
				fragment string
			}{
				name:     name,
				mutate:   mutate,
				fragment: fragment,
			})
		}
		removeCommand := func(command string) func(string) string {
			return func(runbook string) string {
				return strings.Replace(runbook, command+"\n", "", 1)
			}
		}
		appendMutation(
			"NATS CLI version drift",
			func(runbook string) string {
				return strings.Replace(runbook, reviewedNATSCLIVersion, "0.3.2", 1)
			},
			"pinned NATS CLI version assertion",
		)
		appendMutation(
			"missing NATS CLI version assertion",
			removeCommand(natsCLIVersionCommand()),
			"pinned NATS CLI version assertion",
		)
		appendMutation(
			"NATS CLI version assertion after context invocation",
			func(runbook string) string {
				return swapRunbookLines(runbook, natsCLIVersionCommand(), natsContextSaveCommand())
			},
			"must precede every other NATS invocation",
		)
		appendMutation(
			"container removal also removes volumes",
			func(runbook string) string {
				return strings.Replace(
					runbook,
					removeStoppedContainersCommand(),
					composePrefix()+" rm -f -v backend nats",
					1,
				)
			},
			"container removal must not remove volumes",
		)
		appendMutation(
			"missing stopped container removal",
			removeCommand(removeStoppedContainersCommand()),
			"stopped backend+nats container removal",
		)
		appendMutation(
			"missing empty container IDs assertion",
			removeCommand(emptyContainerIDsCommand()),
			"empty backend+nats container ID assertion",
		)
		appendMutation(
			"missing zero attachment assertion",
			removeCommand(zeroVolumeAttachmentsCommand()),
			"zero graph-volume attachment assertion",
		)
		appendMutation(
			"missing Compose project volume label",
			removeCommand(volumeProjectLabelCommand()),
			"Compose project volume label assertion",
		)
		appendMutation(
			"missing Compose logical volume label",
			removeCommand(volumeLogicalLabelCommand()),
			"Compose logical-volume label assertion",
		)
		appendMutation(
			"protected SemSource service stopped",
			func(runbook string) string {
				return strings.Replace(
					runbook,
					backendStopCommand(),
					backendStopCommand()+" semsource",
					1,
				)
			},
			"protected SemSource service",
		)
		appendMutation(
			"duplicate reviewed backend build",
			func(runbook string) string {
				return runbook + backendBuildCommand() + "\n"
			},
			"reviewed backend build",
		)
		appendMutation(
			"missing cutover evidence directory preparation",
			removeCommand(prepareEvidenceDirectoryCommand()),
			"cutover evidence directory preparation",
		)
		for _, directory := range []string{"before", "after"} {
			directory := directory
			appendMutation(
				"missing "+directory+" evidence directory preparation",
				func(runbook string) string {
					return strings.Replace(
						runbook,
						prepareEvidenceDirectoryCommand(),
						strings.Replace(prepareEvidenceDirectoryCommand(), " "+directory, "", 1),
						1,
					)
				},
				"cutover evidence directory preparation",
			)
		}
		appendMutation(
			"cutover evidence directory prepared after write",
			func(runbook string) string {
				return swapRunbookLines(runbook, prepareEvidenceDirectoryCommand(), writeBackendImageNameCommand())
			},
			"must be prepared before evidence-file writes",
		)
		appendMutation(
			"missing reviewed backend image name write",
			removeCommand(writeBackendImageNameCommand()),
			"reviewed backend image name write",
		)
		appendMutation(
			"wrong reviewed backend image tag",
			func(runbook string) string {
				return strings.Replace(
					runbook,
					writeBackendImageNameCommand(),
					strings.Replace(writeBackendImageNameCommand(), reviewedBackendImage, "other-backend", 1),
					1,
				)
			},
			"reviewed backend image name write",
		)
		appendMutation(
			"missing exact rendered backend image name",
			removeCommand(assertBackendImageNameCommand()),
			"exact rendered backend image name assertion",
		)
		appendMutation(
			"wrong exact rendered backend image name",
			func(runbook string) string {
				return strings.Replace(
					runbook,
					assertBackendImageNameCommand(),
					strings.Replace(assertBackendImageNameCommand(), reviewedBackendImage, "other-backend", 1),
					1,
				)
			},
			"exact rendered backend image name assertion",
		)
		appendMutation(
			"missing rendered backend image membership",
			removeCommand(assertRenderedBackendImageMembershipCommand()),
			"rendered backend image membership assertion",
		)
		for _, count := range []string{"0", "2"} {
			count := count
			appendMutation(
				"rendered backend image membership count "+count,
				func(runbook string) string {
					return strings.Replace(
						runbook,
						assertRenderedBackendImageMembershipCommand(),
						strings.Replace(assertRenderedBackendImageMembershipCommand(), `= "1"`, `= "`+count+`"`, 1),
						1,
					)
				},
				"rendered backend image membership assertion",
			)
		}
		appendMutation(
			"nullable JSON backend image resolution",
			func(runbook string) string {
				return runbook + composePrefix() +
					` config --format json | jq -er '.services.backend.image' > ` + backendImageNamePath + "\n"
			},
			"JSON backend image-field resolution is forbidden",
		)
		appendMutation(
			"Compose images q backend lookup",
			func(runbook string) string {
				return runbook + `BACKEND_IMAGE_ID="$(docker image inspect "$(` + composePrefix() +
					` images -q backend)" --format '{{.Id}}')"` + "\n"
			},
			"Compose images -q backend lookup is forbidden",
		)
		appendMutation(
			"missing backend image capture",
			removeCommand(captureBackendImageIDCommand()),
			"backend image ID capture",
		)
		appendMutation(
			"missing sha256 image assertion",
			removeCommand(assertBackendImageIDSHACommand()),
			"sha256 backend image ID assertion",
		)
		appendMutation(
			"seed start may rebuild",
			func(runbook string) string {
				return strings.Replace(runbook, " --no-build --pull never backend nats", " --pull never backend nats", 1)
			},
			"exact seed-enabled start",
		)
		appendMutation(
			"restart may pull",
			func(runbook string) string {
				return strings.Replace(
					runbook,
					" --force-recreate --no-build --pull never backend nats",
					" --force-recreate --no-build --pull always backend nats",
					1,
				)
			},
			"exact all-false restart",
		)
		appendMutation(
			"missing local image assertion",
			removeCommand(assertLocalImageIDCommand()),
			"local backend image ID assertion",
		)
		appendMutation(
			"missing container image assertion",
			removeCommand(assertContainerImageIDCommand()),
			"started backend container image ID assertion",
		)
		appendMutation(
			"missing running writer assignment",
			removeCommand(writerQueryCommand()),
			"running writer-services assignment",
		)
		appendMutation(
			"writer command failure masked",
			func(runbook string) string {
				return strings.Replace(runbook, writerQueryCommand(), writerQueryCommand()+" || true", 1)
			},
			"running writer-services assignment",
		)
		appendMutation(
			"missing Compose ps all",
			func(runbook string) string {
				return strings.Replace(runbook, " ps --all -q backend nats", " ps -q backend nats", 1)
			},
			"all backend+nats container IDs assignment",
		)
		appendMutation(
			"missing container IDs assignment",
			removeCommand(containerIDsQueryCommand()),
			"all backend+nats container IDs assignment",
		)
		appendMutation(
			"container query failure masked",
			func(runbook string) string {
				return strings.Replace(runbook, containerIDsQueryCommand(), containerIDsQueryCommand()+" || true", 1)
			},
			"all backend+nats container IDs assignment",
		)
		appendMutation(
			"missing graph-volume attachments assignment",
			removeCommand(volumeAttachmentsQueryCommand()),
			"graph-volume attachments assignment",
		)

		entityWorkflow := captureWorkflow("before", "entities.normalized.json")
		relationshipWorkflow := captureWorkflow("before", "relationships.normalized.json")
		appendMutation(
			"pre-deletion pipefail removed",
			func(runbook string) string {
				return strings.Replace(runbook, entityWorkflow[0], "set -eu", 1)
			},
			"fail-fast shell mode",
		)
		appendMutation(
			"wrong ENTITY_STATES inventory bucket",
			func(runbook string) string {
				return strings.Replace(runbook, "kv ls ENTITY_STATES", "kv ls OTHER_STATES", 1)
			},
			"reviewed evidence workflow command",
		)
		appendMutation(
			"ENTITY_STATES get is not raw",
			func(runbook string) string {
				return strings.Replace(runbook, ` "$entity_id" --raw`, ` "$entity_id"`, 1)
			},
			"reviewed evidence workflow command",
		)
		appendMutation(
			"minimal EntityState shape accepted",
			func(runbook string) string {
				return strings.Replace(
					runbook,
					entityStateGuard(true),
					`((type == "object") and (.id == $entity_id) and (.triples | type == "array"))`,
					1,
				)
			},
			"reviewed evidence workflow command",
		)
		appendMutation(
			"entity capture projects away triples",
			func(runbook string) string {
				return strings.Replace(
					runbook,
					`then map(.updated_at = "<normalized>"`,
					`then map({id} | .updated_at = "<normalized>"`,
					1,
				)
			},
			"reviewed evidence workflow command",
		)
		appendMutation(
			"recursive walk normalization",
			func(runbook string) string {
				return strings.Replace(
					runbook,
					`map(.updated_at = "<normalized>" | .triples |= map(.timestamp = "<normalized>"))`,
					`map(walk(if ((type == "object") and has("timestamp")) then .timestamp = "<normalized>" else . end))`,
					1,
				)
			},
			"recursive walk normalization is forbidden",
		)
		appendMutation(
			"nested semantic object timestamp normalized",
			func(runbook string) string {
				return strings.Replace(
					runbook,
					`.triples |= map(.timestamp = "<normalized>")`,
					`.triples |= map(.timestamp = "<normalized>" | if (.object | type == "object") then .object.timestamp = "<normalized>" else . end)`,
					1,
				)
			},
			"nested semantic object metadata normalization is forbidden",
		)
		appendMutation(
			"entity capture deletes triples",
			func(runbook string) string {
				return strings.Replace(runbook, " | sort_by(.id) else", " | map(del(.triples)) | sort_by(.id) else", 1)
			},
			"reviewed evidence workflow command",
		)
		appendMutation(
			"entity capture silently falls back",
			func(runbook string) string {
				return strings.Replace(runbook, "then . else error(\"invalid ENTITY_STATES value\")", "then (. // {}) else error(\"invalid ENTITY_STATES value\")", 1)
			},
			"reviewed evidence workflow command",
		)
		appendMutation(
			"missing key to JSONL cardinality",
			removeCommand(entityWorkflow[5]),
			"reviewed evidence workflow command",
		)
		appendMutation(
			"missing entity cardinality",
			removeCommand(entityWorkflow[7]),
			"reviewed evidence workflow command",
		)
		for _, phase := range []string{"before", "after"} {
			phase := phase
			appendMutation(
				"missing non-empty "+phase+" entity IDs assertion",
				removeCommand(captureWorkflow(phase, "entities.normalized.json")[2]),
				"reviewed evidence workflow command",
			)
		}
		appendMutation(
			"missing EntityState ID uniqueness",
			func(runbook string) string {
				return strings.Replace(
					runbook,
					" and (length == ([.[].id] | unique | length))",
					"",
					1,
				)
			},
			"reviewed evidence workflow command",
		)
		appendMutation(
			"missing final EntityState expected count",
			func(runbook string) string {
				return strings.Replace(runbook, "(length == $expected) and ", "", 1)
			},
			"reviewed evidence workflow command",
		)
		appendMutation(
			"relationship pipeline loop",
			func(runbook string) string {
				return strings.Replace(
					runbook,
					relationshipWorkflow[1],
					"cat before/entity-ids.txt | "+strings.TrimSuffix(relationshipWorkflow[1], " < before/entity-ids.txt"),
					1,
				)
			},
			"pipeline-fed evidence loops",
		)
		appendMutation(
			"relationship response is not fail closed",
			func(runbook string) string {
				return strings.Replace(runbook, " | jq -ce 'if ((.errors == null)", " | jq -c 'if ((.errors == null)", 1)
			},
			"reviewed evidence workflow command",
		)
		appendMutation(
			"missing relationship response cardinality",
			removeCommand(relationshipWorkflow[2]),
			"reviewed evidence workflow command",
		)
		for _, mutation := range []struct {
			name string
			old  string
			new  string
		}{
			{"relationship slurp removed", "jq -sSe", "jq -Se"},
			{"relationship merge removed", "[.[].data.relationships[]]", ".[0].data.relationships"},
			{"relationship dedup removed", " | unique_by([.from,.predicate,.to])", ""},
			{"relationship sort removed", " | sort_by(.from,.predicate,.to)", ""},
		} {
			mutation := mutation
			appendMutation(
				mutation.name,
				func(runbook string) string {
					return strings.Replace(
						runbook,
						relationshipWorkflow[3],
						strings.Replace(relationshipWorkflow[3], mutation.old, mutation.new, 1),
						1,
					)
				},
				"reviewed evidence workflow command",
			)
		}
		appendMutation(
			"quest projection drops datatype",
			func(runbook string) string {
				return strings.Replace(
					runbook,
					captureCommand("before", "quest-status.normalized.json"),
					strings.Replace(captureCommand("before", "quest-status.normalized.json"), ",datatype:.datatype", "", 1),
					1,
				)
			},
			"reviewed evidence workflow command",
		)
		appendMutation(
			"DAG projection drops datatype",
			func(runbook string) string {
				return strings.Replace(
					runbook,
					captureCommand("before", "dag-projection.normalized.json"),
					strings.Replace(captureCommand("before", "dag-projection.normalized.json"), ",datatype:.datatype", "", 1),
					1,
				)
			},
			"reviewed evidence workflow command",
		)

		for _, mutation := range mutations {
			mutation := mutation
			t.Run(mutation.name, func(t *testing.T) {
				violations := validateFreshVolumeRunbook(mutation.mutate(baseline), topology)
				if !containsRunbookViolation(violations, mutation.fragment) {
					t.Fatalf("violations = %v, want fragment %q", violations, mutation.fragment)
				}
			})
		}
	})

	t.Run("topology drift fails closed", func(t *testing.T) {
		baseline := validRunbookContractFixture(topology)
		t.Run("graph volume", func(t *testing.T) {
			mutated := loadCutoverTopology(t)
			service := mutated.compose.Services["nats"]
			service.Volumes = []string{"renamed-graph:/data"}
			mutated.compose.Services["nats"] = service
			violations := validateShippedTopology(mutated)
			if !containsRunbookViolation(violations, "nats /data volume") {
				t.Fatalf("violations = %v, want graph volume drift", violations)
			}
		})
		t.Run("backend seed flag", func(t *testing.T) {
			mutated := loadCutoverTopology(t)
			environment := mutated.compose.Services["backend"].Environment.(map[string]any)
			environment["SEED_NEW"] = "false"
			violations := validateShippedTopology(mutated)
			if !containsRunbookViolation(violations, "backend seed flags") {
				t.Fatalf("violations = %v, want backend seed flag drift", violations)
			}
		})
		t.Run("configuration-derived bucket", func(t *testing.T) {
			mutated := loadCutoverTopology(t)
			mutated.configBuckets[shippedConfigPaths[0]] = append(
				mutated.configBuckets[shippedConfigPaths[0]],
				"NEW_GRAPH_BUCKET",
			)
			mutated.candidateBuckets = append(mutated.candidateBuckets, "NEW_GRAPH_BUCKET")
			sort.Strings(mutated.candidateBuckets)
			violations := validateFreshVolumeRunbook(baseline, mutated)
			if !containsRunbookViolation(violations, "candidate bucket set") {
				t.Fatalf("violations = %v, want config bucket drift", violations)
			}
		})
	})
}

func loadCutoverTopology(t *testing.T) cutoverTopology {
	t.Helper()
	composeData, err := os.ReadFile("docker/compose.yml")
	if err != nil {
		t.Fatalf("read docker/compose.yml: %v", err)
	}
	var compose composeContract
	if err := yaml.Unmarshal(composeData, &compose); err != nil {
		t.Fatalf("parse docker/compose.yml: %v", err)
	}

	configBuckets := make(map[string][]string, len(shippedConfigPaths))
	candidateSet := map[string]bool{}
	for _, path := range shippedConfigPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var document any
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		configBuckets[path] = collectKVSubjects(document)
		for _, bucket := range configBuckets[path] {
			candidateSet[bucket] = true
		}
	}

	candidateBuckets := make([]string, 0, len(candidateSet))
	for bucket := range candidateSet {
		candidateBuckets = append(candidateBuckets, bucket)
	}
	sort.Strings(candidateBuckets)
	return cutoverTopology{
		compose:          compose,
		configBuckets:    configBuckets,
		candidateBuckets: candidateBuckets,
	}
}

func validateShippedTopology(topology cutoverTopology) []string {
	var violations []string
	compose := &topology.compose

	actualConfigs, err := filepath.Glob("config/semdragons*.json")
	if err != nil {
		violations = append(violations, "glob shipped configs: "+err.Error())
	} else if !equalStrings(actualConfigs, shippedConfigPaths) {
		violations = append(
			violations,
			fmt.Sprintf("shipped config set = %v, want %v", actualConfigs, shippedConfigPaths),
		)
	}

	wantVolumeMounts := map[string]string{
		"nats":                 graphVolume,
		"nats-local-semsource": "nats-local-semsource-data",
		"nats-semsource":       "nats-semsource-data",
	}
	for service, wantVolume := range wantVolumeMounts {
		got := dataVolume(compose.Services[service].Volumes)
		if got != wantVolume {
			violations = append(
				violations,
				fmt.Sprintf("%s /data volume = %q, want %q", service, got, wantVolume),
			)
		}
		if _, declared := compose.Volumes[wantVolume]; !declared {
			violations = append(violations, "compose missing declared volume "+wantVolume)
		}
	}

	var gotSeedFlags []string
	backendEnvironment, ok := compose.Services["backend"].Environment.(map[string]any)
	if !ok {
		violations = append(violations, "backend environment is not a mapping")
	}
	for name := range backendEnvironment {
		if strings.HasPrefix(name, "SEED_") {
			gotSeedFlags = append(gotSeedFlags, name)
		}
	}
	sort.Strings(gotSeedFlags)
	if !equalStrings(gotSeedFlags, backendSeedFlags) {
		violations = append(
			violations,
			fmt.Sprintf("backend seed flags = %v, want %v", gotSeedFlags, backendSeedFlags),
		)
	}
	if got := fmt.Sprint(backendEnvironment["SEMDRAGONS_NATS_URLS"]); got != "nats://nats:4222" {
		violations = append(
			violations,
			fmt.Sprintf("backend Compose NATS target = %q, want nats://nats:4222", got),
		)
	}

	for _, path := range shippedConfigPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			violations = append(violations, "read "+path+": "+err.Error())
			continue
		}
		var config struct {
			Platform struct {
				Environment string `json:"environment"`
			} `json:"platform"`
			NATS struct {
				URLs []string `json:"urls"`
			} `json:"nats"`
		}
		if err := json.Unmarshal(data, &config); err != nil {
			violations = append(violations, "parse "+path+": "+err.Error())
			continue
		}
		if config.Platform.Environment != "development" {
			violations = append(violations, path+" is not development context")
		}
		if !equalStrings(config.NATS.URLs, []string{developmentNATSURL}) {
			violations = append(
				violations,
				fmt.Sprintf("%s NATS URLs = %v, want [%s]", path, config.NATS.URLs, developmentNATSURL),
			)
		}
	}

	goMod, err := os.ReadFile("go.mod")
	if err != nil {
		violations = append(violations, "read go.mod: "+err.Error())
	} else if !strings.Contains(string(goMod), reviewedSemStreamsPin) {
		violations = append(violations, "go.mod is not pinned to "+reviewedSemStreamsPin)
	}
	return violations
}

func validateFreshVolumeRunbook(runbook string, topology cutoverTopology) []string {
	var violations []string
	requiredFragments := map[string]string{
		"exact reviewed module pin":    reviewedSemStreamsPin,
		"module preflight":             "go list -m github.com/c360studio/semstreams",
		"rendered Compose inventory":   composePrefix() + " config",
		"rendered volume inventory":    composePrefix() + " config --volumes",
		"reviewed Compose project":     "Reviewed Compose project: `" + composeProject + "`",
		"development NATS target":      "Development/test NATS URL: `" + developmentNATSURL + "`",
		"development NATS context":     "Development/test NATS context: `" + developmentNATSContext + "`",
		"reviewed graph volume":        "Reviewed Compose graph volume: `" + graphVolume + "`",
		"reviewed engine volume":       "Reviewed engine volume: `" + engineGraphVolume + "`",
		"operator deletion review":     "operator reviews and copies the literal target",
		"abort before deletion":        "abort before deletion",
		"stopped-writer command":       backendStopCommand(),
		"seed exactly once":            "Seed canonical state exactly once.",
		"same reviewed binary":         "restart the same reviewed binary",
		"no-intervening-write rule":    "No intervening writes are permitted.",
		"normalization allowlist":      "Normalization changes only top-level EntityState updated_at and each triple timestamp.",
		"nested metadata preservation": "Nested semantic object timestamp, revision, and readiness keys are preserved.",
		"preserved replay fields":      "entity IDs, predicates, object values, datatypes, cardinality, relationship targets, quest status, DAG dependencies, and DAG outputs",
		"retained evidence":            "Retain the evidence",
		"reported final state":         "Report the final environment state",
		"protected local SemSource":    "`nats-local-semsource-data`",
		"protected external SemSource": "`nats-semsource-data`",
		"protected embedding cache":    "`semembed-cache`",
		"protected workspaces":         "`workspaces`",
	}
	for _, service := range protectedServices {
		requiredFragments["protected SemSource service "+service] = "`" + service + "`"
	}
	for label, fragment := range requiredFragments {
		if !containsWhitespaceNormalized(runbook, fragment) {
			violations = append(violations, label+" is missing")
		}
	}
	for _, path := range shippedConfigPaths {
		if !strings.Contains(runbook, path) {
			violations = append(violations, "rendered shipped config is missing "+path)
		}
	}
	violations = append(violations, candidateBucketViolations(runbook, topology.candidateBuckets)...)

	commit := fullCommitPattern.FindStringSubmatch(runbook)
	if len(commit) != 2 || strings.Count(runbook, commit[1]) < 2 {
		violations = append(violations, "exact reviewed commit is not recorded and checked")
	}
	if strings.Count(runbook, reviewedSemStreamsPin) < 2 {
		violations = append(violations, "exact reviewed module pin is not recorded and checked")
	}

	commands := runbookCommandLines(runbook)
	deletionCount := 0
	var seedIndexes, restartIndexes, readinessIndexes []int
	for index, command := range commands {
		lower := strings.ToLower(command)
		if strings.Contains(command, "docker compose ") &&
			!strings.Contains(command, "--project-name "+composeProject) {
			violations = append(violations, "Compose project is missing or inconsistent: "+command)
		}
		if strings.Contains(command, " images -q backend") {
			violations = append(violations, "Compose images -q backend lookup is forbidden")
		}
		if strings.Contains(command, "config --format json") &&
			strings.Contains(command, ".services.backend.image") {
			violations = append(violations, "JSON backend image-field resolution is forbidden")
		}
		if strings.Contains(command, "walk(") {
			violations = append(violations, "recursive walk normalization is forbidden")
		}
		if strings.Contains(command, ".object.timestamp") {
			violations = append(violations, "nested semantic object metadata normalization is forbidden")
		}
		if strings.Contains(lower, "docker compose") &&
			strings.Contains(lower, " down") &&
			(strings.Contains(lower, " -v") || strings.Contains(lower, " --volumes")) {
			violations = append(violations, "broad docker compose volume teardown is forbidden")
		}
		if strings.Contains(lower, "docker compose") &&
			strings.Contains(lower, " rm ") &&
			(strings.Contains(lower, " -v") || strings.Contains(lower, " --volumes")) {
			violations = append(violations, "container removal must not remove volumes")
		}
		if strings.Contains(lower, "docker volume prune") {
			violations = append(violations, "unscoped docker volume prune is forbidden")
		}
		if strings.Contains(lower, "docker system prune") && strings.Contains(lower, "--volumes") {
			violations = append(violations, "docker system prune with volumes is forbidden")
		}
		if isNATSDestructiveCommand(lower) {
			violations = append(violations, "NATS bucket or stream deletion is forbidden")
		}
		if strings.Contains(lower, "docker compose") &&
			(strings.Contains(lower, " stop ") || strings.Contains(lower, " rm ")) {
			for _, service := range protectedServices {
				if strings.Contains(" "+lower+" ", " "+service+" ") {
					violations = append(violations, "protected SemSource service in destructive command: "+service)
				}
			}
		}
		if strings.HasPrefix(command, "docker volume rm ") {
			deletionCount++
			targets := volumeRemovalTargets(command)
			if len(targets) != 1 || hasUnsafeTargetSyntax(targets[0]) {
				violations = append(violations, "wildcard or unresolved deletion target")
			} else if targets[0] != engineGraphVolume {
				violations = append(violations, "protected or out-of-scope volume "+targets[0])
			}
		}
		if strings.HasPrefix(command, "until ") {
			if command == readinessCommand() {
				readinessIndexes = append(readinessIndexes, index)
			}
		}
		if hasAllSeedAssignments(command) && strings.Contains(command, "docker compose") {
			if strings.Contains(command, "SEED_AGENTS=true") ||
				strings.Contains(command, "SEED_E2E=true") ||
				strings.Contains(command, "SEED_PROS=true") {
				seedIndexes = append(seedIndexes, index)
			}
			if allSeedFlagsFalse(command) && strings.Contains(command, "--force-recreate") {
				restartIndexes = append(restartIndexes, index)
			}
		}
	}
	if deletionCount != 1 {
		violations = append(violations, fmt.Sprintf("literal deletion command count = %d, want 1", deletionCount))
	}
	if len(readinessIndexes) != 2 {
		violations = append(
			violations,
			fmt.Sprintf("POST graphSummary readiness count = %d, want 2", len(readinessIndexes)),
		)
	}
	if len(seedIndexes) != 1 {
		violations = append(
			violations,
			fmt.Sprintf("seed-enabled start count = %d, want 1", len(seedIndexes)),
		)
	}
	if got := len(exactCommandIndexes(commands, seedEnabledCommand())); got != 1 {
		violations = append(violations, fmt.Sprintf("exact seed-enabled start count = %d, want 1", got))
	}
	if len(restartIndexes) != 1 {
		violations = append(
			violations,
			fmt.Sprintf("all-false restart count = %d, want 1; restart does not set every seed flag false", len(restartIndexes)),
		)
	}
	if got := len(exactCommandIndexes(commands, restartWithoutSeedsCommand())); got != 1 {
		violations = append(violations, fmt.Sprintf("exact all-false restart count = %d, want 1", got))
	}

	stopIndexes := exactCommandIndexes(commands, backendStopCommand())
	writerQueryIndexes := exactCommandIndexes(commands, writerQueryCommand())
	writerIndexes := exactCommandIndexes(commands, writerProofCommand())
	if len(stopIndexes) != 1 {
		violations = append(
			violations,
			fmt.Sprintf("backend stop count = %d, want 1", len(stopIndexes)),
		)
	}
	if len(writerIndexes) != 1 {
		violations = append(violations, "empty stopped-writer assertion is missing or duplicated")
	}
	if len(stopIndexes) == 1 && len(writerQueryIndexes) == 1 && len(writerIndexes) == 1 &&
		(writerQueryIndexes[0] != stopIndexes[0]+1 || writerIndexes[0] != writerQueryIndexes[0]+1) {
		violations = append(violations, "backend stop must be followed by fail-fast writer query and empty assertion")
	}

	exactLifecycleCommands := []struct {
		command string
		label   string
		count   int
	}{
		{shellSafetyCommand(), "fail-fast shell mode", 3},
		{natsContextSaveCommand(), "NATS context creation", 1},
		{natsContextSelectCommand(), "NATS context selection", 1},
		{natsContextVerifyCommand(), "NATS context server verification", 1},
		{natsCLIVersionCommand(), "pinned NATS CLI version assertion", 1},
		{composeProjectVerifyCommand(), "Compose project rendered identity verification", 1},
		{backendBuildCommand(), "reviewed backend build", 1},
		{prepareEvidenceDirectoryCommand(), "cutover evidence directory preparation", 1},
		{writeBackendImageNameCommand(), "reviewed backend image name write", 1},
		{assertBackendImageNameCommand(), "exact rendered backend image name assertion", 1},
		{assertRenderedBackendImageMembershipCommand(), "rendered backend image membership assertion", 1},
		{captureBackendImageIDCommand(), "backend image ID capture", 1},
		{assertBackendImageIDSHACommand(), "sha256 backend image ID assertion", 1},
		{writerQueryCommand(), "running writer-services assignment", 1},
		{removeStoppedContainersCommand(), "stopped backend+nats container removal", 1},
		{containerIDsQueryCommand(), "all backend+nats container IDs assignment", 1},
		{emptyContainerIDsCommand(), "empty backend+nats container ID assertion", 1},
		{volumeAttachmentsQueryCommand(), "graph-volume attachments assignment", 1},
		{zeroVolumeAttachmentsCommand(), "zero graph-volume attachment assertion", 1},
		{engineVolumeVerifyCommand(), "Compose project volume identity verification", 1},
		{volumeProjectLabelCommand(), "Compose project volume label assertion", 1},
		{volumeLogicalLabelCommand(), "Compose logical-volume label assertion", 1},
		{assertLocalImageIDCommand(), "local backend image ID assertion", 4},
		{assertContainerImageIDCommand(), "started backend container image ID assertion", 2},
	}
	for _, contract := range exactLifecycleCommands {
		got := len(exactCommandIndexes(commands, contract.command))
		if got != contract.count {
			violations = append(
				violations,
				fmt.Sprintf("%s count = %d, want %d", contract.label, got, contract.count),
			)
		}
	}
	shellSafetyIndexes := exactCommandIndexes(commands, shellSafetyCommand())
	if len(shellSafetyIndexes) == 3 && shellSafetyIndexes[0] != 0 {
		violations = append(violations, "fail-fast shell mode must precede pre-deletion lifecycle checks")
	}
	evidenceDirectoryIndexes := exactCommandIndexes(commands, prepareEvidenceDirectoryCommand())
	backendImageWriteIndexes := exactCommandIndexes(commands, writeBackendImageNameCommand())
	if len(evidenceDirectoryIndexes) == 1 && len(backendImageWriteIndexes) == 1 &&
		evidenceDirectoryIndexes[0] >= backendImageWriteIndexes[0] {
		violations = append(violations, "cutover evidence directory must be prepared before evidence-file writes")
	}
	natsVersionIndexes := exactCommandIndexes(commands, natsCLIVersionCommand())
	if len(natsVersionIndexes) == 1 {
		for index, command := range commands {
			if command != natsCLIVersionCommand() &&
				isNATSCLIInvocation(command) &&
				index < natsVersionIndexes[0] {
				violations = append(violations, "pinned NATS CLI version assertion must precede every other NATS invocation")
				break
			}
		}
	}

	parityFiles := []string{
		"entities.normalized.json",
		"relationships.normalized.json",
		"quest-status.normalized.json",
		"dag-projection.normalized.json",
	}
	beforeIndexes := make([]int, 0, len(parityFiles))
	afterIndexes := make([]int, 0, len(parityFiles))
	diffIndexes := make([]int, 0, len(parityFiles))
	deletionIndexes := exactCommandIndexes(commands, deletionCommand())
	workflowCounts := map[string]int{}
	for _, phase := range []string{"before", "after"} {
		for _, name := range parityFiles {
			for _, command := range captureWorkflow(phase, name) {
				workflowCounts[command]++
			}
		}
	}
	workflowCounts[shellSafetyCommand()]++
	for _, name := range parityFiles {
		beforePath := "before/" + name
		afterPath := "after/" + name
		before := exactCommandIndexes(commands, captureCommand("before", name))
		after := exactCommandIndexes(commands, captureCommand("after", name))
		diff := exactCommandIndexes(commands, "diff --no-index -- "+beforePath+" "+afterPath)
		if len(before) != 1 {
			violations = append(violations, "executable query+normalizer for "+beforePath+" is missing or duplicated")
		} else {
			beforeIndexes = append(beforeIndexes, before[0])
		}
		if len(after) != 1 {
			violations = append(violations, "executable query+normalizer for "+afterPath+" is missing or duplicated")
		} else {
			afterIndexes = append(afterIndexes, after[0])
		}
		if len(diff) != 1 {
			label := strings.TrimSuffix(name, ".normalized.json")
			violations = append(violations, label+" normalized parity is missing")
		} else {
			diffIndexes = append(diffIndexes, diff[0])
		}
	}
	for command, want := range workflowCounts {
		if got := len(exactCommandIndexes(commands, command)); got != want {
			violations = append(
				violations,
				fmt.Sprintf("reviewed evidence workflow command count = %d, want %d: %s", got, want, command),
			)
		}
	}
	if strings.Contains(runbook, "| while ") {
		violations = append(violations, "pipeline-fed evidence loops are forbidden")
	}

	localImageIndexes := exactCommandIndexes(commands, assertLocalImageIDCommand())
	containerImageIndexes := exactCommandIndexes(commands, assertContainerImageIDCommand())
	if len(seedIndexes) == 1 && len(restartIndexes) == 1 &&
		len(localImageIndexes) == 4 && len(containerImageIndexes) == 2 {
		if localImageIndexes[0]+1 != seedIndexes[0] ||
			seedIndexes[0]+1 != localImageIndexes[1] ||
			localImageIndexes[1]+1 != containerImageIndexes[0] ||
			localImageIndexes[2]+1 != restartIndexes[0] ||
			restartIndexes[0]+1 != localImageIndexes[3] ||
			localImageIndexes[3]+1 != containerImageIndexes[1] {
			violations = append(violations, "local backend image assertions must bracket both starts before container image checks")
		}
	}

	if len(deletionIndexes) == 1 && len(seedIndexes) == 1 && len(readinessIndexes) == 2 &&
		len(beforeIndexes) == len(parityFiles) && len(restartIndexes) == 1 &&
		len(afterIndexes) == len(parityFiles) && len(diffIndexes) == len(parityFiles) {
		if !strictPhaseOrder(
			deletionIndexes[0],
			seedIndexes[0],
			readinessIndexes[0],
			minimum(beforeIndexes),
			maximum(beforeIndexes),
			restartIndexes[0],
			readinessIndexes[1],
			minimum(afterIndexes),
			maximum(afterIndexes),
			minimum(diffIndexes),
		) {
			violations = append(violations, "phase order must be delete→seed→ready→capture-before→restart→ready→capture-after→diff")
		}
	}
	if len(deletionIndexes) == 1 {
		preflightCommands := []string{
			composeProjectVerifyCommand(),
			natsCLIVersionCommand(),
			natsContextSaveCommand(),
			natsContextSelectCommand(),
			natsContextVerifyCommand(),
			prepareEvidenceDirectoryCommand(),
			writeBackendImageNameCommand(),
			assertBackendImageNameCommand(),
			assertRenderedBackendImageMembershipCommand(),
			backendBuildCommand(),
			captureBackendImageIDCommand(),
			assertBackendImageIDSHACommand(),
			backendStopCommand(),
			writerQueryCommand(),
			writerProofCommand(),
			removeStoppedContainersCommand(),
			containerIDsQueryCommand(),
			emptyContainerIDsCommand(),
			volumeAttachmentsQueryCommand(),
			zeroVolumeAttachmentsCommand(),
			engineVolumeVerifyCommand(),
			volumeProjectLabelCommand(),
			volumeLogicalLabelCommand(),
		}
		previous := -1
		for _, command := range preflightCommands {
			indexes := exactCommandIndexes(commands, command)
			if len(indexes) != 1 || indexes[0] <= previous || indexes[0] >= deletionIndexes[0] {
				violations = append(violations, "reviewed build, context, stop, removal, and volume checks must precede deletion")
				break
			}
			previous = indexes[0]
		}
	}
	if len(commit) == 2 {
		commitAssertion := `test "$(git rev-parse HEAD)" = "` + commit[1] + `"`
		moduleAssertion := `test "$(go list -m github.com/c360studio/semstreams)" = "` + reviewedSemStreamsPin + `"`
		buildIndexes := exactCommandIndexes(commands, backendBuildCommand())
		commitIndexes := exactCommandIndexes(commands, commitAssertion)
		moduleIndexes := exactCommandIndexes(commands, moduleAssertion)
		if len(buildIndexes) == 1 &&
			(len(commitIndexes) != 1 || len(moduleIndexes) != 1 ||
				commitIndexes[0] >= buildIndexes[0] || moduleIndexes[0] >= buildIndexes[0]) {
			violations = append(violations, "backend must be built once after reviewed commit and module assertions")
		}
	}
	lastBefore := captureCommand("before", "dag-projection.normalized.json")
	noWrites := "No intervening writes are permitted."
	if !(strings.Index(runbook, lastBefore) < strings.Index(runbook, noWrites) &&
		strings.Index(runbook, noWrites) < strings.Index(runbook, restartWithoutSeedsCommand())) {
		violations = append(violations, "no-intervening-write rule must separate before capture and restart")
	}
	sort.Strings(violations)
	return violations
}

func validRunbookContractFixture(topology cutoverTopology) string {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	buckets := make([]string, 0, len(topology.candidateBuckets))
	for _, bucket := range topology.candidateBuckets {
		buckets = append(buckets, "`"+bucket+"`")
	}
	lines := []string{
		"# Fresh-volume replay",
		"",
		"Reviewed commit: `" + commit + "`",
		shellSafetyCommand(),
		`test "$(git rev-parse HEAD)" = "` + commit + `"`,
		"Reviewed module: `" + reviewedSemStreamsPin + "`",
		"go list -m github.com/c360studio/semstreams",
		`test "$(go list -m github.com/c360studio/semstreams)" = "` + reviewedSemStreamsPin + `"`,
		"Rendered configs: " + composePrefix() + " config",
		"config/semdragons.json",
		"config/semdragons-e2e-tier1.json",
		"config/semdragons-semsource.json",
		"Reviewed Compose project: `" + composeProject + "`",
		"Reviewed Compose graph volume: `" + graphVolume + "`",
		"Reviewed engine volume: `" + engineGraphVolume + "`",
		"Development/test NATS URL: `" + developmentNATSURL + "`",
		"Development/test NATS context: `" + developmentNATSContext + "`",
		"Protected services: `local-semsource`, `nats-local-semsource`, `nats-semsource`, and `semsource`.",
		"Protected volumes: `nats-local-semsource-data`, `nats-semsource-data`, `semembed-cache`, and `workspaces`.",
		fmt.Sprintf("Candidate buckets (%d): %s.", len(buckets), strings.Join(buckets, ", ")),
		composePrefix() + " config",
		composePrefix() + " config --volumes",
		composeProjectVerifyCommand(),
		natsCLIVersionCommand(),
		natsContextSaveCommand(),
		natsContextSelectCommand(),
		natsContextVerifyCommand(),
		prepareEvidenceDirectoryCommand(),
		writeBackendImageNameCommand(),
		assertBackendImageNameCommand(),
		assertRenderedBackendImageMembershipCommand(),
		backendBuildCommand(),
		captureBackendImageIDCommand(),
		assertBackendImageIDSHACommand(),
		"The operator reviews and copies the literal target. Any mismatch must abort before deletion.",
		backendStopCommand(),
		writerQueryCommand(),
		writerProofCommand(),
		removeStoppedContainersCommand(),
		containerIDsQueryCommand(),
		emptyContainerIDsCommand(),
		volumeAttachmentsQueryCommand(),
		zeroVolumeAttachmentsCommand(),
		engineVolumeVerifyCommand(),
		volumeProjectLabelCommand(),
		volumeLogicalLabelCommand(),
		deletionCommand(),
		"Seed canonical state exactly once.",
		assertLocalImageIDCommand(),
		seedEnabledCommand(),
		assertLocalImageIDCommand(),
		assertContainerImageIDCommand(),
		readinessCommand(),
	}
	for _, contract := range evidenceContracts() {
		lines = append(lines, captureWorkflow("before", contract.file)...)
	}
	lines = append(
		lines,
		"No intervening writes are permitted.",
		"Now restart the same reviewed binary.",
		assertLocalImageIDCommand(),
		restartWithoutSeedsCommand(),
		assertLocalImageIDCommand(),
		assertContainerImageIDCommand(),
		readinessCommand(),
	)
	for _, contract := range evidenceContracts() {
		lines = append(lines, captureWorkflow("after", contract.file)...)
	}
	lines = append(
		lines,
		"diff --no-index -- before/entities.normalized.json after/entities.normalized.json",
		"diff --no-index -- before/relationships.normalized.json after/relationships.normalized.json",
		"diff --no-index -- before/quest-status.normalized.json after/quest-status.normalized.json",
		"diff --no-index -- before/dag-projection.normalized.json after/dag-projection.normalized.json",
		"Normalization changes only top-level EntityState updated_at and each triple timestamp.",
		"Nested semantic object timestamp, revision, and readiness keys are preserved.",
		"Preserve entity IDs, predicates, object values, datatypes, cardinality, relationship targets, quest status, DAG dependencies, and DAG outputs.",
		"Retain the evidence. Report the final environment state.",
	)
	return strings.Join(lines, "\n") + "\n"
}

func candidateBucketViolations(runbook string, want []string) []string {
	pattern := regexp.MustCompile(`(?m)^Candidate buckets \(([0-9]+)\): (.+)\.$`)
	match := pattern.FindStringSubmatch(runbook)
	if len(match) != 3 {
		return []string{"candidate bucket set and cardinality are missing"}
	}
	count, err := strconv.Atoi(match[1])
	if err != nil {
		return []string{"candidate bucket cardinality is invalid"}
	}
	tokenPattern := regexp.MustCompile("`([^`]+)`")
	tokenMatches := tokenPattern.FindAllStringSubmatch(match[2], -1)
	got := make([]string, 0, len(tokenMatches))
	for _, token := range tokenMatches {
		got = append(got, token[1])
	}
	sort.Strings(got)
	if count != len(got) || count != len(want) || !equalStrings(got, want) {
		return []string{fmt.Sprintf("candidate bucket set/cardinality = %d %v, want %d %v", count, got, len(want), want)}
	}
	return nil
}

func composePrefix() string {
	return "docker compose --project-name " + composeProject + " -f docker/compose.yml"
}

func composeProjectVerifyCommand() string {
	return `test "$(` + composePrefix() +
		` config --format json | jq -r '.name')" = "` + composeProject + `"`
}

func engineVolumeVerifyCommand() string {
	return `test "$(docker volume inspect ` + engineGraphVolume +
		` --format '{{.Name}}')" = "` + engineGraphVolume + `"`
}

func natsContextSaveCommand() string {
	return "nats context save " + developmentNATSContext + " --server " + developmentNATSURL
}

func natsContextSelectCommand() string {
	return "nats context select " + developmentNATSContext
}

func natsContextVerifyCommand() string {
	return `test "$(nats context info ` + developmentNATSContext +
		` --json | jq -r '.server')" = "` + developmentNATSURL + `"`
}

func shellSafetyCommand() string {
	return "set -euo pipefail"
}

func writerQueryCommand() string {
	return `RUNNING_WRITER_SERVICES="$(` + composePrefix() + ` ps --status running --services backend nats)"`
}

func writerProofCommand() string {
	return `test -z "$RUNNING_WRITER_SERVICES"`
}

func backendStopCommand() string {
	return composePrefix() + " stop backend nats"
}

func removeStoppedContainersCommand() string {
	return composePrefix() + " rm -f backend nats"
}

func containerIDsQueryCommand() string {
	return `BACKEND_NATS_CONTAINER_IDS="$(` + composePrefix() + ` ps --all -q backend nats)"`
}

func emptyContainerIDsCommand() string {
	return `test -z "$BACKEND_NATS_CONTAINER_IDS"`
}

func volumeAttachmentsQueryCommand() string {
	return `GRAPH_VOLUME_ATTACHMENTS="$(docker ps --all --quiet --filter volume=` + engineGraphVolume + `)"`
}

func zeroVolumeAttachmentsCommand() string {
	return `test -z "$GRAPH_VOLUME_ATTACHMENTS"`
}

func volumeProjectLabelCommand() string {
	return `test "$(docker volume inspect ` + engineGraphVolume +
		` --format '{{ index .Labels "com.docker.compose.project" }}')" = "` + composeProject + `"`
}

func volumeLogicalLabelCommand() string {
	return `test "$(docker volume inspect ` + engineGraphVolume +
		` --format '{{ index .Labels "com.docker.compose.volume" }}')" = "` + graphVolume + `"`
}

func natsCLIVersionCommand() string {
	return `test "$(nats --version)" = "` + reviewedNATSCLIVersion + `"`
}

func backendBuildCommand() string {
	return composePrefix() + " build backend"
}

func prepareEvidenceDirectoryCommand() string {
	return "mkdir -p cutover-evidence before after"
}

func writeBackendImageNameCommand() string {
	return `printf '%s\n' '` + reviewedBackendImage + `' > ` + backendImageNamePath
}

func assertBackendImageNameCommand() string {
	return `test "$(cat ` + backendImageNamePath + `)" = "` + reviewedBackendImage + `"`
}

func assertRenderedBackendImageMembershipCommand() string {
	return `test "$(` + composePrefix() + ` config --images | grep -Fxc "$(cat ` +
		backendImageNamePath + `)")" = "1"`
}

func captureBackendImageIDCommand() string {
	return `BACKEND_IMAGE_ID="$(docker image inspect "` + reviewedBackendImage + `" --format '{{.Id}}')"`
}

func assertBackendImageIDSHACommand() string {
	return `case "$BACKEND_IMAGE_ID" in sha256:*) ;; *) exit 1 ;; esac`
}

func assertLocalImageIDCommand() string {
	return `test "$(docker image inspect "` + reviewedBackendImage + `" --format '{{.Id}}')" = "$BACKEND_IMAGE_ID"`
}

func assertContainerImageIDCommand() string {
	return `test "$(docker inspect "$(` + composePrefix() +
		` ps -q backend)" --format '{{.Image}}')" = "$BACKEND_IMAGE_ID"`
}

func deletionCommand() string {
	return "docker volume rm -- " + engineGraphVolume
}

func seedEnabledCommand() string {
	return "SEED_E2E=false SEED_AGENTS=true SEED_PROS=false " +
		composePrefix() + " up -d --no-build --pull never backend nats"
}

func restartWithoutSeedsCommand() string {
	return "SEED_E2E=false SEED_AGENTS=false SEED_PROS=false " +
		composePrefix() + " up -d --force-recreate --no-build --pull never backend nats"
}

func readinessCommand() string {
	return `until curl --fail --silent --show-error --request POST ` +
		`--header 'Content-Type: application/json' ` +
		`--data '{"query":"query CutoverReadiness { graphSummary { total_entities } }"}' ` +
		`http://localhost:8081/graph-gateway/graphql | ` +
		`jq -e '(.errors == null) and (.data.graphSummary.total_entities > 0)' >/dev/null; do sleep 2; done`
}

func captureCommand(phase, name string) string {
	workflow := captureWorkflow(phase, name)
	if len(workflow) == 0 {
		return ""
	}
	return workflow[len(workflow)-1]
}

func crossLabelCaptureCommand(phase string, target, source evidenceContract) string {
	command := captureCommand(phase, source.file)
	return strings.Replace(command, phase+"/"+source.file, phase+"/"+target.file, 1)
}

func replaceBeforeCapture(runbook string, contract evidenceContract, replacement string) string {
	return strings.Replace(runbook, captureCommand("before", contract.file), replacement, 1)
}

func evidenceContracts() []evidenceContract {
	return []evidenceContract{
		{
			label: "entities",
			file:  "entities.normalized.json",
		},
		{
			label: "relationships",
			file:  "relationships.normalized.json",
		},
		{
			label: "quest-status",
			file:  "quest-status.normalized.json",
		},
		{
			label: "dag-projection",
			file:  "dag-projection.normalized.json",
		},
	}
}

func captureWorkflow(phase, name string) []string {
	entityIDs := phase + "/entity-ids.txt"
	rawEntities := phase + "/entities.raw.jsonl"
	normalizedEntities := phase + "/entities.normalized.json"
	relationshipResponses := phase + "/relationship-responses.jsonl"
	keyedEntityGuard := entityStateGuard(true)
	storedEntityGuard := entityStateGuard(false)
	switch name {
	case "entities.normalized.json":
		return []string{
			shellSafetyCommand(),
			"nats --context " + developmentNATSContext + " kv ls ENTITY_STATES | LC_ALL=C sort > " + entityIDs,
			"test -s " + entityIDs,
			": > " + rawEntities,
			`while IFS= read -r entity_id; do nats --context ` + developmentNATSContext +
				` kv get ENTITY_STATES "$entity_id" --raw | jq -ce --arg entity_id "$entity_id" ` +
				`'if ` + keyedEntityGuard + ` then . else error("invalid ENTITY_STATES value") end' >> ` +
				rawEntities + `; done < ` + entityIDs,
			`test "$(wc -l < ` + entityIDs + ` | tr -d ' ')" = "$(wc -l < ` + rawEntities + ` | tr -d ' ')"`,
			`jq -sSe --argjson expected "$(wc -l < ` + entityIDs + ` | tr -d ' ')" ` +
				`'if (length == $expected) and (length == ([.[].id] | unique | length)) and all(.[]; ` + storedEntityGuard +
				`) then map(.updated_at = "<normalized>" | .triples |= map(.timestamp = "<normalized>")) | sort_by(.id) else error("invalid ENTITY_STATES JSONL") end' ` +
				rawEntities + " > " + normalizedEntities,
			`test "$(wc -l < ` + entityIDs + ` | tr -d ' ')" = "$(jq -r 'length' ` + normalizedEntities + `)"`,
		}
	case "relationships.normalized.json":
		return []string{
			": > " + relationshipResponses,
			`while IFS= read -r entity_id; do curl --fail --silent --show-error --request POST ` +
				`--header 'Content-Type: application/json' ` +
				`--data "$(jq -nc --arg entityId "$entity_id" --arg query 'query RelationshipsEvidence($entityId: String!) { relationships(entityId: $entityId) { from to predicate } }' '{query:$query,variables:{entityId:$entityId}}')" ` +
				`http://localhost:8081/graph-gateway/graphql | jq -ce 'if ((.errors == null) and (.data.relationships | type == "array")) then . else error("invalid relationships GraphQL envelope") end' >> ` +
				relationshipResponses + `; done < ` + entityIDs,
			`test "$(wc -l < ` + entityIDs + ` | tr -d ' ')" = "$(wc -l < ` + relationshipResponses + ` | tr -d ' ')"`,
			`jq -sSe 'if all(.[]; ((.errors == null) and (.data.relationships | type == "array"))) then [.[].data.relationships[]] | unique_by([.from,.predicate,.to]) | sort_by(.from,.predicate,.to) else error("invalid relationships GraphQL envelopes") end' ` +
				relationshipResponses + " > " + phase + "/relationships.normalized.json",
		}
	case "quest-status.normalized.json":
		return []string{
			`jq -Se 'if ((type == "array") and all(.[]; ((type == "object") and (.triples | type == "array")))) then [.[] as $entity | $entity.triples[] | select(.predicate == "quest.status.state") | {id:$entity.id,predicate:.predicate,object:.object,datatype:.datatype}] | sort_by(.id,.predicate,.object,.datatype) else error("invalid normalized entity corpus") end' ` +
				normalizedEntities + " > " + phase + "/quest-status.normalized.json",
		}
	case "dag-projection.normalized.json":
		return []string{
			`jq -Se 'if ((type == "array") and all(.[]; ((type == "object") and (.triples | type == "array")))) then [.[] as $entity | $entity.triples[] | select((.predicate | startswith("gateddag.unit.")) or (.predicate | startswith("questdag.unit."))) | {id:$entity.id,predicate:.predicate,object:.object,datatype:.datatype}] | sort_by(.id,.predicate,.object,.datatype) else error("invalid normalized entity corpus") end' ` +
				normalizedEntities + " > " + phase + "/dag-projection.normalized.json",
		}
	}
	return nil
}

func entityStateGuard(matchKey bool) string {
	idGuard := `(.id | type == "string")`
	if matchKey {
		idGuard += ` and (.id == $entity_id)`
	}
	return `(type == "object") and ` + idGuard +
		` and (.message_type | type == "object")` +
		` and (.message_type.domain | type == "string")` +
		` and (.message_type.category | type == "string")` +
		` and (.message_type.version | type == "string")` +
		` and (.version | type == "number")` +
		` and (.updated_at | type == "string")` +
		` and (.triples | type == "array")` +
		` and all(.triples[]; (type == "object")` +
		` and (.subject | type == "string")` +
		` and (.predicate | type == "string")` +
		` and has("object")` +
		` and (.source | type == "string")` +
		` and (.timestamp | type == "string")` +
		` and (.confidence | type == "number"))`
}

func isNATSDestructiveCommand(command string) bool {
	if !strings.HasPrefix(strings.TrimSpace(command), "nats ") {
		return false
	}
	destructive := []string{
		" kv rm ",
		" kv purge ",
		" kv del ",
		" stream rm ",
		" stream purge ",
		" stream delete ",
	}
	padded := " " + command + " "
	for _, fragment := range destructive {
		if strings.Contains(padded, fragment) {
			return true
		}
	}
	return false
}

func isNATSCLIInvocation(command string) bool {
	command = strings.TrimSpace(command)
	return strings.HasPrefix(command, "nats ") ||
		strings.Contains(command, "$(nats ") ||
		strings.Contains(command, " do nats ")
}

func exactCommandIndexes(commands []string, want string) []int {
	var indexes []int
	for index, command := range commands {
		if command == want {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func strictPhaseOrder(indexes ...int) bool {
	for index := 1; index < len(indexes); index++ {
		if indexes[index-1] >= indexes[index] {
			return false
		}
	}
	return true
}

func minimum(values []int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func maximum(values []int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value > result {
			result = value
		}
	}
	return result
}

func swapRunbookLines(runbook, first, second string) string {
	const marker = "\x00RUNBOOK-SWAP\x00"
	runbook = strings.Replace(runbook, first, marker, 1)
	runbook = strings.Replace(runbook, second, first, 1)
	return strings.Replace(runbook, marker, second, 1)
}

func collectKVSubjects(value any) []string {
	subjects := map[string]bool{}
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			portType, _ := typed["type"].(string)
			subject, _ := typed["subject"].(string)
			if subject != "" && (portType == "kv" || portType == "kv-write" || portType == "kv-watch") {
				subjects[subject] = true
			}
			for _, child := range typed {
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	result := make([]string, 0, len(subjects))
	for subject := range subjects {
		result = append(result, subject)
	}
	sort.Strings(result)
	return result
}

func dataVolume(mounts []string) string {
	for _, mount := range mounts {
		parts := strings.Split(mount, ":")
		if len(parts) >= 2 && parts[1] == "/data" {
			return parts[0]
		}
	}
	return ""
}

func runbookCommandLines(runbook string) []string {
	var commands []string
	for _, line := range strings.Split(runbook, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "$ ")
		if strings.HasPrefix(line, "docker ") ||
			strings.HasPrefix(line, "nats ") ||
			strings.HasPrefix(line, "test ") ||
			strings.HasPrefix(line, "set ") ||
			strings.HasPrefix(line, ": ") ||
			strings.HasPrefix(line, "while ") ||
			strings.HasPrefix(line, "BACKEND_") ||
			strings.HasPrefix(line, "RUNNING_") ||
			strings.HasPrefix(line, "GRAPH_") ||
			strings.HasPrefix(line, "case ") ||
			strings.HasPrefix(line, "printf ") ||
			strings.HasPrefix(line, "mkdir ") ||
			strings.HasPrefix(line, "until ") ||
			strings.HasPrefix(line, "SEED_") ||
			strings.HasPrefix(line, "curl ") ||
			strings.HasPrefix(line, "jq ") ||
			strings.HasPrefix(line, "diff ") {
			commands = append(commands, line)
		}
	}
	return commands
}

func volumeRemovalTargets(command string) []string {
	fields := strings.Fields(command)
	if len(fields) < 4 {
		return nil
	}
	targets := fields[3:]
	if len(targets) > 0 && targets[0] == "--" {
		targets = targets[1:]
	}
	return targets
}

func hasUnsafeTargetSyntax(target string) bool {
	return target == "" || strings.ContainsAny(target, `$*?[]{}"'\`)
}

func hasAllSeedAssignments(command string) bool {
	for _, flag := range backendSeedFlags {
		if !strings.Contains(command, flag+"=") {
			return false
		}
	}
	return true
}

func allSeedFlagsFalse(command string) bool {
	for _, flag := range backendSeedFlags {
		if !strings.Contains(command, flag+"=false") {
			return false
		}
	}
	return true
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func assertRunbookViolations(t *testing.T, violations []string) {
	t.Helper()
	for _, violation := range violations {
		t.Error(violation)
	}
}

func containsRunbookViolation(violations []string, fragment string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, fragment) {
			return true
		}
	}
	return false
}

func containsWhitespaceNormalized(document, fragment string) bool {
	normalize := func(value string) string {
		return strings.Join(strings.Fields(value), " ")
	}
	return strings.Contains(normalize(document), normalize(fragment))
}
