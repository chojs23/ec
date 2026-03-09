package engine

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/chojs23/ec/internal/mergeview"
)

// BaseDisplayStrategy defines how base chunks are provided for conflicts.
type BaseDisplayStrategy string

const (
	BaseDisplayExact BaseDisplayStrategy = "exact"
)

func ValidateBaseCompletenessSession(session *mergeview.Session) error {
	for i, ref := range session.Conflicts {
		block, ok := session.Segments[ref.SegmentIndex].(mergeview.ConflictBlock)
		if !ok {
			return fmt.Errorf("internal: conflict %d is not a ConflictBlock", i)
		}
		if len(block.Base) == 0 && block.Labels.BaseLabel == "" {
			return fmt.Errorf("conflict %d is missing base chunk (base display strategy requires exact base for all conflicts)", i)
		}
	}
	return nil
}

// OpenBaseFile opens the base file in $PAGER or $EDITOR for viewing.
// If neither is set, defaults to 'less' for PAGER.
// The command runs interactively with stdio connected.
func OpenBaseFile(basePath string) error {
	if basePath == "" {
		return fmt.Errorf("base file path is empty")
	}

	// Check file exists
	if _, err := os.Stat(basePath); err != nil {
		return fmt.Errorf("cannot access base file: %w", err)
	}

	// Try PAGER first, then EDITOR, then default to 'less'
	viewer := os.Getenv("PAGER")
	if viewer == "" {
		viewer = os.Getenv("EDITOR")
	}
	if viewer == "" {
		viewer = "less"
	}

	cmd := exec.Command(viewer, basePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open %s with %s: %w", basePath, viewer, err)
	}

	return nil
}
