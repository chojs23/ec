package tui

import (
	"context"
	"image/color"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/chojs23/ec/internal/gitutil"
	"github.com/muesli/termenv"
)

func useDiffTrueColor(t *testing.T) {
	t.Helper()
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
}

func TestDiffViewerChangedRowsFillBackground(t *testing.T) {
	useDiffTrueColor(t)
	resetThemeForTest()
	t.Cleanup(resetThemeForTest)
	theme := defaultTheme()
	theme.AddedBg = "#0000ff"
	theme.RemovedBg = "#ff0000"
	applyTheme(theme)
	removed := color.NRGBA{R: 0xff, A: 0xff}
	added := color.NRGBA{B: 0xff, A: 0xff}
	patch := "@@ -1,22 +1,23 @@\n-const label = \"이전\"; // " + strings.Repeat("old ", 40) +
		"\n-short\n+const label = \"새 코드\"; // " + strings.Repeat("new ", 40) + "\n+\n+extra\n" + strings.Repeat(" context\n", 20)
	model := newDiffModel(context.Background(), "/repo", gitutil.WorkingTreeSource(), []gitutil.DiffFile{{Path: "file.ts", Status: "M"}})
	model.explorerVisible = false
	model.focus = diffFocusContent
	model.setPatch([]byte(patch))
	check := func() {
		t.Helper()
		buffer := cellbuf.NewBuffer(model.width, model.height)
		cellbuf.SetContent(buffer, model.View())
		layout := model.calculateLayout()
		type pane struct {
			x, width, offset int
			backgrounds      []color.Color
		}
		panes := []pane{{2, layout.unified, model.viewportPatch.YOffset, []color.Color{nil, removed, removed, added, added, added, nil}}}
		if model.viewMode == diffViewSplit {
			panes = []pane{
				{2, layout.before, model.viewportBefore.YOffset, []color.Color{nil, removed, removed, nil, nil}},
				{layout.before + paneStyle.GetHorizontalFrameSize() + 2, layout.after, model.viewportAfter.YOffset, []color.Color{nil, added, added, added, nil}},
			}
		}
		for _, pane := range panes {
			for row := max(pane.offset, 1); row < len(pane.backgrounds); row++ {
				y := 3 + row - pane.offset
				if y >= 3+model.diffViewportHeight() {
					break
				}
				for column := 0; column < pane.width; column++ {
					cell := buffer.Cell(pane.x+column, y)
					var got color.Color
					if cell != nil {
						if cell.Width == 0 {
							continue
						}
						got = cell.Style.Bg
					}
					want := pane.backgrounds[row]
					if (got == nil) != (want == nil) || (got != nil && color.NRGBAModel.Convert(got) != color.NRGBAModel.Convert(want)) {
						t.Fatalf("mode %d row %d column %d background = %v, want %v", model.viewMode, row, column, got, want)
					}
				}
			}
		}
	}
	for _, event := range []tea.Msg{
		tea.WindowSizeMsg{Width: 120, Height: 18},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}},
		tea.WindowSizeMsg{Width: 160, Height: 18},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}},
		tea.WindowSizeMsg{Width: 78, Height: 18},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}},
	} {
		updated, _ := model.Update(event)
		model = updated.(diffModel)
		check()
	}
}

func TestDiffPatchHighlightsHunkSidesIndependently(t *testing.T) {
	useDiffTrueColor(t)
	patch := "@@ -1,3 +1,3 @@\n-/* old\n+const label = \"new\";\n shared\n-*/\n+const n = 1;\n@@ -20 +20 @@\n const outside = true;\n"
	rendered := renderDiffPatch(patch, "old.ts", "new.ts")
	foreground := func(line string, column int) color.Color {
		t.Helper()
		buffer := cellbuf.NewBuffer(lipgloss.Width(line), 1)
		cellbuf.SetContent(buffer, line)
		cell := buffer.Cell(column, 0)
		if cell == nil || cell.Style.Fg == nil {
			t.Fatalf("no foreground at %d in %q", column, line)
		}
		return color.NRGBAModel.Convert(cell.Style.Fg)
	}
	if got, want := ansi.Strip(diffLinesText(rendered.unified)), strings.TrimSuffix(patch, "\n"); got != want {
		t.Fatalf("unified patch text = %q, want %q", got, want)
	}
	if len(rendered.before) != 6 || len(rendered.after) != 6 {
		t.Fatalf("split row counts = %d and %d, want 6", len(rendered.before), len(rendered.after))
	}
	if foreground(rendered.before[2].text, 1) == foreground(rendered.after[2].text, 1) {
		t.Fatal("shared context should be a comment only on the old side")
	}
	if foreground(rendered.before[5].text, 1) != foreground(rendered.after[1].text, 1) {
		t.Fatal("next hunk should start fresh and highlight const as a keyword")
	}
	if rendered.unified[3].text != rendered.after[2].text {
		t.Fatal("unified context should use the new side's syntax")
	}
}

func TestDiffViewerUsesEachRenamePathForSyntax(t *testing.T) {
	useDiffTrueColor(t)
	model := newDiffModel(context.Background(), "/repo", gitutil.WorkingTreeSource(), []gitutil.DiffFile{{OldPath: "old.unknown", Path: "new.go", Status: "R100"}})
	model.setPatch([]byte("@@ -1 +1 @@\n-var value = 1\n+var value = 2\n"))
	oldLine := model.renderedPatch.before[1].text
	newLine := model.renderedPatch.after[1].text
	if strings.Count(oldLine, "\x1b[") >= strings.Count(newLine, "\x1b[") {
		t.Fatal("renamed Go source should have syntax colors while unknown old path uses plain text")
	}
	model.setDiffText("Loading diff...")
	if len(model.renderedPatch.unified) != 0 || len(model.renderedPatch.before) != 0 || len(model.renderedPatch.after) != 0 {
		t.Fatal("loading another file must discard the previous line backgrounds")
	}
}

func TestDiffPatchPreservesMetadataAndSanitizesCode(t *testing.T) {
	useDiffTrueColor(t)
	for name, patch := range map[string]string{
		"binary": "diff --git a/image.png b/image.png\nBinary files a/image.png and b/image.png differ\n",
		"rename": "diff --git a/old.ts b/new.ts\nsimilarity index 100%\nrename from old.ts\nrename to new.ts\n",
		"code":   "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-\tvar old = 1\n\\ No newline at end of file\n+\tvar new = \"한글\x1b[2J\"\n\\ No newline at end of file\n",
	} {
		t.Run(name, func(t *testing.T) {
			rendered := renderDiffPatch(patch, "a.go", "a.go")
			raw := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
			for i := range raw {
				raw[i] = strings.ReplaceAll(sanitizeTerminalText(raw[i]), "\t", "    ")
			}
			if got, want := ansi.Strip(diffLinesText(rendered.unified)), strings.Join(raw, "\n"); got != want {
				t.Fatalf("rendered text = %q, want %q", got, want)
			}
			for _, line := range append(rendered.before, rendered.after...) {
				if strings.Contains(line.text, "\x1b[2J") {
					t.Fatal("source control sequence reached renderer")
				}
			}
		})
	}
}

func TestDiffPatchTracksOldAndNewLineNumbers(t *testing.T) {
	for _, test := range []struct {
		name  string
		patch string
		want  [][2]int
	}{
		{"edits and hunks", "@@ -660,3 +661,3 @@\n context\n-old\n+new\n tail\n@@ -700 +900 @@\n-last\n+next\n", [][2]int{{0, 0}, {660, 661}, {661, 0}, {0, 662}, {662, 663}, {0, 0}, {700, 0}, {0, 900}}},
		{"new file", "@@ -0,0 +1,2 @@\n+first\n+second\n", [][2]int{{0, 0}, {0, 1}, {0, 2}}},
		{"deleted file", "@@ -1,2 +0,0 @@\n-first\n-second\n", [][2]int{{0, 0}, {1, 0}, {2, 0}}},
		{"no newline markers", "@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n", [][2]int{{0, 0}, {1, 0}, {0, 0}, {0, 1}, {0, 0}}},
		{"invalid range", "@@ invalid @@\n-old\n+new\n", [][2]int{{0, 0}, {0, 0}, {0, 0}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := renderDiffPatch(test.patch, "file.txt", "file.txt")
			got := make([][2]int, len(result.unified))
			for index, line := range result.unified {
				got[index] = [2]int{line.oldNumber, line.newNumber}
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("line numbers = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDiffViewerKeepsLineNumbersVisibleWhenScrolling(t *testing.T) {
	useDiffTrueColor(t)
	model := newDiffModel(context.Background(), "/repo", gitutil.WorkingTreeSource(), []gitutil.DiffFile{{Path: "file.ts", Status: "M"}})
	model.explorerVisible = false
	model.setPatch([]byte("@@ -661 +662 @@\n-const old = \"" + strings.Repeat("old ", 40) + "\";\n+const next = \"" + strings.Repeat("new ", 40) + "\";\n"))
	for name, mode := range map[string]diffViewMode{"split": diffViewSplit, "unified": diffViewUnified} {
		t.Run(name, func(t *testing.T) {
			model.viewMode = mode
			updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
			model = updated.(diffModel)
			model.resetDiffScroll()
			layout := model.calculateLayout()
			rows := func() []string {
				if mode == diffViewSplit {
					return strings.Split(ansi.Strip(renderDiffViewport(model.viewportBefore, model.renderedPatch.before, diffNumbersOld, layout.before)), "\n")
				}
				return strings.Split(ansi.Strip(renderDiffViewport(model.viewportPatch, model.renderedPatch.unified, diffNumbersBoth, layout.unified)), "\n")
			}
			before := rows()[1]
			model.scrollDiffHorizontal(12)
			after := rows()[1]
			prefix := "661 - "
			if mode == diffViewUnified {
				prefix = "661     - "
			}
			if !strings.HasPrefix(before, prefix) || !strings.HasPrefix(after, prefix) {
				t.Fatalf("gutter moved: before %q, after %q", before, after)
			}
			if before == after {
				t.Fatal("code did not scroll independently of gutter")
			}
		})
	}
}

func TestDiffViewerLineNumbersFitNarrowPanes(t *testing.T) {
	for _, width := range []int{40, 60, 80} {
		model := newDiffModel(context.Background(), "/repo", gitutil.WorkingTreeSource(), []gitutil.DiffFile{{Path: "file.ts", Status: "M"}})
		model.setPatch([]byte("@@ -99999999 +99999999 @@\n-old\n+new\n"))
		updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: 20})
		model = updated.(diffModel)
		if got := lipgloss.Width(model.View()); got != width {
			t.Fatalf("rendered width = %d, terminal = %d", got, width)
		}
	}
}

func TestDiffGuttersOnlyMeasureVisibleLineNumbers(t *testing.T) {
	rendered := renderDiffPatch("@@ -1 +100000000 @@\n same\n", "file.txt", "file.txt")
	if got := diffGutterWidth(rendered.before, diffNumbersOld, 10); got != 4 {
		t.Fatalf("old gutter width = %d, want 4 for one digit", got)
	}
	if got := diffGutterWidth(rendered.after, diffNumbersNew, 10); got != 0 {
		t.Fatalf("new gutter width = %d, want hidden because it cannot fit", got)
	}
	if got := diffGutterWidth(rendered.unified, diffNumbersBoth, 40); got != 22 {
		t.Fatalf("unified gutter width = %d, want 22 for both nine-digit columns", got)
	}
}

func TestDiffGuttersTreatRawConflictMarkersAsOrdinaryContent(t *testing.T) {
	patch := "@@ -10,5 +10,7 @@\n <<<<<<< HEAD\n-old value that scrolls beyond the pane\n+new value that scrolls beyond the pane\n =======\n theirs value\n >>>>>>> branch\n+\n+added outside that scrolls beyond the pane\n"
	rendered := renderDiffPatch(patch, "file.ts", "file.ts")
	if got, want := ansi.Strip(diffLinesText(rendered.unified)), strings.TrimSuffix(patch, "\n"); got != want {
		t.Fatalf("raw conflict text = %q, want %q", got, want)
	}

	marker := func(line diffRenderedLine, mode diffNumberMode) string {
		t.Helper()
		return strings.TrimSpace(ansi.Strip(renderDiffGutter(line, 12, mode)))
	}
	assertMarker := func(line diffRenderedLine, mode diffNumberMode, want string) {
		t.Helper()
		got := marker(line, mode)
		if !strings.HasSuffix(got, want) {
			t.Fatalf("gutter %q does not end in marker %q", got, want)
		}
	}

	for _, index := range []int{1, 4, 5, 6} {
		if got := marker(rendered.unified[index], diffNumbersBoth); strings.HasSuffix(got, "+") || strings.HasSuffix(got, "-") {
			t.Fatalf("raw conflict context has a change marker: %q", got)
		}
	}
	assertMarker(rendered.unified[2], diffNumbersBoth, "-")
	for _, index := range []int{3, 7, 8} {
		assertMarker(rendered.unified[index], diffNumbersBoth, "+")
	}

	assertStickyMarker := func(lines []diffRenderedLine, mode diffNumberMode, kind diffLineKind, want string) {
		t.Helper()
		index := -1
		for candidate, line := range lines {
			if line.kind == kind {
				index = candidate
				break
			}
		}
		if index < 0 {
			t.Fatalf("missing line kind %d", kind)
		}
		view := viewport.New(36, len(lines))
		view.SetContent(diffViewportText(lines))
		before := strings.Split(ansi.Strip(renderDiffViewport(view, lines, mode, 36)), "\n")[index]
		view.SetXOffset(8)
		after := strings.Split(ansi.Strip(renderDiffViewport(view, lines, mode, 36)), "\n")[index]
		prefix := ansi.Strip(renderDiffGutter(lines[index], diffGutterWidth(lines, mode, 36), mode))
		if !strings.HasPrefix(before, prefix) || !strings.HasPrefix(after, prefix) || before == after {
			t.Fatalf("%s gutter did not stay fixed while code scrolled: before %q, after %q", want, before, after)
		}
		assertMarker(lines[index], mode, want)
	}
	assertStickyMarker(rendered.unified, diffNumbersBoth, diffLineRemoved, "-")
	assertStickyMarker(rendered.unified, diffNumbersBoth, diffLineAdded, "+")
	assertStickyMarker(rendered.before, diffNumbersOld, diffLineRemoved, "-")
	assertStickyMarker(rendered.after, diffNumbersNew, diffLineAdded, "+")
}
