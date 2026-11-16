package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chojs23/ec/internal/markers"
)

func TestMergeToolQuitChecksSavedOutput(t *testing.T) {
	for _, progress := range []string{"unchanged", "applied", "partially saved", "fully saved"} {
		for _, key := range []tea.KeyMsg{
			{Type: tea.KeyRunes, Runes: []rune{'q'}},
			{Type: tea.KeyCtrlC},
		} {
			t.Run(progress+"/"+key.String(), func(t *testing.T) {
				m := newModelForDoc(t, parseMultiConflictDoc(t))
				m.opts.MergedPath = filepath.Join(t.TempDir(), "merged.txt")
				m.opts.ConfirmIncompleteQuit = true
				if err := os.WriteFile(m.opts.MergedPath, m.state.RenderMerged(), 0o644); err != nil {
					t.Fatal(err)
				}
				switch progress {
				case "applied", "fully saved":
					if err := m.applyAll(markers.ResolutionOurs); err != nil {
						t.Fatal(err)
					}
				case "partially saved":
					if err := m.applyResolution(markers.ResolutionOurs); err != nil {
						t.Fatal(err)
					}
				}
				if progress == "partially saved" || progress == "fully saved" {
					if err := m.writeResolved(); err != nil {
						t.Fatal(err)
					}
				}

				updated, cmd := m.Update(key)
				result := updated.(model)
				wantConfirmation := progress != "fully saved"
				if result.confirmingQuit != wantConfirmation || result.quitting == wantConfirmation {
					t.Fatalf("confirmingQuit = %v, quitting = %v, want confirmation = %v", result.confirmingQuit, result.quitting, wantConfirmation)
				}
				if result.err != nil {
					t.Fatalf("quit error = %v", result.err)
				}
				if wantConfirmation {
					if cmd != nil {
						t.Fatal("incomplete output must wait for confirmation")
					}
				} else if cmd == nil {
					t.Fatal("resolved output must quit")
				} else if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Fatal("resolved output must issue QuitMsg")
				}
			})
		}
	}
}

func TestMergeToolIncompleteQuitConfirmation(t *testing.T) {
	for _, choice := range []tea.KeyMsg{
		{Type: tea.KeyEnter},
		{Type: tea.KeyEsc},
		{Type: tea.KeyRunes, Runes: []rune{'n'}},
		{Type: tea.KeyRunes, Runes: []rune{'y'}},
	} {
		t.Run(choice.String(), func(t *testing.T) {
			m := newModelForDoc(t, parseMultiConflictDoc(t))
			m.opts.MergedPath = filepath.Join(t.TempDir(), "merged.txt")
			m.opts.ConfirmIncompleteQuit = true
			if err := os.WriteFile(m.opts.MergedPath, m.state.RenderMerged(), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := m.applyResolution(markers.ResolutionOurs); err != nil {
				t.Fatal(err)
			}
			if err := m.writeResolved(); err != nil {
				t.Fatal(err)
			}
			saved, err := os.ReadFile(m.opts.MergedPath)
			if err != nil {
				t.Fatal(err)
			}

			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			m = updated.(model)
			for _, text := range []string{"unresolved conflicts", "discard", "continue editing", "abort and quit"} {
				if !strings.Contains(m.View(), text) {
					t.Fatalf("confirmation must show %q: %s", text, m.View())
				}
			}

			updated, cmd := m.Update(choice)
			result := updated.(model)
			wantAbort := choice.String() == "y"
			if result.confirmingQuit || result.quitting != wantAbort || result.err != nil {
				t.Fatalf("confirmingQuit = %v, quitting = %v, err = %v, want abort = %v", result.confirmingQuit, result.quitting, result.err, wantAbort)
			}
			if wantAbort {
				if cmd == nil {
					t.Fatal("confirmed abort must quit")
				}
				if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Fatal("confirmed abort must issue QuitMsg")
				}
			} else if cmd != nil {
				t.Fatal("continuing must return to the resolver")
			}
			got, err := os.ReadFile(m.opts.MergedPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, saved) {
				t.Fatal("quit confirmation must leave saved progress unchanged")
			}
		})
	}
}
