package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chojs23/ec/internal/markers"
)

type resolverHunkState struct {
	selected string
	applied  string // Empty means not applied. NONE is an explicit resolution.
	valid    bool
}

// Selection is keyboard intent. Applied state belongs to the merge document,
// so moving focus must never change the displayed accepted result.
func (m model) currentHunkState() resolverHunkState {
	state := resolverHunkState{selected: "OURS"}
	if m.selectedSide == selectedTheirs {
		state.selected = "THEIRS"
	}
	if m.currentConflict < 0 || m.currentConflict >= len(m.doc.Conflicts) {
		return state
	}
	ref := m.doc.Conflicts[m.currentConflict]
	if ref.SegmentIndex < 0 || ref.SegmentIndex >= len(m.doc.Segments) {
		return state
	}
	seg, ok := m.doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
	if !ok {
		return state
	}
	state.valid = true
	if _, manual := m.manualResolved[m.currentConflict]; manual {
		state.applied = "manual"
		return state
	}
	switch seg.Resolution {
	case markers.ResolutionOurs:
		state.applied = "OURS"
	case markers.ResolutionTheirs:
		state.applied = "THEIRS"
	case markers.ResolutionBoth:
		state.applied = "BOTH"
	case markers.ResolutionNone:
		state.applied = "NONE"
	}
	return state
}

func (m model) resultStatusText() string {
	state := m.currentHunkState()
	if !state.valid {
		return "No conflict selected"
	}
	if state.applied != "" {
		return "Applied: " + state.applied
	}
	return "Preview: " + state.selected + ", not applied"
}

func (m model) renderResolverState(width int) string {
	state := m.currentHunkState()
	if !state.valid || width <= 0 {
		return ""
	}
	applied := "NOT YET"
	statusStyle := statusUnresolvedStyle
	if state.applied != "" {
		applied = strings.ToUpper(state.applied)
		statusStyle = statusResolvedStyle
	}
	selectionStyle := lipgloss.NewStyle().Foreground(selectedSidePaneStyle.GetBorderTopForeground()).Bold(true)
	fields := []string{
		"SELECTED: " + selectionStyle.Render(state.selected),
		"APPLIED: " + statusStyle.Render(applied),
	}

	style := headerStyle.Width(width)
	if width <= style.GetHorizontalFrameSize() {
		style = style.UnsetPadding()
	}
	innerWidth := width - style.GetHorizontalFrameSize()
	var lines []string
	line := ""
	for _, field := range fields {
		// Wrap complete fields instead of truncating the state of a narrow pane.
		if line != "" && lipgloss.Width(line)+3+lipgloss.Width(field) > innerWidth {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " | "
		}
		line += field
	}
	lines = append(lines, line)
	return style.Render(strings.Join(lines, "\n"))
}
