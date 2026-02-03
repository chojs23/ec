package engine

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chojs23/easy-conflict/internal/cli"
	"github.com/chojs23/easy-conflict/internal/gitmerge"
)

func TestApplyAllAndWrite_WritesResolvedAndBackup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration-style test in short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	basePath := filepath.Join(tmpDir, "base.txt")
	localPath := filepath.Join(tmpDir, "local.txt")
	remotePath := filepath.Join(tmpDir, "remote.txt")
	mergedPath := filepath.Join(tmpDir, "merged.txt")

	baseContent := "line1\nbase content\nline3\n"
	localContent := "line1\nlocal change\nline3\n"
	remoteContent := "line1\nremote change\nline3\n"

	if err := os.WriteFile(basePath, []byte(baseContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localPath, []byte(localContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remotePath, []byte(remoteContent), 0o644); err != nil {
		t.Fatal(err)
	}

	mergeView, err := gitmerge.MergeFileDiff3(ctx, localPath, basePath, remotePath)
	if err != nil {
		t.Fatalf("MergeFileDiff3 failed: %v", err)
	}
	if err := os.WriteFile(mergedPath, mergeView, 0o644); err != nil {
		t.Fatal(err)
	}

	opts := cli.Options{
		BasePath:   basePath,
		LocalPath:  localPath,
		RemotePath: remotePath,
		MergedPath: mergedPath,
		ApplyAll:   "ours",
		Backup:     true,
	}

	if err := ApplyAllAndWrite(ctx, opts); err != nil {
		t.Fatalf("ApplyAllAndWrite failed: %v", err)
	}

	resolved, err := os.ReadFile(mergedPath)
	if err != nil {
		t.Fatal(err)
	}

	expected := "line1\nlocal change\nline3\n"
	if string(resolved) != expected {
		t.Errorf("resolved output mismatch:\nexpected: %q\ngot: %q", expected, string(resolved))
	}

	bakPath := mergedPath + ".easy-conflict.bak"
	bak, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("backup not found: %v", err)
	}
	if !bytes.Equal(bak, mergeView) {
		t.Errorf("backup mismatch: expected original merged content")
	}
}

func TestApplyAllAndWrite_NoConflictsNoWrite(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	mergedPath := filepath.Join(tmpDir, "merged.txt")
	if err := os.WriteFile(mergedPath, []byte("clean file\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := cli.Options{
		MergedPath: mergedPath,
		ApplyAll:   "ours",
		Backup:     true,
	}

	if err := ApplyAllAndWrite(ctx, opts); err != nil {
		t.Fatalf("ApplyAllAndWrite failed: %v", err)
	}

	data, err := os.ReadFile(mergedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "clean file\n" {
		t.Fatalf("merged content changed unexpectedly: %q", string(data))
	}

	if _, err := os.Stat(mergedPath + ".easy-conflict.bak"); err == nil {
		t.Fatalf("expected no backup file when no conflicts")
	}
}

func TestCheckResolvedFile(t *testing.T) {
	tmpDir := t.TempDir()

	resolvedPath := filepath.Join(tmpDir, "resolved.txt")
	if err := os.WriteFile(resolvedPath, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := CheckResolvedFile(resolvedPath)
	if err != nil {
		t.Fatalf("CheckResolvedFile error: %v", err)
	}
	if !resolved {
		t.Fatalf("expected resolved true")
	}

	unresolvedPath := filepath.Join(tmpDir, "unresolved.txt")
	if err := os.WriteFile(unresolvedPath, []byte("<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err = CheckResolvedFile(unresolvedPath)
	if err != nil {
		t.Fatalf("CheckResolvedFile error: %v", err)
	}
	if resolved {
		t.Fatalf("expected resolved false")
	}

	malformedPath := filepath.Join(tmpDir, "malformed.txt")
	if err := os.WriteFile(malformedPath, []byte("<<<<<<< HEAD\nno end\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := CheckResolvedFile(malformedPath); err == nil {
		t.Fatalf("expected error for malformed markers")
	}
}
