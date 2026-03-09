package mergeview

import "testing"

func TestReplayMergedResultAndRenderOutput(t *testing.T) {
	session, err := ParseSession([]byte("start\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nend\n"))
	if err != nil {
		t.Fatalf("ParseSession failed: %v", err)
	}
	replayed, manual, labels, known, err := ReplayMergedResult(session, []byte("start\nmanual\nend\n"))
	if err != nil {
		t.Fatalf("ReplayMergedResult failed: %v", err)
	}
	if got := string(manual[0]); got != "manual\n" {
		t.Fatalf("manual[0] = %q", got)
	}
	if known[0] {
		t.Fatalf("known[0] = true, want false")
	}
	rendered, unresolved, err := RenderMergedOutput(replayed, manual, labels, known)
	if err != nil {
		t.Fatalf("RenderMergedOutput failed: %v", err)
	}
	if unresolved {
		t.Fatalf("unresolved = true, want false")
	}
	if string(rendered) != "start\nmanual\nend\n" {
		t.Fatalf("rendered = %q", string(rendered))
	}
}
