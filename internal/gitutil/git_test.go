package gitutil

import (
	"context"
	"os"
	"os/exec"
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
	withFakeGit(t, "#!/bin/sh\necho 'fatal: not a git repository' 1>&2\nexit 1\n")

	rootDir := t.TempDir()
	_, err := RepoRoot(context.Background(), rootDir)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "fatal: not a git repository") {
		t.Fatalf("expected stderr in error, got %v", err)
	}

}

func TestListUnmergedFiles(t *testing.T) {
	withFakeGit(t, `#!/bin/sh
last=""
for arg do
  last="$arg"
done
if [ "$1" = "diff" ] && [ "$2" = "--name-only" ] && [ "$3" = "-z" ] && [ "$4" = "--diff-filter=U" ] && [ "$last" = ":(literal)dir/[abc]*" ]; then
  printf '%s\0' "dir/[abc]*/ leading.txt" "dir/[abc]*/b.txt"
  exit 0
fi
echo "unexpected args: $*" 1>&2
exit 1
`)

	repoRoot := t.TempDir()
	paths, err := ListUnmergedFiles(context.Background(), repoRoot, "dir/[abc]*")
	if err != nil {
		t.Fatalf("ListUnmergedFiles error: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if paths[0] != "dir/[abc]*/ leading.txt" || paths[1] != "dir/[abc]*/b.txt" {
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

func TestUnmergedStages(t *testing.T) {
	withFakeGit(t, `#!/bin/sh
if [ "$1" = "ls-files" ] && [ "$2" = "-u" ] && [ "$3" = "-z" ]; then
  printf '100644 abc 1\tfile.txt\0'
  printf '100755 def 2\tfile.txt\0'
  printf '120000 ghi 3\tfile.txt\0'
  exit 0
fi
exit 1
`)

	repoRoot := t.TempDir()
	stages, err := UnmergedStages(context.Background(), repoRoot, "file.txt")
	if err != nil {
		t.Fatalf("UnmergedStages error: %v", err)
	}
	if stages[1].Mode != "100644" || stages[2].Mode != "100755" || stages[3].Mode != "120000" {
		t.Fatalf("unexpected stages: %#v", stages)
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

func TestWorkingTreeDiffIncludesTrackedRenamedAndUntrackedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	repoRoot := t.TempDir()
	runGitTest(t, repoRoot, "init")
	runGitTest(t, repoRoot, "config", "user.email", "test@example.com")
	runGitTest(t, repoRoot, "config", "user.name", "Test User")

	writeTestFile(t, repoRoot, "tracked.txt", "before\n")
	writeTestFile(t, repoRoot, "old name.txt", "rename me\n")
	writeTestFile(t, repoRoot, "deleted.txt", "delete me\n")
	runGitTest(t, repoRoot, "add", ".")
	runGitTest(t, repoRoot, "commit", "-m", "base")

	writeTestFile(t, repoRoot, "tracked.txt", "after\n")
	runGitTest(t, repoRoot, "mv", "old name.txt", "new name.txt")
	if err := os.Remove(filepath.Join(repoRoot, "deleted.txt")); err != nil {
		t.Fatalf("remove deleted.txt: %v", err)
	}
	writeTestFile(t, repoRoot, "staged.txt", "staged\n")
	runGitTest(t, repoRoot, "add", "staged.txt")
	writeTestFile(t, repoRoot, "untracked.txt", "untracked\n")

	files, err := ListDiffFiles(context.Background(), repoRoot, WorkingTreeSource(), ".")
	if err != nil {
		t.Fatalf("ListDiffFiles error: %v", err)
	}

	byPath := make(map[string]DiffFile, len(files))
	for _, file := range files {
		byPath[file.Path] = file
	}
	for _, path := range []string{"tracked.txt", "new name.txt", "deleted.txt", "staged.txt", "untracked.txt"} {
		if _, ok := byPath[path]; !ok {
			t.Fatalf("changed files = %#v, want %q", files, path)
		}
	}

	rename := byPath["new name.txt"]
	if rename.OldPath != "old name.txt" || !strings.HasPrefix(rename.Status, "R") {
		t.Fatalf("rename = %#v, want old and new paths", rename)
	}
	if deleted := byPath["deleted.txt"]; deleted.Status != "D" {
		t.Fatalf("deleted = %#v, want deleted status", deleted)
	}
	untracked := byPath["untracked.txt"]
	if !untracked.Untracked || untracked.Status != "?" {
		t.Fatalf("untracked = %#v, want untracked marker", untracked)
	}

	renamePatch, err := DiffFilePatch(context.Background(), repoRoot, WorkingTreeSource(), rename)
	if err != nil {
		t.Fatalf("DiffFilePatch rename error: %v", err)
	}
	if !strings.Contains(string(renamePatch), "rename from old name.txt") || !strings.Contains(string(renamePatch), "rename to new name.txt") {
		t.Fatalf("rename patch = %q, want rename metadata", string(renamePatch))
	}

	untrackedPatch, err := DiffFilePatch(context.Background(), repoRoot, WorkingTreeSource(), untracked)
	if err != nil {
		t.Fatalf("DiffFilePatch untracked error: %v", err)
	}
	if !strings.Contains(string(untrackedPatch), "+untracked") {
		t.Fatalf("untracked patch = %q, want added content", string(untrackedPatch))
	}

}

func TestWorkingTreeDiffWithoutHeadUsesCurrentFileContent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	repoRoot := t.TempDir()
	runGitTest(t, repoRoot, "init")
	writeTestFile(t, repoRoot, "tracked.txt", "staged\n")
	runGitTest(t, repoRoot, "add", "tracked.txt")
	writeTestFile(t, repoRoot, "tracked.txt", "working\n")
	writeTestFile(t, repoRoot, "untracked.txt", "new\n")

	files, err := ListDiffFiles(context.Background(), repoRoot, WorkingTreeSource(), ".")
	if err != nil {
		t.Fatalf("ListDiffFiles error: %v", err)
	}
	if len(files) != 2 || files[0].Path != "tracked.txt" || files[1].Path != "untracked.txt" {
		t.Fatalf("changed files = %#v, want tracked and untracked files", files)
	}

	patch, err := DiffFilePatch(context.Background(), repoRoot, WorkingTreeSource(), files[0])
	if err != nil {
		t.Fatalf("DiffFilePatch error: %v", err)
	}
	if !strings.Contains(string(patch), "+working") || strings.Contains(string(patch), "+staged") {
		t.Fatalf("patch = %q, want current working file content", string(patch))
	}
}

func TestCommitDiffSourcesCoverParentAndRootCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	repoRoot := t.TempDir()
	runGitTest(t, repoRoot, "init")
	runGitTest(t, repoRoot, "config", "user.email", "test@example.com")
	runGitTest(t, repoRoot, "config", "user.name", "Test User")

	writeTestFile(t, repoRoot, "root.txt", "root one\nroot two\n")
	runGitTest(t, repoRoot, "add", "root.txt")
	runGitTest(t, repoRoot, "commit", "-m", "root commit")
	rootHash := runGitTest(t, repoRoot, "rev-parse", "HEAD")

	writeTestFile(t, repoRoot, "root.txt", "changed\nroot two\n")
	writeTestFile(t, repoRoot, "next.txt", "next\nline\n")
	runGitTest(t, repoRoot, "add", "root.txt", "next.txt")
	runGitTest(t, repoRoot, "commit", "-m", "next commit")

	sources, err := RecentCommitSources(context.Background(), repoRoot, ".", 10)
	if err != nil {
		t.Fatalf("RecentCommitSources error: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("sources len = %d, want 2", len(sources))
	}
	if sources[0].Subject != "next commit" || sources[0].BaseCommit != rootHash {
		t.Fatalf("latest source = %#v, want first-parent comparison", sources[0])
	}
	if !sources[0].StatsKnown || sources[0].Deletions != 1 || sources[0].Additions != 3 {
		t.Fatalf("latest statistics = %#v, want -1 +3", sources[0])
	}
	if sources[1].Subject != "root commit" || sources[1].BaseCommit == "" {
		t.Fatalf("root source = %#v, want empty-tree comparison", sources[1])
	}
	if !sources[1].StatsKnown || sources[1].Deletions != 0 || sources[1].Additions != 2 {
		t.Fatalf("root statistics = %#v, want -0 +2", sources[1])
	}

	rootFiles, err := ListDiffFiles(context.Background(), repoRoot, sources[1], ".")
	if err != nil {
		t.Fatalf("ListDiffFiles root error: %v", err)
	}
	if len(rootFiles) != 1 || rootFiles[0].Path != "root.txt" || rootFiles[0].Status != "A" {
		t.Fatalf("root files = %#v, want added root.txt", rootFiles)
	}

	patch, err := DiffFilePatch(context.Background(), repoRoot, sources[0], DiffFile{Path: "next.txt", Status: "A"})
	if err != nil {
		t.Fatalf("DiffFilePatch commit error: %v", err)
	}
	if !strings.Contains(string(patch), "+next") {
		t.Fatalf("commit patch = %q, want next file content", string(patch))
	}

}

func TestCommitDiffSourcesCompareMergeWithFirstParent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	repoRoot := t.TempDir()
	runGitTest(t, repoRoot, "init")
	runGitTest(t, repoRoot, "config", "user.email", "test@example.com")
	runGitTest(t, repoRoot, "config", "user.name", "Test User")

	writeTestFile(t, repoRoot, "base.txt", "base\n")
	runGitTest(t, repoRoot, "add", "base.txt")
	runGitTest(t, repoRoot, "commit", "-m", "base")
	mainBranch := runGitTest(t, repoRoot, "branch", "--show-current")

	runGitTest(t, repoRoot, "checkout", "-b", "feature")
	writeTestFile(t, repoRoot, "feature.txt", "feature one\nfeature two\n")
	runGitTest(t, repoRoot, "add", "feature.txt")
	runGitTest(t, repoRoot, "commit", "-m", "feature")

	runGitTest(t, repoRoot, "checkout", mainBranch)
	writeTestFile(t, repoRoot, "main.txt", "main\n")
	runGitTest(t, repoRoot, "add", "main.txt")
	runGitTest(t, repoRoot, "commit", "-m", "main")
	firstParent := runGitTest(t, repoRoot, "rev-parse", "HEAD")
	runGitTest(t, repoRoot, "merge", "--no-ff", "feature", "-m", "merge feature")

	sources, err := RecentCommitSources(context.Background(), repoRoot, ".", 1)
	if err != nil {
		t.Fatalf("RecentCommitSources error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("sources len = %d, want merge commit", len(sources))
	}
	if sources[0].Subject != "merge feature" || sources[0].BaseCommit != firstParent {
		t.Fatalf("merge source = %#v, want first-parent comparison", sources[0])
	}
	if !sources[0].StatsKnown || sources[0].Deletions != 0 || sources[0].Additions != 2 {
		t.Fatalf("merge statistics = %#v, want -0 +2", sources[0])
	}
}

func TestApplyCommitDiffTotalsHandlesRenamesAndBinaryFiles(t *testing.T) {
	sources := []DiffSource{{Commit: "one"}, {Commit: "two"}}
	output := []byte("one\x002\t1\tfile.txt\x00-\t-\tbinary.dat\x00two\x005\t0\t\x00old.txt\x00new.txt\x00")

	if err := applyCommitDiffTotals(output, sources); err != nil {
		t.Fatalf("applyCommitDiffTotals error: %v", err)
	}
	if !sources[0].StatsKnown || sources[0].Additions != 2 || sources[0].Deletions != 1 {
		t.Fatalf("first source = %#v, want text totals and ignored binary counts", sources[0])
	}
	if !sources[1].StatsKnown || sources[1].Additions != 5 || sources[1].Deletions != 0 {
		t.Fatalf("second source = %#v, want rename totals", sources[1])
	}
}

func TestParseNameStatusRejectsMalformedRecords(t *testing.T) {
	cases := [][]byte{
		[]byte("M\x00"),
		[]byte("R100\x00old.txt\x00"),
		[]byte("\x00file.txt\x00"),
	}
	for _, input := range cases {
		if _, err := parseNameStatus(input); err == nil {
			t.Fatalf("parseNameStatus(%q) error = nil, want error", input)
		}
	}
}

func writeTestFile(t *testing.T, root string, path string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
	return strings.TrimSpace(string(output))
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
