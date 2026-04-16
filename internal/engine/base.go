package engine

import (
	"fmt"

	"github.com/chojs23/ec/internal/markers"
)

// ValidateBaseCompleteness checks that every conflict in the document has a base chunk.
// Returns error if any conflict is missing its base section.
func ValidateBaseCompleteness(doc markers.Document) error {
	for i, ref := range doc.Conflicts {
		seg, ok := doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
		if !ok {
			return fmt.Errorf("internal: conflict %d is not a ConflictSegment", i)
		}
		if len(seg.Base) == 0 && seg.BaseLabel == "" {
			return fmt.Errorf("conflict %d is missing base chunk (base completeness requires exact base for all conflicts)", i)
		}
	}
	return nil
}
