package tui

import (
	"fmt"
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
	lines, _ := buildResultLines(doc, 0, selectedOurs, manual, nil)
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

func TestBuildResultLinesSkipsEmptyBoundarySlots(t *testing.T) {
	doc := markers.Document{
		Segments: []markers.Segment{
			markers.TextSegment{Bytes: []byte("start\n")},
			markers.ConflictSegment{Ours: []byte("ours\n"), Theirs: []byte("theirs\n")},
			markers.TextSegment{Bytes: []byte("end\n")},
		},
		Conflicts: []markers.ConflictRef{{SegmentIndex: 1}},
	}

	lines, _ := buildResultLines(doc, 0, selectedTheirs, nil, make([][]byte, len(doc.Segments)+1))
	if len(lines) != 3 {
		t.Fatalf("lines len = %d, want 3", len(lines))
	}
	if lines[0].text != "start" || lines[1].text != "theirs" || lines[2].text != "end" {
		t.Fatalf("lines = %+v", lines)
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

func TestBuildPaneLinesFromEntriesUsesSideRangeForNonRemoved(t *testing.T) {
	doc := markers.Document{
		Segments: []markers.Segment{
			markers.TextSegment{Bytes: []byte("header\n")},
			markers.ConflictSegment{Ours: []byte("ours\n"), Theirs: []byte("theirs\n")},
		},
		Conflicts: []markers.ConflictRef{{SegmentIndex: 1}},
	}

	entries := []lineEntry{
		{text: "package tui", category: categoryDefault, baseIndex: 0},
		{text: "", category: categoryDefault, baseIndex: 1},
		{text: "import (", category: categoryDefault, baseIndex: 2},
		{text: "\"testing\"", category: categoryDefault, baseIndex: 3},
		{text: ")", category: categoryDefault, baseIndex: 4},
		{text: "conflict line 1", category: categoryModified, baseIndex: -1},
		{text: "conflict line 2", category: categoryModified, baseIndex: -1},
		{text: "tail", category: categoryDefault, baseIndex: 5},
	}

	ranges := []conflictRange{{
		baseStart:   1,
		baseEnd:     2,
		oursStart:   5,
		oursEnd:     7,
		theirsStart: 5,
		theirsEnd:   7,
	}}

	lines, _ := buildPaneLinesFromEntries(doc, paneOurs, 0, selectedOurs, entries, ranges)

	startIdx := -1
	for i, line := range lines {
		if line.text == ">> selected hunk start (ours) >>" {
			startIdx = i
			break
		}
	}
	if startIdx == -1 {
		t.Fatalf("expected selected hunk start marker")
	}
	if startIdx != 5 {
		t.Fatalf("start marker index = %d, want 5", startIdx)
	}
	if startIdx+1 >= len(lines) {
		t.Fatalf("missing line after selected hunk start marker")
	}
	if lines[startIdx+1].text != "conflict line 1" || !lines[startIdx+1].selected {
		t.Fatalf("line after marker = %+v, want selected conflict line", lines[startIdx+1])
	}
	if lines[1].selected {
		t.Fatalf("blank line near top should not be selected")
	}
}

func TestComputeConflictRangesTracksEmptySideInsertionPoint(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		baseLines   []string
		oursLines   []string
		theirsLines []string
		want        conflictRange
	}{
		{
			name:        "empty ours",
			input:       []byte("alpha\n<<<<<<< HEAD\n=======\ntheirs\n>>>>>>> branch\nomega\n\nblank\n"),
			baseLines:   []string{"alpha", "omega", "", "blank"},
			oursLines:   []string{"alpha", "omega", "", "blank"},
			theirsLines: []string{"alpha", "theirs", "omega", "", "blank"},
			want:        conflictRange{baseStart: 1, baseEnd: 1, oursStart: 1, oursEnd: 1, theirsStart: 1, theirsEnd: 2},
		},
		{
			name:        "empty theirs",
			input:       []byte("alpha\n<<<<<<< HEAD\nours\n=======\n>>>>>>> branch\nomega\n\nblank\n"),
			baseLines:   []string{"alpha", "omega", "", "blank"},
			oursLines:   []string{"alpha", "ours", "omega", "", "blank"},
			theirsLines: []string{"alpha", "omega", "", "blank"},
			want:        conflictRange{baseStart: 1, baseEnd: 1, oursStart: 1, oursEnd: 2, theirsStart: 1, theirsEnd: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := markers.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			ranges, ok := computeConflictRanges(doc, tt.baseLines, tt.oursLines, tt.theirsLines)
			if !ok {
				t.Fatalf("computeConflictRanges failed")
			}
			if len(ranges) != 1 {
				t.Fatalf("ranges len = %d, want 1", len(ranges))
			}

			if got := ranges[0]; got != tt.want {
				t.Fatalf("range = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBuildPaneLinesFromEntriesAnchorsEmptySideAtInsertionPoint(t *testing.T) {
	tests := []struct {
		name                 string
		selectedPane         paneSide
		selectedSide         selectionSide
		segments             []markers.Segment
		conflictSegmentIndex int
		entries              []lineEntry
		rangeForConflict     conflictRange
		wantStart            int
		wantLineCount        int
		wantMarkerIndex      int
	}{
		{
			name:         "empty ours in middle",
			selectedPane: paneOurs,
			selectedSide: selectedOurs,
			segments: []markers.Segment{
				markers.TextSegment{Bytes: []byte("alpha\n")},
				markers.ConflictSegment{Ours: nil, Theirs: []byte("theirs\n")},
				markers.TextSegment{Bytes: []byte("omega\n")},
			},
			conflictSegmentIndex: 1,
			entries:              []lineEntry{{text: "alpha", category: categoryDefault, baseIndex: 0}, {text: "omega", category: categoryDefault, baseIndex: 1}},
			rangeForConflict:     conflictRange{baseStart: 1, baseEnd: 1, oursStart: 1, oursEnd: 1, theirsStart: 1, theirsEnd: 2},
			wantStart:            1,
			wantLineCount:        4,
			wantMarkerIndex:      1,
		},
		{
			name:         "empty ours at bof",
			selectedPane: paneOurs,
			selectedSide: selectedOurs,
			segments: []markers.Segment{
				markers.ConflictSegment{Ours: nil, Theirs: []byte("theirs\n")},
				markers.TextSegment{Bytes: []byte("tail\n")},
			},
			conflictSegmentIndex: 0,
			entries:              []lineEntry{{text: "tail", category: categoryDefault, baseIndex: 0}},
			rangeForConflict:     conflictRange{baseStart: 0, baseEnd: 0, oursStart: 0, oursEnd: 0, theirsStart: 0, theirsEnd: 1},
			wantStart:            0,
			wantLineCount:        3,
			wantMarkerIndex:      0,
		},
		{
			name:         "empty ours at eof",
			selectedPane: paneOurs,
			selectedSide: selectedOurs,
			segments: []markers.Segment{
				markers.TextSegment{Bytes: []byte("head\n")},
				markers.ConflictSegment{Ours: nil, Theirs: []byte("theirs\n")},
			},
			conflictSegmentIndex: 1,
			entries:              []lineEntry{{text: "head", category: categoryDefault, baseIndex: 0}},
			rangeForConflict:     conflictRange{baseStart: 1, baseEnd: 1, oursStart: 1, oursEnd: 1, theirsStart: 1, theirsEnd: 2},
			wantStart:            1,
			wantLineCount:        3,
			wantMarkerIndex:      1,
		},
		{
			name:         "empty theirs in middle",
			selectedPane: paneTheirs,
			selectedSide: selectedTheirs,
			segments: []markers.Segment{
				markers.TextSegment{Bytes: []byte("alpha\n")},
				markers.ConflictSegment{Ours: []byte("ours\n"), Theirs: nil},
				markers.TextSegment{Bytes: []byte("omega\n")},
			},
			conflictSegmentIndex: 1,
			entries:              []lineEntry{{text: "alpha", category: categoryDefault, baseIndex: 0}, {text: "omega", category: categoryDefault, baseIndex: 1}},
			rangeForConflict:     conflictRange{baseStart: 1, baseEnd: 1, oursStart: 1, oursEnd: 2, theirsStart: 1, theirsEnd: 1},
			wantStart:            1,
			wantLineCount:        4,
			wantMarkerIndex:      1,
		},
		{
			name:         "empty theirs at bof",
			selectedPane: paneTheirs,
			selectedSide: selectedTheirs,
			segments: []markers.Segment{
				markers.ConflictSegment{Ours: []byte("ours\n"), Theirs: nil},
				markers.TextSegment{Bytes: []byte("tail\n")},
			},
			conflictSegmentIndex: 0,
			entries:              []lineEntry{{text: "tail", category: categoryDefault, baseIndex: 0}},
			rangeForConflict:     conflictRange{baseStart: 0, baseEnd: 0, oursStart: 0, oursEnd: 1, theirsStart: 0, theirsEnd: 0},
			wantStart:            0,
			wantLineCount:        3,
			wantMarkerIndex:      0,
		},
		{
			name:         "empty theirs at eof",
			selectedPane: paneTheirs,
			selectedSide: selectedTheirs,
			segments: []markers.Segment{
				markers.TextSegment{Bytes: []byte("head\n")},
				markers.ConflictSegment{Ours: []byte("ours\n"), Theirs: nil},
			},
			conflictSegmentIndex: 1,
			entries:              []lineEntry{{text: "head", category: categoryDefault, baseIndex: 0}},
			rangeForConflict:     conflictRange{baseStart: 1, baseEnd: 1, oursStart: 1, oursEnd: 2, theirsStart: 1, theirsEnd: 1},
			wantStart:            1,
			wantLineCount:        3,
			wantMarkerIndex:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := markers.Document{
				Segments:  tt.segments,
				Conflicts: []markers.ConflictRef{{SegmentIndex: tt.conflictSegmentIndex}},
			}

			lines, start := buildPaneLinesFromEntries(doc, tt.selectedPane, 0, tt.selectedSide, tt.entries, []conflictRange{tt.rangeForConflict})
			if start != tt.wantStart {
				t.Fatalf("start = %d, want %d", start, tt.wantStart)
			}
			if len(lines) != tt.wantLineCount {
				t.Fatalf("lines len = %d, want %d", len(lines), tt.wantLineCount)
			}

			startMarker := fmt.Sprintf(">> selected hunk start (%s) >>", sideLabel(tt.selectedPane))
			if lines[tt.wantMarkerIndex].text != startMarker {
				t.Fatalf("lines[%d].text = %q, want %q", tt.wantMarkerIndex, lines[tt.wantMarkerIndex].text, startMarker)
			}
			if !lines[tt.wantMarkerIndex].selected {
				t.Fatalf("start marker should be selected: %+v", lines[tt.wantMarkerIndex])
			}
			if lines[tt.wantMarkerIndex+1].text != ">> selected hunk end >>" {
				t.Fatalf("lines[%d].text = %q", tt.wantMarkerIndex+1, lines[tt.wantMarkerIndex+1].text)
			}
			if !lines[tt.wantMarkerIndex+1].selected {
				t.Fatalf("end marker should be selected: %+v", lines[tt.wantMarkerIndex+1])
			}

			for i, line := range lines {
				if i == tt.wantMarkerIndex || i == tt.wantMarkerIndex+1 {
					continue
				}
				if line.selected {
					t.Fatalf("non-marker line %d should not be selected: %+v", i, line)
				}
			}
		})
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

	lines, forced, ranges := buildResultPreviewLines(doc, selectedTheirs, nil, 0, nil)
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

func TestBuildResultPreviewLinesSkipsEmptyBoundarySlots(t *testing.T) {
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

	lines, forced, ranges := buildResultPreviewLines(doc, selectedTheirs, nil, 0, make([][]byte, len(doc.Segments)+1))
	if len(forced) != 0 {
		t.Fatalf("forced len = %d, want 0", len(forced))
	}
	if len(ranges) != 1 {
		t.Fatalf("ranges len = %d, want 1", len(ranges))
	}
	if len(lines) != 3 {
		t.Fatalf("lines len = %d, want 3", len(lines))
	}
	if lines[0] != "start" || lines[1] != "theirs" || lines[2] != "end" {
		t.Fatalf("lines = %v", lines)
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
	lines, forced, ranges := buildResultPreviewLines(doc, selectedOurs, manual, 1, nil)
	if len(lines) != 5 {
		t.Fatalf("lines len = %d, want 5", len(lines))
	}
	if lines[1] != "manual" {
		t.Fatalf("manual line = %q, want manual", lines[1])
	}
	if lines[2] != "middle" {
		t.Fatalf("middle line = %q, want middle", lines[2])
	}
	if lines[3] != "[resolved: none]" {
		t.Fatalf("resolved-none marker line = %q, want [resolved: none]", lines[3])
	}
	if lines[4] != "end" {
		t.Fatalf("end line = %q, want end", lines[4])
	}
	if forced[3] != categoryResolved {
		t.Fatalf("forced category = %v, want resolved", forced[3])
	}
	if len(ranges) != 2 {
		t.Fatalf("ranges len = %d, want 2", len(ranges))
	}
	if !ranges[0].resolved {
		t.Fatalf("range 0 resolved = false, want true")
	}
	if !ranges[1].resolved {
		t.Fatalf("range 1 resolved = false, want true")
	}
	if ranges[1].end-ranges[1].start != 1 {
		t.Fatalf("range 1 span len = %d, want 1", ranges[1].end-ranges[1].start)
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
