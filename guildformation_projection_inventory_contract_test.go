package semdragons_test

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var guildEventDTOFragments = []string{
	strings.Join([]string{"Guild", "Created", "Payload"}, ""),
	strings.Join([]string{"Guild", "Joined", "Payload"}, ""),
	strings.Join([]string{"Guild", "Left", "Payload"}, ""),
	strings.Join([]string{"Guild", "Promoted", "Payload"}, ""),
	strings.Join([]string{"Guild", "Disbanded", "Payload"}, ""),
}

var shippedGuildFormationConfigs = []string{
	"config/semdragons.json",
	"config/semdragons-semsource.json",
	"config/semdragons-e2e-tier1.json",
}

func TestGuildFormationRawDTOsHaveNoGraphProjectionSurface(t *testing.T) {
	root := repositoryRoot(t)
	scopes := []sourceInventoryScope{
		{path: "processor/guildformation", recursive: true, extensions: extensions(".go")},
		{path: "cmd/semdragons", recursive: true, extensions: extensions(".go")},
		{path: "componentregistry", recursive: true, extensions: extensions(".go")},
		{path: "config", recursive: true, extensions: extensions(".json", ".yaml", ".yml", ".toml")},
	}
	files := make(map[string][]byte)
	for _, scope := range scopes {
		if count := collectInventoryScope(t, root, scope, files); count == 0 {
			t.Errorf("guildformation inventory scope %q contributed no files", scope.path)
		}
	}

	const relationshipFixture = "graph_relationship_contract_test.go"
	fixture, err := os.ReadFile(filepath.Join(root, relationshipFixture))
	if err != nil {
		t.Fatalf("read guild relationship fixture: %v", err)
	}
	files[relationshipFixture] = fixture

	for _, anchor := range []string{
		"processor/guildformation/payloads.go",
		"processor/guildformation/handler.go",
		"processor/guildformation/handler_test.go",
		"processor/guildformation/component_test.go",
		"cmd/semdragons/main.go",
		"componentregistry/register.go",
		relationshipFixture,
	} {
		if _, ok := files[anchor]; !ok {
			t.Errorf("guildformation inventory is missing required anchor %q", anchor)
		}
	}
	assertGuildFormationEnabledInShippedConfigs(t, files)
	for _, violation := range guildFormationRegistrationViolations(files["componentregistry/register.go"]) {
		t.Errorf("componentregistry/register.go contains guildformation activation violation: %s", violation)
	}

	required := map[string][]string{
		"processor/guildformation/payloads.go": {
			strings.Join([]string{"Subject", "Guild", "Created"}, ""),
			strings.Join([]string{"Subject", "Guild", "Joined"}, ""),
			strings.Join([]string{"Subject", "Guild", "Left"}, ""),
			strings.Join([]string{"Subject", "Guild", "Promoted"}, ""),
			strings.Join([]string{"Subject", "Guild", "Disbanded"}, ""),
			strings.Join([]string{"func (p *", guildEventDTOFragments[0], ") Validate"}, ""),
			strings.Join([]string{"func (p *", guildEventDTOFragments[1], ") Validate"}, ""),
			strings.Join([]string{"func (p *", guildEventDTOFragments[2], ") Validate"}, ""),
			strings.Join([]string{"func (p *", guildEventDTOFragments[3], ") Validate"}, ""),
			strings.Join([]string{"func (p *", guildEventDTOFragments[4], ") Validate"}, ""),
		},
		"processor/guildformation/handler.go": {
			strings.Join([]string{"Subject", "Guild", "Created", ".", "Publish"}, ""),
			strings.Join([]string{"Subject", "Guild", "Joined", ".", "Publish"}, ""),
			strings.Join([]string{"Subject", "Guild", "Left", ".", "Publish"}, ""),
			strings.Join([]string{"Subject", "Guild", "Promoted", ".", "Publish"}, ""),
			strings.Join([]string{"Subject", "Guild", "Disbanded", ".", "Publish"}, ""),
		},
	}
	for path, anchors := range required {
		for _, anchor := range anchors {
			if count := bytes.Count(files[path], []byte(anchor)); count != 1 {
				t.Errorf("%s retained raw-event anchor %q count = %d, want exactly 1", path, anchor, count)
			}
		}
	}

	productionFiles := 0
	compositionFiles := 0
	for path, data := range files {
		for _, violation := range guildFormationDeletedSurfaceViolations(data) {
			t.Errorf("%s contains forbidden guildformation %s", path, violation)
		}
		if path == relationshipFixture {
			for _, dto := range guildEventDTOFragments {
				fragment := strings.Join([]string{"guildformation", ".", dto}, "")
				if bytes.Contains(data, []byte(fragment)) {
					t.Errorf("%s retains forbidden %s graph relationship fixture", path, dto)
				}
			}
		}
		if isGuildFormationProductionPath(path) {
			productionFiles++
		}
		if isGuildFormationCompositionPath(path) {
			compositionFiles++
		}
		for _, violation := range guildFormationHookViolations(path, data) {
			t.Errorf("%s contains forbidden guildformation %s", path, violation)
		}
	}
	if productionFiles == 0 {
		t.Error("guildformation hook inventory examined zero non-test production files")
	}
	if compositionFiles == 0 {
		t.Error("guildformation hook inventory examined zero composition/config files")
	}
}

func TestGuildFormationProjectionInventoryGuardRejectsHookMutations(t *testing.T) {
	tests := []struct {
		name string
		path string
		data string
	}{
		{"payload registry package", "processor/guildformation/register.go", strings.Join([]string{"payload", "registry.Register"}, "")},
		{"payload registry dependency", "processor/guildformation/register.go", strings.Join([]string{"deps.", "Payload", "Registry"}, "")},
		{"payload registry variable", "processor/guildformation/register.go", strings.Join([]string{"payload", "Registry.Register"}, "")},
		{"payload registry short variable", "processor/guildformation/register.go", strings.Join([]string{"payload", "Reg.Register"}, "")},
		{"payload registration function", "processor/guildformation/register.go", strings.Join([]string{"Register", "Payload"}, "")},
		{"projection contract", "processor/guildformation/projection.go", strings.Join([]string{"projection", ".", "Contract"}, "")},
		{"projection bind", "processor/guildformation/projection.go", strings.Join([]string{"projection", ".", "Bind"}, "")},
		{"projection heartbeat bind", "processor/guildformation/projection.go", strings.Join([]string{"projection", ".", "BindAndHeartbeat"}, "")},
		{"base message wrapping", "processor/guildformation/payloads.go", strings.Join([]string{"message.New", "Base", "Message"}, "")},
		{"graph ingest route", "processor/guildformation/payloads.go", strings.Join([]string{"graph", ".", "ingest", ".", "entity"}, "")},
		{"DTO composition registration", "cmd/semdragons/main.go", strings.Join([]string{"guildformation", ".", guildEventDTOFragments[0]}, "")},
		{"DTO component registration", "componentregistry/register.go", strings.Join([]string{"guildformation", ".", guildEventDTOFragments[4]}, "")},
		{"DTO config route", "config/semdragons.json", strings.Join([]string{"guildformation", ".", guildEventDTOFragments[2]}, "")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if violations := guildFormationHookViolations(tt.path, []byte(tt.data)); len(violations) == 0 {
				t.Errorf("guard accepted synthetic forbidden mutation %q in %s", tt.data, tt.path)
			}
		})
	}

	registrationMutations := []struct {
		name          string
		registerAll   bool
		registerProcs bool
		wantViolation bool
	}{
		{
			name:        "exact production registrations",
			registerAll: true, registerProcs: true,
		},
		{
			name:          "RegisterAll registration removed",
			registerProcs: true, wantViolation: true,
		},
		{
			name:        "RegisterProcessors registration removed",
			registerAll: true, wantViolation: true,
		},
		{
			name:          "both production registrations removed",
			wantViolation: true,
		},
	}
	for _, tt := range registrationMutations {
		t.Run(tt.name, func(t *testing.T) {
			source := syntheticGuildFormationRegistrations(tt.registerAll, tt.registerProcs)
			violations := guildFormationRegistrationViolations([]byte(source))
			if tt.wantViolation && len(violations) == 0 {
				t.Errorf("activation guard accepted missing registration(s):\n%s", source)
			}
			if !tt.wantViolation && len(violations) != 0 {
				t.Errorf("activation guard rejected exact production registrations: %v", violations)
			}
		})
	}

	for _, dto := range guildEventDTOFragments {
		for _, method := range []string{"Triples", "EntityID", "Schema"} {
			mutation := strings.Join([]string{"func Test", dto, "_", method, "(t *testing.T) {}"}, "")
			t.Run(dto+" "+method+" graph test", func(t *testing.T) {
				if violations := guildFormationDeletedSurfaceViolations([]byte(mutation)); len(violations) == 0 {
					t.Errorf("guard accepted deleted DTO graph-test mutation %q", mutation)
				}
			})
		}
	}

	declarationMutations := []struct {
		name string
		data string
	}{
		{
			"event pointer receiver",
			"func (event *GuildCreatedPayload) Triples() []message.Triple { return nil }",
		},
		{
			"payload value receiver",
			"func (payload GuildJoinedPayload) EntityID() string { return \"\" }",
		},
		{
			"unnamed pointer receiver",
			"func (*GuildLeftPayload) Schema() types.Type { return types.Type{} }",
		},
		{
			"named Graphable variable",
			"var promotedProjection graph.Graphable = (*GuildPromotedPayload)(nil)",
		},
		{
			"spaced Graphable assertion",
			"var _ graph . Graphable = ( * GuildCreatedPayload ) ( nil )",
		},
		{
			"multiline Graphable declaration",
			"var (\n\tdisbandedProjection graph.Graphable =\n\t\t(*GuildDisbandedPayload)(nil)\n)",
		},
	}
	for _, tt := range declarationMutations {
		t.Run(tt.name, func(t *testing.T) {
			if violations := guildFormationDeletedSurfaceViolations([]byte(tt.data)); len(violations) == 0 {
				t.Errorf("guard accepted deleted DTO declaration mutation %q", tt.data)
			}
		})
	}

	allowed := []struct {
		name string
		data string
	}{
		{"authoritative guild triples test", "func TestGuild_Triples(t *testing.T) {}"},
		{"authoritative guild entity ID test", "func TestGuild_EntityID(t *testing.T) {}"},
		{"authoritative guild schema test", "func TestGuild_Schema(t *testing.T) {}"},
		{"authoritative guild triples method", "func (guild *Guild) Triples() []message.Triple { return nil }"},
		{"authoritative guild entity ID method", "func (guild Guild) EntityID() string { return \"\" }"},
		{"authoritative guild schema method", "func (*Guild) Schema() types.Type { return types.Type{} }"},
		{"generic triple finder", "func findTriple() {}"},
	}
	for _, tt := range allowed {
		t.Run(tt.name, func(t *testing.T) {
			if violations := guildFormationDeletedSurfaceViolations([]byte(tt.data)); len(violations) != 0 {
				t.Errorf("guard rejected legitimate guild-state graph test/helper %q: %v", tt.data, violations)
			}
		})
	}

	for _, subject := range []string{"Created", "Joined", "Left", "Promoted", "Disbanded"} {
		rawConsumer := strings.Join([]string{"Subject", "Guild", subject, ".", "Subscribe"}, "")
		if violations := guildFormationHookViolations(
			"processor/guildformation/consumer.go",
			[]byte(rawConsumer),
		); len(violations) != 0 {
			t.Errorf("guard rejected ordinary raw %s typed-subject consumer: %v", subject, violations)
		}
	}
}

func syntheticGuildFormationRegistrations(registerAll, registerProcessors bool) string {
	registerAllBody := ""
	if registerAll {
		registerAllBody = "_ = guildformation.Register"
	}
	registerProcessorsBody := ""
	if registerProcessors {
		registerProcessorsBody = "_ = guildformation.Register"
	}
	return "package componentregistry\n" +
		"func RegisterAll() {" + registerAllBody + "}\n" +
		"func RegisterProcessors() {" + registerProcessorsBody + "}\n"
}

func assertGuildFormationEnabledInShippedConfigs(t *testing.T, files map[string][]byte) {
	t.Helper()
	type componentConfig struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	type applicationConfig struct {
		Components map[string]componentConfig `json:"components"`
	}

	for _, path := range shippedGuildFormationConfigs {
		t.Run(path, func(t *testing.T) {
			data, ok := files[path]
			if !ok {
				t.Fatalf("guildformation inventory is missing shipped config %q", path)
			}
			var config applicationConfig
			if err := json.Unmarshal(data, &config); err != nil {
				t.Fatalf("decode shipped config %q: %v", path, err)
			}
			component, ok := config.Components["guildformation"]
			if !ok {
				t.Fatalf("%s has no guildformation component", path)
			}
			if component.Type != "processor" {
				t.Errorf("%s guildformation type = %q, want processor", path, component.Type)
			}
			if component.Name != "guildformation" {
				t.Errorf("%s guildformation name = %q, want guildformation", path, component.Name)
			}
			if !component.Enabled {
				t.Errorf("%s guildformation component is disabled", path)
			}
		})
	}
}

func guildFormationRegistrationViolations(data []byte) []string {
	file := parseGuildFormationInventoryFile(data)
	if file == nil {
		return []string{"registration source does not parse as Go"}
	}

	counts := map[string]int{
		"RegisterAll":        -1,
		"RegisterProcessors": -1,
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if _, tracked := counts[function.Name.Name]; !tracked {
			continue
		}
		counts[function.Name.Name] = 0
		ast.Inspect(function.Body, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Register" {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if ok && pkg.Name == "guildformation" {
				counts[function.Name.Name]++
			}
			return true
		})
	}

	var violations []string
	for _, function := range []string{"RegisterAll", "RegisterProcessors"} {
		switch count := counts[function]; count {
		case -1:
			violations = append(violations, function+" declaration is missing")
		case 1:
			// Exactly one guildformation registration is required in each path.
		default:
			violations = append(
				violations,
				function+" guildformation.Register count is "+strconv.Itoa(count)+", want exactly 1",
			)
		}
	}
	return violations
}

func guildFormationDeletedSurfaceViolations(data []byte) []string {
	var violations []string
	file := parseGuildFormationInventoryFile(data)
	if file == nil {
		return violations
	}
	dtoNames := make(map[string]struct{}, len(guildEventDTOFragments))
	for _, dto := range guildEventDTOFragments {
		dtoNames[dto] = struct{}{}
	}
	methodNames := map[string]struct{}{"EntityID": {}, "Triples": {}, "Schema": {}}

	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if declaration.Recv != nil {
				for _, receiver := range declaration.Recv.List {
					dto := guildFormationReceiverType(receiver.Type)
					if _, isDeletedDTO := dtoNames[dto]; isDeletedDTO {
						if _, isGraphMethod := methodNames[declaration.Name.Name]; isGraphMethod {
							violations = append(
								violations,
								dto+" "+declaration.Name.Name+" method declaration",
							)
						}
					}
				}
			}
			for _, dto := range guildEventDTOFragments {
				for method := range methodNames {
					if declaration.Name.Name == "Test"+dto+"_"+method {
						violations = append(violations, dto+" "+method+" test declaration")
					}
				}
			}
		case *ast.GenDecl:
			if declaration.Tok != token.VAR {
				continue
			}
			for _, spec := range declaration.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok || !isGraphableType(valueSpec.Type) {
					continue
				}
				for _, value := range valueSpec.Values {
					dto := guildFormationGraphableValueType(value)
					if _, isDeletedDTO := dtoNames[dto]; isDeletedDTO {
						violations = append(violations, dto+" Graphable assertion")
					}
				}
			}
		}
	}
	return violations
}

func parseGuildFormationInventoryFile(data []byte) *ast.File {
	file, err := parser.ParseFile(token.NewFileSet(), "inventory.go", data, 0)
	if err == nil {
		return file
	}
	wrapped := append([]byte("package inventory\n"), data...)
	file, err = parser.ParseFile(token.NewFileSet(), "inventory.go", wrapped, 0)
	if err != nil {
		return nil
	}
	return file
}

func guildFormationReceiverType(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return guildFormationReceiverType(expression.X)
	case *ast.ParenExpr:
		return guildFormationReceiverType(expression.X)
	default:
		return ""
	}
}

func isGraphableType(expression ast.Expr) bool {
	switch expression := expression.(type) {
	case *ast.SelectorExpr:
		return expression.Sel.Name == "Graphable"
	case *ast.ParenExpr:
		return isGraphableType(expression.X)
	default:
		return false
	}
}

func guildFormationGraphableValueType(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.CallExpr:
		return guildFormationReceiverType(expression.Fun)
	case *ast.ParenExpr:
		return guildFormationGraphableValueType(expression.X)
	case *ast.UnaryExpr:
		if expression.Op == token.AND {
			return guildFormationGraphableValueType(expression.X)
		}
	case *ast.CompositeLit:
		return guildFormationReceiverType(expression.Type)
	}
	return ""
}

func guildFormationHookViolations(path string, data []byte) []string {
	var rules map[string]string
	switch {
	case isGuildFormationProductionPath(path):
		rules = map[string]string{
			"payload registry package reference": strings.Join([]string{"payload", "registry"}, ""),
			"payload registry dependency":        strings.Join([]string{"Payload", "Registry"}, ""),
			"payload registry variable":          strings.Join([]string{"payload", "Registry"}, ""),
			"payload registry short variable":    strings.Join([]string{"payload", "Reg"}, ""),
			"payload registration function":      strings.Join([]string{"Register", "Payload"}, ""),
			"projection contract":                strings.Join([]string{"projection", ".", "Contract"}, ""),
			"projection bind":                    strings.Join([]string{"projection", ".", "Bind"}, ""),
			"BaseMessage wrapping or route":      strings.Join([]string{"Base", "Message"}, ""),
			"graph-ingest entity route":          strings.Join([]string{"graph", ".", "ingest", ".", "entity"}, ""),
		}
	case isGuildFormationCompositionPath(path):
		rules = make(map[string]string, len(guildEventDTOFragments))
		for _, dto := range guildEventDTOFragments {
			rules[dto+" composition reference"] = strings.Join([]string{"guildformation", ".", dto}, "")
		}
	}

	var violations []string
	for description, fragment := range rules {
		if bytes.Contains(data, []byte(fragment)) {
			violations = append(violations, description+" "+fragment)
		}
	}
	return violations
}

func isGuildFormationProductionPath(path string) bool {
	return strings.HasPrefix(path, "processor/guildformation/") &&
		strings.HasSuffix(path, ".go") &&
		!strings.HasSuffix(path, "_test.go")
}

func isGuildFormationCompositionPath(path string) bool {
	return strings.HasPrefix(path, "cmd/semdragons/") ||
		strings.HasPrefix(path, "componentregistry/") ||
		strings.HasPrefix(path, "config/")
}
