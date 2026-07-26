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
	reviewedSemStreamsPin  = "github.com/c360studio/semstreams v1.0.0-beta.135"
	composeProject         = "semdragon-cutover"
	graphVolume            = "nats-data"
	engineGraphVolume      = composeProject + "_" + graphVolume
	developmentNATSURL     = "nats://localhost:4222"
	developmentNATSContext = "semdragon-dev"
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
	label      string
	file       string
	query      string
	normalizer string
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
					return strings.Replace(runbook, `test -z "$(`, `echo "$(`, 1)
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
					return swapRunbookLines(runbook, backendStopCommand(), writerProofCommand())
				},
				fragment: "immediately followed",
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
				fragment: "NATS context creation",
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
				fragment: "executable query+normalizer for before/entities.normalized.json",
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
		specs := evidenceContracts()
		for index, spec := range specs {
			spec := spec
			other := specs[(index+1)%len(specs)]
			appendMutation := func(name string, mutate func(string) string) {
				mutations = append(mutations, struct {
					name     string
					mutate   func(string) string
					fragment string
				}{
					name:     name,
					mutate:   mutate,
					fragment: "executable query+normalizer for before/" + spec.file,
				})
			}
			appendMutation("cross-label "+spec.label, func(runbook string) string {
				return replaceBeforeCapture(
					runbook,
					spec,
					crossLabelCaptureCommand("before", spec, other),
				)
			})
			appendMutation("error validation bypass "+spec.label, func(runbook string) string {
				command := strings.Replace(captureCommand("before", spec.file), ".errors == null", "true", 1)
				return replaceBeforeCapture(runbook, spec, command)
			})
			appendMutation("data validation bypass "+spec.label, func(runbook string) string {
				command := strings.Replace(
					captureCommand("before", spec.file),
					evidenceDataGuard(spec),
					"true",
					1,
				)
				return replaceBeforeCapture(runbook, spec, command)
			})
			appendMutation("silent empty fallback "+spec.label, func(runbook string) string {
				command := strings.Replace(
					captureCommand("before", spec.file),
					evidenceDataField(spec),
					"("+evidenceDataField(spec)+" // [])",
					1,
				)
				return replaceBeforeCapture(runbook, spec, command)
			})
		}
		relationship := specs[1]
		relationshipMutations := []struct {
			name string
			old  string
			new  string
		}{
			{name: "relationship slurp removed", old: "jq -sSe", new: "jq -Se"},
			{name: "relationship merge removed", old: "[.[].data.relationships[]]", new: ".[0].data.relationships"},
			{name: "relationship dedup removed", old: " | unique_by([.from,.predicate,.to])", new: ""},
		}
		for _, mutation := range relationshipMutations {
			mutation := mutation
			mutations = append(mutations, struct {
				name     string
				mutate   func(string) string
				fragment string
			}{
				name: mutation.name,
				mutate: func(runbook string) string {
					command := strings.Replace(
						captureCommand("before", relationship.file),
						mutation.old,
						mutation.new,
						1,
					)
					return replaceBeforeCapture(runbook, relationship, command)
				},
				fragment: "executable query+normalizer for before/" + relationship.file,
			})
		}

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
		"normalization allowlist":      "Normalization removes only observation timestamps, KV revisions, and readiness watermarks.",
		"preserved replay fields":      "entity IDs, predicates, object values, datatypes, cardinality, relationship targets, quest status, DAG dependencies, and DAG outputs",
		"retained evidence":            "Retain the evidence",
		"reported final state":         "Report the final environment state",
		"protected local SemSource":    "`nats-local-semsource-data`",
		"protected external SemSource": "`nats-semsource-data`",
		"protected embedding cache":    "`semembed-cache`",
		"protected workspaces":         "`workspaces`",
	}
	for label, fragment := range requiredFragments {
		if !strings.Contains(runbook, fragment) {
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
		if strings.Contains(lower, "docker compose") &&
			strings.Contains(lower, " down") &&
			(strings.Contains(lower, " -v") || strings.Contains(lower, " --volumes")) {
			violations = append(violations, "broad docker compose volume teardown is forbidden")
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
	if len(restartIndexes) != 1 {
		violations = append(
			violations,
			fmt.Sprintf("all-false restart count = %d, want 1; restart does not set every seed flag false", len(restartIndexes)),
		)
	}
	if strings.Count(runbook, writerProofCommand()) != 1 {
		violations = append(violations, "empty stopped-writer assertion is missing")
	}
	stopIndexes := exactCommandIndexes(commands, backendStopCommand())
	writerIndexes := exactCommandIndexes(commands, writerProofCommand())
	if len(stopIndexes) != 1 {
		violations = append(
			violations,
			fmt.Sprintf("backend stop count = %d, want 1", len(stopIndexes)),
		)
	}
	if len(stopIndexes) == 1 && len(writerIndexes) == 1 &&
		writerIndexes[0] != stopIndexes[0]+1 {
		violations = append(violations, "backend stop must be immediately followed by empty running-services assertion")
	}
	if strings.Count(runbook, natsContextSaveCommand()) != 1 ||
		strings.Count(runbook, natsContextSelectCommand()) != 1 {
		violations = append(violations, "NATS context creation and selection are incomplete")
	}
	if strings.Count(runbook, natsContextVerifyCommand()) != 1 {
		violations = append(violations, "NATS context server verification is missing")
	}
	if strings.Count(runbook, composeProjectVerifyCommand()) != 1 {
		violations = append(violations, "Compose project rendered identity verification is missing")
	}
	if strings.Count(runbook, engineVolumeVerifyCommand()) != 1 {
		violations = append(violations, "Compose project volume identity verification is missing")
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
			engineVolumeVerifyCommand(),
			natsContextSaveCommand(),
			natsContextSelectCommand(),
			natsContextVerifyCommand(),
			writerProofCommand(),
		}
		previous := -1
		for _, command := range preflightCommands {
			indexes := exactCommandIndexes(commands, command)
			if len(indexes) != 1 || indexes[0] <= previous || indexes[0] >= deletionIndexes[0] {
				violations = append(violations, "project, volume, context, and stopped-writer checks must precede deletion")
				break
			}
			previous = indexes[0]
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
		"Protected: `nats-local-semsource-data`, `nats-semsource-data`, `semembed-cache`, and `workspaces`.",
		fmt.Sprintf("Candidate buckets (%d): %s.", len(buckets), strings.Join(buckets, ", ")),
		composePrefix() + " config",
		composePrefix() + " config --volumes",
		composeProjectVerifyCommand(),
		engineVolumeVerifyCommand(),
		natsContextSaveCommand(),
		natsContextSelectCommand(),
		natsContextVerifyCommand(),
		"The operator reviews and copies the literal target. Any mismatch must abort before deletion.",
		backendStopCommand(),
		writerProofCommand(),
		deletionCommand(),
		"Seed canonical state exactly once.",
		seedEnabledCommand(),
		readinessCommand(),
		captureCommand("before", "entities.normalized.json"),
		captureCommand("before", "relationships.normalized.json"),
		captureCommand("before", "quest-status.normalized.json"),
		captureCommand("before", "dag-projection.normalized.json"),
		"No intervening writes are permitted.",
		"Now restart the same reviewed binary.",
		restartWithoutSeedsCommand(),
		readinessCommand(),
		captureCommand("after", "entities.normalized.json"),
		captureCommand("after", "relationships.normalized.json"),
		captureCommand("after", "quest-status.normalized.json"),
		captureCommand("after", "dag-projection.normalized.json"),
		"diff --no-index -- before/entities.normalized.json after/entities.normalized.json",
		"diff --no-index -- before/relationships.normalized.json after/relationships.normalized.json",
		"diff --no-index -- before/quest-status.normalized.json after/quest-status.normalized.json",
		"diff --no-index -- before/dag-projection.normalized.json after/dag-projection.normalized.json",
		"Normalization removes only observation timestamps, KV revisions, and readiness watermarks.",
		"Preserve entity IDs, predicates, object values, datatypes, cardinality, relationship targets, quest status, DAG dependencies, and DAG outputs.",
		"Retain the evidence. Report the final environment state.",
	}
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

func writerProofCommand() string {
	return `test -z "$(` + composePrefix() + ` ps --status running --services backend)"`
}

func backendStopCommand() string {
	return composePrefix() + " stop backend"
}

func deletionCommand() string {
	return "docker volume rm -- " + engineGraphVolume
}

func seedEnabledCommand() string {
	return "SEED_E2E=false SEED_AGENTS=true SEED_PROS=false " + composePrefix() + " up -d backend"
}

func restartWithoutSeedsCommand() string {
	return "SEED_E2E=false SEED_AGENTS=false SEED_PROS=false " +
		composePrefix() + " up -d --force-recreate backend"
}

func readinessCommand() string {
	return `until curl --fail --silent --show-error --request POST ` +
		`--header 'Content-Type: application/json' ` +
		`--data '{"query":"query CutoverReadiness { graphSummary { total_entities } }"}' ` +
		`http://localhost:8081/graph-gateway/graphql | ` +
		`jq -e '(.errors == null) and (.data.graphSummary.total_entities > 0)' >/dev/null; do sleep 2; done`
}

func captureCommand(phase, name string) string {
	for _, contract := range evidenceContracts() {
		if contract.file == name {
			return captureCommandForContract(phase, name, contract)
		}
	}
	return ""
}

func crossLabelCaptureCommand(phase string, target, source evidenceContract) string {
	return captureCommandForContract(phase, target.file, source)
}

func replaceBeforeCapture(runbook string, contract evidenceContract, replacement string) string {
	return strings.Replace(runbook, captureCommand("before", contract.file), replacement, 1)
}

func evidenceDataField(contract evidenceContract) string {
	if contract.label == "relationships" {
		return ".data.relationships"
	}
	return ".data.entitiesByPrefix"
}

func evidenceDataGuard(contract evidenceContract) string {
	return "(" + evidenceDataField(contract) + ` | type == "array")`
}

func captureCommandForContract(phase, outputFile string, contract evidenceContract) string {
	output := phase + "/" + outputFile
	if contract.label == "relationships" {
		return `jq -r '.[].id' ` + phase + `/entities.normalized.json | ` +
			`while IFS= read -r entity_id; do ` +
			`curl --fail --silent --show-error --request POST ` +
			`--header 'Content-Type: application/json' ` +
			`--data "$(jq -nc --arg entityId "$entity_id" --arg query '` + contract.query +
			`' '{query:$query,variables:{entityId:$entityId}}')" ` +
			`http://localhost:8081/graph-gateway/graphql; done | ` +
			`jq -sSe '` + contract.normalizer + `' > ` + output
	}
	return `curl --fail --silent --show-error --request POST ` +
		`--header 'Content-Type: application/json' ` +
		`--data '{"query":"` + strings.ReplaceAll(contract.query, `"`, `\"`) + `"}' ` +
		`http://localhost:8081/graph-gateway/graphql | jq -Se '` + contract.normalizer + `' > ` + output
}

func evidenceContracts() []evidenceContract {
	return []evidenceContract{
		{
			label:      "entities",
			file:       "entities.normalized.json",
			query:      `query EntitiesEvidence { entitiesByPrefix(prefix: "local.dev.game.board1", limit: 10000) { id triples { subject predicate object } } }`,
			normalizer: `if ((.errors == null) and (.data.entitiesByPrefix | type == "array")) then [.data.entitiesByPrefix[] | {id,triples:(.triples | sort_by(.subject,.predicate,.object))}] | sort_by(.id) else error("invalid entities GraphQL envelope") end`,
		},
		{
			label:      "relationships",
			file:       "relationships.normalized.json",
			query:      `query RelationshipsEvidence($entityId: String!) { relationships(entityId: $entityId) { from to predicate } }`,
			normalizer: `if all(.[]; ((.errors == null) and (.data.relationships | type == "array"))) then [.[].data.relationships[]] | unique_by([.from,.predicate,.to]) | sort_by(.from,.predicate,.to) else error("invalid relationships GraphQL envelope") end`,
		},
		{
			label:      "quest-status",
			file:       "quest-status.normalized.json",
			query:      `query QuestStatusEvidence { entitiesByPrefix(prefix: "local.dev.game.board1.quest", limit: 10000) { id triples { subject predicate object } } }`,
			normalizer: `if ((.errors == null) and (.data.entitiesByPrefix | type == "array")) then [.data.entitiesByPrefix[] as $entity | $entity.triples[] | select(.predicate == "quest.status.state") | {id:$entity.id,status:.object}] | sort_by(.id) else error("invalid quest-status GraphQL envelope") end`,
		},
		{
			label:      "dag-projection",
			file:       "dag-projection.normalized.json",
			query:      `query DAGProjectionEvidence { entitiesByPrefix(prefix: "local.dev.game.board1.dagunit", limit: 10000) { id triples { subject predicate object } } }`,
			normalizer: `if ((.errors == null) and (.data.entitiesByPrefix | type == "array")) then [.data.entitiesByPrefix[] as $entity | $entity.triples[] | select((.predicate | startswith("gateddag.unit.")) or (.predicate | startswith("questdag.unit."))) | {id:$entity.id,predicate,object}] | sort_by(.id,.predicate,.object) else error("invalid DAG projection GraphQL envelope") end`,
		},
	}
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
