package tui

import "strings"

type splitDiffLineKind int

const (
	splitDiffContext splitDiffLineKind = iota
	splitDiffMeta
	splitDiffHunk
	splitDiffRemoved
	splitDiffAdded
)

type splitDiffRow struct {
	before     string
	after      string
	beforeKind splitDiffLineKind
	afterKind  splitDiffLineKind
}

func renderSplitDiffPatch(patch string) (string, string) {
	rows := parseSplitDiffRows(patch)
	if len(rows) == 0 {
		return "No changes for this file.", "No changes for this file."
	}

	before := make([]string, 0, len(rows))
	after := make([]string, 0, len(rows))
	for _, row := range rows {
		before = append(before, renderSplitDiffLine(row.before, row.beforeKind))
		after = append(after, renderSplitDiffLine(row.after, row.afterKind))
	}
	return strings.Join(before, "\n"), strings.Join(after, "\n")
}

func parseSplitDiffRows(patch string) []splitDiffRow {
	patch = strings.TrimSuffix(patch, "\n")
	if patch == "" {
		return nil
	}

	lines := strings.Split(patch, "\n")
	rows := make([]splitDiffRow, 0, len(lines))
	inHunk := false
	// Git groups removed lines before added lines. Pair each group by position
	// so the old and new panes remain vertically aligned without loading files.
	for index := 0; index < len(lines); {
		line := sanitizeTerminalText(lines[index])

		if strings.HasPrefix(line, "@@") {
			rows = append(rows, splitDiffRow{
				before: line, after: line,
				beforeKind: splitDiffHunk, afterKind: splitDiffHunk,
			})
			inHunk = true
			index++
			continue
		}

		if inHunk {
			switch {
			case strings.HasPrefix(line, " "):
				rows = append(rows, splitDiffRow{before: line, after: line})
				index++
				continue
			case strings.HasPrefix(line, "-"), strings.HasPrefix(line, "+"):
				var removed []string
				var added []string
				for index < len(lines) {
					changeLine := sanitizeTerminalText(lines[index])
					if strings.HasPrefix(changeLine, "-") {
						removed = append(removed, changeLine)
						index++
						continue
					}
					if strings.HasPrefix(changeLine, "+") {
						added = append(added, changeLine)
						index++
						continue
					}
					break
				}
				rowCount := max(len(removed), len(added))
				for rowIndex := 0; rowIndex < rowCount; rowIndex++ {
					row := splitDiffRow{beforeKind: splitDiffRemoved, afterKind: splitDiffAdded}
					if rowIndex < len(removed) {
						row.before = removed[rowIndex]
					}
					if rowIndex < len(added) {
						row.after = added[rowIndex]
					}
					rows = append(rows, row)
				}
				continue
			case strings.HasPrefix(line, "\\"):
				rows = append(rows, splitDiffRow{
					before: line, after: line,
					beforeKind: splitDiffMeta, afterKind: splitDiffMeta,
				})
				index++
				continue
			default:
				inHunk = false
				continue
			}
		}

		switch {
		case strings.HasPrefix(line, "--- "):
			row := splitDiffRow{before: line, beforeKind: splitDiffMeta, afterKind: splitDiffMeta}
			if index+1 < len(lines) {
				nextLine := sanitizeTerminalText(lines[index+1])
				if strings.HasPrefix(nextLine, "+++ ") {
					row.after = nextLine
					index++
				}
			}
			rows = append(rows, row)
		case strings.HasPrefix(line, "+++ "):
			rows = append(rows, splitDiffRow{after: line, afterKind: splitDiffMeta})
		default:
			rows = append(rows, splitDiffRow{
				before: line, after: line,
				beforeKind: splitDiffMeta, afterKind: splitDiffMeta,
			})
		}
		index++
	}
	return rows
}

func renderSplitDiffLine(line string, kind splitDiffLineKind) string {
	if line == "" {
		return ""
	}
	switch kind {
	case splitDiffMeta:
		return lineNumberStyle.Copy().Bold(true).Render(line)
	case splitDiffHunk:
		return diffHunkStyle.Render(line)
	case splitDiffRemoved:
		return removedLineStyle.Render(line)
	case splitDiffAdded:
		return addedLineStyle.Render(line)
	default:
		return resultLineStyle.Render(line)
	}
}
