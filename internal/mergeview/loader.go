package mergeview

import (
	"context"
	"fmt"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/gitmerge"
	"github.com/chojs23/ec/internal/markers"
)

// LoadCanonicalDocument builds the canonical conflict document from the explicit
// base/local/remote inputs. This keeps conflict structure anchored to the stage
// files instead of the merged working copy.
func LoadCanonicalDocument(ctx context.Context, opts cli.Options) (markers.Document, error) {
	diff3Bytes, err := gitmerge.MergeFileDiff3(ctx, opts.LocalPath, opts.BasePath, opts.RemotePath)
	if err != nil {
		return markers.Document{}, fmt.Errorf("generate diff3 view: %w", err)
	}

	doc, err := markers.Parse(diff3Bytes)
	if err != nil {
		return markers.Document{}, fmt.Errorf("parse diff3 view: %w", err)
	}

	return doc, nil
}
