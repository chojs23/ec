package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestFileItemMethods(t *testing.T) {
	item := fileItem{path: "conflict.txt"}
	if item.Title() != "conflict.txt" {
		t.Fatalf("Title = %q, want conflict.txt", item.Title())
	}
	if item.Description() != "" {
		t.Fatalf("Description = %q, want empty", item.Description())
	}
	if item.FilterValue() != "conflict.txt" {
		t.Fatalf("FilterValue = %q, want conflict.txt", item.FilterValue())
	}
}

func TestFileItemDelegateLayout(t *testing.T) {
	delegate := fileItemDelegate{}
	if delegate.Height() != 1 {
		t.Fatalf("Height = %d, want 1", delegate.Height())
	}
	if delegate.Spacing() != 0 {
		t.Fatalf("Spacing = %d, want 0", delegate.Spacing())
	}

	model := list.New(nil, delegate, 0, 0)
	if cmd := delegate.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}, &model); cmd != nil {
		t.Fatalf("expected nil cmd from delegate.Update")
	}
}

func TestFileItemDelegateRender(t *testing.T) {
	items := []list.Item{
		fileItem{path: "a.txt", resolved: false},
		fileItem{path: "b.txt", resolved: true},
	}
	model := list.New(items, fileItemDelegate{}, 0, 0)
	model.Select(0)

	delegate := fileItemDelegate{}
	var buf bytes.Buffer
	delegate.Render(&buf, model, 0, items[0])
	output := buf.String()
	if !strings.HasPrefix(output, "> ") {
		t.Fatalf("output = %q, want selected cursor prefix", output)
	}
	if !strings.Contains(output, "unresolved") {
		t.Fatalf("output = %q, want unresolved label", output)
	}
	if !strings.Contains(output, "a.txt") {
		t.Fatalf("output = %q, want file path", output)
	}

	buf.Reset()
	model.Select(1)
	delegate.Render(&buf, model, 1, items[1])
	output = buf.String()
	if !strings.HasPrefix(output, "> ") {
		t.Fatalf("output = %q, want selected cursor prefix", output)
	}
	if strings.Contains(output, "unresolved") {
		t.Fatalf("output = %q, did not expect unresolved label", output)
	}
	if !strings.Contains(output, "  resolved") {
		t.Fatalf("output = %q, want resolved label", output)
	}
}

func TestFileSelectModelUpdateEnter(t *testing.T) {
	items := []list.Item{fileItem{path: "a.txt", resolved: false}}
	model := fileSelectModel{list: list.New(items, fileItemDelegate{}, 0, 0)}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := updated.(fileSelectModel)
	if result.selected != "a.txt" {
		t.Fatalf("selected = %q, want a.txt", result.selected)
	}
}

func TestFileSelectModelUpdateQuit(t *testing.T) {
	items := []list.Item{fileItem{path: "a.txt", resolved: false}}
	model := fileSelectModel{list: list.New(items, fileItemDelegate{}, 0, 0)}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	result := updated.(fileSelectModel)
	if result.err != ErrSelectorQuit {
		t.Fatalf("err = %v, want ErrSelectorQuit", result.err)
	}
}

func TestFileSelectModelWindowResize(t *testing.T) {
	items := []list.Item{fileItem{path: "a.txt", resolved: false}}
	model := fileSelectModel{list: list.New(items, fileItemDelegate{}, 0, 0)}

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 4})
	result := updated.(fileSelectModel)
	if result.list.Width() != 40 {
		t.Fatalf("Width = %d, want 40", result.list.Width())
	}
	if result.list.Height() != 3 {
		t.Fatalf("Height = %d, want 3", result.list.Height())
	}
}

func TestFileSelectModelView(t *testing.T) {
	items := []list.Item{fileItem{path: "a.txt", resolved: false}}
	model := fileSelectModel{list: list.New(items, fileItemDelegate{}, 0, 0)}
	view := model.View()
	if !strings.Contains(view, "up/down: move") {
		t.Fatalf("view = %q, want help line", view)
	}
}
