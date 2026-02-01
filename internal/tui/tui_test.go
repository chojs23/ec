package tui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chojs23/easy-conflict/internal/cli"
	"github.com/chojs23/easy-conflict/internal/engine"
	"github.com/chojs23/easy-conflict/internal/markers"
)

func TestModelQuitBackToSelector(t *testing.T) {
	m := model{}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	updatedModel := updated.(model)
	if updatedModel.err != ErrBackToSelector {
		t.Fatalf("expected ErrBackToSelector, got %v", updatedModel.err)
	}
	if !updatedModel.quitting {
		t.Fatalf("expected quitting true")
	}
}

func TestModelWriteDoesNotQuit(t *testing.T) {
	file, err := os.CreateTemp("", "easy-conflict-merged-*")
	if err != nil {
		t.Fatalf("CreateTemp error = %v", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	defer os.Remove(path)

	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	doc := markers.Document{Segments: []markers.Segment{markers.TextSegment{Bytes: []byte("resolved\n")}}}
	state, err := engine.NewState(doc, 1)
	if err != nil {
		t.Fatalf("NewState error = %v", err)
	}

	m := model{
		state: state,
		doc:   doc,
		opts:  cliOptionsWithMergedPath(path),
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	updatedModel := updated.(model)
	if updatedModel.err != nil {
		t.Fatalf("expected no error, got %v", updatedModel.err)
	}
	if updatedModel.quitting {
		t.Fatalf("expected quitting false")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if string(data) != "resolved\n" {
		t.Fatalf("merged content = %q, want %q", string(data), "resolved\n")
	}
}

func cliOptionsWithMergedPath(path string) cli.Options {
	return cli.Options{MergedPath: path}
}
