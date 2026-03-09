package mergeview

import (
	"testing"

	"github.com/chojs23/ec/internal/markers"
)

func TestSessionFromDocumentRoundTrip(t *testing.T) {
	doc, err := markers.Parse([]byte("line1\n<<<<<<< HEAD\nours\n||||||| base\nbase\n=======\ntheirs\n>>>>>>> branch\nline2\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	session, err := SessionFromDocument(doc)
	if err != nil {
		t.Fatalf("SessionFromDocument failed: %v", err)
	}
	roundTrip := session.Document()
	if !markers.DocumentsEqual(doc, roundTrip) {
		t.Fatalf("round trip document mismatch")
	}
}

func TestSessionApplyAll(t *testing.T) {
	doc, err := markers.Parse([]byte("line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nline2\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	session, err := SessionFromDocument(doc)
	if err != nil {
		t.Fatalf("SessionFromDocument failed: %v", err)
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
