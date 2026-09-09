package tui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/chojs23/ec/internal/markers"
)

func TestResolverStateSeparatesSelectionFromAppliedChoice(t *testing.T) {
	for _, width := range []int{40, 54, 56, 58, 60, 80, 120, 210} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := newModelForDoc(t, parseMultiConflictDoc(t))
			m.opts.MergedPath = strings.Repeat("long-directory/", 10) + "source.ts"
			m = updateConflictSyntaxModel(t, m, tea.WindowSizeMsg{Width: width, Height: 40})
			original := append([]byte(nil), m.state.RenderMerged()...)
			steps := []struct {
				key, selected, applied string
			}{
				{selected: "OURS", applied: "NOT YET"},
				{key: "l", selected: "THEIRS", applied: "NOT YET"},
				{key: "a", selected: "THEIRS", applied: "THEIRS"},
				{key: "h", selected: "OURS", applied: "THEIRS"},
				{key: "u", selected: "OURS", applied: "NOT YET"},
				{key: "ctrl+r", selected: "OURS", applied: "THEIRS"},
				{key: "n", selected: "OURS", applied: "NOT YET"},
				{key: "x", selected: "OURS", applied: "NONE"},
				{key: "p", selected: "OURS", applied: "THEIRS"},
			}
			for _, step := range steps {
				if step.key != "" {
					key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(step.key)}
					if step.key == "ctrl+r" {
						key = tea.KeyMsg{Type: tea.KeyCtrlR}
					}
					m = updateConflictSyntaxModel(t, m, key)
				}
				view := ansi.Strip(m.View())
				strip := strings.SplitN(view, "╭", 2)[0]
				if strings.Contains(strip, "HUNK:") {
					t.Fatalf("header should not repeat the hunk status: %q", strip)
				}
				for _, want := range []string{"SELECTED: " + step.selected, "APPLIED: " + step.applied} {
					if !strings.Contains(strip, want) {
						t.Fatalf("key %q: state must remain above panes, missing %q in %q", step.key, want, strip)
					}
				}
				if lipgloss.Width(view) > width || lipgloss.Height(view) != 40 {
					t.Fatalf("key %q: view size = %dx%d, want <=%dx40", step.key, lipgloss.Width(view), lipgloss.Height(view), width)
				}
				if step.applied == "NOT YET" && m.currentConflict == 0 && !bytes.Equal(original, m.state.RenderMerged()) {
					t.Fatal("selection or undo changed unresolved merge bytes")
				}
			}
		})
	}
}

func TestResolverHunkLabelsSeparateSelectionFromResolution(t *testing.T) {
	patch := []byte("<<<<<<< HEAD\nconst value = 1;\n||||||| base\nconst value = 0;\n=======\nconst value = 2;\n>>>>>>> branch\n")
	doc, err := markers.Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	for _, full := range []bool{false, true} {
		t.Run(fmt.Sprintf("full=%t", full), func(t *testing.T) {
			m := newModelForDoc(t, doc)
			if full {
				m.useFullDiff = true
				m.baseLines = []string{"const value = 0;"}
				m.oursLines = []string{"const value = 1;"}
				m.theirsLines = []string{"const value = 2;"}
				var ok bool
				m.conflictRanges, ok = computeConflictRanges(doc, m.baseLines, m.oursLines, m.theirsLines)
				if !ok {
					t.Fatal("fixture should support full-file diff")
				}
			}
			m = updateConflictSyntaxModel(t, m, tea.WindowSizeMsg{Width: 210, Height: 32})
			for _, step := range []struct {
				key, status string
			}{
				{status: "UNRESOLVED"}, {key: "l", status: "UNRESOLVED"},
				{key: "a", status: "RESOLVED"}, {key: "h", status: "RESOLVED"},
				{key: "u", status: "UNRESOLVED"},
			} {
				if step.key != "" {
					m = updateConflictSyntaxModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(step.key)})
				}
				for _, pane := range []struct {
					text     string
					styles   []lipgloss.Style
					selected bool
				}{
					{m.viewportOurs.View(), m.oursLineStyles, m.selectedSide == selectedOurs},
					{m.viewportTheirs.View(), m.theirsLineStyles, m.selectedSide == selectedTheirs},
				} {
					text := ansi.Strip(pane.text)
					if strings.Contains(text, "[SELECTED]") != pane.selected {
						t.Fatalf("only the selected source should be labeled: %q", text)
					}
					want := lineNumberStyle.GetForeground()
					if pane.selected {
						want = selectedHunkMarkerStyle.GetForeground()
					}
					if pane.styles[0].GetForeground() != want {
						t.Fatal("source hunk color does not match selection")
					}
					if pane.styles[len(pane.styles)-1].GetForeground() != lineNumberStyle.GetForeground() {
						t.Fatal("end boundaries should stay neutral")
					}
				}
				if !strings.Contains(ansi.Strip(m.viewportResult.View()), "Conflict 1 ["+step.status+"]") {
					t.Fatal("result hunk is missing its resolution status")
				}
				want := unresolvedLabelStyle.GetForeground()
				if step.status == "RESOLVED" {
					want = statusResolvedStyle.GetForeground()
				}
				if m.resultLineStyles[0].GetForeground() != want {
					t.Fatal("result hunk color does not match resolution")
				}
			}
		})
	}
}
