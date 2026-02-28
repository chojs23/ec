package engine

import (
	"fmt"

	"github.com/chojs23/ec/internal/markers"
)

// State manages resolution state for a conflict document.
type State struct {
	doc markers.Document
}

// NewState creates a new State from a parsed document.
func NewState(doc markers.Document) (*State, error) {
	return &State{
		doc: doc,
	}, nil
}

// ApplyResolution sets the resolution for a conflict at the given index.
// conflictIndex is an index into doc.Conflicts (NOT doc.Segments).
// Returns error if index is out of bounds or resolution is invalid.
func (s *State) ApplyResolution(conflictIndex int, resolution markers.Resolution) error {
	if conflictIndex < 0 || conflictIndex >= len(s.doc.Conflicts) {
		return fmt.Errorf("conflict index %d out of bounds [0, %d)", conflictIndex, len(s.doc.Conflicts))
	}

	// Validate resolution
	switch resolution {
	case markers.ResolutionOurs, markers.ResolutionTheirs, markers.ResolutionBoth, markers.ResolutionNone:
		// Valid
	default:
		return fmt.Errorf("invalid resolution: %q", resolution)
	}

	ref := s.doc.Conflicts[conflictIndex]
	seg, ok := s.doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
	if !ok {
		return fmt.Errorf("internal: conflict index %d points to non-ConflictSegment", conflictIndex)
	}
	if seg.Resolution == resolution {
		return nil
	}

	seg.Resolution = resolution
	s.doc.Segments[ref.SegmentIndex] = seg

	return nil
}

// ApplyAll sets the resolution for all conflicts.
func (s *State) ApplyAll(resolution markers.Resolution) error {
	// Validate resolution
	switch resolution {
	case markers.ResolutionOurs, markers.ResolutionTheirs, markers.ResolutionBoth, markers.ResolutionNone:
		// Valid
	default:
		return fmt.Errorf("invalid resolution: %q", resolution)
	}

	hasChange := false
	for _, ref := range s.doc.Conflicts {
		seg, ok := s.doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
		if !ok {
			return fmt.Errorf("internal: conflict points to non-ConflictSegment")
		}
		if seg.Resolution != resolution {
			hasChange = true
			break
		}
	}
	if !hasChange {
		return nil
	}

	for _, ref := range s.doc.Conflicts {
		seg, ok := s.doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
		if !ok {
			return fmt.Errorf("internal: conflict points to non-ConflictSegment")
		}
		seg.Resolution = resolution
		s.doc.Segments[ref.SegmentIndex] = seg
	}

	return nil
}

// ReplaceDocument replaces the current document.
func (s *State) ReplaceDocument(doc markers.Document) {
	if markers.DocumentsEqual(s.doc, doc) {
		return
	}
	s.doc = markers.CloneDocument(doc)
}

// Preview generates the resolved output by concatenating segments with resolutions applied.
// Uses markers.RenderResolved to produce the final bytes.
// Returns error if any conflict is unresolved.
func (s *State) Preview() ([]byte, error) {
	return markers.RenderResolved(s.doc)
}

func (s *State) Document() markers.Document {
	return markers.CloneDocument(s.doc)
}
