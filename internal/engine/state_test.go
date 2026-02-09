package engine

import (
	"bytes"
	"testing"

	"github.com/chojs23/ec/internal/markers"
)

func TestNewState(t *testing.T) {
	tests := []struct {
		name        string
		maxUndoSize int
		wantErr     bool
	}{
		{"valid size 1", 1, false},
		{"valid size 10", 10, false},
		{"invalid size 0", 0, true},
		{"invalid size -1", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := markers.Document{}
			_, err := NewState(doc, tt.maxUndoSize)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewState() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
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

	state, err := NewState(doc, 10)
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

	state, err := NewState(doc, 10)
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

func TestUndo(t *testing.T) {
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

	state, err := NewState(doc, 10)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}

	t.Run("no undo history initially", func(t *testing.T) {
		err := state.Undo()
		if err == nil {
			t.Error("expected error when no undo history")
		}
	})

	t.Run("undo after single apply", func(t *testing.T) {
		if err := state.ApplyResolution(0, markers.ResolutionOurs); err != nil {
			t.Fatalf("ApplyResolution failed: %v", err)
		}

		seg := state.doc.Segments[state.doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
		if seg.Resolution != markers.ResolutionOurs {
			t.Errorf("expected ours, got %q", seg.Resolution)
		}

		if err := state.Undo(); err != nil {
			t.Fatalf("Undo failed: %v", err)
		}

		seg = state.doc.Segments[state.doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
		if seg.Resolution != markers.ResolutionUnset {
			t.Errorf("expected unset after undo, got %q", seg.Resolution)
		}
	})

	t.Run("multiple undo operations", func(t *testing.T) {
		if err := state.ApplyResolution(0, markers.ResolutionOurs); err != nil {
			t.Fatalf("ApplyResolution 1 failed: %v", err)
		}
		if err := state.ApplyResolution(0, markers.ResolutionTheirs); err != nil {
			t.Fatalf("ApplyResolution 2 failed: %v", err)
		}
		if err := state.ApplyResolution(0, markers.ResolutionBoth); err != nil {
			t.Fatalf("ApplyResolution 3 failed: %v", err)
		}

		seg := state.doc.Segments[state.doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
		if seg.Resolution != markers.ResolutionBoth {
			t.Errorf("expected both, got %q", seg.Resolution)
		}

		if err := state.Undo(); err != nil {
			t.Fatalf("Undo 1 failed: %v", err)
		}
		seg = state.doc.Segments[state.doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
		if seg.Resolution != markers.ResolutionTheirs {
			t.Errorf("expected theirs after first undo, got %q", seg.Resolution)
		}

		if err := state.Undo(); err != nil {
			t.Fatalf("Undo 2 failed: %v", err)
		}
		seg = state.doc.Segments[state.doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
		if seg.Resolution != markers.ResolutionOurs {
			t.Errorf("expected ours after second undo, got %q", seg.Resolution)
		}

		if err := state.Undo(); err != nil {
			t.Fatalf("Undo 3 failed: %v", err)
		}
		seg = state.doc.Segments[state.doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
		if seg.Resolution != markers.ResolutionUnset {
			t.Errorf("expected unset after third undo, got %q", seg.Resolution)
		}

		if err := state.Undo(); err == nil {
			t.Error("expected error after exhausting undo history")
		}
	})
}

func TestRedo(t *testing.T) {
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

	state, err := NewState(doc, 10)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}

	t.Run("no redo history initially", func(t *testing.T) {
		err := state.Redo()
		if err == nil {
			t.Error("expected error when no redo history")
		}
	})

	t.Run("redo after undo", func(t *testing.T) {
		if err := state.ApplyResolution(0, markers.ResolutionOurs); err != nil {
			t.Fatalf("ApplyResolution failed: %v", err)
		}
		if err := state.Undo(); err != nil {
			t.Fatalf("Undo failed: %v", err)
		}

		if err := state.Redo(); err != nil {
			t.Fatalf("Redo failed: %v", err)
		}

		seg := state.doc.Segments[state.doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
		if seg.Resolution != markers.ResolutionOurs {
			t.Errorf("expected ours after redo, got %q", seg.Resolution)
		}
	})

	t.Run("new mutation clears redo history", func(t *testing.T) {
		if err := state.Undo(); err != nil {
			t.Fatalf("Undo failed: %v", err)
		}
		if err := state.ApplyResolution(0, markers.ResolutionTheirs); err != nil {
			t.Fatalf("ApplyResolution failed: %v", err)
		}

		if err := state.Redo(); err == nil {
			t.Error("expected redo error after new mutation")
		}
	})
}

func TestUndoStackLimit(t *testing.T) {
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

	maxUndo := 2
	state, err := NewState(doc, maxUndo)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}

	if err := state.ApplyResolution(0, markers.ResolutionOurs); err != nil {
		t.Fatalf("Apply 1 failed: %v", err)
	}
	if err := state.ApplyResolution(0, markers.ResolutionTheirs); err != nil {
		t.Fatalf("Apply 2 failed: %v", err)
	}
	if err := state.ApplyResolution(0, markers.ResolutionBoth); err != nil {
		t.Fatalf("Apply 3 failed: %v", err)
	}

	if state.UndoDepth() != maxUndo {
		t.Errorf("expected undo depth %d, got %d", maxUndo, state.UndoDepth())
	}

	if err := state.Undo(); err != nil {
		t.Fatalf("Undo 1 failed: %v", err)
	}
	seg := state.doc.Segments[state.doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
	if seg.Resolution != markers.ResolutionTheirs {
		t.Errorf("expected theirs, got %q", seg.Resolution)
	}

	if err := state.Undo(); err != nil {
		t.Fatalf("Undo 2 failed: %v", err)
	}
	seg = state.doc.Segments[state.doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
	if seg.Resolution != markers.ResolutionOurs {
		t.Errorf("expected ours, got %q", seg.Resolution)
	}

	if err := state.Undo(); err == nil {
		t.Error("expected error - first state should have been trimmed")
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

			state, err := NewState(doc, 10)
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

	state, err := NewState(doc, 10)
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

	state, err := NewState(doc, 10)
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

func TestUndoDepth(t *testing.T) {
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

	state, err := NewState(doc, 5)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}

	if state.UndoDepth() != 0 {
		t.Errorf("initial undo depth should be 0, got %d", state.UndoDepth())
	}
	if state.RedoDepth() != 0 {
		t.Errorf("initial redo depth should be 0, got %d", state.RedoDepth())
	}

	if err := state.ApplyResolution(0, markers.ResolutionOurs); err != nil {
		t.Fatalf("ApplyResolution failed: %v", err)
	}

	if state.UndoDepth() != 1 {
		t.Errorf("undo depth after 1 apply should be 1, got %d", state.UndoDepth())
	}

	if err := state.ApplyResolution(0, markers.ResolutionTheirs); err != nil {
		t.Fatalf("ApplyResolution failed: %v", err)
	}

	if state.UndoDepth() != 2 {
		t.Errorf("undo depth after 2 applies should be 2, got %d", state.UndoDepth())
	}

	if err := state.Undo(); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	if state.UndoDepth() != 1 {
		t.Errorf("undo depth after 1 undo should be 1, got %d", state.UndoDepth())
	}
	if state.RedoDepth() != 1 {
		t.Errorf("redo depth after 1 undo should be 1, got %d", state.RedoDepth())
	}

	if err := state.Redo(); err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if state.UndoDepth() != 2 {
		t.Errorf("undo depth after redo should be 2, got %d", state.UndoDepth())
	}
	if state.RedoDepth() != 0 {
		t.Errorf("redo depth after redo should be 0, got %d", state.RedoDepth())
	}
}
