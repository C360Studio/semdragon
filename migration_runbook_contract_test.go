package semdragons_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const (
	freshVolumeRunbookPath = "docs/migrations/semstreams-beta159/fresh-volume-replay-runbook.md"
	composeProject         = "semdragon-cutover"
	graphVolume            = "semdragon-cutover_nats-data"
	semStreams814URL       = "https://github.com/C360Studio/semstreams/issues/814"
)

func TestFreshVolumeReplayRunbookContract(t *testing.T) {
	runbookBytes, err := os.ReadFile(freshVolumeRunbookPath)
	if err != nil {
		t.Fatalf("read %s: %v", freshVolumeRunbookPath, err)
	}
	runbook := string(runbookBytes)
	assertNoRunbookViolations(t, validateZeroStateRunbook(runbook))

	t.Run("critical safety and truthfulness mutations fail closed", func(t *testing.T) {
		mutations := []struct {
			name     string
			mutate   func(string) string
			fragment string
		}{
			{
				name: "wildcard deletion",
				mutate: func(input string) string {
					return strings.Replace(input, deletionCommand(), "docker volume rm -- semdragon-cutover_*", 1)
				},
				fragment: "literal graph-volume deletion",
			},
			{
				name: "broad compose teardown",
				mutate: func(input string) string {
					return input + "\ndocker compose --project-name semdragon-cutover -f docker/compose.yml down -v\n"
				},
				fragment: "broad destructive command",
			},
			{
				name: "restart enables product seed",
				mutate: func(input string) string {
					return strings.Replace(input, restartCommand(), strings.Replace(restartCommand(), "SEED_E2E=false", "SEED_E2E=true", 1), 1)
				},
				fragment: "all-false restart",
			},
			{
				name: "readiness omits rule",
				mutate: func(input string) string {
					return strings.ReplaceAll(input, readinessProducerSet(), `["graph-index", "graph-ingest"]`)
				},
				fragment: "exact readiness producer set",
			},
			{
				name: "graph permits any nonnegative count",
				mutate: func(input string) string {
					return strings.ReplaceAll(input, ".data.graphSummary.total_entities == 0", ".data.graphSummary.total_entities >= 0")
				},
				fragment: "exact-zero flat graph summary",
			},
			{
				name: "catalog emptiness weakened",
				mutate: func(input string) string {
					return strings.Replace(input, ".state.messages == 0", ".state.messages >= 0", 1)
				},
				fragment: "exact-zero catalog stream",
			},
			{
				name: "upstream blocker omitted",
				mutate: func(input string) string {
					return strings.Replace(input, semStreams814URL, "", 1)
				},
				fragment: "SemStreams #814 link",
			},
			{
				name: "empty smoke mislabeled acceptance",
				mutate: func(input string) string {
					return strings.Replace(input, "zero-state preflight passed", "semantic replay passed", 1)
				},
				fragment: "zero-state-only result label",
			},
			{
				name: "raw graph workaround",
				mutate: func(input string) string {
					return input + "\nnats --context semdragon-dev pub graph.mutation.create '{}'\n"
				},
				fragment: "raw graph mutation workaround",
			},
			{
				name: "invented transition against absent entity",
				mutate: func(input string) string {
					return input + "\ncurl --request POST http://localhost:8081/lifecycle-gateway/workflows/quest/missing/transition\n"
				},
				fragment: "lifecycle mutation before #814",
			},
			{
				name: "authenticated discovery omitted",
				mutate: func(input string) string {
					return strings.Replace(input, `--header "Authorization: Bearer $CUTOVER_API_KEY"`, "", 1)
				},
				fragment: "authenticated workflow discovery",
			},
		}

		for _, mutation := range mutations {
			t.Run(mutation.name, func(t *testing.T) {
				violations := validateZeroStateRunbook(mutation.mutate(runbook))
				if !containsRunbookViolation(violations, mutation.fragment) {
					t.Fatalf("violations = %v; want fragment %q", violations, mutation.fragment)
				}
			})
		}
	})
}

func validateZeroStateRunbook(runbook string) []string {
	var violations []string
	requireCount := func(label, fragment string, want int) {
		if got := strings.Count(runbook, fragment); got != want {
			violations = append(violations, fmt.Sprintf("%s count = %d, want %d", label, got, want))
		}
	}
	requireContains := func(label, fragment string) {
		if !strings.Contains(runbook, fragment) {
			violations = append(violations, label+" is missing")
		}
	}

	requireCount("literal graph-volume deletion", deletionCommand(), 1)
	requireContains("exact Compose project", "Reviewed Compose project: `"+composeProject+"`")
	requireContains("exact graph volume", "Reviewed engine volume: `"+graphVolume+"`")
	requireContains("zero-attachment deletion guard", `test -z "$GRAPH_VOLUME_ATTACHMENTS"`)
	requireContains("literal volume-name guard", `test "$(docker volume inspect semdragon-cutover_nats-data --format '{{.Name}}')" = "semdragon-cutover_nats-data"`)
	requireContains("Compose project label guard", `com.docker.compose.project`)
	requireContains("Compose volume label guard", `com.docker.compose.volume`)

	for _, forbidden := range []string{
		"docker compose --project-name semdragon-cutover -f docker/compose.yml down -v",
		"docker volume prune",
		"docker system prune",
		"nats --context semdragon-dev kv rm",
		"nats --context semdragon-dev stream rm",
	} {
		if strings.Contains(runbook, forbidden) {
			violations = append(violations, "broad destructive command is present: "+forbidden)
		}
	}

	requireCount("framework-only initial start", initialStartCommand(), 1)
	requireCount("all-false restart", restartCommand(), 1)
	requireCount("single backend build", "docker compose --project-name semdragon-cutover -f docker/compose.yml build backend", 1)
	requireCount("no-build/no-pull starts", "--no-build --pull never backend nats", 2)
	for _, flag := range []string{"SEED_E2E", "SEED_AGENTS", "SEED_PROS"} {
		if strings.Contains(runbook, flag+"=true") {
			violations = append(violations, "product seed is enabled: "+flag)
		}
		requireCount(flag+" all-false assignments", flag+"=false", 2)
	}

	requireCount("authoritative readiness endpoint", "http://localhost:8081/graph-gateway/readiness", 2)
	requireCount("exact readiness producer set", readinessProducerSet(), 2)
	for _, gate := range []string{
		".known == true",
		".fresh == true",
		".bootstrap_complete == true",
		".lag == 0",
	} {
		requireCount("readiness gate "+gate, gate, 2)
	}
	requireContains("before readiness evidence", `> before/readiness.json`)
	requireContains("after readiness evidence", `> after/readiness.json`)

	requireCount("flat graph summary query", "graphSummary { total_entities }", 2)
	requireCount("exact-zero flat graph summary", ".data.graphSummary.total_entities == 0", 2)
	if strings.Contains(runbook, "total_entities > 0") || strings.Contains(runbook, "total_entities >= 0") {
		violations = append(violations, "graph summary permits a non-zero or unconstrained result")
	}

	requireCount("catalog backing-stream inventory", "nats --context semdragon-dev stream info KV_ENTITY_STATES --json", 2)
	requireCount("exact-zero catalog stream", ".state.messages == 0", 2)
	requireContains("before catalog stream evidence", "before/entity-states-stream.normalized.json")
	requireContains("after catalog stream evidence", "after/entity-states-stream.normalized.json")
	if strings.Contains(runbook, "nats --context semdragon-dev kv ls ENTITY_STATES") {
		violations = append(violations, "empty-bucket key listing is used instead of authoritative stream state")
	}

	requireCount("unauthenticated lifecycle rejection", "--write-out '%{http_code}' http://localhost:8081/lifecycle-gateway/workflows", 2)
	requireCount("401 lifecycle assertion", `= "401"`, 2)
	requireCount("authenticated workflow discovery", `--header "Authorization: Bearer $CUTOVER_API_KEY"`, 2)
	requireCount("exact quest workflow inventory", `.[0].workflow == "quest"`, 2)

	for _, forbidden := range []string{
		"graph.mutation",
		"graph-ingest.mutation",
		"/lifecycle-gateway/workflows/quest/missing/transition",
		"/lifecycle-gateway/workflows/quest/missing/state",
	} {
		if strings.Contains(runbook, forbidden) {
			label := "raw graph mutation workaround"
			if strings.Contains(forbidden, "lifecycle-gateway") {
				label = "lifecycle mutation before #814"
			}
			violations = append(violations, label+" is present: "+forbidden)
		}
	}

	requireContains("SemStreams #814 link", semStreams814URL)
	requireContains("zero-state-only result label", "zero-state preflight passed")
	requireContains("semantic replay blocker", "Semantic replay remains blocked")
	requireContains("issue #59 remains incomplete", "Issue #59 and semantic replay remain incomplete pending SemStreams #814")
	requireContains("no false semantic acceptance", "complete zero-state record is still not successful semantic replay")
	requireContains("no legacy workaround", "No legacy product seed or raw mutation")
	if strings.Contains(runbook, "Report the final environment state as pass") || strings.Contains(runbook, "semantic replay passed") {
		violations = append(violations, "zero-state smoke is mislabeled as semantic acceptance")
	}

	for _, parityFile := range []string{
		"entities.normalized.json",
		"relationships.normalized.json",
		"quest-status.normalized.json",
		"dag-projection.normalized.json",
		"entity-states-stream.normalized.json",
		"graph-summary.normalized.json",
		"workflows.normalized.json",
	} {
		requireCount("zero-state parity for "+parityFile, "diff -u before/"+parityFile+" after/"+parityFile, 1)
	}

	return violations
}

func deletionCommand() string {
	return "docker volume rm -- " + graphVolume
}

func initialStartCommand() string {
	return `SEMDRAGONS_API_KEY="$CUTOVER_API_KEY" SEED_E2E=false SEED_AGENTS=false SEED_PROS=false docker compose --project-name semdragon-cutover -f docker/compose.yml up -d --no-build --pull never backend nats`
}

func restartCommand() string {
	return `SEMDRAGONS_API_KEY="$CUTOVER_API_KEY" SEED_E2E=false SEED_AGENTS=false SEED_PROS=false docker compose --project-name semdragon-cutover -f docker/compose.yml up -d --force-recreate --no-build --pull never backend nats`
}

func readinessProducerSet() string {
	return `["graph-index", "graph-ingest", "rule"]`
}

func assertNoRunbookViolations(t *testing.T, violations []string) {
	t.Helper()
	if len(violations) > 0 {
		t.Fatalf("runbook contract violations:\n- %s", strings.Join(violations, "\n- "))
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
