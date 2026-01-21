package markers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParse2Way(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "2way.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(doc.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(doc.Conflicts))
	}

	if len(doc.Segments) != 3 {
		t.Fatalf("expected 3 segments (text, conflict, text), got %d", len(doc.Segments))
	}

	conflict, ok := doc.Segments[1].(ConflictSegment)
	if !ok {
		t.Fatalf("segment 1 is not ConflictSegment")
	}

	if string(conflict.Ours) != "ours content\n" {
		t.Errorf("ours mismatch: %q", conflict.Ours)
	}
	if string(conflict.Theirs) != "theirs content\n" {
		t.Errorf("theirs mismatch: %q", conflict.Theirs)
	}
	if len(conflict.Base) != 0 {
		t.Errorf("base should be empty, got %q", conflict.Base)
	}
}

func TestParseDiff3(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "diff3.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(doc.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(doc.Conflicts))
	}

	conflict, ok := doc.Segments[0].(ConflictSegment)
	if !ok {
		t.Fatalf("segment 0 is not ConflictSegment")
	}

	if string(conflict.Ours) != "ours version\n" {
		t.Errorf("ours mismatch: %q", conflict.Ours)
	}
	if string(conflict.Base) != "base version\n" {
		t.Errorf("base mismatch: %q", conflict.Base)
	}
	if string(conflict.Theirs) != "theirs version\n" {
		t.Errorf("theirs mismatch: %q", conflict.Theirs)
	}
}

func TestParseMultiple(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "multiple.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(doc.Conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d", len(doc.Conflicts))
	}

	conflict1, ok := doc.Segments[1].(ConflictSegment)
	if !ok {
		t.Fatalf("segment 1 is not ConflictSegment")
	}
	if string(conflict1.Ours) != "conflict 1 ours\n" {
		t.Errorf("conflict1 ours mismatch: %q", conflict1.Ours)
	}

	conflict2, ok := doc.Segments[3].(ConflictSegment)
	if !ok {
		t.Fatalf("segment 3 is not ConflictSegment")
	}
	if string(conflict2.Ours) != "conflict 2 ours\n" {
		t.Errorf("conflict2 ours mismatch: %q", conflict2.Ours)
	}
}

func TestParseFalsePositive(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "false_positive.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(doc.Conflicts) != 0 {
		t.Errorf("expected 0 conflicts (false positive), got %d", len(doc.Conflicts))
	}

	if len(doc.Segments) != 1 {
		t.Fatalf("expected 1 text segment, got %d", len(doc.Segments))
	}

	text, ok := doc.Segments[0].(TextSegment)
	if !ok {
		t.Fatalf("segment 0 is not TextSegment")
	}
	if string(text.Bytes) != string(data) {
		t.Errorf("text mismatch")
	}
}

func TestParseMalformedNoMid(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "malformed_no_mid.input"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = Parse(data)
	if err == nil {
		t.Fatal("expected error for malformed conflict (no mid marker)")
	}
	if !errors.Is(err, ErrMalformedConflict) {
		t.Errorf("expected ErrMalformedConflict, got %v", err)
	}
}

func TestParseMalformedNoEnd(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "malformed_no_end.input"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = Parse(data)
	if err == nil {
		t.Fatal("expected error for malformed conflict (no end marker)")
	}
	if !errors.Is(err, ErrMalformedConflict) {
		t.Errorf("expected ErrMalformedConflict, got %v", err)
	}
}

func TestParseCRLF(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "crlf.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(doc.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(doc.Conflicts))
	}

	text1 := doc.Segments[0].(TextSegment)
	if text1.Bytes[len(text1.Bytes)-1] != '\n' {
		t.Errorf("text segment should preserve line ending")
	}
}

func TestParseNoTrailingNewline(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "no_trailing_newline.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(doc.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(doc.Conflicts))
	}
}

func TestIsResolved(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		resolved bool
	}{
		{"no_conflict", "hello\nworld\n", true},
		{"has_conflict", "<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\n", false},
		{"false_positive", "comment <<<<<<< not a conflict\n", true},
		{"malformed", "<<<<<<< HEAD\nno end marker\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsResolved([]byte(tt.input))
			if result != tt.resolved {
				t.Errorf("IsResolved(%q) = %v, want %v", tt.name, result, tt.resolved)
			}
		})
	}
}
