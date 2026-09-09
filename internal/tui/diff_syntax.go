package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

const (
	// Highlighting runs while pane content is prepared for the TUI. Bounds keep a
	// generated or minified file from delaying input while preserving normal
	// source files. Files outside these bounds still render without syntax color.
	diffSyntaxMaxLines      = 20_000
	diffSyntaxMaxLineBytes  = 32 * 1024
	diffSyntaxMaxTotalBytes = 2 * 1024 * 1024
)

var (
	// TODO: Allow a light syntax palette for custom light themes.
	diffSyntaxStyle    = styles.Get("github-dark")
	diffSyntaxTokenise = chroma.Tokenise
)

// Keep only the latest bounded source stream. Tokens are independent of row
// styles, so resolver selection changes can reuse lexing without stale colors.
type diffSyntaxCache struct {
	filename string
	source   string
	tokens   []chroma.Token
}

// highlightDiffCode colors a source stream in either the diff or conflict view.
// Each base style remains responsible for its row background. Callers should
// expand tabs before this function so horizontal offsets use stable cell widths.
func highlightDiffCode(filename string, lines []string, baseStyles []lipgloss.Style) []string {
	return (*diffSyntaxCache)(nil).highlight(filename, lines, baseStyles)
}

func (cache *diffSyntaxCache) highlight(filename string, lines []string, baseStyles []lipgloss.Style) (rendered []string) {
	rendered = renderBaseDiffCode(lines, baseStyles)
	if len(lines) == 0 {
		return rendered
	}

	// Syntax color is optional. A lexer panic must not take down the diff viewer.
	defer func() {
		if recover() != nil {
			rendered = renderBaseDiffCode(lines, baseStyles)
		}
	}()

	if !withinDiffSyntaxBounds(lines) || len(baseStyles) != len(lines) {
		return rendered
	}

	source := strings.Join(lines, "\n")
	tokens := cache.tokensFor(filename, source)
	if tokens == nil {
		return rendered
	}

	lineBuilders := make([]strings.Builder, len(lines))
	lineIndex := 0
	for _, token := range tokens {
		value := token.Value
		for {
			newline := strings.IndexByte(value, '\n')
			if newline < 0 {
				if value != "" {
					style := diffSyntaxTokenStyle(baseStyles[lineIndex], token.Type)
					lineBuilders[lineIndex].WriteString(style.Render(value))
				}
				break
			}

			if newline > 0 {
				style := diffSyntaxTokenStyle(baseStyles[lineIndex], token.Type)
				lineBuilders[lineIndex].WriteString(style.Render(value[:newline]))
			}
			lineIndex++
			value = value[newline+1:]
		}
	}

	for index := range lineBuilders {
		rendered[index] = lineBuilders[index].String()
	}
	return rendered
}

func (cache *diffSyntaxCache) tokensFor(filename, source string) []chroma.Token {
	if cache != nil && cache.filename == filename && cache.source == source {
		return cache.tokens
	}
	tokens := tokeniseDiffCode(filename, source)
	if cache != nil {
		// Cache failures too, so a bad lexer cannot stall every selection change.
		*cache = diffSyntaxCache{filename: filename, source: source, tokens: tokens}
	}
	return tokens
}

func tokeniseDiffCode(filename, source string) (tokens []chroma.Token) {
	defer func() {
		if recover() != nil {
			tokens = nil
		}
	}()
	lexer := lexers.Match(filename)
	if lexer == nil {
		return nil
	}
	tokens, err := diffSyntaxTokenise(chroma.Coalesce(lexer), nil, source)
	if err != nil {
		return nil
	}

	// Some lexers normalize their input. Only use syntax output when Chroma
	// returns exactly the text that the renderer supplied.
	var tokenText strings.Builder
	tokenText.Grow(len(source))
	for _, token := range tokens {
		tokenText.WriteString(token.Value)
	}
	if tokenText.String() != source {
		return nil
	}
	return tokens
}

func withinDiffSyntaxBounds(lines []string) bool {
	if len(lines) > diffSyntaxMaxLines {
		return false
	}

	totalBytes := max(len(lines)-1, 0)
	for _, line := range lines {
		if len(line) > diffSyntaxMaxLineBytes {
			return false
		}
		totalBytes += len(line)
		if totalBytes > diffSyntaxMaxTotalBytes {
			return false
		}
	}
	return true
}

func renderBaseDiffCode(lines []string, baseStyles []lipgloss.Style) []string {
	rendered := make([]string, len(lines))
	for index, line := range lines {
		if index < len(baseStyles) {
			rendered[index] = baseStyles[index].Render(line)
			continue
		}
		rendered[index] = line
	}
	return rendered
}

func diffSyntaxTokenStyle(base lipgloss.Style, tokenType chroma.TokenType) lipgloss.Style {
	// Syntax can add emphasis, but must not clear resolver selection or preview flags.
	entry := diffSyntaxStyle.Get(tokenType)
	if entry.Colour.IsSet() {
		base = base.Foreground(lipgloss.Color(entry.Colour.String()))
	}
	if entry.Bold == chroma.Yes {
		base = base.Bold(true)
	}
	if entry.Italic == chroma.Yes {
		base = base.Italic(true)
	}
	return base
}
