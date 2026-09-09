package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chojs23/ec/internal/gitutil"
)

func withDiffPatchLoader(t *testing.T, loader func(context.Context, string, gitutil.DiffSource, gitutil.DiffFile) ([]byte, error)) {
	t.Helper()
	old := diffPatchLoader
	diffPatchLoader = loader
	t.Cleanup(func() {
		diffPatchLoader = old
	})
}

func withDiffProgram(t *testing.T, fn func(model tea.Model, ctx context.Context) programRunner, run func()) {
	t.Helper()
	old := diffProgram
	diffProgram = fn
	defer func() {
		diffProgram = old
	}()
	run()
}

func TestDiffViewerLoadsPatchesAndNavigatesFiles(t *testing.T) {
	files := []gitutil.DiffFile{
		{Path: "one.txt", Status: "M"},
		{Path: "new.txt", OldPath: "old.txt", Status: "R100"},
	}
	withDiffPatchLoader(t, func(_ context.Context, _ string, _ gitutil.DiffSource, file gitutil.DiffFile) ([]byte, error) {
		return []byte("diff --git a/" + file.Path + " b/" + file.Path + "\n+" + file.Path + "\n"), nil
	})

	model := newDiffModel(context.Background(), "/repo", gitutil.WorkingTreeSource(), files)
	initialLoad := model.Init()
	if initialLoad == nil {
		t.Fatalf("Init() = nil, want initial patch load")
	}
	updated, _ := model.Update(initialLoad())
	model = updated.(diffModel)
	if !strings.Contains(model.patchText, "+one.txt") {
		t.Fatalf("patchText = %q, want first file patch", model.patchText)
	}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(diffModel)
	if model.selected != 1 || command == nil {
		t.Fatalf("selected = %d, command nil = %v, want second file load", model.selected, command == nil)
	}
	updated, _ = model.Update(command())
	model = updated.(diffModel)
	if !strings.Contains(model.patchText, "+new.txt") {
		t.Fatalf("patchText = %q, want second file patch", model.patchText)
	}

	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(diffModel)
	if model.selected != 0 || command != nil {
		t.Fatalf("cached navigation selected = %d, command nil = %v", model.selected, command == nil)
	}
}

func TestDiffViewerRendersDefaultSplitAndScrollsPatch(t *testing.T) {
	model := newDiffModel(
		context.Background(),
		"/repo",
		gitutil.DiffSource{Kind: gitutil.DiffSourceCommit, ShortHash: "abc1234", Subject: "change files"},
		[]gitutil.DiffFile{{Path: "file.txt", Status: "M"}},
	)
	var patch strings.Builder
	patch.WriteString("diff --git a/file.txt b/file.txt\n")
	patch.WriteString("@@ -0,0 +1,30 @@\n")
	for index := 0; index < 30; index++ {
		patch.WriteString("+a long added line for scrolling\n")
	}
	model.setPatch([]byte(patch.String()))

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 90, Height: 12})
	model = updated.(diffModel)
	view := model.View()
	if got := lipgloss.Height(view); got > model.height {
		t.Fatalf("view height = %d, terminal height = %d", got, model.height)
	}
	if !strings.Contains(view, "s: unified/split") || !strings.Contains(view, "q: back") {
		t.Fatalf("view is missing wrapped footer help:\n%s", view)
	}
	for _, text := range []string{"View Diff", "abc1234", "FILES", "OLD file.txt", "NEW file.txt"} {
		if !strings.Contains(view, text) {
			t.Fatalf("view missing %q:\n%s", text, view)
		}
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model = updated.(diffModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(diffModel)
	if model.viewportBefore.YOffset != 1 || model.viewportAfter.YOffset != 1 {
		t.Fatalf("split offsets = %d and %d, want 1", model.viewportBefore.YOffset, model.viewportAfter.YOffset)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	model = updated.(diffModel)
	if model.viewportBefore.YOffset <= 1 || model.viewportAfter.YOffset != model.viewportBefore.YOffset {
		t.Fatalf("split bottom offsets = %d and %d", model.viewportBefore.YOffset, model.viewportAfter.YOffset)
	}
}

func TestDiffViewerSplitLayoutFitsTerminalWithFooter(t *testing.T) {
	model := newDiffModel(
		context.Background(),
		"/repo",
		gitutil.WorkingTreeSource(),
		[]gitutil.DiffFile{{Path: "file.txt", Status: "M"}},
	)
	model.setPatch([]byte("@@ -1 +1 @@\n-old\n+new\n"))
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updated.(diffModel)
	view := model.View()
	if got := lipgloss.Height(view); got != model.height {
		t.Fatalf("split view height = %d, want terminal height %d\n%s", got, model.height, view)
	}
	for lineNumber, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > model.width {
			t.Fatalf("split view line %d width = %d, terminal width = %d\n%s", lineNumber+1, got, model.width, view)
		}
	}
	if !strings.Contains(view, "option+h/l") || !strings.Contains(view, "q: back") {
		t.Fatalf("split view is missing footer help:\n%s", view)
	}
}

func TestDiffViewerFocusControlsFileSelectionAndDiffScrolling(t *testing.T) {
	files := []gitutil.DiffFile{{Path: "one.txt", Status: "M"}, {Path: "two.txt", Status: "M"}}
	withDiffPatchLoader(t, func(_ context.Context, _ string, _ gitutil.DiffSource, file gitutil.DiffFile) ([]byte, error) {
		return []byte("@@ -1 +1 @@\n-old\n+" + file.Path + "\n" + strings.Repeat(" context\n", 20)), nil
	})

	model := newDiffModel(context.Background(), "/repo", gitutil.WorkingTreeSource(), files)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 90, Height: 12})
	model = updated.(diffModel)
	updated, _ = model.Update(model.Init()())
	model = updated.(diffModel)

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(diffModel)
	if model.selected != 1 || command == nil {
		t.Fatalf("explorer j selected = %d, command nil = %v", model.selected, command == nil)
	}
	updated, _ = model.Update(command())
	model = updated.(diffModel)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model = updated.(diffModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(diffModel)
	if model.focus != diffFocusContent || model.selected != 1 || model.viewportBefore.YOffset != 1 || model.viewportAfter.YOffset != 1 {
		t.Fatalf("diff focus state = focus %d selected %d offsets %d and %d", model.focus, model.selected, model.viewportBefore.YOffset, model.viewportAfter.YOffset)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = updated.(diffModel)
	if model.viewMode != diffViewUnified || model.viewportPatch.YOffset != 1 {
		t.Fatalf("unified state = mode %d offset %d, want offset 1", model.viewMode, model.viewportPatch.YOffset)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(diffModel)
	if model.viewportPatch.YOffset != 2 {
		t.Fatalf("unified offset = %d, want 2", model.viewportPatch.YOffset)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = updated.(diffModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = updated.(diffModel)
	if model.focus != diffFocusExplorer || model.selected != 0 {
		t.Fatalf("explorer focus state = focus %d selected %d", model.focus, model.selected)
	}
}

func TestDiffViewerTogglesExplorerAndSplitView(t *testing.T) {
	model := newDiffModel(
		context.Background(),
		"/repo",
		gitutil.WorkingTreeSource(),
		[]gitutil.DiffFile{{Path: "new.txt", OldPath: "old.txt", Status: "R100"}},
	)
	model.setPatch([]byte("diff --git a/old.txt b/new.txt\n--- a/old.txt\n+++ b/new.txt\n@@ -1 +1 @@\n-old\n+new\n"))
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 16})
	model = updated.(diffModel)

	view := model.View()
	if model.viewMode != diffViewSplit || !strings.Contains(view, "OLD old.txt") || !strings.Contains(view, "NEW new.txt") {
		t.Fatalf("default split view state = mode %d\n%s", model.viewMode, view)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = updated.(diffModel)
	if model.viewMode != diffViewUnified || strings.Contains(model.View(), "OLD old.txt") {
		t.Fatalf("unified toggle state = mode %d\n%s", model.viewMode, model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = updated.(diffModel)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model = updated.(diffModel)
	view = model.View()
	if model.explorerVisible || model.focus != diffFocusContent || strings.Contains(view, "FILES") {
		t.Fatalf("closed explorer state = visible %v focus %d\n%s", model.explorerVisible, model.focus, view)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model = updated.(diffModel)
	if !model.explorerVisible || model.focus != diffFocusExplorer || !strings.Contains(model.View(), "FILES") {
		t.Fatalf("reopened explorer state = visible %v focus %d", model.explorerVisible, model.focus)
	}
}

func TestDiffViewerResizesAndScrollsFocusedPanes(t *testing.T) {
	longPath := strings.Repeat("directory/", 8) + "file.txt"
	model := newDiffModel(
		context.Background(),
		"/repo",
		gitutil.WorkingTreeSource(),
		[]gitutil.DiffFile{{Path: longPath, Status: "M"}},
	)
	model.setPatch([]byte("@@ -1 +1 @@\n-" + strings.Repeat("old", 40) + "\n+" + strings.Repeat("new", 40) + "\n"))
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 14})
	model = updated.(diffModel)

	initialWidth := model.calculateLayout().explorer
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}, Alt: true})
	model = updated.(diffModel)
	if got := model.calculateLayout().explorer; got <= initialWidth {
		t.Fatalf("option+l explorer width = %d, want greater than %d", got, initialWidth)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}, Alt: true})
	model = updated.(diffModel)
	if got := model.calculateLayout().explorer; got != initialWidth {
		t.Fatalf("option+h explorer width = %d, want %d", got, initialWidth)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	model = updated.(diffModel)
	if got := model.calculateLayout().explorer; got <= initialWidth {
		t.Fatalf("option+right explorer width = %d, want greater than %d", got, initialWidth)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft, Alt: true})
	model = updated.(diffModel)
	if got := model.calculateLayout().explorer; got != initialWidth {
		t.Fatalf("option+left explorer width = %d, want %d", got, initialWidth)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	model = updated.(diffModel)
	if model.fileXOffset == 0 {
		t.Fatalf("explorer H/L did not change horizontal offset")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model = updated.(diffModel)
	before := model.viewportBefore.View()
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	model = updated.(diffModel)
	if after := model.viewportBefore.View(); after == before {
		t.Fatalf("diff H/L did not change the visible patch")
	}
}

func TestDiffViewerHorizontalInputScrollsWithoutChangingFocus(t *testing.T) {
	longPath := strings.Repeat("directory/", 8) + "file.txt"
	model := newDiffModel(
		context.Background(),
		"/repo",
		gitutil.WorkingTreeSource(),
		[]gitutil.DiffFile{{Path: longPath, Status: "M"}},
	)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 16})
	model = updated.(diffModel)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(diffModel)
	if model.focus != diffFocusExplorer || model.fileXOffset == 0 {
		t.Fatalf("right input changed focus or did not scroll: focus %d offset %d", model.focus, model.fileXOffset)
	}

	beforeOffset := model.fileXOffset
	updated, _ = model.Update(tea.MouseMsg(tea.MouseEvent{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelRight,
	}))
	model = updated.(diffModel)
	if model.focus != diffFocusExplorer || model.fileXOffset <= beforeOffset {
		t.Fatalf("mouse wheel changed focus or did not scroll: focus %d offset %d", model.focus, model.fileXOffset)
	}

	updated, _ = model.Update(tea.MouseMsg(tea.MouseEvent{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelLeft,
	}))
	model = updated.(diffModel)
	if model.focus != diffFocusExplorer || model.fileXOffset != beforeOffset {
		t.Fatalf("mouse wheel left state = focus %d offset %d, want focus %d offset %d", model.focus, model.fileXOffset, diffFocusExplorer, beforeOffset)
	}
}

func TestDiffViewerCapsExplorerWidthToKeepSplitPanesUsable(t *testing.T) {
	model := newDiffModel(
		context.Background(),
		"/repo",
		gitutil.WorkingTreeSource(),
		[]gitutil.DiffFile{{Path: "file.txt", Status: "M"}},
	)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 18})
	model = updated.(diffModel)
	for range 100 {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}, Alt: true})
		model = updated.(diffModel)
	}
	layout := model.calculateLayout()
	if layout.before < diffContentMinWidth || layout.after < diffContentMinWidth {
		t.Fatalf("split widths = %d and %d, want each at least %d", layout.before, layout.after, diffContentMinWidth)
	}
}

func TestDiffViewerShowsPatchTotalsInPaneTitles(t *testing.T) {
	model := newDiffModel(
		context.Background(),
		"/repo",
		gitutil.WorkingTreeSource(),
		[]gitutil.DiffFile{{Path: "file.txt", Status: "M"}},
	)
	patch := "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1,3 +1,3 @@\n-old one\n-old two\n+new one\n context\n@@ -8 +8 @@\n-old three\n+new two\n"
	model.setPatch([]byte(patch))
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 18})
	model = updated.(diffModel)

	if model.deletions != 3 || model.additions != 2 {
		t.Fatalf("patch totals = -%d +%d, want -3 +2", model.deletions, model.additions)
	}
	if strings.Count(model.View(), "-3 +2") != 2 {
		t.Fatalf("default split titles are missing patch totals:\n%s", model.View())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = updated.(diffModel)
	view := model.View()
	if model.viewMode != diffViewUnified || strings.Count(view, "-3 +2") != 1 {
		t.Fatalf("unified title is missing patch totals:\n%s", view)
	}
}

func TestDiffFileStatusStylesDistinguishGitStates(t *testing.T) {
	resetThemeForTest()
	t.Cleanup(resetThemeForTest)

	cases := map[string]lipgloss.Color{
		"M":    "#d29922",
		"?":    "#a371f7",
		"A":    "#3fb950",
		"D":    "#f85149",
		"R100": "#58a6ff",
		"C100": "#58a6ff",
		"U":    "#ff7b72",
	}
	for status, want := range cases {
		if got := diffFileStatusStyle(status).GetForeground(); got != want {
			t.Fatalf("status %q foreground = %q, want %q", status, got, want)
		}
	}
}

func TestDiffFocusedPaneUsesConflictSelectionBorder(t *testing.T) {
	resetThemeForTest()
	t.Cleanup(resetThemeForTest)

	theme := defaultTheme()
	theme.PaneBorder = "200"
	theme.SelectedPaneBorder = "201"
	theme.SelectedSideBorder = "202"
	applyTheme(theme)

	if got := diffPaneStyle(false).GetBorderTopForeground(); got != lipgloss.Color("200") {
		t.Fatalf("unfocused diff border = %q, want default border 200", got)
	}
	if got := diffPaneStyle(true).GetBorderTopForeground(); got != lipgloss.Color("202") {
		t.Fatalf("focused diff border = %q, want conflict selection border 202", got)
	}
}

func TestDiffViewerHandlesEmptyAndFailedPatches(t *testing.T) {
	empty := newDiffModel(context.Background(), "/repo", gitutil.WorkingTreeSource(), nil)
	updated, _ := empty.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	empty = updated.(diffModel)
	if !strings.Contains(empty.View(), "No changed files") {
		t.Fatalf("empty view = %q", empty.View())
	}

	withDiffPatchLoader(t, func(context.Context, string, gitutil.DiffSource, gitutil.DiffFile) ([]byte, error) {
		return nil, errors.New("cannot read patch")
	})
	failed := newDiffModel(
		context.Background(),
		"/repo",
		gitutil.WorkingTreeSource(),
		[]gitutil.DiffFile{{Path: "bad.txt", Status: "M"}},
	)
	updated, _ = failed.Update(failed.Init()())
	failed = updated.(diffModel)
	if !strings.Contains(failed.patchText, "cannot read patch") {
		t.Fatalf("patchText = %q, want load error", failed.patchText)
	}
}

func TestDiffViewerQuitReturnsToSelector(t *testing.T) {
	model := newDiffModel(context.Background(), "/repo", gitutil.WorkingTreeSource(), nil)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	result := updated.(diffModel)
	if !result.back || !result.quitting {
		t.Fatalf("result = %#v, want back and quitting", result)
	}

	withDiffProgram(t, func(model tea.Model, ctx context.Context) programRunner {
		return stubProgram{model: diffModel{back: true}}
	}, func() {
		err := RunDiff(context.Background(), "/repo", gitutil.WorkingTreeSource(), nil)
		if !errors.Is(err, ErrBackToSelector) {
			t.Fatalf("RunDiff error = %v, want ErrBackToSelector", err)
		}
	})
}

func TestRenderDiffPatchKeepsPatchContent(t *testing.T) {
	patch := "diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1 +1 @@\n-old\n+new\x1b[2J\n context\n"
	rendered := renderDiffPatch(patch)
	for _, line := range []string{"diff --git a/a b/a", "--- a/a", "+++ b/a", "@@ -1 +1 @@", "-old", "+new", " context"} {
		if !strings.Contains(rendered, line) {
			t.Fatalf("rendered patch missing %q: %q", line, rendered)
		}
	}
	if strings.Contains(rendered, "\x1b[2J") {
		t.Fatalf("rendered patch contains terminal control sequence: %q", rendered)
	}
}

func TestSplitDiffPatchAlignsRemovedAndAddedLines(t *testing.T) {
	patch := "diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1,4 +1,3 @@\n same\n-old one\n-old two\n+new one\n tail\n"
	rows := parseSplitDiffRows(patch)

	firstChange := -1
	for index, row := range rows {
		if row.before == "-old one" {
			firstChange = index
			break
		}
	}
	if firstChange < 0 || rows[firstChange].after != "+new one" {
		t.Fatalf("rows = %#v, want first removed and added lines aligned", rows)
	}
	if rows[1].before != "--- a/a" || rows[1].after != "+++ b/a" {
		t.Fatalf("file header row = %#v, want old and new paths aligned", rows[1])
	}
	if rows[firstChange+1].before != "-old two" || rows[firstChange+1].after != "" {
		t.Fatalf("second change row = %#v, want removed line and blank", rows[firstChange+1])
	}
	if rows[firstChange+2].before != " tail" || rows[firstChange+2].after != " tail" {
		t.Fatalf("context row = %#v, want shared context", rows[firstChange+2])
	}
}
