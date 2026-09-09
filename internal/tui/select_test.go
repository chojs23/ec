package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chojs23/ec/internal/gitutil"
)

type stubProgram struct {
	model tea.Model
	err   error
}

func (s stubProgram) Run() (tea.Model, error) {
	return s.model, s.err
}

func withSelectProgram(t *testing.T, fn func(model tea.Model, ctx context.Context) programRunner, run func()) {
	t.Helper()
	old := selectProgram
	selectProgram = fn
	defer func() {
		selectProgram = old
	}()

	run()
}

func TestWorkspaceSelectorShowsConflictAndDiffSections(t *testing.T) {
	model := newWorkspaceSelectModel(
		[]FileCandidate{
			{Path: "resolved.txt", Resolved: true},
			{Path: "unresolved.txt", Resolved: false},
		},
		[]gitutil.DiffSource{
			gitutil.WorkingTreeSource(),
			{
				Kind:       gitutil.DiffSourceCommit,
				ShortHash:  "abc1234",
				Subject:    "add viewer",
				Deletions:  4,
				Additions:  12,
				StatsKnown: true,
			},
		},
	)

	if model.selected != 1 {
		t.Fatalf("selected = %d, want first unresolved conflict at 1", model.selected)
	}
	view := model.View()
	for _, text := range []string{
		"Workspace", "2 conflicts", "2 diff sources", "Conflicts  2",
		"resolved.txt", "unresolved.txt", "View Diff  2", "working tree",
		"abc1234", "add viewer", "-4", "+12",
	} {
		if !strings.Contains(view, text) {
			t.Fatalf("view missing %q:\n%s", text, view)
		}
	}
}

func TestWorkspaceSelectorKeepsCommitStatsVisibleWhenSubjectTruncates(t *testing.T) {
	model := newWorkspaceSelectModel(nil, []gitutil.DiffSource{{
		Kind:       gitutil.DiffSourceCommit,
		ShortHash:  "1234567890abcdef",
		Subject:    strings.Repeat("long subject ", 12),
		Deletions:  123,
		Additions:  45,
		StatsKnown: true,
	}})

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 32, Height: 8})
	view := updated.(workspaceSelectModel).View()
	if !strings.Contains(view, "-123") || !strings.Contains(view, "+45") {
		t.Fatalf("view does not preserve commit totals at narrow width:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if width := lipgloss.Width(line); width > 32 {
			t.Fatalf("line width = %d, want <= 32:\n%s", width, line)
		}
	}
}

func TestWorkspaceSelectorWithoutConflictsFocusesWorkingTree(t *testing.T) {
	model := newWorkspaceSelectModel(nil, []gitutil.DiffSource{gitutil.WorkingTreeSource()})
	if model.selected != 0 {
		t.Fatalf("selected = %d, want working tree at 0", model.selected)
	}
	if strings.Contains(model.View(), "Conflicts") || !strings.Contains(model.View(), "View Diff") {
		t.Fatalf("view = %q, want only the diff section", model.View())
	}
}

func TestWorkspaceSelectorSelectsConflictAndDiffTargets(t *testing.T) {
	model := newWorkspaceSelectModel(
		[]FileCandidate{{Path: "conflict.txt"}},
		[]gitutil.DiffSource{{Kind: gitutil.DiffSourceCommit, Commit: "full", ShortHash: "short", Subject: "subject"}},
	)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	conflictResult := updated.(workspaceSelectModel).result
	if conflictResult.Kind != WorkspaceSelectionConflict || conflictResult.ConflictPath != "conflict.txt" {
		t.Fatalf("conflict result = %#v", conflictResult)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(workspaceSelectModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	diffResult := updated.(workspaceSelectModel).result
	if diffResult.Kind != WorkspaceSelectionDiff || diffResult.DiffSource.Commit != "full" {
		t.Fatalf("diff result = %#v", diffResult)
	}
}

func TestWorkspaceSelectorUnresolvedNavigationStaysInConflictSection(t *testing.T) {
	model := newWorkspaceSelectModel(
		[]FileCandidate{
			{Path: "a.txt", Resolved: false},
			{Path: "b.txt", Resolved: true},
			{Path: "c.txt", Resolved: false},
		},
		[]gitutil.DiffSource{gitutil.WorkingTreeSource()},
	)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(workspaceSelectModel)
	if model.selected != 2 {
		t.Fatalf("selected = %d, want next unresolved at 2", model.selected)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(workspaceSelectModel)
	if model.selected != 2 {
		t.Fatalf("selected = %d, want navigation to stop before diff section", model.selected)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model = updated.(workspaceSelectModel)
	if model.selected != 0 {
		t.Fatalf("selected = %d, want previous unresolved at 0", model.selected)
	}
}

func TestWorkspaceSelectorResizeKeepsSelectionVisible(t *testing.T) {
	sources := []gitutil.DiffSource{gitutil.WorkingTreeSource()}
	for index := 0; index < 10; index++ {
		sources = append(sources, gitutil.DiffSource{
			Kind:      gitutil.DiffSourceCommit,
			ShortHash: "commit",
			Subject:   "history",
		})
	}
	model := newWorkspaceSelectModel(nil, sources)

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 6})
	model = updated.(workspaceSelectModel)
	for range 10 {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(workspaceSelectModel)
	}

	if model.offset == 0 {
		t.Fatalf("offset = 0, want scrolled selector")
	}
	if !strings.Contains(model.View(), "history") {
		t.Fatalf("view does not contain selected history row")
	}
}

func TestWorkspaceSelectorQuit(t *testing.T) {
	model := newWorkspaceSelectModel(nil, []gitutil.DiffSource{gitutil.WorkingTreeSource()})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if result := updated.(workspaceSelectModel); result.err != ErrSelectorQuit {
		t.Fatalf("err = %v, want ErrSelectorQuit", result.err)
	}
}

func TestSelectWorkspaceReturnsSelection(t *testing.T) {
	want := WorkspaceSelection{Kind: WorkspaceSelectionDiff, DiffSource: gitutil.WorkingTreeSource()}
	withSelectProgram(t, func(model tea.Model, ctx context.Context) programRunner {
		if _, ok := model.(workspaceSelectModel); !ok {
			t.Fatalf("model type = %T, want workspaceSelectModel", model)
		}
		return stubProgram{model: workspaceSelectModel{result: want}}
	}, func() {
		selected, err := SelectWorkspace(context.Background(), nil, []gitutil.DiffSource{gitutil.WorkingTreeSource()})
		if err != nil {
			t.Fatalf("SelectWorkspace error = %v", err)
		}
		if selected.Kind != want.Kind || selected.DiffSource.Kind != want.DiffSource.Kind {
			t.Fatalf("SelectWorkspace = %#v, want %#v", selected, want)
		}
	})
}

func TestSelectFileReturnsConflictSelection(t *testing.T) {
	withSelectProgram(t, func(model tea.Model, ctx context.Context) programRunner {
		return stubProgram{model: workspaceSelectModel{result: WorkspaceSelection{
			Kind:         WorkspaceSelectionConflict,
			ConflictPath: "picked.txt",
		}}}
	}, func() {
		selected, err := SelectFile(context.Background(), []FileCandidate{{Path: "picked.txt"}})
		if err != nil {
			t.Fatalf("SelectFile error = %v", err)
		}
		if selected != "picked.txt" {
			t.Fatalf("SelectFile = %q, want picked.txt", selected)
		}
	})
}

func TestSelectWorkspaceReturnsProgramError(t *testing.T) {
	withSelectProgram(t, func(model tea.Model, ctx context.Context) programRunner {
		return stubProgram{err: errors.New("boom")}
	}, func() {
		_, err := SelectWorkspace(context.Background(), nil, []gitutil.DiffSource{gitutil.WorkingTreeSource()})
		if err == nil {
			t.Fatalf("SelectWorkspace error = nil, want error")
		}
	})
}
