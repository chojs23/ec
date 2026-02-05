package run

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chojs23/ec/internal/cli"
)

func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdin: %v", err)
	}
	if _, err := w.WriteString(input); err != nil {
		w.Close()
		r.Close()
		t.Fatalf("write stdin: %v", err)
	}
	if err := w.Close(); err != nil {
		r.Close()
		t.Fatalf("close stdin writer: %v", err)
	}

	old := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = old
		r.Close()
	}()

	fn()
}

func withStdout(t *testing.T, fn func()) {
	t.Helper()
	f, err := os.CreateTemp("", "ec-stdout-*")
	if err != nil {
		t.Fatalf("temp stdout: %v", err)
	}
	old := os.Stdout
	os.Stdout = f
	defer func() {
		os.Stdout = old
		f.Close()
		os.Remove(f.Name())
	}()

	fn()
}

func TestSelectPathSingle(t *testing.T) {
	selected, err := selectPath([]string{"only.txt"})
	if err != nil {
		t.Fatalf("selectPath error: %v", err)
	}
	if selected != "only.txt" {
		t.Fatalf("selectPath = %q, want %q", selected, "only.txt")
	}
}

func TestSelectPathWithInput(t *testing.T) {
	withStdout(t, func() {
		withStdin(t, "0\n2\n", func() {
			selected, err := selectPath([]string{"a.txt", "b.txt"})
			if err != nil {
				t.Fatalf("selectPath error: %v", err)
			}
			if selected != "b.txt" {
				t.Fatalf("selectPath = %q, want %q", selected, "b.txt")
			}
		})
	})
}

func TestSelectPathInvalidAfterRetries(t *testing.T) {
	withStdout(t, func() {
		withStdin(t, "0\n0\n0\n", func() {
			_, err := selectPath([]string{"a.txt", "b.txt"})
			if err == nil {
				t.Fatalf("selectPath expected error")
			}
		})
	})
}

func TestIsTTYFalseForRegularFile(t *testing.T) {
	f, err := os.CreateTemp("", "ec-tty-*")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()

	if isTTY(f) {
		t.Fatalf("isTTY returned true for regular file")
	}
}

func TestBuildFileCandidates(t *testing.T) {
	tmpDir := t.TempDir()
	resolvedPath := filepath.Join(tmpDir, "resolved.txt")
	unresolvedPath := filepath.Join(tmpDir, "unresolved.txt")

	if err := os.WriteFile(resolvedPath, []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write resolved: %v", err)
	}
	if err := os.WriteFile(unresolvedPath, []byte("<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\n"), 0o644); err != nil {
		t.Fatalf("write unresolved: %v", err)
	}

	candidates, err := buildFileCandidates(tmpDir, []string{"resolved.txt", "unresolved.txt"})
	if err != nil {
		t.Fatalf("buildFileCandidates error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates len = %d, want 2", len(candidates))
	}
	if !candidates[0].Resolved {
		t.Fatalf("resolved file marked unresolved")
	}
	if candidates[1].Resolved {
		t.Fatalf("unresolved file marked resolved")
	}
}

func TestWriteTempStages(t *testing.T) {
	base := []byte("base\n")
	local := []byte("local\n")
	remote := []byte("remote\n")

	basePath, localPath, remotePath, cleanup, err := writeTempStages(base, local, remote)
	if err != nil {
		t.Fatalf("writeTempStages error: %v", err)
	}
	defer cleanup()

	gotBase, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("read base: %v", err)
	}
	gotLocal, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read local: %v", err)
	}
	gotRemote, err := os.ReadFile(remotePath)
	if err != nil {
		t.Fatalf("read remote: %v", err)
	}
	if !bytes.Equal(gotBase, base) {
		t.Fatalf("base content mismatch")
	}
	if !bytes.Equal(gotLocal, local) {
		t.Fatalf("local content mismatch")
	}
	if !bytes.Equal(gotRemote, remote) {
		t.Fatalf("remote content mismatch")
	}

	cleanup()
	if _, err := os.Stat(basePath); err == nil {
		t.Fatalf("base temp file still exists")
	} else if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}

func TestIsInteractiveTTYFalse(t *testing.T) {
	withStdout(t, func() {
		withStdin(t, "", func() {
			if isInteractiveTTY() {
				t.Fatalf("isInteractiveTTY returned true")
			}
		})
	})
}

func TestSelectPathInteractiveNonTTY(t *testing.T) {
	withStdout(t, func() {
		withStdin(t, "2\n", func() {
			selected, err := selectPathInteractive(context.Background(), "repo", []string{"a.txt", "b.txt"})
			if err != nil {
				t.Fatalf("selectPathInteractive error: %v", err)
			}
			if selected != "b.txt" {
				t.Fatalf("selectPathInteractive = %q, want b.txt", selected)
			}
		})
	})
}

func TestPrepareInteractiveFromRepoPopulatesOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")

	conflictPath := filepath.Join(repoDir, "conflict.txt")
	if err := os.WriteFile(conflictPath, []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	runGit(t, repoDir, "add", "conflict.txt")
	runGit(t, repoDir, "commit", "-m", "base")

	runGit(t, repoDir, "checkout", "-b", "feature")
	if err := os.WriteFile(conflictPath, []byte("theirs\n"), 0o644); err != nil {
		t.Fatalf("write theirs: %v", err)
	}
	runGit(t, repoDir, "add", "conflict.txt")
	runGit(t, repoDir, "commit", "-m", "theirs")

	runGit(t, repoDir, "checkout", "-")
	if err := os.WriteFile(conflictPath, []byte("ours\n"), 0o644); err != nil {
		t.Fatalf("write ours: %v", err)
	}
	runGit(t, repoDir, "add", "conflict.txt")
	runGit(t, repoDir, "commit", "-m", "ours")

	mergeCmd := exec.Command("git", "merge", "feature")
	mergeCmd.Dir = repoDir
	if output, err := mergeCmd.CombinedOutput(); err == nil {
		t.Fatalf("expected merge conflict, got success: %s", string(output))
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd error: %v", err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir error: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore cwd error: %v", err)
		}
	})

	var opts cli.Options
	var cleanup func()
	withStdout(t, func() {
		withStdin(t, "", func() {
			cleanup, err = prepareInteractiveFromRepo(context.Background(), &opts)
		})
	})
	if err != nil {
		t.Fatalf("prepareInteractiveFromRepo error: %v", err)
	}
	if cleanup == nil {
		t.Fatalf("cleanup function is nil")
	}
	t.Cleanup(cleanup)

	if opts.AllowMissingBase {
		t.Fatalf("AllowMissingBase = true, want false")
	}
	if opts.MergedPath == "" || opts.BasePath == "" || opts.LocalPath == "" || opts.RemotePath == "" {
		t.Fatalf("expected options paths to be set")
	}

	baseBytes, err := os.ReadFile(opts.BasePath)
	if err != nil {
		t.Fatalf("read base temp file: %v", err)
	}
	localBytes, err := os.ReadFile(opts.LocalPath)
	if err != nil {
		t.Fatalf("read local temp file: %v", err)
	}
	remoteBytes, err := os.ReadFile(opts.RemotePath)
	if err != nil {
		t.Fatalf("read remote temp file: %v", err)
	}
	if string(baseBytes) != "base\n" {
		t.Fatalf("base temp content = %q, want base", string(baseBytes))
	}
	if string(localBytes) != "ours\n" {
		t.Fatalf("local temp content = %q, want ours", string(localBytes))
	}
	if string(remoteBytes) != "theirs\n" {
		t.Fatalf("remote temp content = %q, want theirs", string(remoteBytes))
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}
