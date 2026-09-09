package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/gitutil"
)

// WorkspaceData is a repository snapshot. Preparation stays with the caller so
// the TUI does not own repository discovery or temporary Git stage creation.
type WorkspaceData struct {
	RepoRoot        string
	Scope           string
	Conflicts       []FileCandidate
	DiffSources     []gitutil.DiffSource
	Warning         string
	PrepareConflict func(context.Context, string) (PreparedConflict, error)
}

// PreparedConflict transfers temporary-stage ownership to the workspace.
type PreparedConflict struct {
	Options cli.Options
	Cleanup func()
	Warning string
}

type workspaceLoadedMsg struct {
	visitID uint64
	data    WorkspaceData
	err     error
}

type workspaceOpenedMsg struct {
	visitID uint64
	child   tea.Model
	warning string
	err     error
}

type workspaceModel struct {
	tasks       *workspaceTasks
	load        func(context.Context) (WorkspaceData, error)
	data        WorkspaceData
	selector    workspaceSelectModel
	child       tea.Model
	visitID     uint64
	visitCtx    context.Context
	cancelVisit context.CancelFunc
	size        tea.WindowSizeMsg
	loaded      bool
	refreshing  bool
	opening     bool
	quitting    bool
	err         error
}

// RunWorkspace owns one alternate screen for selector, diff, and resolver.
// Returning to the selector changes models rather than restarting the terminal.
func RunWorkspace(ctx context.Context, load func(context.Context) (WorkspaceData, error)) error {
	if err := ensureThemeLoaded(); err != nil {
		return err
	}
	m := newWorkspaceModel(ctx, load)
	defer m.tasks.close()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	final, err := p.Run()
	if err != nil {
		return fmt.Errorf("workspace TUI error: %w", err)
	}
	return final.(workspaceModel).err
}

func newWorkspaceModel(ctx context.Context, load func(context.Context) (WorkspaceData, error)) workspaceModel {
	m := workspaceModel{
		tasks: newWorkspaceTasks(ctx), load: load,
		selector:   newWorkspaceSelectModel(nil, nil),
		size:       tea.WindowSizeMsg{Width: 80, Height: 24},
		refreshing: true,
	}
	m.nextVisit()
	m.selector.notice = "Loading workspace... | q: quit"
	return m
}

func (m *workspaceModel) nextVisit() {
	if m.cancelVisit != nil {
		m.cancelVisit()
		m.tasks.release(m.visitID)
	}
	m.visitID++
	m.visitCtx, m.cancelVisit = context.WithCancel(m.tasks.ctx)
}

func (m workspaceModel) Init() tea.Cmd {
	return m.refresh()
}

func (m workspaceModel) refresh() tea.Cmd {
	return m.tasks.command(m.visitCtx, func() tea.Msg {
		data, err := m.load(m.visitCtx)
		return workspaceLoadedMsg{visitID: m.visitID, data: data, err: err}
	})
}

func (m workspaceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quitting {
		return m, nil
	}
	switch msg := msg.(type) {
	case workspaceLoadedMsg:
		if msg.visitID != m.visitID {
			return m, nil
		}
		m.refreshing = false
		if msg.err != nil {
			if !m.loaded {
				m.err, m.quitting = msg.err, true
				return m, tea.Quit
			}
			m.selector.notice = "Refresh failed: " + msg.err.Error()
			return m, nil
		}
		previous, selected := m.selector.selectedItem()
		next := newWorkspaceSelectModel(msg.data.Conflicts, msg.data.DiffSources)
		if m.loaded {
			next.selected = min(m.selector.selected, max(len(next.items)-1, 0))
			next.offset = m.selector.offset
			for i, item := range next.items {
				if selected && sameWorkspaceItem(previous, item) {
					next.selected = i
					break
				}
			}
		}
		next.notice = msg.data.Warning
		updated, _ := next.Update(m.size)
		m.selector = updated.(workspaceSelectModel)
		m.data, m.loaded = msg.data, true
		return m, nil

	case workspaceOpenedMsg:
		if msg.visitID != m.visitID {
			m.tasks.release(msg.visitID)
			return m, nil
		}
		m.opening = false
		if msg.err != nil {
			m.selector.notice = "Cannot open: " + msg.err.Error() + " | enter: retry | q: quit"
			return m, nil
		}
		m.child = msg.child
		var notice tea.Cmd
		if resolver, ok := m.child.(model); ok && msg.warning != "" {
			notice = resolver.showToast(sanitizeTerminalText(msg.warning), 5)
			m.child = resolver
		}
		child, resize := m.child.Update(m.size)
		m.child = child
		mouse := tea.DisableMouse
		if _, ok := child.(diffModel); ok {
			mouse = tea.EnableMouseCellMotion
		}
		return m, tea.Batch(resize, m.child.Init(), notice, mouse)

	case tea.WindowSizeMsg:
		m.size = msg
		updated, _ := m.selector.Update(msg)
		m.selector = updated.(workspaceSelectModel)

	case tea.KeyMsg:
		key := msg.String()
		if key == keyCtrlC || (key == keyQuit && m.child == nil && !m.opening) {
			m.quitting = true
			m.cancelVisit()
			return m, tea.Quit
		}
		if key == keyQuit {
			m.nextVisit()
			m.child, m.opening = nil, false
			m.refreshing = true
			m.selector.notice = "Refreshing workspace... | q: quit"
			return m, tea.Batch(tea.DisableMouse, m.refresh())
		}
		if m.opening || !m.loaded {
			return m, nil
		}
		if m.child == nil && key == "enter" {
			// Keep navigation responsive, but do not cancel the refresh and open
			// a stale conflict-stage snapshot after returning from the resolver.
			if m.refreshing {
				return m, nil
			}
			if item, ok := m.selector.selectedItem(); ok {
				m.nextVisit()
				m.opening = true
				m.selector.notice = "Opening... | q: back"
				return m, m.open(selectionFromItem(item))
			}
			return m, nil
		}
	}

	if m.child != nil {
		child, cmd := m.child.Update(msg)
		m.child = child
		if resolver, ok := child.(model); ok && resolver.quitting {
			m.err, m.quitting = resolver.err, true
		}
		// Forward framework commands unchanged, especially ExecProcess which
		// intentionally releases the terminal while an external editor runs.
		return m, cmd
	}
	if _, ok := msg.(tea.KeyMsg); ok {
		updated, cmd := m.selector.Update(msg)
		m.selector = updated.(workspaceSelectModel)
		return m, cmd
	}
	return m, nil
}

func (m workspaceModel) open(selection WorkspaceSelection) tea.Cmd {
	return m.tasks.command(m.visitCtx, func() tea.Msg {
		result := workspaceOpenedMsg{visitID: m.visitID}
		switch selection.Kind {
		case WorkspaceSelectionDiff:
			files, err := gitutil.ListDiffFiles(m.visitCtx, m.data.RepoRoot, selection.DiffSource, m.data.Scope)
			if err != nil {
				result.err = err
				return result
			}
			child := newDiffModel(m.visitCtx, m.data.RepoRoot, selection.DiffSource, files)
			child.visitID = m.visitID
			result.child = child
		case WorkspaceSelectionConflict:
			if m.data.PrepareConflict == nil {
				result.err = fmt.Errorf("conflict preparation is unavailable")
				return result
			}
			prepared, err := m.data.PrepareConflict(m.visitCtx, selection.ConflictPath)
			cleanup := prepared.Cleanup
			// A pending loader owns its stages until construction succeeds. This
			// also covers cancellation before its message reaches the root.
			defer func() {
				if cleanup != nil {
					cleanup()
				}
			}()
			if err == nil {
				err = m.visitCtx.Err()
			}
			if err != nil {
				result.err = err
				return result
			}
			child, err := newResolverModel(m.visitCtx, prepared.Options)
			if err != nil {
				result.err = err
				return result
			}
			if !m.tasks.adopt(m.visitCtx, m.visitID, cleanup) {
				return nil
			}
			cleanup = nil
			child.visitID = m.visitID
			result.child, result.warning = child, prepared.Warning
		default:
			result.err = fmt.Errorf("unknown workspace action")
		}
		return result
	})
}

func sameWorkspaceItem(a, b selectorItem) bool {
	return a.kind == b.kind && a.path == b.path && a.diffSource.Kind == b.diffSource.Kind && a.diffSource.Commit == b.diffSource.Commit
}

func (m workspaceModel) View() string {
	if m.quitting {
		return ""
	}
	if m.child != nil {
		return m.child.View()
	}
	return m.selector.View()
}
