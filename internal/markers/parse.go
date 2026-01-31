package markers

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

var ErrMalformedConflict = errors.New("malformed conflict markers")

var (
	markStart = []byte("<<<<<<<")
	markBase  = []byte("|||||||")
	markMid   = []byte("=======")
	markEnd   = []byte(">>>>>>>")
)

// Parse splits a file into text segments and conflict segments.
//
// It is strict: if it encounters a start marker, it requires a full, valid
// marker structure (optionally including a diff3 base section).
func Parse(data []byte) (Document, error) {
	var doc Document

	// Normalize by working line-by-line (keeping line endings).
	lines := splitLinesKeepEOL(data)

	appendText := func(buf *bytes.Buffer) {
		if buf.Len() == 0 {
			return
		}
		doc.Segments = append(doc.Segments, TextSegment{Bytes: append([]byte(nil), buf.Bytes()...)})
		buf.Reset()
	}

	var textBuf bytes.Buffer
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if hasLinePrefix(line, markStart) {
			appendText(&textBuf)
			oursLabel := parseLabel(line, markStart)

			// Collect ours until base/mid.
			i++
			var ours bytes.Buffer
			for ; i < len(lines); i++ {
				if hasLinePrefix(lines[i], markBase) || hasLinePrefix(lines[i], markMid) {
					break
				}
				ours.Write(lines[i])
			}
			if i >= len(lines) {
				return Document{}, fmt.Errorf("%w: missing separator", ErrMalformedConflict)
			}

			// Optional base section.
			var base bytes.Buffer
			baseLabel := ""
			if hasLinePrefix(lines[i], markBase) {
				baseLabel = parseLabel(lines[i], markBase)
				i++
				for ; i < len(lines); i++ {
					if hasLinePrefix(lines[i], markMid) {
						break
					}
					base.Write(lines[i])
				}
				if i >= len(lines) {
					return Document{}, fmt.Errorf("%w: missing ======= after base", ErrMalformedConflict)
				}
			}

			// Must have mid.
			if !hasLinePrefix(lines[i], markMid) {
				return Document{}, fmt.Errorf("%w: expected =======", ErrMalformedConflict)
			}

			// Collect theirs until end.
			i++
			var theirs bytes.Buffer
			for ; i < len(lines); i++ {
				if hasLinePrefix(lines[i], markEnd) {
					break
				}
				theirs.Write(lines[i])
			}
			if i >= len(lines) {
				return Document{}, fmt.Errorf("%w: missing end marker", ErrMalformedConflict)
			}
			theirsLabel := parseLabel(lines[i], markEnd)

			segIndex := len(doc.Segments)
			doc.Segments = append(doc.Segments, ConflictSegment{
				Ours:        ours.Bytes(),
				Base:        base.Bytes(),
				Theirs:      theirs.Bytes(),
				OursLabel:   oursLabel,
				BaseLabel:   baseLabel,
				TheirsLabel: theirsLabel,
				Resolution:  ResolutionUnset,
			})
			doc.Conflicts = append(doc.Conflicts, ConflictRef{SegmentIndex: segIndex})
			continue
		}

		textBuf.Write(line)
	}

	appendText(&textBuf)
	return doc, nil
}

func hasLinePrefix(line, prefix []byte) bool {
	// Markers appear at line start in Git output.
	return bytes.HasPrefix(line, prefix)
}

func splitLinesKeepEOL(b []byte) [][]byte {
	if len(b) == 0 {
		return nil
	}

	var out [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			out = append(out, b[start:i+1])
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}

func parseLabel(line []byte, prefix []byte) string {
	if !bytes.HasPrefix(line, prefix) {
		return ""
	}
	text := strings.TrimSpace(string(line[len(prefix):]))
	return text
}

// IsResolved returns true if the data contains no conflict markers.
//
// It checks for the presence of valid conflict marker sequences.
// False positives (lines starting with <<<<<<< but not followed by a valid
// conflict structure) are NOT considered conflicts.
func IsResolved(data []byte) bool {
	_, err := Parse(data)
	if err != nil {
		return false
	}
	doc, _ := Parse(data)
	return len(doc.Conflicts) == 0
}
