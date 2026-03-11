package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMergeOverlay_ModelRegistryReplacesEntirely(t *testing.T) {
	base := []byte(`{
		"version": "1.0",
		"model_registry": {
			"endpoints": {
				"ollama": {"url": "localhost:11434"}
			},
			"capabilities": {
				"quest-design": {"preferred": ["ollama"]}
			}
		}
	}`)

	overlay := map[string]any{
		"model_registry": map[string]any{
			"endpoints": map[string]any{
				"mock-llm": map[string]any{"url": "http://mockllm:9090"},
			},
			"capabilities": map[string]any{
				"dm-chat": map[string]any{"preferred": []string{"mock-llm"}},
			},
		},
	}
	overlayBytes, _ := json.Marshal(overlay)
	path := writeTempOverlay(t, overlayBytes)

	result, err := mergeOverlay(base, path)
	if err != nil {
		t.Fatal(err)
	}

	var merged map[string]any
	if err := json.Unmarshal(result, &merged); err != nil {
		t.Fatal(err)
	}

	// version preserved from base
	if merged["version"] != "1.0" {
		t.Errorf("version should be preserved, got %v", merged["version"])
	}

	registry := merged["model_registry"].(map[string]any)
	endpoints := registry["endpoints"].(map[string]any)

	// overlay endpoint present
	if _, ok := endpoints["mock-llm"]; !ok {
		t.Error("mock-llm should be present from overlay")
	}
	// base endpoint gone — model_registry replaced entirely
	if _, ok := endpoints["ollama"]; ok {
		t.Error("ollama should NOT survive — model_registry replaces entirely")
	}

	capabilities := registry["capabilities"].(map[string]any)

	// overlay capability present
	if _, ok := capabilities["dm-chat"]; !ok {
		t.Error("dm-chat should be present from overlay")
	}
	// base-only capability gone
	if _, ok := capabilities["quest-design"]; ok {
		t.Error("quest-design should NOT survive — model_registry replaces entirely")
	}
}

func TestMergeOverlay_ComponentConfigMerge(t *testing.T) {
	base := []byte(`{
		"components": {
			"agentic-loop": {
				"type": "processor",
				"enabled": true,
				"config": {"max_iterations": 20, "timeout": "120s", "context": {"compact_threshold": 0.6}}
			},
			"questboard": {
				"type": "processor",
				"enabled": true
			}
		}
	}`)

	overlay := map[string]any{
		"components": map[string]any{
			"agentic-loop": map[string]any{
				"config": map[string]any{"context": map[string]any{"summarization_model": "mockllm"}},
			},
		},
	}
	overlayBytes, _ := json.Marshal(overlay)
	path := writeTempOverlay(t, overlayBytes)

	result, err := mergeOverlay(base, path)
	if err != nil {
		t.Fatal(err)
	}

	var merged map[string]any
	if err := json.Unmarshal(result, &merged); err != nil {
		t.Fatal(err)
	}

	components := merged["components"].(map[string]any)
	loop := components["agentic-loop"].(map[string]any)

	// base component fields preserved
	if loop["type"] != "processor" {
		t.Error("type should be preserved from base")
	}
	if loop["enabled"] != true {
		t.Error("enabled should be preserved from base")
	}

	config := loop["config"].(map[string]any)

	// base config fields preserved
	if config["max_iterations"] != float64(20) {
		t.Errorf("max_iterations should be preserved, got %v", config["max_iterations"])
	}
	if config["timeout"] != "120s" {
		t.Errorf("timeout should be preserved, got %v", config["timeout"])
	}

	// overlay config field replaces (context replaced wholesale by overlay)
	context := config["context"].(map[string]any)
	if context["summarization_model"] != "mockllm" {
		t.Errorf("summarization_model should be mockllm, got %v", context["summarization_model"])
	}

	// questboard untouched from base
	qb := components["questboard"].(map[string]any)
	if qb["type"] != "processor" {
		t.Error("questboard should be preserved from base")
	}
}

func TestMergeOverlay_TopLevelKeyReplace(t *testing.T) {
	base := []byte(`{"version": "1.0", "name": "base"}`)
	overlay := map[string]any{"name": "overlay"}
	overlayBytes, _ := json.Marshal(overlay)
	path := writeTempOverlay(t, overlayBytes)

	result, err := mergeOverlay(base, path)
	if err != nil {
		t.Fatal(err)
	}

	var merged map[string]any
	if err := json.Unmarshal(result, &merged); err != nil {
		t.Fatal(err)
	}

	if merged["version"] != "1.0" {
		t.Errorf("version should be preserved, got %v", merged["version"])
	}
	if merged["name"] != "overlay" {
		t.Errorf("name should be replaced by overlay, got %v", merged["name"])
	}
}

func writeTempOverlay(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "overlay.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
