package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chojs23/ec/internal/gitutil"
)

type FileCandidate struct {
	Path     string
	Resolved bool
}

type WorkspaceSelectionKind int

const (
	WorkspaceSelectionConflict WorkspaceSelectionKind = iota + 1
	WorkspaceSelectionDiff
)

type WorkspaceSelection struct {
	Kind         WorkspaceSelectionKind
	ConflictPath string
	DiffSource   gitutil.DiffSource
}

type selectorItemKind int

const (
	selectorConflict selectorItemKind = iota
	selectorDiff
)

type selectorItem struct {
	kind       selectorItemKind
	path       string
	resolved   bool
	diffSource gitutil.DiffSource
}

type selectorLine struct {
	text      string
	itemIndex int
}

type programRunner interface {
	Run() (tea.Model, error)
}

var (
	resolvedLabelStyle   lipgloss.Style
	unresolvedLabelStyle lipgloss.Style
	selectProgram        = func(model tea.Model, ctx context.Context) programRunner {
		return tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))
	}
)

type workspaceSelectModel struct {
	items         []selectorItem
	conflictCount int
	showDiff      bool
	selected      int
	offset        int
	width         int
	height        int
	result        WorkspaceSelection
	err           error
}

var ErrSelectorQuit = fmt.Errorf("selector quit")

const (
	keyNextUnresolvedFile = keyNextConflict
	keyPrevUnresolvedFile = keyPrevConflict
)

// SelectWorkspace opens the no-argument entry screen for conflict resolution
// and read-only Git diff browsing.
func SelectWorkspace(ctx context.Context, conflicts []FileCandidate, diffSources []gitutil.DiffSource) (WorkspaceSelection, error) {
	if err := ensureThemeLoaded(); err != nil {
		return WorkspaceSelection{}, err
	}

	model := newWorkspaceSelectModel(conflicts, diffSources)
	if len(model.items) == 0 {
		return WorkspaceSelection{}, fmt.Errorf("no workspace actions available")
	}

	program := selectProgram(model, ctx)
	finalModel, err := program.Run()
	if err != nil {
		return WorkspaceSelection{}, fmt.Errorf("workspace selector TUI error: %w", err)
	}

	result, ok := finalModel.(workspaceSelectModel)
	if !ok {
		return WorkspaceSelection{}, fmt.Errorf("workspace selector returned unexpected model")
	}
	if result.err != nil {
		return WorkspaceSelection{}, result.err
	}
	if result.result.Kind == 0 {
		return WorkspaceSelection{}, fmt.Errorf("no workspace action selected")
	}
	return result.result, nil
}

// SelectFile keeps the focused conflict selector API for callers that already
// have a conflict-only workflow.
func SelectFile(ctx context.Context, candidates []FileCandidate) (string, error) {
	selection, err := SelectWorkspace(ctx, candidates, nil)
	if err != nil {
		return "", err
	}
	if selection.Kind != WorkspaceSelectionConflict {
		return "", fmt.Errorf("selector returned a non-conflict action")
	}
	return selection.ConflictPath, nil
}

func newWorkspaceSelectModel(conflicts []FileCandidate, diffSources []gitutil.DiffSource) workspaceSelectModel {
	items := make([]selectorItem, 0, len(conflicts)+len(diffSources))
	for _, conflict := range conflicts {
		items = append(items, selectorItem{
			kind:     selectorConflict,
			path:     conflict.Path,
			resolved: conflict.Resolved,
		})
	}
	for _, source := range diffSources {
		items = append(items, selectorItem{kind: selectorDiff, diffSource: source})
	}

	model := workspaceSelectModel{
		items:         items,
		conflictCount: len(conflicts),
		showDiff:      len(diffSources) > 0,
		width:         80,
		height:        24,
	}
	selectFirstUnresolvedFile(&model)
	return model
}

func (m workspaceSelectModel) Init() tea.Cmd {
	return nil
}

func (m workspaceSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.err = ErrSelectorQuit
			return m, tea.Quit
		case "enter":
			if item, ok := m.selectedItem(); ok {
				m.result = selectionFromItem(item)
				return m, tea.Quit
			}
		case "down", "j":
			m.moveSelection(1)
			return m, nil
		case "up", "k":
			m.moveSelection(-1)
			return m, nil
		case "pgup", "left", "h", "b", "u":
			m.moveSelection(-m.bodyHeight())
			return m, nil
		case "pgdown", "right", "l", "f", "d":
			m.moveSelection(m.bodyHeight())
			return m, nil
		case "home", "g":
			m.moveSelection(-m.selected)
			return m, nil
		case "end", "G":
			m.moveSelection(len(m.items) - 1 - m.selected)
			return m, nil
		case keyNextUnresolvedFile:
			selectAdjacentUnresolvedFile(&m, 1)
			return m, nil
		case keyPrevUnresolvedFile:
			selectAdjacentUnresolvedFile(&m, -1)
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.width = max(msg.Width, 1)
		m.height = max(msg.Height, 3)
		m.ensureSelectionVisible()
	}
	return m, nil
}

func (m workspaceSelectModel) View() string {
	diffCount := len(m.items) - m.conflictCount
	headerText := fmt.Sprintf("ec  Workspace   %d conflicts   %d diff sources", m.conflictCount, diffCount)
	header := headerStyle.Width(m.width).Render(truncateDisplayWidth(headerText, max(m.width-4, 1)))
	footer := m.renderFooter()

	bodyHeight := m.bodyHeight()
	lines := m.renderLines()
	start := min(m.offset, max(len(lines)-bodyHeight, 0))
	end := min(start+bodyHeight, len(lines))
	visible := append([]selectorLine(nil), lines[start:end]...)
	for len(visible) < bodyHeight {
		visible = append(visible, selectorLine{})
	}

	textLines := make([]string, 0, len(visible))
	for _, line := range visible {
		textLines = append(textLines, truncateDisplayWidth(line.text, m.width))
	}
	body := strings.Join(textLines, "\n")
	return strings.Join([]string{header, body, footer}, "\n")
}

func (m workspaceSelectModel) selectedItem() (selectorItem, bool) {
	if m.selected < 0 || m.selected >= len(m.items) {
		return selectorItem{}, false
	}
	return m.items[m.selected], true
}

func (m *workspaceSelectModel) moveSelection(delta int) {
	if len(m.items) == 0 {
		return
	}
	m.selected = min(max(m.selected+delta, 0), len(m.items)-1)
	m.ensureSelectionVisible()
}

func (m *workspaceSelectModel) ensureSelectionVisible() {
	selectedLine := -1
	lines := m.renderLines()
	for index, line := range lines {
		if line.itemIndex == m.selected {
			selectedLine = index
			break
		}
	}
	if selectedLine < 0 {
		return
	}

	bodyHeight := m.bodyHeight()
	if selectedLine < m.offset {
		m.offset = selectedLine
	}
	if selectedLine >= m.offset+bodyHeight {
		m.offset = selectedLine - bodyHeight + 1
	}
	maxOffset := max(len(lines)-bodyHeight, 0)
	m.offset = min(max(m.offset, 0), maxOffset)
}

func (m workspaceSelectModel) bodyHeight() int {
	return max(m.height-1-lipgloss.Height(m.renderFooter()), 1)
}

func (m workspaceSelectModel) renderFooter() string {
	help := []string{"j/k: move", "enter: open", "q: quit"}
	if m.conflictCount > 0 {
		help = []string{"j/k: move", "n/p: unresolved", "enter: open", "q: quit"}
	}
	contentWidth := max(m.width-footerStyle.GetHorizontalFrameSize(), 1)
	return footerStyle.Width(m.width).Render(wrapHelpSegments(help, contentWidth))
}

func (m workspaceSelectModel) renderLines() []selectorLine {
	lines := make([]selectorLine, 0, len(m.items)+3)
	if m.conflictCount > 0 {
		heading := fmt.Sprintf("Conflicts  %d", m.conflictCount)
		lines = append(lines, selectorLine{text: selectorSectionStyle.Render(heading), itemIndex: -1})
		for index := 0; index < m.conflictCount; index++ {
			lines = append(lines, selectorLine{text: m.renderItem(index), itemIndex: index})
		}
	}

	if m.showDiff {
		if len(lines) > 0 {
			lines = append(lines, selectorLine{text: "", itemIndex: -1})
		}
		heading := fmt.Sprintf("View Diff  %d", len(m.items)-m.conflictCount)
		lines = append(lines, selectorLine{text: selectorSectionStyle.Render(heading), itemIndex: -1})
		for index := m.conflictCount; index < len(m.items); index++ {
			lines = append(lines, selectorLine{text: m.renderItem(index), itemIndex: index})
		}
	}
	return lines
}

func (m workspaceSelectModel) renderItem(index int) string {
	item := m.items[index]
	selected := index == m.selected
	cursor := "  "
	if selected {
		cursor = selectorCursorStyle.Render(">") + " "
	}

	if item.kind == selectorConflict {
		label := "unresolved"
		labelStyle := unresolvedLabelStyle
		if item.resolved {
			label = "resolved"
			labelStyle = resolvedLabelStyle
		}
		labelText := fmt.Sprintf("%*s", len("unresolved"), label)
		pathWidth := max(m.width-lipgloss.Width(cursor)-lipgloss.Width(labelText)-2, 1)
		path := truncateDisplayWidth(sanitizeTerminalText(item.path), pathWidth)
		if selected {
			path = selectorSelectedTextStyle.Render(path)
		}
		return cursor + labelStyle.Render(labelText) + "  " + path
	}

	if item.diffSource.Kind == gitutil.DiffSourceWorkingTree {
		label := "working tree"
		descriptionWidth := max(m.width-lipgloss.Width(cursor)-lipgloss.Width(label)-2, 1)
		description := truncateDisplayWidth("staged, unstaged, and untracked changes", descriptionWidth)
		return cursor + selectorWorkingTreeStyle.Render(label) + "  " + selectorMutedStyle.Render(description)
	}
	return m.renderCommitItem(cursor, item.diffSource, selected)
}

func (m workspaceSelectModel) renderCommitItem(cursor string, source gitutil.DiffSource, selected bool) string {
	stats := ""
	statsWidth := 0
	if source.StatsKnown {
		deletions := fmt.Sprintf("-%d", source.Deletions)
		additions := fmt.Sprintf("+%d", source.Additions)
		stats = fileStatusDeletedStyle.Render(deletions) + " " + fileStatusAddedStyle.Render(additions)
		statsWidth = lipgloss.Width(deletions) + 1 + lipgloss.Width(additions)
	}

	cursorWidth := lipgloss.Width(cursor)
	hashWidth := min(10, max(m.width-cursorWidth-statsWidth-1, 1))
	hash := truncateDisplayWidth(sanitizeTerminalText(source.ShortHash), hashWidth)
	hash = fmt.Sprintf("%-*s", hashWidth, hash)
	left := cursor + selectorCommitHashStyle.Render(hash)

	subjectWidth := m.width - lipgloss.Width(left)
	if stats != "" {
		subjectWidth -= statsWidth + 2
	}
	if subjectWidth > 1 {
		subject := truncateDisplayWidth(sanitizeTerminalText(source.Subject), subjectWidth-1)
		if selected {
			subject = selectorSelectedTextStyle.Render(subject)
		}
		left += " " + subject
	}

	if stats == "" {
		return left
	}
	gap := max(m.width-lipgloss.Width(left)-statsWidth, 1)
	return left + strings.Repeat(" ", gap) + stats
}

func selectionFromItem(item selectorItem) WorkspaceSelection {
	if item.kind == selectorConflict {
		return WorkspaceSelection{
			Kind:         WorkspaceSelectionConflict,
			ConflictPath: item.path,
		}
	}
	return WorkspaceSelection{Kind: WorkspaceSelectionDiff, DiffSource: item.diffSource}
}

func selectFirstUnresolvedFile(model *workspaceSelectModel) {
	for index := 0; index < model.conflictCount; index++ {
		if !model.items[index].resolved {
			model.selected = index
			return
		}
	}
}

func selectAdjacentUnresolvedFile(model *workspaceSelectModel, direction int) {
	start := model.selected + direction
	if direction < 0 {
		start = min(start, model.conflictCount-1)
	}
	for index := start; index >= 0 && index < model.conflictCount; index += direction {
		if !model.items[index].resolved {
			model.selected = index
			model.ensureSelectionVisible()
			return
		}
	}
}
