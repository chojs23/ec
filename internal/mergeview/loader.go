package mergeview

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/gitmerge"
	"github.com/chojs23/ec/internal/markers"
)

// LoadCanonicalDocument builds the canonical conflict document for interactive
// resolution.
//
// It prefers the on-disk merged file (what git actually wrote). Git's real
// merge algorithm can draw conflict boundaries differently from
// `git merge-file --diff3`, so reconstructing from stage files would diverge
// from what the user sees and cause valid conflicts to be misclassified as
// manually-resolved. Base sections are backfilled from the 3-way view when
// Ours+Theirs match byte-for-byte; BaseLabel is backfilled positionally so
// base-completeness validation stays satisfied.
//
// Conflicts the user has already resolved inline (by deleting their markers)
// are not surfaced as conflicts — they appear as resolved text in the
// surrounding TextSegments. This matches the model that `git status` uses for
// the file as a whole: an unmerged file stays unmerged until every conflict
// region is resolved, and each resolution simply disappears from view.
//
// Detecting inline pre-resolutions in a way that could label them "manual
// resolved" in the UI was attempted but proved unreliable: when git and
// git-merge-file draw boundaries differently, even the text outside conflicts
// can differ between the two documents, so there is no stable anchor to align
// them against.
//
// Falls back to the pure 3-way view when the merged file is unavailable or
// unparseable.
func LoadCanonicalDocument(ctx context.Context, opts cli.Options) (markers.Document, error) {
	diskDoc, diskValid := readDiskDocument(opts.MergedPath)

	diff3Doc, diff3Err := LoadDiff3Document(ctx, opts)
	if diff3Err != nil && !diskValid {
		return markers.Document{}, diff3Err
	}
	if !diskValid {
		return diff3Doc, nil
	}
	if diff3Err == nil {
		backfillBaseSections(&diskDoc, diff3Doc)
	}
	return diskDoc, nil
}

// LoadDiff3Document runs git's canonical three-way merge on the explicit stage
// files and returns the parsed document.
//
// This ignores the on-disk merged file, which is what non-interactive flows
// (such as `--apply-all`) want: the resolution bytes come from the stage files
// regardless of what the working copy currently contains.
func LoadDiff3Document(ctx context.Context, opts cli.Options) (markers.Document, error) {
	diff3Bytes, err := gitmerge.MergeFileDiff3(ctx, opts.LocalPath, opts.BasePath, opts.RemotePath)
	if err != nil {
		return markers.Document{}, fmt.Errorf("generate diff3 view: %w", err)
	}
	doc, err := markers.Parse(diff3Bytes)
	if err != nil {
		return markers.Document{}, fmt.Errorf("parse diff3 view: %w", err)
	}
	return doc, nil
}

func readDiskDocument(mergedPath string) (markers.Document, bool) {
	if mergedPath == "" {
		return markers.Document{}, false
	}
	data, err := os.ReadFile(mergedPath)
	if err != nil {
		return markers.Document{}, false
	}
	doc, err := markers.Parse(data)
	if err != nil || len(doc.Conflicts) == 0 {
		return markers.Document{}, false
	}
	return doc, true
}

// backfillBaseSections enriches target with Base info from source.
//
// Conflicts are matched monotonically from left to right: for each target
// conflict, the first source conflict at or after the last matched position
// that shares the same Ours+Theirs bytes wins. This prevents a single source
// conflict from being harvested twice when repeated content produces multiple
// byte-identical hunks (common in code with repeated patterns).
//
// For matched pairs, both Base bytes and BaseLabel are copied. For target
// conflicts that find no identity match, BaseLabel is still copied positionally
// from the same-index source conflict so base-completeness validation passes,
// but Base bytes stay empty — we refuse to invent 3-way base content when we
// can't prove it corresponds.
func backfillBaseSections(target *markers.Document, source markers.Document) {
	sourceSegs := make([]markers.ConflictSegment, 0, len(source.Conflicts))
	for _, ref := range source.Conflicts {
		if seg, ok := source.Segments[ref.SegmentIndex].(markers.ConflictSegment); ok {
			sourceSegs = append(sourceSegs, seg)
		}
	}

	cursor := 0
	for i, ref := range target.Conflicts {
		tseg, ok := target.Segments[ref.SegmentIndex].(markers.ConflictSegment)
		if !ok {
			continue
		}

		matched := -1
		for si := cursor; si < len(sourceSegs); si++ {
			if bytes.Equal(tseg.Ours, sourceSegs[si].Ours) && bytes.Equal(tseg.Theirs, sourceSegs[si].Theirs) {
				matched = si
				break
			}
		}

		if matched >= 0 {
			if len(tseg.Base) == 0 {
				tseg.Base = append([]byte(nil), sourceSegs[matched].Base...)
			}
			if tseg.BaseLabel == "" {
				tseg.BaseLabel = sourceSegs[matched].BaseLabel
			}
			cursor = matched + 1
		} else if i < len(sourceSegs) && tseg.BaseLabel == "" {
			tseg.BaseLabel = sourceSegs[i].BaseLabel
		}

		target.Segments[ref.SegmentIndex] = tseg
	}
}
