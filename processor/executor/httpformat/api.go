// Package httpformat renders HTTP response bodies into agent-friendly views.
// Vendored from semspec/tools/httptool (converter.go + format.go) on
// 2026-04-29 to deliver the markdown/Readability layer that the http_request
// tool description has long claimed but neither codebase actually shipped.
//
// Public API: Render + ParseFormat. Internal converter/formatter machinery
// stays unexported so future evolution can change shape without breaking
// downstream callers.
package httpformat

import (
	"strings"
)

// defaultMaxChars caps the agent-facing output for any format that honours
// max_chars. Sized to fit comfortably under the 32K NATS message-truncation
// ceiling with headroom for the surrounding ToolResult envelope.
const defaultMaxChars = 20000

// Render produces the agent-facing string for an HTTP response body.
//
// Behaviour:
//   - HTML responses (Content-Type contains text/html or application/xhtml)
//     run through Readability + html-to-markdown, then through the
//     format-specific renderer.
//   - All other content types (JSON, XML, plain text, binary) bypass the
//     markdown pipeline and return the body capped at maxChars. This
//     preserves agent code paths that POST to JSON APIs and parse the
//     response as JSON.
//   - On conversion failure (Readability fails to extract anything, parser
//     panics caught upstream, etc.), falls back to a raw-bytes view.
//
// maxChars <= 0 uses defaultMaxChars (20000). Callers should pass an
// explicit value to size against their downstream context budget.
func Render(body []byte, contentType, urlStr string, format Format, maxChars int) string {
	if maxChars <= 0 {
		maxChars = defaultMaxChars
	}

	if !isHTMLContentType(contentType) {
		return rawCapped(body, maxChars)
	}

	conv := newConverter()
	result, err := conv.Convert(body, urlStr)
	if err != nil || result == nil {
		return rawCapped(body, maxChars)
	}

	return formatResponse(format, result, body, urlStr, maxChars)
}

// ParseFormat normalises a format string. Empty or unknown values fall back
// to FormatMarkdown so callers that don't pass a format argument get the
// "fetch and read full content" behaviour their agents are trained on.
//
// See Format constants for the supported view modes.
func ParseFormat(raw string) Format {
	return parseFormat(raw)
}

// isHTMLContentType reports whether a Content-Type header indicates HTML.
// Tolerates case differences, charset suffixes, and missing values; an
// empty content-type defaults to non-HTML so we don't accidentally markdown
// a binary or JSON payload that omitted the header.
func isHTMLContentType(ct string) bool {
	if ct == "" {
		return false
	}
	lower := strings.ToLower(ct)
	return strings.Contains(lower, "text/html") ||
		strings.Contains(lower, "application/xhtml")
}

// rawCapped returns body as a UTF-8 string, capped at maxChars. Used when
// content-type does not warrant HTML conversion or when conversion fails.
func rawCapped(body []byte, maxChars int) string {
	s := string(body)
	if len(s) <= maxChars {
		return s
	}
	return s[:maxChars] + "\n[content truncated]"
}
