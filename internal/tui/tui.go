package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chojs23/easy-conflict/internal/cli"
	"github.com/chojs23/easy-conflict/internal/engine"
	"github.com/chojs23/easy-conflict/internal/gitmerge"
	"github.com/chojs23/easy-conflict/internal/markers"
)

const (
	maxUndoSize = 100
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170")).
			Padding(0, 1)

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1)

	selectedPaneStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("205")).
				Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230")).
			Padding(0, 2)

	footerStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("243")).
			Padding(0, 2)

	lineNumberStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	oursHighlightStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("24")).
				Foreground(lipgloss.Color("230"))

	theirsHighlightStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("52")).
				Foreground(lipgloss.Color("230"))

	resultLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	resultHighlightStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("60")).
				Foreground(lipgloss.Color("230"))

	modifiedLineStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("24")).
				Foreground(lipgloss.Color("231"))

	addedLineStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("28")).
			Foreground(lipgloss.Color("231"))

	removedLineStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("237")).
				Foreground(lipgloss.Color("250"))

	conflictedLineStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("88")).
				Foreground(lipgloss.Color("231"))

	insertMarkerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Bold(true)

	selectedHunkBackground = lipgloss.Color("236")

	statusResolvedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Bold(true)

	statusUnresolvedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Bold(true)
)

type model struct {
	ctx             context.Context
	opts            cli.Options
	state           *engine.State
	doc             markers.Document
	currentConflict int
	selectedSide    selectionSide
	pendingScroll   bool
	viewportOurs    viewport.Model
	viewportResult  viewport.Model
	viewportTheirs  viewport.Model
	ready           bool
	width           int
	height          int
	quitting        bool
	err             error
}

type selectionSide int

const (
	selectedOurs selectionSide = iota
	selectedTheirs
)

// Run starts the TUI for interactive conflict resolution.
func Run(ctx context.Context, opts cli.Options) error {
	// Generate diff3 view
	diff3Bytes, err := gitmerge.MergeFileDiff3(ctx, opts.LocalPath, opts.BasePath, opts.RemotePath)
	if err != nil {
		return fmt.Errorf("failed to generate diff3 view: %w", err)
	}

	// Parse conflicts
	doc, err := markers.Parse(diff3Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse conflicts: %w", err)
	}

	// Validate base completeness unless explicitly allowed to proceed without it.
	if !opts.AllowMissingBase {
		if err := engine.ValidateBaseCompleteness(doc); err != nil {
			return fmt.Errorf("base validation failed: %w", err)
		}
	}

	// Initialize state
	state, err := engine.NewState(doc, maxUndoSize)
	if err != nil {
		return fmt.Errorf("failed to create state: %w", err)
	}

	m := model{
		ctx:             ctx,
		opts:            opts,
		state:           state,
		doc:             doc,
		currentConflict: 0,
		selectedSide:    selectedOurs,
		pendingScroll:   true,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Check for errors from the model
	if m, ok := finalModel.(model); ok {
		return m.err
	}

	return nil
}

func (m model) Init() tea.Cmd {
	return nil
}

type editorFinishedMsg struct {
	err error
}

func (m *model) openEditor() tea.Cmd {
	return func() tea.Msg {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}

		if editor == "true" {
			return editorFinishedMsg{err: nil}
		}

		resolved, err := m.state.Preview()
		if err != nil {
			return editorFinishedMsg{err: fmt.Errorf("cannot generate preview for editor: %w", err)}
		}

		mergedBytes, err := os.ReadFile(m.opts.MergedPath)
		if err != nil {
			return editorFinishedMsg{err: fmt.Errorf("read merged for backup: %w", err)}
		}

		if !m.opts.NoBackup {
			bak := m.opts.MergedPath + ".easy-conflict.bak"
			if err := os.WriteFile(bak, mergedBytes, 0o644); err != nil {
				return editorFinishedMsg{err: fmt.Errorf("write backup %s: %w", filepath.Base(bak), err)}
			}
		}

		if err := os.WriteFile(m.opts.MergedPath, resolved, 0o644); err != nil {
			return editorFinishedMsg{err: fmt.Errorf("write merged before editor: %w", err)}
		}

		cmd := exec.Command(editor, m.opts.MergedPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return editorFinishedMsg{err: fmt.Errorf("editor failed: %w", err)}
		}

		return editorFinishedMsg{err: nil}
	}
}

func (m *model) reloadFromFile() error {
	editedBytes, err := os.ReadFile(m.opts.MergedPath)
	if err != nil {
		return fmt.Errorf("read edited file: %w", err)
	}

	diff3Bytes, err := gitmerge.MergeFileDiff3(m.ctx, m.opts.LocalPath, m.opts.BasePath, m.opts.RemotePath)
	if err != nil {
		return fmt.Errorf("regenerate diff3 view: %w", err)
	}

	doc, err := markers.Parse(diff3Bytes)
	if err != nil {
		return fmt.Errorf("parse diff3 view: %w", err)
	}

	if err := engine.ValidateBaseCompleteness(doc); err != nil {
		return fmt.Errorf("base validation failed: %w", err)
	}

	state, err := engine.NewState(doc, maxUndoSize)
	if err != nil {
		return fmt.Errorf("create new state: %w", err)
	}

	editedDoc, err := markers.Parse(editedBytes)
	if err != nil {
		return fmt.Errorf("parse edited file: %w", err)
	}

	for i := range doc.Conflicts {
		if i >= len(editedDoc.Conflicts) {
			continue
		}

		editedRef := editedDoc.Conflicts[i]
		editedSeg, ok := editedDoc.Segments[editedRef.SegmentIndex].(markers.ConflictSegment)
		if !ok {
			continue
		}

		if editedSeg.Resolution != markers.ResolutionUnset {
			if err := state.ApplyResolution(i, editedSeg.Resolution); err != nil {
				return fmt.Errorf("apply resolution from edited file: %w", err)
			}
		}
	}

	m.state = state
	m.doc = state.Document()

	if m.currentConflict >= len(m.doc.Conflicts) {
		m.currentConflict = len(m.doc.Conflicts) - 1
	}
	if m.currentConflict < 0 {
		m.currentConflict = 0
	}

	m.updateViewports()

	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case editorFinishedMsg:
		if msg.err != nil {
			m.err = fmt.Errorf("editor workflow failed: %w", msg.err)
			m.quitting = true
			return m, tea.Quit
		}

		if err := m.reloadFromFile(); err != nil {
			m.err = fmt.Errorf("reload after editor failed: %w", err)
			m.quitting = true
			return m, tea.Quit
		}

		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "n":
			// Next conflict
			if m.currentConflict < len(m.doc.Conflicts)-1 {
				m.currentConflict++
				m.pendingScroll = true
				m.updateViewports()
			}

		case "p":
			// Previous conflict
			if m.currentConflict > 0 {
				m.currentConflict--
				m.pendingScroll = true
				m.updateViewports()
			}

		case "h":
			m.selectedSide = selectedOurs
			m.updateViewports()

		case "l":
			m.selectedSide = selectedTheirs
			m.updateViewports()

		case "o":
			// Apply ours
			if err := m.state.ApplyResolution(m.currentConflict, markers.ResolutionOurs); err != nil {
				m.err = fmt.Errorf("failed to apply ours: %w", err)
				return m, tea.Quit
			}
			m.doc = m.state.Document()
			m.updateViewports()

		case "t":
			// Apply theirs
			if err := m.state.ApplyResolution(m.currentConflict, markers.ResolutionTheirs); err != nil {
				m.err = fmt.Errorf("failed to apply theirs: %w", err)
				return m, tea.Quit
			}
			m.doc = m.state.Document()
			m.updateViewports()

		case "O":
			// Apply ours to all conflicts
			if err := m.state.ApplyAll(markers.ResolutionOurs); err != nil {
				m.err = fmt.Errorf("failed to apply ours to all: %w", err)
				return m, tea.Quit
			}
			m.doc = m.state.Document()
			m.updateViewports()

		case "T":
			// Apply theirs to all conflicts
			if err := m.state.ApplyAll(markers.ResolutionTheirs); err != nil {
				m.err = fmt.Errorf("failed to apply theirs to all: %w", err)
				return m, tea.Quit
			}
			m.doc = m.state.Document()
			m.updateViewports()

		case "a":
			// Accept selected side
			var resolution markers.Resolution
			switch m.selectedSide {
			case selectedTheirs:
				resolution = markers.ResolutionTheirs
			default:
				resolution = markers.ResolutionOurs
			}
			if err := m.state.ApplyResolution(m.currentConflict, resolution); err != nil {
				m.err = fmt.Errorf("failed to apply selection: %w", err)
				return m, tea.Quit
			}
			m.doc = m.state.Document()
			m.updateViewports()

		case "d":
			// Discard selection
			if err := m.state.ApplyResolution(m.currentConflict, markers.ResolutionNone); err != nil {
				m.err = fmt.Errorf("failed to discard selection: %w", err)
				return m, tea.Quit
			}
			m.doc = m.state.Document()
			m.updateViewports()

		case "b":
			// Apply both
			if err := m.state.ApplyResolution(m.currentConflict, markers.ResolutionBoth); err != nil {
				m.err = fmt.Errorf("failed to apply both: %w", err)
				return m, tea.Quit
			}
			m.doc = m.state.Document()
			m.updateViewports()

		case "x":
			// Apply none
			if err := m.state.ApplyResolution(m.currentConflict, markers.ResolutionNone); err != nil {
				m.err = fmt.Errorf("failed to apply none: %w", err)
				return m, tea.Quit
			}
			m.doc = m.state.Document()
			m.updateViewports()

		case "u":
			// Undo
			if err := m.state.Undo(); err == nil {
				m.doc = m.state.Document()
				m.updateViewports()
			}

		case "w":
			// Write and quit
			if err := m.writeResolved(); err != nil {
				m.err = fmt.Errorf("failed to write resolved: %w", err)
			}
			m.quitting = true
			return m, tea.Quit

		case "e":
			return m, m.openEditor()
		}

	case tea.WindowSizeMsg:
		if !m.ready {
			m.width = msg.Width
			m.height = msg.Height

			// Calculate pane dimensions
			headerHeight := 2
			footerHeight := 2
			contentHeight := m.height - headerHeight - footerHeight - 6 // borders + padding

			paneWidth := (m.width - 12) / 3 // 3 panes with borders

			m.viewportOurs = viewport.New(paneWidth, contentHeight)
			m.viewportResult = viewport.New(paneWidth, contentHeight)
			m.viewportTheirs = viewport.New(paneWidth, contentHeight)

			m.ready = true
			m.updateViewports()
		} else {
			m.width = msg.Width
			m.height = msg.Height

			headerHeight := 2
			footerHeight := 2
			contentHeight := m.height - headerHeight - footerHeight - 6

			paneWidth := (m.width - 12) / 3

			m.viewportOurs.Width = paneWidth
			m.viewportOurs.Height = contentHeight
			m.viewportResult.Width = paneWidth
			m.viewportResult.Height = contentHeight
			m.viewportTheirs.Width = paneWidth
			m.viewportTheirs.Height = contentHeight

			m.updateViewports()
		}
	}

	// Update viewports
	m.viewportOurs, cmd = m.viewportOurs.Update(msg)
	cmds = append(cmds, cmd)
	m.viewportResult, cmd = m.viewportResult.Update(msg)
	cmds = append(cmds, cmd)
	m.viewportTheirs, cmd = m.viewportTheirs.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	if m.quitting {
		if m.err != nil {
			return fmt.Sprintf("\n  Error: %v\n", m.err)
		}
		return "\n  Resolved! File written.\n"
	}

	// Header
	fileName := m.opts.MergedPath
	conflictStatus := fmt.Sprintf("Conflict %d/%d", m.currentConflict+1, len(m.doc.Conflicts))
	header := headerStyle.Render(fmt.Sprintf("%s - %s", fileName, conflictStatus))

	// Get current conflict
	if m.currentConflict >= len(m.doc.Conflicts) {
		return "\n  No conflicts found.\n"
	}

	ref := m.doc.Conflicts[m.currentConflict]
	seg, ok := m.doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
	if !ok {
		return "\n  Internal error: invalid conflict segment.\n"
	}

	// Resolution status
	statusText := "Unresolved"
	if seg.Resolution != markers.ResolutionUnset {
		statusText = fmt.Sprintf("Resolved: %s", seg.Resolution)
	}

	// Render panes
	oursPane := paneStyle.Render(
		titleStyle.Render("OURS") + "\n" +
			m.viewportOurs.View(),
	)

	resultPane := selectedPaneStyle.Render(
		titleStyle.Render(fmt.Sprintf("RESULT (%s)", statusText)) + "\n" +
			m.viewportResult.View(),
	)

	theirsPane := paneStyle.Render(
		titleStyle.Render("THEIRS") + "\n" +
			m.viewportTheirs.View(),
	)

	panes := lipgloss.JoinHorizontal(lipgloss.Top, oursPane, resultPane, theirsPane)

	// Footer
	undoInfo := ""
	if m.state.UndoDepth() > 0 {
		undoInfo = fmt.Sprintf(" | Undo available: %d", m.state.UndoDepth())
	}

	footer := footerStyle.Width(m.width).Render(
		fmt.Sprintf("n: next | p: prev | j/k: scroll | h: ours | l: theirs | a: accept | d: discard | o: ours | t: theirs | O: ours all | T: theirs all | b: both | x: none | u: undo | e: editor | w: write & quit | q: quit%s", undoInfo),
	)

	return lipgloss.JoinVertical(lipgloss.Left, header, panes, footer)
}

func (m *model) updateViewports() {
	if m.currentConflict >= len(m.doc.Conflicts) {
		return
	}

	baseStyles := map[lineCategory]lipgloss.Style{
		categoryDefault: resultLineStyle,
	}

	highlightStyles := map[lineCategory]lipgloss.Style{
		categoryModified:     modifiedLineStyle,
		categoryAdded:        addedLineStyle,
		categoryRemoved:      removedLineStyle,
		categoryConflicted:   conflictedLineStyle,
		categoryInsertMarker: insertMarkerStyle,
	}

	selectedStyles := map[lineCategory]lipgloss.Style{
		categoryDefault: resultLineStyle.Copy().Bold(true),
	}
	for category, style := range highlightStyles {
		selectedStyles[category] = style.Copy().Bold(true)
	}

	connectorStyles := map[lineCategory]lipgloss.Style{
		categoryDefault: lineNumberStyle,
	}
	for category, style := range highlightStyles {
		connectorStyles[category] = style
	}

	// Update ours pane (full file, highlight conflicts)
	oursLines, oursStart := buildPaneLines(m.doc, paneOurs, m.currentConflict, m.selectedSide)
	oursContent := renderLines(oursLines, lineNumberStyle, baseStyles, highlightStyles, selectedStyles, connectorStyles)
	m.viewportOurs.SetContent(oursContent)
	if m.pendingScroll {
		ensureVisible(&m.viewportOurs, oursStart, len(oursLines))
	}

	// Update theirs pane (full file, highlight conflicts)
	theirsLines, theirsStart := buildPaneLines(m.doc, paneTheirs, m.currentConflict, m.selectedSide)
	theirsContent := renderLines(theirsLines, lineNumberStyle, baseStyles, highlightStyles, selectedStyles, connectorStyles)
	m.viewportTheirs.SetContent(theirsContent)
	if m.pendingScroll {
		ensureVisible(&m.viewportTheirs, theirsStart, len(theirsLines))
	}

	// Update result pane with full resolved preview
	resultLines, resultStart := buildResultLines(m.doc, m.currentConflict, m.selectedSide)
	resultContent := renderLines(resultLines, lineNumberStyle, baseStyles, highlightStyles, selectedStyles, connectorStyles)
	m.viewportResult.SetContent(resultContent)
	if m.pendingScroll {
		ensureVisible(&m.viewportResult, resultStart, len(resultLines))
	}
	if m.pendingScroll {
		m.pendingScroll = false
	}
}

func ensureVisible(viewportModel *viewport.Model, start int, total int) {
	if viewportModel.Height <= 0 {
		return
	}
	if total <= 0 {
		viewportModel.YOffset = 0
		return
	}

	maxOffset := total - viewportModel.Height
	if maxOffset < 0 {
		maxOffset = 0
	}

	margin := 2
	target := start - margin
	if target < 0 {
		target = 0
	}
	if target > maxOffset {
		target = maxOffset
	}
	viewportModel.YOffset = target
}

func (m *model) writeResolved() error {
	// Generate preview
	resolved, err := m.state.Preview()
	if err != nil {
		return fmt.Errorf("cannot write: %w", err)
	}

	// Read original merged file for backup
	mergedBytes, err := os.ReadFile(m.opts.MergedPath)
	if err != nil {
		return fmt.Errorf("read merged for backup: %w", err)
	}

	// Write backup if not disabled
	if !m.opts.NoBackup {
		bak := m.opts.MergedPath + ".easy-conflict.bak"
		if err := os.WriteFile(bak, mergedBytes, 0o644); err != nil {
			return fmt.Errorf("write backup %s: %w", filepath.Base(bak), err)
		}
	}

	// Write resolved file
	if err := os.WriteFile(m.opts.MergedPath, resolved, 0o644); err != nil {
		return fmt.Errorf("write merged: %w", err)
	}

	// Verify no conflict markers remain
	postDoc, err := markers.Parse(resolved)
	if err != nil {
		return fmt.Errorf("post-parse merged: %w", err)
	}
	if len(postDoc.Conflicts) != 0 {
		return fmt.Errorf("resolution output still contains conflict markers")
	}

	return nil
}
