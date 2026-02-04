package run

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
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
