package tui

import (
	"testing"

	"github.com/chojs23/ec/internal/markers"
)

func parseSingleConflictDoc(t *testing.T) markers.Document {
	t.Helper()
	data := []byte("start\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nend\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	return doc
}

func conflictSegment(t *testing.T, doc markers.Document, index int) markers.ConflictSegment {
	t.Helper()
	ref := doc.Conflicts[index]
	seg, ok := doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
	if !ok {
		t.Fatalf("expected conflict segment")
	}
	return seg
}

func setConflictResolution(doc *markers.Document, index int, res markers.Resolution) {
	ref := doc.Conflicts[index]
	seg := doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
	seg.Resolution = res
	doc.Segments[ref.SegmentIndex] = seg
}

func TestApplyMergedResolutionsMatchesSelections(t *testing.T) {
	testCases := []struct {
		name       string
		merged     string
		resolution markers.Resolution
	}{
		{name: "ours", merged: "start\nours\nend\n", resolution: markers.ResolutionOurs},
		{name: "theirs", merged: "start\ntheirs\nend\n", resolution: markers.ResolutionTheirs},
		{name: "both", merged: "start\nours\ntheirs\nend\n", resolution: markers.ResolutionBoth},
		{name: "none", merged: "start\nend\n", resolution: markers.ResolutionNone},
	}

	for _, tc := range testCases {
		doc := parseSingleConflictDoc(t)
		updated, manual, err := applyMergedResolutions(doc, []byte(tc.merged))
		if err != nil {
			t.Fatalf("%s: applyMergedResolutions error: %v", tc.name, err)
		}
		if len(manual) != 0 {
			t.Fatalf("%s: expected no manual resolutions", tc.name)
		}
		seg := conflictSegment(t, updated, 0)
		if seg.Resolution != tc.resolution {
			t.Fatalf("%s: resolution = %q, want %q", tc.name, seg.Resolution, tc.resolution)
		}
	}
}

func TestApplyMergedResolutionsManualEdit(t *testing.T) {
	doc := parseSingleConflictDoc(t)
	updated, manual, err := applyMergedResolutions(doc, []byte("start\nmanual\nend\n"))
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	seg := conflictSegment(t, updated, 0)
	if seg.Resolution != markers.ResolutionUnset {
		t.Fatalf("resolution = %q, want unset", seg.Resolution)
	}
	if got := string(manual[0]); got != "manual\n" {
		t.Fatalf("manual resolution = %q, want %q", got, "manual\n")
	}
}

func TestApplyMergedResolutionsSkipsConflictMarkers(t *testing.T) {
	merged := "start\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nend\n"
	doc := parseSingleConflictDoc(t)
	updated, manual, err := applyMergedResolutions(doc, []byte(merged))
	if err != nil {
		t.Fatalf("applyMergedResolutions error: %v", err)
	}
	if len(manual) != 0 {
		t.Fatalf("expected no manual resolutions when markers are present")
	}
	seg := conflictSegment(t, updated, 0)
	if seg.Resolution != markers.ResolutionUnset {
		t.Fatalf("resolution = %q, want unset", seg.Resolution)
	}
}

func TestApplyMergedResolutionsAlignmentFailure(t *testing.T) {
	doc := parseSingleConflictDoc(t)
	_, _, err := applyMergedResolutions(doc, []byte("ours\nend\n"))
	if err == nil {
		t.Fatalf("expected alignment error")
	}
}

func TestAllResolvedWithManualOverride(t *testing.T) {
	data := []byte("start\n<<<<<<< HEAD\nours1\n=======\ntheirs1\n>>>>>>> branch\nmid\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> branch\nend\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	setConflictResolution(&doc, 0, markers.ResolutionOurs)
	if allResolved(doc, map[int][]byte{}) {
		t.Fatalf("expected unresolved without manual override")
	}

	manual := map[int][]byte{1: []byte("manual\n")}
	if !allResolved(doc, manual) {
		t.Fatalf("expected resolved with manual override")
	}
}

func TestContainsConflictMarkers(t *testing.T) {
	if !containsConflictMarkers([][]byte{[]byte("<<<<<<< HEAD\n")}) {
		t.Fatalf("expected conflict markers to be detected")
	}
	if containsConflictMarkers([][]byte{[]byte("ok\n"), []byte("line\n")}) {
		t.Fatalf("did not expect conflict markers")
	}
}
