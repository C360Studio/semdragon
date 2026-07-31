package semdragons_test

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var removedSeedingFiles = []string{
	"processor/seeding/component.go",
	"processor/seeding/config.go",
	"processor/seeding/handler.go",
	"processor/seeding/payloads.go",
	"processor/seeding/register.go",
}

var removedSeedingDTOs = []string{
	"StartedPayload",
	"CompletedPayload",
	"AgentSeededPayload",
	"GuildSeededPayload",
}

var removedSeedingSubjects = []string{
	"SubjectSeedingStarted",
	"SubjectSeedingCompleted",
	"SubjectAgentSeeded",
	"SubjectGuildSeeded",
}

var removedSeedingPredicates = []string{
	"PredicateSeedingStarted",
	"PredicateSeedingCompleted",
	"PredicateArenaRoundStarted",
	"PredicateArenaRoundCompleted",
	"PredicateRosterAgentCreated",
	"PredicateNPCSpawned",
	"PredicateNPCRetired",
}

type historicalSeedingLiteral struct {
	value               string
	authoritativeOwners []string
}

// historicalSeedingTripleLiterals records every predicate occurrence emitted
// by the four deleted payload Triples methods. Empty owners mean the literal
// was seeding-only; shared predicates remain valid only in their authoritative
// Agent or Guild owners.
var historicalSeedingTripleLiterals = []historicalSeedingLiteral{
	{value: "seeding.lifecycle.started"},
	{value: "seeding.mode"},
	{value: "seeding.lifecycle.completed"},
	{value: "seeding.stats.agents_created"},
	{value: "seeding.stats.guilds_created"},
	{value: "agent.identity.name", authoritativeOwners: []string{"processor/agentprogression/"}},
	{value: "agent.progression.level", authoritativeOwners: []string{"processor/agentprogression/"}},
	{value: "agent.progression.tier", authoritativeOwners: []string{"processor/agentprogression/"}},
	{value: "agent.skills.has", authoritativeOwners: []string{"processor/agentprogression/"}},
	{value: "agent.guild.member"},
	{value: "guild.identity.name", authoritativeOwners: []string{"domain/"}},
	{value: "guild.status.state", authoritativeOwners: []string{"domain/"}},
	{value: "guild.specialization"},
}

var historicalSeedingSubjectLiterals = []string{
	"seeding.lifecycle.started",
	"seeding.lifecycle.completed",
	"seeding.agent.created",
	"seeding.guild.created",
}

var removedSeedingVocabularyValues = []string{
	"seeding.lifecycle.started",
	"seeding.lifecycle.completed",
	"seeding.arena.roundstarted",
	"seeding.arena.roundcompleted",
	"seeding.roster.agentcreated",
	"seeding.npc.spawned",
	"seeding.npc.retired",
}

func TestSeedingComponentIsFullyRemovedWhileStartupSeedingRemains(t *testing.T) {
	root := repositoryRoot(t)
	if len(historicalSeedingTripleLiterals) != 13 {
		t.Fatalf("historical seeding triple literal inventory = %d, want exactly 13", len(historicalSeedingTripleLiterals))
	}
	for _, path := range removedSeedingFiles {
		if _, err := os.Stat(filepath.Join(root, path)); err == nil {
			t.Errorf("legacy seeding package file still exists: %s", path)
		} else if !os.IsNotExist(err) {
			t.Errorf("stat legacy seeding package file %s: %v", path, err)
		}
	}

	scopes := []sourceInventoryScope{
		{path: "processor", recursive: true, extensions: extensions(".go")},
		{path: "componentregistry", recursive: true, extensions: extensions(".go")},
		{path: "domain", recursive: true, extensions: extensions(".go")},
		{path: "cmd/semdragons", recursive: true, extensions: extensions(".go")},
		{path: "config", recursive: true, extensions: extensions(".json")},
	}
	files := make(map[string][]byte)
	for _, scope := range scopes {
		if count := collectInventoryScope(t, root, scope, files); count == 0 {
			t.Errorf("seeding removal inventory scope %q contributed no files", scope.path)
		}
	}
	const relationshipFixture = "graph_relationship_contract_test.go"
	fixture, err := os.ReadFile(filepath.Join(root, relationshipFixture))
	if err != nil {
		t.Fatalf("read graph relationship fixture: %v", err)
	}
	files[relationshipFixture] = fixture

	for _, anchor := range []string{
		"componentregistry/register.go",
		"domain/vocab.go",
		"cmd/semdragons/main.go",
		"cmd/semdragons/seed_e2e.go",
		"processor/agentprogression/agent.go",
		relationshipFixture,
	} {
		if _, ok := files[anchor]; !ok {
			t.Errorf("seeding removal inventory is missing required anchor %q", anchor)
		}
	}

	for _, path := range removedSeedingFiles {
		data, ok := files[path]
		if !ok {
			continue
		}
		for index, dto := range removedSeedingDTOs {
			if bytes.Contains(data, []byte("graph.Graphable = (*"+dto)) {
				t.Errorf("%s retains Graphable assertion %d/4 for %s", path, index+1, dto)
			}
			for methodIndex, method := range []string{"EntityID", "Triples", "Schema"} {
				if seedingMethodExists(data, dto, method) {
					t.Errorf(
						"%s retains graph method %d/12: %s.%s",
						path,
						index*3+methodIndex+1,
						dto,
						method,
					)
				}
			}
			if bytes.Contains(data, []byte("type "+dto+" struct")) ||
				bytes.Contains(data, []byte(removedSeedingSubjects[index])) ||
				bytes.Contains(data, []byte(removedSeedingSubjects[index]+".Publish")) {
				t.Errorf(
					"%s retains DTO/subject/Publish family %d/4: %s, %s",
					path,
					index+1,
					dto,
					removedSeedingSubjects[index],
				)
			}
		}
	}

	for index, dto := range removedSeedingDTOs {
		fragment := "seeding." + dto
		if bytes.Contains(files[relationshipFixture], []byte(fragment)) {
			t.Errorf(
				"%s retains relationship fixture %d/4 for %s",
				relationshipFixture,
				index+1,
				dto,
			)
		}
	}

	vocab := files["domain/vocab.go"]
	for index, predicate := range removedSeedingPredicates {
		if bytes.Contains(vocab, []byte(predicate+" =")) {
			t.Errorf("domain/vocab.go retains seeding predicate constant %d/7: %s", index+1, predicate)
		}
		if bytes.Contains(vocab, []byte("vocabulary.Register("+predicate)) {
			t.Errorf("domain/vocab.go retains seeding predicate registration %d/7: %s", index+1, predicate)
		}
	}

	registryData := files["componentregistry/register.go"]
	if count := seedingComponentRegistryReferenceCount(t, registryData); count != 0 {
		t.Errorf("componentregistry/register.go seeding references = %d, want 0 (legacy inventory is exactly 5)", count)
	}
	for _, function := range []string{"RegisterAll", "RegisterProcessors", "ComponentNames", "ProcessorNames"} {
		count, exists := seedingFunctionReferenceCount(registryData, function)
		if !exists {
			t.Errorf("componentregistry/register.go is missing stale function anchor %s", function)
		} else if count != 0 {
			t.Errorf(
				"componentregistry/register.go %s seeding selector references = %d, want 0",
				function,
				count,
			)
		}
	}

	assertNoSeedingComponentInShippedConfigs(t, files)
	assertRetainedStartupSeedingAnchors(t, files)

	for path, data := range files {
		for _, violation := range legacySeedingViolationDescriptions(path, data) {
			t.Errorf("%s contains forbidden legacy seeding surface: %s", path, violation)
		}
	}
}

func TestSeedingRemovalInventoryRejectsLegacyMutations(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		data        string
		description string
	}{
		{"package file", "processor/seeding/new.go", "package seeding", "legacy package file"},
		{"DTO reference", "cmd/semdragons/main.go", "seeding.AgentSeededPayload{}", "legacy DTO"},
		{"subject reference", "cmd/semdragons/main.go", "seeding.SubjectAgentSeeded", "legacy subject"},
		{
			"Graphable assertion",
			"processor/seeding/payloads.go",
			"package seeding\nvar _ graph.Graphable = (*StartedPayload)(nil)",
			"legacy Graphable assertion",
		},
		{
			"graph method",
			"processor/seeding/payloads.go",
			"package seeding\nfunc (event *CompletedPayload) Triples() []message.Triple { return nil }",
			"legacy graph method",
		},
		{
			"Publish family",
			"processor/seeding/handler.go",
			"package seeding\nfunc publish() { SubjectGuildSeeded.Publish(ctx, client, GuildSeededPayload{}) }",
			"legacy Publish family",
		},
		{"relationship fixture", "graph_relationship_contract_test.go", "&seeding.GuildSeededPayload{}", "legacy DTO"},
		{
			"component import",
			"componentregistry/register.go",
			`import "github.com/c360studio/semdragons/processor/seeding"`,
			"legacy package import",
		},
		{"component callback", "componentregistry/register.go", "seeding.Register", "legacy component registration"},
		{"component name", "componentregistry/register.go", "seeding.ComponentName", "legacy component name"},
		{
			"config component",
			"config/semdragons.json",
			`{"components":{"seeding":{"type":"processor","enabled":true}}}`,
			"legacy config component",
		},
		{
			"outside NewSubject",
			"processor/bootstrap/routes.go",
			`var seeded = natsclient.NewSubject[Event]("seeding.agent.created")`,
			"legacy subject literal",
		},
		{
			"outside Publish",
			"cmd/semdragons/bootstrap.go",
			`client.Publish(ctx, "seeding.guild.created", payload)`,
			"legacy subject literal",
		},
		{
			"outside subject alias",
			"processor/bootstrap/routes.go",
			`const bootstrapStarted = "seeding.lifecycle.started"`,
			"legacy subject literal",
		},
		{
			"outside subscriber",
			"processor/bootstrap/consumer.go",
			`client.Subscribe("seeding.>"); const route = "seeding.agent.created"`,
			"legacy subscriber",
		},
		{
			"outside payload route",
			"processor/bootstrap/routes.go",
			`const route = "seeding.agent.created"; payloadregistry.RegisterPayload()`,
			"legacy payload route",
		},
		{
			"outside BaseMessage",
			"processor/bootstrap/routes.go",
			`const route = "seeding.agent.created"; message.NewBaseMessage()`,
			"legacy BaseMessage",
		},
		{
			"outside graph ingest",
			"processor/bootstrap/routes.go",
			`const route = "seeding.agent.created"; const output = "graph.ingest.entity"`,
			"legacy graph-ingest route",
		},
		{
			"outside projection contract",
			"processor/bootstrap/projection.go",
			`const route = "seeding.agent.created"; var contract projection.Contract`,
			"legacy projection contract",
		},
		{
			"outside projection bind",
			"processor/bootstrap/projection.go",
			`const route = "seeding.agent.created"; projection.Bind()`,
			"legacy projection bind",
		},
		{
			"outside projection owner",
			"processor/bootstrap/projection.go",
			`const route = "seeding.agent.created"; var owner = projection.Owner`,
			"legacy projection owner",
		},
		{
			"outside projection heartbeat",
			"processor/bootstrap/projection.go",
			`const route = "seeding.agent.created"; projection.BindAndHeartbeat()`,
			"legacy projection heartbeat",
		},
		{
			"unique Agent membership predicate",
			"processor/bootstrap/routes.go",
			`const predicate = "agent.guild.member"`,
			"legacy historical literal",
		},
		{
			"unique Guild specialization predicate",
			"processor/bootstrap/routes.go",
			`const predicate = "guild.specialization"`,
			"legacy historical literal",
		},
		{
			"shared predicate outside Agent owner",
			"processor/bootstrap/routes.go",
			`const predicate = "agent.identity.name"`,
			"shared historical literal outside authoritative owner",
		},
	}
	for _, predicate := range removedSeedingPredicates {
		tests = append(tests, struct {
			name        string
			path        string
			data        string
			description string
		}{
			name:        predicate,
			path:        "domain/vocab.go",
			data:        "const " + predicate + ` = "seeding.reintroduced"`,
			description: "legacy vocabulary",
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := legacySeedingViolationDescriptions(tt.path, []byte(tt.data))
			if !containsSeedingViolation(violations, tt.description) {
				t.Errorf("guard did not reject %s mutation %q; violations: %v", tt.description, tt.data, violations)
			}
		})
	}

	registryMutationBodies := map[string]string{
		"RegisterAll":        "_ = seeding.Register",
		"RegisterProcessors": "_ = seeding.Register",
		"ComponentNames":     "_ = seeding.ComponentName",
		"ProcessorNames":     "_ = seeding.ComponentName",
	}
	for function, body := range registryMutationBodies {
		t.Run("registry "+function, func(t *testing.T) {
			source := syntheticComponentRegistry(function, body)
			if count, exists := seedingFunctionReferenceCount([]byte(source), function); !exists || count != 1 {
				t.Errorf("%s mutation count = %d, exists = %v, want exactly one rejected selector", function, count, exists)
			}
		})
	}
	cleanRegistry := syntheticComponentRegistry("", "")
	for function := range registryMutationBodies {
		if count, exists := seedingFunctionReferenceCount([]byte(cleanRegistry), function); !exists || count != 0 {
			t.Errorf("clean %s count = %d, exists = %v, want zero", function, count, exists)
		}
	}

	allowed := []struct {
		name string
		path string
		data string
	}{
		{"startup seeding", "cmd/semdragons/seed_e2e.go", "maybeSeedE2E(ctx, cfg, client); seedOneAgent(ctx, graph, board, name, spec, 0)"},
		{"Agent Graphable", "processor/agentprogression/agent.go", "func (a *Agent) Triples() []message.Triple { return nil }"},
		{
			"Agent predicates",
			"processor/agentprogression/agent.go",
			`"agent.identity.name"; "agent.progression.level"; "agent.progression.tier"; "agent.skills.has"`,
		},
		{
			"Agent predicate projection",
			"processor/agentprogression/projection.go",
			`const predicate = "agent.identity.name"; var contract projection.Contract; projection.Bind()`,
		},
		{"Guild Graphable", "domain/social.go", "func (g *Guild) Triples() []message.Triple { return nil }"},
		{"Guild predicates", "domain/graphable.go", `"guild.identity.name"; "guild.status.state"`},
		{
			"Guild predicate projection",
			"domain/graphable.go",
			`const predicate = "guild.status.state"; var contract projection.Contract; projection.Bind()`,
		},
		{"authoritative readers", "graphclient.go", "gc.GetAgent(ctx, id); gc.GetGuild(ctx, id)"},
		{"SemStreams lifecycle", "componentregistry/register.go", "lifecycle_transition; lifecycle_complete; lifecycle_fail"},
		{"SemStreams rules", "componentregistry/register.go", "rule.Register(registry)"},
		{"generic dispatch", "cmd/semdragons/main.go", "dispatch(event); handler(event)"},
		{"unrelated payload route", "processor/questboard/routes.go", "payloadregistry.RegisterPayload()"},
		{"unrelated projection", "processor/questboard/projection.go", "projection.BindAndHeartbeat(); projection.Owner"},
	}
	for _, tt := range allowed {
		t.Run("allow "+tt.name, func(t *testing.T) {
			if violations := legacySeedingViolationDescriptions(tt.path, []byte(tt.data)); len(violations) != 0 {
				t.Errorf("guard rejected allowed %s surface: %v", tt.name, violations)
			}
		})
	}
}

func syntheticComponentRegistry(mutatedFunction, body string) string {
	functions := []string{"RegisterAll", "RegisterProcessors", "ComponentNames", "ProcessorNames"}
	var source strings.Builder
	source.WriteString("package componentregistry\n")
	for _, function := range functions {
		source.WriteString("func ")
		source.WriteString(function)
		source.WriteString("() {")
		if function == mutatedFunction {
			source.WriteString(body)
		}
		source.WriteString("}\n")
	}
	return source.String()
}

func seedingMethodExists(data []byte, dto, method string) bool {
	file, err := parser.ParseFile(token.NewFileSet(), "seeding.go", data, 0)
	if err != nil {
		return false
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv == nil || function.Name.Name != method {
			continue
		}
		for _, receiver := range function.Recv.List {
			if seedingReceiverType(receiver.Type) == dto {
				return true
			}
		}
	}
	return false
}

func seedingGraphableAssertionExists(data []byte, dto string) bool {
	file, err := parser.ParseFile(token.NewFileSet(), "seeding.go", data, 0)
	if err != nil {
		return false
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		valueSpec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		selector, ok := valueSpec.Type.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Graphable" {
			return true
		}
		for _, value := range valueSpec.Values {
			call, ok := value.(*ast.CallExpr)
			if ok && seedingReceiverType(call.Fun) == dto {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func seedingReceiverType(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return seedingReceiverType(expression.X)
	case *ast.ParenExpr:
		return seedingReceiverType(expression.X)
	default:
		return ""
	}
}

func seedingComponentRegistryReferenceCount(t *testing.T, data []byte) int {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "register.go", data, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse component registry imports: %v", err)
	}
	count := 0
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) == "github.com/c360studio/semdragons/processor/seeding" {
			count++
		}
	}
	file, err = parser.ParseFile(token.NewFileSet(), "register.go", data, 0)
	if err != nil {
		t.Fatalf("parse component registry: %v", err)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if ok && pkg.Name == "seeding" &&
			(selector.Sel.Name == "Register" || selector.Sel.Name == "ComponentName") {
			count++
		}
		return true
	})
	return count
}

func seedingFunctionReferenceCount(data []byte, name string) (int, bool) {
	file, err := parser.ParseFile(token.NewFileSet(), "register.go", data, 0)
	if err != nil {
		return 0, false
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == name {
			count := 0
			ast.Inspect(function.Body, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := selector.X.(*ast.Ident)
				if ok && pkg.Name == "seeding" &&
					(selector.Sel.Name == "Register" || selector.Sel.Name == "ComponentName") {
					count++
				}
				return true
			})
			return count, true
		}
	}
	return 0, false
}

func assertNoSeedingComponentInShippedConfigs(t *testing.T, files map[string][]byte) {
	t.Helper()
	for _, path := range []string{
		"config/semdragons.json",
		"config/semdragons-semsource.json",
		"config/semdragons-e2e-tier1.json",
	} {
		t.Run(path, func(t *testing.T) {
			data, ok := files[path]
			if !ok {
				t.Fatalf("inventory is missing shipped config %s", path)
			}
			var config struct {
				Components map[string]json.RawMessage `json:"components"`
			}
			if err := json.Unmarshal(data, &config); err != nil {
				t.Fatalf("decode shipped config %s: %v", path, err)
			}
			if _, exists := config.Components["seeding"]; exists {
				t.Errorf("%s retains a seeding component entry", path)
			}
		})
	}
}

func assertRetainedStartupSeedingAnchors(t *testing.T, files map[string][]byte) {
	t.Helper()
	required := map[string][]string{
		"cmd/semdragons/seed_e2e.go": {
			"func maybeSeedE2E(",
			`getenv("SEED_PROS")`,
			`getenv("SEED_E2E")`,
			`getenv("SEED_AGENTS")`,
			"func seedOneAgent(",
			"semdragons.DisabledProjectionMutation(ctx, agent,",
		},
		"cmd/semdragons/main.go": {
			"maybeSeedE2E(ctx, cfg, natsClient)",
		},
		"processor/agentprogression/agent.go": {
			"func (a *Agent) EntityID() string",
			"func (a *Agent) Triples() []message.Triple",
			"func AgentFromEntityState(",
		},
	}
	for path, anchors := range required {
		data := files[path]
		for _, anchor := range anchors {
			if !bytes.Contains(data, []byte(anchor)) {
				t.Errorf("%s is missing retained startup/Agent anchor %q", path, anchor)
			}
		}
	}
}

func legacySeedingViolationDescriptions(path string, data []byte) []string {
	var violations []string
	isLegacyPackageFile := strings.HasPrefix(path, "processor/seeding/") && strings.HasSuffix(path, ".go")
	isProductionGo := strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")
	if isLegacyPackageFile {
		violations = append(violations, "legacy package file")
		for index, dto := range removedSeedingDTOs {
			if bytes.Contains(data, []byte("type "+dto+" struct")) {
				violations = append(violations, "legacy DTO definition "+dto)
			}
			if seedingGraphableAssertionExists(data, dto) {
				violations = append(violations, "legacy Graphable assertion "+dto)
			}
			for _, method := range []string{"EntityID", "Triples", "Schema"} {
				if seedingMethodExists(data, dto, method) {
					violations = append(violations, "legacy graph method "+dto+"."+method)
				}
			}
			if bytes.Contains(data, []byte(removedSeedingSubjects[index]+".Publish")) {
				violations = append(violations, "legacy Publish family "+removedSeedingSubjects[index])
			}
		}
	}
	if bytes.Contains(data, []byte("github.com/c360studio/semdragons/processor/seeding")) {
		violations = append(violations, "legacy package import")
	}
	for _, dto := range removedSeedingDTOs {
		if bytes.Contains(data, []byte("seeding."+dto)) {
			violations = append(violations, "legacy DTO "+dto)
		}
	}
	for _, subject := range removedSeedingSubjects {
		if bytes.Contains(data, []byte("seeding."+subject)) ||
			(isLegacyPackageFile && bytes.Contains(data, []byte(subject))) {
			violations = append(violations, "legacy subject "+subject)
		}
	}
	for _, predicate := range removedSeedingPredicates {
		if bytes.Contains(data, []byte(predicate)) {
			violations = append(violations, "legacy vocabulary "+predicate)
		}
	}
	if isProductionGo {
		for _, historical := range historicalSeedingTripleLiterals {
			if !containsExactSeedingLiteral(data, historical.value) {
				continue
			}
			if len(historical.authoritativeOwners) == 0 {
				violations = append(violations, "legacy historical literal "+historical.value)
			} else if !hasAuthoritativeSeedingLiteralOwner(path, historical.authoritativeOwners) {
				violations = append(
					violations,
					"shared historical literal outside authoritative owner "+historical.value,
				)
			}
		}
		for _, literal := range historicalSeedingSubjectLiterals {
			if containsExactSeedingLiteral(data, literal) {
				violations = append(violations, "legacy subject literal "+literal)
			}
		}
		for _, literal := range removedSeedingVocabularyValues {
			if containsExactSeedingLiteral(data, literal) {
				violations = append(violations, "legacy vocabulary value "+literal)
			}
		}
	}
	if isProductionGo && containsExactSeedingLiteral(data, "seeding.>") {
		violations = append(violations, "legacy subscriber")
	}
	if isProductionGo && hasLegacySeedingRouteMarker(path, data) {
		for description, fragment := range map[string]string{
			"legacy payload route":         "payloadregistry",
			"legacy RegisterPayload route": "RegisterPayload",
			"legacy BaseMessage":           "BaseMessage",
			"legacy graph-ingest route":    "graph.ingest.entity",
			"legacy projection contract":   "projection.Contract",
			"legacy projection bind":       "projection.Bind",
			"legacy projection owner":      "projection.Owner",
			"legacy projection heartbeat":  "BindAndHeartbeat",
		} {
			if bytes.Contains(data, []byte(fragment)) {
				violations = append(violations, description)
			}
		}
	}
	if bytes.Contains(data, []byte("seeding.Register")) {
		violations = append(violations, "legacy component registration")
	}
	if bytes.Contains(data, []byte("seeding.ComponentName")) {
		violations = append(violations, "legacy component name")
	}
	if strings.HasPrefix(path, "config/") {
		var config struct {
			Components map[string]json.RawMessage `json:"components"`
		}
		if json.Unmarshal(data, &config) == nil {
			if _, exists := config.Components["seeding"]; exists {
				violations = append(violations, "legacy config component")
			}
		}
	}
	return violations
}

func containsExactSeedingLiteral(data []byte, value string) bool {
	return bytes.Contains(data, []byte(`"`+value+`"`)) ||
		bytes.Contains(data, []byte("`"+value+"`"))
}

func hasAuthoritativeSeedingLiteralOwner(path string, owners []string) bool {
	for _, owner := range owners {
		if strings.HasPrefix(path, owner) {
			return true
		}
	}
	return false
}

func hasLegacySeedingRouteMarker(path string, data []byte) bool {
	if strings.HasPrefix(path, "processor/seeding/") {
		return true
	}
	for _, literal := range historicalSeedingSubjectLiterals {
		if containsExactSeedingLiteral(data, literal) {
			return true
		}
	}
	for _, literal := range removedSeedingVocabularyValues {
		if containsExactSeedingLiteral(data, literal) {
			return true
		}
	}
	for _, historical := range historicalSeedingTripleLiterals {
		if len(historical.authoritativeOwners) == 0 &&
			containsExactSeedingLiteral(data, historical.value) {
			return true
		}
	}
	for _, marker := range []string{
		"github.com/c360studio/semdragons/processor/seeding",
		"seeding.StartedPayload",
		"seeding.CompletedPayload",
		"seeding.AgentSeededPayload",
		"seeding.GuildSeededPayload",
		"seeding.SubjectSeedingStarted",
		"seeding.SubjectSeedingCompleted",
		"seeding.SubjectAgentSeeded",
		"seeding.SubjectGuildSeeded",
	} {
		if bytes.Contains(data, []byte(marker)) {
			return true
		}
	}
	return false
}

func containsSeedingViolation(violations []string, want string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, want) {
			return true
		}
	}
	return false
}
