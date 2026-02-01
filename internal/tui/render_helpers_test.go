package tui

import (
	"testing"

	"github.com/chojs23/easy-conflict/internal/markers"
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
