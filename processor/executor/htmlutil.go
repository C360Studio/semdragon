package executor

import (
	"io"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// skipElements are HTML elements whose content (including children) should be
// completely omitted from the text output.
var skipElements = map[string]bool{
	"script":   true,
	"style":    true,
	"nav":      true,
	"footer":   true,
	"header":   true,
	"noscript": true,
}

// blockElements are HTML elements that should be preceded by a newline in output.
var blockElements = map[string]bool{
	"p": true, "div": true, "br": true, "tr": true,
	"blockquote": true, "pre": true, "section": true, "article": true,
	"li": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

// headingLevel returns the numeric level of an h1-h6 tag, or 0 if not a heading.
func headingLevel(tag string) int {
	if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
		return int(tag[1] - '0')
	}
	return 0
}

// htmlToText converts HTML to clean readable text with light markdown formatting.
// Returns the converted text and whether it was truncated at maxBytes.
func htmlToText(r io.Reader, maxBytes int) (string, bool) {
	var sb strings.Builder
	truncated := false

	// skipDepth tracks nesting in elements whose content should be skipped.
	// Each entry is the tag name that opened the skip scope.
	skipStack := make([]string, 0, 8)
	// hrefStack tracks href values for nested <a> tags.
	hrefStack := make([]string, 0, 4)

	z := html.NewTokenizer(r)
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}

		// If we are inside a skip element, only watch for the matching end tag.
		if len(skipStack) > 0 {
			if tt == html.EndTagToken {
				tag, _ := z.TagName()
				tagStr := string(tag)
				if tagStr == skipStack[len(skipStack)-1] {
					skipStack = skipStack[:len(skipStack)-1]
				}
			} else if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
				// Push nested skip tags so we don't pop too early.
				tag, _ := z.TagName()
				if skipElements[string(tag)] {
					skipStack = append(skipStack, string(tag))
				}
			}
			continue
		}

		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			tag, hasAttr := z.TagName()
			tagStr := string(tag)

			if skipElements[tagStr] {
				skipStack = append(skipStack, tagStr)
				continue
			}

			if blockElements[tagStr] {
				if sb.Len() > 0 {
					sb.WriteByte('\n')
				}
			}

			level := headingLevel(tagStr)
			if level > 0 {
				sb.WriteString(strings.Repeat("#", level) + " ")
			} else if tagStr == "li" {
				sb.WriteString("- ")
			}

			// Collect href for anchor tags.
			if tagStr == "a" && hasAttr {
				var href string
				for {
					key, val, more := z.TagAttr()
					if string(key) == "href" {
						href = string(val)
					}
					if !more {
						break
					}
				}
				hrefStack = append(hrefStack, href)
			}

		case html.EndTagToken:
			tag, _ := z.TagName()
			tagStr := string(tag)

			if tagStr == "a" && len(hrefStack) > 0 {
				href := hrefStack[len(hrefStack)-1]
				hrefStack = hrefStack[:len(hrefStack)-1]
				if href != "" {
					toWrite := " (" + href + ")"
					if sb.Len()+len(toWrite) > maxBytes {
						truncated = true
						break
					}
					sb.WriteString(toWrite)
				}
			}

		case html.TextToken:
			text := string(z.Text())
			normalized := normalizeWhitespace(text)
			if normalized == "" || normalized == " " {
				// Only write a space if there's content already and the normalized
				// text is a single space (inline content between tags).
				if normalized == " " && sb.Len() > 0 {
					last := sb.String()[sb.Len()-1]
					if last != '\n' && last != ' ' {
						if sb.Len()+1 > maxBytes {
							truncated = true
							break
						}
						sb.WriteByte(' ')
					}
				}
				continue
			}
			if sb.Len()+len(normalized) > maxBytes {
				remaining := maxBytes - sb.Len()
				if remaining > 0 {
					sb.WriteString(normalized[:remaining])
				}
				truncated = true
				break
			}
			sb.WriteString(normalized)
		}

		if truncated {
			break
		}
	}

	result := sb.String()
	result = strings.TrimSpace(result)
	result = collapseNewlines(result)
	return result, truncated
}

// normalizeWhitespace collapses runs of whitespace (space, tab, newline) to a
// single space. Returns empty string for all-whitespace input.
func normalizeWhitespace(s string) string {
	if s == "" {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				sb.WriteByte(' ')
			}
			prevSpace = true
		} else {
			sb.WriteRune(r)
			prevSpace = false
		}
	}
	return sb.String()
}

// collapseNewlines reduces runs of more than 2 consecutive newlines to 2.
func collapseNewlines(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	newlineCount := 0
	for _, r := range s {
		if r == '\n' {
			newlineCount++
			if newlineCount <= 2 {
				sb.WriteRune(r)
			}
		} else {
			newlineCount = 0
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// extractTitle returns the content of the <title> tag, or empty string.
func extractTitle(r io.Reader) string {
	z := html.NewTokenizer(r)
	inTitle := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.StartTagToken:
			tag, _ := z.TagName()
			if string(tag) == "title" {
				inTitle = true
			}
		case html.EndTagToken:
			tag, _ := z.TagName()
			if string(tag) == "title" {
				return ""
			}
		case html.TextToken:
			if inTitle {
				return strings.TrimSpace(string(z.Text()))
			}
		}
	}
	return ""
}

// slugify converts a string to a URL-friendly slug: lowercase, non-alphanumeric
// characters become "-", consecutive runs are collapsed, and the result is
// trimmed to maxLen characters.
func slugify(s string, maxLen int) string {
	s = strings.ToLower(s)
	var sb strings.Builder
	sb.Grow(len(s))
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			prevDash = false
		} else {
			if !prevDash && sb.Len() > 0 {
				sb.WriteByte('-')
				prevDash = true
			}
		}
	}
	result := strings.TrimRight(sb.String(), "-")
	if len(result) > maxLen {
		result = result[:maxLen]
		result = strings.TrimRight(result, "-")
	}
	return result
}
