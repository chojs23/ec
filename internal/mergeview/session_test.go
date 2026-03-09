package mergeview

import (
	"testing"

	"github.com/chojs23/ec/internal/markers"
)

func TestParseSession(t *testing.T) {
	session, err := ParseSession([]byte("line1\n<<<<<<< HEAD\nours\n||||||| base\nbase\n=======\ntheirs\n>>>>>>> branch\nline2\n"))
	if err != nil {
		t.Fatalf("ParseSession failed: %v", err)
	}
	if len(session.Conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(session.Conflicts))
	}
	block := session.Segments[session.Conflicts[0].SegmentIndex].(ConflictBlock)
	if string(block.Ours) != "ours\n" || string(block.Base) != "base\n" || string(block.Theirs) != "theirs\n" {
		t.Fatalf("block mismatch: ours=%q base=%q theirs=%q", string(block.Ours), string(block.Base), string(block.Theirs))
	}
	if block.Labels.OursLabel != "HEAD" || block.Labels.BaseLabel != "base" || block.Labels.TheirsLabel != "branch" {
		t.Fatalf("labels mismatch: %+v", block.Labels)
	}
}

func TestSessionApplyAll(t *testing.T) {
	session, err := ParseSession([]byte("line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nline2\n"))
	if err != nil {
		t.Fatalf("ParseSession failed: %v", err)
	}
	if err := session.ApplyAll(markers.ResolutionBoth); err != nil {
		t.Fatalf("ApplyAll failed: %v", err)
	}
	preview, err := session.Preview()
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	if string(preview) != "line1\nours\ntheirs\nline2\n" {
		t.Fatalf("Preview = %q", string(preview))
	}
}
