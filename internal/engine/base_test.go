package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/markers"
	"github.com/chojs23/ec/internal/mergeview"
)

func sessionForBaseTest(blocks ...mergeview.ConflictBlock) *mergeview.Session {
	segments := make([]mergeview.Segment, 0, len(blocks))
	conflicts := make([]mergeview.ConflictRef, 0, len(blocks))
	for i, block := range blocks {
		segments = append(segments, block)
		conflicts = append(conflicts, mergeview.ConflictRef{SegmentIndex: i})
	}
	return &mergeview.Session{Segments: segments, Conflicts: conflicts}
}

func TestValidateBaseCompletenessSession_AllConflictsHaveBase(t *testing.T) {
	err := ValidateBaseCompletenessSession(sessionForBaseTest(
		mergeview.ConflictBlock{Ours: []byte("ours1\n"), Base: []byte("base1\n"), Theirs: []byte("theirs1\n")},
		mergeview.ConflictBlock{Ours: []byte("ours2\n"), Base: []byte("base2\n"), Theirs: []byte("theirs2\n")},
	))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateBaseCompletenessSession_MissingBase(t *testing.T) {
	err := ValidateBaseCompletenessSession(sessionForBaseTest(
		mergeview.ConflictBlock{Ours: []byte("ours1\n"), Base: []byte("base1\n"), Theirs: []byte("theirs1\n")},
		mergeview.ConflictBlock{Ours: []byte("ours2\n"), Theirs: []byte("theirs2\n")},
	))
	if err == nil {
		t.Fatalf("expected error for missing base chunk")
	}
}

func TestValidateBaseCompletenessSession_EmptyBaseBodyWithLabel(t *testing.T) {
	err := ValidateBaseCompletenessSession(sessionForBaseTest(
		mergeview.ConflictBlock{Ours: []byte("ours\n"), Theirs: []byte("theirs\n"), Labels: mergeview.Labels{BaseLabel: "/tmp/base.txt"}},
	))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestBaseDisplayIntegration_RealConflict(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "base.txt")
	localPath := filepath.Join(tmpDir, "local.txt")
	remotePath := filepath.Join(tmpDir, "remote.txt")
	if err := os.WriteFile(basePath, []byte("line1\nbase content\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localPath, []byte("line1\nlocal change\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remotePath, []byte("line1\nremote change\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	session, err := mergeview.LoadCanonicalSession(ctx, cli.Options{BasePath: basePath, LocalPath: localPath, RemotePath: remotePath})
	if err != nil {
		t.Fatalf("LoadCanonicalSession failed: %v", err)
	}
	if len(session.Conflicts) == 0 {
		t.Fatalf("expected conflict")
	}
	if err := ValidateBaseCompletenessSession(session); err != nil {
		t.Fatalf("ValidateBaseCompletenessSession failed: %v", err)
	}
	block := session.Segments[session.Conflicts[0].SegmentIndex].(mergeview.ConflictBlock)
	if string(block.Base) != "base content\n" || string(block.Ours) != "local change\n" || string(block.Theirs) != "remote change\n" {
		t.Fatalf("block mismatch")
	}
	if err := session.ApplyResolution(0, markers.ResolutionOurs); err != nil {
		t.Fatalf("ApplyResolution failed: %v", err)
	}
	resolved, err := session.Preview()
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	if string(resolved) != "line1\nlocal change\nline3\n" {
		t.Fatalf("resolved = %q", string(resolved))
	}
	parsed, err := mergeview.ParseSession(resolved)
	if err != nil {
		t.Fatalf("ParseSession failed: %v", err)
	}
	if len(parsed.Conflicts) != 0 {
		t.Fatalf("expected no conflicts after preview")
	}
}

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
	if err := OpenBaseFile(baseFile); err != nil {
		t.Fatalf("OpenBaseFile failed: %v", err)
	}
}

func TestOpenBaseFile_MissingFile(t *testing.T) {
	if err := OpenBaseFile("/nonexistent/path/file.txt"); err == nil {
		t.Fatalf("expected error for missing file")
	}
}
