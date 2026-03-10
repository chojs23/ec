package tui

import (
	"context"
	"os"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/engine"
	"github.com/chojs23/ec/internal/markers"
	"github.com/chojs23/ec/internal/mergeview"
)

type resolverDocumentState struct {
	state            *engine.State
	doc              markers.Document
	boundaryText     [][]byte
	manualResolved   map[int][]byte
	mergedLabels     []conflictLabels
	mergedLabelKnown []bool
}

func loadResolverDocumentState(ctx context.Context, opts cli.Options) (resolverDocumentState, error) {
	canonicalDoc, err := mergeview.LoadCanonicalDocument(ctx, opts)
	if err != nil {
		return resolverDocumentState{}, err
	}
	runtimeState, err := engine.NewState(canonicalDoc)
	if err != nil {
		return resolverDocumentState{}, err
	}

	state := buildResolverDocumentState(runtimeState)

	mergedBytes, err := os.ReadFile(opts.MergedPath)
	if err != nil {
		return state, nil
	}

	if err := runtimeState.ImportMerged(mergedBytes); err != nil {
		return resolverDocumentState{}, err
	}
	return buildResolverDocumentState(runtimeState), nil
}

func buildResolverDocumentState(state *engine.State) resolverDocumentState {
	labels, known := state.MergedLabels()
	mergedLabels := make([]conflictLabels, len(labels))
	for i, label := range labels {
		mergedLabels[i] = conflictLabels{
			OursLabel:   label.OursLabel,
			BaseLabel:   label.BaseLabel,
			TheirsLabel: label.TheirsLabel,
		}
	}
	return resolverDocumentState{
		state:            state,
		doc:              state.Document(),
		boundaryText:     state.BoundaryText(),
		manualResolved:   state.ManualResolved(),
		mergedLabels:     mergedLabels,
		mergedLabelKnown: known,
	}
}
