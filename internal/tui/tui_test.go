package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chojs23/easy-conflict/internal/cli"
	"github.com/chojs23/easy-conflict/internal/engine"
	"github.com/chojs23/easy-conflict/internal/gitmerge"
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

func TestOpenEditorWithUnresolvedConflicts(t *testing.T) {
	tmpDir := t.TempDir()

	mergedPath := filepath.Join(tmpDir, "merged.txt")
	mergedContent := []byte("<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\n")
	if err := os.WriteFile(mergedPath, mergedContent, 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	data, err := os.ReadFile(mergedPath)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}

	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}

	state, err := engine.NewState(doc, 1)
	if err != nil {
		t.Fatalf("NewState error = %v", err)
	}

	editorPath := filepath.Join(tmpDir, "editor.sh")
	if err := os.WriteFile(editorPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile editor error = %v", err)
	}

	originalEditor := os.Getenv("EDITOR")
	if err := os.Setenv("EDITOR", editorPath); err != nil {
		t.Fatalf("Setenv error = %v", err)
	}
	defer os.Setenv("EDITOR", originalEditor)

	m := model{
		state: state,
		opts:  cliOptionsWithMergedPath(mergedPath),
	}

	cmd := m.openEditor()
	msg := cmd()
	typeName := fmt.Sprintf("%T", msg)
	if !strings.Contains(typeName, "execMsg") {
		t.Fatalf("unexpected msg type %T", msg)
	}
}

func TestReloadFromFilePreservesManualResolution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration-style test in short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	basePath := filepath.Join(tmpDir, "base.txt")
	localPath := filepath.Join(tmpDir, "local.txt")
	remotePath := filepath.Join(tmpDir, "remote.txt")
	mergedPath := filepath.Join(tmpDir, "merged.txt")

	baseContent := "line1\nbase\nline3\n"
	localContent := "line1\nlocal\nline3\n"
	remoteContent := "line1\nremote\nline3\n"
	mergedContent := "line1\nmanual\nline3\n"

	if err := os.WriteFile(basePath, []byte(baseContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localPath, []byte(localContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remotePath, []byte(remoteContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mergedPath, []byte(mergedContent), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := cli.Options{
		BasePath:   basePath,
		LocalPath:  localPath,
		RemotePath: remotePath,
		MergedPath: mergedPath,
	}

	diff3Bytes, err := gitmerge.MergeFileDiff3(ctx, opts.LocalPath, opts.BasePath, opts.RemotePath)
	if err != nil {
		t.Fatalf("MergeFileDiff3 failed: %v", err)
	}

	doc, err := markers.Parse(diff3Bytes)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}

	state, err := engine.NewState(doc, 10)
	if err != nil {
		t.Fatalf("NewState error = %v", err)
	}

	m := model{
		ctx:   ctx,
		opts:  opts,
		state: state,
		doc:   doc,
	}

	if err := m.reloadFromFile(); err != nil {
		t.Fatalf("reloadFromFile error = %v", err)
	}

	manual, ok := m.manualResolved[0]
	if !ok {
		t.Fatalf("expected manual resolution for conflict 0")
	}
	if string(manual) != "manual\n" {
		t.Fatalf("manual resolution = %q", string(manual))
	}
}

func cliOptionsWithMergedPath(path string) cli.Options {
	return cli.Options{MergedPath: path}
}
