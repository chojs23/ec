package engine

import (
	"bytes"
	"testing"

	"github.com/chojs23/ec/internal/markers"
)

func TestNewState(t *testing.T) {
	doc := markers.Document{}
	state, err := NewState(doc)
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	if state == nil {
		t.Fatalf("NewState() returned nil state")
	}
}

func TestApplyResolution(t *testing.T) {
	input := []byte(`line1
<<<<<<< HEAD
ours
||||||| base
base
=======
theirs
>>>>>>> branch
line2
<<<<<<< HEAD
ours2
=======
theirs2
>>>>>>> branch
line3
`)

	doc, err := markers.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(doc.Conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d", len(doc.Conflicts))
	}

	state, err := NewState(doc)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}

	t.Run("apply ours to first conflict", func(t *testing.T) {
		err := state.ApplyResolution(0, markers.ResolutionOurs)
		if err != nil {
			t.Fatalf("ApplyResolution failed: %v", err)
		}

		seg := state.doc.Segments[state.doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
		if seg.Resolution != markers.ResolutionOurs {
			t.Errorf("expected resolution ours, got %q", seg.Resolution)
		}
	})

	t.Run("apply theirs to second conflict", func(t *testing.T) {
		err := state.ApplyResolution(1, markers.ResolutionTheirs)
		if err != nil {
			t.Fatalf("ApplyResolution failed: %v", err)
		}

		seg := state.doc.Segments[state.doc.Conflicts[1].SegmentIndex].(markers.ConflictSegment)
		if seg.Resolution != markers.ResolutionTheirs {
			t.Errorf("expected resolution theirs, got %q", seg.Resolution)
		}
	})

	t.Run("apply none to first conflict", func(t *testing.T) {
		err := state.ApplyResolution(0, markers.ResolutionNone)
		if err != nil {
			t.Fatalf("ApplyResolution failed: %v", err)
		}

		seg := state.doc.Segments[state.doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
		if seg.Resolution != markers.ResolutionNone {
			t.Errorf("expected resolution none, got %q", seg.Resolution)
		}
	})

	t.Run("invalid conflict index", func(t *testing.T) {
		err := state.ApplyResolution(2, markers.ResolutionOurs)
		if err == nil {
			t.Error("expected error for out of bounds index")
		}
	})

	t.Run("negative conflict index", func(t *testing.T) {
		err := state.ApplyResolution(-1, markers.ResolutionOurs)
		if err == nil {
			t.Error("expected error for negative index")
		}
	})

	t.Run("invalid resolution", func(t *testing.T) {
		err := state.ApplyResolution(0, markers.Resolution("invalid"))
		if err == nil {
			t.Error("expected error for invalid resolution")
		}
	})

	t.Run("unset resolution rejected", func(t *testing.T) {
		err := state.ApplyResolution(0, markers.ResolutionUnset)
		if err == nil {
			t.Error("expected error for unset resolution")
		}
	})
}

func TestApplyAll(t *testing.T) {
	input := []byte(`line1
<<<<<<< HEAD
ours
=======
theirs
>>>>>>> branch
line2
<<<<<<< HEAD
ours2
=======
theirs2
>>>>>>> branch
line3
`)

	doc, err := markers.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	state, err := NewState(doc)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}

	if err := state.ApplyAll(markers.ResolutionTheirs); err != nil {
		t.Fatalf("ApplyAll failed: %v", err)
	}

	for i, ref := range state.doc.Conflicts {
		seg := state.doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
		if seg.Resolution != markers.ResolutionTheirs {
			t.Errorf("conflict %d expected theirs, got %q", i, seg.Resolution)
		}
	}
}

func TestPreview(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		resolutions []markers.Resolution
		want        []byte
		wantErr     bool
	}{
		{
			name: "single conflict ours",
			input: []byte(`line1
<<<<<<< HEAD
ours
=======
theirs
>>>>>>> branch
line2
`),
			resolutions: []markers.Resolution{markers.ResolutionOurs},
			want: []byte(`line1
ours
line2
`),
			wantErr: false,
		},
		{
			name: "single conflict theirs",
			input: []byte(`line1
<<<<<<< HEAD
ours
=======
theirs
>>>>>>> branch
line2
`),
			resolutions: []markers.Resolution{markers.ResolutionTheirs},
			want: []byte(`line1
theirs
line2
`),
			wantErr: false,
		},
		{
			name: "single conflict both",
			input: []byte(`line1
<<<<<<< HEAD
ours
=======
theirs
>>>>>>> branch
line2
`),
			resolutions: []markers.Resolution{markers.ResolutionBoth},
			want: []byte(`line1
ours
theirs
line2
`),
			wantErr: false,
		},
		{
			name: "single conflict none",
			input: []byte(`line1
<<<<<<< HEAD
ours
=======
theirs
>>>>>>> branch
line2
`),
			resolutions: []markers.Resolution{markers.ResolutionNone},
			want: []byte(`line1
line2
`),
			wantErr: false,
		},
		{
			name: "multiple conflicts mixed",
			input: []byte(`line1
<<<<<<< HEAD
ours1
=======
theirs1
>>>>>>> branch
line2
<<<<<<< HEAD
ours2
=======
theirs2
>>>>>>> branch
line3
`),
			resolutions: []markers.Resolution{markers.ResolutionOurs, markers.ResolutionTheirs},
			want: []byte(`line1
ours1
line2
theirs2
line3
`),
			wantErr: false,
		},
		{
			name: "unresolved conflict",
			input: []byte(`line1
<<<<<<< HEAD
ours
=======
theirs
>>>>>>> branch
line2
`),
			resolutions: []markers.Resolution{},
			want:        nil,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := markers.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			state, err := NewState(doc)
			if err != nil {
				t.Fatalf("NewState failed: %v", err)
			}

			for i, res := range tt.resolutions {
				if err := state.ApplyResolution(i, res); err != nil {
					t.Fatalf("ApplyResolution(%d, %q) failed: %v", i, res, err)
				}
			}

			got, err := state.Preview()
			if (err != nil) != tt.wantErr {
				t.Errorf("Preview() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !bytes.Equal(got, tt.want) {
				t.Errorf("Preview() mismatch:\ngot:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestImportMergedManualConflict(t *testing.T) {
	input := []byte("line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nline2\n")
	doc, err := markers.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	state, err := NewState(doc)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}
	if err := state.ImportMerged([]byte("line1\nmanual\nline2\n")); err != nil {
		t.Fatalf("ImportMerged failed: %v", err)
	}
	manual := state.ManualResolved()
	if got := string(manual[0]); got != "manual\n" {
		t.Fatalf("manual[0] = %q, want %q", got, "manual\\n")
	}
	if got := string(state.RenderMerged()); got != "line1\nmanual\nline2\n" {
		t.Fatalf("RenderMerged = %q", got)
	}
}

func TestPreviewDeterministic(t *testing.T) {
	input := []byte(`line1
<<<<<<< HEAD
ours
||||||| base
base
=======
theirs
>>>>>>> branch
line2
`)

	doc, err := markers.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	state, err := NewState(doc)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}

	if err := state.ApplyResolution(0, markers.ResolutionBoth); err != nil {
		t.Fatalf("ApplyResolution failed: %v", err)
	}

	preview1, err := state.Preview()
	if err != nil {
		t.Fatalf("Preview 1 failed: %v", err)
	}

	preview2, err := state.Preview()
	if err != nil {
		t.Fatalf("Preview 2 failed: %v", err)
	}

	if !bytes.Equal(preview1, preview2) {
		t.Errorf("Preview not deterministic:\nfirst:\n%s\nsecond:\n%s", preview1, preview2)
	}
}

func TestDocument(t *testing.T) {
	input := []byte(`line1
<<<<<<< HEAD
ours
=======
theirs
>>>>>>> branch
line2
`)

	doc, err := markers.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	state, err := NewState(doc)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}

	if err := state.ApplyResolution(0, markers.ResolutionOurs); err != nil {
		t.Fatalf("ApplyResolution failed: %v", err)
	}

	retrievedDoc := state.Document()

	if len(retrievedDoc.Conflicts) != len(state.doc.Conflicts) {
		t.Errorf("Document() conflicts mismatch: got %d, want %d", len(retrievedDoc.Conflicts), len(state.doc.Conflicts))
	}

	if len(retrievedDoc.Segments) != len(state.doc.Segments) {
		t.Errorf("Document() segments mismatch: got %d, want %d", len(retrievedDoc.Segments), len(state.doc.Segments))
	}
}

func TestReplaceDocument(t *testing.T) {
	input := []byte(`line1
<<<<<<< HEAD
ours
=======
theirs
>>>>>>> branch
line2
`)

	doc, err := markers.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	state, err := NewState(doc)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}

	edited := markers.CloneDocument(doc)
	editedSeg := edited.Segments[edited.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
	editedSeg.Resolution = markers.ResolutionOurs
	edited.Segments[edited.Conflicts[0].SegmentIndex] = editedSeg

	state.ReplaceDocument(edited)
	seg := state.doc.Segments[state.doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
	if seg.Resolution != markers.ResolutionOurs {
		t.Fatalf("Resolution = %q, want %q after replace", seg.Resolution, markers.ResolutionOurs)
	}
}

func TestImportMergedPreservesLeadingBoundaryTextAfterResolve(t *testing.T) {
	input := []byte("<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\ntail\n")
	doc, err := markers.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	state, err := NewState(doc)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}
	merged := []byte("header\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\ntail\n")
	if err := state.ImportMerged(merged); err != nil {
		t.Fatalf("ImportMerged failed: %v", err)
	}
	boundaries := state.BoundaryText()
	if got := string(boundaries[0]); got != "header\n" {
		t.Fatalf("BoundaryText()[0] = %q, want %q", got, "header\\n")
	}
	if err := state.ApplyResolution(0, markers.ResolutionOurs); err != nil {
		t.Fatalf("ApplyResolution failed: %v", err)
	}
	if got := string(state.RenderMerged()); got != "header\nours\ntail\n" {
		t.Fatalf("RenderMerged = %q, want %q", got, "header\\nours\\ntail\\n")
	}
}

func TestImportMergedPreservesTextBetweenAdjacentConflictsAfterResolve(t *testing.T) {
	input := []byte("<<<<<<< HEAD\nours1\n=======\ntheirs1\n>>>>>>> one\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> two\n")
	doc, err := markers.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	state, err := NewState(doc)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}
	merged := []byte("<<<<<<< HEAD\nours1\n=======\ntheirs1\n>>>>>>> one\nbetween\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> two\n")
	if err := state.ImportMerged(merged); err != nil {
		t.Fatalf("ImportMerged failed: %v", err)
	}
	boundaries := state.BoundaryText()
	if got := string(boundaries[1]); got != "between\n" {
		t.Fatalf("BoundaryText()[1] = %q, want %q", got, "between\\n")
	}
	if err := state.ApplyResolution(0, markers.ResolutionOurs); err != nil {
		t.Fatalf("ApplyResolution failed: %v", err)
	}
	expected := "ours1\nbetween\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> two\n"
	if got := string(state.RenderMerged()); got != expected {
		t.Fatalf("RenderMerged = %q, want %q", got, expected)
	}
}

func TestImportMergedPreservesCanonicalBaseLabelForTwoWayConflict(t *testing.T) {
	input := []byte("intro\n<<<<<<< HEAD\nours line\n||||||| base-commit\n=======\ntheirs line\n>>>>>>> feature\noutro\n")
	doc, err := markers.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	state, err := NewState(doc)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}
	merged := []byte("intro\n<<<<<<< ours-label\nours line\n=======\ntheirs line\n>>>>>>> theirs-label\noutro\n")
	if err := state.ImportMerged(merged); err != nil {
		t.Fatalf("ImportMerged failed: %v", err)
	}

	updated := state.Document()
	seg, ok := updated.Segments[updated.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
	if !ok {
		t.Fatalf("segment is %T, want ConflictSegment", updated.Segments[updated.Conflicts[0].SegmentIndex])
	}
	if len(seg.Base) != 0 {
		t.Fatalf("Base = %q, want empty", string(seg.Base))
	}
	if seg.BaseLabel != "base-commit" {
		t.Fatalf("BaseLabel = %q, want %q", seg.BaseLabel, "base-commit")
	}
	if err := ValidateBaseCompleteness(updated); err != nil {
		t.Fatalf("ValidateBaseCompleteness failed: %v", err)
	}

	labels, known := state.MergedLabels()
	if !known[0] {
		t.Fatalf("MergedLabels known = false, want true")
	}
	if labels[0].OursLabel != "ours-label" || labels[0].TheirsLabel != "theirs-label" {
		t.Fatalf("MergedLabels = %+v", labels[0])
	}
	if labels[0].BaseLabel != "base-commit" {
		t.Fatalf("MergedLabels BaseLabel = %q, want %q", labels[0].BaseLabel, "base-commit")
	}
}

func TestClassifyConflictOutputTreatsSurvivingMarkersAsUnresolved(t *testing.T) {
	// When ImportMerged's line-diff fallback fires (disk and diff3 draw
	// different segment boundaries), conflict markers can end up wrapped in
	// surrounding context text inside a single slot. The markers still
	// indicate an unresolved hunk, not a manual edit — any other
	// classification leaks raw `<<<<<<<` text into the result pane labelled
	// as "manual resolved".
	seg := markers.ConflictSegment{
		Ours:   []byte("ours\n"),
		Theirs: []byte("theirs\n"),
	}
	output := []byte("before\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nafter\n")
	res, unresolved, manual, _, _ := classifyConflictOutput(seg, output)
	if res != markers.ResolutionUnset {
		t.Fatalf("resolution = %q, want Unset", res)
	}
	if !unresolved {
		t.Fatalf("unresolved = false, want true")
	}
	if manual {
		t.Fatalf("manual = true, want false (markers still present)")
	}
}

func TestClassifyConflictOutputMarkerFreeCustomTextIsManual(t *testing.T) {
	seg := markers.ConflictSegment{
		Ours:   []byte("ours\n"),
		Theirs: []byte("theirs\n"),
	}
	output := []byte("user typed a custom resolution here\n")
	res, unresolved, manual, _, _ := classifyConflictOutput(seg, output)
	if res != markers.ResolutionUnset {
		t.Fatalf("resolution = %q, want Unset", res)
	}
	if unresolved {
		t.Fatalf("unresolved = true, want false (no markers left)")
	}
	if !manual {
		t.Fatalf("manual = false, want true")
	}
}

func TestImportMergedRejectsReorderedSeparatedConflicts(t *testing.T) {
	input := []byte("<<<<<<< left-one\nours1\n=======\ntheirs1\n>>>>>>> right-one\n<<<<<<< left-two\nours2\n=======\ntheirs2\n>>>>>>> right-two\n")
	doc, err := markers.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	state, err := NewState(doc)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}
	swapped := []byte("<<<<<<< left-two\nours2\n=======\ntheirs2\n>>>>>>> right-two\n<<<<<<< left-one\nours1\n=======\ntheirs1\n>>>>>>> right-one\n")
	parsed, err := markers.Parse(swapped)
	if err != nil {
		t.Fatalf("Parse swapped failed: %v", err)
	}
	unsafe, detail := state.findUnsafeParsedConflictReorder(parsed)
	if !unsafe {
		t.Fatalf("findUnsafeParsedConflictReorder should detect reorder")
	}
	if detail == "" {
		t.Fatalf("expected reorder detail")
	}
	if err := state.ImportMerged(swapped); err == nil {
		t.Fatal("ImportMerged should reject reordered separated conflicts")
	}
	if got := string(state.RenderMerged()); got != string(input) {
		t.Fatalf("RenderMerged = %q, want original %q", got, string(input))
	}
}
