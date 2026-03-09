package mergeview

import (
	"bytes"
	"fmt"

	"github.com/chojs23/ec/internal/markers"
)

var (
	markStart = []byte("<<<<<<<")
	markBase  = []byte("|||||||")
	markMid   = []byte("=======")
	markEnd   = []byte(">>>>>>>")
)

func ParseSession(data []byte) (*Session, error) {
	session := &Session{}
	lines := markers.SplitLinesKeepEOL(data)

	appendText := func(buf *bytes.Buffer) {
		if buf.Len() == 0 {
			return
		}
		session.Segments = append(session.Segments, TextSegment{Bytes: append([]byte(nil), buf.Bytes()...)})
		buf.Reset()
	}

	var textBuf bytes.Buffer
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if bytes.HasPrefix(line, markStart) {
			appendText(&textBuf)
			oursLabel := parseLabel(line, markStart)

			i++
			var ours bytes.Buffer
			for ; i < len(lines); i++ {
				if bytes.HasPrefix(lines[i], markBase) || bytes.HasPrefix(lines[i], markMid) {
					break
				}
				ours.Write(lines[i])
			}
			if i >= len(lines) {
				return nil, fmt.Errorf("%w: missing separator", markers.ErrMalformedConflict)
			}

			var base bytes.Buffer
			baseLabel := ""
			if bytes.HasPrefix(lines[i], markBase) {
				baseLabel = parseLabel(lines[i], markBase)
				i++
				for ; i < len(lines); i++ {
					if bytes.HasPrefix(lines[i], markMid) {
						break
					}
					base.Write(lines[i])
				}
				if i >= len(lines) {
					return nil, fmt.Errorf("%w: missing ======= after base", markers.ErrMalformedConflict)
				}
			}

			if !bytes.HasPrefix(lines[i], markMid) {
				return nil, fmt.Errorf("%w: expected =======", markers.ErrMalformedConflict)
			}

			i++
			var theirs bytes.Buffer
			for ; i < len(lines); i++ {
				if bytes.HasPrefix(lines[i], markEnd) {
					break
				}
				theirs.Write(lines[i])
			}
			if i >= len(lines) {
				return nil, fmt.Errorf("%w: missing end marker", markers.ErrMalformedConflict)
			}
			theirsLabel := parseLabel(lines[i], markEnd)

			idx := len(session.Segments)
			session.Segments = append(session.Segments, ConflictBlock{
				Ours:   append([]byte(nil), ours.Bytes()...),
				Base:   append([]byte(nil), base.Bytes()...),
				Theirs: append([]byte(nil), theirs.Bytes()...),
				Labels: Labels{OursLabel: oursLabel, BaseLabel: baseLabel, TheirsLabel: theirsLabel},
			})
			session.Conflicts = append(session.Conflicts, ConflictRef{SegmentIndex: idx})
			continue
		}
		textBuf.Write(line)
	}

	appendText(&textBuf)
	return session, nil
}

func parseLabel(line []byte, prefix []byte) string {
	if !bytes.HasPrefix(line, prefix) {
		return ""
	}
	return string(bytes.TrimSpace(line[len(prefix):]))
}
