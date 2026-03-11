package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDeepMerge_TopLevelReplace(t *testing.T) {
	base := map[string]any{
		"version": "1.0",
		"name":    "base",
	}
	overlay := map[string]any{
		"name": "overlay",
	}
	result := deepMerge(base, overlay)
	if result["version"] != "1.0" {
		t.Errorf("version should be preserved from base, got %v", result["version"])
	}
	if result["name"] != "overlay" {
		t.Errorf("name should be replaced by overlay, got %v", result["name"])
	}
}

func TestDeepMerge_NestedMerge(t *testing.T) {
	base := map[string]any{
		"components": map[string]any{
			"agentic-loop": map[string]any{
				"type":    "processor",
				"enabled": true,
				"config": map[string]any{
					"max_iterations": float64(20),
					"timeout":        "300s",
					"context": map[string]any{
						"tool_result_max_age":  float64(3),
						"summarization_model":  "ollama-coder",
						"compact_threshold":    0.6,
					},
				},
			},
			"questboard": map[string]any{
				"type":    "processor",
				"enabled": true,
			},
		},
	}
	overlay := map[string]any{
		"components": map[string]any{
			"agentic-loop": map[string]any{
				"config": map[string]any{
					"context": map[string]any{
						"tool_result_max_age":  float64(20),
						"summarization_model":  "claude-haiku",
					},
				},
			},
		},
	}

	result := deepMerge(base, overlay)

	// Navigate to the merged agentic-loop config
	components := result["components"].(map[string]any)
	loop := components["agentic-loop"].(map[string]any)

	// type and enabled should be preserved from base
	if loop["type"] != "processor" {
		t.Errorf("agentic-loop.type should be preserved, got %v", loop["type"])
	}
	if loop["enabled"] != true {
		t.Errorf("agentic-loop.enabled should be preserved, got %v", loop["enabled"])
	}

	config := loop["config"].(map[string]any)

	// timeout should be preserved from base
	if config["timeout"] != "300s" {
		t.Errorf("config.timeout should be preserved, got %v", config["timeout"])
	}

	context := config["context"].(map[string]any)

	// Overlay values should win
	if context["tool_result_max_age"] != float64(20) {
		t.Errorf("context.tool_result_max_age should be 20, got %v", context["tool_result_max_age"])
	}
	if context["summarization_model"] != "claude-haiku" {
		t.Errorf("context.summarization_model should be claude-haiku, got %v", context["summarization_model"])
	}

	// Base-only values should be preserved
	if context["compact_threshold"] != 0.6 {
		t.Errorf("context.compact_threshold should be preserved, got %v", context["compact_threshold"])
	}

	// questboard should be untouched
	qb := components["questboard"].(map[string]any)
	if qb["type"] != "processor" {
		t.Errorf("questboard should be preserved from base")
	}
}

func TestDeepMerge_ModelRegistryReplace(t *testing.T) {
	base := map[string]any{
		"model_registry": map[string]any{
			"endpoints": map[string]any{
				"ollama": map[string]any{"url": "localhost"},
			},
		},
	}
	overlay := map[string]any{
		"model_registry": map[string]any{
			"endpoints": map[string]any{
				"claude": map[string]any{"url": "api.anthropic.com"},
			},
		},
	}

	result := deepMerge(base, overlay)
	registry := result["model_registry"].(map[string]any)
	endpoints := registry["endpoints"].(map[string]any)

	// Deep merge means both endpoints exist (overlay adds, doesn't replace map)
	if _, ok := endpoints["claude"]; !ok {
		t.Error("claude endpoint should be present from overlay")
	}
	// Since endpoints is a map, deep merge keeps both
	if _, ok := endpoints["ollama"]; !ok {
		t.Error("ollama endpoint should be preserved from base (deep merge)")
	}
}

func TestMergeOverlay_Integration(t *testing.T) {
	// Write a temp overlay file
	overlay := map[string]any{
		"model_registry": map[string]any{
			"endpoints": map[string]any{
				"test-model": map[string]any{
					"provider": "openai",
					"model":    "test",
				},
			},
			"defaults": map[string]any{
				"model": "test-model",
			},
		},
	}
	overlayBytes, _ := json.Marshal(overlay)
	tmpDir := t.TempDir()
	overlayPath := filepath.Join(tmpDir, "test-overlay.json")
	if err := os.WriteFile(overlayPath, overlayBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	base := []byte(`{"version":"1.0","model_registry":{"endpoints":{"old":{"provider":"ollama"}}}}`)
	result, err := mergeOverlay(base, overlayPath)
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

	registry := merged["model_registry"].(map[string]any)
	endpoints := registry["endpoints"].(map[string]any)
	if _, ok := endpoints["test-model"]; !ok {
		t.Error("test-model should be present from overlay")
	}
}
