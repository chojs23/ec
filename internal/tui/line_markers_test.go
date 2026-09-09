package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/chojs23/ec/internal/markers"
)

func TestConflictChangesRemainRelativeToBase(t *testing.T) {
	seg := markers.ConflictSegment{
		Base:   []byte("call(\n  socket,\n  chatData,\n"),
		Ours:   []byte("const service = load();\nservice.send(\n"),
		Theirs: []byte("if (enabled) log();\ncall(\n  socket,\n  chatData,\n"),
	}
	_, theirs := conflictEntries(seg)
	for _, entry := range theirs {
		if entry.text == "  chatData," && entry.category != categoryDefault {
			t.Fatalf("unchanged chatData classified as %v, want context", entry.category)
		}
	}
	doc := markers.Document{Segments: []markers.Segment{seg}, Conflicts: []markers.ConflictRef{{SegmentIndex: 0}}}
	fallback, _ := buildResultLines(doc, 0, selectedTheirs, nil, nil)
	preview, forced, ranges := buildResultPreviewLines(doc, selectedTheirs, nil, 0, nil)
	full, _ := buildResultLinesFromEntries(diffEntries(splitLogicalLines(seg.Base), preview), ranges, 0, forced)
	for _, lines := range [][]lineInfo{fallback, full} {
		for _, line := range lines {
			if line.text == "  chatData," && line.category != categoryDefault {
				t.Fatalf("result marks unchanged chatData as %v", line.category)
			}
			if line.text == "if (enabled) log();" && line.category != categoryAdded {
				t.Fatalf("result marks added line as %v, want addition", line.category)
			}
		}
	}
}

func TestConflictLineMarkersMatchChangeCategories(t *testing.T) {
	useDiffTrueColor(t)
	lines := []lineInfo{
		{text: "const same = 1;", category: categoryDefault},
		{text: "Conflict 1", category: categoryInsertMarker, synthetic: true},
		{text: "-oldValue", category: categoryRemoved},
		{text: "+newValue", category: categoryAdded},
		{text: "const replacement = 3;", category: categoryModified},
		{text: "", category: categoryAdded},
		{text: "Empty result", category: categoryInsertMarker, synthetic: true},
		{text: "End conflict 1", category: categoryInsertMarker, synthetic: true},
	}
	want := []string{"1   const same = 1;", "    Conflict 1", "  - -oldValue", "2 + +newValue", "3 + const replacement = 3;", "4 + ", "    Empty result", "    End conflict 1"}
	rendered, _ := renderLines(lines, "source.ts", &conflictSyntaxCache{})
	rows := strings.Split(ansi.Strip(rendered), "\n")
	if len(rows) != len(want) {
		t.Fatalf("row count = %d, want %d", len(rows), len(want))
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("row %d = %q, want %q", i, rows[i], want[i])
		}
	}
}

func TestConflictPanesShowChangeMarkersWithoutChangingSource(t *testing.T) {
	useDiffTrueColor(t)
	resetThemeForTest()
	t.Cleanup(resetThemeForTest)
	patch := []byte("const shared = 1;\n<<<<<<< HEAD\nconst value = 2;\nconst added = true;\n||||||| base\nconst value = 0;\n=======\nconst value = 3;\n>>>>>>> branch\n")
	doc, err := markers.Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	for _, full := range []bool{false, true} {
		t.Run(fmt.Sprintf("full diff=%t", full), func(t *testing.T) {
			m := newModelForDoc(t, doc)
			m.opts.MergedPath = "source.ts"
			if full {
				m.useFullDiff = true
				m.baseLines = []string{"const shared = 1;", "const value = 0;"}
				m.oursLines = []string{"const shared = 1;", "const value = 2;", "const added = true;"}
				m.theirsLines = []string{"const shared = 1;", "const value = 3;"}
				var ok bool
				m.conflictRanges, ok = computeConflictRanges(doc, m.baseLines, m.oursLines, m.theirsLines)
				if !ok {
					t.Fatal("fixture should support full-file diff")
				}
			}
			m = updateConflictSyntaxModel(t, m, tea.WindowSizeMsg{Width: 180, Height: 32})
			ours := ansi.Strip(m.viewportOurs.View())
			for _, text := range []string{"  - const value = 0;", "2 + const value = 2;", "3 + const added = true;"} {
				if !strings.Contains(ours, text) {
					t.Fatalf("ours has no %q: %q", text, ours)
				}
			}
			if !strings.Contains(ansi.Strip(m.viewportResult.View()), "2 + const value = 2;") {
				t.Fatal("result has no added marker")
			}
			if !strings.Contains(ansi.Strip(m.viewportTheirs.View()), "2 + const value = 3;") {
				t.Fatal("theirs has no added marker")
			}
			for _, key := range []string{"h", "l"} {
				m = updateConflictSyntaxModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
				for _, pane := range []string{m.viewportOurs.View(), m.viewportResult.View(), m.viewportTheirs.View()} {
					text := ansi.Strip(pane)
					if strings.Count(text, "Conflict 1") != 1 || strings.Count(text, "End conflict 1") != 1 {
						t.Fatalf("each pane must label the current block regardless of selected side: %q", text)
					}
				}
			}
			assertConflictMergeBytes(t, m, patch)
		})
	}
}
