package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/chojs23/ec/internal/markers"
)

func TestConflictSyntaxRenderingPreservesDecisionUndoRedoAndSaveBytes(t *testing.T) {
	useDiffTrueColor(t)
	resetThemeForTest()
	t.Cleanup(resetThemeForTest)

	shared := "const shared: string = \"safe\";\n"
	ours := "\tconst choice: string = \"ours\";\n"
	base := "\tconst choice: string = \"base\";\n"
	theirs := "\tconst choice: string = \"theirs\";\n"
	tail := "const terminalText = \"\x1b[31mnot-style\";\n"
	conflicted := []byte(shared +
		"<<<<<<< HEAD\n" + ours +
		"||||||| base\n" + base +
		"=======\n" + theirs +
		">>>>>>> branch\n" + tail)
	wantTheirs := []byte(shared + theirs + tail)

	doc, err := markers.Parse(conflicted)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}

	for _, fullDiff := range []bool{false, true} {
		name := "document fallback"
		if fullDiff {
			name = "full diff"
		}
		t.Run(name, func(t *testing.T) {
			mergedPath := filepath.Join(t.TempDir(), "merged.ts")
			if err := os.WriteFile(mergedPath, conflicted, 0o644); err != nil {
				t.Fatalf("WriteFile error = %v", err)
			}

			m := newModelForDoc(t, doc)
			m.opts.MergedPath = mergedPath
			if fullDiff {
				m.useFullDiff = true
				m.baseLines = []string{strings.TrimSuffix(shared, "\n"), strings.TrimSuffix(base, "\n"), strings.TrimSuffix(tail, "\n")}
				m.oursLines = []string{strings.TrimSuffix(shared, "\n"), strings.TrimSuffix(ours, "\n"), strings.TrimSuffix(tail, "\n")}
				m.theirsLines = []string{strings.TrimSuffix(shared, "\n"), strings.TrimSuffix(theirs, "\n"), strings.TrimSuffix(tail, "\n")}
				var ok bool
				m.conflictRanges, ok = computeConflictRanges(doc, m.baseLines, m.oursLines, m.theirsLines)
				if !ok {
					t.Fatal("fixture should support full-file diff")
				}
			}

			m = updateConflictSyntaxModel(t, m, tea.WindowSizeMsg{Width: 180, Height: 32})
			assertConflictSyntaxView(t, m, "const shared: string")
			assertConflictMergeBytes(t, m, conflicted)

			m = updateConflictSyntaxModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
			assertConflictSyntaxView(t, m, "const shared: string")
			assertConflictMergeBytes(t, m, conflicted)

			m = updateConflictSyntaxModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
			assertConflictSyntaxView(t, m, "const shared: string")
			assertConflictMergeBytes(t, m, wantTheirs)

			m = updateConflictSyntaxModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
			assertConflictSyntaxView(t, m, "const shared: string")
			assertConflictMergeBytes(t, m, conflicted)

			m = updateConflictSyntaxModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
			assertConflictSyntaxView(t, m, "const shared: string")
			assertConflictMergeBytes(t, m, wantTheirs)

			m = updateConflictSyntaxModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
			assertConflictMergeBytes(t, m, wantTheirs)
			saved, err := os.ReadFile(mergedPath)
			if err != nil {
				t.Fatalf("ReadFile error = %v", err)
			}
			if !bytes.Equal(saved, wantTheirs) {
				t.Fatalf("saved bytes = %q, want %q", saved, wantTheirs)
			}
		})
	}
}

func TestConflictSyntaxRefreshesAfterEditorReloadWithoutLosingUndoHistory(t *testing.T) {
	useDiffTrueColor(t)
	resetThemeForTest()
	t.Cleanup(resetThemeForTest)

	conflicted := []byte("shared\n<<<<<<< HEAD\nours\n||||||| base\nbase\n=======\ntheirs\n>>>>>>> branch\ntail\n")
	ours := []byte("shared\nours\ntail\n")
	external := []byte("interface Reloaded {\n\tvalue: string;\n}\nconst terminalText = \"\x1b[31mnot-style\";\n")
	doc, err := markers.Parse(conflicted)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}

	mergedPath := filepath.Join(t.TempDir(), "edited.ts")
	if err := os.WriteFile(mergedPath, conflicted, 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	m := newModelForDoc(t, doc)
	m.opts.MergedPath = mergedPath
	m.opts.AllowMissingBase = true
	m = updateConflictSyntaxModel(t, m, tea.WindowSizeMsg{Width: 180, Height: 32})
	if strings.Contains(m.viewportResult.View(), "38;2;255;123;113") {
		t.Fatalf("initial result unexpectedly contains the TypeScript keyword color: %q", m.viewportResult.View())
	}

	m = updateConflictSyntaxModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	assertConflictMergeBytes(t, m, ours)
	if got := m.undoDepth(); got != 1 {
		t.Fatalf("undo depth after resolution = %d, want 1", got)
	}

	if err := os.WriteFile(mergedPath, external, 0o644); err != nil {
		t.Fatalf("WriteFile external edit error = %v", err)
	}
	m = updateConflictSyntaxModel(t, m, editorFinishedMsg{})
	assertConflictMergeBytes(t, m, external)
	if got := m.undoDepth(); got != 2 {
		t.Fatalf("undo depth after editor reload = %d, want 2", got)
	}
	resultView := m.viewportResult.View()
	if !strings.Contains(resultView, "38;2;255;123;113") {
		t.Fatalf("reloaded result has no TypeScript keyword color: %q", resultView)
	}
	visible := ansi.Strip(resultView)
	if !strings.Contains(visible, "interface Reloaded") || !strings.Contains(visible, "    value: string") {
		t.Fatalf("reloaded result does not contain sanitized edited code: %q", visible)
	}
	if strings.Contains(resultView, "\x1b[31m") {
		t.Fatalf("source terminal escape reached the rendered result: %q", resultView)
	}

	m = updateConflictSyntaxModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	assertConflictMergeBytes(t, m, ours)
	m = updateConflictSyntaxModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	assertConflictMergeBytes(t, m, conflicted)
	m = updateConflictSyntaxModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	assertConflictMergeBytes(t, m, ours)
	m = updateConflictSyntaxModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	assertConflictMergeBytes(t, m, external)
	if !strings.Contains(m.viewportResult.View(), "38;2;255;123;113") {
		t.Fatalf("redo did not restore syntax-highlighted edited result: %q", m.viewportResult.View())
	}
}

func updateConflictSyntaxModel(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	updated, _ := m.Update(msg)
	next := updated.(model)
	if next.err != nil {
		t.Fatalf("Update(%T) error = %v", msg, next.err)
	}
	return next
}

func assertConflictMergeBytes(t *testing.T, m model, want []byte) {
	t.Helper()
	if got := m.state.RenderMerged(); !bytes.Equal(got, want) {
		t.Fatalf("RenderMerged bytes = %q, want %q", got, want)
	}
}

func assertConflictSyntaxView(t *testing.T, m model, visibleText string) {
	t.Helper()
	for name, view := range map[string]string{
		"ours":   m.viewportOurs.View(),
		"result": m.viewportResult.View(),
		"theirs": m.viewportTheirs.View(),
	} {
		if !strings.Contains(view, "38;2;255;123;113") {
			t.Fatalf("%s pane has no TypeScript keyword color: %q", name, view)
		}
		if plain := ansi.Strip(view); !strings.Contains(plain, visibleText) {
			t.Fatalf("%s pane does not contain %q: %q", name, visibleText, plain)
		}
		if strings.Contains(view, "\x1b[31m") {
			t.Fatalf("source terminal escape reached the %s pane: %q", name, view)
		}
	}
}
