package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chojs23/easy-conflict/internal/markers"
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
			style = style.Copy().Foreground(lipgloss.Color("244"))
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

		prefix := numberStyle.Render(numberText) + " " + connectorStyle.Render(connector) + " "

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

func buildPaneLines(doc markers.Document, side paneSide, highlightConflict int, selectedSide selectionSide) ([]lineInfo, int) {
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
