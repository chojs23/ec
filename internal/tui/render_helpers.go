package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chojs23/easy-conflict/internal/markers"
)

type lineInfo struct {
	text      string
	highlight bool
	underline bool
	dim       bool
}

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

func renderLines(lines []lineInfo, numberStyle, lineStyle, highlightStyle lipgloss.Style) string {
	if len(lines) == 0 {
		return ""
	}

	width := len(fmt.Sprintf("%d", len(lines)))
	var b strings.Builder
	for i, line := range lines {
		lineNumber := i + 1
		prefix := fmt.Sprintf("%*d │ ", width, lineNumber)
		styledPrefix := numberStyle.Render(prefix)

		style := lineStyle
		if line.highlight {
			style = highlightStyle
		}
		if line.dim {
			style = style.Copy().Foreground(lipgloss.Color("244"))
		}
		if line.underline {
			style = style.Copy().Underline(true)
		}

		b.WriteString(styledPrefix + style.Render(line.text))
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

type paneSide int

const (
	paneOurs paneSide = iota
	paneTheirs
)

func buildPaneLines(doc markers.Document, side paneSide, highlightConflict int) []lineInfo {
	var lines []lineInfo
	conflictIndex := -1

	for _, seg := range doc.Segments {
		switch s := seg.(type) {
		case markers.TextSegment:
			lines = append(lines, makeLineInfos(splitLines(s.Bytes), false, false, false)...)
		case markers.ConflictSegment:
			conflictIndex++
			highlight := conflictIndex == highlightConflict
			var content []byte
			switch side {
			case paneOurs:
				content = s.Ours
			case paneTheirs:
				content = s.Theirs
			}
			lines = append(lines, makeLineInfos(splitLines(content), false, highlight, false)...)
		}
	}

	return lines
}

func buildResultLines(doc markers.Document, highlightConflict int) []lineInfo {
	var lines []lineInfo
	conflictIndex := -1

	for _, seg := range doc.Segments {
		switch s := seg.(type) {
		case markers.TextSegment:
			lines = append(lines, makeLineInfos(splitLines(s.Bytes), false, false, false)...)
		case markers.ConflictSegment:
			conflictIndex++
			underline := conflictIndex == highlightConflict
			highlight := underline
			if underline {
				lines = append(lines, lineInfo{
					text:      fmt.Sprintf(">> insert %s here >>", resultLabel(s.Resolution)),
					highlight: true,
					dim:       true,
				})
			}
			switch s.Resolution {
			case markers.ResolutionOurs:
				lines = append(lines, makeLineInfos(splitLines(s.Ours), underline, highlight, false)...)
			case markers.ResolutionTheirs:
				lines = append(lines, makeLineInfos(splitLines(s.Theirs), underline, highlight, false)...)
			case markers.ResolutionBoth:
				lines = append(lines, makeLineInfos(splitLines(s.Ours), underline, highlight, false)...)
				lines = append(lines, makeLineInfos(splitLines(s.Theirs), underline, highlight, false)...)
			case markers.ResolutionNone:
				// Write nothing for this conflict.
			default:
				lines = append(lines, lineInfo{text: "[unresolved conflict]", dim: true})
			}
		}
	}

	return lines
}

func makeLineInfos(lines []string, underline bool, highlight bool, dim bool) []lineInfo {
	infos := make([]lineInfo, 0, len(lines))
	for _, line := range lines {
		infos = append(infos, lineInfo{text: line, underline: underline, highlight: highlight, dim: dim})
	}
	return infos
}

func resultLabel(resolution markers.Resolution) string {
	switch resolution {
	case markers.ResolutionOurs:
		return "ours"
	case markers.ResolutionTheirs:
		return "theirs"
	case markers.ResolutionBoth:
		return "both"
	case markers.ResolutionNone:
		return "none"
	default:
		return "selection"
	}
}
