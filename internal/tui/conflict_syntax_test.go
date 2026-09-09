package tui

import (
	"bytes"
	"image/color"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/chojs23/ec/internal/markers"
)

func TestConflictSyntaxReusesTokensForUnchangedSource(t *testing.T) {
	useDiffTrueColor(t)
	originalTokenise := diffSyntaxTokenise
	t.Cleanup(func() { diffSyntaxTokenise = originalTokenise })
	calls := 0
	diffSyntaxTokenise = func(lexer chroma.Lexer, options *chroma.TokeniseOptions, source string) ([]chroma.Token, error) {
		calls++
		return originalTokenise(lexer, options, source)
	}
	// Identical alternatives let h/l change selection styling without changing
	// the result's source. Deleted base rows exercise the second lexical stream.
	hunk := "<<<<<<< HEAD\nconst value = 1;\n||||||| base\nconst value = 0;\n=======\nconst value = 1;\n>>>>>>> branch\n"
	doc, err := markers.Parse([]byte(hunk + "const shared = 2;\n" + hunk))
	if err != nil {
		t.Fatal(err)
	}
	for _, full := range []bool{false, true} {
		m := newModelForDoc(t, doc)
		m.opts.MergedPath = "source.ts"
		if full {
			m.useFullDiff = true
			m.baseLines = []string{"const value = 0;", "const shared = 2;", "const value = 0;"}
			m.oursLines = []string{"const value = 1;", "const shared = 2;", "const value = 1;"}
			m.theirsLines = m.oursLines
			var ok bool
			m.conflictRanges, ok = computeConflictRanges(doc, m.baseLines, m.oursLines, m.theirsLines)
			if !ok {
				t.Fatal("fixture should support full-file diff")
			}
		}
		m = updateConflictSyntaxModel(t, m, tea.WindowSizeMsg{Width: 180, Height: 32})
		initialCalls := calls
		if initialCalls == 0 {
			t.Fatal("initial render did not tokenize")
		}
		for _, key := range []rune{'l', 'h', 'n', 'p', 'l', 'h'} {
			m = updateConflictSyntaxModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
			if calls != initialCalls {
				t.Fatalf("full=%t key=%c repeated tokenization: calls=%d, want %d", full, key, calls, initialCalls)
			}
		}
	}
}

func TestConflictViewHighlightsAllPanes(t *testing.T) {
	useDiffTrueColor(t)
	patch := []byte("const shared = 1;\n<<<<<<< HEAD\nlet value = \"ours\";\n||||||| base\nlet value = \"base\";\n=======\nlet value = \"theirs\";\n>>>>>>> branch\n")
	doc, err := markers.Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	for _, full := range []bool{false, true} {
		name := "document"
		if full {
			name = "full diff"
		}
		t.Run(name, func(t *testing.T) {
			m := newModelForDoc(t, doc)
			m.opts.MergedPath = "source.ts"
			if full {
				m.useFullDiff = true
				m.baseLines = []string{"const shared = 1;", `let value = "base";`}
				m.oursLines = []string{"const shared = 1;", `let value = "ours";`}
				m.theirsLines = []string{"const shared = 1;", `let value = "theirs";`}
				var ok bool
				m.conflictRanges, ok = computeConflictRanges(doc, m.baseLines, m.oursLines, m.theirsLines)
				if !ok {
					t.Fatal("fixture should support full-file diff")
				}
			}
			original := append([]byte(nil), m.state.RenderMerged()...)
			next, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 32})
			m = next.(model)
			for name, pane := range map[string]viewport.Model{"ours": m.viewportOurs, "result": m.viewportResult, "theirs": m.viewportTheirs} {
				if !strings.Contains(pane.View(), "38;2;255;123;113") {
					t.Fatalf("%s pane has no syntax keyword foreground: %q", name, pane.View())
				}
			}
			if !bytes.Equal(original, m.state.RenderMerged()) {
				t.Fatal("syntax rendering changed merge bytes")
			}
		})
	}
}

func TestConflictSyntaxSeparatesDeletedBaseAndSkipsUIRows(t *testing.T) {
	useDiffTrueColor(t)
	lines := []lineInfo{
		{text: "/* shared", category: categoryDefault},
		{text: "removed comment", category: categoryRemoved},
		{text: "*/", category: categoryRemoved},
		{text: "*/", category: categoryInsertMarker},
		{text: "*/", category: categoryDefault, synthetic: true},
		{text: "const stillComment = 1;", category: categoryAdded},
		{text: "*/", category: categoryDefault},
		{text: "const keyword = 2;", category: categoryAdded},
	}
	styles := make([]lipgloss.Style, len(lines))
	rendered := highlightConflictCode("source.ts", lines, styles, &conflictSyntaxCache{})
	for index, line := range lines {
		if got := ansi.Strip(rendered[index]); got != line.text {
			t.Fatalf("line %d = %q, want %q", index, got, line.text)
		}
	}
	for _, index := range []int{1, 5} {
		if !strings.Contains(rendered[index], "38;2;139;147;158") {
			t.Fatalf("line %d must retain multiline comment color: %q", index, rendered[index])
		}
	}
	for _, index := range []int{3, 4} {
		if rendered[index] != styles[index].Render(lines[index].text) {
			t.Fatalf("UI row %d was syntax highlighted", index)
		}
	}
	if !strings.Contains(rendered[7], "38;2;255;123;113") {
		t.Fatal("source lexer did not leave the comment")
	}
}

func TestConflictResultTextHasNoUnderlineOrDimming(t *testing.T) {
	useDiffTrueColor(t)
	resetThemeForTest()
	t.Cleanup(resetThemeForTest)
	patch := []byte("const shared = 1;\n<<<<<<< HEAD\nconst value = 2;\n||||||| base\nconst value = 0;\n=======\nconst value = 3;\n>>>>>>> branch\n")
	doc, err := markers.Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	for _, full := range []bool{false, true} {
		for _, filename := range []string{"source.ts", "source.unknown"} {
			m := newModelForDoc(t, doc)
			m.opts.MergedPath = filename
			if full {
				m.useFullDiff = true
				m.baseLines = []string{"const shared = 1;", "const value = 0;"}
				m.oursLines = []string{"const shared = 1;", "const value = 2;"}
				m.theirsLines = []string{"const shared = 1;", "const value = 3;"}
				var ok bool
				m.conflictRanges, ok = computeConflictRanges(doc, m.baseLines, m.oursLines, m.theirsLines)
				if !ok {
					t.Fatal("fixture should support full-file diff")
				}
			}
			for _, msg := range []tea.Msg{
				tea.WindowSizeMsg{Width: 180, Height: 32},
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}},
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}},
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}},
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}},
			} {
				m = updateConflictSyntaxModel(t, m, msg)
				view := renderConflictViewport(m.viewportResult, m.resultLineStyles)
				if !strings.Contains(ansi.Strip(view), "const shared = 1;") {
					t.Fatal("result lost source text")
				}
				if filename == "source.ts" && !strings.Contains(view, "38;2;255;123;113") {
					t.Fatal("result lost syntax highlighting")
				}
				buffer := cellbuf.NewBuffer(m.viewportResult.Width, m.viewportResult.Height)
				cellbuf.SetContent(buffer, view)
				for y := 0; y < m.viewportResult.Height; y++ {
					for x := 0; x < m.viewportResult.Width; x++ {
						cell := buffer.Cell(x, y)
						if cell != nil && (cell.Style.Attrs&cellbuf.FaintAttr != 0 || cell.Style.UlStyle != cellbuf.NoUnderline) {
							t.Fatalf("full=%t filename=%s msg=%v: result cell (%d,%d) is dim or underlined", full, filename, msg, x, y)
						}
					}
				}
			}
		}
	}
}

func TestConflictSyntaxCacheKeepsCurrentChangeStyle(t *testing.T) {
	useDiffTrueColor(t)
	resetThemeForTest()
	t.Cleanup(resetThemeForTest)
	theme := defaultTheme()
	theme.AddedBg = "#0000ff"
	applyTheme(theme)
	lines := []lineInfo{{text: `const text = "value";`, category: categoryAdded, selected: true}}
	cache := &conflictSyntaxCache{}
	rendered, styles := renderLines(lines, "source.ts", cache)
	buffer := cellbuf.NewBuffer(60, 1)
	cellbuf.SetContent(buffer, rendered)
	start := strings.Index(ansi.Strip(rendered), "const")
	first := buffer.Cell(start, 0)
	if first == nil || first.Style.Bg == nil || color.NRGBAModel.Convert(first.Style.Bg) != (color.NRGBA{B: 255, A: 255}) {
		t.Fatal("syntax lost the added background")
	}
	if first.Style.Attrs&cellbuf.FaintAttr != 0 || first.Style.UlStyle != cellbuf.NoUnderline {
		t.Fatalf("selected code should have normal text style: %+v", first.Style)
	}
	if !strings.Contains(rendered, "38;2;255;123;113") || !strings.HasPrefix(ansi.Strip(rendered), "1 + const") {
		t.Fatal("syntax or change gutter lost")
	}
	view := viewport.New(60, 1)
	view.SetContent(rendered)
	cellbuf.SetContent(buffer, renderConflictViewport(view, styles))
	padding := buffer.Cell(59, 0)
	if padding == nil || padding.Style.Bg == nil || color.NRGBAModel.Convert(padding.Style.Bg) != (color.NRGBA{B: 255, A: 255}) {
		t.Fatal("padding lost the added background")
	}
	if padding.Style.UlStyle != cellbuf.NoUnderline || padding.Style.Attrs != 0 {
		t.Fatalf("text emphasis leaked to trailing padding: %+v", padding.Style)
	}

	// Changing a row's diff category must not leave stale color in cached tokens.
	lines[0].category = categoryDefault
	rendered, _ = renderLines(lines, "source.ts", cache)
	cellbuf.SetContent(buffer, rendered)
	first = buffer.Cell(start, 0)
	if first == nil || first.Style.Bg != nil {
		t.Fatalf("cached syntax kept a stale change background: %+v", first)
	}
}

func TestConflictBackgroundsFillAfterScrollAndResize(t *testing.T) {
	useDiffTrueColor(t)
	resetThemeForTest()
	t.Cleanup(resetThemeForTest)
	theme := defaultTheme()
	theme.AddedBg = "#00ff00"
	theme.RemovedBg = "#ff0000"
	applyTheme(theme)
	lines := []lineInfo{
		{text: "const old = 1;", category: categoryRemoved},
		{text: "", category: categoryAdded},
		{text: "const name = \"한글\"; // " + strings.Repeat("long ", 40), category: categoryDefault},
	}
	rendered, styles := renderLines(lines, "source.ts", &conflictSyntaxCache{})
	view := viewport.New(40, 3)
	view.SetContent(rendered)
	for _, width := range []int{40, 20, 80} {
		view.Width = width
		for _, offset := range []int{0, 12, 80} {
			view.SetXOffset(offset)
			shown := renderConflictViewport(view, styles)
			if lipgloss.Width(shown) != width {
				t.Fatalf("rendered width=%d, want %d", lipgloss.Width(shown), width)
			}
			buffer := cellbuf.NewBuffer(width, 3)
			cellbuf.SetContent(buffer, shown)
			for row, want := range []color.NRGBA{{R: 255, A: 255}, {G: 255, A: 255}} {
				// The line-number gutter keeps its own styling.
				for column := 4; column < width; column++ {
					cell := buffer.Cell(column, row)
					if cell == nil || cell.Style.Bg == nil || color.NRGBAModel.Convert(cell.Style.Bg) != want {
						t.Fatalf("row %d column %d lost background at offset %d width %d", row, column, offset, width)
					}
				}
			}
			if cell := buffer.Cell(width-1, 2); cell != nil && cell.Style.Bg != nil {
				t.Fatal("changed background leaked to context")
			}
		}
	}
}

func TestConflictRenderingSanitizesOnlyDisplayCopies(t *testing.T) {
	useDiffTrueColor(t)
	lines := []lineInfo{{text: "\tconst unsafe = \"hello\x1b[2J\";"}}
	original := lines[0].text
	styles := []lipgloss.Style{lipgloss.NewStyle()}
	for _, filename := range []string{"source.ts", "source.unknown"} {
		result := highlightConflictCode(filename, lines, styles, &conflictSyntaxCache{})
		if got, want := ansi.Strip(result[0]), "    "+strings.ReplaceAll(original[1:], "\x1b", "?"); got != want {
			t.Fatalf("display = %q, want %q", got, want)
		}
		if strings.Contains(result[0], "\x1b[2J") {
			t.Fatal("source terminal escape reached display")
		}
	}
	if lines[0].text != original {
		t.Fatal("highlighting mutated source line")
	}
}
