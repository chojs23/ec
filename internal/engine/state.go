package engine

import (
	"fmt"

	"github.com/chojs23/ec/internal/markers"
	"github.com/chojs23/ec/internal/mergeview"
)

// State manages resolution state for a conflict document.
type State struct {
	session *mergeview.Session
}

func NewStateFromSession(session *mergeview.Session) (*State, error) {
	return &State{
		session: session.Clone(),
	}, nil
}

// ApplyResolution sets the resolution for a conflict at the given index.
// conflictIndex is an index into doc.Conflicts (NOT doc.Segments).
// Returns error if index is out of bounds or resolution is invalid.
func (s *State) ApplyResolution(conflictIndex int, resolution markers.Resolution) error {
	if conflictIndex < 0 || conflictIndex >= len(s.session.Conflicts) {
		return fmt.Errorf("conflict index %d out of bounds [0, %d)", conflictIndex, len(s.session.Conflicts))
	}
	return s.session.ApplyResolution(conflictIndex, resolution)
}

// ApplyAll sets the resolution for all conflicts.
func (s *State) ApplyAll(resolution markers.Resolution) error {
	return s.session.ApplyAll(resolution)
}

func (s *State) ReplaceSession(session *mergeview.Session) {
	if mergeview.SessionsEqual(s.session, session) {
		return
	}
	s.session = session.Clone()
}

// Preview generates the resolved output by concatenating segments with resolutions applied.
// Uses markers.RenderResolved to produce the final bytes.
// Returns error if any conflict is unresolved.
func (s *State) Preview() ([]byte, error) {
	return s.session.Preview()
}

func (s *State) Session() *mergeview.Session {
	return s.session.Clone()
}
