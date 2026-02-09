package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/engine"
	"github.com/chojs23/ec/internal/gitmerge"
	"github.com/chojs23/ec/internal/markers"
)

const (
	maxUndoSize           = 100
	keySeqTimeoutDuration = 350 * time.Millisecond
	keyQuit               = "q"
	keyCtrlC              = "ctrl+c"
	keyCtrlS              = "ctrl+s"
	keyNextConflict       = "n"
	keyPrevConflict       = "p"
	keySelectOurs         = "h"
	keySelectTheirs       = "l"
	keyScrollLeft         = "H"
	keyScrollRight        = "L"
	keyScrollDown         = "j"
	keyScrollUp           = "k"
	keyArrowLeft          = "left"
	keyArrowRight         = "right"
	keyArrowDown          = "down"
	keyArrowUp            = "up"
	keyGoTop              = "g"
	keyGoBottom           = "G"
	keyApplyOurs          = "o"
	keyApplyTheirs        = "t"
	keyApplyOursAll       = "O"
	keyApplyTheirsAll     = "T"
	keyAccept             = "a"
	keyAcceptSpace        = " "
	keyDiscard            = "d"
	keyApplyBoth          = "b"
	keyApplyNone          = "x"
	keyUndo               = "u"
	keyRedo               = "ctrl+r"
	keyWrite              = "w"
	keyEdit               = "e"
)

type keyHelpEntry struct {
	key         string
	description string
}

type keyAction func(*model) (tea.Cmd, error)

var resolverKeyHelp = []keyHelpEntry{
	{key: "n", description: "next"},
	{key: "p", description: "prev"},
	{key: "gg/G", description: "top/bottom"},
	{key: "j/k/up/down", description: "scroll"},
	{key: "H/L/left/right", description: "scroll"},
	{key: "h", description: "ours"},
	{key: "l", description: "theirs"},
	{key: "a/<space>", description: "accept"},
	{key: "o/O", description: "ours/ours all"},
	{key: "t/T", description: "theirs/theirs all"},
	{key: "b", description: "both"},
	{key: "x", description: "none"},
	{key: "d", description: "discard"},
	{key: "u", description: "undo"},
	{key: "ctrl+r", description: "redo"},
	{key: "e", description: "editor"},
	{key: "w/ctrl+s", description: "write"},
	{key: "q", description: "back to selector"},
}

var resolverKeyActions = map[string]keyAction{
	keyQuit:           (*model).handleQuit,
	keyCtrlC:          (*model).handleCtrlC,
	keyNextConflict:   (*model).handleNextConflict,
	keyPrevConflict:   (*model).handlePrevConflict,
	keySelectOurs:     (*model).handleSelectOurs,
	keySelectTheirs:   (*model).handleSelectTheirs,
	keyScrollLeft:     (*model).handleScrollLeft,
	keyScrollRight:    (*model).handleScrollRight,
	keyScrollDown:     (*model).handleScrollDown,
	keyScrollUp:       (*model).handleScrollUp,
	keyArrowLeft:      (*model).handleScrollLeft,
	keyArrowRight:     (*model).handleScrollRight,
	keyArrowDown:      (*model).handleScrollDown,
	keyArrowUp:        (*model).handleScrollUp,
	keyApplyOurs:      (*model).handleApplyOurs,
	keyApplyTheirs:    (*model).handleApplyTheirs,
	keyApplyOursAll:   (*model).handleApplyOursAll,
	keyApplyTheirsAll: (*model).handleApplyTheirsAll,
	keyAccept:         (*model).handleAccept,
	keyAcceptSpace:    (*model).handleAccept,
	keyDiscard:        (*model).handleDiscard,
	keyApplyBoth:      (*model).handleApplyBoth,
	keyApplyNone:      (*model).handleApplyNone,
	keyUndo:           (*model).handleUndo,
	keyRedo:           (*model).handleRedo,
	keyWrite:          (*model).handleWrite,
	keyCtrlS:          (*model).handleWrite,
	keyEdit:           (*model).handleEdit,
}

var (
	titleStyle                lipgloss.Style
	paneStyle                 lipgloss.Style
	selectedPaneStyle         lipgloss.Style
	oursPaneStyle             lipgloss.Style
	theirsPaneStyle           lipgloss.Style
	selectedSidePaneStyle     lipgloss.Style
	headerStyle               lipgloss.Style
	footerStyle               lipgloss.Style
	lineNumberStyle           lipgloss.Style
	oursHighlightStyle        lipgloss.Style
	theirsHighlightStyle      lipgloss.Style
	resultLineStyle           lipgloss.Style
	resultHighlightStyle      lipgloss.Style
	modifiedLineStyle         lipgloss.Style
	addedLineStyle            lipgloss.Style
	removedLineStyle          lipgloss.Style
	conflictedLineStyle       lipgloss.Style
	insertMarkerStyle         lipgloss.Style
	selectedHunkMarkerStyle   lipgloss.Style
	selectedHunkBackground    lipgloss.Color
	statusResolvedStyle       lipgloss.Style
	statusUnresolvedStyle     lipgloss.Style
	resultResolvedMarkerStyle lipgloss.Style
	resultResolvedPaneStyle   lipgloss.Style
	resultUnresolvedPaneStyle lipgloss.Style
	toastStyle                lipgloss.Style
	toastLineStyle            lipgloss.Style
	resultTitleStyle          lipgloss.Style

	dimForegroundLight lipgloss.Color
	dimForegroundDark  lipgloss.Color
	dimForegroundMuted lipgloss.Color
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
	keySeq          string
	keySeqTimeout   int
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
	if err := ensureThemeLoaded(); err != nil {
		return err
	}
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

type keySeqExpiredMsg struct {
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
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	if editor == "true" {
		return func() tea.Msg {
			return editorFinishedMsg{err: nil}
		}
	}

	mergedBytes, err := os.ReadFile(m.opts.MergedPath)
	if err != nil {
		return func() tea.Msg {
			return editorFinishedMsg{err: fmt.Errorf("read merged for backup: %w", err)}
		}
	}

	resolved, err := m.state.Preview()
	if err != nil {
		if errors.Is(err, markers.ErrUnresolved) {
			resolved, err = markers.RenderWithUnresolved(m.state.Document())
			if err != nil {
				return func() tea.Msg {
					return editorFinishedMsg{err: fmt.Errorf("render unresolved preview for editor: %w", err)}
				}
			}
		} else {
			return func() tea.Msg {
				return editorFinishedMsg{err: fmt.Errorf("cannot generate preview for editor: %w", err)}
			}
		}
	}

	if m.opts.Backup {
		bak := m.opts.MergedPath + ".ec.bak"
		if err := os.WriteFile(bak, mergedBytes, 0o644); err != nil {
			return func() tea.Msg {
				return editorFinishedMsg{err: fmt.Errorf("write backup %s: %w", filepath.Base(bak), err)}
			}
		}
	}

	if !bytes.Equal(resolved, mergedBytes) {
		if err := os.WriteFile(m.opts.MergedPath, resolved, 0o644); err != nil {
			return func() tea.Msg {
				return editorFinishedMsg{err: fmt.Errorf("write merged before editor: %w", err)}
			}
		}
	}

	cmd := exec.Command(editor, m.opts.MergedPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return editorFinishedMsg{err: fmt.Errorf("editor failed: %w", err)}
		}
		return editorFinishedMsg{err: nil}
	})
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

	updated, manual, err := applyMergedResolutions(doc, editedBytes)
	if err != nil {
		return fmt.Errorf("apply merged resolutions: %w", err)
	}

	state, err := engine.NewState(updated, maxUndoSize)
	if err != nil {
		return fmt.Errorf("create new state: %w", err)
	}

	m.state = state
	m.doc = state.Document()
	m.manualResolved = manual

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

	case keySeqExpiredMsg:
		if msg.id == m.keySeqTimeout {
			m.keySeq = ""
		}
		return m, nil

	case tea.KeyMsg:
		key := msg.String()
		if key == keyGoTop {
			if m.keySeq == keyGoTop {
				m.keySeq = ""
				m.scrollToTop()
				return m, nil
			}
			m.keySeq = keyGoTop
			m.keySeqTimeout++
			id := m.keySeqTimeout
			return m, tea.Tick(keySeqTimeoutDuration, func(time.Time) tea.Msg {
				return keySeqExpiredMsg{id: id}
			})
		}
		if key == keyGoBottom {
			m.keySeq = ""
			m.scrollToBottom()
			return m, nil
		}
		if m.keySeq != "" {
			m.keySeq = ""
		}
		if action, ok := resolverKeyActions[key]; ok {
			actionCmd, err := action(&m)
			if err != nil {
				m.err = err
				m.quitting = true
				return m, tea.Quit
			}
			if actionCmd != nil {
				return m, actionCmd
			}
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

	if _, ok := msg.(tea.KeyMsg); ok {
		return m, tea.Batch(cmds...)
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
	resultTitle := resultTitleStyle.Render("RESULT " + statusStyle.Render("("+statusText+")"))
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
	redoInfo := ""
	if m.state.RedoDepth() > 0 {
		redoInfo = fmt.Sprintf(" | Redo available: %d", m.state.RedoDepth())
	}

	footerText := footerStyle.Width(m.width).Render(
		fmt.Sprintf("%s%s%s", resolverFooterKeyMapText(), undoInfo, redoInfo),
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

func resolverFooterKeyMapText() string {
	parts := make([]string, 0, len(resolverKeyHelp))
	for _, entry := range resolverKeyHelp {
		parts = append(parts, fmt.Sprintf("%s: %s", entry.key, entry.description))
	}
	return strings.Join(parts, " | ")
}

func (m *model) applySelectedSide() error {
	resolution := markers.ResolutionOurs
	if m.selectedSide == selectedTheirs {
		resolution = markers.ResolutionTheirs
	}
	if err := m.state.ApplyResolution(m.currentConflict, resolution); err != nil {
		return err
	}
	delete(m.manualResolved, m.currentConflict)
	m.doc = m.state.Document()
	m.updateViewports()
	return nil
}

func (m *model) applyResolution(resolution markers.Resolution) error {
	if err := m.state.ApplyResolution(m.currentConflict, resolution); err != nil {
		return err
	}
	delete(m.manualResolved, m.currentConflict)
	m.doc = m.state.Document()
	m.updateViewports()
	return nil
}

func (m *model) applyAll(resolution markers.Resolution) error {
	if err := m.state.ApplyAll(resolution); err != nil {
		return err
	}
	m.manualResolved = map[int][]byte{}
	m.doc = m.state.Document()
	m.updateViewports()
	return nil
}

func (m *model) handleQuit() (tea.Cmd, error) {
	m.err = ErrBackToSelector
	m.quitting = true
	return tea.Quit, nil
}

func (m *model) handleCtrlC() (tea.Cmd, error) {
	m.quitting = true
	return tea.Quit, nil
}

func (m *model) handleNextConflict() (tea.Cmd, error) {
	if m.currentConflict < len(m.doc.Conflicts)-1 {
		m.currentConflict++
		m.pendingScroll = true
		m.updateViewports()
	}
	return nil, nil
}

func (m *model) handlePrevConflict() (tea.Cmd, error) {
	if m.currentConflict > 0 {
		m.currentConflict--
		m.pendingScroll = true
		m.updateViewports()
	}
	return nil, nil
}

func (m *model) handleSelectOurs() (tea.Cmd, error) {
	m.selectedSide = selectedOurs
	m.updateViewports()
	return nil, nil
}

func (m *model) handleSelectTheirs() (tea.Cmd, error) {
	m.selectedSide = selectedTheirs
	m.updateViewports()
	return nil, nil
}

func (m *model) handleScrollLeft() (tea.Cmd, error) {
	m.scrollHorizontal(-4)
	return nil, nil
}

func (m *model) handleScrollRight() (tea.Cmd, error) {
	m.scrollHorizontal(4)
	return nil, nil
}

func (m *model) handleScrollDown() (tea.Cmd, error) {
	m.scrollVertical(1)
	return nil, nil
}

func (m *model) handleScrollUp() (tea.Cmd, error) {
	m.scrollVertical(-1)
	return nil, nil
}

func (m *model) handleApplyOurs() (tea.Cmd, error) {
	if err := m.applyResolution(markers.ResolutionOurs); err != nil {
		return nil, fmt.Errorf("failed to apply ours: %w", err)
	}
	return nil, nil
}

func (m *model) handleApplyTheirs() (tea.Cmd, error) {
	if err := m.applyResolution(markers.ResolutionTheirs); err != nil {
		return nil, fmt.Errorf("failed to apply theirs: %w", err)
	}
	return nil, nil
}

func (m *model) handleApplyOursAll() (tea.Cmd, error) {
	if err := m.applyAll(markers.ResolutionOurs); err != nil {
		return nil, fmt.Errorf("failed to apply ours to all: %w", err)
	}
	return nil, nil
}

func (m *model) handleApplyTheirsAll() (tea.Cmd, error) {
	if err := m.applyAll(markers.ResolutionTheirs); err != nil {
		return nil, fmt.Errorf("failed to apply theirs to all: %w", err)
	}
	return nil, nil
}

func (m *model) handleAccept() (tea.Cmd, error) {
	if err := m.applySelectedSide(); err != nil {
		return nil, fmt.Errorf("failed to apply selection: %w", err)
	}
	return nil, nil
}

func (m *model) handleDiscard() (tea.Cmd, error) {
	if err := m.applyResolution(markers.ResolutionNone); err != nil {
		return nil, fmt.Errorf("failed to discard selection: %w", err)
	}
	return nil, nil
}

func (m *model) handleApplyBoth() (tea.Cmd, error) {
	if err := m.applyResolution(markers.ResolutionBoth); err != nil {
		return nil, fmt.Errorf("failed to apply both: %w", err)
	}
	return nil, nil
}

func (m *model) handleApplyNone() (tea.Cmd, error) {
	if err := m.applyResolution(markers.ResolutionNone); err != nil {
		return nil, fmt.Errorf("failed to apply none: %w", err)
	}
	return nil, nil
}

func (m *model) handleUndo() (tea.Cmd, error) {
	if err := m.state.Undo(); err == nil {
		m.doc = m.state.Document()
		m.updateViewports()
	}
	return nil, nil
}

func (m *model) handleRedo() (tea.Cmd, error) {
	if err := m.state.Redo(); err == nil {
		m.doc = m.state.Document()
		m.updateViewports()
	}
	return nil, nil
}

func (m *model) handleWrite() (tea.Cmd, error) {
	if err := m.writeResolved(); err != nil {
		return nil, fmt.Errorf("failed to write resolved: %w", err)
	}
	m.doc = m.state.Document()
	m.updateViewports()
	return m.showToast("Saved", 2), nil
}

func (m *model) handleEdit() (tea.Cmd, error) {
	return m.openEditor(), nil
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

	centerOffset := start - (viewportModel.Height / 2)
	if centerOffset < 0 {
		centerOffset = 0
	}
	if centerOffset > maxOffset {
		centerOffset = maxOffset
	}
	viewportModel.YOffset = centerOffset
}

func (m *model) scrollToTop() {
	m.viewportOurs.GotoTop()
	m.viewportResult.GotoTop()
	m.viewportTheirs.GotoTop()
}

func (m *model) scrollToBottom() {
	m.viewportOurs.GotoBottom()
	m.viewportResult.GotoBottom()
	m.viewportTheirs.GotoBottom()
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

func (m *model) scrollVertical(delta int) {
	apply := func(viewportModel *viewport.Model) {
		if delta < 0 {
			viewportModel.ScrollUp(-delta)
			return
		}
		if delta > 0 {
			viewportModel.ScrollDown(delta)
		}
	}
	apply(&m.viewportOurs)
	apply(&m.viewportResult)
	apply(&m.viewportTheirs)
}

func (m *model) writeResolved() error {
	// Generate preview
	resolved, err := m.state.Preview()
	allowUnresolved := false
	if err != nil {
		if errors.Is(err, markers.ErrUnresolved) {
			allowUnresolved = true
			resolved, err = markers.RenderWithUnresolved(m.state.Document())
			if err != nil {
				return fmt.Errorf("render unresolved output: %w", err)
			}
		} else {
			return fmt.Errorf("cannot write: %w", err)
		}
	}

	// Read original merged file for backup
	mergedBytes, err := os.ReadFile(m.opts.MergedPath)
	if err != nil {
		return fmt.Errorf("read merged for backup: %w", err)
	}

	// Write backup if enabled
	if m.opts.Backup {
		bak := m.opts.MergedPath + ".ec.bak"
		if err := os.WriteFile(bak, mergedBytes, 0o644); err != nil {
			return fmt.Errorf("write backup %s: %w", filepath.Base(bak), err)
		}
	}

	// Write resolved file
	if err := os.WriteFile(m.opts.MergedPath, resolved, 0o644); err != nil {
		return fmt.Errorf("write merged: %w", err)
	}

	// Verify no conflict markers remain
	if !allowUnresolved {
		postDoc, err := markers.Parse(resolved)
		if err != nil {
			return fmt.Errorf("post-parse merged: %w", err)
		}
		if len(postDoc.Conflicts) != 0 {
			return fmt.Errorf("resolution output still contains conflict markers")
		}
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
