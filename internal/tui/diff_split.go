package tui

import "strings"

type splitDiffRow struct {
	// Indices refer to the original patch lines. -1 is an alignment placeholder.
	beforeIndex int
	afterIndex  int
}

func parseSplitDiffRows(patch string) []splitDiffRow {
	patch = strings.TrimSuffix(patch, "\n")
	if patch == "" {
		return nil
	}

	lines := strings.Split(patch, "\n")
	rows := make([]splitDiffRow, 0, len(lines))
	inHunk := false
	// Keep source indices so split and unified views share highlighted lines.
	// Pair removed and added groups by position without reading file versions.
	for index := 0; index < len(lines); {
		line := lines[index]
		if strings.HasPrefix(line, "@@") {
			rows = append(rows, splitDiffRow{beforeIndex: index, afterIndex: index})
			inHunk = true
			index++
			continue
		}
		if inHunk {
			switch {
			case strings.HasPrefix(line, " "), strings.HasPrefix(line, "\\"):
				rows = append(rows, splitDiffRow{beforeIndex: index, afterIndex: index})
				index++
				continue
			case strings.HasPrefix(line, "-"), strings.HasPrefix(line, "+"):
				var removed, added []int
				for index < len(lines) {
					if strings.HasPrefix(lines[index], "-") {
						removed = append(removed, index)
					} else if strings.HasPrefix(lines[index], "+") {
						added = append(added, index)
					} else {
						break
					}
					index++
				}
				for rowIndex := 0; rowIndex < max(len(removed), len(added)); rowIndex++ {
					row := splitDiffRow{beforeIndex: -1, afterIndex: -1}
					if rowIndex < len(removed) {
						row.beforeIndex = removed[rowIndex]
					}
					if rowIndex < len(added) {
						row.afterIndex = added[rowIndex]
					}
					rows = append(rows, row)
				}
				continue
			default:
				inHunk = false
			}
		}
		switch {
		case strings.HasPrefix(line, "--- "):
			row := splitDiffRow{beforeIndex: index, afterIndex: -1}
			if index+1 < len(lines) && strings.HasPrefix(lines[index+1], "+++ ") {
				row.afterIndex = index + 1
				index++
			}
			rows = append(rows, row)
		case strings.HasPrefix(line, "+++ "):
			rows = append(rows, splitDiffRow{beforeIndex: -1, afterIndex: index})
		default:
			rows = append(rows, splitDiffRow{beforeIndex: index, afterIndex: index})
		}
		index++
	}
	return rows
}
