package run

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chojs23/easy-conflict/internal/cli"
	"github.com/chojs23/easy-conflict/internal/gitmerge"
)

func TestRunCheckResolvedExitCodes(t *testing.T) {
	tmpDir := t.TempDir()

	resolvedPath := filepath.Join(tmpDir, "resolved.txt")
	if err := os.WriteFile(resolvedPath, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code := Run(context.Background(), cli.Options{Check: true, MergedPath: resolvedPath})
	if code != 0 {
		t.Fatalf("resolved check exit code = %d, want 0", code)
	}

	unresolvedPath := filepath.Join(tmpDir, "unresolved.txt")
	if err := os.WriteFile(unresolvedPath, []byte("<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code = Run(context.Background(), cli.Options{Check: true, MergedPath: unresolvedPath})
	if code != 1 {
		t.Fatalf("unresolved check exit code = %d, want 1", code)
	}
}

func TestRunApplyAllExitCodes(t *testing.T) {
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

	code := Run(ctx, cli.Options{
		BasePath:   basePath,
		LocalPath:  localPath,
		RemotePath: remotePath,
		MergedPath: mergedPath,
		ApplyAll:   "ours",
	})
	if code != 0 {
		t.Fatalf("apply-all exit code = %d, want 0", code)
	}

	data, err := os.ReadFile(mergedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "line1\nlocal change\nline3\n" {
		t.Fatalf("resolved content mismatch: %q", string(data))
	}

	code = Run(ctx, cli.Options{
		BasePath:   basePath,
		LocalPath:  localPath,
		RemotePath: remotePath,
		MergedPath: filepath.Join(tmpDir, "missing.txt"),
		ApplyAll:   "ours",
	})
	if code != 2 {
		t.Fatalf("apply-all error exit code = %d, want 2", code)
	}
}
