package engine

import (
	"fmt"

	"github.com/chojs23/ec/internal/markers"
)

// State manages resolution state for a conflict document with undo support.
type State struct {
	doc         markers.Document
	undoStack   []markers.Document
	redoStack   []markers.Document
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
		redoStack:   make([]markers.Document, 0, maxUndoSize),
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

	// Save current state to undo stack before modifying, and invalidate redo history.
	s.beginMutation()

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

	// Save current state to undo stack before modifying, and invalidate redo history.
	s.beginMutation()

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

	// Save current state to redo stack before restoring previous state.
	s.pushWithLimit(&s.redoStack, s.doc)

	// Pop the last state
	lastIdx := len(s.undoStack) - 1
	s.doc = s.undoStack[lastIdx]
	s.undoStack = s.undoStack[:lastIdx]

	return nil
}

// Redo reapplies a previously undone state.
// Returns error if no redo history is available.
func (s *State) Redo() error {
	if len(s.redoStack) == 0 {
		return fmt.Errorf("no redo history available")
	}

	// Save current state to undo stack before restoring redone state.
	s.pushWithLimit(&s.undoStack, s.doc)

	lastIdx := len(s.redoStack) - 1
	s.doc = s.redoStack[lastIdx]
	s.redoStack = s.redoStack[:lastIdx]

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

// RedoDepth returns the current number of redo operations available.
func (s *State) RedoDepth() int {
	return len(s.redoStack)
}

// beginMutation saves the current state to undo and clears redo history.
func (s *State) beginMutation() {
	s.pushWithLimit(&s.undoStack, s.doc)
	s.redoStack = s.redoStack[:0]
}

// pushWithLimit saves a document snapshot into the stack and enforces max size.
func (s *State) pushWithLimit(stack *[]markers.Document, doc markers.Document) {
	*stack = append(*stack, cloneDocument(doc))
	if len(*stack) > s.maxUndoSize {
		*stack = (*stack)[1:]
	}
}

func cloneDocument(doc markers.Document) markers.Document {
	// Deep copy the document to preserve state
	docCopy := markers.Document{
		Segments:  make([]markers.Segment, len(doc.Segments)),
		Conflicts: make([]markers.ConflictRef, len(doc.Conflicts)),
	}

	for i, seg := range doc.Segments {
		switch v := seg.(type) {
		case markers.TextSegment:
			// TextSegment.Bytes is immutable (we never modify it), so shallow copy is safe
			docCopy.Segments[i] = v
		case markers.ConflictSegment:
			// ConflictSegment fields are immutable byte slices and Resolution enum, shallow copy is safe
			docCopy.Segments[i] = v
		}
	}

	copy(docCopy.Conflicts, doc.Conflicts)
	return docCopy
}
