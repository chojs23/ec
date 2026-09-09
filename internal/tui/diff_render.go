package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

type diffLineKind int

const (
	diffLineContext diffLineKind = iota
	diffLineMeta
	diffLineHunk
	diffLineRemoved
	diffLineAdded
)

type diffRenderedLine struct {
	text      string
	content   string
	kind      diffLineKind
	oldNumber int
	newNumber int
}

type diffNumberMode int

const (
	diffNumbersBoth diffNumberMode = iota
	diffNumbersOld
	diffNumbersNew
)

var diffHunkStart = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

type diffRenderedPatch struct {
	unified []diffRenderedLine
	before  []diffRenderedLine
	after   []diffRenderedLine
}

func renderDiffPatch(patch, oldPath, newPath string) diffRenderedPatch {
	lines := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
	before := make([]diffRenderedLine, len(lines))
	after := make([]diffRenderedLine, len(lines))
	kinds := make([]diffLineKind, len(lines))
	var oldIndices, newIndices []int

	highlightSide := func(path string, indices []int, target []diffRenderedLine) {
		code := make([]string, len(indices))
		baseStyles := make([]lipgloss.Style, len(indices))
		for index, source := range indices {
			code[index] = lines[source][1:]
			baseStyles[index] = diffCodeStyle(kinds[source])
		}
		for index, text := range highlightDiffCode(path, code, baseStyles) {
			source := indices[index]
			prefix := baseStyles[index].Render(lines[source][:1])
			target[source].text = prefix + text
			target[source].content = text
		}
	}
	flushHunk := func() {
		// A removed delimiter must not change how the new side is tokenized.
		// Reset at each hunk because the code between hunks is not available.
		highlightSide(oldPath, oldIndices, before)
		highlightSide(newPath, newIndices, after)
		oldIndices = oldIndices[:0]
		newIndices = newIndices[:0]
	}

	inHunk := false
	oldNumber, newNumber := 0, 0
	for index, raw := range lines {
		line := strings.ReplaceAll(sanitizeTerminalText(raw), "\t", "    ")
		lines[index] = line
		kind := diffLineMeta
		oldLine, newLine := 0, 0
		switch {
		case strings.HasPrefix(line, "@@"):
			flushHunk()
			inHunk = true
			kind = diffLineHunk
			oldNumber, newNumber = 0, 0
			if match := diffHunkStart.FindStringSubmatch(line); match != nil {
				if value, err := strconv.Atoi(match[1]); err == nil {
					oldNumber = value
				}
				if value, err := strconv.Atoi(match[2]); err == nil {
					newNumber = value
				}
			}
		case inHunk && strings.HasPrefix(line, " "):
			kind = diffLineContext
			oldLine, newLine = oldNumber, newNumber
			if oldNumber > 0 {
				oldNumber++
			}
			if newNumber > 0 {
				newNumber++
			}
			oldIndices = append(oldIndices, index)
			newIndices = append(newIndices, index)
		case inHunk && strings.HasPrefix(line, "-"):
			kind = diffLineRemoved
			oldLine = oldNumber
			if oldNumber > 0 {
				oldNumber++
			}
			oldIndices = append(oldIndices, index)
		case inHunk && strings.HasPrefix(line, "+"):
			kind = diffLineAdded
			newLine = newNumber
			if newNumber > 0 {
				newNumber++
			}
			newIndices = append(newIndices, index)
		case inHunk && strings.HasPrefix(line, "\\"):
			// The no-newline marker is metadata, not part of either source.
		default:
			flushHunk()
			inHunk = false
		}
		kinds[index] = kind
		style := lineNumberStyle.Copy().Bold(true)
		if kind == diffLineHunk {
			style = diffHunkStyle
		}
		text := style.Render(line)
		before[index] = diffRenderedLine{text: text, content: text, kind: kind, oldNumber: oldLine, newNumber: newLine}
		after[index] = before[index]
	}
	flushHunk()

	result := diffRenderedPatch{unified: make([]diffRenderedLine, len(lines))}
	for index, kind := range kinds {
		result.unified[index] = after[index]
		if kind == diffLineRemoved {
			result.unified[index] = before[index]
		}
	}
	for _, row := range parseSplitDiffRows(patch) {
		oldLine, newLine := diffRenderedLine{}, diffRenderedLine{}
		if row.beforeIndex >= 0 {
			oldLine = before[row.beforeIndex]
		}
		if row.afterIndex >= 0 {
			newLine = after[row.afterIndex]
		}
		result.before = append(result.before, oldLine)
		result.after = append(result.after, newLine)
	}
	return result
}

func diffCodeStyle(kind diffLineKind) lipgloss.Style {
	style := resultLineStyle
	switch kind {
	case diffLineRemoved:
		return style.Background(removedLineStyle.GetBackground())
	case diffLineAdded:
		return style.Background(addedLineStyle.GetBackground())
	default:
		return style
	}
}

func diffLinesText(lines []diffRenderedLine) string {
	text := make([]string, len(lines))
	for index, line := range lines {
		text[index] = line.text
	}
	return strings.Join(text, "\n")
}

func renderDiffViewport(view viewport.Model, lines []diffRenderedLine, mode diffNumberMode, paneWidth int) string {
	visible := strings.Split(view.View(), "\n")
	gutterWidth := diffGutterWidth(lines, mode, paneWidth)
	for index, text := range visible {
		source := view.YOffset + index
		if source >= len(lines) {
			visible[index] = strings.Repeat(" ", gutterWidth) + text
			continue
		}
		kind := lines[source].kind
		// The viewport clips styled code but pads short lines without color.
		// Fill only visible padding, not every cached line up to the longest one.
		if kind == diffLineAdded || kind == diffLineRemoved {
			text = fillLineBackground(text, view.Width, diffCodeStyle(kind))
		}
		visible[index] = renderDiffGutter(lines[source], gutterWidth, mode) + text
	}
	return strings.Join(visible, "\n")
}

func diffViewportText(lines []diffRenderedLine) string {
	content := make([]string, len(lines))
	for index, line := range lines {
		content[index] = line.content
	}
	return strings.Join(content, "\n")
}

func diffGutterWidth(lines []diffRenderedLine, mode diffNumberMode, paneWidth int) int {
	if len(lines) == 0 {
		return 0
	}
	maxNumber := 0
	for _, line := range lines {
		switch mode {
		case diffNumbersOld:
			maxNumber = max(maxNumber, line.oldNumber)
		case diffNumbersNew:
			maxNumber = max(maxNumber, line.newNumber)
		default:
			maxNumber = max(maxNumber, line.oldNumber, line.newNumber)
		}
	}
	digits := len(strconv.Itoa(maxNumber))
	width := digits + 3
	if mode == diffNumbersBoth {
		width = 2*digits + 4
	}
	// Very narrow panes keep code space instead of overflowing with numbers.
	if width >= paneWidth {
		return 0
	}
	return width
}

func renderDiffGutter(line diffRenderedLine, width int, mode diffNumberMode) string {
	if width == 0 || line.kind == diffLineMeta || line.kind == diffLineHunk {
		return strings.Repeat(" ", width)
	}
	number := func(value int) string {
		if value <= 0 {
			return ""
		}
		return strconv.Itoa(value)
	}
	marker := " "
	if line.kind == diffLineRemoved {
		marker = "-"
	} else if line.kind == diffLineAdded {
		marker = "+"
	}
	base := diffCodeStyle(line.kind)
	digits := width - 3
	var numbers string
	if mode == diffNumbersBoth {
		digits = (width - 4) / 2
		numbers = fmt.Sprintf("%*s %*s ", digits, number(line.oldNumber), digits, number(line.newNumber))
	} else {
		value := line.oldNumber
		if mode == diffNumbersNew {
			value = line.newNumber
		}
		numbers = fmt.Sprintf("%*s ", digits, number(value))
	}
	return base.Foreground(lineNumberStyle.GetForeground()).Render(numbers) + base.Render(marker+" ")
}
