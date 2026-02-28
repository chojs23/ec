package markers

import (
	"bytes"
	"errors"
	"fmt"
)

var ErrUnresolved = errors.New("unresolved")

func RenderResolved(doc Document) ([]byte, error) {
	var out bytes.Buffer

	for _, seg := range doc.Segments {
		switch s := seg.(type) {
		case TextSegment:
			out.Write(s.Bytes)
		case ConflictSegment:
			res := s.Resolution
			switch res {
			case ResolutionOurs:
				out.Write(s.Ours)
			case ResolutionTheirs:
				out.Write(s.Theirs)
			case ResolutionBoth:
				out.Write(s.Ours)
				out.Write(s.Theirs)
			case ResolutionNone:
				// Write nothing for this conflict.
			default:
				return nil, fmt.Errorf("%w: conflict without resolution", ErrUnresolved)
			}
		default:
			return nil, fmt.Errorf("unknown segment type %T", seg)
		}
	}

	return out.Bytes(), nil
}

func RenderWithUnresolved(doc Document) ([]byte, error) {
	var out bytes.Buffer

	for _, seg := range doc.Segments {
		switch s := seg.(type) {
		case TextSegment:
			out.Write(s.Bytes)
		case ConflictSegment:
			appendRenderedConflictSegment(&out, s, s.OursLabel, s.BaseLabel, s.TheirsLabel)
		default:
			return nil, fmt.Errorf("unknown segment type %T", seg)
		}
	}

	return out.Bytes(), nil
}

// AppendConflictSegment renders one conflict segment into out using the given labels.
// It returns true when the segment remains unresolved and conflict markers were emitted.
func AppendConflictSegment(out *bytes.Buffer, seg ConflictSegment, oursLabel, baseLabel, theirsLabel string) bool {
	return appendRenderedConflictSegment(out, seg, oursLabel, baseLabel, theirsLabel)
}

func appendRenderedConflictSegment(out *bytes.Buffer, seg ConflictSegment, oursLabel, baseLabel, theirsLabel string) bool {
	writeMarker := func(prefix []byte, label string) {
		out.Write(prefix)
		if label != "" {
			out.WriteByte(' ')
			out.WriteString(label)
		}
		out.WriteByte('\n')
	}

	switch seg.Resolution {
	case ResolutionOurs:
		out.Write(seg.Ours)
		return false
	case ResolutionTheirs:
		out.Write(seg.Theirs)
		return false
	case ResolutionBoth:
		out.Write(seg.Ours)
		out.Write(seg.Theirs)
		return false
	case ResolutionNone:
		return false
	default:
		writeMarker(markStart, oursLabel)
		out.Write(seg.Ours)
		if len(seg.Base) > 0 || baseLabel != "" {
			writeMarker(markBase, baseLabel)
			out.Write(seg.Base)
		}
		writeMarker(markMid, "")
		out.Write(seg.Theirs)
		writeMarker(markEnd, theirsLabel)
		return true
	}
}
