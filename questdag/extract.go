package questdag

import (
	"encoding/json"
	"strings"
)

// ExtractDAG parses a party lead's decompose_quest tool output into a validated
// DAG. The tool contract emits a JSON envelope {"goal": "...", "dag": {"nodes":
// [...]}}; LLMs often wrap it in prose or a markdown fence, so ExtractDAG
// tolerates a leading/trailing text by decoding the first JSON object it finds.
// It returns the DAG, the stated goal, and ok=false when the output is not a
// valid, structurally-sound decomposition (missing keys, malformed JSON, or a
// DAG that fails Validate). Validation here is defense in depth — the tool
// validates on creation, but the output may be truncated or altered in transit.
func ExtractDAG(output string) (dag *DAG, goal string, ok bool) {
	if output == "" {
		return nil, "", false
	}

	// Decode the output as a top-level JSON object; if that fails, scan for the
	// first '{' and decode from there (tolerates a markdown fence / prose lead).
	var raw map[string]any
	if err := json.NewDecoder(strings.NewReader(output)).Decode(&raw); err != nil {
		start := strings.Index(output, "{")
		if start == -1 {
			return nil, "", false
		}
		if err2 := json.NewDecoder(strings.NewReader(output[start:])).Decode(&raw); err2 != nil {
			return nil, "", false
		}
	}

	// Require both keys per the decompose_quest tool contract.
	goalRaw, hasGoal := raw["goal"]
	if !hasGoal {
		return nil, "", false
	}
	dagRaw, hasDag := raw["dag"]
	if !hasDag {
		return nil, "", false
	}

	// Re-marshal then unmarshal the "dag" value into DAG for type safety.
	dagData, err := json.Marshal(dagRaw)
	if err != nil {
		return nil, "", false
	}
	var parsed DAG
	if err := json.Unmarshal(dagData, &parsed); err != nil {
		return nil, "", false
	}
	if err := parsed.Validate(); err != nil {
		return nil, "", false
	}

	goalStr, _ := goalRaw.(string)
	return &parsed, goalStr, true
}
