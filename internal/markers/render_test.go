package markers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderResolvedOurs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "2way.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	conflict := doc.Segments[1].(ConflictSegment)
	conflict.Resolution = ResolutionOurs
	doc.Segments[1] = conflict

	rendered, err := RenderResolved(doc)
	if err != nil {
		t.Fatalf("RenderResolved failed: %v", err)
	}

	expected := "before text\nours content\nafter text\n"
	if string(rendered) != expected {
		t.Errorf("rendered mismatch:\ngot  %q\nwant %q", rendered, expected)
	}
}

func TestRenderResolvedTheirs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "2way.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	conflict := doc.Segments[1].(ConflictSegment)
	conflict.Resolution = ResolutionTheirs
	doc.Segments[1] = conflict

	rendered, err := RenderResolved(doc)
	if err != nil {
		t.Fatalf("RenderResolved failed: %v", err)
	}

	expected := "before text\ntheirs content\nafter text\n"
	if string(rendered) != expected {
		t.Errorf("rendered mismatch:\ngot  %q\nwant %q", rendered, expected)
	}
}

func TestRenderResolvedBoth(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "2way.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	conflict := doc.Segments[1].(ConflictSegment)
	conflict.Resolution = ResolutionBoth
	doc.Segments[1] = conflict

	rendered, err := RenderResolved(doc)
	if err != nil {
		t.Fatalf("RenderResolved failed: %v", err)
	}

	expected := "before text\nours content\ntheirs content\nafter text\n"
	if string(rendered) != expected {
		t.Errorf("rendered mismatch:\ngot  %q\nwant %q", rendered, expected)
	}
}

func TestRenderResolvedNone(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "2way.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	conflict := doc.Segments[1].(ConflictSegment)
	conflict.Resolution = ResolutionNone
	doc.Segments[1] = conflict

	rendered, err := RenderResolved(doc)
	if err != nil {
		t.Fatalf("RenderResolved failed: %v", err)
	}

	expected := "before text\nafter text\n"
	if string(rendered) != expected {
		t.Errorf("rendered mismatch:\ngot  %q\nwant %q", rendered, expected)
	}
}

func TestRenderResolvedUnresolved(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "2way.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	_, err = RenderResolved(doc)
	if err == nil {
		t.Fatal("expected error for unresolved conflict")
	}
}

func TestRenderResolvedMultiple(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "multiple.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	conflict1 := doc.Segments[1].(ConflictSegment)
	conflict1.Resolution = ResolutionOurs
	doc.Segments[1] = conflict1

	conflict2 := doc.Segments[3].(ConflictSegment)
	conflict2.Resolution = ResolutionTheirs
	doc.Segments[3] = conflict2

	rendered, err := RenderResolved(doc)
	if err != nil {
		t.Fatalf("RenderResolved failed: %v", err)
	}

	expected := "first line\nconflict 1 ours\nmiddle text\nconflict 2 theirs\nlast line"
	if string(rendered) != expected {
		t.Errorf("rendered mismatch:\ngot  %q\nwant %q", rendered, expected)
	}
}

func TestRenderResolvedPreservesCRLF(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "crlf.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	conflict := doc.Segments[1].(ConflictSegment)
	conflict.Resolution = ResolutionOurs
	doc.Segments[1] = conflict

	rendered, err := RenderResolved(doc)
	if err != nil {
		t.Fatalf("RenderResolved failed: %v", err)
	}

	text1 := doc.Segments[0].(TextSegment)
	if len(text1.Bytes) > 0 && text1.Bytes[len(text1.Bytes)-2] == '\r' && text1.Bytes[len(text1.Bytes)-1] == '\n' {
		if len(rendered) < 2 || rendered[6] != '\r' || rendered[7] != '\n' {
			t.Errorf("CRLF not preserved in rendered output")
		}
	}
}

func TestRenderResolvedNoTrailingNewline(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "no_trailing_newline.input"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	conflict := doc.Segments[0].(ConflictSegment)
	conflict.Resolution = ResolutionOurs
	doc.Segments[0] = conflict

	rendered, err := RenderResolved(doc)
	if err != nil {
		t.Fatalf("RenderResolved failed: %v", err)
	}

	expected := "ours\n"
	if string(rendered) != expected {
		t.Errorf("rendered mismatch:\ngot  %q\nwant %q", rendered, expected)
	}
}
