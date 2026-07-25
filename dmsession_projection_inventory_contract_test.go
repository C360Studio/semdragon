package semdragons_test

import (
	"bytes"
	"strings"
	"testing"
)

func TestDMSessionRawDTOsHaveNoGraphProjectionSurface(t *testing.T) {
	root := repositoryRoot(t)
	scopes := []sourceInventoryScope{
		{path: "processor/dmsession", recursive: true, extensions: extensions(".go")},
		{path: "domain", recursive: true, extensions: extensions(".go")},
		{path: "cmd/semdragons", recursive: true, extensions: extensions(".go")},
		{path: "componentregistry", recursive: true, extensions: extensions(".go")},
		{path: "config", recursive: true, extensions: extensions(".json", ".yaml", ".yml", ".toml")},
	}
	files := make(map[string][]byte)
	for _, scope := range scopes {
		if count := collectInventoryScope(t, root, scope, files); count == 0 {
			t.Errorf("dmsession inventory scope %q contributed no files", scope.path)
		}
	}

	for _, anchor := range []string{
		"processor/dmsession/payloads.go",
		"processor/dmsession/session.go",
		"domain/vocab.go",
		"cmd/semdragons/main.go",
		"componentregistry/register.go",
		"config/semdragons.json",
	} {
		if _, ok := files[anchor]; !ok {
			t.Errorf("dmsession inventory is missing required anchor %q", anchor)
		}
	}

	required := map[string][]string{
		"processor/dmsession/payloads.go": {
			strings.Join([]string{"Subject", "Session", "Start"}, ""),
			strings.Join([]string{"Subject", "Session", "End"}, ""),
			strings.Join([]string{"func (p *", "SessionStartPayload", ") Validate"}, ""),
			strings.Join([]string{"func (p *", "SessionEndPayload", ") Validate"}, ""),
		},
		"processor/dmsession/session.go": {
			strings.Join([]string{"Subject", "Session", "Start", ".", "Publish"}, ""),
			strings.Join([]string{"Subject", "Session", "End", ".", "Publish"}, ""),
		},
	}
	for path, anchors := range required {
		for _, anchor := range anchors {
			if count := bytes.Count(files[path], []byte(anchor)); count != 1 {
				t.Errorf("%s retained raw-event anchor %q count = %d, want exactly 1", path, anchor, count)
			}
		}
	}

	forbidden := map[string]string{
		"start DTO EntityID method": strings.Join([]string{"func (p *", "SessionStartPayload", ") EntityID"}, ""),
		"start DTO Triples method":  strings.Join([]string{"func (p *", "SessionStartPayload", ") Triples"}, ""),
		"start DTO Schema method":   strings.Join([]string{"func (p *", "SessionStartPayload", ") Schema"}, ""),
		"end DTO EntityID method":   strings.Join([]string{"func (p *", "SessionEndPayload", ") EntityID"}, ""),
		"end DTO Triples method":    strings.Join([]string{"func (p *", "SessionEndPayload", ") Triples"}, ""),
		"end DTO Schema method":     strings.Join([]string{"func (p *", "SessionEndPayload", ") Schema"}, ""),
		"start Graphable assertion": strings.Join([]string{"graph", ".", "Graphable = (*", "SessionStartPayload"}, ""),
		"end Graphable assertion":   strings.Join([]string{"graph", ".", "Graphable = (*", "SessionEndPayload"}, ""),
		"latent mode predicate":     strings.Join([]string{"dm", ".", "session", ".", "mode"}, ""),
		"latent name predicate":     strings.Join([]string{"dm", ".", "session", ".", "name"}, ""),
		"latent completed predicate": strings.Join(
			[]string{"dm", ".", "session", ".", "quests_", "completed"},
			"",
		),
		"latent failed predicate": strings.Join(
			[]string{"dm", ".", "session", ".", "quests_", "failed"},
			"",
		),
		"external payload registration": strings.Join(
			[]string{"dmsession", ".", "Register", "Payloads"},
			"",
		),
		"external projection registration": strings.Join(
			[]string{"dmsession", ".", "Projection", "Bindings"},
			"",
		),
	}
	productionFiles := 0
	compositionFiles := 0
	for path, data := range files {
		for description, fragment := range forbidden {
			if bytes.Contains(data, []byte(fragment)) {
				t.Errorf("%s contains forbidden %s %q", path, description, fragment)
			}
		}
		if isDMSessionProductionPath(path) {
			productionFiles++
		}
		if isDMSessionCompositionPath(path) {
			compositionFiles++
		}
		for _, violation := range dmsessionHookViolations(path, data) {
			t.Errorf("%s contains forbidden dmsession %s", path, violation)
		}
	}
	if productionFiles == 0 {
		t.Error("dmsession hook inventory examined zero non-test production files")
	}
	if compositionFiles == 0 {
		t.Error("dmsession hook inventory examined zero composition/config files")
	}
}

func TestDMSessionProjectionInventoryGuardRejectsHookMutations(t *testing.T) {
	tests := []struct {
		name string
		path string
		data string
	}{
		{"payload registry package", "processor/dmsession/register.go", strings.Join([]string{"payload", "registry.Register"}, "")},
		{"payload registry dependency", "processor/dmsession/register.go", strings.Join([]string{"deps.", "Payload", "Registry"}, "")},
		{"payload registry variable", "processor/dmsession/register.go", strings.Join([]string{"payload", "Registry.Register"}, "")},
		{"payload registry short variable", "processor/dmsession/register.go", strings.Join([]string{"payload", "Reg.Register"}, "")},
		{"payload registration function", "processor/dmsession/register.go", strings.Join([]string{"Register", "Payload"}, "")},
		{"projection contract", "processor/dmsession/projection.go", strings.Join([]string{"projection", ".", "Contract"}, "")},
		{"projection bind", "processor/dmsession/projection.go", strings.Join([]string{"projection", ".", "Bind"}, "")},
		{"projection heartbeat bind", "processor/dmsession/projection.go", strings.Join([]string{"projection", ".", "BindAndHeartbeat"}, "")},
		{"base message wrapping", "processor/dmsession/payloads.go", strings.Join([]string{"message.New", "Base", "Message"}, "")},
		{"graph ingest route", "processor/dmsession/payloads.go", strings.Join([]string{"graph", ".", "ingest", ".", "entity"}, "")},
		{"start DTO composition registration", "cmd/semdragons/main.go", strings.Join([]string{"dmsession", ".", "SessionStartPayload"}, "")},
		{"end DTO component registration", "componentregistry/register.go", strings.Join([]string{"dmsession", ".", "SessionEndPayload"}, "")},
		{"start DTO config route", "config/semdragons.json", strings.Join([]string{"dmsession", ".", "SessionStartPayload"}, "")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if violations := dmsessionHookViolations(tt.path, []byte(tt.data)); len(violations) == 0 {
				t.Errorf("guard accepted synthetic forbidden mutation %q in %s", tt.data, tt.path)
			}
		})
	}

	rawConsumer := strings.Join([]string{"Subject", "Session", "Start", ".", "Subscribe"}, "")
	if violations := dmsessionHookViolations(
		"processor/dmsession/consumer.go",
		[]byte(rawConsumer),
	); len(violations) != 0 {
		t.Errorf("guard rejected ordinary raw typed-subject consumer: %v", violations)
	}
}

func dmsessionHookViolations(path string, data []byte) []string {
	var rules map[string]string
	switch {
	case isDMSessionProductionPath(path):
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
	case isDMSessionCompositionPath(path):
		rules = map[string]string{
			"SessionStartPayload composition reference": strings.Join(
				[]string{"dmsession", ".", "SessionStartPayload"},
				"",
			),
			"SessionEndPayload composition reference": strings.Join(
				[]string{"dmsession", ".", "SessionEndPayload"},
				"",
			),
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

func isDMSessionProductionPath(path string) bool {
	return strings.HasPrefix(path, "processor/dmsession/") &&
		strings.HasSuffix(path, ".go") &&
		!strings.HasSuffix(path, "_test.go")
}

func isDMSessionCompositionPath(path string) bool {
	return strings.HasPrefix(path, "cmd/semdragons/") ||
		strings.HasPrefix(path, "componentregistry/") ||
		strings.HasPrefix(path, "config/")
}
