package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/chojs23/ec/internal/markers"
)

type lineInfo struct {
	synthetic bool // UI-only placeholders must not affect multiline syntax state.
	text      string
	category  lineCategory
	selected  bool
	block     blockMarker // Display-only block state, never a source-line category.
}

type blockMarker int

const (
	blockNone blockMarker = iota
	blockStart
	blockEnd
	blockSelected
	blockUnresolved
	blockResolved
)

type conflictSyntaxCache struct {
	live diffSyntaxCache
	base diffSyntaxCache
}

type lineCategory int

const (
	categoryDefault lineCategory = iota
	categoryModified
	categoryAdded
	categoryRemoved
	categoryInsertMarker
)

func (line lineInfo) changeMarker() string {
	if line.synthetic {
		return " "
	}
	switch line.category {
	case categoryRemoved:
		return "-"
	case categoryAdded, categoryModified:
		return "+"
	default:
		return " "
	}
}

func splitLines(content []byte) []string {
	if len(content) == 0 {
		return []string{""}
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) > 0 && content[len(content)-1] == '\n' {
		lines = lines[:len(lines)-1]
	}
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	return lines
}

func splitLogicalLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	return splitLines(content)
}

func renderLines(lines []lineInfo, filename string, cache *conflictSyntaxCache) (string, []lipgloss.Style) {
	if len(lines) == 0 {
		return "", nil
	}
	lineStyles := make([]lipgloss.Style, len(lines))
	sourceCount := 0
	for i, line := range lines {
		style := resultLineStyle
		switch line.category {
		case categoryRemoved:
			style = diffCodeStyle(diffLineRemoved)
		case categoryAdded, categoryModified:
			style = diffCodeStyle(diffLineAdded)
		case categoryInsertMarker:
			style = lineNumberStyle
			switch line.block {
			case blockSelected:
				style = selectedHunkMarkerStyle
			case blockUnresolved:
				style = unresolvedLabelStyle.Bold(true).Background(selectedHunkMarkerStyle.GetBackground())
			case blockResolved:
				style = statusResolvedStyle.Background(selectedHunkMarkerStyle.GetBackground())
			}
		}
		lineStyles[i] = style
		if !line.synthetic && line.category != categoryInsertMarker && line.category != categoryRemoved {
			sourceCount++
		}
	}
	code := highlightConflictCode(filename, lines, lineStyles, cache)
	width := len(fmt.Sprintf("%d", max(sourceCount, 1)))
	sourceNumber := 0
	var b strings.Builder
	for i, line := range lines {
		number := ""
		if !line.synthetic && line.category != categoryInsertMarker && line.category != categoryRemoved {
			sourceNumber++
			number = fmt.Sprintf("%d", sourceNumber)
		}
		markerStyle := lineStyles[i]
		switch line.category {
		case categoryRemoved:
			markerStyle = markerStyle.Foreground(removedLineStyle.GetForeground())
		case categoryAdded, categoryModified:
			markerStyle = markerStyle.Foreground(addedLineStyle.GetForeground())
		}
		b.WriteString(lineNumberStyle.Render(fmt.Sprintf("%*s ", width, number)))
		b.WriteString(markerStyle.Render(line.changeMarker() + " "))
		b.WriteString(code[i])
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String(), lineStyles
}

// UI labels and deleted base text must not affect the current source's lexer.
func highlightConflictCode(filename string, lines []lineInfo, styles []lipgloss.Style, cache *conflictSyntaxCache) []string {
	code := make([]string, len(lines))
	rendered := make([]string, len(lines))
	var liveIndices, baseIndices []int
	hasRemoved := false
	for index, line := range lines {
		text := strings.ReplaceAll(sanitizeTerminalText(line.text), "\t", "    ")
		code[index] = text
		rendered[index] = styles[index].Render(text)
		if line.synthetic || line.category == categoryInsertMarker {
			continue
		}
		if line.category == categoryRemoved {
			baseIndices = append(baseIndices, index)
			hasRemoved = true
		} else {
			liveIndices = append(liveIndices, index)
			if line.category == categoryDefault {
				baseIndices = append(baseIndices, index)
			}
		}
	}
	highlight := func(indices []int, base bool) {
		source := make([]string, len(indices))
		lineStyles := make([]lipgloss.Style, len(indices))
		for index, target := range indices {
			source[index] = code[target]
			lineStyles[index] = styles[target]
		}
		streamCache := &cache.live
		if base {
			streamCache = &cache.base
		}
		for index, text := range streamCache.highlight(filename, source, lineStyles) {
			target := indices[index]
			if base {
				if lines[target].category == categoryRemoved {
					rendered[target] = text
				}
			} else {
				rendered[target] = text
			}
		}
	}
	highlight(liveIndices, false)
	if hasRemoved {
		highlight(baseIndices, true)
	}
	return rendered
}

func renderConflictViewport(view viewport.Model, styles []lipgloss.Style) string {
	visible := strings.Split(view.View(), "\n")
	for index, text := range visible {
		source := view.YOffset + index
		if source >= len(styles) {
			break
		}
		if _, noBackground := styles[source].GetBackground().(lipgloss.NoColor); noBackground {
			continue
		}
		visible[index] = fillLineBackground(text, view.Width, styles[source])
	}
	return strings.Join(visible, "\n")
}

// Pad only the visible part of a row after the viewport has clipped its ANSI text.
func fillLineBackground(text string, width int, style lipgloss.Style) string {
	text = strings.TrimRight(text, " ")
	padding := max(width-lipgloss.Width(text), 0)
	background := lipgloss.NewStyle().Background(style.GetBackground())
	return text + background.Render(strings.Repeat(" ", padding))
}

type paneSide int

const (
	paneOurs paneSide = iota
	paneTheirs
)

type conflictRange struct {
	baseStart   int
	baseEnd     int
	oursStart   int
	oursEnd     int
	theirsStart int
	theirsEnd   int
}

func (r conflictRange) sideRange(side paneSide) (int, int) {
	if side == paneTheirs {
		return r.theirsStart, r.theirsEnd
	}
	return r.oursStart, r.oursEnd
}

type resultRange struct {
	start    int
	end      int
	resolved bool
}

func conflictBlockLine(index int, end bool) lineInfo {
	text := fmt.Sprintf("Conflict %d", index+1)
	block := blockStart
	if end {
		text = fmt.Sprintf("End conflict %d", index+1)
		block = blockEnd
	}
	return lineInfo{text: text, category: categoryInsertMarker, synthetic: true, selected: true, block: block}
}

func setBlockPresentation(lines []lineInfo, block blockMarker) {
	label := ""
	switch block {
	case blockSelected:
		label = " [SELECTED]"
	case blockUnresolved:
		label = " [UNRESOLVED]"
	case blockResolved:
		label = " [RESOLVED]"
	}
	for i := range lines {
		if lines[i].block == blockStart {
			lines[i].text += label
			lines[i].block = block
		}
	}
}

func buildPaneLinesFromDoc(doc markers.Document, side paneSide, highlightConflict int, selectedSide selectionSide) ([]lineInfo, int) {
	var lines []lineInfo
	conflictIndex := -1
	currentStart := 0
	for _, seg := range doc.Segments {
		switch s := seg.(type) {
		case markers.TextSegment:
			lines = append(lines, makeLineInfos(splitLines(s.Bytes), categoryDefault)...)
		case markers.ConflictSegment:
			conflictIndex++
			selected := conflictIndex == highlightConflict
			if selected {
				currentStart = len(lines)
				lines = append(lines, conflictBlockLine(conflictIndex, false))
			}
			ours, theirs := conflictEntries(s)
			entries := ours
			if side == paneTheirs {
				entries = theirs
			}
			for _, entry := range entries {
				lines = append(lines, lineInfo{text: entry.text, category: entry.category, selected: selected})
			}
			if selected {
				lines = append(lines, conflictBlockLine(conflictIndex, true))
			}
		}
	}
	return lines, currentStart
}

func buildPaneLinesFromEntries(doc markers.Document, side paneSide, highlightConflict int, selectedSide selectionSide, entries []lineEntry, ranges []conflictRange) ([]lineInfo, int) {
	var lines []lineInfo
	currentStart := 0
	selectedFound := false
	lastSelected := false
	sideLineIndex := 0

	selectedRange := conflictRange{baseStart: -1, baseEnd: -1, oursStart: -1, oursEnd: -1, theirsStart: -1, theirsEnd: -1}
	if highlightConflict >= 0 && highlightConflict < len(ranges) {
		selectedRange = ranges[highlightConflict]
	}

	baseStart := selectedRange.baseStart
	baseEnd := selectedRange.baseEnd
	sideStart, sideEnd := selectedRange.sideRange(side)
	emptySideSelection := highlightConflict >= 0 && baseStart == baseEnd && sideStart >= 0 && sideStart == sideEnd

	addStartMarker := func() {
		lines = append(lines, conflictBlockLine(highlightConflict, false))
	}
	addEndMarker := func() {
		lines = append(lines, conflictBlockLine(highlightConflict, true))
	}

	for _, entry := range entries {
		if emptySideSelection && !selectedFound && entry.category != categoryRemoved && sideLineIndex == sideStart {
			selectedFound = true
			currentStart = len(lines)
			addStartMarker()
			addEndMarker()
		}

		selected := false
		if highlightConflict >= 0 {
			if entry.category == categoryRemoved {
				if entry.baseIndex >= 0 && baseStart >= 0 && entry.baseIndex >= baseStart && entry.baseIndex < baseEnd {
					selected = true
				}
			} else if sideStart >= 0 && sideLineIndex >= sideStart && sideLineIndex < sideEnd {
				selected = true
			}
		}

		if selected && !selectedFound {
			selectedFound = true
			currentStart = len(lines)
			addStartMarker()
		}
		if !selected && lastSelected {
			addEndMarker()
		}

		lines = append(lines, lineInfo{
			text:     entry.text,
			category: entry.category,
			selected: selected,
		})

		if entry.category != categoryRemoved {
			sideLineIndex++
		}
		lastSelected = selected
	}

	if emptySideSelection && !selectedFound && sideLineIndex == sideStart {
		selectedFound = true
		currentStart = len(lines)
		addStartMarker()
		addEndMarker()
	}

	if lastSelected {
		addEndMarker()
	}

	return lines, currentStart
}

func computeConflictRanges(doc markers.Document, baseLines []string, oursLines []string, theirsLines []string) ([]conflictRange, bool) {
	if len(doc.Conflicts) == 0 {
		return nil, true
	}

	ranges := make([]conflictRange, 0, len(doc.Conflicts))
	basePos := 0
	oursPos := 0
	theirsPos := 0

	for _, seg := range doc.Segments {
		switch s := seg.(type) {
		case markers.TextSegment:
			textLines := splitLogicalLines(s.Bytes)
			if !matchLinesAt(baseLines, textLines, basePos) || !matchLinesAt(oursLines, textLines, oursPos) || !matchLinesAt(theirsLines, textLines, theirsPos) {
				return nil, false
			}
			basePos += len(textLines)
			oursPos += len(textLines)
			theirsPos += len(textLines)
		case markers.ConflictSegment:
			baseSeq := splitLogicalLines(s.Base)
			oursSeq := splitLogicalLines(s.Ours)
			theirsSeq := splitLogicalLines(s.Theirs)

			if !matchLinesAt(baseLines, baseSeq, basePos) || !matchLinesAt(oursLines, oursSeq, oursPos) || !matchLinesAt(theirsLines, theirsSeq, theirsPos) {
				return nil, false
			}

			ranges = append(ranges, conflictRange{
				baseStart:   basePos,
				baseEnd:     basePos + len(baseSeq),
				oursStart:   oursPos,
				oursEnd:     oursPos + len(oursSeq),
				theirsStart: theirsPos,
				theirsEnd:   theirsPos + len(theirsSeq),
			})

			basePos += len(baseSeq)
			oursPos += len(oursSeq)
			theirsPos += len(theirsSeq)
		default:
			return nil, false
		}
	}

	if len(ranges) != len(doc.Conflicts) {
		return nil, false
	}

	if basePos != len(baseLines) || oursPos != len(oursLines) || theirsPos != len(theirsLines) {
		return nil, false
	}

	return ranges, true
}

func matchLinesAt(lines []string, seq []string, start int) bool {
	if start < 0 || start > len(lines) {
		return false
	}
	if len(seq) == 0 {
		return true
	}
	if start+len(seq) > len(lines) {
		return false
	}

	for i, line := range seq {
		if lines[start+i] != line {
			return false
		}
	}

	return true
}

func buildResultLines(doc markers.Document, highlightConflict int, selectedSide selectionSide, manualResolved map[int][]byte, boundaryText [][]byte) ([]lineInfo, int) {
	var lines []lineInfo
	conflictIndex := -1
	currentStart := 0
	appendBoundary := func(index int) {
		if index >= 0 && index < len(boundaryText) && len(boundaryText[index]) > 0 {
			lines = append(lines, makeLineInfos(splitLines(boundaryText[index]), categoryDefault)...)
		}
	}
	appendBoundary(0)
	for segIndex, seg := range doc.Segments {
		switch s := seg.(type) {
		case markers.TextSegment:
			lines = append(lines, makeLineInfos(splitLines(s.Bytes), categoryDefault)...)
		case markers.ConflictSegment:
			conflictIndex++
			selected := conflictIndex == highlightConflict
			if selected {
				currentStart = len(lines)
				lines = append(lines, conflictBlockLine(conflictIndex, false))
			}
			content, manual := manualResolved[conflictIndex]
			preview := !manual && s.Resolution == markers.ResolutionUnset
			if !manual {
				resolution := s.Resolution
				if preview {
					resolution = resolutionFromSelection(selectedSide)
				}
				switch resolution {
				case markers.ResolutionOurs:
					content = s.Ours
				case markers.ResolutionTheirs:
					content = s.Theirs
				case markers.ResolutionBoth:
					content = []byte(string(s.Ours) + string(s.Theirs))
				}
			}
			var entries []lineEntry
			if len(s.Base) == 0 && s.BaseLabel == "" {
				// An empty labeled diff3 base is known. Only a missing base is unknown.
				entries = entriesFromLines(splitLogicalLines(content), categoryDefault)
			} else {
				entries = diffEntries(splitLogicalLines(s.Base), splitLogicalLines(content))
			}
			for _, entry := range entries {
				if entry.category != categoryRemoved {
					lines = append(lines, lineInfo{text: entry.text, category: entry.category, selected: selected})
				}
			}
			if selected && len(content) == 0 {
				text := "Empty result"
				if preview {
					text = "Empty preview"
				}
				lines = append(lines, lineInfo{text: text, category: categoryInsertMarker, synthetic: true})
			}
			if selected {
				lines = append(lines, conflictBlockLine(conflictIndex, true))
			}
		}
		appendBoundary(segIndex + 1)
	}
	return lines, currentStart
}

func buildResultPreviewLines(doc markers.Document, selectedSide selectionSide, manualResolved map[int][]byte, highlightConflict int, boundaryText [][]byte) ([]string, map[int]lineCategory, []resultRange) {
	var lines []string
	forced := map[int]lineCategory{}
	ranges := make([]resultRange, 0, len(doc.Conflicts))
	conflictIndex := -1

	appendLines := func(newLines []string) {
		if len(newLines) == 0 {
			return
		}
		lines = append(lines, newLines...)
	}
	appendBoundary := func(index int) {
		if index < 0 || index >= len(boundaryText) {
			return
		}
		if len(boundaryText[index]) == 0 {
			return
		}
		appendLines(splitLines(boundaryText[index]))
	}

	appendBoundary(0)
	for segIndex, seg := range doc.Segments {
		switch s := seg.(type) {
		case markers.TextSegment:
			appendLines(splitLines(s.Bytes))
		case markers.ConflictSegment:
			conflictIndex++
			start := len(lines)

			if manualBytes, ok := manualResolved[conflictIndex]; ok {
				appendLines(splitLogicalLines(manualBytes))
				if len(lines) == start && conflictIndex == highlightConflict {
					forced[len(lines)] = categoryInsertMarker
					appendLines([]string{"Empty result"})
				}
				ranges = append(ranges, resultRange{start: start, end: len(lines), resolved: true})
				appendBoundary(segIndex + 1)
				continue
			}

			resolved := s.Resolution != markers.ResolutionUnset
			resolution := s.Resolution
			if !resolved {
				resolution = resolutionFromSelection(selectedSide)
			}

			switch resolution {
			case markers.ResolutionOurs:
				appendLines(splitLogicalLines(s.Ours))
			case markers.ResolutionTheirs:
				appendLines(splitLogicalLines(s.Theirs))
			case markers.ResolutionBoth:
				appendLines(splitLogicalLines([]byte(string(s.Ours) + string(s.Theirs))))
			case markers.ResolutionNone:
				if !resolved {
					placeholder := "Empty preview"
					forced[len(lines)] = categoryInsertMarker
					appendLines([]string{placeholder})
				} else if conflictIndex == highlightConflict {
					placeholder := "Empty result"
					forced[len(lines)] = categoryInsertMarker
					appendLines([]string{placeholder})
				}
			}

			if len(lines) == start && conflictIndex == highlightConflict {
				text := "Empty result"
				if !resolved {
					text = "Empty preview"
				}
				forced[len(lines)] = categoryInsertMarker
				appendLines([]string{text})
			}
			ranges = append(ranges, resultRange{start: start, end: len(lines), resolved: resolved})
		}
		appendBoundary(segIndex + 1)
	}

	return lines, forced, ranges
}

func buildResultLinesFromEntries(entries []lineEntry, resultRanges []resultRange, highlightConflict int, forcedCategories map[int]lineCategory) ([]lineInfo, int) {
	var lines []lineInfo
	currentStart := 0
	resultLineIndex := 0
	selectedStart, selectedEnd := -1, -1
	if highlightConflict >= 0 && highlightConflict < len(resultRanges) {
		selectedStart = resultRanges[highlightConflict].start
		selectedEnd = resultRanges[highlightConflict].end
	}
	started, ended := false, false
	addBoundaries := func() {
		if selectedStart >= 0 && resultLineIndex == selectedStart && !started {
			currentStart = len(lines)
			lines = append(lines, conflictBlockLine(highlightConflict, false))
			started = true
		}
		if started && !ended && resultLineIndex == selectedEnd {
			lines = append(lines, conflictBlockLine(highlightConflict, true))
			ended = true
		}
	}
	for _, entry := range entries {
		if entry.category == categoryRemoved {
			continue
		}
		addBoundaries()
		category := entry.category
		forced, synthetic := forcedCategories[resultLineIndex]
		if synthetic {
			category = forced
		}
		lines = append(lines, lineInfo{
			text:      entry.text,
			synthetic: synthetic,
			category:  category,
			selected:  resultLineIndex >= selectedStart && resultLineIndex < selectedEnd,
		})
		resultLineIndex++
	}
	addBoundaries()
	return lines, currentStart
}

func makeLineInfos(lines []string, category lineCategory) []lineInfo {
	infos := make([]lineInfo, 0, len(lines))
	for _, line := range lines {
		infos = append(infos, lineInfo{text: line, category: category})
	}
	return infos
}

type lineEntry struct {
	text      string
	category  lineCategory
	baseIndex int
}

type diffOpKind int

const (
	opEqual diffOpKind = iota
	opRemove
	opAdd
)

type diffOp struct {
	kind      diffOpKind
	text      string
	baseIndex int
}

func conflictEntries(seg markers.ConflictSegment) ([]lineEntry, []lineEntry) {
	baseLines := splitLogicalLines(seg.Base)
	oursLines := splitLogicalLines(seg.Ours)
	theirsLines := splitLogicalLines(seg.Theirs)

	if len(baseLines) == 0 && seg.BaseLabel == "" {
		return entriesFromLines(oursLines, categoryDefault), entriesFromLines(theirsLines, categoryDefault)
	}

	oursEntries := diffEntries(baseLines, oursLines)
	theirsEntries := diffEntries(baseLines, theirsLines)
	return oursEntries, theirsEntries
}

func entriesFromLines(lines []string, category lineCategory) []lineEntry {
	entries := make([]lineEntry, 0, len(lines))
	for _, line := range lines {
		entries = append(entries, lineEntry{text: line, category: category, baseIndex: -1})
	}
	return entries
}

func diffEntries(baseLines []string, sideLines []string) []lineEntry {
	ops := diffOps(baseLines, sideLines)
	entries := make([]lineEntry, 0, len(ops))
	lastRemovedIndex := -1

	for _, op := range ops {
		switch op.kind {
		case opEqual:
			entries = append(entries, lineEntry{text: op.text, category: categoryDefault, baseIndex: op.baseIndex})
			lastRemovedIndex = -1
		case opRemove:
			entries = append(entries, lineEntry{text: op.text, category: categoryRemoved, baseIndex: op.baseIndex})
			lastRemovedIndex = op.baseIndex
		case opAdd:
			cat := categoryAdded
			baseIndex := -1
			if lastRemovedIndex >= 0 {
				cat = categoryModified
				baseIndex = lastRemovedIndex
				lastRemovedIndex = -1
			}
			entries = append(entries, lineEntry{text: op.text, category: cat, baseIndex: baseIndex})
		}
	}

	return entries
}

func diffOps(baseLines []string, sideLines []string) []diffOp {
	if len(baseLines) == 0 && len(sideLines) == 0 {
		return nil
	}

	lcs := make([][]int, len(baseLines)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(sideLines)+1)
	}

	for i := len(baseLines) - 1; i >= 0; i-- {
		for j := len(sideLines) - 1; j >= 0; j-- {
			if baseLines[i] == sideLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var ops []diffOp
	i := 0
	j := 0
	for i < len(baseLines) && j < len(sideLines) {
		if baseLines[i] == sideLines[j] {
			ops = append(ops, diffOp{kind: opEqual, text: baseLines[i], baseIndex: i})
			i++
			j++
			continue
		}

		if lcs[i+1][j] >= lcs[i][j+1] {
			ops = append(ops, diffOp{kind: opRemove, text: baseLines[i], baseIndex: i})
			i++
			continue
		}

		ops = append(ops, diffOp{kind: opAdd, text: sideLines[j], baseIndex: -1})
		j++
	}

	for i < len(baseLines) {
		ops = append(ops, diffOp{kind: opRemove, text: baseLines[i], baseIndex: i})
		i++
	}

	for j < len(sideLines) {
		ops = append(ops, diffOp{kind: opAdd, text: sideLines[j], baseIndex: -1})
		j++
	}

	return ops
}

func resolutionFromSelection(selectedSide selectionSide) markers.Resolution {
	if selectedSide == selectedTheirs {
		return markers.ResolutionTheirs
	}
	return markers.ResolutionOurs
}
