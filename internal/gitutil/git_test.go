package gitutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoRootSuccess(t *testing.T) {
	withFakeGit(t, `#!/bin/sh
if [ "$1" = "rev-parse" ] && [ "$2" = "--show-toplevel" ]; then
  echo "/tmp/repo"
  exit 0
fi
echo "unexpected args" 1>&2
exit 1
`)

	rootDir := t.TempDir()
	root, err := RepoRoot(context.Background(), rootDir)
	if err != nil {
		t.Fatalf("RepoRoot error: %v", err)
	}
	if root != "/tmp/repo" {
		t.Fatalf("RepoRoot = %q, want /tmp/repo", root)
	}
}

func TestRepoRootFailure(t *testing.T) {
	withFakeGit(t, "#!/bin/sh\nexit 1\n")

	rootDir := t.TempDir()
	if _, err := RepoRoot(context.Background(), rootDir); err == nil {
		t.Fatalf("expected error")
	}
}

func TestListUnmergedFiles(t *testing.T) {
	withFakeGit(t, `#!/bin/sh
if [ "$1" = "diff" ] && [ "$2" = "--name-only" ] && [ "$3" = "--diff-filter=U" ]; then
  echo "a.txt"
  echo "dir/b.txt"
  exit 0
fi
exit 1
`)

	repoRoot := t.TempDir()
	paths, err := ListUnmergedFiles(context.Background(), repoRoot, ".")
	if err != nil {
		t.Fatalf("ListUnmergedFiles error: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if paths[0] != "a.txt" || paths[1] != "dir/b.txt" {
		t.Fatalf("unexpected paths: %v", paths)
	}
}

func TestListUnmergedFilesEmpty(t *testing.T) {
	withFakeGit(t, "#!/bin/sh\nexit 0\n")

	repoRoot := t.TempDir()
	paths, err := ListUnmergedFiles(context.Background(), repoRoot, ".")
	if err != nil {
		t.Fatalf("ListUnmergedFiles error: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected no paths, got %v", paths)
	}
}

func TestShowStage(t *testing.T) {
	withFakeGit(t, `#!/bin/sh
if [ "$1" = "show" ] && [ "$2" = ":2:file.txt" ]; then
  printf "content\n"
  exit 0
fi
exit 1
`)

	repoRoot := t.TempDir()
	data, err := ShowStage(context.Background(), repoRoot, 2, "file.txt")
	if err != nil {
		t.Fatalf("ShowStage error: %v", err)
	}
	if string(data) != "content\n" {
		t.Fatalf("ShowStage data = %q", string(data))
	}
}

func withFakeGit(t *testing.T, script string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "git")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}

	original := os.Getenv("PATH")
	pathEnv := strings.Join([]string{dir, original}, string(os.PathListSeparator))
	t.Setenv("PATH", pathEnv)
}
