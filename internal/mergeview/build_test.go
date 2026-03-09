package mergeview

import "testing"

func TestBuildSessionFromInputsOneSidedEdit(t *testing.T) {
	session := buildSessionFromInputs(
		[]byte("a\nbase\nc\n"),
		[]byte("a\nours\nc\n"),
		[]byte("a\nbase\nc\n"),
	)
	if len(session.Conflicts) != 0 {
		t.Fatalf("conflicts = %d, want 0", len(session.Conflicts))
	}
	preview, err := session.Preview()
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	if string(preview) != "a\nours\nc\n" {
		t.Fatalf("preview = %q", string(preview))
	}
}

func TestBuildSessionFromInputsIdenticalEdits(t *testing.T) {
	session := buildSessionFromInputs(
		[]byte("a\nbase\nc\n"),
		[]byte("a\nsame\nc\n"),
		[]byte("a\nsame\nc\n"),
	)
	if len(session.Conflicts) != 0 {
		t.Fatalf("conflicts = %d, want 0", len(session.Conflicts))
	}
	preview, err := session.Preview()
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	if string(preview) != "a\nsame\nc\n" {
		t.Fatalf("preview = %q", string(preview))
	}
}

func TestBuildSessionFromInputsConflict(t *testing.T) {
	session := buildSessionFromInputs(
		[]byte("a\nbase\nc\n"),
		[]byte("a\nours\nc\n"),
		[]byte("a\ntheirs\nc\n"),
	)
	if len(session.Conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(session.Conflicts))
	}
	block := session.Segments[session.Conflicts[0].SegmentIndex].(ConflictBlock)
	if string(block.Base) != "base\n" || string(block.Ours) != "ours\n" || string(block.Theirs) != "theirs\n" {
		t.Fatalf("block = base:%q ours:%q theirs:%q", string(block.Base), string(block.Ours), string(block.Theirs))
	}
}

func TestBuildSessionFromInputsNonOverlappingEditsAutoMerge(t *testing.T) {
	session := buildSessionFromInputs(
		[]byte("a\nbase1\nmid\nbase2\nz\n"),
		[]byte("a\nours1\nmid\nbase2\nz\n"),
		[]byte("a\nbase1\nmid\ntheirs2\nz\n"),
	)
	if len(session.Conflicts) != 0 {
		t.Fatalf("conflicts = %d, want 0", len(session.Conflicts))
	}
	preview, err := session.Preview()
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	if string(preview) != "a\nours1\nmid\ntheirs2\nz\n" {
		t.Fatalf("preview = %q", string(preview))
	}
}

func TestBuildSessionFromInputsOverlappingInsertAndReplaceConflicts(t *testing.T) {
	session := buildSessionFromInputs(
		[]byte("a\nbase\nz\n"),
		[]byte("a\ninsert\nbase\nz\n"),
		[]byte("a\nreplace\nz\n"),
	)
	if len(session.Conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(session.Conflicts))
	}
	block := session.Segments[session.Conflicts[0].SegmentIndex].(ConflictBlock)
	if string(block.Base) != "base\n" {
		t.Fatalf("base = %q", string(block.Base))
	}
	if string(block.Ours) != "insert\nbase\n" {
		t.Fatalf("ours = %q", string(block.Ours))
	}
	if string(block.Theirs) != "replace\n" {
		t.Fatalf("theirs = %q", string(block.Theirs))
	}
}

func TestBuildSessionFromInputsAddAddConflictWithoutBase(t *testing.T) {
	session := buildSessionFromInputs(
		nil,
		[]byte("ours\n"),
		[]byte("theirs\n"),
	)
	if len(session.Conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(session.Conflicts))
	}
	block := session.Segments[session.Conflicts[0].SegmentIndex].(ConflictBlock)
	if len(block.Base) != 0 {
		t.Fatalf("base = %q, want empty", string(block.Base))
	}
	if string(block.Ours) != "ours\n" || string(block.Theirs) != "theirs\n" {
		t.Fatalf("block = ours:%q theirs:%q", string(block.Ours), string(block.Theirs))
	}
}
