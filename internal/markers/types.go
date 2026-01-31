package markers

type Resolution string

const (
	ResolutionUnset  Resolution = ""
	ResolutionOurs   Resolution = "ours"
	ResolutionTheirs Resolution = "theirs"
	ResolutionBoth   Resolution = "both"
	ResolutionNone   Resolution = "none"
)

type Document struct {
	Segments  []Segment
	Conflicts []ConflictRef
}

type Segment interface{ isSegment() }

type TextSegment struct{ Bytes []byte }

func (TextSegment) isSegment() {}

type ConflictSegment struct {
	Ours   []byte
	Base   []byte // may be nil if not present
	Theirs []byte

	OursLabel   string
	BaseLabel   string
	TheirsLabel string

	// For future: labels (e.g., HEAD, branch name)
	Resolution Resolution
}

func (ConflictSegment) isSegment() {}

// ConflictRef points to a conflict segment inside Document.Segments.
//
// We keep an index list for convenient iteration and stable ordering.
type ConflictRef struct {
	SegmentIndex int
}
