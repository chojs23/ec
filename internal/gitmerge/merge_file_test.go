package gitmerge

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMergeFileDiff3Clean(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "base.txt")
	localPath := filepath.Join(tmpDir, "local.txt")
	remotePath := filepath.Join(tmpDir, "remote.txt")

	content := []byte("line\n")
	if err := os.WriteFile(basePath, content, 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := os.WriteFile(localPath, content, 0o644); err != nil {
		t.Fatalf("write local: %v", err)
	}
	if err := os.WriteFile(remotePath, content, 0o644); err != nil {
		t.Fatalf("write remote: %v", err)
	}

	got, err := MergeFileDiff3(context.Background(), localPath, basePath, remotePath)
	if err != nil {
		t.Fatalf("MergeFileDiff3 error: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("clean merge output mismatch: %q", string(got))
	}
}

func TestMergeFileDiff3Conflict(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "base.txt")
	localPath := filepath.Join(tmpDir, "local.txt")
	remotePath := filepath.Join(tmpDir, "remote.txt")

	if err := os.WriteFile(basePath, []byte("line\n"), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("line\nlocal\n"), 0o644); err != nil {
		t.Fatalf("write local: %v", err)
	}
	if err := os.WriteFile(remotePath, []byte("line\nremote\n"), 0o644); err != nil {
		t.Fatalf("write remote: %v", err)
	}

	got, err := MergeFileDiff3(context.Background(), localPath, basePath, remotePath)
	if err != nil {
		t.Fatalf("MergeFileDiff3 error: %v", err)
	}
	if !bytes.Contains(got, []byte("<<<<<<<")) || !bytes.Contains(got, []byte("=======")) || !bytes.Contains(got, []byte(">>>>>>>")) {
		t.Fatalf("expected conflict markers in output")
	}
}
