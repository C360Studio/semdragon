package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// mergeOverlay reads a model overlay file and applies it on top of the base
// config. The overlay semantics are intentionally simple:
//
//   - "model_registry": replaces the base model_registry entirely
//   - "components": per-component replacement (overlay component wins wholly)
//   - Any other top-level key: replaces the base value
//
// No recursive deep-merge — overlay values win at the granularity above.
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

	for k, v := range overlayMap {
		if k == "components" {
			// Per-component shallow merge: for each named component in the
			// overlay, merge its top-level fields into the base component.
			// e.g. overlay setting "config" merges into the base component
			// that already has "type", "enabled", "ports", etc.
			baseComponents, _ := baseMap["components"].(map[string]any)
			overlayComponents, _ := v.(map[string]any)
			if baseComponents == nil {
				baseComponents = make(map[string]any)
			}
			for name, comp := range overlayComponents {
				baseComp, _ := baseComponents[name].(map[string]any)
				overlayComp, _ := comp.(map[string]any)
				if baseComp != nil && overlayComp != nil {
					for field, val := range overlayComp {
						// "config" gets one more level of merge so overlays
						// can set config.context.summarization_model without
						// losing config.max_iterations from the base.
						if field == "config" {
							baseConfig, _ := baseComp["config"].(map[string]any)
							overlayConfig, _ := val.(map[string]any)
							if baseConfig != nil && overlayConfig != nil {
								for ck, cv := range overlayConfig {
									baseConfig[ck] = cv
								}
								continue
							}
						}
						baseComp[field] = val
					}
				} else {
					baseComponents[name] = comp
				}
			}
			baseMap["components"] = baseComponents
		} else {
			// Everything else (model_registry, etc.) replaces entirely.
			baseMap[k] = v
		}
	}

	result, err := json.Marshal(baseMap)
	if err != nil {
		return nil, fmt.Errorf("marshal merged config: %w", err)
	}
	return result, nil
}
