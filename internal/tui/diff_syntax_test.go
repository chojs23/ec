package tui

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

var trueColorForegroundPattern = regexp.MustCompile(`38;2;[0-9]+;[0-9]+;[0-9]+`)

func TestHighlightDiffCodePreservesLinesAndDiffBackgrounds(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	lines := []string{
		"/* comment starts",
		"and continues */",
		`const value = "text";`,
		"",
	}
	base := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#f0f0f0")).
		Background(lipgloss.Color("#112233"))
	baseStyles := []lipgloss.Style{base, base, base, base}

	got := highlightDiffCode("example.ts", lines, baseStyles)
	if len(got) != len(lines) {
		t.Fatalf("highlighted line count = %d, want %d", len(got), len(lines))
	}
	for index, line := range lines {
		if plain := ansi.Strip(got[index]); plain != line {
			t.Fatalf("line %d text = %q, want %q", index, plain, line)
		}
	}

	// The second line is only a comment when the lexer keeps state across lines.
	if !strings.Contains(got[1], "38;2;139;147;158") {
		t.Fatalf("continued comment does not use GitHub Dark comment color: %q", got[1])
	}
	if !strings.Contains(got[1], "48;2;17;34;51") {
		t.Fatalf("syntax styling replaced the diff background: %q", got[1])
	}
	for index, line := range got {
		if strings.Contains(line, "48;2;13;17;23") {
			t.Fatalf("line %d contains the GitHub Dark palette background: %q", index, line)
		}
	}
	if !strings.Contains(got[2], "38;2;255;123;113") {
		t.Fatalf("TypeScript keyword does not use GitHub Dark keyword color: %q", got[2])
	}
}

func TestHighlightDiffCodeUsesEachLinesDiffBackgroundForMultilineTokens(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	lines := []string{"/* context", "changed */"}
	styles := []lipgloss.Style{
		lipgloss.NewStyle().Background(lipgloss.Color("#442222")),
		lipgloss.NewStyle().Background(lipgloss.Color("#114422")),
	}

	got := highlightDiffCode("example.ts", lines, styles)
	if !strings.Contains(got[0], "48;2;68;34;34") {
		t.Fatalf("first line lost its context background: %q", got[0])
	}
	if !strings.Contains(got[1], "48;2;17;68;34") {
		t.Fatalf("second line lost its changed background: %q", got[1])
	}
}

func TestHighlightDiffCodeSupportsLanguagesWithoutFinalNewline(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	tests := []struct {
		name     string
		filename string
		source   string
	}{
		{name: "Go", filename: "main.go", source: "package main\nfunc main() { println(\"x\") }"},
		{name: "Rust", filename: "main.rs", source: `fn main() { let value = "x"; }`},
		{name: "TypeScript", filename: "main.ts", source: `const value: string = "x";`},
		{name: "Python", filename: "main.py", source: "def hello(name):\n    return f\"Hi {name}\""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if strings.HasSuffix(test.source, "\n") {
				t.Fatal("fixture unexpectedly has a final newline")
			}
			lines := strings.Split(test.source, "\n")
			baseStyles := make([]lipgloss.Style, len(lines))
			for index := range baseStyles {
				baseStyles[index] = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#ffffff")).
					Background(lipgloss.Color("#112233"))
			}

			got := highlightDiffCode(test.filename, lines, baseStyles)
			if plain := ansi.Strip(strings.Join(got, "\n")); plain != test.source {
				t.Fatalf("highlighted text = %q, want %q", plain, test.source)
			}
			colors := make(map[string]struct{})
			for _, color := range trueColorForegroundPattern.FindAllString(strings.Join(got, "\n"), -1) {
				colors[color] = struct{}{}
			}
			if len(colors) < 2 {
				t.Fatalf("highlighted output has %d foreground color, want at least 2: %q", len(colors), got)
			}
		})
	}
}

func TestHighlightDiffCodeFallsBackWithoutChangingText(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	base := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#abcdef")).
		Background(lipgloss.Color("#102030"))
	tests := []struct {
		name     string
		filename string
		lines    []string
	}{
		{
			name:     "unknown language",
			filename: "notes.ec-unknown-language",
			lines:    []string{"const value = 1", "", "plain text"},
		},
		{
			name:     "line exceeds safety bound",
			filename: "large.go",
			lines:    []string{strings.Repeat("x", diffSyntaxMaxLineBytes+1)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			styles := make([]lipgloss.Style, len(test.lines))
			for index := range styles {
				styles[index] = base
			}
			got := highlightDiffCode(test.filename, test.lines, styles)
			if len(got) != len(test.lines) {
				t.Fatalf("highlighted line count = %d, want %d", len(got), len(test.lines))
			}
			for index, line := range test.lines {
				want := base.Render(line)
				if got[index] != want {
					t.Fatalf("line %d = %q, want base rendering %q", index, got[index], want)
				}
			}
		})
	}
}

func TestHighlightDiffCodeFallsBackWhenTokenisingFails(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	originalTokenise := diffSyntaxTokenise
	t.Cleanup(func() { diffSyntaxTokenise = originalTokenise })

	lines := []string{"package main"}
	base := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#abcdef")).
		Background(lipgloss.Color("#102030"))
	tests := []struct {
		name     string
		tokenise func(chroma.Lexer, *chroma.TokeniseOptions, string) ([]chroma.Token, error)
	}{
		{
			name: "error",
			tokenise: func(chroma.Lexer, *chroma.TokeniseOptions, string) ([]chroma.Token, error) {
				return nil, errors.New("test lexer error")
			},
		},
		{
			name: "panic",
			tokenise: func(chroma.Lexer, *chroma.TokeniseOptions, string) ([]chroma.Token, error) {
				panic("test lexer panic")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diffSyntaxTokenise = test.tokenise
			got := highlightDiffCode("main.go", lines, []lipgloss.Style{base})
			if want := base.Render(lines[0]); len(got) != 1 || got[0] != want {
				t.Fatalf("fallback = %q, want %q", got, want)
			}
		})
	}
}

func TestDiffSyntaxCacheRefreshesWhenSourceOrFilenameChanges(t *testing.T) {
	useDiffTrueColor(t)
	originalTokenise := diffSyntaxTokenise
	t.Cleanup(func() { diffSyntaxTokenise = originalTokenise })
	calls := 0
	diffSyntaxTokenise = func(lexer chroma.Lexer, options *chroma.TokeniseOptions, source string) ([]chroma.Token, error) {
		calls++
		return originalTokenise(lexer, options, source)
	}
	var cache diffSyntaxCache
	styles := []lipgloss.Style{lipgloss.NewStyle()}
	for _, test := range []struct {
		filename string
		line     string
		calls    int
	}{
		{"main.go", "package main", 1},
		{"main.go", "package main", 1},
		{"main.go", "package changed", 2},
		{"main.ts", "package changed", 3},
		{"main.ts", "package changed", 3},
	} {
		lines := []string{test.line}
		got := cache.highlight(test.filename, lines, styles)
		if calls != test.calls {
			t.Fatalf("filename=%s source=%s calls=%d, want %d", test.filename, test.line, calls, test.calls)
		}
		if len(got) != 1 || ansi.Strip(got[0]) != test.line {
			t.Fatalf("cache changed source: %q", got)
		}
	}
}

func TestDiffSyntaxCacheDoesNotRetryFailedLexersForUnchangedSource(t *testing.T) {
	originalTokenise := diffSyntaxTokenise
	t.Cleanup(func() { diffSyntaxTokenise = originalTokenise })
	for _, failure := range []string{"error", "panic", "modified text"} {
		t.Run(failure, func(t *testing.T) {
			calls := 0
			diffSyntaxTokenise = func(chroma.Lexer, *chroma.TokeniseOptions, string) ([]chroma.Token, error) {
				calls++
				switch failure {
				case "panic":
					panic("test lexer panic")
				case "modified text":
					return []chroma.Token{{Type: chroma.Text, Value: "changed"}}, nil
				default:
					return nil, errors.New("test lexer error")
				}
			}
			var cache diffSyntaxCache
			for range 2 {
				got := cache.highlight("main.go", []string{"package main"}, []lipgloss.Style{lipgloss.NewStyle()})
				if len(got) != 1 || got[0] != "package main" {
					t.Fatalf("cached fallback changed text: %q", got)
				}
			}
			if calls != 1 {
				t.Fatalf("failed lexer called %d times, want 1", calls)
			}
		})
	}
}

func TestWithinDiffSyntaxBounds(t *testing.T) {
	sharedLine := strings.Repeat("x", diffSyntaxMaxLineBytes)
	totalLimitExceeded := make([]string, diffSyntaxMaxTotalBytes/diffSyntaxMaxLineBytes+1)
	for index := range totalLimitExceeded {
		totalLimitExceeded[index] = sharedLine
	}

	tests := []struct {
		name  string
		lines []string
		want  bool
	}{
		{name: "ordinary", lines: []string{"package main", ""}, want: true},
		{name: "line count limit", lines: make([]string, diffSyntaxMaxLines), want: true},
		{name: "line count exceeded", lines: make([]string, diffSyntaxMaxLines+1), want: false},
		{name: "line size limit", lines: []string{sharedLine}, want: true},
		{name: "line size exceeded", lines: []string{sharedLine + "x"}, want: false},
		{name: "total size exceeded", lines: totalLimitExceeded, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := withinDiffSyntaxBounds(test.lines); got != test.want {
				t.Fatalf("withinDiffSyntaxBounds() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestHighlightDiffCodeASCIIProfileReturnsReadableText(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	lines := []string{"package main", "", `const message = "hello"`}
	styles := make([]lipgloss.Style, len(lines))
	for index := range styles {
		styles[index] = lipgloss.NewStyle().Background(lipgloss.Color("#112233"))
	}

	got := highlightDiffCode("main.go", lines, styles)
	if len(got) != len(lines) {
		t.Fatalf("highlighted line count = %d, want %d", len(got), len(lines))
	}
	for index, line := range lines {
		if got[index] != line {
			t.Fatalf("line %d = %q, want %q", index, got[index], line)
		}
	}
}
