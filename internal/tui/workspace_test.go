package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/gitutil"
)

func updateWorkspace(m workspaceModel, msg tea.Msg) (workspaceModel, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(workspaceModel), cmd
}

func workspaceTestData(t *testing.T) WorkspaceData {
	t.Helper()
	dir := t.TempDir()
	merged := filepath.Join(dir, "conflict.txt")
	text := "start\n<<<<<<< HEAD\nours\n||||||| base\nbase\n=======\ntheirs\n>>>>>>> branch\nend\n"
	if err := os.WriteFile(merged, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	stages := t.TempDir()
	for _, name := range []string{"base", "ours", "theirs"} {
		if err := os.WriteFile(filepath.Join(stages, name), []byte("start\n"+name+"\nend\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	opts := cli.Options{
		BasePath: filepath.Join(stages, "base"), LocalPath: filepath.Join(stages, "ours"),
		RemotePath: filepath.Join(stages, "theirs"), MergedPath: merged,
	}
	return WorkspaceData{
		RepoRoot: dir, Scope: ".",
		Conflicts:   []FileCandidate{{Path: "conflict.txt"}},
		DiffSources: []gitutil.DiffSource{gitutil.WorkingTreeSource()},
		PrepareConflict: func(context.Context, string) (PreparedConflict, error) {
			return PreparedConflict{Options: opts}, nil
		},
	}
}

func TestWorkspaceOpensExistingScreensAndRetainsRefreshedSelection(t *testing.T) {
	data := workspaceTestData(t)
	runGitCmd(t, data.RepoRoot, "init")
	m := newWorkspaceModel(context.Background(), func(context.Context) (WorkspaceData, error) { return data, nil })
	defer m.tasks.close()
	m, _ = updateWorkspace(m, m.Init()())
	m, _ = updateWorkspace(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	for _, selection := range []int{0, 1} {
		m.selector.selected = selection
		var cmd tea.Cmd
		m, cmd = updateWorkspace(m, tea.KeyMsg{Type: tea.KeyEnter})
		m, cmd = updateWorkspace(m, cmd())
		if m.err != nil || m.child == nil || m.quitting {
			t.Fatalf("open failed: %s", m.selector.notice)
		}
		switch child := m.child.(type) {
		case model:
			if !child.ready || child.width != 120 || child.height != 30 || child.visitID != m.visitID {
				t.Fatal("resolver must receive size and visit identity")
			}
		case diffModel:
			if !child.ready || child.width != 120 || child.height != 30 || len(child.files) == 0 || child.visitID != m.visitID {
				t.Fatal("diff must receive files, size, and visit identity")
			}
		default:
			t.Fatalf("unexpected child %T", child)
		}
		assertNavigationCommand(t, cmd)
		m, _ = updateWorkspace(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		data.Conflicts[0].Resolved = true
		m, _ = updateWorkspace(m, m.refresh()())
		if m.selector.selected != selection || !m.selector.items[0].resolved {
			t.Fatal("refresh must update saved status without moving the selection")
		}
	}
	data.Conflicts = nil
	m, _ = updateWorkspace(m, m.refresh()())
	if item, ok := m.selector.selectedItem(); !ok || item.kind != selectorDiff || m.selector.selected != 0 {
		t.Fatal("selection must follow its target when a preceding item disappears")
	}
}

func TestWorkspaceIgnoresMessagesFromDepartedScreens(t *testing.T) {
	data := workspaceTestData(t)
	m := newWorkspaceModel(context.Background(), func(context.Context) (WorkspaceData, error) { return data, nil })
	defer m.tasks.close()
	m, _ = updateWorkspace(m, m.Init()())
	oldID := m.visitID
	m.nextVisit()
	for _, msg := range []tea.Msg{
		workspaceLoadedMsg{visitID: oldID, err: errors.New("old load error")},
		workspaceOpenedMsg{visitID: oldID, child: model{}},
	} {
		m, _ = updateWorkspace(m, msg)
		if m.child != nil || m.quitting || m.err != nil {
			t.Fatal("stale load changed the active screen")
		}
	}
	resolver := newModelForDoc(t, parseSingleConflictDoc(t))
	resolver.visitID, resolver.keySeq, resolver.keySeqTimeout = m.visitID, "g", 1
	resolver.toastMessage, resolver.toastSeq = "Saved", 1
	m.child = resolver
	for _, msg := range []tea.Msg{
		keySeqExpiredMsg{visitID: oldID, id: 1},
		toastExpiredMsg{visitID: oldID, id: 1},
		editorFinishedMsg{visitID: oldID, err: errors.New("old editor error")},
	} {
		m, _ = updateWorkspace(m, msg)
	}
	got := m.child.(model)
	if got.keySeq != "g" || got.toastMessage != "Saved" || got.quitting || m.quitting {
		t.Fatal("stale timer or editor result changed the resolver")
	}
	diff := newDiffModel(m.visitCtx, data.RepoRoot, gitutil.WorkingTreeSource(), []gitutil.DiffFile{{Path: "same.txt"}})
	diff.visitID, diff.keySeq, diff.keySeqTimeout = m.visitID, "g", 1
	m.child = diff
	m, _ = updateWorkspace(m, diffLoadedMsg{visitID: oldID, key: diff.selectedFileKey(), patch: []byte("old patch")})
	m, _ = updateWorkspace(m, keySeqExpiredMsg{visitID: oldID, id: 1})
	if got := m.child.(diffModel); got.patchText != "Loading diff..." || got.keySeq != "g" {
		t.Fatal("stale patch or timer changed a new visit to the same file")
	}
}

func TestWorkspaceReleasesPreparedResources(t *testing.T) {
	for _, scenario := range []string{"back", "quit", "late result", "prepare error", "model error", "cancel pending", "quit pending"} {
		t.Run(scenario, func(t *testing.T) {
			data := workspaceTestData(t)
			prepare := data.PrepareConflict
			started, finish := make(chan struct{}), make(chan struct{})
			var cleaned atomic.Int32
			resource := filepath.Join(t.TempDir(), "stage")
			data.PrepareConflict = func(ctx context.Context, path string) (PreparedConflict, error) {
				prepared, err := prepare(ctx, path)
				if err != nil {
					return prepared, err
				}
				if err := os.WriteFile(resource, []byte("stage"), 0o600); err != nil {
					return prepared, err
				}
				prepared.Cleanup = func() { cleaned.Add(1); os.Remove(resource) }
				if strings.HasSuffix(scenario, "pending") {
					close(started)
					<-finish
				}
				if scenario == "prepare error" {
					return prepared, errors.New("prepare failed")
				}
				if scenario == "model error" {
					prepared.Options.BasePath = ""
				}
				return prepared, nil
			}
			m := newWorkspaceModel(context.Background(), func(context.Context) (WorkspaceData, error) { return data, nil })
			defer m.tasks.close()
			m, _ = updateWorkspace(m, m.Init()())
			m, cmd := updateWorkspace(m, tea.KeyMsg{Type: tea.KeyEnter})
			key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
			if strings.HasPrefix(scenario, "quit") {
				key = tea.KeyMsg{Type: tea.KeyCtrlC}
			}
			if strings.HasSuffix(scenario, "pending") {
				result := make(chan tea.Msg, 1)
				go func() { result <- cmd() }()
				select {
				case <-started:
				case <-time.After(5 * time.Second):
					t.Fatal("preparation did not start")
				}
				m, _ = updateWorkspace(m, key)
				close(finish)
				m.tasks.close()
				m, _ = updateWorkspace(m, <-result)
			} else {
				msg := cmd()
				if scenario == "late result" {
					m, _ = updateWorkspace(m, key)
				}
				m, _ = updateWorkspace(m, msg)
				if strings.HasSuffix(scenario, "error") {
					if m.child != nil || m.quitting || !strings.Contains(m.selector.notice, "Cannot open:") {
						t.Fatal("open failure must stay in selector with an error")
					}
				} else if scenario != "late result" {
					if m.child == nil {
						t.Fatalf("resolver did not open: %s", m.selector.notice)
					}
					m, _ = updateWorkspace(m, key)
				}
				m.tasks.close()
			}
			if cleaned.Load() != 1 {
				t.Fatalf("cleanup calls = %d, want exactly one", cleaned.Load())
			}
			if _, err := os.Stat(resource); !os.IsNotExist(err) {
				t.Fatalf("stage remains: %v", err)
			}
		})
	}
}

func TestWorkspaceTaskCloseRejectsUnstartedCommands(t *testing.T) {
	tasks := newWorkspaceTasks(context.Background())
	cmd := tasks.command(tasks.ctx, func() tea.Msg {
		t.Fatal("closed workspace started a loader")
		return nil
	})
	tasks.close()
	if got := cmd(); got != nil {
		t.Fatalf("closed command = %v", got)
	}
}

func TestWorkspaceTaskCloseWaitsForPendingPreparation(t *testing.T) {
	tasks := newWorkspaceTasks(context.Background())
	started, cancelled, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
	cmd := tasks.command(tasks.ctx, func() tea.Msg {
		close(started)
		<-tasks.ctx.Done()
		close(cancelled)
		<-finish
		return nil
	})
	go cmd()
	<-started
	closed := make(chan struct{})
	go func() { tasks.close(); close(closed) }()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		close(finish)
		t.Fatal("closing workspace did not cancel preparation")
	}
	select {
	case <-closed:
		close(finish)
		t.Fatal("workspace exited before preparation finished")
	default:
	}
	close(finish)
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("workspace did not finish closing")
	}
}

func TestWorkspaceReportsErrorsWithoutLosingExistingSelector(t *testing.T) {
	want := errors.New("repository unavailable")
	m := newWorkspaceModel(context.Background(), func(context.Context) (WorkspaceData, error) { return WorkspaceData{}, want })
	defer m.tasks.close()
	initial, cmd := updateWorkspace(m, m.Init()())
	if initial.err != want || !initial.quitting {
		t.Fatal("initial load failure must report an application error")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("initial load failure must exit the program")
	}
	m.loaded = true
	m.selector = newWorkspaceSelectModel([]FileCandidate{{Path: "conflict.txt"}}, nil)
	m, cmd = updateWorkspace(m, m.Init()())
	if m.err != nil || m.quitting || m.selector.items[0].path != "conflict.txt" || !strings.Contains(m.View(), "Refresh failed:") {
		t.Fatal("refresh failure must keep the existing selector and show an error")
	}
	assertNavigationCommand(t, cmd)
	m.child = model{visitID: m.visitID}
	m, cmd = updateWorkspace(m, editorFinishedMsg{visitID: m.visitID, err: want})
	if !m.quitting || !errors.Is(m.err, want) {
		t.Fatal("fatal resolver errors must propagate to the workspace")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("fatal resolver error must exit the program")
	}
}

func TestWorkspaceBackKeepsSelectorInSameProgram(t *testing.T) {
	for _, screen := range []string{"diff", "resolver", "loading"} {
		t.Run(screen, func(t *testing.T) {
			data := WorkspaceData{Conflicts: []FileCandidate{{Path: "one.go"}, {Path: "two.go"}}, DiffSources: []gitutil.DiffSource{gitutil.WorkingTreeSource()}}
			m := newWorkspaceModel(context.Background(), func(context.Context) (WorkspaceData, error) { return data, nil })
			defer m.tasks.close()
			m, _ = updateWorkspace(m, m.Init()())
			m.selector.selected = 1
			m.selector.offset = 1
			switch screen {
			case "diff":
				m.child = newDiffModel(m.visitCtx, "/repo", gitutil.WorkingTreeSource(), nil)
			case "resolver":
				m.child = newModelForDoc(t, parseSingleConflictDoc(t))
			case "loading":
				m.opening = true
			}
			oldCtx := m.visitCtx
			m, cmd := updateWorkspace(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
			if m.child != nil || m.opening || m.quitting || m.selector.selected != 1 || m.selector.offset != 1 {
				t.Fatal("back must restore the same selector without quitting")
			}
			if oldCtx.Err() == nil {
				t.Fatal("back must cancel the departed screen")
			}
			assertNavigationCommand(t, cmd)
			m, cmd = updateWorkspace(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
			if !m.quitting {
				t.Fatal("q at the selector must exit the application")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("application exit must quit the program")
			}
		})
	}
}

func TestWorkspaceWaitsForFreshDataBeforeReopening(t *testing.T) {
	data := workspaceTestData(t)
	m := newWorkspaceModel(context.Background(), func(context.Context) (WorkspaceData, error) { return data, nil })
	defer m.tasks.close()
	m, _ = updateWorkspace(m, m.Init()())
	m.child = newModelForDoc(t, parseSingleConflictDoc(t))
	m, _ = updateWorkspace(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	refreshID, refreshCtx := m.visitID, m.visitCtx
	m, cmd := updateWorkspace(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.opening || m.child != nil || m.visitID != refreshID || refreshCtx.Err() != nil {
		t.Fatal("Enter must not cancel refresh and reopen an old snapshot")
	}
	m, _ = updateWorkspace(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.selector.selected != 1 {
		t.Fatal("selector movement must remain available during refresh")
	}
	m, _ = updateWorkspace(m, m.refresh()())
	m, cmd = updateWorkspace(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.opening {
		t.Fatal("Enter must work again once the snapshot is fresh")
	}
}

// Navigation commands may load data or change mouse mode, but must not end the
// program or switch out of its alternate screen.
func assertNavigationCommand(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	switch msg := msg.(type) {
	case tea.QuitMsg:
		t.Fatal("navigation emitted QuitMsg")
	case tea.BatchMsg:
		for _, child := range msg {
			assertNavigationCommand(t, child)
		}
	default:
		if msg != nil && (msg == tea.ExitAltScreen()) {
			t.Fatal("navigation exited the alternate screen")
		}
	}
}
