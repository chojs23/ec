package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/engine"
	"github.com/chojs23/ec/internal/gitmerge"
	"github.com/chojs23/ec/internal/markers"
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
	file, err := os.CreateTemp("", "ec-merged-*")
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

func TestModelInitReturnsNil(t *testing.T) {
	if cmd := (model{}).Init(); cmd != nil {
		t.Fatalf("Init() = %v, want nil", cmd)
	}
}

func TestRunReturnsThemeLoadError(t *testing.T) {
	resetThemeForTest()
	t.Cleanup(resetThemeForTest)

	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)

	configPath := filepath.Join(configDir, "ec", themeConfigFileName)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte("{bad"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	if err := Run(context.Background(), cli.Options{}); err == nil {
		t.Fatal("Run() error = nil, want error")
	}
}

func TestFirstHexRun(t *testing.T) {
	start, end := firstHexRun("x1234567y")
	if start != 1 || end != 8 {
		t.Fatalf("firstHexRun = (%d, %d), want (1, 8)", start, end)
	}

	start, end = firstHexRun("nohex")
	if start != -1 || end != -1 {
		t.Fatalf("firstHexRun = (%d, %d), want (-1, -1)", start, end)
	}

	start, end = firstHexRun("x1234y")
	if start != -1 || end != -1 {
		t.Fatalf("firstHexRun = (%d, %d), want (-1, -1)", start, end)
	}
}

func TestHexHelpers(t *testing.T) {
	if !isHexRune('F') {
		t.Fatalf("isHexRune('F') = false, want true")
	}
	if isHexRune('g') {
		t.Fatalf("isHexRune('g') = true, want false")
	}
	if !isHexByte('a') {
		t.Fatalf("isHexByte('a') = false, want true")
	}
	if isHexByte('G') {
		t.Fatalf("isHexByte('G') = true, want false")
	}
}

func cliOptionsWithMergedPath(path string) cli.Options {
	return cli.Options{MergedPath: path}
}

func TestModelViewNotReady(t *testing.T) {
	m := model{}
	if !strings.Contains(m.View(), "Initializing") {
		t.Fatalf("expected initializing view")
	}
}

func TestModelViewQuittingStates(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{name: "back", err: ErrBackToSelector, want: "Returning to selector"},
		{name: "error", err: fmt.Errorf("boom"), want: "Error:"},
		{name: "resolved", err: nil, want: "Resolved! File written."},
	}

	for _, tc := range testCases {
		m := model{ready: true, quitting: true, err: tc.err}
		if !strings.Contains(m.View(), tc.want) {
			t.Fatalf("%s: expected %q in view", tc.name, tc.want)
		}
	}
}

func TestModelViewNoConflicts(t *testing.T) {
	doc := markers.Document{Segments: []markers.Segment{markers.TextSegment{Bytes: []byte("hello\n")}}}
	m := model{ready: true, doc: doc, opts: cliOptionsWithMergedPath("merged.txt")}
	if !strings.Contains(m.View(), "No conflicts found") {
		t.Fatalf("expected no conflicts view")
	}
}

func TestModelViewReady(t *testing.T) {
	doc := parseSingleConflictDoc(t)
	state, err := engine.NewState(doc, 10)
	if err != nil {
		t.Fatalf("NewState error = %v", err)
	}
	m := model{
		ready:           true,
		opts:            cliOptionsWithMergedPath("merged.txt"),
		state:           state,
		doc:             doc,
		currentConflict: 0,
		selectedSide:    selectedOurs,
		manualResolved:  map[int][]byte{},
		viewportOurs:    viewport.New(10, 5),
		viewportResult:  viewport.New(10, 5),
		viewportTheirs:  viewport.New(10, 5),
		width:           80,
		height:          20,
	}
	m.updateViewports()

	view := m.View()
	if !strings.Contains(view, "Conflict 1/1") {
		t.Fatalf("expected conflict status in view")
	}
	if !strings.Contains(view, "RESULT") {
		t.Fatalf("expected RESULT header in view")
	}
}

func TestRenderToastLine(t *testing.T) {
	m := model{width: 20, toastMessage: "Saved"}
	if !strings.Contains(m.renderToastLine(), "Saved") {
		t.Fatalf("expected toast line to include message")
	}

	m.toastMessage = ""
	if strings.Contains(m.renderToastLine(), "Saved") {
		t.Fatalf("did not expect toast message when empty")
	}
}

func TestUpdateNavigationKeys(t *testing.T) {
	doc := parseMultiConflictDoc(t)
	m := newModelForDoc(t, doc)
	m.pendingScroll = false

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	next := updated.(model)
	if next.currentConflict != 1 {
		t.Fatalf("currentConflict = %d, want 1", next.currentConflict)
	}
	if next.pendingScroll {
		t.Fatalf("expected pendingScroll false after updateViewports")
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	prev := updated.(model)
	if prev.currentConflict != 0 {
		t.Fatalf("currentConflict = %d, want 0", prev.currentConflict)
	}
}

func TestUpdateApplyAndUndo(t *testing.T) {
	doc := parseSingleConflictDoc(t)
	m := newModelForDoc(t, doc)
	m.manualResolved = map[int][]byte{0: []byte("manual\n")}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	applied := updated.(model)
	if len(applied.manualResolved) != 0 {
		t.Fatalf("manualResolved len = %d, want 0", len(applied.manualResolved))
	}
	if got := conflictResolution(t, applied.doc, 0); got != markers.ResolutionOurs {
		t.Fatalf("resolution = %q, want ours", got)
	}

	updated, _ = applied.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	undone := updated.(model)
	if got := conflictResolution(t, undone.doc, 0); got != markers.ResolutionUnset {
		t.Fatalf("resolution = %q, want unset", got)
	}
}

func TestUpdateApplyAllClearsManual(t *testing.T) {
	doc := parseMultiConflictDoc(t)
	m := newModelForDoc(t, doc)
	m.manualResolved = map[int][]byte{0: []byte("manual\n"), 1: []byte("manual\n")}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	applied := updated.(model)
	if len(applied.manualResolved) != 0 {
		t.Fatalf("manualResolved len = %d, want 0", len(applied.manualResolved))
	}
	for i := range applied.doc.Conflicts {
		if got := conflictResolution(t, applied.doc, i); got != markers.ResolutionOurs {
			t.Fatalf("conflict %d resolution = %q, want ours", i, got)
		}
	}
}

func TestUpdateDiscardSelection(t *testing.T) {
	doc := parseSingleConflictDoc(t)
	m := newModelForDoc(t, doc)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	result := updated.(model)
	if got := conflictResolution(t, result.doc, 0); got != markers.ResolutionNone {
		t.Fatalf("resolution = %q, want none", got)
	}
}

func TestUpdateAcceptSelection(t *testing.T) {
	doc := parseSingleConflictDoc(t)
	m := newModelForDoc(t, doc)
	m.selectedSide = selectedTheirs
	m.manualResolved = map[int][]byte{0: []byte("manual\n")}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	result := updated.(model)
	if got := conflictResolution(t, result.doc, 0); got != markers.ResolutionTheirs {
		t.Fatalf("resolution = %q, want theirs", got)
	}
	if len(result.manualResolved) != 0 {
		t.Fatalf("manualResolved len = %d, want 0", len(result.manualResolved))
	}
}

func TestUpdateAcceptSelectionWithSpace(t *testing.T) {
	doc := parseSingleConflictDoc(t)
	m := newModelForDoc(t, doc)
	m.selectedSide = selectedTheirs
	m.manualResolved = map[int][]byte{0: []byte("manual\n")}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	result := updated.(model)
	if got := conflictResolution(t, result.doc, 0); got != markers.ResolutionTheirs {
		t.Fatalf("resolution = %q, want theirs", got)
	}
	if len(result.manualResolved) != 0 {
		t.Fatalf("manualResolved len = %d, want 0", len(result.manualResolved))
	}
}

func TestUpdateApplyTheirs(t *testing.T) {
	doc := parseSingleConflictDoc(t)
	m := newModelForDoc(t, doc)
	m.manualResolved = map[int][]byte{0: []byte("manual\n")}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	result := updated.(model)
	if got := conflictResolution(t, result.doc, 0); got != markers.ResolutionTheirs {
		t.Fatalf("resolution = %q, want theirs", got)
	}
	if len(result.manualResolved) != 0 {
		t.Fatalf("manualResolved len = %d, want 0", len(result.manualResolved))
	}
}

func TestUpdateApplyTheirsAll(t *testing.T) {
	doc := parseMultiConflictDoc(t)
	m := newModelForDoc(t, doc)
	m.manualResolved = map[int][]byte{0: []byte("manual\n"), 1: []byte("manual\n")}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	result := updated.(model)
	for i := range result.doc.Conflicts {
		if got := conflictResolution(t, result.doc, i); got != markers.ResolutionTheirs {
			t.Fatalf("conflict %d resolution = %q, want theirs", i, got)
		}
	}
	if len(result.manualResolved) != 0 {
		t.Fatalf("manualResolved len = %d, want 0", len(result.manualResolved))
	}
}

func TestUpdateApplyBothAndNone(t *testing.T) {
	doc := parseSingleConflictDoc(t)
	m := newModelForDoc(t, doc)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	result := updated.(model)
	if got := conflictResolution(t, result.doc, 0); got != markers.ResolutionBoth {
		t.Fatalf("resolution = %q, want both", got)
	}

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	result = updated.(model)
	if got := conflictResolution(t, result.doc, 0); got != markers.ResolutionNone {
		t.Fatalf("resolution = %q, want none", got)
	}
}

func TestUpdateScrollHorizontalKeys(t *testing.T) {
	content := "0123456789"
	m := model{
		viewportOurs:   viewport.New(5, 1),
		viewportResult: viewport.New(5, 1),
		viewportTheirs: viewport.New(5, 1),
	}
	for _, viewportModel := range []*viewport.Model{&m.viewportOurs, &m.viewportResult, &m.viewportTheirs} {
		viewportModel.SetContent(content)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	result := updated.(model)
	if got := result.viewportOurs.View(); got != "45678" {
		t.Fatalf("View = %q, want 45678 after L", got)
	}

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	result = updated.(model)
	if got := result.viewportOurs.View(); got != "01234" {
		t.Fatalf("View = %q, want 01234 after H", got)
	}

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRight})
	result = updated.(model)
	if got := result.viewportOurs.View(); got != "45678" {
		t.Fatalf("View = %q, want 45678 after right", got)
	}

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyLeft})
	result = updated.(model)
	if got := result.viewportOurs.View(); got != "01234" {
		t.Fatalf("View = %q, want 01234 after left", got)
	}
}

func TestUpdateKeySeqScroll(t *testing.T) {
	lines := strings.Join([]string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}, "\n")
	m := model{
		viewportOurs:   viewport.New(5, 3),
		viewportResult: viewport.New(5, 3),
		viewportTheirs: viewport.New(5, 3),
	}
	for _, viewportModel := range []*viewport.Model{&m.viewportOurs, &m.viewportResult, &m.viewportTheirs} {
		viewportModel.SetContent(lines)
		viewportModel.ScrollDown(5)
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	result := updated.(model)
	if cmd == nil {
		t.Fatalf("expected tick cmd for key sequence")
	}
	if result.keySeq != "g" {
		t.Fatalf("keySeq = %q, want g", result.keySeq)
	}

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	result = updated.(model)
	if result.keySeq != "" {
		t.Fatalf("keySeq = %q, want cleared", result.keySeq)
	}
	if result.viewportOurs.YOffset != 0 {
		t.Fatalf("YOffset = %d, want 0 after gg", result.viewportOurs.YOffset)
	}

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	result = updated.(model)
	if result.viewportOurs.YOffset != 7 {
		t.Fatalf("YOffset = %d, want 7 after G", result.viewportOurs.YOffset)
	}
}

func TestUpdateIgnoresUnmappedViewportKeys(t *testing.T) {
	lines := strings.Join([]string{"one", "two", "three", "four", "five", "six"}, "\n")
	m := model{
		viewportOurs:   viewport.New(5, 3),
		viewportResult: viewport.New(5, 3),
		viewportTheirs: viewport.New(5, 3),
	}
	for _, viewportModel := range []*viewport.Model{&m.viewportOurs, &m.viewportResult, &m.viewportTheirs} {
		viewportModel.SetContent(lines)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	result := updated.(model)

	if result.viewportOurs.YOffset != 0 {
		t.Fatalf("YOffset = %d, want 0 after unmapped key", result.viewportOurs.YOffset)
	}
	if result.viewportResult.YOffset != 0 {
		t.Fatalf("result YOffset = %d, want 0 after unmapped key", result.viewportResult.YOffset)
	}
	if result.viewportTheirs.YOffset != 0 {
		t.Fatalf("theirs YOffset = %d, want 0 after unmapped key", result.viewportTheirs.YOffset)
	}
}

func TestUpdateVerticalScrollKeys(t *testing.T) {
	lines := strings.Join([]string{"one", "two", "three", "four", "five", "six"}, "\n")
	m := model{
		viewportOurs:   viewport.New(5, 3),
		viewportResult: viewport.New(5, 3),
		viewportTheirs: viewport.New(5, 3),
	}
	for _, viewportModel := range []*viewport.Model{&m.viewportOurs, &m.viewportResult, &m.viewportTheirs} {
		viewportModel.SetContent(lines)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	result := updated.(model)
	if result.viewportOurs.YOffset != 1 {
		t.Fatalf("YOffset = %d, want 1 after j", result.viewportOurs.YOffset)
	}

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	result = updated.(model)
	if result.viewportOurs.YOffset != 0 {
		t.Fatalf("YOffset = %d, want 0 after k", result.viewportOurs.YOffset)
	}

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyDown})
	result = updated.(model)
	if result.viewportOurs.YOffset != 1 {
		t.Fatalf("YOffset = %d, want 1 after down", result.viewportOurs.YOffset)
	}

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyUp})
	result = updated.(model)
	if result.viewportOurs.YOffset != 0 {
		t.Fatalf("YOffset = %d, want 0 after up", result.viewportOurs.YOffset)
	}
}

func TestUpdateWriteKey(t *testing.T) {
	tmpDir := t.TempDir()
	mergedPath := filepath.Join(tmpDir, "merged.txt")
	if err := os.WriteFile(mergedPath, []byte("original\n"), 0o644); err != nil {
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
		opts:  cliOptionsWithMergedPath(mergedPath),
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	result := updated.(model)
	if result.toastMessage != "Saved" {
		t.Fatalf("toastMessage = %q, want Saved", result.toastMessage)
	}
	if cmd == nil {
		t.Fatalf("expected toast cmd")
	}

	data, err := os.ReadFile(mergedPath)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if string(data) != "resolved\n" {
		t.Fatalf("merged content = %q, want resolved\\n", string(data))
	}
}

func TestUpdateEditorKey(t *testing.T) {
	originalEditor := os.Getenv("EDITOR")
	if err := os.Setenv("EDITOR", "true"); err != nil {
		t.Fatalf("Setenv error = %v", err)
	}
	defer os.Setenv("EDITOR", originalEditor)

	m := model{}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	_ = updated.(model)
	if cmd == nil {
		t.Fatalf("expected editor cmd")
	}
	if _, ok := cmd().(editorFinishedMsg); !ok {
		t.Fatalf("expected editorFinishedMsg")
	}
}

func TestUpdateCtrlC(t *testing.T) {
	m := model{}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	result := updated.(model)
	if !result.quitting {
		t.Fatalf("expected quitting true")
	}
}

func TestPrepareFullDiffGuards(t *testing.T) {
	doc := parseSingleConflictDoc(t)

	_, _, _, _, useFullDiff := prepareFullDiff(doc, cli.Options{AllowMissingBase: true})
	if useFullDiff {
		t.Fatalf("expected useFullDiff false when AllowMissingBase is set")
	}

	_, _, _, _, useFullDiff = prepareFullDiff(doc, cli.Options{})
	if useFullDiff {
		t.Fatalf("expected useFullDiff false when paths are missing")
	}
}

func TestPrepareFullDiffLoadFailure(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "base.txt")
	if err := os.WriteFile(basePath, []byte("base\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	opts := cli.Options{
		BasePath:   basePath,
		LocalPath:  filepath.Join(tmpDir, "missing-local.txt"),
		RemotePath: filepath.Join(tmpDir, "missing-remote.txt"),
	}
	_, _, _, _, useFullDiff := prepareFullDiff(parseSingleConflictDoc(t), opts)
	if useFullDiff {
		t.Fatalf("expected useFullDiff false when loadLines fails")
	}
}

func TestPrepareFullDiffRangeFailure(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "base.txt")
	localPath := filepath.Join(tmpDir, "local.txt")
	remotePath := filepath.Join(tmpDir, "remote.txt")

	if err := os.WriteFile(basePath, []byte("different\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	if err := os.WriteFile(localPath, []byte("ours\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	if err := os.WriteFile(remotePath, []byte("theirs\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	opts := cli.Options{BasePath: basePath, LocalPath: localPath, RemotePath: remotePath}
	_, _, _, _, useFullDiff := prepareFullDiff(parseSingleConflictDoc(t), opts)
	if useFullDiff {
		t.Fatalf("expected useFullDiff false when conflict ranges cannot be computed")
	}
}

func parseMultiConflictDoc(t *testing.T) markers.Document {
	t.Helper()
	data := []byte("start\n<<<<<<< HEAD\nours1\n=======\ntheirs1\n>>>>>>> branch\nmid\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> branch\nend\n")
	doc, err := markers.Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	return doc
}

func newModelForDoc(t *testing.T, doc markers.Document) model {
	t.Helper()
	state, err := engine.NewState(doc, 10)
	if err != nil {
		t.Fatalf("NewState error = %v", err)
	}
	return model{
		state:           state,
		doc:             doc,
		currentConflict: 0,
		selectedSide:    selectedOurs,
		manualResolved:  map[int][]byte{},
		viewportOurs:    viewport.New(10, 5),
		viewportResult:  viewport.New(10, 5),
		viewportTheirs:  viewport.New(10, 5),
	}
}

func conflictResolution(t *testing.T, doc markers.Document, index int) markers.Resolution {
	t.Helper()
	ref := doc.Conflicts[index]
	seg, ok := doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
	if !ok {
		t.Fatalf("expected conflict segment")
	}
	return seg.Resolution
}

func TestEnsureVisibleOffsets(t *testing.T) {
	viewportModel := viewport.New(10, 4)
	viewportModel.YOffset = 3
	ensureVisible(&viewportModel, 0, 10)
	if viewportModel.YOffset != 0 {
		t.Fatalf("YOffset = %d, want 0", viewportModel.YOffset)
	}

	ensureVisible(&viewportModel, 9, 10)
	if viewportModel.YOffset != 6 {
		t.Fatalf("YOffset = %d, want 6", viewportModel.YOffset)
	}

	viewportModel.YOffset = 5
	ensureVisible(&viewportModel, 1, 0)
	if viewportModel.YOffset != 0 {
		t.Fatalf("YOffset = %d, want 0 for empty total", viewportModel.YOffset)
	}

	viewportModel.Height = 0
	viewportModel.YOffset = 5
	ensureVisible(&viewportModel, 2, 10)
	if viewportModel.YOffset != 5 {
		t.Fatalf("YOffset = %d, want unchanged when height is zero", viewportModel.YOffset)
	}
}

func TestScrollToTopAndBottom(t *testing.T) {
	lines := strings.Join([]string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}, "\n")

	m := model{
		viewportOurs:   viewport.New(5, 3),
		viewportResult: viewport.New(5, 3),
		viewportTheirs: viewport.New(5, 3),
	}
	for _, viewportModel := range []*viewport.Model{&m.viewportOurs, &m.viewportResult, &m.viewportTheirs} {
		viewportModel.SetContent(lines)
		viewportModel.ScrollDown(5)
	}

	m.scrollToTop()
	for _, viewportModel := range []*viewport.Model{&m.viewportOurs, &m.viewportResult, &m.viewportTheirs} {
		if viewportModel.YOffset != 0 {
			t.Fatalf("YOffset = %d, want 0 after scrollToTop", viewportModel.YOffset)
		}
	}

	m.scrollToBottom()
	for _, viewportModel := range []*viewport.Model{&m.viewportOurs, &m.viewportResult, &m.viewportTheirs} {
		if viewportModel.YOffset != 7 {
			t.Fatalf("YOffset = %d, want 7 after scrollToBottom", viewportModel.YOffset)
		}
	}
}

func TestScrollHorizontal(t *testing.T) {
	content := "0123456789"

	m := model{
		viewportOurs:   viewport.New(5, 1),
		viewportResult: viewport.New(5, 1),
		viewportTheirs: viewport.New(5, 1),
	}
	for _, viewportModel := range []*viewport.Model{&m.viewportOurs, &m.viewportResult, &m.viewportTheirs} {
		viewportModel.SetContent(content)
	}

	m.scrollHorizontal(4)
	for _, viewportModel := range []*viewport.Model{&m.viewportOurs, &m.viewportResult, &m.viewportTheirs} {
		if got := viewportModel.View(); got != "45678" {
			t.Fatalf("View = %q, want 45678 after scrollHorizontal", got)
		}
	}

	m.scrollHorizontal(-2)
	for _, viewportModel := range []*viewport.Model{&m.viewportOurs, &m.viewportResult, &m.viewportTheirs} {
		if got := viewportModel.View(); got != "23456" {
			t.Fatalf("View = %q, want 23456 after scrollHorizontal left", got)
		}
	}
}

func TestToastAndKeySeqExpiry(t *testing.T) {
	m := model{
		toastMessage:   "Saved",
		toastSeq:       2,
		keySeq:         "g",
		keySeqTimeout:  4,
		viewportOurs:   viewport.New(1, 1),
		viewportResult: viewport.New(1, 1),
		viewportTheirs: viewport.New(1, 1),
	}

	updated, _ := m.Update(toastExpiredMsg{id: 1})
	updatedModel := updated.(model)
	if updatedModel.toastMessage == "" {
		t.Fatalf("toastMessage cleared for mismatched id")
	}

	updated, _ = updatedModel.Update(toastExpiredMsg{id: 2})
	updatedModel = updated.(model)
	if updatedModel.toastMessage != "" {
		t.Fatalf("toastMessage not cleared for matching id")
	}

	updatedModel.keySeq = "g"
	updated, _ = updatedModel.Update(keySeqExpiredMsg{id: 3})
	updatedModel = updated.(model)
	if updatedModel.keySeq == "" {
		t.Fatalf("keySeq cleared for mismatched id")
	}

	updated, _ = updatedModel.Update(keySeqExpiredMsg{id: 4})
	updatedModel = updated.(model)
	if updatedModel.keySeq != "" {
		t.Fatalf("keySeq not cleared for matching id")
	}
}

func TestWriteResolvedAllowsUnresolved(t *testing.T) {
	tmpDir := t.TempDir()
	mergedPath := filepath.Join(tmpDir, "merged.txt")
	if err := os.WriteFile(mergedPath, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	input := []byte("<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\n")
	doc, err := markers.Parse(input)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	state, err := engine.NewState(doc, 1)
	if err != nil {
		t.Fatalf("NewState error = %v", err)
	}

	m := model{
		state: state,
		opts:  cli.Options{MergedPath: mergedPath},
	}

	if err := m.writeResolved(); err != nil {
		t.Fatalf("writeResolved error = %v", err)
	}

	data, err := os.ReadFile(mergedPath)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if !bytes.Contains(data, []byte("<<<<<<<")) {
		t.Fatalf("expected unresolved markers to be written")
	}
}

func TestWriteResolvedCreatesBackup(t *testing.T) {
	tmpDir := t.TempDir()
	mergedPath := filepath.Join(tmpDir, "merged.txt")
	if err := os.WriteFile(mergedPath, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	doc := markers.Document{Segments: []markers.Segment{markers.TextSegment{Bytes: []byte("resolved\n")}}}
	state, err := engine.NewState(doc, 1)
	if err != nil {
		t.Fatalf("NewState error = %v", err)
	}

	m := model{
		state: state,
		opts:  cli.Options{MergedPath: mergedPath, Backup: true},
	}

	if err := m.writeResolved(); err != nil {
		t.Fatalf("writeResolved error = %v", err)
	}

	backupPath := mergedPath + ".ec.bak"
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("ReadFile backup error = %v", err)
	}
	if string(backup) != "original\n" {
		t.Fatalf("backup content = %q, want %q", string(backup), "original\\n")
	}
}
