package tui

import (
	"context"
	"fmt"
	"os"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/markers"
	"github.com/chojs23/ec/internal/mergeview"
)

type resolverDocumentState struct {
	session          *mergeview.Session
	doc              markers.Document
	manualResolved   map[int][]byte
	mergedLabels     []conflictLabels
	mergedLabelKnown []bool
}

func loadResolverDocumentState(ctx context.Context, opts cli.Options) (resolverDocumentState, error) {
	canonicalSession, err := mergeview.LoadCanonicalSession(ctx, opts)
	if err != nil {
		return resolverDocumentState{}, err
	}
	canonicalDoc := canonicalSession.Document()

	state := resolverDocumentState{
		session:          canonicalSession,
		doc:              canonicalDoc,
		manualResolved:   map[int][]byte{},
		mergedLabels:     make([]conflictLabels, len(canonicalDoc.Conflicts)),
		mergedLabelKnown: make([]bool, len(canonicalDoc.Conflicts)),
	}

	mergedBytes, err := os.ReadFile(opts.MergedPath)
	if err != nil {
		return state, nil
	}

	updated, manual, labels, known, err := applyMergedResolutions(canonicalDoc, mergedBytes)
	if err != nil {
		return resolverDocumentState{}, fmt.Errorf("apply merged resolutions: %w", err)
	}
	replayedSession, err := mergeview.SessionFromDocument(updated)
	if err != nil {
		return resolverDocumentState{}, err
	}

	state.session = replayedSession
	state.doc = updated
	state.manualResolved = manual
	state.mergedLabels = labels
	state.mergedLabelKnown = known
	return state, nil
}
