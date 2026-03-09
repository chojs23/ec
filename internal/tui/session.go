package tui

import (
	"context"
	"fmt"
	"os"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/gitmerge"
	"github.com/chojs23/ec/internal/markers"
)

type resolverDocumentState struct {
	doc              markers.Document
	manualResolved   map[int][]byte
	mergedLabels     []conflictLabels
	mergedLabelKnown []bool
}

func loadResolverDocumentState(ctx context.Context, opts cli.Options) (resolverDocumentState, error) {
	canonicalDoc, err := loadCanonicalDiff3Document(ctx, opts)
	if err != nil {
		return resolverDocumentState{}, err
	}

	state := resolverDocumentState{
		doc:              canonicalDoc,
		manualResolved:   map[int][]byte{},
		mergedLabels:     make([]conflictLabels, len(canonicalDoc.Conflicts)),
		mergedLabelKnown: make([]bool, len(canonicalDoc.Conflicts)),
	}

	mergedBytes, err := os.ReadFile(opts.MergedPath)
	if err != nil {
		return state, nil
	}

	if mergedDoc, ok := tryBuildMarkerDrivenDocument(canonicalDoc, mergedBytes); ok {
		state.doc = mergedDoc
		return state, nil
	}

	updated, manual, labels, known, err := applyMergedResolutions(canonicalDoc, mergedBytes)
	if err != nil {
		return resolverDocumentState{}, fmt.Errorf("apply merged resolutions: %w", err)
	}

	state.doc = updated
	state.manualResolved = manual
	state.mergedLabels = labels
	state.mergedLabelKnown = known
	return state, nil
}

func loadCanonicalDiff3Document(ctx context.Context, opts cli.Options) (markers.Document, error) {
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

func tryBuildMarkerDrivenDocument(canonicalDoc markers.Document, mergedBytes []byte) (markers.Document, bool) {
	mergedDoc, err := markers.Parse(mergedBytes)
	if err != nil {
		return markers.Document{}, false
	}
	if len(mergedDoc.Conflicts) == 0 {
		return markers.Document{}, false
	}
	if len(mergedDoc.Conflicts) != len(canonicalDoc.Conflicts) {
		return markers.Document{}, false
	}

	enriched, err := enrichMergedDocumentWithBase(canonicalDoc, mergedDoc)
	if err != nil {
		return markers.Document{}, false
	}

	return enriched, true
}

func enrichMergedDocumentWithBase(canonicalDoc markers.Document, mergedDoc markers.Document) (markers.Document, error) {
	if len(mergedDoc.Conflicts) != len(canonicalDoc.Conflicts) {
		return markers.Document{}, fmt.Errorf("conflict count mismatch: merged=%d canonical=%d", len(mergedDoc.Conflicts), len(canonicalDoc.Conflicts))
	}

	out := markers.Document{
		Segments:  make([]markers.Segment, 0, len(mergedDoc.Segments)),
		Conflicts: make([]markers.ConflictRef, 0, len(mergedDoc.Conflicts)),
	}

	conflictIndex := 0
	for _, seg := range mergedDoc.Segments {
		switch s := seg.(type) {
		case markers.TextSegment:
			out.Segments = append(out.Segments, markers.TextSegment{Bytes: append([]byte(nil), s.Bytes...)})

		case markers.ConflictSegment:
			if conflictIndex >= len(canonicalDoc.Conflicts) {
				return markers.Document{}, fmt.Errorf("merged conflict index %d out of bounds", conflictIndex)
			}

			ref := canonicalDoc.Conflicts[conflictIndex]
			canonicalSeg, ok := canonicalDoc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
			if !ok {
				return markers.Document{}, fmt.Errorf("canonical conflict %d is not a conflict segment", conflictIndex)
			}

			enriched := canonicalSeg
			enriched.Ours = append([]byte(nil), s.Ours...)
			enriched.Theirs = append([]byte(nil), s.Theirs...)
			if len(s.Base) > 0 || s.BaseLabel != "" {
				enriched.Base = append([]byte(nil), s.Base...)
				enriched.BaseLabel = s.BaseLabel
			}
			if s.OursLabel != "" {
				enriched.OursLabel = s.OursLabel
			}
			if s.TheirsLabel != "" {
				enriched.TheirsLabel = s.TheirsLabel
			}
			enriched.Resolution = markers.ResolutionUnset

			segIndex := len(out.Segments)
			out.Segments = append(out.Segments, enriched)
			out.Conflicts = append(out.Conflicts, markers.ConflictRef{SegmentIndex: segIndex})
			conflictIndex++

		default:
			return markers.Document{}, fmt.Errorf("unknown merged segment type %T", seg)
		}
	}

	if conflictIndex != len(canonicalDoc.Conflicts) {
		return markers.Document{}, fmt.Errorf("merged conflict count mismatch after enrichment: got %d want %d", conflictIndex, len(canonicalDoc.Conflicts))
	}

	return out, nil
}
