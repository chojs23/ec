package tui

import (
	"testing"

	"github.com/chojs23/ec/internal/markers"
)

func TestConnectorForResult(t *testing.T) {
	if got := connectorForResult(true, false); got != "v" {
		t.Fatalf("connectorForResult(resolved=true) = %q, want v", got)
	}
	if got := connectorForResult(false, true); got != "|" {
		t.Fatalf("connectorForResult(selected=true) = %q, want |", got)
	}
	if got := connectorForResult(false, false); got != " " {
		t.Fatalf("connectorForResult(default) = %q, want space", got)
	}
}

func TestBuildResultLinesManualResolved(t *testing.T) {
	input := []byte("start\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nend\n")
	doc, err := markers.Parse(input)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	manual := map[int][]byte{0: []byte("manual\n")}
	lines, _ := buildResultLines(doc, 0, selectedOurs, manual)
	if len(lines) == 0 {
		t.Fatalf("expected lines")
	}
	found := false
	for _, line := range lines {
		if line.category == categoryResolved {
			found = true
			if line.connector != "v" {
				t.Fatalf("connector = %q, want v", line.connector)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected resolved lines")
	}
}

func TestApplyMergedResolutionsManualHunk(t *testing.T) {
	diff3 := []byte("header\n<<<<<<< HEAD\nours1\n||||||| base\nbase1\n=======\ntheirs1\n>>>>>>> branch\nmid\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> branch\nfooter\n")
	doc, err := markers.Parse(diff3)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}

	merged := []byte("header\nmanual1\nmid\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> branch\nfooter\n")
	updated, manual, err := applyMergedResolutions(doc, merged)
	if err != nil {
		t.Fatalf("applyMergedResolutions error = %v", err)
	}
	if len(manual) != 1 {
		t.Fatalf("manualResolved count = %d, want 1", len(manual))
	}
	if _, ok := manual[0]; !ok {
		t.Fatalf("manualResolved missing conflict 0")
	}
	ref := updated.Conflicts[0]
	seg := updated.Segments[ref.SegmentIndex].(markers.ConflictSegment)
	if seg.Resolution != markers.ResolutionUnset {
		t.Fatalf("conflict 0 resolution = %q, want unset", seg.Resolution)
	}
}

func TestDiffEntriesCategories(t *testing.T) {
	base := []string{"line1", "line2"}
	side := []string{"line1", "line2-mod"}
	entries := diffEntries(base, side)
	if len(entries) != 3 {
		t.Fatalf("entries len = %d, want 3", len(entries))
	}
	if entries[1].category != categoryRemoved {
		t.Fatalf("removed category = %v, want removed", entries[1].category)
	}
	if entries[2].category != categoryModified {
		t.Fatalf("modified category = %v, want modified", entries[2].category)
	}
	if entries[2].baseIndex != 1 {
		t.Fatalf("modified baseIndex = %d, want 1", entries[2].baseIndex)
	}

	base = []string{"line1"}
	side = []string{"line1", "line2"}
	entries = diffEntries(base, side)
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
	if entries[1].category != categoryAdded {
		t.Fatalf("added category = %v, want added", entries[1].category)
	}
}

func TestMarkConflictedInRanges(t *testing.T) {
	ours := []lineEntry{{text: "same", category: categoryDefault, baseIndex: 0}, {text: "ours", category: categoryDefault, baseIndex: 1}}
	theirs := []lineEntry{{text: "same", category: categoryDefault, baseIndex: 0}, {text: "theirs", category: categoryDefault, baseIndex: 1}}
	ranges := []conflictRange{{baseStart: 0, baseEnd: 1}}

	markConflictedInRanges(&ours, &theirs, ranges)
	if ours[0].category != categoryDefault || theirs[0].category != categoryDefault {
		t.Fatalf("unexpected conflict marking for base index 0")
	}
	if ours[1].category != categoryDefault || theirs[1].category != categoryDefault {
		t.Fatalf("unexpected conflict marking outside range")
	}
}

func TestBuildPaneLinesFromEntriesMarkers(t *testing.T) {
	data := []byte("start\n<<<<<<< HEAD\nours\n||||||| base\nbase\n=======\ntheirs\n>>>>>>> branch\nend\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	baseLines := []string{"start", "base", "end"}
	oursLines := []string{"start", "ours", "end"}
	theirsLines := []string{"start", "theirs", "end"}

	ranges, ok := computeConflictRanges(doc, baseLines, oursLines, theirsLines)
	if !ok {
		t.Fatalf("computeConflictRanges failed")
	}

	entries := diffEntries(baseLines, oursLines)
	lines, _ := buildPaneLinesFromEntries(doc, paneOurs, 0, selectedOurs, entries, ranges)

	foundStart := false
	foundEnd := false
	for _, line := range lines {
		switch line.text {
		case ">> selected hunk start (ours) >>":
			foundStart = line.category == categoryInsertMarker && line.selected
		case ">> selected hunk end >>":
			foundEnd = line.category == categoryInsertMarker && line.selected
		}
	}
	if !foundStart || !foundEnd {
		t.Fatalf("expected selected hunk markers in pane lines")
	}
}

func TestBuildResultLinesFromEntriesUnresolvedRange(t *testing.T) {
	entries := []lineEntry{{text: "ours", category: categoryAdded, baseIndex: -1}}
	ranges := []resultRange{{start: 0, end: 1, resolved: false}}
	lines, _ := buildResultLinesFromEntries(entries, ranges, 0, map[int]lineCategory{})
	if len(lines) != 1 {
		t.Fatalf("lines len = %d, want 1", len(lines))
	}
	if lines[0].category != categoryConflicted {
		t.Fatalf("category = %v, want conflicted", lines[0].category)
	}
	if !lines[0].dim {
		t.Fatalf("expected dim line for unresolved range")
	}
	if lines[0].connector != "|" {
		t.Fatalf("connector = %q, want |", lines[0].connector)
	}
}

func TestBuildResultPreviewLinesUsesSelection(t *testing.T) {
	doc := markers.Document{
		Segments: []markers.Segment{
			markers.TextSegment{Bytes: []byte("start\n")},
			markers.ConflictSegment{
				Ours:   []byte("ours\n"),
				Base:   []byte("base\n"),
				Theirs: []byte("theirs\n"),
			},
			markers.TextSegment{Bytes: []byte("end\n")},
		},
		Conflicts: []markers.ConflictRef{{SegmentIndex: 1}},
	}

	lines, forced, ranges := buildResultPreviewLines(doc, selectedTheirs, nil)
	if len(forced) != 0 {
		t.Fatalf("forced len = %d, want 0", len(forced))
	}
	if len(ranges) != 1 {
		t.Fatalf("ranges len = %d, want 1", len(ranges))
	}
	if ranges[0].resolved {
		t.Fatalf("range resolved = true, want false")
	}
	if len(lines) != 3 || lines[1] != "theirs" {
		t.Fatalf("lines = %v, want theirs in conflict output", lines)
	}
}

func TestBuildResultPreviewLinesManualAndNone(t *testing.T) {
	doc := markers.Document{
		Segments: []markers.Segment{
			markers.TextSegment{Bytes: []byte("start\n")},
			markers.ConflictSegment{
				Ours:   []byte("ours\n"),
				Base:   []byte("base\n"),
				Theirs: []byte("theirs\n"),
			},
			markers.TextSegment{Bytes: []byte("middle\n")},
			markers.ConflictSegment{
				Ours:       []byte("o2\n"),
				Theirs:     []byte("t2\n"),
				Resolution: markers.ResolutionNone,
			},
			markers.TextSegment{Bytes: []byte("end\n")},
		},
		Conflicts: []markers.ConflictRef{{SegmentIndex: 1}, {SegmentIndex: 3}},
	}

	manual := map[int][]byte{0: []byte("manual\n")}
	lines, forced, ranges := buildResultPreviewLines(doc, selectedOurs, manual)
	if len(lines) != 5 {
		t.Fatalf("lines len = %d, want 5", len(lines))
	}
	if lines[1] != "manual" {
		t.Fatalf("manual line = %q, want manual", lines[1])
	}
	if lines[3] != "[unresolved conflict]" {
		t.Fatalf("placeholder line = %q, want unresolved conflict", lines[3])
	}
	if forced[3] != categoryConflicted {
		t.Fatalf("forced category = %v, want conflicted", forced[3])
	}
	if len(ranges) != 2 {
		t.Fatalf("ranges len = %d, want 2", len(ranges))
	}
	if !ranges[0].resolved {
		t.Fatalf("range 0 resolved = false, want true")
	}
}

func TestEntriesFromLines(t *testing.T) {
	entries := entriesFromLines([]string{"a", "b"}, categoryAdded)
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
	if entries[0].category != categoryAdded || entries[0].baseIndex != -1 {
		t.Fatalf("entry 0 = %+v, want added with baseIndex -1", entries[0])
	}
	if entries[1].text != "b" {
		t.Fatalf("entry 1 text = %q, want b", entries[1].text)
	}
}

func TestResultLabel(t *testing.T) {
	cases := []struct {
		resolution markers.Resolution
		preview    bool
		want       string
	}{
		{markers.ResolutionOurs, false, "ours"},
		{markers.ResolutionTheirs, false, "theirs"},
		{markers.ResolutionBoth, false, "both"},
		{markers.ResolutionNone, false, "none"},
		{markers.ResolutionOurs, true, "selected ours"},
	}
	for _, tc := range cases {
		if got := resultLabel(tc.resolution, tc.preview); got != tc.want {
			t.Fatalf("resultLabel(%q, preview=%v) = %q, want %q", tc.resolution, tc.preview, got, tc.want)
		}
	}
}
