package tui

import (
	"context"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// Bubble Tea does not wait for loader commands at exit. Keep stage preparation
// alive until its temporary files are either adopted by a screen or removed.
type workspaceTasks struct {
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	closed   bool
	wg       sync.WaitGroup
	cleanups map[uint64]func()
}

func newWorkspaceTasks(ctx context.Context) *workspaceTasks {
	ctx, cancel := context.WithCancel(ctx)
	return &workspaceTasks{ctx: ctx, cancel: cancel, cleanups: make(map[uint64]func())}
}

func (t *workspaceTasks) command(ctx context.Context, load func() tea.Msg) tea.Cmd {
	return func() tea.Msg {
		t.mu.Lock()
		if t.closed || ctx.Err() != nil {
			t.mu.Unlock()
			return nil
		}
		// Register only when the command starts. An unexecuted command must not
		// leave Close waiting forever, and no Add may race with its Wait.
		t.wg.Add(1)
		t.mu.Unlock()
		defer t.wg.Done()
		return load()
	}
}

func (t *workspaceTasks) adopt(ctx context.Context, visitID uint64, cleanup func()) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || ctx.Err() != nil {
		return false
	}
	t.cleanups[visitID] = cleanup
	return true
}

func (t *workspaceTasks) release(visitID uint64) {
	t.mu.Lock()
	cleanup := t.cleanups[visitID]
	delete(t.cleanups, visitID)
	t.mu.Unlock()
	if cleanup != nil {
		cleanup()
	}
}

func (t *workspaceTasks) close() {
	t.mu.Lock()
	t.closed = true
	t.mu.Unlock()
	t.cancel()
	t.wg.Wait()
	t.mu.Lock()
	cleanups := t.cleanups
	t.cleanups = make(map[uint64]func())
	t.mu.Unlock()
	for _, cleanup := range cleanups {
		if cleanup != nil {
			cleanup()
		}
	}
}
