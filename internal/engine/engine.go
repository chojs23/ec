package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chojs23/easy-conflict/internal/cli"
	"github.com/chojs23/easy-conflict/internal/gitmerge"
	"github.com/chojs23/easy-conflict/internal/markers"
)

func CheckResolvedFile(mergedPath string) (bool, error) {
	data, err := os.ReadFile(mergedPath)
	if err != nil {
		return false, fmt.Errorf("read merged: %w", err)
	}

	doc, err := markers.Parse(data)
	if err != nil {
		// Treat malformed markers as an error to avoid false success.
		return false, err
	}

	return len(doc.Conflicts) == 0, nil
}

func ApplyAllAndWrite(ctx context.Context, opts cli.Options) error {
	if opts.ApplyAll == "" {
		return errors.New("internal: ApplyAllAndWrite called without apply mode")
	}

	mergedBytes, err := os.ReadFile(opts.MergedPath)
	if err != nil {
		return fmt.Errorf("read merged: %w", err)
	}
	mergedDoc, err := markers.Parse(mergedBytes)
	if err != nil {
		return err
	}
	if len(mergedDoc.Conflicts) == 0 {
		// Per plan: no conflicts detected → exit 0 without writing.
		return nil
	}

	mergeViewBytes, err := gitmerge.MergeFileDiff3(ctx, opts.LocalPath, opts.BasePath, opts.RemotePath)
	if err != nil {
		return err
	}
	viewDoc, err := markers.Parse(mergeViewBytes)
	if err != nil {
		return err
	}
	if len(viewDoc.Conflicts) == 0 {
		return fmt.Errorf("computed diff3 view has no conflicts but %s contains conflict markers", opts.MergedPath)
	}

	if err := ValidateBaseCompleteness(viewDoc); err != nil {
		return fmt.Errorf("base display validation failed: %w", err)
	}

	for _, ref := range viewDoc.Conflicts {
		seg, ok := viewDoc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
		if !ok {
			return fmt.Errorf("internal: conflict index %d is not a ConflictSegment", ref.SegmentIndex)
		}
		seg.Resolution = markers.Resolution(opts.ApplyAll)
		viewDoc.Segments[ref.SegmentIndex] = seg
	}

	resolved, err := markers.RenderResolved(viewDoc)
	if err != nil {
		return err
	}

	if bytes.Equal(resolved, mergedBytes) {
		// Already matches (unlikely), but keep it safe: don't write.
		return nil
	}

	if opts.Backup {
		bak := opts.MergedPath + ".easy-conflict.bak"
		if err := os.WriteFile(bak, mergedBytes, 0o644); err != nil {
			return fmt.Errorf("write backup %s: %w", filepath.Base(bak), err)
		}
	}

	if err := os.WriteFile(opts.MergedPath, resolved, 0o644); err != nil {
		return fmt.Errorf("write merged: %w", err)
	}

	// Verify no conflict markers remain.
	postDoc, err := markers.Parse(resolved)
	if err != nil {
		return fmt.Errorf("post-parse merged: %w", err)
	}
	if len(postDoc.Conflicts) != 0 {
		return errors.New("resolution output still contains conflict markers")
	}

	return nil
}
