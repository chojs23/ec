package mergeview

import (
	"bytes"
	"fmt"

	"github.com/chojs23/ec/internal/markers"
)

type Segment interface{ isSessionSegment() }

type TextSegment struct {
	Bytes []byte
}

func (TextSegment) isSessionSegment() {}

type ConflictBlock struct {
	Ours   []byte
	Base   []byte
	Theirs []byte

	Labels     Labels
	Resolution markers.Resolution
}

func (ConflictBlock) isSessionSegment() {}

type Labels struct {
	OursLabel   string
	BaseLabel   string
	TheirsLabel string
}

type ConflictRef struct {
	SegmentIndex int
}

type Session struct {
	Segments  []Segment
	Conflicts []ConflictRef
}

func SessionFromDocument(doc markers.Document) (*Session, error) {
	session := &Session{
		Segments:  make([]Segment, 0, len(doc.Segments)),
		Conflicts: make([]ConflictRef, 0, len(doc.Conflicts)),
	}

	for _, seg := range doc.Segments {
		switch s := seg.(type) {
		case markers.TextSegment:
			session.Segments = append(session.Segments, TextSegment{Bytes: append([]byte(nil), s.Bytes...)})
		case markers.ConflictSegment:
			idx := len(session.Segments)
			session.Segments = append(session.Segments, ConflictBlock{
				Ours:   append([]byte(nil), s.Ours...),
				Base:   append([]byte(nil), s.Base...),
				Theirs: append([]byte(nil), s.Theirs...),
				Labels: Labels{
					OursLabel:   s.OursLabel,
					BaseLabel:   s.BaseLabel,
					TheirsLabel: s.TheirsLabel,
				},
				Resolution: s.Resolution,
			})
			session.Conflicts = append(session.Conflicts, ConflictRef{SegmentIndex: idx})
		default:
			return nil, fmt.Errorf("unknown marker segment type %T", seg)
		}
	}

	if len(session.Conflicts) != len(doc.Conflicts) {
		return nil, fmt.Errorf("conflict count mismatch: session=%d doc=%d", len(session.Conflicts), len(doc.Conflicts))
	}

	return session, nil
}

func (s *Session) Clone() *Session {
	if s == nil {
		return nil
	}

	clone := &Session{
		Segments:  make([]Segment, 0, len(s.Segments)),
		Conflicts: append([]ConflictRef(nil), s.Conflicts...),
	}

	for _, seg := range s.Segments {
		switch v := seg.(type) {
		case TextSegment:
			clone.Segments = append(clone.Segments, TextSegment{Bytes: append([]byte(nil), v.Bytes...)})
		case ConflictBlock:
			clone.Segments = append(clone.Segments, ConflictBlock{
				Ours:       append([]byte(nil), v.Ours...),
				Base:       append([]byte(nil), v.Base...),
				Theirs:     append([]byte(nil), v.Theirs...),
				Labels:     v.Labels,
				Resolution: v.Resolution,
			})
		}
	}

	return clone
}

func SessionsEqual(left, right *Session) bool {
	if left == nil || right == nil {
		return left == right
	}
	if len(left.Segments) != len(right.Segments) || len(left.Conflicts) != len(right.Conflicts) {
		return false
	}
	for i := range left.Conflicts {
		if left.Conflicts[i] != right.Conflicts[i] {
			return false
		}
	}
	for i := range left.Segments {
		switch l := left.Segments[i].(type) {
		case TextSegment:
			r, ok := right.Segments[i].(TextSegment)
			if !ok || !bytes.Equal(l.Bytes, r.Bytes) {
				return false
			}
		case ConflictBlock:
			r, ok := right.Segments[i].(ConflictBlock)
			if !ok {
				return false
			}
			if !bytes.Equal(l.Ours, r.Ours) || !bytes.Equal(l.Base, r.Base) || !bytes.Equal(l.Theirs, r.Theirs) {
				return false
			}
			if l.Labels != r.Labels || l.Resolution != r.Resolution {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (s *Session) Document() markers.Document {
	if s == nil {
		return markers.Document{}
	}

	doc := markers.Document{
		Segments:  make([]markers.Segment, 0, len(s.Segments)),
		Conflicts: make([]markers.ConflictRef, 0, len(s.Conflicts)),
	}

	for _, seg := range s.Segments {
		switch v := seg.(type) {
		case TextSegment:
			doc.Segments = append(doc.Segments, markers.TextSegment{Bytes: append([]byte(nil), v.Bytes...)})
		case ConflictBlock:
			idx := len(doc.Segments)
			doc.Segments = append(doc.Segments, markers.ConflictSegment{
				Ours:        append([]byte(nil), v.Ours...),
				Base:        append([]byte(nil), v.Base...),
				Theirs:      append([]byte(nil), v.Theirs...),
				OursLabel:   v.Labels.OursLabel,
				BaseLabel:   v.Labels.BaseLabel,
				TheirsLabel: v.Labels.TheirsLabel,
				Resolution:  v.Resolution,
			})
			doc.Conflicts = append(doc.Conflicts, markers.ConflictRef{SegmentIndex: idx})
		}
	}

	return doc
}

func (s *Session) ApplyResolution(conflictIndex int, resolution markers.Resolution) error {
	if err := validateResolution(resolution); err != nil {
		return err
	}
	if conflictIndex < 0 || conflictIndex >= len(s.Conflicts) {
		return fmt.Errorf("conflict index %d out of bounds [0, %d)", conflictIndex, len(s.Conflicts))
	}

	ref := s.Conflicts[conflictIndex]
	block, ok := s.Segments[ref.SegmentIndex].(ConflictBlock)
	if !ok {
		return fmt.Errorf("internal: conflict index %d points to non-conflict block", conflictIndex)
	}
	block.Resolution = resolution
	s.Segments[ref.SegmentIndex] = block
	return nil
}

func (s *Session) ApplyAll(resolution markers.Resolution) error {
	if err := validateResolution(resolution); err != nil {
		return err
	}
	for idx := range s.Conflicts {
		if err := s.ApplyResolution(idx, resolution); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) Preview() ([]byte, error) {
	return markers.RenderResolved(s.Document())
}

func validateResolution(resolution markers.Resolution) error {
	switch resolution {
	case markers.ResolutionOurs, markers.ResolutionTheirs, markers.ResolutionBoth, markers.ResolutionNone:
		return nil
	default:
		return fmt.Errorf("invalid resolution: %q", resolution)
	}
}
