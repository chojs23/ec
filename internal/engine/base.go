package engine

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/chojs23/easy-conflict/internal/markers"
)

// BaseDisplayStrategy defines how base chunks are provided for conflicts.
type BaseDisplayStrategy string

const (
	// BaseDisplayExact means every conflict must have an exact base chunk
	// from diff3 output. This is the only supported strategy in MVP.
	BaseDisplayExact BaseDisplayStrategy = "exact"
)

// ValidateBaseCompleteness checks that every conflict in the document has a base chunk.
// Returns error if any conflict is missing its base section.
func ValidateBaseCompleteness(doc markers.Document) error {
	for i, ref := range doc.Conflicts {
		seg, ok := doc.Segments[ref.SegmentIndex].(markers.ConflictSegment)
		if !ok {
			return fmt.Errorf("internal: conflict %d is not a ConflictSegment", i)
		}
		if len(seg.Base) == 0 {
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
