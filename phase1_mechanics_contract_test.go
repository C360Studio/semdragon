package semdragons_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var shippedRuntimeProfiles = []string{
	"config/semdragons.json",
	"config/semdragons-semsource.json",
	"config/semdragons-e2e-tier1.json",
}

var completeQuestRulePack = []string{
	"config/rules/quest/01-claim-start.json",
	"config/rules/quest/02-submit.json",
	"config/rules/quest/03-fail.json",
	"config/rules/quest/04-mechanics-stub.json",
}

type runtimeProfileContract struct {
	Components map[string]runtimeComponentContract `json:"components"`
}

type runtimeComponentContract struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Config  struct {
		RulesFiles          []string `json:"rules_files"`
		EntityWatchPatterns []string `json:"entity_watch_patterns"`
	} `json:"config"`
}

func TestPhase1MechanicsDisabledInEveryShippedProfile(t *testing.T) {
	for _, path := range shippedRuntimeProfiles {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			var profile runtimeProfileContract
			if err := json.Unmarshal(data, &profile); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}

			for _, violation := range runtimeProfileViolations(profile) {
				t.Error(violation)
			}
		})
	}
}

func TestPhase1AgentProgressionQuarantinedInEveryShippedProfile(t *testing.T) {
	for _, path := range shippedRuntimeProfiles {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			var profile runtimeProfileContract
			if err := json.Unmarshal(data, &profile); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}

			progression, exists := profile.Components["agent_progression"]
			if !exists {
				t.Fatal("components.agent_progression must remain registered but disabled for Phase 1")
			}
			if progression.Enabled {
				t.Error("components.agent_progression.enabled = true, want false")
			}
			for _, violation := range runtimeProfileViolations(profile) {
				t.Error(violation)
			}
		})
	}
}

var removedAgentProgressionProjectionSymbols = []string{
	"AgentXPPayload",
	"PredicateAgentXP",
	"PredicateAgentLevelUp",
	"PredicateAgentLevelDown",
	"PredicateAgentDeath",
	"PredicateAgentCooldown",
	"PredicateAgentReady",
}

var removedAgentProgressionProjectionLiterals = []string{
	"agent.progression.>",
	"AGENT_EVENTS",
	"agent-xp",
	"agent.progression.xp.delta",
	"agent.progression.xp.quest",
	"agent.progression.xp.awarded",
	"agent.progression.xp.penalty",
}

func TestPhase1AgentProgressionProjectionFacadeRemovedWhileSharedPolicyRemains(t *testing.T) {
	productionGo := readProductionGoFiles(t)
	for _, violation := range agentProgressionProjectionViolations(productionGo) {
		t.Error(violation)
	}

	if _, err := os.Stat("processor/agentprogression/payloads.go"); !os.IsNotExist(err) {
		t.Errorf("processor/agentprogression/payloads.go must be deleted; stat error = %v", err)
	}

	assertFileContains(t, "processor/agentprogression/agent.go",
		"type Agent struct",
		"func AgentFromEntityState(",
		`"agent.progression.xp.current"`,
	)
	assertFileContains(t, "processor/agentprogression/xp.go",
		"type XPEngine interface",
		"type XPAward struct",
		"type XPPenalty struct",
		"type FailureType string",
	)
	assertFileContains(t, "processor/agentprogression/register.go",
		`const ComponentName = "agent_progression"`,
		"func Register(",
	)
	assertFileExcludes(t, "processor/agentprogression/component.go",
		`Name:        "agent-xp"`,
		`StreamName:      "AGENT_EVENTS"`,
		`Subjects:        []string{"agent.progression.>"}`,
	)
	assertFileExcludes(t, "graph_relationship_contract_test.go", "AgentXPPayload")
}

func agentProgressionProjectionViolations(files map[string][]byte) []string {
	var violations []string
	for path, data := range files {
		for _, symbol := range removedAgentProgressionProjectionSymbols {
			if strings.Contains(string(data), symbol) {
				violations = append(violations,
					path+" retains removed agent progression projection symbol "+symbol)
			}
		}
		for _, literal := range removedAgentProgressionProjectionLiterals {
			if strings.Contains(string(data), `"`+literal+`"`) ||
				strings.Contains(string(data), "`"+literal+"`") {
				violations = append(violations,
					path+" retains removed agent progression projection literal "+literal)
			}
		}
	}
	return violations
}

func runtimeProfileViolations(profile runtimeProfileContract) []string {
	var violations []string
	if _, exists := profile.Components["bossbattle"]; exists {
		violations = append(violations,
			"components.bossbattle must be absent while Phase-1 mechanics are disabled")
	}

	rules, exists := profile.Components["rule-processor"]
	if !exists {
		return append(violations, "components.rule-processor is missing")
	}
	if rules.Type != "processor" {
		violations = append(violations, "components.rule-processor.type = "+rules.Type+", want processor")
	}
	if rules.Name != "rule-processor" {
		violations = append(violations,
			"components.rule-processor.name = "+rules.Name+", want rule-processor")
	}
	if !rules.Enabled {
		violations = append(violations, "components.rule-processor.enabled = false, want true")
	}
	if !slices.Equal(rules.Config.RulesFiles, completeQuestRulePack) {
		violations = append(violations, "rules_files does not equal the complete ordered quest pack")
	}
	if !slices.Equal(rules.Config.EntityWatchPatterns, []string{"*.*.game.*.quest.*"}) {
		violations = append(violations, "entity_watch_patterns does not equal the quest entity pattern")
	}
	return violations
}

var removedBattleRawSymbols = []string{
	"PredicateBattleStarted",
	"PredicateBattleVerdict",
	"PredicateBattleVictory",
	"PredicateBattleDefeat",
	"PredicateBattleRetreat",
	"EventBattleStarted",
	"EventBattleVictory",
	"EventBattleDefeat",
	"containsBattleEventTypes",
	"BattleRetreatPayload",
	"BattleRetreat",
}

var removedBattleRawLiterals = []string{
	"battle.review.started",
	"battle.review.verdict",
	"battle.review.victory",
	"battle.review.defeat",
	"battle.review.retreat",
	"battle.review.",
	"battle.review.>",
	"BATTLE_EVENTS",
}

var deletedRawBattleReviewEventLiterals = []string{
	"battle.review.started",
	"battle.review.verdict",
	"battle.review.victory",
	"battle.review.defeat",
	"battle.review.retreat",
}

func TestPhase1RawBattleSurfaceRemovedWhileAuthoritativeModelRemainsDormant(t *testing.T) {
	productionGo := readProductionGoFiles(t)

	for _, violation := range battleRawSurfaceViolations(productionGo) {
		t.Error(violation)
	}

	if _, err := os.Stat("processor/bossbattle/payloads.go"); !os.IsNotExist(err) {
		t.Errorf("processor/bossbattle/payloads.go must be deleted; stat error = %v", err)
	}

	assertFileContains(t, "processor/bossbattle/battle.go",
		"type BossBattle struct",
		`"battle.assignment.quest"`,
		`"battle.assignment.agent"`,
	)
	assertFileContains(t, "processor/bossbattle/evaluator.go", "type BattleEvaluator interface")
	assertFileContains(t, "processor/bossbattle/handler.go", "func (c *Component) processQuestWatchUpdates()")
	assertFileContains(t, "processor/bossbattle/config.go", "type Config struct")
	assertFileContains(t, "processor/bossbattle/register.go",
		`const ComponentName = "bossbattle"`,
		"func Register(",
	)
	assertFileContains(t, "processor/bossbattle/component.go",
		`Name:        "battle-state"`,
		"component.KVWritePort",
	)
	assertFileContains(t, "componentregistry/register.go",
		"bossbattle.Register",
		"bossbattle.ComponentName",
	)

	assertFileExcludes(t, "graph_relationship_contract_test.go", "BattleRetreatPayload")
	assertFileExcludes(t, "processor/bossbattle/handler_test.go", "BattleRetreatPayload")
	assertFileExcludes(t, "semsource/graphmanifest_test.go", `"battle.review.verdict"`)
	assertFileContains(t, "semsource/graphmanifest_test.go", `"battle.status.state"`)
	assertFileExcludes(t, "processor/dmworldstate/component_test.go", `"battle.review.started"`)
	assertFileContains(t, "processor/dmworldstate/component_test.go", `"battle.started"`)
	assertFileExcludes(t, "cmd/openapi-gen/enums.go", `"retreat"`)
}

func TestPhase1RawBattleEventsAbsentFromUIAndContributorVocabulary(t *testing.T) {
	targets := map[string]string{
		"UI event union":         "ui/src/lib/api/types.ts",
		"SSE event mapping":      "ui/src/lib/services/sse.ts",
		"predicate color test":   "ui/src/lib/utils/entity-colors.test.ts",
		"contributor vocabulary": "CLAUDE.md",
	}

	for name, path := range targets {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			for _, literal := range deletedRawBattleReviewEventLiterals {
				if strings.Contains(string(data), literal) {
					t.Errorf("%s retains deleted raw battle event literal %q", path, literal)
				}
			}
		})
	}
}

func battleRawSurfaceViolations(files map[string][]byte) []string {
	var violations []string
	for path, data := range files {
		for _, symbol := range removedBattleRawSymbols {
			if strings.Contains(string(data), symbol) {
				violations = append(violations, path+" retains removed raw battle symbol "+symbol)
			}
		}
		for _, literal := range removedBattleRawLiterals {
			if strings.Contains(string(data), `"`+literal+`"`) ||
				strings.Contains(string(data), "`"+literal+"`") {
				violations = append(violations, path+" retains removed raw battle literal "+literal)
			}
		}
	}
	return violations
}

func TestPhase1MechanicsContractsRejectReintroduction(t *testing.T) {
	validProfile := func() runtimeProfileContract {
		var rules runtimeComponentContract
		rules.Type = "processor"
		rules.Name = "rule-processor"
		rules.Enabled = true
		rules.Config.RulesFiles = slices.Clone(completeQuestRulePack)
		rules.Config.EntityWatchPatterns = []string{"*.*.game.*.quest.*"}
		return runtimeProfileContract{
			Components: map[string]runtimeComponentContract{"rule-processor": rules},
		}
	}

	profileMutations := map[string]func(*runtimeProfileContract){
		"bossbattle component": func(profile *runtimeProfileContract) {
			profile.Components["bossbattle"] = runtimeComponentContract{Enabled: true}
		},
		"disabled rule processor": func(profile *runtimeProfileContract) {
			rules := profile.Components["rule-processor"]
			rules.Enabled = false
			profile.Components["rule-processor"] = rules
		},
		"wrong rule processor type": func(profile *runtimeProfileContract) {
			rules := profile.Components["rule-processor"]
			rules.Type = "gateway"
			profile.Components["rule-processor"] = rules
		},
		"wrong rule processor name": func(profile *runtimeProfileContract) {
			rules := profile.Components["rule-processor"]
			rules.Name = "rules"
			profile.Components["rule-processor"] = rules
		},
		"missing mechanics stub": func(profile *runtimeProfileContract) {
			rules := profile.Components["rule-processor"]
			rules.Config.RulesFiles = rules.Config.RulesFiles[:3]
			profile.Components["rule-processor"] = rules
		},
		"extra rule": func(profile *runtimeProfileContract) {
			rules := profile.Components["rule-processor"]
			rules.Config.RulesFiles = append(rules.Config.RulesFiles, "config/rules/quest/05-battle.json")
			profile.Components["rule-processor"] = rules
		},
		"wrong watch pattern": func(profile *runtimeProfileContract) {
			rules := profile.Components["rule-processor"]
			rules.Config.EntityWatchPatterns = []string{"*.*.game.*.battle.*"}
			profile.Components["rule-processor"] = rules
		},
	}
	for name, mutate := range profileMutations {
		t.Run(name, func(t *testing.T) {
			profile := validProfile()
			mutate(&profile)
			if violations := runtimeProfileViolations(profile); len(violations) == 0 {
				t.Fatal("synthetic profile mutation escaped the Phase-1 config contract")
			}
		})
	}

	for _, symbol := range removedBattleRawSymbols {
		t.Run("symbol/"+symbol, func(t *testing.T) {
			files := map[string][]byte{"processor/reintroduced.go": []byte("package processor\nvar _ = " + symbol)}
			if violations := battleRawSurfaceViolations(files); len(violations) == 0 {
				t.Fatalf("synthetic raw symbol %q escaped the inventory contract", symbol)
			}
		})
	}
	for _, literal := range removedBattleRawLiterals {
		t.Run("literal/"+literal, func(t *testing.T) {
			files := map[string][]byte{
				"processor/reintroduced.go": []byte("package processor\nconst route = `" + literal + "`"),
			}
			if violations := battleRawSurfaceViolations(files); len(violations) == 0 {
				t.Fatalf("synthetic raw literal %q escaped the inventory contract", literal)
			}
		})
	}

	authoritative := map[string][]byte{
		"processor/bossbattle/battle.go": []byte(
			"package bossbattle\nconst level = `battle.review.level`\n" +
				"const quest = `battle.assignment.quest`\n"),
	}
	if violations := battleRawSurfaceViolations(authoritative); len(violations) != 0 {
		t.Fatalf("authoritative BossBattle predicates were rejected: %v", violations)
	}
}

func readProductionGoFiles(t *testing.T) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != "." && (strings.HasPrefix(entry.Name(), ".") ||
				path == "ui" || path == "openspec") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		files[filepath.ToSlash(path)] = data
		return nil
	})
	if err != nil {
		t.Fatalf("walk production Go: %v", err)
	}
	return files
}

func assertFileContains(t *testing.T, path string, needles ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, needle := range needles {
		if !strings.Contains(string(data), needle) {
			t.Errorf("%s missing authoritative anchor %q", path, needle)
		}
	}
}

func assertFileExcludes(t *testing.T, path string, needles ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, needle := range needles {
		if strings.Contains(string(data), needle) {
			t.Errorf("%s retains removed raw surface %q", path, needle)
		}
	}
}
