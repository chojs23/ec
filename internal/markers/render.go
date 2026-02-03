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
