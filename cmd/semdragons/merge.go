package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// mergeOverlay reads a model overlay file and deep-merges it into the base
// config bytes. The overlay can set:
//   - "model_registry": replaces the base model_registry entirely
//   - "components": per-component deep merge (overlay keys win)
//   - Any other top-level key: replaces the base value
//
// Within "components", each component entry is deep-merged so the overlay
// only needs to specify the fields it wants to override. For example, an
// overlay can set just "agentic-loop.config.context.summarization_model"
// without repeating the entire agentic-loop config.
func mergeOverlay(base []byte, overlayPath string) ([]byte, error) {
	overlayData, err := os.ReadFile(overlayPath)
	if err != nil {
		return nil, fmt.Errorf("read overlay %s: %w", overlayPath, err)
	}

	var baseMap, overlayMap map[string]any
	if err := json.Unmarshal(base, &baseMap); err != nil {
		return nil, fmt.Errorf("parse base config: %w", err)
	}
	if err := json.Unmarshal(overlayData, &overlayMap); err != nil {
		return nil, fmt.Errorf("parse overlay %s: %w", overlayPath, err)
	}

	merged := deepMerge(baseMap, overlayMap)

	result, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("marshal merged config: %w", err)
	}
	return result, nil
}

// deepMerge recursively merges overlay into base. For map values, it recurses.
// For all other types (arrays, strings, numbers, bools), overlay wins.
func deepMerge(base, overlay map[string]any) map[string]any {
	result := make(map[string]any, len(base))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		baseVal, exists := result[k]
		if exists {
			baseMap, baseIsMap := baseVal.(map[string]any)
			overlayMap, overlayIsMap := v.(map[string]any)
			if baseIsMap && overlayIsMap {
				result[k] = deepMerge(baseMap, overlayMap)
				continue
			}
		}
		result[k] = v
	}
	return result
}
