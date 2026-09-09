package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chojs23/ec/internal/gitutil"
)

const (
	diffHorizontalScrollStep = 4
	diffExplorerResizeStep   = 2
	diffExplorerMinWidth     = 14
	diffContentMinWidth      = 24
)

type diffViewMode int

const (
	diffViewUnified diffViewMode = iota
	diffViewSplit
)

type diffFocus int

const (
	diffFocusExplorer diffFocus = iota
	diffFocusContent
)

var (
	diffProgram = func(model tea.Model, ctx context.Context) programRunner {
		return tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx), tea.WithMouseCellMotion())
	}
	diffPatchLoader = gitutil.DiffFilePatch
)

type diffLoadedMsg struct {
	key   string
	patch []byte
	err   error
}

type diffLayout struct {
	explorer int
	unified  int
	before   int
	after    int
}

type diffModel struct {
	ctx             context.Context
	repoRoot        string
	source          gitutil.DiffSource
	files           []gitutil.DiffFile
	selected        int
	fileOffset      int
	fileXOffset     int
	patchCache      map[string][]byte
	patchText       string
	deletions       int
	additions       int
	viewportPatch   viewport.Model
	viewportBefore  viewport.Model
	viewportAfter   viewport.Model
	viewMode        diffViewMode
	focus           diffFocus
	explorerVisible bool
	explorerWidth   int
	ready           bool
	width           int
	height          int
	keySeq          string
	keySeqTimeout   int
	back            bool
	quitting        bool
}

// RunDiff opens a read-only file explorer and diff viewer.
func RunDiff(ctx context.Context, repoRoot string, source gitutil.DiffSource, files []gitutil.DiffFile) error {
	if err := ensureThemeLoaded(); err != nil {
		return err
	}

	model := newDiffModel(ctx, repoRoot, source, files)
	program := diffProgram(model, ctx)
	finalModel, err := program.Run()
	if err != nil {
		return fmt.Errorf("diff viewer TUI error: %w", err)
	}

	result, ok := finalModel.(diffModel)
	if !ok {
		return fmt.Errorf("diff viewer returned unexpected model")
	}
	if result.back {
		return ErrBackToSelector
	}
	return nil
}

func newDiffModel(ctx context.Context, repoRoot string, source gitutil.DiffSource, files []gitutil.DiffFile) diffModel {
	patchText := "Loading diff..."
	if len(files) == 0 {
		patchText = "No changed files."
	}

	model := diffModel{
		ctx:             ctx,
		repoRoot:        repoRoot,
		source:          source,
		files:           append([]gitutil.DiffFile(nil), files...),
		patchCache:      make(map[string][]byte),
		patchText:       patchText,
		viewportPatch:   viewport.New(1, 1),
		viewportBefore:  viewport.New(1, 1),
		viewportAfter:   viewport.New(1, 1),
		viewMode:        diffViewSplit,
		focus:           diffFocusExplorer,
		explorerVisible: true,
		explorerWidth:   26,
		width:           80,
		height:          24,
	}
	model.setDiffText(patchText)
	return model
}

func (m diffModel) Init() tea.Cmd {
	return m.loadSelectedPatch()
}

func (m diffModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case diffLoadedMsg:
		if msg.key != m.selectedFileKey() {
			return m, nil
		}
		if msg.err != nil {
			m.setDiffText(fmt.Sprintf("Error loading diff: %v", msg.err))
			return m, nil
		}
		m.patchCache[msg.key] = append([]byte(nil), msg.patch...)
		m.setPatch(msg.patch)
		return m, nil

	case keySeqExpiredMsg:
		if msg.id == m.keySeqTimeout {
			m.keySeq = ""
		}
		return m, nil

	case tea.KeyMsg:
		if command, handled := m.handleKey(msg); handled {
			return m, command
		}

	case tea.MouseMsg:
		if command, handled := m.handleMouse(tea.MouseEvent(msg)); handled {
			return m, command
		}

	case tea.WindowSizeMsg:
		m.width = max(msg.Width, 1)
		m.height = max(msg.Height, 6)
		m.ready = true
		m.resizeViewports()
		return m, nil
	}

	return m, nil
}

func (m *diffModel) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	key := msg.String()
	if key != keyGoTop && m.keySeq != "" {
		m.keySeq = ""
	}

	switch key {
	case keyQuit:
		m.back = true
		m.quitting = true
		return tea.Quit, true
	case keyCtrlC:
		m.quitting = true
		return tea.Quit, true
	case "e":
		m.toggleExplorer()
		return nil, true
	case "s":
		m.toggleViewMode()
		return nil, true
	case "alt+h", "alt+left":
		m.adjustExplorerWidth(-diffExplorerResizeStep)
		return nil, true
	case "alt+l", "alt+right":
		m.adjustExplorerWidth(diffExplorerResizeStep)
		return nil, true
	case keySelectOurs:
		if m.explorerVisible {
			m.focus = diffFocusExplorer
		}
		return nil, true
	case keySelectTheirs:
		m.focus = diffFocusContent
		return nil, true
	case keyArrowLeft:
		m.scrollFocusedHorizontal(-diffHorizontalScrollStep)
		return nil, true
	case keyArrowRight:
		m.scrollFocusedHorizontal(diffHorizontalScrollStep)
		return nil, true
	case keyScrollDown, keyArrowDown:
		return m.scrollFocusedVertical(1), true
	case keyScrollUp, keyArrowUp:
		return m.scrollFocusedVertical(-1), true
	case keyScrollLeft:
		m.scrollFocusedHorizontal(-diffHorizontalScrollStep)
		return nil, true
	case keyScrollRight:
		m.scrollFocusedHorizontal(diffHorizontalScrollStep)
		return nil, true
	case keyCtrlD:
		if m.focus == diffFocusExplorer && m.explorerVisible {
			return m.moveFileSelection(max(m.diffViewportHeight()/2, 1)), true
		}
		m.scrollDiffHalfPage(1)
		return nil, true
	case keyCtrlU:
		if m.focus == diffFocusExplorer && m.explorerVisible {
			return m.moveFileSelection(-max(m.diffViewportHeight()/2, 1)), true
		}
		m.scrollDiffHalfPage(-1)
		return nil, true
	case keyGoTop:
		if m.keySeq == keyGoTop {
			m.keySeq = ""
			if m.focus == diffFocusExplorer && m.explorerVisible {
				return m.selectFile(0), true
			}
			m.gotoDiffTop()
			return nil, true
		}
		m.keySeq = keyGoTop
		m.keySeqTimeout++
		id := m.keySeqTimeout
		return tea.Tick(keySeqTimeoutDuration, func(time.Time) tea.Msg {
			return keySeqExpiredMsg{id: id}
		}), true
	case keyGoBottom:
		if m.focus == diffFocusExplorer && m.explorerVisible {
			return m.selectFile(len(m.files) - 1), true
		}
		m.gotoDiffBottom()
		return nil, true
	}

	return nil, false
}

func (m *diffModel) handleMouse(event tea.MouseEvent) (tea.Cmd, bool) {
	if event.Action != tea.MouseActionPress || !event.IsWheel() {
		return nil, false
	}

	switch event.Button {
	case tea.MouseButtonWheelLeft:
		m.scrollFocusedHorizontal(-diffHorizontalScrollStep)
		return nil, true
	case tea.MouseButtonWheelRight:
		m.scrollFocusedHorizontal(diffHorizontalScrollStep)
		return nil, true
	case tea.MouseButtonWheelUp:
		return m.scrollFocusedVertical(-1), true
	case tea.MouseButtonWheelDown:
		return m.scrollFocusedVertical(1), true
	default:
		return nil, false
	}
}

func (m diffModel) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}
	if m.quitting {
		if m.back {
			return "\n  Returning to selector...\n"
		}
		return "\n  Closing diff viewer...\n"
	}

	mode := "unified"
	if m.viewMode == diffViewSplit {
		mode = "split"
	}
	headerText := fmt.Sprintf("View Diff - %s [%s]", diffSourceTitle(m.source), mode)
	header := headerStyle.Width(m.width).Render(truncateDisplayWidth(headerText, max(m.width-4, 1)))
	layout := m.calculateLayout()
	panes := make([]string, 0, 3)

	if m.explorerVisible {
		style := diffPaneStyle(m.focus == diffFocusExplorer)
		// Short paths must not shrink the pane after its width is allocated.
		style = style.Width(layout.explorer + style.GetHorizontalPadding())
		panes = append(panes, style.Render(
			renderPaneTitle(fmt.Sprintf("FILES  %d", len(m.files)), layout.explorer, selectorSectionStyle)+"\n"+m.renderFileList(layout.explorer),
		))
	}

	diffStyle := diffPaneStyle(m.focus == diffFocusContent)
	if m.viewMode == diffViewUnified {
		panes = append(panes, diffStyle.Render(
			m.renderDiffPaneTitle(m.selectedFileTitle(), layout.unified)+"\n"+m.viewportPatch.View(),
		))
	} else {
		panes = append(panes,
			diffStyle.Render(m.renderDiffPaneTitle(m.beforeTitle(), layout.before)+"\n"+m.viewportBefore.View()),
			diffStyle.Render(m.renderDiffPaneTitle(m.afterTitle(), layout.after)+"\n"+m.viewportAfter.View()),
		)
	}

	footer := m.renderFooter()
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		lipgloss.JoinHorizontal(lipgloss.Top, panes...),
		footer,
	)
}

func diffPaneStyle(focused bool) lipgloss.Style {
	if focused {
		// The diff viewer follows the conflict view's selected-side border so
		// one theme setting represents focus consistently across both screens.
		return selectedSidePaneStyle
	}
	return paneStyle
}

func (m diffModel) loadSelectedPatch() tea.Cmd {
	file, ok := m.selectedFile()
	if !ok {
		return nil
	}

	key := diffFileCacheKey(file)
	if patch, ok := m.patchCache[key]; ok {
		return func() tea.Msg {
			return diffLoadedMsg{key: key, patch: append([]byte(nil), patch...)}
		}
	}

	ctx := m.ctx
	repoRoot := m.repoRoot
	source := m.source
	return func() tea.Msg {
		patch, err := diffPatchLoader(ctx, repoRoot, source, file)
		return diffLoadedMsg{key: key, patch: patch, err: err}
	}
}

func (m *diffModel) scrollFocusedVertical(delta int) tea.Cmd {
	if m.focus == diffFocusExplorer && m.explorerVisible {
		return m.moveFileSelection(delta)
	}
	m.scrollDiffVertical(delta)
	return nil
}

func (m *diffModel) scrollFocusedHorizontal(delta int) {
	if m.focus == diffFocusExplorer && m.explorerVisible {
		m.scrollExplorerHorizontal(delta)
		return
	}
	m.scrollDiffHorizontal(delta)
}

func (m *diffModel) moveFileSelection(delta int) tea.Cmd {
	return m.selectFile(m.selected + delta)
}

func (m *diffModel) selectFile(index int) tea.Cmd {
	if len(m.files) == 0 {
		return nil
	}
	next := min(max(index, 0), len(m.files)-1)
	if next == m.selected {
		return nil
	}
	m.selected = next
	m.ensureFileVisible()
	m.resetDiffScroll()

	key := m.selectedFileKey()
	if patch, ok := m.patchCache[key]; ok {
		m.setPatch(patch)
		return nil
	}
	m.setDiffText("Loading diff...")
	return m.loadSelectedPatch()
}

func (m *diffModel) toggleExplorer() {
	m.explorerVisible = !m.explorerVisible
	if m.explorerVisible {
		m.focus = diffFocusExplorer
	} else {
		m.focus = diffFocusContent
	}
	m.resizeViewports()
}

func (m *diffModel) toggleViewMode() {
	yOffset := m.diffYOffset()
	m.viewMode = diffViewMode((int(m.viewMode) + 1) % 2)
	m.resizeViewports()
	m.setDiffYOffset(yOffset)
}

func (m *diffModel) adjustExplorerWidth(delta int) {
	if !m.explorerVisible {
		return
	}
	layout := m.calculateLayout()
	m.explorerWidth = layout.explorer + delta
	m.resizeViewports()
}

func (m *diffModel) resizeViewports() {
	layout := m.calculateLayout()
	height := m.diffViewportHeight()
	if m.explorerVisible {
		m.explorerWidth = layout.explorer
	}
	m.viewportPatch.Width = layout.unified
	m.viewportPatch.Height = height
	m.viewportBefore.Width = layout.before
	m.viewportBefore.Height = height
	m.viewportAfter.Width = layout.after
	m.viewportAfter.Height = height
	m.ensureFileVisible()
	m.clampExplorerXOffset(layout.explorer)
}

func (m diffModel) calculateLayout() diffLayout {
	diffPaneCount := 1
	if m.viewMode == diffViewSplit {
		diffPaneCount = 2
	}
	totalPanes := diffPaneCount
	if m.explorerVisible {
		totalPanes++
	}
	paneFrameWidth := paneStyle.GetHorizontalFrameSize()
	contentWidth := max(m.width-(totalPanes*paneFrameWidth), totalPanes)
	diffWidth := contentWidth
	layout := diffLayout{}

	if m.explorerVisible {
		requiredDiffWidth := diffPaneCount * diffContentMinWidth
		minExplorer := diffExplorerMinWidth
		maxExplorer := contentWidth - requiredDiffWidth
		if maxExplorer < minExplorer {
			// When the terminal cannot fit every preferred minimum, equal panes
			// retain more useful diff space than letting the explorer dominate.
			minExplorer = max(contentWidth/totalPanes, 1)
			maxExplorer = minExplorer
		}
		// Include padding and borders in the explorer's half-terminal limit.
		maxExplorer = min(maxExplorer, max(m.width/2-paneFrameWidth, 1))
		minExplorer = min(minExplorer, maxExplorer)
		layout.explorer = min(max(m.explorerWidth, minExplorer), maxExplorer)
		diffWidth = max(contentWidth-layout.explorer, diffPaneCount)
	}

	if m.viewMode == diffViewUnified {
		layout.unified = max(diffWidth, 1)
		layout.before = layout.unified
		layout.after = layout.unified
		return layout
	}

	layout.before = max(diffWidth/2, 1)
	layout.after = max(diffWidth-layout.before, 1)
	layout.unified = max(diffWidth, 1)
	return layout
}

func (m *diffModel) setPatch(patch []byte) {
	if len(patch) == 0 {
		m.setDiffText("No changes for this file.")
		return
	}

	rawPatch := string(patch)
	m.deletions, m.additions = countDiffChanges(rawPatch)
	m.patchText = renderDiffPatch(rawPatch)
	before, after := renderSplitDiffPatch(rawPatch)
	m.viewportPatch.SetContent(m.patchText)
	m.viewportBefore.SetContent(before)
	m.viewportAfter.SetContent(after)
	m.resetDiffScroll()
}

func (m *diffModel) setDiffText(text string) {
	m.patchText = text
	m.deletions = 0
	m.additions = 0
	m.viewportPatch.SetContent(text)
	m.viewportBefore.SetContent(text)
	m.viewportAfter.SetContent(text)
	m.resetDiffScroll()
}

func (m *diffModel) scrollDiffVertical(delta int) {
	if m.viewMode == diffViewUnified {
		if delta < 0 {
			m.viewportPatch.LineUp(-delta)
		} else {
			m.viewportPatch.LineDown(delta)
		}
		return
	}
	if delta < 0 {
		m.viewportBefore.LineUp(-delta)
	} else {
		m.viewportBefore.LineDown(delta)
	}
	m.viewportAfter.SetYOffset(m.viewportBefore.YOffset)
}

func (m *diffModel) scrollDiffHalfPage(direction int) {
	if m.viewMode == diffViewUnified {
		if direction < 0 {
			m.viewportPatch.HalfPageUp()
		} else {
			m.viewportPatch.HalfPageDown()
		}
		return
	}
	if direction < 0 {
		m.viewportBefore.HalfPageUp()
	} else {
		m.viewportBefore.HalfPageDown()
	}
	m.viewportAfter.SetYOffset(m.viewportBefore.YOffset)
}

func (m *diffModel) scrollDiffHorizontal(delta int) {
	viewports := []*viewport.Model{&m.viewportPatch}
	if m.viewMode == diffViewSplit {
		viewports = []*viewport.Model{&m.viewportBefore, &m.viewportAfter}
	}
	for _, target := range viewports {
		if delta < 0 {
			target.ScrollLeft(-delta)
		} else {
			target.ScrollRight(delta)
		}
	}
}

func (m *diffModel) gotoDiffTop() {
	m.viewportPatch.GotoTop()
	m.viewportBefore.GotoTop()
	m.viewportAfter.GotoTop()
}

func (m *diffModel) gotoDiffBottom() {
	if m.viewMode == diffViewUnified {
		m.viewportPatch.GotoBottom()
		return
	}
	m.viewportBefore.GotoBottom()
	m.viewportAfter.SetYOffset(m.viewportBefore.YOffset)
}

func (m *diffModel) resetDiffScroll() {
	m.gotoDiffTop()
	m.viewportPatch.SetXOffset(0)
	m.viewportBefore.SetXOffset(0)
	m.viewportAfter.SetXOffset(0)
}

func (m diffModel) diffYOffset() int {
	if m.viewMode == diffViewSplit {
		return m.viewportBefore.YOffset
	}
	return m.viewportPatch.YOffset
}

func (m *diffModel) setDiffYOffset(offset int) {
	if m.viewMode == diffViewSplit {
		m.viewportBefore.SetYOffset(offset)
		m.viewportAfter.SetYOffset(m.viewportBefore.YOffset)
		return
	}
	m.viewportPatch.SetYOffset(offset)
}

func (m *diffModel) scrollExplorerHorizontal(delta int) {
	m.fileXOffset += delta
	m.clampExplorerXOffset(m.calculateLayout().explorer)
}

func (m *diffModel) clampExplorerXOffset(width int) {
	pathWidth := 0
	for _, file := range m.files {
		path := file.Path
		if file.OldPath != "" && file.OldPath != file.Path {
			path = file.OldPath + " => " + file.Path
		}
		pathWidth = max(pathWidth, lipgloss.Width(sanitizeTerminalText(path)))
	}
	const prefixWidth = 7
	maxOffset := max(pathWidth-max(width-prefixWidth, 1), 0)
	m.fileXOffset = min(max(m.fileXOffset, 0), maxOffset)
}

func (m diffModel) selectedFile() (gitutil.DiffFile, bool) {
	if m.selected < 0 || m.selected >= len(m.files) {
		return gitutil.DiffFile{}, false
	}
	return m.files[m.selected], true
}

func (m diffModel) selectedFileKey() string {
	file, ok := m.selectedFile()
	if !ok {
		return ""
	}
	return diffFileCacheKey(file)
}

func (m diffModel) selectedFileTitle() string {
	file, ok := m.selectedFile()
	if !ok {
		return "PATCH"
	}
	return sanitizeTerminalText(file.Path)
}

func (m diffModel) beforeTitle() string {
	file, ok := m.selectedFile()
	if !ok {
		return "OLD"
	}
	path := file.Path
	if file.OldPath != "" {
		path = file.OldPath
	}
	return "OLD " + sanitizeTerminalText(path)
}

func (m diffModel) afterTitle() string {
	file, ok := m.selectedFile()
	if !ok {
		return "NEW"
	}
	return "NEW " + sanitizeTerminalText(file.Path)
}

func (m diffModel) renderDiffPaneTitle(label string, width int) string {
	if width <= 0 {
		return ""
	}

	contentWidth := width - titleStyle.GetHorizontalFrameSize()
	if contentWidth <= 0 {
		return truncateDisplayWidth(label, width)
	}

	deletions := fmt.Sprintf("-%d", m.deletions)
	additions := fmt.Sprintf("+%d", m.additions)
	statsWidth := lipgloss.Width(deletions) + 1 + lipgloss.Width(additions)
	stats := fileStatusDeletedStyle.Render(deletions) + " " + fileStatusAddedStyle.Render(additions)
	const gap = "  "
	if statsWidth+lipgloss.Width(gap) >= contentWidth {
		if statsWidth <= contentWidth {
			return lipgloss.NewStyle().Padding(0, 1).Render(stats)
		}
		return titleStyle.Render(truncateDisplayWidth(deletions+" "+additions, contentWidth))
	}

	label = truncateDisplayWidth(label, contentWidth-lipgloss.Width(gap)-statsWidth)
	label = titleStyle.Copy().Padding(0).Render(label)
	return lipgloss.NewStyle().Padding(0, 1).Render(label + gap + stats)
}

func (m diffModel) renderFileList(width int) string {
	height := m.diffViewportHeight()
	if len(m.files) == 0 {
		lines := []string{"No changed files."}
		for len(lines) < height {
			lines = append(lines, "")
		}
		return strings.Join(lines, "\n")
	}

	end := min(m.fileOffset+height, len(m.files))
	lines := make([]string, 0, height)
	for index := m.fileOffset; index < end; index++ {
		file := m.files[index]
		cursor := "  "
		if index == m.selected {
			cursor = selectedHunkMarkerStyle.Render(">") + " "
		}
		status := diffFileStatusStyle(file.Status).Render(fmt.Sprintf("%-4s", file.Status))
		path := sanitizeTerminalText(file.Path)
		if file.OldPath != "" && file.OldPath != file.Path {
			path = sanitizeTerminalText(file.OldPath) + " => " + sanitizeTerminalText(file.Path)
		}
		prefix := cursor + status + " "
		path = dropDisplayWidth(path, m.fileXOffset)
		path = truncateDisplayWidth(path, max(width-lipgloss.Width(prefix), 1))
		lines = append(lines, prefix+path)
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func diffFileStatusStyle(status string) lipgloss.Style {
	status = strings.TrimSpace(status)
	switch {
	case strings.Contains(status, "U"):
		return fileStatusConflictedStyle
	case strings.HasPrefix(status, "?"):
		return fileStatusUntrackedStyle
	case strings.HasPrefix(status, "A"):
		return fileStatusAddedStyle
	case strings.HasPrefix(status, "D"):
		return fileStatusDeletedStyle
	case strings.HasPrefix(status, "R"), strings.HasPrefix(status, "C"):
		return fileStatusRenamedStyle
	default:
		return fileStatusModifiedStyle
	}
}

func (m *diffModel) ensureFileVisible() {
	height := m.diffViewportHeight()
	if m.selected < m.fileOffset {
		m.fileOffset = m.selected
	}
	if m.selected >= m.fileOffset+height {
		m.fileOffset = m.selected - height + 1
	}
	maxOffset := max(len(m.files)-height, 0)
	m.fileOffset = min(max(m.fileOffset, 0), maxOffset)
}

func (m diffModel) diffViewportHeight() int {
	const (
		headerHeight    = 1
		paneTitleHeight = 1
		paneFrameHeight = 2
	)
	footerHeight := lipgloss.Height(m.renderFooter())
	return max(m.height-headerHeight-footerHeight-paneTitleHeight-paneFrameHeight, 1)
}

var diffFooterHelp = []string{
	"h/l: focus",
	"j/k: move or scroll",
	"H/L: horizontal",
	"e: explorer",
	"s: unified/split",
	"option+h/l: resize",
	"q: back",
}

func (m diffModel) renderFooter() string {
	contentWidth := max(m.width-footerStyle.GetHorizontalFrameSize(), 1)
	return footerStyle.Width(m.width).Render(wrapHelpSegments(diffFooterHelp, contentWidth))
}

func wrapHelpSegments(segments []string, width int) string {
	if len(segments) == 0 || width <= 0 {
		return ""
	}

	lines := make([]string, 0, len(segments))
	current := ""
	for _, segment := range segments {
		segment = truncateDisplayWidth(segment, width)
		candidate := segment
		if current != "" {
			candidate = current + " | " + segment
		}
		if current != "" && lipgloss.Width(candidate) > width {
			lines = append(lines, current)
			current = segment
			continue
		}
		current = candidate
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}

func diffFileCacheKey(file gitutil.DiffFile) string {
	return file.OldPath + "\x00" + file.Path
}

func diffSourceTitle(source gitutil.DiffSource) string {
	if source.Kind == gitutil.DiffSourceWorkingTree {
		return "Working tree"
	}
	if source.Subject == "" {
		return sanitizeTerminalText(source.ShortHash)
	}
	return sanitizeTerminalText(strings.TrimSpace(source.ShortHash + " " + source.Subject))
}

func renderDiffPatch(patch string) string {
	lines := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
	for index, line := range lines {
		line = sanitizeTerminalText(line)
		switch {
		case strings.HasPrefix(line, "diff --git "),
			strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "new file mode "),
			strings.HasPrefix(line, "deleted file mode "),
			strings.HasPrefix(line, "similarity index "),
			strings.HasPrefix(line, "rename from "),
			strings.HasPrefix(line, "rename to "),
			strings.HasPrefix(line, "--- "),
			strings.HasPrefix(line, "+++ "):
			lines[index] = lineNumberStyle.Copy().Bold(true).Render(line)
		case strings.HasPrefix(line, "@@"):
			lines[index] = diffHunkStyle.Render(line)
		case strings.HasPrefix(line, "+"):
			lines[index] = addedLineStyle.Render(line)
		case strings.HasPrefix(line, "-"):
			lines[index] = removedLineStyle.Render(line)
		default:
			lines[index] = resultLineStyle.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}

func countDiffChanges(patch string) (deletions int, additions int) {
	inHunk := false
	for _, line := range strings.Split(strings.TrimSuffix(patch, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inHunk = false
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		case !inHunk:
			continue
		case strings.HasPrefix(line, "-"):
			deletions++
		case strings.HasPrefix(line, "+"):
			additions++
		case line == "":
			inHunk = false
		}
	}
	return deletions, additions
}

func sanitizeTerminalText(value string) string {
	return strings.Map(func(char rune) rune {
		if char == '\t' {
			return char
		}
		if unicode.IsControl(char) {
			return '?'
		}
		return char
	}, value)
}

func dropDisplayWidth(value string, width int) string {
	if width <= 0 {
		return value
	}
	consumed := 0
	for index, char := range value {
		charWidth := lipgloss.Width(string(char))
		if consumed+charWidth > width {
			return value[index:]
		}
		consumed += charWidth
		if consumed == width {
			return value[index+len(string(char)):]
		}
	}
	return ""
}
