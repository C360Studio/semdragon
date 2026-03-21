package executor

import (
	"strings"
	"testing"
)

func TestHtmlToText_BasicParagraphs(t *testing.T) {
	t.Parallel()
	input := `<html><body><p>Hello world.</p><p>Second paragraph.</p></body></html>`
	got, truncated := htmlToText(strings.NewReader(input), 10000)
	if truncated {
		t.Error("expected no truncation")
	}
	if !strings.Contains(got, "Hello world.") {
		t.Errorf("expected 'Hello world.' in output, got: %q", got)
	}
	if !strings.Contains(got, "Second paragraph.") {
		t.Errorf("expected 'Second paragraph.' in output, got: %q", got)
	}
	// Paragraphs should be separated by newlines.
	idx1 := strings.Index(got, "Hello world.")
	idx2 := strings.Index(got, "Second paragraph.")
	between := got[idx1+len("Hello world.") : idx2]
	if !strings.Contains(between, "\n") {
		t.Errorf("expected newline between paragraphs, got: %q", between)
	}
}

func TestHtmlToText_ScriptStyleStripped(t *testing.T) {
	t.Parallel()
	input := `<html><head><style>body { color: red; }</style><script>alert('x')</script></head>
<body><p>Content here.</p></body></html>`
	got, _ := htmlToText(strings.NewReader(input), 10000)
	if strings.Contains(got, "body { color") {
		t.Errorf("style content should be stripped, got: %q", got)
	}
	if strings.Contains(got, "alert") {
		t.Errorf("script content should be stripped, got: %q", got)
	}
	if !strings.Contains(got, "Content here.") {
		t.Errorf("expected 'Content here.' in output, got: %q", got)
	}
}

func TestHtmlToText_LinksPreserved(t *testing.T) {
	t.Parallel()
	input := `<html><body><p>Visit <a href="https://example.com">Example</a> for more.</p></body></html>`
	got, _ := htmlToText(strings.NewReader(input), 10000)
	if !strings.Contains(got, "Example") {
		t.Errorf("expected link text 'Example' in output, got: %q", got)
	}
	if !strings.Contains(got, "https://example.com") {
		t.Errorf("expected href 'https://example.com' in output, got: %q", got)
	}
	// The href should appear in parentheses after the link text.
	if !strings.Contains(got, "Example (https://example.com)") {
		t.Errorf("expected 'Example (https://example.com)' format, got: %q", got)
	}
}

func TestHtmlToText_HeadingsMarkdown(t *testing.T) {
	t.Parallel()
	input := `<html><body><h1>Title</h1><h2>Subtitle</h2><h3>Section</h3></body></html>`
	got, _ := htmlToText(strings.NewReader(input), 10000)
	if !strings.Contains(got, "# Title") {
		t.Errorf("expected '# Title', got: %q", got)
	}
	if !strings.Contains(got, "## Subtitle") {
		t.Errorf("expected '## Subtitle', got: %q", got)
	}
	if !strings.Contains(got, "### Section") {
		t.Errorf("expected '### Section', got: %q", got)
	}
}

func TestHtmlToText_ListItems(t *testing.T) {
	t.Parallel()
	input := `<html><body><ul><li>Apple</li><li>Banana</li><li>Cherry</li></ul></body></html>`
	got, _ := htmlToText(strings.NewReader(input), 10000)
	if !strings.Contains(got, "- Apple") {
		t.Errorf("expected '- Apple', got: %q", got)
	}
	if !strings.Contains(got, "- Banana") {
		t.Errorf("expected '- Banana', got: %q", got)
	}
	if !strings.Contains(got, "- Cherry") {
		t.Errorf("expected '- Cherry', got: %q", got)
	}
}

func TestHtmlToText_NavFooterHeaderStripped(t *testing.T) {
	t.Parallel()
	input := `<html><body>
<nav><a href="/">Home</a><a href="/about">About</a></nav>
<main><p>Main content here.</p></main>
<header><h1>Site Header</h1></header>
<footer><p>Copyright 2024</p></footer>
</body></html>`
	got, _ := htmlToText(strings.NewReader(input), 10000)
	if strings.Contains(got, "Copyright 2024") {
		t.Errorf("footer content should be stripped, got: %q", got)
	}
	if strings.Contains(got, "Site Header") {
		t.Errorf("header content should be stripped, got: %q", got)
	}
	// Nav links may appear if their href is retained, but nav text should be gone.
	// We check that the nav anchor text "Home" and "About" are stripped.
	// (They are inside <nav>, which is a skip element.)
	if strings.Contains(got, "Home") || strings.Contains(got, "About") {
		t.Errorf("nav content should be stripped, got: %q", got)
	}
	if !strings.Contains(got, "Main content here.") {
		t.Errorf("expected 'Main content here.' in output, got: %q", got)
	}
}

func TestHtmlToText_Truncation(t *testing.T) {
	t.Parallel()
	// Build HTML with content that will exceed our small limit.
	input := `<html><body><p>` + strings.Repeat("x", 200) + `</p></body></html>`
	got, truncated := htmlToText(strings.NewReader(input), 50)
	if !truncated {
		t.Error("expected truncation flag to be true")
	}
	if len(got) > 60 { // Allow a small buffer for trimming operations.
		t.Errorf("expected output near 50 chars, got %d chars: %q", len(got), got)
	}
}

func TestHtmlToText_EmptyInput(t *testing.T) {
	t.Parallel()
	got, truncated := htmlToText(strings.NewReader(""), 10000)
	if got != "" {
		t.Errorf("expected empty output for empty input, got: %q", got)
	}
	if truncated {
		t.Error("expected no truncation for empty input")
	}
}

func TestHtmlToText_PlainText(t *testing.T) {
	t.Parallel()
	// Non-HTML text should pass through with whitespace normalized.
	input := "Hello   world\t\ttest"
	got, _ := htmlToText(strings.NewReader(input), 10000)
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "world") || !strings.Contains(got, "test") {
		t.Errorf("expected plain text content preserved, got: %q", got)
	}
	// Consecutive whitespace should be collapsed.
	if strings.Contains(got, "   ") {
		t.Errorf("expected collapsed whitespace, got: %q", got)
	}
}

func TestExtractTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "basic title",
			input: `<html><head><title>My Page Title</title></head><body></body></html>`,
			want:  "My Page Title",
		},
		{
			name:  "title with whitespace",
			input: `<html><head><title>  Trimmed Title  </title></head></html>`,
			want:  "Trimmed Title",
		},
		{
			name:  "no title tag",
			input: `<html><body><p>No title here.</p></body></html>`,
			want:  "",
		},
		{
			name:  "empty title tag",
			input: `<html><head><title></title></head></html>`,
			want:  "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractTitle(strings.NewReader(tc.input))
			if got != tc.want {
				t.Errorf("extractTitle() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSlugify(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "basic slug",
			input:  "Hello World",
			maxLen: 40,
			want:   "hello-world",
		},
		{
			name:   "special characters",
			input:  "My Page: The (Best) One!",
			maxLen: 40,
			want:   "my-page-the-best-one",
		},
		{
			name:   "truncate to maxLen",
			input:  "this is a very long title that should be truncated at some point",
			maxLen: 20,
			want:   "this-is-a-very-long",
		},
		{
			name:   "consecutive specials collapse",
			input:  "foo---bar",
			maxLen: 40,
			want:   "foo-bar",
		},
		{
			name:   "leading/trailing specials trimmed",
			input:  "  hello world  ",
			maxLen: 40,
			want:   "hello-world",
		},
		{
			name:   "all special characters",
			input:  "!!!",
			maxLen: 40,
			want:   "",
		},
		{
			name:   "numbers preserved",
			input:  "Go 1.22 Release",
			maxLen: 40,
			want:   "go-1-22-release",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := slugify(tc.input, tc.maxLen)
			if got != tc.want {
				t.Errorf("slugify(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
			}
		})
	}
}
