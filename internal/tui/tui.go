package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

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

	oursPaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("255")).
			Padding(0, 1)

	theirsPaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("255")).
			Padding(0, 1)

	selectedSidePaneStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("33")).
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
			Foreground(lipgloss.Color("231"))

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
				Background(lipgloss.Color("131")).
				Foreground(lipgloss.Color("231"))

	insertMarkerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Bold(true)

	selectedHunkMarkerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("226")).
				Background(lipgloss.Color("88")).
				Bold(true)

	selectedHunkBackground = lipgloss.Color("236")

	statusResolvedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Bold(true)

	statusUnresolvedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Bold(true)

	resultResolvedMarkerStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("42")).
					Bold(true)

	resultResolvedPaneStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("42")).
				Padding(0, 1)

	resultUnresolvedPaneStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("196")).
					Padding(0, 1)

	toastStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("22")).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1)

	toastLineStyle = lipgloss.NewStyle().
			Align(lipgloss.Right).
			Padding(0, 2)
)

var ErrBackToSelector = fmt.Errorf("back to selector")

type model struct {
	ctx             context.Context
	opts            cli.Options
	state           *engine.State
	doc             markers.Document
	baseLines       []string
	oursLines       []string
	theirsLines     []string
	conflictRanges  []conflictRange
	useFullDiff     bool
	currentConflict int
	selectedSide    selectionSide
	manualResolved  map[int][]byte
	pendingScroll   bool
	viewportOurs    viewport.Model
	viewportResult  viewport.Model
	viewportTheirs  viewport.Model
	ready           bool
	width           int
	height          int
	quitting        bool
	toastMessage    string
	toastSeq        int
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

	manualResolved := map[int][]byte{}
	if mergedBytes, err := os.ReadFile(opts.MergedPath); err == nil {
		updated, manual, updateErr := applyMergedResolutions(doc, mergedBytes)
		if updateErr == nil {
			doc = updated
			manualResolved = manual
		}
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

	baseLines, oursLines, theirsLines, ranges, useFullDiff := prepareFullDiff(doc, opts)

	m := model{
		ctx:             ctx,
		opts:            opts,
		state:           state,
		doc:             doc,
		baseLines:       baseLines,
		oursLines:       oursLines,
		theirsLines:     theirsLines,
		conflictRanges:  ranges,
		useFullDiff:     useFullDiff,
		currentConflict: 0,
		selectedSide:    selectedOurs,
		manualResolved:  manualResolved,
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

type toastExpiredMsg struct {
	id int
}

func (m *model) showToast(message string, duration time.Duration) tea.Cmd {
	m.toastMessage = message
	m.toastSeq++
	seq := m.toastSeq
	return tea.Tick(duration*time.Second, func(time.Time) tea.Msg {
		return toastExpiredMsg{id: seq}
	})
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

		if m.opts.Backup {
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

func prepareFullDiff(doc markers.Document, opts cli.Options) ([]string, []string, []string, []conflictRange, bool) {
	if opts.AllowMissingBase {
		return nil, nil, nil, nil, false
	}
	if opts.BasePath == "" || opts.LocalPath == "" || opts.RemotePath == "" {
		return nil, nil, nil, nil, false
	}

	baseLines, err := loadLines(opts.BasePath)
	if err != nil {
		return nil, nil, nil, nil, false
	}
	oursLines, err := loadLines(opts.LocalPath)
	if err != nil {
		return nil, nil, nil, nil, false
	}
	theirsLines, err := loadLines(opts.RemotePath)
	if err != nil {
		return nil, nil, nil, nil, false
	}

	ranges, ok := computeConflictRanges(doc, baseLines, oursLines, theirsLines)
	if !ok {
		return nil, nil, nil, nil, false
	}

	return baseLines, oursLines, theirsLines, ranges, true
}

func loadLines(path string) ([]string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return splitLines(bytes), nil
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

	case toastExpiredMsg:
		if msg.id == m.toastSeq {
			m.toastMessage = ""
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			m.err = ErrBackToSelector
			m.quitting = true
			return m, tea.Quit

		case "ctrl+c":
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

		case "H":
			m.scrollHorizontal(-4)

		case "L":
			m.scrollHorizontal(4)

		case "o":
			// Apply ours
			if err := m.state.ApplyResolution(m.currentConflict, markers.ResolutionOurs); err != nil {
				m.err = fmt.Errorf("failed to apply ours: %w", err)
				return m, tea.Quit
			}
			delete(m.manualResolved, m.currentConflict)
			m.doc = m.state.Document()
			m.updateViewports()

		case "t":
			// Apply theirs
			if err := m.state.ApplyResolution(m.currentConflict, markers.ResolutionTheirs); err != nil {
				m.err = fmt.Errorf("failed to apply theirs: %w", err)
				return m, tea.Quit
			}
			delete(m.manualResolved, m.currentConflict)
			m.doc = m.state.Document()
			m.updateViewports()

		case "O":
			// Apply ours to all conflicts
			if err := m.state.ApplyAll(markers.ResolutionOurs); err != nil {
				m.err = fmt.Errorf("failed to apply ours to all: %w", err)
				return m, tea.Quit
			}
			m.manualResolved = map[int][]byte{}
			m.doc = m.state.Document()
			m.updateViewports()

		case "T":
			// Apply theirs to all conflicts
			if err := m.state.ApplyAll(markers.ResolutionTheirs); err != nil {
				m.err = fmt.Errorf("failed to apply theirs to all: %w", err)
				return m, tea.Quit
			}
			m.manualResolved = map[int][]byte{}
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
			delete(m.manualResolved, m.currentConflict)
			m.doc = m.state.Document()
			m.updateViewports()

		case "d":
			// Discard selection
			if err := m.state.ApplyResolution(m.currentConflict, markers.ResolutionNone); err != nil {
				m.err = fmt.Errorf("failed to discard selection: %w", err)
				return m, tea.Quit
			}
			delete(m.manualResolved, m.currentConflict)
			m.doc = m.state.Document()
			m.updateViewports()

		case "b":
			// Apply both
			if err := m.state.ApplyResolution(m.currentConflict, markers.ResolutionBoth); err != nil {
				m.err = fmt.Errorf("failed to apply both: %w", err)
				return m, tea.Quit
			}
			delete(m.manualResolved, m.currentConflict)
			m.doc = m.state.Document()
			m.updateViewports()

		case "x":
			// Apply none
			if err := m.state.ApplyResolution(m.currentConflict, markers.ResolutionNone); err != nil {
				m.err = fmt.Errorf("failed to apply none: %w", err)
				return m, tea.Quit
			}
			delete(m.manualResolved, m.currentConflict)
			m.doc = m.state.Document()
			m.updateViewports()

		case "u":
			// Undo
			if err := m.state.Undo(); err == nil {
				m.doc = m.state.Document()
				m.updateViewports()
			}

		case "w":
			// Write resolved file
			if err := m.writeResolved(); err != nil {
				m.err = fmt.Errorf("failed to write resolved: %w", err)
				return m, tea.Quit
			}
			m.doc = m.state.Document()
			m.updateViewports()
			return m, m.showToast("Saved", 2)

		case "e":
			return m, m.openEditor()
		}

	case tea.WindowSizeMsg:
		if !m.ready {
			m.width = msg.Width
			m.height = msg.Height

			// Calculate pane dimensions
			headerHeight := 2
			footerHeight := 3
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
			footerHeight := 3
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
			if errors.Is(m.err, ErrBackToSelector) {
				return "\n  Returning to selector...\n"
			}
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
	statusStyle := statusUnresolvedStyle
	if _, ok := m.manualResolved[m.currentConflict]; ok {
		statusText = "Resolved (manual)"
		statusStyle = statusResolvedStyle
	} else if seg.Resolution != markers.ResolutionUnset {
		statusText = fmt.Sprintf("Resolved: %s", seg.Resolution)
		statusStyle = statusResolvedStyle
	}

	// Render panes
	oursStyle := oursPaneStyle
	if m.selectedSide == selectedOurs {
		oursStyle = selectedSidePaneStyle
	}
	oursTitle := "OURS"
	if label := formatLabel(seg.OursLabel); label != "" {
		oursTitle = fmt.Sprintf("OURS (%s)", label)
	}
	oursPane := oursStyle.Render(
		titleStyle.Render(oursTitle) + "\n" +
			m.viewportOurs.View(),
	)

	resultStyle := resultUnresolvedPaneStyle
	if allResolved(m.doc, m.manualResolved) {
		resultStyle = resultResolvedPaneStyle
	}
	resultTitle := lipgloss.NewStyle().
		Bold(true).
		Background(lipgloss.Color("62")).
		Padding(0, 2).
		Render("RESULT " + statusStyle.Render("("+statusText+")"))
	resultPane := resultStyle.Render(
		resultTitle + "\n" +
			m.viewportResult.View(),
	)

	theirsStyle := theirsPaneStyle
	if m.selectedSide == selectedTheirs {
		theirsStyle = selectedSidePaneStyle
	}
	theirsTitle := "THEIRS"
	if label := formatLabel(seg.TheirsLabel); label != "" {
		theirsTitle = fmt.Sprintf("THEIRS (%s)", label)
	}
	theirsPane := theirsStyle.Render(
		titleStyle.Render(theirsTitle) + "\n" +
			m.viewportTheirs.View(),
	)

	panes := lipgloss.JoinHorizontal(lipgloss.Top, oursPane, resultPane, theirsPane)

	// Footer
	undoInfo := ""
	if m.state.UndoDepth() > 0 {
		undoInfo = fmt.Sprintf(" | Undo available: %d", m.state.UndoDepth())
	}

	footerText := footerStyle.Width(m.width).Render(
		fmt.Sprintf("n: next | p: prev | j/k: scroll | H/L: scroll | h: ours | l: theirs | a: accept | d: discard | o: ours | t: theirs | O: ours all | T: theirs all | b: both | x: none | u: undo | e: editor | w: write | q: back to selector%s", undoInfo),
	)
	footer := lipgloss.JoinVertical(lipgloss.Left, footerText, m.renderToastLine())

	return lipgloss.JoinVertical(lipgloss.Left, header, panes, footer)
}

func (m model) renderToastLine() string {
	content := ""
	if m.toastMessage != "" {
		content = toastStyle.Render(m.toastMessage)
	}
	return toastLineStyle.Width(m.width).Render(content)
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
	selectedStyles[categoryInsertMarker] = selectedHunkMarkerStyle

	connectorStyles := map[lineCategory]lipgloss.Style{
		categoryDefault:  lineNumberStyle,
		categoryResolved: resultResolvedMarkerStyle,
	}
	for category, style := range highlightStyles {
		connectorStyles[category] = style
	}

	// Update ours pane (full file, highlight conflicts)
	var oursLines []lineInfo
	var oursStart int
	var theirsLines []lineInfo
	var theirsStart int
	useFullDiff := m.useFullDiff
	if useFullDiff && len(m.conflictRanges) != len(m.doc.Conflicts) {
		useFullDiff = false
	}

	if useFullDiff {
		oursEntries := diffEntries(m.baseLines, m.oursLines)
		theirsEntries := diffEntries(m.baseLines, m.theirsLines)
		markConflictedInRanges(&oursEntries, &theirsEntries, m.conflictRanges)
		oursLines, oursStart = buildPaneLinesFromEntries(m.doc, paneOurs, m.currentConflict, m.selectedSide, oursEntries, m.conflictRanges)
		theirsLines, theirsStart = buildPaneLinesFromEntries(m.doc, paneTheirs, m.currentConflict, m.selectedSide, theirsEntries, m.conflictRanges)
	} else {
		oursLines, oursStart = buildPaneLinesFromDoc(m.doc, paneOurs, m.currentConflict, m.selectedSide)
		theirsLines, theirsStart = buildPaneLinesFromDoc(m.doc, paneTheirs, m.currentConflict, m.selectedSide)
	}
	oursContent := renderLines(oursLines, lineNumberStyle, baseStyles, highlightStyles, selectedStyles, connectorStyles, false)
	m.viewportOurs.SetContent(oursContent)
	if m.pendingScroll {
		ensureVisible(&m.viewportOurs, oursStart, len(oursLines))
	}

	// Update theirs pane (full file, highlight conflicts)
	theirsContent := renderLines(theirsLines, lineNumberStyle, baseStyles, highlightStyles, selectedStyles, connectorStyles, false)
	m.viewportTheirs.SetContent(theirsContent)
	if m.pendingScroll {
		ensureVisible(&m.viewportTheirs, theirsStart, len(theirsLines))
	}

	// Update result pane with full resolved preview
	var resultLines []lineInfo
	var resultStart int
	if useFullDiff {
		previewLines, forced, resultRanges := buildResultPreviewLines(m.doc, m.selectedSide, m.manualResolved)
		resultEntries := diffEntries(m.baseLines, previewLines)
		resultLines, resultStart = buildResultLinesFromEntries(resultEntries, resultRanges, m.currentConflict, forced)
	} else {
		resultLines, resultStart = buildResultLines(m.doc, m.currentConflict, m.selectedSide, m.manualResolved)
	}
	resultContent := renderLines(resultLines, lineNumberStyle, baseStyles, highlightStyles, selectedStyles, connectorStyles, true)
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

func (m *model) scrollHorizontal(delta int) {
	apply := func(viewportModel *viewport.Model) {
		if delta < 0 {
			viewportModel.ScrollLeft(-delta)
			return
		}
		if delta > 0 {
			viewportModel.ScrollRight(delta)
		}
	}
	apply(&m.viewportOurs)
	apply(&m.viewportResult)
	apply(&m.viewportTheirs)
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

	// Write backup if enabled
	if m.opts.Backup {
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

func allResolved(doc markers.Document, manualResolved map[int][]byte) bool {
	for idx, ref := range doc.Conflicts {
		if _, ok := manualResolved[idx]; ok {
			continue
		}
		seg, ok := doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
		if !ok {
			return false
		}
		if seg.Resolution == markers.ResolutionUnset {
			return false
		}
	}
	return true
}

func formatLabel(label string) string {
	_ = label
	return ""
}

func firstHexRun(label string) (int, int) {
	start := -1
	for i, r := range label {
		if isHexRune(r) {
			start = i
			break
		}
	}
	if start == -1 {
		return -1, -1
	}
	end := start
	count := 0
	for i := start; i < len(label); i++ {
		if !isHexByte(label[i]) {
			break
		}
		end = i + 1
		count++
	}
	if count < 7 {
		return -1, -1
	}
	return start, end
}

func isHexRune(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isHexByte(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func applyMergedResolutions(doc markers.Document, mergedBytes []byte) (markers.Document, map[int][]byte, error) {
	mergedLines := splitLinesKeepEOL(mergedBytes)
	pos := 0
	manualResolved := map[int][]byte{}

	conflictIndex := -1
	for i, seg := range doc.Segments {
		switch s := seg.(type) {
		case markers.TextSegment:
			textLines := splitLinesKeepEOL(s.Bytes)
			if len(textLines) == 0 {
				continue
			}
			idx := findSubslice(mergedLines, pos, textLines)
			if idx == -1 {
				return doc, manualResolved, fmt.Errorf("failed to align text segment")
			}
			pos = idx + len(textLines)

		case markers.ConflictSegment:
			conflictIndex++
			nextTextLines := nextTextSegmentLines(doc.Segments, i+1)
			nextIdx := -1
			if len(nextTextLines) > 0 {
				nextIdx = findSubslice(mergedLines, pos, nextTextLines)
			}
			if nextIdx == -1 {
				nextIdx = len(mergedLines)
			}
			if nextIdx < pos {
				return doc, manualResolved, fmt.Errorf("failed to align conflict segment")
			}
			spanLines := mergedLines[pos:nextIdx]
			if containsConflictMarkers(spanLines) {
				pos = nextIdx
				continue
			}
			resolution, matched := matchResolution(spanLines, s)
			if matched {
				s.Resolution = resolution
				doc.Segments[i] = s
				pos = nextIdx
				continue
			}
			manualResolved[conflictIndex] = joinLines(spanLines)
			pos = nextIdx
		}
	}

	return doc, manualResolved, nil
}

func nextTextSegmentLines(segments []markers.Segment, start int) [][]byte {
	for i := start; i < len(segments); i++ {
		if text, ok := segments[i].(markers.TextSegment); ok {
			lines := splitLinesKeepEOL(text.Bytes)
			if len(lines) > 0 {
				return lines
			}
		}
	}
	return nil
}

func matchResolution(lines [][]byte, seg markers.ConflictSegment) (markers.Resolution, bool) {
	ours := splitLinesKeepEOL(seg.Ours)
	theirs := splitLinesKeepEOL(seg.Theirs)
	both := append(append([][]byte{}, ours...), theirs...)

	if linesEqual(lines, ours) {
		return markers.ResolutionOurs, true
	}
	if linesEqual(lines, theirs) {
		return markers.ResolutionTheirs, true
	}
	if linesEqual(lines, both) {
		return markers.ResolutionBoth, true
	}
	if len(lines) == 0 {
		return markers.ResolutionNone, true
	}
	return markers.ResolutionUnset, false
}

func containsConflictMarkers(lines [][]byte) bool {
	for _, line := range lines {
		if bytes.HasPrefix(line, []byte("<<<<<<<")) ||
			bytes.HasPrefix(line, []byte("|||||||")) ||
			bytes.HasPrefix(line, []byte("=======")) ||
			bytes.HasPrefix(line, []byte(">>>>>>>")) {
			return true
		}
	}
	return false
}

func splitLinesKeepEOL(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}

	var out [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			out = append(out, data[start:i+1])
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}

func findSubslice(haystack [][]byte, start int, needle [][]byte) int {
	if len(needle) == 0 {
		return start
	}
	if start < 0 {
		start = 0
	}
	for i := start; i+len(needle) <= len(haystack); i++ {
		matched := true
		for j := range needle {
			if !bytesEqual(haystack[i+j], needle[j]) {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func linesEqual(left [][]byte, right [][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !bytesEqual(left[i], right[i]) {
			return false
		}
	}
	return true
}

func bytesEqual(left []byte, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func joinLines(lines [][]byte) []byte {
	if len(lines) == 0 {
		return nil
	}
	var b bytes.Buffer
	for _, line := range lines {
		b.Write(line)
	}
	return b.Bytes()
}
