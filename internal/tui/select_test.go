package tui

import (
	"context"
	"errors"
	"fmt"
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

func TestWorkspaceSelectorUnresolvedNavigationAcrossSections(t *testing.T) {
	sources := []gitutil.DiffSource{
		gitutil.WorkingTreeSource(),
		{Kind: gitutil.DiffSourceCommit, ShortHash: "commit1"},
		{Kind: gitutil.DiffSourceCommit, ShortHash: "commit2"},
	}
	for _, tc := range []struct {
		name      string
		conflicts []FileCandidate
		previous  []int
		next      []int
	}{
		{
			name: "skip resolved conflicts without wrapping",
			conflicts: []FileCandidate{
				{Path: "a.txt", Resolved: true},
				{Path: "b.txt"},
				{Path: "c.txt", Resolved: true},
				{Path: "d.txt"},
				{Path: "e.txt", Resolved: true},
			},
			previous: []int{0, 1, 1, 1, 3, 3, 3, 3},
			next:     []int{1, 3, 3, 3, 4, 5, 6, 7},
		},
		{
			name:      "all conflicts resolved",
			conflicts: []FileCandidate{{Path: "done.txt", Resolved: true}},
			previous:  []int{0, 1, 2, 3},
			next:      []int{0, 1, 2, 3},
		},
		{name: "no conflicts", previous: []int{0, 1, 2}, next: []int{0, 1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for key, expected := range map[rune][]int{'p': tc.previous, 'n': tc.next} {
				for start, want := range expected {
					model := newWorkspaceSelectModel(tc.conflicts, sources)
					model.width, model.height = 40, 6
					model.selected = start
					model.ensureSelectionVisible()
					updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
					model = updated.(workspaceSelectModel)
					if model.selected != want {
						t.Errorf("%c from %d selected %d, want %d", key, start, model.selected, want)
					}
					if !strings.Contains(model.View(), model.renderItem(model.selected)) {
						t.Errorf("%c from %d leaves the selected row offscreen", key, start)
					}
				}
			}
		})
	}
}

func TestWorkspaceSelectorPageNavigation(t *testing.T) {
	conflicts := []FileCandidate{{Path: "a.txt"}, {Path: "b.txt"}}
	sources := []gitutil.DiffSource{gitutil.WorkingTreeSource()}
	for index := range 30 {
		sources = append(sources, gitutil.DiffSource{Kind: gitutil.DiffSourceCommit, ShortHash: fmt.Sprintf("commit%d", index)})
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 12}, {Width: 40, Height: 6}} {
		for _, tc := range []struct {
			keys      []tea.KeyMsg
			direction int
		}{
			{
				keys: []tea.KeyMsg{
					{Type: tea.KeyPgUp}, {Type: tea.KeyLeft},
					{Type: tea.KeyRunes, Runes: []rune{'h'}},
					{Type: tea.KeyRunes, Runes: []rune{'b'}},
					{Type: tea.KeyRunes, Runes: []rune{'u'}},
				},
				direction: -1,
			},
			{
				keys: []tea.KeyMsg{
					{Type: tea.KeyPgDown}, {Type: tea.KeyRight},
					{Type: tea.KeyRunes, Runes: []rune{'l'}},
					{Type: tea.KeyRunes, Runes: []rune{'f'}},
					{Type: tea.KeyRunes, Runes: []rune{'d'}},
				},
				direction: 1,
			},
		} {
			for _, key := range tc.keys {
				for _, start := range []int{0, 1, 15, len(conflicts) + len(sources) - 1} {
					model := newWorkspaceSelectModel(conflicts, sources)
					updated, _ := model.Update(size)
					model = updated.(workspaceSelectModel)
					model.selected = start
					model.ensureSelectionVisible()
					want := min(max(start+tc.direction*model.bodyHeight(), 0), len(model.items)-1)
					updated, _ = model.Update(key)
					model = updated.(workspaceSelectModel)
					if model.selected != want {
						t.Errorf("%s from %d at %dx%d selected %d, want %d", key.String(), start, size.Width, size.Height, model.selected, want)
					}
					if !strings.Contains(model.View(), model.renderItem(model.selected)) {
						t.Errorf("%s leaves the selected row offscreen", key.String())
					}
				}
			}
		}
	}
}

func TestWorkspaceSelectorFirstAndLastNavigation(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyHome}, {Type: tea.KeyEnd},
		{Type: tea.KeyRunes, Runes: []rune{'g'}},
		{Type: tea.KeyRunes, Runes: []rune{'G'}},
	} {
		for _, count := range []int{0, 1, 40} {
			conflicts := make([]FileCandidate, count)
			for index := range conflicts {
				conflicts[index].Path = fmt.Sprintf("file%d.txt", index)
			}
			model := newWorkspaceSelectModel(conflicts, nil)
			model.width, model.height = 40, 6
			model.selected = count / 2
			want := 0
			if key.Type == tea.KeyEnd || key.String() == "G" {
				want = max(count-1, 0)
			}
			updated, _ := model.Update(key)
			model = updated.(workspaceSelectModel)
			if model.selected != want {
				t.Errorf("%s with %d items selected %d, want %d", key.String(), count, model.selected, want)
			}
			if count > 0 && !strings.Contains(model.View(), model.renderItem(model.selected)) {
				t.Errorf("%s leaves the selected row offscreen", key.String())
			}
		}
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
