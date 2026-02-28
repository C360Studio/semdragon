package semdragons

import (
	"log/slog"
	"strings"
)

// =============================================================================
// LLM UTILITIES - Shared helpers for LLM response parsing
// =============================================================================

// ExtractJSONFromLLMResponse extracts JSON from an LLM response that may be
// wrapped in markdown code blocks or contain other text.
func ExtractJSONFromLLMResponse(s string) string {
	s = strings.TrimSpace(s)

	// Try to find JSON in ```json code blocks
	if start := strings.Index(s, "```json"); start != -1 {
		start += 7
		if end := strings.Index(s[start:], "```"); end != -1 {
			return strings.TrimSpace(s[start : start+end])
		}
	}

	// Try to find JSON in generic ``` code blocks
	if start := strings.Index(s, "```"); start != -1 {
		start += 3
		// Skip language identifier if present
		if newline := strings.Index(s[start:], "\n"); newline != -1 {
			start += newline + 1
		}
		if end := strings.Index(s[start:], "```"); end != -1 {
			return strings.TrimSpace(s[start : start+end])
		}
	}

	// Try to find raw JSON (starts with {)
	if start := strings.Index(s, "{"); start != -1 {
		// Find matching closing brace
		depth := 0
		for i := start; i < len(s); i++ {
			if s[i] == '{' {
				depth++
			} else if s[i] == '}' {
				depth--
				if depth == 0 {
					return s[start : i+1]
				}
			}
		}
	}

	return s
}

// TruncateForLog truncates a string to a maximum length for logging.
func TruncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// LogLLMParseFailure logs a warning when LLM response parsing fails.
// This helps with debugging LLM response format issues.
func LogLLMParseFailure(operation string, err error, rawContent, extractedJSON string) {
	slog.Warn("failed to parse LLM response",
		"operation", operation,
		"error", err,
		"raw_content", TruncateForLog(rawContent, 500),
		"extracted_json", TruncateForLog(extractedJSON, 500),
	)
}
