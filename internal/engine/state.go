package engine

import (
	"fmt"

	"github.com/chojs23/easy-conflict/internal/markers"
)

// State manages resolution state for a conflict document with undo support.
type State struct {
	doc         markers.Document
	undoStack   []markers.Document
	maxUndoSize int
}

// NewState creates a new State from a parsed document.
// maxUndoSize controls how many undo operations to retain (must be >= 1).
func NewState(doc markers.Document, maxUndoSize int) (*State, error) {
	if maxUndoSize < 1 {
		return nil, fmt.Errorf("maxUndoSize must be >= 1, got %d", maxUndoSize)
	}
	return &State{
		doc:         doc,
		undoStack:   make([]markers.Document, 0, maxUndoSize),
		maxUndoSize: maxUndoSize,
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

	// Save current state to undo stack before modifying
	s.pushUndo()

	// Apply the resolution
	ref := s.doc.Conflicts[conflictIndex]
	seg, ok := s.doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
	if !ok {
		return fmt.Errorf("internal: conflict index %d points to non-ConflictSegment", conflictIndex)
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

	// Save current state to undo stack before modifying
	s.pushUndo()

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

// Undo restores the previous state.
// Returns error if no undo history is available.
func (s *State) Undo() error {
	if len(s.undoStack) == 0 {
		return fmt.Errorf("no undo history available")
	}

	// Pop the last state
	lastIdx := len(s.undoStack) - 1
	s.doc = s.undoStack[lastIdx]
	s.undoStack = s.undoStack[:lastIdx]

	return nil
}

// Preview generates the resolved output by concatenating segments with resolutions applied.
// Uses markers.RenderResolved to produce the final bytes.
// Returns error if any conflict is unresolved.
func (s *State) Preview() ([]byte, error) {
	return markers.RenderResolved(s.doc)
}

// Document returns a copy of the current document state.
func (s *State) Document() markers.Document {
	return s.doc
}

// UndoDepth returns the current number of undo operations available.
func (s *State) UndoDepth() int {
	return len(s.undoStack)
}

// pushUndo saves the current document state to the undo stack.
// If the stack exceeds maxUndoSize, the oldest entry is removed.
func (s *State) pushUndo() {
	// Deep copy the document to preserve state
	docCopy := markers.Document{
		Segments:  make([]markers.Segment, len(s.doc.Segments)),
		Conflicts: make([]markers.ConflictRef, len(s.doc.Conflicts)),
	}

	for i, seg := range s.doc.Segments {
		switch v := seg.(type) {
		case markers.TextSegment:
			// TextSegment.Bytes is immutable (we never modify it), so shallow copy is safe
			docCopy.Segments[i] = v
		case markers.ConflictSegment:
			// ConflictSegment fields are immutable byte slices and Resolution enum, shallow copy is safe
			docCopy.Segments[i] = v
		}
	}

	copy(docCopy.Conflicts, s.doc.Conflicts)

	s.undoStack = append(s.undoStack, docCopy)

	// Trim if exceeds max size
	if len(s.undoStack) > s.maxUndoSize {
		s.undoStack = s.undoStack[1:]
	}
}
