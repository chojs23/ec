package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chojs23/easy-conflict/internal/gitmerge"
	"github.com/chojs23/easy-conflict/internal/markers"
)

// TestValidateBaseCompleteness_AllConflictsHaveBase tests that validation passes
// when all conflicts have base chunks.
func TestValidateBaseCompleteness_AllConflictsHaveBase(t *testing.T) {
	doc := markers.Document{
		Segments: []markers.Segment{
			markers.ConflictSegment{
				Ours:   []byte("ours1\n"),
				Base:   []byte("base1\n"),
				Theirs: []byte("theirs1\n"),
			},
			markers.ConflictSegment{
				Ours:   []byte("ours2\n"),
				Base:   []byte("base2\n"),
				Theirs: []byte("theirs2\n"),
			},
		},
		Conflicts: []markers.ConflictRef{
			{SegmentIndex: 0},
			{SegmentIndex: 1},
		},
	}

	err := ValidateBaseCompleteness(doc)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// TestValidateBaseCompleteness_MissingBase tests that validation fails
// when a conflict is missing its base chunk.
func TestValidateBaseCompleteness_MissingBase(t *testing.T) {
	doc := markers.Document{
		Segments: []markers.Segment{
			markers.ConflictSegment{
				Ours:   []byte("ours1\n"),
				Base:   []byte("base1\n"),
				Theirs: []byte("theirs1\n"),
			},
			markers.ConflictSegment{
				Ours:   []byte("ours2\n"),
				Base:   nil,
				Theirs: []byte("theirs2\n"),
			},
		},
		Conflicts: []markers.ConflictRef{
			{SegmentIndex: 0},
			{SegmentIndex: 1},
		},
	}

	err := ValidateBaseCompleteness(doc)
	if err == nil {
		t.Fatal("expected error for missing base chunk, got nil")
	}
	if !contains(err.Error(), "conflict 1") || !contains(err.Error(), "missing base chunk") {
		t.Errorf("expected error about conflict 1 missing base chunk, got: %v", err)
	}
}

// TestBaseDisplayIntegration_RealGitConflict creates a real git conflict using
// temp git repos and validates that the diff3 view has base chunks for all conflicts.
func TestBaseDisplayIntegration_RealGitConflict(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	baseDir := filepath.Join(tmpDir, "base")
	localDir := filepath.Join(tmpDir, "local")
	remoteDir := filepath.Join(tmpDir, "remote")

	for _, dir := range []string{baseDir, localDir, remoteDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	file := "conflict.txt"
	baseContent := "line1\nbase content\nline3\n"
	localContent := "line1\nlocal change\nline3\n"
	remoteContent := "line1\nremote change\nline3\n"

	basePath := filepath.Join(baseDir, file)
	localPath := filepath.Join(localDir, file)
	remotePath := filepath.Join(remoteDir, file)

	if err := os.WriteFile(basePath, []byte(baseContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localPath, []byte(localContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remotePath, []byte(remoteContent), 0o644); err != nil {
		t.Fatal(err)
	}

	mergeViewBytes, err := gitmerge.MergeFileDiff3(ctx, localPath, basePath, remotePath)
	if err != nil {
		t.Fatalf("MergeFileDiff3 failed: %v", err)
	}

	viewDoc, err := markers.Parse(mergeViewBytes)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(viewDoc.Conflicts) == 0 {
		t.Fatal("expected at least one conflict in merge view")
	}

	if err := ValidateBaseCompleteness(viewDoc); err != nil {
		t.Errorf("ValidateBaseCompleteness failed: %v", err)
	}

	for i, ref := range viewDoc.Conflicts {
		seg, ok := viewDoc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
		if !ok {
			t.Fatalf("conflict %d is not a ConflictSegment", i)
		}

		if len(seg.Base) == 0 {
			t.Errorf("conflict %d has empty base chunk", i)
		}

		if !bytes.Contains(seg.Ours, []byte("local change")) {
			t.Errorf("conflict %d ours section missing expected content", i)
		}
		if !bytes.Contains(seg.Theirs, []byte("remote change")) {
			t.Errorf("conflict %d theirs section missing expected content", i)
		}
		if !bytes.Contains(seg.Base, []byte("base content")) {
			t.Errorf("conflict %d base section missing expected content", i)
		}
	}

	seg, _ := viewDoc.Segments[viewDoc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
	seg.Resolution = markers.ResolutionOurs
	viewDoc.Segments[viewDoc.Conflicts[0].SegmentIndex] = seg

	resolved, err := markers.RenderResolved(viewDoc)
	if err != nil {
		t.Fatalf("RenderResolved failed: %v", err)
	}

	expectedResolved := "line1\nlocal change\nline3\n"
	if string(resolved) != expectedResolved {
		t.Errorf("resolved output mismatch:\nexpected: %q\ngot: %q", expectedResolved, string(resolved))
	}

	postDoc, err := markers.Parse(resolved)
	if err != nil {
		t.Fatalf("post-parse resolved: %v", err)
	}
	if len(postDoc.Conflicts) != 0 {
		t.Errorf("expected 0 conflicts after resolution, got %d", len(postDoc.Conflicts))
	}
}

// TestOpenBaseFile_WithPager tests that OpenBaseFile uses $PAGER when set.
func TestOpenBaseFile_WithPager(t *testing.T) {
	tmpDir := t.TempDir()
	baseFile := filepath.Join(tmpDir, "base.txt")
	if err := os.WriteFile(baseFile, []byte("base content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	testScript := filepath.Join(tmpDir, "test-pager.sh")
	if err := os.WriteFile(testScript, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	originalPager := os.Getenv("PAGER")
	defer os.Setenv("PAGER", originalPager)

	os.Setenv("PAGER", testScript)

	err := OpenBaseFile(baseFile)
	if err != nil {
		t.Errorf("OpenBaseFile failed: %v", err)
	}
}

// TestOpenBaseFile_MissingFile tests that OpenBaseFile returns error for non-existent file.
func TestOpenBaseFile_MissingFile(t *testing.T) {
	err := OpenBaseFile("/nonexistent/path/file.txt")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			bytes.Contains([]byte(s), []byte(substr))))
}

// TestBaseDisplayIntegration_MultipleConflicts tests that multiple conflicts
// in a single file all have base chunks.
func TestBaseDisplayIntegration_MultipleConflicts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	baseContent := "section1\nbase1\nsection2\nbase2\nsection3\n"
	localContent := "section1\nlocal1\nsection2\nlocal2\nsection3\n"
	remoteContent := "section1\nremote1\nsection2\nremote2\nsection3\n"

	localPath := filepath.Join(tmpDir, "local.txt")
	basePath := filepath.Join(tmpDir, "base.txt")
	remotePath := filepath.Join(tmpDir, "remote.txt")

	if err := os.WriteFile(basePath, []byte(baseContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localPath, []byte(localContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remotePath, []byte(remoteContent), 0o644); err != nil {
		t.Fatal(err)
	}

	mergeViewBytes, err := gitmerge.MergeFileDiff3(ctx, localPath, basePath, remotePath)
	if err != nil {
		t.Fatalf("MergeFileDiff3 failed: %v", err)
	}

	viewDoc, err := markers.Parse(mergeViewBytes)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(viewDoc.Conflicts) < 2 {
		t.Fatalf("expected at least 2 conflicts, got %d", len(viewDoc.Conflicts))
	}

	if err := ValidateBaseCompleteness(viewDoc); err != nil {
		t.Errorf("ValidateBaseCompleteness failed: %v", err)
	}

	for i, ref := range viewDoc.Conflicts {
		seg, ok := viewDoc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
		if !ok {
			t.Fatalf("conflict %d is not a ConflictSegment", i)
		}

		if len(seg.Base) == 0 {
			t.Errorf("conflict %d has empty base chunk", i)
		}
	}
}
