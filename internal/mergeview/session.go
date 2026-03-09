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
	var out bytes.Buffer
	for _, seg := range s.Segments {
		switch v := seg.(type) {
		case TextSegment:
			out.Write(v.Bytes)
		case ConflictBlock:
			switch v.Resolution {
			case markers.ResolutionOurs:
				out.Write(v.Ours)
			case markers.ResolutionTheirs:
				out.Write(v.Theirs)
			case markers.ResolutionBoth:
				out.Write(v.Ours)
				out.Write(v.Theirs)
			case markers.ResolutionNone:
			default:
				return nil, fmt.Errorf("%w: conflict without resolution", markers.ErrUnresolved)
			}
		default:
			return nil, fmt.Errorf("unknown session segment type %T", seg)
		}
	}
	return out.Bytes(), nil
}

func validateResolution(resolution markers.Resolution) error {
	switch resolution {
	case markers.ResolutionOurs, markers.ResolutionTheirs, markers.ResolutionBoth, markers.ResolutionNone:
		return nil
	default:
		return fmt.Errorf("invalid resolution: %q", resolution)
	}
}
