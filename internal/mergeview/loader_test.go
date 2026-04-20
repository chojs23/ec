package mergeview

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/markers"
)

// TestLoadCanonicalDocumentPrefersOnDiskOverMergeFile reproduces the
// neo-script rebase bug: `git merge-file --diff3` and git's real merge can
// draw conflict boundaries differently. The on-disk file is what the user
// actually sees, so canonical must mirror it.
func TestLoadCanonicalDocumentPrefersOnDiskOverMergeFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	basePath := filepath.Join(tmpDir, "base.txt")
	localPath := filepath.Join(tmpDir, "local.txt")
	remotePath := filepath.Join(tmpDir, "remote.txt")
	mergedPath := filepath.Join(tmpDir, "merged.txt")

	// The stage files and the disk-merged file disagree on what the "ours"
	// side looks like. Whatever git merge-file would produce from the stages
	// should NOT clobber the disk bytes that the user is looking at.
	mustWrite(t, basePath, "head\nbase\ntail\n")
	mustWrite(t, localPath, "head\nlocal\ntail\n")
	mustWrite(t, remotePath, "head\nremote\ntail\n")
	mustWrite(t, mergedPath, "head\n<<<<<<< ours-label\nlocal AS WRITTEN BY GIT\n=======\nremote AS WRITTEN BY GIT\n>>>>>>> theirs-label\ntail\n")

	doc, err := LoadCanonicalDocument(ctx, cli.Options{
		BasePath:   basePath,
		LocalPath:  localPath,
		RemotePath: remotePath,
		MergedPath: mergedPath,
	})
	if err != nil {
		t.Fatalf("LoadCanonicalDocument error = %v", err)
	}
	if len(doc.Conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(doc.Conflicts))
	}

	seg := doc.Segments[doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
	if string(seg.Ours) != "local AS WRITTEN BY GIT\n" {
		t.Fatalf("seg.Ours = %q, want disk bytes", string(seg.Ours))
	}
	if string(seg.Theirs) != "remote AS WRITTEN BY GIT\n" {
		t.Fatalf("seg.Theirs = %q, want disk bytes", string(seg.Theirs))
	}
	if seg.OursLabel != "ours-label" || seg.TheirsLabel != "theirs-label" {
		t.Fatalf("labels = ours=%q theirs=%q, want disk labels", seg.OursLabel, seg.TheirsLabel)
	}
	// Backfill should at minimum supply a BaseLabel so downstream base-completeness
	// validation passes, even when the bytes cannot be mapped.
	if seg.BaseLabel == "" {
		t.Fatalf("BaseLabel empty; expected positional backfill from diff3 view")
	}
}

// TestLoadCanonicalDocumentBackfillsBaseWhenIdentityMatches covers the common
// case where git's merge and merge-file agree on conflict boundaries (so
// Ours/Theirs match exactly), but the disk file is rendered without `|||||||`
// base sections. Base bytes and label should be borrowed from the diff3 view.
func TestLoadCanonicalDocumentBackfillsBaseWhenIdentityMatches(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	basePath := filepath.Join(tmpDir, "base.txt")
	localPath := filepath.Join(tmpDir, "local.txt")
	remotePath := filepath.Join(tmpDir, "remote.txt")
	mergedPath := filepath.Join(tmpDir, "merged.txt")

	mustWrite(t, basePath, "head\nbase\ntail\n")
	mustWrite(t, localPath, "head\nlocal\ntail\n")
	mustWrite(t, remotePath, "head\nremote\ntail\n")
	mustWrite(t, mergedPath, "head\n<<<<<<< HEAD\nlocal\n=======\nremote\n>>>>>>> branch\ntail\n")

	doc, err := LoadCanonicalDocument(ctx, cli.Options{
		BasePath:   basePath,
		LocalPath:  localPath,
		RemotePath: remotePath,
		MergedPath: mergedPath,
	})
	if err != nil {
		t.Fatalf("LoadCanonicalDocument error = %v", err)
	}

	seg := doc.Segments[doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
	if string(seg.Base) != "base\n" {
		t.Fatalf("seg.Base = %q, want backfilled 'base\\n'", string(seg.Base))
	}
	if seg.BaseLabel == "" {
		t.Fatalf("BaseLabel empty; want backfilled from diff3 view")
	}
}

// TestLoadCanonicalDocumentFallsBackToDiff3WhenDiskMissing guards the
// fallback path: if the working copy was moved or corrupted, reconstruction
// from stage files should still succeed.
func TestLoadCanonicalDocumentFallsBackToDiff3WhenDiskMissing(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	basePath := filepath.Join(tmpDir, "base.txt")
	localPath := filepath.Join(tmpDir, "local.txt")
	remotePath := filepath.Join(tmpDir, "remote.txt")

	mustWrite(t, basePath, "head\nbase\ntail\n")
	mustWrite(t, localPath, "head\nlocal\ntail\n")
	mustWrite(t, remotePath, "head\nremote\ntail\n")

	doc, err := LoadCanonicalDocument(ctx, cli.Options{
		BasePath:   basePath,
		LocalPath:  localPath,
		RemotePath: remotePath,
		MergedPath: filepath.Join(tmpDir, "does-not-exist.txt"),
	})
	if err != nil {
		t.Fatalf("LoadCanonicalDocument error = %v", err)
	}
	if len(doc.Conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(doc.Conflicts))
	}
	seg := doc.Segments[doc.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
	if string(seg.Ours) != "local\n" || string(seg.Base) != "base\n" || string(seg.Theirs) != "remote\n" {
		t.Fatalf("fallback seg = %+v", seg)
	}
}

// TestBackfillBaseSectionsMatchesMonotonically guards against a subtle bug:
// when two conflicts have byte-identical Ours+Theirs (common in repeated code
// patterns), a first-match lookup would hand the same diff3 source segment to
// both target conflicts, so the second would inherit the wrong Base.
// Monotonic left-to-right matching prevents that.
func TestBackfillBaseSectionsMatchesMonotonically(t *testing.T) {
	mkConflict := func(ours, base, theirs, baseLabel string) markers.ConflictSegment {
		return markers.ConflictSegment{
			Ours:      []byte(ours),
			Base:      []byte(base),
			Theirs:    []byte(theirs),
			BaseLabel: baseLabel,
		}
	}

	target := markers.Document{
		Segments: []markers.Segment{
			markers.TextSegment{Bytes: []byte("a\n")},
			mkConflict("X\n", "", "Y\n", ""),
			markers.TextSegment{Bytes: []byte("b\n")},
			mkConflict("X\n", "", "Y\n", ""),
			markers.TextSegment{Bytes: []byte("c\n")},
		},
		Conflicts: []markers.ConflictRef{{SegmentIndex: 1}, {SegmentIndex: 3}},
	}
	source := markers.Document{
		Segments: []markers.Segment{
			markers.TextSegment{Bytes: []byte("a\n")},
			mkConflict("X\n", "BASE-1\n", "Y\n", "base-label"),
			markers.TextSegment{Bytes: []byte("b\n")},
			mkConflict("X\n", "BASE-2\n", "Y\n", "base-label"),
			markers.TextSegment{Bytes: []byte("c\n")},
		},
		Conflicts: []markers.ConflictRef{{SegmentIndex: 1}, {SegmentIndex: 3}},
	}

	backfillBaseSections(&target, source)

	got0 := target.Segments[target.Conflicts[0].SegmentIndex].(markers.ConflictSegment)
	got1 := target.Segments[target.Conflicts[1].SegmentIndex].(markers.ConflictSegment)
	if string(got0.Base) != "BASE-1\n" {
		t.Fatalf("conflict 0 Base = %q, want %q", string(got0.Base), "BASE-1\n")
	}
	if string(got1.Base) != "BASE-2\n" {
		t.Fatalf("conflict 1 Base = %q, want %q (second identical conflict must take the next source, not reuse the first)", string(got1.Base), "BASE-2\n")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
