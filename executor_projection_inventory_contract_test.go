package semdragons_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutorProjectionInventoryRetainsOnlyLiveRawLifecycleSurface(t *testing.T) {
	root := repositoryRoot(t)
	scopes := []sourceInventoryScope{
		{path: "domain", recursive: true, extensions: extensions(".go")},
		{path: "processor/executor", recursive: true, extensions: extensions(".go")},
		{path: "processor/questbridge", recursive: true, extensions: extensions(".go")},
		{path: "cmd/semdragons", recursive: true, extensions: extensions(".go")},
		{path: "componentregistry", recursive: true, extensions: extensions(".go")},
		{path: "config", recursive: true, extensions: extensions(".json", ".yaml", ".yml", ".toml")},
	}
	files := make(map[string][]byte)
	for _, scope := range scopes {
		if count := collectInventoryScope(t, root, scope, files); count == 0 {
			t.Errorf("executor inventory scope %q contributed no files", scope.path)
		}
	}
	const relationshipFixture = "graph_relationship_contract_test.go"
	data, err := os.ReadFile(filepath.Join(root, relationshipFixture))
	if err != nil {
		t.Fatalf("read executor relationship fixture: %v", err)
	}
	files[relationshipFixture] = data

	for _, path := range []string{
		"domain/vocab.go",
		"processor/executor/payloads.go",
		"processor/executor/component.go",
		"processor/executor/tools.go",
		"processor/questbridge/handler.go",
		"cmd/semdragons/main.go",
		"componentregistry/register.go",
		relationshipFixture,
	} {
		if _, ok := files[path]; !ok {
			t.Errorf("executor inventory is missing required anchor %q", path)
		}
	}

	required := map[string][]string{
		"processor/executor/payloads.go": {
			strings.Join([]string{"Subject", "Execution", "Started"}, ""),
			strings.Join([]string{"Subject", "Execution", "Completed"}, ""),
			strings.Join([]string{"Subject", "Execution", "Failed"}, ""),
		},
		"processor/executor/component.go": {
			strings.Join([]string{"Subject", "Execution", "Started", ".", "Publish"}, ""),
			strings.Join([]string{"Subject", "Execution", "Completed", ".", "Publish"}, ""),
			strings.Join([]string{"Subject", "Execution", "Failed", ".", "Publish"}, ""),
		},
		"processor/questbridge/handler.go": {
			strings.Join([]string{"Subject", "Execution", "Started", ".", "Publish"}, ""),
			strings.Join([]string{"Subject", "Execution", "Completed", ".", "Publish"}, ""),
			strings.Join([]string{"Subject", "Execution", "Failed", ".", "Publish"}, ""),
		},
	}
	for path, anchors := range required {
		data, ok := files[path]
		if !ok {
			continue
		}
		for _, anchor := range anchors {
			if !bytes.Contains(data, []byte(anchor)) {
				t.Errorf("%s is missing live lifecycle anchor %q", path, anchor)
			}
		}
	}

	forbidden := map[string]string{
		"dead tool-call payload":    strings.Join([]string{"Tool", "Call", "Payload"}, ""),
		"dead tool-result payload":  strings.Join([]string{"Tool", "Result", "Payload"}, ""),
		"dead tool-call subject":    strings.Join([]string{"Subject", "Tool", "Call"}, ""),
		"dead tool-result subject":  strings.Join([]string{"Subject", "Tool", "Result"}, ""),
		"dead tool-call vocab":      strings.Join([]string{"Predicate", "Tool", "Call"}, ""),
		"dead tool-result vocab":    strings.Join([]string{"Predicate", "Tool", "Result"}, ""),
		"tool-call graph family":    strings.Join([]string{"execution", ".", "tool", ".", "call"}, ""),
		"tool-result graph family":  strings.Join([]string{"execution", ".", "tool", ".", "result"}, ""),
		"executor projection bind":  strings.Join([]string{"executor", ".", "Projection", "Bindings"}, ""),
		"executor payload registry": strings.Join([]string{"executor", ".", "Register", "Payloads"}, ""),
	}
	referenceDatatype := strings.Join([]string{"semcompat", ".", "Entity", "Reference", "Datatype"}, "")
	executorReferenceCount := 0
	for path, data := range files {
		if strings.HasPrefix(path, "processor/executor/") && !strings.HasSuffix(path, "_test.go") {
			executorReferenceCount += bytes.Count(data, []byte(referenceDatatype))
		}
		for description, fragment := range forbidden {
			if bytes.Contains(data, []byte(fragment)) {
				t.Errorf("%s contains forbidden %s %q", path, description, fragment)
			}
		}
		if strings.HasPrefix(path, "processor/executor/") ||
			strings.HasPrefix(path, "processor/questbridge/") {
			for description, fragment := range map[string]string{
				"projection contract": strings.Join([]string{"projection", ".", "Contract"}, ""),
				"projection bind":     strings.Join([]string{"projection", ".", "Bind"}, ""),
				"payload registration": strings.Join(
					[]string{"Register", "Payload"},
					"",
				),
				"graph-ingest route": strings.Join([]string{"graph", ".", "ingest", ".", "entity"}, ""),
			} {
				if bytes.Contains(data, []byte(fragment)) {
					t.Errorf("%s contains forbidden executor %s %q", path, description, fragment)
				}
			}
		}
	}
	if executorReferenceCount != 3 {
		t.Errorf("processor/executor @id relationship declarations = %d, want exactly 3", executorReferenceCount)
	}
}
