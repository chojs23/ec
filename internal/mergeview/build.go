package mergeview

import (
	"bytes"

	"github.com/chojs23/ec/internal/markers"
)

type sideChange struct {
	baseStart int
	baseEnd   int
	lines     [][]byte
}

type diffOpKind int

const (
	opEqual diffOpKind = iota
	opRemove
	opAdd
)

type diffOp struct {
	kind      diffOpKind
	line      []byte
	baseIndex int
}

func buildSessionFromInputs(baseBytes, oursBytes, theirsBytes []byte) *Session {
	baseLines := markers.SplitLinesKeepEOL(baseBytes)
	oursLines := markers.SplitLinesKeepEOL(oursBytes)
	theirsLines := markers.SplitLinesKeepEOL(theirsBytes)

	oursChanges := buildSideChanges(baseLines, oursLines)
	theirsChanges := buildSideChanges(baseLines, theirsLines)

	session := &Session{}
	var text bytes.Buffer
	flushText := func() {
		if text.Len() == 0 {
			return
		}
		session.Segments = append(session.Segments, TextSegment{Bytes: append([]byte(nil), text.Bytes()...)})
		text.Reset()
	}
	appendLines := func(lines [][]byte) {
		for _, line := range lines {
			text.Write(line)
		}
	}
	appendConflict := func(baseChunk, oursChunk, theirsChunk [][]byte) {
		flushText()
		idx := len(session.Segments)
		session.Segments = append(session.Segments, ConflictBlock{
			Base:   joinLines(baseChunk),
			Ours:   joinLines(oursChunk),
			Theirs: joinLines(theirsChunk),
		})
		session.Conflicts = append(session.Conflicts, ConflictRef{SegmentIndex: idx})
	}

	basePos := 0
	oi := 0
	ti := 0
	for basePos < len(baseLines) || oi < len(oursChanges) || ti < len(theirsChanges) {
		nextStart := len(baseLines)
		if oi < len(oursChanges) && oursChanges[oi].baseStart < nextStart {
			nextStart = oursChanges[oi].baseStart
		}
		if ti < len(theirsChanges) && theirsChanges[ti].baseStart < nextStart {
			nextStart = theirsChanges[ti].baseStart
		}
		if nextStart > basePos {
			appendLines(baseLines[basePos:nextStart])
			basePos = nextStart
			continue
		}

		hasOurs := oi < len(oursChanges) && oursChanges[oi].baseStart == basePos
		hasTheirs := ti < len(theirsChanges) && theirsChanges[ti].baseStart == basePos
		if hasOurs && !hasTheirs {
			appendLines(oursChanges[oi].lines)
			basePos = max(basePos, oursChanges[oi].baseEnd)
			oi++
			continue
		}
		if hasTheirs && !hasOurs {
			appendLines(theirsChanges[ti].lines)
			basePos = max(basePos, theirsChanges[ti].baseEnd)
			ti++
			continue
		}
		if !hasOurs && !hasTheirs {
			if basePos < len(baseLines) {
				appendLines(baseLines[basePos : basePos+1])
				basePos++
				continue
			}
			break
		}

		clusterStart := basePos
		clusterEnd := clusterStart
		oursCluster := make([]sideChange, 0, 2)
		theirsCluster := make([]sideChange, 0, 2)
		for {
			consumed := false
			for oi < len(oursChanges) && changeTouchesCluster(oursChanges[oi], clusterStart, clusterEnd) {
				oursCluster = append(oursCluster, oursChanges[oi])
				if oursChanges[oi].baseEnd > clusterEnd {
					clusterEnd = oursChanges[oi].baseEnd
				}
				oi++
				consumed = true
			}
			for ti < len(theirsChanges) && changeTouchesCluster(theirsChanges[ti], clusterStart, clusterEnd) {
				theirsCluster = append(theirsCluster, theirsChanges[ti])
				if theirsChanges[ti].baseEnd > clusterEnd {
					clusterEnd = theirsChanges[ti].baseEnd
				}
				ti++
				consumed = true
			}
			if !consumed {
				break
			}
		}

		baseChunk := cloneLines(baseLines[clusterStart:clusterEnd])
		oursChunk := applyCluster(baseLines, clusterStart, clusterEnd, oursCluster)
		theirsChunk := applyCluster(baseLines, clusterStart, clusterEnd, theirsCluster)

		switch {
		case linesEqual(oursChunk, theirsChunk):
			appendLines(oursChunk)
		case linesEqual(oursChunk, baseChunk):
			appendLines(theirsChunk)
		case linesEqual(theirsChunk, baseChunk):
			appendLines(oursChunk)
		default:
			appendConflict(baseChunk, oursChunk, theirsChunk)
		}

		basePos = clusterEnd
	}

	if basePos < len(baseLines) {
		appendLines(baseLines[basePos:])
	}
	flushText()
	return session
}

func buildSideChanges(baseLines, sideLines [][]byte) []sideChange {
	ops := diffLines(baseLines, sideLines)
	changes := make([]sideChange, 0, len(ops)/2)
	basePos := 0
	var current *sideChange
	flush := func() {
		if current == nil {
			return
		}
		changes = append(changes, *current)
		current = nil
	}
	for _, op := range ops {
		switch op.kind {
		case opEqual:
			flush()
			basePos++
		case opRemove:
			if current == nil {
				current = &sideChange{baseStart: basePos, baseEnd: basePos}
			}
			basePos++
			current.baseEnd = basePos
		case opAdd:
			if current == nil {
				current = &sideChange{baseStart: basePos, baseEnd: basePos}
			}
			current.lines = append(current.lines, append([]byte(nil), op.line...))
		}
	}
	flush()
	return changes
}

func diffLines(baseLines, sideLines [][]byte) []diffOp {
	if len(baseLines) == 0 && len(sideLines) == 0 {
		return nil
	}
	lcs := make([][]int, len(baseLines)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(sideLines)+1)
	}
	for i := len(baseLines) - 1; i >= 0; i-- {
		for j := len(sideLines) - 1; j >= 0; j-- {
			if bytes.Equal(baseLines[i], sideLines[j]) {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	ops := make([]diffOp, 0, len(baseLines)+len(sideLines))
	i, j := 0, 0
	for i < len(baseLines) && j < len(sideLines) {
		if bytes.Equal(baseLines[i], sideLines[j]) {
			ops = append(ops, diffOp{kind: opEqual, line: baseLines[i], baseIndex: i})
			i++
			j++
			continue
		}
		if lcs[i+1][j] >= lcs[i][j+1] {
			ops = append(ops, diffOp{kind: opRemove, line: baseLines[i], baseIndex: i})
			i++
			continue
		}
		ops = append(ops, diffOp{kind: opAdd, line: sideLines[j], baseIndex: -1})
		j++
	}
	for i < len(baseLines) {
		ops = append(ops, diffOp{kind: opRemove, line: baseLines[i], baseIndex: i})
		i++
	}
	for j < len(sideLines) {
		ops = append(ops, diffOp{kind: opAdd, line: sideLines[j], baseIndex: -1})
		j++
	}
	return ops
}

func changeTouchesCluster(change sideChange, clusterStart, clusterEnd int) bool {
	if clusterStart == clusterEnd {
		return change.baseStart == clusterStart
	}
	if change.baseStart == change.baseEnd {
		return change.baseStart >= clusterStart && change.baseStart <= clusterEnd
	}
	return change.baseStart < clusterEnd && change.baseEnd > clusterStart || change.baseStart == clusterStart
}

func applyCluster(baseLines [][]byte, clusterStart, clusterEnd int, changes []sideChange) [][]byte {
	if len(changes) == 0 {
		return cloneLines(baseLines[clusterStart:clusterEnd])
	}
	out := make([][]byte, 0, clusterEnd-clusterStart)
	pos := clusterStart
	for _, change := range changes {
		if change.baseStart > pos {
			out = append(out, cloneLines(baseLines[pos:change.baseStart])...)
		}
		out = append(out, cloneLines(change.lines)...)
		if change.baseEnd > pos {
			pos = change.baseEnd
		}
	}
	if pos < clusterEnd {
		out = append(out, cloneLines(baseLines[pos:clusterEnd])...)
	}
	return out
}

func cloneLines(lines [][]byte) [][]byte {
	if len(lines) == 0 {
		return nil
	}
	cloned := make([][]byte, 0, len(lines))
	for _, line := range lines {
		cloned = append(cloned, append([]byte(nil), line...))
	}
	return cloned
}

func joinLines(lines [][]byte) []byte {
	return bytes.Join(lines, nil)
}

func linesEqual(left, right [][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !bytes.Equal(left[i], right[i]) {
			return false
		}
	}
	return true
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
