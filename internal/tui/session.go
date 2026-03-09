package tui

import (
	"context"
	"fmt"
	"os"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/mergeview"
)

type resolverDocumentState struct {
	session          *mergeview.Session
	manualResolved   map[int][]byte
	mergedLabels     []conflictLabels
	mergedLabelKnown []bool
}

func loadResolverDocumentState(ctx context.Context, opts cli.Options) (resolverDocumentState, error) {
	canonicalSession, err := mergeview.LoadCanonicalSession(ctx, opts)
	if err != nil {
		return resolverDocumentState{}, err
	}
	state := resolverDocumentState{
		session:          canonicalSession,
		manualResolved:   map[int][]byte{},
		mergedLabels:     make([]conflictLabels, len(canonicalSession.Conflicts)),
		mergedLabelKnown: make([]bool, len(canonicalSession.Conflicts)),
	}

	mergedBytes, err := os.ReadFile(opts.MergedPath)
	if err != nil {
		return state, nil
	}

	replayedSession, manual, labels, known, err := mergeview.ReplayMergedResult(canonicalSession, mergedBytes)
	if err != nil {
		return resolverDocumentState{}, fmt.Errorf("apply merged resolutions: %w", err)
	}

	state.session = replayedSession
	state.manualResolved = manual
	state.mergedLabels = labels
	state.mergedLabelKnown = known
	return state, nil
}
