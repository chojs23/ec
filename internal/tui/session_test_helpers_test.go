package tui

import (
	"testing"

	"github.com/chojs23/ec/internal/engine"
	"github.com/chojs23/ec/internal/mergeview"
)

func parseSessionText(t *testing.T, data []byte) *mergeview.Session {
	t.Helper()
	session, err := mergeview.ParseSession(data)
	if err != nil {
		t.Fatalf("ParseSession error: %v", err)
	}
	return session
}

func stateFromFixture(t *testing.T, session *mergeview.Session) *engine.State {
	t.Helper()
	state, err := engine.NewStateFromSession(session)
	if err != nil {
		t.Fatalf("NewStateFromSession error: %v", err)
	}
	return state
}

func sessionFromDoc(t *testing.T, session *mergeview.Session) *mergeview.Session {
	t.Helper()
	return session
}
