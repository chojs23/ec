package mergeview

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/chojs23/ec/internal/markers"
)

func ReplayMergedResult(session *Session, mergedBytes []byte) (*Session, map[int][]byte, []Labels, []bool, error) {
	replayed, manual, labels, known, err := replayMergedSession(session.Clone(), mergedBytes)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return replayed, manual, labels, known, nil
}

func RenderMergedOutput(session *Session, manualResolved map[int][]byte, mergedLabels []Labels, mergedLabelKnown []bool) ([]byte, bool, error) {
	var out bytes.Buffer
	hasUnresolved := false
	conflictIndex := -1

	for _, seg := range session.Segments {
		switch s := seg.(type) {
		case TextSegment:
			out.Write(s.Bytes)
		case ConflictBlock:
			conflictIndex++
			if manualBytes, ok := manualResolved[conflictIndex]; ok {
				out.Write(manualBytes)
				continue
			}
			labels := s.Labels
			if conflictIndex < len(mergedLabels) && conflictIndex < len(mergedLabelKnown) && mergedLabelKnown[conflictIndex] {
				labels = mergedLabels[conflictIndex]
			}
			if appendConflictBlock(&out, s, labels) {
				hasUnresolved = true
			}
		default:
			return nil, false, fmt.Errorf("unknown session segment type %T", seg)
		}
	}

	return out.Bytes(), hasUnresolved, nil
}

func AllResolved(session *Session, manualResolved map[int][]byte) bool {
	for idx, ref := range session.Conflicts {
		if _, ok := manualResolved[idx]; ok {
			continue
		}
		block, ok := session.Segments[ref.SegmentIndex].(ConflictBlock)
		if !ok || block.Resolution == markers.ResolutionUnset {
			return false
		}
	}
	return true
}

func replayMergedSession(session *Session, mergedBytes []byte) (*Session, map[int][]byte, []Labels, []bool, error) {
	mergedLines := markers.SplitLinesKeepEOL(mergedBytes)
	pos := 0
	manualResolved := map[int][]byte{}
	alignedLabels := make([]Labels, len(session.Conflicts))
	alignedLabelKnown := make([]bool, len(session.Conflicts))
	conflictIndex := -1
	pendingTextIndex := -1
	pendingTextStart := 0

	setPendingText := func(end int) error {
		if pendingTextIndex < 0 {
			return nil
		}
		if end < pendingTextStart {
			end = pendingTextStart
		}
		if end > len(mergedLines) {
			end = len(mergedLines)
		}
		textSeg, ok := session.Segments[pendingTextIndex].(TextSegment)
		if !ok {
			return fmt.Errorf("internal: expected text segment at index %d", pendingTextIndex)
		}
		textSeg.Bytes = bytes.Join(mergedLines[pendingTextStart:end], nil)
		session.Segments[pendingTextIndex] = textSeg
		pendingTextIndex = -1
		return nil
	}

	for i, seg := range session.Segments {
		switch s := seg.(type) {
		case TextSegment:
			_ = s
			pendingTextIndex = i
			pendingTextStart = pos
		case ConflictBlock:
			conflictIndex++
			searchPos := pos
			var pendingTextLines [][]byte
			if pendingTextIndex >= 0 {
				textSeg, ok := session.Segments[pendingTextIndex].(TextSegment)
				if !ok {
					return nil, manualResolved, alignedLabels, alignedLabelKnown, fmt.Errorf("internal: expected text segment at index %d", pendingTextIndex)
				}
				pendingTextLines = markers.SplitLinesKeepEOL(textSeg.Bytes)
				if len(pendingTextLines) > 0 {
					searchPos = alignTextSegmentEnd(mergedLines, pos, pendingTextLines)
					if searchPos < pos {
						searchPos = pos
					}
					if searchPos > len(mergedLines) {
						searchPos = len(mergedLines)
					}
				}
			}
			nextTextLines := nextTextSegmentLinesSession(session.Segments, i+1)
			nextIdx := -1
			if len(nextTextLines) > 0 {
				nextIdx = findSubslice(mergedLines, searchPos, nextTextLines)
				if nextIdx == -1 {
					nextIdx = findApproxSubslice(mergedLines, searchPos, nextTextLines)
				}
				if nextIdx == searchPos && textLinesBlankOnly(nextTextLines) {
					if fallbackIdx := findNextConflictBoundarySession(mergedLines, searchPos, session.Segments, i+1); fallbackIdx > searchPos {
						nextIdx = fallbackIdx
					}
				}
			}
			if nextIdx == -1 {
				nextIdx = len(mergedLines)
			}
			conflictPos := pos
			if textAlignedBeforeConflict(mergedLines, pos, searchPos, pendingTextLines) {
				conflictPos = searchPos
			}
			if nextIdx < conflictPos {
				return nil, manualResolved, alignedLabels, alignedLabelKnown, fmt.Errorf("failed to align conflict segment")
			}
			spanLines := mergedLines[conflictPos:nextIdx]
			start, end, resolution, manualBytes, labels, labelsKnown := classifyConflictSpan(spanLines, pendingTextLines, s)
			if start < 0 || end < start || end > len(spanLines) {
				return nil, manualResolved, alignedLabels, alignedLabelKnown, fmt.Errorf("internal: invalid conflict span classification")
			}
			if err := setPendingText(conflictPos + start); err != nil {
				return nil, manualResolved, alignedLabels, alignedLabelKnown, err
			}
			if labelsKnown {
				alignedLabels[conflictIndex] = labels
				alignedLabelKnown[conflictIndex] = true
			}
			if manualBytes != nil {
				manualResolved[conflictIndex] = manualBytes
			} else {
				s.Resolution = resolution
				session.Segments[i] = s
			}
			pos = conflictPos + end
		}
	}
	if err := setPendingText(len(mergedLines)); err != nil {
		return nil, manualResolved, alignedLabels, alignedLabelKnown, err
	}
	return session, manualResolved, alignedLabels, alignedLabelKnown, nil
}

func classifyConflictSpan(spanLines [][]byte, pendingTextLines [][]byte, seg ConflictBlock) (int, int, markers.Resolution, []byte, Labels, bool) {
	if markerStart, markerEnd, ok := locateConflictMarkerSpan(spanLines); ok {
		labels := labelsFromConflictSpan(spanLines[markerStart:markerEnd])
		return markerStart, markerEnd, markers.ResolutionUnset, nil, labels, true
	}
	if len(spanLines) == 0 {
		return 0, 0, inferEmptyOutputResolution(seg), nil, Labels{}, false
	}
	if textLinesBlankOnly(spanLines) {
		return len(spanLines), len(spanLines), inferEmptyOutputResolution(seg), nil, Labels{}, false
	}
	if matchStart, matchEnd, resolution, ok := findBestResolutionMatch(spanLines, seg); ok {
		return matchStart, matchEnd, resolution, nil, Labels{}, false
	}
	manualStart, manualExact := detectManualStart(spanLines, pendingTextLines)
	if manualStart < len(spanLines) && textLinesBlankOnly(spanLines[manualStart:]) {
		return len(spanLines), len(spanLines), inferEmptyOutputResolution(seg), nil, Labels{}, false
	}
	if manualStart == len(spanLines) {
		if manualExact && (!textLinesBlankOnly(pendingTextLines) || textLinesBlankOnly(spanLines)) {
			return manualStart, len(spanLines), inferEmptyOutputResolution(seg), nil, Labels{}, false
		}
		return 0, len(spanLines), markers.ResolutionUnset, bytes.Join(spanLines, nil), Labels{}, false
	}
	return manualStart, len(spanLines), markers.ResolutionUnset, bytes.Join(spanLines[manualStart:], nil), Labels{}, false
}

func inferEmptyOutputResolution(seg ConflictBlock) markers.Resolution {
	oursEmpty := len(markers.SplitLinesKeepEOL(seg.Ours)) == 0
	theirsEmpty := len(markers.SplitLinesKeepEOL(seg.Theirs)) == 0
	if oursEmpty && !theirsEmpty {
		return markers.ResolutionOurs
	}
	if theirsEmpty && !oursEmpty {
		return markers.ResolutionTheirs
	}
	return markers.ResolutionNone
}

func detectManualStart(spanLines [][]byte, pendingTextLines [][]byte) (int, bool) {
	if len(spanLines) == 0 || len(pendingTextLines) == 0 {
		return 0, false
	}
	if idx := findSubslice(spanLines, 0, pendingTextLines); idx != -1 {
		start := idx + len(pendingTextLines)
		if start > len(spanLines) {
			return len(spanLines), true
		}
		return start, true
	}
	if idx := findApproxSubslice(spanLines, 0, pendingTextLines); idx != -1 {
		start := idx + len(pendingTextLines)
		if start < 0 {
			start = 0
		}
		if start > len(spanLines) {
			start = len(spanLines)
		}
		return start, false
	}
	return 0, false
}

func locateConflictMarkerSpan(lines [][]byte) (int, int, bool) {
	start := -1
	for i, line := range lines {
		if bytes.HasPrefix(line, []byte("<<<<<<<")) {
			start = i
			break
		}
	}
	if start == -1 {
		return -1, -1, false
	}
	for i := start + 1; i < len(lines); i++ {
		if bytes.HasPrefix(lines[i], []byte(">>>>>>>")) {
			return start, i + 1, true
		}
	}
	return start, len(lines), true
}

func findBestResolutionMatch(spanLines [][]byte, seg ConflictBlock) (int, int, markers.Resolution, bool) {
	if len(spanLines) == 0 {
		return 0, 0, inferEmptyOutputResolution(seg), true
	}
	ours := markers.SplitLinesKeepEOL(seg.Ours)
	theirs := markers.SplitLinesKeepEOL(seg.Theirs)
	both := append(append([][]byte{}, ours...), theirs...)
	candidates := []struct {
		resolution markers.Resolution
		lines      [][]byte
	}{{markers.ResolutionOurs, ours}, {markers.ResolutionTheirs, theirs}, {markers.ResolutionBoth, both}}
	found := false
	bestStart, bestEnd, bestTotal, bestSuffix, bestPrefix := 0, 0, 0, 0, 0
	bestResolution := markers.ResolutionUnset
	for _, candidate := range candidates {
		if len(candidate.lines) == 0 {
			continue
		}
		searchStart := 0
		for {
			idx := findSubslice(spanLines, searchStart, candidate.lines)
			if idx == -1 {
				break
			}
			end := idx + len(candidate.lines)
			prefix := idx
			suffix := len(spanLines) - end
			total := prefix + suffix
			if !found || total < bestTotal || (total == bestTotal && suffix < bestSuffix) || (total == bestTotal && suffix == bestSuffix && prefix < bestPrefix) {
				found = true
				bestStart = idx
				bestEnd = end
				bestResolution = candidate.resolution
				bestTotal = total
				bestSuffix = suffix
				bestPrefix = prefix
			}
			searchStart = idx + 1
		}
	}
	if !found {
		return 0, 0, markers.ResolutionUnset, false
	}
	return bestStart, bestEnd, bestResolution, true
}

func findApproxSubslice(haystack [][]byte, start int, needle [][]byte) int {
	if len(needle) == 0 {
		return start
	}
	if start < 0 {
		start = 0
	}
	if len(needle) == 1 {
		return findApproxLineIndex(haystack, start, needle[0])
	}
	window := len(needle)
	if window > 8 {
		window = 8
	}
	for size := window; size >= 2; size-- {
		for offset := 0; offset+size <= len(needle); offset++ {
			chunk := needle[offset : offset+size]
			idx := findSubslice(haystack, start, chunk)
			if idx == -1 {
				continue
			}
			candidateStart := idx - offset
			if candidateStart < start {
				continue
			}
			return candidateStart
		}
	}
	return -1
}

func findApproxLineIndex(lines [][]byte, start int, needle []byte) int {
	needleTrimmed := bytes.TrimRight(needle, "\r\n")
	if len(needleTrimmed) == 0 {
		return -1
	}
	bestIndex := -1
	bestScore := 0
	for i := start; i < len(lines); i++ {
		score := lineSimilarityPercent(lines[i], needle)
		if score > bestScore {
			bestScore = score
			bestIndex = i
		}
	}
	if bestScore >= 70 {
		return bestIndex
	}
	return -1
}

func textAlignedBeforeConflict(mergedLines [][]byte, pos int, searchPos int, pendingTextLines [][]byte) bool {
	if pos < 0 || searchPos <= pos || searchPos > len(mergedLines) {
		return false
	}
	alignedLines := mergedLines[pos:searchPos]
	if len(alignedLines) == 0 {
		return false
	}
	if idx := findSubslice(pendingTextLines, 0, alignedLines); idx != -1 {
		return true
	}
	if idx := findApproxSubslice(pendingTextLines, 0, alignedLines); idx != -1 {
		return true
	}
	return canAlignPreservedText(alignedLines, pendingTextLines)
}

func canAlignPreservedText(alignedLines [][]byte, pendingTextLines [][]byte) bool {
	alignedIndex := 0
	pendingIndex := 0
	for alignedIndex < len(alignedLines) && pendingIndex < len(pendingTextLines) {
		if linesEquivalentForAlignment(alignedLines[alignedIndex], pendingTextLines[pendingIndex]) {
			alignedIndex++
			pendingIndex++
			continue
		}
		if alignedIndex+1 < len(alignedLines) && linesEquivalentForAlignment(alignedLines[alignedIndex+1], pendingTextLines[pendingIndex]) {
			alignedIndex++
			continue
		}
		if pendingIndex+1 < len(pendingTextLines) && linesEquivalentForAlignment(alignedLines[alignedIndex], pendingTextLines[pendingIndex+1]) {
			pendingIndex++
			continue
		}
		if lineSimilarityPercent(alignedLines[alignedIndex], pendingTextLines[pendingIndex]) >= 70 {
			alignedIndex++
			pendingIndex++
			continue
		}
		return false
	}
	return alignedIndex == len(alignedLines)
}

func alignTextSegmentEnd(mergedLines [][]byte, start int, textLines [][]byte) int {
	if start < 0 {
		start = 0
	}
	if start > len(mergedLines) {
		return len(mergedLines)
	}
	if len(textLines) == 0 {
		return start
	}
	if idx := findSubslice(mergedLines, start, textLines); idx != -1 {
		return idx + len(textLines)
	}
	mergedIndex := start
	textIndex := 0
	for textIndex < len(textLines) && mergedIndex < len(mergedLines) {
		if linesEquivalentForAlignment(mergedLines[mergedIndex], textLines[textIndex]) {
			mergedIndex++
			textIndex++
			continue
		}
		if mergedIndex+1 < len(mergedLines) && linesEquivalentForAlignment(mergedLines[mergedIndex+1], textLines[textIndex]) {
			mergedIndex++
			continue
		}
		if textIndex+1 < len(textLines) && linesEquivalentForAlignment(mergedLines[mergedIndex], textLines[textIndex+1]) {
			textIndex++
			continue
		}
		mergedIndex++
		textIndex++
	}
	if mergedIndex > len(mergedLines) {
		return len(mergedLines)
	}
	return mergedIndex
}

func linesEquivalentForAlignment(a []byte, b []byte) bool {
	aTrimmed := bytes.TrimRight(a, "\r\n")
	bTrimmed := bytes.TrimRight(b, "\r\n")
	if bytes.Equal(aTrimmed, bTrimmed) {
		return true
	}
	if len(aTrimmed) == 0 || len(bTrimmed) == 0 {
		return false
	}
	return lineSimilarityPercent(a, b) >= 88
}

func lineSimilarityPercent(a []byte, b []byte) int {
	aTrimmed := bytes.TrimRight(a, "\r\n")
	bTrimmed := bytes.TrimRight(b, "\r\n")
	if bytes.Equal(aTrimmed, bTrimmed) {
		return 100
	}
	maxLen := len(aTrimmed)
	if len(bTrimmed) > maxLen {
		maxLen = len(bTrimmed)
	}
	if maxLen == 0 {
		return 100
	}
	minLen := len(aTrimmed)
	if len(bTrimmed) < minLen {
		minLen = len(bTrimmed)
	}
	best := 0
	if minLen > 0 && (bytes.Contains(aTrimmed, bTrimmed) || bytes.Contains(bTrimmed, aTrimmed)) {
		best = minLen * 100 / maxLen
	}
	prefix := commonPrefixLen(aTrimmed, bTrimmed)
	suffix := commonSuffixLen(aTrimmed, bTrimmed, prefix)
	if prefix+suffix > minLen {
		suffix = minLen - prefix
		if suffix < 0 {
			suffix = 0
		}
	}
	combined := (prefix + suffix) * 100 / maxLen
	if combined > best {
		best = combined
	}
	return best
}

func commonPrefixLen(a []byte, b []byte) int {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	count := 0
	for count < limit && a[count] == b[count] {
		count++
	}
	return count
}

func commonSuffixLen(a []byte, b []byte, prefix int) int {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	if prefix > limit {
		prefix = limit
	}
	maxSuffix := limit - prefix
	count := 0
	for count < maxSuffix {
		ai := len(a) - 1 - count
		bi := len(b) - 1 - count
		if a[ai] != b[bi] {
			break
		}
		count++
	}
	return count
}

func labelsFromConflictSpan(lines [][]byte) Labels {
	var labels Labels
	for _, line := range lines {
		text := strings.TrimRight(string(line), "\r\n")
		switch {
		case strings.HasPrefix(text, "<<<<<<<"):
			labels.OursLabel = strings.TrimSpace(strings.TrimPrefix(text, "<<<<<<<"))
		case strings.HasPrefix(text, "|||||||"):
			labels.BaseLabel = strings.TrimSpace(strings.TrimPrefix(text, "|||||||"))
		case strings.HasPrefix(text, ">>>>>>>"):
			labels.TheirsLabel = strings.TrimSpace(strings.TrimPrefix(text, ">>>>>>>"))
		}
	}
	return labels
}

func textLinesBlankOnly(lines [][]byte) bool {
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) != 0 {
			return false
		}
	}
	return true
}

func findNextConflictBoundarySession(mergedLines [][]byte, start int, segments []Segment, segmentStart int) int {
	best := findConflictMarkerLineIndex(mergedLines, start)
	nextConflict, ok := nextConflictBlock(segments, segmentStart)
	if !ok {
		return best
	}
	for _, candidate := range conflictMatchCandidates(nextConflict) {
		if len(candidate) == 0 {
			continue
		}
		idx := findSubslice(mergedLines, start, candidate)
		if idx == -1 {
			continue
		}
		if best == -1 || idx < best {
			best = idx
		}
	}
	return best
}

func nextConflictBlock(segments []Segment, start int) (ConflictBlock, bool) {
	for i := start; i < len(segments); i++ {
		if seg, ok := segments[i].(ConflictBlock); ok {
			return seg, true
		}
	}
	return ConflictBlock{}, false
}

func conflictMatchCandidates(seg ConflictBlock) [][][]byte {
	ours := markers.SplitLinesKeepEOL(seg.Ours)
	theirs := markers.SplitLinesKeepEOL(seg.Theirs)
	both := append(append([][]byte{}, ours...), theirs...)
	return [][][]byte{ours, theirs, both}
}

func findConflictMarkerLineIndex(lines [][]byte, start int) int {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(lines); i++ {
		if bytes.HasPrefix(lines[i], []byte("<<<<<<<")) {
			return i
		}
	}
	return -1
}

func nextTextSegmentLinesSession(segments []Segment, start int) [][]byte {
	for i := start; i < len(segments); i++ {
		if text, ok := segments[i].(TextSegment); ok {
			lines := markers.SplitLinesKeepEOL(text.Bytes)
			if len(lines) > 0 {
				return lines
			}
		}
	}
	return nil
}

func findSubslice(haystack [][]byte, start int, needle [][]byte) int {
	if len(needle) == 0 {
		return start
	}
	if start < 0 {
		start = 0
	}
	for i := start; i+len(needle) <= len(haystack); i++ {
		matched := true
		for j := range needle {
			if !bytes.Equal(haystack[i+j], needle[j]) {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func MatchResolution(lines [][]byte, block ConflictBlock) (markers.Resolution, bool) {
	ours := markers.SplitLinesKeepEOL(block.Ours)
	theirs := markers.SplitLinesKeepEOL(block.Theirs)
	both := append(append([][]byte{}, ours...), theirs...)
	if slices.EqualFunc(lines, ours, bytes.Equal) {
		return markers.ResolutionOurs, true
	}
	if slices.EqualFunc(lines, theirs, bytes.Equal) {
		return markers.ResolutionTheirs, true
	}
	if slices.EqualFunc(lines, both, bytes.Equal) {
		return markers.ResolutionBoth, true
	}
	if len(lines) == 0 {
		return markers.ResolutionNone, true
	}
	return markers.ResolutionUnset, false
}

func appendConflictBlock(out *bytes.Buffer, block ConflictBlock, labels Labels) bool {
	writeMarker := func(prefix string, label string) {
		out.WriteString(prefix)
		if label != "" {
			out.WriteByte(' ')
			out.WriteString(label)
		}
		out.WriteByte('\n')
	}

	switch block.Resolution {
	case markers.ResolutionOurs:
		out.Write(block.Ours)
		return false
	case markers.ResolutionTheirs:
		out.Write(block.Theirs)
		return false
	case markers.ResolutionBoth:
		out.Write(block.Ours)
		out.Write(block.Theirs)
		return false
	case markers.ResolutionNone:
		return false
	default:
		writeMarker("<<<<<<<", labels.OursLabel)
		out.Write(block.Ours)
		if len(block.Base) > 0 || labels.BaseLabel != "" {
			writeMarker("|||||||", labels.BaseLabel)
			out.Write(block.Base)
		}
		writeMarker("=======", "")
		out.Write(block.Theirs)
		writeMarker(">>>>>>>", labels.TheirsLabel)
		return true
	}
}
