package semdragons_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type sourceInventoryScope struct {
	path       string
	recursive  bool
	extensions map[string]struct{}
}

func TestPromptAssemblyHasNoGraphProjectionSurface(t *testing.T) {
	root := repositoryRoot(t)
	goFiles := extensions(".go")
	configFiles := extensions(".go", ".json", ".yaml", ".yml", ".toml")
	uiFiles := extensions(".ts", ".js", ".svelte", ".json")
	scopes := []sourceInventoryScope{
		{path: ".", extensions: goFiles},
		{path: "cmd", recursive: true, extensions: goFiles},
		{path: "componentregistry", recursive: true, extensions: goFiles},
		{path: "config", recursive: true, extensions: configFiles},
		{path: "domain", recursive: true, extensions: goFiles},
		{path: "domains", recursive: true, extensions: goFiles},
		{path: "internal", recursive: true, extensions: goFiles},
		{path: "processor", recursive: true, extensions: goFiles},
		{path: "questdag", recursive: true, extensions: goFiles},
		{path: "questlifecycle", recursive: true, extensions: goFiles},
		{path: "semsource", recursive: true, extensions: goFiles},
		{path: "service", recursive: true, extensions: goFiles},
		{path: "ui/src", recursive: true, extensions: uiFiles},
		{path: "ui/e2e", recursive: true, extensions: uiFiles},
	}

	files := make(map[string][]byte)
	for _, scope := range scopes {
		count := collectInventoryScope(t, root, scope, files)
		if count == 0 {
			t.Errorf("source inventory scope %q contributed no files", scope.path)
		}
	}

	// These anchors stale-check the semantically important inventory lanes:
	// prompt composition, application payload bootstrap, relationship fixtures,
	// and runtime rule configuration.
	for _, anchor := range []string{
		"processor/promptmanager/assembler.go",
		"cmd/semdragons/main.go",
		"graph_relationship_contract_test.go",
		"config/rules/quest/01-claim-start.json",
	} {
		if _, ok := files[anchor]; !ok {
			t.Errorf("source inventory is missing required anchor %q", anchor)
		}
	}

	globalRules := map[string]string{
		"deleted payload type":   strings.Join([]string{"Prompt", "Assembled", "Payload"}, ""),
		"deleted typed subject":  strings.Join([]string{"Subject", "Prompt", "Assembled"}, ""),
		"deleted vocabulary key": strings.Join([]string{"Predicate", "Prompt", "Assembled"}, ""),
		"graph predicate family": strings.Join([]string{"prompt", ".", "assembly", "."}, ""),
		"external payload registration": strings.Join(
			[]string{"promptmanager", ".", "Register", "Payloads"},
			"",
		),
		"external projection contract": strings.Join(
			[]string{"promptmanager", ".", "Projection", "Bindings"},
			"",
		),
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		data := files[path]
		for description, forbidden := range globalRules {
			if bytes.Contains(data, []byte(forbidden)) {
				t.Errorf("%s contains forbidden %s %q", path, description, forbidden)
			}
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve source inventory test location")
	}
	return filepath.Dir(filename)
}

func extensions(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func collectInventoryScope(
	t *testing.T,
	root string,
	scope sourceInventoryScope,
	files map[string][]byte,
) int {
	t.Helper()
	scopeRoot := filepath.Join(root, filepath.FromSlash(scope.path))
	info, err := os.Stat(scopeRoot)
	if err != nil {
		t.Fatalf("stat source inventory scope %q: %v", scope.path, err)
	}
	if !info.IsDir() {
		t.Fatalf("source inventory scope %q is not a directory", scope.path)
	}

	count := 0
	err = filepath.WalkDir(scopeRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != scopeRoot && !scope.recursive {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := scope.extensions[filepath.Ext(entry.Name())]; !ok {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, duplicate := files[relative]; duplicate {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[relative] = data
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("walk source inventory scope %q: %v", scope.path, err)
	}
	return count
}
