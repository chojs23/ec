package run

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/gitmerge"
	"github.com/chojs23/ec/internal/tui"
)

func TestRunInteractiveSavedOutputExitCodes(t *testing.T) {
	unresolved := "<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\n"
	for _, tc := range []struct {
		name   string
		saved  string
		tuiErr error
		want   int
	}{
		{name: "resolved", saved: "resolved\n", want: 0},
		{name: "resolved on quit", saved: "resolved\n", tuiErr: tui.ErrBackToSelector, want: 0},
		{name: "unresolved", saved: unresolved, want: 1},
		{name: "unresolved on quit", saved: unresolved, tuiErr: tui.ErrBackToSelector, want: 1},
		{name: "TUI error", saved: "resolved\n", tuiErr: errors.New("TUI failed"), want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mergedPath := filepath.Join(t.TempDir(), "merged.txt")
			if err := os.WriteFile(mergedPath, []byte("before TUI\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			previous := runResolver
			t.Cleanup(func() { runResolver = previous })
			called := false
			runResolver = func(_ context.Context, opts cli.Options) error {
				called = true
				if !opts.ConfirmIncompleteQuit {
					t.Fatal("explicit-path session must confirm incomplete quits")
				}
				if opts.MergedPath != mergedPath {
					t.Fatalf("merged path = %q, want %q", opts.MergedPath, mergedPath)
				}
				if err := os.WriteFile(mergedPath, []byte(tc.saved), 0o644); err != nil {
					t.Fatal(err)
				}
				return tc.tuiErr
			}

			code := Run(context.Background(), cli.Options{
				BasePath:   "base.txt",
				LocalPath:  "local.txt",
				RemotePath: "remote.txt",
				MergedPath: mergedPath,
			})
			if !called {
				t.Fatal("explicit-path session did not run the resolver")
			}
			if code != tc.want {
				t.Fatalf("interactive exit code = %d, want %d", code, tc.want)
			}
		})
	}
}

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
