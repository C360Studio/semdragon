package executor

import (
	"fmt"
	"path/filepath"
	"strings"
)

// lintContent performs a basic syntax check on file content before writing.
// Returns an error message if the content has obvious syntax issues, or empty string if OK.
// This is a structural guard (SWE-agent pattern) — it catches common agent mistakes
// like writing shell commands as Python code or producing malformed JSON.
//
// The check is intentionally lightweight — we catch obvious errors, not style issues.
// Full linting happens via the lint_check tool after the file is written.
func lintContent(path, content string) string {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".py":
		return lintPython(content)
	case ".json":
		return lintJSON(content)
	case ".go":
		return lintGo(content)
	case ".js", ".ts", ".jsx", ".tsx":
		return lintJavaScript(content)
	case ".java":
		return lintJava(content)
	default:
		return "" // No linting for unknown file types
	}
}

// lintPython catches common Python syntax issues.
func lintPython(content string) string {
	lines := strings.Split(content, "\n")

	// Check for shell-like content that shouldn't be in a Python file
	if len(lines) > 0 {
		first := strings.TrimSpace(lines[0])
		if strings.HasPrefix(first, "$ ") || strings.HasPrefix(first, "# !") {
			// Allow shebangs
			if !strings.HasPrefix(first, "#!") {
				return "Content looks like shell commands, not Python code. Use write_file for source code, bash for commands."
			}
		}
	}

	// Check for unclosed triple quotes (common when agents dump markdown as Python)
	tripleDouble := strings.Count(content, `"""`)
	tripleSingle := strings.Count(content, `'''`)
	if tripleDouble%2 != 0 {
		return fmt.Sprintf("Unclosed triple-double-quote (\"\"\") — found %d occurrences (should be even)", tripleDouble)
	}
	if tripleSingle%2 != 0 {
		return fmt.Sprintf("Unclosed triple-single-quote (''') — found %d occurrences (should be even)", tripleSingle)
	}

	return ""
}

// lintJSON catches malformed JSON.
func lintJSON(content string) string {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) == 0 {
		return "Empty JSON file"
	}

	// Must start with { or [
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return "JSON must start with { or [ — this doesn't look like valid JSON"
	}

	// Check matching brackets
	opens := strings.Count(trimmed, "{") + strings.Count(trimmed, "[")
	closes := strings.Count(trimmed, "}") + strings.Count(trimmed, "]")
	if opens != closes {
		return fmt.Sprintf("Mismatched brackets: %d opening vs %d closing", opens, closes)
	}

	return ""
}

// lintGo catches common Go syntax issues.
func lintGo(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "package ") {
		return "Go files must start with a package declaration (e.g., 'package main')"
	}
	return ""
}

// lintJavaScript catches obvious JS/TS issues.
func lintJavaScript(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 {
		first := strings.TrimSpace(lines[0])
		if strings.HasPrefix(first, "$ ") {
			return "Content looks like shell commands, not JavaScript. Use write_file for source code, bash for commands."
		}
	}
	return ""
}

// lintJava catches obvious Java issues.
func lintJava(content string) string {
	trimmed := strings.TrimSpace(content)
	// Java files should have package or import or class/interface near the top
	if !strings.HasPrefix(trimmed, "package ") &&
		!strings.HasPrefix(trimmed, "import ") &&
		!strings.Contains(trimmed[:min(500, len(trimmed))], "class ") &&
		!strings.Contains(trimmed[:min(500, len(trimmed))], "interface ") {
		return "Java files should start with a package declaration, import, or class/interface definition"
	}
	return ""
}
