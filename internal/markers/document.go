package markers

import "bytes"

func CloneDocument(doc Document) Document {
	cloned := Document{
		Segments:  make([]Segment, len(doc.Segments)),
		Conflicts: make([]ConflictRef, len(doc.Conflicts)),
	}
	for i, seg := range doc.Segments {
		switch v := seg.(type) {
		case TextSegment:
			cloned.Segments[i] = v
		case ConflictSegment:
			cloned.Segments[i] = v
		}
	}
	copy(cloned.Conflicts, doc.Conflicts)
	return cloned
}

func DocumentsEqual(left, right Document) bool {
	if len(left.Conflicts) != len(right.Conflicts) || len(left.Segments) != len(right.Segments) {
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
		case ConflictSegment:
			r, ok := right.Segments[i].(ConflictSegment)
			if !ok {
				return false
			}
			if !bytes.Equal(l.Ours, r.Ours) || !bytes.Equal(l.Base, r.Base) || !bytes.Equal(l.Theirs, r.Theirs) {
				return false
			}
			if l.OursLabel != r.OursLabel || l.BaseLabel != r.BaseLabel || l.TheirsLabel != r.TheirsLabel {
				return false
			}
			if l.Resolution != r.Resolution {
				return false
			}
		default:
			return false
		}
	}
	return true
}
