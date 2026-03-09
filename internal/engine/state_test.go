package engine

import (
	"bytes"
	"testing"

	"github.com/chojs23/ec/internal/markers"
	"github.com/chojs23/ec/internal/mergeview"
)

func parseSession(t *testing.T, input []byte) *mergeview.Session {
	t.Helper()
	session, err := mergeview.ParseSession(input)
	if err != nil {
		t.Fatalf("ParseSession failed: %v", err)
	}
	return session
}

func conflictBlockAt(t *testing.T, session *mergeview.Session, index int) mergeview.ConflictBlock {
	t.Helper()
	ref := session.Conflicts[index]
	block, ok := session.Segments[ref.SegmentIndex].(mergeview.ConflictBlock)
	if !ok {
		t.Fatalf("expected conflict block")
	}
	return block
}

func TestNewStateFromSession(t *testing.T) {
	state, err := NewStateFromSession(&mergeview.Session{})
	if err != nil {
		t.Fatalf("NewStateFromSession error = %v", err)
	}
	if state == nil {
		t.Fatalf("NewStateFromSession returned nil state")
	}
}

func TestApplyResolution(t *testing.T) {
	state, err := NewStateFromSession(parseSession(t, []byte("line1\n<<<<<<< HEAD\nours\n||||||| base\nbase\n=======\ntheirs\n>>>>>>> branch\nline2\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> branch\nline3\n")))
	if err != nil {
		t.Fatalf("NewStateFromSession failed: %v", err)
	}

	if err := state.ApplyResolution(0, markers.ResolutionOurs); err != nil {
		t.Fatalf("ApplyResolution failed: %v", err)
	}
	if got := conflictBlockAt(t, state.Session(), 0).Resolution; got != markers.ResolutionOurs {
		t.Fatalf("resolution = %q, want %q", got, markers.ResolutionOurs)
	}

	if err := state.ApplyResolution(1, markers.ResolutionTheirs); err != nil {
		t.Fatalf("ApplyResolution failed: %v", err)
	}
	if got := conflictBlockAt(t, state.Session(), 1).Resolution; got != markers.ResolutionTheirs {
		t.Fatalf("resolution = %q, want %q", got, markers.ResolutionTheirs)
	}

	if err := state.ApplyResolution(0, markers.ResolutionNone); err != nil {
		t.Fatalf("ApplyResolution failed: %v", err)
	}
	if got := conflictBlockAt(t, state.Session(), 0).Resolution; got != markers.ResolutionNone {
		t.Fatalf("resolution = %q, want %q", got, markers.ResolutionNone)
	}

	if err := state.ApplyResolution(2, markers.ResolutionOurs); err == nil {
		t.Fatalf("expected out of bounds error")
	}
	if err := state.ApplyResolution(-1, markers.ResolutionOurs); err == nil {
		t.Fatalf("expected negative index error")
	}
	if err := state.ApplyResolution(0, markers.Resolution("invalid")); err == nil {
		t.Fatalf("expected invalid resolution error")
	}
	if err := state.ApplyResolution(0, markers.ResolutionUnset); err == nil {
		t.Fatalf("expected unset resolution error")
	}
}

func TestApplyAll(t *testing.T) {
	state, err := NewStateFromSession(parseSession(t, []byte("line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nline2\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> branch\nline3\n")))
	if err != nil {
		t.Fatalf("NewStateFromSession failed: %v", err)
	}
	if err := state.ApplyAll(markers.ResolutionTheirs); err != nil {
		t.Fatalf("ApplyAll failed: %v", err)
	}
	session := state.Session()
	for i := range session.Conflicts {
		if got := conflictBlockAt(t, session, i).Resolution; got != markers.ResolutionTheirs {
			t.Fatalf("conflict %d resolution = %q, want %q", i, got, markers.ResolutionTheirs)
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
		{name: "ours", input: []byte("line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nline2\n"), resolutions: []markers.Resolution{markers.ResolutionOurs}, want: []byte("line1\nours\nline2\n")},
		{name: "theirs", input: []byte("line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nline2\n"), resolutions: []markers.Resolution{markers.ResolutionTheirs}, want: []byte("line1\ntheirs\nline2\n")},
		{name: "both", input: []byte("line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nline2\n"), resolutions: []markers.Resolution{markers.ResolutionBoth}, want: []byte("line1\nours\ntheirs\nline2\n")},
		{name: "none", input: []byte("line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nline2\n"), resolutions: []markers.Resolution{markers.ResolutionNone}, want: []byte("line1\nline2\n")},
		{name: "mixed", input: []byte("line1\n<<<<<<< HEAD\nours1\n=======\ntheirs1\n>>>>>>> branch\nline2\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> branch\nline3\n"), resolutions: []markers.Resolution{markers.ResolutionOurs, markers.ResolutionTheirs}, want: []byte("line1\nours1\nline2\ntheirs2\nline3\n")},
		{name: "unresolved", input: []byte("line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nline2\n"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := NewStateFromSession(parseSession(t, tt.input))
			if err != nil {
				t.Fatalf("NewStateFromSession failed: %v", err)
			}
			for i, res := range tt.resolutions {
				if err := state.ApplyResolution(i, res); err != nil {
					t.Fatalf("ApplyResolution(%d) failed: %v", i, err)
				}
			}
			got, err := state.Preview()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Preview error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !bytes.Equal(got, tt.want) {
				t.Fatalf("Preview mismatch: got %q want %q", string(got), string(tt.want))
			}
		})
	}
}

func TestPreviewDeterministic(t *testing.T) {
	state, err := NewStateFromSession(parseSession(t, []byte("line1\n<<<<<<< HEAD\nours\n||||||| base\nbase\n=======\ntheirs\n>>>>>>> branch\nline2\n")))
	if err != nil {
		t.Fatalf("NewStateFromSession failed: %v", err)
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
		t.Fatalf("preview not deterministic")
	}
}

func TestReplaceSession(t *testing.T) {
	state, err := NewStateFromSession(parseSession(t, []byte("line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nline2\n")))
	if err != nil {
		t.Fatalf("NewStateFromSession failed: %v", err)
	}
	replacement := parseSession(t, []byte("line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nline2\n"))
	if err := replacement.ApplyResolution(0, markers.ResolutionOurs); err != nil {
		t.Fatalf("ApplyResolution failed: %v", err)
	}
	state.ReplaceSession(replacement)
	if got := conflictBlockAt(t, state.Session(), 0).Resolution; got != markers.ResolutionOurs {
		t.Fatalf("resolution = %q, want %q", got, markers.ResolutionOurs)
	}
}
