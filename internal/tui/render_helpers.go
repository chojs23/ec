package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chojs23/ec/internal/markers"
)

type lineInfo struct {
	text      string
	category  lineCategory
	highlight bool
	selected  bool
	underline bool
	dim       bool
	connector string
}

type lineCategory int

const (
	categoryDefault lineCategory = iota
	categoryModified
	categoryAdded
	categoryRemoved
	categoryConflicted
	categoryInsertMarker
	categoryResolved
)

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

func renderLines(
	lines []lineInfo,
	numberStyle lipgloss.Style,
	baseStyles map[lineCategory]lipgloss.Style,
	highlightStyles map[lineCategory]lipgloss.Style,
	selectedStyles map[lineCategory]lipgloss.Style,
	connectorStyles map[lineCategory]lipgloss.Style,
	useWhiteDim bool,
) string {
	if len(lines) == 0 {
		return ""
	}

	width := len(fmt.Sprintf("%d", len(lines)))
	var b strings.Builder
	for i, line := range lines {
		lineNumber := i + 1
		connector := line.connector
		if connector == "" {
			connector = " "
		}

		numberText := fmt.Sprintf("%*d", width, lineNumber)

		style := styleForCategory(baseStyles, line.category, lipgloss.NewStyle())
		if line.highlight {
			style = styleForCategory(highlightStyles, line.category, style)
		}
		if line.selected {
			style = styleForCategory(selectedStyles, line.category, style)
		}
		if line.dim {
			// In the result pane we dim unresolved-preview lines by muting the text.
			// For conflicted lines, keep strong contrast against the (light) red background.
			if useWhiteDim {
				style = style.Copy().Foreground(lipgloss.Color("231"))
			} else if line.category == categoryConflicted {
				style = style.Copy().Foreground(lipgloss.Color("16"))
			} else {
				style = style.Copy().Foreground(lipgloss.Color("244"))
			}
		}
		if line.underline {
			style = style.Copy().Underline(true)
		}

		connectorStyle := styleForCategory(connectorStyles, line.category, numberStyle)
		if line.highlight {
			connectorStyle = styleForCategory(highlightStyles, line.category, connectorStyle)
		}
		if line.selected {
			connectorStyle = styleForCategory(selectedStyles, line.category, connectorStyle)
		}

		prefix := numberStyle.Render(numberText) + " " + connectorStyle.Render(connector+" ")

		b.WriteString(prefix + style.Render(line.text))
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func styleForCategory(styles map[lineCategory]lipgloss.Style, category lineCategory, fallback lipgloss.Style) lipgloss.Style {
	if style, ok := styles[category]; ok {
		return style
	}
	if style, ok := styles[categoryDefault]; ok {
		return style
	}
	return fallback
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

func buildPaneLinesFromDoc(doc markers.Document, side paneSide, highlightConflict int, selectedSide selectionSide) ([]lineInfo, int) {
	var lines []lineInfo
	conflictIndex := -1
	currentStart := -1

	for _, seg := range doc.Segments {
		switch s := seg.(type) {
		case markers.TextSegment:
			segmentLines := splitLines(s.Bytes)
			lines = append(lines, makeLineInfos(segmentLines, categoryDefault, false, false, false, false, "")...)
		case markers.ConflictSegment:
			conflictIndex++
			if conflictIndex == highlightConflict {
				currentStart = len(lines)
			}
			selected := conflictIndex == highlightConflict
			oursEntries, theirsEntries := conflictEntries(s)
			var entries []lineEntry
			switch side {
			case paneOurs:
				entries = oursEntries
			case paneTheirs:
				entries = theirsEntries
			}

			if selected && selectedSideMatchesPane(selectedSide, side) {
				lines = append(lines, lineInfo{
					text:      fmt.Sprintf(">> selected hunk start (%s) >>", sideLabel(side)),
					category:  categoryInsertMarker,
					highlight: true,
					selected:  true,
					underline: false,
					dim:       false,
					connector: connectorForSide(side),
				})
			}

			resolution := s.Resolution
			if resolution == markers.ResolutionUnset && selected {
				resolution = resolutionFromSelection(selectedSide)
			}

			connector := ""
			if selected && resolutionIncludes(resolution, side) {
				connector = connectorForSide(side)
			}

			for _, entry := range entries {
				text := entry.text
				highlight := entry.category != categoryDefault
				dim := entry.category == categoryRemoved
				if entry.category == categoryRemoved {
					text = "- " + text
				}
				lines = append(lines, lineInfo{
					text:      text,
					category:  entry.category,
					highlight: highlight,
					selected:  selected,
					underline: false,
					dim:       dim,
					connector: connector,
				})
			}

			if selected && selectedSideMatchesPane(selectedSide, side) {
				lines = append(lines, lineInfo{
					text:      ">> selected hunk end >>",
					category:  categoryInsertMarker,
					highlight: true,
					selected:  true,
					underline: false,
					dim:       false,
					connector: connectorForSide(side),
				})
			}
		}
	}

	if currentStart == -1 {
		currentStart = 0
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

	resolution := conflictResolutionForIndex(doc, highlightConflict, selectedSide)
	connector := ""
	if highlightConflict >= 0 && resolutionIncludes(resolution, side) {
		connector = connectorForSide(side)
	}

	addStartMarker := func() {
		if !selectedSideMatchesPane(selectedSide, side) {
			return
		}
		lines = append(lines, lineInfo{
			text:      fmt.Sprintf(">> selected hunk start (%s) >>", sideLabel(side)),
			category:  categoryInsertMarker,
			highlight: true,
			selected:  true,
			underline: false,
			dim:       false,
			connector: connectorForSide(side),
		})
	}

	addEndMarker := func() {
		if !selectedSideMatchesPane(selectedSide, side) {
			return
		}
		lines = append(lines, lineInfo{
			text:      ">> selected hunk end >>",
			category:  categoryInsertMarker,
			highlight: true,
			selected:  true,
			underline: false,
			dim:       false,
			connector: connectorForSide(side),
		})
	}

	for _, entry := range entries {
		selected := false
		if highlightConflict >= 0 {
			if entry.baseIndex >= 0 && baseStart >= 0 && entry.baseIndex >= baseStart && entry.baseIndex < baseEnd {
				selected = true
			} else if entry.category != categoryRemoved && sideStart >= 0 && sideLineIndex >= sideStart && sideLineIndex < sideEnd {
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

		text := entry.text
		highlight := entry.category != categoryDefault
		dim := entry.category == categoryRemoved
		if entry.category == categoryRemoved {
			text = "- " + text
		}
		lineConnector := ""
		if selected {
			lineConnector = connector
		}
		lines = append(lines, lineInfo{
			text:      text,
			category:  entry.category,
			highlight: highlight,
			selected:  selected,
			underline: false,
			dim:       dim,
			connector: lineConnector,
		})

		if entry.category != categoryRemoved {
			sideLineIndex++
		}
		lastSelected = selected
	}

	if lastSelected {
		addEndMarker()
	}

	return lines, currentStart
}

func conflictResolutionForIndex(doc markers.Document, conflictIndex int, selectedSide selectionSide) markers.Resolution {
	if conflictIndex < 0 || conflictIndex >= len(doc.Conflicts) {
		return markers.ResolutionUnset
	}

	ref := doc.Conflicts[conflictIndex]
	seg, ok := doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
	if !ok {
		return markers.ResolutionUnset
	}

	resolution := seg.Resolution
	if resolution == markers.ResolutionUnset {
		resolution = resolutionFromSelection(selectedSide)
	}
	return resolution
}

func computeConflictRanges(doc markers.Document, baseLines []string, oursLines []string, theirsLines []string) ([]conflictRange, bool) {
	if len(doc.Conflicts) == 0 {
		return nil, true
	}

	ranges := make([]conflictRange, 0, len(doc.Conflicts))
	basePos := 0
	oursPos := 0
	theirsPos := 0

	for _, ref := range doc.Conflicts {
		seg, ok := doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
		if !ok {
			return nil, false
		}

		baseSeq := splitLines(seg.Base)
		oursSeq := splitLines(seg.Ours)
		theirsSeq := splitLines(seg.Theirs)

		baseStart, baseEnd, okBase := findSequence(baseLines, baseSeq, basePos)
		oursStart, oursEnd, okOurs := findSequence(oursLines, oursSeq, oursPos)
		theirsStart, theirsEnd, okTheirs := findSequence(theirsLines, theirsSeq, theirsPos)
		if !okBase || !okOurs || !okTheirs {
			return nil, false
		}

		ranges = append(ranges, conflictRange{
			baseStart:   baseStart,
			baseEnd:     baseEnd,
			oursStart:   oursStart,
			oursEnd:     oursEnd,
			theirsStart: theirsStart,
			theirsEnd:   theirsEnd,
		})

		basePos = baseEnd
		oursPos = oursEnd
		theirsPos = theirsEnd
	}

	return ranges, true
}

func findSequence(lines []string, seq []string, start int) (int, int, bool) {
	if start < 0 {
		start = 0
	}
	if len(seq) == 0 {
		return start, start, true
	}
	if len(lines) == 0 {
		return -1, -1, false
	}
	if len(seq) > len(lines) {
		return -1, -1, false
	}

	limit := len(lines) - len(seq)
	for i := start; i <= limit; i++ {
		match := true
		for j, line := range seq {
			if lines[i+j] != line {
				match = false
				break
			}
		}
		if match {
			return i, i + len(seq), true
		}
	}

	return -1, -1, false
}

func buildResultLines(doc markers.Document, highlightConflict int, selectedSide selectionSide, manualResolved map[int][]byte) ([]lineInfo, int) {
	var lines []lineInfo
	conflictIndex := -1
	currentStart := -1

	for _, seg := range doc.Segments {
		switch s := seg.(type) {
		case markers.TextSegment:
			segmentLines := splitLines(s.Bytes)
			lines = append(lines, makeLineInfos(segmentLines, categoryDefault, false, false, false, false, "")...)
		case markers.ConflictSegment:
			conflictIndex++
			selected := conflictIndex == highlightConflict
			underline := selected
			if manualBytes, ok := manualResolved[conflictIndex]; ok {
				manualLines := splitLines(manualBytes)
				if selected {
					currentStart = len(lines)
				}
				for _, line := range manualLines {
					lines = append(lines, lineInfo{
						text:      line,
						category:  categoryResolved,
						highlight: false,
						selected:  selected,
						underline: underline,
						dim:       false,
						connector: connectorForResult(true, selected),
					})
				}
				continue
			}
			preview := s.Resolution == markers.ResolutionUnset
			effectiveResolution := s.Resolution
			if preview {
				effectiveResolution = resolutionFromSelection(selectedSide)
			}

			oursEntries, theirsEntries := conflictEntries(s)
			var entries []lineEntry
			switch effectiveResolution {
			case markers.ResolutionOurs:
				entries = oursEntries
			case markers.ResolutionTheirs:
				entries = theirsEntries
			case markers.ResolutionBoth:
				entries = append(entries, oursEntries...)
				entries = append(entries, theirsEntries...)
			case markers.ResolutionNone:
				entries = nil
			default:
				entries = nil
			}

			if selected {
				currentStart = len(lines)
			}

			if len(entries) == 0 {
				if preview {
					lines = append(lines, lineInfo{
						text:      "[unresolved conflict]",
						category:  categoryConflicted,
						dim:       true,
						connector: connectorForResult(false, selected),
					})
				}
				continue
			}

			resolved := !preview
			for _, entry := range entries {
				if entry.category == categoryRemoved {
					continue
				}
				highlight := entry.category != categoryDefault
				category := entry.category
				if resolved {
					category = categoryResolved
				}
				lines = append(lines, lineInfo{
					text:      entry.text,
					category:  category,
					highlight: highlight,
					selected:  selected,
					underline: underline,
					dim:       preview,
					connector: connectorForResult(resolved, selected),
				})
			}

		}
	}

	if currentStart == -1 {
		currentStart = 0
	}
	return lines, currentStart
}

func buildResultPreviewLines(doc markers.Document, selectedSide selectionSide, manualResolved map[int][]byte) ([]string, map[int]lineCategory, []resultRange) {
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

	for _, seg := range doc.Segments {
		switch s := seg.(type) {
		case markers.TextSegment:
			appendLines(splitLines(s.Bytes))
		case markers.ConflictSegment:
			conflictIndex++
			start := len(lines)

			if manualBytes, ok := manualResolved[conflictIndex]; ok {
				appendLines(splitLines(manualBytes))
				ranges = append(ranges, resultRange{start: start, end: len(lines), resolved: true})
				continue
			}

			resolved := s.Resolution != markers.ResolutionUnset
			resolution := s.Resolution
			if !resolved {
				resolution = resolutionFromSelection(selectedSide)
			}

			switch resolution {
			case markers.ResolutionOurs:
				appendLines(splitLines(s.Ours))
			case markers.ResolutionTheirs:
				appendLines(splitLines(s.Theirs))
			case markers.ResolutionBoth:
				appendLines(splitLines(s.Ours))
				appendLines(splitLines(s.Theirs))
			case markers.ResolutionNone:
				placeholder := "[unresolved conflict]"
				forced[len(lines)] = categoryConflicted
				appendLines([]string{placeholder})
			}

			ranges = append(ranges, resultRange{start: start, end: len(lines), resolved: resolved})
		}
	}

	return lines, forced, ranges
}

func buildResultLinesFromEntries(entries []lineEntry, resultRanges []resultRange, highlightConflict int, forcedCategories map[int]lineCategory) ([]lineInfo, int) {
	var lines []lineInfo
	currentStart := 0
	selectedFound := false
	resultLineIndex := 0
	rangeIndex := 0
	activeRange := resultRange{start: -1, end: -1}

	if len(resultRanges) > 0 {
		activeRange = resultRanges[0]
	}

	advanceRange := func() {
		for rangeIndex < len(resultRanges) && resultLineIndex >= activeRange.end {
			rangeIndex++
			if rangeIndex < len(resultRanges) {
				activeRange = resultRanges[rangeIndex]
			} else {
				activeRange = resultRange{start: -1, end: -1}
			}
		}
	}

	selectedStart := -1
	selectedEnd := -1
	if highlightConflict >= 0 && highlightConflict < len(resultRanges) {
		selectedStart = resultRanges[highlightConflict].start
		selectedEnd = resultRanges[highlightConflict].end
	}

	for _, entry := range entries {
		if entry.category == categoryRemoved {
			continue
		}

		advanceRange()

		selected := false
		if highlightConflict >= 0 {
			if resultLineIndex >= selectedStart && resultLineIndex < selectedEnd {
				selected = true
			}
		}

		if selected && !selectedFound {
			selectedFound = true
			currentStart = len(lines)
		}

		connector := ""
		resolved := false
		if resultLineIndex >= activeRange.start && resultLineIndex < activeRange.end {
			resolved = activeRange.resolved
			connector = connectorForResult(resolved, selected)
		}

		category := entry.category
		if forced, ok := forcedCategories[resultLineIndex]; ok {
			category = forced
		}
		if resolved == false && resultLineIndex >= activeRange.start && resultLineIndex < activeRange.end && category != categoryDefault {
			category = categoryConflicted
		}

		highlight := category != categoryDefault
		underline := selected
		dim := !resolved && resultLineIndex >= activeRange.start && resultLineIndex < activeRange.end

		lines = append(lines, lineInfo{
			text:      entry.text,
			category:  category,
			highlight: highlight,
			selected:  selected,
			underline: underline,
			dim:       dim,
			connector: connector,
		})

		resultLineIndex++
	}

	return lines, currentStart
}

func makeLineInfos(lines []string, category lineCategory, underline bool, highlight bool, selected bool, dim bool, connector string) []lineInfo {
	infos := make([]lineInfo, 0, len(lines))
	for _, line := range lines {
		infos = append(infos, lineInfo{text: line, category: category, underline: underline, highlight: highlight, selected: selected, dim: dim, connector: connector})
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
	baseLines := splitLines(seg.Base)
	oursLines := splitLines(seg.Ours)
	theirsLines := splitLines(seg.Theirs)

	if len(baseLines) == 0 {
		return entriesFromLines(oursLines, categoryConflicted), entriesFromLines(theirsLines, categoryConflicted)
	}

	oursEntries := diffEntries(baseLines, oursLines)
	theirsEntries := diffEntries(baseLines, theirsLines)
	markConflicted(&oursEntries, &theirsEntries)
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

func markConflicted(oursEntries *[]lineEntry, theirsEntries *[]lineEntry) {
	oursMap := map[int]int{}
	for i, entry := range *oursEntries {
		if entry.baseIndex >= 0 && entry.category != categoryRemoved {
			oursMap[entry.baseIndex] = i
		}
	}

	theirsMap := map[int]int{}
	for i, entry := range *theirsEntries {
		if entry.baseIndex >= 0 && entry.category != categoryRemoved {
			theirsMap[entry.baseIndex] = i
		}
	}

	for baseIndex, oursIdx := range oursMap {
		theirsIdx, ok := theirsMap[baseIndex]
		if !ok {
			continue
		}

		ours := (*oursEntries)[oursIdx]
		theirs := (*theirsEntries)[theirsIdx]
		if ours.text != theirs.text {
			ours.category = categoryConflicted
			theirs.category = categoryConflicted
			(*oursEntries)[oursIdx] = ours
			(*theirsEntries)[theirsIdx] = theirs
		}
	}
}

func markConflictedInRanges(oursEntries *[]lineEntry, theirsEntries *[]lineEntry, ranges []conflictRange) {
	if len(ranges) == 0 {
		return
	}

	oursMap := map[int]int{}
	for i, entry := range *oursEntries {
		if entry.baseIndex >= 0 && entry.category != categoryRemoved && baseIndexInRanges(entry.baseIndex, ranges) {
			oursMap[entry.baseIndex] = i
		}
	}

	theirsMap := map[int]int{}
	for i, entry := range *theirsEntries {
		if entry.baseIndex >= 0 && entry.category != categoryRemoved && baseIndexInRanges(entry.baseIndex, ranges) {
			theirsMap[entry.baseIndex] = i
		}
	}

	for baseIndex, oursIdx := range oursMap {
		theirsIdx, ok := theirsMap[baseIndex]
		if !ok {
			continue
		}

		ours := (*oursEntries)[oursIdx]
		theirs := (*theirsEntries)[theirsIdx]
		if ours.text != theirs.text {
			ours.category = categoryConflicted
			theirs.category = categoryConflicted
			(*oursEntries)[oursIdx] = ours
			(*theirsEntries)[theirsIdx] = theirs
		}
	}
}

func baseIndexInRanges(index int, ranges []conflictRange) bool {
	for _, r := range ranges {
		if index >= r.baseStart && index < r.baseEnd {
			return true
		}
	}
	return false
}

func resolutionIncludes(resolution markers.Resolution, side paneSide) bool {
	if resolution == markers.ResolutionUnset {
		return false
	}

	switch resolution {
	case markers.ResolutionOurs:
		return side == paneOurs
	case markers.ResolutionTheirs:
		return side == paneTheirs
	case markers.ResolutionBoth:
		return true
	default:
		return false
	}
}

func resolutionFromSelection(selectedSide selectionSide) markers.Resolution {
	if selectedSide == selectedTheirs {
		return markers.ResolutionTheirs
	}
	return markers.ResolutionOurs
}

func connectorForSide(side paneSide) string {
	switch side {
	case paneOurs:
		return ">"
	case paneTheirs:
		return "<"
	default:
		return " "
	}
}

func connectorForResult(resolved bool, selected bool) string {
	if resolved {
		return "v"
	}
	if selected {
		return "|"
	}
	return " "
}

func selectedSideMatchesPane(selectedSide selectionSide, side paneSide) bool {
	if selectedSide == selectedTheirs {
		return side == paneTheirs
	}
	return side == paneOurs
}

func sideLabel(side paneSide) string {
	if side == paneTheirs {
		return "theirs"
	}
	return "ours"
}

func resultLabel(resolution markers.Resolution, preview bool) string {
	label := "selection"
	switch resolution {
	case markers.ResolutionOurs:
		label = "ours"
	case markers.ResolutionTheirs:
		label = "theirs"
	case markers.ResolutionBoth:
		label = "both"
	case markers.ResolutionNone:
		label = "none"
	}
	if preview {
		return "selected " + label
	}
	return label
}
